package handlers

import (
	"context"
	"encoding/json"
)

type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type PromptProvider interface {
	ListPrompts() []Prompt
	GetPrompt(name string, args map[string]interface{}) (string, error)
}

type ListPromptsHandler struct {
	provider PromptProvider
}

func NewListPromptsHandler(provider PromptProvider) *ListPromptsHandler {
	return &ListPromptsHandler{
		provider: provider,
	}
}

func (h *ListPromptsHandler) Method() string {
	return "prompts/list"
}

func (h *ListPromptsHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req ListPromptsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	prompts := h.provider.ListPrompts()

	return &ListPromptsResult{
		Prompts: prompts,
	}, nil
}

type ListPromptsRequest struct {
	Cursor *string `json:"cursor,omitempty"`
}

type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

type GetPromptHandler struct {
	provider PromptProvider
}

func NewGetPromptHandler(provider PromptProvider) *GetPromptHandler {
	return &GetPromptHandler{
		provider: provider,
	}
}

func (h *GetPromptHandler) Method() string {
	return "prompts/get"
}

func (h *GetPromptHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req GetPromptRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	result, err := h.provider.GetPrompt(req.Name, req.Arguments)
	if err != nil {
		return nil, &MCPError{
			Code:    -32603,
			Message: "Failed to get prompt",
			Data:    err.Error(),
		}
	}

	return &GetPromptResult{
		Messages: []PromptMessage{
			{
				Role: "user",
				Content: PromptContent{
					Type: "text",
					Text: result,
				},
			},
		},
	}, nil
}

type GetPromptRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type GetPromptResult struct {
	Messages []PromptMessage `json:"messages"`
}

type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
