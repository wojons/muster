package repository

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrationManager(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()
	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	if err != nil {
		t.Fatalf("NewMigrationManager failed: %v", err)
	}

	t.Run("CreateMigration", func(t *testing.T) {
		name := "test_migration"
		path, err := mm.CreateMigration(name)
		if err != nil {
			t.Fatalf("CreateMigration failed: %v", err)
		}
		if path == "" {
			t.Fatal("path is empty")
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("migration file does not exist: %s", path)
		}
	})

	t.Run("Status", func(t *testing.T) {
		status, err := mm.Status()
		if err != nil {
			t.Fatalf("Status failed: %v", err)
		}
		if status.Current < 0 {
			t.Errorf("expected Current >= 0, got %d", status.Current)
		}
		t.Logf("Migration status: Current=%d, Latest=%d, Pending=%d",
			status.Current, status.Latest, status.Pending)
	})

	t.Run("Validate", func(t *testing.T) {
		if err := mm.Validate(); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
	})
}

func TestMigrationManagerWithMigrations(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()

	t.Run("CreateMigrationFile", func(t *testing.T) {
		mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
		if err != nil {
			t.Fatalf("NewMigrationManager failed: %v", err)
		}

		timestamp := time.Now().Format("20060102150405")
		name := timestamp + "_test.sql"
		path := filepath.Join(dir, name)
		content := `-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS test_table (
    id INTEGER PRIMARY KEY,
    name TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS test_table;
-- +goose StatementEnd
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		if err := mm.Validate(); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		status, err := mm.Status()
		if err != nil {
			t.Fatalf("Status failed: %v", err)
		}

		if status.Latest < 1 {
			t.Errorf("expected Latest >= 1, got %d", status.Latest)
		}
	})
}

func TestCreateMigrationFilename(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()
	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	if err != nil {
		t.Fatalf("NewMigrationManager failed: %v", err)
	}

	path, err := mm.CreateMigration("add_users_table")
	if err != nil {
		t.Fatalf("CreateMigration failed: %v", err)
	}

	if path == "" {
		t.Fatal("path is empty")
	}

	if filepath.Base(path) == path {
		t.Error("path should include filename")
	}
}
