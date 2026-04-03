package repository

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSanitizeMigrationName(t *testing.T) {
	t.Run("ValidNames", func(t *testing.T) {
		validNames := []string{
			"add_users_table",
			"create_posts",
			"update_schema_v1",
			"AddUsersTable",
			"ADD_USERS_TABLE",
			"users123",
			"_underscore_",
			"a",
			"ABC123_xyz",
		}

		for _, name := range validNames {
			t.Run(name, func(t *testing.T) {
				sanitized, err := sanitizeMigrationName(name)
				if err != nil {
					t.Errorf("unexpected error for %q: %v", name, err)
				}
				if sanitized != name {
					t.Errorf("expected %q, got %q", name, sanitized)
				}
			})
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		_, err := sanitizeMigrationName("")
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("NullBytes", func(t *testing.T) {
		nullByteNames := []string{
			"test\x00name",
			"\x00",
			"name\x00",
		}

		for _, name := range nullByteNames {
			t.Run(name, func(t *testing.T) {
				_, err := sanitizeMigrationName(name)
				if err == nil {
					t.Errorf("expected error for null byte in %q", name)
				}
			})
		}
	})

	t.Run("PathSeparators", func(t *testing.T) {
		invalidNames := []string{
			"test/name",
			"test\\name",
			"/etc/passwd",
			"C:\\Windows\\System32",
			"../traversal",
			"..\\traversal",
			"test/../name",
			"test\\..\\name",
			"../../etc/passwd",
			"..\\..\\etc\\passwd",
		}

		for _, name := range invalidNames {
			t.Run(name, func(t *testing.T) {
				_, err := sanitizeMigrationName(name)
				if err == nil {
					t.Errorf("expected error for path separator in %q", name)
				}
			})
		}
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		invalidNames := []string{
			"test-name",
			"test.name",
			"test name",
			"test@name",
			"test#name",
			"test$name",
			"test%name",
			"test^name",
			"test&name",
			"test*name",
			"test(name)",
			"test[name]",
			"test{name}",
			"test|name",
			"test;name",
			"test:name",
			"test'name",
			"test\"name",
			"test<name>",
			"test,name",
			"test?name",
			"test!name",
			"test~name",
			"test`name",
		}

		for _, name := range invalidNames {
			t.Run(name, func(t *testing.T) {
				_, err := sanitizeMigrationName(name)
				if err == nil {
					t.Errorf("expected error for special character in %q", name)
				}
			})
		}
	})
}

func TestValidatePathIsWithinDir(t *testing.T) {
	t.Run("ValidPath", func(t *testing.T) {
		baseDir := "/tmp/migrations"
		fullPath := "/tmp/migrations/001_test.sql"

		err := validatePathIsWithinDir(baseDir, fullPath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("PathTraversal", func(t *testing.T) {
		baseDir := "/tmp/migrations"
		fullPath := "/etc/passwd"

		err := validatePathIsWithinDir(baseDir, fullPath)
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})

	t.Run("RelativePathTraversal", func(t *testing.T) {
		baseDir := "/tmp/migrations"
		fullPath := "/tmp/migrations/../../../etc/passwd"

		err := validatePathIsWithinDir(baseDir, fullPath)
		if err == nil {
			t.Error("expected error for relative path traversal")
		}
	})
}

func TestCreateMigration_PathTraversalPrevention(t *testing.T) {
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

	t.Run("PathTraversalWithDotDot", func(t *testing.T) {
		_, err := mm.CreateMigration("../../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal with ..")
		}
	})

	t.Run("PathTraversalWithSlash", func(t *testing.T) {
		_, err := mm.CreateMigration("test/name")
		if err == nil {
			t.Error("expected error for path traversal with /")
		}
	})

	t.Run("PathTraversalWithBackslash", func(t *testing.T) {
		_, err := mm.CreateMigration("test\\name")
		if err == nil {
			t.Error("expected error for path traversal with \\")
		}
	})

	t.Run("NullByteInjection", func(t *testing.T) {
		_, err := mm.CreateMigration("test\x00name")
		if err == nil {
			t.Error("expected error for null byte injection")
		}
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		_, err := mm.CreateMigration("test-name")
		if err == nil {
			t.Error("expected error for special characters")
		}
	})

	t.Run("ValidName", func(t *testing.T) {
		path, err := mm.CreateMigration("valid_migration_name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path == "" {
			t.Error("expected non-empty path")
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("migration file does not exist: %s", path)
		}
	})
}
