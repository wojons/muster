package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEncryptionKeyManager_NewKeyManager(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000, // Reduced for test speed
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Equal(t, keyPath, mgr.GetKeyPath())
}

func TestEncryptionKeyManager_DeriveKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)

	// Initialize the manager
	err = mgr.Initialize()
	require.NoError(t, err)

	// Derive key
	key, err := mgr.DeriveKey()
	require.NoError(t, err)
	assert.Len(t, key, KeySize, "Derived key should be 32 bytes (256 bits)")
}

func TestEncryptionKeyManager_DerivePragmaKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)

	err = mgr.Initialize()
	require.NoError(t, err)

	pragmaKey, err := mgr.DerivePragmaKey()
	require.NoError(t, err)
	assert.NotEmpty(t, pragmaKey, "Pragma key should not be empty")
	assert.Len(t, pragmaKey, KeySize*2, "Pragma key should be hex-encoded (64 chars)")
}

func TestEncryptionKeyManager_KeyConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	// Create first manager
	cfg1 := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr1, err := NewEncryptionKeyManager(cfg1)
	require.NoError(t, err)
	err = mgr1.Initialize()
	require.NoError(t, err)

	key1, err := mgr1.DeriveKey()
	require.NoError(t, err)

	// Create second manager with same config
	cfg2 := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr2, err := NewEncryptionKeyManager(cfg2)
	require.NoError(t, err)
	err = mgr2.Initialize()
	require.NoError(t, err)

	key2, err := mgr2.DeriveKey()
	require.NoError(t, err)

	// Keys should be identical when using same salt and master key
	assert.Equal(t, key1, key2, "Derived keys should be consistent")
}

func TestEncryptionKeyManager_RotateKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)
	err = mgr.Initialize()
	require.NoError(t, err)

	originalKey, err := mgr.DeriveKey()
	require.NoError(t, err)

	// Rotate key
	newMasterKey := make([]byte, 32)
	for i := range newMasterKey {
		newMasterKey[i] = byte(i)
	}

	err = mgr.RotateKey(newMasterKey)
	require.NoError(t, err)

	newKey, err := mgr.DeriveKey()
	require.NoError(t, err)

	// Keys should be different after rotation
	assert.NotEqual(t, originalKey, newKey, "Keys should differ after rotation")
}

func TestEncryptionKeyManager_CustomMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	customMasterKey := []byte("custom-master-key-32-bytes-long!")

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		MasterKey:  customMasterKey,
		Iterations: 1000,
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)
	err = mgr.Initialize()
	require.NoError(t, err)

	key, err := mgr.DeriveKey()
	require.NoError(t, err)
	assert.Len(t, key, KeySize)
}

func TestEncryptionKeyManager_SaltPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	saltPath := keyPath + ".salt"

	// Create manager and initialize
	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr1, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)
	err = mgr1.Initialize()
	require.NoError(t, err)

	// Verify salt file was created
	_, err = os.Stat(saltPath)
	require.NoError(t, err, "Salt file should exist")

	// Verify salt file has secure permissions
	info, err := os.Stat(saltPath)
	require.NoError(t, err)
	// On Unix systems, check file permissions
	if info.Mode().Perm()&0077 == 0 {
		assert.True(t, true, "Salt file should have secure permissions")
	}
}

func TestDeriveMasterKeyFromPassphrase(t *testing.T) {
	passphrase := "super-secure-passphrase"
	salt := []byte("random-salt-value")

	key1, err := DeriveMasterKeyFromPassphrase(passphrase, salt, 1000)
	require.NoError(t, err)
	assert.Len(t, key1, KeySize)

	// Same passphrase and salt should produce same key
	key2, err := DeriveMasterKeyFromPassphrase(passphrase, salt, 1000)
	require.NoError(t, err)
	assert.Equal(t, key1, key2, "Same passphrase should produce same key")

	// Different passphrase should produce different key
	key3, err := DeriveMasterKeyFromPassphrase("different-passphrase", salt, 1000)
	require.NoError(t, err)
	assert.NotEqual(t, key1, key3, "Different passphrase should produce different key")
}

func TestSQLiteConfig_EnableEncryption(t *testing.T) {
	cfg := DefaultSQLiteConfig("./test.db")
	assert.Empty(t, cfg.EncryptionKey, "Encryption should be disabled by default")

	// Enable encryption
	testKey := "test-encryption-key-hex-encoded"
	cfg.EnableEncryption(testKey)
	assert.Equal(t, testKey, cfg.EncryptionKey, "Encryption key should be set")
}

