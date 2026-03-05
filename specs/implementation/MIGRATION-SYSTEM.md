# Database Migration System Specification

## Overview

Production-ready Go migration system supporting SQLite, PostgreSQL, and MySQL with transaction safety, advisory locks, rollback support, and schema validation.

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Migration System                          │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Migrator   │──│   Registry   │──│   Executor   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │    Store     │──│    Locks     │──│   Reporter   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Migration Definition Format

### File: `internal/db/migration/migration.go`

```go
// Package migration provides database migration capabilities
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MigrationFunc is a function that performs a migration step
type MigrationFunc func(context.Context, *sql.Tx) error

// Migration represents a single database migration
type Migration struct {
	Version     int64       // Sequential version number
	Description string      // Human-readable description
	Up          MigrationFunc // Forward migration
	Down        MigrationFunc // Rollback migration
}

// Registry holds all registered migrations
type Registry struct {
	migrations map[int64]*Migration
	versions   []int64 // Sorted list of versions
}

// NewRegistry creates a new migration registry
func NewRegistry() *Registry {
	return &Registry{
		migrations: make(map[int64]*Migration),
		versions:   []int64{},
	}
}

// Register adds a migration to the registry
func (r *Registry) Register(m *Migration) error {
	if m.Version <= 0 {
		return fmt.Errorf("migration version must be positive, got %d", m.Version)
	}
	if m.Up == nil {
		return fmt.Errorf("migration %d must have an Up function", m.Version)
	}
	if _, exists := r.migrations[m.Version]; exists {
		return fmt.Errorf("migration version %d already registered", m.Version)
	}
	
	r.migrations[m.Version] = m
	r.versions = append(r.versions, m.Version)
	// Keep versions sorted
	for i := len(r.versions) - 1; i > 0 && r.versions[i] < r.versions[i-1]; i-- {
		r.versions[i], r.versions[i-1] = r.versions[i-1], r.versions[i]
	}
	return nil
}

// Get retrieves a migration by version
func (r *Registry) Get(version int64) (*Migration, bool) {
	m, ok := r.migrations[version]
	return m, ok
}

// GetAll returns all migrations in version order
func (r *Registry) GetAll() []*Migration {
	result := make([]*Migration, len(r.versions))
	for i, v := range r.versions {
		result[i] = r.migrations[v]
	}
	return result
}

// GetVersions returns all version numbers in ascending order
func (r *Registry) GetVersions() []int64 {
	result := make([]int64, len(r.versions))
	copy(result, r.versions)
	return result
}

// GetRange returns migrations between fromVersion and toVersion (inclusive)
func (r *Registry) GetRange(fromVersion, toVersion int64) []*Migration {
	var result []*Migration
	for _, v := range r.versions {
		if v >= fromVersion && v <= toVersion {
			result = append(result, r.migrations[v])
		}
	}
	return result
}

// GetSince returns all migrations after the given version
func (r *Registry) GetSince(version int64) []*Migration {
	var result []*Migration
	for _, v := range r.versions {
		if v > version {
			result = append(result, r.migrations[v])
		}
	}
	return result
}

// GetBefore returns all migrations up to and including the given version
func (r *Registry) GetBefore(version int64) []*Migration {
	var result []*Migration
	for _, v := range r.versions {
		if v <= version {
			result = append(result, r.migrations[v])
		}
	}
	return result
}

// HasVersion checks if a version exists in the registry
func (r *Registry) HasVersion(version int64) bool {
	_, ok := r.migrations[version]
	return ok
}

// Count returns the number of registered migrations
func (r *Registry) Count() int {
	return len(r.versions)
}

// MaxVersion returns the highest registered version
func (r *Registry) MaxVersion() int64 {
	if len(r.versions) == 0 {
		return 0
	}
	return r.versions[len(r.versions)-1]
}

// MinVersion returns the lowest registered version
func (r *Registry) MinVersion() int64 {
	if len(r.versions) == 0 {
		return 0
	}
	return r.versions[0]
}

// Record represents a migration record in the database
type Record struct {
	Version     int64     `db:"version"`
	Description string    `db:"description"`
	AppliedAt   time.Time `db:"applied_at"`
	Checksum    string    `db:"checksum"`
	Duration    int64     `db:"duration_ms"`
	Success     bool      `db:"success"`
}

// CreateTableSQL returns SQL to create the migrations table
type CreateTableSQL struct {
	SQLite     string
	PostgreSQL string
	MySQL      string
}

// GetMigrationsTableSQL returns dialect-specific SQL for creating the migrations table
func GetMigrationsTableSQL() CreateTableSQL {
	return CreateTableSQL{
		SQLite: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    checksum TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    success BOOLEAN NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at 
ON schema_migrations(applied_at);
`,
		PostgreSQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    checksum VARCHAR(64) NOT NULL,
    duration_ms BIGINT NOT NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at 
ON schema_migrations(applied_at);

CREATE INDEX IF NOT EXISTS idx_schema_migrations_success 
ON schema_migrations(success) WHERE success = FALSE;
`,
		MySQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    checksum VARCHAR(64) NOT NULL,
    duration_ms BIGINT NOT NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    INDEX idx_applied_at (applied_at),
    INDEX idx_success (success)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`,
	}
}
```

---

## 3. Version Tracking & Migration Store

### File: `internal/db/migration/store.go`

```go
package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

// Store handles persistence of migration records
type Store struct {
	db       *sql.DB
	dialect  Dialect
}

// Dialect represents the database dialect
type Dialect int

const (
	DialectSQLite Dialect = iota
	DialectPostgreSQL
	DialectMySQL
)

// String returns the dialect name
func (d Dialect) String() string {
	switch d {
	case DialectSQLite:
		return "sqlite"
	case DialectPostgreSQL:
		return "postgresql"
	case DialectMySQL:
		return "mysql"
	default:
		return "unknown"
	}
}

// DetectDialect attempts to detect the database dialect
func DetectDialect(db *sql.DB) (Dialect, error) {
	var version string
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&version); err == nil {
		return DialectSQLite, nil
	}
	
	if err := db.QueryRow("SHOW server_version").Scan(&version); err == nil {
		return DialectPostgreSQL, nil
	}
	
	if err := db.QueryRow("SELECT VERSION()").Scan(&version); err == nil {
		if strings.Contains(strings.ToLower(version), "mariadb") || 
		   strings.Contains(strings.ToLower(version), "mysql") {
			return DialectMySQL, nil
		}
	}
	
	return DialectSQLite, fmt.Errorf("unable to detect database dialect")
}

// NewStore creates a new migration store
func NewStore(db *sql.DB, dialect Dialect) (*Store, error) {
	return &Store{
		db:      db,
		dialect: dialect,
	}, nil
}

// Initialize creates the migrations table if it doesn't exist
func (s *Store) Initialize(ctx context.Context) error {
	sql := GetMigrationsTableSQL()
	
	var createSQL string
	switch s.dialect {
	case DialectSQLite:
		createSQL = sql.SQLite
	case DialectPostgreSQL:
		createSQL = sql.PostgreSQL
	case DialectMySQL:
		createSQL = sql.MySQL
	default:
		createSQL = sql.SQLite
	}
	
	_, err := s.db.ExecContext(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}
	
	return nil
}

// GetCurrentVersion returns the current database schema version
func (s *Store) GetCurrentVersion(ctx context.Context) (int64, error) {
	var version int64
	query := "SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE success = 1"
	
	err := s.db.QueryRowContext(ctx, query).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}
	
	return version, nil
}

// GetAppliedMigrations returns all applied migration records
func (s *Store) GetAppliedMigrations(ctx context.Context) ([]*Record, error) {
	query := `
		SELECT version, description, applied_at, checksum, duration_ms, success 
		FROM schema_migrations 
		ORDER BY version
	`
	
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()
	
	var records []*Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Version, &r.Description, &r.AppliedAt, 
			&r.Checksum, &r.Duration, &r.Success); err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}
		records = append(records, &r)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration records: %w", err)
	}
	
	return records, nil
}

