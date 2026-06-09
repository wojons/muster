// SELinux context handling for Linux security isolation
// REQUIRES: Complete implementation for production security features

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SELinuxInterface provides Linux SELinux context management
// Implements process context isolation for security enforcement
type SELinuxInterface struct {
	enabled bool
}

func NewSELinux() (*SELinuxInterface, error) {
	enabled, err := isSELinuxEnabled()
	if err != nil {
		// If detection fails, assume SELinux is disabled
		return &SELinuxInterface{enabled: false}, nil
	}
	return &SELinuxInterface{enabled: enabled}, nil
	// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md#6-security plan=phase-2/task-2-1/step-2-1-1 impl=isSELinuxEnabled
}

// isSELinuxEnabled detects SELinux using the modern detection chain:
//
//  1. Try the getenforce command (from policycoreutils). Returns
//     "Enforcing", "Permissive", or "Disabled". Only "Disabled" means
//     SELinux is fully off.
//  2. Fall back to checking /sys/fs/selinux (the modern SELinux pseudo-
//     filesystem mount point). The legacy /selinux path was removed in
//     kernel 2.6+ and should not be used.
//
// Returns true when SELinux is confirmed enabled, false otherwise.
func isSELinuxEnabled() (bool, error) {
	// Method 1: getenforce — most reliable on modern systems
	cmd := exec.Command("getenforce")
	output, err := cmd.Output()
	if err == nil {
		status := strings.TrimSpace(string(output))
		return status != "Disabled", nil
	}

	// Method 2: /sys/fs/selinux mount point (modern fallback)
	if fi, err := os.Stat("/sys/fs/selinux"); err == nil && fi.IsDir() {
		return true, nil
	}

	return false, nil
}

// GetContext retrieves current process's SELinux context
func (s *SELinuxInterface) GetContext() (string, error) {
	if !s.enabled {
		return "unconfined_u:unconfined_r:unconfined_t:s0", nil
	}

	data, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		return "", fmt.Errorf("failed to read SELinux context: %w", err)
	}

	context := strings.TrimSpace(string(data))
	if context == "" {
		return "", fmt.Errorf("empty SELinux context")
	}

	return context, nil
}

// SetContext sets process SELinux context
func (s *SELinuxInterface) SetContext(context string) error {
	if !s.enabled {
		return nil
	}

	f, err := os.OpenFile("/proc/self/attr/current", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open SELinux interface: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	_, err = f.WriteString(context)
	if err != nil {
		return fmt.Errorf("failed to set SELinux context: %w", err)
	}

	return nil
}

// GetFileContext retrieves file/directory SELinux context.
//
// Uses stat --printf='%C' which outputs the raw SELinux context string
// directly, unlike ls -Z which requires brittle output parsing.
// stat(1) returns "?" when SELinux is not available on the filesystem.
//
// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md#6-security plan=phase-2/task-2-2/step-2-2-1 impl=pkg/platform/selinux.go
func (s *SELinuxInterface) GetFileContext(path string) (string, error) {
	if !s.enabled {
		return "", nil
	}

	cmd := exec.Command("stat", "--printf=%C", path)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file context: %w", err)
	}

	context := strings.TrimSpace(string(output))
	if context == "" || context == "?" || strings.Contains(context, "unsupported") {
		return "", fmt.Errorf("invalid SELinux context for path: %s", path)
	}

	return context, nil
}

// SetFileContext sets file/directory SELinux context
func (s *SELinuxInterface) SetFileContext(path, context string) error {
	if !s.enabled {
		return nil
	}

	cmd := exec.Command("chcon", context, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set file context: %w, output: %s", err, string(output))
	}

	return nil
}

// Enabled returns whether SELinux is available on system
func (s *SELinuxInterface) Enabled() bool {
	return s.enabled
}

// RestoreContext restores default context for file
func (s *SELinuxInterface) RestoreContext(path string) error {
	if !s.enabled {
		return nil
	}

	cmd := exec.Command("restorecon", "-v", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore context: %w, output: %s", err, string(output))
	}

	return nil
}
