package generator

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/openapi"
)

var errUnknownCredential = errors.New("credential not found")

// stubAuthResolution points resolveStoredFn at a canned credential for the
// duration of the test, so no test touches the platform keychain.
func stubAuthResolution(t *testing.T, name, header, value string, resolveErr error) {
	t.Helper()
	orig := resolveStoredFn
	resolveStoredFn = func(n string) (auth.CredentialHeader, error) {
		require.Equal(t, name, n)
		if resolveErr != nil {
			return auth.CredentialHeader{}, resolveErr
		}
		return auth.CredentialHeader{Name: header, Value: value, Shielded: "MASKED"}, nil
	}
	t.Cleanup(func() { resolveStoredFn = orig })
}

func newTestGenerator(t *testing.T, baseURL string) *Generator {
	t.Helper()
	gen, err := NewGenerator(Config{
		AppName: "test",
		BaseURL: baseURL,
	})
	require.NoError(t, err)
	return gen
}

// The generated operation command registers the runtime --auth flag and
// its execution attaches the resolved bearer credential.
func TestGeneratedCommand_AuthAttachesBearerHeader(t *testing.T) {
	var gotAuth, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "authsvc", "Authorization", "Bearer gen-token-1", nil)
	gen := newTestGenerator(t, server.URL)

	op := &openapi.Operation{ID: "listTasks", Path: "/tasks", Method: "GET"}
	cmd := &cobra.Command{
		Use:  "list-tasks",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)
	// Mirror buildOperationCommand's runtime flag stamping.
	require.Nil(t, cmd.Flags().Lookup(AuthFlagName))
	cmd.Flags().String(AuthFlagName, "", "Attach a stored credential by name (see 'auth add')")
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AuthFlagAnnotation] = "true"

	cmd.SetArgs([]string{"--auth", "authsvc"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	require.Equal(t, "Bearer gen-token-1", gotAuth)
	require.Equal(t, "", gotQuery, "runtime --auth must never leak as a query parameter")
}

// api-key credentials attach as X-API-Key through the generated path.
func TestGeneratedCommand_AuthAttachesAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "k", "X-API-Key", "sk-gen-1", nil)
	gen := newTestGenerator(t, server.URL)

	op := &openapi.Operation{ID: "listTasks", Path: "/tasks", Method: "GET"}
	cmd := &cobra.Command{
		Use:  "list-tasks",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)
	cmd.Flags().String(AuthFlagName, "", "Attach a stored credential by name (see 'auth add')")
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AuthFlagAnnotation] = "true"

	cmd.SetArgs([]string{"--auth", "k"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	require.Equal(t, "sk-gen-1", gotAPIKey)
}

// An unknown credential name fails the command; no request is sent.
func TestGeneratedCommand_AuthUnknownCredentialFails(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "ghost", "", "", errUnknownCredential)
	gen := newTestGenerator(t, server.URL)

	op := &openapi.Operation{ID: "listTasks", Path: "/tasks", Method: "GET"}
	cmd := &cobra.Command{
		Use:  "list-tasks",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)
	cmd.Flags().String(AuthFlagName, "", "Attach a stored credential by name (see 'auth add')")
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AuthFlagAnnotation] = "true"

	cmd.SetArgs([]string{"--auth", "ghost"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve --auth")
	require.False(t, called)
}

// buildOperationCommand registers the runtime --auth flag, and skips it
// when the spec already declares a parameter that normalizes to "auth"
// (the spec's flag must win and must not be redefined).
func TestBuildOperationCommand_AuthFlagRegistration(t *testing.T) {
	gen := newTestGenerator(t, "http://example.com")

	// Without a spec "auth" parameter: runtime flag is registered.
	op := &openapi.Operation{ID: "listTasks", Path: "/tasks", Method: "GET"}
	cmd := gen.buildOperationCommand(op, map[string]bool{})
	require.NotNil(t, cmd.Flags().Lookup(AuthFlagName))
	require.Equal(t, "true", cmd.Annotations[AuthFlagAnnotation])

	// With a spec parameter named "auth": the spec's flag wins, no
	// runtime stamp.
	opAuth := &openapi.Operation{
		ID:         "listTasks",
		Path:       "/tasks",
		Method:     "GET",
		Parameters: []openapi.Parameter{{Name: "auth", In: "query", Type: "string"}},
	}
	cmdAuth := gen.buildOperationCommand(opAuth, map[string]bool{})
	authFlag := cmdAuth.Flags().Lookup("auth")
	require.NotNil(t, authFlag)
	require.Equal(t, "", cmdAuth.Annotations[AuthFlagAnnotation],
		"a spec parameter named auth must not be stamped as the runtime credential flag")
}