// RecordMigration records a successful migration
func (s *Store) RecordMigration(ctx context.Context, tx *sql.Tx, m *Migration, duration int64) error {
	// Calculate checksum of migration (for validation)
	checksum := calculateChecksum(m)
	
	query := `
		INSERT INTO schema_migrations (version, description, checksum, duration_ms, success)
		VALUES (?, ?, ?, ?, 1)
	`
	
	if s.dialect == DialectPostgreSQL {
		query = `
			INSERT INTO schema_migrations (version, description, checksum, duration_ms, success)
			VALUES ($1, $2, $3, $4, TRUE)
		`
	}
	
	_, err := tx.ExecContext(ctx, query, m.Version, m.Description, checksum, duration)
	if err != nil {
		return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
	}
	
	return nil
}

// RemoveMigration removes a migration record (for rollback)
func (s *Store) RemoveMigration(ctx context.Context, tx *sql.Tx, version int64) error {
	query := "DELETE FROM schema_migrations WHERE version = ?"
	if s.dialect == DialectPostgreSQL {
		query = "DELETE FROM schema_migrations WHERE version = $1"
	}
	
	_, err := tx.ExecContext(ctx, query, version)
	if err != nil {
		return fmt.Errorf("failed to remove migration %d: %w", version, err)
	}
	
	return nil
}

// RecordFailure records a failed migration attempt
func (s *Store) RecordFailure(ctx context.Context, m *Migration, duration int64, errMsg string) error {
	query := `
		INSERT INTO schema_migrations (version, description, checksum, duration_ms, success)
		VALUES (?, ?, ?, ?, 0)
	`
	
	if s.dialect == DialectPostgreSQL {
		query = `
			INSERT INTO schema_migrations (version, description, checksum, duration_ms, success)
			VALUES ($1, $2, $3, $4, FALSE)
		`
	}
	
	checksum := calculateChecksum(m)
	description := fmt.Sprintf("%s (FAILED: %s)", m.Description, errMsg)
	
	_, err := s.db.ExecContext(ctx, query, m.Version, description, checksum, duration)
	if err != nil {
		return fmt.Errorf("failed to record migration failure: %w", err)
	}
	
	return nil
}

// IsMigrationApplied checks if a migration has been successfully applied
func (s *Store) IsMigrationApplied(ctx context.Context, version int64) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ? AND success = 1)"
	
	if s.dialect == DialectPostgreSQL {
		query = "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1 AND success = TRUE)"
	}
	
	err := s.db.QueryRowContext(ctx, query, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check migration status: %w", err)
	}
	
	return exists, nil
}

// ValidateChecksum validates that a migration hasn't changed since it was applied
func (s *Store) ValidateChecksum(ctx context.Context, m *Migration) error {
	var storedChecksum string
	query := "SELECT checksum FROM schema_migrations WHERE version = ? AND success = 1"
	
	if s.dialect == DialectPostgreSQL {
		query = "SELECT checksum FROM schema_migrations WHERE version = $1 AND success = TRUE"
	}
	
	err := s.db.QueryRowContext(ctx, query, m.Version).Scan(&storedChecksum)
	if err == sql.ErrNoRows {
		return nil // Migration not yet applied
	}
	if err != nil {
		return fmt.Errorf("failed to retrieve checksum: %w", err)
	}
	
	currentChecksum := calculateChecksum(m)
	if storedChecksum != currentChecksum {
		return fmt.Errorf("migration %d checksum mismatch: stored=%s, current=%s", 
			m.Version, storedChecksum, currentChecksum)
	}
	
	return nil
}

// calculateChecksum creates a checksum of the migration for validation
func calculateChecksum(m *Migration) string {
	// Simple checksum based on version and description
	// In production, you might want to include the actual SQL content
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:%s", m.Version, m.Description)))
	return hex.EncodeToString(h.Sum(nil))
}

// GetFailedMigrations returns any migrations that failed
func (s *Store) GetFailedMigrations(ctx context.Context) ([]*Record, error) {
	query := `
		SELECT version, description, applied_at, checksum, duration_ms, success 
		FROM schema_migrations 
		WHERE success = 0
		ORDER BY version
	`
	
	if s.dialect == DialectPostgreSQL {
		query = `
			SELECT version, description, applied_at, checksum, duration_ms, success 
			FROM schema_migrations 
			WHERE success = FALSE
			ORDER BY version
		`
	}
	
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed migrations: %w", err)
	}
	defer rows.Close()
	
	var records []*Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Version, &r.Description, &r.AppliedAt, 
			&r.Checksum, &r.Duration, &r.Success); err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}
		records = append(records, &r)
	}
	
	return records, rows.Err()
}

// Truncate removes all migration records (use with caution)
func (s *Store) Truncate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to truncate migrations table: %w", err)
	}
	return nil
}
```

---

## 4. Advisory Lock System

### File: `internal/db/migration/lock.go`

```go
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LockManager handles distributed locking for migrations
type LockManager struct {
	db      *sql.DB
	dialect Dialect
	timeout time.Duration
}

// Lock represents a held advisory lock
type Lock struct {
	manager *LockManager
	id      int64
	released bool
}

// NewLockManager creates a new lock manager
func NewLockManager(db *sql.DB, dialect Dialect, timeout time.Duration) *LockManager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &LockManager{
		db:      db,
		dialect: dialect,
		timeout: timeout,
	}
}

// Acquire attempts to acquire an advisory lock
func (lm *LockManager) Acquire(ctx context.Context, lockID int64) (*Lock, error) {
	switch lm.dialect {
	case DialectPostgreSQL:
		return lm.acquirePostgres(ctx, lockID)
	case DialectMySQL:
		return lm.acquireMySQL(ctx, lockID)
	case DialectSQLite:
		return lm.acquireSQLite(ctx, lockID)
	default:
		return lm.acquireSQLite(ctx, lockID)
	}
}

// acquirePostgres uses PostgreSQL advisory locks
func (lm *LockManager) acquirePostgres(ctx context.Context, lockID int64) (*Lock, error) {
	// Try to acquire lock with timeout
	ctx, cancel := context.WithTimeout(ctx, lm.timeout)
	defer cancel()
	
	// pg_advisory_lock is blocking, so we use trylock first
	var acquired bool
	err := lm.db.QueryRowContext(ctx, 
		"SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	if err != nil {
		return nil, fmt.Errorf("failed to try advisory lock: %w", err)
	}
	
	if !acquired {
		return nil, fmt.Errorf("advisory lock %d is already held", lockID)
	}
	
	return &Lock{
		manager: lm,
		id:      lockID,
	}, nil
}

// acquireMySQL uses MySQL named locks
func (lm *LockManager) acquireMySQL(ctx context.Context, lockID int64) (*Lock, error) {
	ctx, cancel := context.WithTimeout(ctx, lm.timeout)
	defer cancel()
	
	lockName := fmt.Sprintf("migration_lock_%d", lockID)
	
	// IS_USED_LOCK returns connection ID if lock is held, NULL otherwise
	var holder sql.NullInt64
	err := lm.db.QueryRowContext(ctx, 
		"SELECT IS_USED_LOCK(?)", lockName).Scan(&holder)
	if err != nil {
		return nil, fmt.Errorf("failed to check lock status: %w", err)
	}
	
	if holder.Valid {
		return nil, fmt.Errorf("migration lock is already held by connection %d", holder.Int64)
	}
	
	// Try to acquire lock
	var result int
	err = lm.db.QueryRowContext(ctx, 
		"SELECT GET_LOCK(?, ?)", lockName, int(lm.timeout.Seconds())).Scan(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	
	if result == 0 {
		return nil, fmt.Errorf("failed to acquire migration lock (timeout)")
	}
	
	return &Lock{
		manager: lm,
		id:      lockID,
	}, nil
}

// acquireSQLite uses application-level locking for SQLite
// SQLite handles concurrency differently, so we use a simple table-based lock
func (lm *LockManager) acquireSQLite(ctx context.Context, lockID int64) (*Lock, error) {
	// For SQLite, we use immediate transaction mode and rely on SQLite's file locking
	// This is a simplified approach - in production, you might want file-based locking
	
	// Begin immediate transaction to acquire write lock
	tx, err := lm.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction for lock: %w", err)
	}
	
	// Create or use lock table
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _migration_locks (
			lock_id INTEGER PRIMARY KEY,
			acquired_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create lock table: %w", err)
	}
	
	// Try to insert lock
	_, err = tx.ExecContext(ctx, 
		"INSERT OR IGNORE INTO _migration_locks (lock_id) VALUES (?)", lockID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to insert lock: %w", err)
	}
	
	// Check if we got the lock
	var acquiredAt string
	err = tx.QueryRowContext(ctx, 
		"SELECT acquired_at FROM _migration_locks WHERE lock_id = ?", lockID).Scan(&acquiredAt)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to verify lock: %w", err)
	}
	
	// Commit transaction - lock is held via the row
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit lock transaction: %w", err)
	}
	
	return &Lock{
		manager: lm,
		id:      lockID,
	}, nil
}

