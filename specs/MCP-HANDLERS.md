# MCP Handlers Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: MCP Handlers

## Overview

Complete specification of **MCP protocol handlers** for processing JSON-RPC requests and mapping CLI commands to MCP tools.

## 1. Handler Architecture

```go
type MCPHandler interface {
    Method() string
    Handle(ctx context.Context, params json.RawMessage) (interface{}, error)
}

type HandlerRegistry struct {
    handlers map[string]MCPHandler
}

func (r *HandlerRegistry) Register(handler MCPHandler) {
    r.handlers[handler.Method()] = handler
}

func (r *HandlerRegistry) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
    handler, ok := r.handlers[method]
    if !ok {
        return nil, &MCPError{Code: -32601, Message: "Method not found"}
    }
    return handler.Handle(ctx, params)
}
```

## 2. Tool Handlers

### 2.1 List Tools Handler

```go
type ListToolsHandler struct {
    commandRegistry *CommandRegistry
}

func (h *ListToolsHandler) Method() string {
    return "tools/list"
}

func (h *ListToolsHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
    commands := h.commandRegistry.ListCommands()
    
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

func (h *ListToolsHandler) generateInputSchema(cmd *Command) json.RawMessage {
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
```

### 2.2 Call Tool Handler

```go
type CallToolHandler struct {
    executor *CommandExecutor
}

func (h *CallToolHandler) Method() string {
    return "tools/call"
}

func (h *CallToolHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
    var req CallToolRequest
    if err := json.Unmarshal(params, &req); err != nil {
        return nil, &MCPError{Code: -32602, Message: "Invalid params"}
    }
    
    // Execute command
    result, err := h.executor.Execute(ctx, req.Name, req.Arguments)
    if err != nil {
        return &CallToolResult{
            IsError: true,
            Content: []ToolContent{
                {Type: "text", Text: fmt.Sprintf("Error: %v", err)},
            },
        }, nil
    }
    
    // Format result
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
```

## 3. Resource Handlers

```go
type ReadResourceHandler struct {
    resourceProvider ResourceProvider
}

func (h *ReadResourceHandler) Method() string {
    return "resources/read"
}

func (h *ReadResourceHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
    var req ReadResourceRequest
    if err := json.Unmarshal(params, &req); err != nil {
        return nil, &MCPError{Code: -32602, Message: "Invalid params"}
    }
    
    resource, err := h.resourceProvider.GetResource(req.URI)
    if err != nil {
        return nil, &MCPError{
            Code:    -32603,
            Message: fmt.Sprintf("Resource not found: %s", req.URI),
        }
    }
    
    return &ReadResourceResult{
        Contents: []ResourceContents{
            {
                URI:      req.URI,
                MimeType: resource.MimeType,
                Text:     string(resource.Data),
            },
        },
    }, nil
}
```

## 4. Error Handling

```go
type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

func (e *MCPError) Error() string {
    return fmt.Sprintf("MCP Error %d: %s", e.Code, e.Message)
}

func handleError(err error) *MCPError {
    switch e := err.(type) {
    case *MCPError:
        return e
    case *CommandError:
        return &MCPError{
            Code:    -32000,
            Message: e.Message,
            Data:    e.Details,
        }
    default:
        return &MCPError{
            Code:    -32603,
            Message: "Internal error",
            Data:    err.Error(),
        }
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
