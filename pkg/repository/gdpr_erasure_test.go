package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockCache struct {
	data map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string]string),
	}
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if matchPattern(k, pattern) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func matchPattern(key, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) == 0 {
		return true
	}
	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	return key == pattern
}

type mockKeychain struct {
	data map[string]string
}

func newMockKeychain() *mockKeychain {
	return &mockKeychain{
		data: make(map[string]string),
	}
}

func (m *mockKeychain) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func TestSQLiteManager_DataSubjectErasure_Full(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	createTestData(t, sm, "user123")

	cache := newMockCache()
	cache.data["user:user123:session"] = "data1"
	cache.data["user:user123:prefs"] = "data2"
	cache.data["user:user456:session"] = "data3"

	keychain := newMockKeychain()
	keychain.data["muster.credentials.user123"] = "cred1"
	keychain.data["muster.tokens.user123"] = "token1"
	keychain.data["muster.refresh.user123"] = "refresh1"

	result, err := sm.DataSubjectErasure(ctx, "user123", ErasureTypeFull, cache, keychain)
	require.NoError(t, err)

	assert.Equal(t, "user123", result.UserID)
	assert.Contains(t, result.RequestID, "erasure-user123-")
	assert.Greater(t, result.RowsDeleted, int64(0))
	assert.Contains(t, result.TablesAffected, "configs")
	assert.Equal(t, 2, result.CacheKeysDeleted)
	assert.Equal(t, 3, result.KeychainItemsDeleted)
	assert.Greater(t, result.Duration, time.Duration(0))

	verifyUserDataDeleted(t, sm, "user123")
	assert.NotContains(t, cache.data, "user:user123:session")
	assert.NotContains(t, cache.data, "user:user123:prefs")
	assert.Contains(t, cache.data, "user:user456:session")

	assert.NotContains(t, keychain.data, "muster.credentials.user123")
	assert.NotContains(t, keychain.data, "muster.tokens.user123")
	assert.NotContains(t, keychain.data, "muster.refresh.user123")
}

func TestSQLiteManager_DataSubjectErasure_Anonymize(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	createTestData(t, sm, "user123")

	result, err := sm.DataSubjectErasure(ctx, "user123", ErasureTypeAnonymize, nil, nil)
	require.NoError(t, err)

	assert.Greater(t, result.RowsDeleted, int64(0))
}

func TestSQLiteManager_DataSubjectErasure_EmptyDatabase(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	result, err := sm.DataSubjectErasure(ctx, "nonexistent", ErasureTypeFull, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "nonexistent", result.UserID)
	assert.Equal(t, int64(0), result.RowsDeleted)
	assert.Equal(t, 0, result.CacheKeysDeleted)
	assert.Equal(t, 0, result.KeychainItemsDeleted)
}

func TestSQLiteManager_DataSubjectErasure_NoCacheOrKeychain(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	createTestData(t, sm, "user123")

	result, err := sm.DataSubjectErasure(ctx, "user123", ErasureTypeFull, nil, nil)
	require.NoError(t, err)

	assert.Greater(t, result.RowsDeleted, int64(0))
	assert.Equal(t, 0, result.CacheKeysDeleted)
	assert.Equal(t, 0, result.KeychainItemsDeleted)
}

func TestSQLiteManager_DataSubjectErasure_CacheError(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	createTestData(t, sm, "user123")

	errCache := &errorCache{}

	result, err := sm.DataSubjectErasure(ctx, "user123", ErasureTypeFull, errCache, nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.RowsDeleted, int64(0))
	assert.Equal(t, 0, result.CacheKeysDeleted)
}

type errorCache struct{}

func (e *errorCache) Delete(ctx context.Context, key string) error {
	return assert.AnError
}

func (e *errorCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	return nil, assert.AnError
}

func TestSQLiteManager_DeleteUserRecordsFromTable(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	insertTestConfig(t, sm, "config1", "user123")

	tx, err := sm.db.BeginTx(ctx, nil)
	require.NoError(t, err)

	deleted, err := sm.deleteUserRecordsFromTable(ctx, tx, "configs", "user123", ErasureTypeFull)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	require.NoError(t, tx.Commit())

	configs := getConfigs(t, sm)
	assert.Len(t, configs, 0)
}

func TestSQLiteManager_DeleteUserRecordsFromTable_Anonymize(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	insertTestConfig(t, sm, "config1", "user123")

	tx, err := sm.db.BeginTx(ctx, nil)
	require.NoError(t, err)

	deleted, err := sm.deleteUserRecordsFromTable(ctx, tx, "configs", "user123", ErasureTypeAnonymize)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	require.NoError(t, tx.Commit())

	rows, err := sm.db.QueryContext(ctx, "SELECT user_id FROM configs")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	found := false
	for rows.Next() {
		var userID string
		require.NoError(t, rows.Scan(&userID))
		if userID != "user123" && userID[:10] == "anonymous-" {
			found = true
		}
	}

	assert.True(t, found, "User ID should be anonymized")
}

