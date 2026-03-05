# Database Connection Management

Complete database connection management specification with connection pooling, multi-driver support, transactions, and health checks.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Database Manager                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ SQLite Pool │  │ PostgreSQL  │  │   MySQL     │             │
│  │  (CGO)      │  │ Pool (pq)   │  │  Pool       │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
├─────────────────────────────────────────────────────────────────┤
│  Connection Lifecycle │ Transaction Manager │ Health Checker  │
└─────────────────────────────────────────────────────────────────┘
```

## 1. Connection Pool

### Pool Configuration

**Location:** `internal/db/pool.go`

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// PoolConfig defines connection pool settings
type PoolConfig struct {
	// Pool sizing
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`       // Maximum open connections
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`       // Maximum idle connections
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"` // Max connection lifetime
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"` // Max idle time

	// Timeouts
	ConnectTimeout  time.Duration `json:"connect_timeout" yaml:"connect_timeout"`   // Connection establishment timeout
	QueryTimeout    time.Duration `json:"query_timeout" yaml:"query_timeout"`         // Query execution timeout

	// Retry settings
	MaxRetries      int           `json:"max_retries" yaml:"max_retries"`             // Max retry attempts
	RetryDelay      time.Duration `json:"retry_delay" yaml:"retry_delay"`             // Delay between retries

	// Health check
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
}

// DefaultPoolConfig returns sensible defaults
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxOpenConns:        25,
		MaxIdleConns:        5,
		ConnMaxLifetime:     time.Hour,
		ConnMaxIdleTime:     time.Minute * 30,
		ConnectTimeout:      time.Second * 10,
		QueryTimeout:        time.Second * 30,
		MaxRetries:          3,
		RetryDelay:          time.Second * 1,
		HealthCheckInterval: time.Second * 30,
	}
}

// ConnectionPool wraps sql.DB with enhanced functionality
type ConnectionPool struct {
	db     *sql.DB
	config *PoolConfig
	driver string
	mutex  sync.RWMutex
	
	// Metrics
	stats PoolStats
}

// PoolStats tracks pool metrics
type PoolStats struct {
	OpenConnections     int32     `json:"open_connections"`
	InUseConnections    int32     `json:"in_use_connections"`
	IdleConnections     int32     `json:"idle_connections"`
	WaitCount           int64     `json:"wait_count"`
	WaitDuration        time.Duration `json:"wait_duration"`
	MaxIdleClosed       int64     `json:"max_idle_closed"`
	MaxLifetimeClosed   int64     `json:"max_lifetime_closed"`
	LastHealthCheck     time.Time `json:"last_health_check"`
	HealthCheckStatus   string    `json:"health_check_status"`
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(driver string, dsn string, config *PoolConfig) (*ConnectionPool, error) {
	if config == nil {
		config = DefaultPoolConfig()
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure pool settings
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Verify connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	pool := &ConnectionPool{
		db:     db,
		config: config,
		driver: driver,
	}

	// Start background health checker
	go pool.healthCheckLoop()

	return pool, nil
}

// DB returns the underlying sql.DB
func (p *ConnectionPool) DB() *sql.DB {
	return p.db
}

// Close gracefully closes the pool
func (p *ConnectionPool) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Stats returns current pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.db == nil {
		return PoolStats{}
	}

	stats := p.db.Stats()
	return PoolStats{
		OpenConnections:   stats.OpenConnections,
		InUseConnections:  stats.InUse,
		IdleConnections:   stats.Idle,
		WaitCount:         stats.WaitCount,
		WaitDuration:      stats.WaitDuration,
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
	}
}

// healthCheckLoop runs periodic health checks
func (p *ConnectionPool) healthCheckLoop() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := p.HealthCheck(); err != nil {
			p.mutex.Lock()
			p.stats.HealthCheckStatus = "unhealthy"
			p.mutex.Unlock()
		} else {
			p.mutex.Lock()
			p.stats.HealthCheckStatus = "healthy"
			p.stats.LastHealthCheck = time.Now()
			p.mutex.Unlock()
		}
	}
}

// HealthCheck performs a ping check
func (p *ConnectionPool) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
	defer cancel()
	return p.db.PingContext(ctx)
}

// WithRetry executes an operation with retry logic
func (p *ConnectionPool) WithRetry(operation func() error) error {
	var lastErr error
	for i := 0; i <= p.config.MaxRetries; i++ {
		if err := operation(); err != nil {
			lastErr = err
			if i < p.config.MaxRetries {
				time.Sleep(p.config.RetryDelay * time.Duration(i+1))
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("operation failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

// IsHealthy returns true if pool is healthy
func (p *ConnectionPool) IsHealthy() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.stats.HealthCheckStatus == "healthy"
}

// GetQueryContext returns context with query timeout
func (p *ConnectionPool) GetQueryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.config.QueryTimeout)
}

## 2. Driver Implementations

### Driver Interface

**Location:** `internal/db/driver.go`

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Driver interface for database implementations
type Driver interface {
	// Name returns the driver name
	Name() string
	
	// Open creates a new connection pool
	Open(dsn string, config *PoolConfig) (*ConnectionPool, error)
	
	// DSNBuilder creates DSN from configuration
	DSNBuilder() DSNBuilder
	
	// Features returns supported features
	Features() DriverFeatures
	
	// QuoteIdentifier quotes an identifier (table/column name)
	QuoteIdentifier(identifier string) string
	
	// Placeholder returns parameterized placeholder (?, $1, etc.)
	Placeholder(index int) string
	
	// IsDuplicateError checks if error is a duplicate key error
	IsDuplicateError(err error) bool
	
	// IsNotFoundError checks if error is a not found error
	IsNotFoundError(err error) bool
	
	// IsConnectionError checks if error is a connection error
	IsConnectionError(err error) bool
}

// DriverFeatures indicates supported database features
type DriverFeatures struct {
	SupportsTransactions   bool
	SupportsSavepoints       bool
	SupportsReturning        bool // RETURNING clause
	SupportsJSON             bool
	SupportsJSONB            bool // PostgreSQL specific
	SupportsArrays           bool
	SupportsCTE              bool // Common Table Expressions
	SupportsWindowFunctions  bool
	SupportsUpsert           bool // ON CONFLICT/ON DUPLICATE KEY
	MaxIdentifierLength      int
	SupportsSSL              bool
	SupportsMultiSchema      bool
}

// DSNBuilder creates database-specific DSN strings
type DSNBuilder interface {
	// WithHost sets the database host
	WithHost(host string) DSNBuilder
	
	// WithPort sets the database port
	WithPort(port int) DSNBuilder
	
	// WithDatabase sets the database name
	WithDatabase(database string) DSNBuilder
	
	// WithUser sets the username
	WithUser(user string) DSNBuilder
	
	// WithPassword sets the password
	WithPassword(password string) DSNBuilder
	
	// WithSSLMode sets SSL mode (disable, require, verify-ca, verify-full)
	WithSSLMode(mode string) DSNBuilder
	
	// WithSSLCert sets SSL certificate path
	WithSSLCert(certPath string) DSNBuilder
	
	// WithSSLKey sets SSL key path
	WithSSLKey(keyPath string) DSNBuilder
	
	// WithSSLRootCert sets SSL root certificate path
	WithSSLRootCert(rootCertPath string) DSNBuilder
	
	// WithParams adds additional parameters
	WithParams(params map[string]string) DSNBuilder
	
	// Build returns the constructed DSN
	Build() string
}

// BaseDSNBuilder provides common DSN builder functionality
type BaseDSNBuilder struct {
	host           string
	port           int
	database       string
	user           string
	password       string
	sslMode        string
	sslCert        string
	sslKey         string
	sslRootCert    string
	params         map[string]string
}

// NewBaseDSNBuilder creates a new base DSN builder
func NewBaseDSNBuilder() *BaseDSNBuilder {
	return &BaseDSNBuilder{
		params: make(map[string]string),
	}
}

func (b *BaseDSNBuilder) WithHost(host string) DSNBuilder {
	b.host = host
	return b
}

func (b *BaseDSNBuilder) WithPort(port int) DSNBuilder {
	b.port = port
	return b
}

func (b *BaseDSNBuilder) WithDatabase(database string) DSNBuilder {
	b.database = database
	return b
}

func (b *BaseDSNBuilder) WithUser(user string) DSNBuilder {
	b.user = user
	return b
}

func (b *BaseDSNBuilder) WithPassword(password string) DSNBuilder {
	b.password = password
	return b
}

func (b *BaseDSNBuilder) WithSSLMode(mode string) DSNBuilder {
	b.sslMode = mode
	return b
}

func (b *BaseDSNBuilder) WithSSLCert(certPath string) DSNBuilder {
	b.sslCert = certPath
	return b
}

func (b *BaseDSNBuilder) WithSSLKey(keyPath string) DSNBuilder {
	b.sslKey = keyPath
	return b
}

func (b *BaseDSNBuilder) WithSSLRootCert(rootCertPath string) DSNBuilder {
	b.sslRootCert = rootCertPath
	return b
}

func (b *BaseDSNBuilder) WithParams(params map[string]string) DSNBuilder {
	for k, v := range params {
		b.params[k] = v
	}
	return b
}

// DriverRegistry manages available drivers
type DriverRegistry struct {
	drivers map[string]Driver
	mutex   sync.RWMutex
}

// NewDriverRegistry creates a new driver registry
func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{
		drivers: make(map[string]Driver),
	}
}

// Register adds a driver to the registry
func (r *DriverRegistry) Register(driver Driver) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if _, exists := r.drivers[driver.Name()]; exists {
		return fmt.Errorf("driver %s already registered", driver.Name())
	}
	
	r.drivers[driver.Name()] = driver
	return nil
}

