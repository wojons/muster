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
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Content   string    `db:"content" json:"content"`
	AliasID   int64     `db:"alias_id" json:"alias_id"`
	ModuleID  int64     `db:"module_id" json:"module_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
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
