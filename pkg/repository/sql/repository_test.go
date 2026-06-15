// Package sql provides SQL-backed repository implementations.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/wojons/muster/pkg/repository"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// openDB opens a fresh in-memory SQLite database.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open :memory: db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// testQuerySet returns a fully populated QuerySet that works with SQLite :memory:
// tables created by createTables. The SQL matches the column order expected by every
// Scan call in the repository methods.
func testQuerySet() *QuerySet {
	return &QuerySet{
		// ---- Alias ----
		AliasCreate:           "INSERT INTO aliases(name, description, inventory_id, module_id) VALUES(?, ?, ?, ?)",
		AliasGetByID:          "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE id = ?",
		AliasGetByName:        "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE name = ?",
		AliasGetByInventory:   "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE inventory_id = ? ORDER BY id LIMIT ? OFFSET ?",
		AliasGetByModule:      "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE module_id = ? ORDER BY id LIMIT ? OFFSET ?",
		AliasUpdate:           "UPDATE aliases SET name = ?, description = ?, inventory_id = ?, module_id = ?, updated_at = ? WHERE id = ?",
		AliasDelete:           "DELETE FROM aliases WHERE id = ?",
		AliasDeleteByInventory: "DELETE FROM aliases WHERE inventory_id = ?",
		// ---- Audit ----
		AuditCreate:         "INSERT INTO audit_logs(action, entity_type, entity_id, user_id, data) VALUES(?, ?, ?, ?, ?)",
		AuditGetByID:        "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs WHERE id = ?",
		AuditGetByEntity:    "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs WHERE entity_type = ? AND entity_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		AuditList:           "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?",
		AuditCount:          "SELECT COUNT(*) FROM audit_logs",
		AuditCleanup:        "DELETE FROM audit_logs WHERE created_at < ?",
		// ---- Fingerprint ----
		FingerprintCreate:     "INSERT INTO fingerprints(hash, spec_id, algorithm) VALUES(?, ?, ?)",
		FingerprintGetByID:    "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE id = ?",
		FingerprintGetBySpec:  "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE spec_id = ?",
		FingerprintGetByHash:  "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE hash = ?",
		FingerprintUpdate:     "UPDATE fingerprints SET hash = ?, spec_id = ?, algorithm = ? WHERE id = ?",
		FingerprintDelete:     "DELETE FROM fingerprints WHERE id = ?",
		FingerprintDeleteBySpec: "DELETE FROM fingerprints WHERE spec_id = ?",
		FingerprintExists:     "SELECT EXISTS(SELECT 1 FROM fingerprints WHERE hash = ?)",
		// ---- Inventory ----
		InventoryCreate:       "INSERT INTO inventories(name, description, path) VALUES(?, ?, ?)",
		InventoryGetByID:      "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE id = ?",
		InventoryGetByAlias:   "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE name = ?",
		InventoryGetByModule:  "SELECT i.id, i.name, i.description, i.path, i.created_at, i.updated_at FROM inventories i JOIN modules m ON m.inventory_id = i.id WHERE m.id = ?",
		InventoryUpdate:       "UPDATE inventories SET name = ?, description = ?, path = ?, updated_at = ? WHERE id = ?",
		InventoryDelete:       "DELETE FROM inventories WHERE id = ?",
		InventoryList:         "SELECT id, name, description, path, created_at, updated_at FROM inventories ORDER BY id LIMIT ? OFFSET ?",
		InventoryCount:        "SELECT COUNT(*) FROM inventories",
		InventorySearch:       "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE name LIKE ? OR description LIKE ? ORDER BY id LIMIT ? OFFSET ?",
		// ---- RateLimit ----
		RateLimitCreate:       "INSERT INTO rate_limits(key, count, window, expires_at) VALUES(?, ?, ?, ?)",
		RateLimitGetByID:      "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE id = ?",
		RateLimitGetByKey:     "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE key = ?",
		RateLimitUpdate:       "UPDATE rate_limits SET key = ?, count = ?, window = ?, expires_at = ? WHERE id = ?",
		RateLimitDelete:       "DELETE FROM rate_limits WHERE id = ?",
		RateLimitDeleteByKey:  "DELETE FROM rate_limits WHERE key = ?",
		RateLimitCleanup:      "DELETE FROM rate_limits WHERE expires_at < ?",
		RateLimitIncrement:    "INSERT INTO rate_limits(key, count, window, expires_at) VALUES(?, 1, ?, ?) ON CONFLICT(key) DO UPDATE SET count = count + 1",
	}
}

