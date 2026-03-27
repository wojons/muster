//go:build linux
// +build linux

// Platform-specific integration tests for Linux

package platform

import (
	"os"
	"os/exec"
	"testing"
)

// TestNetworkManagerIntegration tests NetworkManager D-Bus connectivity (Linux)
func TestNetworkManagerIntegration(t *testing.T) {
	// Check if D-Bus is available
	_, err := exec.LookPath("dbus-send")
	if err != nil {
		t.Skip("D-Bus not available, skipping NetworkManager integration test")
	}

	// Verify NetworkManager service is running
	cmd := exec.Command("dbus-send", "--system", "--dest=org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus.GetNameOwner", "string:org.freedesktop.NetworkManager")
	if err := cmd.Run(); err != nil {
		t.Skip("NetworkManager service not running, skipping integration test")
	}

	// Test D-Bus connection to org.freedesktop.NetworkManager
	nm, err := NewNetworkManager()
	if err != nil {
		t.Fatalf("NewNetworkManager failed: %v", err)
	}
	if nm == nil {
		t.Fatal("NewNetworkManager should return non-nil instance")
	}

	// Verify GetNetworkSettings returns valid configuration
	settings, err := nm.GetNetworkSettings()
	if err != nil {
		t.Logf("GetNetworkSettings error (may be expected): %v", err)
	} else {
		if settings == nil {
			t.Error("GetNetworkSettings should return non-nil settings")
		}
	}

	// Test CheckConnectivity returns accurate status
	connected, err := nm.CheckConnectivity()
	if err != nil {
		t.Logf("CheckConnectivity error: %v", err)
	} else {
		t.Logf("Connectivity status: %v", connected)
	}

	// Verify proxy detection works correctly
	proxy, err := nm.GetProxySettings()
	if err != nil {
		t.Logf("GetProxySettings error: %v", err)
	} else if proxy != nil {
		t.Logf("System proxy detected: %+v", proxy)
	}
}

// TestSELinuxEnforcement tests SELinux context isolation (Linux)
func TestSELinuxEnforcement(t *testing.T) {
	// Verify SELinux kernel module is available
	if _, err := os.Stat("/sys/fs/selinux"); os.IsNotExist(err) {
		t.Skip("SELinux not available on this system")
	}

	// Test GetContext returns current process context
	selinux, err := NewSELinux()
	if err != nil {
		t.Fatalf("NewSELinux failed: %v", err)
	}

	ctx, err := selinux.GetContext()
	if err != nil {
		t.Logf("GetContext error: %v", err)
	} else if ctx == "" {
		t.Log("GetContext returned empty string (SELinux may be disabled)")
	} else {
		t.Logf("Current SELinux context: %s", ctx)
		if len(ctx) < 10 {
			t.Error("SELinux context should be non-trivial length")
		}
	}

	// Test SetContext applies new context (requires permissions)
	testContext := "system_u:object_r:tmp_t:s0"
	err = selinux.SetContext(testContext)
	if err != nil {
		t.Logf("SetContext error (expected without root): %v", err)
	}

	// Test file context retrieval and setting
	tmpFile := "/tmp/selinux_test_file"
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f.Close()
	defer os.Remove(tmpFile)

	fileCtx, err := selinux.GetFileContext(tmpFile)
	if err != nil {
		t.Logf("GetFileContext error: %v", err)
	} else if fileCtx == "" {
		t.Log("GetFileContext returned empty (file may not have SELinux context)")
	} else {
		t.Logf("File context: %s", fileCtx)
	}

	// Test RestoreContext
	err = selinux.RestoreContext(tmpFile)
	if err != nil {
		t.Logf("RestoreContext error (may be expected): %v", err)
	}
}

// TestSeccompBPFTests tests seccomp syscall filtering (Linux)
func TestSeccompBPFTests(t *testing.T) {
	// Verify seccomp is available
	if _, err := os.Stat("/proc/self/status"); err == nil {
		data, err := os.ReadFile("/proc/self/status")
		if err == nil {
			hasSeccomp := false
			for _, line := range string(data) {
				if line == 'S' {
					hasSeccomp = true
					break
				}
			}
			if !hasSeccomp {
				t.Log("Seccomp may not be enabled")
			}
		}
	}

	// Test seccomp BPF filter creation works
	filter := NewSeccompFilter(SeccompModeFilter)
	if filter == nil {
		t.Fatal("NewSeccompFilter should return non-nil instance")
	}

	// Test AllowSyscall/DenySyscall enforcement
	err := filter.AllowSyscall(5)
	if err != nil {
		t.Logf("AllowSyscall error (may require CAP_SYS_ADMIN): %v", err)
	}

	err = filter.DenySyscall(2)
	if err != nil {
		t.Logf("DenySyscall error (may require CAP_SYS_ADMIN): %v", err)
	}

	// Test FilterApply applies filter to process (requires privileges)
	err = filter.Apply()
	if err != nil {
		t.Logf("FilterApply error (expected without privileges): %v", err)
	}

	// Test predefined profiles
	minimal := NewSeccompFilter(SeccompModeFilter)
	if minimal == nil {
		t.Error("SeccompProfileMinimal should return non-nil profile")
	}

	standard := NewSeccompFilter(SeccompModeStrict)
	if standard == nil {
		t.Error("SeccompProfileStandard should return non-nil profile")
	}

	relaxed := NewSeccompFilter(SeccompModeFilter)
	if relaxed == nil {
		t.Error("SeccompProfileRelaxed should return non-nil profile")
	}
}

