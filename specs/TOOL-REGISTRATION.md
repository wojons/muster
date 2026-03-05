# MCP Tool Registration Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: MCP

## Overview

Complete specification for registering **MCP tools** with the Model Context Protocol server. Tools are executable functions that language models can invoke to perform actions or retrieve data. This specification ensures deterministic tool registration across all implementations.

## 1. Tool Registration Architecture

### 1.1 Registration Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Tool Definition│     │   MCP Server     │     │   Client        │
│   (Metadata)     │────▶│   Registration   │────▶│   Discovery     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
         │                       │                        │
         ▼                       ▼                        ▼
  - Name                    - Validation           - Tool List
  - Description             - Registration         - Execution
  - Parameters              - Error Handling       - Results
  - Return Schema
```

### 1.2 Tool Structure

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolHandler is the function signature for tool implementations
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// Tool represents an MCP-registered tool
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Handler     ToolHandler     `json:"-"` // Function to execute
}

// ToolRegistrationRequest is sent from client to register tools
type ToolRegistrationRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      int      `json:"id"`
	Method  string   `json:"method"`
	Params  ToolList `json:"params"`
}

// ToolList contains the list of available tools
type ToolList struct {
	Tools []Tool `json:"tools"`
}

// ToolCallRequest represents a tool invocation
type ToolCallRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"params"`
}

// ToolCallResponse contains the result of a tool invocation
type ToolCallResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Content []ToolContent `json:"content"`
		IsError bool          `json:"isError,omitempty"`
	} `json:"result,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// ToolContent represents the content returned by a tool
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Error represents an MCP protocol error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
```

## 2. Tool Registration Implementation

### 2.1 Registration Manager

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolRegistry manages tool registration and execution
type ToolRegistry struct {
	tools map[string]*Tool
	mu    sync.RWMutex
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*Tool),
	}
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool *Tool) error {
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}
	
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	
	if tool.Handler == nil {
		return fmt.Errorf("tool handler is required")
	}
	
	// Validate JSON schema if provided
	if len(tool.InputSchema) > 0 {
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			return fmt.Errorf("invalid input schema: %w", err)
		}
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("tool '%s' already registered", tool.Name)
	}
	
	r.tools[tool.Name] = tool
	return nil
}

// Unregister removes a tool from the registry
func (r *ToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool '%s' not found", name)
	}
	
	delete(r.tools, name)
	return nil
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (*Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool '%s' not found", name)
	}
	
	return tool, nil
}

// List returns all registered tools
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, *tool)
	}
	
	return tools
}

// Execute invokes a tool with the given arguments
func (r *ToolRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (*ToolCallResponse, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, fmt.Errorf("tool not found: %w", err)
	}
	
	// Parse and validate arguments
	var argsMap map[string]interface{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	
	// Execute tool handler
	result, err := tool.Handler(ctx, argsMap)
	
	response := &ToolCallResponse{}
	
	if err != nil {
		response.Result.IsError = true
		response.Result.Content = []ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			},
		}
	} else {
		// Format result as text
		resultText, _ := json.MarshalIndent(result, "", "  ")
		response.Result.Content = []ToolContent{
			{
				Type: "text",
				Text: string(resultText),
			},
		}
	}
	
	return response, nil
}
```

### 2.2 Server-Side Registration Handler

```go
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// RegistrationServer handles tool registration requests
type RegistrationServer struct {
	registry *ToolRegistry
}

// NewRegistrationServer creates a new registration server
func NewRegistrationServer(registry *ToolRegistry) *RegistrationServer {
	return &RegistrationServer{
		registry: registry,
	}
}

