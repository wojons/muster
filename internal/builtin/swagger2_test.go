package builtin

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
)

// PROTO-006: Swagger 2.0 documents flow through the REAL discover and
// generate paths. Pre-PROTO-006 measured behavior (the defect these tests
// pin): discover printed the FILE PATH as the Base URL and no conversion
// note; generate silently produced commands bound to the default
// https://api.example.com base URL with no parameter flags — a parse that
// succeeded while dropping host/basePath/servers, query/body parameters,
// and every #/definitions schema.

func swagger2FixturePath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../demo/openapi-specs/petstore-v2.json")
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.NoError(t, err, "petstore-v2.json fixture should exist")
	return path
}

func TestDiscoverCommand_Swagger2_StatesConversionAndRealBaseURL(t *testing.T) {
	cmd := newDiscoverCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.RunE(cmd, []string{swagger2FixturePath(t)})
	require.NoError(t, err)

	out := buf.String()
	// Acceptance: the discovery output states explicitly that a 2.0
	// document was converted.
	assert.Contains(t, out, "Note: converted from Swagger/OpenAPI 2.0 to OpenAPI 3.x")
	// host/basePath/schemes must surface as the real server URL — not the
	// spec's file path (the measured pre-fix output).
	assert.Contains(t, out, "Base URL: https://petstore.swagger.io/v2")
	assert.NotContains(t, out, "Base URL: /", "local file path must not be printed as the base URL")
	// The operations themselves survive.
	assert.Contains(t, out, "getPetById")
	assert.Contains(t, out, "findPetsByStatus")
	assert.Contains(t, out, "deleteUser")
}

func TestGenerateCommand_Swagger2_StoreCarriesRealBaseURLAndFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	storePath := filepath.Join(t.TempDir(), "commands.json")
	cfg := &config.Config{PersistedCommandsPath: storePath}

	genCmd := newGenerateCommand(cfg)
	err := genCmd.RunE(genCmd, []string{swagger2FixturePath(t)})
	require.NoError(t, err)

	sd := cmdstore.NewCommandStore(storePath).List()
	require.NotNil(t, sd)
	// Acceptance: the generator never returns an empty command set for a
	// document that parsed successfully.
	require.NotEmpty(t, sd.Commands)

	byName := make(map[string]map[string]bool, len(sd.Commands))
	for _, c := range sd.Commands {
		flags := make(map[string]bool, len(c.Flags))
		for _, f := range c.Flags {
			flags[f.Name] = true
		}
		byName[c.Name] = flags
		assert.Equal(t, "https://petstore.swagger.io/v2", c.BaseURL,
			"command %s must carry the converted server URL, not the default placeholder", c.Name)
	}

	// Query parameters survive (pre-fix they were silently dropped).
	statusFlags, ok := byName["find-pets-by-status"]
	require.True(t, ok, "find-pets-by-status command should exist")
	assert.True(t, statusFlags["status"], "query parameter 'status' must become a flag")

	// in:body with a $ref schema resolves per-property (pre-fix: zero body
	// flags — #/definitions never resolved).
	updateFlags, ok := byName["update-pet"]
	require.True(t, ok, "update-pet command should exist")
	assert.True(t, updateFlags["body"], "typed --body flag should exist for the JSON request body")
	for _, prop := range []string{"name", "photo-urls", "status", "category"} {
		assert.True(t, updateFlags[prop], "body property %q must become a flag (definitions resolved)", prop)
	}

	// formData parameters convert into a urlencoded request body (proven
	// at the document level in pkg/openapi). The generator renders
	// per-property flags for multipart/form-data bodies only — measured
	// pre-existing behavior, identical for a native 3.x document with the
	// same body (the parity fixture pins it), so only the standard flags
	// appear here. Rendering urlencoded properties is a generator-wide
	// gap, out of PROTO-006's conversion scope.
	formFlags, ok := byName["update-pet-with-form"]
	require.True(t, ok, "update-pet-with-form command should exist")
	assert.False(t, formFlags["name"], "urlencoded body properties are not rendered as flags (parity with native 3.x)")
	assert.True(t, formFlags["raw"], "raw --body flag is available for the form body")
}

func TestGenerateCommand_Swagger2PathsLess_FailsLoudly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "empty-v2.json")
	specContent := []byte(`{
		"swagger": "2.0",
		"info": {"title": "Empty", "version": "1.0.0"},
		"host": "example.com",
		"paths": {}
	}`)
	require.NoError(t, os.WriteFile(specPath, specContent, 0644))

	storePath := filepath.Join(tmpDir, "commands.json")
	cfg := &config.Config{PersistedCommandsPath: storePath}

	genCmd := newGenerateCommand(cfg)
	err := genCmd.RunE(genCmd, []string{specPath})
	// Acceptance: a successfully parsed document never yields an empty
	// command set — a paths-less 2.0 document is refused loudly instead.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no operations")

	_, statErr := os.Stat(storePath)
	assert.True(t, os.IsNotExist(statErr), "no empty command store may be persisted")
}

func TestDiscoverCommand_Swagger2YAML(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec-v2.yaml")
	specContent := []byte(`swagger: "2.0"
info:
  title: YAML Petstore
  version: "1.0.0"
host: petstore.example.com
basePath: /v1
schemes:
  - https
paths:
  /widgets:
    get:
      operationId: listWidgets
      produces:
        - application/json
      parameters:
        - name: limit
          in: query
          type: integer
          required: false
      responses:
        200:
          description: OK
`)
	require.NoError(t, os.WriteFile(specPath, specContent, 0644))

	cmd := newDiscoverCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.RunE(cmd, []string{specPath})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Note: converted from Swagger/OpenAPI 2.0 to OpenAPI 3.x")
	assert.Contains(t, out, "Base URL: https://petstore.example.com/v1")
	assert.Contains(t, out, "listWidgets")
}
