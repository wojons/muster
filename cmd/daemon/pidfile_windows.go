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

// isProcessRunning reports whether a process with the given PID exists.
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

// terminateProcess kills the process with the given PID.
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// processStartTime on Windows returns "unknown".
func processStartTime(pid int) string {
	return "unknown"
}

// systemBootTime is unused on Windows but kept for interface symmetry.
func systemBootTime() time.Time {
	return time.Now()
}
