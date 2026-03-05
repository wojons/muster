package platform

import (
	"os/exec"
	"runtime"
	"strings"
)

type SecureStorage interface {
	Store(service, account, password string) error
	Retrieve(service, account string) (string, error)
	Delete(service, account string) error
	List(service string) ([]string, error)
}

type MacOSKeychain struct{}

func (k *MacOSKeychain) Store(service, account, password string) error {
	cmd := exec.Command("security", "add-generic-password",
		"-s", service,
		"-a", account,
		"-w", password,
		"-U",
	)
	return cmd.Run()
}

func (k *MacOSKeychain) Retrieve(service, account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w",
	)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "could not be found") {
				return "", nil
			}
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (k *MacOSKeychain) Delete(service, account string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", service,
		"-a", account,
	)
	_ = cmd.Run()
	return nil
}

func (k *MacOSKeychain) List(service string) ([]string, error) {
	cmd := exec.Command("security", "dump-keychain")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var accounts []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, service) {
			accounts = append(accounts, service)
		}
	}
	return accounts, nil
}

type LinuxKeychain struct{}

func (k *LinuxKeychain) Store(service, account, password string) error {
	return nil
}

func (k *LinuxKeychain) Retrieve(service, account string) (string, error) {
	return "", nil
}

func (k *LinuxKeychain) Delete(service, account string) error {
	return nil
}

func (k *LinuxKeychain) List(service string) ([]string, error) {
	return nil, nil
}

type WindowsKeychain struct{}

func (k *WindowsKeychain) Store(service, account, password string) error {
	return nil
}

func (k *WindowsKeychain) Retrieve(service, account string) (string, error) {
	return "", nil
}

func (k *WindowsKeychain) Delete(service, account string) error {
	return nil
}

func (k *WindowsKeychain) List(service string) ([]string, error) {
	return nil, nil
}

func DefaultSecureStorage() SecureStorage {
	switch {
	case isDarwin():
		return &MacOSKeychain{}
	case isLinux():
		return &LinuxKeychain{}
	case isWindows():
		return &WindowsKeychain{}
	default:
		return &MacOSKeychain{}
	}
}

func isDarwin() bool {
	return runtime.GOOS == "darwin"
}

func isLinux() bool {
	return runtime.GOOS == "linux"
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}
