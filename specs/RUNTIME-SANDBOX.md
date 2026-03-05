# Runtime Sandbox Specification

## Overview

Execution sandboxing system providing process isolation, resource containment, and security boundaries for untrusted code execution.

## Core Components

### Sandbox Configuration

```go
// SandboxConfig defines isolation parameters
type SandboxConfig struct {
    MemoryLimit     int64         `json:"memory_limit"`
    TimeLimit       time.Duration `json:"time_limit"`
    CPUQuota        int64         `json:"cpu_quota"`      // microseconds per period
    CPUPeriod       int64         `json:"cpu_period"`     // microseconds
    MaxProcs        int           `json:"max_procs"`
    MaxOpenFiles    int           `json:"max_open_files"`
    WorkingDir      string        `json:"working_dir"`
    ReadOnlyPaths   []string      `json:"read_only_paths"`
    WritablePaths   []string      `json:"writable_paths"`
    NetworkPolicy   NetworkPolicy `json:"network_policy"`
    SeccompProfile  string        `json:"seccomp_profile"`
    NoNewPrivileges bool          `json:"no_new_privileges"`
}

type NetworkPolicy string

const (
    NetworkNone     NetworkPolicy = "none"
    NetworkLocal    NetworkPolicy = "local"
    NetworkRestricted NetworkPolicy = "restricted"
    NetworkFull     NetworkPolicy = "full"
)

// DefaultSandboxConfig returns safe defaults
func DefaultSandboxConfig() SandboxConfig {
    return SandboxConfig{
        MemoryLimit:     128 * 1024 * 1024,  // 128MB
        TimeLimit:       30 * time.Second,
        CPUQuota:        100000,             // 100ms
        CPUPeriod:       100000,             // 100ms period
        MaxProcs:        10,
        MaxOpenFiles:    1024,
        NetworkPolicy:   NetworkNone,
        NoNewPrivileges: true,
    }
}
```

### Sandbox Runtime

```go
// Sandbox provides isolated execution environment
type Sandbox struct {
    config    SandboxConfig
    namespace *Namespace
    cgroup    *Cgroup
    seccomp   *SeccompFilter
    limits    *ResourceLimiter
    cancel    context.CancelFunc
}

// NewSandbox creates isolated execution environment
func NewSandbox(config SandboxConfig) (*Sandbox, error) {
    sb := &Sandbox{config: config}
    
    // Create mount namespace
    if err := sb.setupNamespace(); err != nil {
        return nil, fmt.Errorf("namespace setup: %w", err)
    }
    
    // Setup cgroup for resource limits
    if err := sb.setupCgroup(); err != nil {
        return nil, fmt.Errorf("cgroup setup: %w", err)
    }
    
    // Load seccomp filter
    if err := sb.loadSeccomp(); err != nil {
        return nil, fmt.Errorf("seccomp load: %w", err)
    }
    
    return sb, nil
}

func (sb *Sandbox) Execute(ctx context.Context, fn SandboxFunc) (*SandboxResult, error) {
    ctx, cancel := context.WithTimeout(ctx, sb.config.TimeLimit)
    defer cancel()
    sb.cancel = cancel
    
    // Apply resource limits
    if err := sb.limits.Apply(); err != nil {
        return nil, fmt.Errorf("apply limits: %w", err)
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
        
        res, err := sb.runIsolated(ctx, fn)
        if err != nil {
            errChan <- err
            return
        }
        result <- res
    }()
    
    select {
    case res := <-result:
        return res, nil
    case err := <-errChan:
        return nil, err
    case <-ctx.Done():
        sb.cancel()
        return nil, &SandboxError{Code: ErrTimeout, Cause: ctx.Err()}
    }
}

func (sb *Sandbox) Cleanup() error {
    if sb.cgroup != nil {
        sb.cgroup.Remove()
    }
    if sb.namespace != nil {
        sb.namespace.Teardown()
    }
    return nil
}
```

## Resource Isolation

### Namespace Management

