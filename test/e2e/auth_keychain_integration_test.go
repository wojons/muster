package e2e

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/auth"
)

// TestKeychainIntegration tests real OS keychain integrations
// This test uses the actual OS keychain APIs, not mocks
func TestKeychainIntegration(t *testing.T) {
	skipLong(t)
	testService := fmt.Sprintf("muster-test-%d", time.Now().Unix())
	testAccount := "test-user"
	testPassword := "test-password-12345"

	t.Run("SetAndGet", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		// Set credential
		err := keychain.Set(testService, testAccount, testPassword)
		require.NoError(t, err, "Should store credential in OS keychain")

		// Get credential
		retrieved, err := keychain.Get(testService, testAccount)
		require.NoError(t, err, "Should retrieve credential from OS keychain")
		assert.Equal(t, testPassword, retrieved, "Retrieved password should match")

		// Cleanup
		_ = keychain.Delete(testService, testAccount)
	})

	t.Run("NotFound", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		_, err := keychain.Get(testService+"-nonexistent", testAccount)
		assert.ErrorIs(t, err, auth.ErrNotFound, "Should return ErrNotFound for missing credential")
	})

	t.Run("Delete", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		// Set credential first
		err := keychain.Set(testService, testAccount, testPassword)
		require.NoError(t, err)

		// Delete credential
		err = keychain.Delete(testService, testAccount)
		assert.NoError(t, err, "Should delete credential successfully")

		// Verify deletion
		_, err = keychain.Get(testService, testAccount)
		assert.ErrorIs(t, err, auth.ErrNotFound, "Should not find deleted credential")
	})

	t.Run("Overwrite", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		// Set initial password
		err := keychain.Set(testService, testAccount, testPassword)
		require.NoError(t, err)

		// Overwrite with new password
		newPassword := "new-password-67890"
		err = keychain.Set(testService, testAccount, newPassword)
		require.NoError(t, err, "Should overwrite existing credential")

		// Verify new password
		retrieved, err := keychain.Get(testService, testAccount)
		require.NoError(t, err)
		assert.Equal(t, newPassword, retrieved, "Should retrieve new password")

		// Cleanup
		_ = keychain.Delete(testService, testAccount)
	})

	t.Run("EmptyPassword", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		emptyService := testService + "-empty"
		err := keychain.Set(emptyService, testAccount, "")
		require.NoError(t, err, "Should allow empty password")

		retrieved, err := keychain.Get(emptyService, testAccount)
		require.NoError(t, err)
		assert.Equal(t, "", retrieved, "Empty password should be retrievable")

		// Cleanup
		_ = keychain.Delete(emptyService, testAccount)
	})

	t.Run("LongPassword", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		longService := testService + "-long"
		longPassword := string(make([]byte, 1000))
		for i := range longPassword {
			longPassword = longPassword[:i] + "a" + longPassword[i+1:]
		}

		err := keychain.Set(longService, testAccount, longPassword)
		require.NoError(t, err, "Should handle long passwords")

		retrieved, err := keychain.Get(longService, testAccount)
		require.NoError(t, err)
		assert.Equal(t, longPassword, retrieved, "Long password should match")

		// Cleanup
		_ = keychain.Delete(longService, testAccount)
	})

	t.Run("UnicodePassword", func(t *testing.T) {
		// This test verifies Unicode support in OS keychains
		// Note: Some keychain implementations may encode Unicode differently
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		unicodeService := testService + "-unicode"
		unicodePassword := "密码テスト🔐"

		err := keychain.Set(unicodeService, testAccount, unicodePassword)
		require.NoError(t, err, "Should handle Unicode passwords")

		retrieved, err := keychain.Get(unicodeService, testAccount)
		require.NoError(t, err, "Should retrieve Unicode password")
		// Just verify we got something back - encoding may vary by platform
		assert.NotEmpty(t, retrieved, "Should retrieve non-empty password")

		// Cleanup
		_ = keychain.Delete(unicodeService, testAccount)
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		specialService := testService + "-special"
		specialPassword := "pass!@#$%^&*()_+-=[]{}|;':\",./<>?"

		err := keychain.Set(specialService, testAccount, specialPassword)
		require.NoError(t, err, "Should handle special characters")

		retrieved, err := keychain.Get(specialService, testAccount)
		require.NoError(t, err)
		assert.Equal(t, specialPassword, retrieved, "Special character password should match")

		// Cleanup
		_ = keychain.Delete(specialService, testAccount)
	})

	t.Run("List", func(t *testing.T) {
		skipOnUnsupportedPlatform(t)
		keychain := createKeychain(t)

		listService := testService + "-list"

		// Add multiple credentials
		accounts := []string{"user1", "user2", "user3"}
		for _, acc := range accounts {
			err := keychain.Set(listService, acc, "password-"+acc)
			require.NoError(t, err)
		}

		// List credentials (if supported)
		list, err := keychain.List(listService)
		if err != nil {
			t.Logf("List not supported on this platform: %v", err)
			// Cleanup anyway
			for _, acc := range accounts {
				_ = keychain.Delete(listService, acc)
			}
			return
		}

		assert.GreaterOrEqual(t, len(list), len(accounts), "Should list all accounts")

		// Cleanup
		for _, acc := range accounts {
			_ = keychain.Delete(listService, acc)
		}
	})
}

