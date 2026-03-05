# Database Repository Pattern Specification

## Overview

Repository pattern implementation for all database operations with support for multiple drivers (SQLite, PostgreSQL, MySQL), connection pooling, transactions, and comprehensive error handling.

## Repository Interfaces

### Core Repository Interface

```go
// Package repository provides database access patterns for the Muster project.
package repository

import (
	"context"
	"database/sql"
	"time"
)

// Repository is the core interface for all repository implementations.
type Repository interface {
	// Close closes the database connection pool.
	Close() error
	
	// Ping verifies the database connection is alive.
	Ping() error
	
	// BeginTx starts a new database transaction.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Transaction, error)
	
	// Stats returns database statistics.
	Stats() sql.DBStats
}

// Transaction represents a database transaction.
type Transaction interface {
	// Commit commits the transaction.
	Commit() error
	
	// Rollback rolls back the transaction.
	Rollback() error
	
	// GetTx returns the underlying sql.Tx.
	GetTx() *sql.Tx
}

// QueryExecutor provides a common interface for executing queries.
type QueryExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}
```

### Entity Repository Interfaces

```go
// InventoryRepository provides access to inventory operations.
type InventoryRepository interface {
	Create(ctx context.Context, inv *Inventory) error
	GetByID(ctx context.Context, id int64) (*Inventory, error)
	GetByAlias(ctx context.Context, alias string) (*Inventory, error)
	GetByModule(ctx context.Context, moduleID int64) ([]*Inventory, error)
	Update(ctx context.Context, inv *Inventory) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, opts ListOptions) ([]*Inventory, error)
	Count(ctx context.Context, opts ListOptions) (int64, error)
	Search(ctx context.Context, query string, opts ListOptions) ([]*Inventory, error)
}

// AliasRepository provides access to alias operations.
type AliasRepository interface {
	Create(ctx context.Context, alias *Alias) error
	GetByID(ctx context.Context, id int64) (*Alias, error)
	GetByName(ctx context.Context, name string) (*Alias, error)
	GetByInventory(ctx context.Context, inventoryID int64) ([]*Alias, error)
	GetByModule(ctx context.Context, moduleID int64) ([]*Alias, error)
	Update(ctx context.Context, alias *Alias) error
	Delete(ctx context.Context, id int64) error
	DeleteByInventory(ctx context.Context, inventoryID int64) error
}

// ModuleRepository provides access to module operations.
type ModuleRepository interface {
	Create(ctx context.Context, mod *Module) error
	GetByID(ctx context.Context, id int64) (*Module, error)
	GetByPath(ctx context.Context, path string) (*Module, error)
	GetByInventory(ctx context.Context, inventoryID int64) ([]*Module, error)
	Update(ctx context.Context, mod *Module) error
	Delete(ctx context.Context, id int64) error
	GetOrphans(ctx context.Context) ([]*Module, error)
	CountByInventory(ctx context.Context, inventoryID int64) (int64, error)
}

// SpecRepository provides access to spec operations.
type SpecRepository interface {
	Create(ctx context.Context, spec *Spec) error
	GetByID(ctx context.Context, id int64) (*Spec, error)
	GetByAlias(ctx context.Context, aliasID int64) ([]*Spec, error)
	GetByModule(ctx context.Context, moduleID int64) ([]*Spec, error)
	Update(ctx context.Context, spec *Spec) error
	Delete(ctx context.Context, id int64) error
	DeleteByAlias(ctx context.Context, aliasID int64) error
	DeleteByModule(ctx context.Context, moduleID int64) error
}

// FingerprintRepository provides access to fingerprint operations.
type FingerprintRepository interface {
	Create(ctx context.Context, fp *Fingerprint) error
	GetByID(ctx context.Context, id int64) (*Fingerprint, error)
	GetBySpec(ctx context.Context, specID int64) ([]*Fingerprint, error)
	GetByHash(ctx context.Context, hash string) (*Fingerprint, error)
	Update(ctx context.Context, fp *Fingerprint) error
	Delete(ctx context.Context, id int64) error
	DeleteBySpec(ctx context.Context, specID int64) error
	Exists(ctx context.Context, hash string) (bool, error)
}

// AuditRepository provides access to audit log operations.
type AuditRepository interface {
	Create(ctx context.Context, audit *Audit) error
	GetByID(ctx context.Context, id int64) (*Audit, error)
	GetByEntity(ctx context.Context, entityType string, entityID int64) ([]*Audit, error)
	List(ctx context.Context, opts ListOptions) ([]*Audit, error)
	Count(ctx context.Context, opts ListOptions) (int64, error)
	Cleanup(ctx context.Context, before time.Time) (int64, error)
}

// RateLimitRepository provides access to rate limiting operations.
type RateLimitRepository interface {
	Create(ctx context.Context, rl *RateLimit) error
	GetByID(ctx context.Context, id int64) (*RateLimit, error)
	GetByKey(ctx context.Context, key string) (*RateLimit, error)
	Update(ctx context.Context, rl *RateLimit) error
	Delete(ctx context.Context, id int64) error
	DeleteByKey(ctx context.Context, key string) error
	Cleanup(ctx context.Context, before time.Time) (int64, error)
	Increment(ctx context.Context, key string) error
}
```

