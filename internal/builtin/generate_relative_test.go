package builtin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
)

// relativeServerAPISpecJSON declares a RELATIVE server URL; fetched over
// http(s) it must resolve against the spec fetch URL (DF-015).
const relativeServerAPISpecJSON = `{
	"openapi": "3.0.0",
	"info": {"title": "Rel API", "version": "1.0.0"},
	"servers": [{"url": "/api/v3"}],
	"paths": {
		"/pets/{petId}": {
			"get": {
				"operationId": "get-pet",
				"parameters": [{"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}],
				"responses": {"200": {"description": "ok"}}
			}
		}
	}
}`

// TestGenerateCommand_RelativeServerOverHTTP_ResolvesBaseURL proves DF-015
// end to end on the CLI surface: a spec with a RELATIVE servers[].url
// fetched over http(s) generates commands whose base URL resolves against
// the spec fetch URL, and the reloaded generated command issues a real
// request against the resolved base.
func TestGenerateCommand_RelativeServerOverHTTP_ResolvesBaseURL(t *testing.T) {
	t.Setenv("MUSTER_BASE_URL", "")
	var absRequested, relativeRequested bool
	specSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rel-spec.json":
			_, _ = w.Write([]byte(relativeServerAPISpecJSON))
		case "/api/v3/pets/1":
			absRequested = true
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") {
				relativeRequested = true
			}
			http.NotFound(w, r)
		}
	}))
	defer specSrv.Close()

	storePath := filepath.Join(t.TempDir(), "commands.json")
	cfg := &config.Config{PersistedCommandsPath: storePath}

	genCmd := newGenerateCommand(cfg)
	require.NoError(t, genCmd.RunE(genCmd, []string{specSrv.URL + "/rel-spec.json"}))

	sd := cmdstore.NewCommandStore(storePath).List()
	require.NotNil(t, sd)
	require.NotEmpty(t, sd.Commands)
	require.Equal(t, specSrv.URL+"/api/v3", sd.Commands[0].BaseURL,
		"persisted BaseURL must resolve the relative server against the spec fetch URL")

	// Reload through the same path the CLI root uses (loadPersistedCommands:
	// NewLoader + GetBaseURL + LoadCommands) and run the leaf.
	loader := cmdstore.NewLoader(cmdstore.NewCommandStore(storePath), "")
	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	loader = cmdstore.NewLoader(cmdstore.NewCommandStore(storePath), baseURL)
	cmds, err := loader.LoadCommands()
	require.NoError(t, err)

	var leaf *cobra.Command
	for _, c := range cmds {
		if c.Name() == "get-pet" {
			leaf = c
		}
	}
	require.NotNil(t, leaf, "generated command not reloaded from the persisted store")

	var buf bytes.Buffer
	leaf.SetOut(&buf)
	leaf.SetErr(&buf)
	require.NoError(t, leaf.RunE(leaf, []string{"1"}))
	require.True(t, absRequested, "request must hit the resolved absolute base URL")
	require.False(t, relativeRequested, "no request may go out with a raw relative base")
	// DF-016: the reloaded path now formats per --output (default table),
	// so the body is rendered as a formatted cell rather than the raw
	// JSON dump; assert the response content survives formatting.
	require.Contains(t, buf.String(), "1")
}
