# MCP Handlers Package Examples

This document provides comprehensive examples for using the `pkg/mcp/handlers` package, which implements MCP protocol handlers for tool execution and resource access.

## Overview

The `mcp/handlers` package provides a complete implementation of the Model Context Protocol (MCP) 2024-11-05 specification, including tool handlers, resource handlers, and the full request/response lifecycle.

## Initializing the MCP Server

### Basic Server Setup

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/openapi-cli/openapi-cli/pkg/mcp"
    "github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
)

func main() {
    // Create default registry
    registry := handlers.NewDefaultRegistry()

    // Configure server
    config := &mcp.ServerConfig{
        Name:    "openapi-cli-mcp",
        Version: "1.0.0",
    }

    // Create server
    server, err := mcp.NewServer(config, registry)
    if err != nil {
        panic(err)
    }

    // Run using stdio
    if err := server.RunStdio(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

## Tool Handlers

### Creating a Simple Tool Handler

```go
import (
    "github.com/openapi-cli/openapi-cli/pkg/mcp/types"
)

func createHelloHandler() handlers.ToolHandlerFunc {
    return func(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
        name := req.Params.Get("name").String()
        message := fmt.Sprintf("Hello, %s!", name)

        return &types.ToolCallResponse{
            Result: map[string]interface{}{
                "message": message,
            },
        }, nil
    }
}
```

### Registering Tool Handlers

```go
func registerHelloTool(registry *handlers.DefaultRegistry) {
    tool := &types.Tool{
        Name:        "hello",
        Description: "Greets the user by name",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "name": map[string]interface{}{
                    "type":        "string",
                    "description": "Name to greet",
                },
            },
            "required": []string{"name"},
        },
    }

    registry.RegisterTool("hello", tool, &handlers.ToolOptions{
        Handler: createHelloHandler(),
    })
}
```

### Example: API Call Handler

```go
type APIHandler struct {
    client *http.Client
}

func NewAPIHandler() *APIHandler {
    return &APIHandler{
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (h *APIHandler) HandleAPICall(ctx context.Context, req *types.ToolCallRequest) (*types ToolCallResponse, error) {
    // Extract parameters
    url := req.Params.Get("url").String()
    method := req.Params.Get("method").String("GET")
    headers := req.Params.Get("headers").Map()

    // Create request
    httpReq, err := http.NewRequestWithContext(ctx, method, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // Add headers
    for key, value := range headers {
        if valueStr, ok := value.(string); ok {
            httpReq.Header.Set(key, valueStr)
        }
    }

    // Execute request
    resp, err := h.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    // Read response
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    return &types.ToolCallResponse{
        Result: map[string]interface{}{
            "status":  resp.StatusCode,
            "headers": resp.Header,
            "body":    string(body),
        },
    }, nil
}

func RegisterAPIHandler(registry *handlers.DefaultRegistry) {
    handler := NewAPIHandler()

    tool := &types.Tool{
        Name:        "api_call",
        Description: "Make HTTP requests to APIs",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "url": map[string]interface{}{
                    "type":        "string",
                    "description": "URL to request",
                },
                "method": map[string]interface{}{
                    "type":        "string",
                    "description": "HTTP method",
                    "enum":        []string{"GET", "POST", "PUT", "DELETE"},
                    "default":     "GET",
                },
                "headers": map[string]interface{}{
                    "type":        "object",
                    "description": "HTTP headers to include",
                },
                "body": map[string]interface{}{
                    "type":        "string",
                    "description": "Request body",
                },
            },
            "required": []string{"url"},
        },
    }

    registry.RegisterTool("api_call", tool, &handlers.ToolOptions{
        Handler: handler.HandleAPICall,
    })
}
```

## Resource Handlers

### Creating a Resource Handler

```go
type DocumentHandler struct {
    documents map[string]string
}

func NewDocumentHandler() *DocumentHandler {
    return &DocumentHandler{
        documents: map[string]string{
            "readme":   "# Welcome\n\nThis is a readme document.",
            "license":  "MIT License\n\nCopyright (c) 2026",
            "changelog": "## Changelog\n\n- Added new features\n- Fixed bugs",
        },
    }
}

func (h *DocumentHandler) ReadResource(ctx context.Context, uri string) ([]byte, error) {
    // Extract document ID from URI
    key := strings.TrimPrefix(uri, "docs://")

    content, ok := h.documents[key]
    if !ok {
        return nil, fmt.Errorf("document not found: %s", uri)
    }

    return []byte(content), nil
}

func RegisterDocumentResource(registry *handlers.DefaultRegistry) {
    handler := NewDocumentHandler()

    resource := &types.Resource{
        URI:         "docs://readme",
        Name:        "Readme",
        Description: "Project readme document",
        MimeType:    "text/plain",
    }

    registry.RegisterResource(resource, handler.ReadResource, nil)
}
```

### Listable Resource Handler

```go
type ListableDocsHandler struct {
    docs []string
}

func NewListableHandler() *ListableDocsHandler {
    return &ListableDocsHandler{
        docs: []string{"readme", "license", "changelog"},
    }
}

func (h *ListableDocsHandler) ListResources(_ context.Context) ([]*types.Resource, error) {
    resources := make([]*types.Resource, len(h.docs))
    for i, docName := range h.docs {
        resources[i] = &types.Resource{
            URI:         fmt.Sprintf("docs://%s", docName),
            Name:        strings.Title(docName),
            Description: fmt.Sprintf("Documentation for %s", docName),
            MimeType:    "text/markdown",
        }
    }
    return resources, nil
}

func (h *ListableDocsHandler) ReadResource(_ context.Context, uri string) ([]byte, error) {
    docName := strings.TrimPrefix(uri, "docs://")
    content := fmt.Sprintf("# %s\n\nThis is the content of %s", strings.Title(docName), docName)
    return []byte(content), nil
}

func RegisterListableResource(registry *handlers.DefaultRegistry) {
    handler := NewListableHandler()

    // Register list capability
    registry.RegisterResourceList(handler.ListResources)

    // Register individual read handlers
    for _, docName := range handler.docs {
        uri := fmt.Sprintf("docs://%s", docName)
        registry.RegisterResource(
            &types.Resource{URI: uri},
            handler.ReadResource,
            nil,
        )
    }
}
```

## Prompts and Templates

### Creating Prompts

```go
type PromptHandler struct {
    prompts map[string]*types.Prompt
}

func NewPromptHandler() *PromptHandler {
    prompts := map[string]*types.Prompt{
        "summarize": {
            Name:        "summarize",
            Description: "Summarize a text",
            Arguments: []types.PromptArgument{
                {
                    Name:        "text",
                    Description: "Text to summarize",
                    Required:    true,
                },
            },
        },
    }
    return &PromptHandler{prompts: prompts}
}

func (h *PromptHandler) GetPrompt(_ context.Context, name string, args map[string]interface{}) (*types.PromptMessage, error) {
    prompt, ok := h.prompts[name]
    if !ok {
        return nil, fmt.Errorf("prompt not found: %s", name)
    }

    text := args["text"].(string)
    message := &types.PromptMessage{
        Role:    types.RoleUser,
        Content: fmt.Sprintf("Please summarize this text:\n\n%s", text),
    }

    return message, nil
}

func RegisterPromptHandler(registry *handlers.DefaultRegistry) {
    handler := NewPromptHandler()

    // Register prompts
    for _, prompt := range handler.prompts {
        registry.RegisterPrompt(prompt, handler.GetPrompt)
    }
}
```

## Complete MCP Server Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/openapi-cli/openapi-cli/pkg/mcp"
    "github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
    "github.com/openapi-cli/openapi-cli/pkg/mcp/types"
)

func main() {
    ctx := context.Background()

    // Create registry
    registry := handlers.NewDefaultRegistry()

    // Register tools
    registerHelloTool(registry)
    RegisterAPIHandler(registry)

    // Register resources
    RegisterDocumentResource(registry)
    RegisterListableResource(registry)

    // Register prompts
    RegisterPromptHandler(registry)

    // Configure server
    config := &mcp.ServerConfig{
        Name:         "openapi-cli-mcp",
        Version:      "1.0.0",
        Vendor:       "OpenAPI CLI",
        Product:      "OpenAPI CLI MCP Server",
        Instructions: "This server provides API access tools and documentation",
    }

    // Create server
    server, err := mcp.NewServer(config, registry)
    if err != nil {
        panic(err)
    }

    // Setup signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    // Start server in goroutine
    done := make(chan error, 1)
    go func() {
        done <- server.RunStdio(ctx)
    }()

    // Wait for signal or completion
    select {
    case sig := <-sigCh:
        fmt.Printf("Received signal: %v\n", sig)
        server.Shutdown(ctx)
    case err := <-done:
        if err != nil {
            fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
            os.Exit(1)
        }
    }
}

func registerHelloTool(registry *handlers.DefaultRegistry) {
    tool := &types.Tool{
        Name:        "hello",
        Description: "Greets the user by name",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "name": map[string]interface{}{
                    "type":        "string",
                    "description": "Name to greet",
                },
            },
            "required": []string{"name"},
        },
    }

    handler := func(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
        name := req.Params.Get("name").String()
        return &types.ToolCallResponse{
            Result: map[string]interface{}{
                "message": fmt.Sprintf("Hello, %s!", name),
            },
        }, nil
    }

    registry.RegisterTool("hello", tool, &handlers.ToolOptions{
        Handler: handler,
    })
}

