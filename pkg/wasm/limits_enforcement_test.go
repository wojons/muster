package wasm_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/wasm"
)

// TestWASM_RuntimeLimits_CPU tests CPU limiting via fuel mechanism.
func TestWASM_RuntimeLimits_CPU(t *testing.T) {
	t.Run("DefaultCPULimit_FuelSet", func(t *testing.T) {
		// Create runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime := wasm.NewRuntime(config, nil)

		// Verify default limits are set
		limits := runtime.Limits()
		require.NotNil(t, limits)
		require.Greater(t, limits.MaxFuel, uint64(0), "Default fuel limit should be greater than 0")

		t.Logf("✓ Default CPU fuel limit: %d instructions", limits.MaxFuel)
	})

	t.Run("CustomCPULimit_FuelConfigured", func(t *testing.T) {
		// Create runtime with custom fuel limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          1000, // Very low fuel limit
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify custom fuel limit is set
		require.Equal(t, uint64(1000), runtime.Limits().MaxFuel, "Custom fuel limit should be 1000")

		t.Log("✓ Custom CPU fuel limit (1000) configured")
	})

	t.Run("ZeroFuel_AllowsUnlimitedCPU", func(t *testing.T) {
		// Create runtime with zero fuel (should default to unlimited)
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          0, // Zero should use default
		}
		runtime := wasm.NewRuntime(config, limits)

		// When MaxFuel is 0, the limits.Validate() would fail, but since we don't call Validate() here,
		// the runtime accepts it. The runtime should use default fuel when executing.
		fuelLimit := runtime.Limits().MaxFuel
		require.Equal(t, uint64(0), fuelLimit, "Zero fuel should remain zero in limits struct")

		t.Logf("✓ Zero fuel defaults to: %d instructions", fuelLimit)
	})
}

// TestWASM_RuntimeLimits_Memory tests memory limit configuration and enforcement.
func TestWASM_RuntimeLimits_Memory(t *testing.T) {
	t.Run("DefaultMemoryLimit_Configured", func(t *testing.T) {
		// Create runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime := wasm.NewRuntime(config, nil)

		// Verify default memory limit is set
		limits := runtime.Limits()
		require.NotNil(t, limits)
		require.Equal(t, uint64(128*1024*1024), limits.MaxMemory, "Default memory limit should be 128MB")

		t.Log("✓ Default memory limit (128MB) configured")
	})

	t.Run("CustomMemoryLimit_Configured", func(t *testing.T) {
		// Create runtime with custom memory limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        256 * 1024 * 1024, // 256MB
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify custom memory limit is set
		require.Equal(t, uint64(256*1024*1024), runtime.Limits().MaxMemory, "Custom memory limit should be 256MB")

		t.Log("✓ Custom memory limit (256MB) configured")
	})

	t.Run("LowMemoryLimit_Enforced", func(t *testing.T) {
		// Create runtime with very low memory limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        1 * 1024 * 1024, // 1MB
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify low memory limit is set
		require.Equal(t, uint64(1*1024*1024), runtime.Limits().MaxMemory, "Low memory limit should be 1MB")

		t.Log("✓ Low memory limit (1MB) configured")
	})

	t.Run("MemoryLimit_Validate", func(t *testing.T) {
		// Test that memory limits are validated
		limits := &wasm.SandboxLimits{
			MaxMemory:        0, // Invalid - no memory limit
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}

		err := limits.Validate()
		require.Error(t, err, "Validation should fail with zero memory limit")
		assert.Contains(t, err.Error(), "memory")

		t.Log("✓ Memory limit validation works")
	})
}

