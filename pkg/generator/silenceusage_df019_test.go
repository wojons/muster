package generator

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/openapi"
)

// DF-019 regression: a generated leaf command that fails at RUNTIME (e.g. an
// HTTP 404) must print the error without the full cobra Usage/Examples/Flags
// block. A scripting user must not see a usage dump that looks like a flag
// mistake. SilenceUsage=true + FlagErrorFunc achieves this; see
// internal/builtin/request.go for the origin pattern.

// df019Operation is a GET with one path param — the get-pet-by-id shape from
// the live repro.
func df019Operation() *openapi.Operation {
	return &openapi.Operation{
		ID:     "getPetById",
		Path:   "/pets/{petId}",
		Method: "GET",
	}
}

// TestGeneratedCommand_RuntimeError_SuppressesUsage proves criterion 1: a
// runtime (HTTP 4xx) error does NOT print the usage block.
func TestGeneratedCommand_RuntimeError_SuppressesUsage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`Pet not found`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	cmd := gen.buildOperationCommand(df019Operation(), map[string]bool{})

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"99999999"})

	execErr := cmd.Execute()
	require.Error(t, execErr)
	require.Contains(t, execErr.Error(), "HTTP error")

	// The usage block must NOT appear on any stream.
	combined := out.String() + errOut.String()
	require.NotContains(t, combined, "Usage:")
	require.NotContains(t, combined, "Flags:")
}

// TestGeneratedCommand_FlagError_ShowsUsage proves criterion 2 (flag class):
// an unknown flag STILL prints the usage block.
func TestGeneratedCommand_FlagError_ShowsUsage(t *testing.T) {
	gen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})
	require.NoError(t, err)

	cmd := gen.buildOperationCommand(df019Operation(), map[string]bool{})

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"123", "--no-such-flag"})

	execErr := cmd.Execute()
	require.Error(t, execErr)

	combined := out.String() + errOut.String()
	require.Contains(t, combined, "Usage:")
}

// TestGeneratedCommand_WrongArgCount_ShowsUsage proves criterion 2 (arg class):
// a wrong positional-arg count still prints usage, and does not reach RunE.
func TestGeneratedCommand_WrongArgCount_ShowsUsage(t *testing.T) {
	gen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})
	require.NoError(t, err)

	cmd := gen.buildOperationCommand(df019Operation(), map[string]bool{})

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	// get-pet-by-id takes exactly one positional arg; pass two.
	cmd.SetArgs([]string{"123", "extra"})

	execErr := cmd.Execute()
	require.Error(t, execErr)

	combined := out.String() + errOut.String()
	require.Contains(t, combined, "Usage:")
	require.NotContains(t, combined, "HTTP error")
}

// TestGeneratedCommand_SilenceUsageField pins the construction contract so a
// future refactor cannot silently drop SilenceUsage (the RED half of DF-019).
func TestGeneratedCommand_SilenceUsageField(t *testing.T) {
	gen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})
	require.NoError(t, err)
	cmd := gen.buildOperationCommand(df019Operation(), map[string]bool{})
	require.True(t, cmd.SilenceUsage,
		"generated leaf commands must set SilenceUsage so runtime errors don't dump usage")
}

// TestGeneratedCommand_UnknownCommandOnParent unaffected sanity: the parent
// group command construction is untouched by DF-019.
func TestGeneratedCommand_ParentGroupUntouched(t *testing.T) {
	gen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})
	require.NoError(t, err)
	parent := gen.buildResourceCommand("pets", []*openapi.Operation{df019Operation()})
	require.False(t, parent.SilenceUsage)
	require.Len(t, parent.Commands(), 1)
	require.True(t, parent.Commands()[0].SilenceUsage)
}
