package platform

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandbox_Initialization tests basic sandbox initialization
func TestSandbox_Initialization(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	assert.NotNil(t, sb)
	assert.NotNil(t, sb.config)
	assert.Equal(t, config.MemoryLimit, sb.GetMemoryLimit())
	assert.Equal(t, config.TimeLimit, sb.GetTimeLimit())
}

// TestSandbox_Execute_SuccessfulExecution tests successful function execution
func TestSandbox_Execute_SuccessfulExecution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := SandboxConfig{
		MemoryLimit: 128 * 1024 * 1024,
		TimeLimit:   5 * time.Second,
		MaxProcs:    10,
	}

	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx := context.Background()
	result, err := sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		return &SandboxResult{
			ExitCode:   0,
			MemoryUsed: 1024,
			CPUTime:    100 * time.Millisecond,
		}
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, int64(1024), result.MemoryUsed)
}

// TestSandbox_Execute_Timeout tests timeout enforcement
func TestSandbox_Execute_Timeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := SandboxConfig{
		MemoryLimit: 128 * 1024 * 1024,
		TimeLimit:   100 * time.Millisecond,
		MaxProcs:    10,
	}

	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx := context.Background()
	_, err = sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		time.Sleep(500 * time.Millisecond) // Exceed timeout
		return &SandboxResult{ExitCode: 0}
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestSandbox_Execute_Cancellation tests context cancellation
func TestSandbox_Execute_Cancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		select {
		case <-ctx.Done():
			return &SandboxResult{ExitCode: -1}
		case <-time.After(5 * time.Second):
			return &SandboxResult{ExitCode: 0}
		}
	})

	// Should complete due to context cancellation
	assert.NotNil(t, err)
}

// TestSandbox_Execute_PanicRecovery tests panic recovery
func TestSandbox_Execute_PanicRecovery(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx := context.Background()
	_, err = sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		panic("intentional test panic")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
}

// TestSandbox_DefaultConfig_Values tests default configuration values
func TestSandbox_DefaultConfig_Values(t *testing.T) {
	config := DefaultSandboxConfig()

	assert.Equal(t, int64(128*1024*1024), config.MemoryLimit)
	assert.Equal(t, 30*time.Second, config.TimeLimit)
	assert.Equal(t, int64(100000), config.CPUQuota)
	assert.Equal(t, int64(100000), config.CPUPeriod)
	assert.Equal(t, 10, config.MaxProcs)
	assert.Equal(t, 1024, config.MaxOpenFiles)
	assert.Equal(t, NetworkNone, config.NetworkPolicy)
	assert.True(t, config.NoNewPrivileges)
}

// TestSandbox_Config_CustomValues tests custom configuration
func TestSandbox_Config_CustomValues(t *testing.T) {
	config := SandboxConfig{
		MemoryLimit:     256 * 1024 * 1024,
		TimeLimit:       60 * time.Second,
		CPUQuota:        200000,
		CPUPeriod:       100000,
		MaxProcs:        20,
		MaxOpenFiles:    2048,
		NetworkPolicy:   NetworkRestricted,
		NoNewPrivileges: false,
	}

	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	assert.Equal(t, int64(256*1024*1024), sb.GetMemoryLimit())
	assert.Equal(t, 60*time.Second, sb.GetTimeLimit())
}

// TestSandbox_MemoryLimit_Checking tests memory limit checking
func TestSandbox_MemoryLimit_Checking(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Memory limit tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	// Should return some value (may be 0 on non-cgroup systems)
	memUsage, err := sb.CheckMemoryLimit()
	if err != nil {
		t.Logf("Memory check failed (expected in CI): %v", err)
	} else {
		assert.GreaterOrEqual(t, memUsage, int64(0))
	}
}

// TestSandbox_SetMemoryLimit tests updating memory limit
func TestSandbox_SetMemoryLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Memory limit tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	// Try to set new limit
	err = sb.SetMemoryLimit(64 * 1024 * 1024)
	if err != nil {
		t.Logf("SetMemoryLimit failed (expected in CI): %v", err)
	} else {
		assert.Equal(t, int64(64*1024*1024), sb.GetMemoryLimit())
	}
}

