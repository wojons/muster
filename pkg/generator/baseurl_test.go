package generator

import (
	"net/http"
	"net/http/httptest"
	"sync"
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

// petSpecWithServerJSON is the REVIEW-MUSTER-28 fixture: a spec that declares
// its own root servers[0]. `generate --base-url` must outrank it.
const petSpecWithServerJSON = `{
	"openapi": "3.0.0",
	"info": {"title": "Abs API", "version": "1.0.0"},
	"servers": [{"url": "http://spec-server.invalid"}],
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

// petSpecWithoutServerJSON declares no servers at all, so the configured base
// URL is the only source available.
const petSpecWithoutServerJSON = `{
	"openapi": "3.0.0",
	"info": {"title": "No Server API", "version": "1.0.0"},
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

// newRecordingAPIServer returns a server that records the request path of
// every call and answers with a small JSON object, plus an accessor for the
// recorded paths.
func newRecordingAPIServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","name":"pet"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(paths))
		copy(out, paths)
		return out
	}
}

// TestGenerator_ExplicitBaseURLOverride_WinsOverSpecServers is the
// REVIEW-MUSTER-28 regression: when the caller explicitly supplies a base URL
// (Config.BaseURLOverride, set by `generate --base-url`), the generated
// command must target it even though the spec declares servers[0].
func TestGenerator_ExplicitBaseURLOverride_WinsOverSpecServers(t *testing.T) {
	t.Setenv("MUSTER_BASE_URL", "")

	overrideSrv, overrideHits := newRecordingAPIServer(t)
	specSrv, specHits := newRecordingAPIServer(t)

	doc, err := openapi3.NewLoader().LoadFromData([]byte(petSpecWithServerJSON))
	require.NoError(t, err)
	// Point the spec's declared server at a real (recording) server so a
	// regression is observable as "the spec URL was called" rather than as a
	// DNS failure.
	doc.Servers[0].URL = specSrv.URL

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: overrideSrv.URL, BaseURLOverride: true})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	leaf := findCommand(commands, "get-pet")
	require.NotNil(t, leaf, "generated get-pet command not found")

	require.NoError(t, leaf.RunE(leaf, []string{"1"}))

	require.Equal(t, []string{"/pet/1"}, overrideHits(),
		"--base-url must win over the spec's servers[0]")
	require.Empty(t, specHits(), "the spec's declared server must not be called")
}

// TestGenerator_ExplicitBaseURLOverride_UsedWhenSpecHasNoServers keeps the
// flag honoured for specs that declare no servers at all.
func TestGenerator_ExplicitBaseURLOverride_UsedWhenSpecHasNoServers(t *testing.T) {
	t.Setenv("MUSTER_BASE_URL", "")

	overrideSrv, hits := newRecordingAPIServer(t)

	doc, err := openapi3.NewLoader().LoadFromData([]byte(petSpecWithoutServerJSON))
	require.NoError(t, err)

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: overrideSrv.URL, BaseURLOverride: true})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	leaf := findCommand(commands, "get-pet")
	require.NotNil(t, leaf, "generated get-pet command not found")
	require.NoError(t, leaf.RunE(leaf, []string{"1"}))
	require.Equal(t, []string{"/pet/1"}, hits())
}

// TestGenerator_NoOverride_SpecServerStillWins pins the unchanged default:
// without an explicit override the spec's servers[0] keeps precedence over
// the config fallback (DF-007).
func TestGenerator_NoOverride_SpecServerStillWins(t *testing.T) {
	t.Setenv("MUSTER_BASE_URL", "")

	fallbackSrv, fallbackHits := newRecordingAPIServer(t)
	specSrv, specHits := newRecordingAPIServer(t)

	doc, err := openapi3.NewLoader().LoadFromData([]byte(petSpecWithServerJSON))
	require.NoError(t, err)
	doc.Servers[0].URL = specSrv.URL

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: fallbackSrv.URL})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	leaf := findCommand(commands, "get-pet")
	require.NotNil(t, leaf)
	require.NoError(t, leaf.RunE(leaf, []string{"1"}))

	require.Equal(t, []string{"/pet/1"}, specHits(), "spec servers[0] keeps precedence without --base-url")
	require.Empty(t, fallbackHits(), "the config fallback must not be used when the spec declares a server")
}

// TestGenerator_BaseURLOverride_StampsEffectiveServerAnnotation keeps the
// persisted-metadata annotation in agreement with the URL actually called.
func TestGenerator_BaseURLOverride_StampsEffectiveServerAnnotation(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData([]byte(petSpecWithServerJSON))
	require.NoError(t, err)
	doc.Servers[0].URL = "http://spec-server.invalid"

	overrideGen, err := NewGenerator(Config{AppName: "test", BaseURL: "http://127.0.0.1:19211", BaseURLOverride: true})
	require.NoError(t, err)
	commands, err := overrideGen.GenerateCommands(doc)
	require.NoError(t, err)
	leaf := findCommand(commands, "get-pet")
	require.NotNil(t, leaf)
	require.Equal(t, "http://127.0.0.1:19211", leaf.Annotations[AnnotationOperationServer])

	plainGen, err := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})
	require.NoError(t, err)
	commands, err = plainGen.GenerateCommands(doc)
	require.NoError(t, err)
	leaf = findCommand(commands, "get-pet")
	require.NotNil(t, leaf)
	require.Equal(t, "http://spec-server.invalid", leaf.Annotations[AnnotationOperationServer])
}