### Entity Models

```go
// Inventory represents a collection of modules and aliases.
type Inventory struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	Path        string    `db:"path" json:"path"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// Alias represents a named reference to modules or specs.
type Alias struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	InventoryID int64     `db:"inventory_id" json:"inventory_id"`
	ModuleID    *int64    `db:"module_id" json:"module_id,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// Module represents a code module in the system.
type Module struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Path        string    `db:"path" json:"path"`
	Language    string    `db:"language" json:"language"`
	InventoryID int64     `db:"inventory_id" json:"inventory_id"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// Spec represents a specification or contract.
type Spec struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Content     string    `db:"content" json:"content"`
	AliasID     int64     `db:"alias_id" json:"alias_id"`
	ModuleID    int64     `db:"module_id" json:"module_id"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// Fingerprint represents a content hash for tracking.
type Fingerprint struct {
	ID        int64     `db:"id" json:"id"`
	Hash      string    `db:"hash" json:"hash"`
	SpecID    int64     `db:"spec_id" json:"spec_id"`
	Algorithm string    `db:"algorithm" json:"algorithm"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Audit represents an audit log entry.
type Audit struct {
	ID         int64     `db:"id" json:"id"`
	Action     string    `db:"action" json:"action"`
	EntityType string    `db:"entity_type" json:"entity_type"`
	EntityID   int64     `db:"entity_id" json:"entity_id"`
	UserID     *string   `db:"user_id" json:"user_id,omitempty"`
	Data       []byte    `db:"data" json:"data"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// RateLimit represents a rate limiting entry.
type RateLimit struct {
	ID        int64     `db:"id" json:"id"`
	Key       string    `db:"key" json:"key"`
	Count     int64     `db:"count" json:"count"`
	Window    time.Time `db:"window" json:"window"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ListOptions provides pagination and filtering options.
type ListOptions struct {
	Limit   int
	Offset  int
	OrderBy string
	Order   string // ASC or DESC
}
```

## SQL Implementation

### Base Repository

```go
// Package sql implements repository interfaces using SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
	
	_ "github.com/go-sql-driver/mysql"  // MySQL driver
	_ "github.com/lib/pq"               // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3"     // SQLite driver
	
	"muster/internal/repository"
)

// Driver represents the supported database drivers.
type Driver string

const (
	DriverSQLite     Driver = "sqlite"
	DriverPostgreSQL Driver = "postgres"
	DriverMySQL      Driver = "mysql"
)

// Config holds database configuration.
type Config struct {
	Driver             Driver
	DSN                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetime    time.Duration
	ConnMaxIdleTime    time.Duration
	QueryTimeout       time.Duration
	EnableQueryLogging bool
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:       25,
		MaxIdleConns:       5,
		ConnMaxLifetime:    5 * time.Minute,
		ConnMaxIdleTime:    time.Minute,
		QueryTimeout:       30 * time.Second,
		EnableQueryLogging: false,
	}
}

// sqlRepository is the base implementation for SQL repositories.
type sqlRepository struct {
	db     *sql.DB
	driver Driver
	logger *slog.Logger
	config Config
}

// Close implements Repository.Close.
func (r *sqlRepository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// Ping implements Repository.Ping.
func (r *sqlRepository) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.config.QueryTimeout)
	defer cancel()
	return r.db.PingContext(ctx)
}

