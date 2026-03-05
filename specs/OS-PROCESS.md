# OS Process Specification

Complete specification for process management, background processes, and PID file handling.

## Overview

Cross-platform process utilities for spawning, monitoring, and managing background processes with PID file support.

## Process Management

```go
package procutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Process represents a managed process
type Process struct {
	cmd    *exec.Cmd
	name   string
	pid    int
	pidFile string
}

// ProcessInfo holds process information
type ProcessInfo struct {
	PID      int
	Name     string
	Command  string
	Status   string
	CPU      float64
	Memory   int64
	StartTime time.Time
}
```

## Spawning Processes

```go
// StartBackground starts a detached background process
func StartBackground(name string, args ...string) (*Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Detach from parent
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	return &Process{
		cmd: cmd,
		name: name,
		pid:  cmd.Process.Pid,
	}, nil
}

// StartWithPID starts process and writes PID file
func StartWithPID(name string, pidFile string, args ...string) (*Process, error) {
	proc, err := StartBackground(name, args...)
	if err != nil {
		return nil, err
	}

	proc.pidFile = pidFile
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(proc.pid)), 0644); err != nil {
		proc.Kill()
		return nil, fmt.Errorf("write pid file: %w", err)
	}

	return proc, nil
}
```

## Process Monitoring

```go
// IsRunning checks if process is running
func (p *Process) IsRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	return p.cmd.Process.Signal(syscall.Signal(0)) == nil
}

// Wait blocks until process exits
func (p *Process) Wait() error {
	return p.cmd.Wait()
}

// Kill terminates the process
func (p *Process) Kill() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// Stop gracefully stops process with timeout
func (p *Process) Stop(timeout time.Duration) error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	// Try graceful shutdown first
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return p.Kill()
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return p.Kill()
	}
}
```

## PID File Management

```go
// PIDFile manages PID file for single instance
type PIDFile struct {
	path string
}

// NewPIDFile creates PID file manager
func NewPIDFile(path string) *PIDFile {
	return &PIDFile{path: path}
}

// Create writes current PID to file
func (p *PIDFile) Create() error {
	// Check if already running
	if pid, err := p.Read(); err == nil && pid > 0 {
		if isPIDRunning(pid) {
			return fmt.Errorf("process already running (pid %d)", pid)
		}
		// Stale PID file, remove it
		os.Remove(p.path)
	}

	pid := os.Getpid()
	return os.WriteFile(p.path, []byte(strconv.Itoa(pid)), 0644)
}

// Read returns PID from file
func (p *PIDFile) Read() (int, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}

	return pid, nil
}

// Remove deletes PID file
func (p *PIDFile) Remove() error {
	return os.Remove(p.path)
}

// isPIDRunning checks if process with PID exists
func isPIDRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	if runtime.GOOS == "windows" {
		return process.Signal(syscall.Signal(0)) == nil
	}
	
	return process.Signal(syscall.Signal(0)) == nil
}
```

## Process Discovery

```go
// FindByName finds processes by name
func FindByName(name string) ([]ProcessInfo, error) {
	if runtime.GOOS == "windows" {
		return findByNameWindows(name)
	}
	return findByNameUnix(name)
}

func findByNameUnix(name string) ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var result []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdline, _ := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if strings.Contains(string(cmdline), name) {
			result = append(result, ProcessInfo{
				PID:     pid,
				Name:    name,
				Command: strings.ReplaceAll(string(cmdline), "\x00", " "),
			})
		}
	}

	return result, nil
}
```

## Usage Example

```go
func RunDaemon() error {
	pidFile := procutil.NewPIDFile("/var/run/muster.pid")
	
	if err := pidFile.Create(); err != nil {
		return err
	}
	defer pidFile.Remove()

	// Run main loop
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	return nil
}
```

## Version

**1.0.0** - 2024-01-15