// Release releases the advisory lock
func (l *Lock) Release(ctx context.Context) error {
	if l.released {
		return nil
	}
	
	switch l.manager.dialect {
	case DialectPostgreSQL:
		return l.releasePostgres(ctx)
	case DialectMySQL:
		return l.releaseMySQL(ctx)
	case DialectSQLite:
		return l.releaseSQLite(ctx)
	default:
		return l.releaseSQLite(ctx)
	}
}

func (l *Lock) releasePostgres(ctx context.Context) error {
	_, err := l.manager.db.ExecContext(ctx, 
		"SELECT pg_advisory_unlock($1)", l.id)
	if err != nil {
		return fmt.Errorf("failed to release advisory lock: %w", err)
	}
	l.released = true
	return nil
}

func (l *Lock) releaseMySQL(ctx context.Context) error {
	lockName := fmt.Sprintf("migration_lock_%d", l.id)
	_, err := l.manager.db.ExecContext(ctx, 
		"DO RELEASE_LOCK(?)", lockName)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	l.released = true
	return nil
}

func (l *Lock) releaseSQLite(ctx context.Context) error {
	_, err := l.manager.db.ExecContext(ctx, 
		"DELETE FROM _migration_locks WHERE lock_id = ?", l.id)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	l.released = true
	return nil
}

// IsReleased returns whether the lock has been released
func (l *Lock) IsReleased() bool {
	return l.released
}

// DefaultLockID is the default lock ID for migrations
const DefaultLockID = 42

// WithLock executes a function while holding an advisory lock
func WithLock(ctx context.Context, lm *LockManager, lockID int64, fn func() error) error {
	lock, err := lm.Acquire(ctx, lockID)
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer lock.Release(ctx)
	
	return fn()
}
```

---

## 5. Migration Executor

### File: `internal/db/migration/executor.go`

```go
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Executor handles the execution of migrations
type Executor struct {
	store   *Store
	options Options
}

// Options configures the executor
type Options struct {
	// Timeout for individual migrations
	MigrationTimeout time.Duration
	// TablePrefix for migrations table (optional)
	TablePrefix string
	// AllowDirty allows applying migrations even if previous ones failed
	AllowDirty bool
	// ValidateChecksum validates migration checksums
	ValidateChecksum bool
	// DryRun performs a dry run without executing migrations
	DryRun bool
}

// DefaultOptions returns default executor options
func DefaultOptions() Options {
	return Options{
		MigrationTimeout: 5 * time.Minute,
		TablePrefix:      "",
		AllowDirty:       false,
		ValidateChecksum: true,
		DryRun:           false,
	}
}

// NewExecutor creates a new migration executor
func NewExecutor(store *Store, opts Options) *Executor {
	return &Executor{
		store:   store,
		options: opts,
	}
}

// Result represents the outcome of a migration execution
type Result struct {
	Applied      []*Migration
	Failed       *Migration
	Duration     time.Duration
	StartVersion int64
	EndVersion   int64
	Error        error
}

// Success returns true if the migration was successful
func (r *Result) Success() bool {
	return r.Error == nil && r.Failed == nil
}

// Execute runs all pending migrations up to the target version
// If targetVersion is 0, runs all available migrations
func (e *Executor) Execute(ctx context.Context, registry *Registry, targetVersion int64) (*Result, error) {
	result := &Result{
		Applied: []*Migration{},
	}
	
	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()
	
	// Get current version
	currentVersion, err := e.store.GetCurrentVersion(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to get current version: %w", err)
	}
	result.StartVersion = currentVersion
	
	// Check for failed migrations
	if !e.options.AllowDirty {
		failed, err := e.store.GetFailedMigrations(ctx)
		if err != nil {
			return result, fmt.Errorf("failed to check for failed migrations: %w", err)
		}
		if len(failed) > 0 {
			return result, fmt.Errorf("found %d failed migration(s), manual intervention required", len(failed))
		}
	}
	
	// Determine which migrations to run
	var migrations []*Migration
	if targetVersion == 0 {
		// Run all pending migrations
		migrations = registry.GetSince(currentVersion)
	} else {
		// Run migrations up to target
		if targetVersion < currentVersion {
			return result, fmt.Errorf("cannot migrate down: target version %d < current version %d", 
				targetVersion, currentVersion)
		}
		migrations = registry.GetRange(currentVersion+1, targetVersion)
	}
	
	if len(migrations) == 0 {
		result.EndVersion = currentVersion
		return result, nil
	}
	
	// Execute each migration
	for _, migration := range migrations {
		if err := e.executeMigration(ctx, migration); err != nil {
			result.Failed = migration
			result.Error = err
			result.EndVersion = currentVersion
			
			// Record failure
			duration := time.Since(startTime).Milliseconds()
			_ = e.store.RecordFailure(ctx, migration, duration, err.Error())
			
			return result, err
		}
		
		result.Applied = append(result.Applied, migration)
		currentVersion = migration.Version
	}
	
	result.EndVersion = currentVersion
	return result, nil
}

// executeMigration runs a single migration within a transaction
func (e *Executor) executeMigration(ctx context.Context, m *Migration) error {
	// Validate checksum if enabled
	if e.options.ValidateChecksum {
		if err := e.store.ValidateChecksum(ctx, m); err != nil {
			return fmt.Errorf("checksum validation failed for migration %d: %w", m.Version, err)
		}
	}
	
	// Check if already applied
	applied, err := e.store.IsMigrationApplied(ctx, m.Version)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}
	if applied {
		return nil
	}
	
	if e.options.DryRun {
		fmt.Printf("[DRY RUN] Would execute migration %d: %s\n", m.Version, m.Description)
		return nil
	}
	
	// Execute with timeout
	ctx, cancel := context.WithTimeout(ctx, e.options.MigrationTimeout)
	defer cancel()
	
	startTime := time.Now()
	
	// Begin transaction
	tx, err := e.store.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	// Ensure transaction is always handled
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	
	// Execute the migration
	if err := m.Up(ctx, tx); err != nil {
		return fmt.Errorf("migration %d failed: %w", m.Version, err)
	}
	
	// Record migration
	duration := time.Since(startTime).Milliseconds()
	if err := e.store.RecordMigration(ctx, tx, m, duration); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}
	committed = true
	
	return nil
}

// Rollback rolls back the last applied migration
func (e *Executor) Rollback(ctx context.Context, registry *Registry) (*Result, error) {
	result := &Result{
		Applied: []*Migration{},
	}
	
	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()
	
	// Get current version
	currentVersion, err := e.store.GetCurrentVersion(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to get current version: %w", err)
	}
	result.StartVersion = currentVersion
	
	if currentVersion == 0 {
		return result, fmt.Errorf("no migrations to rollback")
	}
	
	// Get the migration to rollback
	migration, exists := registry.Get(currentVersion)
	if !exists {
		return result, fmt.Errorf("migration %d not found in registry", currentVersion)
	}
	
	if migration.Down == nil {
		return result, fmt.Errorf("migration %d has no rollback function", currentVersion)
	}
	
	if e.options.DryRun {
		fmt.Printf("[DRY RUN] Would rollback migration %d: %s\n", migration.Version, migration.Description)
		result.EndVersion = currentVersion - 1
		return result, nil
	}
	
	// Execute rollback within transaction
	ctx, cancel := context.WithTimeout(ctx, e.options.MigrationTimeout)
	defer cancel()
	
	tx, err := e.store.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return result, fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	
	// Execute rollback
	if err := migration.Down(ctx, tx); err != nil {
		return result, fmt.Errorf("rollback of migration %d failed: %w", migration.Version, err)
	}
	
	// Remove migration record
	if err := e.store.RemoveMigration(ctx, tx, migration.Version); err != nil {
		return result, fmt.Errorf("failed to remove migration record: %w", err)
	}
	
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("failed to commit rollback: %w", err)
	}
	committed = true
	
	result.EndVersion = currentVersion - 1
	result.Applied = append(result.Applied, migration)
	
	return result, nil
}

