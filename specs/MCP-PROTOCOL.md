# MCP Protocol Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: MCP Protocol

## Overview

Complete specification of the **Model Context Protocol (MCP)** implementation for stdio-based communication with AI clients.

## 1. Protocol Basics

### 1.1 Transport

```go
// MCP uses JSON-RPC 2.0 over stdio
type MCPTransport struct {
    stdin  io.Reader
    stdout io.Writer
    stderr io.Writer
}
```

### 1.2 Message Format

```go
type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      interface{}     `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      interface{}     `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

## 2. Lifecycle Methods

### 2.1 Initialize

```go
// Request
type InitializeRequest struct {
    ProtocolVersion string                 `json:"protocolVersion"`
    Capabilities    ClientCapabilities   `json:"capabilities"`
    ClientInfo      Implementation       `json:"clientInfo"`
}

// Response
type InitializeResult struct {
    ProtocolVersion string                 `json:"protocolVersion"`
    Capabilities    ServerCapabilities   `json:"capabilities"`
    ServerInfo      Implementation       `json:"serverInfo"`
}

func (s *MCPServer) HandleInitialize(req *InitializeRequest) (*InitializeResult, error) {
    return &InitializeResult{
        ProtocolVersion: "2024-11-05",
        Capabilities: ServerCapabilities{
            Tools:     &ToolsCapability{},
            Resources: &ResourcesCapability{},
            Prompts:   &PromptsCapability{},
        },
        ServerInfo: Implementation{
            Name:    "openapi-cli-mcp",
            Version: "1.0.0",
        },
    }, nil
}
```

### 2.2 Initialized Notification

```go
type InitializedNotification struct{}

func (s *MCPServer) HandleInitialized() {
    s.initialized = true
    s.loadTools()
}
```

## 3. Tool Methods

### 3.1 tools/list

```go
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
```

### 3.2 tools/call

```go
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
```

## 4. Resource Methods

### 4.1 resources/list

```go
type ListResourcesRequest struct {
    Cursor *string `json:"cursor,omitempty"`
}

type ListResourcesResult struct {
    Resources  []Resource `json:"resources"`
    NextCursor string     `json:"nextCursor,omitempty"`
}

type Resource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

### 4.2 resources/read

```go
type ReadResourceRequest struct {
    URI string `json:"uri"`
}

type ReadResourceResult struct {
    Contents []ResourceContents `json:"contents"`
}
```

## 5. Error Codes

| Code | Name | Description |
|------|------|-------------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid request | Invalid request object |
| -32601 | Method not found | Unknown method |
| -32602 | Invalid params | Invalid method parameters |
| -32603 | Internal error | Internal error |
| -32000 | Server error | Generic server error |

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