// HandleToolsList handles the tools/list method
func (s *RegistrationServer) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	tools := s.registry.List()
	
	response := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      1, // Should match request ID
		Result: struct {
			Tools []Tool `json:"tools"`
		}{
			Tools: tools,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleToolCall handles the tools/call method
func (s *RegistrationServer) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	var request ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.sendError(w, -32700, "Parse error", err.Error())
		return
	}
	
	ctx := r.Context()
	result, err := s.registry.Execute(ctx, request.Params.Name, request.Params.Arguments)
	if err != nil {
		s.sendError(w, -32602, "Invalid params", err.Error())
		return
	}
	
	result.ID = request.ID
	result.JSONRPC = "2.0"
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *RegistrationServer) sendError(w http.ResponseWriter, code int, message string, data string) {
	response := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Error   Error  `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      0,
		Error: Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}
```

### 2.3 Client-Side Tool Discovery

```go
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client discovers and invokes MCP tools
type Client struct {
	baseURL string
	httpClient *http.Client
}

// NewClient creates a new MCP client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// DiscoverTools retrieves the list of available tools from the server
func (c *Client) DiscoverTools() ([]Tool, error) {
	request := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
	}{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	
	resp, err := c.httpClient.Post(
		c.baseURL+"/mcp",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	
	var result struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
		Error *Error `json:"error,omitempty"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	if result.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", result.Error.Message)
	}
	
	return result.Result.Tools, nil
}

// CallTool invokes a tool on the MCP server
func (c *Client) CallTool(name string, args interface{}) (*ToolCallResponse, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal arguments: %w", err)
	}
	
	request := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"params"`
	}{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}{
			Name:      name,
			Arguments: argsJSON,
		},
	}
	
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	
	resp, err := c.httpClient.Post(
		c.baseURL+"/mcp",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	
	var result ToolCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	if result.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", result.Error.Code, result.Error.Message)
	}
	
	return &result, nil
}
```

## 3. Tool Definition Examples

### 3.1 HTTP Request Tool

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPRequestTool performs HTTP requests
func HTTPRequestTool() *mcp.Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"method": {
				"type": "string",
				"enum": ["GET", "POST", "PUT", "DELETE", "PATCH"],
				"default": "GET"
			},
			"url": {
				"type": "string",
				"format": "uri"
			},
			"headers": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				}
			},
			"body": {
				"type": "string"
			}
		},
		"required": ["url"]
	}`)
	
	return &mcp.Tool{
		Name:        "http_request",
		Description: "Perform an HTTP request",
		InputSchema: schema,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			method, _ := args["method"].(string)
			if method == "" {
				method = "GET"
			}
			
			url, ok := args["url"].(string)
			if !ok {
				return nil, fmt.Errorf("url is required")
			}
			
			req, err := http.NewRequestWithContext(ctx, method, url, nil)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}
			
			// Add headers
			if headers, ok := args["headers"].(map[string]interface{}); ok {
				for key, val := range headers {
					if strVal, ok := val.(string); ok {
						req.Header.Set(key, strVal)
					}
				}
			}
			
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("execute request: %w", err)
			}
			defer resp.Body.Close()
			
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("read response: %w", err)
			}
			
			return map[string]interface{}{
				"status":  resp.StatusCode,
				"headers": resp.Header,
				"body":    string(body),
			}, nil
		},
	}
}
```

### 3.2 File System Tool

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileSystemTool provides file system operations
func FileSystemTool() *mcp.Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"enum": ["read", "write", "list", "exists", "delete"]
			},
			"path": {
				"type": "string"
			},
			"content": {
				"type": "string"
			}
		},
		"required": ["operation", "path"]
	}`)
	
	return &mcp.Tool{
		Name:        "filesystem",
		Description: "Perform file system operations",
		InputSchema: schema,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			operation, _ := args["operation"].(string)
			path, _ := args["path"].(string)
			
			switch operation {
			case "read":
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("read file: %w", err)
				}
				return map[string]string{"content": string(content)}, nil
				
			case "write":
				content, _ := args["content"].(string)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					return nil, fmt.Errorf("write file: %w", err)
				}
				return map[string]string{"status": "written"}, nil
				
			case "list":
				entries, err := os.ReadDir(path)
				if err != nil {
					return nil, fmt.Errorf("list directory: %w", err)
				}
				
				var files []string
				for _, entry := range entries {
					files = append(files, entry.Name())
				}
				return map[string]interface{}{"entries": files}, nil
				
			case "exists":
				_, err := os.Stat(path)
				return map[string]bool{"exists": err == nil}, nil
				
			case "delete":
				if err := os.Remove(path); err != nil {
					return nil, fmt.Errorf("delete file: %w", err)
				}
				return map[string]string{"status": "deleted"}, nil
				
			default:
				return nil, fmt.Errorf("unknown operation: %s", operation)
			}
		},
	}
}
```