```go
type Namespace struct {
    mountNS   string
    netNS     string
    pidNS     string
    userNS    string
}

func (sb *Sandbox) setupNamespace() error {
    // Unshare namespaces
    if err := unix.Unshare(unix.CLONE_NEWNS | unix.CLONE_NEWPID | 
                          unix.CLONE_NEWNET | unix.CLONE_NEWUSER); err != nil {
        return fmt.Errorf("unshare: %w", err)
    }
    
    // Setup mount propagation
    if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
        return fmt.Errorf("mount propagation: %w", err)
    }
    
    // Mount tmpfs for working directory
    if err := sb.setupTmpfs(); err != nil {
        return fmt.Errorf("tmpfs setup: %w", err)
    }
    
    // Bind mount read-only paths
    for _, path := range sb.config.ReadOnlyPaths {
        if err := sb.bindMountReadOnly(path); err != nil {
            return fmt.Errorf("bind mount %s: %w", path, err)
        }
    }
    
    return nil
}

func (sb *Sandbox) setupTmpfs() error {
    workDir := sb.config.WorkingDir
    if workDir == "" {
        workDir = "/sandbox"
    }
    
    if err := os.MkdirAll(workDir, 0755); err != nil {
        return err
    }
    
    // Mount tmpfs with size limit
    sizeOpt := fmt.Sprintf("size=%d", sb.config.MemoryLimit)
    return unix.Mount("tmpfs", workDir, "tmpfs", 0, sizeOpt)
}

func (sb *Sandbox) bindMountReadOnly(source string) error {
    target := filepath.Join(sb.config.WorkingDir, source)
    
    if err := os.MkdirAll(target, 0755); err != nil {
        return err
    }
    
    if err := unix.Mount(source, target, "", unix.MS_BIND|unix.MS_RDONLY, ""); err != nil {
        return err
    }
    
    // Remount read-only
    return unix.Mount("", target, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, "")
}
```

### Cgroup Limits

```go
type Cgroup struct {
    path    string
    subsystem string
}

func (sb *Sandbox) setupCgroup() error {
    cgroupPath := fmt.Sprintf("/sys/fs/cgroup/muster/%s", generateID())
    
    if err := os.MkdirAll(cgroupPath, 0755); err != nil {
        return fmt.Errorf("create cgroup: %w", err)
    }
    
    sb.cgroup = &Cgroup{path: cgroupPath, subsystem: "muster"}
    
    // Memory limit
    if sb.config.MemoryLimit > 0 {
        if err := sb.writeCgroup("memory.limit_in_bytes", 
            fmt.Sprintf("%d", sb.config.MemoryLimit)); err != nil {
            return err
        }
    }
    
    // CPU quota
    if sb.config.CPUQuota > 0 {
        if err := sb.writeCgroup("cpu.cfs_quota_us", 
            fmt.Sprintf("%d", sb.config.CPUQuota)); err != nil {
            return err
        }
        if err := sb.writeCgroup("cpu.cfs_period_us", 
            fmt.Sprintf("%d", sb.config.CPUPeriod)); err != nil {
            return err
        }
    }
    
    // Pids limit
    if sb.config.MaxProcs > 0 {
        if err := sb.writeCgroup("pids.max", 
            fmt.Sprintf("%d", sb.config.MaxProcs)); err != nil {
            return err
        }
    }
    
    // Add current process to cgroup
    return sb.writeCgroup("cgroup.procs", fmt.Sprintf("%d", os.Getpid()))
}

func (cg *Cgroup) writeCgroup(file, value string) error {
    path := filepath.Join(cg.path, file)
    return os.WriteFile(path, []byte(value), 0644)
}

func (cg *Cgroup) Remove() error {
    return os.RemoveAll(cg.path)
}
```

## Time Limits

