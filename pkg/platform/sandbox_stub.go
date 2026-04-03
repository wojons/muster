//go:build !linux
// +build !linux

// Stub implementation for non-Linux platforms
package platform

import (
	"context"
	"fmt"
	"time"
)

// NetworkPolicy defines network isolation levels (stub for non-Linux)
type NetworkPolicy string

const (
	NetworkNone       NetworkPolicy = "none"
	NetworkLocal      NetworkPolicy = "local"
	NetworkRestricted NetworkPolicy = "restricted"
	NetworkFull       NetworkPolicy = "full"
)

// SandboxConfig defines isolation parameters (stub for non-Linux)
type SandboxConfig struct {
	MemoryLimit     int64         `json:"memory_limit"`
	TimeLimit       time.Duration `json:"time_limit"`
	CPUQuota        int64         `json:"cpu_quota"`
	CPUPeriod       int64         `json:"cpu_period"`
	MaxProcs        int           `json:"max_procs"`
	MaxOpenFiles    int           `json:"max_open_files"`
	WorkingDir      string        `json:"working_dir"`
	ReadOnlyPaths   []string      `json:"read_only_paths"`
	WritablePaths   []string      `json:"writable_paths"`
	NetworkPolicy   NetworkPolicy `json:"network_policy"`
	SeccompProfile  string        `json:"seccomp_profile"`
	NoNewPrivileges bool          `json:"no_new_privileges"`
}

// DefaultSandboxConfig returns safe defaults (stub for non-Linux)
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MemoryLimit:     128 * 1024 * 1024,
		TimeLimit:       30 * time.Second,
		CPUQuota:        100000,
		CPUPeriod:       100000,
		MaxProcs:        10,
		MaxOpenFiles:    1024,
		NetworkPolicy:   NetworkNone,
		NoNewPrivileges: true,
	}
}

// ErrorCode represents sandbox error types (stub for non-Linux)
type ErrorCode string

const (
	ErrTimeout        ErrorCode = "TIMEOUT"
	ErrMemoryExceeded ErrorCode = "MEMORY_EXCEEDED"
	ErrCPULimit       ErrorCode = "CPU_LIMIT"
	ErrSyscallBlocked ErrorCode = "SYSCALL_BLOCKED"
	ErrSandboxInit    ErrorCode = "INIT_FAILED"
	ErrResourceLimit  ErrorCode = "RESOURCE_LIMIT"
)

// SandboxError represents a sandbox error (stub for non-Linux)
type SandboxError struct {
	Code     ErrorCode `json:"code"`
	Cause    error     `json:"cause,omitempty"`
	Limit    int64     `json:"limit,omitempty"`
	Current  int64     `json:"current,omitempty"`
	Deadline time.Time `json:"deadline,omitempty"`
}

func (e *SandboxError) Error() string {
	switch e.Code {
	case ErrTimeout:
		return fmt.Sprintf("execution timeout (deadline: %v)", e.Deadline)
	case ErrMemoryExceeded:
		return fmt.Sprintf("memory limit exceeded (%d > %d bytes)", e.Current, e.Limit)
	case ErrCPULimit:
		return fmt.Sprintf("CPU limit exceeded")
	case ErrSyscallBlocked:
		return fmt.Sprintf("blocked syscall: %v", e.Cause)
	case ErrSandboxInit:
		return fmt.Sprintf("sandbox initialization failed: %v", e.Cause)
	case ErrResourceLimit:
		return fmt.Sprintf("resource limit exceeded: %v", e.Cause)
	default:
		return fmt.Sprintf("sandbox error %s: %v", e.Code, e.Cause)
	}
}

// Unwrap returns the wrapped error (stub for non-Linux)
func (e *SandboxError) Unwrap() error {
	return e.Cause
}

// SandboxResult represents the result of sandboxed execution (stub for non-Linux)
type SandboxResult struct {
	ExitCode   int           `json:"exit_code"`
	MemoryUsed int64         `json:"memory_used"`
	CPUTime    time.Duration `json:"cpu_time"`
	Duration   time.Duration `json:"duration"`
}

// SandboxFunc is the function type for sandboxed execution (stub for non-Linux)
type SandboxFunc func(context.Context) *SandboxResult

// ExecutionTimer tracks execution time (stub for non-Linux)
type ExecutionTimer struct {
	deadline time.Time
}

// NewExecutionTimer creates a new execution timer (stub for non-Linux)
func NewExecutionTimer(limit time.Duration) *ExecutionTimer {
	return &ExecutionTimer{deadline: time.Now().Add(limit)}
}

// Check returns an error if time limit exceeded (stub for non-Linux)
func (et *ExecutionTimer) Check() error {
	if time.Now().After(et.deadline) {
		return &SandboxError{Code: ErrTimeout}
	}
	return nil
}

// Stop stops the timer (stub for non-Linux)
func (et *ExecutionTimer) Stop() {}

// Sandbox provides isolated execution environment (stub for non-Linux)
type Sandbox struct {
	config SandboxConfig
}

// NewSandbox creates isolated execution environment (stub for non-Linux)
func NewSandbox(config SandboxConfig) (*Sandbox, error) {
	return &Sandbox{config: config}, nil
}

// Execute runs a function with timeout and resource limits (stub for non-Linux)
func (sb *Sandbox) Execute(ctx context.Context, fn SandboxFunc) (*SandboxResult, error) {
	// Just execute the function without isolation
	return fn(ctx), nil
}

// Cleanup removes resources (stub for non-Linux)
func (sb *Sandbox) Cleanup() error {
	return nil
}

// GetMemoryLimit returns the configured memory limit (stub for non-Linux)
func (sb *Sandbox) GetMemoryLimit() int64 {
	return sb.config.MemoryLimit
}

// GetTimeLimit returns the configured time limit (stub for non-Linux)
func (sb *Sandbox) GetTimeLimit() time.Duration {
	return sb.config.TimeLimit
}

// SetMemoryLimit updates the memory limit (stub for non-Linux)
func (sb *Sandbox) SetMemoryLimit(limit int64) error {
	sb.config.MemoryLimit = limit
	return nil
}

// CheckMemoryLimit returns current memory usage (stub for non-Linux)
func (sb *Sandbox) CheckMemoryLimit() (int64, error) {
	return 0, nil
}

// checkResourceUsage checks resource usage (stub for non-Linux)
func (sb *Sandbox) checkResourceUsage() error {
	return nil
}

// IsNamespaceSupported checks if namespaces are supported (stub for non-Linux)
func IsNamespaceSupported() bool {
	return false
}

// IsCgroupV2 returns true if using cgroup v2 (stub for non-Linux)
func IsCgroupV2() bool {
	return false
}

// GetCgroupVersion returns the cgroup version string (stub for non-Linux)
func GetCgroupVersion() string {
	return ""
}

// IsSeccompAvailable checks if seccomp is available (stub for non-Linux)
func IsSeccompAvailable() bool {
	return false
}

// Cgroup represents a cgroup for resource limiting (stub for non-Linux)
type Cgroup struct{}

// Namespace represents Linux namespaces (stub for non-Linux)
type Namespace struct{}
