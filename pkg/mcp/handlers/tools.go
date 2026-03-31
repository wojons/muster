package handlers

import (
	"context"
	"encoding/json"
	"fmt"
)

// Command represents a registered command in the MCP tools system.
//
// Commands are the building blocks of MCP tools. Each command describes:
//   - Its name and purpose (Description)
//   - Available parameters (Flags)
//   - Expected behavior and return values
//
// # Integration with Command System
//
// Commands are provided by implementations of CommandRegistry interface.
// The ListToolsHandler converts Commands to MCP Tool objects with JSON Schema
// input validation.
//
// # Example
//
//	cmd := Command{
//	    Name:        "getPet",
//	    Description: "Retrieve a pet by ID from the store",
//	    Flags: []Flag{
//	        {
//	            Name:        "petId",
//	            Type:        "integer",
//	            Description: "Unique identifier of the pet",
//	            Required:    true,
//	        },
//	        {
//	            Name:        "includeDetails",
//	            Type:        "boolean",
//	            Description: "Include extended pet details",
//	            Required:    false,
//	        },
//	    },
//	}
//
// # JSON Schema Generation
//
// When converted to MCP tools, commands generate JSON Schema input schemas:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "petId": {
//	      "type": "integer",
//	      "description": "Unique identifier of the pet"
//	    },
//	    "includeDetails": {
//	      "type": "boolean",
//	      "description": "Include extended pet details"
//	    }
//	  },
//	  "required": ["petId"]
//	}
//
// # Thread Safety
//
// Command structs are immutable after creation and safe for concurrent reads.
type Command struct {
	Name        string
	Description string
	Flags       []Flag
}

// Flag represents a command parameter or option.
//
// Flags define the input parameters that a command accepts. Each flag specifies:
//   - Name: Parameter identifier used in tool calls
//   - Type: JSON Schema type (string, integer, boolean, array, object)
//   - Description: Human-readable parameter description
//   - Required: Whether the parameter is mandatory
//
// # Supported Types
//
// Flag types correspond to JSON Schema types:
//   - "string": Text values
//   - "integer": Whole numbers
//   - "number": Decimal numbers
//   - "boolean": true/false values
//   - "array": List of values
//   - "object": Nested object structure
//
// # Example Flags
//
//	// Required string parameter
//	Flag{
//	    Name:        "username",
//	    Type:        "string",
//	    Description: "User's login name",
//	    Required:    true,
//	}
//
//	// Optional integer with default behavior
//	Flag{
//	    Name:        "limit",
//	    Type:        "integer",
//	    Description: "Maximum number of results to return (default: 100)",
//	    Required:    false,
//	}
//
//	// Boolean toggle
//	Flag{
//	    Name:        "verbose",
//	    Type:        "boolean",
//	    Description: "Enable verbose output",
//	    Required:    false,
//	}
//
// # Validation
//
// Required flags are added to the "required" array in the generated JSON Schema.
// The MCP client should validate inputs against this schema before calling tools.
//
// # Best Practices
//
//   - Use clear, descriptive parameter names
//   - Provide comprehensive descriptions with examples
//   - Mark parameters as required only when truly necessary
//   - Document default values for optional parameters
//   - Use appropriate JSON Schema types for type safety
type Flag struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// CommandRegistry provides access to registered commands for tool listing.
//
// This interface abstracts the command storage layer, allowing the MCP handlers
// to retrieve command metadata without knowing the underlying implementation.
//
// # Implementations
//
// Typical implementations include:
//   - cmdstore.Loader: Loads commands from SQLite database
//   - In-memory registry: For testing or dynamic command registration
//   - Composite registry: Aggregates multiple command sources
//
// # Usage
//
//	type MyCommandRegistry struct {
//	    store *cmdstore.Loader
//	}
//
//	func (r *MyCommandRegistry) ListCommands() []Command {
//	    return r.store.GetAllCommands()
//	}
//
//	func (r *MyCommandRegistry) GetCommand(name string) *Command {
//	    return r.store.GetCommandByName(name)
//	}
//
//	// Use with MCP handlers
//	registry := NewListToolsHandler(&MyCommandRegistry{store})
//
// # Thread Safety
//
// Implementations should be thread-safe as ListCommands() may be called
// concurrently by multiple MCP requests.
type CommandRegistry interface {
	// ListCommands returns all available commands.
	//
	// Returns a slice of all registered commands. The slice should be
	// safe for concurrent access (either immutable or copied on return).
	//
	// Returns:
	//   - []Command: Slice of all available commands
	//
	// Notes:
	//   - May return empty slice if no commands registered
	//   - Should not return nil (return empty slice instead)
	//   - Performance: Called on every tools/list request
	ListCommands() []Command

	// GetCommand retrieves a specific command by name.
	//
	// Parameters:
	//   - name: Command name to look up
	//
	// Returns:
	//   - *Command: Pointer to command if found, nil if not found
	//
	// Notes:
	//   - Returns nil if command doesn't exist (caller should check)
	//   - Used by tools/call handler to validate command exists before execution
	GetCommand(name string) *Command
}

