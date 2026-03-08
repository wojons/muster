package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Manager handles PID file lifecycle
type Manager struct {
	pidFilePath string
	socketPath  string
	version     string
}

// Config for PID manager initialization
type Config struct {
	PidFilePath string
	SocketPath  string
	Version     string
}

// Status represents the current daemon status
type Status struct {
	Running    bool
	PID        int
	StartTime  time.Time
	SocketPath string
	Version    string
	Stale      bool
}

const (
	DefaultPIDVersion = "1.0.0"
)

// NewManager creates a new PID file manager
func NewManager(cfg Config) (*Manager, error) {
	if cfg.PidFilePath == "" {
		cfg.PidFilePath = DefaultPIDFilePath()
	}
	if cfg.Version == "" {
		cfg.Version = DefaultPIDVersion
	}

	return &Manager{
		pidFilePath: cfg.PidFilePath,
		socketPath:  cfg.SocketPath,
		version:     cfg.Version,
	}, nil
}

// DefaultPIDFilePath returns the default PID file path for the current platform
func DefaultPIDFilePath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "openapi-cli", "daemon.pid")
	default:
		dir := os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".local", "share")
		}
		return filepath.Join(dir, "openapi-cli", "daemon.pid")
	}
}

// Acquire attempts to create PID file, returns error if another instance is running
func (m *Manager) Acquire() error {
	// Ensure directory exists
	dir := filepath.Dir(m.pidFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create pid directory: %w", err)
	}

	// Check existing PID file
	if err := m.checkExisting(); err != nil {
		return err
	}

	// Create new PID file atomically
	tempFile := m.pidFilePath + ".tmp"
	content := fmt.Sprintf("%d\n%d\n%s\n%s\n",
		os.Getpid(),
		time.Now().UnixNano(),
		m.socketPath,
		m.version,
	)

	if err := os.WriteFile(tempFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write temp pid file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, m.pidFilePath); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("rename pid file: %w", err)
	}

	return nil
}

// checkExisting verifies if another daemon is running
func (m *Manager) checkExisting() error {
	data, err := os.ReadFile(m.pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing file
		}
		return fmt.Errorf("read pid file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 1 {
		return nil // Corrupted file, treat as non-existent
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		// Corrupted PID, remove stale file
		_ = os.Remove(m.pidFilePath)
		return nil
	}

	// Check if process is still running
	if isProcessRunning(pid) {
		return fmt.Errorf("daemon already running (PID: %d)", pid)
	}

	// Stale PID file, remove it
	_ = os.Remove(m.pidFilePath)
	return nil
}

// isProcessRunning checks if a process with given PID exists
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, must send signal 0 to check
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Release removes the PID file
func (m *Manager) Release() error {
	// Verify we own this PID file
	data, err := os.ReadFile(m.pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already cleaned up
		}
		return fmt.Errorf("read pid file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) >= 1 {
		pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
		if pid != os.Getpid() {
			return fmt.Errorf("PID file owned by different process (PID: %d)", pid)
		}
	}

	if err := os.Remove(m.pidFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pid file: %w", err)
	}

	return nil
}

// GetStatus reads the PID file and returns daemon status
func (m *Manager) GetStatus() (*Status, error) {
	data, err := os.ReadFile(m.pidFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Status{Running: false}, nil
		}
		return nil, fmt.Errorf("read pid file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 4 {
		return &Status{Running: false, Stale: true}, nil
	}

	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	timestamp, _ := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	socketPath := strings.TrimSpace(lines[2])
	version := strings.TrimSpace(lines[3])

	running := isProcessRunning(pid)

	return &Status{
		Running:    running,
		PID:        pid,
		StartTime:  time.Unix(0, timestamp),
		SocketPath: socketPath,
		Version:    version,
		Stale:      !running && pid > 0,
	}, nil
}