// Get retrieves a driver by name
func (r *DriverRegistry) Get(name string) (Driver, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	driver, exists := r.drivers[name]
	if !exists {
		return nil, fmt.Errorf("driver %s not found", name)
	}
	
	return driver, nil
}

// List returns all registered driver names
func (r *DriverRegistry) List() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	names := make([]string, 0, len(r.drivers))
	for name := range r.drivers {
		names = append(names, name)
	}
	return names
}

// Global driver registry
var DefaultDriverRegistry = NewDriverRegistry()

// RegisterDriver registers a driver globally
func RegisterDriver(driver Driver) error {
	return DefaultDriverRegistry.Register(driver)
}

// GetDriver retrieves a driver from the global registry
func GetDriver(name string) (Driver, error) {
	return DefaultDriverRegistry.Get(name)
}
```

### 2.1 SQLite Driver (CGO)

**Location:** `internal/db/drivers/sqlite/sqlite.go`

```go
//go:build cgo
// +build cgo

package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	
	"github.com/mattn/go-sqlite3"
	"muster/internal/db"
)

func init() {
	db.RegisterDriver(&SQLiteDriver{})
}

// SQLiteDriver implements the Driver interface for SQLite
type SQLiteDriver struct{}

// Name returns the driver name
func (d *SQLiteDriver) Name() string {
	return "sqlite3"
}

// Open creates a new SQLite connection pool
func (d *SQLiteDriver) Open(dsn string, config *db.PoolConfig) (*db.ConnectionPool, error) {
	// SQLite specific: configure busy timeout and other pragmas
	connPool, err := db.NewConnectionPool("sqlite3", dsn, config)
	if err != nil {
		return nil, err
	}
	
	// Set SQLite pragmas for better concurrency
	db := connPool.DB()
	
	// WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	
	// Foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	
	// Synchronous mode (NORMAL for better performance)
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}
	
	// Cache size (negative value = KB)
	if _, err := db.Exec("PRAGMA cache_size=-64000"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set cache size: %w", err)
	}
	
	// Busy timeout (milliseconds)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}
	
	// Temporary storage (memory)
	if _, err := db.Exec("PRAGMA temp_store=memory"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set temp store: %w", err)
	}
	
	return connPool, nil
}

// DSNBuilder returns a SQLite-specific DSN builder
func (d *SQLiteDriver) DSNBuilder() db.DSNBuilder {
	return &SQLiteDSNBuilder{
		BaseDSNBuilder: db.NewBaseDSNBuilder(),
		mode:           "rwc",
		cache:          "shared",
		params:         make(map[string]string),
	}
}

// Features returns SQLite capabilities
func (d *SQLiteDriver) Features() db.DriverFeatures {
	return db.DriverFeatures{
		SupportsTransactions:  true,
		SupportsSavepoints:    true,
		SupportsReturning:     true, // SQLite 3.35.0+
		SupportsJSON:          true,
		SupportsJSONB:         false,
		SupportsArrays:        false,
		SupportsCTE:           true,
		SupportsWindowFunctions: true,
		SupportsUpsert:        true,
		MaxIdentifierLength:   128,
		SupportsSSL:           false,
		SupportsMultiSchema:   false,
	}
}

// QuoteIdentifier quotes an identifier
func (d *SQLiteDriver) QuoteIdentifier(identifier string) string {
	return fmt.Sprintf(`"%s"`, strings.Replace(identifier, `"`, `""`, -1))
}

// Placeholder returns parameterized placeholder
func (d *SQLiteDriver) Placeholder(index int) string {
	return "?"
}

// IsDuplicateError checks for duplicate key errors
func (d *SQLiteDriver) IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// IsNotFoundError checks for not found errors
func (d *SQLiteDriver) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows")
}

// IsConnectionError checks for connection errors
func (d *SQLiteDriver) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite errors that indicate connection issues
	return strings.Contains(err.Error(), "unable to open") ||
		strings.Contains(err.Error(), "disk I/O error") ||
		strings.Contains(err.Error(), "database is locked")
}

// SQLiteDSNBuilder builds SQLite DSN strings
type SQLiteDSNBuilder struct {
	*db.BaseDSNBuilder
	mode     string // ro, rw, rwc, memory
	cache    string // shared, private
	params   map[string]string
}

// WithMode sets the access mode
func (b *SQLiteDSNBuilder) WithMode(mode string) *SQLiteDSNBuilder {
	b.mode = mode
	return b
}

// WithCache sets the cache mode
func (b *SQLiteDSNBuilder) WithCache(cache string) *SQLiteDSNBuilder {
	b.cache = cache
	return b
}

// Build constructs the SQLite DSN
func (b *SQLiteDSNBuilder) Build() string {
	// Handle in-memory database
	if b.database == ":memory:" {
		return ":memory:"
	}
	
	// Handle file path
	path := b.database
	if path == "" {
		path = "file::memory:?cache=shared"
	}
	
	// Build query parameters
	params := make(url.Values)
	
	params.Set("mode", b.mode)
	params.Set("cache", b.cache)
	
	// Add additional parameters
	for k, v := range b.params {
		params.Set(k, v)
	}
	
	// Construct DSN
	if strings.HasPrefix(path, "file:") {
		return fmt.Sprintf("%s?%s", path, params.Encode())
	}
	
	// Ensure absolute path
	if !filepath.IsAbs(path) {
		// Relative to current directory
		path = filepath.Join(".", path)
	}
	
	return fmt.Sprintf("file:%s?%s", path, params.Encode())
}

// MigrateOptions returns sqlite3-specific migration options
func (d *SQLiteDriver) MigrateOptions() sqlite3.SQLiteDriver {
	return sqlite3.SQLiteDriver{}
}
```

### 2.2 PostgreSQL Driver (pq)

**Location:** `internal/db/drivers/postgres/postgres.go`

```go
package postgres

import (
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	
	_ "github.com/lib/pq" // PostgreSQL driver
	"muster/internal/db"
)

func init() {
	db.RegisterDriver(&PostgresDriver{})
}

// PostgresDriver implements the Driver interface for PostgreSQL
type PostgresDriver struct{}

// Name returns the driver name
func (d *PostgresDriver) Name() string {
	return "postgres"
}

// Open creates a new PostgreSQL connection pool
func (d *PostgresDriver) Open(dsn string, config *db.PoolConfig) (*db.ConnectionPool, error) {
	connPool, err := db.NewConnectionPool("postgres", dsn, config)
	if err != nil {
		return nil, err
	}
	
	// Set PostgreSQL-specific settings
	db := connPool.DB()
	
	// Application name for connection identification
	if _, err := db.Exec("SET application_name = 'muster'"); err != nil {
		// Non-critical, log but continue
		fmt.Printf("warning: failed to set application name: %v\n", err)
	}
	
	// Timezone
	if _, err := db.Exec("SET timezone = 'UTC'"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set timezone: %w", err)
	}
	
	return connPool, nil
}

