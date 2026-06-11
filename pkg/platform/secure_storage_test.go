package platform

import (
	"os/exec"
	"strings"
	"testing"
)

// hasSecretService checks whether the D-Bus Secret Service is available
// on the session bus. Returns true only when the bus and the service
// are both reachable.
func hasSecretService() bool {
	cmd := exec.Command("dbus-send", "--session",
		"--dest=org.freedesktop.DBus",
		"--type=method_call",
		"--print-reply",
		"/org/freedesktop/DBus",
		"org.freedesktop.DBus.ListNames")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "org.freedesktop.secrets")
}

func TestLinuxKeychain(t *testing.T) {
	if !hasSecretService() {
		t.Skip("Linux Secret Service (org.freedesktop.secrets) not available on this system")
	}

	k := &LinuxKeychain{}

	t.Run("Store", func(t *testing.T) {
		if err := k.Store("test-svc", "test-acct", "test-pw"); err != nil {
			t.Error("LinuxKeychain.Store should succeed:", err)
		}
	})

	t.Run("Retrieve", func(t *testing.T) {
		pw, err := k.Retrieve("test-svc", "test-acct")
		if err != nil {
			t.Error("LinuxKeychain.Retrieve should succeed:", err)
		}
		if pw != "test-pw" {
			t.Errorf("expected password 'test-pw', got '%s'", pw)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := k.Delete("test-svc", "test-acct"); err != nil {
			t.Error("LinuxKeychain.Delete should succeed:", err)
		}
	})

	t.Run("DeleteNonexistent", func(t *testing.T) {
		// Deleting a non-existent credential should be idempotent
		if err := k.Delete("nonexistent-svc", "nonexistent-acct"); err != nil {
			t.Error("LinuxKeychain.Delete should be idempotent:", err)
		}
	})

	t.Run("RetrieveNotFound", func(t *testing.T) {
		pw, err := k.Retrieve("nonexistent-svc", "nonexistent-acct")
		if err != nil {
			t.Error("LinuxKeychain.Retrieve on missing should return nil, not error:", err)
		}
		if pw != "" {
			t.Errorf("expected empty password for missing entry, got '%s'", pw)
		}
	})

	t.Run("List", func(t *testing.T) {
		_, err := k.List("test-svc")
		if err != nil {
			t.Logf("LinuxKeychain.List: %v (not critical)", err)
		}
	})
}


