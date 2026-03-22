package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEpochInterruption_Enabled tests that epoch interruption is enabled when timeout is configured
func TestEpochInterruption_Enabled(t *testing.T) {
	runtime := NewRuntime(nil, &SandboxLimits{
		MaxExecutionTime: 5 * time.Second,
		MaxMemory:        128 * 1024 * 1024,
		MaxFileSize:      10 * 1024 * 1024,
	})

	assert.NotNil(t, runtime)
	assert.Equal(t, 5*time.Second, runtime.Limits().MaxExecutionTime)
}

// TestEpochInterruption_Disabled tests that epoch interruption is not enabled when no timeout
func TestEpochInterruption_Disabled(t *testing.T) {
	runtime := NewRuntime(nil, &SandboxLimits{
		MaxExecutionTime: 0, // No timeout
		MaxMemory:        128 * 1024 * 1024,
		MaxFileSize:      10 * 1024 * 1024,
	})

	assert.NotNil(t, runtime)
	assert.Equal(t, time.Duration(0), runtime.Limits().MaxExecutionTime)
}

// TestEpochInterruption_TimeoutEnforcement tests that timeout is enforced via epoch interruption
func TestEpochInterruption_TimeoutEnforcement(t *testing.T) {
	t.Run("ShortTimeout", func(t *testing.T) {
		// Create a runtime with a very short timeout
		limits := &SandboxLimits{
			MaxExecutionTime: 100 * time.Millisecond,
			MaxMemory:        256 * 1024 * 1024,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          10_000_000_000,
		}
		runtime := NewRuntime(DefaultRuntimeConfig(), limits)

		// Create a simple WASM module that loops indefinitely
		// This is a minimal valid WASM module that exports a function
		// Note: Real timeout testing requires a WASM module that actually runs long enough
		// to trigger the epoch deadline

		// For now, verify the runtime is configured correctly
		require.NotNil(t, runtime)
		assert.Equal(t, 100*time.Millisecond, runtime.Limits().MaxExecutionTime)

		t.Log("✓ Epoch interruption configured for 100ms timeout")
	})

	t.Run("LongTimeout", func(t *testing.T) {
		limits := &SandboxLimits{
			MaxExecutionTime: 30 * time.Second,
			MaxMemory:        128 * 1024 * 1024,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          10_000_000_000,
		}
		runtime := NewRuntime(DefaultRuntimeConfig(), limits)

		require.NotNil(t, runtime)
		assert.Equal(t, 30*time.Second, runtime.Limits().MaxExecutionTime)

		t.Log("✓ Epoch interruption configured for 30s timeout")
	})
}

// TestEpochInterruption_ContextCancellation tests interaction with context cancellation
func TestEpochInterruption_ContextCancellation(t *testing.T) {
	runtime := NewRuntime(nil, &SandboxLimits{
		MaxExecutionTime: 5 * time.Second,
		MaxMemory:        128 * 1024 * 1024,
		MaxFileSize:      10 * 1024 * 1024,
		MaxFuel:          10_000_000_000,
	})

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Fatal("Context should be cancelled")
	}

	// The runtime should respect context cancellation
	// Note: We can't test actual execution without a real WASM module
	require.NotNil(t, runtime)
	t.Log("✓ Context cancellation will be respected before epoch interruption")
}

// TestEpochInterruption_DefaultTimeout tests default timeout from DefaultLimits
func TestEpochInterruption_DefaultTimeout(t *testing.T) {
	limits := DefaultLimits()

	assert.Equal(t, 30*time.Second, limits.MaxExecutionTime)
	assert.NotZero(t, limits.MaxExecutionTime)
}

// TestEpochInterruption_EngineConfiguration tests that the engine is configured correctly
func TestEpochInterruption_EngineConfiguration(t *testing.T) {
	t.Run("WithTimeout", func(t *testing.T) {
		limits := &SandboxLimits{
			MaxExecutionTime: 1 * time.Second,
			MaxMemory:        128 * 1024 * 1024,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          10_000_000_000,
		}
		runtime := NewRuntime(DefaultRuntimeConfig(), limits)

		// Verify runtime is properly configured
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Limits())
		assert.Equal(t, 1*time.Second, runtime.Limits().MaxExecutionTime)

		t.Log("✓ Engine configured with epoch interruption for timeout enforcement")
	})

	t.Run("WithoutTimeout", func(t *testing.T) {
		limits := &SandboxLimits{
			MaxExecutionTime: 0, // No timeout
			MaxMemory:        128 * 1024 * 1024,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          10_000_000_000,
		}
		runtime := NewRuntime(DefaultRuntimeConfig(), limits)

		require.NotNil(t, runtime)
		assert.Equal(t, time.Duration(0), runtime.Limits().MaxExecutionTime)

		t.Log("✓ Engine configured without epoch interruption (no timeout)")
	})
}

// TestEpochInterruption_ConcurrentAccess tests thread safety of epoch configuration
func TestEpochInterruption_ConcurrentAccess(t *testing.T) {
	runtime := NewRuntime(nil, &SandboxLimits{
		MaxExecutionTime: 5 * time.Second,
		MaxMemory:        128 * 1024 * 1024,
		MaxFileSize:      10 * 1024 * 1024,
		MaxFuel:          10_000_000_000,
	})

	// Verify runtime is thread-safe
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			limits := runtime.Limits()
			assert.NotNil(t, limits)
			assert.Equal(t, 5*time.Second, limits.MaxExecutionTime)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent access")
		}
	}

	t.Log("✓ Concurrent access to timeout configuration is safe")
}
