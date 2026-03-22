package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Mock prompt provider for testing
type mockTestPromptProvider struct {
	prompts     []Prompt
	getPromptFn func(name string, args map[string]interface{}) (string, error)
}

func (m *mockTestPromptProvider) ListPrompts() []Prompt {
	return m.prompts
}

func (m *mockTestPromptProvider) GetPrompt(name string, args map[string]interface{}) (string, error) {
	if m.getPromptFn != nil {
		return m.getPromptFn(name, args)
	}
	return "test prompt content", nil
}

func TestNewListPromptsHandler(t *testing.T) {
	provider := &mockTestPromptProvider{}
	handler := NewListPromptsHandler(provider)

	if handler == nil {
		t.Fatal("expected handler, got nil")
	}

	if handler.Method() != "prompts/list" {
		t.Errorf("expected method 'prompts/list', got %s", handler.Method())
	}

	if handler.provider != provider {
		t.Error("expected provider to be set")
	}
}

func TestListPromptsHandler_Handle_Success(t *testing.T) {
	provider := &mockTestPromptProvider{
		prompts: []Prompt{
			{
				Name:        "generate-code",
				Description: "Generate code based on description",
				Arguments: []PromptArgument{
					{
						Name:        "language",
						Description: "Programming language",
						Required:    true,
					},
					{
						Name:        "description",
						Description: "Code description",
						Required:    true,
					},
				},
			},
			{
				Name:        "explain-code",
				Description: "Explain code snippet",
				Arguments: []PromptArgument{
					{
						Name:        "code",
						Description: "Code to explain",
						Required:    true,
					},
				},
			},
		},
	}
	handler := NewListPromptsHandler(provider)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListPromptsResult)
	if !ok {
		t.Fatalf("expected ListPromptsResult, got %T", result)
	}

	if len(listResult.Prompts) != 2 {
		t.Errorf("expected 2 prompts, got %d", len(listResult.Prompts))
	}

	// Verify first prompt
	if listResult.Prompts[0].Name != "generate-code" {
		t.Errorf("expected prompt name 'generate-code', got %s", listResult.Prompts[0].Name)
	}

	if len(listResult.Prompts[0].Arguments) != 2 {
		t.Errorf("expected 2 arguments, got %d", len(listResult.Prompts[0].Arguments))
	}

	// Verify second prompt
	if listResult.Prompts[1].Name != "explain-code" {
		t.Errorf("expected prompt name 'explain-code', got %s", listResult.Prompts[1].Name)
	}
}

func TestListPromptsHandler_Handle_EmptyList(t *testing.T) {
	provider := &mockTestPromptProvider{
		prompts: []Prompt{},
	}
	handler := NewListPromptsHandler(provider)

	result, err := handler.Handle(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListPromptsResult)
	if !ok {
		t.Fatalf("expected ListPromptsResult, got %T", result)
	}

	if len(listResult.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(listResult.Prompts))
	}
}

func TestListPromptsHandler_Handle_InvalidParams(t *testing.T) {
	provider := &mockTestPromptProvider{}
	handler := NewListPromptsHandler(provider)

	_, err := handler.Handle(context.Background(), []byte("invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatalf("expected MCPError, got %T", err)
	}

	if mcpErr.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", mcpErr.Code)
	}

	if mcpErr.Message != "Invalid params" {
		t.Errorf("expected message 'Invalid params', got %s", mcpErr.Message)
	}
}

func TestListPromptsHandler_Handle_WithCursor(t *testing.T) {
	provider := &mockTestPromptProvider{
		prompts: []Prompt{
			{Name: "test", Description: "Test prompt"},
		},
	}
	handler := NewListPromptsHandler(provider)

	req := ListPromptsRequest{Cursor: stringPtr("next-page")}
	data, _ := json.Marshal(req)

	result, err := handler.Handle(context.Background(), data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListPromptsResult)
	if !ok {
		t.Fatalf("expected ListPromptsResult, got %T", result)
	}

	if len(listResult.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(listResult.Prompts))
	}
}

func TestNewGetPromptHandler(t *testing.T) {
	provider := &mockTestPromptProvider{}
	handler := NewGetPromptHandler(provider)

	if handler == nil {
		t.Fatal("expected handler, got nil")
	}

	if handler.Method() != "prompts/get" {
		t.Errorf("expected method 'prompts/get', got %s", handler.Method())
	}

	if handler.provider != provider {
		t.Error("expected provider to be set")
	}
}

func TestGetPromptHandler_Handle_Success(t *testing.T) {
	provider := &mockTestPromptProvider{
		getPromptFn: func(name string, args map[string]interface{}) (string, error) {
			if name != "generate-code" {
				t.Errorf("expected prompt name 'generate-code', got %s", name)
			}
			if args["language"] != "go" {
				t.Errorf("expected language 'go', got %v", args["language"])
			}
			return "Generated Go code here", nil
		},
	}
	handler := NewGetPromptHandler(provider)

	req := GetPromptRequest{
		Name: "generate-code",
		Arguments: map[string]interface{}{
			"language":    "go",
			"description": "A simple HTTP server",
		},
	}
	data, _ := json.Marshal(req)

	result, err := handler.Handle(context.Background(), data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	getResult, ok := result.(*GetPromptResult)
	if !ok {
		t.Fatalf("expected GetPromptResult, got %T", result)
	}

	if len(getResult.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(getResult.Messages))
	}

	msg := getResult.Messages[0]
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got %s", msg.Role)
	}

	if msg.Content.Type != "text" {
		t.Errorf("expected content type 'text', got %s", msg.Content.Type)
	}

	if msg.Content.Text != "Generated Go code here" {
		t.Errorf("expected text 'Generated Go code here', got %s", msg.Content.Text)
	}
}