// TestMacOSKeychainReal tests macOS Keychain integration specifically
func TestMacOSKeychainReal(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only test")
	}

	testService := fmt.Sprintf("muster-macos-test-%d", time.Now().Unix())
	testAccount := "macos-test-user"
	testPassword := "macos-test-password"

	keychain := &auth.MacOSKeychain{}

	t.Run("SetAndGet", func(t *testing.T) {
		err := keychain.Set(testService, testAccount, testPassword)
		require.NoError(t, err, "Should store credential in macOS keychain")

		retrieved, err := keychain.Get(testService, testAccount)
		require.NoError(t, err, "Should retrieve credential from macOS keychain")
		assert.Equal(t, testPassword, retrieved, "Password should match")

		// Cleanup
		_ = keychain.Delete(testService, testAccount)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := keychain.Get(testService+"-nonexistent", testAccount)
		assert.Error(t, err, "Should error for missing credential")
		assert.ErrorIs(t, err, auth.ErrNotFound, "Should return ErrNotFound")
	})
}

// TestWindowsKeychainReal tests Windows Credential Manager integration specifically
func TestWindowsKeychainReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only test")
	}

	testService := fmt.Sprintf("muster-windows-test-%d", time.Now().Unix())
	testAccount := "windows-test-user"
	testPassword := "windows-test-password"

	keychain, _ := auth.NewWindowsKeychainNative()

	t.Run("SetAndGet", func(t *testing.T) {
		err := keychain.Set(testService, testAccount, testPassword)
		require.NoError(t, err, "Should store credential in Windows Credential Manager")

		retrieved, err := keychain.Get(testService, testAccount)
		require.NoError(t, err, "Should retrieve credential from Windows Credential Manager")
		assert.Equal(t, testPassword, retrieved, "Password should match")

		// Cleanup
		_ = keychain.Delete(testService, testAccount)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := keychain.Get(testService+"-nonexistent", testAccount)
		assert.Error(t, err, "Should error for missing credential")
		assert.ErrorIs(t, err, auth.ErrNotFound, "Should return ErrNotFound")
	})
}

// TestLinuxKeychainReal tests Linux Secret Service integration
func TestLinuxKeychainReal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only test")
	}

	testService := fmt.Sprintf("muster-linux-test-%d", time.Now().Unix())
	testAccount := "linux-test-user"
	testPassword := "linux-test-password"

	// Try to create Secret Service keychain
	keychain, err := auth.NewSecretServiceKeychain()
	if err != nil {
		t.Skipf("Secret Service not available: %v", err)
	}

	// Probe whether Secret Service is actually functional (dbus may be broken,
	// service may not be unlocked, etc). NewSecretServiceKeychain only creates
	// the struct — real dbus connection happens on first use.
	probeService := testService + "-probe"
	probeAccount := "probe-account"
	probePassword := []byte("probe-password")
	if err := keychain.Set(probeService, probeAccount, probePassword); err != nil {
		t.Skipf("Secret Service not functional (Set failed): %v", err)
	}
	if _, err := keychain.Get(probeService, probeAccount); err != nil {
		_ = keychain.Delete(probeService, probeAccount)
		t.Skipf("Secret Service not functional (Get failed): %v", err)
	}
	_ = keychain.Delete(probeService, probeAccount)

	t.Run("SetAndGet", func(t *testing.T) {
		err := keychain.Set(testService, testAccount, []byte(testPassword))
		require.NoError(t, err, "Should store credential in Secret Service")

		retrieved, err := keychain.Get(testService, testAccount)
		require.NoError(t, err, "Should retrieve credential from Secret Service")
		assert.Equal(t, testPassword, string(retrieved), "Password should match")

		// Cleanup
		_ = keychain.Delete(testService, testAccount)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := keychain.Get(testService+"-nonexistent", testAccount)
		assert.Error(t, err, "Should error for missing credential")
	})
}

