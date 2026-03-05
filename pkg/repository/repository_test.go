package repository

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestGetDB(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer sm.Close()

	db := sm.GetDB()
	require.NotNil(t, db)
}

func TestTransaction_Rollback(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer sm.Close()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, nil)
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)
}

func TestTransaction_GetTx(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer sm.Close()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, nil)
	require.NoError(t, err)

	sqlTx := tx.GetTx()
	require.NotNil(t, sqlTx)

	_ = tx.Rollback()
}

func TestSQLiteManager_ConnConfig(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	cfg.MaxOpenConns = 10
	cfg.MaxIdleConns = 5
	cfg.ConnMaxLifetime = 30 * time.Minute
	cfg.ConnMaxIdleTime = 2 * time.Minute

	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer sm.Close()

	stats := sm.Stats()
	require.GreaterOrEqual(t, stats.MaxOpenConnections-stats.OpenConnections, 0)
}

func TestBeginTx_withContext(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer sm.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := sm.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestSQLiteManager_NilConfig(t *testing.T) {
	sm, err := NewSQLiteManager(nil, nil)
	require.NoError(t, err)
	defer sm.Close()

	db := sm.GetDB()
	require.NotNil(t, db)
}

func TestSQLiteManager_EmptyPath(t *testing.T) {
	cfg := &SQLiteConfig{
		Path: "",
	}

	_, err := NewSQLiteManager(cfg, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database path is required")
}
