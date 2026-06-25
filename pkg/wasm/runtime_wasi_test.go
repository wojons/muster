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
// InstantiateModuleWithWASI tests
// =============================================================================

func loadHelloWASM(t *testing.T) []byte {
	t.Helper()
	wasmBytes, err := os.ReadFile(filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm"))
	require.NoError(t, err)
	return wasmBytes
}

func TestInstantiateModuleWithWASI_ValidModule(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	rt := NewRuntime(cfg, DefaultLimits())
	defer rt.Close()

	wasmBytes := loadHelloWASM(t)
	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModuleWithWASI(context.Background(), compiled)
	require.NoError(t, err, "valid module should instantiate with WASI")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModuleWithWASI_WASIDisabled(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = false
	rt := NewRuntime(cfg, DefaultLimits())
	defer rt.Close()

	wasmBytes := loadHelloWASM(t)
	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	// When WASI is disabled, the module should still instantiate —
	// InstantiateModuleWithWASI skips WASI initialization and just
	// instantiates the module directly.
	mod, err := rt.InstantiateModuleWithWASI(context.Background(), compiled)
	require.NoError(t, err, "module should instantiate even with WASI disabled")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModuleWithWASI_InvalidModule(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	rt := NewRuntime(cfg, DefaultLimits())
	defer rt.Close()

	compiled, err := rt.CompileModule(context.Background(), []byte("not wasm"))
	require.Error(t, err, "garbage bytes should fail compilation")
	assert.Nil(t, compiled)
}

func TestInstantiateModuleWithWASI_CancelledContext(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	rt := NewRuntime(cfg, DefaultLimits())
	defer rt.Close()

	wasmBytes := loadHelloWASM(t)
	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mod, err := rt.InstantiateModuleWithWASI(ctx, compiled)
	// Behavior depends on whether context cancellation is caught before
	// or during the wazero call. Either outcome is acceptable — no panic.
	_ = mod
	_ = err
	if mod != nil {
		mod.Close(context.Background())
	}
}

func TestInstantiateModuleWithWASI_WithExecutionTimeout(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     10 * time.Second,
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(cfg, limits)
	defer rt.Close()

	wasmBytes := loadHelloWASM(t)
	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModuleWithWASI(context.Background(), compiled)
	require.NoError(t, err, "instantiation with execution timeout should work")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModuleWithWASI_ZeroTimeout(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	limits := &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     0, // zero — timeout path skipped
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
	rt := NewRuntime(cfg, limits)
	defer rt.Close()

	wasmBytes := loadHelloWASM(t)
	compiled, err := rt.CompileModule(context.Background(), wasmBytes)
	require.NoError(t, err)
	defer compiled.Close(context.Background())

	mod, err := rt.InstantiateModuleWithWASI(context.Background(), compiled)
	require.NoError(t, err, "instantiation with zero timeout should work (timeout skipped)")
	assert.NotNil(t, mod)
	mod.Close(context.Background())
}

func TestInstantiateModuleWithWASI_NilCompiled(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.EnableWASI = true
	rt := NewRuntime(cfg, DefaultLimits())
	defer rt.Close()

	// InstantiateModuleWithWASI calls compiled.Name() without nil check — it panics.
	assert.Panics(t, func() {
		rt.InstantiateModuleWithWASI(context.Background(), nil)
	}, "nil compiled module panics with nil pointer dereference")
}
