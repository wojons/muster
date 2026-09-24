# Database Schema Specification

> STATUS: HISTORICAL — the file paths in this spec (`migrations/001_initial_schema.*`,
> `queries/*.sql`, `sqlc.yaml`, `internal/db/...`) were never built. The real schema is the
> goose migration set in `migrations/` (`00001_configs.sql` … `00009_hipaa_rbac.sql`),
> applied out-of-band; there is no `queries/` directory, no `sqlc.yaml` and no Go migration
> runner. Persistence is `pkg/repository/sqlite.go` with per-entity stores under
> `pkg/repository/sql/`. Treat every `### File:` path below as a design-time label.

## Overview

Complete SQL schema definitions for SQLite, MySQL, and PostgreSQL with migrations and sqlc configuration.

## 1. Database Support

### SQLite (Default - Development)
- File-based: `~/.local/share/openapi-cli/cache.db`
- Zero configuration
- Good for single-user scenarios

### MySQL (Production)
- Connection: `user:pass@tcp(host:3306)/dbname`
- Good for medium scale

### PostgreSQL (Enterprise)
- Connection: `postgres://user:pass@host/dbname`
- Best for high concurrency

---

## 2. Complete SQL Schema

> STATUS: HISTORICAL — this path was never built. Real implementation: `migrations/00001_configs.sql`.

### File: `migrations/001_initial_schema.up.sql` (real: `migrations/00001_configs.sql`)

```sql
-- inventories: Store fetched inventory versions
CREATE TABLE IF NOT EXISTS inventories (
    id TEXT PRIMARY KEY,                          -- UUID v4
    host TEXT NOT NULL,                           -- Inventory host URL
    version TEXT NOT NULL,                        -- Inventory version
    content JSONB NOT NULL,                       -- Full inventory JSON
    sha256 TEXT NOT NULL,                         -- Content hash
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,                         -- Cache expiration
    etag TEXT,                                    -- HTTP ETag
    UNIQUE(host, version)
);

-- aliases: Named inventory configurations
CREATE TABLE IF NOT EXISTS aliases (
    name TEXT PRIMARY KEY,                        -- Alias name (e.g., "prod")
    host TEXT NOT NULL,
    version TEXT,
    pinned BOOLEAN DEFAULT FALSE,                 -- Pin to specific version
    config JSONB,                                 -- Additional config
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- modules: Cached WASM modules
CREATE TABLE IF NOT EXISTS modules (
    sha256 TEXT PRIMARY KEY,                      -- Content-addressed
    name TEXT NOT NULL,
    content BLOB NOT NULL,                        -- WASM binary
    signature TEXT,                               -- Module signature
    size_bytes INTEGER NOT NULL,
    downloaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,                       -- For LRU eviction
    use_count INTEGER DEFAULT 0                   -- For popularity-based eviction
);

-- specs: Cached OpenAPI specs
CREATE TABLE IF NOT EXISTS specs (
    sha256 TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    content JSONB NOT NULL,
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- fingerprints: Client fingerprints
CREATE TABLE IF NOT EXISTS fingerprints (
    id TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    fingerprint TEXT NOT NULL,                    -- Fingerprint string
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    regenerated_at TIMESTAMP,
    ip_address TEXT,
    UNIQUE(host, fingerprint)
);

-- revocation_cache: Cached revocation lists
CREATE TABLE IF NOT EXISTS revocation_cache (
    host TEXT PRIMARY KEY,
    content JSONB NOT NULL,
    etag TEXT,
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP
);

-- audit_log: Security audit events
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,         -- SQLite
    -- id SERIAL PRIMARY KEY,                      -- PostgreSQL
    -- id INT AUTO_INCREMENT PRIMARY KEY,          -- MySQL
    
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_type TEXT NOT NULL,
    client_id TEXT,
    user_id TEXT,
    fingerprint TEXT,
    host TEXT,
    details JSONB,
    severity TEXT DEFAULT 'info'                  -- debug, info, warn, error
);

-- rate_limit_state: Client-side rate limiting
CREATE TABLE IF NOT EXISTS rate_limit_state (
    id TEXT PRIMARY KEY,                          -- host + endpoint hash
    host TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    tokens INTEGER NOT NULL,                      -- Current token bucket
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    window_start TIMESTAMP                        -- Rate limit window
);

-- Index for common queries
CREATE INDEX IF NOT EXISTS idx_inventories_host ON inventories(host);
CREATE INDEX IF NOT EXISTS idx_inventories_fetched ON inventories(fetched_at);
CREATE INDEX IF NOT EXISTS idx_modules_name ON modules(name);
CREATE INDEX IF NOT EXISTS idx_modules_last_used ON modules(last_used_at);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_log(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_host ON audit_log(host);
```