// CommandExecutor executes commands with provided arguments.
//
// This interface abstracts the command execution layer, allowing the MCP
// handlers to execute tools without knowing the underlying implementation.
//
// # Implementations
//
// Typical implementations include:
//   - client.Executor: Executes HTTP requests against OpenAPI endpoints
//   - DSL Executor: Executes Starlark scripts
//   - WASM Executor: Runs WebAssembly modules
//   - Shell Executor: Runs shell commands (with appropriate sandboxing)
//
// # Usage
//
//	type MyCommandExecutor struct {
//	    httpClient *client.Executor
//	}
//
//	func (e *MyCommandExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
//	    // Build HTTP request from command name and arguments
//	    req, err := e.httpClient.BuildRequest(ctx, name, args)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    // Execute request and return response
//	    return e.httpClient.Execute(req)
//	}
//
//	// Use with MCP handlers
//	executor := NewCallToolHandler(&MyCommandExecutor{httpClient})
//
// # Argument Handling
//
// The args map contains parameter values keyed by flag name:
//
//	args := map[string]interface{}{
//	    "petId": float64(123),        // JSON numbers decode as float64
//	    "includeDetails": true,       // Booleans preserved
//	    "tags": []interface{}{"dog", "friendly"},  // Arrays as slices
//	}
//
// # Error Handling
//
// Implementations should return:
//   - (result, nil) for successful execution
//   - (nil, error) for execution failures
//
// Errors are wrapped by CallToolHandler into AI-consumable error messages.
//
// # Context Propagation
//
// The ctx parameter should be respected for:
//   - Cancellation: Stop execution if ctx.Done() is closed
//   - Timeouts: Fail if execution exceeds deadline
//   - Request-scoped values: Auth tokens, trace IDs, etc.
//
// # Thread Safety
//
// Implementations must be thread-safe as Execute() may be called
// concurrently by multiple MCP requests.
type CommandExecutor interface {
	// Execute runs a command with the provided arguments.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeouts, and request-scoped values
	//   - name: Command name to execute
	//   - args: Map of parameter names to values
	//
	// Returns:
	//   - interface{}: Command result (will be JSON-marshaled for response)
	//   - error: Execution error (wrapped into AI-consumable format)
	//
	// Example:
	//
	//	result, err := executor.Execute(ctx, "getPet", map[string]interface{}{
	//	    "petId": float64(123),
	//	})
	//	if err != nil {
	//	    return nil, fmt.Errorf("failed to get pet: %w", err)
	//	}
	//	return result, nil
	//
	// Notes:
	//   - Arguments are pre-validated against JSON Schema
	//   - Result should be JSON-serializable
	//   - Errors should be descriptive for AI consumption
	Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
}

