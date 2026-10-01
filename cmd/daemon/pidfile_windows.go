//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) is the minimal access right
// needed to query process existence without requiring elevated privileges.
const processQueryLimitedInformation uint32 = 0x1000

// processStillActive is STILL_ACTIVE (winbase.h): the exit code
// GetExitCodeProcess reports while the process is still running.
const processStillActive uint32 = 259

// readPIDFile reads a PID from the given file path.
func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file %s: %w", path, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid PID in %s: %d", path, pid)
	}
	return pid, nil
}

// writePIDFile writes a PID to the given file path.
func writePIDFile(path string, pid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create PID file directory: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

// isProcessRunning reports whether a process with the given PID is running.
//
// Opening the process is not enough: Windows keeps a terminated process's
// object openable while any handle still references it, so a process this
// program just killed (the killer's own os.Process handle) kept reporting as
// running and `restart` failed with "daemon (PID N) did not exit after
// SIGTERM" after burning its whole 5s budget. Only STILL_ACTIVE is liveness.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == processStillActive
}

// terminateProcess kills the process with the given PID and immediately
// releases the handle os.FindProcess opened. Leaving it open keeps the
// terminated process object alive, which is what made isProcessRunning report
// a killed daemon as still running.
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	killErr := proc.Kill()
	_ = proc.Release()
	return killErr
}

// processStartTime on Windows returns "unknown".
func processStartTime(pid int) string {
	return "unknown"
}

// systemBootTime is unused on Windows but kept for interface symmetry.
func systemBootTime() time.Time {
	return time.Now()
}
