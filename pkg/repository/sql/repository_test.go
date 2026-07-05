package sql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/wojons/muster/pkg/repository"
)

// setupDB creates an in-memory SQLite database with all required tables.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	// Create all tables
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			path TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			count INTEGER NOT NULL,
			window DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			user_id TEXT NOT NULL,
			data TEXT,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS fingerprints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash TEXT NOT NULL,
			spec_id INTEGER NOT NULL,
			algorithm TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			inventory_id INTEGER NOT NULL,
			module_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

// fullQuerySet returns a QuerySet with all SQL queries populated for SQLite.
func fullQuerySet() *QuerySet {
	return &QuerySet{
		// Inventory
		InventoryCreate:      "INSERT INTO inventories (name, description, path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))",
		InventoryGetByID:     "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE id = ?",
		InventoryGetByAlias:  "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE name = ?",
		InventoryGetByModule: "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE id = ?",
		InventoryUpdate:      "UPDATE inventories SET name = ?, description = ?, path = ?, updated_at = ? WHERE id = ?",
		InventoryDelete:      "DELETE FROM inventories WHERE id = ?",
		InventoryList:        "SELECT id, name, description, path, created_at, updated_at FROM inventories ORDER BY id LIMIT ? OFFSET ?",
		InventoryCount:       "SELECT COUNT(*) FROM inventories",
		InventorySearch:      "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE name LIKE ? OR description LIKE ? ORDER BY id LIMIT ? OFFSET ?",

		// RateLimit
		RateLimitCreate:      "INSERT INTO rate_limits (key, count, window, expires_at, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		RateLimitGetByID:     "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE id = ?",
		RateLimitGetByKey:    "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE key = ?",
		RateLimitUpdate:      "UPDATE rate_limits SET key = ?, count = ?, window = ?, expires_at = ? WHERE id = ?",
		RateLimitDelete:      "DELETE FROM rate_limits WHERE id = ?",
		RateLimitDeleteByKey: "DELETE FROM rate_limits WHERE key = ?",
		RateLimitCleanup:     "DELETE FROM rate_limits WHERE expires_at < ?",
		RateLimitIncrement:   "INSERT INTO rate_limits (key, count, window, expires_at, created_at) VALUES (?, 1, ?, ?, datetime('now')) ON CONFLICT(key) DO UPDATE SET count = count + 1",

		// Audit
		AuditCreate:      "INSERT INTO audits (action, entity_type, entity_id, user_id, data, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))",
		AuditGetByID:     "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audits WHERE id = ?",
		AuditGetByEntity: "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audits WHERE entity_type = ? AND entity_id = ? ORDER BY id LIMIT ? OFFSET ?",
		AuditList:        "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audits ORDER BY id LIMIT ? OFFSET ?",
		AuditCount:       "SELECT COUNT(*) FROM audits",
		AuditCleanup:     "DELETE FROM audits WHERE created_at < ?",

		// Fingerprint
		FingerprintCreate:       "INSERT INTO fingerprints (hash, spec_id, algorithm, created_at) VALUES (?, ?, ?, datetime('now'))",
		FingerprintGetByID:      "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE id = ?",
		FingerprintGetBySpec:    "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE spec_id = ?",
		FingerprintGetByHash:    "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE hash = ?",
		FingerprintUpdate:       "UPDATE fingerprints SET hash = ?, spec_id = ?, algorithm = ? WHERE id = ?",
		FingerprintDelete:       "DELETE FROM fingerprints WHERE id = ?",
		FingerprintDeleteBySpec: "DELETE FROM fingerprints WHERE spec_id = ?",
		FingerprintExists:       "SELECT EXISTS(SELECT 1 FROM fingerprints WHERE hash = ?)",

		// Alias
		AliasCreate:            "INSERT INTO aliases (name, description, inventory_id, module_id, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))",
		AliasGetByID:           "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE id = ?",
		AliasGetByName:         "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE name = ?",
		AliasGetByInventory:    "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE inventory_id = ? ORDER BY id LIMIT ? OFFSET ?",
		AliasGetByModule:       "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE module_id = ? ORDER BY id LIMIT ? OFFSET ?",
		AliasUpdate:            "UPDATE aliases SET name = ?, description = ?, inventory_id = ?, module_id = ?, updated_at = ? WHERE id = ?",
		AliasDelete:            "DELETE FROM aliases WHERE id = ?",
		AliasDeleteByInventory: "DELETE FROM aliases WHERE inventory_id = ?",
	}
}

// ── sqlRepository Base ───────────────────────────────────────