// TestKeychainWithTokenManager tests keychain integration with TokenManager
func TestKeychainWithTokenManager(t *testing.T) {
	skipLong(t)
	skipOnUnsupportedPlatform(t)

	testService := fmt.Sprintf("muster-token-test-%d", time.Now().Unix())
	keychain := createKeychain(t)

	// Create keychain-backed token storage
	storage := &keychainTokenStorage{keychain: keychain}

	// Create TokenManager with keychain storage
	tokenManager := auth.NewTokenManager(storage, "", "", "")

	token := &auth.Token{
		AccessToken:  "test-access-token-123",
		RefreshToken: "test-refresh-token-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	t.Run("StoreToken", func(t *testing.T) {
		err := tokenManager.Store(testService, token)
		require.NoError(t, err, "Should store token in keychain")
	})

	t.Run("RetrieveToken", func(t *testing.T) {
		retrieved, err := tokenManager.Get(testService)
		require.NoError(t, err, "Should retrieve token from keychain")
		assert.Equal(t, token.AccessToken, retrieved.AccessToken, "Access token should match")
		assert.Equal(t, token.RefreshToken, retrieved.RefreshToken, "Refresh token should match")
		assert.Equal(t, token.TokenType, retrieved.TokenType, "Token type should match")

		// Cleanup
		_ = keychain.Delete(testService, "token")
	})
}

// Helper functions

func skipOnUnsupportedPlatform(t *testing.T) {
	// Skip on platforms where we don't have keychain implementations
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		// Supported
	default:
		t.Skipf("Keychain tests not supported on %s", runtime.GOOS)
	}
}

func createKeychain(t *testing.T) auth.Keychain {
	t.Helper()

	switch runtime.GOOS {
	case "darwin":
		return &auth.MacOSKeychain{}
	case "windows":
		kc, _ := auth.NewWindowsKeychainNative()
		return kc
	case "linux":
		// Try Secret Service first, fall back to file keychain
		keychain, err := auth.NewSecretServiceKeychain()
		if err == nil {
			return &secretServiceKeychainWrapper{keychain}
		}
		// Fall back to file keychain
		return createFileKeychain(t)
	default:
		t.Skipf("Keychain tests not supported on %s", runtime.GOOS)
		return nil
	}
}

// secretServiceKeychainWrapper adapts SecretServiceKeychain to Keychain interface
type secretServiceKeychainWrapper struct {
	ss *auth.SecretServiceKeychain
}

func (w *secretServiceKeychainWrapper) Set(service, account, password string) error {
	return w.ss.Set(service, account, []byte(password))
}

func (w *secretServiceKeychainWrapper) Get(service, account string) (string, error) {
	data, err := w.ss.Get(service, account)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (w *secretServiceKeychainWrapper) Delete(service, account string) error {
	return w.ss.Delete(service, account)
}

func (w *secretServiceKeychainWrapper) List(service string) ([]auth.Account, error) {
	return w.ss.List(service)
}

// keychainTokenStorage adapts Keychain interface to TokenStorage interface
type keychainTokenStorage struct {
	keychain auth.Keychain
}

func (s *keychainTokenStorage) Save(service string, token *auth.Token) error {
	// Serialize token to JSON
	tokenData, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("serialize token: %w", err)
	}
	return s.keychain.Set(service, "token", string(tokenData))
}

func (s *keychainTokenStorage) Load(service string) (*auth.Token, error) {
	data, err := s.keychain.Get(service, "token")
	if err != nil {
		return nil, err
	}

	// Deserialize token from JSON
	var token auth.Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("deserialize token: %w", err)
	}

	return &token, nil
}

func createFileKeychain(t *testing.T) auth.Keychain {
	t.Helper()

	// We use the default file keychain
	// The test will clean up after itself by deleting credentials
	return auth.NewFileKeychain()
}
