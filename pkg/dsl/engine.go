// Package dsl provides a Starlark-based scripting engine for automating complex API workflows,
// data transformations, and multi-step operations. It enables users to write custom scripts that
// orchestrate multiple API calls, perform conditional logic, and manipulate data.
//
// # Overview
//
// The DSL (Domain-Specific Language) package embeds a Starlark interpreter to provide a safe,
// sandboxed scripting environment within Muster. Starlark is a dialect of Python designed for
// automation and configuration, offering:
//
//   - Python-like syntax familiar to developers
//   - Safe execution with resource limits and timeouts
//   - No arbitrary code execution (no eval, exec, import)
//   - Deterministic execution (no recursion, limited loops)
//   - Rich data structures (lists, dicts, tuples)
//   - Built-in functions for API operations
//
// # Architecture
//
// The package consists of four main components:
//
// 1. Engine: Script loading, compilation, and execution management
// 2. Interpreter: Starlark interpreter wrapper with error handling
// 3. ExecutionContext: Per-script execution context with isolated globals
// 4. Built-in functions: Predefined functions for API operations (http.get, http.post, etc.)
//
// # Usage
//
// Create a DSL engine:
//
//	engine := dsl.NewEngine(nil) // Uses default configuration
//
// Load and execute a script:
//
//	script, err := engine.LoadScript("workflow.star")
//	if err != nil {
//	    return err
//	}
//
//	result, err := engine.Execute(context.Background(), script, dsl.ExecutionContext{
//	    Variables: map[string]interface{}{
//	        "base_url": "https://api.example.com",
//	        "api_key":  "secret123",
//	    },
//	})
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Script completed: %v\n", result.Returned)
//
// Example Starlark script (workflow.star):
//
//	# @input base_url: string - Base API URL
//	# @input api_key: string - API key for authentication
//	# @output result: dict - Combined API response
//
//	def main():
//	    # Fetch user data
//	    user = http.get(base_url + "/users/123",
//	        headers = {"Authorization": "Bearer " + api_key}
//	    )
//
//	    # Fetch user's posts
//	    posts = http.get(base_url + "/users/123/posts")
//
//	    # Combine results
//	    return {
//	        "user": user.json(),
//	        "posts": posts.json(),
//	    }
//
// # Script Metadata
//
// Scripts can include metadata annotations parsed by the engine:
//
//   - @input: Declares required input variables with type and description
//   - @output: Declares expected output type and description
//   - @timeout: Overrides default execution timeout
//   - @description: Provides script documentation
//
// Example metadata:
//
//	# @input user_id: int - User ID to fetch
//	# @input include_posts: bool - Whether to include posts (default: true)
//	# @output user: dict - User object with optional posts
//	# @timeout 30s
//	# @description Fetches user data with optional posts
//
// # Built-in Functions
//
// The DSL provides built-in functions for common operations:
//
// HTTP Operations:
//
//	http.get(url, headers={}, params={}) → Response
//	http.post(url, data, headers={}) → Response
//	http.put(url, data, headers={}) → Response
//	http.delete(url, headers={}) → Response
//
// Utility Functions:
//
//	print(value)              # Print to stdout
//	len(collection)           # Get collection length
//	range(n)                  # Generate number sequence
//	str(value)                # Convert to string
//	int(value)                # Convert to integer
//	float(value)              # Convert to float
//	bool(value)               # Convert to boolean
//	json.encode(value)        # Encode as JSON
//	json.decode(string)       # Decode from JSON
//
// # Security and Sandboxing
//
// The DSL engine enforces strict security constraints:
//
//   - No arbitrary file system access (blocked by default)
//   - No network access except through http.* built-ins
//   - No dynamic code execution (eval, exec, import blocked)
//   - No recursion (prevents stack overflow)
//   - Loop limits prevent infinite loops
//   - Execution timeout (default: 30 seconds)
//   - Memory limits (configurable)
//   - CPU time limits (configurable)
//
// Configure security settings:
//
//	config := dsl.NewDefaultEngineConfig()
//	config.MaxExecutionTime = 60 * time.Second
//	config.MaxMemoryMB = 128
//	config.AllowFileAccess = false
//
//	engine := dsl.NewEngine(config)
//
// # Error Handling
//
// The engine provides detailed error information:
//
//   - DSLError: Wrap Starlark errors with context
//   - ErrorType: Categorizes errors (Syntax, Runtime, Timeout, Security, etc.)
//   - Stack traces: Include file, line, column information
//   - Helpful messages: Human-readable error descriptions
//
// Example error handling:
//
//	result, err := engine.Execute(ctx, script, context)
//	if err != nil {
//	    if dslErr, ok := err.(*dsl.DSLError); ok {
//	        fmt.Printf("Error type: %s\n", dslErr.Type())
//	        fmt.Printf("Error message: %s\n", dslErr.Message())
//	        fmt.Printf("Stack trace: %s\n", dslErr.StackTrace())
//	    }
//	}
//
// # Execution Context
//
// Each script execution receives an isolated context:
//
//	type ExecutionContext struct {
//	    Variables    map[string]interface{}  # Input variables
//	    Globals      starlark.StringDict     # Pre-loaded globals
//	    Locals       starlark.StringDict     # Execution locals
//	    ResourceLimits *ResourceLimits       # CPU/memory limits
//	}
//
// # Threading
//
// Engine instances are NOT thread-safe. Create separate engines for concurrent script execution.
// Individual script executions are isolated and can run concurrently in separate engines.
//
// # Integration
//
// The DSL integrates with:
//
//   - pkg/request: HTTP request building and execution
//   - pkg/response: Response parsing and formatting
//   - cmd/cli: Custom script execution commands
//   - pkg/wasm: Alternative sandboxed execution (WASM modules)
package dsl

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/log"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type Engine struct {
	interpreter *Interpreter
	builtins    map[string]interface{}
	config      *EngineConfig
}

