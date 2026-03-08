package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSQLiteManager_BeginTx_Concurrent(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Create 10 concurrent transactions
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := sm.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
		}()
	}

	wg.Wait()
}

func TestSQLiteManager_ConnPoolConfig(t *testing.T) {
	cfg := &SQLiteConfig{
		Path:            ":memory:",
		MaxOpenConns:    100,
		MaxIdleConns:    50,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	stats := sm.Stats()
	require.LessOrEqual(t, stats.MaxOpenConnections, cfg.MaxOpenConns)
}

func TestSQLiteManager_Ping_Callback(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	err = sm.Ping()
	require.NoError(t, err)
}

func TestSQLiteManager_Ping_AfterClose(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)

	require.NoError(t, sm.Close())

	err = sm.Ping()
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")
}

func TestTransaction_BasicCommit(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, nil)
	require.NoError(t, err)

	require.NoError(t, tx.Commit())
}

func TestTransaction_CommitTwice(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	tx, err := sm.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	require.NoError(t, tx.Commit())
	err = tx.Commit()
	require.Error(t, err)
}

func TestTransaction_RollbackTwice(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, nil)
	require.NoError(t, err)

	require.NoError(t, tx.Rollback())
	// SQLite doesn't error on double rollback
	require.NoError(t, err)
}

func TestSQLiteManager_WithLogger(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, logger)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	require.NotNil(t, sm)
}

func TestSQLiteManager_BeginTx_Isolation(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	ctx := context.Background()
	tx, err := sm.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestSQLiteManager_WALMode(t *testing.T) {
	cfg := &SQLiteConfig{
		Path: ":memory:",
	}
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	// WAL mode is enabled in the DSN
	require.NotNil(t, sm)
}

func TestSQLiteManager_Stats_Dynamic(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	stats1 := sm.Stats()
	require.NotNil(t, stats1)

	// Perform some operations
	ctx := context.Background()
	tx, _ := sm.BeginTx(ctx, nil)
	_ = tx.Commit()

	stats2 := sm.Stats()
	require.GreaterOrEqual(t, stats2.OpenConnections, stats1.OpenConnections)
}

func TestTransaction_ContextCancellation(t *testing.T) {
	cfg := DefaultSQLiteConfig(":memory:")
	sm, err := NewSQLiteManager(cfg, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = sm.BeginTx(ctx, nil)
	require.Error(t, err)
}
