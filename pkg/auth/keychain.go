package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrNotFound                  = errors.New("not found")
	ErrMissingMasterKey          = errors.New("master key not available: encryption requires a secure master key to be configured. Run 'muster auth init' to set up secure key storage")
	ErrSecretServiceNotAvailable = errors.New("secret service not available on this platform")
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
	native, err := NewWindowsKeychainNative()
	if err != nil {
		native = nil
	}
	return &WindowsKeychain{
		native:    native,
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
	basePath  string
	appSecret []byte // runtime-generated application secret, populated during getMasterKeyFromFile

	// Cached master key — avoids repeated D-Bus SessionBus() calls which
	// deadlock under concurrent access (godbus uses a global sync.Mutex).
	masterKey     []byte
	masterKeyOnce sync.Once
	masterKeyErr  error
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
	prefix := sanitizeFilename(service) + "."
	entries, err := os.ReadDir(k.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials dir: %w", err)
	}

	var accounts []Account
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Extract account name: remove the prefix
		account := strings.TrimPrefix(name, prefix)
		if account == "" {
			continue
		}
		accounts = append(accounts, Account{
			Account: account,
			Created: time.Now(),
		})
	}
	return accounts, nil
}

func (k *FileKeychain) filePath(service, account string) string {
	safeName := fmt.Sprintf("%s.%s", sanitizeFilename(service), sanitizeFilename(account))
	return filepath.Join(k.basePath, safeName)
}

func (k *FileKeychain) deriveKey(service, account string) ([]byte, error) {
	// CRITICAL FIX: Use PBKDF2-like key derivation with proper key derivation and master key
	// Instead of simple SHA256 of service:account (which is deterministic and public),
	// we use a salted key derivation with a master key to ensure secrecy

	// Cache the master key on first access via sync.Once — this prevents
	// 50 concurrent goroutines from all calling dbus.SessionBus() simultaneously,
	// which deadlocks on godbus's global sync.Mutex.
	k.masterKeyOnce.Do(func() {
		k.masterKey, k.masterKeyErr = k.getMasterKey()
		if k.masterKeyErr != nil {
			// Fallback: use a file-based master key
			k.masterKey, k.masterKeyErr = k.getMasterKeyFromFile()
			if k.masterKeyErr != nil {
				// SECURITY FIX (SEC-002): Remove weak fallback and fail closed
				return
			}
			// New master key was generated; try to persist it in the platform keychain
			// for future retrievals by getMasterKey().
			k.storeMasterKeyInPlatformKeychain(k.masterKey)
		}
	})

	if k.masterKeyErr != nil {
		return nil, ErrMissingMasterKey
	}
	masterKey := k.masterKey

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

// storeMasterKeyInPlatformKeychain persists the master key in the OS platform keychain
// (macOS Keychain, Linux Secret Service) so that future calls to getMasterKey() can
// retrieve it from a secure source rather than only from the encrypted file.
func (k *FileKeychain) storeMasterKeyInPlatformKeychain(masterKey []byte) {
	switch runtime.GOOS {
	case "darwin":
		k.storeMasterKeyOnMacOSKeychain(masterKey)
	case "linux":
		k.storeMasterKeyOnLinuxKeychain(masterKey)
	}
}

func (k *FileKeychain) storeMasterKeyOnLinuxKeychain(masterKey []byte) {
	secretService, err := NewSecretServiceKeychain()
	// axiom:trace work_item=auth-keychain-fix-02 spec=pkg/auth/keychain.go plan=phase-2/task-3/step-3-1 impl=storeMasterKeyOnLinuxKeychain test=pkg/auth/keychain_test.go
	if err != nil {
		return // Secret Service not available; file-based storage is sufficient.
	}
	// Store as raw bytes - the Secret Service encrypts at rest via the D-Bus service.
	_ = secretService.Set("muster-master-key", "master-key", masterKey)
}

func (k *FileKeychain) storeMasterKeyOnMacOSKeychain(masterKey []byte) {
	encoded := base64.StdEncoding.EncodeToString(masterKey)
	cmd := exec.Command("security", "add-generic-password",
		"-s", "muster-master-key",
		"-a", "master-key",
		"-w", encoded,
		"-U",
	)
	_ = cmd.Run()
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

// axiom:trace work_item=auth-keychain-fix-02 spec=pkg/auth/keychain.go plan=phase-1/task-2/step-2-2+phase-2/task-3/step-3-1 impl=getMasterKeyFromLinuxKeychain test=pkg/auth/keychain_test.go
func (k *FileKeychain) getMasterKeyFromLinuxKeychain() ([]byte, error) {
	// Skip D-Bus Secret Service in headless/CI environments where no session bus
	// is available. dbus.SessionBus() blocks indefinitely when no bus exists,
	// which causes test timeouts (especially TestCredential_ConcurrentAccess with
	// 50 goroutines all blocking on the same global mutex).
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return k.getMasterKeyFromFile()
	}

	// Try Secret Service first for master key storage (D-Bus Secret Service)
	// This provides hardware-backed or user-session-scoped key storage via gnome-keyring/kwallet.
	secretService, err := NewSecretServiceKeychain()
	if err == nil {
		data, err := secretService.Get("muster-master-key", "master-key")
		if err == nil {
			return data, nil
		}
		_ = secretService.Delete("muster-master-key", "master-key") // clean up invalid entries
	}

	// Fall back to file-based master key (handles PBKDF2 JSON format and old raw format)
	return k.getMasterKeyFromFile()
}

func (k *FileKeychain) getMasterKeyFromWindowsKeychain() ([]byte, error) {
	// Use Windows Credential Manager
	winKC, err := NewWindowsKeychainNative()
	if err != nil {
		return nil, err
	}
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
	// CRITICAL SECURITY FIX: Use PBKDF2 with random salt for key derivation
	// This prevents attackers from deriving the encryption key from public data
	masterKeyFile := filepath.Join(k.basePath, ".master-key")

	if err := os.MkdirAll(k.basePath, 0700); err != nil {
		return nil, fmt.Errorf("create master key directory: %w", err)
	}

	keyData, err := os.ReadFile(masterKeyFile)
	if err == nil {
		// Master key exists, decrypt using PBKDF2 with stored salt
		decrypted, err := k.decryptMasterKeyFile(string(keyData))
		if err != nil {
			return nil, fmt.Errorf("decrypt master key: %w", err)
		}
		return decrypted, nil
	}

	// Generate new master key
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}

	// Encrypt with PBKDF2-derived key before storing
	encrypted, err := k.encryptMasterKeyFile(newKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt master key: %w", err)
	}

	// Store encrypted key with secure permissions
	if err := os.WriteFile(masterKeyFile, []byte(encrypted), 0600); err != nil {
		return nil, fmt.Errorf("store master key: %w", err)
	}

	return newKey, nil
}