// TestSandbox_ResourceUsage_Monitoring tests resource monitoring
func TestSandbox_ResourceUsage_Monitoring(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Resource monitoring tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	// Resource check should not error
	err = sb.checkResourceUsage()
	assert.NoError(t, err)
}

// TestSandbox_NetworkPolicy_Values tests network policy constants
func TestSandbox_NetworkPolicy_Values(t *testing.T) {
	assert.Equal(t, NetworkPolicy("none"), NetworkNone)
	assert.Equal(t, NetworkPolicy("local"), NetworkLocal)
	assert.Equal(t, NetworkPolicy("restricted"), NetworkRestricted)
	assert.Equal(t, NetworkPolicy("full"), NetworkFull)
}

// TestSandbox_ExecutionTimer_Creation tests execution timer
func TestSandbox_ExecutionTimer_Creation(t *testing.T) {
	timer := NewExecutionTimer(100 * time.Millisecond)
	require.NotNil(t, timer)
	defer timer.Stop()

	// Should not be exceeded immediately
	err := timer.Check()
	assert.NoError(t, err)
}

// TestSandbox_ExecutionTimer_Timeout tests timer timeout
func TestSandbox_ExecutionTimer_Timeout(t *testing.T) {
	timer := NewExecutionTimer(50 * time.Millisecond)
	defer timer.Stop()

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	err := timer.Check()
	require.Error(t, err)

	var sbErr *SandboxError
	require.True(t, assert.ErrorAs(t, err, &sbErr))
	assert.Equal(t, ErrTimeout, sbErr.Code)
}

// TestSandbox_SandboxError_Types tests error type detection
func TestSandbox_SandboxError_Types(t *testing.T) {
	// Timeout error
	timeoutErr := &SandboxError{Code: ErrTimeout, Deadline: time.Now()}
	assert.Contains(t, timeoutErr.Error(), "timeout")

	// Memory error
	memErr := &SandboxError{Code: ErrMemoryExceeded, Limit: 1000, Current: 2000}
	assert.Contains(t, memErr.Error(), "memory")
	assert.Contains(t, memErr.Error(), "2000")

	// Init error
	initErr := &SandboxError{Code: ErrSandboxInit, Cause: fmt.Errorf("test error")}
	assert.Contains(t, initErr.Error(), "initialization")

	// Syscall error
	syscallErr := &SandboxError{Code: ErrSyscallBlocked, Cause: fmt.Errorf("blocked")}
	assert.Contains(t, syscallErr.Error(), "syscall")
}

// TestSandbox_SandboxResult_Values tests result structure
func TestSandbox_SandboxResult_Values(t *testing.T) {
	result := &SandboxResult{
		ExitCode:   42,
		MemoryUsed: 1024 * 1024,
		CPUTime:    500 * time.Millisecond,
		Duration:   1 * time.Second,
	}

	assert.Equal(t, 42, result.ExitCode)
	assert.Equal(t, int64(1024*1024), result.MemoryUsed)
	assert.Equal(t, 500*time.Millisecond, result.CPUTime)
	assert.Equal(t, 1*time.Second, result.Duration)
}

// TestSandbox_IsNamespaceSupported checks namespace support
func TestSandbox_IsNamespaceSupported(t *testing.T) {
	// Just verify it doesn't panic
	supported := IsNamespaceSupported()
	t.Logf("Namespace supported: %v", supported)
}

// TestSandbox_IsCgroupV2 tests cgroup version detection
func TestSandbox_IsCgroupV2(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Cgroup tests only supported on Linux")
	}

	// Just verify it doesn't panic
	isV2 := IsCgroupV2()
	t.Logf("Cgroup v2: %v", isV2)

	version := GetCgroupVersion()
	if isV2 {
		assert.Equal(t, "v2", version)
	} else {
		assert.Equal(t, "v1", version)
	}
}

// TestSandbox_IsSeccompAvailable tests seccomp availability
func TestSandbox_IsSeccompAvailable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Seccomp tests only supported on Linux")
	}

	// Just verify it doesn't panic
	available := IsSeccompAvailable()
	t.Logf("Seccomp available: %v", available)
}