func TestGetPromptHandler_Handle_NoArguments(t *testing.T) {
	provider := &mockTestPromptProvider{
		getPromptFn: func(name string, args map[string]interface{}) (string, error) {
			return "Prompt without args", nil
		},
	}
	handler := NewGetPromptHandler(provider)

	req := GetPromptRequest{
		Name:      "simple-prompt",
		Arguments: nil,
	}
	data, _ := json.Marshal(req)

	result, err := handler.Handle(context.Background(), data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	getResult, ok := result.(*GetPromptResult)
	if !ok {
		t.Fatalf("expected GetPromptResult, got %T", result)
	}

	if len(getResult.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(getResult.Messages))
	}
}

func TestGetPromptHandler_Handle_InvalidParams(t *testing.T) {
	provider := &mockTestPromptProvider{}
	handler := NewGetPromptHandler(provider)

	_, err := handler.Handle(context.Background(), []byte("not valid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatalf("expected MCPError, got %T", err)
	}

	if mcpErr.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", mcpErr.Code)
	}
}

func TestGetPromptHandler_Handle_ProviderError(t *testing.T) {
	provider := &mockTestPromptProvider{
		getPromptFn: func(name string, args map[string]interface{}) (string, error) {
			return "", errors.New("prompt not found")
		},
	}
	handler := NewGetPromptHandler(provider)

	req := GetPromptRequest{Name: "unknown-prompt"}
	data, _ := json.Marshal(req)

	_, err := handler.Handle(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for provider failure")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatalf("expected MCPError, got %T", err)
	}

	if mcpErr.Code != -32603 {
		t.Errorf("expected error code -32603, got %d", mcpErr.Code)
	}

	if mcpErr.Message != "Failed to get prompt" {
		t.Errorf("expected message 'Failed to get prompt', got %s", mcpErr.Message)
	}

	if mcpErr.Data != "prompt not found" {
		t.Errorf("expected data 'prompt not found', got %v", mcpErr.Data)
	}
}

func TestPrompt_Struct(t *testing.T) {
	prompt := Prompt{
		Name:        "test-prompt",
		Description: "A test prompt",
		Arguments: []PromptArgument{
			{
				Name:        "arg1",
				Description: "First argument",
				Required:    true,
			},
		},
	}

	data, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("failed to marshal prompt: %v", err)
	}

	var unmarshaled Prompt
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal prompt: %v", err)
	}

	if unmarshaled.Name != prompt.Name {
		t.Errorf("expected name %s, got %s", prompt.Name, unmarshaled.Name)
	}

	if unmarshaled.Description != prompt.Description {
		t.Errorf("expected description %s, got %s", prompt.Description, unmarshaled.Description)
	}

	if len(unmarshaled.Arguments) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(unmarshaled.Arguments))
	}

	if unmarshaled.Arguments[0].Name != "arg1" {
		t.Errorf("expected argument name 'arg1', got %s", unmarshaled.Arguments[0].Name)
	}

	if !unmarshaled.Arguments[0].Required {
		t.Error("expected argument to be required")
	}
}

func TestPromptArgument_Struct(t *testing.T) {
	arg := PromptArgument{
		Name:        "test-arg",
		Description: "Test argument description",
		Required:    false,
	}

	data, err := json.Marshal(arg)
	if err != nil {
		t.Fatalf("failed to marshal argument: %v", err)
	}

	var unmarshaled PromptArgument
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal argument: %v", err)
	}

	if unmarshaled.Name != arg.Name {
		t.Errorf("expected name %s, got %s", arg.Name, unmarshaled.Name)
	}

	if unmarshaled.Required != arg.Required {
		t.Error("expected Required to match")
	}
}

