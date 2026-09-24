//go:build linux && !seccomp
// +build linux,!seccomp

// Stub implementations of the seccomp API for builds without the seccomp
// build tag. The real CGO implementation (seccomp.go) requires
// libseccomp-dev and pkg-config; this stub allows full-module builds
// (go build ./..., go test ./...) to succeed on systems without those
// system packages. Build with -tags seccomp to enable the real
// implementation.
package platform

import (
	"fmt"
	"runtime"
	"sync"
)

// SeccompMode represents different seccomp filtering modes
const (
	SeccompModeFilter SeccompFilterMode = iota + 1
	SeccompModeStrict
)

type SeccompFilterMode int

// SeccompProfile defines the allowed syscalls
type SeccompProfile int

const (
	SeccompProfileMinimal SeccompProfile = iota
	SeccompProfileStandard
	SeccompProfileRelaxed
)

var (
	lastFailedSyscall uintptr
	lastFailedArch    uintptr
	failedMutex       sync.RWMutex
)

// errSeccompNotCompiled is returned by all stub operations.
var errSeccompNotCompiled = fmt.Errorf("seccomp support not compiled in (build with -tags seccomp to enable)")

// SeccompFilter provides Linux seccomp BPF filtering for syscall restriction.
// This is the stub implementation used when the seccomp build tag is absent.
type SeccompFilter struct {
	mode         SeccompFilterMode
	profile      SeccompProfile
	allowed      map[uint32]bool
	denied       map[uint32]bool
	allowedPaths []string
	loaded       bool
}

// NewSeccompFilter creates a new seccomp filter (stub).
func NewSeccompFilter(mode SeccompFilterMode) *SeccompFilter {
	return &SeccompFilter{
		mode:    mode,
		allowed: make(map[uint32]bool),
		denied:  make(map[uint32]bool),
	}
}

// AllowSyscall adds a system call to the allowlist (stub: no-op).
func (sf *SeccompFilter) AllowSyscall(syscallID uintptr) error {
	return errSeccompNotCompiled
}

// DenySyscall adds a system call to the denylist (stub: no-op).
func (sf *SeccompFilter) DenySyscall(syscallID uintptr) error {
	return errSeccompNotCompiled
}

// Apply applies the seccomp filter to current process (stub: returns error).
func (sf *SeccompFilter) Apply() error {
	return errSeccompNotCompiled
}

// AllowReadOnly allows only read-only syscalls (stub: returns error).
func (sf *SeccompFilter) AllowReadOnly() error {
	return errSeccompNotCompiled
}

// DenyWrite denies all write operations (stub: returns error).
func (sf *SeccompFilter) DenyWrite() error {
	return errSeccompNotCompiled
}

// SetSocketRestriction restricts network socket operations (stub: returns error).
func (sf *SeccompFilter) SetSocketRestriction(allowBind, allowConnect bool) error {
	return errSeccompNotCompiled
}

// LimitFileAccess restricts file system access (stub: records paths, returns nil).
func (sf *SeccompFilter) LimitFileAccess(allowedPaths []string) error {
	if len(allowedPaths) == 0 {
		return fmt.Errorf("no allowed paths specified")
	}
	sf.allowedPaths = make([]string, len(allowedPaths))
	copy(sf.allowedPaths, allowedPaths)
	return nil
}

// CheckAccess returns true if path was listed in allowedPaths.
func (sf *SeccompFilter) CheckAccess(path string) bool {
	for _, allowed := range sf.allowedPaths {
		if path == allowed {
			return true
		}
		if len(path) > len(allowed) && path[:len(allowed)] == allowed && path[len(allowed)] == '/' {
			return true
		}
	}
	return false
}

// EnableNetworkRestriction restricts network I/O (stub: returns error).
func (sf *SeccompFilter) EnableNetworkRestriction(allowListen, allowConnect bool) error {
	return errSeccompNotCompiled
}

// ApplyToProcess applies filter to specific process by PID (stub: returns error).
func ApplyToProcess(pid int, filter *SeccompFilter) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}
	return errSeccompNotCompiled
}

// VerifySyscall checks if a syscall is allowed (stub: always not-in-list).
func (sf *SeccompFilter) VerifySyscall(syscallID uintptr) (bool, error) {
	id := uint32(syscallID)
	if sf.allowed[id] {
		return true, nil
	}
	if sf.denied[id] {
		return false, nil
	}
	return false, fmt.Errorf("syscall %d not in allowlist or denylist", syscallID)
}

// GetFailedSyscall returns syscall that caused ENOSYS failure.
func GetFailedSyscall() (uintptr, uintptr, error) {
	failedMutex.RLock()
	defer failedMutex.RUnlock()
	if lastFailedSyscall == 0 {
		return 0, 0, fmt.Errorf("no failed syscall recorded")
	}
	return lastFailedSyscall, lastFailedArch, nil
}

// EnableStrictMode enables strict seccomp mode (stub: returns error).
func (sf *SeccompFilter) EnableStrictMode() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("seccomp is only supported on Linux")
	}
	return errSeccompNotCompiled
}

// ApplyProfile applies a predefined seccomp profile (stub: returns error).
func (sf *SeccompFilter) ApplyProfile(profile SeccompProfile) error {
	sf.profile = profile
	return errSeccompNotCompiled
}

// CheckSeccompSupport checks if seccomp is supported (stub: always false).
func CheckSeccompSupport() bool {
	return false
}

// Syscall constants for common operations
const (
	SYS_READ  uintptr = 0
	SYS_WRITE uintptr = 1
	SYS_OPEN  uintptr = 2
	SYS_STAT  uintptr = 4
)

// RecordFailedSyscall records the last syscall that was denied by seccomp.
func RecordFailedSyscall(syscallNum, arch uintptr) {
	failedMutex.Lock()
	lastFailedSyscall = syscallNum
	lastFailedArch = arch
	failedMutex.Unlock()
}

// toSyscallName converts syscall number to a human-readable name (static table).
func toSyscallName(num uintptr) string {
	switch num {
	case 0:
		return "read"
	case 1:
		return "write"
	case 2:
		return "open"
	case 3:
		return "close"
	case 4:
		return "stat"
	case 5:
		return "fstat"
	case 6:
		return "lstat"
	case 8:
		return "lseek"
	case 9:
		return "mmap"
	case 10:
		return "mprotect"
	case 11:
		return "munmap"
	case 12:
		return "brk"
	case 59:
		return "execve"
	case 60:
		return "exit"
	case 231:
		return "exit_group"
	default:
		return "unknown"
	}
}

// ToSyscallName resolves the syscall number to a name.
// Without the seccomp tag, only the static table is available.
func ToSyscallName(num uintptr) string {
	return toSyscallName(num)
}