func TestNewRepository_Valid(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo := NewRepository(db, DriverSQLite)
	if repo == nil {
		t.Fatal("NewRepository returned nil")
	}
	if repo.DB() != db {
		t.Error("DB() should return the original db")
	}
}

func TestNewSQLRepository_Valid(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo := NewSQLRepository(db)
	if repo == nil {
		t.Fatal("NewSQLRepository returned nil")
	}
	if repo.DB() != db {
		t.Error("DB() should return the original db")
	}
}

func TestGetQuerySet_Valid(t *testing.T) {
	qs, err := GetQuerySet(DriverSQLite)
	if err != nil {
		t.Fatalf("GetQuerySet(DriverSQLite): %v", err)
	}
	if qs == nil {
		t.Fatal("GetQuerySet returned nil")
	}
}

func TestGetQuerySet_UnsupportedDriver(t *testing.T) {
	qs, err := GetQuerySet("postgres")
	if err == nil {
		t.Error("GetQuerySet with unsupported driver should return error")
	}
	if qs != nil {
		t.Error("GetQuerySet with unsupported driver should return nil QuerySet")
	}
}

func TestSqlRepository_DB(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo := NewSQLRepository(db)
	if repo.DB() != db {
		t.Error("DB() returned wrong db")
	}
}

// ── Inventory Repository ─────────────────────────────────────

func TestInventoryRepository_New(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, err := NewInventoryRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewInventoryRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewInventoryRepository returned nil")
	}
}

func TestInventoryRepository_CreateAndGetByID(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	inv := &repository.Inventory{Name: "test-inv", Description: "desc", Path: "/path/to/inv"}

	err := repo.Create(ctx, inv)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.ID == 0 {
		t.Error("ID should be set after create")
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "test-inv" {
		t.Errorf("Name = %q, want %q", got.Name, "test-inv")
	}
	if got.Description != "desc" {
		t.Errorf("Description = %q, want %q", got.Description, "desc")
	}
}

func TestInventoryRepository_GetByID_NotFound(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	_, err := repo.GetByID(context.Background(), 999)
	if err == nil {
		t.Fatal("expected ErrNotFound")
	}
	if _, ok := err.(repository.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}

func TestInventoryRepository_Update(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	inv := &repository.Inventory{Name: "original", Path: "/p"}
	_ = repo.Create(ctx, inv)

	inv.Name = "updated"
	inv.Description = "new desc"
	if err := repo.Update(ctx, inv); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, inv.ID)
	if got.Name != "updated" {
		t.Errorf("Name = %q, want %q", got.Name, "updated")
	}
}

func TestInventoryRepository_Delete(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	inv := &repository.Inventory{Name: "del", Path: "/d"}
	_ = repo.Create(ctx, inv)

	if err := repo.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(ctx, inv.ID)
	if err == nil {
		t.Error("GetByID after Delete should return error")
	}
}

func TestInventoryRepository_List(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = repo.Create(ctx, &repository.Inventory{Name: "inv", Path: "/p"})
	}

	results, err := repo.List(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("List len = %d, want 3", len(results))
	}
}

func TestInventoryRepository_List_Pagination(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = repo.Create(ctx, &repository.Inventory{Name: "inv", Path: "/p"})
	}

	results, err := repo.List(ctx, repository.ListOptions{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("List paginated: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List len = %d, want 2", len(results))
	}
}

func TestInventoryRepository_Count(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = repo.Create(ctx, &repository.Inventory{Name: "inv", Path: "/p"})
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Errorf("Count = %d, want 3", count)
	}
}

func TestInventoryRepository_Search(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Inventory{Name: "alpha", Description: "first", Path: "/a"})
	_ = repo.Create(ctx, &repository.Inventory{Name: "beta", Description: "second", Path: "/b"})

	results, err := repo.Search(ctx, "alpha", repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search len = %d, want 1", len(results))
	}
	if results[0].Name != "alpha" {
		t.Errorf("Search result = %q, want %q", results[0].Name, "alpha")
	}
}

