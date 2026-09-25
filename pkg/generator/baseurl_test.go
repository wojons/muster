package generator

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// relativeServerSpecJSON is a minimal spec declaring a RELATIVE server URL —
// the DF-015 class: per the OpenAPI specification `servers: [{url: /api/v3}]`
// resolves against the spec's host, but a naive read treats "/api/v3" as a
// usable base URL and requests die with `unsupported protocol scheme ""`.
const relativeServerSpecJSON = `{
	"openapi": "3.0.0",
	"info": {"title": "Rel API", "version": "1.0.0"},
	"servers": [{"url": "/api/v3"}],
	"paths": {
		"/pet/{petId}": {
			"get": {
				"operationId": "get-pet",
				"parameters": [{"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}],
				"responses": {"200": {"description": "ok"}}
			}
		}
	}
}`

// findCommand walks a generated command tree looking for a command by name.
func findCommand(commands []*cobra.Command, name string) *cobra.Command {
	var found *cobra.Command
	var walk func(cs []*cobra.Command)
	walk = func(cs []*cobra.Command) {
		for _, c := range cs {
			if c.Name() == name {
				found = c
			}
			walk(c.Commands())
		}
	}
	walk(commands)
	return found
}

// TestGeneratedRunHandler_RelativeBaseURL_ActionableError pins the DF-015 UX
// contract: when the resolved base URL is still RELATIVE (local-file spec,
// no source URL to resolve against, no runtime override), the run handler
// must fail with the actionable --base-url / MUSTER_BASE_URL hint — never
// the bare `unsupported protocol scheme ""` http error.
func TestGeneratedRunHandler_RelativeBaseURL_ActionableError(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData([]byte(relativeServerSpecJSON))
	require.NoError(t, err)

	t.Setenv("MUSTER_BASE_URL", "")
	gen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://unused-fallback.example.com"})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	leaf := findCommand(commands, "get-pet")
	require.NotNil(t, leaf, "generated get-pet command not found")

	err = leaf.RunE(leaf, []string{"1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--base-url", "error must name the --base-url escape hatch")
	require.Contains(t, err.Error(), "MUSTER_BASE_URL", "error must name the env escape hatch")
	require.NotContains(t, err.Error(), "unsupported protocol scheme", "bare http error must not surface")
}