// DSNBuilder returns a PostgreSQL-specific DSN builder
func (d *PostgresDriver) DSNBuilder() db.DSNBuilder {
	return &PostgresDSNBuilder{
		BaseDSNBuilder: db.NewBaseDSNBuilder(),
		sslMode:        "prefer",
		params:         make(map[string]string),
	}
}

// Features returns PostgreSQL capabilities
func (d *PostgresDriver) Features() db.DriverFeatures {
	return db.DriverFeatures{
		SupportsTransactions:    true,
		SupportsSavepoints:      true,
		SupportsReturning:       true,
		SupportsJSON:            true,
		SupportsJSONB:           true,
		SupportsArrays:          true,
		SupportsCTE:             true,
		SupportsWindowFunctions: true,
		SupportsUpsert:          true,
		MaxIdentifierLength:     63,
		SupportsSSL:             true,
		SupportsMultiSchema:     true,
	}
}

// QuoteIdentifier quotes an identifier
func (d *PostgresDriver) QuoteIdentifier(identifier string) string {
	return fmt.Sprintf(`"%s"`, strings.Replace(identifier, `"`, `""`, -1))
}

// Placeholder returns parameterized placeholder
func (d *PostgresDriver) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// IsDuplicateError checks for duplicate key errors
func (d *PostgresDriver) IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique violation error code: 23505
	return strings.Contains(err.Error(), "pq: duplicate key value") ||
		strings.Contains(err.Error(), "23505")
}

// IsNotFoundError checks for not found errors
func (d *PostgresDriver) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows in result")
}

// IsConnectionError checks for connection errors
func (d *PostgresDriver) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "pq: SSL") ||
		strings.Contains(errStr, "dial tcp")
}

// PostgresDSNBuilder builds PostgreSQL DSN strings
type PostgresDSNBuilder struct {
	*db.BaseDSNBuilder
	sslMode string
	params  map[string]string
}

// WithSSLMode overrides the base method for PostgreSQL
func (b *PostgresDSNBuilder) WithSSLMode(mode string) db.DSNBuilder {
	b.sslMode = mode
	return b
}

// Build constructs the PostgreSQL DSN
func (b *PostgresDSNBuilder) Build() string {
	// Start with host
	host := b.host
	if host == "" {
		host = "localhost"
	}
	
	// Default port
	port := b.port
	if port == 0 {
		port = 5432
	}
	
	// Build DSN using URL format (preferred for pq)
	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   b.database,
	}
	
	// Add user and password
	if b.user != "" {
		u.User = url.UserPassword(b.user, b.password)
	}
	
	// Build query parameters
	q := u.Query()
	
	// SSL mode
	if b.sslMode != "" {
		q.Set("sslmode", b.sslMode)
	}
	
	// SSL certificates
	if b.sslCert != "" {
		q.Set("sslcert", b.sslCert)
	}
	if b.sslKey != "" {
		q.Set("sslkey", b.sslKey)
	}
	if b.sslRootCert != "" {
		q.Set("sslrootcert", b.sslRootCert)
	}
	
	// Additional parameters
	for k, v := range b.params {
		q.Set(k, v)
	}
	
	// Important parameters for connection pooling
	q.Set("connect_timeout", "10")
	
	u.RawQuery = q.Encode()
	
	return u.String()
}

// BuildDSN returns legacy DSN format (for compatibility)
func (b *PostgresDSNBuilder) BuildDSN() string {
	// Key-value format
	parts := []string{}
	
	if b.host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", b.host))
	}
	if b.port != 0 {
		parts = append(parts, fmt.Sprintf("port=%d", b.port))
	}
	if b.database != "" {
		parts = append(parts, fmt.Sprintf("dbname=%s", b.database))
	}
	if b.user != "" {
		parts = append(parts, fmt.Sprintf("user=%s", b.user))
	}
	if b.password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", b.password))
	}
	if b.sslMode != "" {
		parts = append(parts, fmt.Sprintf("sslmode=%s", b.sslMode))
	}
	if b.sslCert != "" {
		parts = append(parts, fmt.Sprintf("sslcert=%s", b.sslCert))
	}
	if b.sslKey != "" {
		parts = append(parts, fmt.Sprintf("sslkey=%s", b.sslKey))
	}
	if b.sslRootCert != "" {
		parts = append(parts, fmt.Sprintf("sslrootcert=%s", b.sslRootCert))
	}
	
	// Add additional parameters
	for k, v := range b.params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	
	return strings.Join(parts, " ")
}
```

### 2.3 MySQL Driver

**Location:** `internal/db/drivers/mysql/mysql.go`

```go
package mysql

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
	
	_ "github.com/go-sql-driver/mysql" // MySQL driver
	"muster/internal/db"
)

func init() {
	db.RegisterDriver(&MySQLDriver{})
}

// MySQLDriver implements the Driver interface for MySQL
type MySQLDriver struct{}

// Name returns the driver name
func (d *MySQLDriver) Name() string {
	return "mysql"
}

// Open creates a new MySQL connection pool
func (d *MySQLDriver) Open(dsn string, config *db.PoolConfig) (*db.ConnectionPool, error) {
	connPool, err := db.NewConnectionPool("mysql", dsn, config)
	if err != nil {
		return nil, err
	}
	
	// MySQL-specific settings
	db := connPool.DB()
	
	// Set session timezone to UTC
	if _, err := db.Exec("SET time_zone = '+00:00'"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set timezone: %w", err)
	}
	
	// SQL mode for strictness
	if _, err := db.Exec("SET SESSION sql_mode = 'STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"); err != nil {
		connPool.Close()
		return nil, fmt.Errorf("failed to set sql_mode: %w", err)
	}
	
	return connPool, nil
}

// DSNBuilder returns a MySQL-specific DSN builder
func (d *MySQLDriver) DSNBuilder() db.DSNBuilder {
	return &MySQLDSNBuilder{
		BaseDSNBuilder: db.NewBaseDSNBuilder(),
		params:         make(map[string]string),
		collation:      "utf8mb4_unicode_ci",
		parseTime:      true,
		loc:            "UTC",
	}
}

// Features returns MySQL capabilities
func (d *MySQLDriver) Features() db.DriverFeatures {
	return db.DriverFeatures{
		SupportsTransactions:    true,
		SupportsSavepoints:      true,
		SupportsReturning:       false, // MySQL 8.0.19+ but limited
		SupportsJSON:            true,
		SupportsJSONB:           false,
		SupportsArrays:          false,
		SupportsCTE:             true, // MySQL 8.0+
		SupportsWindowFunctions: true, // MySQL 8.0+
		SupportsUpsert:          true, // INSERT ... ON DUPLICATE KEY UPDATE
		MaxIdentifierLength:     64,
		SupportsSSL:             true,
		SupportsMultiSchema:     true,
	}
}

// QuoteIdentifier quotes an identifier
func (d *MySQLDriver) QuoteIdentifier(identifier string) string {
	return fmt.Sprintf("`%s`", strings.Replace(identifier, "`", "``", -1))
}

// Placeholder returns parameterized placeholder
func (d *MySQLDriver) Placeholder(index int) string {
	return "?"
}

// IsDuplicateError checks for duplicate key errors
func (d *MySQLDriver) IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	// MySQL error code 1062 = duplicate entry
	return strings.Contains(err.Error(), "Error 1062") ||
		strings.Contains(err.Error(), "Duplicate entry")
}

// IsNotFoundError checks for not found errors
func (d *MySQLDriver) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows in result")
}

// IsConnectionError checks for connection errors
func (d *MySQLDriver) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "bad connection") ||
		strings.Contains(errStr, "invalid connection") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "Too many connections")
}

// MySQLDSNBuilder builds MySQL DSN strings
type MySQLDSNBuilder struct {
	*db.BaseDSNBuilder
	collation   string
	parseTime   bool
	loc         string
	tlsConfig   string
	params      map[string]string
}

