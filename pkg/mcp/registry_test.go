package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

// --- NewOpenAPIRegistry ---

func TestNewOpenAPIRegistry_WithTools(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "tool1", Description: "First tool", Method: "GET", Path: "/items", BaseURL: "http://example.com", InputSchema: json.RawMessage(`{}`)},
		{Name: "tool2", Description: "Second tool", Method: "POST", Path: "/items", BaseURL: "http://example.com", InputSchema: json.RawMessage(`{}`)},
	}
	configs := map[string]ExecutionConfig{
		"tool1": {BaseURL: "http://example.com", Method: "GET", Path: "/items"},
	}
	registry := NewOpenAPIRegistry(tools, configs)
	if registry == nil {
		t.Fatal("NewOpenAPIRegistry returned nil")
	}
	if len(registry.tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(registry.tools))
	}
	if registry.configs == nil {
		t.Fatal("configs is nil")
	}
}

func TestNewOpenAPIRegistry_EmptyTools(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	if registry == nil {
		t.Fatal("NewOpenAPIRegistry returned nil")
	}
	if len(registry.tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(registry.tools))
	}
}

func TestNewOpenAPIRegistry_NilConfigs(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "tool1", Description: "Test"},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	if len(registry.tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(registry.tools))
	}
}

// --- ListCommands ---

func TestToolRegistry_ListCommands_Populated(t *testing.T) {
	tools := []OpenAPITool{
		{
			Name: "list-items", Description: "List all items",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","description":"Max items"}}}`),
		},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	commands := registry.ListCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].Name != "list-items" {
		t.Errorf("command name = %q, want %q", commands[0].Name, "list-items")
	}
}

func TestToolRegistry_ListCommands_Empty(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	commands := registry.ListCommands()
	if len(commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(commands))
	}
}

func TestToolRegistry_ListCommands_WithFlags(t *testing.T) {
	tools := []OpenAPITool{
		{
			Name: "search",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"}},"required":["query"]}`),
		},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	commands := registry.ListCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	cmd := commands[0]
	if len(cmd.Flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(cmd.Flags))
	}
	if cmd.Flags[0].Name != "query" {
		t.Errorf("flag name = %q, want %q", cmd.Flags[0].Name, "query")
	}
	if !cmd.Flags[0].Required {
		t.Error("flag should be required")
	}
}

// --- GetCommand ---

func TestToolRegistry_GetCommand_Found(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "get-item", Description: "Get an item", InputSchema: json.RawMessage(`{}`)},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	cmd := registry.GetCommand("get-item")
	if cmd == nil {
		t.Fatal("GetCommand returned nil for existing tool")
	}
	if cmd.Name != "get-item" {
		t.Errorf("name = %q, want %q", cmd.Name, "get-item")
	}
}

func TestToolRegistry_GetCommand_NotFound(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	cmd := registry.GetCommand("nonexistent")
	if cmd != nil {
		t.Error("GetCommand should return nil for unknown tool")
	}
}

func TestToolRegistry_GetCommand_WithFlags(t *testing.T) {
	tools := []OpenAPITool{
		{
			Name: "update-item",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","in":"path","description":"Item ID"},"name":{"type":"string","description":"New name"}},"required":["id"]}`),
		},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	cmd := registry.GetCommand("update-item")
	if cmd == nil {
		t.Fatal("GetCommand returned nil")
	}
	// body properties are skipped in ListCommands/GetCommand, non-required "name" should appear
	if len(cmd.Flags) < 1 {
		t.Fatalf("expected at least 1 flag, got %d", len(cmd.Flags))
	}
}

// --- ListTools ---

func TestToolRegistry_ListTools_Populated(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "tool-a", Description: "Tool A", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "tool-b", Description: "Tool B", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	result := registry.ListTools()
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	// Map-derived slice — use name-set membership check (Go map iteration is non-deterministic)
	names := make(map[string]bool)
	for _, tool := range result {
		names[tool.Name] = true
	}
	if !names["tool-a"] || !names["tool-b"] {
		t.Errorf("missing expected tool names in result: %v", names)
	}
}

func TestToolRegistry_ListTools_Empty(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	result := registry.ListTools()
	if len(result) != 0 {
		t.Errorf("expected 0 tools, got %d", len(result))
	}
}

// --- NewOpenAPIExecutor ---

func TestNewOpenAPIExecutor_WithConfigs(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"tool1": {BaseURL: "http://example.com", Method: "GET", Path: "/api"},
	}
	exec := NewOpenAPIExecutor(configs)
	if exec == nil {
		t.Fatal("NewOpenAPIExecutor returned nil")
	}
	if len(exec.configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(exec.configs))
	}
}

func TestNewOpenAPIExecutor_EmptyConfigs(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	if exec == nil {
		t.Fatal("NewOpenAPIExecutor returned nil")
	}
	if len(exec.configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(exec.configs))
	}
}