func TestSQLiteManager_DeleteUserCache(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	cache := newMockCache()
	cache.data["user:user123:session"] = "data1"
	cache.data["user:user123:prefs"] = "data2"
	cache.data["user:user456:session"] = "data3"

	deleted := sm.deleteUserCache(ctx, cache, "user123")

	assert.Equal(t, 2, deleted)
	assert.Contains(t, cache.data, "user:user456:session")
	assert.NotContains(t, cache.data, "user:user123:session")
	assert.NotContains(t, cache.data, "user:user123:prefs")
}

func TestSQLiteManager_RedactUserLogs(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	insertAuditLog(t, sm, "user123", `{"user_id": "user123", "action": "login"}`)

	tx, err := sm.db.BeginTx(ctx, nil)
	require.NoError(t, err)

	redacted := sm.redactUserLogs(ctx, tx, "user123")
	assert.Equal(t, 1, redacted)

	require.NoError(t, tx.Commit())

	data := getAuditLogData(t, sm, "user123")
	assert.Contains(t, data, "[REDACTED]")
}

func TestSQLiteManager_DeleteUserKeychain(t *testing.T) {
	sm, tempPath, err := createTestSQLiteManager(t)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()
	defer deleteTempDB(t, tempPath)

	ctx := context.Background()

	keychain := newMockKeychain()
	keychain.data["muster.credentials.user123"] = "cred1"
	keychain.data["muster.tokens.user123"] = "token1"
	keychain.data["muster.refresh.user123"] = "refresh1"
	keychain.data["muster.credentials.user456"] = "cred2"

	deleted := sm.deleteUserKeychain(ctx, keychain, "user123")

	assert.Equal(t, 3, deleted)
	assert.Contains(t, keychain.data, "muster.credentials.user456")
	assert.NotContains(t, keychain.data, "muster.credentials.user123")
	assert.NotContains(t, keychain.data, "muster.tokens.user123")
	assert.NotContains(t, keychain.data, "muster.refresh.user123")
}

func TestErasureTypeConstants(t *testing.T) {
	assert.Equal(t, ErasureType("FULL"), ErasureTypeFull)
	assert.Equal(t, ErasureType("PARTIAL"), ErasureTypePartial)
	assert.Equal(t, ErasureType("ANONYMIZE"), ErasureTypeAnonymize)
}

func TestErasureStatusConstants(t *testing.T) {
	assert.Equal(t, ErasureStatus("PENDING"), ErasureStatusPending)
	assert.Equal(t, ErasureStatus("PROCESSING"), ErasureStatusProcessing)
	assert.Equal(t, ErasureStatus("COMPLETED"), ErasureStatusCompleted)
	assert.Equal(t, ErasureStatus("FAILED"), ErasureStatusFailed)
}

func TestErasureRequestStruct(t *testing.T) {
	req := ErasureRequest{
		ID:           "req123",
		UserID:       "user123",
		Reason:       "GDPR Article 17",
		RequestType:  ErasureTypeFull,
		Status:       ErasureStatusCompleted,
		RequestedAt:  time.Now(),
		DeletedCount: 100,
	}

	assert.Equal(t, "req123", req.ID)
	assert.Equal(t, "user123", req.UserID)
	assert.Equal(t, "GDPR Article 17", req.Reason)
	assert.Equal(t, ErasureTypeFull, req.RequestType)
	assert.Equal(t, ErasureStatusCompleted, req.Status)
	assert.Greater(t, req.DeletedCount, int64(0))
}

