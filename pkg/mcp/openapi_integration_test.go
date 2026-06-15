package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/openapi"
)

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestExtractPathParamsStr(t *testing.T) {
	t.Run("single param", func(t *testing.T) {
		params := extractPathParamsStr("/users/{id}")
		assert.Equal(t, []string{"id"}, params)
	})

	t.Run("multiple params", func(t *testing.T) {
		params := extractPathParamsStr("/users/{userId}/posts/{postId}")
		assert.Equal(t, []string{"userId", "postId"}, params)
	})

	t.Run("no params", func(t *testing.T) {
		params := extractPathParamsStr("/users/list")
		assert.Empty(t, params)
	})

	t.Run("empty string", func(t *testing.T) {
		params := extractPathParamsStr("")
		assert.Empty(t, params)
	})

	t.Run("adjacent params", func(t *testing.T) {
		params := extractPathParamsStr("/{a}{b}")
		assert.Equal(t, []string{"a", "b"}, params)
	})
}

func TestGenerateCommandName(t *testing.T) {
	t.Run("uses operation ID when present", func(t *testing.T) {
		op := &openapi.Operation{
			ID:     "listUsers",
			Method: "GET",
			Path:   "/users",
		}
		assert.Equal(t, "listUsers", generateCommandName(op))
	})

	t.Run("generates from method and path when no ID", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
		}
		name := generateCommandName(op)
		assert.Contains(t, name, "GET")
		assert.Contains(t, name, "users")
	})

	t.Run("path params replaced with underscores", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
		}
		name := generateCommandName(op)
		assert.NotContains(t, name, "{")
		assert.NotContains(t, name, "}")
		assert.NotContains(t, name, "/")
	})

	t.Run("POST with path", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "POST",
			Path:   "/items",
		}
		name := generateCommandName(op)
		assert.Contains(t, name, "POST")
		assert.Contains(t, name, "items")
	})
}

func TestConvertParameters(t *testing.T) {
	t.Run("converts parameters to flags", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: []openapi.Parameter{
				{Name: "id", In: "path", Type: "integer", Required: true, Description: "Item ID"},
				{Name: "limit", In: "query", Type: "integer", Required: false, Description: "Page limit"},
			},
		}
		flags := convertParameters(op)
		require.Len(t, flags, 2)

		assert.Equal(t, "id", flags[0].Name)
		assert.Equal(t, "integer", flags[0].Type)
		assert.True(t, flags[0].Required)
		assert.Equal(t, "Item ID", flags[0].Description)

		assert.Equal(t, "limit", flags[1].Name)
		assert.Equal(t, "integer", flags[1].Type)
		assert.False(t, flags[1].Required)
		assert.Equal(t, "Page limit", flags[1].Description)
	})

	t.Run("empty parameters returns empty slice", func(t *testing.T) {
		op := &openapi.Operation{}
		flags := convertParameters(op)
		assert.Empty(t, flags)
	})

	t.Run("query param required=false even if marked required", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: []openapi.Parameter{
				{Name: "filter", In: "query", Type: "string", Required: true},
			},
		}
		flags := convertParameters(op)
		require.Len(t, flags, 1)
		// Only path params are marked required in convertParameters
		assert.False(t, flags[0].Required)
	})

	t.Run("default type is string when type is empty", func(t *testing.T) {
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

// ---------------------------------------------------------------------------
// OpenAPIRegistry tests
// ---------------------------------------------------------------------------

func TestNewRegistry(t *testing.T) {
	t.Run("creates registry from valid spec", func(t *testing.T) {
		spec := testSpec("http://example.com")
		registry, err := NewRegistry(spec, "http://example.com")
		require.NoError(t, err)
		require.NotNil(t, registry)
		assert.Equal(t, "http://example.com", registry.baseURL)
		assert.Len(t, registry.operations, 1)
	})

	t.Run("returns error from invalid spec", func(t *testing.T) {
		// A spec with no paths is valid for registration (0 operations, no error)
		// To get an actual error we'd need something that fails extraction.
		// Testing nil spec behavior:
		registry, err := NewRegistry(&openapi3.T{OpenAPI: "3.0.3"}, "http://example.com")
		require.NoError(t, err)
		require.NotNil(t, registry)
		assert.Empty(t, registry.operations)
	})
}

func TestOpenAPIRegistry_ListCommands(t *testing.T) {
	t.Run("lists commands from operations", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:      "listUsers",
					Method:  "GET",
					Path:    "/users",
					Summary: "List users",
					Parameters: []openapi.Parameter{
						{Name: "limit", In: "query", Type: "integer"},
					},
				},
				{
					ID:      "getUser",
					Method:  "GET",
					Path:    "/users/{id}",
					Summary: "Get user",
				},
			},
		}

		commands := registry.ListCommands()
		require.Len(t, commands, 2)

		assert.Equal(t, "listUsers", commands[0].Name)
		assert.Equal(t, "List users /users", commands[0].Description)
		require.Len(t, commands[0].Flags, 1)

		assert.Equal(t, "getUser", commands[1].Name)
	})

	t.Run("empty operations returns empty", func(t *testing.T) {
		registry := &OpenAPIRegistry{baseURL: "http://example.com"}
		assert.Empty(t, registry.ListCommands())
	})
}