// WithCollation sets the collation
func (b *MySQLDSNBuilder) WithCollation(collation string) *MySQLDSNBuilder {
	b.collation = collation
	return b
}

// WithParseTime sets parseTime parameter
func (b *MySQLDSNBuilder) WithParseTime(parse bool) *MySQLDSNBuilder {
	b.parseTime = parse
	return b
}

// WithLocation sets the location
func (b *MySQLDSNBuilder) WithLocation(loc string) *MySQLDSNBuilder {
	b.loc = loc
	return b
}

// WithTLS sets TLS configuration
func (b *MySQLDSNBuilder) WithTLS(tls string) *MySQLDSNBuilder {
	b.tlsConfig = tls
	return b
}

// Build constructs the MySQL DSN
func (b *MySQLDSNBuilder) Build() string {
	// Build using Config struct from go-sql-driver/mysql
	cfg := &mysql.Config{
		User:   b.user,
		Passwd: b.password,
		Net:    "tcp",
		Addr:   fmt.Sprintf("%s:%d", b.getHost(), b.getPort()),
		DBName: b.database,
		Params: make(map[string]string),
	}
	
	// Collation
	cfg.Collation = b.collation
	
	// Location
	if b.loc != "" {
		cfg.Loc, _ = time.LoadLocation(b.loc)
	}
	
	// ParseTime
	cfg.ParseTime = b.parseTime
	
	// TLS
	if b.tlsConfig != "" {
		cfg.TLSConfig = b.tlsConfig
	}
	
	// Timeout
	cfg.Timeout = b.ConnectTimeout
	
	// Additional parameters
	for k, v := range b.params {
		cfg.Params[k] = v
	}
	
	return cfg.FormatDSN()
}

func (b *MySQLDSNBuilder) getHost() string {
	if b.host != "" {
		return b.host
	}
	return "localhost"
}

func (b *MySQLDSNBuilder) getPort() int {
	if b.port != 0 {
		return b.port
	}
	return 3306
}

// BuildURL constructs a URL-style DSN (for compatibility)
func (b *MySQLDSNBuilder) BuildURL() string {
	u := &url.URL{
		Scheme: "mysql",
		User:   url.UserPassword(b.user, b.password),
		Host:   fmt.Sprintf("%s:%d", b.getHost(), b.getPort()),
		Path:   "/" + b.database,
	}
	
	q := u.Query()
	q.Set("charset", "utf8mb4")
	q.Set("collation", b.collation)
	q.Set("parseTime", fmt.Sprintf("%t", b.parseTime))
	q.Set("loc", b.loc)
	
	if b.tlsConfig != "" {
		q.Set("tls", b.tlsConfig)
	}
	
	// Additional parameters
	for k, v := range b.params {
		q.Set(k, v)
	}
	
	u.RawQuery = q.Encode()
	
	return u.String()
}

## 3. Transaction Management

### Transaction Interface

**Location:** `internal/db/transaction.go`

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Transaction represents a database transaction
type Transaction interface {
	// Commit commits the transaction
	Commit() error
	
	// Rollback rolls back the transaction
	Rollback() error
	
	// Query executes a query returning multiple rows
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	
	// QueryRow executes a query returning a single row
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row
	
	// Exec executes a query without returning rows
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	
	// Prepare creates a prepared statement
	Prepare(ctx context.Context, query string) (*sql.Stmt, error)
	
	// IsCommitted returns true if transaction is committed
	IsCommitted() bool
	
	// IsRolledBack returns true if transaction is rolled back
	IsRolledBack() bool
	
	// IsActive returns true if transaction is still active
	IsActive() bool
}

// Savepoint represents a transaction savepoint
type Savepoint interface {
	Transaction
	
	// ReleaseSavepoint releases the savepoint
	ReleaseSavepoint() error
	
	// RollbackToSavepoint rolls back to this savepoint
	RollbackToSavepoint() error
}

// TransactionOptions configures transaction behavior
type TransactionOptions struct {
	// Isolation level: ReadUncommitted, ReadCommitted, WriteCommitted, RepeatableRead, Snapshot, Serializable, Linerizable
	Isolation sql.IsolationLevel
	
	// ReadOnly transaction
	ReadOnly bool
}

// DefaultTransactionOptions returns default transaction options
func DefaultTransactionOptions() *TransactionOptions {
	return &TransactionOptions{
		Isolation: sql.LevelDefault,
		ReadOnly:  false,
	}
}

// TransactionManager handles transaction lifecycle
type TransactionManager struct {
	pool *ConnectionPool
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(pool *ConnectionPool) *TransactionManager {
	return &TransactionManager{pool: pool}
}

// Begin starts a new transaction
func (tm *TransactionManager) Begin(ctx context.Context) (*Tx, error) {
	return tm.BeginTx(ctx, DefaultTransactionOptions())
}

// BeginTx starts a new transaction with options
func (tm *TransactionManager) BeginTx(ctx context.Context, opts *TransactionOptions) (*Tx, error) {
	tx, err := tm.pool.DB().BeginTx(ctx, &sql.TxOptions{
		Isolation: opts.Isolation,
		ReadOnly:  opts.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	return &Tx{
		tx:     tx,
		pool:   tm.pool,
		active: true,
	}, nil
}

// RunInTransaction executes a function within a transaction
func (tm *TransactionManager) RunInTransaction(ctx context.Context, fn func(Transaction) error) error {
	return tm.RunInTransactionWithOptions(ctx, DefaultTransactionOptions(), fn)
}

// RunInTransactionWithOptions executes a function within a transaction with options
func (tm *TransactionManager) RunInTransactionWithOptions(
	ctx context.Context,
	opts *TransactionOptions,
	fn func(Transaction) error,
) error {
	tx, err := tm.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	
	defer func() {
		if tx.IsActive() {
			_ = tx.Rollback()
		}
	}()
	
	if err := fn(tx); err != nil {
		return err
	}
	
	return tx.Commit()
}

// Tx wraps sql.Tx with additional functionality
type Tx struct {
	tx       *sql.Tx
	pool     *ConnectionPool
	active   bool
	committed bool
	rolledBack bool
	savepoints  []*SavepointTx
	mutex       sync.RWMutex
}

// Commit commits the transaction
func (t *Tx) Commit() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	if !t.active {
		return fmt.Errorf("transaction is not active")
	}
	
	if t.committed {
		return fmt.Errorf("transaction already committed")
	}
	
	if t.rolledBack {
		return fmt.Errorf("transaction has been rolled back")
	}
	
	// Release all savepoints first
	for _, sp := range t.savepoints {
		if sp.active {
			if err := sp.ReleaseSavepoint(); err != nil {
				return fmt.Errorf("failed to release savepoint: %w", err)
			}
		}
	}
	
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	t.committed = true
	t.active = false
	return nil
}

// Rollback rolls back the transaction
func (t *Tx) Rollback() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	if !t.active {
		return nil // Already finished
	}
	
	if t.committed {
		return fmt.Errorf("transaction already committed")
	}
	
	if t.rolledBack {
		return nil // Already rolled back
	}
	
	err := t.tx.Rollback()
	if err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	
	t.rolledBack = true
	t.active = false
	return nil
}

// Query executes a query
func (t *Tx) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	if !t.active {
		return nil, fmt.Errorf("transaction is not active")
	}
	
	return t.tx.QueryContext(ctx, query, args...)
}

// QueryRow executes a query returning a single row
func (t *Tx) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	if !t.active {
		return nil
	}
	
	return t.tx.QueryRowContext(ctx, query, args...)
}

// Exec executes a query without returning rows
func (t *Tx) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	if !t.active {
		return nil, fmt.Errorf("transaction is not active")
	}
	
	return t.tx.ExecContext(ctx, query, args...)
}

// Prepare creates a prepared statement
func (t *Tx) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	if !t.active {
		return nil, fmt.Errorf("transaction is not active")
	}
	
	return t.tx.PrepareContext(ctx, query)
}

// IsCommitted returns true if transaction is committed
func (t *Tx) IsCommitted() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.committed
}

