// Package handlers provides MCP protocol handler interfaces and base types.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
)

// Handler is the interface for MCP method handlers.
type Handler interface {
	// Method returns the MCP method name this handler handles.
	Method() string
	// Handle processes the request with the given parameters.
	Handle(ctx context.Context, params json.RawMessage) (interface{}, error)
}

// Registry manages MCP method handlers.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

// Register adds a handler to the registry.
func (r *Registry) Register(h Handler) {
	r.handlers[h.Method()] = h
}

// Get returns the handler for the given method, or nil if not found.
func (r *Registry) Get(method string) Handler {
	return r.handlers[method]
}

// Handle looks up the handler for the given method and delegates to it.
// Returns an error if no handler is registered for the method.
func (r *Registry) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	// Validate params is valid JSON
	if len(params) > 0 && !json.Valid(params) {
		return nil, fmt.Errorf("invalid JSON params: %s", string(params))
	}
	handler := r.Get(method)
	if handler == nil {
		return nil, fmt.Errorf("Method not found: %s", method)
	}
	return handler.Handle(ctx, params)
}

// Methods returns all registered method names.
func (r *Registry) Methods() []string {
	methods := make([]string, 0, len(r.handlers))
	for m := range r.handlers {
		methods = append(methods, m)
	}
	return methods
}

// Result types for MCP methods.

// ListToolsResult is the result of a tools/list request.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single MCP tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToHandlersTool is a no-op convenience method.
func (t Tool) ToHandlersTool() Tool { return t }

// CallToolResult is the result of a tools/call request.
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent represents content returned by a tool call.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ListPromptsResult is the result of a prompts/list request.
type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

// Prompt represents a single MCP prompt.
type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Arguments   interface{} `json:"arguments,omitempty"`
}

// PromptMessage represents a message in a prompt result.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content,omitempty"`
}

// GetPromptResult is the result of a prompts/get request.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// ListResourcesResult is the result of a resources/list request.
type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// Resource represents a single MCP resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContents represents the content of a resource.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ReadResourceResult is the result of a resources/read request.
type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

// InitializeResult is the result of an initialize request.
type InitializeResult struct {
	ProtocolVersion string      `json:"protocolVersion"`
	Capabilities    interface{} `json:"capabilities"`
	ServerInfo      ServerInfo  `json:"serverInfo"`
}

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ListRootsResult is the result of a roots/list request.
type ListRootsResult struct {
	Roots []Root `json:"roots"`
}

// Root represents a filesystem root in MCP.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// MCPError represents an error returned by an MCP method handler.
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *MCPError) Error() string {
	return e.Message
}

// Command represents an MCP command/tool definition.
type Command struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Flags       []Flag   `json:"flags,omitempty"`
	Args        []string `json:"args,omitempty"`
}

// Flag represents a command flag.
type Flag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Shorthand   string `json:"shorthand,omitempty"`
	Default     string `json:"default,omitempty"`
}

// CommandRegistry is the interface for command registries.
type CommandRegistry interface {
	ListCommands() []Command
	GetCommand(name string) *Command
	ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error)
	AddCommand(cmd Command) error
	RemoveCommand(name string) error
	UpdateCommand(name string, cmd Command) error
}

// baseHandler is a simple handler implementation.
type baseHandler struct {
	method string
	fn     func(ctx context.Context, params json.RawMessage) (interface{}, error)
}

func (h *baseHandler) Method() string { return h.method }
func (h *baseHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return h.fn(ctx, params)
}

// NewInitializeHandler creates a handler for the initialize method.
func NewInitializeHandler(serverName, serverVersion string) Handler {
	return &baseHandler{
		method: "initialize",
		fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return &InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities:    map[string]interface{}{},
				ServerInfo: ServerInfo{
					Name:    serverName,
					Version: serverVersion,
				},
			}, nil
		},
	}
}

// NewInitializedHandler creates a handler for the initialized notification.
func NewInitializedHandler() Handler {
	return &baseHandler{
		method: "notifications/initialized",
		fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return nil, nil
		},
	}
}

// ToolLister is an optional interface for registries that can directly list tools with input schemas.
type ToolLister interface {
	ListTools() []Tool
}

// NewListToolsHandler creates a handler for tools/list.
func NewListToolsHandler(registry CommandRegistry) Handler {
	return &baseHandler{
		method: "tools/list",
		fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			// If the registry implements ToolLister, use direct tool list (preserves inputSchema)
			if lister, ok := registry.(ToolLister); ok {
				return &ListToolsResult{Tools: lister.ListTools()}, nil
			}

			// Fallback: convert Commands to Tools (inputSchema will be empty)
			commands := registry.ListCommands()
			tools := make([]Tool, 0, len(commands))
			for _, cmd := range commands {
				tools = append(tools, Tool{
					Name:        cmd.Name,
					Description: cmd.Description,
				})
			}
			return &ListToolsResult{Tools: tools}, nil
		},
	}
}

// NewCallToolHandler creates a handler for tools/call.
func NewCallToolHandler(executor interface{}) Handler {
	return &baseHandler{
		method: "tools/call",
		fn: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var req struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, &MCPError{Code: -32602, Message: "Invalid params"}
			}
			// Try to call executor via duck-typed interface
			if exec, ok := executor.(interface {
				Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
			}); ok {
				result, err := exec.Execute(ctx, req.Name, req.Arguments)
				if err != nil {
					// Return error as CallToolResult with isError=true
					return &CallToolResult{
						Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
						IsError: true,
					}, nil
				}
				// Marshal result to JSON text for content
				resultJSON, err := json.Marshal(result)
				if err != nil {
					return &CallToolResult{
						Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("%v", result)}},
					}, nil
				}
				return &CallToolResult{
					Content: []ToolContent{{Type: "text", Text: string(resultJSON)}},
				}, nil
			}
			return nil, &MCPError{Code: -32603, Message: "Tool executor not available"}
		},
	}
}