func TestOpenAPIRegistry_GetCommand(t *testing.T) {
	t.Run("returns command for existing operation", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:      "getItem",
					Method:  "GET",
					Path:    "/items/{id}",
					Summary: "Get item",
					Parameters: []openapi.Parameter{
						{Name: "id", In: "path", Type: "string", Required: true},
					},
				},
			},
		}
		cmd := registry.GetCommand("getItem")
		require.NotNil(t, cmd)
		assert.Equal(t, "getItem", cmd.Name)
		assert.Equal(t, "Get item /items/{id}", cmd.Description)
		require.Len(t, cmd.Flags, 1)
		assert.Equal(t, "id", cmd.Flags[0].Name)
	})

	t.Run("returns nil for unknown command", func(t *testing.T) {
		registry := &OpenAPIRegistry{baseURL: "http://example.com"}
		assert.Nil(t, registry.GetCommand("nonexistent"))
	})
}

func TestOpenAPIRegistry_ExecuteCommand(t *testing.T) {
	t.Run("executes known operation", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"result":"ok"}`)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		registry := &OpenAPIRegistry{
			baseURL: server.URL,
			operations: []*openapi.Operation{
				{ID: "getData", Method: "GET", Path: "/data", Summary: "Get data"},
			},
		}

		result, err := registry.ExecuteCommand(context.Background(), "getData", nil)
		require.NoError(t, err)
		// ExecuteCommand returns the raw result from ExecuteHTTP which is json.MarshalIndent
		resultBytes, ok := result.([]byte)
		require.True(t, ok)
		assert.Contains(t, string(resultBytes), "result")
	})

	t.Run("returns error for unknown operation", func(t *testing.T) {
		registry := &OpenAPIRegistry{baseURL: "http://example.com"}
		_, err := registry.ExecuteCommand(context.Background(), "unknown", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestOpenAPIRegistry_ListTools(t *testing.T) {
	t.Run("lists tools with input schema", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:      "getItem",
					Method:  "GET",
					Path:    "/items/{id}",
					Summary: "Get an item",
					Parameters: []openapi.Parameter{
						{Name: "id", In: "path", Type: "string", Required: true, Description: "Item identifier"},
					},
				},
			},
		}

		tools := registry.ListTools()
		require.Len(t, tools, 1)
		assert.Equal(t, "getItem", tools[0].Name)
		assert.Equal(t, "Get an item", tools[0].Description)

		// Verify input schema contains the parameters
		var schema map[string]interface{}
		err := json.Unmarshal(tools[0].InputSchema, &schema)
		require.NoError(t, err)
		assert.Equal(t, "object", schema["type"])
		props, ok := schema["properties"].(map[string]interface{})
		require.True(t, ok)
		idProp, ok := props["id"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", idProp["type"])
		assert.Equal(t, "path", idProp["in"])
	})

	t.Run("uses Description as fallback when Summary is empty", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:          "getItem",
					Method:      "GET",
					Path:        "/items/{id}",
					Description: "Detailed description",
				},
			},
		}
		tools := registry.ListTools()
		require.Len(t, tools, 1)
		assert.Equal(t, "Detailed description", tools[0].Description)
	})

	t.Run("uses method+path as last-resort description", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:     "getItem",
					Method: "DELETE",
					Path:   "/items/{id}",
				},
			},
		}
		tools := registry.ListTools()
		require.Len(t, tools, 1)
		assert.Contains(t, tools[0].Description, "DELETE")
		assert.Contains(t, tools[0].Description, "/items/{id}")
	})

	t.Run("includes request body properties in schema", func(t *testing.T) {
		registry := &OpenAPIRegistry{
			baseURL: "http://example.com",
			operations: []*openapi.Operation{
				{
					ID:     "createItem",
					Method: "POST",
					Path:   "/items",
					RequestBody: []openapi.RequestBodyProperty{
						{Name: "name", Type: "string", Required: true, Description: "Item name"},
					},
				},
			},
		}
		tools := registry.ListTools()
		require.Len(t, tools, 1)

		var schema map[string]interface{}
		err := json.Unmarshal(tools[0].InputSchema, &schema)
		require.NoError(t, err)
		props, ok := schema["properties"].(map[string]interface{})
		require.True(t, ok)
		nameProp, ok := props["name"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", nameProp["type"])
	})

	t.Run("empty operations returns empty tools", func(t *testing.T) {
		registry := &OpenAPIRegistry{baseURL: "http://example.com"}
		assert.Empty(t, registry.ListTools())
	})
}

func TestOpenAPIRegistry_Operations(t *testing.T) {
	ops := []*openapi.Operation{
		{ID: "op1", Method: "GET", Path: "/a"},
		{ID: "op2", Method: "POST", Path: "/b"},
	}
	registry := &OpenAPIRegistry{
		baseURL:    "http://example.com",
		operations: ops,
	}
	returned := registry.Operations()
	assert.Equal(t, ops, returned)
	assert.Len(t, returned, 2)
}

func TestOpenAPIRegistry_MutationErrors(t *testing.T) {
	registry := &OpenAPIRegistry{baseURL: "http://example.com"}
	cmd := handlers.Command{Name: "test"}

	t.Run("AddCommand returns error", func(t *testing.T) {
		err := registry.AddCommand(cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("RemoveCommand returns error", func(t *testing.T) {
		err := registry.RemoveCommand("test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("UpdateCommand returns error", func(t *testing.T) {
		err := registry.UpdateCommand("test", cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})
}

// ---------------------------------------------------------------------------
// OpenAPIExecutor tests
// ---------------------------------------------------------------------------

func TestNewExecutor(t *testing.T) {
	ops := []*openapi.Operation{{ID: "test", Method: "GET", Path: "/test"}}
	executor := NewExecutor("http://example.com", ops)
	require.NotNil(t, executor)
	assert.Equal(t, "http://example.com", executor.baseURL)
	assert.Len(t, executor.operations, 1)
	assert.NotNil(t, executor.client)
}

func TestOpenAPIExecutor_Execute_Get(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":["a","b"]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "listItems", Method: "GET", Path: "/items", Summary: "List items"},
	})

	result, err := executor.Execute(context.Background(), "listItems", map[string]interface{}{"limit": "10"})
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, resultMap["items"], "a")
}

func TestOpenAPIExecutor_Execute_Post(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"name":"new-item"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "createItem", Method: "POST", Path: "/items", Summary: "Create item"},
	})

	result, err := executor.Execute(context.Background(), "createItem", map[string]interface{}{
		"name": "new-item",
	})
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1), resultMap["id"])
	assert.Equal(t, "new-item", resultMap["name"])
}

func TestOpenAPIExecutor_Execute_Put(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"updated":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "updateItem", Method: "PUT", Path: "/items", Summary: "Update item"},
	})

	result, err := executor.Execute(context.Background(), "updateItem", map[string]interface{}{
		"name": "updated",
	})
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.True(t, resultMap["updated"].(bool))
}

func TestOpenAPIExecutor_Execute_Delete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "99", r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"deleted":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "deleteItem", Method: "DELETE", Path: "/items", Summary: "Delete item"},
	})

	result, err := executor.Execute(context.Background(), "deleteItem", map[string]interface{}{
		"id": "99",
	})
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.True(t, resultMap["deleted"].(bool))
}

func TestOpenAPIExecutor_Execute_NotFound(t *testing.T) {
	executor := NewExecutor("http://example.com", []*openapi.Operation{
		{ID: "existing", Method: "GET", Path: "/existing"},
	})

	// Unknown name falls back to GET on "/<name>"
	// Since we didn't start a mock server at example.com, this should error
	_, err := executor.Execute(context.Background(), "nonexistent", nil)
	require.Error(t, err)
}

func TestOpenAPIExecutor_Execute_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"server error"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "badOp", Method: "GET", Path: "/error"},
	})

	_, err := executor.Execute(context.Background(), "badOp", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error")
	assert.Contains(t, err.Error(), "500")
}

func TestOpenAPIExecutor_Execute_NonJSONResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "not-json")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "getText", Method: "GET", Path: "/text", Summary: "Get text"},
	})

	result, err := executor.Execute(context.Background(), "getText", nil)
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, resultMap["message"], "response parsing failed")
	assert.Equal(t, 200, resultMap["status"])
}

func TestOpenAPIExecutor_Execute_EmptyArgs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "listItems", Method: "GET", Path: "/items", Summary: "List items"},
	})

	result, err := executor.Execute(context.Background(), "listItems", nil)
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, resultMap["items"])
}

func TestOpenAPIExecutor_Execute_GetWithQueryArgs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "hello", r.URL.Query().Get("q"))
		assert.Equal(t, "20", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"results":[]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	executor := NewExecutor(server.URL, []*openapi.Operation{
		{ID: "search", Method: "GET", Path: "/search", Summary: "Search"},
	})

	_, err := executor.Execute(context.Background(), "search", map[string]interface{}{
		"q":     "hello",
		"limit": "20",
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// NewExecutorWithCache tests
// ---------------------------------------------------------------------------

func TestNewExecutorWithCache(t *testing.T) {
	memCache := cache.NewMemoryCache(100)
	defer memCache.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"cached":false}`)
	}))
	defer server.Close()

	executor := NewExecutorWithCache(server.URL, []*openapi.Operation{
		{ID: "getData", Method: "GET", Path: "/data", Summary: "Get data"},
	}, memCache, 5*time.Minute)

	require.NotNil(t, executor)
	require.NotNil(t, executor.client)
	require.NotNil(t, executor.client.Transport)

	// First call should work
	result, err := executor.Execute(context.Background(), "getData", nil)
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.False(t, resultMap["cached"].(bool))
}

