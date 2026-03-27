package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type mockCommandExecutor struct {
	commands map[string]interface{}
	errors   map[string]error
}

func newMockCommandExecutor() *mockCommandExecutor {
	return &mockCommandExecutor{
		commands: make(map[string]interface{}),
		errors:   make(map[string]error),
	}
}

func (m *mockCommandExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	if err, ok := m.errors[name]; ok {
		return nil, err
	}
	if result, ok := m.commands[name]; ok {
		return result, nil
	}
	return nil, nil
}

func TestListToolsHandler_Method(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)
	method := handler.Method()
	if method != "tools/list" {
		t.Errorf("expected method 'tools/list', got '%s'", method)
	}
}

func TestListToolsHandler_Handle_EmptyRegistry(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)

	ctx := context.Background()
	result, err := handler.Handle(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListToolsResult)
	if !ok {
		t.Fatal("expected ListToolsResult")
	}

	if len(listResult.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(listResult.Tools))
	}
}

func TestListToolsHandler_Handle_WithCommands(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{
			{
				Name:        "get-pets",
				Description: "Retrieve all pets",
				Flags: []Flag{
					{Name: "limit", Type: "integer", Description: "Max results", Required: false},
					{Name: "status", Type: "string", Description: "Filter by status", Required: true},
				},
			},
			{
				Name:        "create-pet",
				Description: "Create a new pet",
				Flags: []Flag{
					{Name: "name", Type: "string", Description: "Pet name", Required: true},
					{Name: "type", Type: "string", Description: "Pet type", Required: true},
				},
			},
		},
	}
	handler := NewListToolsHandler(registry)

	ctx := context.Background()
	result, err := handler.Handle(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListToolsResult)
	if !ok {
		t.Fatal("expected ListToolsResult")
	}

	if len(listResult.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(listResult.Tools))
	}

	if listResult.Tools[0].Name != "get-pets" {
		t.Errorf("expected first tool name 'get-pets', got '%s'", listResult.Tools[0].Name)
	}

	if listResult.Tools[1].Name != "create-pet" {
		t.Errorf("expected second tool name 'create-pet', got '%s'", listResult.Tools[1].Name)
	}
}

func TestListToolsHandler_Handle_InvalidJSON(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)

	ctx := context.Background()
	_, err := handler.Handle(ctx, json.RawMessage("invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatal("expected MCPError")
	}

	if mcpErr.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", mcpErr.Code)
	}
}

func TestListToolsHandler_GenerateInputSchema_NoFlags(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)

	cmd := Command{
		Name:        "simple-command",
		Description: "A simple command",
		Flags:       []Flag{},
	}

	schema := handler.generateInputSchema(cmd)

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	if schemaMap["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", schemaMap["type"])
	}

	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	if len(properties) != 0 {
		t.Errorf("expected 0 properties, got %d", len(properties))
	}
}

func TestListToolsHandler_GenerateInputSchema_WithFlags(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)

	cmd := Command{
		Name:        "complex-command",
		Description: "A complex command",
		Flags: []Flag{
			{Name: "verbose", Type: "boolean", Description: "Verbose output", Required: false},
			{Name: "count", Type: "integer", Description: "Number of items", Required: true},
			{Name: "format", Type: "string", Description: "Output format", Required: true},
		},
	}

	schema := handler.generateInputSchema(cmd)

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	if len(properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(properties))
	}

	required, ok := schemaMap["required"].([]interface{})
	if !ok {
		t.Fatal("expected required to be a slice")
	}

	if len(required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(required))
	}

	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}

	if !requiredSet["count"] {
		t.Error("expected 'count' to be required")
	}
	if !requiredSet["format"] {
		t.Error("expected 'format' to be required")
	}
	if requiredSet["verbose"] {
		t.Error("expected 'verbose' to not be required")
	}
}

func TestListToolsHandler_GenerateInputSchema_PropertyStructure(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)

	cmd := Command{
		Name:        "test-command",
		Description: "Test command",
		Flags: []Flag{
			{Name: "flag1", Type: "string", Description: "Test flag", Required: true},
		},
	}

	schema := handler.generateInputSchema(cmd)

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	flag1, ok := properties["flag1"].(map[string]interface{})
	if !ok {
		t.Fatal("expected flag1 to be a map")
	}

	if flag1["type"] != "string" {
		t.Errorf("expected type 'string', got '%v'", flag1["type"])
	}

	if flag1["description"] != "Test flag" {
		t.Errorf("expected description 'Test flag', got '%v'", flag1["description"])
	}
}

