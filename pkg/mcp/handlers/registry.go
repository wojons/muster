// Package handlers provides MCP (Model Context Protocol) request handlers for implementing
// AI assistant tool integrations. This package implements the core handler infrastructure
// that routes JSON-RPC 2.0 requests to appropriate handlers and manages tool registrations.
//
// # Overview
//
// The handlers package provides a registry-based architecture for managing MCP protocol
// handlers. It supports:
//
//   - Handler registration and discovery via method names
//   - JSON-RPC 2.0 compliant request routing
//   - Structured error handling with standard error codes
//   - Context propagation for cancellation and timeouts
//   - Type-safe request/response handling
//
// # Architecture
//
// The package consists of three main components:
//
// 1. Handler Interface: Defines the contract for all MCP handlers
// 2. Registry: Central handler registry for method-based routing
// 3. MCPError: Standardized error type with JSON-RPC error codes
//
// # Usage
//
// Create a registry and register handlers:
//
//	registry := handlers.NewRegistry()
//	registry.Register(handlers.NewInitializeHandler())
//	registry.Register(handlers.NewListToolsHandler(commandRegistry))
//	registry.Register(handlers.NewCallToolHandler(commandExecutor))
//
// Handle incoming JSON-RPC requests:
//
//	ctx := context.Background()
//	result, err := registry.Handle(ctx, "tools/list", params)
//	if err != nil {
//	    return handlers.HandleError(err)
//	}
//
// # Error Handling
//
// The package uses standard JSON-RPC 2.0 error codes:
//
//   - -32600: Invalid Request
//   - -32601: Method not found
//   - -32602: Invalid params
//   - -32603: Internal error
//
// Errors are wrapped using HandleError() to ensure proper JSON-RPC format:
//
//	result, err := handler.Handle(ctx, params)
//	if err != nil {
//	    mcpErr := handlers.HandleError(err)
//	    return nil, mcpErr
//	}
//
// # Thread Safety
//
// The Registry is NOT thread-safe by default. For concurrent access, wrap
// Register() and Handle() calls with appropriate synchronization or use
// a thread-safe wrapper.
//
// # Integration with Command System
//
// MCP handlers integrate with Muster's command system through interfaces:
//
//   - CommandRegistry: Provides list of available commands
//   - CommandExecutor: Executes commands with arguments
//
// See tools.go for command-specific handlers (tools/list, tools/call).
// See lifecycle.go for session lifecycle handlers (initialize, initialized).
// See resources.go for resource management handlers.
// See prompts.go for prompt management handlers.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
)

// Handler defines the contract for all MCP protocol handlers.
//
// Every MCP handler must implement this interface, which provides method
// identification and request handling capabilities. The Handler interface
// enables the Registry to route JSON-RPC requests to the appropriate handler.
//
// # Method Name
//
// The Method() function returns the JSON-RPC method name this handler processes.
// For example:
//   - "initialize" - Session initialization
//   - "tools/list" - List available tools
//   - "tools/call" - Execute a tool
//   - "resources/list" - List available resources
//
// # Request Handling
//
// The Handle() method processes incoming JSON-RPC requests:
//   - ctx: Context for cancellation, timeouts, and request-scoped values
//   - params: Raw JSON parameters from the JSON-RPC request
//   - Returns: Result object (will be JSON-marshaled) or error
//
// # Error Handling
//
// Handlers should return:
//   - (result, nil) for successful requests
//   - (nil, *MCPError) for MCP protocol errors
//   - (nil, error) for unexpected errors (wrapped by HandleError)
//
// # Example Implementation
//
//	type InitializeHandler struct {
//	    version string
//	}
//
//	func (h *InitializeHandler) Method() string {
//	    return "initialize"
//	}
//
//	func (h *InitializeHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
//	    var req InitializeRequest
//	    if err := json.Unmarshal(params, &req); err != nil {
//	        return nil, &MCPError{Code: -32602, Message: "Invalid params"}
//	    }
//	    return &InitializeResult{Version: h.version}, nil
//	}
type Handler interface {
	// Method returns the JSON-RPC method name this handler processes.
	// This is used by the Registry to route requests to the correct handler.
	Method() string

	// Handle processes an incoming JSON-RPC request.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeouts, and request-scoped values
	//   - params: Raw JSON parameters from the JSON-RPC request (json.RawMessage)
	//
	// Returns:
	//   - interface{}: Handler-specific result object (will be JSON-marshaled)
	//   - error: *MCPError for protocol errors, or error for unexpected failures
	Handle(ctx context.Context, params json.RawMessage) (interface{}, error)
}