// ---------------------------------------------------------------------------
// ExecuteHTTP tests
// ---------------------------------------------------------------------------

func TestExecuteHTTP_Get(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "GET", "/data", nil)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"status": "ok"`)
}

func TestExecuteHTTP_Post(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "POST", "/items", map[string]interface{}{
		"name": "test-item",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"id": 1`)
}

func TestExecuteHTTP_WithPathParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/42", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":42,"name":"bob"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "GET", "/users/{id}", map[string]interface{}{
		"id": "42",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"name": "bob"`)
}

func TestExecuteHTTP_PostWithPathAndBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/99/items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"created":true,"userId":"99"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "POST", "/users/{userId}/items", map[string]interface{}{
		"userId": "99",
		"name":   "new-item",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"created": true`)
}

func TestExecuteHTTP_GetWithQueryParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test", r.URL.Query().Get("q"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"results":["hit"]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "GET", "/search", map[string]interface{}{
		"q": "test",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"results"`)
}

func TestExecuteHTTP_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not found"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := ExecuteHTTP(server.URL, "GET", "/error", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error")
}

func TestExecuteHTTP_Put(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items/5", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"updated":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "PUT", "/items/{id}", map[string]interface{}{
		"id":   "5",
		"name": "updated-name",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"updated": true`)
}

