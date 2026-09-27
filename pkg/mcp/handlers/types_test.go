package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- Registry Tests ---

func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	require.NotNil(t, r.handlers)
	require.Empty(t, r.handlers)
}

func TestRegistry_Register_Get(t *testing.T) {
	r := NewRegistry()
	h := &baseHandler{method: "test/method", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return map[string]string{"ok": "true"}, nil
	}}
	r.Register(h)
	got := r.Get("test/method")
	require.NotNil(t, got)
	require.Equal(t, "test/method", got.Method())
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	got := r.Get("nonexistent")
	require.Nil(t, got)
}

func TestRegistry_Register_DuplicateOverwrites(t *testing.T) {
	r := NewRegistry()
	h1 := &baseHandler{method: "test/method", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "first", nil
	}}
	h2 := &baseHandler{method: "test/method", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "second", nil
	}}
	r.Register(h1)
	r.Register(h2)
	got := r.Get("test/method")
	result, err := got.Handle(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "second", result)
}

func TestRegistry_Methods(t *testing.T) {
	r := NewRegistry()
	r.Register(&baseHandler{method: "a"})
	r.Register(&baseHandler{method: "b"})
	r.Register(&baseHandler{method: "c"})
	names := make(map[string]bool)
	for _, m := range r.Methods() {
		names[m] = true
	}
	require.True(t, names["a"], "expected method 'a'")
	require.True(t, names["b"], "expected method 'b'")
	require.True(t, names["c"], "expected method 'c'")
}

func TestRegistry_Methods_Empty(t *testing.T) {
	r := NewRegistry()
	require.Empty(t, r.Methods())
}

func TestRegistry_Handle(t *testing.T) {
	r := NewRegistry()
	h := &baseHandler{method: "test/method", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return map[string]string{"key": "value"}, nil
	}}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test/method", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"key": "value"}, result)
}

func TestRegistry_Handle_MethodNotFound(t *testing.T) {
	r := NewRegistry()
	result, err := r.Handle(context.Background(), "nonexistent", nil)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "Method not found")
}

func TestRegistry_Handle_InvalidParams(t *testing.T) {
	r := NewRegistry()
	result, err := r.Handle(context.Background(), "any", json.RawMessage("not-json"))
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "invalid JSON params")
}

func TestRegistry_Handle_NilParams(t *testing.T) {
	r := NewRegistry()
	h := &baseHandler{method: "test", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		require.Empty(t, params, "nil/empty params should be passed as empty")
		return "ok", nil
	}}
	r.Register(h)
	result, err := r.Handle(context.Background(), "test", nil)
	require.NoError(t, err)
	require.Equal(t, "ok", result)
}

// --- Handler Constructors ---

func TestNewInitializeHandler(t *testing.T) {
	h := NewInitializeHandler("test-server", "1.0.0")
	require.Equal(t, "initialize", h.Method())

	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	initResult, ok := result.(*InitializeResult)
	require.True(t, ok)
	require.Equal(t, "2024-11-05", initResult.ProtocolVersion)
	require.Equal(t, "test-server", initResult.ServerInfo.Name)
	require.Equal(t, "1.0.0", initResult.ServerInfo.Version)
	require.NotNil(t, initResult.Capabilities)
}

// TestNewInitializeHandler_AdvertisesToolsCapability verifies the initialize
// result advertises the tools capability (DF-032). An empty capabilities map
// fails this test: clients would conclude the server serves no tools even
// though tools/list and tools/call are registered.
func TestNewInitializeHandler_AdvertisesToolsCapability(t *testing.T) {
	h := NewInitializeHandler("test-server", "1.0.0")

	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	initResult, ok := result.(*InitializeResult)
	require.True(t, ok)

	caps, ok := initResult.Capabilities.(map[string]interface{})
	require.True(t, ok, "capabilities should be a map[string]interface{}, got %T", initResult.Capabilities)

	tools, ok := caps["tools"]
	require.True(t, ok, "initialize must advertise the tools capability, got %v", caps)
	require.IsType(t, map[string]interface{}{}, tools)
}

func TestNewInitializedHandler(t *testing.T) {
	h := NewInitializedHandler()
	require.Equal(t, "notifications/initialized", h.Method())

	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, result)
}

// mockCommandRegistry implements CommandRegistry for testing
type mockCommandRegistry struct {
	commands []Command
}

