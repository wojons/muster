//go:build linux
// +build linux

package linux

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// SELinuxContext holds security context information
type SELinuxContext struct {
	User  string
	Role  string
	Type  string
	Level string
}

// IsSELinuxEnabled checks if SELinux is enabled on the system
func IsSELinuxEnabled() bool {
	cmd := exec.Command("getenforce")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	status := strings.TrimSpace(string(output))
	return status == "Enforcing" || status == "Permissive"
}

// GetFileContext retrieves the SELinux context of a file
func GetFileContext(path string) (*SELinuxContext, error) {
	cmd := exec.Command("ls", "-Z", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output: system_u:object_r:etc_t:s0 /etc/passwd
	parts := strings.Fields(string(output))
	if len(parts) < 4 {
		return nil, nil // No SELinux context
	}

	contextParts := strings.Split(parts[2], ":")
	if len(contextParts) < 3 {
		return nil, nil
	}

	ctx := &SELinuxContext{
		User: contextParts[0],
		Role: contextParts[1],
		Type: contextParts[2],
	}

	if len(contextParts) > 3 {
		ctx.Level = contextParts[3]
	}

	return ctx, nil
}

// SetFileContext sets the SELinux context for a file
func SetFileContext(path, context string) error {
	cmd := exec.Command("chcon", context, path)
	return cmd.Run()
}

// RestoreCon restores the default SELinux context for a file
func RestoreCon(path string, recursive bool) error {
	args := []string{"-v"}
	if recursive {
		args = append(args, "-R")
	}
	args = append(args, path)

	cmd := exec.Command("restorecon", args...)
	return cmd.Run()
}

// ApplyDaemonContexts applies appropriate SELinux contexts for daemon files
func ApplyDaemonContexts(baseDir string) error {
	if !IsSELinuxEnabled() {
		return nil
	}

	contexts := map[string]string{
		filepath.Join(baseDir, "etc"):        "etc_t",
		filepath.Join(baseDir, "bin/muster"): "bin_t",
		filepath.Join(baseDir, "var/lib"):    "var_lib_t",
		filepath.Join(baseDir, "var/log"):    "var_log_t",
		filepath.Join(baseDir, "run"):        "var_run_t",
	}

	for path, contextType := range contexts {
		context := "system_u:object_r:" + contextType + ":s0"
		if err := SetFileContext(path, context); err != nil {
			return err
		}
	}

	// Restore contexts recursively
	return RestoreCon(baseDir, true)
}
