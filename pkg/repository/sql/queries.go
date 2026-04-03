// Package sql provides SQL repository implementations for the Muster database.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Driver represents a supported database driver.
type Driver string

const (
	DriverSQLite     Driver = "sqlite3"
	DriverPostgreSQL Driver = "postgres"
	DriverMySQL      Driver = "mysql"
)

// QuerySet contains SQL queries for a specific driver.
type QuerySet struct {
	// Inventory queries
	InventoryCreate      string
	InventoryGetByID     string
	InventoryGetByAlias  string
	InventoryGetByModule string
	InventoryUpdate      string
	InventoryDelete      string
	InventoryList        string
	InventoryCount       string
	InventorySearch      string

	// Alias queries
	AliasCreate            string
	AliasGetByID           string
	AliasGetByName         string
	AliasGetByInventory    string
	AliasGetByModule       string
	AliasUpdate            string
	AliasDelete            string
	AliasDeleteByInventory string

	// Module queries
	ModuleCreate           string
	ModuleGetByID          string
	ModuleGetByPath        string
	ModuleGetByInventory   string
	ModuleUpdate           string
	ModuleDelete           string
	ModuleGetOrphans       string
	ModuleCountByInventory string

	// Spec queries
	SpecCreate         string
	SpecGetByID        string
	SpecGetByAlias     string
	SpecGetByModule    string
	SpecUpdate         string
	SpecDelete         string
	SpecDeleteByAlias  string
	SpecDeleteByModule string

	// Fingerprint queries
	FingerprintCreate       string
	FingerprintGetByID      string
	FingerprintGetBySpec    string
	FingerprintGetByHash    string
	FingerprintUpdate       string
	FingerprintDelete       string
	FingerprintDeleteBySpec string
	FingerprintExists       string

	// Audit queries
	AuditCreate      string
	AuditGetByID     string
	AuditGetByEntity string
	AuditList        string
	AuditCount       string
	AuditCleanup     string

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

// sqliteQueries defines queries for SQLite.
var sqliteQueries = &QuerySet{
	// Inventory queries
	InventoryCreate:      "INSERT INTO inventories (name, description, path) VALUES (?, ?, ?)",
	InventoryGetByID:     "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE id = ?",
	InventoryGetByAlias:  "SELECT i.id, i.name, i.description, i.path, i.created_at, i.updated_at FROM inventories i JOIN aliases a ON i.id = a.inventory_id WHERE a.name = ?",
	InventoryGetByModule: "SELECT i.id, i.name, i.description, i.path, i.created_at, i.updated_at FROM inventories i JOIN modules m ON i.id = m.inventory_id WHERE m.id = ?",
	InventoryUpdate:      "UPDATE inventories SET name = ?, description = ?, path = ?, updated_at = ? WHERE id = ?",
	InventoryDelete:      "DELETE FROM inventories WHERE id = ?",
	InventoryList:        "SELECT id, name, description, path, created_at, updated_at FROM inventories ORDER BY id LIMIT ? OFFSET ?",
	InventoryCount:       "SELECT COUNT(*) FROM inventories",
	InventorySearch:      "SELECT id, name, description, path, created_at, updated_at FROM inventories WHERE name LIKE ? OR description LIKE ? ORDER BY id LIMIT ? OFFSET ?",

	// Alias queries
	AliasCreate:            "INSERT INTO aliases (name, description, inventory_id, module_id) VALUES (?, ?, ?, ?)",
	AliasGetByID:           "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE id = ?",
	AliasGetByName:         "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE name = ?",
	AliasGetByInventory:    "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE inventory_id = ? ORDER BY name LIMIT ? OFFSET ?",
	AliasGetByModule:       "SELECT id, name, description, inventory_id, module_id, created_at, updated_at FROM aliases WHERE module_id = ? ORDER BY name LIMIT ? OFFSET ?",
	AliasUpdate:            "UPDATE aliases SET name = ?, description = ?, inventory_id = ?, module_id = ?, updated_at = ? WHERE id = ?",
	AliasDelete:            "DELETE FROM aliases WHERE id = ?",
	AliasDeleteByInventory: "DELETE FROM aliases WHERE inventory_id = ?",

	// Module queries
	ModuleCreate:           "INSERT INTO modules (name, path, language, inventory_id) VALUES (?, ?, ?, ?)",
	ModuleGetByID:          "SELECT id, name, path, language, inventory_id, created_at, updated_at FROM modules WHERE id = ?",
	ModuleGetByPath:        "SELECT id, name, path, language, inventory_id, created_at, updated_at FROM modules WHERE path = ?",
	ModuleGetByInventory:   "SELECT id, name, path, language, inventory_id, created_at, updated_at FROM modules WHERE inventory_id = ? ORDER BY name LIMIT ? OFFSET ?",
	ModuleUpdate:           "UPDATE modules SET name = ?, path = ?, language = ?, inventory_id = ?, updated_at = ? WHERE id = ?",
	ModuleDelete:           "DELETE FROM modules WHERE id = ?",
	ModuleGetOrphans:       "SELECT id, name, path, language, inventory_id, created_at, updated_at FROM modules WHERE inventory_id IS NULL OR inventory_id NOT IN (SELECT id FROM inventories)",
	ModuleCountByInventory: "SELECT COUNT(*) FROM modules WHERE inventory_id = ?",

	// Spec queries
	SpecCreate:         "INSERT INTO specs (name, content, fingerprint) VALUES (?, ?, ?)",
	SpecGetByID:        "SELECT id, name, content, fingerprint, created_at, updated_at FROM specs WHERE id = ?",
	SpecGetByAlias:     "SELECT s.id, s.name, s.content, s.fingerprint, s.created_at, s.updated_at FROM specs s JOIN aliases a ON s.id = a.spec_id WHERE a.id = ? ORDER BY s.name LIMIT ? OFFSET ?",
	SpecGetByModule:    "SELECT s.id, s.name, s.content, s.fingerprint, s.created_at, s.updated_at FROM specs s JOIN modules m ON s.id = m.spec_id WHERE m.id = ? ORDER BY s.name LIMIT ? OFFSET ?",
	SpecUpdate:         "UPDATE specs SET name = ?, content = ?, fingerprint = ?, updated_at = ? WHERE id = ?",
	SpecDelete:         "DELETE FROM specs WHERE id = ?",
	SpecDeleteByAlias:  "DELETE FROM specs WHERE id IN (SELECT spec_id FROM aliases WHERE id = ?)",
	SpecDeleteByModule: "DELETE FROM specs WHERE id IN (SELECT spec_id FROM modules WHERE id = ?)",

	// Fingerprint queries
	FingerprintCreate:       "INSERT INTO fingerprints (hash, spec_id, algorithm) VALUES (?, ?, ?)",
	FingerprintGetByID:      "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE id = ?",
	FingerprintGetBySpec:    "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE spec_id = ?",
	FingerprintGetByHash:    "SELECT id, hash, spec_id, algorithm, created_at FROM fingerprints WHERE hash = ?",
	FingerprintUpdate:       "UPDATE fingerprints SET hash = ?, spec_id = ?, algorithm = ? WHERE id = ?",
	FingerprintDelete:       "DELETE FROM fingerprints WHERE id = ?",
	FingerprintDeleteBySpec: "DELETE FROM fingerprints WHERE spec_id = ?",
	FingerprintExists:       "SELECT EXISTS(SELECT 1 FROM fingerprints WHERE hash = ?)",

	// Audit queries
	AuditCreate:      "INSERT INTO audit_logs (action, entity_type, entity_id, user_id, data) VALUES (?, ?, ?, ?, ?)",
	AuditGetByID:     "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs WHERE id = ?",
	AuditGetByEntity: "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs WHERE entity_type = ? AND entity_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
	AuditList:        "SELECT id, action, entity_type, entity_id, user_id, data, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?",
	AuditCount:       "SELECT COUNT(*) FROM audit_logs",
	AuditCleanup:     "DELETE FROM audit_logs WHERE created_at < ?",

	// RateLimit queries
	RateLimitCreate:      "INSERT INTO rate_limits (key, count, window, expires_at) VALUES (?, ?, ?, ?)",
	RateLimitGetByID:     "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE id = ?",
	RateLimitGetByKey:    "SELECT id, key, count, window, expires_at, created_at FROM rate_limits WHERE key = ?",
	RateLimitUpdate:      "UPDATE rate_limits SET key = ?, count = ?, window = ?, expires_at = ? WHERE id = ?",
	RateLimitDelete:      "DELETE FROM rate_limits WHERE id = ?",
	RateLimitDeleteByKey: "DELETE FROM rate_limits WHERE key = ?",
	RateLimitCleanup:     "DELETE FROM rate_limits WHERE expires_at < ?",
	RateLimitIncrement:   "INSERT INTO rate_limits (key, count, window, expires_at) VALUES (?, 1, ?, ?) ON CONFLICT(key) DO UPDATE SET count = count + 1",
}

// postgresQueries defines queries for PostgreSQL.
var postgresQueries = &QuerySet{
	// Similar to sqlite but with $N placeholders
	// This would be populated with PostgreSQL-specific queries
	// For now, we focus on SQLite as the primary driver
}

// mysqlQueries defines queries for MySQL.
var mysqlQueries = &QuerySet{
	// Similar to sqlite but with different syntax
	// This would be populated with MySQL-specific queries
	// For now, we focus on SQLite as the primary driver
}

// querySets holds queries for each supported driver.
var querySets = map[Driver]*QuerySet{
	DriverSQLite: sqliteQueries,
}

// GetQuerySet returns the query set for the specified driver.
func GetQuerySet(driver Driver) (*QuerySet, error) {
	qs, ok := querySets[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
	return qs, nil
}

// now returns the current time (helper for mocking in tests).
func now() time.Time {
	return time.Now().UTC()
}

// sqlRepository provides the base implementation for SQL repositories.
type sqlRepository struct {
	db     *sql.DB
	driver Driver
}

// NewRepository creates a new base SQL repository.
func NewRepository(db *sql.DB, driver Driver) *sqlRepository {
	return &sqlRepository{
		db:     db,
		driver: driver,
	}
}

// exec executes a query without returning rows.
func (r *sqlRepository) exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.ExecContext(ctx, query, args...)
}

// queryRow executes a query that returns at most one row.
func (r *sqlRepository) queryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return r.db.QueryRowContext(ctx, query, args...)
}

// query executes a query that returns multiple rows.
func (r *sqlRepository) query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.QueryContext(ctx, query, args...)
}

// mapError maps database errors to repository errors.
func mapError(err error) error {
	if err == sql.ErrNoRows {
		return err
	}
	return err
}
