package cmdstore

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// DF-019 regression: a RELOADED persisted command (reconstructCommand path)
// must behave like a freshly generated one — runtime/HTTP errors print
// without the full usage block, flag/arg errors still show usage.

// df019Meta is a persisted GET command with one positional arg.
func df019Meta() CommandMetadata {
	return CommandMetadata{
		Name:   "get-pet-by-id",
		Use:    "get-pet-by-id <petId>",
		Short:  "Find pet by ID",
		Method: "GET",
		Path:   "/pets/{petId}",
		Args:   []string{"petId"},
	}
}

func df019LoadedCmd(t *testing.T, baseURL string) *cobra.Command {
	t.Helper()
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{df019Meta()}
	l := NewLoader(cs, baseURL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	return cmds[0]
}

// TestLoadedCommand_RuntimeError_SuppressesUsage proves criterion 1 on the
// reload path: an HTTP 404 prints the error WITHOUT the usage block.
func TestLoadedCommand_RuntimeError_SuppressesUsage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`Pet not found`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	cmd := df019LoadedCmd(t, ts.URL)

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"99999999"})

	execErr := cmd.Execute()
	require.Error(t, execErr)
	require.Contains(t, execErr.Error(), "HTTP error")

	combined := out.String() + errOut.String()
	require.NotContains(t, combined, "Usage:")
	require.NotContains(t, combined, "Flags:")
}

// TestLoadedCommand_FlagError_ShowsUsage proves criterion 2 on the reload
// path: an unknown flag still prints the usage block.
func TestLoadedCommand_FlagError_ShowsUsage(t *testing.T) {
	cmd := df019LoadedCmd(t, "http://127.0.0.1:1")

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"123", "--no-such-flag"})

	execErr := cmd.Execute()
	require.Error(t, execErr)

	combined := out.String() + errOut.String()
	require.Contains(t, combined, "Usage:")
}

// TestLoadedCommand_WrongArgCount_ShowsUsage proves criterion 2 (arg class)
// on the reload path: a wrong positional-arg count still prints usage.
func TestLoadedCommand_WrongArgCount_ShowsUsage(t *testing.T) {
	cmd := df019LoadedCmd(t, "http://127.0.0.1:1")

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"123", "extra"})

	execErr := cmd.Execute()
	require.Error(t, execErr)

	combined := out.String() + errOut.String()
	require.Contains(t, combined, "Usage:")
	require.NotContains(t, combined, "HTTP error")
}

// TestLoadedCommand_SilenceUsageField pins the reload-path construction
// contract (RED half of DF-019 on the cmdstore side).
func TestLoadedCommand_SilenceUsageField(t *testing.T) {
	cmd := df019LoadedCmd(t, "http://example.com")
	require.True(t, cmd.SilenceUsage,
		"reloaded persisted commands must set SilenceUsage so runtime errors don't dump usage")
}
