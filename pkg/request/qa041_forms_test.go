package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// QA-MUSTER-041: form-encoded bodies.
//
// FormValuesFromMap is the single encoder shared by the CLI generator, the
// persisted-command loader and the MCP executors, so its rules are pinned here:
// scalars once, slices repeated, nested maps in bracket notation (what Stripe /
// Rails / PHP consume).
// =============================================================================

func TestQA041_FormValuesFromMap_EncodingRules(t *testing.T) {
	values := FormValuesFromMap(map[string]interface{}{
		"name":     "Foo Bar",
		"quantity": 3,
		"active":   true,
		"items":    []interface{}{"a", "b"},
		"tags":     []string{"x"},
		"metadata": map[string]interface{}{"owner": "qa", "team": "muster"},
		"skipped":  nil,
	})

	assert.Equal(t, "Foo Bar", values.Get("name"))
	assert.Equal(t, "3", values.Get("quantity"))
	assert.Equal(t, "true", values.Get("active"))
	assert.Equal(t, []string{"a", "b"}, values["items"], "arrays repeat the key")
	assert.Equal(t, []string{"x"}, values["tags"])
	assert.Equal(t, "qa", values.Get("metadata[owner]"), "nested maps use bracket notation")
	assert.Equal(t, "muster", values.Get("metadata[team]"))
	_, present := values["skipped"]
	assert.False(t, present, "a nil value is omitted rather than encoding \"<nil>\"")

	// Deterministic output: Encode() sorts keys, nested maps are pre-sorted.
	assert.Equal(t,
		"active=true&items=a&items=b&metadata%5Bowner%5D=qa&metadata%5Bteam%5D=muster&name=Foo+Bar&quantity=3&tags=x",
		values.Encode())
}

func TestQA041_FormValuesFromMap_Empty(t *testing.T) {
	assert.Empty(t, FormValuesFromMap(nil).Encode())
	assert.Empty(t, FormValuesFromMap(map[string]interface{}{}).Encode())
}

func TestQA041_ParseFormBody_Validation(t *testing.T) {
	values, err := ParseFormBody("name=Raw+Body&metadata[owner]=qa")
	require.NoError(t, err)
	assert.Equal(t, "Raw Body", values.Get("name"))
	assert.Equal(t, "qa", values.Get("metadata[owner]"))

	for _, raw := range []string{"", "   ", ";;;"} {
		_, err := ParseFormBody(raw)
		assert.Error(t, err, "an empty/unparseable form body must be rejected, not sent as Content-Length: 0")
	}
}

func TestQA041_Builder_FormBody_SendsContentType(t *testing.T) {
	var gotBody, gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	builder := NewBuilder(ts.URL, "/v1/customers", http.MethodPost)
	builder.FormBody(neturl.Values{"name": {"Foo"}, "email": {"foo@example.com"}})
	resp, err := builder.Execute(context.Background())
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, ContentTypeFormEncoded, gotCT)
	assert.Equal(t, "email=foo%40example.com&name=Foo", gotBody)
}

// Build() is used by the MCP executors, which never call Execute(); the media
// type must still reach the wire there.
func TestQA041_Builder_Build_AppliesContentType(t *testing.T) {
	builder := NewBuilder("https://api.example.com", "/v1/customers", http.MethodPost)
	builder.FormBody(neturl.Values{"name": {"Foo"}})
	req, err := builder.Build()
	require.NoError(t, err)
	assert.Equal(t, ContentTypeFormEncoded, req.Header.Get("Content-Type"))
}

// A multipart operation whose text fields were supplied but no file must still
// send those fields (pre-fix: no file → no body at all).
func TestQA041_Builder_MultipartFieldsWithoutFile(t *testing.T) {
	var gotCT, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	builder := NewBuilder(ts.URL, "/documents", http.MethodPost)
	builder.WithContentType(ContentTypeMultipart)
	builder.WithMultipartField("title", "Quarterly report")
	resp, err := builder.Execute(context.Background())
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Contains(t, gotCT, ContentTypeMultipart)
	assert.Contains(t, gotBody, `name="title"`)
	assert.Contains(t, gotBody, "Quarterly report")
}
