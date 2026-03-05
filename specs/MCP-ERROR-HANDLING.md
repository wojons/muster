# MCP Error Handling Specification

Standardized error handling for Model Context Protocol.

## Error Codes

```go
// JSON-RPC 2.0 standard errors
const (
    ParseErrorCode     = -32700 // Invalid JSON
    InvalidRequestCode = -32600 // Invalid Request object
    MethodNotFoundCode = -32601 // Method doesn't exist
    InvalidParamsCode  = -32602 // Invalid method parameters
    InternalErrorCode  = -32603 // Internal JSON-RPC error
    ServerErrorCodeBase = -32000 // Server errors (-32000 to -32099)
)

// MCP-specific errors
const (
    ToolExecutionErrorCode         = -32001
    ToolNotFoundErrorCode          = -32002
    ToolValidationErrorCode        = -32003
    UnsupportedProtocolVersionCode = -32004
    UnsupportedOperationCode       = -32005
    ResourceNotFoundCode           = -32006
    ResourceAccessDeniedCode       = -32007
)
```

## Error Response Format

```go
// RPCError represents JSON-RPC error
type RPCError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
    return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Response with error
type Response struct {
    JSONRPC string     `json:"jsonrpc"`
    ID      *RequestID `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *RPCError  `json:"error,omitempty"`
}
```

## Error Builder

```go
// ErrorBuilder constructs RPC errors
type ErrorBuilder struct {
    code    int
    message string
    data    interface{}
}

func NewError(code int) *ErrorBuilder {
    return &ErrorBuilder{code: code}
}

func (b *ErrorBuilder) Message(msg string) *ErrorBuilder {
    b.message = msg
    return b
}

func (b *ErrorBuilder) Data(data interface{}) *ErrorBuilder {
    b.data = data
    return b
}

func (b *ErrorBuilder) Build() *RPCError {
    var dataJSON json.RawMessage
    if b.data != nil {
        dataJSON, _ = json.Marshal(b.data)
    }
    return &RPCError{
        Code:    b.code,
        Message: b.message,
        Data:    dataJSON,
    }
}

// Convenience constructors
func ParseError(msg string) *RPCError {
    return NewError(ParseErrorCode).Message(msg).Build()
}

func MethodNotFound(method string) *RPCError {
    return NewError(MethodNotFoundCode).
        Message(fmt.Sprintf("Method not found: %s", method)).Build()
}

func InvalidParams(msg string) *RPCError {
    return NewError(InvalidParamsCode).Message(msg).Build()
}

func ToolNotFound(tool string) *RPCError {
    return NewError(ToolNotFoundErrorCode).
        Message(fmt.Sprintf("Tool not found: %s", tool)).Build()
}
```
