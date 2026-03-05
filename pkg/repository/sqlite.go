package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// SQLiteConfig holds configuration for SQLite database
type SQLiteConfig struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultSQLiteConfig returns default configuration
func DefaultSQLiteConfig(path string) *SQLiteConfig {
	return &SQLiteConfig{
		Path:            path,
		MaxOpenConns:    25,
		MaxIdleConns:    25,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// SQLiteManager manages SQLite database connections
type SQLiteManager struct {
	db  *sql.DB
	log *zap.Logger
	cfg *SQLiteConfig
}

// NewSQLiteManager creates a new SQLite manager
func NewSQLiteManager(cfg *SQLiteConfig, log *zap.Logger) (*SQLiteManager, error) {
	if cfg == nil {
		cfg = DefaultSQLiteConfig("./muster.db")
	}

	dsn := cfg.Path
	if dsn == "" {
		return nil, fmt.Errorf("database path is required")
	}

	dsn += "?_journal_mode=WAL&_foreign_keys=on&_timeout=5000"

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	sm := &SQLiteManager{
		db:  db,
		log: log,
		cfg: cfg,
	}

	return sm, nil
}

// Close closes the database connection
func (sm *SQLiteManager) Close() error {
	if sm.db == nil {
		return nil
	}
	return sm.db.Close()
}

// Ping verifies the database connection is alive
func (sm *SQLiteManager) Ping() error {
	if sm.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return sm.db.Ping()
}

// BeginTx starts a new database transaction
func (sm *SQLiteManager) BeginTx(ctx context.Context, opts *sql.TxOptions) (Transaction, error) {
	if sm.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	tx, err := sm.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &sqliteTransaction{tx: tx}, nil
}

// Stats returns database statistics
func (sm *SQLiteManager) Stats() sql.DBStats {
	if sm.db == nil {
		return sql.DBStats{}
	}
	return sm.db.Stats()
}

// GetDB returns the underlying *sql.DB
func (sm *SQLiteManager) GetDB() *sql.DB {
	return sm.db
}

// sqliteTransaction implements the Transaction interface
type sqliteTransaction struct {
	tx *sql.Tx
}

// Commit commits the transaction
func (st *sqliteTransaction) Commit() error {
	return st.tx.Commit()
}

// Rollback rolls back the transaction
func (st *sqliteTransaction) Rollback() error {
	return st.tx.Rollback()
}

// GetTx returns the underlying sql.Tx
func (st *sqliteTransaction) GetTx() *sql.Tx {
	return st.tx
}
