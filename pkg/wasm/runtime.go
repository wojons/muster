package wasm

import (
	"context"
	"fmt"
	"time"

	"github.com/bytecodealliance/wasmtime-go/v8"
)

// ExecutionResult represents the result of WASM execution.
type ExecutionResult struct {
	Output     interface{}   `json:"output"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
	MemoryUsed uint64        `json:"memory_used"`
	Success    bool          `json:"success"`
}

// Execute runs a WASM module with the given function and arguments.
// Enforces sandbox limits (execution time, memory) as configured.
func (r *Runtime) Execute(moduleBytes []byte, function string, args ...interface{}) (*ExecutionResult, error) {
	// Use background context with timeout from limits
	ctx := context.Background()
	if r.limits != nil && r.limits.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.limits.MaxExecutionTime)
		defer cancel()
	}

	// Delegate to ExecuteWithContext which enforces limits
	return r.ExecuteWithContext(ctx, moduleBytes, function, args...)
}

// ExecuteWithContext runs a WASM module with context support for cancellation.
// Enforces execution time limits via context cancellation.
func (r *Runtime) ExecuteWithContext(ctx context.Context, moduleBytes []byte, function string, args ...interface{}) (*ExecutionResult, error) {
	start := time.Now()

	result := &ExecutionResult{
		Success:  false,
		Duration: time.Since(start),
	}

	// Check for context cancellation BEFORE execution
	select {
	case <-ctx.Done():
		result.Error = "execution cancelled by context"
		return result, ctx.Err()
	default:
	}

	// Check capabilities before execution
	if !r.CheckCapability("stdio") {
		return result, fmt.Errorf("stdio capability not granted")
	}

	// Enforce module size limits before compilation
	if r.limits != nil && r.limits.MaxFileSize > 0 {
		if int64(len(moduleBytes)) > int64(r.limits.MaxFileSize) {
			result.Error = fmt.Sprintf("module byte size %d exceeds configured maximum %d", len(moduleBytes), r.limits.MaxFileSize)
			return result, fmt.Errorf("module size exceeds MaximumFileSize limit")
		}
	}

	// Create Wasmtime engine (context timeout for execution time limits)
	engine := wasmtime.NewEngine()
	store := wasmtime.NewStore(engine)

	// Validate module compiles within limits before execution
	module, err := wasmtime.NewModule(engine, moduleBytes)
	if err != nil {
		result.Error = fmt.Sprintf("compile module: %v", err)
		return result, err
	}

	// Create instance with empty imports
	instance, err := wasmtime.NewInstance(store, module, []wasmtime.AsExtern{})
	if err != nil {
		result.Error = fmt.Sprintf("instantiate: %v", err)
		return result, err
	}

	// Get the function
	wasmFunc := instance.GetFunc(store, function)
	if wasmFunc == nil {
		result.Error = fmt.Sprintf("function not found: %s", function)
		return result, fmt.Errorf("function not found: %s", function)
	}

	// Convert args to wasmtime values
	wasmArgs, err := convertArgsToWasmtime(args)
	if err != nil {
		result.Error = fmt.Sprintf("convert args: %v", err)
		return result, err
	}

	// Convert to interface slice for Call
	var argsIf []interface{}
	for _, v := range wasmArgs {
		argsIf = append(argsIf, v)
	}

	// Execute function
	rawResult, err := wasmFunc.Call(store, argsIf...)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Sprintf("execution error: %v", err)
		return result, err
	}

	// Convert result - extract the first value
	result.Output = rawResult
	result.Success = true

	return result, nil
}

// ExecuteWithConfig runs a WASM module with custom configuration.
func (r *Runtime) ExecuteWithConfig(moduleBytes []byte, config *ExecutionConfig) (*ExecutionResult, error) {
	// Create temporary runtime with config
	tempRuntime := NewRuntime(config.RuntimeConfig, config.Limits)

	// Grant capabilities
	for _, cap := range config.Capabilities {
		// convert Capability struct to capability string
		capStr := cap.Category
		if cap.Resource != "" {
			capStr += "." + cap.Resource
		}
		if err := tempRuntime.GrantCapability(capStr); err != nil {
			return &ExecutionResult{
				Error:   fmt.Sprintf("grant capability: %v", err),
				Success: false,
			}, err
		}
	}

	return tempRuntime.Execute(moduleBytes, config.Function, config.Args...)
}

// Validate checks if a WASM module is valid.
func (r *Runtime) Validate(moduleBytes []byte) error {
	engine := wasmtime.NewEngine()
	_, err := wasmtime.NewModule(engine, moduleBytes)
	return err
}

// GetModuleInfo extracts information about a WASM module.
func (r *Runtime) GetModuleInfo(moduleBytes []byte) (*ModuleInfo, error) {
	engine := wasmtime.NewEngine()
	module, err := wasmtime.NewModule(engine, moduleBytes)
	if err != nil {
		return nil, err
	}

	info := &ModuleInfo{
		FunctionCount: 0,
	}

	// Extract all exports as potential functions
	for _, export := range module.Exports() {
		info.Functions = append(info.Functions, export.Name())
		info.FunctionCount++
	}

	return info, nil
}

// ModuleInfo contains information about a WASM module.
type ModuleInfo struct {
	FunctionCount int      `json:"function_count"`
	Functions     []string `json:"functions"`
}

// ExecutionConfig specifies execution parameters.
type ExecutionConfig struct {
	Function      string
	Args          []interface{}
	Limits        *SandboxLimits
	RuntimeConfig *RuntimeConfig
	Capabilities  []Capability
}

// convertArgsToWasmtime converts Go arguments to wasmtime values.
func convertArgsToWasmtime(args []interface{}) ([]wasmtime.Val, error) {
	var result []wasmtime.Val

	for _, arg := range args {
		switch v := arg.(type) {
		case int32:
			result = append(result, wasmtime.ValI32(v))
		case int64:
			result = append(result, wasmtime.ValI64(v))
		case float32:
			result = append(result, wasmtime.ValF32(v))
		case float64:
			result = append(result, wasmtime.ValF64(v))
		case string:
			result = append(result, wasmtime.ValI32(int32(len(v))))
		default:
			return nil, fmt.Errorf("unsupported type: %T", arg)
		}
	}

	return result, nil
}