// Registry is the central handler registry for MCP protocol request routing.
//
// The Registry maintains a mapping of method names to handlers and routes
// incoming JSON-RPC requests to the appropriate handler based on the method name.
//
// # Thread Safety
//
// Registry is NOT thread-safe. For concurrent access:
//   - Use a mutex to protect Register() calls
//   - Handle() is safe for concurrent reads after initialization
//
// # Usage
//
//	registry := handlers.NewRegistry()
//
//	// Register handlers during initialization
//	registry.Register(NewInitializeHandler())
//	registry.Register(NewListToolsHandler(cmdRegistry))
//	registry.Register(NewCallToolHandler(cmdExecutor))
//
//	// Handle requests concurrently
//	go func() {
//	    result, err := registry.Handle(ctx, "tools/list", params)
//	}()
//
// # Integration
//
// The Registry integrates with:
//   - cmdstore.Loader: Loads command metadata for tools/list
//   - client.Executor: Executes HTTP requests for tools/call
//   - mcp.Server: Wraps registry for JSON-RPC server functionality
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates a new empty handler registry.
//
// The registry is initialized with an empty handler map. Handlers must be
// registered using Register() before handling requests.
//
// Returns:
//   - *Registry: New registry instance ready for handler registration
//
// Example:
//
//	registry := handlers.NewRegistry()
//	registry.Register(NewInitializeHandler())
//	registry.Register(NewListToolsHandler(commandRegistry))
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

// Register adds a handler to the registry under its method name.
//
// The handler is registered using the method name returned by handler.Method().
// If a handler with the same method name already exists, it will be overwritten.
//
// Parameters:
//   - handler: Handler implementation to register
//
// Panics:
//   - If handler is nil (should be validated before calling)
//
// Example:
//
//	registry := handlers.NewRegistry()
//
//	// Register lifecycle handlers
//	registry.Register(NewInitializeHandler())
//	registry.Register(NewInitializedHandler())
//
//	// Register tool handlers
//	registry.Register(NewListToolsHandler(cmdRegistry))
//	registry.Register(NewCallToolHandler(cmdExecutor))
//
//	// Register resource handlers
//	registry.Register(NewListResourcesHandler())
//	registry.Register(NewReadResourceHandler())
//
// Notes:
//   - Registration should happen during application initialization
//   - Not safe for concurrent registration (use mutex if needed)
//   - Last registration wins if multiple handlers use the same method name
func (r *Registry) Register(handler Handler) {
	r.handlers[handler.Method()] = handler
}

// Handle routes an incoming JSON-RPC request to the appropriate handler.
//
// This method looks up the handler registered for the given method name and
// delegates the request to handler.Handle(). If no handler is found, returns
// a "Method not found" MCP error.
//
// Parameters:
//   - ctx: Context for cancellation, timeouts, and request-scoped values
//   - method: JSON-RPC method name (e.g., "initialize", "tools/list")
//   - params: Raw JSON parameters from the JSON-RPC request
//
// Returns:
//   - interface{}: Handler-specific result object
//   - error: *MCPError if method not found or handler fails
//
// Example:
//
//	ctx := context.Background()
//	params := json.RawMessage(`{"capabilities": {...}}`)
//
//	result, err := registry.Handle(ctx, "initialize", params)
//	if err != nil {
//	    return handlers.HandleError(err)
//	}
//
//	// Marshal result to JSON for response
//	response, _ := json.Marshal(result)
//
// Error Codes:
//   - -32601: Method not found (no handler registered for method)
//   - Handler-specific errors: Propagated from handler.Handle()
//
// Notes:
//   - Context is propagated to handler for cancellation support
//   - params should be pre-validated JSON (validation happens in handler)
//   - Safe for concurrent Handle() calls after initialization
func (r *Registry) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	handler, ok := r.handlers[method]
	if !ok {
		return nil, &MCPError{Code: -32601, Message: "Method not found"}
	}
	return handler.Handle(ctx, params)
}