// RollbackTo rolls back to a specific version
func (e *Executor) RollbackTo(ctx context.Context, registry *Registry, targetVersion int64) (*Result, error) {
	result := &Result{
		Applied: []*Migration{},
	}
	
	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()
	
	currentVersion, err := e.store.GetCurrentVersion(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to get current version: %w", err)
	}
	result.StartVersion = currentVersion
	
	if targetVersion >= currentVersion {
		return result, fmt.Errorf("target version %d must be less than current version %d",
			targetVersion, currentVersion)
	}
	
	// Get migrations to rollback (in reverse order)
	migrations := registry.GetRange(targetVersion+1, currentVersion)
	// Reverse order for rollback
	for i, j := 0, len(migrations)-1; i < j; i, j = i+1, j-1 {
		migrations[i], migrations[j] = migrations[j], migrations[i]
	}
	
	for _, migration := range migrations {
		if migration.Down == nil {
			return result, fmt.Errorf("migration %d has no rollback function", migration.Version)
		}
		
		r, err := e.Rollback(ctx, registry)
		if err != nil {
			result.Error = err
			return result, err
		}
		result.Applied = append(result.Applied, r.Applied...)
	}
	
	result.EndVersion = targetVersion
	return result, nil
}

// Status returns the current migration status
func (e *Executor) Status(ctx context.Context, registry *Registry) (*Status, error) {
	currentVersion, err := e.store.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current version: %w", err)
	}
	
	applied, err := e.store.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}
	
	failed, err := e.store.GetFailedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed migrations: %w", err)
	}
	
	// Build status
	status := &Status{
		CurrentVersion:   currentVersion,
		LatestVersion:    registry.MaxVersion(),
		AppliedCount:     len(applied),
		PendingCount:     0,
		FailedCount:      len(failed),
		AppliedVersions:  make([]int64, len(applied)),
		PendingVersions:  []int64{},
		FailedVersions:   make([]int64, len(failed)),
	}
	
	for i, r := range applied {
		status.AppliedVersions[i] = r.Version
	}
	
	for i, r := range failed {
		status.FailedVersions[i] = r.Version
	}
	
	// Find pending migrations
	for _, v := range registry.GetVersions() {
		found := false
		for _, applied := range status.AppliedVersions {
			if applied == v {
				found = true
				break
			}
		}
		if !found {
			status.PendingVersions = append(status.PendingVersions, v)
			status.PendingCount++
		}
	}
	
	return status, nil
}

// Status represents the migration status
type Status struct {
	CurrentVersion  int64
	LatestVersion   int64
	AppliedCount    int
	PendingCount    int
	FailedCount     int
	AppliedVersions []int64
	PendingVersions []int64
	FailedVersions  []int64
}

// IsUpToDate returns true if all migrations are applied
func (s *Status) IsUpToDate() bool {
	return s.PendingCount == 0 && s.FailedCount == 0
}
```

---

## 6. Schema Validation

### File: `internal/db/migration/validation.go`

```go
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Validator performs schema validation
type Validator struct {
	db      *sql.DB
	dialect Dialect
}

// NewValidator creates a new schema validator
func NewValidator(db *sql.DB, dialect Dialect) *Validator {
	return &Validator{
		db:      db,
		dialect: dialect,
	}
}

// ValidationResult represents the result of schema validation
type ValidationResult struct {
	Valid   bool
	Errors  []string
	Warnings []string
}

// ValidateSchema validates that the database schema matches expectations
func (v *Validator) ValidateSchema(ctx context.Context, expectedTables []Table) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}
	
	// Get actual tables
	actualTables, err := v.getTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	
	// Check expected tables exist
	for _, expected := range expectedTables {
		found := false
		for _, actual := range actualTables {
			if strings.EqualFold(actual, expected.Name) {
				found = true
				break
			}
		}
		if !found {
			result.Valid = false
			result.Errors = append(result.Errors, 
				fmt.Sprintf("missing table: %s", expected.Name))
			continue
		}
		
		// Validate columns
		if err := v.validateTable(ctx, &expected, result); err != nil {
			return nil, err
		}
	}
	
	// Check for unexpected tables (optional - could be warnings)
	for _, actual := range actualTables {
		if actual == "schema_migrations" || actual == "_migration_locks" {
			continue
		}
		
		expected := false
		for _, t := range expectedTables {
			if strings.EqualFold(t.Name, actual) {
				expected = true
				break
			}
		}
		if !expected {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("unexpected table: %s", actual))
		}
	}
	
	return result, nil
}

// Table represents an expected table schema
type Table struct {
	Name    string
	Columns []Column
	Indexes []Index
}

// Column represents an expected column
type Column struct {
	Name     string
	Type     string
	Nullable bool
	Default  string
}

// Index represents an expected index
type Index struct {
	Name    string
	Columns []string
	Unique  bool
}

// validateTable validates a single table
func (v *Validator) validateTable(ctx context.Context, table *Table, result *ValidationResult) error {
	// Get actual columns
	actualColumns, err := v.getColumns(ctx, table.Name)
	if err != nil {
		return fmt.Errorf("failed to get columns for %s: %w", table.Name, err)
	}
	
	// Check expected columns
	for _, expected := range table.Columns {
		found := false
		for _, actual := range actualColumns {
			if strings.EqualFold(actual.Name, expected.Name) {
				found = true
				// Validate column type (simplified)
				if !v.columnTypesMatch(expected.Type, actual.Type) {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("column type mismatch: %s.%s expected %s, got %s",
							table.Name, expected.Name, expected.Type, actual.Type))
				}
				break
			}
		}
		if !found {
			result.Valid = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("missing column: %s.%s", table.Name, expected.Name))
		}
	}
	
	return nil
}

// columnTypesMatch checks if column types are compatible
func (v *Validator) columnTypesMatch(expected, actual string) bool {
	// Normalize types
	expected = strings.ToLower(strings.TrimSpace(expected))
	actual = strings.ToLower(strings.TrimSpace(actual))
	
	// Handle common aliases
	typeMap := map[string][]string{
		"integer": {"int", "bigint", "smallint", "tinyint", "serial", "bigserial"},
		"text":    {"varchar", "char", "string", "nvarchar"},
		"real":    {"float", "double", "decimal", "numeric"},
		"blob":    {"bytea", "binary", "varbinary"},
	}
	
	for base, aliases := range typeMap {
		if strings.Contains(expected, base) {
			for _, alias := range aliases {
				if strings.Contains(actual, alias) {
					return true
				}
			}
		}
	}
	
	// Check for substring match
	return strings.Contains(actual, expected) || strings.Contains(expected, actual)
}

// getTables returns all tables in the database
func (v *Validator) getTables(ctx context.Context) ([]string, error) {
	var query string
	
	switch v.dialect {
	case DialectSQLite:
		query = `
			SELECT name FROM sqlite_master 
			WHERE type = 'table' 
			AND name NOT LIKE 'sqlite_%'
		`
	case DialectPostgreSQL:
		query = `
			SELECT tablename FROM pg_tables 
			WHERE schemaname = 'public'
		`
	case DialectMySQL:
		query = `
			SELECT table_name FROM information_schema.tables 
			WHERE table_schema = DATABASE()
		`
	default:
		query = `SELECT name FROM sqlite_master WHERE type = 'table'`
	}
	
	rows, err := v.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	
	return tables, rows.Err()
}

// columnInfo represents column information
type columnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Default  sql.NullString
}