// ListToolsHandler handles MCP "tools/list" requests.
//
// This handler retrieves all registered commands from the CommandRegistry
// and converts them to MCP Tool objects with JSON Schema input validation.
//
// # JSON-RPC Method
//
//	method: "tools/list"
//	params: ListToolsRequest (optional cursor for pagination)
//	result: ListToolsResult{Tools: []Tool}
//
// # Request Flow
//
// 1. Client sends: {"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
// 2. Handler retrieves commands from CommandRegistry
// 3. Each Command is converted to Tool with JSON Schema
// 4. Response: {"jsonrpc": "2.0", "result": {"tools": [...]}, "id": 1}
//
// # JSON Schema Generation
//
// For each Command, generates input schema:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "flagName": {
//	      "type": "string|integer|boolean|array|object",
//	      "description": "Flag description"
//	    }
//	  },
//	  "required": ["requiredFlag1", "requiredFlag2"]
//	}
//
// # Example Response
//
//	{
//	  "tools": [
//	    {
//	      "name": "getPet",
//	      "description": "Retrieve a pet by ID",
//	      "inputSchema": {
//	        "type": "object",
//	        "properties": {
//	          "petId": {
//	            "type": "integer",
//	            "description": "Pet ID to retrieve"
//	          }
//	        },
//	        "required": ["petId"]
//	      }
//	    }
//	  ]
//	}
//
// # Pagination
//
// Supports cursor-based pagination for large tool lists:
//   - Request: {"cursor": "eyJvZmZzZXQiOjEwMH0="}
//   - Response: {"tools": [...], "nextCursor": "eyJvZmZzZXQiOjIwMH0="}
//
// # Usage
//
//	handler := handlers.NewListToolsHandler(commandRegistry)
//	registry.Register(handler)
//
// # Thread Safety
//
// ListToolsHandler is thread-safe if CommandRegistry is thread-safe.
type ListToolsHandler struct {
	registry CommandRegistry
}

// NewListToolsHandler creates a new tools/list handler.
//
// Parameters:
//   - registry: CommandRegistry implementation providing command metadata
//
// Returns:
//   - *ListToolsHandler: Handler ready for registration
//
// Example:
//
//	cmdRegistry := cmdstore.NewLoader(db)
//	toolsHandler := handlers.NewListToolsHandler(cmdRegistry)
//	registry.Register(toolsHandler)
func NewListToolsHandler(registry CommandRegistry) *ListToolsHandler {
	return &ListToolsHandler{
		registry: registry,
	}
}

// Method returns the JSON-RPC method name for this handler.
//
// Returns: "tools/list"
//
// This method implements the Handler interface, allowing the
// ListToolsHandler to be registered in the Registry.
func (h *ListToolsHandler) Method() string {
	return "tools/list"
}

// Handle processes a tools/list request.
//
// This method retrieves all commands from the registry, converts them to
// MCP Tool objects with JSON Schema input validation, and returns them
// to the client.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - params: JSON-RPC parameters (ListToolsRequest with optional cursor)
//
// Returns:
//   - *ListToolsResult: List of available tools with schemas
//   - error: *MCPError for invalid params or registry errors
//
// Request Format:
//
//	{"cursor": "eyJvZmZzZXQiOjEwMH0="}  // Optional pagination cursor
//
// Response Format:
//
//	{
//	  "tools": [
//	    {
//	      "name": "getPet",
//	      "description": "Retrieve a pet by ID",
//	      "inputSchema": {"type": "object", ...}
//	    }
//	  ],
//	  "nextCursor": "eyJvZmZzZXQiOjIwMH0="  // If more results available
//	}
//
// Error Cases:
//   - Invalid JSON in params: Returns -32602 (Invalid params)
//   - Registry error: Returns -32603 (Internal error)
//
// Notes:
//   - Cursor pagination is optional (implement in CommandRegistry)
//   - Empty tool list returns {"tools": []}
//   - All errors wrapped with HandleError()
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

// generateInputSchema converts a Command to JSON Schema format.
//
// This method generates a JSON Schema object describing the command's
// input parameters. The schema is used by MCP clients for:
//   - Parameter validation before tool call
//   - IDE autocomplete and type checking
//   - AI assistant parameter suggestions
//
// Parameters:
//   - cmd: Command to convert
//
// Returns:
//   - json.RawMessage: JSON Schema object
//
// Schema Format:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "paramName": {
//	      "type": "string|integer|boolean|array|object",
//	      "description": "Parameter description"
//	    }
//	  },
//	  "required": ["requiredParam1", "requiredParam2"]
//	}
//
// Example Output:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "petId": {
//	      "type": "integer",
//	      "description": "Unique identifier of the pet"
//	    },
//	    "includeDetails": {
//	      "type": "boolean",
//	      "description": "Include extended pet details"
//	    }
//	  },
//	  "required": ["petId"]
//	}
//
// Type Mapping:
//   - Flag.Type "string" → JSON Schema "string"
//   - Flag.Type "integer" → JSON Schema "integer"
//   - Flag.Type "number" → JSON Schema "number"
//   - Flag.Type "boolean" → JSON Schema "boolean"
//   - Flag.Type "array" → JSON Schema "array"
//   - Flag.Type "object" → JSON Schema "object"
//
// Notes:
//   - Required flags added to "required" array
//   - Schema is compact (no whitespace) to reduce payload size
//   - Returns empty properties object if no flags defined
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

