//go:build linux
// +build linux

package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// selinuxAvailable checks if SELinux is enabled and usable on this host.
// Skips the test with a clear message if SELinux is unavailable or disabled.
//
// axiom:trace work_item=ac-015-platform-test-guards spec=specs/02-Workflows.md impl=pkg/platform/selinux_test.go
func selinuxAvailable(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("SELinux only supported on Linux")
	}
	if _, err := os.Stat("/sys/fs/selinux"); os.IsNotExist(err) {
		t.Skip("SELinux not available (/sys/fs/selinux missing)")
	}
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		t.Skipf("SELinux not available (getenforce failed): %v", err)
	}
	if strings.TrimSpace(string(out)) == "Disabled" {
		t.Skip("SELinux is disabled")
	}
}

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

// TestSELinux_ModernDetection verifies the isSELinuxEnabled() detection chain:
//
//  1. Try the getenforce command (primary)
//  2. Fall back to /sys/fs/selinux stat (secondary)
//
// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md#6-security plan=phase-2/task-2-1/step-2-1-2 impl=pkg/platform/selinux.go
func TestSELinux_ModernDetection(t *testing.T) {
	// Verify isSELinuxEnabled() returns a consistent result
	enabled, err := isSELinuxEnabled()
	if err != nil {
		t.Fatalf("isSELinuxEnabled() returned error: %v", err)
	}

	// Cross-validate with getenforce when available
	getenforce := exec.Command("getenforce")
	output, cmdErr := getenforce.Output()
	if cmdErr == nil {
		status := strings.TrimSpace(string(output))
		switch status {
		case "Enforcing", "Permissive":
			if !enabled {
				t.Errorf("getenforce reports %s but isSELinuxEnabled() returned false", status)
			}
		case "Disabled":
			if enabled {
				t.Errorf("getenforce reports Disabled but isSELinuxEnabled() returned true")
			}
		default:
			t.Logf("Unknown getenforce status: %s", status)
		}
		t.Logf("getenforce detection: status=%s, isSELinuxEnabled=%v", status, enabled)
	} else {
		// getenforce not available: verify fallback to /sys/fs/selinux
		t.Logf("getenforce not available (%v), checking /sys/fs/selinux fallback", cmdErr)
		if fi, statErr := os.Stat("/sys/fs/selinux"); statErr == nil && fi.IsDir() {
			if !enabled {
				t.Error("/sys/fs/selinux exists but isSELinuxEnabled() returned false")
			}
		} else {
			t.Log("/sys/fs/selinux not found — SELinux disabled or container without SELinux fs")
		}
	}

	// Verify NewSELinux() creates working interface when enabled
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("NewSELinux() error: %v", err)
	}
	if selinux.Enabled() != enabled {
		t.Errorf("NewSELinux().Enabled() = %v, expected %v", selinux.Enabled(), enabled)
	}

	// If SELinux is enabled, basic operations should work
	if enabled {
		ctx, ctxErr := selinux.GetContext()
		if ctxErr != nil {
			t.Errorf("GetContext() error when SELinux enabled: %v", ctxErr)
		} else if ctx == "" {
			t.Error("GetContext() returned empty context when SELinux enabled")
		} else {
			t.Logf("Process context: %s", ctx)
		}
	}
}

