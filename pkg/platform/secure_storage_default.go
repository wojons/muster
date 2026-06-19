//go:build !linux
// +build !linux

package platform

import "fmt"

// LinuxKeychain stubs for non-Linux platforms.
// The real D-Bus implementation is in secure_storage_linux.go.

func (k *LinuxKeychain) Store(service, account, password string) error {
	return fmt.Errorf("Linux keychain requires Linux (GOOS=linux)")
}

func (k *LinuxKeychain) Retrieve(service, account string) (string, error) {
	return "", fmt.Errorf("Linux keychain requires Linux (GOOS=linux)")
}

func (k *LinuxKeychain) Delete(service, account string) error {
	return fmt.Errorf("Linux keychain requires Linux (GOOS=linux)")
}

func (k *LinuxKeychain) List(service string) ([]string, error) {
	return nil, fmt.Errorf("Linux keychain requires Linux (GOOS=linux)")
}