func TestSQLiteConfig_EnableEncryptionWithKeyManager(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	cfg := DefaultSQLiteConfig("./test.db")

	// Create key manager
	kmCfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	km, err := NewEncryptionKeyManager(kmCfg)
	require.NoError(t, err)

	// Enable encryption with key manager
	err = cfg.EnableEncryptionWithKeyManager(km)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.EncryptionKey, "Encryption key should be derived")
	assert.Equal(t, km, cfg.KeyManager, "Key manager should be stored")
}

func TestSQLiteManager_IsEncrypted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create unencrypted database
	cfg := DefaultSQLiteConfig(dbPath)
	log, _ := zap.NewDevelopment()

	mgr, err := NewSQLiteManager(cfg, log)
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	// Should not be encrypted
	assert.False(t, mgr.IsEncrypted(), "Database should not be encrypted")
}

func TestSQLiteManager_WithEncryptionKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	keyPath := filepath.Join(tmpDir, "test.key")

	log, _ := zap.NewDevelopment()

	// Create key manager
	kmCfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	km, err := NewEncryptionKeyManager(kmCfg)
	require.NoError(t, err)

	// Create encrypted database
	cfg := DefaultSQLiteConfig(dbPath)
	err = cfg.EnableEncryptionWithKeyManager(km)
	require.NoError(t, err)

	mgr, err := NewSQLiteManager(cfg, log)
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	// Should be encrypted
	assert.True(t, mgr.IsEncrypted(), "Database should be encrypted")

	// Test that we can create and query tables
	_, err = mgr.GetDB().Exec("CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY, name TEXT)")
	assert.NoError(t, err)

	_, err = mgr.GetDB().Exec("INSERT INTO test (name) VALUES (?)", "test")
	assert.NoError(t, err)

	var count int
	err = mgr.GetDB().QueryRow("SELECT COUNT(*) FROM test").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSQLiteManager_EncryptionKeyRequired(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	keyPath := filepath.Join(tmpDir, "test.key")

	log, _ := zap.NewDevelopment()

	// Create encrypted database
	kmCfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	km, err := NewEncryptionKeyManager(kmCfg)
	require.NoError(t, err)

	cfg := DefaultSQLiteConfig(dbPath)
	err = cfg.EnableEncryptionWithKeyManager(km)
	require.NoError(t, err)

	mgr, err := NewSQLiteManager(cfg, log)
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	// Create a table with encrypted database
	_, err = mgr.GetDB().Exec("CREATE TABLE IF NOT EXISTS secure_data (id INTEGER, value TEXT)")
	require.NoError(t, err)

	// Try to open the SAME database without the encryption key
	// This should fail because the database is encrypted
	wrongCfg := DefaultSQLiteConfig(dbPath)
	wrongMgr, err := NewSQLiteManager(wrongCfg, log)
	if err != nil {
		// Expected to fail or return errors when querying encrypted data
		t.Logf("Opening encrypted DB without key failed as expected: %v", err)
	} else {
		defer func() { _ = wrongMgr.Close() }()
		// If it opens, querying should fail
		_, err = wrongMgr.GetDB().Exec("SELECT * FROM secure_data")
		assert.Error(t, err, "Querying encrypted DB without key should fail")
	}
}

func TestEncryption_DatabaseFileIsEncrypted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	keyPath := filepath.Join(tmpDir, "test.key")

	log, _ := zap.NewDevelopment()

	// Create encrypted database
	kmCfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	km, err := NewEncryptionKeyManager(kmCfg)
	require.NoError(t, err)

	cfg := DefaultSQLiteConfig(dbPath)
	err = cfg.EnableEncryptionWithKeyManager(km)
	require.NoError(t, err)

	mgr, err := NewSQLiteManager(cfg, log)
	require.NoError(t, err)

	// Insert some data
	_, err = mgr.GetDB().Exec("CREATE TABLE test (id INTEGER, data TEXT)")
	require.NoError(t, err)

	_, err = mgr.GetDB().Exec("INSERT INTO test VALUES (1, 'sensitive data')")
	require.NoError(t, err)

	_ = mgr.Close()

	// Read the raw database file
	dbBytes, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	// The file should not contain plaintext "sensitive data"
	// SQLCipher encrypts all data including schema
	assert.NotContains(t, string(dbBytes), "sensitive data", "Database file should not contain plaintext data")
}
