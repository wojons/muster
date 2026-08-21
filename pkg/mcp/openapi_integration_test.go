package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/openapi"
)

// petstoreSpec returns a minimal OpenAPI 3.0 petstore spec as JSON.
func petstoreSpec() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "Pet Store", "version": "1.0.0"},
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "description": "Returns all pets in the store",
        "parameters": [
          {"name": "limit", "in": "query", "schema": {"type": "integer"}}
        ],
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "operationId": "createPet",
        "summary": "Create a pet",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": {"type": "string"},
                  "species": {"type": "string"}
                },
                "required": ["name"]
              }
            }
          }
        },
        "responses": {"201": {"description": "Created"}}
      }
    },
    "/pets/{petId}": {
      "get": {
        "operationId": "getPetById",
        "summary": "Get pet by ID",
        "parameters": [
          {"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "OK"}}
      },
      "delete": {
        "operationId": "deletePet",
        "summary": "Delete a pet",
        "parameters": [
          {"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {"204": {"description": "Deleted"}}
      }
    }
  }
}`
}

// petstoreSpecNoID returns a spec without operationId, so names are generated.
func petstoreSpecNoID() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "No ID", "version": "1.0.0"},
  "paths": {
    "/items": {
      "get": {
        "summary": "List items",
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

func loadSpec(t *testing.T, specJSON string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specJSON))
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

// =============================================================================
// NewRegistry
// =============================================================================

func TestNewRegistry(t *testing.T) {
	t.Run("valid spec", func(t *testing.T) {
		spec := loadSpec(t, petstoreSpec())
		registry, err := NewRegistry(spec, "https://api.example.com")
		require.NoError(t, err)
		require.NotNil(t, registry)
		assert.Equal(t, "https://api.example.com", registry.baseURL)
		assert.Len(t, registry.operations, 4) // listPets, createPet, getPetById, deletePet
	})

	t.Run("nil spec returns empty registry", func(t *testing.T) {
		// Parser.ExtractOperations handles nil gracefully (returns nil, nil)
		registry, err := NewRegistry(nil, "https://api.example.com")
		require.NoError(t, err) // no operations, but no error either
		assert.Len(t, registry.operations, 0)
	})

	t.Run("empty spec has no operations", func(t *testing.T) {
		spec := loadSpec(t, `{"openapi":"3.0.0","info":{"title":"Empty","version":"1.0.0"},"paths":{}}`)
		registry, err := NewRegistry(spec, "https://api.example.com")
		require.NoError(t, err)
		assert.Len(t, registry.operations, 0)
	})
}

// =============================================================================
// OpenAPIRegistry methods
// =============================================================================

func TestOpenAPIRegistry_ListCommands(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	commands := registry.ListCommands()
	assert.Len(t, commands, 4)

	// Order is non-deterministic (Go map iteration)
	names := make(map[string]bool)
	for _, cmd := range commands {
		names[cmd.Name] = true
	}
	assert.True(t, names["listPets"])
	assert.True(t, names["createPet"])
	assert.True(t, names["getPetById"])
	assert.True(t, names["deletePet"])
}

func TestOpenAPIRegistry_GetCommand(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	t.Run("found by operationId", func(t *testing.T) {
		cmd := registry.GetCommand("listPets")
		require.NotNil(t, cmd)
		assert.Equal(t, "listPets", cmd.Name)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		cmd := registry.GetCommand("nonexistent")
		assert.Nil(t, cmd)
	})
}

func TestOpenAPIRegistry_ExecuteCommand(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	t.Run("command not found", func(t *testing.T) {
		_, err := registry.ExecuteCommand(context.Background(), "nonexistent", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestOpenAPIRegistry_AddCommand(t *testing.T) {
	registry := &OpenAPIRegistry{}
	err := registry.AddCommand(handlers.Command{Name: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}

func TestOpenAPIRegistry_RemoveCommand(t *testing.T) {
	registry := &OpenAPIRegistry{}
	err := registry.RemoveCommand("test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}

func TestOpenAPIRegistry_UpdateCommand(t *testing.T) {
	registry := &OpenAPIRegistry{}
	err := registry.UpdateCommand("test", handlers.Command{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}

func TestOpenAPIRegistry_Operations(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	ops := registry.Operations()
	assert.Len(t, ops, 4)
}

func TestOpenAPIRegistry_ListTools(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	tools := registry.ListTools()
	assert.Len(t, tools, 4)

	// Verify all expected tool names are present (order is non-deterministic)
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.InputSchema)
	}
	assert.True(t, toolNames["listPets"])
	assert.True(t, toolNames["createPet"])
	assert.True(t, toolNames["getPetById"])
	assert.True(t, toolNames["deletePet"])

	// Verify InputSchema is valid JSON
	var schema map[string]interface{}
	err = json.Unmarshal(tools[0].InputSchema, &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])

	// DF-009: inputSchema properties must contain ONLY standard JSON Schema keys.
	// The OpenAPI parameter location ("in": "query"/"path") is NOT a valid JSON
	// Schema property and must not leak into MCP tool arguments.
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for propName, propVal := range props {
			propMap, ok := propVal.(map[string]interface{})
			require.True(t, ok, "property %s is not a map", propName)
			_, hasIn := propMap["in"]
			assert.False(t, hasIn, "property %s must not contain non-standard 'in' key", propName)
		}
	}
}

// =============================================================================
// NewExecutor
// =============================================================================

func TestNewExecutor(t *testing.T) {
	e := NewExecutor("https://api.example.com", nil)
	require.NotNil(t, e)
	assert.Equal(t, "https://api.example.com", e.baseURL)
	assert.NotNil(t, e.client)
}

func TestNewExecutorWithRateLimit(t *testing.T) {
	t.Run("with rate limit", func(t *testing.T) {
		e := NewExecutorWithRateLimit("https://api.example.com", nil, 10, 5)
		require.NotNil(t, e)
		assert.NotNil(t, e.client.Transport)
	})

	t.Run("zero rate disables rate limit", func(t *testing.T) {
		e := NewExecutorWithRateLimit("https://api.example.com", nil, 0, 5)
		require.NotNil(t, e)
		// Should use default transport
	})
}

func TestNewExecutorWithCache(t *testing.T) {
	memCache := cache.NewMemoryCache(1000)
	t.Cleanup(func() { _ = memCache.Close() })

	e := NewExecutorWithCache("https://api.example.com", nil, memCache, 5*time.Minute)
	require.NotNil(t, e)
	assert.NotNil(t, e.client.Transport)
}

// =============================================================================
// Execute
// =============================================================================

func TestOpenAPIExecutor_Execute(t *testing.T) {
	t.Run("fallback GET when operation not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/unknown", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}))
		defer srv.Close()

		e := NewExecutor(srv.URL, nil)
		result, err := e.Execute(context.Background(), "unknown", nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("context cancellation", func(t *testing.T) {
		e := NewExecutor("http://127.0.0.1:1", nil) // unreachable
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := e.Execute(ctx, "anything", nil)
		assert.Error(t, err)
	})
}

func TestOpenAPIExecutor_lookupOperation(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	e := NewExecutor("https://api.example.com", registry.Operations())

	t.Run("found", func(t *testing.T) {
		method, path := e.lookupOperation("listPets")
		assert.Equal(t, "GET", method)
		assert.Equal(t, "/pets", path)
	})

	t.Run("not found", func(t *testing.T) {
		method, path := e.lookupOperation("nonexistent")
		assert.Equal(t, "", method)
		assert.Equal(t, "", path)
	})
}

// =============================================================================
// NewToolRegistration + RegisterHandlers
// =============================================================================

func TestNewToolRegistration(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	tr, err := NewToolRegistration(spec, "https://api.example.com")
	require.NoError(t, err)
	require.NotNil(t, tr)
	assert.NotNil(t, tr.registry)
	assert.NotNil(t, tr.executor)
}

func TestToolRegistration_RegisterHandlers(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	tr, err := NewToolRegistration(spec, "https://api.example.com")
	require.NoError(t, err)

	server := handlers.NewRegistry()
	tr.RegisterHandlers(server)

	// Should register list and call handlers — verify via Methods()
	methods := server.Methods()
	assert.Contains(t, methods, "tools/list")
	assert.Contains(t, methods, "tools/call")
}

// =============================================================================
// ExecuteHTTP
// =============================================================================

func TestExecuteHTTP(t *testing.T) {
	t.Run("GET with query params", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/pets", r.URL.Path)
			assert.Equal(t, "10", r.URL.Query().Get("limit"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"count": 3})
		}))
		defer srv.Close()

		result, err := ExecuteHTTP(srv.URL, "GET", "/pets", map[string]interface{}{
			"limit": "10",
		})
		require.NoError(t, err)
		assert.Contains(t, string(result), "count")
	})

	t.Run("POST with JSON body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			var body map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&body)
			require.NoError(t, err)
			assert.Equal(t, "Fluffy", body["name"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 1, "name": "Fluffy"})
		}))
		defer srv.Close()

		result, err := ExecuteHTTP(srv.URL, "POST", "/pets", map[string]interface{}{
			"name": "Fluffy",
		})
		require.NoError(t, err)
		assert.Contains(t, string(result), "Fluffy")
	})

	t.Run("PUT with body and path params", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PUT", r.Method)
			assert.Contains(t, r.URL.Path, "/pets/")
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			assert.Equal(t, "Rex", body["name"])
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		}))
		defer srv.Close()

		result, err := ExecuteHTTP(srv.URL, "PUT", "/pets/{petId}", map[string]interface{}{
			"petId": "123",
			"name":  "Rex",
		})
		require.NoError(t, err)
		assert.Contains(t, string(result), "updated")
	})

	t.Run("DELETE", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "DELETE", r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"deleted":true}`))
		}))
		defer srv.Close()

		result, err := ExecuteHTTP(srv.URL, "DELETE", "/pets/1", nil)
		require.NoError(t, err)
		assert.Contains(t, string(result), "deleted")
	})

	t.Run("HTTP error returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := ExecuteHTTP(srv.URL, "GET", "/nonexistent", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP error")
	})

	t.Run("unreachable server", func(t *testing.T) {
		_, err := ExecuteHTTP("http://127.0.0.1:1", "GET", "/test", nil)
		assert.Error(t, err)
	})

	// DF-016: a top-level JSON array response must decode without error.
	t.Run("GET returns JSON array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 1, "name": "Rex"},
				{"id": 2, "name": "Whiskers"},
			})
		}))
		defer srv.Close()

		result, err := ExecuteHTTP(srv.URL, "GET", "/pets", nil)
		require.NoError(t, err)
		// result must be a JSON array, not an isError map
		assert.Contains(t, string(result), "Rex")
		assert.Contains(t, string(result), "Whiskers")
		assert.NotContains(t, string(result), "cannot unmarshal array")

		// Verify the decoded structure is an array
		var decoded interface{}
		require.NoError(t, json.Unmarshal(result, &decoded))
		_, ok := decoded.([]interface{})
		assert.True(t, ok, "decoded result should be a JSON array")
	})
}

