// Package dsl provides a Starlark-based domain-specific language (DSL) interpreter
// for automating API workflows and orchestration tasks.
//
// The DSL engine enables users to write scripts in Starlark (a Python-like language)
// that can execute HTTP requests, manipulate data, and compose complex API operations.
// Scripts run in a sandboxed environment with configurable resource limits and security
// controls.
//
// # Main Components
//
// The package provides three main components:
//
//  1. Script - Represents a parsed Starlark script with metadata
//  2. Engine - Executes scripts with resource limits and security controls
//  3. ExecutionContext - Provides runtime context (variables, secrets, inputs)
//
// # Basic Usage
//
// Create and execute a script:
//
//	// Parse a Starlark script
//	script, err := ParseScript(sourceCode)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create execution context with inputs
//	ctx := &ExecutionContext{
//	    Inputs: map[string]interface{}{
//	        "api_key": "secret-key",
//	        "limit": 100,
//	    },
//	    Config: NewDefaultEngineConfig(),
//	}
//
//	// Execute the script
//	result, err := engine.Execute(script, ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Result: %+v\n", result)
//
// # Script Metadata
//
// Scripts can include metadata in comments at the top of the file:
//
//	# name: My API Workflow
//	# description: Automates user provisioning across multiple APIs
//	# version: 1.0.0
//	# author: Jane Doe
//	#
//	# @input api_key string "API authentication key" (required)
//	# @input limit number "Maximum records to process" (default: 100)
//	# @output result string "Processing result summary"
//	#
//	# Tags: automation, api, provisioning
//
// # Security Features
//
// The DSL engine provides multiple security controls:
//
//   - Resource limits (max steps, execution time, memory)
//   - Network access control (allowed hosts list)
//   - Sandboxed execution (no arbitrary file system access)
//   - Input validation and sanitization
//   - Secret management with encryption
//
// # Error Handling
//
// All DSL errors are wrapped in DSLError with contextual information:
//
//	if err != nil {
//	    if dslErr, ok := err.(*DSLError); ok {
//	        log.Printf("Error at line %d: %s", dslErr.Line, dslErr.Message)
//	    }
//	}
package dsl

import (
	"go.starlark.net/syntax"
)

// Script represents a parsed Starlark script ready for execution.
//
// A Script contains the abstract syntax tree (AST), metadata extracted from
// comments, and runtime globals. Scripts are immutable after parsing and can
// be executed multiple times with different ExecutionContext configurations.
//
// # Fields
//
// Name is the human-readable name extracted from script metadata.
//
// Description provides a brief explanation of what the script does.
//
// Version follows semantic versioning (e.g., "1.0.0") for script versioning.
//
// Metadata contains structured information about inputs, outputs, tags, etc.
//
// AST is the parsed Starlark abstract syntax tree used for execution.
//
// Source holds the original Starlark source code for reference and debugging.
//
// Globals contains global variables available during script execution.
// These are populated by the DSL engine with built-in functions and user-provided values.
//
// # Example
//
//	script := &dsl.Script{
//	    Name:        "API Workflow",
//	    Description: "Automates multi-step API process",
//	    Version:     "1.0.0",
//	    Metadata: &dsl.ScriptMetadata{
//	        Author: "Jane Doe",
//	        Inputs: []dsl.Input{
//	            {Name: "api_key", Type: "string", Required: true},
//	        },
//	    },
//	    Source: "# name: API Workflow\nprint('Hello')",
//	}
type Script struct {
	Name        string
	Description string
	Version     string
	Metadata    *ScriptMetadata
	AST         *syntax.File
	Source      string
	Globals     map[string]interface{}
}