func TestInventoryRepository_GetByAlias(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Inventory{Name: "my-alias", Description: "alias desc", Path: "/alias"})
	_ = repo.Create(ctx, &repository.Inventory{Name: "other", Description: "other desc", Path: "/other"})

	// Found by name (alias)
	inv, err := repo.GetByAlias(ctx, "my-alias")
	if err != nil {
		t.Fatalf("GetByAlias: %v", err)
	}
	if inv.Name != "my-alias" {
		t.Errorf("Name = %q, want my-alias", inv.Name)
	}
	if inv.Path != "/alias" {
		t.Errorf("Path = %q, want /alias", inv.Path)
	}

	// Not found
	_, err = repo.GetByAlias(ctx, "nonexistent")
	if err == nil {
		t.Error("GetByAlias for nonexistent should return error")
	}
	if _, ok := err.(repository.ErrNotFound); !ok {
		t.Errorf("GetByAlias error should be ErrNotFound, got %T: %v", err, err)
	}
}

func TestInventoryRepository_GetByModule(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewInventoryRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Inventory{Name: "mod-inv", Description: "module inventory", Path: "/mod"})

	// The test QuerySet maps InventoryGetByModule to "WHERE id = ?"
	// GetByID returns the created inventory, let's use its ID
	inv, err := repo.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// GetByModule with the inventory ID (note: test query uses id, not module_id)
	got, err := repo.GetByModule(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByModule: %v", err)
	}
	if got.ID != inv.ID {
		t.Errorf("ID = %d, want %d", got.ID, inv.ID)
	}
	if got.Name != "mod-inv" {
		t.Errorf("Name = %q, want mod-inv", got.Name)
	}

	// Not found
	_, err = repo.GetByModule(ctx, 99999)
	if err == nil {
		t.Error("GetByModule for nonexistent should return error")
	}
	if _, ok := err.(repository.ErrNotFound); !ok {
		t.Errorf("GetByModule error should be ErrNotFound, got %T: %v", err, err)
	}
}

// ── RateLimit Repository ─────────────────────────────────────

func TestRateLimitRepository_New(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, err := NewRateLimitRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewRateLimitRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewRateLimitRepository returned nil")
	}
}

func TestRateLimitRepository_CreateAndGetByID(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	rl := &repository.RateLimit{Key: "api:login", Count: 5, Window: now, ExpiresAt: now.Add(time.Hour)}

	err := repo.Create(ctx, rl)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rl.ID == 0 {
		t.Error("ID should be set")
	}

	got, err := repo.GetByID(ctx, rl.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Key != "api:login" {
		t.Errorf("Key = %q, want %q", got.Key, "api:login")
	}
}

func TestRateLimitRepository_GetByKey(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.Create(ctx, &repository.RateLimit{Key: "special-key", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)})

	got, err := repo.GetByKey(ctx, "special-key")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got.Key != "special-key" {
		t.Errorf("Key = %q, want %q", got.Key, "special-key")
	}
}

func TestRateLimitRepository_GetByKey_NotFound(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	_, err := repo.GetByKey(context.Background(), "missing-key")
	if err == nil {
		t.Fatal("expected ErrNotFound")
	}
}

func TestRateLimitRepository_Update(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	rl := &repository.RateLimit{Key: "old-key", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)}
	_ = repo.Create(ctx, rl)

	rl.Key = "new-key"
	rl.Count = 10
	if err := repo.Update(ctx, rl); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, rl.ID)
	if got.Key != "new-key" {
		t.Errorf("Key = %q, want %q", got.Key, "new-key")
	}
}

func TestRateLimitRepository_Delete(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	rl := &repository.RateLimit{Key: "to-delete", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)}
	_ = repo.Create(ctx, rl)

	if err := repo.Delete(ctx, rl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, rl.ID)
	if err == nil {
		t.Error("GetByID after Delete should return error")
	}
}

func TestRateLimitRepository_DeleteByKey(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.Create(ctx, &repository.RateLimit{Key: "api:by-key", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)})

	if err := repo.DeleteByKey(ctx, "api:by-key"); err != nil {
		t.Fatalf("DeleteByKey: %v", err)
	}
	_, err := repo.GetByKey(ctx, "api:by-key")
	if err == nil {
		t.Error("GetByKey after DeleteByKey should return error")
	}
}

func TestRateLimitRepository_Cleanup(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()
	// Create one expired entry
	_ = repo.Create(ctx, &repository.RateLimit{
		Key: "expired", Count: 1, Window: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-1 * time.Hour),
	})
	// Create one valid entry
	_ = repo.Create(ctx, &repository.RateLimit{
		Key: "valid", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour),
	})

	deleted, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Cleanup deleted = %d, want 1", deleted)
	}
}

func TestRateLimitRepository_Increment(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewRateLimitRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	now := time.Now().UTC()

	// Increment creates a new entry
	if err := repo.Increment(ctx, "api:incr", now); err != nil {
		t.Fatalf("Increment (create): %v", err)
	}

	got, _ := repo.GetByKey(ctx, "api:incr")
	if got == nil {
		t.Fatal("Increment should create entry")
	}
}

