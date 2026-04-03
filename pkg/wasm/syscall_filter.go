package wasm

import (
	"fmt"
	"runtime"
	"sync"
)

// SyscallFilter provides system call filtering for WASM execution
// This interface abstracts platform-specific syscall filtering (seccomp on Linux)
type SyscallFilter interface {
	// AllowSyscall allows a specific system call
	AllowSyscall(syscallName string) error
	// DenySyscall denies a specific system call
	DenySyscall(syscallName string) error
	// Apply applies the filter to the current process
	Apply() error
	// IsAllowed checks if a syscall is allowed
	IsAllowed(syscallName string) bool
	// Profile returns the current security profile
	Profile() SyscallProfile
	// CheckSyscall checks if a syscall would be allowed and returns an error if not
	CheckSyscall(syscallName string) error
}

// SyscallProfile defines the security level for syscall filtering
type SyscallProfile int

const (
	// SyscallProfileMinimal allows only essential syscalls (read, write, exit)
	SyscallProfileMinimal SyscallProfile = iota
	// SyscallProfileStandard allows common syscalls for network/file operations
	SyscallProfileStandard
	// SyscallProfileRelaxed allows most syscalls but blocks dangerous ones
	SyscallProfileRelaxed
)

// String returns the string representation of the profile
func (p SyscallProfile) String() string {
	switch p {
	case SyscallProfileMinimal:
		return "minimal"
	case SyscallProfileStandard:
		return "standard"
	case SyscallProfileRelaxed:
		return "relaxed"
	default:
		return "unknown"
	}
}

// DefaultMinimalSyscalls are the minimal set of syscalls needed for basic operation
var DefaultMinimalSyscalls = []string{
	"read", "write", "close", "exit", "exit_group",
	"sigreturn", "rt_sigreturn", "rt_sigaction",
	"mmap", "munmap", "mprotect",
	"brk", "sbrk",
}

// DefaultStandardSyscalls adds network and file operations to minimal set
var DefaultStandardSyscalls = []string{
	// Include all minimal syscalls
	"read", "write", "close", "exit", "exit_group",
	"sigreturn", "rt_sigreturn", "rt_sigaction",
	"mmap", "munmap", "mprotect",
	"brk", "sbrk",
	// Network operations
	"socket", "connect", "accept", "bind", "listen",
	"sendto", "recvfrom", "sendmsg", "recvmsg",
	"setsockopt", "getsockopt", "getsockname", "getpeername",
	"shutdown",
	// File operations
	"openat", "open", "creat",
	"lseek", "pread64", "pwrite64",
	"stat", "fstat", "lstat", "newfstatat",
	"access", "faccessat",
	"getdents", "getdents64",
	// Process info
	"getpid", "getppid", "gettid",
	"getuid", "getgid", "geteuid", "getegid",
	"setuid", "setgid", "seteuid", "setegid",
	"getgroups", "setgroups",
	// Time
	"gettimeofday", "clock_gettime", "clock_getres",
	"nanosleep", "clock_nanosleep",
}

// DangerousSyscalls should always be blocked in sandboxed environments
var DangerousSyscalls = []string{
	"execve", "execveat", "fork", "vfork", "clone", "clone3",
	"ptrace", "process_vm_writev", "process_vm_readv",
	"open_by_handle_at", "name_to_handle_at",
	"setns", "unshare", "chroot",
	"mount", "umount", "umount2",
	"swapon", "swapoff",
	"reboot", "kexec_load", "kexec_file_load",
	"init_module", "finit_module", "delete_module",
	"ioperm", "iopl", "iopl_set",
	"perf_event_open",
	"bpf", "socketpair",
}

// InMemorySyscallFilter is a cross-platform syscall filter that tracks allowed/denied syscalls
// For Linux with CGO, it can wrap the actual seccomp implementation
type InMemorySyscallFilter struct {
	profile SyscallProfile
	allowed map[string]bool
	denied  map[string]bool
	mu      sync.RWMutex
	applied bool
	onDeny  func(syscallName string) error // callback when syscall is denied
}

// NewSyscallFilter creates a new syscall filter with the specified profile
func NewSyscallFilter(profile SyscallProfile) *InMemorySyscallFilter {
	filter := &InMemorySyscallFilter{
		profile: profile,
		allowed: make(map[string]bool),
		denied:  make(map[string]bool),
		onDeny: func(syscallName string) error {
			return fmt.Errorf("syscall %s denied by security policy", syscallName)
		},
	}

	// Initialize with profile defaults
	filter.initProfileDefaults()

	return filter
}

// initProfileDefaults sets up default syscalls based on profile
func (f *InMemorySyscallFilter) initProfileDefaults() {
	// Always deny dangerous syscalls
	for _, syscall := range DangerousSyscalls {
		f.denied[syscall] = true
	}

	// Allow based on profile
	switch f.profile {
	case SyscallProfileMinimal:
		for _, syscall := range DefaultMinimalSyscalls {
			f.allowed[syscall] = true
		}
	case SyscallProfileStandard:
		for _, syscall := range DefaultStandardSyscalls {
			f.allowed[syscall] = true
		}
	case SyscallProfileRelaxed:
		// Allow everything except dangerous syscalls
		// This is done implicitly - if not denied, it's allowed
	}
}

