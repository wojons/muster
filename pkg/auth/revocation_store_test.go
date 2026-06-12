package auth

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/stretchr/testify/require"
)

func setupTestDatabase(t *testing.T) *DatabaseRevocationStore {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_revocation.db")

	db, err := createTestDatabase(dbPath)
	require.NoError(t, err)

	store := NewDatabaseRevocationStore(db)
	t.Cleanup(func() {
		_ = db.Close()
	})

	return store
}

func createTestDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	createTableSQL := `
		CREATE TABLE IF NOT EXISTS revocation_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash VARCHAR(64) NOT NULL,
			reason VARCHAR(255),
			revoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(token_hash)
		);
		
		CREATE INDEX IF NOT EXISTS idx_revocation_cache_token ON revocation_cache(token_hash);
		CREATE INDEX IF NOT EXISTS idx_revocation_cache_expires ON revocation_cache(expires_at);
		CREATE INDEX IF NOT EXISTS idx_revocation_cache_revoked_at ON revocation_cache(revoked_at);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func TestDatabaseRevocationStore_Revoke(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_12345"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token, expiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDatabaseRevocationStore_Revoke_Update(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_update"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token, expiresAt)
	require.NoError(t, err)

	newExpiresAt := time.Now().Add(48 * time.Hour)
	err = store.Revoke(token, newExpiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.True(t, revoked)

	rc, err := store.GetRevokedToken(token)
	require.NoError(t, err)
	require.NotNil(t, rc)
	require.Equal(t, hashToken(token), rc.TokenHash)
}

func TestDatabaseRevocationStore_IsRevoked_NotRevoked(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_not_revoked"

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestDatabaseRevocationStore_IsRevoked_Expired(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_expired"
	expiresAt := time.Now().Add(-1 * time.Hour)

	err := store.Revoke(token, expiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestDatabaseRevocationStore_Cleanup(t *testing.T) {
	store := setupTestDatabase(t)

	token1 := "test_token_cleanup_1"
	expiresAt1 := time.Now().Add(-2 * time.Hour)

	token2 := "test_token_cleanup_2"
	expiresAt2 := time.Now().Add(2 * time.Hour)

	err := store.Revoke(token1, expiresAt1)
	require.NoError(t, err)

	err = store.Revoke(token2, expiresAt2)
	require.NoError(t, err)

	cutoff := time.Now().Add(-1 * time.Hour)
	err = store.Cleanup(cutoff)
	require.NoError(t, err)

	revoked1, err := store.IsRevoked(token1)
	require.NoError(t, err)
	require.False(t, revoked1)

	revoked2, err := store.IsRevoked(token2)
	require.NoError(t, err)
	require.True(t, revoked2)
}

func TestDatabaseRevocationStore_RevokeWithReason(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_with_reason"
	reason := "security breach"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.RevokeWithReason(token, reason, expiresAt)
	require.NoError(t, err)

	rc, err := store.GetRevokedToken(token)
	require.NoError(t, err)
	require.NotNil(t, rc)
	require.Equal(t, reason, rc.Reason)
}

func TestDatabaseRevocationStore_GetRevokedToken_NotFound(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_not_found"

	rc, err := store.GetRevokedToken(token)
	require.NoError(t, err)
	require.Nil(t, rc)
}

func TestDatabaseRevocationStore_ListRevokedTokens(t *testing.T) {
	store := setupTestDatabase(t)

	testTokens := []struct {
		token     string
		expiresAt time.Time
	}{
		{"token_1", time.Now().Add(24 * time.Hour)},
		{"token_2", time.Now().Add(48 * time.Hour)},
		{"token_3", time.Now().Add(72 * time.Hour)},
	}

	for _, tc := range testTokens {
		err := store.Revoke(tc.token, tc.expiresAt)
		require.NoError(t, err)
	}

	revoked, err := store.ListRevokedTokens(10)
	require.NoError(t, err)
	require.Len(t, revoked, 3)

	revokedLimited, err := store.ListRevokedTokens(2)
	require.NoError(t, err)
	require.Len(t, revokedLimited, 2)
}

func TestDatabaseRevocationStore_ListRevokedTokens_Empty(t *testing.T) {
	store := setupTestDatabase(t)

	revoked, err := store.ListRevokedTokens(10)
	require.NoError(t, err)
	require.Empty(t, revoked)
}

func TestDatabaseRevocationStore_ListRevokedTokens_ZeroLimit(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_zero_limit"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token, expiresAt)
	require.NoError(t, err)

	revoked, err := store.ListRevokedTokens(0)
	require.NoError(t, err)
	require.Len(t, revoked, 1)
}

func TestDatabaseRevocationStore_ConcurrentAccess(t *testing.T) {
	store := setupTestDatabase(t)

	numGoroutines := 10
	tokens := make(chan string, numGoroutines)
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			token := "test_token_concurrent_" + string(rune(id))
			expiresAt := time.Now().Add(24 * time.Hour)

			err := store.Revoke(token, expiresAt)
			if err != nil {
				t.Errorf("failed to revoke token: %v", err)
				return
			}

			tokens <- token
			done <- true
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	close(tokens)

	revoked, err := store.ListRevokedTokens(numGoroutines)
	require.NoError(t, err)
	require.Len(t, revoked, numGoroutines)
}

func TestDatabaseRevocationStore_DifferentTokens(t *testing.T) {
	store := setupTestDatabase(t)

	token1 := "test_token_different_1"
	token2 := "test_token_different_2"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token1, expiresAt)
	require.NoError(t, err)

	err = store.Revoke(token2, expiresAt)
	require.NoError(t, err)

	revoked1, err := store.IsRevoked(token1)
	require.NoError(t, err)
	require.True(t, revoked1)

	revoked2, err := store.IsRevoked(token2)
	require.NoError(t, err)
	require.True(t, revoked2)
}

func TestDatabaseRevocationStore_LongToken(t *testing.T) {
	store := setupTestDatabase(t)

	longToken := "very_long_token_" + string(make([]byte, 1000))
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(longToken, expiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(longToken)
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDatabaseRevocationStore_UnicodeToken(t *testing.T) {
	store := setupTestDatabase(t)

	unicodeToken := "test_token_你好_世界_🌍"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(unicodeToken, expiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(unicodeToken)
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDatabaseRevocationStore_EmptyToken(t *testing.T) {
	store := setupTestDatabase(t)

	emptyToken := ""
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(emptyToken, expiresAt)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(emptyToken)
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDatabaseRevocationStore_MultipleRevokeSameToken(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_multiple_revoke"
	expiresAt1 := time.Now().Add(24 * time.Hour)
	expiresAt2 := time.Now().Add(48 * time.Hour)
	expiresAt3 := time.Now().Add(72 * time.Hour)

	err := store.Revoke(token, expiresAt1)
	require.NoError(t, err)

	err = store.Revoke(token, expiresAt2)
	require.NoError(t, err)

	err = store.Revoke(token, expiresAt3)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.True(t, revoked)

	rc, err := store.GetRevokedToken(token)
	require.NoError(t, err)
	require.NotNil(t, rc)
	require.Equal(t, hashToken(token), rc.TokenHash)
}

func TestDatabaseRevocationStore_Cleanup_NoExpiredTokens(t *testing.T) {
	store := setupTestDatabase(t)

	token := "test_token_no_expired"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token, expiresAt)
	require.NoError(t, err)

	cutoff := time.Now().Add(12 * time.Hour)
	err = store.Cleanup(cutoff)
	require.NoError(t, err)

	revoked, err := store.IsRevoked(token)
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDatabaseRevocationStore_Cleanup_AllTokensExpired(t *testing.T) {
	store := setupTestDatabase(t)

	token1 := "test_token_all_expired_1"
	token2 := "test_token_all_expired_2"

	err := store.Revoke(token1, time.Now().Add(-2*time.Hour))
	require.NoError(t, err)

	err = store.Revoke(token2, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	cutoff := time.Now()
	err = store.Cleanup(cutoff)
	require.NoError(t, err)

	revoked1, err := store.IsRevoked(token1)
	require.NoError(t, err)
	require.False(t, revoked1)

	revoked2, err := store.IsRevoked(token2)
	require.NoError(t, err)
	require.False(t, revoked2)

	revoked, err := store.ListRevokedTokens(10)
	require.NoError(t, err)
	require.Empty(t, revoked)
}