// ListToolsRequest represents a tools/list request.
//
// Parameters:
//   - Cursor: Optional pagination cursor for large tool lists
//
// Request Format:
//
//	{"cursor": "eyJvZmZzZXQiOjEwMH0="}
//
// Pagination:
//   - First request: cursor = nil or omitted
//   - Subsequent requests: cursor = nextCursor from previous response
//   - Encoding: Base64-encoded JSON with offset/metadata
//
// Example:
//
//	// First page
//	req := ListToolsRequest{}
//
//	// Second page (cursor from previous response)
//	req := ListToolsRequest{
//	    Cursor: &nextCursor,
//	}
//
// Notes:
//   - Cursor is optional (omitempty)
//   - Pointer allows distinguishing nil vs empty string
//   - Implementation depends on CommandRegistry supporting pagination
type ListToolsRequest struct {
	Cursor *string `json:"cursor,omitempty"`
}

// ListToolsResult represents a tools/list response.
//
// Fields:
//   - Tools: Array of available MCP tools with schemas
//   - NextCursor: Optional cursor for next page (pagination)
//
// Response Format:
//
//	{
//	  "tools": [
//	    {
//	      "name": "getPet",
//	      "description": "Retrieve a pet by ID",
//	      "inputSchema": {...}
//	    }
//	  ],
//	  "nextCursor": "eyJvZmZzZXQiOjIwMH0="
//	}
//
// Pagination:
//   - nextCursor present: More results available
//   - nextCursor absent/empty: Last page reached
//   - Cursor encoding: Implementation-specific (typically base64 JSON)
//
// Example:
//
//	result := &ListToolsResult{
//	    Tools: []Tool{
//	        {
//	            Name: "getPet",
//	            Description: "Retrieve pet by ID",
//	            InputSchema: json.RawMessage(`{"type":"object",...}`),
//	        },
//	    },
//	    NextCursor: "",  // Empty = no more results
//	}
//
// Notes:
//   - Empty tools list is valid: {"tools": []}
//   - NextCursor omitted if no pagination needed
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Tool represents an MCP tool (converted from Command).
//
// Tools are the MCP protocol's representation of callable functions.
// Each tool includes:
//   - Name: Unique identifier for the tool
//   - Description: Human-readable purpose and behavior
//   - InputSchema: JSON Schema for parameter validation
//
// # JSON Representation
//
//	{
//	  "name": "getPet",
//	  "description": "Retrieve a pet by ID from the store",
//	  "inputSchema": {
//	    "type": "object",
//	    "properties": {
//	      "petId": {
//	        "type": "integer",
//	        "description": "Unique identifier of the pet"
//	      }
//	    },
//	    "required": ["petId"]
//	  }
//	}
//
// # Input Schema
//
// The inputSchema field is a JSON Schema object that defines:
//   - Parameter types (string, integer, boolean, array, object)
//   - Parameter descriptions for AI assistants
//   - Required parameters for validation
//   - Nested object structures for complex parameters
//
// # Usage by AI Assistants
//
// AI assistants use the tool definition to:
//  1. Understand what the tool does (Description)
//  2. Know what parameters are needed (InputSchema)
//  3. Validate parameters before calling (InputSchema.required)
//  4. Format the tool call arguments correctly
//
// # Example
//
//	tool := Tool{
//	    Name:        "listPets",
//	    Description: "List all pets in the store with pagination",
//	    InputSchema: json.RawMessage(`{
//	        "type": "object",
//	        "properties": {
//	            "limit": {"type": "integer", "description": "Max results (default: 100)"},
//	            "offset": {"type": "integer", "description": "Offset for pagination"},
//	            "status": {"type": "string", "description": "Filter by status: available, pending, sold"}
//	        },
//	        "required": []
//	    }`),
//	}
//
// # Conversion from Command
//
// Tools are automatically generated from Commands by ListToolsHandler:
//
//	command := Command{
//	    Name: "getPet",
//	    Description: "Retrieve a pet by ID",
//	    Flags: []Flag{{Name: "petId", Type: "integer", Required: true}},
//	}
//
//	tool := Tool{
//	    Name: command.Name,
//	    Description: command.Description,
//	    InputSchema: handler.generateInputSchema(command),
//	}
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// CallToolHandler handles MCP "tools/call" requests.
//
// This handler executes a specific tool with the provided arguments
// using the CommandExecutor. It wraps execution results in the MCP
// response format and handles errors gracefully.
//
// # JSON-RPC Method
//
//	method: "tools/call"
//	params: CallToolRequest{name, arguments}
//	result: CallToolResult{content: [{type: "text", text: "..."}]}
//
// # Request Flow
//
// 1. Client sends: {"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "getPet", "arguments": {"petId": 123}}, "id": 2}
// 2. Handler validates tool exists
// 3. Executor runs command with arguments
// 4. Result formatted as text content
// 5. Response: {"jsonrpc": "2.0", "result": {"content": [{...}]}, "id": 2}
//
// # Error Handling
//
// Execution errors are wrapped in AI-consumable format:
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "Error: failed to execute getPet: connection refused"
//	    }
//	  ],
//	  "isError": true
//	}
//
// This allows AI assistants to see and handle errors appropriately.
//
// # Result Formatting
//
// Successful results are pretty-printed JSON:
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "{\n  \"id\": 123,\n  \"name\": \"Fluffy\",\n  \"status\": \"available\"\n}"
//	    }
//	  ]
//	}
//
// # Usage
//
//	executor := client.NewExecutor(httpClient)
//	callHandler := handlers.NewCallToolHandler(executor)
//	registry.Register(callHandler)
//
// # Thread Safety
//
// CallToolHandler is thread-safe if CommandExecutor is thread-safe.
type CallToolHandler struct {
	executor CommandExecutor
}