func TestCallToolHandler_Method(t *testing.T) {
	executor := newMockCommandExecutor()
	handler := NewCallToolHandler(executor)
	method := handler.Method()
	if method != "tools/call" {
		t.Errorf("expected method 'tools/call', got '%s'", method)
	}
}

func TestCallToolHandler_Handle_SuccessfulExecution(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["test-tool"] = map[string]interface{}{
		"result": "success",
		"data":   []int{1, 2, 3},
	}

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name: "test-tool",
		Arguments: map[string]interface{}{
			"arg1": "value1",
			"arg2": 42,
		},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if callResult.IsError {
		t.Error("expected IsError to be false")
	}

	if len(callResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResult.Content))
	}

	if callResult.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got '%s'", callResult.Content[0].Type)
	}

	if !strings.Contains(callResult.Content[0].Text, "success") {
		t.Errorf("expected content to contain 'success', got: %s", callResult.Content[0].Text)
	}
}

func TestCallToolHandler_Handle_ExecutionError(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.errors["failing-tool"] = context.DeadlineExceeded

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "failing-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error (error should be in result), got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !callResult.IsError {
		t.Error("expected IsError to be true")
	}

	if len(callResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResult.Content))
	}

	if !strings.Contains(callResult.Content[0].Text, "Error:") {
		t.Errorf("expected content to contain 'Error:', got: %s", callResult.Content[0].Text)
	}
}

func TestCallToolHandler_Handle_InvalidJSON(t *testing.T) {
	executor := newMockCommandExecutor()
	handler := NewCallToolHandler(executor)

	ctx := context.Background()
	_, err := handler.Handle(ctx, json.RawMessage("invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatal("expected MCPError")
	}

	if mcpErr.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", mcpErr.Code)
	}
}

func TestCallToolHandler_Handle_NilResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["nil-tool"] = nil

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "nil-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if callResult.IsError {
		t.Error("expected IsError to be false")
	}
}

func TestCallToolHandler_Handle_ComplexResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["complex-tool"] = map[string]interface{}{
		"users": []map[string]interface{}{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		},
		"total": 2,
	}

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name: "complex-tool",
		Arguments: map[string]interface{}{
			"filter": "active",
		},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !strings.Contains(callResult.Content[0].Text, "Alice") {
		t.Error("expected content to contain 'Alice'")
	}
	if !strings.Contains(callResult.Content[0].Text, "Bob") {
		t.Error("expected content to contain 'Bob'")
	}
}

func TestCallToolHandler_Handle_StringResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["string-tool"] = "simple string result"

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "string-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !strings.Contains(callResult.Content[0].Text, "simple string result") {
		t.Errorf("expected content to contain 'simple string result', got: %s", callResult.Content[0].Text)
	}
}

func TestCallToolHandler_Handle_NumericResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["number-tool"] = 42

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "number-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !strings.Contains(callResult.Content[0].Text, "42") {
		t.Errorf("expected content to contain '42', got: %s", callResult.Content[0].Text)
	}
}

func TestCallToolHandler_Handle_BooleanResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["bool-tool"] = true

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "bool-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !strings.Contains(callResult.Content[0].Text, "true") {
		t.Errorf("expected content to contain 'true', got: %s", callResult.Content[0].Text)
	}
}

func TestCallToolHandler_Handle_ArrayResult(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["array-tool"] = []int{1, 2, 3, 4, 5}

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "array-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	content := callResult.Content[0].Text
	if !strings.Contains(content, "1") || !strings.Contains(content, "2") ||
		!strings.Contains(content, "3") || !strings.Contains(content, "4") ||
		!strings.Contains(content, "5") {
		t.Errorf("expected content to contain array elements, got: %s", content)
	}
}

