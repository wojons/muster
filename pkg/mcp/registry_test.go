package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func jsonRaw(s string) json.RawMessage {
	return json.RawMessage(s)
}

var schemaWithParams = jsonRaw(`{
	"type": "object",
	"properties": {
		"petId": {
			"type": "string",
			"in": "path",
			"description": "ID of the pet"
		},
		"status": {
			"type": "string",
			"in": "query",
			"description": "Filter by status"
		}
	},
	"required": ["petId"]
}`)

var schemaWithBody = jsonRaw(`{
	"type": "object",
	"properties": {
		"name": {
			"type": "string",
			"description": "Name of the resource"
		},
		"body": {
			"type": "object",
			"description": "Request body payload"
		}
	},
	"required": ["name"]
}`)

var schemaWithHeader = jsonRaw(`{
	"type": "object",
	"properties": {
		"X-API-Key": {
			"type": "string",
			"in": "header",
			"description": "API key"
		},
		"limit": {
			"type": "integer",
			"in": "query"
		}
	}
}`)

var emptySchema = jsonRaw(`{"type":"object","properties":{}}`)

func makeTool(name, desc string, schema json.RawMessage) OpenAPITool {
	return OpenAPITool{
		Name:        name,
		Description: desc,
		Method:      "GET",
		Path:        "/" + name,
		BaseURL:     "http://example.com",
		InputSchema: schema,
	}
}

func makeConfig(name, baseURL, method, path string, schema json.RawMessage) ExecutionConfig {
	return ExecutionConfig{
		BaseURL:     baseURL,
		Method:      method,
		Path:        path,
		InputSchema: schema,
	}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// ---------------------------------------------------------------------------
// NewOpenAPIRegistry
// ---------------------------------------------------------------------------

func TestNewOpenAPIRegistry_Empty(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	assert.NotNil(t, reg)
	assert.Empty(t, reg.ListTools())
	assert.Empty(t, reg.ListCommands())

	reg2 := NewOpenAPIRegistry([]OpenAPITool{}, nil)
	assert.NotNil(t, reg2)
	assert.Empty(t, reg2.ListTools())
}

func TestNewOpenAPIRegistry_WithTools(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("getPet", "Get a pet", schemaWithParams),
		makeTool("listPets", "List all pets", emptySchema),
	}
	configs := map[string]ExecutionConfig{
		"getPet":    makeConfig("getPet", "http://example.com", "GET", "/pets/{petId}", schemaWithParams),
		"listPets": makeConfig("listPets", "http://example.com", "GET", "/pets", emptySchema),
	}

	reg := NewOpenAPIRegistry(tools, configs)
	assert.NotNil(t, reg)
	assert.Len(t, reg.ListTools(), 2)
}

// ---------------------------------------------------------------------------
// ListTools
// ---------------------------------------------------------------------------

func TestListTools_Basic(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("getPet", "Get a pet", schemaWithParams),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	result := reg.ListTools()
	require.Len(t, result, 1)
	assert.Equal(t, "getPet", result[0].Name)
	assert.Equal(t, "Get a pet", result[0].Description)
	assert.Equal(t, schemaWithParams, result[0].InputSchema)
}

func TestListTools_Empty(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	assert.Empty(t, reg.ListTools())
}

// ---------------------------------------------------------------------------
// ListCommands
// ---------------------------------------------------------------------------

func TestListCommands_Empty(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	cmds := reg.ListCommands()
	assert.Empty(t, cmds)
}

func TestListCommands_WithFlags(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("getPet", "Get a pet", schemaWithParams),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	cmds := reg.ListCommands()
	require.Len(t, cmds, 1)
	assert.Equal(t, "getPet", cmds[0].Name)
	assert.Equal(t, "Get a pet", cmds[0].Description)

	require.Len(t, cmds[0].Flags, 2)
	assert.Equal(t, "petId", cmds[0].Flags[0].Name)
	assert.Equal(t, "string", cmds[0].Flags[0].Type)
	assert.Equal(t, "ID of the pet", cmds[0].Flags[0].Description)
	assert.True(t, cmds[0].Flags[0].Required)

	assert.Equal(t, "status", cmds[0].Flags[1].Name)
	assert.Equal(t, "string", cmds[0].Flags[1].Type)
	assert.Equal(t, "Filter by status", cmds[0].Flags[1].Description)
	assert.False(t, cmds[0].Flags[1].Required)
}

