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

// isProcessRunning is declared below; fmt, strings, and os are used by it.

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

// isProcessRunning reports whether a process with the given PID exists and
// is not a zombie. Signal 0 checks existence; /proc/<pid>/status excludes
// zombies (State: Z) that have exited but not been reaped by their parent.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	// Exclude zombies — see internal/builtin/isprocess_unix.go for the
	// full rationale. In containers where PID 1 does not reap children,
	// a stopped daemon becomes a zombie and signal 0 still succeeds.
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
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