// IsRolledBack returns true if transaction is rolled back
func (t *Tx) IsRolledBack() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.rolledBack
}

// IsActive returns true if transaction is still active
func (t *Tx) IsActive() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.active
}

// CreateSavepoint creates a new savepoint
func (t *Tx) CreateSavepoint(name string) (*SavepointTx, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	if !t.active {
		return nil, fmt.Errorf("transaction is not active")
	}
	
	if t.pool.driver != "postgres" && t.pool.driver != "sqlite3" {
		return nil, fmt.Errorf("savepoints not supported for driver %s", t.pool.driver)
	}
	
	// Create savepoint
	_, err := t.tx.Exec(fmt.Sprintf("SAVEPOINT %s", name))
	if err != nil {
		return nil, fmt.Errorf("failed to create savepoint: %w", err)
	}
	
	sp := &SavepointTx{
		tx:   t,
		name: name,
		active: true,
	}
	
	t.savepoints = append(t.savepoints, sp)
	return sp, nil
}

// SavepointTx represents a transaction savepoint
type SavepointTx struct {
	tx       *Tx
	name     string
	active   bool
	released bool
	savepoint *SavepointTx // Nested savepoint
}

// Commit commits the savepoint (releases it)
func (s *SavepointTx) Commit() error {
	return s.ReleaseSavepoint()
}

// Rollback rolls back the savepoint
func (s *SavepointTx) Rollback() error {
	return s.RollbackToSavepoint()
}

// ReleaseSavepoint releases the savepoint
func (s *SavepointTx) ReleaseSavepoint() error {
	if !s.active {
		return nil
	}
	
	if s.released {
		return fmt.Errorf("savepoint already released")
	}
	
	_, err := s.tx.tx.Exec(fmt.Sprintf("RELEASE SAVEPOINT %s", s.name))
	if err != nil {
		return fmt.Errorf("failed to release savepoint: %w", err)
	}
	
	s.released = true
	s.active = false
	return nil
}

// RollbackToSavepoint rolls back to this savepoint
func (s *SavepointTx) RollbackToSavepoint() error {
	if !s.active {
		return nil
	}
	
	_, err := s.tx.tx.Exec(fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", s.name))
	if err != nil {
		return fmt.Errorf("failed to rollback to savepoint: %w", err)
	}
	
	// Re-create the savepoint after rollback
	_, err = s.tx.tx.Exec(fmt.Sprintf("SAVEPOINT %s", s.name))
	if err != nil {
		return fmt.Errorf("failed to recreate savepoint: %w", err)
	}
	
	return nil
}

// Query executes a query within the savepoint context
func (s *SavepointTx) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.tx.Query(ctx, query, args...)
}

// QueryRow executes a query returning a single row
func (s *SavepointTx) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.tx.QueryRow(ctx, query, args...)
}

// Exec executes a query without returning rows
func (s *SavepointTx) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.tx.Exec(ctx, query, args...)
}

// Prepare creates a prepared statement
func (s *SavepointTx) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return s.tx.Prepare(ctx, query)
}

// IsCommitted returns true if savepoint is released
func (s *SavepointTx) IsCommitted() bool {
	return s.released
}

// IsRolledBack returns false for savepoints (they don't rollback like transactions)
func (s *SavepointTx) IsRolledBack() bool {
	return false
}

// IsActive returns true if savepoint is still active
func (s *SavepointTx) IsActive() bool {
	return s.active
}

// NestedTransaction manages nested transaction state
type NestedTransaction struct {
	parent    *Tx
	savepoint *SavepointTx
	depth     int
	committed bool
	rolledBack bool
}

// BeginNested begins a nested transaction using savepoints
func (t *Tx) BeginNested() (*NestedTransaction, error) {
	name := fmt.Sprintf("sp_%d", len(t.savepoints))
	sp, err := t.CreateSavepoint(name)
	if err != nil {
		return nil, err
	}
	
	return &NestedTransaction{
		parent:    t,
		savepoint: sp,
		depth:     len(t.savepoints),
	}, nil
}

// Commit commits the nested transaction (release savepoint)
func (n *NestedTransaction) Commit() error {
	if n.committed {
		return fmt.Errorf("nested transaction already committed")
	}
	
	n.committed = true
	return n.savepoint.ReleaseSavepoint()
}

// Rollback rolls back the nested transaction
func (n *NestedTransaction) Rollback() error {
	if n.rolledBack {
		return nil
	}
	
	n.rolledBack = true
	return n.savepoint.RollbackToSavepoint()
}

// TransactionStatus represents the current transaction state
type TransactionStatus int

const (
	TxStatusIdle TransactionStatus = iota
	TxStatusActive
	TxStatusCommitted
	TxStatusRolledBack
)

// String returns the status as a string
func (s TransactionStatus) String() string {
	switch s {
	case TxStatusIdle:
		return "idle"
	case TxStatusActive:
		return "active"
	case TxStatusCommitted:
		return "committed"
	case TxStatusRolledBack:
		return "rolled_back"
	default:
		return "unknown"
	}
}

## 4. Configuration

### Database Configuration

**Location:** `internal/db/config.go`