// NewCallToolHandler creates a new tools/call handler.
//
// Parameters:
//   - executor: CommandExecutor implementation for running tools
//
// Returns:
//   - *CallToolHandler: Handler ready for registration
//
// Example:
//
//	httpExecutor := client.NewExecutor(httpClient)
//	callHandler := handlers.NewCallToolHandler(httpExecutor)
//	registry.Register(callHandler)
//
// Integration:
//   - executor should be thread-safe for concurrent calls
//   - executor should respect context cancellation
//   - executor should handle argument type conversion
func NewCallToolHandler(executor CommandExecutor) *CallToolHandler {
	return &CallToolHandler{
		executor: executor,
	}
}

// Method returns the JSON-RPC method name for this handler.
//
// Returns: "tools/call"
//
// This method implements the Handler interface, allowing the
// CallToolHandler to be registered in the Registry.
func (h *CallToolHandler) Method() string {
	return "tools/call"
}

// Handle processes a tools/call request.
//
// This method executes the specified tool with the provided arguments
// and returns the result in MCP format.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - params: JSON-RPC parameters (CallToolRequest)
//
// Returns:
//   - *CallToolResult: Execution result with content array
//   - nil: Error is embedded in result.isError field
//
// Request Format:
//
//	{
//	  "name": "getPet",
//	  "arguments": {
//	    "petId": 123
//	  }
//	}
//
// Success Response:
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "{\n  \"id\": 123,\n  \"name\": \"Fluffy\",\n  \"status\": \"available\"\n}"
//	    }
//	  ]
//	}
//
// Error Response:
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "Error: failed to execute getPet: pet not found"
//	    }
//	  ],
//	  "isError": true
//	}
//
// Error Handling:
//   - Invalid JSON: Returns nil, &MCPError{-32602, "Invalid params"}
//   - Execution error: Returns CallToolResult{isError: true}
//   - Marshal error: Falls back to fmt.Sprintf("%v", result)
//
// Notes:
//   - Errors are NOT returned as error type (MCP convention)
//   - Errors embedded in result for AI visibility
//   - Result pretty-printed for readability
//   - Context propagated to executor for cancellation
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

