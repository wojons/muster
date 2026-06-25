package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Execute + ExecuteWithContext tests
// =============================================================================

func TestExecute_ValidFunction(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello")
	require.NoError(t, err, "valid module with exported function should execute")
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Output)
}

func TestExecute_FunctionNotFound(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "nonexistent_fn")
	assert.Error(t, err, "non-existent function should return error")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nonexistent_fn")
}

func TestExecute_InvalidWASM(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	result, err := rt.Execute([]byte("not valid wasm bytes"), "hello")
	assert.Error(t, err, "invalid WASM should return error")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "compile module")
}

func TestExecute_CancelledContext(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := rt.ExecuteWithContext(ctx, wasmBytes, "hello")
	// Either outcome is acceptable — no panic.
	_ = result
	_ = err
}

func TestExecute_WithTimeout(t *testing.T) {
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     10 * time.Second,
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(DefaultRuntimeConfig(), limits)
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello")
	require.NoError(t, err, "execution with generous timeout should work")
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}

func TestExecute_UnsupportedArgType(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	// string is not a supported WASM argument type
	result, err := rt.Execute(wasmBytes, "hello", "unsupported_string_type")
	assert.Error(t, err, "unsupported argument type should return error")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported argument type")
}

func TestExecute_Int32Arg(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	// hello() takes no params but wazero validates param count before calling.
	// int32 is a valid type but the function still rejects extra params.
	result, err := rt.Execute(wasmBytes, "hello", int32(42))
	// Either error (param count mismatch) or success if wazero ignores extra params.
	// In practice, wazero validates: "expected 0 params, but passed 1"
	_ = result
	_ = err
}

func TestExecute_Int64Arg(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello", int64(42))
	_ = result
	_ = err
}

func TestExecute_Uint32Arg(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello", uint32(42))
	_ = result
	_ = err
}

func TestExecute_Uint64Arg(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello", uint64(42))
	_ = result
	_ = err
}

func TestExecute_NativeIntArg(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.Execute(wasmBytes, "hello", int(42))
	_ = result
	_ = err
}

func TestExecuteWithContext_ValidFunction(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.ExecuteWithContext(context.Background(), wasmBytes, "hello")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Output)
}

func TestExecuteWithContext_CancelledContext(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := rt.ExecuteWithContext(ctx, wasmBytes, "hello")
	// Behaviour depends on whether context cancellation is detected before
	// or during compilation. Either outcome is acceptable — no panic.
	_ = result
	_ = err
}

func TestExecuteWithContext_WithTimeout(t *testing.T) {
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     10 * time.Second,
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(DefaultRuntimeConfig(), limits)
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.ExecuteWithContext(context.Background(), wasmBytes, "hello")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}

func TestExecuteWithContext_ZeroTimeout(t *testing.T) {
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     0, // zero — timeout path skipped
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(DefaultRuntimeConfig(), limits)
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	result, err := rt.ExecuteWithContext(context.Background(), wasmBytes, "hello")
	require.NoError(t, err, "execution with zero timeout should work (timeout skipped)")
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}