func (m *mockCommandRegistry) ListCommands() []Command         { return m.commands }
func (m *mockCommandRegistry) GetCommand(name string) *Command { return nil }
func (m *mockCommandRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockCommandRegistry) AddCommand(cmd Command) error    { return fmt.Errorf("read-only") }
func (m *mockCommandRegistry) RemoveCommand(name string) error { return fmt.Errorf("read-only") }
func (m *mockCommandRegistry) UpdateCommand(name string, cmd Command) error {
	return fmt.Errorf("read-only")
}

func TestNewListToolsHandler_Empty(t *testing.T) {
	mock := &mockCommandRegistry{}
	h := NewListToolsHandler(mock)
	require.Equal(t, "tools/list", h.Method())

	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	listResult, ok := result.(*ListToolsResult)
	require.True(t, ok)
	require.Empty(t, listResult.Tools)
}

func TestNewListToolsHandler_WithCommands(t *testing.T) {
	mock := &mockCommandRegistry{
		commands: []Command{
			{Name: "list-pets", Description: "List all pets"},
			{Name: "get-pet", Description: "Get a pet by ID"},
		},
	}
	h := NewListToolsHandler(mock)
	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	listResult := result.(*ListToolsResult)
	require.Len(t, listResult.Tools, 2)

	names := make(map[string]string)
	for _, tool := range listResult.Tools {
		names[tool.Name] = tool.Description
	}
	require.Equal(t, "List all pets", names["list-pets"])
	require.Equal(t, "Get a pet by ID", names["get-pet"])
}

// toolListerRegistry implements both CommandRegistry and ToolLister
type toolListerRegistry struct {
	tools []Tool
}

func (t *toolListerRegistry) ListTools() []Tool       { return t.tools }
func (t *toolListerRegistry) ListCommands() []Command { return nil }
func (t *toolListerRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return nil, nil
}
func (t *toolListerRegistry) AddCommand(cmd Command) error                 { return nil }
func (t *toolListerRegistry) RemoveCommand(name string) error              { return nil }
func (t *toolListerRegistry) UpdateCommand(name string, cmd Command) error { return nil }
func (t *toolListerRegistry) GetCommand(name string) *Command              { return nil }