```go
package db

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	// Driver configuration
	Driver   string      `json:"driver" yaml:"driver" env:"DB_DRIVER"`
	Database string      `json:"database" yaml:"database" env:"DB_NAME"`
	User     string      `json:"user" yaml:"user" env:"DB_USER"`
	Password string      `json:"password" yaml:"password" env:"DB_PASSWORD"`
	Host     string      `json:"host" yaml:"host" env:"DB_HOST"`
	Port     int         `json:"port" yaml:"port" env:"DB_PORT"`
	
	// Pool configuration
	Pool *PoolConfig `json:"pool" yaml:"pool"`
	
	// SSL/TLS configuration
	SSL *SSLConfig `json:"ssl" yaml:"ssl"`
	
	// Driver-specific parameters
	Params map[string]string `json:"params" yaml:"params"`
	
	// Migration settings
	Migrations *MigrationConfig `json:"migrations" yaml:"migrations"`
}

// SSLConfig represents SSL/TLS configuration
type SSLConfig struct {
	Mode       string `json:"mode" yaml:"mode" env:"DB_SSL_MODE"`           // disable, require, verify-ca, verify-full
	CertPath   string `json:"cert_path" yaml:"cert_path" env:"DB_SSL_CERT"`   // Client certificate
	KeyPath    string `json:"key_path" yaml:"key_path" env:"DB_SSL_KEY"`      // Client key
	RootCertPath string `json:"root_cert_path" yaml:"root_cert_path" env:"DB_SSL_ROOT_CERT"` // CA certificate
	ServerName string `json:"server_name" yaml:"server_name" env:"DB_SSL_SERVER_NAME"` // Server hostname
	InsecureSkipVerify bool `json:"insecure_skip_verify" yaml:"insecure_skip_verify"` // Skip certificate verification (not recommended)
}

// MigrationConfig represents migration settings
type MigrationConfig struct {
	TableName       string `json:"table_name" yaml:"table_name"`         // Migration tracking table
	Path            string `json:"path" yaml:"path"`                     // Migration files path
	Version         int    `json:"version" yaml:"version"`               // Target version (0 = latest)
	ForceVersion    bool   `json:"force_version" yaml:"force_version"`   // Force version even if dirty
}

// DefaultDatabaseConfig returns default configuration
func DefaultDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Driver:   "sqlite3",
		Database: "muster.db",
		Host:     "localhost",
		Pool:     DefaultPoolConfig(),
		SSL: &SSLConfig{
			Mode: "disable",
		},
		Migrations: &MigrationConfig{
			TableName: "schema_migrations",
			Path:      "migrations",
		},
	}
}

// LoadConfig loads configuration from multiple sources (file, env, defaults)
func LoadConfig(path string) (*DatabaseConfig, error) {
	config := DefaultDatabaseConfig()
	
	// 1. Load from file if provided
	if path != "" {
		if err := config.LoadFromFile(path); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}
	
	// 2. Load from environment (overrides file)
	if err := config.LoadFromEnv(); err != nil {
		return nil, fmt.Errorf("failed to load config from environment: %w", err)
	}
	
	// 3. Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return config, nil
}

// LoadFromFile loads configuration from JSON or YAML file
func (c *DatabaseConfig) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	
	// Try YAML first
	if err := yaml.Unmarshal(data, c); err == nil {
		return nil
	}
	
	// Try JSON
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config file (expected YAML or JSON): %w", err)
	}
	
	return nil
}

// LoadFromEnv loads configuration from environment variables
func (c *DatabaseConfig) LoadFromEnv() error {
	// Load .env file if exists
	_ = godotenv.Load()
	
	// Driver
	if v := os.Getenv("DB_DRIVER"); v != "" {
		c.Driver = v
	}
	
	// Database
	if v := os.Getenv("DB_NAME"); v != "" {
		c.Database = v
	}
	if v := os.Getenv("DB_DATABASE"); v != "" {
		c.Database = v
	}
	
	// User
	if v := os.Getenv("DB_USER"); v != "" {
		c.User = v
	}
	if v := os.Getenv("DB_USERNAME"); v != "" {
		c.User = v
	}
	
	// Password
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		c.Password = v
	}
	if v := os.Getenv("DB_PASS"); v != "" {
		c.Password = v
	}
	
	// Host
	if v := os.Getenv("DB_HOST"); v != "" {
		c.Host = v
	}
	
	// Port
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := parseInt(v); err == nil {
			c.Port = port
		}
	}
	
	// SSL Mode
	if v := os.Getenv("DB_SSL_MODE"); v != "" {
		if c.SSL == nil {
			c.SSL = &SSLConfig{}
		}
		c.SSL.Mode = v
	}
	
	// SSL Cert
	if v := os.Getenv("DB_SSL_CERT"); v != "" {
		if c.SSL == nil {
			c.SSL = &SSLConfig{}
		}
		c.SSL.CertPath = v
	}
	
	// SSL Key
	if v := os.Getenv("DB_SSL_KEY"); v != "" {
		if c.SSL == nil {
			c.SSL = &SSLConfig{}
		}
		c.SSL.KeyPath = v
	}
	
	// SSL Root Cert
	if v := os.Getenv("DB_SSL_ROOT_CERT"); v != "" {
		if c.SSL == nil {
			c.SSL = &SSLConfig{}
		}
		c.SSL.RootCertPath = v
	}
	
	return nil
}

// Validate validates the configuration
func (c *DatabaseConfig) Validate() error {
	if c.Driver == "" {
		return fmt.Errorf("driver is required")
	}
	
	if c.Database == "" {
		return fmt.Errorf("database is required")
	}
	
	// Validate driver-specific requirements
	switch c.Driver {
	case "sqlite3":
		// SQLite can work with just a database path
		
	case "postgres":
		if c.Host == "" {
			return fmt.Errorf("host is required for PostgreSQL")
		}
		if c.User == "" {
			return fmt.Errorf("user is required for PostgreSQL")
		}
		if c.Port == 0 {
			c.Port = 5432
		}
		
	case "mysql":
		if c.Host == "" {
			return fmt.Errorf("host is required for MySQL")
		}
		if c.User == "" {
			return fmt.Errorf("user is required for MySQL")
		}
		if c.Port == 0 {
			c.Port = 3306
		}
		
	default:
		return fmt.Errorf("unsupported driver: %s", c.Driver)
	}
	
	// Validate SSL configuration
	if c.SSL != nil && c.SSL.Mode != "" && c.SSL.Mode != "disable" {
		validModes := map[string]bool{
			"disable":     true,
			"require":     true,
			"verify-ca":   true,
			"verify-full": true,
		}
		if !validModes[c.SSL.Mode] {
			return fmt.Errorf("invalid SSL mode: %s", c.SSL.Mode)
		}
	}
	
	return nil
}

// BuildDSN constructs the DSN string based on driver
func (c *DatabaseConfig) BuildDSN() (string, error) {
	driver, err := GetDriver(c.Driver)
	if err != nil {
		return "", err
	}
	
	builder := driver.DSNBuilder()
	
	// Set common parameters
	builder.WithHost(c.Host).
		WithPort(c.Port).
		WithDatabase(c.Database).
		WithUser(c.User).
		WithPassword(c.Password)
	
	// Set SSL parameters
	if c.SSL != nil {
		builder.WithSSLMode(c.SSL.Mode).
			WithSSLCert(c.SSL.CertPath).
			WithSSLKey(c.SSL.KeyPath).
			WithSSLRootCert(c.SSL.RootCertPath)
	}
	
	// Set additional parameters
	if len(c.Params) > 0 {
		builder.WithParams(c.Params)
	}
	
	return builder.Build(), nil
}

// BuildTLSConfig creates TLS configuration from SSL settings
func (c *SSLConfig) BuildTLSConfig() (*tls.Config, error) {
	if c.Mode == "disable" || c.Mode == "" {
		return nil, nil
	}
	
	tlsConfig := &tls.Config{
		ServerName:         c.ServerName,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
	
	// Load root CA certificate
	if c.RootCertPath != "" {
		rootPEM, err := ioutil.ReadFile(c.RootCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read root certificate: %w", err)
		}
		
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(rootPEM) {
			return nil, fmt.Errorf("failed to parse root certificate")
		}
		
		tlsConfig.RootCAs = pool
	}
	
	// Load client certificate
	if c.CertPath != "" && c.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(c.CertPath, c.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	return tlsConfig, nil
}

// DSNFormats provides examples for all drivers
var DSNFormats = map[string]map[string]string{
	"sqlite3": {
		"file":        "file:/path/to/database.db?mode=rwc&cache=shared",
		"memory":      ":memory:",
		"memory_shared": "file::memory:?cache=shared",
		"relative":    "./database.db",
		"absolute":    "/absolute/path/to/database.db",
	},
	"postgres": {
		"url_simple":     "postgres://user:password@localhost:5432/dbname",
		"url_with_ssl":   "postgres://user:password@localhost:5432/dbname?sslmode=verify-full&sslrootcert=/path/to/ca.crt",
		"keyvalue":       "host=localhost port=5432 dbname=mydb user=myuser password=mypass sslmode=require",
		"url_all_params": "postgres://user:password@localhost:5432/dbname?sslmode=verify-ca&sslcert=/path/to/client.crt&sslkey=/path/to/client.key&sslrootcert=/path/to/ca.crt&connect_timeout=10",
	},
	"mysql": {
		"simple":       "user:password@tcp(localhost:3306)/dbname",
		"with_params":  "user:password@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=UTC&tls=true",
		"with_tls":     "user:password@tcp(localhost:3306)/dbname?tls=custom",
		"unix_socket":  "user:password@unix(/var/run/mysqld/mysqld.sock)/dbname",
	},
}

// GetDSNExamples returns DSN examples for a driver
func GetDSNExamples(driver string) (map[string]string, error) {
	examples, ok := DSNFormats[driver]
	if !ok {
		return nil, fmt.Errorf("no examples for driver: %s", driver)
	}
	return examples, nil
}

// Helper function
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}
```

### SSL/TLS Configuration Examples

```go
// Example: PostgreSQL with SSL
pgConfig := &db.DatabaseConfig{
    Driver:   "postgres",
    Host:     "db.example.com",
    Port:     5432,
    Database: "myapp",
    User:     "appuser",
    Password: "secretpassword",
    SSL: &db.SSLConfig{
        Mode:       "verify-full",  // verify-ca, require, disable
        CertPath:   "/certs/client.crt",
        KeyPath:    "/certs/client.key",
        RootCertPath: "/certs/ca.crt",
        ServerName: "db.example.com",
    },
}

// Example: MySQL with TLS
mysqlConfig := &db.DatabaseConfig{
    Driver:   "mysql",
    Host:     "db.example.com",
    Port:     3306,
    Database: "myapp",
    User:     "appuser",
    Password: "secretpassword",
    SSL: &db.SSLConfig{
        Mode:       "verify-ca",  // true, false, skip-verify, preferred
        CertPath:   "/certs/client-cert.pem",
        KeyPath:    "/certs/client-key.pem",
        RootCertPath: "/certs/ca.pem",
    },
    Params: map[string]string{
        "tls": "custom",  // Enable custom TLS
    },
}

// Example: SQLite (no SSL needed)
sqliteConfig := &db.DatabaseConfig{
    Driver:   "sqlite3",
    Database: "/var/lib/app/data.db",
}
```