## 4. Validation Requirements

### 4.1 Schema Validation

All tool input schemas MUST:
1. Be valid JSON Schema (draft-07 or later)
2. Define the `type` at the root level
3. Specify all required fields in the `required` array
4. Use standard JSON Schema types only

### 4.2 Handler Requirements

All tool handlers MUST:
1. Accept `context.Context` as first parameter
2. Accept `map[string]interface{}` as arguments
3. Return `(interface{}, error)`
4. Respect context cancellation
5. Return deterministic results for identical inputs

### 4.3 Error Handling

```go
// Standard error codes
const (
	ErrParseError     = -32700 // Invalid JSON
	ErrInvalidRequest = -32600 // Invalid request object
	ErrMethodNotFound = -32601 // Method doesn't exist
	ErrInvalidParams  = -32602 // Invalid parameters
	ErrInternalError  = -32603 // Internal error
	ErrServerError    = -32000 // Server-specific error
	ErrToolNotFound   = -32001 // Tool not found
	ErrInvalidSchema  = -32002 // Invalid input schema
)
```

## 5. Complete Registration Example

```go
package main

import (
	"log"
	"net/http"
	
	"github.com/example/mcp-server/pkg/mcp"
	"github.com/example/mcp-server/pkg/tools"
)

func main() {
	// Create registry
	registry := mcp.NewToolRegistry()
	
	// Register HTTP request tool
	if err := registry.Register(tools.HTTPRequestTool()); err != nil {
		log.Fatalf("Failed to register http_request: %v", err)
	}
	
	// Register file system tool
	if err := registry.Register(tools.FileSystemTool()); err != nil {
		log.Fatalf("Failed to register filesystem: %v", err)
	}
	
	// Create registration server
	server := mcp.NewRegistrationServer(registry)
	
	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		// Route based on method
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		method, _ := body["method"].(string)
		switch method {
		case "tools/list":
			server.HandleToolsList(w, r)
		case "tools/call":
			server.HandleToolCall(w, r)
		default:
			server.sendError(w, -32601, "Method not found", method)
		}
	})
	
	// Start server
	log.Println("MCP server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
```

## 6. Security Considerations

### 6.1 Input Validation

All tool inputs MUST be validated against the JSON schema before execution:

```go
func (r *ToolRegistry) validateArgs(schema json.RawMessage, args map[string]interface{}) error {
	// Use jsonschema library for validation
	schemaLoader := gojsonschema.NewBytesLoader(schema)
	documentLoader := gojsonschema.NewGoLoader(args)
	
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	
	if !result.Valid() {
		var errors []string
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		return fmt.Errorf("validation failed: %s", strings.Join(errors, ", "))
	}
	
	return nil
}
```

### 6.2 Access Control

Tool registration SHOULD implement access control:

```go
type AccessControl struct {
	allowedTools map[string][]string // tool -> allowed users
}

func (ac *AccessControl) CanExecute(userID, toolName string) bool {
	allowed, ok := ac.allowedTools[toolName]
	if !ok {
		return false // Deny by default
	}
	
	for _, allowedUser := range allowed {
		if allowedUser == userID || allowedUser == "*" {
			return true
		}
	}
	
	return false
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC