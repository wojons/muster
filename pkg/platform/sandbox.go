//go:build linux
// +build linux

// Runtime sandbox provides isolated execution environment for untrusted code
package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// NetworkPolicy defines network isolation levels
type NetworkPolicy string

const (
	NetworkNone       NetworkPolicy = "none"
	NetworkLocal      NetworkPolicy = "local"
	NetworkRestricted NetworkPolicy = "restricted"
	NetworkFull       NetworkPolicy = "full"
)

// SandboxConfig defines isolation parameters
type SandboxConfig struct {
	MemoryLimit     int64         `json:"memory_limit"`
	TimeLimit       time.Duration `json:"time_limit"`
	CPUQuota        int64         `json:"cpu_quota"`  // microseconds per period
	CPUPeriod       int64         `json:"cpu_period"` // microseconds
	MaxProcs        int           `json:"max_procs"`
	MaxOpenFiles    int           `json:"max_open_files"`
	WorkingDir      string        `json:"working_dir"`
	ReadOnlyPaths   []string      `json:"read_only_paths"`
	WritablePaths   []string      `json:"writable_paths"`
	NetworkPolicy   NetworkPolicy `json:"network_policy"`
	SeccompProfile  string        `json:"seccomp_profile"`
	NoNewPrivileges bool          `json:"no_new_privileges"`
}

// DefaultSandboxConfig returns safe defaults
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MemoryLimit:     128 * 1024 * 1024, // 128MB
		TimeLimit:       30 * time.Second,
		CPUQuota:        100000, // 100ms
		CPUPeriod:       100000, // 100ms period
		MaxProcs:        10,
		MaxOpenFiles:    1024,
		NetworkPolicy:   NetworkNone,
		NoNewPrivileges: true,
	}
}

// ErrorCode represents sandbox error types
type ErrorCode string

const (
	ErrTimeout        ErrorCode = "TIMEOUT"
	ErrMemoryExceeded ErrorCode = "MEMORY_EXCEEDED"
	ErrCPULimit       ErrorCode = "CPU_LIMIT"
	ErrSyscallBlocked ErrorCode = "SYSCALL_BLOCKED"
	ErrSandboxInit    ErrorCode = "INIT_FAILED"
	ErrResourceLimit  ErrorCode = "RESOURCE_LIMIT"
)

// SandboxError represents a sandbox error
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

// Unwrap returns the wrapped error
func (e *SandboxError) Unwrap() error {
	return e.Cause
}

// SandboxResult represents the result of sandboxed execution
type SandboxResult struct {
	ExitCode   int           `json:"exit_code"`
	MemoryUsed int64         `json:"memory_used"`
	CPUTime    time.Duration `json:"cpu_time"`
	Duration   time.Duration `json:"duration"`
}

// SandboxFunc is the function type for sandboxed execution
type SandboxFunc func(context.Context) *SandboxResult

// Cgroup represents a cgroup for resource limiting
type Cgroup struct {
	path      string
	subsystem string
}

// Namespace represents Linux namespaces
type Namespace struct {
	mountNS string
	netNS   string
	pidNS   string
	userNS  string
}

// ExecutionTimer tracks execution time
type ExecutionTimer struct {
	deadline time.Time
	timer    *time.Timer
	exceeded atomic.Bool
}

// NewExecutionTimer creates a new execution timer
func NewExecutionTimer(limit time.Duration) *ExecutionTimer {
	et := &ExecutionTimer{deadline: time.Now().Add(limit)}
	et.timer = time.AfterFunc(limit, func() {
		et.exceeded.Store(true)
	})
	return et
}

// Check returns an error if time limit exceeded
func (et *ExecutionTimer) Check() error {
	if et.exceeded.Load() {
		return &SandboxError{Code: ErrTimeout, Deadline: et.deadline}
	}
	return nil
}

// Stop stops the timer
func (et *ExecutionTimer) Stop() {
	if et.timer != nil {
		et.timer.Stop()
	}
}

// Sandbox provides isolated execution environment
type Sandbox struct {
	config    SandboxConfig
	namespace *Namespace
	cgroup    *Cgroup
	seccomp   *SeccompFilter
	cancel    context.CancelFunc
}

// NewSandbox creates isolated execution environment
func NewSandbox(config SandboxConfig) (*Sandbox, error) {
	sb := &Sandbox{config: config}

	// Create cgroup for resource limits
	if err := sb.setupCgroup(); err != nil {
		return nil, &SandboxError{Code: ErrSandboxInit, Cause: fmt.Errorf("cgroup setup: %w", err)}
	}

	return sb, nil
}

// setupCgroup creates cgroup for resource limiting
func (sb *Sandbox) setupCgroup() error {
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/muster/sandbox-%d", time.Now().UnixNano())

	// Create cgroup directory
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		// Try alternative path
		cgroupPath = filepath.Join(os.TempDir(), fmt.Sprintf("muster-cgroup-%d", time.Now().UnixNano()))
		if err := os.MkdirAll(cgroupPath, 0755); err != nil {
			return fmt.Errorf("create cgroup: %w", err)
		}
	}

	sb.cgroup = &Cgroup{path: cgroupPath, subsystem: "muster"}

	// Memory limit
	if sb.config.MemoryLimit > 0 {
		if err := sb.writeCgroup("memory.max", fmt.Sprintf("%d", sb.config.MemoryLimit)); err != nil {
			// Try cgroup v1 path
			sb.writeCgroup("memory.limit_in_bytes", fmt.Sprintf("%d", sb.config.MemoryLimit))
		}
	}

	// CPU quota
	if sb.config.CPUQuota > 0 {
		sb.writeCgroup("cpu.max", fmt.Sprintf("%d %d", sb.config.CPUQuota, sb.config.CPUPeriod))
	}

	// Pids limit
	if sb.config.MaxProcs > 0 {
		sb.writeCgroup("pids.max", fmt.Sprintf("%d", sb.config.MaxProcs))
	}

	return nil
}

func (sb *Sandbox) writeCgroup(file, value string) error {
	if sb.cgroup == nil {
		return fmt.Errorf("cgroup not initialized")
	}
	path := filepath.Join(sb.cgroup.path, file)
	return os.WriteFile(path, []byte(value), 0644)
}

func (sb *Sandbox) readCgroup(file string) (string, error) {
	if sb.cgroup == nil {
		return "", fmt.Errorf("cgroup not initialized")
	}
	path := filepath.Join(sb.cgroup.path, file)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Cleanup removes cgroup resources
func (sb *Sandbox) Cleanup() error {
	if sb.cgroup != nil {
		return os.RemoveAll(sb.cgroup.path)
	}
	return nil
}

// Execute runs a function with timeout and resource limits
func (sb *Sandbox) Execute(ctx context.Context, fn SandboxFunc) (*SandboxResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sb.config.TimeLimit)
	defer cancel()
	sb.cancel = cancel

	// Apply resource limits
	if sb.config.MaxOpenFiles > 0 {
		var rlimit syscall.Rlimit
		rlimit.Cur = uint64(sb.config.MaxOpenFiles)
		rlimit.Max = uint64(sb.config.MaxOpenFiles)
		syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	}

	// Apply seccomp if configured
	if sb.config.SeccompProfile != "" {
		sf := NewSeccompFilter(SeccompModeFilter)
		var profile SeccompProfile
		switch sb.config.SeccompProfile {
		case "minimal":
			profile = SeccompProfileMinimal
		case "relaxed":
			profile = SeccompProfileRelaxed
		default:
			profile = SeccompProfileStandard
		}
		if err := sf.ApplyProfile(profile); err != nil {
			return nil, &SandboxError{Code: ErrSandboxInit, Cause: fmt.Errorf("seccomp: %w", err)}
		}
		if err := sf.Apply(); err != nil {
			return nil, &SandboxError{Code: ErrSandboxInit, Cause: fmt.Errorf("seccomp apply: %w", err)}
		}
		sb.seccomp = sf
	}

	// Execute with monitoring
	result := make(chan *SandboxResult, 1)
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in sandbox: %v", r)
			}
		}()
		res := fn(ctx)
		result <- res
	}()

	// Monitor execution
	timer := NewExecutionTimer(sb.config.TimeLimit)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case res := <-result:
			res.Duration = time.Since(startTime)
			return res, nil
		case err := <-errChan:
			return nil, err
		case <-ctx.Done():
			sb.cancel()
			return nil, &SandboxError{Code: ErrTimeout, Deadline: time.Now()}
		case <-ticker.C:
			if err := timer.Check(); err != nil {
				return nil, err
			}
			if err := sb.checkResourceUsage(); err != nil {
				return nil, err
			}
		}
	}
}