// --- Execute ---

func TestToolExecutor_Execute_UnknownTool(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	_, err := exec.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestToolExecutor_Execute_WithHTTPServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	configs := map[string]ExecutionConfig{
		"test-tool": {
			BaseURL:     server.URL,
			Method:      "GET",
			Path:        "/api/data",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)
	result, err := exec.Execute(context.Background(), "test-tool", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if resultMap["status"] != "ok" {
		t.Errorf("status = %v, want ok", resultMap["status"])
	}
}

func TestToolExecutor_Execute_ContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configs := map[string]ExecutionConfig{
		"test-tool": {
			BaseURL:     server.URL,
			Method:      "GET",
			Path:        "/api/data",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	exec := NewOpenAPIExecutor(configs)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err := exec.Execute(ctx, "test-tool", nil)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// --- ExecuteCommand ---

func TestToolRegistry_ExecuteCommand_Found(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "test-cmd", Description: "Test"},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	result, err := registry.ExecuteCommand(context.Background(), "test-cmd", nil)
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	if resultMap["executed"] != true {
		t.Error("expected executed=true")
	}
}

func TestToolRegistry_ExecuteCommand_NotFound(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	_, err := registry.ExecuteCommand(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// --- AddCommand / RemoveCommand / UpdateCommand ---

func TestToolRegistry_AddCommand(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	err := registry.AddCommand(handlers.Command{Name: "test"})
	if err == nil {
		t.Fatal("AddCommand should return error (not supported)")
	}
}

func TestToolRegistry_RemoveCommand(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	err := registry.RemoveCommand("test")
	if err == nil {
		t.Fatal("RemoveCommand should return error (not supported)")
	}
}

func TestToolRegistry_UpdateCommand(t *testing.T) {
	registry := NewOpenAPIRegistry(nil, nil)
	err := registry.UpdateCommand("test", handlers.Command{})
	if err == nil {
		t.Fatal("UpdateCommand should return error (not supported)")
	}
}

// --- Converter ---

func TestNewConverter(t *testing.T) {
	c := NewConverter("http://example.com")
	if c == nil {
		t.Fatal("NewConverter returned nil")
	}
	if c.BaseURL != "http://example.com" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, "http://example.com")
	}
}

func TestNewConverter_EmptyBaseURL(t *testing.T) {
	c := NewConverter("")
	if c == nil {
		t.Fatal("NewConverter returned nil")
	}
	if c.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty", c.BaseURL)
	}
}

func TestConverter_ConvertToTools_NilDoc(t *testing.T) {
	c := NewConverter("http://example.com")
	tools, configs, err := c.ConvertToTools(nil)
	if err != nil {
		t.Fatalf("ConvertToTools(nil) returned error: %v", err)
	}
	if tools != nil {
		t.Error("expected nil tools for nil doc")
	}
	if configs != nil {
		t.Error("expected nil configs for nil doc")
	}
}

func TestConverter_ConvertToTools_NilPaths(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{Paths: nil}
	tools, configs, err := c.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools returned error: %v", err)
	}
	if tools != nil {
		t.Error("expected nil tools for nil Paths")
	}
	if configs != nil {
		t.Error("expected nil configs for nil Paths")
	}
}

func TestConverter_ConvertToTools_ValidSimpleSpec(t *testing.T) {
	c := NewConverter("http://example.com")
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Paths: &openapi3.Paths{
			Extensions: map[string]interface{}{},
		},
	}
	doc.Paths.Set("/items", &openapi3.PathItem{
		Get: &openapi3.Operation{
			OperationID: "listItems",
			Summary:     "List all items",
			Parameters: openapi3.Parameters{
				{Value: &openapi3.Parameter{Name: "limit", In: "query", Description: "Max items", Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}}},
			},
		},
	})
	tools, configs, err := c.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "listItems" {
		t.Errorf("tool name = %q, want %q", tools[0].Name, "listItems")
	}
	if tools[0].BaseURL != "http://example.com" {
		t.Errorf("baseURL = %q", tools[0].BaseURL)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
}

// --- classifyParams ---

func TestClassifyParams(t *testing.T) {
	t.Run("path parameter", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","in":"path"}}}`)
		result := classifyParams(schema)
		if result["id"] != "path" {
			t.Errorf("id = %q, want path", result["id"])
		}
	})

	t.Run("query parameter", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","in":"query"}}}`)
		result := classifyParams(schema)
		if result["q"] != "query" {
			t.Errorf("q = %q, want query", result["q"])
		}
	})

	t.Run("header parameter", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object","properties":{"x-api-key":{"type":"string","in":"header"}}}`)
		result := classifyParams(schema)
		if result["x-api-key"] != "header" {
			t.Errorf("x-api-key = %q, want header", result["x-api-key"])
		}
	})

	t.Run("body parameter (no in field)", func(t *testing.T) {
		schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
		result := classifyParams(schema)
		if result["name"] != "body" {
			t.Errorf("name = %q, want body", result["name"])
		}
	})

	t.Run("empty schema", func(t *testing.T) {
		result := classifyParams(json.RawMessage(`{}`))
		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		result := classifyParams(json.RawMessage(`not json`))
		if len(result) != 0 {
			t.Errorf("expected empty for invalid JSON, got %v", result)
		}
	})
}

