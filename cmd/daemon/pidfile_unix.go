//go:build !windows

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
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// terminateProcess sends SIGTERM to the process with the given PID.
func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// processStartTime returns the wall-clock start time of the process with
// the given PID, read from /proc/<pid>/stat.
func processStartTime(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "unknown"
	}

	idx := strings.LastIndexByte(string(data), ')')
	if idx < 0 || idx+2 >= len(data) {
		return "unknown"
	}

	fields := strings.Fields(string(data)[idx+2:])
	if len(fields) < 20 {
		return "unknown"
	}
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return "unknown"
	}

	ticksPerSec := int64(100)
	if v := os.Getenv("USER_HZ"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			ticksPerSec = n
		}
	}
	bootTime := systemBootTime()
	started := bootTime.Add(time.Duration(startTicks * int64(time.Second) / ticksPerSec))
	return started.Format("2006-01-02 15:04:05")
}

// systemBootTime returns the wall-clock time of system boot.
func systemBootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Now()
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			secs, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			if err == nil {
				return time.Unix(secs, 0)
			}
		}
	}
	return time.Now()
}
