package repository

import (
	"context"
	"database/sql"
	"fmt"
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
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Content     string    `db:"content" json:"content"`
	Fingerprint string    `db:"fingerprint" json:"fingerprint"`
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

// RevocationCache represents a revoked token cache entry.
type RevocationCache struct {
	ID        int64     `db:"id" json:"id"`
	TokenHash string    `db:"token_hash" json:"token_hash"`
	Reason    string    `db:"reason" json:"reason"`
	RevokedAt time.Time `db:"revoked_at" json:"revoked_at"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ListOptions provides pagination and filtering options.
type ListOptions struct {
	Limit   int
	Offset  int
	OrderBy string
	Order   string // ASC or DESC
}

// ErrNotFound is returned when an entity is not found.
type ErrNotFound struct {
	Entity string
	ID     int64
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("%s with ID %d not found", e.Entity, e.ID)
}

// ErrValidation is returned when validation fails.
type ErrValidation struct {
	Field   string
	Message string
}

func (e ErrValidation) Error() string {
	return fmt.Sprintf("validation error for field %s: %s", e.Field, e.Message)
}

// InventoryRepository defines CRUD operations for inventories.
type InventoryRepository interface {
	Create(ctx context.Context, inv *Inventory) error
	GetByID(ctx context.Context, id int64) (*Inventory, error)
	GetByAlias(ctx context.Context, alias string) (*Inventory, error)
	GetByModule(ctx context.Context, moduleID int64) (*Inventory, error)
	Update(ctx context.Context, inv *Inventory) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, opts ListOptions) ([]*Inventory, error)
	Count(ctx context.Context) (int64, error)
	Search(ctx context.Context, query string, opts ListOptions) ([]*Inventory, error)
}

// AliasRepository defines CRUD operations for aliases.
type AliasRepository interface {
	Create(ctx context.Context, alias *Alias) error
	GetByID(ctx context.Context, id int64) (*Alias, error)
	GetByName(ctx context.Context, name string) (*Alias, error)
	GetByInventory(ctx context.Context, inventoryID int64, opts ListOptions) ([]*Alias, error)
	GetByModule(ctx context.Context, moduleID int64, opts ListOptions) ([]*Alias, error)
	Update(ctx context.Context, alias *Alias) error
	Delete(ctx context.Context, id int64) error
	DeleteByInventory(ctx context.Context, inventoryID int64) error
}

// ModuleRepository defines CRUD operations for modules.
type ModuleRepository interface {
	Create(ctx context.Context, mod *Module) error
	GetByID(ctx context.Context, id int64) (*Module, error)
	GetByPath(ctx context.Context, path string) (*Module, error)
	GetByInventory(ctx context.Context, inventoryID int64, opts ListOptions) ([]*Module, error)
	Update(ctx context.Context, mod *Module) error
	Delete(ctx context.Context, id int64) error
	GetOrphans(ctx context.Context) ([]*Module, error)
	CountByInventory(ctx context.Context, inventoryID int64) (int64, error)
}

// SpecRepository defines CRUD operations for specs.
type SpecRepository interface {
	Create(ctx context.Context, spec *Spec) error
	GetByID(ctx context.Context, id int64) (*Spec, error)
	GetByAlias(ctx context.Context, aliasID int64, opts ListOptions) ([]*Spec, error)
	GetByModule(ctx context.Context, moduleID int64, opts ListOptions) ([]*Spec, error)
	Update(ctx context.Context, spec *Spec) error
	Delete(ctx context.Context, id int64) error
	DeleteByAlias(ctx context.Context, aliasID int64) error
	DeleteByModule(ctx context.Context, moduleID int64) error
}

// FingerprintRepository defines CRUD operations for fingerprints.
type FingerprintRepository interface {
	Create(ctx context.Context, fp *Fingerprint) error
	GetByID(ctx context.Context, id int64) (*Fingerprint, error)
	GetBySpec(ctx context.Context, specID int64) (*Fingerprint, error)
	GetByHash(ctx context.Context, hash string) (*Fingerprint, error)
	Update(ctx context.Context, fp *Fingerprint) error
	Delete(ctx context.Context, id int64) error
	DeleteBySpec(ctx context.Context, specID int64) error
	Exists(ctx context.Context, hash string) (bool, error)
}

// AuditRepository defines CRUD operations for audit logs.
type AuditRepository interface {
	Create(ctx context.Context, audit *Audit) error
	GetByID(ctx context.Context, id int64) (*Audit, error)
	GetByEntity(ctx context.Context, entityType string, entityID int64, opts ListOptions) ([]*Audit, error)
	List(ctx context.Context, opts ListOptions) ([]*Audit, error)
	Count(ctx context.Context) (int64, error)
	Cleanup(ctx context.Context, olderThan time.Time) (int64, error)
}

// RateLimitRepository defines CRUD operations for rate limits.
type RateLimitRepository interface {
	Create(ctx context.Context, rl *RateLimit) error
	GetByID(ctx context.Context, id int64) (*RateLimit, error)
	GetByKey(ctx context.Context, key string) (*RateLimit, error)
	Update(ctx context.Context, rl *RateLimit) error
	Delete(ctx context.Context, id int64) error
	DeleteByKey(ctx context.Context, key string) error
	Cleanup(ctx context.Context, olderThan time.Time) (int64, error)
	Increment(ctx context.Context, key string, window time.Time) error
}