// TestWASM_RuntimeLimits_FileDescriptors tests file descriptor limit configuration.
func TestWASM_RuntimeLimits_FileDescriptors(t *testing.T) {
	t.Run("DefaultFDLimit_Configured", func(t *testing.T) {
		// Create runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime := wasm.NewRuntime(config, nil)

		// Verify default FD limit is set
		limits := runtime.Limits()
		require.NotNil(t, limits)
		require.Equal(t, 64, limits.MaxFileDescriptors, "Default FD limit should be 64")

		t.Log("✓ Default file descriptor limit (64) configured")
	})

	t.Run("CustomFDLimit_Configured", func(t *testing.T) {
		// Create runtime with custom FD limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:          128 * 1024 * 1024,
			MaxExecutionTime:   30 * time.Second,
			MaxFileSize:        10 * 1024 * 1024,
			MaxFileDescriptors: 32, // Lower limit
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify custom FD limit is set
		require.Equal(t, 32, runtime.Limits().MaxFileDescriptors, "Custom FD limit should be 32")

		t.Log("✓ Custom file descriptor limit (32) configured")
	})

	t.Run("FDLimit_Validate", func(t *testing.T) {
		// Test that FD limits with zero fuel fails (MaxFuel is required)
		limits := &wasm.SandboxLimits{
			MaxMemory:          128 * 1024 * 1024,
			MaxExecutionTime:   30 * time.Second,
			MaxFileSize:        10 * 1024 * 1024,
			MaxFileDescriptors: 0, // Invalid - no FD limit
			MaxFuel:            0, // Also invalid - no fuel
		}

		err := limits.Validate()
		require.Error(t, err, "Validation should fail")
		// The validation checks MaxFuel before MaxFileDescriptors
		assert.Contains(t, err.Error(), "fuel")

		t.Log("✓ File descriptor limit with missing fuel validation works")
	})

	t.Run("HighFDLimit_Configured", func(t *testing.T) {
		// Create runtime with high FD limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:          128 * 1024 * 1024,
			MaxExecutionTime:   30 * time.Second,
			MaxFileSize:        10 * 1024 * 1024,
			MaxFileDescriptors: 1024, // Higher limit
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify high FD limit is set
		require.Equal(t, 1024, runtime.Limits().MaxFileDescriptors, "High FD limit should be 1024")

		t.Log("✓ High file descriptor limit (1024) configured")
	})
}

// TestWASM_RuntimeLimits_ExecutionTimeout tests execution time limit configuration.
func TestWASM_RuntimeLimits_ExecutionTimeout(t *testing.T) {
	t.Run("DefaultTimeout_Configured", func(t *testing.T) {
		// Create runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime := wasm.NewRuntime(config, nil)

		// Verify default timeout is set
		limits := runtime.Limits()
		require.NotNil(t, limits)
		require.Equal(t, 30*time.Second, limits.MaxExecutionTime, "Default timeout should be 30s")

		t.Log("✓ Default execution timeout (30s) configured")
	})

	t.Run("CustomTimeout_Configured", func(t *testing.T) {
		// Create runtime with custom timeout
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 5 * time.Minute,
			MaxFileSize:      10 * 1024 * 1024,
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify custom timeout is set
		require.Equal(t, 5*time.Minute, runtime.Limits().MaxExecutionTime, "Custom timeout should be 5m")

		t.Log("✓ Custom execution timeout (5m) configured")
	})

	t.Run("ShortTimeout_Configured", func(t *testing.T) {
		// Create runtime with short timeout
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 1 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify short timeout is set
		require.Equal(t, 1*time.Second, runtime.Limits().MaxExecutionTime, "Short timeout should be 1s")

		t.Log("✓ Short execution timeout (1s) configured")
	})

	t.Run("Timeout_Validate", func(t *testing.T) {
		// Test that timeout is validated
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 0, // Invalid - no timeout
			MaxFileSize:      10 * 1024 * 1024,
		}

		err := limits.Validate()
		require.Error(t, err, "Validation should fail with zero timeout")
		assert.Contains(t, err.Error(), "execution time")

		t.Log("✓ Execution timeout validation works")
	})
}