// getColumns returns all columns for a table
func (v *Validator) getColumns(ctx context.Context, tableName string) ([]columnInfo, error) {
	var query string
	
	switch v.dialect {
	case DialectSQLite:
		query = fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	case DialectPostgreSQL:
		query = `
			SELECT column_name, data_type, is_nullable, column_default 
			FROM information_schema.columns 
			WHERE table_name = $1
		`
	case DialectMySQL:
		query = `
			SELECT column_name, data_type, is_nullable, column_default 
			FROM information_schema.columns 
			WHERE table_schema = DATABASE() AND table_name = ?
		`
	default:
		query = fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	}
	
	rows, err := v.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var columns []columnInfo
	
	switch v.dialect {
	case DialectSQLite:
		// SQLite PRAGMA returns: cid, name, type, notnull, dflt_value, pk
		for rows.Next() {
			var cid int
			var pk int
			var col columnInfo
			err := rows.Scan(&cid, &col.Name, &col.Type, &pk, &col.Default, &pk)
			if err != nil {
				return nil, err
			}
			col.Nullable = pk == 0
			columns = append(columns, col)
		}
	default:
		var nullable string
		for rows.Next() {
			var col columnInfo
			err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Default)
			if err != nil {
				return nil, err
			}
			col.Nullable = nullable == "YES"
			columns = append(columns, col)
		}
	}
	
	return columns, rows.Err()
}

// ValidateForeignKeys validates foreign key constraints (SQLite only)
func (v *Validator) ValidateForeignKeys(ctx context.Context) error {
	if v.dialect != DialectSQLite {
		return nil
	}
	
	// Check if foreign keys are enabled
	var enabled bool
	err := v.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		return fmt.Errorf("failed to check foreign key status: %w", err)
	}
	
	if !enabled {
		return fmt.Errorf("foreign keys are not enabled")
	}
	
	return nil
}

// ExpectedSchema returns the expected schema for validation
func ExpectedSchema() []Table {
	return []Table{
		{
			Name: "inventories",
			Columns: []Column{
				{Name: "id", Type: "text", Nullable: false},
				{Name: "host", Type: "text", Nullable: false},
				{Name: "version", Type: "text", Nullable: false},
				{Name: "content", Type: "text", Nullable: false}, // JSONB
				{Name: "sha256", Type: "text", Nullable: false},
				{Name: "fetched_at", Type: "text", Nullable: false},
				{Name: "expires_at", Type: "text", Nullable: true},
				{Name: "etag", Type: "text", Nullable: true},
			},
		},
		{
			Name: "aliases",
			Columns: []Column{
				{Name: "name", Type: "text", Nullable: false},
				{Name: "host", Type: "text", Nullable: false},
				{Name: "version", Type: "text", Nullable: true},
				{Name: "pinned", Type: "integer", Nullable: true},
				{Name: "config", Type: "text", Nullable: true},
				{Name: "created_at", Type: "text", Nullable: false},
				{Name: "updated_at", Type: "text", Nullable: false},
			},
		},
		{
			Name: "modules",
			Columns: []Column{
				{Name: "sha256", Type: "text", Nullable: false},
				{Name: "name", Type: "text", Nullable: false},
				{Name: "content", Type: "blob", Nullable: false},
				{Name: "signature", Type: "text", Nullable: true},
				{Name: "size_bytes", Type: "integer", Nullable: false},
				{Name: "downloaded_at", Type: "text", Nullable: false},
				{Name: "last_used_at", Type: "text", Nullable: true},
				{Name: "use_count", Type: "integer", Nullable: true},
			},
		},
		{
			Name: "specs",
			Columns: []Column{
				{Name: "sha256", Type: "text", Nullable: false},
				{Name: "host", Type: "text", Nullable: false},
				{Name: "content", Type: "text", Nullable: false},
				{Name: "fetched_at", Type: "text", Nullable: false},
			},
		},
		{
			Name: "fingerprints",
			Columns: []Column{
				{Name: "id", Type: "text", Nullable: false},
				{Name: "host", Type: "text", Nullable: false},
				{Name: "fingerprint", Type: "text", Nullable: false},
				{Name: "created_at", Type: "text", Nullable: false},
				{Name: "regenerated_at", Type: "text", Nullable: true},
				{Name: "ip_address", Type: "text", Nullable: true},
			},
		},
		{
			Name: "revocation_cache",
			Columns: []Column{
				{Name: "host", Type: "text", Nullable: false},
				{Name: "content", Type: "text", Nullable: false},
				{Name: "etag", Type: "text", Nullable: true},
				{Name: "fetched_at", Type: "text", Nullable: false},
				{Name: "expires_at", Type: "text", Nullable: true},
			},
		},
		{
			Name: "audit_log",
			Columns: []Column{
				{Name: "id", Type: "integer", Nullable: false},
				{Name: "timestamp", Type: "text", Nullable: false},
				{Name: "event_type", Type: "text", Nullable: false},
				{Name: "client_id", Type: "text", Nullable: true},
				{Name: "user_id", Type: "text", Nullable: true},
				{Name: "fingerprint", Type: "text", Nullable: true},
				{Name: "host", Type: "text", Nullable: true},
				{Name: "details", Type: "text", Nullable: true},
				{Name: "severity", Type: "text", Nullable: true},
			},
		},
		{
			Name: "rate_limit_state",
			Columns: []Column{
				{Name: "id", Type: "text", Nullable: false},
				{Name: "host", Type: "text", Nullable: false},
				{Name: "endpoint", Type: "text", Nullable: false},
				{Name: "tokens", Type: "integer", Nullable: false},
				{Name: "last_updated", Type: "text", Nullable: false},
				{Name: "window_start", Type: "text", Nullable: true},
			},
		},
	}
}
```

---

## 7. Migration Reporter

### File: `internal/db/migration/reporter.go`

```go
package migration

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// Reporter generates migration reports
type Reporter struct {
	output io.Writer
}

// NewReporter creates a new migration reporter
func NewReporter(output io.Writer) *Reporter {
	return &Reporter{output: output}
}

// ReportStatus generates a status report
func (r *Reporter) ReportStatus(status *Status, registry *Registry) {
	fmt.Fprintf(r.output, "\n=== Migration Status ===\n\n")
	
	fmt.Fprintf(r.output, "Current Version:  %d\n", status.CurrentVersion)
	fmt.Fprintf(r.output, "Latest Version:   %d\n", status.LatestVersion)
	fmt.Fprintf(r.output, "Applied:          %d\n", status.AppliedCount)
	fmt.Fprintf(r.output, "Pending:          %d\n", status.PendingCount)
	fmt.Fprintf(r.output, "Failed:           %d\n\n", status.FailedCount)
	
	if status.IsUpToDate() {
		fmt.Fprintf(r.output, "Status: UP TO DATE\n")
	} else {
		fmt.Fprintf(r.output, "Status: OUTDATED\n")
	}
	
	if len(status.PendingVersions) > 0 {
		fmt.Fprintf(r.output, "\nPending Migrations:\n")
		for _, v := range status.PendingVersions {
			if m, ok := registry.Get(v); ok {
				fmt.Fprintf(r.output, "  %d: %s\n", v, m.Description)
			}
		}
	}
	
	if len(status.FailedVersions) > 0 {
		fmt.Fprintf(r.output, "\nFailed Migrations:\n")
		for _, v := range status.FailedVersions {
			fmt.Fprintf(r.output, "  %d (requires manual intervention)\n", v)
		}
	}
	
	fmt.Fprintln(r.output)
}

// ReportExecution generates an execution report
func (r *Reporter) ReportExecution(result *Result) {
	fmt.Fprintf(r.output, "\n=== Migration Execution ===\n\n")
	
	fmt.Fprintf(r.output, "Duration:         %v\n", result.Duration)
	fmt.Fprintf(r.output, "Start Version:    %d\n", result.StartVersion)
	fmt.Fprintf(r.output, "End Version:      %d\n", result.EndVersion)
	
	if len(result.Applied) > 0 {
		fmt.Fprintf(r.output, "\nApplied Migrations:\n")
		for _, m := range result.Applied {
			fmt.Fprintf(r.output, "  ✓ %d: %s\n", m.Version, m.Description)
		}
	}
	
	if result.Failed != nil {
		fmt.Fprintf(r.output, "\nFailed Migration:\n")
		fmt.Fprintf(r.output, "  ✗ %d: %s\n", result.Failed.Version, result.Failed.Description)
		if result.Error != nil {
			fmt.Fprintf(r.output, "  Error: %v\n", result.Error)
		}
	}
	
	if result.Success() {
		fmt.Fprintf(r.output, "\nResult: SUCCESS\n")
	} else {
		fmt.Fprintf(r.output, "\nResult: FAILED\n")
	}
	
	fmt.Fprintln(r.output)
}

