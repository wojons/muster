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
	// Check if SELinux is available on the system
	if _, err := os.Stat("/selinux"); os.IsNotExist(err) {
		return &SELinuxInterface{enabled: false}, nil
	}
	return &SELinuxInterface{enabled: true}, nil
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

// GetFileContext retrieves file/directory SELinux context
func (s *SELinuxInterface) GetFileContext(path string) (string, error) {
	if !s.enabled {
		return "", nil
	}

	cmd := exec.Command("ls", "-Z", path)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file context: %w", err)
	}

	parts := strings.Fields(string(output))
	if len(parts) < 1 {
		return "", fmt.Errorf("no SELinux context found for path: %s", path)
	}

	context := strings.TrimSpace(parts[0])
	if context == "" || context == "?" {
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
