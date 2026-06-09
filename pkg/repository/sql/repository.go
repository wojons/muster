// Package sql provides SQL-backed repository implementations.
// This file provides the shared types needed by the repository packages.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Driver represents a SQL driver type.
type Driver string

const (
	// DriverSQLite is the SQLite driver identifier.
	DriverSQLite Driver = "sqlite3"
)

// sqlRepository is the shared base for SQL-backed repository implementations.
type sqlRepository struct {
	db *sql.DB
}

// QuerySet represents a collection of SQL queries for a repository.
type QuerySet struct {
	Get    string
	List   string
	Insert string
	Update string
	Delete string

	// Alias queries
	AliasCreate          string
	AliasGetByID         string
	AliasGetByName       string
	AliasGetByInventory  string
	AliasGetByModule     string
	AliasUpdate          string
	AliasDelete          string
	AliasDeleteByInventory string

	// Audit queries
	AuditCreate    string
	AuditGetByID   string
	AuditGetByEntity string
	AuditList      string
	AuditCount     string
	AuditCleanup   string

	// Fingerprint queries
	FingerprintCreate        string
	FingerprintGetByID       string
	FingerprintGetBySpec     string
	FingerprintGetByHash     string
	FingerprintUpdate        string
	FingerprintDelete        string
	FingerprintDeleteBySpec  string
	FingerprintExists        string

	// Inventory queries
	InventoryCreate       string
	InventoryGetByID      string
	InventoryGetByAlias   string
	InventoryGetByModule  string
	InventoryUpdate       string
	InventoryDelete       string
	InventoryList         string
	InventoryCount        string
	InventorySearch       string

	// RateLimit queries
	RateLimitCreate      string
	RateLimitGetByID     string
	RateLimitGetByKey    string
	RateLimitUpdate      string
	RateLimitDelete      string
	RateLimitDeleteByKey string
	RateLimitCleanup     string
	RateLimitIncrement   string
}

// NewRepository creates a new SQL repository base.
func NewRepository(db *sql.DB, driver Driver) *sqlRepository {
	_ = driver
	return &sqlRepository{db: db}
}

// GetQuerySet returns the appropriate query set for the given driver.
func GetQuerySet(driver Driver) (*QuerySet, error) {
	if driver != DriverSQLite {
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
	return &QuerySet{}, nil
}

// now returns the current time in UTC.
func now() time.Time {
	return time.Now().UTC()
}

// DB returns the underlying database connection.
func (r *sqlRepository) DB() *sql.DB {
	return r.db
}

// exec executes a SQL statement that doesn't return rows.
func (r *sqlRepository) exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.ExecContext(ctx, query, args...)
}

// query executes a query that returns rows.
func (r *sqlRepository) query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.QueryContext(ctx, query, args...)
}

// queryRow executes a query expected to return at most one row.
func (r *sqlRepository) queryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return r.db.QueryRowContext(ctx, query, args...)
}

// NewSQLRepository creates a new SQL repository base.
func NewSQLRepository(db *sql.DB) *sqlRepository {
	return &sqlRepository{db: db}
}