// ScriptMetadata contains structured metadata about a Starlark script.
//
// Metadata is extracted from specially formatted comments at the beginning
// of the script. This enables documentation generation, input validation,
// and script discovery/search functionality.
//
// # Fields
//
// Name is the human-readable script identifier (e.g., "User Provisioning Workflow").
//
// Description provides a detailed explanation of the script's purpose and functionality.
//
// Version follows semantic versioning (MAJOR.MINOR.PATCH) for tracking script changes.
//
// Author identifies the script's creator or maintainer (name or email).
//
// Inputs defines the expected input parameters with types and validation rules.
//
// Outputs describes the values returned by script execution.
//
// Tags provides categorization labels for organizing and searching scripts
// (e.g., "automation", "api", "provisioning").
//
// # Example
//
//	metadata := &dsl.ScriptMetadata{
//	    Name:        "GitHub User Lookup",
//	    Description: "Fetches GitHub user information via API",
//	    Version:     "1.0.0",
//	    Author:      "dev@example.com",
//	    Inputs: []dsl.Input{
//	        {
//	            Name:        "username",
//	            Type:        "string",
//	            Required:    true,
//	            Description: "GitHub username to lookup",
//	        },
//	    },
//	    Outputs: []dsl.Output{
//	        {
//	            Name:        "user_data",
//	            Type:        "object",
//	            Description: "GitHub user profile data",
//	        },
//	    },
//	    Tags: map[string]string{
//	        "category": "api",
//	        "platform": "github",
//	    },
//	}
type ScriptMetadata struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Author      string            `json:"author"`
	Inputs      []Input           `json:"inputs,omitempty"`
	Outputs     []Output          `json:"outputs,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// Input defines a script input parameter with type information and validation rules.
//
// Inputs are used to parameterize scripts, enabling reuse across different contexts.
// The DSL engine validates inputs against their type declarations before execution.
//
// # Fields
//
// Name is the parameter identifier used within the script (e.g., "api_key", "limit").
//
// Type specifies the expected data type. Supported types include:
//   - "string": Text values
//   - "number": Numeric values (integers or floats)
//   - "boolean": True/false values
//   - "array": List of values
//   - "object": Key-value map
//   - "any": Any type accepted
//
// Required indicates whether the input must be provided (true) or has a default (false).
// Required inputs without defaults will cause validation errors if not supplied.
//
// Default provides a fallback value when the input is not explicitly provided.
// The default value must match the declared type.
//
// Description documents the purpose and expected format of the input parameter.
// This appears in generated documentation and help text.
//
// # Example
//
//	// Required string input
//	apiKeyInput := dsl.Input{
//	    Name:        "api_key",
//	    Type:        "string",
//	    Required:    true,
//	    Description: "API authentication key (obtained from dashboard)",
//	}
//
//	// Optional number with default
//	limitInput := dsl.Input{
//	    Name:        "limit",
//	    Type:        "number",
//	    Required:    false,
//	    Default:     100,
//	    Description: "Maximum number of results to return",
//	}
//
//	// Boolean flag
//	verboseInput := dsl.Input{
//	    Name:        "verbose",
//	    Type:        "boolean",
//	    Required:    false,
//	    Default:     false,
//	    Description: "Enable verbose logging output",
//	}
type Input struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description"`
}

// Output defines a script output value with type information.
//
// Outputs describe the values returned by script execution. They serve as
// documentation for script consumers and enable type checking and validation.
//
// # Fields
//
// Name is the identifier for the output value (e.g., "result", "user_data", "error_message").
//
// Type specifies the data type of the output. Supported types include:
//   - "string": Text values
//   - "number": Numeric values
//   - "boolean": True/false values
//   - "array": List of values
//   - "object": Structured data (map)
//   - "any": Any type
//
// Description documents what the output represents and its expected format.
// This appears in generated documentation and helps users understand script results.
//
// # Example
//
//	// String result summary
//	resultOutput := dsl.Output{
//	    Name:        "summary",
//	    Type:        "string",
//	    Description: "Brief summary of processing results",
//	}
//
//	// Object with structured data
//	userDataOutput := dsl.Output{
//	    Name:        "user_profile",
//	    Type:        "object",
//	    Description: "Complete user profile including name, email, and roles",
//	}
//
//	// Array of results
//	itemsOutput := dsl.Output{
//	    Name:        "processed_items",
//	    Type:        "array",
//	    Description: "List of all processed items with status",
//	}
type Output struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ScriptResult contains the outcome of a script execution.
//
// ScriptResult captures all aspects of script execution including the return value,
// exit status, any errors encountered, and performance metrics. It provides a
// comprehensive view of how the script performed.
//
// # Fields
//
// Output holds the value returned by the script (typically the last expression evaluated).
// For scripts with explicit return statements, this contains the returned value.
// For scripts without explicit returns, this may be None or the result of the last operation.
//
// ExitCode indicates the script's termination status:
//   - 0: Successful execution
//   - Non-zero: Error or abnormal termination
//
// Error contains the error message if execution failed. Empty string on success.
// The error message includes stack traces and contextual information for debugging.
//
// Duration measures the total execution time as a formatted string (e.g., "1.234s", "567ms").
// This is useful for performance monitoring and identifying slow scripts.
//
// # Example
//
//	result, err := engine.Execute(script, ctx)
//	if err != nil {
//	    log.Fatalf("Execution failed: %v", err)
//	}
//
//	fmt.Printf("Exit Code: %d\n", result.ExitCode)
//	fmt.Printf("Output: %v\n", result.Output)
//	fmt.Printf("Duration: %s\n", result.Duration)
//	if result.Error != "" {
//	    fmt.Printf("Error: %s\n", result.Error)
//	}
type ScriptResult struct {
	Output   interface{} `json:"output"`
	ExitCode int         `json:"exit_code"`
	Error    string      `json:"error,omitempty"`
	Duration string      `json:"duration"`
}

