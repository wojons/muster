package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CompileModule tests
// =============================================================================

func TestCompileModule_ValidWASM(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err, "should be able to read hello.wasm")

	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err, "valid WASM bytes should compile")
	assert.NotNil(t, compiled)
	compiled.Close(context.Background())
}

func TestCompileModule_EmptyBytes(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	compiled, err := rt.CompileModule(context.Background(), []byte{})
	assert.Error(t, err, "empty WASM bytes should fail")
	assert.Nil(t, compiled)
}

func TestCompileModule_InvalidBytes(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	compiled, err := rt.CompileModule(context.Background(), []byte("not a valid wasm module"))
	assert.Error(t, err, "invalid WASM bytes should fail")
	assert.Nil(t, compiled)
}

func TestCompileModule_GarbageMagic(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	// Start with correct magic then garbage — fails parse
	compiled, err := rt.CompileModule(context.Background(), []byte{0x00, 0x61, 0x73, 0x6d, 0xFF, 0xFF, 0xFF, 0xFF})
	assert.Error(t, err, "garbage after magic should fail")
	assert.Nil(t, compiled)
}

func TestCompileModule_CancelledContext(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	// wazero may or may not check context during compilation
	// Accept either outcome; the point is no panic
	_ = compiled
	_ = err
}

// =============================================================================
// LoadModuleFromFile tests
// =============================================================================

func TestLoadModuleFromFile_ValidWASMFile(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm")
	compiled, err := rt.LoadModuleFromFile(context.Background(), wasmPath)
	require.NoError(t, err, "valid .wasm file should load and compile")
	require.NotNil(t, compiled)
	compiled.Close(context.Background())
}

func TestLoadModuleFromFile_NonExistent(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	compiled, err := rt.LoadModuleFromFile(context.Background(), "/tmp/nonexistent-module.wasm")
	assert.Error(t, err, "non-existent file should fail")
	assert.Nil(t, compiled)
	assert.Contains(t, err.Error(), "read module file")
}

func TestLoadModuleFromFile_Directory(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	tmpDir := t.TempDir()
	compiled, err := rt.LoadModuleFromFile(context.Background(), tmpDir)
	assert.Error(t, err, "directory path should fail (read returns error)")
	assert.Nil(t, compiled)
	assert.Contains(t, err.Error(), "read module file")
}

func TestLoadModuleFromFile_InvalidWASM(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	tmpFile := filepath.Join(t.TempDir(), "invalid.wasm")
	require.NoError(t, os.WriteFile(tmpFile, []byte("not wasm"), 0644))

	compiled, err := rt.LoadModuleFromFile(context.Background(), tmpFile)
	assert.Error(t, err, "invalid WASM content should fail at compile step")
	assert.Nil(t, compiled)
}

func TestLoadModuleFromFile_ContextCancelled(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	compiled, err := rt.LoadModuleFromFile(ctx, filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	// The file read may succeed even with cancelled context on some OSes;
	// the compile step may fail. Either is fine — verify no panic.
	_ = compiled
	_ = err
}

// =============================================================================
// InstantiateModule tests
// =============================================================================

func TestInstantiateModule_CompiledModule(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModule(context.Background(), compiled)
	require.NoError(t, err, "compiled module should instantiate")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModule_NilModule(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	// InstantiateModule accesses compiled.Name() without nil check — it panics.
	// This is the current behavior; document it.
	assert.Panics(t, func() {
		rt.InstantiateModule(context.Background(), nil)
	}, "nil compiled module panics with nil pointer dereference")
}

func TestInstantiateModule_ContextCancelled(t *testing.T) {
	rt := NewRuntime(DefaultRuntimeConfig(), DefaultLimits())
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mod, err := rt.InstantiateModule(ctx, compiled)
	// May or may not fail with cancelled context before the wazero call;
	// the important thing is no panic.
	_ = mod
	_ = err
}

func TestInstantiateModule_WithTimeLimit(t *testing.T) {
	// Create runtime with short execution time limit
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     10 * 1000 * 1000 * 1000, // 10 seconds in nanoseconds
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(DefaultRuntimeConfig(), limits)
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModule(context.Background(), compiled)
	require.NoError(t, err, "instantiation with time limit should work")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModule_ZeroTimeLimit(t *testing.T) {
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     0, // zero — timeout not applied
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(DefaultRuntimeConfig(), limits)
	defer rt.Close()

	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)

	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModule(context.Background(), compiled)
	require.NoError(t, err, "instantiation with zero time limit should work")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}