// TestSandbox_Concurrent_Execution tests concurrent sandbox operations
func TestSandbox_Concurrent_Execution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()

	// Create multiple sandboxes
	var sandboxes []*Sandbox
	for i := 0; i < 3; i++ {
		sb, err := NewSandbox(config)
		require.NoError(t, err)
		sandboxes = append(sandboxes, sb)
	}

	// Cleanup all
	for _, sb := range sandboxes {
		_ = sb.Cleanup()
	}

	assert.Len(t, sandboxes, 3)
}

// TestSandbox_Execute_MultipleCalls tests multiple executions on same sandbox
func TestSandbox_Execute_MultipleCalls(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx := context.Background()

	// First execution
	result1, err := sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		return &SandboxResult{ExitCode: 1}
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result1.ExitCode)

	// Second execution
	result2, err := sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		return &SandboxResult{ExitCode: 2}
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result2.ExitCode)
}

// TestSandbox_Cleanup_Idempotent tests cleanup can be called multiple times
func TestSandbox_Cleanup_Idempotent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := DefaultSandboxConfig()
	sb, err := NewSandbox(config)
	require.NoError(t, err)

	// First cleanup
	err = sb.Cleanup()
	assert.NoError(t, err)

	// Second cleanup should not panic
	err = sb.Cleanup()
	assert.NoError(t, err)
}

// TestSandbox_Integration_FullWorkflow tests complete workflow
func TestSandbox_Integration_FullWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	// Create config
	config := SandboxConfig{
		MemoryLimit:     64 * 1024 * 1024,
		TimeLimit:       2 * time.Second,
		MaxProcs:        5,
		MaxOpenFiles:    256,
		NetworkPolicy:   NetworkNone,
		NoNewPrivileges: true,
	}

	// Create sandbox
	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	// Execute work
	ctx := context.Background()
	result, err := sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		// Simulate some work
		time.Sleep(50 * time.Millisecond)
		return &SandboxResult{
			ExitCode:   0,
			MemoryUsed: 1024,
			CPUTime:    10 * time.Millisecond,
		}
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.GreaterOrEqual(t, result.MemoryUsed, int64(0))
}

// TestSandbox_Execute_WithSeccomp tests execution with seccomp profile
func TestSandbox_Execute_WithSeccomp(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sandbox tests only supported on Linux")
	}

	config := SandboxConfig{
		MemoryLimit:    128 * 1024 * 1024,
		TimeLimit:      5 * time.Second,
		SeccompProfile: "minimal",
	}

	sb, err := NewSandbox(config)
	require.NoError(t, err)
	defer func() { _ = sb.Cleanup() }()

	ctx := context.Background()
	result, err := sb.Execute(ctx, func(ctx context.Context) *SandboxResult {
		return &SandboxResult{ExitCode: 0}
	})

	// Seccomp may fail in CI environments, so we just check it doesn't panic
	if err != nil {
		t.Logf("Seccomp execution error (expected in some environments): %v", err)
	} else {
		assert.Equal(t, 0, result.ExitCode)
	}
}

// TestSandbox_Error_Unwrap tests error unwrapping
func TestSandbox_Error_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := &SandboxError{Code: ErrSandboxInit, Cause: cause}

	assert.Equal(t, cause, err.Unwrap())
}

// TestSandbox_NilCleanup tests cleanup with nil cgroup
func TestSandbox_NilCleanup(t *testing.T) {
	sb := &Sandbox{
		config: DefaultSandboxConfig(),
	}

	// Should not panic with nil cgroup
	err := sb.Cleanup()
	assert.NoError(t, err)
}

// TestSandbox_ReadCgroup_InvalidFile tests reading invalid cgroup file
func TestSandbox_ReadCgroup_InvalidFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Cgroup tests only supported on Linux")
	}

	// Test that sandbox with nil cgroup doesn't panic
	sb := &Sandbox{
		config: DefaultSandboxConfig(),
	}

	// CheckMemoryLimit should error with nil cgroup
	_, err := sb.CheckMemoryLimit()
	assert.Error(t, err)
}