// ExecutionContext provides the runtime environment for script execution.
//
// ExecutionContext encapsulates all the data and configuration needed to execute
// a Starlark script. It isolates script executions from each other and provides
// controlled access to external resources.
//
// # Fields
//
// Inputs contains the values for script input parameters as defined in ScriptMetadata.
// Keys match input names, values must conform to declared types.
// Example: map[string]interface{}{"api_key": "secret123", "limit": 100}
//
// Config holds the engine configuration controlling resource limits, network access,
// and security settings. Use NewDefaultEngineConfig() for sensible defaults.
//
// Secrets provides a secure key-value store for sensitive data (API keys, passwords, tokens).
// Secrets are encrypted at rest and redacted from logs and error messages.
// Access via built-in secret() function within scripts.
//
// Variables contains user-defined variables accessible during script execution.
// Unlike Inputs (which are fixed at script start), Variables can be modified during execution.
// Use for temporary state, counters, or intermediate results.
//
// Env provides environment variables inherited from the host system or set explicitly.
// Useful for configuration that varies by deployment environment.
//
// # Security Considerations
//
// ExecutionContext isolates script executions but shares the same process space.
// Do not share mutable state between executions unless explicitly intended.
// Use separate ExecutionContext instances for untrusted scripts.
//
// # Example
//
//	ctx := &dsl.ExecutionContext{
//	    Inputs: map[string]interface{}{
//	        "api_endpoint": "https://api.example.com",
//	        "api_key": os.Getenv("API_KEY"),
//	        "batch_size": 50,
//	    },
//	    Config: dsl.NewDefaultEngineConfig(),
//	    Secrets: map[string]string{
//	        "database_password": "super-secret",
//	    },
//	    Variables: map[string]interface{}{
//	        "retry_count": 0,
//	        "processed": []string{},
//	    },
//	    Env: map[string]string{
//	        "LOG_LEVEL": "debug",
//	        "TIMEOUT": "30s",
//	    },
//	}
type ExecutionContext struct {
	Inputs    map[string]interface{}
	Config    *EngineConfig
	Secrets   map[string]string
	Variables map[string]interface{}
	Env       map[string]string
}

