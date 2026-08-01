//go:build windows

package builtin

import "os"

// terminateProcess sends SIGTERM to the process with the given PID.
// Windows implementation: os.Process.Kill (TerminateProcess) — SIGTERM
// is not a real signal on Windows.
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