func TestListCommands_SkipsBodyParam(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("createResource", "Create a resource", schemaWithBody),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	cmds := reg.ListCommands()
	require.Len(t, cmds, 1)
	require.Len(t, cmds[0].Flags, 1)
	assert.Equal(t, "name", cmds[0].Flags[0].Name)
}

func TestListCommands_WithHeader(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("getWithHeader", "Get with header", schemaWithHeader),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	cmds := reg.ListCommands()
	require.Len(t, cmds, 1)
	require.Len(t, cmds[0].Flags, 2)
	flagsByName := make(map[string]handlers.Flag)
	for _, f := range cmds[0].Flags {
		flagsByName[f.Name] = f
	}
	assert.Equal(t, "string", flagsByName["X-API-Key"].Type)
	assert.Equal(t, "integer", flagsByName["limit"].Type)
}

func TestListCommands_InvalidSchema(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("badTool", "Bad schema", jsonRaw(`{invalid json}`)),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	cmds := reg.ListCommands()
	require.Len(t, cmds, 1)
	assert.Empty(t, cmds[0].Flags)
}

// ---------------------------------------------------------------------------
// GetCommand
// ---------------------------------------------------------------------------

func TestGetCommand_Found(t *testing.T) {
	tools := []OpenAPITool{
		makeTool("getPet", "Get a pet", schemaWithParams),
	}
	reg := NewOpenAPIRegistry(tools, nil)

	cmd := reg.GetCommand("getPet")
	require.NotNil(t, cmd)
	assert.Equal(t, "getPet", cmd.Name)
	assert.Len(t, cmd.Flags, 2)
}

func TestGetCommand_NotFound(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	cmd := reg.GetCommand("nonexistent")
	assert.Nil(t, cmd)
}