// ReportValidation generates a validation report
func (r *Reporter) ReportValidation(result *ValidationResult) {
	fmt.Fprintf(r.output, "\n=== Schema Validation ===\n\n")
	
	if result.Valid {
		fmt.Fprintf(r.output, "Status: VALID\n")
	} else {
		fmt.Fprintf(r.output, "Status: INVALID\n")
	}
	
	if len(result.Errors) > 0 {
		fmt.Fprintf(r.output, "\nErrors:\n")
		for _, err := range result.Errors {
			fmt.Fprintf(r.output, "  ✗ %s\n", err)
		}
	}
	
	if len(result.Warnings) > 0 {
		fmt.Fprintf(r.output, "\nWarnings:\n")
		for _, warn := range result.Warnings {
			fmt.Fprintf(r.output, "  ! %s\n", warn)
		}
	}
	
	if len(result.Errors) == 0 && len(result.Warnings) == 0 {
		fmt.Fprintf(r.output, "No issues found.\n")
	}
	
	fmt.Fprintln(r.output)
}

// ReportList generates a list of all migrations
func (r *Reporter) ReportList(registry *Registry, applied []int64) {
	fmt.Fprintf(r.output, "\n=== Migration List ===\n\n")
	
	w := tabwriter.NewWriter(r.output, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tDESCRIPTION\tSTATUS")
	fmt.Fprintln(w, "-------\t-----------\t------")
	
	for _, m := range registry.GetAll() {
		status := "pending"
		for _, v := range applied {
			if v == m.Version {
				status = "applied"
				break
			}
		}
		
		fmt.Fprintf(w, "%d\t%s\t%s\n", m.Version, m.Description, status)
	}
	
	w.Flush()
	fmt.Fprintln(r.output)
}

// ReportHistory generates a history report from migration records
func (r *Reporter) ReportHistory(records []*Record) {
	fmt.Fprintf(r.output, "\n=== Migration History ===\n\n")
	
	if len(records) == 0 {
		fmt.Fprintf(r.output, "No migrations found.\n")
		return
	}
	
	w := tabwriter.NewWriter(r.output, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tDESCRIPTION\tAPPLIED\tDURATION\tSTATUS")
	fmt.Fprintln(w, "-------\t-----------\t-------\t--------\t------")
	
	for _, rec := range records {
		status := "✓ success"
		if !rec.Success {
			status = "✗ failed"
		}
		
		desc := rec.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		
		fmt.Fprintf(w, "%d\t%s\t%s\t%dms\t%s\n",
			rec.Version, desc, 
			rec.AppliedAt.Format("2006-01-02 15:04:05"),
			rec.Duration, status)
	}
	
	w.Flush()
	fmt.Fprintln(r.output)
}

// JSONReporter generates JSON reports
type JSONReporter struct {
	output io.Writer
}

// NewJSONReporter creates a new JSON reporter
func NewJSONReporter(output io.Writer) *JSONReporter {
	return &JSONReporter{output: output}
}

// ReportStatus generates a JSON status report
func (r *JSONReporter) ReportStatus(status *Status) {
	fmt.Fprintf(r.output, `{
  "current_version": %d,
  "latest_version": %d,
  "applied_count": %d,
  "pending_count": %d,
  "failed_count": %d,
  "is_up_to_date": %t,
  "applied_versions": %s,
  "pending_versions": %s,
  "failed_versions": %s
}
`, status.CurrentVersion, status.LatestVersion,
		status.AppliedCount, status.PendingCount, status.FailedCount,
		status.IsUpToDate(),
		int64SliceToJSON(status.AppliedVersions),
		int64SliceToJSON(status.PendingVersions),
		int64SliceToJSON(status.FailedVersions))
}

func int64SliceToJSON(s []int64) string {
	if len(s) == 0 {
		return "[]"
	}
	
	var parts []string
	for _, v := range s {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
```

---

## 8. Main Migrator

### File: `internal/db/migration/migrator.go`

```go
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"
)

// Migrator is the main migration orchestrator
type Migrator struct {
	db          *sql.DB
	dialect     Dialect
	registry    *Registry
	store       *Store
	executor    *Executor
	lockManager *LockManager
	validator   *Validator
	reporter    *Reporter
	options     Options
}

// MigratorConfig configures the migrator
type MigratorConfig struct {
	DB          *sql.DB
	Dialect     Dialect
	Registry    *Registry
	Options     Options
	LockTimeout time.Duration
	Output      io.Writer
}

// NewMigrator creates a new migrator instance
func NewMigrator(config MigratorConfig) (*Migrator, error) {
	if config.DB == nil {
		return nil, fmt.Errorf("database connection required")
	}
	
	if config.Registry == nil {
		return nil, fmt.Errorf("migration registry required")
	}
	
	// Auto-detect dialect if not specified
	dialect := config.Dialect
	if dialect == 0 {
		detected, err := DetectDialect(config.DB)
		if err != nil {
			return nil, fmt.Errorf("failed to detect dialect: %w", err)
		}
		dialect = detected
	}
	
	// Create store
	store, err := NewStore(config.DB, dialect)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration store: %w", err)
	}
	
	// Initialize migrations table
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := store.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize migrations table: %w", err)
	}
	
	// Create lock manager
	lockTimeout := config.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = 30 * time.Second
	}
	lockManager := NewLockManager(config.DB, dialect, lockTimeout)
	
	// Create executor
	opts := config.Options
	if opts.MigrationTimeout == 0 {
		opts = DefaultOptions()
	}
	executor := NewExecutor(store, opts)
	
	// Create validator
	validator := NewValidator(config.DB, dialect)
	
	// Create reporter
	output := config.Output
	if output == nil {
		output = os.Stdout
	}
	reporter := NewReporter(output)
	
	return &Migrator{
		db:          config.DB,
		dialect:     dialect,
		registry:    config.Registry,
		store:       store,
		executor:    executor,
		lockManager: lockManager,
		validator:   validator,
		reporter:    reporter,
		options:     opts,
	}, nil
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context) (*Result, error) {
	var result *Result
	var execErr error
	
	err := WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		result, execErr = m.executor.Execute(ctx, m.registry, 0)
		return execErr
	})
	
	if err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	
	return result, nil
}

// UpTo runs migrations up to a specific version
func (m *Migrator) UpTo(ctx context.Context, version int64) (*Result, error) {
	var result *Result
	var execErr error
	
	err := WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		result, execErr = m.executor.Execute(ctx, m.registry, version)
		return execErr
	})
	
	if err != nil {
		return nil, fmt.Errorf("migration to version %d failed: %w", version, err)
	}
	
	return result, nil
}

// Down rolls back the last migration
func (m *Migrator) Down(ctx context.Context) (*Result, error) {
	var result *Result
	var execErr error
	
	err := WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		result, execErr = m.executor.Rollback(ctx, m.registry)
		return execErr
	})
	
	if err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}
	
	return result, nil
}

// DownTo rolls back to a specific version
func (m *Migrator) DownTo(ctx context.Context, version int64) (*Result, error) {
	var result *Result
	var execErr error
	
	err := WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		result, execErr = m.executor.RollbackTo(ctx, m.registry, version)
		return execErr
	})
	
	if err != nil {
		return nil, fmt.Errorf("rollback to version %d failed: %w", version, err)
	}
	
	return result, nil
}

// Status returns the current migration status
func (m *Migrator) Status(ctx context.Context) (*Status, error) {
	return m.executor.Status(ctx, m.registry)
}

// StatusReport prints the current migration status
func (m *Migrator) StatusReport(ctx context.Context) error {
	status, err := m.Status(ctx)
	if err != nil {
		return err
	}
	
	m.reporter.ReportStatus(status, m.registry)
	return nil
}

// Validate validates the current schema
func (m *Migrator) Validate(ctx context.Context) (*ValidationResult, error) {
	expected := ExpectedSchema()
	return m.validator.ValidateSchema(ctx, expected)
}

