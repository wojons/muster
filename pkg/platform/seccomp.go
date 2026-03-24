//go:build linux
// +build linux

// Seccomp BPF filters for secure system call filtering on Linux
package platform

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"

	"github.com/seccomp/libseccomp-golang"
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

// SeccompFilter provides Linux seccomp BPF filtering for syscall restriction
type SeccompFilter struct {
	mode    SeccompFilterMode
	filter  *seccomp.ScmpFilter
	profile SeccompProfile
	allowed map[uint32]bool
	denied  map[uint32]bool
	loaded  bool
}

// NewSeccompFilter creates a new seccomp filter
func NewSeccompFilter(mode SeccompFilterMode) *SeccompFilter {
	return &SeccompFilter{
		mode:    mode,
		allowed: make(map[uint32]bool),
		denied:  make(map[uint32]bool),
	}
}

// AllowSyscall adds a system call to the allowlist
func (sf *SeccompFilter) AllowSyscall(syscallID uintptr) error {
	if sf.filter == nil {
		var err error
		sf.filter, err = seccomp.NewFilter(seccomp.ActErrno.SetReturnCode(int16(syscall.EPERM)))
		if err != nil {
			return fmt.Errorf("failed to create seccomp filter: %w", err)
		}
	}

	id := uint32(syscallID)
	if err := sf.filter.AddRule(seccomp.Syscall(id), seccomp.ActAllow); err != nil {
		return fmt.Errorf("failed to allow syscall %d: %w", syscallID, err)
	}
	sf.allowed[id] = true
	return nil
}

// DenySyscall adds a system call to the denylist
func (sf *SeccompFilter) DenySyscall(syscallID uintptr) error {
	if sf.filter == nil {
		var err error
		sf.filter, err = seccomp.NewFilter(seccomp.ActAllow)
		if err != nil {
			return fmt.Errorf("failed to create seccomp filter: %w", err)
		}
	}

	id := uint32(syscallID)
	if err := sf.filter.AddRule(seccomp.Syscall(id), seccomp.ActErrno.SetReturnCode(int16(syscall.EPERM))); err != nil {
		return fmt.Errorf("failed to deny syscall %d: %w", syscallID, err)
	}
	sf.denied[id] = true
	return nil
}

// Apply applies the seccomp filter to current process
func (sf *SeccompFilter) Apply() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("seccomp is only supported on Linux")
	}

	if sf.filter == nil {
		return fmt.Errorf("filter not initialized")
	}

	if err := sf.filter.Load(); err != nil {
		return fmt.Errorf("failed to load seccomp filter: %w", err)
	}
	sf.loaded = true
	return nil
}

// AllowReadOnly allows only read-only syscalls
func (sf *SeccompFilter) AllowReadOnly() error {
	readOnlySyscalls := []string{
		"read", "open", "openat", "close",
		"stat", "fstat", "lstat", "newfstatat",
		"lseek", "mmap", "mprotect", "munmap",
		"brk", "rt_sigaction", "rt_sigprocmask",
		"pread64", "pwrite64",
		"access", "pipe", "pipe2",
		"select", "poll", "epoll_create", "epoll_create1",
		"epoll_ctl", "epoll_wait", "epoll_pwait",
		"dup", "dup2", "fcntl",
		"getpid", "gettid", "getppid", "getpgrp",
		"exit", "exit_group",
		"arch_prctl",
		"set_tid_address",
		"set_robust_list",
		"futex",
		"sched_getaffinity",
		"getrandom",
	}

	for _, name := range readOnlySyscalls {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}
		if err := sf.AllowSyscall(uintptr(id)); err != nil {
			return err
		}
	}

	return nil
}

// DenyWrite denies all write operations
func (sf *SeccompFilter) DenyWrite() error {
	writeSyscalls := []string{
		"write", "writev", "pwrite64",
		"ioctl", "fcntl",
		"creat", "unlink", "unlinkat",
		"rename", "renameat", "renameat2",
		"mkdir", "mkdirat", "rmdir",
		"chmod", "fchmod", "fchmodat",
		"chown", "fchown", "lchown", "fchownat",
		"symlink", "symlinkat",
		"truncate", "ftruncate",
		"fsync", "fdatasync",
		"sync", "syncfs",
	}

	for _, name := range writeSyscalls {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}
		if err := sf.DenySyscall(uintptr(id)); err != nil {
			return err
		}
	}

	return nil
}