// MCPError represents a JSON-RPC 2.0 protocol error.
//
// MCPError implements the JSON-RPC 2.0 error response format with standard
// error codes and optional error data. This type is used throughout the
// handlers package to report protocol errors and application errors.
//
// # Error Codes
//
// Standard JSON-RPC 2.0 error codes:
//   - -32600: Invalid Request - Request is not valid JSON-RPC
//   - -32601: Method not found - No handler registered for method
//   - -32602: Invalid params - Parameter validation failed
//   - -32603: Internal error - Unexpected error during processing
//
// # Structure
//
// The error format follows JSON-RPC 2.0 specification:
//
//	{
//	  "code": -32601,
//	  "message": "Method not found",
//	  "data": {...}  // Optional additional error information
//	}
//
// # Usage
//
// Create protocol errors:
//
//	if method != "initialize" {
//	    return nil, &handlers.MCPError{
//	        Code: -32600,
//	        Message: "Invalid Request",
//	    }
//	}
//
// Create errors with additional data:
//
//	if !valid {
//	    return nil, &handlers.MCPError{
//	        Code: -32602,
//	        Message: "Invalid params",
//	        Data: map[string]string{
//	            "field": "name",
//	            "reason": "missing required field",
//	        },
//	    }
//	}
//
// # Integration with errors.Package
//
// For integration with Muster's error handling, use HandleError() to
// wrap non-MCP errors:
//
//	result, err := someOperation()
//	if err != nil {
//	    return nil, handlers.HandleError(err)
//	}
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface for MCPError.
//
// Returns a formatted error string following the pattern:
// "MCP Error {code}: {message}"
//
// Example output:
//   - "MCP Error -32601: Method not found"
//   - "MCP Error -32602: Invalid params"
//
// This method enables MCPError to be used wherever Go's error interface
// is expected, including:
//   - Return values from handler.Handle()
//   - Error wrapping with fmt.Errorf()
//   - Error checking with errors.Is()
func (e *MCPError) Error() string {
	return fmt.Sprintf("MCP Error %d: %s", e.Code, e.Message)
}

// HandleError wraps a generic error into an MCPError with appropriate error code.
//
// This function provides centralized error handling for MCP handlers. It ensures
// all errors returned to clients follow the JSON-RPC 2.0 error format.
//
// Parameters:
//   - err: Error to wrap (may be *MCPError or generic error)
//
// Returns:
//   - *MCPError: Wrapped error with proper JSON-RPC format
//
// Behavior:
//   - If err is already *MCPError: Returns as-is
//   - If err is generic error: Wraps as -32603 (Internal error) with err.Error() in Data field
//
// Example:
//
//	result, err := database.Query(sql)
//	if err != nil {
//	    return nil, handlers.HandleError(err)
//	    // Returns: &MCPError{Code: -32603, Message: "Internal error", Data: "sql: no rows in result set"}
//	}
//
// Usage in handlers:
//
//	func (h *Handler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
//	    result, err := h.execute(params)
//	    if err != nil {
//	        return nil, handlers.HandleError(err)
//	    }
//	    return result, nil
//	}
//
// Best Practices:
//   - Use HandleError() for unexpected errors only
//   - Return specific *MCPError for known validation errors
//   - Include contextual information in Data field for debugging
func HandleError(err error) *MCPError {
	if e, ok := err.(*MCPError); ok {
		return e
	}
	return &MCPError{
		Code:    -32603,
		Message: "Internal error",
		Data:    err.Error(),
	}
}
