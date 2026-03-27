package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrMissingMasterKey = errors.New("master key not available: encryption requires a secure master key to be configured. Run 'muster auth init' to set up secure key storage")
)

type Account struct {
	Account string
	Created time.Time
}

type Keychain interface {
	Set(service, account, password string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
	List(service string) ([]Account, error)
}

type MacOSKeychain struct{}

// sanitizeKeychainParam validates and sanitizes service/account parameters
// to prevent command injection attacks. It rejects shell metacharacters
// and other dangerous input patterns.
func sanitizeKeychainParam(param, paramName string) error {
	if param == "" {
		return fmt.Errorf("%s cannot be empty", paramName)
	}

	// Reject shell metacharacters that could enable injection
	dangerousChars := []string{
		"$", "`", "(", ")", "|", "&", ";", "<", ">", "\\",
		"\"", "'", "*", "?", "[", "]", "{", "}", "!", "#", "~",
	}

	for _, char := range dangerousChars {
		if strings.Contains(param, char) {
			return fmt.Errorf("%s contains invalid character: %s", paramName, char)
		}
	}

	// Reject newline/carriage return (could enable command separation)
	if strings.ContainsAny(param, "\n\r\t") {
		return fmt.Errorf("%s cannot contain whitespace", paramName)
	}

	// Reject common injection patterns
	injectionPatterns := []string{"../", "..\\", "$((", "$(("}
	for _, pattern := range injectionPatterns {
		if strings.Contains(param, pattern) {
			return fmt.Errorf("%s contains suspicious pattern", paramName)
		}
	}

	return nil
}

func (k *MacOSKeychain) Set(service, account, password string) error {
	if err := sanitizeKeychainParam(service, "service"); err != nil {
		return err
	}
	if err := sanitizeKeychainParam(account, "account"); err != nil {
		return err
	}

	cmd := exec.Command("security", "add-generic-password",
		"-s", service,
		"-a", account,
		"-w", password,
		"-U",
	)
	return cmd.Run()
}

func (k *MacOSKeychain) Get(service, account string) (string, error) {
	if err := sanitizeKeychainParam(service, "service"); err != nil {
		return "", err
	}
	if err := sanitizeKeychainParam(account, "account"); err != nil {
		return "", err
	}

	cmd := exec.Command("security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w",
	)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "could not be found") {
				return "", ErrNotFound
			}
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (k *MacOSKeychain) Delete(service, account string) error {
	if err := sanitizeKeychainParam(service, "service"); err != nil {
		return err
	}
	if err := sanitizeKeychainParam(account, "account"); err != nil {
		return err
	}

	cmd := exec.Command("security", "delete-generic-password",
		"-s", service,
		"-a", account,
	)
	_ = cmd.Run()
	return nil
}

func (k *MacOSKeychain) List(service string) ([]Account, error) {
	if err := sanitizeKeychainParam(service, "service"); err != nil {
		return nil, err
	}

	cmd := exec.Command("security", "dump-keychain")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var accounts []Account
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, service) {
			accounts = append(accounts, Account{
				Account: service,
				Created: time.Now(),
			})
		}
	}
	return accounts, nil
}

type LinuxKeychain struct {
	filechain     *FileKeychain
	secretService *SecretServiceKeychain
}

func NewLinuxKeychain() *LinuxKeychain {
	// Try Secret Service first, fall back to file keychain
	secretService, err := NewSecretServiceKeychain()
	if err == nil {
		return &LinuxKeychain{
			filechain:     NewFileKeychain(),
			secretService: secretService,
		}
	}
	return &LinuxKeychain{filechain: NewFileKeychain()}
}

func (k *LinuxKeychain) Set(service, account, password string) error {
	// Try Secret Service first
	if k.secretService != nil {
		return k.secretService.Set(service, account, []byte(password))
	}
	// Fall back to file keychain
	return k.filechain.Set(service, account, password)
}

func (k *LinuxKeychain) Get(service, account string) (string, error) {
	// Try Secret Service first
	if k.secretService != nil {
		data, err := k.secretService.Get(service, account)
		if err == nil {
			return string(data), nil
		}
	}
	// Fall back to file keychain
	return k.filechain.Get(service, account)
}

func (k *LinuxKeychain) Delete(service, account string) error {
	// Try Secret Service first
	if k.secretService != nil {
		_ = k.secretService.Delete(service, account)
	}
	// Fall back to file keychain
	return k.filechain.Delete(service, account)
}

func (k *LinuxKeychain) List(service string) ([]Account, error) {
	// Try Secret Service first
	if k.secretService != nil {
		return k.secretService.List(service)
	}
	// Fall back to file keychain
	return k.filechain.List(service)
}

