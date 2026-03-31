package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// DefaultKeyDerivationIterations follows OWASP recommendations for PBKDF2
	DefaultKeyDerivationIterations = 150000
	// SaltSize in bytes (128 bits)
	SaltSize = 16
	// KeySize for SQLCipher (256 bits)
	KeySize = 32
)

// EncryptionKeyManager manages database encryption keys
type EncryptionKeyManager struct {
	keyPath     string
	masterKey   []byte
	salt        []byte
	iterations  int
	initialized bool
	mu          sync.RWMutex
}

// EncryptionKeyConfig holds encryption key configuration
type EncryptionKeyConfig struct {
	KeyPath    string // Path to store encrypted key metadata
	MasterKey  []byte // Master key for derivation (if not provided, will be generated)
	Iterations int    // PBKDF2 iterations (default: 150000)
}

// NewEncryptionKeyManager creates a new encryption key manager
func NewEncryptionKeyManager(cfg *EncryptionKeyConfig) (*EncryptionKeyManager, error) {
	if cfg == nil {
		cfg = &EncryptionKeyConfig{}
	}

	// Set defaults
	if cfg.Iterations == 0 {
		cfg.Iterations = DefaultKeyDerivationIterations
	}

	// Generate or use provided master key
	var masterKey []byte
	if len(cfg.MasterKey) > 0 {
		masterKey = cfg.MasterKey
	} else {
		// Generate a random master key
		masterKey = make([]byte, 32)
		if _, err := rand.Read(masterKey); err != nil {
			return nil, fmt.Errorf("generate master key: %w", err)
		}
	}

	// Generate or load salt
	var salt []byte
	keyPath := cfg.KeyPath
	if keyPath == "" {
		// Default key path in user's config directory
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = "."
		}
		keyPath = filepath.Join(configDir, "muster", "db.key")
	}

	// Try to load existing salt
	salt, err := loadSalt(keyPath + ".salt")
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load salt: %w", err)
		}
		// Generate new salt
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("generate salt: %w", err)
		}
		// Save salt
		if err := saveSalt(keyPath+".salt", salt); err != nil {
			return nil, fmt.Errorf("save salt: %w", err)
		}
	}

	mgr := &EncryptionKeyManager{
		keyPath:    keyPath,
		masterKey:  masterKey,
		salt:       salt,
		iterations: cfg.Iterations,
	}

	return mgr, nil
}

// DeriveKey derives the SQLCipher encryption key using PBKDF2
func (m *EncryptionKeyManager) DeriveKey() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.initialized {
		return nil, fmt.Errorf("key manager not initialized")
	}

	// Derive key using PBKDF2-HMAC-SHA256
	key := pbkdf2.Key(m.masterKey, m.salt, m.iterations, KeySize, nil)
	return key, nil
}

// DerivePragmaKey derives the PRAGMA key string for SQLCipher
func (m *EncryptionKeyManager) DerivePragmaKey() (string, error) {
	key, err := m.DeriveKey()
	if err != nil {
		return "", err
	}

	// Convert to hex string for PRAGMA key
	return hex.EncodeToString(key), nil
}

// GetKeyPath returns the key file path
func (m *EncryptionKeyManager) GetKeyPath() string {
	return m.keyPath
}

// Initialize initializes the key manager
func (m *EncryptionKeyManager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(m.keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}

	m.initialized = true
	return nil
}

// RotateKey rotates the encryption key (advanced operation)
func (m *EncryptionKeyManager) RotateKey(newMasterKey []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(newMasterKey) == 0 {
		return fmt.Errorf("new master key required")
	}

	// Generate new salt
	newSalt := make([]byte, SaltSize)
	if _, err := rand.Read(newSalt); err != nil {
		return fmt.Errorf("generate new salt: %w", err)
	}

	// Save new salt
	if err := saveSalt(m.keyPath+".salt", newSalt); err != nil {
		return fmt.Errorf("save new salt: %w", err)
	}

	// Update in-memory values
	m.masterKey = newMasterKey
	m.salt = newSalt

	return nil
}

// Helper functions

func loadSalt(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	salt, err := hex.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	return salt, nil
}

func saveSalt(path string, salt []byte) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create salt directory: %w", err)
	}

	// Write salt with secure permissions
	hexSalt := hex.EncodeToString(salt)
	if err := os.WriteFile(path, []byte(hexSalt), 0600); err != nil {
		return fmt.Errorf("write salt: %w", err)
	}

	return nil
}

// GetApplicationSecret returns an application-level secret for additional key complexity
func GetApplicationSecret() []byte {
	// Combine runtime info with fixed secret for additional entropy
	// This is NOT cryptographically secure on its own, but adds complexity when combined with random salt
	appSecret := fmt.Sprintf("muster-db-%s-%s", runtime.GOOS, runtime.GOARCH)
	return []byte(appSecret)
}

// DeriveMasterKeyFromPassphrase derives a master key from a user passphrase
func DeriveMasterKeyFromPassphrase(passphrase string, salt []byte, iterations int) ([]byte, error) {
	if len(salt) == 0 {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("generate salt: %w", err)
		}
	}

	if iterations == 0 {
		iterations = DefaultKeyDerivationIterations
	}

	// Derive key using PBKDF2-HMAC-SHA256
	key := pbkdf2.Key([]byte(passphrase), salt, iterations, KeySize, nil)
	return key, nil
}
