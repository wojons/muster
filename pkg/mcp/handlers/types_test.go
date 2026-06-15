package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/mcp/handlers"
)

// --- Mock implementations ---

// mockHandler implements handlers.Handler for registry tests.
type mockHandler struct {
	method string
	result interface{}
	err    error
}

func (h *mockHandler) Method() string {
	return h.method
}

func (h *mockHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return h.result, h.err
}

// mockCommandRegistry implements handlers.CommandRegistry for testing the
// NewListToolsHandler fallback path (non-ToolLister).
type mockCommandRegistry struct {
	commands []handlers.Command
}

func (m *mockCommandRegistry) ListCommands() []handlers.Command {
	return m.commands
}

func (m *mockCommandRegistry) GetCommand(name string) *handlers.Command {
	for _, c := range m.commands {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

func (m *mockCommandRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return "executed", nil
}

func (m *mockCommandRegistry) AddCommand(cmd handlers.Command) error {
	return nil
}

func (m *mockCommandRegistry) RemoveCommand(name string) error {
	return nil
}

func (m *mockCommandRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return nil
}

// mockToolLister implements both handlers.CommandRegistry and handlers.ToolLister.
// This allows testing the ToolLister fast path in NewListToolsHandler.
type mockToolLister struct {
	commands []handlers.Command
	tools    []handlers.Tool
}

func (m *mockToolLister) ListCommands() []handlers.Command {
	return m.commands
}

func (m *mockToolLister) GetCommand(name string) *handlers.Command {
	return nil
}

func (m *mockToolLister) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func (m *mockToolLister) AddCommand(cmd handlers.Command) error {
	return nil
}

func (m *mockToolLister) RemoveCommand(name string) error {
	return nil
}

func (m *mockToolLister) UpdateCommand(name string, cmd handlers.Command) error {
	return nil
}

func (m *mockToolLister) ListTools() []handlers.Tool {
	return m.tools
}

// mockExecutor implements the duck-typed Execute interface expected by NewCallToolHandler.
type mockExecutor struct {
	result interface{}
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	return m.result, m.err
}

// =============================================================================
// Registry tests
// =============================================================================

func TestRegistry_NewRegistry(t *testing.T) {
	r := handlers.NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.Methods())
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := handlers.NewRegistry()
	h := &mockHandler{method: "test/method", result: "hello"}
	r.Register(h)

	got := r.Get("test/method")
	require.NotNil(t, got)
	assert.Equal(t, "test/method", got.Method())
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := handlers.NewRegistry()
	got := r.Get("nonexistent")
	assert.Nil(t, got)
}

func TestRegistry_Handle_Success(t *testing.T) {
	r := handlers.NewRegistry()
	h := &mockHandler{method: "test/method", result: "success"}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test/method", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestRegistry_Handle_NoHandler(t *testing.T) {
	r := handlers.NewRegistry()
	result, err := r.Handle(context.Background(), "unknown", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Method not found")
}

func TestRegistry_Handle_InvalidParams(t *testing.T) {
	r := handlers.NewRegistry()
	result, err := r.Handle(context.Background(), "test", json.RawMessage(`{invalid}`))
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid JSON params")
}

func TestRegistry_Handle_NilParams(t *testing.T) {
	r := handlers.NewRegistry()
	h := &mockHandler{method: "test/method", result: "ok"}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test/method", nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestRegistry_Handle_EmptyParams(t *testing.T) {
	r := handlers.NewRegistry()
	h := &mockHandler{method: "test/method", result: "ok"}
	r.Register(h)

	result, err := r.Handle(context.Background(), "test/method", json.RawMessage{})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestRegistry_Methods(t *testing.T) {
	r := handlers.NewRegistry()
	r.Register(&mockHandler{method: "a"})
	r.Register(&mockHandler{method: "b"})
	r.Register(&mockHandler{method: "c"})

	methods := r.Methods()
	assert.Len(t, methods, 3)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, methods)
}

func TestRegistry_Methods_Empty(t *testing.T) {
	r := handlers.NewRegistry()
	methods := r.Methods()
	assert.Empty(t, methods)
}

func TestRegistry_Get_AfterMultipleRegistrations(t *testing.T) {
	r := handlers.NewRegistry()
	r.Register(&mockHandler{method: "tools/list", result: "list"})
	r.Register(&mockHandler{method: "tools/call", result: "call"})

	h := r.Get("tools/list")
	require.NotNil(t, h)
	assert.Equal(t, "tools/list", h.Method())

	h = r.Get("tools/call")
	require.NotNil(t, h)
	assert.Equal(t, "tools/call", h.Method())

	// unregistered returns nil
	h = r.Get("tools/unknown")
	assert.Nil(t, h)
}

func TestRegistry_Register_Overwrite(t *testing.T) {
	r := handlers.NewRegistry()
	r.Register(&mockHandler{method: "test", result: "first"})
	r.Register(&mockHandler{method: "test", result: "second"})

	result, err := r.Handle(context.Background(), "test", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "second", result)
}

func TestRegistry_Handle_HandlerError(t *testing.T) {
	r := handlers.NewRegistry()
	r.Register(&mockHandler{method: "err/method", result: nil, err: errors.New("handler error")})

	result, err := r.Handle(context.Background(), "err/method", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "handler error")
}

// =============================================================================
// Tool tests
// =============================================================================

func TestTool_ToHandlersTool(t *testing.T) {
	tool := handlers.Tool{
		Name:        "test-tool",
		Description: "A test tool description",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	result := tool.ToHandlersTool()
	assert.Equal(t, tool, result)
}

func TestTool_ToHandlersTool_Empty(t *testing.T) {
	tool := handlers.Tool{}
	result := tool.ToHandlersTool()
	assert.Equal(t, tool, result)
}

// =============================================================================
// MCPError tests
// =============================================================================

func TestMCPError_Error(t *testing.T) {
	err := &handlers.MCPError{Code: -32602, Message: "Invalid params"}
	assert.Equal(t, "Invalid params", err.Error())
}

func TestMCPError_ImplementsErrorInterface(t *testing.T) {
	var err error = &handlers.MCPError{Code: -32600, Message: "bad request"}
	assert.Equal(t, "bad request", err.Error())
}

func TestMCPError_EmptyMessage(t *testing.T) {
	err := &handlers.MCPError{Code: 0}
	assert.Equal(t, "", err.Error())
}

// =============================================================================
// NewInitializeHandler tests
// =============================================================================

func TestNewInitializeHandler_Method(t *testing.T) {
	h := handlers.NewInitializeHandler("test-server", "1.0.0")
	assert.Equal(t, "initialize", h.Method())
}

func TestNewInitializeHandler_Result(t *testing.T) {
	h := handlers.NewInitializeHandler("my-server", "2.1.0")
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	initResult, ok := result.(*handlers.InitializeResult)
	require.True(t, ok)
	assert.Equal(t, "2024-11-05", initResult.ProtocolVersion)
	assert.Equal(t, "my-server", initResult.ServerInfo.Name)
	assert.Equal(t, "2.1.0", initResult.ServerInfo.Version)
	assert.NotNil(t, initResult.Capabilities)
}

func TestNewInitializeHandler_ServerInfo_EmptyValues(t *testing.T) {
	h := handlers.NewInitializeHandler("", "")
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	initResult, ok := result.(*handlers.InitializeResult)
	require.True(t, ok)
	assert.Equal(t, "", initResult.ServerInfo.Name)
	assert.Equal(t, "", initResult.ServerInfo.Version)
}

// =============================================================================
// NewInitializedHandler tests
// =============================================================================

func TestNewInitializedHandler_Method(t *testing.T) {
	h := handlers.NewInitializedHandler()
	assert.Equal(t, "notifications/initialized", h.Method())
}

func TestNewInitializedHandler_Result(t *testing.T) {
	h := handlers.NewInitializedHandler()
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestNewInitializedHandler_WithParams(t *testing.T) {
	h := handlers.NewInitializedHandler()
	// The handler ignores params entirely
	result, err := h.Handle(context.Background(), json.RawMessage(`{"some":"data"}`))
	require.NoError(t, err)
	assert.Nil(t, result)
}

// =============================================================================
// NewListToolsHandler tests
// =============================================================================

func TestNewListToolsHandler_WithToolLister(t *testing.T) {
	tools := []handlers.Tool{
		{Name: "tool1", Description: "desc1", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "tool2", Description: "desc2", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
	}
	registry := &mockToolLister{tools: tools}

	h := handlers.NewListToolsHandler(registry)
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	listResult, ok := result.(*handlers.ListToolsResult)
	require.True(t, ok)
	assert.Len(t, listResult.Tools, 2)
	assert.Equal(t, "tool1", listResult.Tools[0].Name)
	assert.Equal(t, `{"type":"object"}`, string(listResult.Tools[0].InputSchema))
	assert.Equal(t, "tool2", listResult.Tools[1].Name)
}

func TestNewListToolsHandler_WithoutToolLister(t *testing.T) {
	commands := []handlers.Command{
		{Name: "cmd1", Description: "First command"},
		{Name: "cmd2", Description: "Second command"},
	}
	registry := &mockCommandRegistry{commands: commands}

	h := handlers.NewListToolsHandler(registry)
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	listResult, ok := result.(*handlers.ListToolsResult)
	require.True(t, ok)
	assert.Len(t, listResult.Tools, 2)
	assert.Equal(t, "cmd1", listResult.Tools[0].Name)
	assert.Equal(t, "First command", listResult.Tools[0].Description)
	assert.Equal(t, "cmd2", listResult.Tools[1].Name)
	assert.Equal(t, "Second command", listResult.Tools[1].Description)
}

func TestNewListToolsHandler_EmptyRegistry(t *testing.T) {
	registry := &mockCommandRegistry{}

	h := handlers.NewListToolsHandler(registry)
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	listResult, ok := result.(*handlers.ListToolsResult)
	require.True(t, ok)
	assert.Empty(t, listResult.Tools)
}

func TestNewListToolsHandler_ToolLister_Empty(t *testing.T) {
	registry := &mockToolLister{}

	h := handlers.NewListToolsHandler(registry)
	result, err := h.Handle(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	listResult, ok := result.(*handlers.ListToolsResult)
	require.True(t, ok)
	assert.Empty(t, listResult.Tools)
}

// =============================================================================
// NewCallToolHandler tests
// =============================================================================

func TestNewCallToolHandler_InvalidParams(t *testing.T) {
	h := handlers.NewCallToolHandler(nil)
	result, err := h.Handle(context.Background(), json.RawMessage(`{invalid}`))

	mcpErr, ok := err.(*handlers.MCPError)
	require.True(t, ok)
	assert.Equal(t, -32602, mcpErr.Code)
	assert.Equal(t, "Invalid params", mcpErr.Message)
	assert.Nil(t, result)
}

func TestNewCallToolHandler_ExecutorSuccess(t *testing.T) {
	executor := &mockExecutor{result: map[string]interface{}{"status": "ok", "value": 42}, err: nil}
	h := handlers.NewCallToolHandler(executor)
	params := json.RawMessage(`{"name":"test-tool","arguments":{"key":"value"}}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	assert.Equal(t, "text", callResult.Content[0].Type)
	assert.Contains(t, callResult.Content[0].Text, `"status":"ok"`)
	assert.False(t, callResult.IsError)
}

func TestNewCallToolHandler_ExecutorError(t *testing.T) {
	executor := &mockExecutor{err: errors.New("execution failed")}
	h := handlers.NewCallToolHandler(executor)
	params := json.RawMessage(`{"name":"test-tool","arguments":{}}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	assert.True(t, callResult.IsError)
	assert.Contains(t, callResult.Content[0].Text, "Error: execution failed")
}

func TestNewCallToolHandler_ExecutorNotAvailable(t *testing.T) {
	h := handlers.NewCallToolHandler("not-an-executor")
	params := json.RawMessage(`{"name":"test-tool","arguments":{}}`)

	result, err := h.Handle(context.Background(), params)

	mcpErr, ok := err.(*handlers.MCPError)
	require.True(t, ok)
	assert.Equal(t, -32603, mcpErr.Code)
	assert.Equal(t, "Tool executor not available", mcpErr.Message)
	assert.Nil(t, result)
}

func TestNewCallToolHandler_ResultMarshalError(t *testing.T) {
	// Channels cannot be marshaled to JSON, triggering the fallback formatting.
	executor := &mockExecutor{result: make(chan int), err: nil}
	h := handlers.NewCallToolHandler(executor)
	params := json.RawMessage(`{"name":"test-tool","arguments":{}}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	assert.Equal(t, "text", callResult.Content[0].Type)
	// Should contain the channel pointer representation (fallback formatting).
	assert.NotEmpty(t, callResult.Content[0].Text)
	assert.False(t, callResult.IsError)
}

func TestNewCallToolHandler_MissingArguments(t *testing.T) {
	executor := &mockExecutor{result: "done", err: nil}
	h := handlers.NewCallToolHandler(executor)
	// Name provided but no arguments field — Arguments will be nil.
	params := json.RawMessage(`{"name":"test-tool"}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	assert.Contains(t, callResult.Content[0].Text, "done")
}

func TestNewCallToolHandler_ExecutorStringResult(t *testing.T) {
	executor := &mockExecutor{result: "simple string result", err: nil}
	h := handlers.NewCallToolHandler(executor)
	params := json.RawMessage(`{"name":"test-tool","arguments":{}}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	// A string result marshals to JSON as a quoted string.
	assert.Contains(t, callResult.Content[0].Text, "simple string result")
}

func TestNewCallToolHandler_ExecutorNilResult(t *testing.T) {
	executor := &mockExecutor{result: nil, err: nil}
	h := handlers.NewCallToolHandler(executor)
	params := json.RawMessage(`{"name":"test-tool","arguments":{}}`)

	result, err := h.Handle(context.Background(), params)
	require.NoError(t, err)

	callResult, ok := result.(*handlers.CallToolResult)
	require.True(t, ok)
	require.Len(t, callResult.Content, 1)
	// json.Marshal(nil) produces "null"
	assert.Equal(t, "null", callResult.Content[0].Text)
}

// =============================================================================
// Struct construction / JSON round-trip tests
// =============================================================================

func TestStructs_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name: "ListToolsResult",
			input: &handlers.ListToolsResult{
				Tools: []handlers.Tool{{Name: "t1", Description: "d1", InputSchema: json.RawMessage(`{}`)}},
			},
			expected: `{"tools":[{"name":"t1","description":"d1","inputSchema":{}}]}`,
		},
		{
			name: "CallToolResult with isError",
			input: &handlers.CallToolResult{
				Content: []handlers.ToolContent{{Type: "text", Text: "error"}},
				IsError: true,
			},
			expected: `{"content":[{"type":"text","text":"error"}],"isError":true}`,
		},
		{
			name: "InitializeResult",
			input: &handlers.InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities:    map[string]interface{}{},
				ServerInfo:      handlers.ServerInfo{Name: "srv", Version: "1.0"},
			},
			expected: `{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"srv","version":"1.0"}}`,
		},
		{
			name: "MCPError",
			input: &handlers.MCPError{
				Code:    -32700,
				Message: "Parse error",
			},
			expected: `{"code":-32700,"message":"Parse error"}`,
		},
		{
			name: "ListPromptsResult",
			input: &handlers.ListPromptsResult{
				Prompts: []handlers.Prompt{{Name: "p1", Description: "prompt 1"}},
			},
			expected: `{"prompts":[{"name":"p1","description":"prompt 1"}]}`,
		},
		{
			name: "ListResourcesResult",
			input: &handlers.ListResourcesResult{
				Resources: []handlers.Resource{{URI: "file:///tmp", Name: "tmp", MimeType: "text/plain"}},
			},
			expected: `{"resources":[{"uri":"file:///tmp","name":"tmp","mimeType":"text/plain"}]}`,
		},
		{
			name: "ReadResourceResult",
			input: &handlers.ReadResourceResult{
				Contents: []handlers.ResourceContents{{URI: "file:///tmp/f", Text: "hello"}},
			},
			expected: `{"contents":[{"uri":"file:///tmp/f","text":"hello"}]}`,
		},
		{
			name: "ListRootsResult",
			input: &handlers.ListRootsResult{
				Roots: []handlers.Root{{URI: "file:///", Name: "root"}},
			},
			expected: `{"roots":[{"uri":"file:///","name":"root"}]}`,
		},
		{
			name: "Command with Flags",
			input: &handlers.Command{
				Name:        "test-cmd",
				Description: "Test command",
				Flags: []handlers.Flag{
					{Name: "verbose", Type: "bool", Description: "Verbose output", Shorthand: "v"},
				},
			},
			expected: `{"name":"test-cmd","description":"Test command","flags":[{"name":"verbose","type":"bool","description":"Verbose output","shorthand":"v"}]}`,
		},
		{
			name: "GetPromptResult",
			input: &handlers.GetPromptResult{
				Description: "Test prompt",
				Messages:    []handlers.PromptMessage{{Role: "user", Content: "hello"}},
			},
			expected: `{"description":"Test prompt","messages":[{"role":"user","content":"hello"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

// =============================================================================
// Struct zero-value / default tests
// =============================================================================

func TestStructs_ZeroValues(t *testing.T) {
	t.Run("Tool zero value", func(t *testing.T) {
		var tool handlers.Tool
		assert.Empty(t, tool.Name)
		assert.Empty(t, tool.Description)
		assert.Nil(t, tool.InputSchema)
	})

	t.Run("CallToolResult zero value", func(t *testing.T) {
		var ctr handlers.CallToolResult
		assert.Empty(t, ctr.Content)
		assert.False(t, ctr.IsError)
	})

	t.Run("ToolContent zero value", func(t *testing.T) {
		var tc handlers.ToolContent
		assert.Empty(t, tc.Type)
		assert.Empty(t, tc.Text)
	})

	t.Run("InitializeResult zero value", func(t *testing.T) {
		var ir handlers.InitializeResult
		assert.Empty(t, ir.ProtocolVersion)
		assert.Nil(t, ir.Capabilities)
		assert.Empty(t, ir.ServerInfo.Name)
	})

	t.Run("ServerInfo zero value", func(t *testing.T) {
		var si handlers.ServerInfo
		assert.Empty(t, si.Name)
		assert.Empty(t, si.Version)
	})

	t.Run("Flag zero value", func(t *testing.T) {
		var f handlers.Flag
		assert.Empty(t, f.Name)
		assert.Empty(t, f.Type)
		assert.False(t, f.Required)
	})

	t.Run("Command zero value", func(t *testing.T) {
		var c handlers.Command
		assert.Empty(t, c.Name)
		assert.Empty(t, c.Flags)
		assert.Empty(t, c.Args)
	})

	t.Run("MCPError zero value", func(t *testing.T) {
		var e handlers.MCPError
		assert.Equal(t, 0, e.Code)
		assert.Empty(t, e.Message)
	})

	t.Run("Prompt zero value", func(t *testing.T) {
		var p handlers.Prompt
		assert.Empty(t, p.Name)
		assert.Nil(t, p.Arguments)
	})

	t.Run("Resource zero value", func(t *testing.T) {
		var r handlers.Resource
		assert.Empty(t, r.URI)
		assert.Empty(t, r.MimeType)
	})

	t.Run("Root zero value", func(t *testing.T) {
		var r handlers.Root
		assert.Empty(t, r.URI)
		assert.Empty(t, r.Name)
	})
}