func TestCallToolRequest_MarshalJSON(t *testing.T) {
	req := CallToolRequest{
		Name: "test-tool",
		Arguments: map[string]interface{}{
			"arg1": "value1",
			"arg2": 123,
			"arg3": true,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled CallToolRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Name != req.Name {
		t.Errorf("name mismatch: expected %s, got %s", req.Name, unmarshaled.Name)
	}
}

func TestCallToolResult_MarshalJSON(t *testing.T) {
	result := &CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "Test content"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled CallToolResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(unmarshaled.Content))
	}
}

func TestToolContent_MarshalJSON(t *testing.T) {
	content := ToolContent{
		Type: "text",
		Text: "Sample text",
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"type":"text","text":"Sample text"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestTool_MarshalJSON(t *testing.T) {
	tool := Tool{
		Name:        "test-tool",
		Description: "Test tool description",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Tool
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Name != tool.Name {
		t.Errorf("name mismatch: expected %s, got %s", tool.Name, unmarshaled.Name)
	}
	if unmarshaled.Description != tool.Description {
		t.Errorf("description mismatch: expected %s, got %s", tool.Description, unmarshaled.Description)
	}
}

func TestListToolsResult_MarshalJSON(t *testing.T) {
	result := &ListToolsResult{
		Tools: []Tool{
			{
				Name:        "tool1",
				Description: "First tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		NextCursor: "cursor123",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ListToolsResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(unmarshaled.Tools))
	}
	if unmarshaled.NextCursor != "cursor123" {
		t.Errorf("expected cursor 'cursor123', got '%s'", unmarshaled.NextCursor)
	}
}

func TestNewListToolsHandler_CreatesCorrectInstance(t *testing.T) {
	registry := &mockRegistry{commands: []Command{}}
	handler := NewListToolsHandler(registry)
	if handler == nil {
		t.Fatal("expected handler instance, got nil")
	}
	if handler.registry != registry {
		t.Error("expected registry to be set")
	}
}

func TestNewCallToolHandler_CreatesCorrectInstance(t *testing.T) {
	executor := newMockCommandExecutor()
	handler := NewCallToolHandler(executor)
	if handler == nil {
		t.Fatal("expected handler instance, got nil")
	}
	if handler.executor != executor {
		t.Error("expected executor to be set")
	}
}

func TestListToolsHandler_Handle_WithCursor(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{
			{Name: "tool1", Description: "Tool 1"},
			{Name: "tool2", Description: "Tool 2"},
		},
	}
	handler := NewListToolsHandler(registry)

	req := ListToolsRequest{
		Cursor: ptrString("cursor123"),
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListToolsResult)
	if !ok {
		t.Fatal("expected ListToolsResult")
	}

	if len(listResult.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(listResult.Tools))
	}
}

func ptrString(s string) *string {
	return &s
}

func TestCallToolHandler_Handle_ContextCancellation(t *testing.T) {
	executor := &mockCommandExecutorWithContext{
		checkContext: true,
	}

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "context-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error (error should be in result), got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if !callResult.IsError {
		t.Error("expected IsError to be true for cancelled context")
	}
}

type mockCommandExecutorWithContext struct {
	checkContext bool
}

func (m *mockCommandExecutorWithContext) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	if m.checkContext {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return map[string]interface{}{"executed": name}, nil
}

func TestListToolsHandler_SingleCommand(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{
			{
				Name:        "single-tool",
				Description: "Only tool",
				Flags:       []Flag{},
			},
		},
	}
	handler := NewListToolsHandler(registry)

	ctx := context.Background()
	result, err := handler.Handle(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListToolsResult)
	if !ok {
		t.Fatal("expected ListToolsResult")
	}

	if len(listResult.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(listResult.Tools))
	}

	if listResult.Tools[0].Name != "single-tool" {
		t.Errorf("expected tool name 'single-tool', got '%s'", listResult.Tools[0].Name)
	}
}

func TestCallToolHandler_Handle_EmptyArguments(t *testing.T) {
	executor := newMockCommandExecutor()
	executor.commands["no-args-tool"] = "success"

	handler := NewCallToolHandler(executor)

	req := CallToolRequest{
		Name:      "no-args-tool",
		Arguments: map[string]interface{}{},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	callResult, ok := result.(*CallToolResult)
	if !ok {
		t.Fatal("expected CallToolResult")
	}

	if callResult.IsError {
		t.Error("expected IsError to be false")
	}
}
