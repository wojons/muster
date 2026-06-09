//go:build linux
// +build linux

package platform

import (
	"context"
	"fmt"
	"sync"

	linuxkr "github.com/wojons/muster/internal/platform/linux"
)

// LinuxKeychainStore implements SecureStorage for Linux using the Secret Service
// D-Bus API (org.freedesktop.secrets), which backs GNOME Keyring, KDE Wallet,
// and other libsecret-compatible key managers.
//
// Connection to the D-Bus session bus is lazy: the first call to any method
// initializes the keyring. Errors are surfaced on first use.
//
// axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/LINUX-IMPLEMENTATION.md#2-secret-service plan=phase-3/task-3-1/step-3-1-1,phase-3/task-3-1/step-3-1-2 impl=pkg/platform/secure_storage_linux.go
var (
	linuxKeyring     *linuxkr.SecretServiceKeyring
	linuxKeyringOnce sync.Once
	linuxKeyringErr  error
)

// getLinuxKeyring returns the cached SecretServiceKeyring, initializing it
// on the first call. Subsequent calls return the same instance (or error).
func getLinuxKeyring() (*linuxkr.SecretServiceKeyring, error) {
	linuxKeyringOnce.Do(func() {
		linuxKeyring, linuxKeyringErr = linuxkr.NewSecretServiceKeyring()
	})
	if linuxKeyringErr != nil {
		return nil, fmt.Errorf("Linux keychain unavailable: %w", linuxKeyringErr)
	}
	return linuxKeyring, nil
}

// Store saves a password in the Linux keyring.
// The service and account are stored as D-Bus attributes for retrieval.
func (k *LinuxKeychain) Store(service, account, password string) error {
	kr, err := getLinuxKeyring()
	if err != nil {
		return err
	}

	// Key is composed as "service/account" for unique identification
	cred := &linuxkr.Credential{
		Type:     "generic-password",
		Value:    password,
		Username: account,
	}

	return kr.Set(context.Background(), service+"/"+account, cred)
}

// Retrieve fetches a password from the Linux keyring by service and account.
func (k *LinuxKeychain) Retrieve(service, account string) (string, error) {
	kr, err := getLinuxKeyring()
	if err != nil {
		return "", err
	}

	cred, err := kr.Get(context.Background(), service+"/"+account)
	if err != nil {
		if err == linuxkr.ErrCredentialNotFound {
			return "", nil
		}
		return "", err
	}

	return cred.Value, nil
}

// Delete removes a credential from the Linux keyring.
func (k *LinuxKeychain) Delete(service, account string) error {
	kr, err := getLinuxKeyring()
	if err != nil {
		return err
	}

	err = kr.Delete(context.Background(), service+"/"+account)
	if err != nil && err == linuxkr.ErrCredentialNotFound {
		return nil // idempotent: already gone
	}
	return err
}

// List returns all account names stored under the given service name.
func (k *LinuxKeychain) List(service string) ([]string, error) {
	_, err := getLinuxKeyring()
	if err != nil {
		return nil, err
	}

	// The SecretServiceKeyring doesn't have a List method directly.
	// A full implementation would use SearchItems on the collection
	// with a wildcard attribute pattern, but the Secret Service API
	// doesn't support prefix matching natively. This is partially
	// addressed in Phase 4 integration tests.
	return nil, fmt.Errorf("List not supported by Linux keychain: %w", linuxkr.ErrCredentialNotFound)
}

// Close releases the keyring connection. Safe to call multiple times.
func CloseLinuxKeychain() error {
	if linuxKeyring != nil {
		return linuxKeyring.Disconnect()
	}
	return nil
}
