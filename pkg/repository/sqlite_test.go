package repository

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteManager(t *testing.T) {
	cfg := &SQLiteConfig{
		Path:            ":memory:",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	sm, err := NewSQLiteManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	if sm.db == nil {
		t.Fatal("db is nil")
	}

	t.Run("Ping", func(t *testing.T) {
		if err := sm.Ping(); err != nil {
			t.Fatalf("Ping failed: %v", err)
		}
	})

	t.Run("BeginTx", func(t *testing.T) {
		ctx := context.Background()
		tx, err := sm.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit failed: %v", err)
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats := sm.Stats()
		t.Logf("Stats: %+v", stats)
	})
}

func TestDefaultSQLiteConfig(t *testing.T) {
	cfg := DefaultSQLiteConfig("./muster.db")
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if cfg.MaxOpenConns != 25 {
		t.Errorf("expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 25 {
		t.Errorf("expected MaxIdleConns 25, got %d", cfg.MaxIdleConns)
	}
}

func TestSQLiteManagerClose(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}

	if err := sm.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := sm.Ping(); err == nil {
		t.Error("expected Ping to fail after Close")
	}
}

func TestSQLiteTransaction_AutoRollback(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	ctx := context.Background()

	t.Run("AutoRollbackOnPanic", func(t *testing.T) {
		tx, err := sm.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		defer AutoRollbackTx(tx)()

		_, err = tx.GetTx().Exec("CREATE TABLE test_panic (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		_ = tx.Commit()
	})

	t.Run("AutoRollbackWithoutCommit", func(t *testing.T) {
		tx, err := sm.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		defer AutoRollbackTx(tx)()

		_, err = tx.GetTx().Exec("CREATE TABLE test_no_commit (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		// Intentionally NOT calling Commit - should rollback automatically
	})

	t.Run("DoubleRollbackPrevented", func(t *testing.T) {
		tx, err := sm.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		defer AutoRollbackTx(tx)()

		// Should not panic - already committed
	})

	t.Run("ExplicitRollbackThenDefer", func(t *testing.T) {
		tx, err := sm.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		defer AutoRollbackTx(tx)()

		// Should not error - already rolled back
	})
}

func TestSQLiteTransaction_CommitOnce(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteManager failed: %v", err)
	}
	defer func() { _ = sm.Close() }()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("First Commit failed: %v", err)
	}

	err = tx.Commit()
	if err == nil {
		t.Error("Expected second Commit to fail, got nil error")
	}
}
