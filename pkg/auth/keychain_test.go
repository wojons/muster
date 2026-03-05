package auth

import (
	"testing"
)

func TestNewKeychain(t *testing.T) {
	kc, err := NewKeychain()
	if err != nil {
		t.Fatalf("NewKeychain() error = %v", err)
	}
	if kc == nil {
		t.Fatal("NewKeychain() returned nil keychain")
	}
}

func TestMacOSKeychain_Set(t *testing.T) {
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	err := kc.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestMacOSKeychain_Get(t *testing.T) {
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	_ = kc.Set("test-service-get", "test-account-get", "test-password-get")

	password, err := kc.Get("test-service-get", "test-account-get")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if password != "test-password-get" {
		t.Errorf("Get() = %v, want %v", password, "test-password-get")
	}
}

func TestMacOSKeychain_GetNotFound(t *testing.T) {
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	_, err := kc.Get("nonexistent-service", "nonexistent-account")
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func execLookPath(file string) (string, error) {
	return "", nil
}
