package cmdstore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// DF-043 regression: the reloaded command path (executeAndReadResponse)
// renders persisted command responses in table/json/yaml without masking
// credential-shaped values, while the generated path (pkg/generator,
// DF-039) masks them. The pkg/cmdstore ↔ pkg/generator import cycle
// prevents sharing the helper directly (same reason AuthFlagName is
// mirrored and pinned by internal/builtin/authflag_parity_test.go), so
// cmdstore mirrors the rule and this suite pins the mirrored behavior:
// formatted output masks cs_[a-z]{2}_ session keys and "Bearer <token>"
// values; --raw remains the explicit escape hatch that dumps the body
// untouched, byte for byte.

const df043SessionKey = "cs_sk_9f2a1c8d4e5b6a7f"

// df043Body is the JSON the test server returns: a session credential next
// to ordinary fields.
const df043Body = `{"status":"created","api_key":"` + df043SessionKey + `","owner":"lexi"}`

// df043Server returns the create-session response shape.
func df043Server() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(df043Body))
	})
	return httptest.NewServer(handler)
}

// df043Command loads one persisted GET command through the real reload path
// and returns the command with its output wired to a fresh buffer.
func df043Command(t *testing.T, srvURL string) (*cobra.Command, *testWriter) {
	t.Helper()
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "get-session",
			Use:    "get-session",
			Method: "GET",
			Path:   "/sessions",
		},
	}
	l := NewLoader(cs, srvURL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)
	return cmds[0], buf
}

// TestLoader_CreateRunHandler_TableFormat_MasksSessionKey proves the default
// table rendering masks the cs_XX_ credential.
func TestLoader_CreateRunHandler_TableFormat_MasksSessionKey(t *testing.T) {
	srv := df043Server()
	defer srv.Close()

	cmd, buf := df043Command(t, srv.URL)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "api_key")
	require.Contains(t, output, "cs_sk_***")
	require.NotContains(t, output, df043SessionKey)
}

// TestLoader_CreateRunHandler_JSONFormat_MasksSessionKey proves the JSON
// rendering masks the credential too.
func TestLoader_CreateRunHandler_JSONFormat_MasksSessionKey(t *testing.T) {
	srv := df043Server()
	defer srv.Close()

	cmd, buf := df043Command(t, srv.URL)
	require.NoError(t, cmd.Flags().Set("output", "json"))

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, `"api_key": "cs_sk_***"`)
	require.NotContains(t, output, df043SessionKey)
}

// TestLoader_CreateRunHandler_YAMLFormat_MasksSessionKey proves the YAML
// rendering masks the credential too.
func TestLoader_CreateRunHandler_YAMLFormat_MasksSessionKey(t *testing.T) {
	srv := df043Server()
	defer srv.Close()

	cmd, buf := df043Command(t, srv.URL)
	require.NoError(t, cmd.Flags().Set("output", "yaml"))

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "cs_sk_***")
	require.NotContains(t, output, df043SessionKey)
}

// TestLoader_CreateRunHandler_RawFlag_DoesNotMask proves --raw is the escape
// hatch: the body is dumped untouched, byte for byte, key included.
func TestLoader_CreateRunHandler_RawFlag_DoesNotMask(t *testing.T) {
	srv := df043Server()
	defer srv.Close()

	cmd, buf := df043Command(t, srv.URL)
	require.NoError(t, cmd.Flags().Set("raw", "true"))

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	// Byte-for-byte: the raw dump must be the served body exactly, plus
	// only the loader's line-print newline — no masking, no reformatting.
	require.Equal(t, df043Body+"\n", buf.String())
}

// TestLoader_CreateRunHandler_BearerValue_Masked proves the Bearer rule also
// applies on the reloaded path.
func TestLoader_CreateRunHandler_BearerValue_Masked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization":"Bearer abc123","kind":"header"}`))
	}))
	defer srv.Close()

	cmd, buf := df043Command(t, srv.URL)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "Bearer ***")
	require.NotContains(t, output, "abc123")
}