// Stats returns database statistics.
func (r *sqlRepository) Stats() sql.DBStats {
	return r.db.Stats()
}

// sqlTransaction wraps a database transaction.
type sqlTransaction struct {
	tx *sql.Tx
}

// Commit implements Transaction.Commit.
func (t *sqlTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback implements Transaction.Rollback.
func (t *sqlTransaction) Rollback() error {
	return t.tx.Rollback()
}

// GetTx returns the underlying sql.Tx.
func (t *sqlTransaction) GetTx() *sql.Tx {
	return t.tx
}

// queryLogger logs database queries when enabled.
type queryLogger struct {
	logger *slog.Logger
	enable bool
}

func (l *queryLogger) Log(query string, args []interface{}, duration time.Duration, err error) {
	if !l.enable || l.logger == nil {
		return
	}
	
	if err != nil && err != sql.ErrNoRows {
		l.logger.Error("database query failed",
			slog.String("query", query),
			slog.Any("args", args),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
	} else {
		l.logger.Debug("database query executed",
			slog.String("query", query),
			slog.Int("arg_count", len(args)),
			slog.Duration("duration", duration),
		)
	}
}

// withTimeout creates a context with configured timeout.
func (r *sqlRepository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.config.QueryTimeout)
}

// exec executes a query with logging and timeout.
func (r *sqlRepository) exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	
	result, err := r.db.ExecContext(ctx, query, args...)
	duration := time.Since(start)
	
	log := queryLogger{logger: r.logger, enable: r.config.EnableQueryLogging}
	log.Log(query, args, duration, err)
	
	return result, err
}

// query executes a query with logging and timeout.
func (r *sqlRepository) query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	
	rows, err := r.db.QueryContext(ctx, query, args...)
	duration := time.Since(start)
	
	log := queryLogger{logger: r.logger, enable: r.config.EnableQueryLogging}
	log.Log(query, args, duration, err)
	
	return rows, err
}

// queryRow executes a single row query with logging and timeout.
func (r *sqlRepository) queryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	
	row := r.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)
	
	log := queryLogger{logger: r.logger, enable: r.config.EnableQueryLogging}
	log.Log(query, args, duration, nil)
	
	return row
}
```

## Factory Pattern

```go
// factory.go implements the repository factory pattern.
package sql

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
	
	"muster/internal/repository"
)

// Factory creates repository instances for different database drivers.
type Factory struct {
	config Config
	logger *slog.Logger
}

// NewFactory creates a new repository factory.
func NewFactory(config Config, logger *slog.Logger) *Factory {
	if logger == nil {
		logger = slog.Default()
	}
	return &Factory{
		config: config,
		logger: logger,
	}
}

// Create opens a database connection and returns a repository instance.
func (f *Factory) Create(driver Driver, dsn string) (*sqlRepository, error) {
	// Open database connection
	db, err := sql.Open(string(driver), dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	// Configure connection pool
	db.SetMaxOpenConns(f.config.MaxOpenConns)
	db.SetMaxIdleConns(f.config.MaxIdleConns)
	db.SetConnMaxLifetime(f.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(f.config.ConnMaxIdleTime)
	
	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), f.config.QueryTimeout)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	
	repo := &sqlRepository{
		db:     db,
		driver: driver,
		logger: f.logger,
		config: f.config,
	}
	
	f.logger.Info("database connection established",
		slog.String("driver", string(driver)),
		slog.Int("max_open_conns", f.config.MaxOpenConns),
		slog.Int("max_idle_conns", f.config.MaxIdleConns),
	)
	
	return repo, nil
}

// CreateInventory creates an InventoryRepository.
func (f *Factory) CreateInventory(repo *sqlRepository) repository.InventoryRepository {
	return NewInventoryRepository(repo)
}

// CreateAlias creates an AliasRepository.
func (f *Factory) CreateAlias(repo *sqlRepository) repository.AliasRepository {
	return NewAliasRepository(repo)
}

// CreateModule creates a ModuleRepository.
func (f *Factory) CreateModule(repo *sqlRepository) repository.ModuleRepository {
	return NewModuleRepository(repo)
}

// CreateSpec creates a SpecRepository.
func (f *Factory) CreateSpec(repo *sqlRepository) repository.SpecRepository {
	return NewSpecRepository(repo)
}

