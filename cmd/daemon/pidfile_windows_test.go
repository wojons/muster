//go:build windows

package main

import (
	"os/exec"
	"testing"
	"time"
)

// TestTerminateProcess_StopsReportingTheProcessAsRunning is the regression for
// the windows-latest `restart` failure: after terminateProcess, the killed
// daemon must stop being reported as running. OpenProcess keeps succeeding for
// a terminated process while any handle references its object — including the
// killer's own os.Process handle, which was never released — so `restart`
// spun for its whole 5s budget and failed with "daemon (PID N) did not exit
// after SIGTERM" while the process was already dead.
func TestTerminateProcess_StopsReportingTheProcessAsRunning(t *testing.T) {
	// A child that lives long enough to be observed and killed.
	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 > NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	pid := cmd.Process.Pid
	if !isProcessRunning(pid) {
		t.Fatalf("child %d should be reported as running", pid)
	}

	if err := terminateProcess(pid); err != nil {
		t.Fatalf("terminateProcess(%d) = %v, want nil", pid, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && isProcessRunning(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if isProcessRunning(pid) {
		t.Fatalf("child %d is still reported as running after terminateProcess", pid)
	}
}
