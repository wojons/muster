package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitializeHandler_MethodName(t *testing.T) {
	handler := NewInitializeHandler("test-server", "1.0.0")
	require.Equal(t, "initialize", handler.Method())
}

func TestInitializeHandler_Handler(t *testing.T) {
	handler := NewInitializeHandler("test-server", "1.0.0")

	params := `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}`
	result, err := handler.Handle(context.Background(), []byte(params))
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestInitializeHandler_InvalidParams(t *testing.T) {
	handler := NewInitializeHandler("test-server", "1.0.0")

	_, err := handler.Handle(context.Background(), []byte("invalid"))
	require.Error(t, err)

	if mcpErr, ok := err.(*MCPError); ok {
		require.Equal(t, -32602, mcpErr.Code)
		require.Contains(t, mcpErr.Message, "Invalid params")
	} else {
		t.Fatal("expected MCPError")
	}
}

func TestInitializedHandler(t *testing.T) {
	handler := NewInitializedHandler()
	require.NotNil(t, handler)
	require.Equal(t, "notifications/initialized", handler.Method())

	result, err := handler.Handle(context.Background(), []byte("{}"))
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestCallToolHandler(t *testing.T) {
	executor := &mockExecutor{
		results: map[string]interface{}{
			"test": "test result",
		},
	}
	handler := NewCallToolHandler(executor)

	params := `{"name":"test","arguments":{"key":"value"}}`
	result, err := handler.Handle(context.Background(), json.RawMessage(params))
	require.NoError(t, err)

	callResult, ok := result.(*CallToolResult)
	require.True(t, ok)
	require.False(t, callResult.IsError)
	require.Len(t, callResult.Content, 1)
	require.Equal(t, "text", callResult.Content[0].Type)
}

func TestCallToolHandler_WithComplexResult(t *testing.T) {
	executor := &mockExecutor{
		results: map[string]interface{}{
			"test": map[string]string{"key": "value"},
		},
	}
	handler := NewCallToolHandler(executor)

	params := `{"name":"test","arguments":{}}`
	result, err := handler.Handle(context.Background(), json.RawMessage(params))
	require.NoError(t, err)

	callResult, ok := result.(*CallToolResult)
	require.True(t, ok)
	require.False(t, callResult.IsError)
	require.Contains(t, callResult.Content[0].Text, `"key"`)
}

func TestCallToolHandler_Error(t *testing.T) {
	executor := &mockExecutor{
		err: map[string]error{
			"test": &testError{msg: "execution failed"},
		},
	}
	handler := NewCallToolHandler(executor)

	params := `{"name":"test","arguments":{}}`
	result, err := handler.Handle(context.Background(), json.RawMessage(params))
	require.NoError(t, err)

	callResult, ok := result.(*CallToolResult)
	require.True(t, ok)
	require.True(t, callResult.IsError)
	require.Contains(t, callResult.Content[0].Text, "execution failed")
}

func TestCallToolHandler_InvalidParams(t *testing.T) {
	executor := &mockExecutor{}
	handler := NewCallToolHandler(executor)

	_, err := handler.Handle(context.Background(), []byte("invalid"))
	require.Error(t, err)

	if mcpErr, ok := err.(*MCPError); ok {
		require.Equal(t, -32602, mcpErr.Code)
	}
}

func TestListResourcesHandler_Error(t *testing.T) {
	provider := &errorResourceProvider{
		err: &testError{msg: "provider error"},
	}
	handler := NewListResourcesHandler(provider)

	_, err := handler.Handle(context.Background(), []byte("{}"))
	require.Error(t, err)

	if mcpErr, ok := err.(*MCPError); ok {
		require.Equal(t, -32603, mcpErr.Code)
		require.Contains(t, mcpErr.Message, "Failed to list resources")
	}
}

func TestReadResourceHandler_NotFound(t *testing.T) {
	provider := &mockResourceProvider{
		data: map[string]*StoredResource{},
	}
	handler := NewReadResourceHandler(provider)

	req := ReadResourceRequest{URI: "file:///missing"}
	data, _ := json.Marshal(req)
	_, err := handler.Handle(context.Background(), data)
	require.Error(t, err)

	if mcpErr, ok := err.(*MCPError); ok {
		require.Equal(t, -32603, mcpErr.Code)
		require.Contains(t, mcpErr.Message, "Resource not found")
	}
}

func TestListToolsHandler_WithFlags(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{
			{
				Name:        "test",
				Description: "Test command",
				Flags: []Flag{
					{Name: "required", Type: "string", Description: "Required flag", Required: true},
					{Name: "optional", Type: "boolean", Description: "Optional flag", Required: false},
				},
			},
		},
	}
	handler := NewListToolsHandler(registry)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	require.NoError(t, err)

	listResult, ok := result.(*ListToolsResult)
	require.True(t, ok)
	require.Len(t, listResult.Tools, 1)

	var schema map[string]interface{}
	_ = json.Unmarshal(listResult.Tools[0].InputSchema, &schema)
	require.NotNil(t, schema["properties"])
	props := schema["properties"].(map[string]interface{})
	require.Contains(t, props, "required")
	require.Contains(t, props, "optional")
}

func TestListToolsHandler_InvalidParams(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{{Name: "test", Description: "Test"}},
	}
	handler := NewListToolsHandler(registry)

	_, err := handler.Handle(context.Background(), []byte("invalid"))
	require.Error(t, err)

	if mcpErr, ok := err.(*MCPError); ok {
		require.Equal(t, -32602, mcpErr.Code)
	}
}

func TestMCPError_Error(t *testing.T) {
	err := &MCPError{Code: -32600, Message: "test error", Data: "extra info"}
	str := err.Error()
	require.Contains(t, str, "-32600")
	require.Contains(t, str, "test error")
}

func TestHandleError_NonMCPError(t *testing.T) {
	innerErr := &testError{msg: "inner error"}
	result := HandleError(innerErr)

	require.NotNil(t, result)
	require.Equal(t, -32603, result.Code)
	require.Contains(t, result.Data, "inner error")
}

type mockExecutor struct {
	results map[string]interface{}
	err     map[string]error
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	if err, ok := m.err[name]; ok {
		return nil, err
	}
	return m.results[name], nil
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

type errorResourceProvider struct {
	err error
}

func (m *errorResourceProvider) ListResources() ([]ResourceInfo, error) {
	return nil, m.err
}

func (m *errorResourceProvider) GetResource(uri string) (*StoredResource, error) {
	return nil, m.err
}

type errorPromptProvider struct {
	prompts []Prompt
}

func (m *errorPromptProvider) ListPrompts() []Prompt {
	return m.prompts
}

func (m *errorPromptProvider) GetPrompt(name string, args map[string]interface{}) (string, error) {
	return "", &testError{msg: "not found"}
}
