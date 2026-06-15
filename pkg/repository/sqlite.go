// Package repository provides database storage abstractions and SQLite implementation.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested entity is not found.
type ErrNotFound struct {
	Entity string
	ID     int64
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("%s with id %d not found", e.Entity, e.ID)
}

// ListOptions provides pagination options for list queries.
type ListOptions struct {
	Limit  int
	Offset int
}

// Alias represents a command alias stored in the repository.
type Alias struct {
	ID          int64
	Name        string
	Description string
	InventoryID int64
	ModuleID    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Audit represents an audit log entry stored in the repository.
type Audit struct {
	ID         int64
	Action     string
	EntityType string
	EntityID   int64
	UserID     string
	Data       string
	CreatedAt  time.Time
}

// Fingerprint represents a spec fingerprint stored in the repository.
type Fingerprint struct {
	ID        int64
	Hash      string
	SpecID    int64
	Algorithm string
	CreatedAt time.Time
}

// Inventory represents a spec inventory stored in the repository.
type Inventory struct {
	ID          int64
	Name        string
	Description string
	Path        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RateLimit represents a rate limit entry stored in the repository.
type RateLimit struct {
	ID        int64
	Key       string
	Count     int64
	Window    time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Store is the primary data store interface.
type Store interface {
	// Close shuts down the store and releases resources.
	Close() error
	// DB returns the underlying database connection.
	DB() *sql.DB
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string
}

// NewSQLiteStore opens or creates a SQLite database at the given path.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	store := &SQLiteStore{
		db:   db,
		path: path,
	}

	return store, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying database connection.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Exec executes a SQL statement within a context.
func (s *SQLiteStore) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

// Query executes a query and returns rows.
func (s *SQLiteStore) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query expected to return at most one row.
func (s *SQLiteStore) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}