> STATUS: HISTORICAL — this path was never built; no real counterpart exists.

### File: `migrations/001_initial_schema.down.sql` (never implemented)

```sql
DROP TABLE IF EXISTS rate_limit_state;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS revocation_cache;
DROP TABLE IF EXISTS fingerprints;
DROP TABLE IF EXISTS specs;
DROP TABLE IF EXISTS modules;
DROP TABLE IF EXISTS aliases;
DROP TABLE IF EXISTS inventories;
```

---

## 3. Database-Specific Variations

### SQLite-Specific (`migrations/001_initial_schema.sqlite.sql`)

```sql
-- SQLite uses INTEGER PRIMARY KEY for auto-increment
-- SQLite has JSON support via json1 extension
-- No ALTER TYPE support, must recreate tables

-- For SQLite, enable foreign keys
PRAGMA foreign_keys = ON;

-- Enable WAL mode for better concurrency
PRAGMA journal_mode = WAL;
```

### PostgreSQL-Specific (`migrations/001_initial_schema.postgres.sql`)

```sql
-- Use SERIAL for auto-increment
-- Use native JSONB type
-- Add indexes with specific types

-- For PostgreSQL audit_log
CREATE TABLE IF NOT EXISTS audit_log (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    event_type VARCHAR(50) NOT NULL,
    client_id VARCHAR(255),
    user_id VARCHAR(255),
    fingerprint VARCHAR(255),
    host VARCHAR(255),
    details JSONB,
    severity VARCHAR(20) DEFAULT 'info'
);

-- GIN index for JSONB queries
CREATE INDEX idx_audit_details ON audit_log USING GIN (details);

-- Partial index for error events
CREATE INDEX idx_audit_errors ON audit_log(timestamp) 
    WHERE severity = 'error';

-- Partition audit_log by month for large deployments
-- CREATE TABLE audit_log_2024_01 PARTITION OF audit_log
--     FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
```

### MySQL-Specific (`migrations/001_initial_schema.mysql.sql`)

```sql
-- Use AUTO_INCREMENT
-- Use JSON type (available in MySQL 5.7+)
-- Specify engine and charset

CREATE TABLE IF NOT EXISTS audit_log (
    id INT AUTO_INCREMENT PRIMARY KEY,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_type VARCHAR(50) NOT NULL,
    client_id VARCHAR(255),
    user_id VARCHAR(255),
    fingerprint VARCHAR(255),
    host VARCHAR(255),
    details JSON,
    severity VARCHAR(20) DEFAULT 'info',
    INDEX idx_timestamp (timestamp),
    INDEX idx_event_type (event_type),
    INDEX idx_host (host)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Full-text search on JSON (MySQL 8.0+)
-- ALTER TABLE audit_log ADD FULLTEXT INDEX idx_details (details);
```

---

## 4. sqlc Configuration

> STATUS: HISTORICAL — this path was never built; no real counterpart exists.

### File: `sqlc.yaml` (never implemented)