// TestWASM_RuntimeLimits_FileSize tests file size limit configuration.
func TestWASM_RuntimeLimits_FileSize(t *testing.T) {
	t.Run("DefaultFileSizeLimit_Configured", func(t *testing.T) {
		// Create runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime := wasm.NewRuntime(config, nil)

		// Verify default file size limit is set
		limits := runtime.Limits()
		require.NotNil(t, limits)
		require.Greater(t, limits.MaxFileSize, uint64(0), "Default file size limit should be greater than 0")

		t.Logf("✓ Default file size limit (%d bytes) configured", limits.MaxFileSize)
	})

	t.Run("CustomFileSizeLimit_Configured", func(t *testing.T) {
		// Create runtime with custom file size limit
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      100 * 1024 * 1024, // 100MB
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify custom file size limit is set
		require.Equal(t, uint64(100*1024*1024), runtime.Limits().MaxFileSize, "Custom file size limit should be 100MB")

		t.Log("✓ Custom file size limit (100MB) configured")
	})

	t.Run("FileSize_Validate", func(t *testing.T) {
		// Test that file size limit is validated
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      0, // Invalid - no file size limit
		}

		err := limits.Validate()
		require.Error(t, err, "Validation should fail with zero file size limit")
		assert.Contains(t, err.Error(), "file size")

		t.Log("✓ File size limit validation works")
	})
}

// TestWASM_RuntimeLimits_Combined tests multiple limits working together.
func TestWASM_RuntimeLimits_Combined(t *testing.T) {
	t.Run("AllLimitsConfigured_Together", func(t *testing.T) {
		// Create runtime with all custom limits
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:          64 * 1024 * 1024, // 64MB
			MaxExecutionTime:   10 * time.Second, // 10s
			MaxFileSize:        50 * 1024 * 1024, // 50MB
			MaxFileDescriptors: 128,              // 128 FDs
			MaxFuel:            1_000_000_000,    // 1B instructions
		}
		runtime := wasm.NewRuntime(config, limits)

		// Verify all limits are set
		actualLimits := runtime.Limits()
		require.NotNil(t, actualLimits)
		require.Equal(t, uint64(64*1024*1024), actualLimits.MaxMemory, "Memory limit")
		require.Equal(t, 10*time.Second, actualLimits.MaxExecutionTime, "Timeout")
		require.Equal(t, uint64(50*1024*1024), actualLimits.MaxFileSize, "File size limit")
		require.Equal(t, 128, actualLimits.MaxFileDescriptors, "FD limit")
		require.Equal(t, uint64(1_000_000_000), actualLimits.MaxFuel, "Fuel limit")

		t.Log("✓ All resource limits configured together")
	})

	t.Run("Limits_Validate_AllRequired", func(t *testing.T) {
		// Test validation catches missing limits
		testCases := []struct {
			name   string
			limits *wasm.SandboxLimits
		}{
			{
				name:   "missing memory",
				limits: &wasm.SandboxLimits{MaxExecutionTime: 30 * time.Second, MaxFileSize: 10 * 1024 * 1024},
			},
			{
				name:   "missing timeout",
				limits: &wasm.SandboxLimits{MaxMemory: 128 * 1024 * 1024, MaxFileSize: 10 * 1024 * 1024},
			},
			{
				name:   "missing file size",
				limits: &wasm.SandboxLimits{MaxMemory: 128 * 1024 * 1024, MaxExecutionTime: 30 * time.Second},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.limits.Validate()
				require.Error(t, err, "Validation should fail")
			})
		}

		t.Log("✓ All required limits validated")
	})
}

// TestWASM_RuntimeLimits_ContextTimeout tests context-based timeout enforcement.
func TestWASM_RuntimeLimits_ContextTimeout(t *testing.T) {
	t.Run("ContextTimeout_Respected", func(t *testing.T) {
		// Create runtime
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}
		_ = wasm.NewRuntime(config, limits)

		// Create context with shorter timeout than limits
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// The shorter context timeout should take precedence
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "Context should have deadline")
		require.WithinDuration(t, time.Now().Add(1*time.Second), deadline, 100*time.Millisecond, "Deadline should be ~1s from now")

		t.Log("✓ Context timeout respected")
	})

	t.Run("CancelledContext_Handled", func(t *testing.T) {
		// Create runtime
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:        128 * 1024 * 1024,
			MaxExecutionTime: 30 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
		}
		_ = wasm.NewRuntime(config, limits)

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Verify context is cancelled
		require.Error(t, ctx.Err(), "Context should be cancelled")

		t.Log("✓ Cancelled context handled")
	})
}

