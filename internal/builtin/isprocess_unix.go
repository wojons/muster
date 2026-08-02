//go:build !windows
// +build !windows

package builtin

import (
	"os"
	"syscall"
)

// isProcessRunning reports whether a process with the given PID exists.
// Unix implementation: signal 0 checks for existence without delivering a
// real signal. os.FindProcess always succeeds on Windows (where this file
// is excluded), so the signal probe is the actual existence check here.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks for existence without delivering a real signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
