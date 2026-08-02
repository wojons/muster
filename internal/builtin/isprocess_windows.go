//go:build windows
// +build windows

package builtin

import "syscall"

// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) is the minimal access right
// needed to query process existence without requiring elevated privileges.
const processQueryLimitedInformation uint32 = 0x1000

// isProcessRunning reports whether a process with the given PID exists.
// Windows implementation: os.FindProcess always succeeds on Windows (it
// does not verify existence), and Process.Signal is unsupported for
// signal 0 — so neither can be used. OpenProcess with a minimal query
// access right is the reliable existence check.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(handle)
	return true
}