// EngineConfig configures the Starlark execution engine with resource limits and security settings.
//
// EngineConfig provides fine-grained control over script execution, enabling administrators
// to prevent resource exhaustion attacks and enforce security policies.
//
// # Fields
//
// MaxSteps limits the number of Starlark bytecode instructions executed before termination.
// This prevents infinite loops and denial-of-service attacks. Default: 100,000 steps.
// Set to -1 for unlimited (not recommended for untrusted scripts).
//
// MaxExecutionTime specifies the maximum wall-clock time in seconds before execution is aborted.
// This protects against scripts that consume resources indefinitely. Default: 30 seconds.
// Set to 0 to disable time-based termination.
//
// MaxMemoryMB caps the memory usage of script execution in megabytes.
// Exceeding this limit causes immediate termination. Default: 256 MB.
// Set to 0 for unlimited (use with caution).
//
// EnableNetwork controls whether scripts can make network requests (HTTP, WebSocket, etc.).
// Disable for scripts that should only perform local computations. Default: true.
//
// AllowedHosts provides a whitelist of hostnames/IPs that scripts can connect to.
// Empty list means all hosts are allowed (when EnableNetwork is true).
// Use specific hostnames to restrict network access:
//   - "api.example.com" - Single hostname
//   - "*.example.com" - All subdomains
//   - "10.0.0.0/8" - CIDR notation for IP ranges
//   - "localhost", "127.0.0.1" - Local connections
//
// # Security Best Practices
//
// For untrusted scripts, use restrictive settings:
//
//	config := &dsl.EngineConfig{
//	    MaxSteps:         10000,      // Limit computation
//	    MaxExecutionTime: 10,         // 10 second timeout
//	    MaxMemoryMB:      64,         // 64 MB limit
//	    EnableNetwork:    false,      // No network access
//	    AllowedHosts:     []string{}, // Empty deny-all
//	}
//
// For trusted automation scripts with network access:
//
//	config := &dsl.EngineConfig{
//	    MaxSteps:         100000,     // Reasonable step limit
//	    MaxExecutionTime: 60,         // 1 minute for long operations
//	    MaxMemoryMB:      256,        // Adequate memory
//	    EnableNetwork:    true,       // Network required
//	    AllowedHosts:     []string{"api.example.com", "*.internal.local"},
//	}
//
// # Example
//
//	// Default configuration for most use cases
//	config := dsl.NewDefaultEngineConfig()
//
//	// Custom configuration for resource-constrained environment
//	customConfig := &dsl.EngineConfig{
//	    MaxSteps:         50000,
//	    MaxExecutionTime: 15,
//	    MaxMemoryMB:      128,
//	    EnableNetwork:    true,
//	    AllowedHosts:     []string{"api.trusted.com"},
//	}
type EngineConfig struct {
	MaxSteps         int64
	MaxExecutionTime int
	MaxMemoryMB      int
	EnableNetwork    bool
	AllowedHosts     []string
}

// ErrorType categorizes DSL errors for proper handling and reporting.
//
// ErrorType enables script authors and the engine to distinguish between different
// classes of errors and apply appropriate error handling strategies.
//
// # Error Categories
//
// ErrorParse: Syntax errors during Starlark parsing (invalid syntax, undefined variables).
// These errors occur before execution begins and indicate bugs in the script.
//
// ErrorRuntime: Errors during script execution (network failures, timeouts, resource limits).
// These errors may be transient and could succeed on retry.
//
// ErrorValidation: Input validation failures, type mismatches, or constraint violations.
// These errors indicate incorrect usage and require user action to fix.
//
// ErrorResource: Resource exhaustion errors (memory limit exceeded, max steps reached,
// execution timeout). These errors indicate the script needs more resources or optimization.
type ErrorType string

const (
	// ErrorParse indicates Starlark syntax errors or parsing failures.
	// Examples: invalid expression, unexpected token, undefined symbol.
	// Resolution: Fix script syntax errors.
	ErrorParse ErrorType = "parse"

	// ErrorRuntime indicates execution failures during script evaluation.
	// Examples: HTTP request failed, division by zero, type conversion error.
	// Resolution: Handle error in script or fix runtime conditions.
	ErrorRuntime ErrorType = "runtime"

	// ErrorValidation indicates input validation or constraint violations.
	// Examples: missing required input, invalid type, out-of-range value.
	// Resolution: Provide valid inputs or adjust constraints.
	ErrorValidation ErrorType = "validation"

	// ErrorResource indicates resource limit exhaustion.
	// Examples: max steps exceeded, timeout, memory limit reached.
	// Resolution: Optimize script or increase resource limits.
	ErrorResource ErrorType = "resource"
)

