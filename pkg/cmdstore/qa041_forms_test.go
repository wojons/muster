package cmdstore

import (
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// QA-MUSTER-041: the RELOADED command path must send form-encoded bodies.
//
// `openapi-cli generate` persists command metadata; the next process rebuilds
// every command from that metadata. Pre-fix the metadata carried no body media
// type and the loader only knew JSON, so `openapi-cli post-customers --email …`
// sent Content-Length: 0 (611/612 Stripe operations).
// =============================================================================

func qa041LoaderServer(t *testing.T) (*httptest.Server, *string, *string) {
	t.Helper()
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cus_1"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotCT, &gotBody
}

func qa041FormMeta(baseURL string) CommandMetadata {
	return CommandMetadata{
		Name:        "post-customers",
		Use:         "post-customers",
		Method:      http.MethodPost,
		Path:        "/v1/customers",
		BaseURL:     baseURL,
		ContentType: "application/x-www-form-urlencoded",
		Flags: []FlagMetadata{
			{Name: "email", Type: "string", Kind: FlagKindBody, JSONName: "email"},
			{Name: "name", Type: "string", Kind: FlagKindBody, JSONName: "name"},
			{Name: "balance", Type: "int", Kind: FlagKindBody, JSONName: "balance"},
			{Name: "body", Type: "string", Kind: FlagKindRawBody},
		},
	}
}

func TestQA041_Loader_FormBody_SendsURLEncoded(t *testing.T) {
	srv, gotCT, gotBody := qa041LoaderServer(t)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{qa041FormMeta(srv.URL)}
	cmds, err := NewLoader(cs, srv.URL).LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmd := cmds[0]
	buf := new(testWriter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("email", "foo@example.com"))
	require.NoError(t, cmd.Flags().Set("name", "Foo Bar"))
	require.NoError(t, cmd.Flags().Set("balance", "500"))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, "application/x-www-form-urlencoded", *gotCT)
	require.NotEmpty(t, *gotBody, "the reloaded command must not send Content-Length: 0")

	values, err := neturl.ParseQuery(*gotBody)
	require.NoError(t, err)
	assert.Equal(t, "foo@example.com", values.Get("email"))
	assert.Equal(t, "Foo Bar", values.Get("name"))
	assert.Equal(t, "500", values.Get("balance"))
}

func TestQA041_Loader_FormBody_RawBodyVerbatim(t *testing.T) {
	srv, gotCT, gotBody := qa041LoaderServer(t)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{qa041FormMeta(srv.URL)}
	cmds, err := NewLoader(cs, srv.URL).LoadCommands()
	require.NoError(t, err)

	cmd := cmds[0]
	buf := new(testWriter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	raw := "email=raw%40example.com&metadata[owner]=qa"
	require.NoError(t, cmd.Flags().Set("body", raw))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, raw, *gotBody, "--body is sent verbatim, not re-encoded")
	assert.Equal(t, "application/x-www-form-urlencoded", *gotCT)
}

func TestQA041_Loader_FormBody_RawBodyRejectsBadInput(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{qa041FormMeta("http://127.0.0.1:1")}
	cmds, err := NewLoader(cs, "http://127.0.0.1:1").LoadCommands()
	require.NoError(t, err)

	cmd := cmds[0]
	buf := new(testWriter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("body", `{"email":"foo@example.com"}`))
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "urlencoded")
}

// A store written before the media type was recorded keeps JSON behavior.
func TestQA041_Loader_EmptyContentTypeStaysJSON(t *testing.T) {
	srv, gotCT, gotBody := qa041LoaderServer(t)

	meta := qa041FormMeta(srv.URL)
	meta.ContentType = ""
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{meta}
	cmds, err := NewLoader(cs, srv.URL).LoadCommands()
	require.NoError(t, err)

	cmd := cmds[0]
	buf := new(testWriter)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("email", "foo@example.com"))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, "application/json", *gotCT)
	assert.JSONEq(t, `{"email":"foo@example.com"}`, *gotBody)
}

// A legacy store can carry a flag named like one of the CLI's own flags
// (--header). Re-registering it panicked the whole CLI at startup; now it is
// skipped with a warning so every other command still loads (QA-MUSTER-041).
func TestQA041_Loader_DuplicateCLIFlagDoesNotPanic(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:        "post-quotes",
			Use:         "post-quotes",
			Method:      http.MethodPost,
			Path:        "/v1/quotes",
			ContentType: "application/x-www-form-urlencoded",
			Flags: []FlagMetadata{
				{Name: "header", Type: "string", Kind: FlagKindBody, JSONName: "header"},
				{Name: "footer", Type: "string", Kind: FlagKindBody, JSONName: "footer"},
			},
		},
	}

	cmds, err := NewLoader(cs, "http://127.0.0.1:1").LoadCommands()
	require.NoError(t, err, "a duplicate CLI flag name must not abort command loading")
	require.Len(t, cmds, 1)

	// The CLI's own -H/--header survives; the colliding property is dropped.
	require.NotNil(t, cmds[0].Flag("header"))
	require.Equal(t, "H", cmds[0].Flag("header").Shorthand)
	require.NotNil(t, cmds[0].Flag("footer"), "non-colliding body flags still load")
}
