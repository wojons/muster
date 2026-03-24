//go:build linux
// +build linux

package platform

import (
	"os"
	"testing"
)

func TestNewSELinux(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}
	if selinux == nil {
		t.Fatal("Expected SELinux interface, got nil")
	}
}

func TestSELinux_Enabled(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	enabled := selinux.Enabled()
	t.Logf("SELinux enabled: %v", enabled)
}

func TestSELinux_GetContext(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	context, err := selinux.GetContext()
	if err != nil {
		t.Fatalf("Failed to get context: %v", err)
	}

	if context == "" {
		t.Fatal("Expected non-empty context")
	}

	t.Logf("Process SELinux context: %s", context)
}

func TestSELinux_GetFileContext(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	tmpfile, err := os.CreateTemp("", "selinux-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()
	_ = tmpfile.Close()

	context, err := selinux.GetFileContext(tmpfile.Name())
	if err != nil {
		t.Logf("Warning: Failed to get file context: %v", err)
		return
	}

	if context == "" {
		t.Log("No SELinux context found (expected on non-SELinux systems)")
		return
	}

	t.Logf("File SELinux context: %s", context)
}

func TestSELinux_GetFileContext_NonExistent(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	_, err = selinux.GetFileContext("/nonexistent/path/file")
	if err == nil {
		t.Fatal("Expected error for non-existent path")
	}

	t.Logf("Got expected error: %v", err)
}

func TestSELinux_SetFileContext(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	tmpfile, err := os.CreateTemp("", "selinux-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()
	_ = tmpfile.Close()

	err = selinux.SetFileContext(tmpfile.Name(), "unconfined_u:object_r:user_home_t:s0")
	if err != nil {
		t.Logf("Warning: Failed to set file context (may require root): %v", err)
		return
	}

	context, err := selinux.GetFileContext(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to get context after setting: %v", err)
	}

	if context == "" {
		t.Fatal("Expected context after setting")
	}

	t.Logf("Set and retrieved context: %s", context)
}

func TestSELinux_RestoreContext(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	tmpfile, err := os.CreateTemp("", "selinux-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()
	_ = tmpfile.Close()

	err = selinux.RestoreContext(tmpfile.Name())
	if err != nil {
		t.Logf("Warning: Failed to restore context (may require root): %v", err)
		return
	}

	t.Log("Successfully restored context")
}

func TestSELinux_SetContext(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	err = selinux.SetContext("unconfined_u:unconfined_r:unconfined_t:s0")
	if err != nil {
		t.Logf("Warning: Failed to set context (may require root): %v", err)
		return
	}

	context, err := selinux.GetContext()
	if err != nil {
		t.Fatalf("Failed to get context after setting: %v", err)
	}

	t.Logf("Set and retrieved context: %s", context)
}