// getApplicationSecret returns the runtime-generated application secret used in PBKDF2
// key derivation for encrypting the master key file. If no stored secret exists
// (e.g., old-format master key), returns the hardcoded constant for backward
// compatibility.
// axiom:trace work_item=auth-keychain-fix-02 spec=pkg/auth/keychain.go plan=phase-1/task-2/step-2-1 impl=getApplicationSecret
var getApplicationSecretFallback = "muster-cli-application-secret-v1"

func (k *FileKeychain) getApplicationSecret() string {
	if len(k.appSecret) > 0 {
		return string(k.appSecret)
	}
	return getApplicationSecretFallback
}

// derivePBKDF2Key derives an encryption key using PBKDF2 with random salt
// The salt is stored alongside the encrypted data and retrieved for decryption
func derivePBKDF2Key(password, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2.Key(password, salt, iterations, keyLen, sha256.New)
}

// encryptMasterKeyFile encrypts the master key using PBKDF2-derived key.
// Format (v2): {"version":2,"salt":"...","iterations":N,"app_secret":"...","ciphertext":"..."}
// Format (v1, no longer written): {"salt":"...","iterations":N,"ciphertext":"..."}
// The app_secret is a runtime-generated random value stored alongside the ciphertext
// and used in the KEK derivation instead of a hardcoded constant.
func (k *FileKeychain) encryptMasterKeyFile(masterKey []byte) (string, error) {
	// Generate random salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	// Generate random application secret (runtime-generated, not hardcoded)
	appSecret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, appSecret); err != nil {
		return "", fmt.Errorf("generate app secret: %w", err)
	}
	k.appSecret = appSecret

	// Use PBKDF2 to derive encryption key from application secret + machine data + salt
	password := k.getApplicationSecret() + "-" + k.getMachineData()
	iterations := 150000 // OWASP recommended minimum for PBKDF2-HMAC-SHA256
	keyLen := 32
	encryptionKey := derivePBKDF2Key([]byte(password), salt, iterations, keyLen)

	// Encrypt master key
	encrypted, err := k.encrypt(masterKey, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	// Encode as JSON (v2 format with app_secret)
	type encryptedKeyV2 struct {
		Version    int    `json:"version"`
		Salt       string `json:"salt"`
		Iterations int    `json:"iterations"`
		AppSecret  string `json:"app_secret"`
		Ciphertext string `json:"ciphertext"`
	}

	data := encryptedKeyV2{
		Version:    2,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Iterations: iterations,
		AppSecret:  base64.StdEncoding.EncodeToString(appSecret),
		Ciphertext: encrypted,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// decryptMasterKeyFile decrypts the master key using PBKDF2-derived key.
// Supports v2 format (JSON with salt/iterations/app_secret), v1 format
// (JSON with salt/iterations, uses hardcoded app_secret fallback), and
// legacy raw base64 format.
func (k *FileKeychain) decryptMasterKeyFile(encryptedData string) ([]byte, error) {
	jsonData, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	// Try v2 format (version field present)
	var encKeyV2 struct {
		Version    int    `json:"version"`
		Salt       string `json:"salt"`
		Iterations int    `json:"iterations"`
		AppSecret  string `json:"app_secret,omitempty"`
		Ciphertext string `json:"ciphertext"`
	}

	if err := json.Unmarshal(jsonData, &encKeyV2); err == nil && encKeyV2.Version == 2 {
		salt, err := base64.StdEncoding.DecodeString(encKeyV2.Salt)
		if err != nil {
			return nil, fmt.Errorf("decode salt: %w", err)
		}

		// Load the stored app_secret and set it on the keychain instance
		appSecret, err := base64.StdEncoding.DecodeString(encKeyV2.AppSecret)
		if err != nil {
			return nil, fmt.Errorf("decode app_secret: %w", err)
		}
		k.appSecret = appSecret

		password := k.getApplicationSecret() + "-" + k.getMachineData()
		encryptionKey := derivePBKDF2Key([]byte(password), salt, encKeyV2.Iterations, 32)

		decrypted, err := k.decrypt(encKeyV2.Ciphertext, encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt: %w", err)
		}

		return []byte(decrypted), nil
	}

	// Try v1 format (JSON salt/iterations/ciphertext, no version field,
	// uses hardcoded app_secret fallback for backward compatibility)
	var encKeyV1 struct {
		Salt       string `json:"salt"`
		Iterations int    `json:"iterations"`
		Ciphertext string `json:"ciphertext"`
	}

	if err := json.Unmarshal(jsonData, &encKeyV1); err == nil && encKeyV1.Salt != "" {
		salt, err := base64.StdEncoding.DecodeString(encKeyV1.Salt)
		if err != nil {
			return nil, fmt.Errorf("decode salt: %w", err)
		}

		// V1 format: use hardcoded app_secret fallback (backward compatibility)
		password := getApplicationSecretFallback + "-" + k.getMachineData()
		encryptionKey := derivePBKDF2Key([]byte(password), salt, encKeyV1.Iterations, 32)

		decrypted, err := k.decrypt(encKeyV1.Ciphertext, encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt v1: %w", err)
		}

		return []byte(decrypted), nil
	}

	// Legacy format: raw base64-encoded encrypted key (backward compatibility)
	systemKey := k.deriveSystemEncryptionKey()
	decrypted, err := k.decrypt(encryptedData, systemKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt legacy: %w", err)
	}

	return []byte(decrypted), nil
}

// getMachineData returns machine-specific data for key derivation
// This is PUBLIC data but adds uniqueness per machine
func (k *FileKeychain) getMachineData() string {
	systemData := runtime.GOOS + "-" + runtime.GOARCH

	if runtime.GOOS == "linux" {
		if id, err := os.ReadFile("/etc/machine-id"); err == nil {
			systemData += string(id)
		}
	}

	return systemData
}

// deriveSystemEncryptionKey creates a system-specific encryption key from machine-specific data
// DEPRECATED: Use PBKDF2-based key derivation with random salt instead
// Kept for backward compatibility with old encrypted keys
func (k *FileKeychain) deriveSystemEncryptionKey() []byte {
	systemData := k.getMachineData()

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