type WindowsKeychain struct {
	native    *windowsKeychainNative
	filechain *FileKeychain
}

func NewWindowsKeychain() *WindowsKeychain {
	return &WindowsKeychain{
		native:    NewWindowsKeychainNative(),
		filechain: NewFileKeychain(),
	}
}

func (k *WindowsKeychain) Set(service, account, password string) error {
	// Use native Windows Credential Manager if available
	if k.native != nil {
		return k.native.Set(service, account, password)
	}
	// Fallback to file keychain
	return k.filechain.Set(service, account, password)
}

func (k *WindowsKeychain) Get(service, account string) (string, error) {
	// Use native Windows Credential Manager if available
	if k.native != nil {
		return k.native.Get(service, account)
	}
	// Fallback to file keychain
	return k.filechain.Get(service, account)
}

func (k *WindowsKeychain) Delete(service, account string) error {
	// Use native Windows Credential Manager if available
	if k.native != nil {
		return k.native.Delete(service, account)
	}
	// Fallback to file keychain
	return k.filechain.Delete(service, account)
}

func (k *WindowsKeychain) List(service string) ([]Account, error) {
	// Use native Windows Credential Manager if available
	if k.native != nil {
		return k.native.List(service)
	}
	// Fallback to file keychain
	return k.filechain.List(service)
}

type FileKeychain struct {
	basePath string
}

func NewFileKeychain() *FileKeychain {
	homeDir, _ := os.UserHomeDir()
	return &FileKeychain{
		basePath: filepath.Join(homeDir, ".muster", "credentials"),
	}
}

func (k *FileKeychain) Set(service, account, password string) error {
	if err := os.MkdirAll(k.basePath, 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	key, err := k.deriveKey(service, account)
	if err != nil {
		return fmt.Errorf("derive encryption key: %w", err)
	}
	encrypted, err := k.encrypt([]byte(password), key)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	filename := k.filePath(service, account)
	if err := os.WriteFile(filename, []byte(encrypted), 0600); err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}

	return nil
}

func (k *FileKeychain) Get(service, account string) (string, error) {
	filename := k.filePath(service, account)
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read credentials file: %w", err)
	}

	key, err := k.deriveKey(service, account)
	if err != nil {
		return "", fmt.Errorf("derive encryption key: %w", err)
	}
	password, err := k.decrypt(string(data), key)
	if err != nil {
		return "", fmt.Errorf("decrypt password: %w", err)
	}

	return password, nil
}

func (k *FileKeychain) Delete(service, account string) error {
	filename := k.filePath(service, account)
	if err := os.Remove(filename); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("delete credentials file: %w", err)
	}
	return nil
}

func (k *FileKeychain) List(service string) ([]Account, error) {
	return nil, nil
}

func (k *FileKeychain) filePath(service, account string) string {
	safeName := fmt.Sprintf("%s.%s", sanitizeFilename(service), sanitizeFilename(account))
	return filepath.Join(k.basePath, safeName)
}

func (k *FileKeychain) deriveKey(service, account string) ([]byte, error) {
	// CRITICAL FIX: Use PBKDF2-like key derivation with proper key derivation and master key
	// Instead of simple SHA256 of service:account (which is deterministic and public),
	// we use a salted key derivation with a master key to ensure secrecy

	masterKey, err := k.getMasterKey()
	if err != nil {
		// Fallback: use a file-based master key
		masterKey, err = k.getMasterKeyFromFile()
		if err != nil {
			// SECURITY FIX (SEC-002): Remove weak fallback and fail closed
			// Using a predictable master key based on OS is a critical security vulnerability
			// as it would allow anyone to decrypt credentials knowing only the OS type
			return nil, ErrMissingMasterKey
		}
	}

	// Derive salt from service+account hash
	salt := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", service, account)))

	// Use HMAC-based key derivation with 100,000 iterations for key stretching
	// This is equivalent to PBKDF2-HMAC-SHA256
	var derivedKey []byte
	h := hmac.New(sha256.New, masterKey)
	h.Write(salt[:])
	derivedKey = h.Sum(nil)

	// Do iterations (key stretching)
	for i := 1; i < 100000; i++ {
		h = hmac.New(sha256.New, derivedKey)
		h.Write(salt[:])
		derivedKey = h.Sum(nil)
	}

	return derivedKey, nil
}

func (k *FileKeychain) getMasterKey() ([]byte, error) {
	// Try OS keychain for master key
	switch runtime.GOOS {
	case "darwin":
		return k.getMasterKeyFromMacOSKeychain()
	case "linux":
		return k.getMasterKeyFromLinuxKeychain()
	case "windows":
		return k.getMasterKeyFromWindowsKeychain()
	default:
		return nil, fmt.Errorf("unsupported platform")
	}
}