func TestNewListToolsHandler_ToolLister(t *testing.T) {
	tl := &toolListerRegistry{
		tools: []Tool{
			{Name: "list-pets", Description: "List pets", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	h := NewListToolsHandler(tl)
	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	listResult := result.(*ListToolsResult)
	require.Len(t, listResult.Tools, 1)
	require.Equal(t, "list-pets", listResult.Tools[0].Name)
	require.Equal(t, json.RawMessage(`{"type":"object"}`), listResult.Tools[0].InputSchema)
}

// --- NewCallToolHandler ---

type mockExecutor struct {
	executeResult interface{}
	executeErr    error
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	return m.executeResult, m.executeErr
}

func TestNewCallToolHandler_Success(t *testing.T) {
	exec := &mockExecutor{
		executeResult: map[string]string{"status": "ok"},
	}
	h := NewCallToolHandler(exec)
	require.Equal(t, "tools/call", h.Method())

	params := json.RawMessage(`{"name":"list-pets","arguments":{}}`)
	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult := result.(*CallToolResult)
	require.False(t, callResult.IsError)
	require.Len(t, callResult.Content, 1)
	require.Equal(t, "text", callResult.Content[0].Type)
	require.Equal(t, `{"status":"ok"}`, callResult.Content[0].Text)
}

func TestNewCallToolHandler_ExecutorError(t *testing.T) {
	exec := &mockExecutor{
		executeErr: fmt.Errorf("tool not found"),
	}
	h := NewCallToolHandler(exec)

	params := json.RawMessage(`{"name":"invalid-tool","arguments":{}}`)
	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err) // handler doesn't return error, it sets IsError
	_ = err

	callResult := result.(*CallToolResult)
	require.True(t, callResult.IsError)
	require.Contains(t, callResult.Content[0].Text, "Error: tool not found")
}

func TestNewCallToolHandler_InvalidParams(t *testing.T) {
	exec := &mockExecutor{}
	h := NewCallToolHandler(exec)

	params := json.RawMessage(`invalid`)
	_, err := h.Handle(context.Background(), params)
	require.Error(t, err)

	mcpErr, ok := err.(*MCPError)
	require.True(t, ok)
	require.Equal(t, -32602, mcpErr.Code)
}

func TestNewCallToolHandler_NoExecutor(t *testing.T) {
	// Pass a non-executor type
	h := NewCallToolHandler("not-an-executor")

	params := json.RawMessage(`{"name":"test","arguments":{}}`)
	_, err := h.Handle(context.Background(), params)
	require.Error(t, err)

	mcpErr, ok := err.(*MCPError)
	require.True(t, ok)
	require.Equal(t, -32603, mcpErr.Code)
	require.Equal(t, "Tool executor not available", mcpErr.Message)
}

func TestNewCallToolHandler_MarshalError(t *testing.T) {
	// Return a value that cannot be marshaled (channel)
	exec := &mockExecutor{
		executeResult: make(chan int),
	}
	h := NewCallToolHandler(exec)

	params := json.RawMessage(`{"name":"test","arguments":{}}`)
	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult := result.(*CallToolResult)
	require.False(t, callResult.IsError)
	require.Contains(t, callResult.Content[0].Text, "0x") // fmt.Sprintf of a channel
}

// --- Tool.ToHandlersTool ---

func TestTool_ToHandlersTool(t *testing.T) {
	tool := Tool{Name: "test-tool", Description: "desc"}
	result := tool.ToHandlersTool()
	require.Equal(t, tool.Name, result.Name)
	require.Equal(t, tool.Description, result.Description)
}

func TestTool_ToHandlersTool_Empty(t *testing.T) {
	tool := Tool{}
	result := tool.ToHandlersTool()
	require.Empty(t, result.Name)
}

// --- MCPError ---

func TestMCPError_Error(t *testing.T) {
	e := &MCPError{Code: -32600, Message: "Invalid Request"}
	require.Equal(t, "Invalid Request", e.Error())
}

// --- JSON Serialization ---

func TestListToolsResult_JSON(t *testing.T) {
	result := ListToolsResult{
		Tools: []Tool{
			{Name: "tool1", Description: "First tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(data), `"name":"tool1"`)
	require.Contains(t, string(data), `"inputSchema":{"type":"object"}`)

	var parsed ListToolsResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Len(t, parsed.Tools, 1)
	require.Equal(t, "tool1", parsed.Tools[0].Name)
}

func TestCallToolResult_JSON(t *testing.T) {
	result := CallToolResult{
		Content: []ToolContent{{Type: "text", Text: "hello"}},
		IsError: true,
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(data), `"isError":true`)

	var parsed CallToolResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.True(t, parsed.IsError)
}

func TestCallToolResult_JSON_NoIsError(t *testing.T) {
	result := CallToolResult{
		Content: []ToolContent{{Type: "text", Text: "hello"}},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(data), "isError")
}

func TestInitializeResult_JSON(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]interface{}{"tools": map[string]interface{}{"listChanged": true}},
		ServerInfo:      ServerInfo{Name: "muster", Version: "1.0.0"},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(data), `"protocolVersion":"2024-11-05"`)
	require.Contains(t, string(data), `"name":"muster"`)

	var parsed InitializeResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "muster", parsed.ServerInfo.Name)
}

func TestListResourcesResult_JSON(t *testing.T) {
	result := ListResourcesResult{
		Resources: []Resource{
			{URI: "file:///test", Name: "Test Resource", MimeType: "text/plain"},
		},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(data), `"uri":"file:///test"`)
}

func TestListPromptsResult_JSON(t *testing.T) {
	result := ListPromptsResult{
		Prompts: []Prompt{
			{Name: "greeting", Description: "A friendly greeting"},
		},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(data), `"name":"greeting"`)
}

func TestToolContent_JSON(t *testing.T) {
	tc := ToolContent{Type: "text", Text: `{"key":"value"}`}
	data, err := json.Marshal(tc)
	require.NoError(t, err)
	require.Contains(t, string(data), `"type":"text"`)

	var parsed ToolContent
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "text", parsed.Type)
}

func TestTool_JSON_OptionalFields(t *testing.T) {
	tool := Tool{Name: "minimal"}
	data, err := json.Marshal(tool)
	require.NoError(t, err)
	// Description is omitempty and should not appear
	require.NotContains(t, string(data), "description")
}

func TestCommand_JSON(t *testing.T) {
	cmd := Command{
		Name:        "list-pets",
		Description: "List all pets",
		Flags:       []Flag{{Name: "limit", Type: "int", Required: true}},
	}
	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	var parsed Command
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "list-pets", parsed.Name)
	require.Len(t, parsed.Flags, 1)
	require.True(t, parsed.Flags[0].Required)
}