// --- extractPathParams ---

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		expect []string
	}{
		{"single param", "/items/{id}", []string{"id"}},
		{"multiple params", "/users/{userId}/posts/{postId}", []string{"userId", "postId"}},
		{"no params", "/api/v1/status", nil},
		{"unclosed brace", "/items/{id", nil},
		{"empty path", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPathParams(tt.path)
			if len(result) != len(tt.expect) {
				t.Fatalf("extractPathParams(%q) = %v, want %v", tt.path, result, tt.expect)
			}
			for i, p := range tt.expect {
				if result[i] != p {
					t.Errorf("param[%d] = %q, want %q", i, result[i], p)
				}
			}
		})
	}
}

// --- isPathParam ---

func TestIsPathParam(t *testing.T) {
	if !isPathParam("/items/{id}", "id") {
		t.Error("isPathParam should return true for matching param")
	}
	if isPathParam("/items/{id}", "name") {
		t.Error("isPathParam should return false for non-matching param")
	}
	if isPathParam("/items", "id") {
		t.Error("isPathParam should return false when path has no params")
	}
}

// --- paramTypeToJSONSchema ---

func TestParamTypeToJSONSchema(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"string", "string"},
		{"STRING", "string"},
		{"integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"array", "array"},
		{"object", "object"},
		{"unknown", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := paramTypeToJSONSchema(tt.input)
			if got != tt.want {
				t.Errorf("paramTypeToJSONSchema(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- bodyPropTypeToJSONSchema ---

func TestBodyPropTypeToJSONSchema(t *testing.T) {
	if bodyPropTypeToJSONSchema("integer") != "integer" {
		t.Error("bodyPropTypeToJSONSchema should delegate to paramTypeToJSONSchema")
	}
}

// --- getString ---

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"name": "hello",
		"num":  42,
	}

	if getString(m, "name", "default") != "hello" {
		t.Error("getString should return string value")
	}
	if getString(m, "num", "default") != "default" {
		t.Error("getString should return default for non-string value")
	}
	if getString(m, "missing", "fallback") != "fallback" {
		t.Error("getString should return default for missing key")
	}
}

// --- isRequired ---

func TestIsRequired(t *testing.T) {
	schema := map[string]interface{}{
		"required": []interface{}{"name", "email"},
	}

	if !isRequired(schema, "name") {
		t.Error("isRequired should return true for required field")
	}
	if !isRequired(schema, "email") {
		t.Error("isRequired should return true for required field")
	}
	if isRequired(schema, "phone") {
		t.Error("isRequired should return false for optional field")
	}

	noReqSchema := map[string]interface{}{}
	if isRequired(noReqSchema, "name") {
		t.Error("isRequired should return false when no required array")
	}
}

// --- validateArgs ---

func TestToolExecutor_ValidateArgs_EmptySchema(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	err := exec.validateArgs(json.RawMessage(`{}`), map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("validateArgs with empty schema should not error: %v", err)
	}
}

func TestToolExecutor_ValidateArgs_NoSchema(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	err := exec.validateArgs(nil, map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("validateArgs with nil schema should not error: %v", err)
	}
}

func TestToolExecutor_ValidateArgs_InvalidSchema(t *testing.T) {
	exec := NewOpenAPIExecutor(nil)
	err := exec.validateArgs(json.RawMessage(`not json`), map[string]interface{}{"key": "value"})
	if err == nil {
		t.Fatal("validateArgs with invalid schema should return error")
	}
}

// --- OpenAPITool ToHandlersTool ---

func TestOpenAPITool_ToHandlersTool(t *testing.T) {
	tool := OpenAPITool{
		Name:        "test-tool",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	ht := tool.ToHandlersTool()
	if ht.Name != "test-tool" {
		t.Errorf("Name = %q, want test-tool", ht.Name)
	}
	if ht.Description != "A test tool" {
		t.Errorf("Description = %q", ht.Description)
	}
	if string(ht.InputSchema) != `{"type":"object"}` {
		t.Errorf("InputSchema = %s", string(ht.InputSchema))
	}
}

// --- OpenAPITool implements ToolLister ---

func TestToolRegistry_ImplementsToolLister(t *testing.T) {
	tools := []OpenAPITool{
		{Name: "tool1"},
	}
	registry := NewOpenAPIRegistry(tools, nil)
	_, ok := interface{}(registry).(handlers.ToolLister)
	if !ok {
		t.Fatal("ToolRegistry should implement handlers.ToolLister")
	}
}