// ── Audit Repository ─────────────────────────────────────────

func TestAuditRepository_New(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, err := NewAuditRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewAuditRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewAuditRepository returned nil")
	}
}

func TestAuditRepository_CreateAndGetByID(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAuditRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	audit := &repository.Audit{Action: "create", EntityType: "inventory", EntityID: 1, UserID: "user1", Data: `{"key":"val"}`}

	err := repo.Create(ctx, audit)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if audit.ID == 0 {
		t.Error("ID should be set")
	}

	got, err := repo.GetByID(ctx, audit.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Action != "create" {
		t.Errorf("Action = %q, want %q", got.Action, "create")
	}
}

func TestAuditRepository_GetByEntity(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAuditRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Audit{Action: "create", EntityType: "task", EntityID: 42, UserID: "u1", Data: "d1"})
	_ = repo.Create(ctx, &repository.Audit{Action: "update", EntityType: "task", EntityID: 42, UserID: "u2", Data: "d2"})
	_ = repo.Create(ctx, &repository.Audit{Action: "delete", EntityType: "task", EntityID: 99, UserID: "u1", Data: "d3"})

	results, err := repo.GetByEntity(ctx, "task", 42, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetByEntity len = %d, want 2", len(results))
	}
}

func TestAuditRepository_List(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAuditRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Audit{Action: "a", EntityType: "t", EntityID: 1, UserID: "u", Data: "d"})

	results, err := repo.List(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("List len = %d, want 1", len(results))
	}
}

func TestAuditRepository_Count(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAuditRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Audit{Action: "a", EntityType: "t", EntityID: 1, UserID: "u", Data: "d"})
	_ = repo.Create(ctx, &repository.Audit{Action: "b", EntityType: "t", EntityID: 2, UserID: "u", Data: "d"})

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}
}

func TestAuditRepository_Cleanup(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAuditRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Audit{Action: "old", EntityType: "t", EntityID: 1, UserID: "u", Data: "d"})
	// Wait a tiny bit so created_at differs
	time.Sleep(100 * time.Millisecond)
	cutoff := time.Now().UTC()

	deleted, err := repo.Cleanup(ctx, cutoff)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Cleanup deleted = %d, want 1", deleted)
	}
}

// ── Fingerprint Repository ───────────────────────────────────

func TestFingerprintRepository_New(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, err := NewFingerprintRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewFingerprintRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewFingerprintRepository returned nil")
	}
}

func TestFingerprintRepository_CreateAndGetByID(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	fp := &repository.Fingerprint{Hash: "abc123", SpecID: 1, Algorithm: "sha256"}

	err := repo.Create(ctx, fp)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fp.ID == 0 {
		t.Error("ID should be set")
	}

	got, _ := repo.GetByID(ctx, fp.ID)
	if got.Hash != "abc123" {
		t.Errorf("Hash = %q, want %q", got.Hash, "abc123")
	}
}

func TestFingerprintRepository_GetBySpec(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "h1", SpecID: 10, Algorithm: "md5"})
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "h2", SpecID: 20, Algorithm: "sha1"})

	got, err := repo.GetBySpec(ctx, 10)
	if err != nil {
		t.Fatalf("GetBySpec: %v", err)
	}
	if got.Hash != "h1" {
		t.Errorf("Hash = %q, want %q", got.Hash, "h1")
	}
}

func TestFingerprintRepository_GetByHash(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "unique-hash", SpecID: 5, Algorithm: "sha512"})

	got, err := repo.GetByHash(ctx, "unique-hash")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.Algorithm != "sha512" {
		t.Errorf("Algorithm = %q, want %q", got.Algorithm, "sha512")
	}
}

func TestFingerprintRepository_Update(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	fp := &repository.Fingerprint{Hash: "old", SpecID: 1, Algorithm: "sha1"}
	_ = repo.Create(ctx, fp)

	fp.Hash = "new"
	fp.Algorithm = "sha256"
	if err := repo.Update(ctx, fp); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, fp.ID)
	if got.Hash != "new" {
		t.Errorf("Hash = %q, want %q", got.Hash, "new")
	}
}

func TestFingerprintRepository_Delete(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	fp := &repository.Fingerprint{Hash: "del", SpecID: 1, Algorithm: "sha1"}
	_ = repo.Create(ctx, fp)

	if err := repo.Delete(ctx, fp.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, fp.ID)
	if err == nil {
		t.Error("GetByID after Delete should return error")
	}
}

