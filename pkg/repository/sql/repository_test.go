package sql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/mutecomm/go-sqlcipher"
	"github.com/openapi-cli/openapi-cli/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an in-memory database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			path TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) UNIQUE NOT NULL,
			description TEXT,
			inventory_id INTEGER NOT NULL,
			module_id INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE modules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			path TEXT NOT NULL,
			language VARCHAR(50) NOT NULL,
			inventory_id INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE specs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			fingerprint VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE fingerprints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash VARCHAR(255) NOT NULL,
			spec_id INTEGER NOT NULL,
			algorithm VARCHAR(50) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action VARCHAR(255) NOT NULL,
			entity_type VARCHAR(255) NOT NULL,
			entity_id INTEGER NOT NULL,
			user_id VARCHAR(255),
			data BLOB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key VARCHAR(255) NOT NULL UNIQUE,
			count INTEGER NOT NULL DEFAULT 0,
			window TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)

	return db
}

func TestInventoryRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory description",
		Path:        "/test/path",
	}

	err = repo.Create(context.Background(), inv)
	assert.NoError(t, err)
	assert.NotZero(t, inv.ID)
	assert.NotZero(t, inv.CreatedAt)
	assert.NotZero(t, inv.UpdatedAt)
}

func TestInventoryRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	// Create test inventory
	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory description",
		Path:        "/test/path",
	}
	err = repo.Create(context.Background(), inv)
	require.NoError(t, err)

	// Get by ID
	retrieved, err := repo.GetByID(context.Background(), inv.ID)
	assert.NoError(t, err)
	assert.Equal(t, inv.Name, retrieved.Name)
	assert.Equal(t, inv.Description, retrieved.Description)
	assert.Equal(t, inv.Path, retrieved.Path)
}

func TestInventoryRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	_, err = repo.GetByID(context.Background(), 99999)
	assert.Error(t, err)
}

func TestInventoryRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	// Create test inventory
	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory description",
		Path:        "/test/path",
	}
	err = repo.Create(context.Background(), inv)
	require.NoError(t, err)

	// Update
	inv.Name = "updated-name"
	inv.Description = "updated-description"
	err = repo.Update(context.Background(), inv)
	assert.NoError(t, err)

	// Verify update
	retrieved, err := repo.GetByID(context.Background(), inv.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated-name", retrieved.Name)
	assert.Equal(t, "updated-description", retrieved.Description)
}

func TestInventoryRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	// Create test inventory
	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory description",
		Path:        "/test/path",
	}
	err = repo.Create(context.Background(), inv)
	require.NoError(t, err)

	// Delete
	err = repo.Delete(context.Background(), inv.ID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByID(context.Background(), inv.ID)
	assert.Error(t, err)
}

func TestInventoryRepository_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)

	// Create test inventories
	for i := 0; i < 3; i++ {
		inv := &repository.Inventory{
			Name:        fmt.Sprintf("test-inventory-%d", i),
			Description: "Test inventory description",
			Path:        "/test/path",
		}
		err := repo.Create(context.Background(), inv)
		require.NoError(t, err)
	}

	// List
	inventories, err := repo.List(context.Background(), repository.ListOptions{Limit: 10, Offset: 0})
	assert.NoError(t, err)
	assert.Len(t, inventories, 3)
}

func TestAliasRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create test inventory first
	invRepo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)
	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory",
		Path:        "/test/path",
	}
	err = invRepo.Create(context.Background(), inv)
	require.NoError(t, err)

	repo, err := NewAliasRepository(db, DriverSQLite)
	require.NoError(t, err)

	alias := &repository.Alias{
		Name:        "test-alias",
		Description: "Test alias description",
		InventoryID: inv.ID,
	}

	err = repo.Create(context.Background(), alias)
	assert.NoError(t, err)
	assert.NotZero(t, alias.ID)
	assert.NotZero(t, alias.CreatedAt)
}

func TestModuleRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create test inventory first
	invRepo, err := NewInventoryRepository(db, DriverSQLite)
	require.NoError(t, err)
	inv := &repository.Inventory{
		Name:        "test-inventory",
		Description: "Test inventory",
		Path:        "/test/path",
	}
	err = invRepo.Create(context.Background(), inv)
	require.NoError(t, err)

	repo, err := NewModuleRepository(db, DriverSQLite)
	require.NoError(t, err)

	mod := &repository.Module{
		Name:        "test-module",
		Path:        "/test/module.go",
		Language:    "go",
		InventoryID: inv.ID,
	}

	err = repo.Create(context.Background(), mod)
	assert.NoError(t, err)
	assert.NotZero(t, mod.ID)
}

func TestSpecRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewSpecRepository(db, DriverSQLite)
	require.NoError(t, err)

	spec := &repository.Spec{
		Name:        "test-spec",
		Content:     "openapi: 3.0.0\ninfo:\n  title: Test",
		Fingerprint: "abc123",
	}

	err = repo.Create(context.Background(), spec)
	assert.NoError(t, err)
	assert.NotZero(t, spec.ID)
}

func TestFingerprintRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create test spec first
	specRepo, err := NewSpecRepository(db, DriverSQLite)
	require.NoError(t, err)
	spec := &repository.Spec{
		Name:        "test-spec",
		Content:     "openapi: 3.0.0",
		Fingerprint: "abc123",
	}
	err = specRepo.Create(context.Background(), spec)
	require.NoError(t, err)

	repo, err := NewFingerprintRepository(db, DriverSQLite)
	require.NoError(t, err)

	fp := &repository.Fingerprint{
		Hash:      "sha256:abcdef",
		SpecID:    spec.ID,
		Algorithm: "sha256",
	}

	err = repo.Create(context.Background(), fp)
	assert.NoError(t, err)
	assert.NotZero(t, fp.ID)
}

func TestAuditRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewAuditRepository(db, DriverSQLite)
	require.NoError(t, err)

	audit := &repository.Audit{
		Action:     "CREATE",
		EntityType: "inventory",
		EntityID:   1,
		Data:       []byte(`{"name": "test"}`),
	}

	err = repo.Create(context.Background(), audit)
	assert.NoError(t, err)
	assert.NotZero(t, audit.ID)
}

func TestRateLimitRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo, err := NewRateLimitRepository(db, DriverSQLite)
	require.NoError(t, err)

	rl := &repository.RateLimit{
		Key:       "test-key",
		Count:     1,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err = repo.Create(context.Background(), rl)
	assert.NoError(t, err)
	assert.NotZero(t, rl.ID)
}