## 5. Health Checks

### Health Checker Implementation

**Location:** `internal/db/health.go`

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health state of a connection
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

// String returns the health status as a string
func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheckResult contains health check results
type HealthCheckResult struct {
	Status      HealthStatus  `json:"status"`
	Message     string        `json:"message"`
	LastChecked time.Time     `json:"last_checked"`
	Latency     time.Duration `json:"latency"`
	Error       string        `json:"error,omitempty"`
	Stats       PoolStats     `json:"stats"`
}

// IsHealthy returns true if status is healthy
func (r *HealthCheckResult) IsHealthy() bool {
	return r.Status == HealthHealthy
}

// HealthChecker performs health checks on database connections
type HealthChecker struct {
	pool           *ConnectionPool
	config         *HealthCheckConfig
	lastResult     *HealthCheckResult
	mutex          sync.RWMutex
	stopCh         chan struct{}
	running        bool
	onStatusChange func(old, new HealthStatus)
}

// HealthCheckConfig configures health checking behavior
type HealthCheckConfig struct {
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
	HealthCheckQuery   string
	EnableStats        bool
	OnStatusChange     func(old, new HealthStatus)
}

// DefaultHealthCheckConfig returns default configuration
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval:           30 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		HealthCheckQuery:   "SELECT 1",
		EnableStats:        true,
	}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(pool *ConnectionPool, config *HealthCheckConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}
	return &HealthChecker{
		pool:       pool,
		config:     config,
		lastResult: &HealthCheckResult{Status: HealthUnknown},
		stopCh:     make(chan struct{}),
	}
}

// Start begins periodic health checks
func (hc *HealthChecker) Start() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	if hc.running {
		return
	}
	hc.running = true
	go hc.run()
}

// Stop stops periodic health checks
func (hc *HealthChecker) Stop() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	if !hc.running {
		return
	}
	close(hc.stopCh)
	hc.running = false
}

// run is the main health check loop
func (hc *HealthChecker) run() {
	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()
	hc.check()
	for {
		select {
		case <-ticker.C:
			hc.check()
		case <-hc.stopCh:
			return
		}
	}
}

// check performs a single health check
func (hc *HealthChecker) check() {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()
	result := &HealthCheckResult{LastChecked: time.Now(), Latency: 0}

	err := hc.pool.DB().PingContext(ctx)
	if err != nil {
		result.Status = HealthUnhealthy
		result.Error = err.Error()
		result.Message = fmt.Sprintf("Ping failed: %v", err)
	} else {
		var one int
		row := hc.pool.DB().QueryRowContext(ctx, hc.config.HealthCheckQuery)
		err = row.Scan(&one)
		if err != nil {
			result.Status = HealthUnhealthy
			result.Error = err.Error()
			result.Message = fmt.Sprintf("Health check query failed: %v", err)
		} else {
			result.Status = HealthHealthy
			result.Latency = time.Since(start)
			result.Message = "Health check passed"
		}
	}

	if hc.config.EnableStats {
		result.Stats = hc.pool.Stats()
	}
	hc.updateResult(result)
}

// updateResult updates the health status with threshold logic
func (hc *HealthChecker) updateResult(newResult *HealthCheckResult) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	oldStatus := hc.lastResult.Status
	switch newResult.Status {
	case HealthHealthy:
		if oldStatus == HealthUnhealthy {
			hc.lastResult = newResult
		}
	case HealthUnhealthy:
		hc.lastResult = newResult
	}
	if oldStatus != hc.lastResult.Status && hc.onStatusChange != nil {
		go hc.onStatusChange(oldStatus, hc.lastResult.Status)
	}
}

// GetLastResult returns the last health check result
func (hc *HealthChecker) GetLastResult() *HealthCheckResult {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return hc.lastResult
}

// IsHealthy returns true if the connection is healthy
func (hc *HealthChecker) IsHealthy() bool {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return hc.lastResult.Status == HealthHealthy
}

// WaitForHealthy blocks until the connection becomes healthy or timeout
func (hc *HealthChecker) WaitForHealthy(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for healthy connection")
		case <-ticker.C:
			if hc.IsHealthy() {
				return nil
			}
		}
	}
}

// RetryWithBackoff attempts an operation with exponential backoff
func RetryWithBackoff(maxRetries int, initialDelay time.Duration, operation func() error) error {
	delay := initialDelay
	for i := 0; i <= maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		if i == maxRetries {
			return fmt.Errorf("operation failed after %d retries: %w", maxRetries, err)
		}
		time.Sleep(delay)
		delay *= 2
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
	}
	return nil
}

// ConnectionValidator validates connections before use
type ConnectionValidator struct {
	pool    *ConnectionPool
	query   string
	timeout time.Duration
}

// NewConnectionValidator creates a new validator
func NewConnectionValidator(pool *ConnectionPool) *ConnectionValidator {
	return &ConnectionValidator{
		pool:    pool,
		query:   "SELECT 1",
		timeout: 5 * time.Second,
	}
}

// Validate checks if a connection is valid
func (v *ConnectionValidator) Validate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()
	if err := v.pool.DB().PingContext(ctx); err != nil {
		return fmt.Errorf("connection validation failed (ping): %w", err)
	}
	var one int
	row := v.pool.DB().QueryRowContext(ctx, v.query)
	if err := row.Scan(&one); err != nil {
		return fmt.Errorf("connection validation failed (query): %w", err)
	}
	return nil
}

// ValidateWithRetry validates with retry logic
func (v *ConnectionValidator) ValidateWithRetry(ctx context.Context, maxRetries int) error {
	return RetryWithBackoff(maxRetries, 100*time.Millisecond, func() error {
		return v.Validate(ctx)
	})
}

// PingResult contains detailed ping information
type PingResult struct {
	Success   bool          `json:"success"`
	Latency   time.Duration `json:"latency"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// Ping performs a detailed ping check
func (hc *HealthChecker) Ping() *PingResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()
	err := hc.pool.DB().PingContext(ctx)
	result := &PingResult{
		Success:   err == nil,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

// MultiPing performs multiple pings and returns statistics
func (hc *HealthChecker) MultiPing(count int, interval time.Duration) *PingStats {
	stats := &PingStats{
		Total:     count,
		Timestamp: time.Now(),
	}
	for i := 0; i < count; i++ {
		result := hc.Ping()
		if result.Success {
			stats.Successful++
			if stats.MinLatency == 0 || result.Latency < stats.MinLatency {
				stats.MinLatency = result.Latency
			}
			if result.Latency > stats.MaxLatency {
				stats.MaxLatency = result.Latency
			}
			stats.TotalLatency += result.Latency
		} else {
			stats.Failed++
		}
		if i < count-1 {
			time.Sleep(interval)
		}
	}
	if stats.Successful > 0 {
		stats.AvgLatency = stats.TotalLatency / time.Duration(stats.Successful)
	}
	return stats
}

// PingStats contains aggregated ping statistics
type PingStats struct {
	Total        int           `json:"total"`
	Successful   int           `json:"successful"`
	Failed       int           `json:"failed"`
	MinLatency   time.Duration `json:"min_latency"`
	MaxLatency   time.Duration `json:"max_latency"`
	AvgLatency   time.Duration `json:"avg_latency"`
	TotalLatency time.Duration `json:"total_latency"`
	Timestamp    time.Time     `json:"timestamp"`
}

// SuccessRate returns the success rate as a percentage
func (s *PingStats) SuccessRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Successful) / float64(s.Total) * 100
}
```

## 6. Migrations

### Migration Runner

**Location:** `internal/db/migrations/migrations.go`

```go
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration represents a single database migration
type Migration struct {
	Version   int
	Name      string
	Up        string
	Down      string
	Timestamp time.Time
}

// Migrator handles database migrations
type Migrator struct {
	db        *sql.DB
	tableName string
	path      string
}

// NewMigrator creates a new migrator
func NewMigrator(db *sql.DB, tableName, path string) *Migrator {
	if tableName == "" {
		tableName = "schema_migrations"
	}
	return &Migrator{
		db:        db,
		tableName: tableName,
		path:      path,
	}
}

// Init creates the migrations table
func (m *Migrator) Init() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			dirty BOOLEAN DEFAULT FALSE
		)
	`, m.tableName)
	_, err := m.db.Exec(query)
	return err
}