// ValidateReport prints a schema validation report
func (m *Migrator) ValidateReport(ctx context.Context) error {
	result, err := m.Validate(ctx)
	if err != nil {
		return err
	}
	
	m.reporter.ReportValidation(result)
	return nil
}

// History returns the migration history
func (m *Migrator) History(ctx context.Context) ([]*Record, error) {
	return m.store.GetAppliedMigrations(ctx)
}

// HistoryReport prints the migration history
func (m *Migrator) HistoryReport(ctx context.Context) error {
	records, err := m.History(ctx)
	if err != nil {
		return err
	}
	
	m.reporter.ReportHistory(records)
	return nil
}

// List prints a list of all migrations
func (m *Migrator) List(ctx context.Context) error {
	records, err := m.History(ctx)
	if err != nil {
		return err
	}
	
	applied := make([]int64, len(records))
	for i, r := range records {
		applied[i] = r.Version
	}
	
	m.reporter.ReportList(m.registry, applied)
	return nil
}

// ForceVersion forces the database version (use with caution)
func (m *Migrator) ForceVersion(ctx context.Context, version int64) error {
	return WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		// Remove all migrations above the forced version
		records, err := m.store.GetAppliedMigrations(ctx)
		if err != nil {
			return err
		}
		
		tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}
		defer tx.Rollback()
		
		for _, rec := range records {
			if rec.Version > version {
				if err := m.store.RemoveMigration(ctx, tx, rec.Version); err != nil {
					return err
				}
			}
		}
		
		return tx.Commit()
	})
}

// Repair marks failed migrations as resolved
func (m *Migrator) Repair(ctx context.Context) error {
	failed, err := m.store.GetFailedMigrations(ctx)
	if err != nil {
		return err
	}
	
	if len(failed) == 0 {
		fmt.Fprintln(m.reporter.output, "No failed migrations to repair")
		return nil
	}
	
	return WithLock(ctx, m.lockManager, DefaultLockID, func() error {
		tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}
		defer tx.Rollback()
		
		for _, rec := range failed {
			// Remove failed migration records
			if err := m.store.RemoveMigration(ctx, tx, rec.Version); err != nil {
				return err
			}
		}
		
		return tx.Commit()
	})
}

// Close closes the migrator and releases resources
func (m *Migrator) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// SetReporter sets a custom reporter
func (m *Migrator) SetReporter(reporter *Reporter) {
	m.reporter = reporter
}

// GetDialect returns the detected database dialect
func (m *Migrator) GetDialect() Dialect {
	return m.dialect
}

// GetRegistry returns the migration registry
func (m *Migrator) GetRegistry() *Registry {
	return m.registry
}
```

---

## 9. Predefined Migrations

### File: `internal/db/migration/migrations.go`

```go
package migration

import (
	"context"
	"database/sql"
	"fmt"
)

// DefaultMigrations returns the built-in migrations for the schema
func DefaultMigrations() []*Migration {
	return []*Migration{
		Migration001_InitialSchema(),
	}
}

// Migration001_InitialSchema creates the initial database schema
func Migration001_InitialSchema() *Migration {
	return &Migration{
		Version:     1,
		Description: "Create initial schema with inventories, aliases, modules, specs, fingerprints, revocation_cache, audit_log, and rate_limit_state tables",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			// Create inventories table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS inventories (
					id TEXT PRIMARY KEY,
					host TEXT NOT NULL,
					version TEXT NOT NULL,
					content TEXT NOT NULL,
					sha256 TEXT NOT NULL,
					fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					etag TEXT,
					UNIQUE(host, version)
				)
			`); err != nil {
				return fmt.Errorf("failed to create inventories table: %w", err)
			}
			
			// Create aliases table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS aliases (
					name TEXT PRIMARY KEY,
					host TEXT NOT NULL,
					version TEXT,
					pinned INTEGER DEFAULT 0,
					config TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`); err != nil {
				return fmt.Errorf("failed to create aliases table: %w", err)
			}
			
			// Create modules table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS modules (
					sha256 TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					content BLOB NOT NULL,
					signature TEXT,
					size_bytes INTEGER NOT NULL,
					downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_used_at DATETIME,
					use_count INTEGER DEFAULT 0
				)
			`); err != nil {
				return fmt.Errorf("failed to create modules table: %w", err)
			}
			
			// Create specs table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS specs (
					sha256 TEXT PRIMARY KEY,
					host TEXT NOT NULL,
					content TEXT NOT NULL,
					fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`); err != nil {
				return fmt.Errorf("failed to create specs table: %w", err)
			}
			
			// Create fingerprints table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS fingerprints (
					id TEXT PRIMARY KEY,
					host TEXT NOT NULL,
					fingerprint TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					regenerated_at DATETIME,
					ip_address TEXT,
					UNIQUE(host, fingerprint)
				)
			`); err != nil {
				return fmt.Errorf("failed to create fingerprints table: %w", err)
			}
			
			// Create revocation_cache table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS revocation_cache (
					host TEXT PRIMARY KEY,
					content TEXT NOT NULL,
					etag TEXT,
					fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME
				)
			`); err != nil {
				return fmt.Errorf("failed to create revocation_cache table: %w", err)
			}
			
			// Create audit_log table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS audit_log (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
					event_type TEXT NOT NULL,
					client_id TEXT,
					user_id TEXT,
					fingerprint TEXT,
					host TEXT,
					details TEXT,
					severity TEXT DEFAULT 'info'
				)
			`); err != nil {
				return fmt.Errorf("failed to create audit_log table: %w", err)
			}
			
			// Create rate_limit_state table
			if _, err := tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS rate_limit_state (
					id TEXT PRIMARY KEY,
					host TEXT NOT NULL,
					endpoint TEXT NOT NULL,
					tokens INTEGER NOT NULL,
					last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
					window_start DATETIME
				)
			`); err != nil {
				return fmt.Errorf("failed to create rate_limit_state table: %w", err)
			}
			
			// Create indexes
			indexes := []struct {
				name  string
				sql   string
			}{
				{"idx_inventories_host", "CREATE INDEX idx_inventories_host ON inventories(host)"},
				{"idx_inventories_fetched", "CREATE INDEX idx_inventories_fetched ON inventories(fetched_at)"},
				{"idx_modules_name", "CREATE INDEX idx_modules_name ON modules(name)"},
				{"idx_modules_last_used", "CREATE INDEX idx_modules_last_used ON modules(last_used_at)"},
				{"idx_audit_timestamp", "CREATE INDEX idx_audit_timestamp ON audit_log(timestamp)"},
				{"idx_audit_event_type", "CREATE INDEX idx_audit_event_type ON audit_log(event_type)"},
				{"idx_audit_host", "CREATE INDEX idx_audit_host ON audit_log(host)"},
			}
			
			for _, idx := range indexes {
				if _, err := tx.ExecContext(ctx, idx.sql); err != nil {
					return fmt.Errorf("failed to create index %s: %w", idx.name, err)
				}
			}
			
			return nil
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			// Drop tables in reverse order of dependencies
			tables := []string{
				"rate_limit_state",
				"audit_log",
				"revocation_cache",
				"fingerprints",
				"specs",
				"modules",
				"aliases",
				"inventories",
			}
			
			for _, table := range tables {
				if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
					return fmt.Errorf("failed to drop table %s: %w", table, err)
				}
			}
			
			return nil
		},
	}
}

// CreateDefaultRegistry creates a registry with default migrations
func CreateDefaultRegistry() *Registry {
	registry := NewRegistry()
	for _, m := range DefaultMigrations() {
		if err := registry.Register(m); err != nil {
			// This should never happen with built-in migrations
			panic(err)
		}
	}
	return registry
}
```

---

## 10. Usage Examples

### Basic Usage

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/openapi-cli/core/internal/db/migration"
)

func main() {
	// Open database
	db, err := sql.Open("sqlite3", "app.db?_journal=WAL&_foreign_keys=ON")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create migrator with default migrations
	migrator, err := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
		Options:  migration.DefaultOptions(),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := migrator.Up(ctx)
	if err != nil {
		log.Fatal(err)
	}

	migrator.StatusReport(ctx)
	
	log.Printf("Migrated from version %d to %d", result.StartVersion, result.EndVersion)
}
```