// AllowSyscall adds a syscall to the allowlist
func (f *InMemorySyscallFilter) AllowSyscall(syscallName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.denied[syscallName] {
		return fmt.Errorf("cannot allow syscall %s: it is in the denylist", syscallName)
	}

	f.allowed[syscallName] = true
	return nil
}

// DenySyscall adds a syscall to the denylist
func (f *InMemorySyscallFilter) DenySyscall(syscallName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.allowed, syscallName)
	f.denied[syscallName] = true
	return nil
}

// IsAllowed checks if a syscall is allowed
func (f *InMemorySyscallFilter) IsAllowed(syscallName string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// If explicitly denied, return false
	if f.denied[syscallName] {
		return false
	}

	// For minimal and standard profiles, must be explicitly allowed
	if f.profile == SyscallProfileMinimal || f.profile == SyscallProfileStandard {
		return f.allowed[syscallName]
	}

	// For relaxed profile, everything not denied is allowed
	return true
}

// Profile returns the current security profile
func (f *InMemorySyscallFilter) Profile() SyscallProfile {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.profile
}

// Apply applies the syscall filter
// Note: On Linux with CGO, this would apply the actual seccomp filter
// On other platforms, it just marks the filter as applied
func (f *InMemorySyscallFilter) Apply() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.applied {
		return fmt.Errorf("syscall filter already applied")
	}

	// On Linux with CGO, we would apply the actual seccomp filter here
	// For now, we just mark it as applied for tracking purposes
	f.applied = true

	return nil
}

// IsApplied returns whether the filter has been applied
func (f *InMemorySyscallFilter) IsApplied() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.applied
}

// SetDenyCallback sets a callback function to be called when a syscall is denied
func (f *InMemorySyscallFilter) SetDenyCallback(callback func(syscallName string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onDeny = callback
}

// GetAllowedSyscalls returns a copy of the allowed syscall list
func (f *InMemorySyscallFilter) GetAllowedSyscalls() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]string, 0, len(f.allowed))
	for syscall := range f.allowed {
		result = append(result, syscall)
	}
	return result
}

// GetDeniedSyscalls returns a copy of the denied syscall list
func (f *InMemorySyscallFilter) GetDeniedSyscalls() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]string, 0, len(f.denied))
	for syscall := range f.denied {
		result = append(result, syscall)
	}
	return result
}

// CheckSyscall checks if a syscall would be allowed and returns an error if not
func (f *InMemorySyscallFilter) CheckSyscall(syscallName string) error {
	if !f.IsAllowed(syscallName) {
		f.mu.RLock()
		onDeny := f.onDeny
		f.mu.RUnlock()

		if onDeny != nil {
			return onDeny(syscallName)
		}
		return fmt.Errorf("syscall %s denied by security policy", syscallName)
	}
	return nil
}

// RuntimeWithSyscallFilter wraps a Runtime with syscall filtering capabilities
type RuntimeWithSyscallFilter struct {
	*Runtime
	filter SyscallFilter
}

// NewRuntimeWithSyscallFilter creates a new runtime with syscall filtering
func NewRuntimeWithSyscallFilter(config *RuntimeConfig, limits *SandboxLimits, profile SyscallProfile) (*RuntimeWithSyscallFilter, error) {
	runtime := NewRuntime(config, limits)
	if runtime == nil {
		return nil, fmt.Errorf("failed to create runtime")
	}

	filter := NewSyscallFilter(profile)

	return &RuntimeWithSyscallFilter{
		Runtime: runtime,
		filter:  filter,
	}, nil
}

// SyscallFilter returns the syscall filter associated with this runtime
func (r *RuntimeWithSyscallFilter) SyscallFilter() SyscallFilter {
	return r.filter
}

// ApplySyscallFilter applies the syscall filter
func (r *RuntimeWithSyscallFilter) ApplySyscallFilter() error {
	return r.filter.Apply()
}

// IsSyscallAllowed checks if a syscall is allowed by the filter
func (r *RuntimeWithSyscallFilter) IsSyscallAllowed(syscallName string) bool {
	return r.filter.IsAllowed(syscallName)
}

// CheckSyscall checks if a syscall would be allowed
func (r *RuntimeWithSyscallFilter) CheckSyscall(syscallName string) error {
	return r.filter.CheckSyscall(syscallName)
}

// PlatformSupportsSyscallFiltering returns true if the current platform supports
// actual syscall filtering (Linux with seccomp)
func PlatformSupportsSyscallFiltering() bool {
	return runtime.GOOS == "linux"
}

// GetPlatformSpecificSyscalls returns platform-specific syscalls that may be needed
func GetPlatformSpecificSyscalls() []string {
	switch runtime.GOOS {
	case "linux":
		return []string{
			// Linux-specific syscalls that may be needed
			"epoll_create", "epoll_create1", "epoll_ctl", "epoll_wait", "epoll_pwait",
			"eventfd", "eventfd2",
			"timerfd_create", "timerfd_settime", "timerfd_gettime",
			"signalfd", "signalfd4",
			"inotify_init", "inotify_init1", "inotify_add_watch", "inotify_rm_watch",
			"pipe", "pipe2",
			"poll", "ppoll",
			"select", "pselect6",
		}
	case "darwin":
		return []string{
			// macOS-specific syscalls (generally fewer exposed directly)
			"kevent", "kqueue",
		}
	default:
		return []string{}
	}
}
