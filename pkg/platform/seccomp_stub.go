//go:build !linux
// +build !linux

// Stub implementations for seccomp on non-Linux platforms
package platform

import "fmt"

// SeccompFilterMode represents different seccomp filtering modes (stub for non-Linux)
type SeccompFilterMode int

const (
	SeccompModeFilter SeccompFilterMode = iota + 1
	SeccompModeStrict
)

// SeccompProfile defines the allowed syscalls (stub for non-Linux)
type SeccompProfile int

const (
	SeccompProfileMinimal SeccompProfile = iota
	SeccompProfileStandard
	SeccompProfileRelaxed
)

// SeccompFilter provides stub seccomp BPF filtering for non-Linux platforms
type SeccompFilter struct {
	mode    SeccompFilterMode
	profile SeccompProfile
	allowed map[uint32]bool
	denied  map[uint32]bool
	loaded  bool
}

// NewSeccompFilter creates a new seccomp filter (stub for non-Linux)
func NewSeccompFilter(mode SeccompFilterMode) *SeccompFilter {
	return &SeccompFilter{
		mode:    mode,
		allowed: make(map[uint32]bool),
		denied:  make(map[uint32]bool),
	}
}

// AllowSyscall adds a system call to the allowlist (stub for non-Linux)
func (sf *SeccompFilter) AllowSyscall(syscallID uintptr) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// DenySyscall adds a system call to the denylist (stub for non-Linux)
func (sf *SeccompFilter) DenySyscall(syscallID uintptr) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// Apply applies the seccomp filter to current process (stub for non-Linux)
func (sf *SeccompFilter) Apply() error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// AllowReadOnly allows only read-only syscalls (stub for non-Linux)
func (sf *SeccompFilter) AllowReadOnly() error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// DenyWrite denies all write operations (stub for non-Linux)
func (sf *SeccompFilter) DenyWrite() error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// SetSocketRestriction restricts network socket operations (stub for non-Linux)
func (sf *SeccompFilter) SetSocketRestriction(allowBind, allowConnect bool) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// LimitFileAccess restricts file system access (stub for non-Linux)
func (sf *SeccompFilter) LimitFileAccess(allowedPaths []string) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// EnableNetworkRestriction restricts network I/O (stub for non-Linux)
func (sf *SeccompFilter) EnableNetworkRestriction(allowListen, allowConnect bool) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// EnableStrictMode enables strict seccomp mode (stub for non-Linux)
func (sf *SeccompFilter) EnableStrictMode() error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// ApplyProfile applies a predefined seccomp profile (stub for non-Linux)
func (sf *SeccompFilter) ApplyProfile(profile SeccompProfile) error {
	sf.profile = profile
	return nil
}

// VerifySyscall checks if a syscall is allowed (stub for non-Linux)
func (sf *SeccompFilter) VerifySyscall(syscallID uintptr) (bool, error) {
	return false, fmt.Errorf("seccomp is only supported on Linux")
}

// CheckSeccompSupport checks if seccomp is supported (stub for non-Linux)
func CheckSeccompSupport() bool {
	return false
}

// Syscall constants (stub for non-Linux)
const (
	SYS_READ  uintptr = 0
	SYS_WRITE uintptr = 1
	SYS_OPEN  uintptr = 2
	SYS_STAT  uintptr = 4
)

// toSyscallName converts syscall number to name (stub for non-Linux)
func toSyscallName(num uintptr) string {
	switch num {
	case 0:
		return "read"
	case 1:
		return "write"
	case 2:
		return "open"
	case 4:
		return "stat"
	default:
		return "unknown"
	}
}

// ApplyToProcess applies filter to specific process by PID (stub for non-Linux)
func ApplyToProcess(pid int, filter *SeccompFilter) error {
	return fmt.Errorf("seccomp is only supported on Linux")
}

// GetFailedSyscall returns syscall that caused ENOSYS failure (stub for non-Linux)
func GetFailedSyscall() (uintptr, uintptr, error) {
	return 0, 0, fmt.Errorf("seccomp is only supported on Linux")
}
