//go:build !windows
// +build !windows

package builtin

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// isProcessRunning reports whether a process with the given PID exists and
// is not a zombie. Unix implementation: signal 0 checks for existence
// without delivering a real signal, then /proc/<pid>/status is read to
// exclude zombie (State: Z) processes that have been reaped by init but
// not yet waited on by their parent. os.FindProcess always succeeds on
// Windows (where this file is excluded), so the signal probe is the actual
// existence check here.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks for existence without delivering a real signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	// Exclude zombies: signal 0 succeeds for zombies, but they are not
	// truly running — their parent has not yet called waitpid(). In a
	// container where PID 1 does not reap, a stopped daemon becomes a
	// zombie and isProcessRunning would otherwise report it as alive.
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		// /proc read failed but signal 0 succeeded — treat as running
		// (defensive: some environments restrict /proc access).
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "Z" {
				return false
			}
			break
		}
	}
	return true
}