// GetCurrentVersion returns the current migration version
func (m *Migrator) GetCurrentVersion() (int, bool, error) {
	query := fmt.Sprintf("SELECT version, dirty FROM %s ORDER BY version DESC LIMIT 1", m.tableName)
	row := m.db.QueryRow(query)
	
	var version int
	var dirty bool
	err := row.Scan(&version, &dirty)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

// LoadMigrations loads all migrations from the migrations directory
func (m *Migrator) LoadMigrations() ([]*Migration, error) {
	entries, err := os.ReadDir(m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []*Migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql") {
			continue
		}
		
		version, migrationName, err := parseMigrationFilename(name)
		if err != nil {
			continue
		}
		
		// Check if we already have this migration
		var existing *Migration
		for _, m := range migrations {
			if m.Version == version {
				existing = m
				break
			}
		}
		
		if existing == nil {
			existing = &Migration{
				Version: version,
				Name:    migrationName,
			}
			migrations = append(migrations, existing)
		}
		
		// Load migration content
		content, err := os.ReadFile(filepath.Join(m.path, name))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", name, err)
		}
		
		if strings.HasSuffix(name, ".up.sql") {
			existing.Up = string(content)
		} else {
			existing.Down = string(content)
		}
	}
	
	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	
	return migrations, nil
}

// Migrate runs all pending migrations
func (m *Migrator) Migrate(ctx context.Context) error {
	if err := m.Init(); err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	
	migrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}
	
	currentVersion, dirty, err := m.GetCurrentVersion()
	if err != nil {
		return err
	}
	
	if dirty {
		return fmt.Errorf("database is dirty at version %d", currentVersion)
	}
	
	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}
		
		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}
	
	return nil
}

// MigrateTo migrates to a specific version
func (m *Migrator) MigrateTo(ctx context.Context, targetVersion int) error {
	if err := m.Init(); err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	
	migrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}
	
	currentVersion, dirty, err := m.GetCurrentVersion()
	if err != nil {
		return err
	}
	
	if dirty {
		return fmt.Errorf("database is dirty at version %d", currentVersion)
	}
	
	if targetVersion > currentVersion {
		// Migrate up
		for _, migration := range migrations {
			if migration.Version > currentVersion && migration.Version <= targetVersion {
				if err := m.applyMigration(ctx, migration); err != nil {
					return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
				}
			}
		}
	} else if targetVersion < currentVersion {
		// Migrate down
		for i := len(migrations) - 1; i >= 0; i-- {
			migration := migrations[i]
			if migration.Version <= currentVersion && migration.Version > targetVersion {
				if err := m.rollbackMigration(ctx, migration); err != nil {
					return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
				}
			}
		}
	}
	
	return nil
}

// applyMigration applies a single migration
func (m *Migrator) applyMigration(ctx context.Context, migration *Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	
	defer tx.Rollback()
	
	// Mark as dirty
	_, err = tx.ExecContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (version, dirty) VALUES ($1, TRUE)", m.tableName),
		migration.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to mark migration dirty: %w", err)
	}
	
	// Execute migration
	_, err = tx.ExecContext(ctx, migration.Up)
	if err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}
	
	// Mark as clean
	_, err = tx.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s SET dirty = FALSE WHERE version = $1", m.tableName),
		migration.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to mark migration clean: %w", err)
	}
	
	return tx.Commit()
}

// rollbackMigration rolls back a single migration
func (m *Migrator) rollbackMigration(ctx context.Context, migration *Migration) error {
	if migration.Down == "" {
		return fmt.Errorf("migration %d has no rollback script", migration.Version)
	}
	
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	
	defer tx.Rollback()
	
	// Execute rollback
	_, err = tx.ExecContext(ctx, migration.Down)
	if err != nil {
		return fmt.Errorf("failed to execute rollback: %w", err)
	}
	
	// Remove from migrations table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE version = $1", m.tableName),
		migration.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}
	
	return tx.Commit()
}

// GetStatus returns the current migration status
func (m *Migrator) GetStatus() (*MigrationStatus, error) {
	if err := m.Init(); err != nil {
		return nil, err
	}
	
	migrations, err := m.LoadMigrations()
	if err != nil {
		return nil, err
	}
	
	currentVersion, dirty, err := m.GetCurrentVersion()
	if err != nil {
		return nil, err
	}
	
	status := &MigrationStatus{
		CurrentVersion: currentVersion,
		Dirty:          dirty,
		TotalMigrations: len(migrations),
	}
	
	for _, migration := range migrations {
		if migration.Version <= currentVersion && !dirty {
			status.Applied++
			status.AppliedMigrations = append(status.AppliedMigrations, migration.Version)
		} else {
			status.Pending++
			status.PendingMigrations = append(status.PendingMigrations, migration.Version)
		}
	}
	
	return status, nil
}

// MigrationStatus represents the migration status
type MigrationStatus struct {
	CurrentVersion    int   `json:"current_version"`
	Dirty             bool  `json:"dirty"`
	Applied           int   `json:"applied"`
	Pending           int   `json:"pending"`
	TotalMigrations   int   `json:"total_migrations"`
	AppliedMigrations []int `json:"applied_migrations"`
	PendingMigrations []int `json:"pending_migrations"`
}

// IsUpToDate returns true if all migrations are applied
func (s *MigrationStatus) IsUpToDate() bool {
	return s.Pending == 0 && !s.Dirty
}

// parseMigrationFilename parses a migration filename
func parseMigrationFilename(filename string) (int, string, error) {
	// Format: 001_create_users.up.sql
	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid migration filename format")
	}
	
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid version number")
	}
	
	// Extract name (everything between version and extension)
	nameParts := parts[1:]
	name := strings.Join(nameParts, "_")
	name = strings.TrimSuffix(name, ".up.sql")
	name = strings.TrimSuffix(name, ".down.sql")
	
	return version, name, nil
}

// CreateMigration creates a new migration file
func CreateMigration(path string, name string) error {
	// Get next version
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	
	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version, _, err := parseMigrationFilename(entry.Name())
		if err == nil && version > maxVersion {
			maxVersion = version
		}
	}
	
	nextVersion := maxVersion + 1
	versionStr := fmt.Sprintf("%03d", nextVersion)
	
	// Create up file
	upFilename := fmt.Sprintf("%s_%s.up.sql", versionStr, name)
	upPath := filepath.Join(path, upFilename)
	upFile, err := os.Create(upPath)
	if err != nil {
		return err
	}
	upFile.Close()
	
	// Create down file
	downFilename := fmt.Sprintf("%s_%s.down.sql", versionStr, name)
	downPath := filepath.Join(path, downFilename)
	downFile, err := os.Create(downPath)
	if err != nil {
		os.Remove(upPath)
		return err
	}
	downFile.Close()
	
	return nil
}
```

### Migration SQL Examples

```sql
-- 001_create_users.up.sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);

-- 001_create_users.down.sql
DROP TABLE IF EXISTS users;

-- 002_create_projects.up.sql
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_projects_user_id ON projects(user_id);

-- 002_create_projects.down.sql
DROP TABLE IF EXISTS projects;
```

### Usage Examples

```go
// Create and run migrations
migrator := migrations.NewMigrator(pool.DB(), "schema_migrations", "./migrations")

// Run all pending migrations
if err := migrator.Migrate(ctx); err != nil {
    log.Fatal(err)
}

// Check migration status
status, err := migrator.GetStatus()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Current version: %d\n", status.CurrentVersion)
fmt.Printf("Applied: %d, Pending: %d\n", status.Applied, status.Pending)

// Migrate to specific version
if err := migrator.MigrateTo(ctx, 10); err != nil {
    log.Fatal(err)
}

// Create new migration file
if err := migrations.CreateMigration("./migrations", "add_notifications_table"); err != nil {
    log.Fatal(err)
}
```