// SetSocketRestriction restricts network socket operations
func (sf *SeccompFilter) SetSocketRestriction(allowBind, allowConnect bool) error {
	socketSyscalls := []string{
		"socket", "accept", "accept4",
		"listen", "setsockopt", "getsockopt",
		"getsockname", "getpeername", "shutdown",
		"sendto", "sendmsg", "sendmmsg",
		"recvfrom", "recvmsg", "recvmmsg",
		"socketpair",
	}

	restrictedSyscalls := []string{"bind", "connect"}
	if allowBind {
		restrictedSyscalls = restrictedSyscalls[:0]
	} else {
		if !allowConnect {
			restrictedSyscalls = append(restrictedSyscalls, "connect")
		}
	}

	for _, name := range socketSyscalls {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}
		isRestricted := false
		for _, restricted := range restrictedSyscalls {
			if name == restricted {
				isRestricted = true
				break
			}
		}
		if isRestricted {
			if err := sf.DenySyscall(uintptr(id)); err != nil {
				return err
			}
		} else {
			if err := sf.AllowSyscall(uintptr(id)); err != nil {
				return err
			}
		}
	}

	return nil
}

// LimitFileAccess restricts file system access
func (sf *SeccompFilter) LimitFileAccess(allowedPaths []string) error {
	if len(allowedPaths) == 0 {
		return fmt.Errorf("no allowed paths specified")
	}

	fileSyscalls := []string{
		"open", "openat", "creat",
		"mkdir", "mkdirat", "rmdir",
		"unlink", "unlinkat",
		"rename", "renameat", "renameat2",
		"symlink", "symlinkat", "readlink", "readlinkat",
	}

	for _, name := range fileSyscalls {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}
		if err := sf.DenySyscall(uintptr(id)); err != nil {
			return err
		}
	}

	return nil
}

// EnableNetworkRestriction restricts network I/O
func (sf *SeccompFilter) EnableNetworkRestriction(allowListen, allowConnect bool) error {
	networkSyscalls := []string{
		"socket", "connect", "accept", "accept4",
		"bind", "listen",
		"sendto", "sendmsg", "sendmmsg",
		"recvfrom", "recvmsg", "recvmmsg",
	}

	for _, name := range networkSyscalls {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}

		shouldAllow := false
		if allowListen && (name == "listen" || name == "accept" || name == "accept4") {
			shouldAllow = true
		}
		if allowConnect && name == "connect" {
			shouldAllow = true
		}
		if name == "socket" || name == "bind" {
			shouldAllow = true
		}

		if shouldAllow {
			if err := sf.AllowSyscall(uintptr(id)); err != nil {
				return err
			}
		} else {
			if err := sf.DenySyscall(uintptr(id)); err != nil {
				return err
			}
		}
	}

	return nil
}

// ApplyToProcess applies filter to specific process by PID
func ApplyToProcess(pid int, filter *SeccompFilter) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	if filter == nil || !filter.loaded {
		return fmt.Errorf("filter not applied or not loaded")
	}

	return fmt.Errorf("applying filter to external process requires ptrace and is not supported in this implementation")
}

// VerifySyscall checks if a syscall is allowed
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

// GetFailedSyscall returns syscall that caused ENOSYS failure
func GetFailedSyscall() (uintptr, uintptr, error) {
	failedMutex.RLock()
	defer failedMutex.RUnlock()

	if lastFailedSyscall == 0 {
		return 0, 0, fmt.Errorf("no failed syscall recorded")
	}

	return lastFailedSyscall, lastFailedArch, nil
}

// EnableStrictMode enables strict seccomp mode
func (sf *SeccompFilter) EnableStrictMode() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("seccomp is only supported on Linux")
	}

	filter, err := seccomp.NewFilter(seccomp.ActKill)
	if err != nil {
		return fmt.Errorf("failed to create strict filter: %w", err)
	}

	if err := filter.Load(); err != nil {
		return fmt.Errorf("failed to load strict filter: %w", err)
	}

	return nil
}