func TestFingerprintRepository_DeleteBySpec(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "fp1", SpecID: 100, Algorithm: "sha1"})
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "fp2", SpecID: 100, Algorithm: "sha256"})

	if err := repo.DeleteBySpec(ctx, 100); err != nil {
		t.Fatalf("DeleteBySpec: %v", err)
	}

	// Both fingerprints for spec 100 should be gone
	_, err1 := repo.GetByHash(ctx, "fp1")
	_, err2 := repo.GetByHash(ctx, "fp2")
	if err1 == nil || err2 == nil {
		t.Error("GetByHash after DeleteBySpec should return errors")
	}
}

func TestFingerprintRepository_Exists(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewFingerprintRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Fingerprint{Hash: "existing", SpecID: 1, Algorithm: "sha1"})

	exists, err := repo.Exists(ctx, "existing")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("Exists should be true for existing hash")
	}

	exists, err = repo.Exists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Exists (missing): %v", err)
	}
	if exists {
		t.Error("Exists should be false for nonexistent hash")
	}
}

// ── Alias Repository ─────────────────────────────────────────

func TestAliasRepository_New(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, err := NewAliasRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewAliasRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewAliasRepository returned nil")
	}
}

func TestAliasRepository_CreateAndGetByID(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	alias := &repository.Alias{Name: "my-alias", Description: "test", InventoryID: 1, ModuleID: 2}

	err := repo.Create(ctx, alias)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if alias.ID == 0 {
		t.Error("ID should be set")
	}

	got, _ := repo.GetByID(ctx, alias.ID)
	if got.Name != "my-alias" {
		t.Errorf("Name = %q, want %q", got.Name, "my-alias")
	}
}

func TestAliasRepository_GetByName(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Alias{Name: "unique-name", InventoryID: 1, ModuleID: 1})

	got, err := repo.GetByName(ctx, "unique-name")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "unique-name" {
		t.Errorf("Name = %q, want %q", got.Name, "unique-name")
	}
}

func TestAliasRepository_GetByInventory(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Alias{Name: "a1", InventoryID: 10, ModuleID: 1})
	_ = repo.Create(ctx, &repository.Alias{Name: "a2", InventoryID: 10, ModuleID: 2})
	_ = repo.Create(ctx, &repository.Alias{Name: "a3", InventoryID: 20, ModuleID: 1})

	results, err := repo.GetByInventory(ctx, 10, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("GetByInventory: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetByInventory len = %d, want 2", len(results))
	}
}

func TestAliasRepository_GetByModule(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Alias{Name: "m1", InventoryID: 1, ModuleID: 100})
	_ = repo.Create(ctx, &repository.Alias{Name: "m2", InventoryID: 2, ModuleID: 100})
	_ = repo.Create(ctx, &repository.Alias{Name: "m3", InventoryID: 1, ModuleID: 200})

	results, err := repo.GetByModule(ctx, 100, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("GetByModule: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetByModule len = %d, want 2", len(results))
	}
}

func TestAliasRepository_Update(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	alias := &repository.Alias{Name: "old", InventoryID: 1, ModuleID: 1}
	_ = repo.Create(ctx, alias)

	alias.Name = "new-name"
	alias.Description = "updated"
	if err := repo.Update(ctx, alias); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, alias.ID)
	if got.Name != "new-name" {
		t.Errorf("Name = %q, want new-name", got.Name)
	}
}

func TestAliasRepository_Delete(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	alias := &repository.Alias{Name: "del-me", InventoryID: 1, ModuleID: 1}
	_ = repo.Create(ctx, alias)

	if err := repo.Delete(ctx, alias.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, alias.ID)
	if err == nil {
		t.Error("GetByID after Delete should return error")
	}
}

func TestAliasRepository_DeleteByInventory(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	repo, _ := NewAliasRepository(db, DriverSQLite)
	repo.qs = fullQuerySet()

	ctx := context.Background()
	_ = repo.Create(ctx, &repository.Alias{Name: "inv1-a", InventoryID: 50, ModuleID: 1})
	_ = repo.Create(ctx, &repository.Alias{Name: "inv1-b", InventoryID: 50, ModuleID: 2})

	if err := repo.DeleteByInventory(ctx, 50); err != nil {
		t.Fatalf("DeleteByInventory: %v", err)
	}

	results, _ := repo.GetByInventory(ctx, 50, repository.ListOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("GetByInventory after DeleteByInventory should return empty, got %d", len(results))
	}
}