```yaml
version: "2"
sql:
  - schema: "migrations/001_initial_schema.up.sql"
    queries: "queries/"
    engine: "sqlite"
    gen:
      go:
        package: "db"
        out: "internal/db/sqlite"
        sql_package: "database/sql"
        overrides:
          - db_type: "JSONB"
            go_type: "encoding/json.RawMessage"
          - db_type: "TIMESTAMP"
            go_type: "time.Time"
          - db_type: "BLOB"
            go_type: "[]byte"

  - schema: "migrations/001_initial_schema.postgres.sql"
    queries: "queries/"
    engine: "postgresql"
    gen:
      go:
        package: "db"
        out: "internal/db/postgres"
        sql_package: "pgx/v5"
        overrides:
          - db_type: "jsonb"
            go_type: "encoding/json.RawMessage"
          - db_type: "timestamptz"
            go_type: "time.Time"

  - schema: "migrations/001_initial_schema.mysql.sql"
    queries: "queries/"
    engine: "mysql"
    gen:
      go:
        package: "db"
        out: "internal/db/mysql"
        sql_package: "database/sql"
        overrides:
          - db_type: "JSON"
            go_type: "encoding/json.RawMessage"
          - db_type: "TIMESTAMP"
            go_type: "time.Time"
```

---

## 5. SQL Queries

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sql/inventory_repository.go`.

### File: `queries/inventory.sql` (real: `pkg/repository/sql/inventory_repository.go`)

```sql
-- name: GetInventory :one
SELECT * FROM inventories 
WHERE host = $1 AND version = $2;

-- name: GetLatestInventory :one
SELECT * FROM inventories 
WHERE host = $1 
ORDER BY fetched_at DESC 
LIMIT 1;

-- name: ListInventoryVersions :many
SELECT version, fetched_at, sha256 
FROM inventories 
WHERE host = $1 
ORDER BY fetched_at DESC;