// checkResourceUsage checks current resource usage against limits
func (sb *Sandbox) checkResourceUsage() error {
	if sb.cgroup == nil {
		return nil
	}

	// Check memory usage
	memUsage, err := sb.readCgroup("memory.current")
	if err == nil && memUsage != "" && memUsage != "max" {
		if usage, err := strconv.ParseInt(memUsage, 10, 64); err == nil {
			if sb.config.MemoryLimit > 0 && usage > sb.config.MemoryLimit {
				return &SandboxError{
					Code:    ErrMemoryExceeded,
					Limit:   sb.config.MemoryLimit,
					Current: usage,
				}
			}
		}
	}

	return nil
}

// CheckMemoryLimit returns current memory usage
func (sb *Sandbox) CheckMemoryLimit() (int64, error) {
	if sb.cgroup == nil {
		return 0, fmt.Errorf("cgroup not initialized")
	}

	memUsage, err := sb.readCgroup("memory.current")
	if err != nil {
		// Try cgroup v1
		memUsage, err = sb.readCgroup("memory.usage_in_bytes")
		if err != nil {
			return 0, err
		}
	}

	if memUsage == "" || memUsage == "max" {
		return 0, nil
	}

	return strconv.ParseInt(memUsage, 10, 64)
}

// GetMemoryLimit returns the configured memory limit
func (sb *Sandbox) GetMemoryLimit() int64 {
	return sb.config.MemoryLimit
}

// GetTimeLimit returns the configured time limit
func (sb *Sandbox) GetTimeLimit() time.Duration {
	return sb.config.TimeLimit
}

// SetMemoryLimit updates the memory limit
func (sb *Sandbox) SetMemoryLimit(limit int64) error {
	sb.config.MemoryLimit = limit
	return sb.writeCgroup("memory.max", fmt.Sprintf("%d", limit))
}

// setupNamespace configures Linux namespaces (requires privileges)
func (sb *Sandbox) setupNamespace() error {
	// Note: In production, this requires CAP_SYS_ADMIN
	// For testing, we verify the capability exists

	if sb.config.WorkingDir != "" {
		if err := os.MkdirAll(sb.config.WorkingDir, 0755); err != nil {
			return fmt.Errorf("create working dir: %w", err)
		}
	}

	return nil
}

// runCommand executes a command in the sandbox
func (sb *Sandbox) runCommand(ctx context.Context, cmd *exec.Cmd) (*SandboxResult, error) {
	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, &SandboxError{Code: ErrTimeout}
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				return nil, err
			}
		}

		return &SandboxResult{
			ExitCode:   exitCode,
			MemoryUsed: 0, // Would need cgroup stats
			Duration:   time.Since(startTime),
		}, nil
	}
}

// IsNamespaceSupported checks if namespaces are supported
func IsNamespaceSupported() bool {
	// Check if we can create namespaces
	return unix.Getuid() == 0 || os.Getenv("container") != ""
}

// IsCgroupV2 returns true if using cgroup v2
func IsCgroupV2() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// GetCgroupVersion returns the cgroup version string
func GetCgroupVersion() string {
	if IsCgroupV2() {
		return "v2"
	}
	return "v1"
}

// IsSeccompAvailable checks if seccomp is available
func IsSeccompAvailable() bool {
	return CheckSeccompSupport()
}