type APIHandler struct {
    client *http.Client
}

func NewAPIHandler() *APIHandler {
    return &APIHandler{
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (h *APIHandler) HandleAPICall(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
    url := req.Params.Get("url").String()
    method := req.Params.Get("method").String("GET")

    httpReq, err := http.NewRequestWithContext(ctx, method, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := h.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    return &types.ToolCallResponse{
        Result: map[string]interface{}{
            "status": resp.StatusCode,
            "body":   string(body),
        },
    }, nil
}

func RegisterAPIHandler(registry *handlers.DefaultRegistry) {
    handler := NewAPIHandler()

    tool := &types.Tool{
        Name:        "api_call",
        Description: "Make HTTP requests to APIs",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "url": map[string]interface{}{
                    "type":        "string",
                    "description": "URL to request",
                },
                "method": map[string]interface{}{
                    "type":        "string",
                    "description": "HTTP method",
                    "enum":        []string{"GET", "POST", "PUT", "DELETE"},
                    "default":     "GET",
                },
            },
            "required": []string{"url"},
        },
    }

    registry.RegisterTool("api_call", tool, &handlers.ToolOptions{
        Handler: handler.HandleAPICall,
    })
}
```

## Error Handling

### Custom Error Responses

```go
func handleWithErrors(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
    name := req.Params.Get("name").String()

    if name == "" {
        return nil, &types.MCPError{
            Code:    types.CodeInvalidParams,
            Message: "Name parameter is required",
        }
    }

    if len(name) < 3 {
        return nil, &types.MCPError{
            Code:    types.CodeInvalidParams,
            Message: "Name must be at least 3 characters",
        }
    }

    return &types.ToolCallResponse{
        Result: map[string]interface{}{
            "message": fmt.Sprintf("Hello, %s!", name),
        },
    }, nil
}
```

### Error Codes

The MCP protocol supports several error codes:

- `CodeInternalError`: Internal server error
- `CodeInvalidParams`: Invalid or missing parameters
- `CodeNotFound`: Resource not found
- `CodeUnauthorized`: Authentication required
- `CodeMethodNotFound`: Method not found
- `CodeTimeout`: Request timeout

## Best Practices

### 1. Use Context for Cancellation

```go
func handler(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // Handle request
}
```

### 2. Validate Inputs

```go
func handler(ctx context.Context, req *types.ToolCallRequest) (*types.ToolCallResponse, error) {
    name := req.Params.Get("name").String()
    if name == "" {
        return nil, &types.MCPError{
            Code:    types.CodeInvalidParams,
            Message: "Name is required",
        }
    }
    // Continue
}
```

### 3. Return Structured Errors

```go
return nil, &types.MCPError{
    Code:    types.CodeInvalidParams,
    Message: "Invalid parameter",
    Data: map[string]interface{}{
        "parameter": "name",
        "reason":    "must be at least 3 characters",
    },
}
```

### 4. Use Tool Options for Configuration

```go
registry.RegisterTool("tool_name", tool, &handlers.ToolOptions{
    Handler: handler,
    Timeout: 30 * time.Second,
    RateLimit: 100,
})
```

## Testing MCP Handlers

### Unit Test Example

```go
func TestHelloHandler(t *testing.T) {
    handler := createHelloHandler()

    req := &types.ToolCallRequest{
        Params: types.RawMessage{
            "name": "World",
        },
    }

    resp, err := handler(context.Background(), req)
    require.NoError(t, err)

    message := resp.Result["message"].(string)
    assert.Equal(t, "Hello, World!", message)
}
```