// createTables runs all CREATE TABLE statements on the given connection.
func createTables(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			inventory_id INTEGER NOT NULL DEFAULT 0,
			module_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL DEFAULT 0,
			user_id TEXT NOT NULL DEFAULT '',
			data TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS fingerprints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash TEXT NOT NULL,
			spec_id INTEGER NOT NULL DEFAULT 0,
			algorithm TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			count INTEGER NOT NULL DEFAULT 0,
			window DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS modules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			inventory_id INTEGER NOT NULL DEFAULT 0
		)`,
	}
	ctx := context.Background()
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("create table: %v\nSQL: %s", err, s)
		}
	}
}

// setupTest opens an :memory: SQLite database and runs all CREATE TABLE statements.
func setupTest(t *testing.T) *sql.DB {
	t.Helper()
	db := openDB(t)
	createTables(t, db)
	return db
}

// ---------------------------------------------------------------------------
// 1. NewRepository  — creates sqlRepository with non-nil db
// ---------------------------------------------------------------------------

func TestNewRepository(t *testing.T) {
	db := openDB(t)
	r := NewRepository(db, DriverSQLite)
	if r == nil {
		t.Fatal("NewRepository returned nil")
	}
	if r.DB() != db {
		t.Error("DB() returned a different *sql.DB pointer")
	}
}

// ---------------------------------------------------------------------------
// 2. GetQuerySet with DriverSQLite — returns valid QuerySet (non-nil, no error)
// 3. GetQuerySet with unknown driver — returns error
//
// NOTE: The current GetQuerySet implementation returns an empty QuerySet.
//       The test validates the contract (non-nil, no error).
// ---------------------------------------------------------------------------

func TestGetQuerySet_SQLite(t *testing.T) {
	qs, err := GetQuerySet(DriverSQLite)
	if err != nil {
		t.Fatalf("GetQuerySet(DriverSQLite) returned unexpected error: %v", err)
	}
	if qs == nil {
		t.Fatal("GetQuerySet returned nil")
	}
}

func TestGetQuerySet_UnknownDriver(t *testing.T) {
	_, err := GetQuerySet("postgres")
	if err == nil {
		t.Fatal("expected error for unsupported driver, got nil")
	}
}

// ---------------------------------------------------------------------------
// 4. Each repository: NewXxxRepository with valid db+driver — returns repo, no error
// ---------------------------------------------------------------------------

func TestNewAliasRepository(t *testing.T) {
	db := setupTest(t)
	repo, err := NewAliasRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewAliasRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewAliasRepository returned nil")
	}
}

func TestNewAliasRepository_UnknownDriver(t *testing.T) {
	db := openDB(t)
	_, err := NewAliasRepository(db, "postgres")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestNewAuditRepository(t *testing.T) {
	db := setupTest(t)
	repo, err := NewAuditRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewAuditRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewAuditRepository returned nil")
	}
}

func TestNewFingerprintRepository(t *testing.T) {
	db := setupTest(t)
	repo, err := NewFingerprintRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewFingerprintRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewFingerprintRepository returned nil")
	}
}

func TestNewInventoryRepository(t *testing.T) {
	db := setupTest(t)
	repo, err := NewInventoryRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewInventoryRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewInventoryRepository returned nil")
	}
}

func TestNewRateLimitRepository(t *testing.T) {
	db := setupTest(t)
	repo, err := NewRateLimitRepository(db, DriverSQLite)
	if err != nil {
		t.Fatalf("NewRateLimitRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("NewRateLimitRepository returned nil")
	}
}

// ---------------------------------------------------------------------------
// 5–11. AliasRepository: Create, GetByID (existing + missing), GetByName, List
//       (via GetByInventory), Update, Delete
// ---------------------------------------------------------------------------

func TestAliasRepository_CRUD(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	base := NewRepository(db, DriverSQLite)
	repo := &aliasRepository{base: base, qs: testQuerySet()}

	t.Run("Create_sets_id", func(t *testing.T) {
		a := &repository.Alias{
			Name:        "my-alias",
			Description: "test alias",
			InventoryID: 10,
			ModuleID:    20,
		}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if a.ID <= 0 {
			t.Fatal("expected ID > 0 after Create")
		}
		if a.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
		if a.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
		if a.Name != "my-alias" {
			t.Errorf("Name = %q, want %q", a.Name, "my-alias")
		}
	})

	t.Run("GetByID_existing", func(t *testing.T) {
		a := &repository.Alias{Name: "getbyid", Description: "desc", InventoryID: 1, ModuleID: 2}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != a.ID {
			t.Errorf("ID = %d, want %d", got.ID, a.ID)
		}
		if got.Name != a.Name {
			t.Errorf("Name = %q, want %q", got.Name, a.Name)
		}
		if got.Description != a.Description {
			t.Errorf("Description = %q, want %q", got.Description, a.Description)
		}
		if got.InventoryID != a.InventoryID {
			t.Errorf("InventoryID = %d, want %d", got.InventoryID, a.InventoryID)
		}
		if got.ModuleID != a.ModuleID {
			t.Errorf("ModuleID = %d, want %d", got.ModuleID, a.ModuleID)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
		if got.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt")
		}
	})

	t.Run("GetByID_non_existent_returns_ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("expected error for non-existent alias")
		}
		var notFound repository.ErrNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("expected ErrNotFound, got %T: %v", err, err)
		}
	})

	t.Run("GetByName", func(t *testing.T) {
		a := &repository.Alias{Name: "find-me-by-name", Description: "d", InventoryID: 1, ModuleID: 1}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByName(ctx, "find-me-by-name")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if got.ID != a.ID {
			t.Errorf("ID = %d, want %d", got.ID, a.ID)
		}
	})

	t.Run("GetByName_non_existent", func(t *testing.T) {
		_, err := repo.GetByName(ctx, "no-such-name")
		if err == nil {
			t.Fatal("expected error for non-existent name")
		}
	})

	t.Run("GetByInventory_lists_matching_aliases", func(t *testing.T) {
		invID := int64(99)
		a1 := &repository.Alias{Name: "li-alias-1", Description: "d1", InventoryID: invID, ModuleID: 1}
		a2 := &repository.Alias{Name: "li-alias-2", Description: "d2", InventoryID: invID, ModuleID: 2}
		if err := repo.Create(ctx, a1); err != nil {
			t.Fatalf("Create a1: %v", err)
		}
		if err := repo.Create(ctx, a2); err != nil {
			t.Fatalf("Create a2: %v", err)
		}
		results, err := repo.GetByInventory(ctx, invID, repository.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("GetByInventory: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("Update", func(t *testing.T) {
		a := &repository.Alias{Name: "before-update", Description: "original", InventoryID: 1, ModuleID: 1}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}

		a.Name = "after-update"
		a.Description = "updated desc"
		if err := repo.Update(ctx, a); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := repo.GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.Name != "after-update" {
			t.Errorf("Name = %q, want %q", got.Name, "after-update")
		}
		if got.Description != "updated desc" {
			t.Errorf("Description = %q, want %q", got.Description, "updated desc")
		}
		if got.UpdatedAt.Before(got.CreatedAt) {
			t.Error("UpdatedAt should be >= CreatedAt after update")
		}
	})

	t.Run("Delete_and_GetByID_fails", func(t *testing.T) {
		a := &repository.Alias{Name: "delete-me", Description: "bye", InventoryID: 1, ModuleID: 1}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Delete(ctx, a.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.GetByID(ctx, a.ID)
		if err == nil {
			t.Fatal("expected error after Delete")
		}
	})
}

// ---------------------------------------------------------------------------
// 5–10, 12. AuditRepository: Create, GetByID (existing + missing), List,
//           List with options, Update (not applicable — no Update method),
//           Delete
// ---------------------------------------------------------------------------

func TestAuditRepository_CRUD(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	base := NewRepository(db, DriverSQLite)
	repo := &auditRepository{base: base, qs: testQuerySet()}

	t.Run("Create_sets_id", func(t *testing.T) {
		a := &repository.Audit{
			Action:     "create",
			EntityType: "widget",
			EntityID:   1,
			UserID:     "user1",
			Data:       `{"key":"val"}`,
		}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if a.ID <= 0 {
			t.Fatal("expected ID > 0 after Create")
		}
		if a.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("GetByID_existing", func(t *testing.T) {
		a := &repository.Audit{Action: "test-action", EntityType: "type-a", EntityID: 1, UserID: "u1", Data: "data1"}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != a.ID {
			t.Errorf("ID = %d, want %d", got.ID, a.ID)
		}
		if got.Action != a.Action {
			t.Errorf("Action = %q, want %q", got.Action, a.Action)
		}
		if got.EntityType != a.EntityType {
			t.Errorf("EntityType = %q, want %q", got.EntityType, a.EntityType)
		}
		if got.EntityID != a.EntityID {
			t.Errorf("EntityID = %d, want %d", got.EntityID, a.EntityID)
		}
		if got.UserID != a.UserID {
			t.Errorf("UserID = %q, want %q", got.UserID, a.UserID)
		}
		if got.Data != a.Data {
			t.Errorf("Data = %q, want %q", got.Data, a.Data)
		}
	})

	t.Run("GetByID_non_existent_returns_ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("expected error for non-existent audit log")
		}
		var notFound repository.ErrNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("expected ErrNotFound, got %T: %v", err, err)
		}
	})

	t.Run("List_returns_created_entities", func(t *testing.T) {
		// Create two audit entries
		a1 := &repository.Audit{Action: "act1", EntityType: "t1", EntityID: 1, UserID: "u1", Data: "d1"}
		a2 := &repository.Audit{Action: "act2", EntityType: "t2", EntityID: 2, UserID: "u2", Data: "d2"}
		if err := repo.Create(ctx, a1); err != nil {
			t.Fatalf("Create a1: %v", err)
		}
		if err := repo.Create(ctx, a2); err != nil {
			t.Fatalf("Create a2: %v", err)
		}

		results, err := repo.List(ctx, repository.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// Should include the two just created plus any from earlier subtests,
		// so just verify we got at least 2.
		if len(results) < 2 {
			t.Fatalf("expected at least 2 audit logs, got %d", len(results))
		}
	})

	t.Run("List_with_options_pagination", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			a := &repository.Audit{
				Action:     "page-act",
				EntityType: "page",
				EntityID:   int64(i),
				UserID:     "u",
				Data:       "",
			}
			if err := repo.Create(ctx, a); err != nil {
				t.Fatalf("Create #%d: %v", i, err)
			}
		}
		// Limit = 2, expect exactly 2 results
		results, err := repo.List(ctx, repository.ListOptions{Limit: 2})
		if err != nil {
			t.Fatalf("List with limit: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results with Limit=2, got %d", len(results))
		}
	})

	t.Run("GetByEntity", func(t *testing.T) {
		// Create two audit logs for the same entity type+id
		a1 := &repository.Audit{Action: "create", EntityType: "order", EntityID: 42, UserID: "u1", Data: "init"}
		a2 := &repository.Audit{Action: "update", EntityType: "order", EntityID: 42, UserID: "u2", Data: "modified"}
		if err := repo.Create(ctx, a1); err != nil {
			t.Fatalf("Create a1: %v", err)
		}
		if err := repo.Create(ctx, a2); err != nil {
			t.Fatalf("Create a2: %v", err)
		}
		results, err := repo.GetByEntity(ctx, "order", 42, repository.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("GetByEntity: %v", err)
		}
		if len(results) < 2 {
			t.Fatalf("expected at least 2 audit logs for entity order/42, got %d", len(results))
		}
	})

	t.Run("Count", func(t *testing.T) {
		count, err := repo.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count < 1 {
			t.Errorf("expected count >= 1, got %d", count)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Cleanup entries older than 1 hour ago — all entries were just created,
		// so none should be affected. Just verify no error.
		older := time.Now().UTC().Add(-1 * time.Hour)
		_, err := repo.Cleanup(ctx, older)
		if err != nil {
			t.Fatalf("Cleanup: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 5–10, 13. FingerprintRepository: Create, GetByID (existing + missing),
//           GetByHash, Update, Delete
// ---------------------------------------------------------------------------

func TestFingerprintRepository_CRUD(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	base := NewRepository(db, DriverSQLite)
	repo := &fingerprintRepository{base: base, qs: testQuerySet()}

	t.Run("Create_sets_id", func(t *testing.T) {
		fp := &repository.Fingerprint{
			Hash:      "abc123",
			SpecID:    1,
			Algorithm: "sha256",
		}
		if err := repo.Create(ctx, fp); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if fp.ID <= 0 {
			t.Fatal("expected ID > 0 after Create")
		}
		if fp.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("GetByID_existing", func(t *testing.T) {
		fp := &repository.Fingerprint{Hash: "hash-for-get", SpecID: 2, Algorithm: "md5"}
		if err := repo.Create(ctx, fp); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, fp.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != fp.ID {
			t.Errorf("ID = %d, want %d", got.ID, fp.ID)
		}
		if got.Hash != fp.Hash {
			t.Errorf("Hash = %q, want %q", got.Hash, fp.Hash)
		}
		if got.SpecID != fp.SpecID {
			t.Errorf("SpecID = %d, want %d", got.SpecID, fp.SpecID)
		}
		if got.Algorithm != fp.Algorithm {
			t.Errorf("Algorithm = %q, want %q", got.Algorithm, fp.Algorithm)
		}
	})

	t.Run("GetByID_non_existent_returns_ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("expected error for non-existent fingerprint")
		}
		var notFound repository.ErrNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("expected ErrNotFound, got %T: %v", err, err)
		}
	})

	t.Run("GetByHash", func(t *testing.T) {
		fp := &repository.Fingerprint{Hash: "unique-hash-xyz", SpecID: 3, Algorithm: "sha1"}
		if err := repo.Create(ctx, fp); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByHash(ctx, "unique-hash-xyz")
		if err != nil {
			t.Fatalf("GetByHash: %v", err)
		}
		if got.ID != fp.ID {
			t.Errorf("ID = %d, want %d", got.ID, fp.ID)
		}
		if got.Hash != fp.Hash {
			t.Errorf("Hash = %q, want %q", got.Hash, fp.Hash)
		}
	})

	t.Run("GetByHash_non_existent", func(t *testing.T) {
		_, err := repo.GetByHash(ctx, "no-such-hash")
		if err == nil {
			t.Fatal("expected error for non-existent hash")
		}
	})

	t.Run("Update", func(t *testing.T) {
		fp := &repository.Fingerprint{Hash: "before-update", SpecID: 10, Algorithm: "alg1"}
		if err := repo.Create(ctx, fp); err != nil {
			t.Fatalf("Create: %v", err)
		}
		fp.Hash = "after-update"
		fp.Algorithm = "alg2"
		if err := repo.Update(ctx, fp); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repo.GetByID(ctx, fp.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.Hash != "after-update" {
			t.Errorf("Hash = %q, want %q", got.Hash, "after-update")
		}
		if got.Algorithm != "alg2" {
			t.Errorf("Algorithm = %q, want %q", got.Algorithm, "alg2")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		fp := &repository.Fingerprint{Hash: "to-delete", SpecID: 0, Algorithm: "alg"}
		if err := repo.Create(ctx, fp); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Delete(ctx, fp.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.GetByID(ctx, fp.ID)
		if err == nil {
			t.Fatal("expected error after Delete")
		}
	})
}

// ---------------------------------------------------------------------------
// 5–10. InventoryRepository: Create, GetByID (existing + missing), List, Update, Delete
// ---------------------------------------------------------------------------

func TestInventoryRepository_CRUD(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	base := NewRepository(db, DriverSQLite)
	repo := &inventoryRepository{base: base, qs: testQuerySet()}

	t.Run("Create_sets_id", func(t *testing.T) {
		inv := &repository.Inventory{
			Name:        "my-inventory",
			Description: "test inventory",
			Path:        "/path/to/inv",
		}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if inv.ID <= 0 {
			t.Fatal("expected ID > 0 after Create")
		}
		if inv.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
		if inv.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
	})

	t.Run("GetByID_existing", func(t *testing.T) {
		inv := &repository.Inventory{Name: "getbyid-inv", Description: "desc", Path: "/p"}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, inv.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != inv.ID {
			t.Errorf("ID = %d, want %d", got.ID, inv.ID)
		}
		if got.Name != inv.Name {
			t.Errorf("Name = %q, want %q", got.Name, inv.Name)
		}
		if got.Description != inv.Description {
			t.Errorf("Description = %q, want %q", got.Description, inv.Description)
		}
		if got.Path != inv.Path {
			t.Errorf("Path = %q, want %q", got.Path, inv.Path)
		}
	})

	t.Run("GetByID_non_existent_returns_ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("expected error for non-existent inventory")
		}
		var notFound repository.ErrNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("expected ErrNotFound, got %T: %v", err, err)
		}
	})

	t.Run("List_returns_created_entities", func(t *testing.T) {
		repo := &inventoryRepository{base: base, qs: testQuerySet()}
		invA := &repository.Inventory{Name: "list-a", Description: "da", Path: "/a"}
		invB := &repository.Inventory{Name: "list-b", Description: "db", Path: "/b"}
		if err := repo.Create(ctx, invA); err != nil {
			t.Fatalf("Create invA: %v", err)
		}
		if err := repo.Create(ctx, invB); err != nil {
			t.Fatalf("Create invB: %v", err)
		}
		results, err := repo.List(ctx, repository.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// At minimum the two we just created
		if len(results) < 2 {
			t.Fatalf("expected at least 2 inventories, got %d", len(results))
		}
	})

	t.Run("Update", func(t *testing.T) {
		inv := &repository.Inventory{Name: "inv-before", Description: "orig", Path: "/orig"}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create: %v", err)
		}
		inv.Name = "inv-after"
		inv.Description = "updated"
		if err := repo.Update(ctx, inv); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repo.GetByID(ctx, inv.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.Name != "inv-after" {
			t.Errorf("Name = %q, want %q", got.Name, "inv-after")
		}
		if got.Description != "updated" {
			t.Errorf("Description = %q, want %q", got.Description, "updated")
		}
		if got.UpdatedAt.Before(got.CreatedAt) {
			t.Error("UpdatedAt should be >= CreatedAt after update")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		inv := &repository.Inventory{Name: "to-delete", Description: "bye", Path: "/x"}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Delete(ctx, inv.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.GetByID(ctx, inv.ID)
		if err == nil {
			t.Fatal("expected error after Delete")
		}
	})
}

// ---------------------------------------------------------------------------
// 5–10. RateLimitRepository: Create, GetByID (existing + missing), GetByKey,
//          Update, Delete
// ---------------------------------------------------------------------------

func TestRateLimitRepository_CRUD(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	base := NewRepository(db, DriverSQLite)
	repo := &rateLimitRepository{base: base, qs: testQuerySet()}
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("Create_sets_id", func(t *testing.T) {
		rl := &repository.RateLimit{
			Key:       "rate:test1",
			Count:     5,
			Window:    now,
			ExpiresAt: now.Add(time.Hour),
		}
		if err := repo.Create(ctx, rl); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if rl.ID <= 0 {
			t.Fatal("expected ID > 0 after Create")
		}
		if rl.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("GetByID_existing", func(t *testing.T) {
		rl := &repository.RateLimit{Key: "rate:getbyid", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)}
		if err := repo.Create(ctx, rl); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, rl.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != rl.ID {
			t.Errorf("ID = %d, want %d", got.ID, rl.ID)
		}
		if got.Key != rl.Key {
			t.Errorf("Key = %q, want %q", got.Key, rl.Key)
		}
		if got.Count != rl.Count {
			t.Errorf("Count = %d, want %d", got.Count, rl.Count)
		}
	})

	t.Run("GetByID_non_existent_returns_ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("expected error for non-existent rate limit")
		}
		var notFound repository.ErrNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("expected ErrNotFound, got %T: %v", err, err)
		}
	})

	t.Run("GetByKey", func(t *testing.T) {
		rl := &repository.RateLimit{Key: "rate:getbykey", Count: 3, Window: now, ExpiresAt: now.Add(time.Hour)}
		if err := repo.Create(ctx, rl); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByKey(ctx, "rate:getbykey")
		if err != nil {
			t.Fatalf("GetByKey: %v", err)
		}
		if got.ID != rl.ID {
			t.Errorf("ID = %d, want %d", got.ID, rl.ID)
		}
		if got.Count != 3 {
			t.Errorf("Count = %d, want %d", got.Count, 3)
		}
	})

	t.Run("GetByKey_non_existent", func(t *testing.T) {
		_, err := repo.GetByKey(ctx, "rate:nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent key")
		}
	})

	t.Run("Update", func(t *testing.T) {
		rl := &repository.RateLimit{Key: "rate:update", Count: 1, Window: now, ExpiresAt: now.Add(time.Hour)}
		if err := repo.Create(ctx, rl); err != nil {
			t.Fatalf("Create: %v", err)
		}
		rl.Count = 100
		rl.Window = now.Add(time.Minute)
		if err := repo.Update(ctx, rl); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repo.GetByID(ctx, rl.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.Count != 100 {
			t.Errorf("Count = %d, want %d", got.Count, 100)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		rl := &repository.RateLimit{Key: "rate:delete", Count: 0, Window: now, ExpiresAt: now.Add(time.Hour)}
		if err := repo.Create(ctx, rl); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Delete(ctx, rl.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.GetByID(ctx, rl.ID)
		if err == nil {
			t.Fatal("expected error after Delete")
		}
	})
}

// ---------------------------------------------------------------------------
// 14. sqlRepository exec() with SQLite :memory: — verify it works
// ---------------------------------------------------------------------------

func TestSQLRepository_Exec(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	r := NewRepository(db, DriverSQLite)

	result, err := r.exec(ctx,
		"INSERT INTO aliases(name, description, inventory_id, module_id) VALUES(?, ?, ?, ?)",
		"exec-test", "desc", 1, 1,
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected id > 0, got %d", id)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", rowsAffected)
	}
}

// ---------------------------------------------------------------------------
// 15. sqlRepository exec() with bad SQL — returns error
// ---------------------------------------------------------------------------

func TestSQLRepository_Exec_BadSQL(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	r := NewRepository(db, DriverSQLite)

	_, err := r.exec(ctx, "INSERT INTO nonexistent_table VALUES(?)", 1)
	if err == nil {
		t.Fatal("expected error for bad SQL")
	}
}

// ---------------------------------------------------------------------------
// 16. sqlRepository query() — INSERT then SELECT
// ---------------------------------------------------------------------------

func TestSQLRepository_Query(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	r := NewRepository(db, DriverSQLite)

	// Insert a row
	_, err := r.exec(ctx,
		"INSERT INTO aliases(name, description, inventory_id, module_id) VALUES(?, ?, ?, ?)",
		"query-test-name", "desc", 1, 1,
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	// Query it back
	rows, err := r.query(ctx,
		"SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE name = ?",
		"query-test-name",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected at least one row from query")
	}
	var id int64
	var name, description string
	var inventoryID, moduleID int64
	var createdAt, updatedAt time.Time
	if err := rows.Scan(&id, &name, &description, &inventoryID, &moduleID, &createdAt, &updatedAt); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if name != "query-test-name" {
		t.Errorf("name = %q, want %q", name, "query-test-name")
	}
	if id <= 0 {
		t.Errorf("expected id > 0, got %d", id)
	}

	// Should be no more rows
	if rows.Next() {
		t.Error("expected exactly one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 17. sqlRepository queryRow() — INSERT then single row SELECT
// ---------------------------------------------------------------------------

func TestSQLRepository_QueryRow(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	r := NewRepository(db, DriverSQLite)

	// Insert
	_, err := r.exec(ctx,
		"INSERT INTO aliases(name, description, inventory_id, module_id) VALUES(?, ?, ?, ?)",
		"row-test-name", "desc", 1, 1,
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	// QueryRow
	var id int64
	var name string
	err = r.queryRow(ctx,
		"SELECT id, name FROM aliases WHERE name = ?",
		"row-test-name",
	).Scan(&id, &name)
	if err != nil {
		t.Fatalf("queryRow: %v", err)
	}
	if name != "row-test-name" {
		t.Errorf("name = %q, want %q", name, "row-test-name")
	}
	if id <= 0 {
		t.Errorf("expected id > 0, got %d", id)
	}
}

// ---------------------------------------------------------------------------
// 18. sqlRepository queryRow() with no rows — returns sql.ErrNoRows
// ---------------------------------------------------------------------------

func TestSQLRepository_QueryRow_NoRows(t *testing.T) {
	db := setupTest(t)
	ctx := context.Background()
	r := NewRepository(db, DriverSQLite)

	var id int64
	var name string
	err := r.queryRow(ctx,
		"SELECT id, name FROM aliases WHERE name = ?",
		"does-not-exist",
	).Scan(&id, &name)

	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Additional: NewSQLRepository
// ---------------------------------------------------------------------------

func TestNewSQLRepository(t *testing.T) {
	db := openDB(t)
	r := NewSQLRepository(db)
	if r == nil {
		t.Fatal("NewSQLRepository returned nil")
	}
	if r.DB() != db {
		t.Error("DB() returned a different *sql.DB pointer")
	}
}
