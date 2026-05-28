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
	mode         SeccompFilterMode
	filter       *seccomp.ScmpFilter
	profile      SeccompProfile
	allowed      map[uint32]bool
	denied       map[uint32]bool
	allowedPaths []string
	loaded       bool
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

// LimitFileAccess restricts file system access.
//
// seccomp operates at the syscall level, not the filesystem-path level.
// Path-based access control requires additional mechanisms (SELinux, BPF
// argument inspection, or a higher-level wrapper). allowedPaths is recorded
// for use by callers (e.g. a path-aware sandbox layer) but does not alter
// the syscall-level deny rules applied here.
//
// Use CheckAccess(path) after calling LimitFileAccess to programmatically
// ask whether a path was declared allowed.
func (sf *SeccompFilter) LimitFileAccess(allowedPaths []string) error {
	if len(allowedPaths) == 0 {
		return fmt.Errorf("no allowed paths specified")
	}

	// Record allowed paths so higher-level wrappers can inspect them.
	sf.allowedPaths = make([]string, len(allowedPaths))
	copy(sf.allowedPaths, allowedPaths)

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

// CheckAccess returns true if path was listed in allowedPaths (or a parent
// directory prefix-match). Call this after LimitFileAccess.
func (sf *SeccompFilter) CheckAccess(path string) bool {
	for _, allowed := range sf.allowedPaths {
		if path == allowed {
			return true
		}
		// Allow sub-paths of declared directories
		if len(path) > len(allowed) && path[:len(allowed)] == allowed && path[len(allowed)] == '/' {
			return true
		}
	}
	return false
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

// RecordFailedSyscall records the last syscall that was denied by seccomp.
// Call this from seccomp notification handlers or BPF action callbacks.
func RecordFailedSyscall(syscallNum, arch uintptr) {
	failedMutex.Lock()
	lastFailedSyscall = syscallNum
	lastFailedArch = arch
	failedMutex.Unlock()
}

// toSyscallName converts syscall number to a human-readable name.
//
// The mapping is populated at init time from the libseccomp syscall table
// when available (Linux). The switch below provides the common x86-64
// numbering for offline / non-Linux reference. Use ToSyscallName() for
// the runtime-resolved name when accuracy matters.
func toSyscallName(num uintptr) string {
	switch num {
	// File I/O
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
	case 17:
		return "pread64"
	case 18:
		return "pwrite64"
	case 21:
		return "access"
	case 22:
		return "pipe"
	case 32:
		return "dup"
	case 33:
		return "dup2"
	case 72:
		return "fcntl"
	case 85:
		return "creat"
	case 87:
		return "unlink"
	case 88:
		return "symlink"
	case 89:
		return "readlink"
	case 257:
		return "openat"
	case 258:
		return "mkdirat"
	case 263:
		return "unlinkat"
	case 264:
		return "renameat"
	case 267:
		return "readlinkat"
	case 276:
		return "renameat2"
	// Process / Signals
	case 13:
		return "rt_sigaction"
	case 14:
		return "rt_sigprocmask"
	case 15:
		return "rt_sigreturn"
	case 39:
		return "getpid"
	case 56:
		return "clone"
	case 57:
		return "fork"
	case 58:
		return "vfork"
	case 59:
		return "execve"
	case 60:
		return "exit"
	case 61:
		return "wait4"
	case 62:
		return "kill"
	case 110:
		return "gettid"
	case 157:
		return "prctl"
	case 158:
		return "arch_prctl"
	case 231:
		return "exit_group"
	// Sockets / Network
	case 41:
		return "socket"
	case 42:
		return "connect"
	case 43:
		return "accept"
	case 49:
		return "bind"
	case 50:
		return "listen"
	case 44:
		return "sendto"
	case 45:
		return "recvfrom"
	case 46:
		return "sendmsg"
	case 47:
		return "recvmsg"
	case 48:
		return "shutdown"
	case 54:
		return "setsockopt"
	case 55:
		return "getsockopt"
	// Filesystem
	case 79:
		return "getcwd"
	case 80:
		return "chdir"
	case 83:
		return "mkdir"
	case 84:
		return "rmdir"
	case 90:
		return "chmod"
	case 91:
		return "fchmod"
	case 268:
		return "fchmodat"
	case 92:
		return "chown"
	case 93:
		return "fchown"
	case 94:
		return "lchown"
	case 260:
		return "fchownat"
	case 74:
		return "fsync"
	case 75:
		return "fdatasync"
	case 162:
		return "syncfs"
	case 76:
		return "ftruncate"
	case 77:
		return "truncate"
	// IO Multiplexing
	case 23:
		return "select"
	case 7:
		return "poll"
	case 213:
		return "epoll_create"
	case 291:
		return "epoll_create1"
	case 233:
		return "epoll_ctl"
	case 232:
		return "epoll_wait"
	case 281:
		return "epoll_pwait"
	// Time
	case 201:
		return "time"
	case 96:
		return "gettimeofday"
	case 228:
		return "clock_gettime"
	case 229:
		return "clock_getres"
	case 35:
		return "nanosleep"
	case 230:
		return "clock_nanosleep"
	// Memory / Futex
	case 202:
		return "futex"
	case 16:
		return "ioctl"
	case 19:
		return "readv"
	case 20:
		return "writev"
	case 218:
		return "set_tid_address"
	case 273:
		return "set_robust_list"
	case 204:
		return "sched_getaffinity"
	case 318:
		return "getrandom"
	case 293:
		return "pipe2"
	case 288:
		return "accept4"
	case 320:
		return "memfd_create"
	case 321:
		return "memfd_secret"
	case 322:
		return "execveat"
	case 247:
		return "waitid"
	case 200:
		return "tkill"
	case 234:
		return "tgkill"
	case 109:
		return "setpgid"
	case 121:
		return "getpgid"
	case 112:
		return "setsid"
	case 124:
		return "getsid"
	case 125:
		return "capget"
	case 126:
		return "capset"
	case 323:
		return "userfaultfd"
	case 101:
		return "ptrace"
	case 135:
		return "personality"
	case 275:
		return "mknod"
	case 259:
		return "mknodat"
	case 282:
		return "signalfd"
	case 289:
		return "signalfd4"
	case 283:
		return "timerfd_create"
	case 287:
		return "timerfd_settime"
	case 286:
		return "timerfd_gettime"
	case 307:
		return "sendmmsg"
	case 299:
		return "recvmmsg"
	case 304:
		return "socketpair"
	default:
		return "unknown"
	}
}

// ToSyscallName resolves the syscall number using the libseccomp table
// when available (Linux only). Falls back to the static toSyscallName
// mapping on non-Linux or when resolution fails.
func ToSyscallName(num uintptr) string {
	if runtime.GOOS == "linux" {
		id := seccomp.Syscall(num)
		name, err := id.GetName()
		if err == nil {
			return name
		}
	}
	return toSyscallName(num)
}