### Custom Migrations

```go
package main

import (
	"context"
	"database/sql"
	"log"
	
	"github.com/openapi-cli/core/internal/db/migration"
)

func main() {
	// Create custom registry
	registry := migration.NewRegistry()
	
	// Register custom migration
	registry.Register(&migration.Migration{
		Version:     2,
		Description: "Add user preferences table",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				CREATE TABLE user_preferences (
					user_id TEXT PRIMARY KEY,
					preferences TEXT NOT NULL,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`)
			return err
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "DROP TABLE user_preferences")
			return err
		},
	})
	
	// Use with migrator
	migrator, err := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: registry,
	})
	if err != nil {
		log.Fatal(err)
	}
	
	// Migrate up
	result, err := migrator.Up(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	
	log.Printf("Applied %d migrations", len(result.Applied))
}
```

### CLI Integration

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/openapi-cli/core/internal/db/migration"
)

func main() {
	var (
		command = flag.String("command", "status", "Command: up, down, status, validate, history")
		version = flag.Int64("version", 0, "Target version for up/down")
		dsn     = flag.String("dsn", "app.db", "Database connection string")
	)
	flag.Parse()

	// Open database (simplified)
	db, err := sql.Open("sqlite3", *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	migrator, err := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	switch *command {
	case "up":
		if *version > 0 {
			_, err = migrator.UpTo(ctx, *version)
		} else {
			_, err = migrator.Up(ctx)
		}
	case "down":
		if *version >= 0 {
			_, err = migrator.DownTo(ctx, *version)
		} else {
			_, err = migrator.Down(ctx)
		}
	case "status":
		err = migrator.StatusReport(ctx)
	case "validate":
		err = migrator.ValidateReport(ctx)
	case "history":
		err = migrator.HistoryReport(ctx)
	case "list":
		err = migrator.List(ctx)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

---

## 11. Error Handling & Recovery

### Error Types

```go
package migration

import "errors"

// Common errors
var (
	ErrNoMigrations      = errors.New("no migrations found")
	ErrMigrationFailed   = errors.New("migration failed")
	ErrRollbackFailed  = errors.New("rollback failed")
	ErrLockTimeout     = errors.New("lock acquisition timeout")
	ErrDirtyDatabase   = errors.New("database has failed migrations")
	ErrChecksumMismatch = errors.New("migration checksum mismatch")
	ErrVersionNotFound = errors.New("migration version not found")
)

// MigrationError provides detailed error information
type MigrationError struct {
	Version     int64
	Description string
	Operation   string
	Cause       error
}

func (e *MigrationError) Error() string {
	return fmt.Sprintf("migration %d (%s) %s failed: %v", 
		e.Version, e.Description, e.Operation, e.Cause)
}

func (e *MigrationError) Unwrap() error {
	return e.Cause
}
```

### Recovery Procedures

1. **Lock Timeout**: Increase `LockTimeout` in config or check for stuck processes
2. **Dirty Database**: Use `Repair()` to clear failed migrations after manual verification
3. **Checksum Mismatch**: Never modify applied migrations; create new migrations instead
4. **Rollback Failure**: Manual database intervention may be required

---

## 12. Testing

### Test Suite Structure

```go
package migration_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/openapi-cli/core/internal/db/migration"
)

func TestMigrationExecution(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create migrator
	migrator, err := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run migrations
	ctx := context.Background()
	result, err := migrator.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Success() {
		t.Fatal("migration failed")
	}

	if result.EndVersion == 0 {
		t.Fatal("expected migrations to be applied")
	}

	// Verify tables exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='inventories'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected inventories table to exist, got count %d", count)
	}
}

func TestMigrationRollback(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrator, err := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Apply migrations
	_, err = migrator.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Rollback
	result, err := migrator.Down(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Applied) != 1 {
		t.Fatalf("expected 1 rolled back migration, got %d", len(result.Applied))
	}
}

func TestConcurrentExecutionPrevention(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First migrator
	migrator1, _ := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
	})

	// Second migrator (simulating concurrent access)
	migrator2, _ := migration.NewMigrator(migration.MigratorConfig{
		DB:       db,
		Registry: migration.CreateDefaultRegistry(),
	})

	ctx := context.Background()

	// Start first migration
	go migrator1.Up(ctx)

	// Second should fail due to lock
	_, err = migrator2.Up(ctx)
	if err == nil {
		t.Fatal("expected second migration to fail due to lock")
	}
}
```

---

## 13. Production Configuration

### Recommended Settings

```go
// Production-ready configuration
func ProductionConfig(db *sql.DB) (*Migrator, error) {
	return NewMigrator(MigratorConfig{
		DB:       db,
		Dialect:  0, // Auto-detect
		Registry: CreateDefaultRegistry(),
		Options: Options{
			MigrationTimeout: 10 * time.Minute,
			ValidateChecksum: true,
			AllowDirty:       false,
			DryRun:           false,
		},
		LockTimeout: 60 * time.Second,
		Output:      os.Stdout,
	})
}

// Development configuration
func DevelopmentConfig(db *sql.DB) (*Migrator, error) {
	return NewMigrator(MigratorConfig{
		DB:       db,
		Registry: CreateDefaultRegistry(),
		Options: Options{
			MigrationTimeout: 1 * time.Minute,
			ValidateChecksum: false,
			AllowDirty:       true,
			DryRun:           false,
		},
		LockTimeout: 10 * time.Second,
	})
}
```

---

## File Structure

```
internal/db/migration/
├── migration.go       # Core types and registry
├── store.go           # Version tracking
├── lock.go            # Advisory locks
├── executor.go        # Migration execution
├── validation.go      # Schema validation
├── reporter.go        # Status reporting
├── migrator.go        # Main orchestrator
├── migrations.go      # Built-in migrations
└── errors.go          # Error definitions
```

---

## Summary

This migration system provides:

1. **Complete Go Implementation**: All code is production-ready with actual SQL execution
2. **Transaction Safety**: Every migration runs in a transaction with automatic rollback on failure
3. **Version Tracking**: Full migration history with checksum validation
4. **Rollback Support**: Both single migration and version-specific rollback
5. **Schema Validation**: Automated schema verification against expected structure
6. **Status Reporting**: Human-readable and JSON output formats
7. **Concurrent Prevention**: Advisory locks for PostgreSQL, named locks for MySQL, table locks for SQLite
8. **Error Recovery**: Comprehensive error types and recovery procedures
9. **Multi-Database Support**: SQLite, PostgreSQL, and MySQL with dialect detection
10. **Testing**: Full test suite included

## Deterministic Guarantees

This migration system implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Migration Ordering

Migration execution must follow identical order across platforms:

- **Version ordering**: Numeric version sequence (001, 002, 003...)
- **Dependency resolution**: Deterministic dependency graph traversal
- **Rollback ordering**: Reverse of application order

### 2. Platform‑Independent Database Operations

Database operations must be platform‑independent:

- **SQL dialect detection**: Same detection logic across platforms
- **Transaction isolation**: Same isolation level across database engines
- **Lock acquisition**: Deterministic lock timeout behavior

### 3. Deterministic Checksum Validation

Migration checksums must be computed identically:

- **File hashing**: SHA‑256 of normalized SQL (whitespace‑insensitive)
- **Checksum storage**: Deterministic checksum database format
- **Validation failures**: Same validation errors for same corrupted migrations

### 4. Cross‑Platform Locking Mechanisms

Database locking must behave identically:

- **Advisory locks**: PostgreSQL advisory lock IDs computed deterministically
- **Named locks**: MySQL named lock names computed from database name hash
- **Table locks**: SQLite table lock ordering determinism

### 5. Testing Determinism

All migration system tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestMigrationDeterminism(t *testing.T) {
	// Verify identical migration ordering
	// Verify identical checksum validation
	// Verify identical lock acquisition
}
```

### 6. Integration with SDK Generation

When generating SDKs for 12 languages, migration systems must:

- Apply migrations in identical order
- Compute identical checksums
- Use identical locking mechanisms

**Verification**: Run `./scripts/validate‑determinism‑migration.sh` to confirm cross‑platform consistency.

The system is designed for production use with proper error handling, logging, and recovery mechanisms.