func TestCommandRegistry_Interface_Satisfied(t *testing.T) {
	// Verify mockCommandRegistry satisfies the interface
	var _ CommandRegistry = (*mockCommandRegistry)(nil)
}

func TestToolLister_Interface_Satisfied(t *testing.T) {
	var _ ToolLister = (*toolListerRegistry)(nil)
}

func TestNewCallToolHandler_NonExecutableStruct(t *testing.T) {
	// Pass a struct that has Execute but with wrong signature
	type wrongExecutor struct{}
	h := NewCallToolHandler(wrongExecutor{})

	params := json.RawMessage(`{"name":"test","arguments":{}}`)
	result, err := h.Handle(context.Background(), params)
	require.Error(t, err)
	require.Nil(t, result)
	mcpErr, ok := err.(*MCPError)
	require.True(t, ok)
	require.Equal(t, -32603, mcpErr.Code)
}

func TestNewListToolsHandler_ContextCancelled(t *testing.T) {
	mock := &mockCommandRegistry{}
	h := NewListToolsHandler(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The handler doesn't check context itself but the registry could
	result, err := h.Handle(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestRegistry_Handle_MultipleHandlers(t *testing.T) {
	r := NewRegistry()
	r.Register(&baseHandler{method: "a", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "A", nil
	}})
	r.Register(&baseHandler{method: "b", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "B", nil
	}})

	result, err := r.Handle(context.Background(), "a", nil)
	require.NoError(t, err)
	require.Equal(t, "A", result)

	result, err = r.Handle(context.Background(), "b", nil)
	require.NoError(t, err)
	require.Equal(t, "B", result)
}

func TestRegistry_Handle_HandlerReturnsError(t *testing.T) {
	r := NewRegistry()
	testErr := fmt.Errorf("internal failure")
	r.Register(&baseHandler{method: "fail", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return nil, testErr
	}})

	result, err := r.Handle(context.Background(), "fail", nil)
	require.ErrorIs(t, err, testErr)
	require.Nil(t, result)
}

func TestToolContent_EmptyText(t *testing.T) {
	tc := ToolContent{Type: "image"}
	data, err := json.Marshal(tc)
	require.NoError(t, err)
	require.NotContains(t, string(data), "text") // omitempty
}

func TestNewInitializeHandler_DifferentNames(t *testing.T) {
	for _, tc := range []struct {
		name, version string
	}{
		{"server-a", "1.0.0"},
		{"server-b", "2.3.4-beta"},
		{"", ""},
	} {
		t.Run(tc.name+"_"+tc.version, func(t *testing.T) {
			h := NewInitializeHandler(tc.name, tc.version)
			result, _ := h.Handle(context.Background(), nil)
			ir := result.(*InitializeResult)
			require.Equal(t, tc.name, ir.ServerInfo.Name)
			require.Equal(t, tc.version, ir.ServerInfo.Version)
		})
	}
}

// Test map ordering safety — Handler names should be checked by membership, not position
func TestRegistry_Methods_OrderSafety(t *testing.T) {
	r := NewRegistry()
	r.Register(&baseHandler{method: "z"})
	r.Register(&baseHandler{method: "a"})

	methods := r.Methods()
	require.Len(t, methods, 2)
	names := make(map[string]bool)
	for _, m := range methods {
		names[m] = true
	}
	require.True(t, names["z"])
	require.True(t, names["a"])
}

// Verify CommandRegistry can handle empty ListCommands (no panic)
func TestCommandRegistry_EmptyCommands(t *testing.T) {
	mock := &mockCommandRegistry{}
	cmds := mock.ListCommands()
	require.Nil(t, cmds) // not initialized, returns nil

	mock2 := &mockCommandRegistry{commands: []Command{}}
	cmds2 := mock2.ListCommands()
	require.Empty(t, cmds2)
}

// Test JSON round-trip for PromptMessage
func TestPromptMessage_JSON(t *testing.T) {
	pm := PromptMessage{Role: "user", Content: "hello"}
	data, err := json.Marshal(pm)
	require.NoError(t, err)

	var parsed PromptMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "user", parsed.Role)
}