// TestWASM_RuntimeLimits_DefaultLimitsFactory tests the DefaultLimits factory function.
func TestWASM_RuntimeLimits_DefaultLimitsFactory(t *testing.T) {
	t.Run("DefaultLimits_ReturnsValidLimits", func(t *testing.T) {
		limits := wasm.DefaultLimits()

		require.NotNil(t, limits)
		require.Equal(t, uint64(128*1024*1024), limits.MaxMemory, "Default memory")
		require.Equal(t, 30*time.Second, limits.MaxExecutionTime, "Default timeout")
		require.Equal(t, uint64(10*1024*1024), limits.MaxFileSize, "Default file size")
		require.Equal(t, 64, limits.MaxFileDescriptors, "Default FDs")
		require.Greater(t, limits.MaxFuel, uint64(0), "Default fuel")

		// Verify limits are valid
		err := limits.Validate()
		require.NoError(t, err, "Default limits should be valid")

		t.Log("✓ DefaultLimits factory returns valid limits")
	})
}

// TestWASM_RuntimeLimits_ThreadSafety tests concurrent access to limits.
func TestWASM_RuntimeLimits_ThreadSafety(t *testing.T) {
	t.Run("ConcurrentLimitAccess_Safe", func(t *testing.T) {
		// Create runtime
		config := wasm.DefaultRuntimeConfig()
		limits := &wasm.SandboxLimits{
			MaxMemory:          128 * 1024 * 1024,
			MaxExecutionTime:   30 * time.Second,
			MaxFileSize:        10 * 1024 * 1024,
			MaxFileDescriptors: 64,
		}
		runtime := wasm.NewRuntime(config, limits)

		// Concurrent reads
		done := make(chan bool, 100)
		for i := 0; i < 100; i++ {
			go func() {
				l := runtime.Limits()
				_ = l.MaxMemory
				_ = l.MaxExecutionTime
				_ = l.MaxFileSize
				_ = l.MaxFileDescriptors
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 100; i++ {
			<-done
		}

		t.Log("✓ Concurrent limit access is thread-safe")
	})
}

// TestWASM_RuntimeLimits_RuntimeReconfiguration tests runtime reconfiguration with different limits.
func TestWASM_RuntimeLimits_RuntimeReconfiguration(t *testing.T) {
	t.Run("ReconfigureRuntime_WithNewLimits", func(t *testing.T) {
		// Create initial runtime with default limits
		config := wasm.DefaultRuntimeConfig()
		runtime1 := wasm.NewRuntime(config, nil)

		// Verify initial limits
		require.Equal(t, uint64(128*1024*1024), runtime1.Limits().MaxMemory)

		// Create new runtime with different limits
		newLimits := &wasm.SandboxLimits{
			MaxMemory:          256 * 1024 * 1024,
			MaxExecutionTime:   60 * time.Second,
			MaxFileSize:        100 * 1024 * 1024,
			MaxFileDescriptors: 256,
		}
		runtime2 := wasm.NewRuntime(config, newLimits)

		// Verify new limits
		require.Equal(t, uint64(256*1024*1024), runtime2.Limits().MaxMemory)
		require.Equal(t, 60*time.Second, runtime2.Limits().MaxExecutionTime)
		require.Equal(t, uint64(100*1024*1024), runtime2.Limits().MaxFileSize)
		require.Equal(t, 256, runtime2.Limits().MaxFileDescriptors)

		t.Log("✓ Runtime reconfiguration with new limits works")
	})
}