// CallToolRequest represents a tools/call request.
//
// Fields:
//   - Name: Tool name to execute (must match registered tool)
//   - Arguments: Map of parameter names to values
//
// Request Format:
//
//	{
//	  "name": "getPet",
//	  "arguments": {
//	    "petId": 123,
//	    "includeDetails": true
//	  }
//	}
//
// Arguments:
//   - Keys match Flag.Name from tool definition
//   - Values are JSON types (string, number, boolean, array, object)
//   - Numbers decode as float64 (convert to int if needed)
//   - Missing required args: Executor should validate
//   - Extra args: Typically ignored (forward-compatible)
//
// Example:
//
//	req := CallToolRequest{
//	    Name: "listPets",
//	    Arguments: map[string]interface{}{
//	        "limit": float64(50),
//	        "offset": float64(0),
//	        "status": "available",
//	    },
//	}
//
// Type Conversion:
//
//	// JSON numbers decode as float64
//	petID := int(req.Arguments["petId"].(float64))
//
//	// Type assertion with check
//	if limit, ok := req.Arguments["limit"].(float64); ok {
//	    limitInt := int(limit)
//	}
//
// Validation:
//   - Arguments pre-validated against InputSchema by MCP client
//   - Executor should still validate for defense in depth
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolResult represents a tools/call response.
//
// Fields:
//   - Content: Array of content items (text, image, etc.)
//   - IsError: true if execution failed (error embedded in content)
//
// Response Format (Success):
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "{\n  \"id\": 123,\n  \"name\": \"Fluffy\"\n}"
//	    }
//	  ]
//	}
//
// Response Format (Error):
//
//	{
//	  "content": [
//	    {
//	      "type": "text",
//	      "text": "Error: pet not found"
//	    }
//	  ],
//	  "isError": true
//	}
//
// Content Types:
//   - "text": Plain text or JSON (most common)
//   - "image": Base64-encoded image (future support)
//   - "resource": Reference to MCP resource (future support)
//
// isError Flag:
//   - false/omitted: Successful execution
//   - true: Execution failed, error message in content
//
// Why Embed Errors:
//
// Errors are embedded in the result (not returned as error type) so:
//  1. AI assistants can see and interpret errors
//  2. Errors don't trigger JSON-RPC error handling
//  3. Consistent response format for success/failure
//
// Example:
//
//	// Success
//	result := &CallToolResult{
//	    Content: []ToolContent{
//	        {Type: "text", Text: prettyJSON},
//	    },
//	}
//
//	// Error
//	result := &CallToolResult{
//	    Content: []ToolContent{
//	        {Type: "text", Text: "Error: " + err.Error()},
//	    },
//	    IsError: true,
//	}
//
// Notes:
//   - Content is array (supports multi-modal responses)
//   - First content item typically used by AI
//   - isError optional (defaults to false)
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent represents a single content item in a tool response.
//
// Fields:
//   - Type: Content type ("text", "image", "resource")
//   - Text: Content text (for type="text")
//
// Content Format:
//
//	{"type": "text", "text": "Result content here"}
//
// Supported Types:
//   - "text": Plain text or JSON (currently supported)
//   - "image": Base64-encoded image data (future)
//   - "resource": MCP resource reference (future)
//
// Text Content:
//   - Can be plain text: "Operation completed successfully"
//   - Can be JSON: "{\"id\": 123, \"name\": \"Fluffy\"}"
//   - Can be formatted: Pretty-printed JSON with indentation
//   - Can be markdown: "**Success!** Pet retrieved." (future)
//
// Multi-Modal Responses:
//
// The content array supports multiple items for future multi-modal responses:
//
//	{
//	  "content": [
//	    {"type": "text", "text": "Here's the pet you requested:"},
//	    {"type": "image", "data": "base64..."}  // Future
//	  ]
//	}
//
// Current Usage:
//   - Single text item: Most common
//   - JSON output: Pretty-printed for readability
//   - Error messages: Prefixed with "Error: "
//
// Example:
//
//	// Success response
//	content := ToolContent{
//	    Type: "text",
//	    Text: "{\n  \"id\": 123,\n  \"name\": \"Fluffy\"\n}",
//	}
//
//	// Error response
//	content := ToolContent{
//	    Type: "text",
//	    Text: "Error: failed to connect to API",
//	}
//
// Notes:
//   - Text field omitempty (not included if empty)
//   - Type is always required
//   - Additional fields for image/resource types TBD
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
