package cmdstore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// DF-015: a persisted command whose base URL is RELATIVE (stored by an
// older generate run, e.g. from a local-file spec declaring
// `servers: [{url: /api/v3}]`) or EMPTY must fail with the actionable
// --base-url / MUSTER_BASE_URL hint instead of the bare
// `unsupported protocol scheme ""` http error.
func TestLoader_CreateRunHandler_RelativeStoredBaseURL_ActionableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request may be sent when the base URL is relative")
	}))
	defer srv.Close()
	t.Setenv("MUSTER_BASE_URL", "")

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{Name: "get-pet", Use: "get-pet", Method: "GET", Path: "/pet/{petId}", Args: []string{"petId"}, BaseURL: "/api/v3"},
	}
	l := NewLoader(cs, "")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	err = cmds[0].RunE(cmds[0], []string{"1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "relative server URL")
	require.Contains(t, err.Error(), "--base-url")
	require.Contains(t, err.Error(), "MUSTER_BASE_URL")
	require.NotContains(t, err.Error(), "unsupported protocol scheme")
}

func TestLoader_CreateRunHandler_EmptyBaseURL_ActionableError(t *testing.T) {
	t.Setenv("MUSTER_BASE_URL", "")

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{Name: "list-pets", Use: "list-pets", Method: "GET", Path: "/pets"},
	}
	l := NewLoader(cs, "")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	err = cmds[0].RunE(cmds[0], []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--base-url")
	require.Contains(t, err.Error(), "MUSTER_BASE_URL")
	require.NotContains(t, err.Error(), "unsupported protocol scheme")
}

// TestLoader_CreateRunHandler_BaseURLOverrideRescuesRelativeStoredBase
// proves the escape hatches still work: an explicit --base-url override
// replaces a relative stored base entirely and the request succeeds.
func TestLoader_CreateRunHandler_BaseURLOverrideRescuesRelativeStoredBase(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()
	t.Setenv("MUSTER_BASE_URL", "")

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{Name: "get-pet", Use: "get-pet", Method: "GET", Path: "/pet/{petId}", Args: []string{"petId"}, BaseURL: "/api/v3"},
	}
	l := NewLoader(cs, "")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	require.NoError(t, cmds[0].Flags().Set("base-url", srv.URL))
	require.NoError(t, cmds[0].RunE(cmds[0], []string{"1"}))
	require.Equal(t, "/pet/1", gotPath)
}