// Test JSON round-trip for ReadResourceResult
func TestReadResourceResult_JSON(t *testing.T) {
	result := ReadResourceResult{
		Contents: []ResourceContents{
			{URI: "file:///data.txt", MimeType: "text/plain", Text: "content"},
		},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed ReadResourceResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Len(t, parsed.Contents, 1)
	require.Equal(t, "content", parsed.Contents[0].Text)
}

// Test Root JSON
func TestRoot_JSON(t *testing.T) {
	root := Root{URI: "file:///root", Name: "workspace"}
	data, err := json.Marshal(root)
	require.NoError(t, err)

	var parsed Root
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "file:///root", parsed.URI)
}

// Test Flag JSON
func TestFlag_JSON(t *testing.T) {
	f := Flag{Name: "verbose", Type: "bool", Shorthand: "v", Description: "Enable verbose output"}
	data, err := json.Marshal(f)
	require.NoError(t, err)

	var parsed Flag
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, "v", parsed.Shorthand)
}

// Test NewCallToolHandler with empty params
func TestNewCallToolHandler_EmptyParams(t *testing.T) {
	exec := &mockExecutor{executeResult: "ok"}
	h := NewCallToolHandler(exec)

	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	callResult := result.(*CallToolResult)
	require.False(t, callResult.IsError)
}

// Test ListToolsResult JSON omitempty description
func TestListToolsResult_JSON_MinimalTool(t *testing.T) {
	result := ListToolsResult{
		Tools: []Tool{{Name: "minimal"}},
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(data), "description")
	// Note: InputSchema is json.RawMessage — nil serializes as "null"
	require.Contains(t, string(data), `"inputSchema":null`)
}

// Verify that ToolContent.Type always serializes
func TestToolContent_TypeAlwaysSerialized(t *testing.T) {
	tc := ToolContent{Type: "resource", Text: ""}
	data, err := json.Marshal(tc)
	require.NoError(t, err)
	require.Contains(t, string(data), `"type":"resource"`)
}

// Test Registry.Handle with empty string params (valid JSON)
func TestRegistry_Handle_EmptyStringParams(t *testing.T) {
	r := NewRegistry()
	h := &baseHandler{method: "test", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "ok", nil
	}}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test", json.RawMessage(""))
	require.NoError(t, err)
	require.Equal(t, "ok", result)
}

// --- baseHandler compliance ---
func TestBaseHandler_Interface(t *testing.T) {
	var _ Handler = (*baseHandler)(nil)
}

// Test NewListToolsHandler with nil commands
func TestNewListToolsHandler_NilCommands(t *testing.T) {
	mock := &mockCommandRegistry{commands: nil}
	h := NewListToolsHandler(mock)
	result, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)

	listResult := result.(*ListToolsResult)
	require.NotNil(t, listResult.Tools)
	require.Empty(t, listResult.Tools)
}

// Test Registry.Handle with params that are valid JSON array (should pass validation)
func TestRegistry_Handle_ArrayParams(t *testing.T) {
	r := NewRegistry()
	h := &baseHandler{method: "test", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return string(params), nil
	}}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test", json.RawMessage(`[1,2,3]`))
	require.NoError(t, err)
	require.Equal(t, "[1,2,3]", result)
}

// Test Registry.Handle passes through handler errors
func TestRegistry_Handle_PropagatesHandlerError(t *testing.T) {
	r := NewRegistry()
	expected := &MCPError{Code: -32600, Message: "custom error"}
	r.Register(&baseHandler{method: "err", fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return nil, expected
	}})

	result, err := r.Handle(context.Background(), "err", nil)
	require.ErrorIs(t, err, expected)
	require.Nil(t, result)
}

// Verify json.Valid check properly rejects non-JSON
func TestRegistry_Handle_InvalidJSON_EdgeCases(t *testing.T) {
	r := NewRegistry()

	tests := []struct {
		name, params string
	}{
		{"unquoted", `{foo: bar}`},
		{"trailing_comma", `{"a":1,}`},
		{"single_quotes", `{'a':1}`},
		{"incomplete", `{"a":`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := r.Handle(context.Background(), "any", json.RawMessage(tc.params))
			require.Error(t, err)
			require.Nil(t, result)
			require.True(t, strings.Contains(err.Error(), "invalid JSON params"), "expected invalid JSON params error for %q", tc.params)
		})
	}
}

// Test NewCallToolHandler with nil params
func TestNewCallToolHandler_NilParams(t *testing.T) {
	exec := &mockExecutor{}
	h := NewCallToolHandler(exec)

	result, err := h.Handle(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, result)
	mcpErr, ok := err.(*MCPError)
	require.True(t, ok)
	require.Equal(t, -32602, mcpErr.Code)
}
