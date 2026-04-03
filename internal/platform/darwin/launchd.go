//go:build darwin
// +build darwin

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Launchd provides macOS launchd/launchctl integration for managing
// LaunchAgents and LaunchDaemons.
type Launchd struct {
	userMode bool // true for LaunchAgent, false for LaunchDaemon
}

// NewLaunchd creates a new Launchd manager.
// userMode: true for user LaunchAgent, false for system LaunchDaemon.
func NewLaunchd(userMode bool) *Launchd {
	return &Launchd{userMode: userMode}
}

// Load loads a plist file into launchd.
func (l *Launchd) Load(plistPath string) error {
	if _, err := os.Stat(plistPath); err != nil {
		return fmt.Errorf("plist file not found: %w", err)
	}

	cmd := exec.Command("launchctl", "load", plistPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to load plist: %w (output: %s)", err, string(output))
	}

	return nil
}

// Unload unloads a plist from launchd.
func (l *Launchd) Unload(plistPath string) error {
	cmd := exec.Command("launchctl", "unload", plistPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore "Could not find specified service" errors
		if strings.Contains(string(output), "Could not find") {
			return nil
		}
		return fmt.Errorf("failed to unload plist: %w (output: %s)", err, string(output))
	}

	return nil
}

// Start starts a service by label.
func (l *Launchd) Start(label string) error {
	cmd := exec.Command("launchctl", "start", label)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start service: %w (output: %s)", err, string(output))
	}

	return nil
}

// Stop stops a service by label.
func (l *Launchd) Stop(label string) error {
	cmd := exec.Command("launchctl", "stop", label)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore "Could not find specified service" errors
		if strings.Contains(string(output), "Could not find") {
			return nil
		}
		return fmt.Errorf("failed to stop service: %w (output: %s)", err, string(output))
	}

	return nil
}

// Restart stops and starts a service.
func (l *Launchd) Restart(label string) error {
	if err := l.Stop(label); err != nil {
		return err
	}
	return l.Start(label)
}

// IsRunning checks if a service is currently running.
func (l *Launchd) IsRunning(label string) bool {
	cmd := exec.Command("launchctl", "list", label)
	err := cmd.Run()
	return err == nil
}

// GetLogPaths returns the configured log paths from a plist.
// Returns stdout, stderr paths and any error.
func (l *Launchd) GetLogPaths(plistPath string) (stdout, stderr string, err error) {
	// Parse plist to get log paths
	// This is a simplified version - in production use plist parsing library
	content, err := os.ReadFile(plistPath)
	if err != nil {
		return "", "", err
	}

	data := string(content)

	// Extract stdout path
	if idx := strings.Index(data, "<key>StandardOutPath</key>"); idx != -1 {
		start := strings.Index(data[idx:], "<string>") + idx + 8
		end := strings.Index(data[start:], "</string>") + start
		if start >= 8 && end > start {
			stdout = data[start:end]
			// Expand $USER
			stdout = strings.ReplaceAll(stdout, "$USER", os.Getenv("USER"))
		}
	}

	// Extract stderr path
	if idx := strings.Index(data, "<key>StandardErrorPath</key>"); idx != -1 {
		start := strings.Index(data[idx:], "<string>") + idx + 8
		end := strings.Index(data[start:], "</string>") + start
		if start >= 8 && end > start {
			stderr = data[start:end]
			// Expand $USER
			stderr = strings.ReplaceAll(stderr, "$USER", os.Getenv("USER"))
		}
	}

	return stdout, stderr, nil
}

// EnsureLogDirectory creates the log directory if it doesn't exist.
func (l *Launchd) EnsureLogDirectory(logPath string) error {
	if logPath == "" {
		return nil
	}

	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	return nil
}

// AgentPlistPath returns the path for a user LaunchAgent plist.
func AgentPlistPath(label string) string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", label+".plist")
}

// DaemonPlistPath returns the path for a system LaunchDaemon plist.
func DaemonPlistPath(label string) string {
	return filepath.Join("/Library", "LaunchDaemons", label+".plist")
}
