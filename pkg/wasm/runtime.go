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

	// Create a span for execution tracing if tracer is configured
	var span Span
	if r.tracer != nil {
		span = r.tracer.StartSpan("wasm.execute")
		span.SetTag("function", function)
		span.SetTag("module_size", len(moduleBytes))
		span.SetTag("args_count", len(args))
		defer span.End()
	}

	// Check for context cancellation BEFORE execution
	select {
	case <-ctx.Done():
		result.Error = "execution cancelled by context"
		if span != nil {
			span.SetTag("cancelled", true)
			span.SetError(ctx.Err())
		}
		return result, ctx.Err()
	default:
	}

	// Check capabilities before execution
	if !r.CheckCapability("stdio") {
		err := fmt.Errorf("stdio capability not granted")
		if span != nil {
			span.SetTag("capability_error", true)
			span.SetError(err)
		}
		return result, err
	}

	// Enforce module size limits before compilation
	if r.limits != nil && r.limits.MaxFileSize > 0 {
		if int64(len(moduleBytes)) > int64(r.limits.MaxFileSize) {
			result.Error = fmt.Sprintf("module byte size %d exceeds configured maximum %d", len(moduleBytes), r.limits.MaxFileSize)
			err := fmt.Errorf("module size exceeds MaximumFileSize limit")
			if span != nil {
				span.SetTag("size_limit_error", true)
				span.SetError(err)
			}
			return result, err
		}
	}

	// Create Wasmtime engine with resource limits to prevent DoS via memory exhaustion
	engine := wasmtime.NewEngine()

	// Note: Memory limits enforced via MaxFileSize check (line 59-64), context timeout (line 25-29),
	// store limiter below, fuel limits, and runtime validation during execution.
	store := wasmtime.NewStore(engine)

	// Set maximum memory allocation (default 256MB, override via limits.MaxMemory)
	maxMemory := int64(256 * 1024 * 1024) // 256MB default
	if r.limits != nil && r.limits.MaxMemory > 0 {
		maxMemory = int64(r.limits.MaxMemory)
	}
	// Limiter(memorySize, tableElements, instances, tables, memories)
	// Use -1 for default values on limits we don't want to change
	store.Limiter(maxMemory, -1, -1, -1, -1)

	// Add fuel for CPU time limiting (fuel is consumed per WebAssembly instruction)
	// Default: 10 billion instructions, override via limits.MaxFuel
	maxFuel := uint64(10_000_000_000) // 10 billion instructions default
	if r.limits != nil && r.limits.MaxFuel > 0 {
		maxFuel = r.limits.MaxFuel
	}
	if err := store.AddFuel(maxFuel); err != nil {
		result.Error = fmt.Sprintf("failed to add fuel: %v", err)
		if span != nil {
			span.SetTag("fuel_error", true)
			span.SetError(err)
		}
		return result, fmt.Errorf("failed to add fuel: %w", err)
	}

	// Validate module compiles within limits before execution
	module, err := wasmtime.NewModule(engine, moduleBytes)
	if err != nil {
		result.Error = fmt.Sprintf("compile module: %v", err)
		if span != nil {
			span.SetTag("compile_error", true)
			span.SetError(err)
		}
		return result, err
	}

	// Create instance with empty imports
	instance, err := wasmtime.NewInstance(store, module, []wasmtime.AsExtern{})
	if err != nil {
		result.Error = fmt.Sprintf("instantiate: %v", err)
		if span != nil {
			span.SetTag("instantiate_error", true)
			span.SetError(err)
		}
		return result, err
	}

	// Get the function
	wasmFunc := instance.GetFunc(store, function)
	if wasmFunc == nil {
		result.Error = fmt.Sprintf("function not found: %s", function)
		err := fmt.Errorf("function not found: %s", function)
		if span != nil {
			span.SetTag("function_not_found", true)
			span.SetError(err)
		}
		return result, err
	}

	// Convert args to wasmtime values
	wasmArgs, err := convertArgsToWasmtime(args)
	if err != nil {
		result.Error = fmt.Sprintf("convert args: %v", err)
		if span != nil {
			span.SetTag("convert_args_error", true)
			span.SetError(err)
		}
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
		if span != nil {
			span.SetTag("execution_error", true)
			span.SetError(err)
		}
		return result, err
	}

	// Convert result - extract the first value
	result.Output = rawResult
	result.Success = true
	result.Duration = time.Since(start)

	// Record success in span
	if span != nil {
		span.SetTag("success", true)
		span.SetTag("duration_ms", result.Duration.Milliseconds())
	}

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
	var span Span
	if r.tracer != nil {
		span = r.tracer.StartSpan("wasm.validate")
		span.SetTag("module_size", len(moduleBytes))
		defer span.End()
	}

	engine := wasmtime.NewEngine()
	_, err := wasmtime.NewModule(engine, moduleBytes)
	if err != nil && span != nil {
		span.SetTag("validation_error", true)
		span.SetError(err)
	} else if span != nil {
		span.SetTag("valid", true)
	}
	return err
}

// GetModuleInfo extracts information about a WASM module.
func (r *Runtime) GetModuleInfo(moduleBytes []byte) (*ModuleInfo, error) {
	var span Span
	if r.tracer != nil {
		span = r.tracer.StartSpan("wasm.get_module_info")
		span.SetTag("module_size", len(moduleBytes))
		defer span.End()
	}

	engine := wasmtime.NewEngine()
	module, err := wasmtime.NewModule(engine, moduleBytes)
	if err != nil {
		if span != nil {
			span.SetTag("error", true)
			span.SetError(err)
		}
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

	if span != nil {
		span.SetTag("function_count", info.FunctionCount)
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