```go
type ExecutionTimer struct {
    deadline  time.Time
    timer     *time.Timer
    exceeded  atomic.Bool
}

func NewExecutionTimer(limit time.Duration) *ExecutionTimer {
    et := &ExecutionTimer{deadline: time.Now().Add(limit)}
    et.timer = time.AfterFunc(limit, func() {
        et.exceeded.Store(true)
    })
    return et
}

func (et *ExecutionTimer) Check() error {
    if et.exceeded.Load() {
        return &SandboxError{Code: ErrTimeout, Deadline: et.deadline}
    }
    return nil
}

func (et *ExecutionTimer) Stop() {
    if et.timer != nil {
        et.timer.Stop()
    }
}

// Instrumented execution with checkpoints
func (sb *Sandbox) runIsolated(ctx context.Context, fn SandboxFunc) (*SandboxResult, error) {
    timer := NewExecutionTimer(sb.config.TimeLimit)
    defer timer.Stop()
    
    // Periodic checkpoint
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    done := make(chan *SandboxResult, 1)
    go func() {
        done <- fn(ctx)
    }()
    
    for {
        select {
        case res := <-done:
            return res, nil
        case <-ticker.C:
            if err := timer.Check(); err != nil {
                return nil, err
            }
            if err := sb.checkResourceUsage(); err != nil {
                return nil, err
            }
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

## Memory Limits

```go
type MemoryMonitor struct {
    limit    int64
    current  int64
    oomKill  bool
}

func (sb *Sandbox) checkResourceUsage() error {
    // Check memory from cgroup
    memUsage := filepath.Join(sb.cgroup.path, "memory.usage_in_bytes")
    data, err := os.ReadFile(memUsage)
    if err != nil {
        return err
    }
    
    usage, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
    if err != nil {
        return err
    }
    
    if usage > sb.config.MemoryLimit {
        return &SandboxError{
            Code:    ErrMemoryExceeded,
            Limit:   sb.config.MemoryLimit,
            Current: usage,
        }
    }
    
    return nil
}

func (sb *Sandbox) SetMemoryLimit(limit int64) error {
    sb.config.MemoryLimit = limit
    return sb.writeCgroup("memory.limit_in_bytes", fmt.Sprintf("%d", limit))
}
```

## Security Boundaries

### Seccomp Filter

```go
type SeccompFilter struct {
    profile FilterProfile
}

type FilterProfile string

const (
    ProfileMinimal FilterProfile = "minimal"
    ProfileStandard FilterProfile = "standard"
    ProfileStrict  FilterProfile = "strict"
)

func (sb *Sandbox) loadSeccomp() error {
    if sb.config.SeccompProfile == "" {
        sb.config.SeccompProfile = string(ProfileStandard)
    }
    
    filter := seccomp.NewFilter(seccomp.ActErrno)
    
    // Allow basic syscalls
    for _, syscall := range sb.getAllowedSyscalls() {
        if err := filter.AddRule(syscall, seccomp.ActAllow); err != nil {
            return fmt.Errorf("seccomp rule: %w", err)
        }
    }
    
    if err := filter.Load(); err != nil {
        return fmt.Errorf("load seccomp: %w", err)
    }
    
    sb.seccomp = &SeccompFilter{profile: FilterProfile(sb.config.SeccompProfile)}
    return nil
}

func (sb *Sandbox) getAllowedSyscalls() []string {
    base := []string{
        "read", "write", "open", "close",
        "mmap", "munmap", "mprotect",
        "exit", "exit_group",
        "rt_sigreturn", "rt_sigaction",
    }
    
    switch sb.config.SeccompProfile {
    case string(ProfileStandard):
        base = append(base, "socket", "connect", "sendto", "recvfrom")
    }
    
    return base
}
```

### Error Types

```go
const (
    ErrTimeout        ErrorCode = "TIMEOUT"
    ErrMemoryExceeded ErrorCode = "MEMORY_EXCEEDED"
    ErrCPULimit       ErrorCode = "CPU_LIMIT"
    ErrSyscallBlocked ErrorCode = "SYSCALL_BLOCKED"
    ErrSandboxInit    ErrorCode = "INIT_FAILED"
)

type SandboxError struct {
    Code     ErrorCode     `json:"code"`
    Cause    error         `json:"cause,omitempty"`
    Limit    int64         `json:"limit,omitempty"`
    Current  int64         `json:"current,omitempty"`
    Deadline time.Time     `json:"deadline,omitempty"`
}

func (e *SandboxError) Error() string {
    switch e.Code {
    case ErrTimeout:
        return fmt.Sprintf("execution timeout (deadline: %v)", e.Deadline)
    case ErrMemoryExceeded:
        return fmt.Sprintf("memory limit exceeded (%d > %d bytes)", e.Current, e.Limit)
    default:
        return fmt.Sprintf("sandbox error %s: %v", e.Code, e.Cause)
    }
}
```