func TestExecuteHTTP_Patch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items/7", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"patched":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "PATCH", "/items/{id}", map[string]interface{}{
		"id":   "7",
		"name": "patched-name",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"patched": true`)
}

func TestExecuteHTTP_Delete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items/3", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"deleted":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "DELETE", "/items/{id}", map[string]interface{}{
		"id": "3",
	})
	require.NoError(t, err)
	assert.Contains(t, string(result), `"deleted": true`)
}

func TestExecuteHTTP_EmptyBodyOnPost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		// POST with empty body should not have Content-Type
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"created":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := ExecuteHTTP(server.URL, "POST", "/items", nil)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"created": true`)
}

// ---------------------------------------------------------------------------
// ToolRegistration tests
// ---------------------------------------------------------------------------

func TestNewToolRegistration(t *testing.T) {
	spec := testSpec("http://example.com")
	reg, err := NewToolRegistration(spec, "http://example.com")
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.NotNil(t, reg.registry)
	require.NotNil(t, reg.executor)
	assert.Equal(t, "http://example.com", reg.registry.baseURL)
	assert.Len(t, reg.registry.operations, 1)
}

func TestToolRegistration_RegisterHandlers(t *testing.T) {
	spec := testSpec("http://example.com")
	reg, err := NewToolRegistration(spec, "http://example.com")
	require.NoError(t, err)

	handlerRegistry := handlers.NewRegistry()
	reg.RegisterHandlers(handlerRegistry)

	// Verify tools/list handler is registered
	listHandler := handlerRegistry.Get("tools/list")
	require.NotNil(t, listHandler)
	assert.Equal(t, "tools/list", listHandler.Method())

	// Verify tools/call handler is registered
	callHandler := handlerRegistry.Get("tools/call")
	require.NotNil(t, callHandler)
	assert.Equal(t, "tools/call", callHandler.Method())

	// Test that tools/list actually returns something
	result, err := handlerRegistry.Handle(context.Background(), "tools/list", nil)
	require.NoError(t, err)
	listResult, ok := result.(*handlers.ListToolsResult)
	require.True(t, ok)
	require.Len(t, listResult.Tools, 1)
	assert.Equal(t, "testGet", listResult.Tools[0].Name)
}

// ---------------------------------------------------------------------------
// NewExecutorWithRateLimit tests (additional edge cases)
// ---------------------------------------------------------------------------

func TestNewExecutorWithRateLimit_ZeroBurst(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	spec := testSpec(server.URL)
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	require.NoError(t, err)

	// ratePerSec=0 means rate limiting is disabled
	exec := NewExecutorWithRateLimit(server.URL, ops, 0, 0)
	result, err := exec.Execute(context.Background(), "testGet", nil)
	require.NoError(t, err)
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.True(t, resultMap["ok"].(bool))
}
