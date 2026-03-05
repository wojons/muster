package handlers

import (
	"context"
	"encoding/json"
	"fmt"
)

type Command struct {
	Name        string
	Description string
	Flags       []Flag
}

type Flag struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

type CommandRegistry interface {
	ListCommands() []Command
	GetCommand(name string) *Command
}

type CommandExecutor interface {
	Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
}

type ListToolsHandler struct {
	registry CommandRegistry
}

func NewListToolsHandler(registry CommandRegistry) *ListToolsHandler {
	return &ListToolsHandler{
		registry: registry,
	}
}

func (h *ListToolsHandler) Method() string {
	return "tools/list"
}

func (h *ListToolsHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req ListToolsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	commands := h.registry.ListCommands()

	tools := make([]Tool, 0, len(commands))
	for _, cmd := range commands {
		tool := Tool{
			Name:        cmd.Name,
			Description: cmd.Description,
			InputSchema: h.generateInputSchema(cmd),
		}
		tools = append(tools, tool)
	}

	return &ListToolsResult{Tools: tools}, nil
}

func (h *ListToolsHandler) generateInputSchema(cmd Command) json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	for _, flag := range cmd.Flags {
		prop := map[string]interface{}{
			"type":        flag.Type,
			"description": flag.Description,
		}
		schema["properties"].(map[string]interface{})[flag.Name] = prop

		if flag.Required {
			schema["required"] = append(schema["required"].([]string), flag.Name)
		}
	}

	data, _ := json.Marshal(schema)
	return data
}

type ListToolsRequest struct {
	Cursor *string `json:"cursor,omitempty"`
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type CallToolHandler struct {
	executor CommandExecutor
}

func NewCallToolHandler(executor CommandExecutor) *CallToolHandler {
	return &CallToolHandler{
		executor: executor,
	}
}

func (h *CallToolHandler) Method() string {
	return "tools/call"
}

func (h *CallToolHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	result, err := h.executor.Execute(ctx, req.Name, req.Arguments)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []ToolContent{
				{Type: "text", Text: fmt.Sprintf("Error: %v", err)},
			},
		}, nil
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		output = []byte(fmt.Sprintf("%v", result))
	}

	return &CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: string(output)},
		},
	}, nil
}

type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
