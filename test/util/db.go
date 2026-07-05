// Package util provides shared test utilities for integration and end-to-end tests.
// This replaces the missing test/util package from the corrupted git repo.
package util

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestDB creates an in-memory SQLite database for testing and returns it
// along with a cleanup function.
func TestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("failed to ping test database: %v", err)
	}

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Logf("warning: failed to close test database: %v", err)
		}
	}

	return db, cleanup
}

// MustExec executes a SQL statement and fails the test on error.
func MustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("failed to execute %q: %v", query, err)
	}
}

// MustQueryRow executes a query expected to return one row and fails on error.
func MustQueryRow(t *testing.T, db *sql.DB, query string, args ...interface{}) *sql.Row {
	t.Helper()
	return db.QueryRowContext(context.Background(), query, args...)
}

// Logf is a convenience wrapper for formatted test logging.
func Logf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	t.Logf(format, args...)
}

// Fatalf is a convenience wrapper for formatted test fatal.
func Fatalf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	t.Fatalf(format, args...)
}

// Ensure type is used
var _ = fmt.Sprintf