func TestGetCommand_EmptyTools(t *testing.T) {
	reg := NewOpenAPIRegistry([]OpenAPITool{}, nil)
	cmd := reg.GetCommand("anything")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// ToHandlersTool
// ---------------------------------------------------------------------------

func TestToHandlersTool(t *testing.T) {
	tool := OpenAPITool{
		Name:        "getPet",
		Description: "Get a pet",
		InputSchema: schemaWithParams,
	}

	ht := tool.ToHandlersTool()
	assert.Equal(t, "getPet", ht.Name)
	assert.Equal(t, "Get a pet", ht.Description)
	assert.Equal(t, schemaWithParams, ht.InputSchema)
}

// ---------------------------------------------------------------------------
// ToolExecutor — New / ValidateArgs
// ---------------------------------------------------------------------------

func TestNewOpenAPIExecutor(t *testing.T) {
	exec := NewOpenAPIExecutor(map[string]ExecutionConfig{
		"getPet": makeConfig("getPet", "http://example.com", "GET", "/pets/{petId}", schemaWithParams),
	})
	assert.NotNil(t, exec)

	exec2 := NewOpenAPIExecutor(nil)
	assert.NotNil(t, exec2)
}

func TestValidateArgs_NoSchema(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	assert.NoError(t, exec.validateArgs(nil, map[string]interface{}{"k": "v"}))
	assert.NoError(t, exec.validateArgs(json.RawMessage{}, map[string]interface{}{"k": "v"}))
}

func TestValidateArgs_Valid(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	schema := jsonRaw(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name"]}`)
	err := exec.validateArgs(schema, map[string]interface{}{"name": "Fido", "age": float64(3)})
	assert.NoError(t, err)
}

func TestValidateArgs_MissingRequired(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	schema := jsonRaw(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	err := exec.validateArgs(schema, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestValidateArgs_WrongType(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	schema := jsonRaw(`{"type":"object","properties":{"age":{"type":"integer"}}}`)
	err := exec.validateArgs(schema, map[string]interface{}{"age": "not-a-number"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

// ---------------------------------------------------------------------------
// ToolExecutor — Execute (with HTTP test server)
// ---------------------------------------------------------------------------

func TestToolExecutor_Execute_NotFound(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	_, err := exec.Execute(context.Background(), "nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestToolExecutor_Execute_ValidationFailure(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"testTool": {
			BaseURL: "http://example.com", Method: "GET", Path: "/test",
			InputSchema: jsonRaw(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)
	_, err := exec.Execute(context.Background(), "testTool", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestToolExecutor_Execute_GetSuccess(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"testGet": {BaseURL: ts.URL, Method: "GET", Path: "/test", InputSchema: emptySchema},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "testGet", map[string]interface{}{})
	require.NoError(t, err)
	rm, _ := result.(map[string]interface{})
	assert.Equal(t, "ok", rm["status"])
}

func TestToolExecutor_Execute_HTTPError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"testGet": {BaseURL: ts.URL, Method: "GET", Path: "/test", InputSchema: emptySchema},
	}
	exec := NewOpenAPIExecutor(configs)

	_, err := exec.Execute(context.Background(), "testGet", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestToolExecutor_Execute_PostWithBody(t *testing.T) {
	var gotMethod, gotContentType string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"name":"test"}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"createTest": {
			BaseURL: ts.URL, Method: "POST", Path: "/test",
			InputSchema: jsonRaw(`{"type":"object","properties":{"name":{"type":"string"}}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "createTest", map[string]interface{}{"name": "test"})
	require.NoError(t, err)
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	rm, _ := result.(map[string]interface{})
	assert.Equal(t, "test", rm["name"])
}

func TestToolExecutor_Execute_GetWithQueryParam(t *testing.T) {
	var rawQuery string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"search": {
			BaseURL: ts.URL, Method: "GET", Path: "/search",
			InputSchema: jsonRaw(`{"type":"object","properties":{"q":{"type":"string","in":"query"}}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "search", map[string]interface{}{"q": "hello"})
	require.NoError(t, err)
	assert.Contains(t, rawQuery, "q=hello")
	rm, _ := result.(map[string]interface{})
	assert.True(t, rm["ok"].(bool))
}

func TestToolExecutor_Execute_WithPathParam(t *testing.T) {
	var pathHit string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		pathHit = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"42"}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"getById": {
			BaseURL: ts.URL, Method: "GET", Path: "/items/{itemId}",
			InputSchema: jsonRaw(`{"type":"object","properties":{"itemId":{"type":"string","in":"path"}},"required":["itemId"]}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "getById", map[string]interface{}{"itemId": "42"})
	require.NoError(t, err)
	assert.Contains(t, pathHit, "42")
	rm, _ := result.(map[string]interface{})
	assert.Equal(t, "42", rm["id"])
}

func TestToolExecutor_Execute_WithHeaderParam(t *testing.T) {
	var gotHeader string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authorized":true}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"authGet": {
			BaseURL: ts.URL, Method: "GET", Path: "/secure",
			InputSchema: jsonRaw(`{"type":"object","properties":{"X-API-Key":{"type":"string","in":"header"}}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "authGet", map[string]interface{}{"X-API-Key": "s3kr3t"})
	require.NoError(t, err)
	assert.Equal(t, "s3kr3t", gotHeader)
	rm, _ := result.(map[string]interface{})
	assert.True(t, rm["authorized"].(bool))
}

func TestToolExecutor_Execute_NonJSONResponse(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`plain text body`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"getText": {BaseURL: ts.URL, Method: "GET", Path: "/text", InputSchema: emptySchema},
	}
	exec := NewOpenAPIExecutor(configs)

	result, err := exec.Execute(context.Background(), "getText", map[string]interface{}{})
	require.NoError(t, err)
	rm, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 200, rm["status"])
	assert.Contains(t, rm["body"], "plain text body")
}

func TestToolExecutor_Execute_NonPathParamFallsBackToQuery(t *testing.T) {
	var rawQuery string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer ts.Close()

	configs := map[string]ExecutionConfig{
		"getExtra": {
			BaseURL: ts.URL, Method: "GET", Path: "/items",
			InputSchema: jsonRaw(`{"type":"object","properties":{"extra":{"type":"string","in":"query"}}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)

	_, err := exec.Execute(context.Background(), "getExtra", map[string]interface{}{"extra": "val"})
	require.NoError(t, err)
	assert.Contains(t, rawQuery, "extra=val")
}

// ---------------------------------------------------------------------------
// ExecuteCommand (ToolRegistry — returns {"executed":true})
// ---------------------------------------------------------------------------

func TestToolRegistry_ExecuteCommand_Found(t *testing.T) {
	tools := []OpenAPITool{makeTool("getPet", "Get a pet", schemaWithParams)}
	reg := NewOpenAPIRegistry(tools, nil)
	result, err := reg.ExecuteCommand(context.Background(), "getPet", map[string]interface{}{"petId": "123"})
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["executed"])
}

func TestToolRegistry_ExecuteCommand_NotFound(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	_, err := reg.ExecuteCommand(context.Background(), "nope", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// AddCommand / RemoveCommand / UpdateCommand (unsupported → error)
// ---------------------------------------------------------------------------

func TestRegistry_AddCommand_ReturnsError(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	err := reg.AddCommand(handlers.Command{Name: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestRegistry_RemoveCommand_ReturnsError(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	err := reg.RemoveCommand("test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestRegistry_UpdateCommand_ReturnsError(t *testing.T) {
	reg := NewOpenAPIRegistry(nil, nil)
	err := reg.UpdateCommand("test", handlers.Command{Name: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

// ---------------------------------------------------------------------------
// classifyParams
// ---------------------------------------------------------------------------

func TestClassifyParams_Empty(t *testing.T) {
	assert.Empty(t, classifyParams(jsonRaw(`{}`)))
	assert.Empty(t, classifyParams(jsonRaw(`{"type":"object"}`)))
}

func TestClassifyParams_ByLocation(t *testing.T) {
	schema := jsonRaw(`{
		"type": "object",
		"properties": {
			"petId":  {"type":"string","in":"path"},
			"status": {"type":"string","in":"query"},
			"X-Token":{"type":"string","in":"header"}
		}
	}`)
	r := classifyParams(schema)
	assert.Equal(t, "path", r["petId"])
	assert.Equal(t, "query", r["status"])
	assert.Equal(t, "header", r["X-Token"])
}

func TestClassifyParams_DefaultsToBody(t *testing.T) {
	schema := jsonRaw(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}}}`)
	r := classifyParams(schema)
	assert.Equal(t, "body", r["name"])
	assert.Equal(t, "body", r["age"])
}

func TestClassifyParams_NonMapProperty(t *testing.T) {
	schema := jsonRaw(`{"type":"object","properties":{"name":"justastring"}}`)
	r := classifyParams(schema)
	assert.Equal(t, "body", r["name"])
}

func TestClassifyParams_InvalidJSON(t *testing.T) {
	assert.Empty(t, classifyParams(jsonRaw(`{invalid`)))
}

// ---------------------------------------------------------------------------
// extractPathParams
// ---------------------------------------------------------------------------

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/pets/{petId}", []string{"petId"}},
		{"/users/{uid}/posts/{pid}", []string{"uid", "pid"}},
		{"/plain", nil},
		{"", nil},
		{"/{a}{b}", []string{"a", "b"}},
		{"/_{x}_", []string{"x"}},
		{"{alone}", []string{"alone"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, extractPathParams(tt.path))
		})
	}
}

// ---------------------------------------------------------------------------
// isPathParam
// ---------------------------------------------------------------------------

func TestIsPathParam(t *testing.T) {
	assert.True(t, isPathParam("/pets/{petId}", "petId"))
	assert.False(t, isPathParam("/pets/{petId}", "name"))
	assert.False(t, isPathParam("/plain", "x"))
	assert.True(t, isPathParam("/{id}", "id"))
}

// ---------------------------------------------------------------------------
// getString
// ---------------------------------------------------------------------------

func TestGetString(t *testing.T) {
	assert.Equal(t, "v", getString(map[string]interface{}{"k": "v"}, "k", "d"))
	assert.Equal(t, "d", getString(map[string]interface{}{}, "k", "d"))
	assert.Equal(t, "d", getString(map[string]interface{}{"k": 42}, "k", "d"))
	assert.Equal(t, "d", getString(nil, "k", "d"))
}

// ---------------------------------------------------------------------------
// isRequired
// ---------------------------------------------------------------------------

func TestIsRequired(t *testing.T) {
	s := map[string]interface{}{"required": []interface{}{"a", "b"}}
	assert.True(t, isRequired(s, "a"))
	assert.False(t, isRequired(s, "c"))
	assert.False(t, isRequired(map[string]interface{}{}, "x"))
	assert.False(t, isRequired(map[string]interface{}{"required": "str"}, "x"))
}

// ---------------------------------------------------------------------------
// paramTypeToJSONSchema / bodyPropTypeToJSONSchema
// ---------------------------------------------------------------------------

func TestParamTypeToJSONSchema(t *testing.T) {
	tests := []struct{ in, want string }{
		{"string", "string"}, {"integer", "integer"}, {"number", "number"},
		{"boolean", "boolean"}, {"array", "array"}, {"object", "object"},
		{"", "string"}, {"unknown", "string"},
		{"STRING", "string"}, {"Integer", "integer"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, paramTypeToJSONSchema(tt.in))
		})
	}
}

func TestBodyPropTypeToJSONSchema(t *testing.T) {
	assert.Equal(t, "string", bodyPropTypeToJSONSchema("string"))
	assert.Equal(t, "integer", bodyPropTypeToJSONSchema("integer"))
	assert.Equal(t, "string", bodyPropTypeToJSONSchema(""))
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------

func TestNewConverter(t *testing.T) {
	c := NewConverter("http://example.com")
	assert.NotNil(t, c)
	assert.Equal(t, "http://example.com", c.BaseURL)
}

func TestConverter_ConvertToTools_NilDoc(t *testing.T) {
	tools, configs, err := NewConverter("x").ConvertToTools(nil)
	assert.NoError(t, err)
	assert.Nil(t, tools)
	assert.Nil(t, configs)
}

func TestConverter_ConvertToTools_NilPaths(t *testing.T) {
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "T", Version: "1"}}
	tools, configs, err := NewConverter("x").ConvertToTools(doc)
	assert.NoError(t, err)
	assert.Nil(t, tools)
	assert.Nil(t, configs)
}

func TestConverter_ConvertToTools_WithOperationID(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{
		OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Servers: openapi3.Servers{{URL: "http://example.com"}},
		Paths:   openapi3.NewPaths(),
	}
	op := &openapi3.Operation{
		OperationID: "getPet",
		Summary:     "Get a pet by ID",
		Responses:   openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: stringPtr("OK")}})
	doc.Paths.Set("/pets/{petId}", &openapi3.PathItem{Get: op})

	tools, configs, err := c.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Len(t, configs, 1)

	assert.Equal(t, "getPet", tools[0].Name)
	assert.Equal(t, "Get a pet by ID", tools[0].Description)
	assert.Equal(t, "GET", tools[0].Method)
	assert.Equal(t, "/pets/{petId}", tools[0].Path)
	assert.Equal(t, "http://example.com", tools[0].BaseURL)
	assert.NotEmpty(t, tools[0].InputSchema)

	cfg, ok := configs["getPet"]
	require.True(t, ok)
	assert.Equal(t, "GET", cfg.Method)
	assert.Equal(t, "/pets/{petId}", cfg.Path)
}

func TestConverter_ConvertToTools_GeneratedName(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{
		OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Servers: openapi3.Servers{{URL: "http://example.com"}},
		Paths:   openapi3.NewPaths(),
	}
	postOp := &openapi3.Operation{
		Summary: "Create a pet",
		Responses: func() *openapi3.Responses {
			r := openapi3.NewResponses()
			r.Set("201", &openapi3.ResponseRef{Value: &openapi3.Response{Description: stringPtr("Created")}})
			return r
		}(),
	}
	doc.Paths.Set("/pets", &openapi3.PathItem{Post: postOp})

	tools, configs, err := c.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "post_pets", tools[0].Name)
	_, ok := configs["post_pets"]
	require.True(t, ok)
}

func TestConverter_ConvertToTools_MultipleOperations(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{
		OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Servers: openapi3.Servers{{URL: "http://example.com"}},
		Paths:   openapi3.NewPaths(),
	}
	listOp := &openapi3.Operation{
		OperationID: "listPets", Summary: "List pets",
		Responses: func() *openapi3.Responses {
			r := openapi3.NewResponses()
			r.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: stringPtr("OK")}})
			return r
		}(),
	}
	createOp := &openapi3.Operation{
		OperationID: "createPet", Summary: "Create pet",
		Responses: func() *openapi3.Responses {
			r := openapi3.NewResponses()
			r.Set("201", &openapi3.ResponseRef{Value: &openapi3.Response{Description: stringPtr("Created")}})
			return r
		}(),
	}
	doc.Paths.Set("/pets", &openapi3.PathItem{Get: listOp, Post: createOp})

	tools, configs, err := c.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Len(t, configs, 2)
	// Order depends on PathItem field iteration; check by content
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	assert.True(t, names["listPets"])
	assert.True(t, names["createPet"])
}

func TestConverter_ConvertToTools_DescriptionFallback(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{
		OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Servers: openapi3.Servers{{URL: "http://example.com"}},
		Paths:   openapi3.NewPaths(),
	}
	op := &openapi3.Operation{
		OperationID: "noDesc",
		Responses: func() *openapi3.Responses {
			r := openapi3.NewResponses()
			r.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: stringPtr("OK")}})
			return r
		}(),
	}
	doc.Paths.Set("/status", &openapi3.PathItem{Get: op})

	tools, _, err := c.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "GET /status", tools[0].Description)
}

// ---------------------------------------------------------------------------
// OpenAPITool formatting
// ---------------------------------------------------------------------------

func TestOpenAPITool_String(t *testing.T) {
	tool := makeTool("testTool", "A test tool", emptySchema)
	s := fmt.Sprintf("%v", tool)
	assert.Contains(t, s, "testTool")
}