func TestListPromptsRequest_Struct(t *testing.T) {
	cursor := "page-2"
	req := ListPromptsRequest{
		Cursor: &cursor,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	var unmarshaled ListPromptsRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if unmarshaled.Cursor == nil {
		t.Fatal("expected cursor to be set")
	}

	if *unmarshaled.Cursor != cursor {
		t.Errorf("expected cursor %s, got %s", cursor, *unmarshaled.Cursor)
	}
}

func TestListPromptsResult_Struct(t *testing.T) {
	result := ListPromptsResult{
		Prompts: []Prompt{
			{Name: "prompt1", Description: "First prompt"},
			{Name: "prompt2", Description: "Second prompt"},
		},
		NextCursor: "next-page-token",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var unmarshaled ListPromptsResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(unmarshaled.Prompts) != 2 {
		t.Errorf("expected 2 prompts, got %d", len(unmarshaled.Prompts))
	}

	if unmarshaled.NextCursor != result.NextCursor {
		t.Errorf("expected next cursor %s, got %s", result.NextCursor, unmarshaled.NextCursor)
	}
}

func TestGetPromptRequest_Struct(t *testing.T) {
	req := GetPromptRequest{
		Name: "my-prompt",
		Arguments: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	var unmarshaled GetPromptRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if unmarshaled.Name != req.Name {
		t.Errorf("expected name %s, got %s", req.Name, unmarshaled.Name)
	}

	if unmarshaled.Arguments["key1"] != "value1" {
		t.Errorf("expected key1 to be 'value1', got %v", unmarshaled.Arguments["key1"])
	}

	if unmarshaled.Arguments["key2"] != float64(42) {
		t.Errorf("expected key2 to be 42, got %v", unmarshaled.Arguments["key2"])
	}
}

func TestGetPromptResult_Struct(t *testing.T) {
	result := GetPromptResult{
		Messages: []PromptMessage{
			{
				Role: "user",
				Content: PromptContent{
					Type: "text",
					Text: "Hello, world!",
				},
			},
			{
				Role: "assistant",
				Content: PromptContent{
					Type: "text",
					Text: "Hi there!",
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var unmarshaled GetPromptResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(unmarshaled.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(unmarshaled.Messages))
	}

	if unmarshaled.Messages[0].Role != "user" {
		t.Errorf("expected first role 'user', got %s", unmarshaled.Messages[0].Role)
	}

	if unmarshaled.Messages[1].Role != "assistant" {
		t.Errorf("expected second role 'assistant', got %s", unmarshaled.Messages[1].Role)
	}

	if unmarshaled.Messages[0].Content.Text != "Hello, world!" {
		t.Errorf("expected first content 'Hello, world!', got %s", unmarshaled.Messages[0].Content.Text)
	}
}

func TestPromptMessage_Struct(t *testing.T) {
	msg := PromptMessage{
		Role: "system",
		Content: PromptContent{
			Type: "text",
			Text: "System message",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var unmarshaled PromptMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if unmarshaled.Role != msg.Role {
		t.Errorf("expected role %s, got %s", msg.Role, unmarshaled.Role)
	}

	if unmarshaled.Content.Type != msg.Content.Type {
		t.Errorf("expected content type %s, got %s", msg.Content.Type, unmarshaled.Content.Type)
	}
}

func TestPromptContent_Struct(t *testing.T) {
	content := PromptContent{
		Type: "text",
		Text: "Sample content",
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("failed to marshal content: %v", err)
	}

	var unmarshaled PromptContent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}

	if unmarshaled.Type != content.Type {
		t.Errorf("expected type %s, got %s", content.Type, unmarshaled.Type)
	}

	if unmarshaled.Text != content.Text {
		t.Errorf("expected text %s, got %s", content.Text, unmarshaled.Text)
	}
}

func TestPromptProvider_Interface(t *testing.T) {
	// Verify that mockTestPromptProvider implements PromptProvider
	var _ PromptProvider = &mockTestPromptProvider{}
}

func TestListPromptsHandler_ConcurrentAccess(t *testing.T) {
	provider := &mockTestPromptProvider{
		prompts: []Prompt{
			{Name: "prompt1", Description: "First"},
			{Name: "prompt2", Description: "Second"},
		},
	}
	handler := NewListPromptsHandler(provider)

	// Run multiple concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result, err := handler.Handle(context.Background(), []byte("{}"))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			listResult, ok := result.(*ListPromptsResult)
			if !ok {
				t.Errorf("expected ListPromptsResult, got %T", result)
			}
			if len(listResult.Prompts) != 2 {
				t.Errorf("expected 2 prompts, got %d", len(listResult.Prompts))
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetPromptHandler_ConcurrentAccess(t *testing.T) {
	provider := &mockTestPromptProvider{
		getPromptFn: func(name string, args map[string]interface{}) (string, error) {
			return "concurrent test", nil
		},
	}
	handler := NewGetPromptHandler(provider)

	req := GetPromptRequest{
		Name: "test",
		Arguments: map[string]interface{}{
			"key": "value",
		},
	}
	data, _ := json.Marshal(req)

	// Run multiple concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result, err := handler.Handle(context.Background(), data)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			getResult, ok := result.(*GetPromptResult)
			if !ok {
				t.Errorf("expected GetPromptResult, got %T", result)
			}
			if len(getResult.Messages) != 1 {
				t.Errorf("expected 1 message, got %d", len(getResult.Messages))
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestListPromptsHandler_Handle_ContextCancellation(t *testing.T) {
	provider := &mockTestPromptProvider{
		prompts: []Prompt{
			{Name: "test", Description: "Test"},
		},
	}
	handler := NewListPromptsHandler(provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// The handler doesn't check context cancellation, but we test it doesn't panic
	_, err := handler.Handle(ctx, []byte("{}"))
	// This will succeed because the handler doesn't check context
	// In a real implementation, you might want to check context
	if err != nil {
		t.Logf("Handler returned error with cancelled context: %v", err)
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