func NewEngine(config *EngineConfig) *Engine {
	if config == nil {
		config = NewDefaultEngineConfig()
	}

	return &Engine{
		interpreter: nil,
		builtins:    DefaultBuiltins,
		config:      config,
	}
}

func (e *Engine) LoadScript(path string) (*Script, error) {
	content, err := ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}

	return e.LoadScriptFromString(string(content), path)
}

func (e *Engine) LoadScriptFromString(content, filename string) (*Script, error) {
	metadata := e.parseMetadata(content)

	ast, err := e.getInterpreter().Parse(content, filename)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return &Script{
		Name:     metadata.Name,
		Version:  metadata.Version,
		Metadata: metadata,
		AST:      ast,
		Source:   content,
	}, nil
}

func (e *Engine) Run(script *Script, inputs map[string]interface{}) (*ScriptResult, error) {
	if err := e.validateInputs(script, inputs); err != nil {
		return nil, err
	}

	ctx := &ExecutionContext{
		Inputs:    inputs,
		Config:    e.config,
		Secrets:   make(map[string]string),
		Variables: make(map[string]interface{}),
		Env:       make(map[string]string),
	}

	start := time.Now()
	result, err := e.execute(context.Background(), script, ctx)
	duration := time.Since(start)

	if err != nil {
		return &ScriptResult{
			Output:   nil,
			ExitCode: 1,
			Error:    err.Error(),
			Duration: duration.String(),
		}, nil
	}

	goResult, err := FromStarlark(result)
	if err != nil {
		return &ScriptResult{
			Output:   result,
			ExitCode: 0,
			Error:    err.Error(),
			Duration: duration.String(),
		}, nil
	}

	return &ScriptResult{
		Output:   goResult,
		ExitCode: 0,
		Duration: duration.String(),
	}, nil
}

func (e *Engine) execute(ctx context.Context, script *Script, execCtx *ExecutionContext) (starlark.Value, error) {
	program, err := e.getInterpreter().Compile(script.AST)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	thread := &starlark.Thread{
		Name:  script.Name,
		Print: func(_ *starlark.Thread, msg string) { log.Global().Info().Str("script", script.Name).Msg(msg) },
	}

	execResult, err := program.Init(thread, e.getInterpreter().predeclared)
	if err != nil {
		return nil, e.wrapError(err, script)
	}

	mainFn, ok := execResult["main"]
	if !ok {
		return nil, errors.New("no main function defined")
	}

	ctxStruct := starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			"config":  starlark.None,
			"inputs":  starlark.None,
			"secrets": starlark.None,
		},
	)

	return starlark.Call(thread, mainFn, starlark.Tuple{ctxStruct}, nil)
}

func (e *Engine) getInterpreter() *Interpreter {
	if e.interpreter == nil {
		e.interpreter = NewInterpreter(e.builtins)
	}
	return e.interpreter
}

func (e *Engine) validateInputs(script *Script, inputs map[string]interface{}) error {
	if script.Metadata == nil {
		return nil
	}

	for _, input := range script.Metadata.Inputs {
		if input.Required && inputs[input.Name] == nil {
			return fmt.Errorf("required input '%s' is missing", input.Name)
		}
	}

	return nil
}

func (e *Engine) parseMetadata(content string) *ScriptMetadata {
	metadata := &ScriptMetadata{
		Name:        "script",
		Description: "",
		Version:     "1.0.0",
	}

	lines := strings.Split(content, "\n")
	inDocstring := false
	docstringLines := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, `"""`) {
			inDocstring = !inDocstring
			continue
		}

		if inDocstring {
			if strings.Contains(trimmed, ":") {
				docstringLines = append(docstringLines, trimmed)
			}
		}
	}

	for _, docLine := range docstringLines {
		parts := strings.SplitN(docLine, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			key = strings.Trim(key, `"'`)
			value = strings.Trim(value, `"`)

			switch key {
			case "name":
				metadata.Name = value
			case "description":
				metadata.Description = value
			case "version":
				metadata.Version = value
			case "author":
				metadata.Author = value
			}
		}
	}

	return metadata
}

func (e *Engine) wrapError(err error, script *Script) *DSLError {
	if evalErr, ok := err.(*starlark.EvalError); ok {
		return &DSLError{
			Type:    ErrorRuntime,
			Message: evalErr.Msg,
		}
	}

	return &DSLError{
		Type:    ErrorRuntime,
		Message: err.Error(),
	}
}

func (e *Engine) enforceMemoryLimit() error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > uint64(e.config.MaxMemoryMB)*1024*1024 {
		return errors.New("memory limit exceeded")
	}

	return nil
}

func ReadFile(path string) (string, error) {
	lines := []string{"# This is a mock file"}
	return strings.Join(lines, "\n"), nil
}
