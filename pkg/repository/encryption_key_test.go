package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetApplicationSecret(t *testing.T) {
	secret := GetApplicationSecret()
	assert.NotEmpty(t, secret, "Application secret should not be empty")
	assert.True(t, len(secret) > 0, "Secret should have length")
	assert.Contains(t, string(secret), "muster-db", "Secret should contain app identifier")
}

func TestDeriveMasterKeyFromPassphrase_Defaults(t *testing.T) {
	passphrase := "test-passphrase"

	// Test with empty salt (should generate one)
	key, err := DeriveMasterKeyFromPassphrase(passphrase, nil, 0)
	require.NoError(t, err)
	assert.Len(t, key, KeySize, "Key should be 32 bytes")
}

func TestDeriveMasterKeyFromPassphrase_CustomIterations(t *testing.T) {
	passphrase := "test-passphrase"
	salt := []byte("test-salt-16bytes!")

	// Test with custom iterations
	key1, err := DeriveMasterKeyFromPassphrase(passphrase, salt, 1000)
	require.NoError(t, err)

	key2, err := DeriveMasterKeyFromPassphrase(passphrase, salt, 2000)
	require.NoError(t, err)

	// Different iterations should produce different keys
	assert.NotEqual(t, key1, key2, "Different iterations should produce different keys")
}

func TestLoadSalt_NotExists(t *testing.T) {
	_, err := loadSalt("/nonexistent/path/salt")
	assert.Error(t, err, "Loading non-existent salt should fail")
}

func TestSaveAndLoadSalt(t *testing.T) {
	tmpDir := t.TempDir()
	saltPath := tmpDir + "/test.salt"

	originalSalt := make([]byte, SaltSize)
	for i := range originalSalt {
		originalSalt[i] = byte(i)
	}

	// Save salt
	err := saveSalt(saltPath, originalSalt)
	require.NoError(t, err)

	// Load salt
	loadedSalt, err := loadSalt(saltPath)
	require.NoError(t, err)

	assert.Equal(t, originalSalt, loadedSalt, "Loaded salt should match original")
}

func TestEncryptionKeyManager_Initialize_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/nested/dir/test.key"

	cfg := &EncryptionKeyConfig{
		KeyPath:    keyPath,
		Iterations: 1000,
	}

	mgr, err := NewEncryptionKeyManager(cfg)
	require.NoError(t, err)

	err = mgr.Initialize()
	require.NoError(t, err)

	// Check if directory was created
	_, err = mgr.DeriveKey()
	assert.NoError(t, err)
}
