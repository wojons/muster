//go:build !windows

package builtin

import (
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM to the process with the given PID.
// Unix implementation: direct syscall.Kill (SIGTERM lets the daemon
// clean up its socket and PID file before exiting).
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
