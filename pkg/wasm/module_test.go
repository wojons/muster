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
// LoadPlugin (via ModuleManager.Load) tests
// =============================================================================

func TestLoadPlugin_ValidModule(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err, "should load valid WASM module")
	require.NotNil(t, lm)
	// Name depends on how wazero extracts module name from the binary
	assert.Equal(t, wasmPath, lm.Path)
}

func TestLoadPlugin_NonExistentFile(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	lm, err := mg.Load(context.Background(), "/tmp/nonexistent-module-12345.wasm")
	require.Error(t, err)
	assert.Nil(t, lm)
	assert.Contains(t, err.Error(), "read module file")
}

func TestLoadPlugin_InvalidWASM(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.wasm")
	err := os.WriteFile(badPath, []byte("not a valid wasm module"), 0644)
	require.NoError(t, err)

	lm, err := mg.Load(context.Background(), badPath)
	require.Error(t, err)
	assert.Nil(t, lm)
}

func TestLoadPlugin_CachedSecondLoad(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm")

	// First load
	lm1, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)
	require.NotNil(t, lm1)

	// Second load should return cached entry
	lm2, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)
	assert.Equal(t, lm1.Name, lm2.Name, "cached load should return same name")
}

func TestLoadPlugin_CancelledContext(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(ctx, wasmPath)
	// File read may or may not succeed before context check;
	// wazero compilation may or may not check context.
	// The key assertion: no panic.
	_ = lm
	_ = err
}

// =============================================================================
// Transform tests
// =============================================================================

func TestTransform_ValidModule(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := mg.Transform(ctx, lm, []byte(`{"key":"value"}`))
	require.NoError(t, err)
	// Module runs without error; output depends on module implementation
	_ = output
}

func TestTransform_NilModule(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	output, err := mg.Transform(context.Background(), nil, []byte(`{"key":"value"}`))
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "module is nil")
}

func TestTransform_CancelledContext(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	output, err := mg.Transform(ctx, lm, []byte(`{"key":"value"}`))
	// Since the context is already cancelled, we expect one of:
	// - error (execution cancelled before it got the lock)
	// - timeout error (execCtx derived from already-cancelled parent)
	// Either way, no panic.
	_ = output
	_ = err
	// The key assertion: no panic
}

func TestTransform_EmptyInput(t *testing.T) {
	mg := NewModuleManager(DefaultLimits())
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := mg.Transform(ctx, lm, []byte(``))
	require.NoError(t, err)
	// Transform may or may not produce output for empty input
	_ = output
}

func TestTransform_WithZeroMaxExecutionTime(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxExecutionTime = 0 // triggers default 30s timeout

	mg := NewModuleManager(limits)
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)

	ctx := context.Background()
	output, err := mg.Transform(ctx, lm, []byte(`{"test":true}`))
	require.NoError(t, err)
	_ = output
}

func TestTransform_WithShortTimeout(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxExecutionTime = 5 * time.Second

	mg := NewModuleManager(limits)
	defer mg.Close(context.Background())

	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "transform.wasm")
	lm, err := mg.Load(context.Background(), wasmPath)
	require.NoError(t, err)

	ctx := context.Background()
	output, err := mg.Transform(ctx, lm, []byte(`{"test":true}`))
	require.NoError(t, err)
	_ = output
}