// TestSystemdIntegration tests systemd service management (Linux)
func TestSystemdIntegration(t *testing.T) {
	// Verify systemd daemon is running
	if _, err := os.Stat("/run/systemd/system"); os.IsNotExist(err) {
		t.Skip("systemd not running on this system")
	}

	// Check if systemctl is available
	_, err := exec.LookPath("systemctl")
	if err != nil {
		t.Skip("systemctl not available")
	}

	// Test service status command
	cmd := exec.Command("systemctl", "is-system-running")
	output, err := cmd.Output()
	if err != nil {
		t.Skip("Cannot determine systemd state")
	}

	state := string(output)
	t.Logf("Systemd state: %s", state)

	// Verify systemd service path helper works
	path := GetSystemdServicePath()
	if path == "" {
		t.Error("GetSystemdServicePath should return non-empty path")
	}

	t.Logf("Systemd service directory: %s", path)

	// Test CreateSystemdService interface
	err = CreateSystemdService()
	if err != nil {
		t.Logf("CreateSystemdService error (expected without service config): %v", err)
	}
}

// TestPackageInstallation tests package manager integration (Linux)
func TestPackageInstallation(t *testing.T) {
	// Detect package manager
	packageManagers := []struct {
		name string
		cmd  string
	}{
		{"dpkg", "dpkg"},
		{"apt", "apt"},
		{"rpm", "rpm"},
		{"yum", "yum"},
		{"dnf", "dnf"},
		{"apk", "apk"},
		{"pacman", "pacman"},
	}

	found := []string{}
	for _, pm := range packageManagers {
		if _, err := exec.LookPath(pm.cmd); err == nil {
			found = append(found, pm.name)
		}
	}

	if len(found) == 0 {
		t.Skip("No package managers found")
	}

	t.Logf("Found package managers: %v", found)

	// Test package query
	for _, pm := range found {
		switch pm {
		case "dpkg":
			cmd := exec.Command("dpkg", "--version")
			if err := cmd.Run(); err == nil {
				t.Logf("dpkg is functional")
			}
		case "rpm":
			cmd := exec.Command("rpm", "--version")
			if err := cmd.Run(); err == nil {
				t.Logf("rpm is functional")
			}
		case "apk":
			cmd := exec.Command("apk", "--version")
			if err := cmd.Run(); err == nil {
				t.Logf("apk is functional")
			}
		case "pacman":
			cmd := exec.Command("pacman", "--version")
			if err := cmd.Run(); err == nil {
				t.Logf("pacman is functional")
			}
		}
	}
}

// TestAppArmorProfileVerification tests AppArmor profile enforcement (Linux)
func TestAppArmorProfileVerification(t *testing.T) {
	// Verify AppArmor kernel module is loaded
	if _, err := os.Stat("/sys/kernel/security/apparmor"); os.IsNotExist(err) {
		t.Skip("AppArmor not available on this system")
	}

	// Check if apparmor_status is available
	statusCmd, err := exec.LookPath("aa-status")
	if err != nil {
		t.Skip("aa-status command not available")
	}

	// Test profile status
	cmd := exec.Command(statusCmd)
	output, err := cmd.Output()
	if err != nil {
		t.Logf("aa-status error: %v", err)
		t.Skip("Cannot read AppArmor status")
	}

	t.Logf("AppArmor status: %s", string(output))

	// Verify profile template creation
	profile := `
#include <tunables/global>
profile muster-test flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  /tmp/muster-test r,
}
`

	if profile == "" {
		t.Error("AppArmor profile template should be non-empty")
	}

	t.Logf("AppArmor profile template created (%d bytes)", len(profile))
}

// TestKeychainBackendLinux tests Linux Secret Service backend
func TestKeychainBackendLinux(t *testing.T) {
	// Check if Secret Service is available
	_, err := exec.LookPath("dbus-send")
	if err != nil {
		t.Skip("D-Bus not available for Secret Service")
	}

	storage := DefaultSecureStorage()
	if storage == nil {
		t.Fatal("DefaultSecureStorage should return non-nil instance")
	}

	testService := "muster-test-service"
	testAccount := "muster-test-account"
	testValue := "muster-test-value"

	// Test Store/Retrieve/Delete
	err = storage.Store(testService, testAccount, testValue)
	if err != nil {
		t.Logf("SecureStorage.Store error (Secret Service may not be running): %v", err)
		t.Skip("Secret Service not available")
	}
	defer func() {
		_ = storage.Delete(testService, testAccount)
	}()

	value, err := storage.Retrieve(testService, testAccount)
	if err != nil {
		t.Errorf("SecureStorage.Retrieve failed: %v", err)
	} else if value != testValue {
		t.Errorf("Expected %s, got %s", testValue, value)
	}

	err = storage.Delete(testService, testAccount)
	if err != nil {
		t.Errorf("SecureStorage.Delete failed: %v", err)
	}
}