// CreateFingerprint creates a FingerprintRepository.
func (f *Factory) CreateFingerprint(repo *sqlRepository) repository.FingerprintRepository {
	return NewFingerprintRepository(repo)
}

// CreateAudit creates an AuditRepository.
func (f *Factory) CreateAudit(repo *sqlRepository) repository.AuditRepository {
	return NewAuditRepository(repo)
}

// CreateRateLimit creates a RateLimitRepository.
func (f *Factory) CreateRateLimit(repo *sqlRepository) repository.RateLimitRepository {
	return NewRateLimitRepository(repo)
}

// RepositorySet holds all repositories for easy access.
type RepositorySet struct {
	Inventory   repository.InventoryRepository
	Alias       repository.AliasRepository
	Module      repository.ModuleRepository
	Spec        repository.SpecRepository
	Fingerprint repository.FingerprintRepository
	Audit       repository.AuditRepository
	RateLimit   repository.RateLimitRepository
}

// CreateRepositorySet creates all repositories at once.
func (f *Factory) CreateRepositorySet(repo *sqlRepository) *RepositorySet {
	return &RepositorySet{
		Inventory:   f.CreateInventory(repo),
		Alias:       f.CreateAlias(repo),
		Module:      f.CreateModule(repo),
		Spec:        f.CreateSpec(repo),
		Fingerprint: f.CreateFingerprint(repo),
		Audit:       f.CreateAudit(repo),
		RateLimit:   f.CreateRateLimit(repo),
	}
}
```

## Error Handling

```go
// errors.go provides error types for the repository layer.
package repository

import (
	"errors"
	"fmt"
)

// Common repository errors.
var (
	ErrInvalidDriver   = errors.New("invalid database driver")
	ErrAlreadyExists   = errors.New("entity already exists")
	ErrConnection      = errors.New("database connection error")
	ErrTimeout         = errors.New("database operation timeout")
	ErrTransaction     = errors.New("transaction error")
	ErrConstraint      = errors.New("constraint violation")
	ErrValidation      = errors.New("validation error")
)

// ErrNotFound is returned when an entity is not found.
type ErrNotFound struct {
	Entity string
	ID     int64
	Key    string
	Value  interface{}
}

func (e ErrNotFound) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("%s with %s=%v not found", e.Entity, e.Key, e.Value)
	}
	return fmt.Sprintf("%s with ID %d not found", e.Entity, e.ID)
}

func (e ErrNotFound) Is(target error) bool {
	_, ok := target.(ErrNotFound)
	return ok
}
```

## Usage Example

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	
	"muster/internal/repository"
	"muster/internal/repository/sql"
)

func main() {
	// Configure database
	config := sql.DefaultConfig()
	config.Driver = sql.DriverSQLite
	config.DSN = "./muster.db"
	config.EnableQueryLogging = true
	
	// Create factory
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	factory := sql.NewFactory(config, logger)
	
	// Create repository
	repo, err := factory.Create(config.Driver, config.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()
	
	// Create all repositories
	repos := factory.CreateRepositorySet(repo)
	
	// Use repositories
	ctx := context.Background()
	
	// Create inventory
	inv := &repository.Inventory{
		Name:        "My Inventory",
		Description: "Test inventory",
		Path:        "/path/to/inventory",
	}
	
	if err := repos.Inventory.Create(ctx, inv); err != nil {
		log.Fatal(err)
	}
	
	// Retrieve inventory
	found, err := repos.Inventory.GetByID(ctx, inv.ID)
	if err != nil {
		log.Fatal(err)
	}
	
	log.Printf("Found inventory: %s", found.Name)
}
```

## File Structure

```
internal/
└── repository/
    ├── repository.go           # Core interfaces and entity models
    ├── errors.go               # Error types
    └── sql/
        ├── repository.go         # SQL base implementation
        ├── factory.go          # Repository factory
        ├── queries.go          # Query definitions
        ├── sqlite_queries.go   # SQLite queries
        ├── postgres_queries.go # PostgreSQL queries
        ├── mysql_queries.go    # MySQL queries
        ├── errors.go           # SQL error mapping
        ├── inventory_repository.go
        ├── alias_repository.go
        ├── module_repository.go
        ├── spec_repository.go
        ├── fingerprint_repository.go
        ├── audit_repository.go
        └── ratelimit_repository.go
```
