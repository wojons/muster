package cmdstore

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/auth"
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

func newQuietCmd(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
}

// --- run-handler --auth attachment ---

func TestLoaderRunHandler_AuthAttachesBearerHeader(t *testing.T) {
	var gotAuth, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "authsvc", "Authorization", "Bearer test-token-123", nil)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmds[0].SetArgs([]string{"--auth", "authsvc"})
	newQuietCmd(t, cmds[0])
	require.NoError(t, cmds[0].Execute())

	require.Equal(t, "Bearer test-token-123", gotAuth)
	require.Equal(t, "", gotAPIKey)
}

func TestLoaderRunHandler_AuthAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "k", "X-API-Key", "sk-secret-1", nil)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmds[0].SetArgs([]string{"--auth", "k"})
	newQuietCmd(t, cmds[0])
	require.NoError(t, cmds[0].Execute())

	require.Equal(t, "sk-secret-1", gotAPIKey)
}

// An explicit --header with the same name overrides the stored credential.
func TestLoaderRunHandler_ExplicitHeaderWinsOverAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "authsvc", "Authorization", "Bearer stored-token", nil)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmds[0].SetArgs([]string{"--auth", "authsvc", "--header", "Authorization: Bearer manual"})
	newQuietCmd(t, cmds[0])
	require.NoError(t, cmds[0].Execute())

	require.Equal(t, "Bearer manual", gotAuth)
}

// An unknown credential name fails the command with the actionable error
// and never sends the request.
func TestLoaderRunHandler_AuthUnknownCredentialFails(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "ghost", "", "", errUnknownCredential)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmds[0].SetArgs([]string{"--auth", "ghost"})
	newQuietCmd(t, cmds[0])
	err = cmds[0].Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve --auth")
	require.False(t, called)
}

// --- flag registration + metadata routing ---

// The reloaded command registers --auth locally, and a persisted
// FlagKindAuth flag (the generator-stamped runtime flag) is consumed by the
// run handler — never forwarded as an ?auth= query parameter.
func TestLoader_AuthFlagRegisteredAndNotAQueryParam(t *testing.T) {
	var gotQuery, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubAuthResolution(t, "authsvc", "Authorization", "Bearer test-token-123", nil)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
		Flags: []FlagMetadata{
			{Name: "auth", Type: "string", Kind: FlagKindAuth, Description: "runtime flag"},
		},
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	require.NotNil(t, cmds[0].Flag("auth"), "--auth must be registered on reloaded commands")

	cmds[0].SetArgs([]string{"--auth", "authsvc"})
	newQuietCmd(t, cmds[0])
	require.NoError(t, cmds[0].Execute())

	require.Equal(t, "", gotQuery, "runtime --auth must never leak as a query parameter")
	require.Equal(t, "Bearer test-token-123", gotAuth)
}

// A spec parameter genuinely named "auth" (persisted WITHOUT
// FlagKindAuth) must still reach the server as a query parameter.
func TestLoader_SpecAuthParamStillForwardsAsQuery(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer server.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{
		Name:    "list-tasks",
		Use:     "list-tasks",
		Short:   "List tasks",
		Method:  "GET",
		Path:    "/tasks",
		BaseURL: server.URL,
		Flags: []FlagMetadata{
			{Name: "auth", Type: "string", Description: "spec query parameter"},
		},
	}}
	l := NewLoader(cs, server.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmds[0].SetArgs([]string{"--auth", "specvalue"})
	newQuietCmd(t, cmds[0])
	require.NoError(t, cmds[0].Execute())
	require.Equal(t, "auth=specvalue", gotQuery)
}