// DSLError represents an error that occurred during DSL script parsing or execution.
//
// DSLError provides structured error information including error category, descriptive
// message, and optional source location (line/column). This enables precise error
// reporting and debugging assistance.
//
// # Fields
//
// Type categorizes the error for appropriate handling (parse, runtime, validation, resource).
// See ErrorType constants for available categories.
//
// Message provides a human-readable description of the error including context and
// suggested resolutions when possible.
//
// Line indicates the source line number where the error occurred (1-indexed).
// Zero if line number is unavailable or error is not source-related.
//
// Column indicates the column position within the line (1-indexed).
// Zero if column position is unavailable.
//
// # Error Formatting
//
// The Error() method formats errors differently based on type:
//   - Parse errors: Include line:column location
//   - Runtime errors: Include contextual message
//   - Validation errors: Describe the validation failure
//   - Resource errors: Specify which limit was exceeded
//
// # Example
//
//	result, err := engine.Execute(script, ctx)
//	if err != nil {
//	    if dslErr, ok := err.(*dsl.DSLError); ok {
//	        if dslErr.Line > 0 {
//	            log.Printf("Error at line %d:%d [%s]: %s",
//	                dslErr.Line, dslErr.Column, dslErr.Type, dslErr.Message)
//	        } else {
//	            log.Printf("Error [%s]: %s", dslErr.Type, dslErr.Message)
//	        }
//	    } else {
//	        log.Printf("Unexpected error: %v", err)
//	    }
//	}
//
// # Error Inspection
//
// Check error type for appropriate handling:
//
//	if dslErr, ok := err.(*dsl.DSLError); ok {
//	    switch dslErr.Type {
//	    case dsl.ErrorParse:
//	        // Fix script syntax
//	    case dsl.ErrorRuntime:
//	        // Retry or handle failure
//	    case dsl.ErrorValidation:
//	        // Correct inputs
//	    case dsl.ErrorResource:
//	        // Increase limits or optimize
//	    }
//	}
type DSLError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Line    int       `json:"line,omitempty"`
	Column  int       `json:"column,omitempty"`
}

// Error implements the error interface for DSLError, formatting the error message
// with source location information when available.
//
// For errors with line numbers (typically parse errors), the format is:
// "line:column: message"
//
// For errors without line numbers (runtime, validation, resource errors),
// only the message is returned.
//
// # Example
//
//	err := &dsl.DSLError{
//	    Type:    dsl.ErrorParse,
//	    Message: "unexpected token '}'",
//	    Line:    42,
//	    Column:  15,
//	}
//	fmt.Println(err.Error()) // Output: "42:15: unexpected token '}'"
//
//	err2 := &dsl.DSLError{
//	    Type:    dsl.ErrorRuntime,
//	    Message: "HTTP request failed: connection timeout",
//	}
//	fmt.Println(err2.Error()) // Output: "HTTP request failed: connection timeout"
func (e *DSLError) Error() string {
	if e.Line > 0 {
		return e.Message
	}
	return e.Message
}

// NewDefaultEngineConfig creates a new EngineConfig with sensible default values
// suitable for most script execution scenarios.
//
// The default configuration balances security and functionality:
//   - MaxSteps: 100,000 (prevents infinite loops while allowing complex scripts)
//   - MaxExecutionTime: 30 seconds (adequate for most API operations)
//   - MaxMemoryMB: 256 (sufficient for data processing)
//   - EnableNetwork: true (allows HTTP requests)
//   - AllowedHosts: [] (empty = all hosts allowed when network enabled)
//
// # When to Customize
//
// Use custom configuration when:
//   - Executing untrusted scripts (use stricter limits)
//   - Running compute-intensive scripts (increase MaxSteps, MaxMemoryMB)
//   - Performing long-running operations (increase MaxExecutionTime)
//   - Restricting network access (populate AllowedHosts)
//   - Running in resource-constrained environments (reduce limits)
//
// # Security Recommendation
//
// For untrusted scripts, use more restrictive settings:
//
//	config := dsl.NewDefaultEngineConfig()
//	config.MaxSteps = 10000      // Reduce from 100k
//	config.MaxExecutionTime = 10 // Reduce from 30s
//	config.MaxMemoryMB = 64      // Reduce from 256MB
//	config.EnableNetwork = false // Disable network
//
// # Example
//
//	// Use defaults for trusted scripts
//	config := dsl.NewDefaultEngineConfig()
//	engine := dsl.NewEngine(config)
//
//	// Customize for resource-intensive script
//	config := dsl.NewDefaultEngineConfig()
//	config.MaxSteps = 500000     // Increase to 500k
//	config.MaxMemoryMB = 512     // Increase to 512MB
//	config.MaxExecutionTime = 120 // 2 minutes for long operations
//
// # Returns
//
// A pointer to a new EngineConfig instance with default values.
// Safe to modify after creation.
func NewDefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxSteps:         100000,
		MaxExecutionTime: 30,
		MaxMemoryMB:      256,
		EnableNetwork:    true,
		AllowedHosts:     []string{},
	}
}