// ApplyProfile applies a predefined seccomp profile
func (sf *SeccompFilter) ApplyProfile(profile SeccompProfile) error {
	if sf.filter == nil {
		var err error
		sf.filter, err = seccomp.NewFilter(seccomp.ActErrno.SetReturnCode(int16(syscall.EPERM)))
		if err != nil {
			return fmt.Errorf("failed to create seccomp filter: %w", err)
		}
	}
	sf.profile = profile

	switch profile {
	case SeccompProfileMinimal:
		return sf.applyMinimalProfile()
	case SeccompProfileStandard:
		return sf.applyStandardProfile()
	case SeccompProfileRelaxed:
		return sf.applyRelaxedProfile()
	default:
		return fmt.Errorf("unknown profile: %d", profile)
	}
}

func (sf *SeccompFilter) applyMinimalProfile() error {
	syscalls := []string{
		"read", "write", "open", "openat", "close",
		"stat", "fstat", "lstat", "newfstatat",
		"lseek", "mmap", "mprotect", "munmap",
		"brk", "rt_sigaction", "rt_sigprocmask",
		"ioctl", "pread64", "pwrite64",
		"access", "pipe", "pipe2",
		"select", "poll", "epoll_create", "epoll_create1",
		"epoll_ctl", "epoll_wait", "epoll_pwait",
		"dup", "dup2", "fcntl",
		"getpid", "gettid", "getppid", "getpgrp",
		"exit", "exit_group",
		"arch_prctl",
		"set_tid_address",
		"set_robust_list",
		"futex",
		"sched_getaffinity",
		"getrandom",
	}

	return sf.addSyscalls(syscalls)
}

func (sf *SeccompFilter) applyStandardProfile() error {
	if err := sf.applyMinimalProfile(); err != nil {
		return err
	}

	networkSyscalls := []string{
		"socket", "connect", "accept", "accept4",
		"bind", "listen", "setsockopt", "getsockopt",
		"getsockname", "getpeername", "shutdown",
		"sendto", "sendmsg", "sendmmsg",
		"recvfrom", "recvmsg", "recvmmsg",
		"socketpair",
	}

	fileSyscalls := []string{
		"creat", "unlink", "unlinkat",
		"rename", "renameat", "renameat2",
		"mkdir", "mkdirat", "rmdir",
		"chmod", "fchmod", "fchmodat",
		"chown", "fchown", "lchown", "fchownat",
		"symlink", "symlinkat", "readlink", "readlinkat",
		"truncate", "ftruncate",
		"fsync", "fdatasync",
		"sync", "syncfs",
	}

	timeSyscalls := []string{
		"time", "gettimeofday", "clock_gettime",
		"clock_getres", "nanosleep", "clock_nanosleep",
	}

	processSyscalls := []string{
		"clone", "fork", "vfork", "execve", "execveat",
		"wait4", "waitid", "kill", "tkill", "tgkill",
		"setpgid", "getpgid", "setsid", "getsid",
		"prctl", "capget", "capset",
	}

	allSyscalls := append(networkSyscalls, fileSyscalls...)
	allSyscalls = append(allSyscalls, timeSyscalls...)
	allSyscalls = append(allSyscalls, processSyscalls...)

	return sf.addSyscalls(allSyscalls)
}

func (sf *SeccompFilter) applyRelaxedProfile() error {
	if err := sf.applyStandardProfile(); err != nil {
		return err
	}

	relaxedSyscalls := []string{
		"ptrace", "personality", "userfaultfd",
		"memfd_create", "memfd_secret",
		"bpf", "perf_event_open",
	}

	return sf.addSyscalls(relaxedSyscalls)
}

func (sf *SeccompFilter) addSyscalls(names []string) error {
	for _, name := range names {
		id, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			continue
		}
		if err := sf.filter.AddRule(seccomp.Syscall(id), seccomp.ActAllow); err != nil {
			return fmt.Errorf("failed to add syscall %s: %w", name, err)
		}
		sf.allowed[uint32(id)] = true
	}
	return nil
}

// CheckSeccompSupport checks if seccomp is supported
func CheckSeccompSupport() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	filter, err := seccomp.NewFilter(seccomp.ActAllow)
	if err != nil {
		return false
	}
	defer filter.Release()
	return true
}

// Syscall constants for common operations
const (
	SYS_READ  uintptr = 0
	SYS_WRITE uintptr = 1
	SYS_OPEN  uintptr = 2
	SYS_STAT  uintptr = 4
)

// toSyscallName converts syscall number to name
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
