package handlers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("expected registry, got nil")
	}

	testHandler := &mockHandler{method: "test"}
	registry.Register(testHandler)

	if len(registry.handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(registry.handlers))
	}
}

func TestRegistryHandle(t *testing.T) {
	registry := NewRegistry()
	testHandler := &mockHandler{method: "test"}
	registry.Register(testHandler)

	result, err := registry.Handle(context.Background(), "test", []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "test-result" {
		t.Errorf("expected test-result, got %v", result)
	}

	_, err = registry.Handle(context.Background(), "unknown", []byte("{}"))
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if mcpErr, ok := err.(*MCPError); ok {
		if mcpErr.Code != -32601 {
			t.Errorf("expected error code -32601, got %d", mcpErr.Code)
		}
	} else {
		t.Error("expected MCPError")
	}
}

func TestHandleError(t *testing.T) {
	mcpErr := &MCPError{Code: -32000, Message: "test error"}
	result := HandleError(mcpErr)
	if result.Code != -32000 {
		t.Errorf("expected code -32000, got %d", result.Code)
	}

	genericErr := json.Unmarshal([]byte("invalid"), &struct{}{})
	result = HandleError(genericErr)
	if result.Code != -32603 {
		t.Errorf("expected code -32603, got %d", result.Code)
	}
}

func TestMCPError(t *testing.T) {
	err := &MCPError{Code: -32603, Message: "test"}
	str := err.Error()
	if str == "" {
		t.Error("expected error string, got empty")
	}
}

type mockHandler struct {
	method string
}

func (h *mockHandler) Method() string {
	return h.method
}

func (h *mockHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return "test-result", nil
}

func TestListToolsHandler(t *testing.T) {
	registry := &mockRegistry{
		commands: []Command{
			{Name: "test", Description: "Test command", Flags: []Flag{}},
		},
	}
	handler := NewListToolsHandler(registry)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListToolsResult)
	if !ok {
		t.Fatal("expected ListToolsResult")
	}
	if len(listResult.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(listResult.Tools))
	}
}

func TestListResourcesHandler(t *testing.T) {
	provider := &mockResourceProvider{
		resources: []ResourceInfo{
			{URI: "file:///test", Name: "Test", MimeType: "text/plain"},
		},
	}
	handler := NewListResourcesHandler(provider)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListResourcesResult)
	if !ok {
		t.Fatal("expected ListResourcesResult")
	}
	if len(listResult.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(listResult.Resources))
	}
}

func TestReadResourceHandler(t *testing.T) {
	provider := &mockResourceProvider{
		data: map[string]*StoredResource{
			"file:///test": {Data: []byte("test content"), MimeType: "text/plain"},
		},
	}
	handler := NewReadResourceHandler(provider)

	req := ReadResourceRequest{URI: "file:///test"}
	data, _ := json.Marshal(req)
	result, err := handler.Handle(context.Background(), data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	readResult, ok := result.(*ReadResourceResult)
	if !ok {
		t.Fatal("expected ReadResourceResult")
	}
	if len(readResult.Contents) != 1 {
		t.Errorf("expected 1 content item, got %d", len(readResult.Contents))
	}
}

type mockRegistry struct {
	commands []Command
}

func (m *mockRegistry) ListCommands() []Command {
	return m.commands
}

func (m *mockRegistry) GetCommand(name string) *Command {
	return nil
}

type mockResourceProvider struct {
	resources []ResourceInfo
	data      map[string]*StoredResource
}

func (m *mockResourceProvider) ListResources() ([]ResourceInfo, error) {
	return m.resources, nil
}

func (m *mockResourceProvider) GetResource(uri string) (*StoredResource, error) {
	if res, ok := m.data[uri]; ok {
		return res, nil
	}
	return nil, &MCPError{Code: -32603, Message: "Not found"}
}

func TestListPromptsHandler(t *testing.T) {
	provider := &mockPromptProvider{
		prompts: []Prompt{
			{Name: "test", Description: "Test prompt"},
		},
	}
	handler := NewListPromptsHandler(provider)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListPromptsResult)
	if !ok {
		t.Fatal("expected ListPromptsResult")
	}
	if len(listResult.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(listResult.Prompts))
	}
}

type mockPromptProvider struct {
	prompts []Prompt
}

func (m *mockPromptProvider) ListPrompts() []Prompt {
	return m.prompts
}

func (m *mockPromptProvider) GetPrompt(name string, args map[string]interface{}) (string, error) {
	return "test prompt", nil
}
