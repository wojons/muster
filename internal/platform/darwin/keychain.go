//go:build darwin
// +build darwin

// Package darwin provides macOS-specific platform implementations.
package darwin

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// Keychain provides macOS Keychain Services integration.
type Keychain struct {
	// fields if needed
}

// NewKeychain creates a new macOS Keychain instance.
func NewKeychain() *Keychain {
	return &Keychain{}
}

// Get retrieves a password from the macOS Keychain.
// service: The service name (usually the application name)
// account: The account name (username or identifier)
func (k *Keychain) Get(service, account string) (string, error) {
	if err := k.validateParams(service, account); err != nil {
		return "", err
	}

	cmd := exec.Command("security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "could not be found") {
			return "", fmt.Errorf("keychain item not found: service=%s, account=%s", service, account)
		}
		return "", fmt.Errorf("failed to retrieve from keychain: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Set stores a password in the macOS Keychain.
// service: The service name
// account: The account name
// password: The password to store
func (k *Keychain) Set(service, account, password string) error {
	if err := k.validateParams(service, account); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Check if item exists
	_, err := k.Get(service, account)
	if err == nil {
		// Update existing item
		cmd := exec.Command("security", "add-generic-password",
			"-s", service,
			"-a", account,
			"-w", password,
			"-U") // Update flag

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to update keychain item: %w (output: %s)", err, string(output))
		}
	} else {
		// Create new item
		cmd := exec.Command("security", "add-generic-password",
			"-s", service,
			"-a", account,
			"-w", password,
			"-T", "", // Allow any application to access (optional security setting)
			"-U") // Update if exists

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add keychain item: %w (output: %s)", err, string(output))
		}
	}

	return nil
}

// Delete removes a password from the macOS Keychain.
func (k *Keychain) Delete(service, account string) error {
	if err := k.validateParams(service, account); err != nil {
		return err
	}

	cmd := exec.Command("security", "delete-generic-password",
		"-s", service,
		"-a", account)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "could not be found") {
			return fmt.Errorf("keychain item not found: service=%s, account=%s", service, account)
		}
		return fmt.Errorf("failed to delete keychain item: %w (output: %s)", err, string(output))
	}

	return nil
}

// List returns all accounts for a given service.
func (k *Keychain) List(service string) ([]string, error) {
	if service == "" {
		return nil, fmt.Errorf("service cannot be empty")
	}

	cmd := exec.Command("security", "dump-keychain",
		"-d", // Dump all data
		"-r", // Raw output
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error - dump-keychain may fail if no items exist
		return []string{}, nil
	}

	var accounts []string
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if strings.Contains(line, fmt.Sprintf("svce<blob>=%s", service)) {
			// Look for the preceding "acct" line
			for j := i - 1; j >= max(0, i-5); j-- {
				if strings.Contains(lines[j], "acct<blob>=") {
					acct := strings.TrimPrefix(lines[j], "acct<blob>=")
					acct = strings.Trim(acct, "\"")
					accounts = append(accounts, acct)
					break
				}
			}
		}
	}

	return accounts, nil
}

// GetMasterKey retrieves the master key from the macOS Keychain.
// This is used for encrypting/decrypting other stored credentials.
func (k *Keychain) GetMasterKey() ([]byte, error) {
	password, err := k.Get("muster-master-key", "muster")
	if err != nil {
		return nil, err
	}

	key, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}

	return key, nil
}

// SetMasterKey stores the master key in the macOS Keychain.
func (k *Keychain) SetMasterKey(key []byte) error {
	encoded := base64.StdEncoding.EncodeToString(key)
	return k.Set("muster-master-key", "muster", encoded)
}

// validateParams checks that service and account are valid.
func (k *Keychain) validateParams(service, account string) error {
	if service == "" {
		return fmt.Errorf("service cannot be empty")
	}
	if account == "" {
		return fmt.Errorf("account cannot be empty")
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