func TestErasureResultStruct(t *testing.T) {
	result := ErasureResult{
		RequestID:            "erasure123",
		UserID:               "user123",
		TablesAffected:       []string{"configs", "users"},
		RowsDeleted:          100,
		CacheKeysDeleted:     10,
		LogEntriesRedacted:   5,
		KeychainItemsDeleted: 3,
		Duration:             time.Second,
	}

	assert.Equal(t, "erasure123", result.RequestID)
	assert.Equal(t, "user123", result.UserID)
	assert.Len(t, result.TablesAffected, 2)
	assert.Equal(t, int64(100), result.RowsDeleted)
	assert.Equal(t, 10, result.CacheKeysDeleted)
	assert.Equal(t, 5, result.LogEntriesRedacted)
	assert.Equal(t, 3, result.KeychainItemsDeleted)
	assert.Equal(t, time.Second, result.Duration)
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		key      string
		pattern  string
		expected bool
	}{
		{"user:123:session", "user:123:*", true},
		{"user:123:prefs", "user:123:*", true},
		{"user:456:session", "user:123:*", false},
		{"user:123:session", "user:123:session", true},
		{"user:123:session", "*", true},
		{"anything", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.key+"_"+tt.pattern, func(t *testing.T) {
			result := matchPattern(tt.key, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func createTestSQLiteManager(t *testing.T) (*SQLiteManager, string, error) {
	tempPath := createTempDB(t)

	log, _ := zap.NewDevelopment()

	cfg := DefaultSQLiteConfig(tempPath)

	sm, err := NewSQLiteManager(cfg, log)
	if err != nil {
		return nil, "", err
	}

	ctx := context.Background()

	createTestTables(ctx, sm.db)

	return sm, tempPath, nil
}

func createTestTables(ctx context.Context, db *sql.DB) {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key VARCHAR(255) UNIQUE NOT NULL,
			value TEXT NOT NULL,
			profile VARCHAR(255) DEFAULT 'default',
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			path TEXT,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS modules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			path TEXT,
			language VARCHAR(50),
			inventory_id INTEGER,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			inventory_id INTEGER,
			module_id INTEGER,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS specs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			content TEXT,
			alias_id INTEGER,
			module_id INTEGER,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS fingerprints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash VARCHAR(255) NOT NULL,
			spec_id INTEGER,
			algorithm VARCHAR(50),
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action VARCHAR(255) NOT NULL,
			entity_type VARCHAR(255),
			entity_id INTEGER,
			user_id VARCHAR(255),
			data TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key VARCHAR(255) UNIQUE NOT NULL,
			count INTEGER DEFAULT 0,
			window TIMESTAMP,
			expires_at TIMESTAMP,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS revocation_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash VARCHAR(255) NOT NULL,
			reason TEXT,
			revoked_at TIMESTAMP,
			expires_at TIMESTAMP,
			user_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range tables {
		_, _ = db.ExecContext(ctx, query)
	}
}

func createTempDB(t *testing.T) string {
	tmpDir := t.TempDir()
	return tmpDir + "/test.db"
}

func deleteTempDB(t *testing.T, path string) {

}

func createTestData(t *testing.T, sm *SQLiteManager, userID string) {
	insertTestConfig(t, sm, "config-"+userID, userID)
	insertTestInventory(t, sm, "inventory-"+userID, userID)
}

func insertTestConfig(t *testing.T, sm *SQLiteManager, name, userID string) {
	ctx := context.Background()

	_, err := sm.db.ExecContext(ctx,
		"INSERT INTO configs (key, value, profile, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, `{"key": "value"}`, "default", userID, time.Now(), time.Now())
	require.NoError(t, err)
}

func insertTestInventory(t *testing.T, sm *SQLiteManager, name, userID string) {
	ctx := context.Background()

	_, err := sm.db.ExecContext(ctx,
		"INSERT INTO inventories (name, description, path, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, "Test inventory", "/path/to/inventory", userID, time.Now(), time.Now())
	require.NoError(t, err)
}

func insertAuditLog(t *testing.T, sm *SQLiteManager, userID, data string) {
	ctx := context.Background()

	_, err := sm.db.ExecContext(ctx,
		"INSERT INTO audit (action, entity_type, entity_id, user_id, data, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		"login", "user", "123", userID, data, time.Now())
	require.NoError(t, err)
}

func verifyUserDataDeleted(t *testing.T, sm *SQLiteManager, userID string) {
	ctx := context.Background()

	tables := []string{"configs", "inventories", "modules", "aliases", "specs", "fingerprints", "rate_limits", "revocation_cache"}

	for _, table := range tables {
		var count int64
		err := sm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", userID).Scan(&count)
		if err != nil && err != sql.ErrNoRows {
			// Intentionally ignore errors for tables without user_id column
			continue
		}
		assert.Equal(t, int64(0), count, "Table %s should have no records for user %s", table, userID)
	}
}

func getConfigs(t *testing.T, sm *SQLiteManager) []string {
	ctx := context.Background()

	rows, err := sm.db.QueryContext(ctx, "SELECT key FROM configs")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var configs []string
	for rows.Next() {
		var key string
		require.NoError(t, rows.Scan(&key))
		configs = append(configs, key)
	}

	return configs
}

func getAuditLogData(t *testing.T, sm *SQLiteManager, userID string) string {
	ctx := context.Background()

	var data string
	err := sm.db.QueryRowContext(ctx, "SELECT data FROM audit WHERE user_id = ?", userID).Scan(&data)

	if err == sql.ErrNoRows {

		return ""
	}

	require.NoError(t, err)
	return data
}
