package generator

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/request"
)

// DF-039 regression: generated typed commands print the response body in
// table and JSON output, and real harness APIs return session-scoped API
// keys (cs_XX_…) that a script then echoes into logs. Formatted output must
// mask credential-shaped values by default; the existing --raw flag is the
// explicit escape hatch (it already dumps the body untouched).

const df039SessionKey = "cs_sk_9f2a1c8d4e5b6a7f"

// df039SessionServer returns the create-session response shape: a session
// credential next to ordinary fields.
func df039SessionServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"created","api_key":"` + df039SessionKey + `","owner":"lexi"}`))
	})
	return httptest.NewServer(handler)
}

// TestExecuteAndPrint_TableFormat_MasksSessionKey proves the table column
// dump masks the cs_XX_ credential by default.
func TestExecuteAndPrint_TableFormat_MasksSessionKey(t *testing.T) {
	ts := df039SessionServer()
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/sessions", "POST")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "api_key")
	require.Contains(t, output, "cs_sk_***")
	require.NotContains(t, output, df039SessionKey)
}

// TestExecuteAndPrint_JSONFormat_MasksSessionKey proves the JSON output
// masks the credential by default too.
func TestExecuteAndPrint_JSONFormat_MasksSessionKey(t *testing.T) {
	ts := df039SessionServer()
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/sessions", "POST")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "json")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, `"api_key": "cs_sk_***"`)
	require.NotContains(t, output, df039SessionKey)
}

// TestExecuteAndPrint_RawFlag_DoesNotMask proves --raw is the escape hatch:
// the body is dumped untouched, key included.
func TestExecuteAndPrint_RawFlag_DoesNotMask(t *testing.T) {
	ts := df039SessionServer()
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/sessions", "POST")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	require.NoError(t, cmd.Flags().Set("raw", "true"))

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	require.Contains(t, buf.String(), df039SessionKey)
}

// TestMaskCredentialString covers the value-level rule: full values,
// embedded tokens, non-credentials, and non-matching cs_ shapes are
// handled.
func TestMaskCredentialString(t *testing.T) {
	require.Equal(t, "cs_sk_***", maskCredentialString(df039SessionKey))
	require.Equal(t, "cs_sk_***", maskCredentialString("cs_sk_"))
	require.Equal(t, "token cs_sk_*** created", maskCredentialString("token cs_sk_abc123 created"))
	require.Equal(t, "Bearer ***", maskCredentialString("Bearer abc123"))
	// Scheme casing and original whitespace are preserved.
	require.Equal(t, "bearer	***", maskCredentialString("bearer	abc123"))
	require.Equal(t, "not-a-credential", maskCredentialString("not-a-credential"))
	// Other cs_ shapes that do not match the cs_[a-z]{2}_ pattern stay.
	require.Equal(t, "cs_custom_value", maskCredentialString("cs_custom_value"))
}

// TestMaskCredentialValues_Nested proves masking walks maps and slices, so
// a key buried in a nested object (rendered as compact JSON in tables) is
// masked too.
func TestMaskCredentialValues_Nested(t *testing.T) {
	in := map[string]interface{}{
		"session": map[string]interface{}{
			"keys": []interface{}{df039SessionKey, "plain"},
		},
	}
	out := maskCredentialValues(in)
	nested := out.(map[string]interface{})["session"].(map[string]interface{})["keys"].([]interface{})
	require.Equal(t, "cs_sk_***", nested[0])
	require.Equal(t, "plain", nested[1])
}