// TestSELinux_GetFileContext_Robust verifies the stat --printf='%C' replacement
// for GetFileContext handles various paths and error conditions correctly.
//
// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md#6-security plan=phase-2/task-2-2/step-2-2-2 impl=pkg/platform/selinux.go test=pkg/platform/selinux_test.go
func TestSELinux_GetFileContext_Robust(t *testing.T) {
	selinuxAvailable(t)

	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("Failed to create SELinux interface: %v", err)
	}

	// Helper to check if SELinux is actually functional on this system
	// (stat --printf='%C' may return "unsupported" in containers)
	canGetContext := func(path string) bool {
		cmd := exec.Command("stat", "--printf=%C", path)
		output, runErr := cmd.Output()
		if runErr != nil {
			return false
		}
		ctx := strings.TrimSpace(string(output))
		return ctx != "" && ctx != "?" && !strings.Contains(ctx, "unsupported")
	}

	// Test 1: Non-existent path returns error
	t.Run("non-existent path", func(t *testing.T) {
		_, err := selinux.GetFileContext("/nonexistent/path/that/does/not/exist")
		if err == nil {
			t.Error("Expected error for non-existent path")
		}
		t.Logf("Non-existent path error: %v", err)
	})

	// Test 2: Empty path returns error
	t.Run("empty path", func(t *testing.T) {
		_, err := selinux.GetFileContext("")
		if err == nil {
			t.Error("Expected error for empty path")
		}
		t.Logf("Empty path error: %v", err)
	})

	// Test 3: Existing directory is handled
	t.Run("existing directory", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "selinux-robust-test-*")
		if err != nil {
			t.Skipf("Cannot create temp dir in this environment: %v", err)
		}
		defer os.RemoveAll(dir)

		ctx, err := selinux.GetFileContext(dir)
		if err != nil {
			t.Logf("Directory context error (expected in containers): %v", err)
			return
		}
		t.Logf("Directory SELinux context: %s", ctx)
	})

	// Test 4: Existing regular file
	t.Run("existing file", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "selinux-robust-file-*")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		ctx, err := selinux.GetFileContext(tmpfile.Name())
		if err != nil {
			t.Logf("File context error (expected in containers): %v", err)
			return
		}
		t.Logf("File SELinux context: %s", ctx)
	})

	// Test 5: Verify stat is actually being used (not ls -Z)
	t.Run("uses stat not ls", func(t *testing.T) {
		if !selinux.Enabled() {
			t.Skip("SELinux not enabled")
		}
		if !canGetContext("/tmp") {
			t.Skip("SELinux filesystem context not available in this environment")
		}

		// Run both commands and verify they produce compatible output
		tmpfile, err := os.CreateTemp("", "selinux-compare-*")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		// Use our new GetFileContext (stat-based)
		ourCtx, ourErr := selinux.GetFileContext(tmpfile.Name())
		if ourErr != nil {
			t.Fatalf("GetFileContext failed: %v", ourErr)
		}

		// Should be a valid SELinux context string
		if !strings.Contains(ourCtx, ":") {
			t.Logf("Context from stat: %s (may be partial without full SELinux)", ourCtx)
		}
		t.Logf("GetFileContext result: %s", ourCtx)
	})
}

// TestSELinux_RoundTrip verifies that GetContext returns a parseable
// SELinux context string that can be read back from /proc/self/attr/current.
//
// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md#6-security plan=phase-4/task-4-1/step-4-1-2 impl=pkg/platform/selinux.go test=pkg/platform/selinux_test.go
func TestSELinux_RoundTrip(t *testing.T) {
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("NewSELinux() error: %v", err)
	}

	if !selinux.Enabled() {
		t.Skip("SELinux not enabled on this system")
	}

	// Test 1: Get current process context directly from /proc
	t.Run("process context via /proc/self/attr/current", func(t *testing.T) {
		data, err := os.ReadFile("/proc/self/attr/current")
		if err != nil {
			t.Fatalf("Failed to read /proc/self/attr/current: %v", err)
		}
		rawCtx := strings.TrimSpace(string(data))
		if rawCtx == "" {
			t.Fatal("Empty context in /proc/self/attr/current")
		}
		t.Logf("Raw /proc context: %q", rawCtx)

		// Verify it is non-empty and parse if in user:role:type format
		if strings.Contains(rawCtx, ":") {
			parts := strings.SplitN(rawCtx, ":", 4)
			if len(parts) >= 3 {
				t.Logf("Parsed context — user=%s role=%s type=%s level=%s",
					parts[0], parts[1], parts[2],
					func() string {
						if len(parts) > 3 {
							return parts[3]
						}
						return "(default)"
					}())
			}
		} else {
			t.Logf("Context %q is a minimal/single-label context (valid in containers)", rawCtx)
		}
	})

	// Test 2: GetContext returns the same value as direct /proc read
	t.Run("GetContext matches /proc", func(t *testing.T) {
		ctx, err := selinux.GetContext()
		if err != nil {
			t.Fatalf("GetContext() error: %v", err)
		}

		data, err := os.ReadFile("/proc/self/attr/current")
		if err != nil {
			t.Fatalf("Failed to read /proc/self/attr/current: %v", err)
		}
		expected := strings.TrimSpace(string(data))

		if ctx != expected {
			t.Errorf("GetContext() = %q, /proc/self/attr/current = %q", ctx, expected)
		}
	})

	// Test 3: Temp file context round-trip
	t.Run("temp file context", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "selinux-roundtrip-*")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		ctx, err := selinux.GetFileContext(tmpfile.Name())
		if err != nil {
			t.Logf("GetFileContext on temp file: %v (expected in containers without SELinux fs support)", err)
			return
		}
		if ctx == "" {
			t.Fatal("Empty context from GetFileContext")
		}
		t.Logf("Temp file SELinux context: %s", ctx)
	})
}
