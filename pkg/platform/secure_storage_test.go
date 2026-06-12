//go:build darwin

package platform

import (
	"errors"
	"testing"
)

func TestMacOSKeychain_Store(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	k := &MacOSKeychain{}

	t.Run("StorePassword", func(t *testing.T) {
		err := k.Store("test-service", "test-account", "test-password")
		if err != nil {
			t.Error("Store should succeed:", err)
		}
	})
}

func TestMacOSKeychain_Retrieve(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	k := &MacOSKeychain{}

	t.Run("RetrievePassword", func(t *testing.T) {
		password, err := k.Retrieve("test-service", "test-account")
		if err != nil {
			t.Error("Retrieve should succeed:", err)
		}
		if password == "" {
			t.Error("Password should not be empty")
		}
	})

	t.Run("RetrieveNotFound", func(t *testing.T) {
		_, err := k.Retrieve("nonexistent-service", "nonexistent-account")
		if err != nil {
			var notFound interface {
				NotFound() bool
			}
			if errors.As(err, &notFound) {
				t.Log("Not found error handled correctly")
			}
		}
	})
}

func TestMacOSKeychain_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	k := &MacOSKeychain{}

	t.Run("DeletePassword", func(t *testing.T) {
		err := k.Delete("test-service", "test-account")
		if err != nil {
			t.Error("Delete should succeed:", err)
		}
	})

	t.Run("DeleteNonexistent", func(t *testing.T) {
		err := k.Delete("nonexistent-service", "nonexistent-account")
		if err != nil {
			t.Error("Delete should succeed even for nonexistent entries:", err)
		}
	})
}

func TestMacOSKeychain_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	k := &MacOSKeychain{}

	t.Run("ListAccounts", func(t *testing.T) {
		accounts, err := k.List("test-service")
		if err != nil {
			t.Error("List should succeed:", err)
		}
		if accounts == nil {
			t.Error("Accounts should not be nil")
		}
	})
}

func TestLinuxKeychain(t *testing.T) {
	k := &LinuxKeychain{}

	t.Run("Store", func(t *testing.T) {
		if err := k.Store("service", "account", "password"); err != nil {
			t.Error("LinuxKeychain.Store should return nil:", err)
		}
	})

	t.Run("Retrieve", func(t *testing.T) {
		_, err := k.Retrieve("service", "account")
		if err != nil {
			t.Error("LinuxKeychain.Retrieve should succeed:", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := k.Delete("service", "account"); err != nil {
			t.Error("LinuxKeychain.Delete should return nil:", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		_, err := k.List("service")
		if err != nil {
			t.Error("LinuxKeychain.List should succeed:", err)
		}
	})
}

func TestWindowsKeychain(t *testing.T) {
	k := &WindowsKeychain{}

	t.Run("Store", func(t *testing.T) {
		if err := k.Store("service", "account", "password"); err != nil {
			t.Error("WindowsKeychain.Store should return nil:", err)
		}
	})

	t.Run("Retrieve", func(t *testing.T) {
		_, err := k.Retrieve("service", "account")
		if err != nil {
			t.Error("WindowsKeychain.Retrieve should succeed:", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := k.Delete("service", "account"); err != nil {
			t.Error("WindowsKeychain.Delete should return nil:", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		_, err := k.List("service")
		if err != nil {
			t.Error("WindowsKeychain.List should succeed:", err)
		}
	})
}