-- name: SaveInventory :exec
INSERT INTO inventories (id, host, version, content, sha256, etag, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (host, version) DO UPDATE SET
    content = EXCLUDED.content,
    sha256 = EXCLUDED.sha256,
    etag = EXCLUDED.etag,
    expires_at = EXCLUDED.expires_at,
    fetched_at = CURRENT_TIMESTAMP;

-- name: DeleteOldInventories :exec
DELETE FROM inventories 
WHERE host = $1 
AND fetched_at < datetime('now', '-' || $2 || ' days');

-- name: CountInventories :one
SELECT COUNT(*) FROM inventories WHERE host = $1;
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sql/alias_repository.go`.

### File: `queries/alias.sql` (real: `pkg/repository/sql/alias_repository.go`)

```sql
-- name: GetAlias :one
SELECT * FROM aliases WHERE name = $1;

-- name: ListAliases :many
SELECT * FROM aliases ORDER BY name;

-- name: CreateAlias :exec
INSERT INTO aliases (name, host, version, pinned, config, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO UPDATE SET
    host = EXCLUDED.host,
    version = EXCLUDED.version,
    pinned = EXCLUDED.pinned,
    config = EXCLUDED.config,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteAlias :exec
DELETE FROM aliases WHERE name = $1;

-- name: UpdateAliasVersion :exec
UPDATE aliases 
SET version = $2, updated_at = CURRENT_TIMESTAMP 
WHERE name = $1;
```

> STATUS: HISTORICAL — this path was never built; no real counterpart exists.

### File: `queries/module.sql` (never implemented)

```sql
-- name: GetModule :one
SELECT * FROM modules WHERE sha256 = $1;

-- name: SaveModule :exec
INSERT INTO modules (sha256, name, content, signature, size_bytes, downloaded_at)
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
ON CONFLICT (sha256) DO UPDATE SET
    last_used_at = CURRENT_TIMESTAMP,
    use_count = modules.use_count + 1;

-- name: UpdateModuleUsage :exec
UPDATE modules 
SET last_used_at = CURRENT_TIMESTAMP, use_count = use_count + 1 
WHERE sha256 = $1;

-- name: ListLRUModules :many
SELECT sha256, size_bytes, last_used_at 
FROM modules 
ORDER BY last_used_at ASC 
LIMIT $1;

-- name: DeleteModule :exec
DELETE FROM modules WHERE sha256 = $1;

-- name: GetTotalCacheSize :one
SELECT COALESCE(SUM(size_bytes), 0) FROM modules;

-- name: CountModules :one
SELECT COUNT(*) FROM modules;
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sql/audit_repository.go`.

### File: `queries/audit.sql` (real: `pkg/repository/sql/audit_repository.go`)

```sql
-- name: LogAuditEvent :exec
INSERT INTO audit_log (timestamp, event_type, client_id, user_id, fingerprint, host, details, severity)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetAuditEvents :many
SELECT * FROM audit_log 
WHERE host = $1 
AND timestamp >= $2 
AND timestamp <= $3
ORDER BY timestamp DESC 
LIMIT $4 OFFSET $5;

-- name: GetAuditEventsByType :many
SELECT * FROM audit_log 
WHERE event_type = $1 
AND timestamp >= $2 
ORDER BY timestamp DESC 
LIMIT $3;

-- name: GetRecentErrors :many
SELECT * FROM audit_log 
WHERE severity = 'error' 
AND timestamp >= datetime('now', '-1 day')
ORDER BY timestamp DESC 
LIMIT $1;

-- name: PurgeOldAuditLogs :exec
DELETE FROM audit_log 
WHERE timestamp < datetime('now', '-' || $1 || ' days');

-- name: CountAuditEvents :one
SELECT COUNT(*) FROM audit_log 
WHERE host = $1 AND timestamp >= $2;
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sql/fingerprint_repository.go`.

### File: `queries/fingerprint.sql` (real: `pkg/repository/sql/fingerprint_repository.go`)

```sql
-- name: GetFingerprint :one
SELECT * FROM fingerprints 
WHERE host = $1 AND fingerprint = $2;

-- name: SaveFingerprint :exec
INSERT INTO fingerprints (id, host, fingerprint, created_at, ip_address)
VALUES ($1, $2, $3, CURRENT_TIMESTAMP, $4)
ON CONFLICT (host, fingerprint) DO UPDATE SET
    regenerated_at = CURRENT_TIMESTAMP,
    ip_address = EXCLUDED.ip_address;

-- name: UpdateFingerprintIP :exec
UPDATE fingerprints 
SET regenerated_at = CURRENT_TIMESTAMP, ip_address = $2 
WHERE id = $1;

-- name: DeleteOldFingerprints :exec
DELETE FROM fingerprints 
WHERE created_at < datetime('now', '-90 days');
```

> STATUS: HISTORICAL — this path was never built; no real counterpart exists.

### File: `queries/revocation.sql` (never implemented)

```sql
-- name: GetRevocationCache :one
SELECT * FROM revocation_cache WHERE host = $1;

-- name: SaveRevocationCache :exec
INSERT INTO revocation_cache (host, content, etag, fetched_at, expires_at)
VALUES ($1, $2, $3, CURRENT_TIMESTAMP, $4)
ON CONFLICT (host) DO UPDATE SET
    content = EXCLUDED.content,
    etag = EXCLUDED.etag,
    fetched_at = CURRENT_TIMESTAMP,
    expires_at = EXCLUDED.expires_at;

-- name: DeleteRevocationCache :exec
DELETE FROM revocation_cache WHERE host = $1;
```

---

## 6. Go Database Layer

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sqlite.go`.

### File: `internal/db/store.go` (real: `pkg/repository/sqlite.go`)

```go
package db

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    _ "github.com/mattn/go-sqlite3"  // SQLite driver
    _ "github.com/lib/pq"            // PostgreSQL driver
    _ "github.com/go-sql-driver/mysql" // MySQL driver
)

// Store defines the database interface
type Store interface {
    // Inventory operations
    GetInventory(ctx context.Context, host, version string) (*Inventory, error)
    GetLatestInventory(ctx context.Context, host string) (*Inventory, error)
    ListInventoryVersions(ctx context.Context, host string) ([]InventoryVersion, error)
    SaveInventory(ctx context.Context, inv *Inventory) error
    DeleteOldInventories(ctx context.Context, host string, days int) error
    
    // Alias operations
    GetAlias(ctx context.Context, name string) (*Alias, error)
    ListAliases(ctx context.Context) ([]Alias, error)
    CreateAlias(ctx context.Context, alias *Alias) error
    DeleteAlias(ctx context.Context, name string) error
    UpdateAliasVersion(ctx context.Context, name, version string) error
    
    // Module operations
    GetModule(ctx context.Context, sha256 string) (*Module, error)
    SaveModule(ctx context.Context, mod *Module) error
    UpdateModuleUsage(ctx context.Context, sha256 string) error
    ListLRUModules(ctx context.Context, limit int) ([]Module, error)
    DeleteModule(ctx context.Context, sha256 string) error
    GetTotalCacheSize(ctx context.Context) (int64, error)
    CountModules(ctx context.Context) (int, error)
    
    // Audit operations
    LogAuditEvent(ctx context.Context, event *AuditEvent) error
    GetAuditEvents(ctx context.Context, host string, start, end time.Time, limit, offset int) ([]AuditEvent, error)
    PurgeOldAuditLogs(ctx context.Context, days int) error
    
    // Fingerprint operations
    GetFingerprint(ctx context.Context, host, fingerprint string) (*Fingerprint, error)
    SaveFingerprint(ctx context.Context, fp *Fingerprint) error
    UpdateFingerprintIP(ctx context.Context, id, ip string) error
    
    // Revocation operations
    GetRevocationCache(ctx context.Context, host string) (*RevocationCache, error)
    SaveRevocationCache(ctx context.Context, cache *RevocationCache) error
    
    // General
    Close() error
    Ping(ctx context.Context) error
}

// Inventory represents a cached inventory
type Inventory struct {
    ID          string          `db:"id" json:"id"`
    Host        string          `db:"host" json:"host"`
    Version     string          `db:"version" json:"version"`
    Content     json.RawMessage `db:"content" json:"content"`
    SHA256      string          `db:"sha256" json:"sha256"`
    FetchedAt   time.Time       `db:"fetched_at" json:"fetched_at"`
    ExpiresAt   sql.NullTime    `db:"expires_at" json:"expires_at,omitempty"`
    Etag        sql.NullString  `db:"etag" json:"etag,omitempty"`
}

// Alias represents a named inventory configuration
type Alias struct {
    Name      string          `db:"name" json:"name"`
    Host      string          `db:"host" json:"host"`
    Version   sql.NullString  `db:"version" json:"version,omitempty"`
    Pinned    bool            `db:"pinned" json:"pinned"`
    Config    json.RawMessage `db:"config" json:"config,omitempty"`
    CreatedAt time.Time       `db:"created_at" json:"created_at"`
    UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

// Module represents a cached WASM module
type Module struct {
    SHA256     string         `db:"sha256" json:"sha256"`
    Name       string         `db:"name" json:"name"`
    Content    []byte         `db:"content" json:"-"` // Don't serialize binary
    Signature  sql.NullString `db:"signature" json:"signature,omitempty"`
    SizeBytes  int64          `db:"size_bytes" json:"size_bytes"`
    DownloadedAt time.Time    `db:"downloaded_at" json:"downloaded_at"`
    LastUsedAt sql.NullTime   `db:"last_used_at" json:"last_used_at,omitempty"`
    UseCount   int64          `db:"use_count" json:"use_count"`
}

// AuditEvent represents a security audit log entry
type AuditEvent struct {
    ID          int64           `db:"id" json:"id"`
    Timestamp   time.Time       `db:"timestamp" json:"timestamp"`
    EventType   string          `db:"event_type" json:"event_type"`
    ClientID    sql.NullString  `db:"client_id" json:"client_id,omitempty"`
    UserID      sql.NullString  `db:"user_id" json:"user_id,omitempty"`
    Fingerprint sql.NullString  `db:"fingerprint" json:"fingerprint,omitempty"`
    Host        sql.NullString  `db:"host" json:"host,omitempty"`
    Details     json.RawMessage `db:"details" json:"details,omitempty"`
    Severity    string          `db:"severity" json:"severity"`
}

// Fingerprint represents a client fingerprint
type Fingerprint struct {
    ID           string         `db:"id" json:"id"`
    Host         string         `db:"host" json:"host"`
    Fingerprint  string         `db:"fingerprint" json:"fingerprint"`
    CreatedAt    time.Time      `db:"created_at" json:"created_at"`
    RegeneratedAt sql.NullTime  `db:"regenerated_at" json:"regenerated_at,omitempty"`
    IPAddress    sql.NullString `db:"ip_address" json:"ip_address,omitempty"`
}

// RevocationCache represents cached revocation list
type RevocationCache struct {
    Host       string          `db:"host" json:"host"`
    Content    json.RawMessage `db:"content" json:"content"`
    Etag       sql.NullString  `db:"etag" json:"etag,omitempty"`
    FetchedAt  time.Time       `db:"fetched_at" json:"fetched_at"`
    ExpiresAt  sql.NullTime    `db:"expires_at" json:"expires_at,omitempty"`
}
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/repository/sqlite.go`.

### File: `internal/db/sqlite/store.go` (real: `pkg/repository/sqlite.go`)

```go
package sqlite

import (
    "context"
    "database/sql"
    "fmt"
    
    "github.com/openapi-cli/core/internal/db"
    "github.com/openapi-cli/core/internal/db/sqlite/queries"
    _ "github.com/mattn/go-sqlite3"
)

// Store implements db.Store for SQLite
type Store struct {
    db      *sql.DB
    queries *queries.Queries
}

// New creates a new SQLite store
func New(dbPath string) (db.Store, error) {
    // Ensure directory exists
    if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
        return nil, fmt.Errorf("failed to create database directory: %w", err)
    }
    
    // Open database
    dsn := fmt.Sprintf("%s?_journal=WAL&_foreign_keys=ON", dbPath)
    database, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    
    // Test connection
    if err := database.Ping(); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }
    
    // Run migrations
    if err := runMigrations(database); err != nil {
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }
    
    return &Store{
        db:      database,
        queries: queries.New(database),
    }, nil
}

func (s *Store) GetInventory(ctx context.Context, host, version string) (*db.Inventory, error) {
    inv, err := s.queries.GetInventory(ctx, queries.GetInventoryParams{
        Host:    host,
        Version: version,
    })
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, db.ErrNotFound
        }
        return nil, err
    }
    
    return &db.Inventory{
        ID:        inv.ID,
        Host:      inv.Host,
        Version:   inv.Version,
        Content:   inv.Content,
        SHA256:    inv.Sha256,
        FetchedAt: inv.FetchedAt,
        ExpiresAt: inv.ExpiresAt,
        Etag:      inv.Etag,
    }, nil
}

func (s *Store) SaveInventory(ctx context.Context, inv *db.Inventory) error {
    return s.queries.SaveInventory(ctx, queries.SaveInventoryParams{
        ID:        inv.ID,
        Host:      inv.Host,
        Version:   inv.Version,
        Content:   inv.Content,
        Sha256:    inv.SHA256,
        Etag:      inv.Etag,
        ExpiresAt: inv.ExpiresAt,
    })
}

// ... implement other methods

func (s *Store) Close() error {
    return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
    return s.db.PingContext(ctx)
}
```

---

## 7. Migration Management

> STATUS: HISTORICAL — this path was never built; no real counterpart exists.

### File: `internal/db/migrate.go` (never implemented)

```go
package db

import (
    "embed"
    "fmt"
    
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/sqlite3"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/database/mysql"
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending migrations
func RunMigrations(databaseURL string) error {
    d, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("failed to create migration source: %w", err)
    }
    
    m, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
    if err != nil {
        return fmt.Errorf("failed to create migrator: %w", err)
    }
    
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("failed to apply migrations: %w", err)
    }
    
    return nil
}
```

---

## 8. Usage Examples

### Basic Operations

```go
// Initialize store
store, err := sqlite.New("~/.local/share/openapi-cli/cache.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Save inventory
inv := &db.Inventory{
    ID:      uuid.New().String(),
    Host:    "api.example.com",
    Version: "1.2.3",
    Content: json.RawMessage(`{...}`),
    SHA256:  "abc123...",
}
if err := store.SaveInventory(ctx, inv); err != nil {
    log.Error("Failed to save inventory", "error", err)
}

// Retrieve inventory
inv, err := store.GetInventory(ctx, "api.example.com", "1.2.3")
if err != nil {
    if err == db.ErrNotFound {
        // Cache miss - fetch from server
    }
}

// Cache eviction (LRU)
modules, err := store.ListLRUModules(ctx, 10)
if err != nil {
    log.Error("Failed to list LRU modules", "error", err)
}

for _, mod := range modules {
    if err := store.DeleteModule(ctx, mod.SHA256); err != nil {
        log.Error("Failed to delete module", "sha256", mod.SHA256, "error", err)
    }
}

// Audit logging
event := &db.AuditEvent{
    EventType: "INVENTORY_FETCH",
    Host:      sql.NullString{String: "api.example.com", Valid: true},
    Details:   json.RawMessage(`{"cache_hit": false}`),
    Severity:  "info",
}
if err := store.LogAuditEvent(ctx, event); err != nil {
    log.Error("Failed to log audit event", "error", err)
}
```

---

## Deterministic Guarantees

This database schema implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Schema Generation

Database schemas must be identical across platforms:

- **SQL dialect variations**: Use same CREATE TABLE statements with identical column ordering
- **Index creation**: Identical index definitions (same column order, same collation)
- **Constraint naming**: Deterministic constraint names (e.g., `pk_inventory_cache_host_version`)

### 2. Platform‑Independent Data Types

Data types must map consistently across database engines:

- **Integer types**: Use `INTEGER` for SQLite, `INT` for PostgreSQL/MySQL
- **Text encoding**: UTF‑8 with same collation (`C` locale for sorting)
- **Timestamp storage**: UTC timestamps with microsecond precision

### 3. Deterministic Migration Ordering

Database migrations must be applied in identical order:

- **Migration versioning**: Sequential integer versions, deterministic ordering
- **Rollback scripts**: Symmetric up/down migrations
- **Transaction boundaries**: Same transaction isolation level across platforms

### 4. Cross‑Platform SQLite Compatibility

SQLite database files must be portable across platforms:

- **Page size**: 4096 bytes (default)
- **Journal mode**: WAL (Write‑Ahead Logging)
- **Synchronous**: NORMAL mode for performance, FULL for data integrity

### 5. Deterministic Query Results

SQL queries must produce identical result ordering:

- **ORDER BY clauses**: Explicit ordering even when not required
- **GROUP BY aggregation**: Deterministic aggregate function results
- **JOIN ordering**: Consistent table join order

### 6. Testing Determinism

All database tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestDatabaseDeterminism(t *testing.T) {
	// Verify identical schema creation
	// Verify identical migration application
	// Verify identical query results
}
```

### 7. Integration with SDK Generation

When generating SDKs for 12 languages, database schemas must:

- Use identical table definitions
- Generate identical query interfaces
- Enforce identical constraints

**Verification**: Run `./scripts/validate‑determinism‑database.sh` to confirm cross‑platform consistency.

This specification provides complete database schema definitions for SQLite, PostgreSQL, and MySQL with sqlc integration, migrations, and Go implementation examples for Ralph Loop.