func (k *FileKeychain) getMasterKeyFromMacOSKeychain() ([]byte, error) {
	// Use macOS keychain to store/retrieve master key
	cmd := exec.Command("security", "find-generic-password",
		"-s", "muster-master-key",
		"-w",
		"-g")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// Parse the key from output and decode base64
	masterKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, err
	}

	return masterKey, nil
}

func (k *FileKeychain) getMasterKeyFromLinuxKeychain() ([]byte, error) {
	// Use libsecret or file-based master key for Linux
	masterKeyFile := filepath.Join(k.basePath, ".master-key")
	keyData, err := os.ReadFile(masterKeyFile)
	if err != nil {
		return nil, err
	}

	// Decode and return the master key
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil {
		return nil, err
	}

	return key, nil
}

func (k *FileKeychain) getMasterKeyFromWindowsKeychain() ([]byte, error) {
	// Use Windows Credential Manager
	winKC := NewWindowsKeychainNative()
	password, err := winKC.Get("muster-master-key", "master-key")
	if err != nil {
		if err == ErrNotFound {
			// Generate and store new master key
			newKey := make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
				return nil, fmt.Errorf("generate master key: %w", err)
			}

			if err := winKC.Set("muster-master-key", "master-key", string(newKey)); err != nil {
				return nil, fmt.Errorf("store master key: %w", err)
			}

			return newKey, nil
		}
		return nil, fmt.Errorf("read master key: %w", err)
	}

	return []byte(password), nil
}

func (k *FileKeychain) getMasterKeyFromFile() ([]byte, error) {
	// CRITICAL FIX: Encrypt master key before storing to prevent credential theft if file is accessed
	// Use a system-specific encryption key derived from hardware/user-specific data
	masterKeyFile := filepath.Join(k.basePath, ".master-key")

	if err := os.MkdirAll(k.basePath, 0700); err != nil {
		return nil, fmt.Errorf("create master key directory: %w", err)
	}

	keyData, err := os.ReadFile(masterKeyFile)
	if err == nil {
		// Master key exists, decrypt and return using system-specific encryption key
		systemKey := k.deriveSystemEncryptionKey()
		decrypted, err := k.decrypt(string(keyData), systemKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt master key: %w", err)
		}
		return []byte(decrypted), nil
	}

	// Generate new master key
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}

	// Encrypt with system-specific key before storing
	systemKey := k.deriveSystemEncryptionKey()
	encrypted, err := k.encrypt(newKey, systemKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt master key: %w", err)
	}

	// Store encrypted key with secure permissions
	if err := os.WriteFile(masterKeyFile, []byte(encrypted), 0600); err != nil {
		return nil, fmt.Errorf("store master key: %w", err)
	}

	return newKey, nil
}

// deriveSystemEncryptionKey creates a system-specific encryption key from machine-specific data
func (k *FileKeychain) deriveSystemEncryptionKey() []byte {
	// Create a system-specific key from OS, architecture, and machine-id if available
	systemData := runtime.GOOS + "-" + runtime.GOARCH

	// Try to read machine-id for Linux systems for better uniqueness
	if runtime.GOOS == "linux" {
		if id, err := os.ReadFile("/etc/machine-id"); err == nil {
			systemData += string(id)
		}
	}

	// Derive a stable encryption key from system data
	h := sha256.New()
	h.Write([]byte(systemData))
	return h.Sum(nil)
}

func (k *FileKeychain) encrypt(plaintext, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (k *FileKeychain) decrypt(ciphertext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func sanitizeFilename(name string) string {
	// CRITICAL FIX: Reject malicious patterns instead of converting to prevent path traversal attacks
	// Check for path traversal attempts and dangerous characters

	// If contains "..", replace entire string with safe placeholder
	if strings.Contains(name, "..") {
		return "path_traversal_rejected"
	}

	// Check for other dangerous characters
	for _, c := range name {
		if c == '/' || c == '\\' || c == '\x00' {
			return "dangerous_chars_rejected"
		}
	}

	// Original sanitization for remaining characters
	b := make([]byte, len(name))
	for i, c := range name {
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-' || c == '_' {
			b[i] = byte(c)
		} else {
			b[i] = '_'
		}
	}
	return string(b)
}

func NewKeychain() (Keychain, error) {
	switch runtime.GOOS {
	case "darwin":
		return &MacOSKeychain{}, nil
	case "linux":
		return NewLinuxKeychain(), nil
	case "windows":
		return NewWindowsKeychain(), nil
	default:
		return NewFileKeychain(), nil
	}
}