// =============================================================================
// extractPathParamsStr
// =============================================================================

func TestExtractPathParamsStr(t *testing.T) {
	t.Run("single param", func(t *testing.T) {
		result := extractPathParamsStr("/pets/{petId}")
		assert.Equal(t, []string{"petId"}, result)
	})

	t.Run("multiple params", func(t *testing.T) {
		result := extractPathParamsStr("/users/{userId}/posts/{postId}")
		assert.Equal(t, []string{"userId", "postId"}, result)
	})

	t.Run("no params", func(t *testing.T) {
		result := extractPathParamsStr("/pets")
		assert.Empty(t, result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := extractPathParamsStr("")
		assert.Empty(t, result)
	})

	t.Run("unclosed brace", func(t *testing.T) {
		result := extractPathParamsStr("/pets/{petId")
		assert.Empty(t, result)
	})
}

// =============================================================================
// generateCommandName
// =============================================================================

func TestGenerateCommandName(t *testing.T) {
	t.Run("uses operationId when present", func(t *testing.T) {
		op := &openapi.Operation{ID: "listPets", Method: "GET", Path: "/pets"}
		assert.Equal(t, "listPets", generateCommandName(op))
	})

	t.Run("generates from method and path when no ID", func(t *testing.T) {
		op := &openapi.Operation{Method: "GET", Path: "/items"}
		name := generateCommandName(op)
		assert.True(t, strings.HasPrefix(name, "GET"))
		assert.Contains(t, name, "items")
	})

	t.Run("replaces braces with underscores", func(t *testing.T) {
		op := &openapi.Operation{Method: "GET", Path: "/users/{id}"}
		name := generateCommandName(op)
		assert.NotContains(t, name, "{")
		assert.NotContains(t, name, "}")
	})
}

// =============================================================================
// convertParameters
// =============================================================================

func TestConvertParameters(t *testing.T) {
	t.Run("converts parameters", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: []openapi.Parameter{
				{Name: "limit", Type: "integer", In: "query", Description: "Max results"},
				{Name: "petId", Type: "string", In: "path", Required: true, Description: "Pet ID"},
			},
		}
		flags := convertParameters(op)
		assert.Len(t, flags, 2)
		assert.Equal(t, "limit", flags[0].Name)
		assert.Equal(t, "integer", flags[0].Type)
		assert.False(t, flags[0].Required) // query param
		assert.Equal(t, "petId", flags[1].Name)
		assert.True(t, flags[1].Required) // path param + required
	})

	t.Run("empty parameters", func(t *testing.T) {
		op := &openapi.Operation{}
		flags := convertParameters(op)
		assert.Empty(t, flags)
	})

	t.Run("default type is string", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: []openapi.Parameter{
				{Name: "name", In: "query"},
			},
		}
		flags := convertParameters(op)
		require.Len(t, flags, 1)
		assert.Equal(t, "string", flags[0].Type)
	})
}

// =============================================================================
// Full integration: spec with no operationId
// =============================================================================

func TestRegistry_NoOperationID(t *testing.T) {
	spec := loadSpec(t, petstoreSpecNoID())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	commands := registry.ListCommands()
	assert.Len(t, commands, 1)
	assert.Contains(t, commands[0].Name, "GET")

	tools := registry.ListTools()
	assert.Len(t, tools, 1)
}

// =============================================================================
// Execute with real spec-mapped operations
// =============================================================================

func TestOpenAPIExecutor_Execute_WithOperations(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	registry, err := NewRegistry(spec, "https://api.example.com")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/pets" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 1, "name": "Fluffy"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	e := NewExecutor(srv.URL, registry.Operations())
	result, err := e.Execute(context.Background(), "listPets", nil)
	require.NoError(t, err)

	// DF-016: the response is a JSON array; the executor must return it
	// as a []interface{}, not a soft-fallback error map.
	arr, ok := result.([]interface{})
	require.True(t, ok, "expected []interface{} for array response, got %T", result)
	require.Len(t, arr, 1)

	first, ok := arr[0].(map[string]interface{})
	require.True(t, ok, "first array element should be a map")
	assert.Equal(t, "Fluffy", first["name"])
}
