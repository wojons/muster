package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mutecomm/go-sqlcipher"
	"go.uber.org/zap"
)

// SQLiteConfig holds configuration for SQLite database
type SQLiteConfig struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	EncryptionKey   string // SQLCipher encryption key (hex-encoded). Empty disables encryption
	KeyManager      *EncryptionKeyManager
}

// DefaultSQLiteConfig returns default configuration
func DefaultSQLiteConfig(path string) *SQLiteConfig {
	return &SQLiteConfig{
		Path:            path,
		MaxOpenConns:    25,
		MaxIdleConns:    25,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 5 * time.Minute,
		EncryptionKey:   "", // Encryption disabled by default
	}
}

// EnableEncryption enables database encryption with the provided key
func (cfg *SQLiteConfig) EnableEncryption(key string) {
	cfg.EncryptionKey = key
}

// EnableEncryptionWithKeyManager enables database encryption using a key manager
func (cfg *SQLiteConfig) EnableEncryptionWithKeyManager(km *EncryptionKeyManager) error {
	if km == nil {
		return fmt.Errorf("key manager is required")
	}

	pragmKey, err := km.DerivePragmaKey()
	if err != nil {
		return fmt.Errorf("derive encryption key: %w", err)
	}

	cfg.EncryptionKey = pragmKey
	cfg.KeyManager = km
	return nil
}

// SQLiteManager manages SQLite database connections
type SQLiteManager struct {
	db  *sql.DB
	log *zap.Logger
	cfg *SQLiteConfig
}

// NewSQLiteManager creates a new SQLite manager
func NewSQLiteManager(cfg *SQLiteConfig, log *zap.Logger) (*SQLiteManager, error) {
	if cfg == nil {
		cfg = DefaultSQLiteConfig("./muster.db")
	}

	dsn := cfg.Path
	if dsn == "" {
		return nil, fmt.Errorf("database path is required")
	}

	// Build connection string with encryption support
	queryParams := "_journal_mode=WAL&_timeout=5000"

	// Initialize key manager if provided
	if cfg.KeyManager != nil {
		if err := cfg.KeyManager.Initialize(); err != nil {
			return nil, fmt.Errorf("initialize key manager: %w", err)
		}

		// Derive encryption key if not already set
		if cfg.EncryptionKey == "" {
			pragmKey, err := cfg.KeyManager.DerivePragmaKey()
			if err != nil {
				return nil, fmt.Errorf("derive encryption key: %w", err)
			}
			cfg.EncryptionKey = pragmKey
		}
	}

	// Open database connection
	fullDSN := fmt.Sprintf("%s?%s", dsn, queryParams)
	db, err := sql.Open("sqlite3", fullDSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Apply encryption key BEFORE pinging
	if cfg.EncryptionKey != "" {
		if err := applyEncryptionKey(db, cfg.EncryptionKey); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply encryption key: %w", err)
		}
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	sm := &SQLiteManager{
		db:  db,
		log: log,
		cfg: cfg,
	}

	return sm, nil
}

// Close closes the database connection
func (sm *SQLiteManager) Close() error {
	if sm.db == nil {
		return nil
	}
	return sm.db.Close()
}

// Ping verifies the database connection is alive
func (sm *SQLiteManager) Ping() error {
	if sm.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return sm.db.Ping()
}

// BeginTx starts a new database transaction
func (sm *SQLiteManager) BeginTx(ctx context.Context, opts *sql.TxOptions) (Transaction, error) {
	if sm.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	tx, err := sm.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &sqliteTransaction{tx: tx}, nil
}

// Stats returns database statistics
func (sm *SQLiteManager) Stats() sql.DBStats {
	if sm.db == nil {
		return sql.DBStats{}
	}
	return sm.db.Stats()
}

// GetDB returns the underlying *sql.DB
func (sm *SQLiteManager) GetDB() *sql.DB {
	return sm.db
}

// applyEncryptionKey applies the SQLCipher encryption key to the database
func applyEncryptionKey(db *sql.DB, key string) error {
	// Set the encryption key using PRAGMA
	// SQLCipher expects the key in hex format
	pragmaKey := fmt.Sprintf("PRAGMA key = \"%s\"", key)
	_, err := db.Exec(pragmaKey)
	if err != nil {
		return fmt.Errorf("set encryption key: %w", err)
	}

	// Verify encryption is working by checking cipher_version
	var cipherVersion string
	err = db.QueryRow("PRAGMA cipher_version").Scan(&cipherVersion)
	if err != nil {
		// SQLCipher not available or encryption failed
		return fmt.Errorf("SQLCipher not available: %w", err)
	}

	// Additional security settings
	// Set cipher_page_size for better security (default is 4096)
	_, err = db.Exec("PRAGMA cipher_page_size = 4096")
	if err != nil {
		return fmt.Errorf("set cipher page size: %w", err)
	}

	// Enable KDF (Key Derivation Function) for stronger encryption
	_, err = db.Exec("PRAGMA kdf_iter = 256000")
	if err != nil {
		return fmt.Errorf("set kdf iterations: %w", err)
	}

	return nil
}

// IsEncrypted checks if the database is encrypted
func (sm *SQLiteManager) IsEncrypted() bool {
	if sm.db == nil {
		return false
	}

	if sm.cfg == nil || sm.cfg.EncryptionKey == "" {
		return false
	}

	// Verify encryption is active by checking cipher_version
	var cipherVersion string
	err := sm.db.QueryRow("PRAGMA cipher_version").Scan(&cipherVersion)
	return err == nil && cipherVersion != ""
}

// GetEncryptionConfig returns the current encryption configuration
func (sm *SQLiteManager) GetEncryptionConfig() *SQLiteConfig {
	if sm.cfg == nil {
		return nil
	}
	return sm.cfg
}

// sqliteTransaction implements the Transaction interface
type sqliteTransaction struct {
	tx        *sql.Tx
	committed bool
}

// Commit commits the transaction
func (st *sqliteTransaction) Commit() error {
	if st.committed {
		return fmt.Errorf("transaction already committed")
	}
	st.committed = true
	return st.tx.Commit()
}

// Rollback rolls back the transaction
func (st *sqliteTransaction) Rollback() error {
	if st.committed {
		return nil
	}
	return st.tx.Rollback()
}

// GetTx returns the underlying sql.Tx
func (st *sqliteTransaction) GetTx() *sql.Tx {
	return st.tx
}

// AutoRollbackTx wraps a transaction to automatically rollback if not committed.
// This should be used with defer to ensure rollback on panic/error.
func AutoRollbackTx(tx Transaction) func() {
	return func() {
		if st, ok := tx.(*sqliteTransaction); ok {
			if !st.committed {
				_ = st.Rollback()
			}
		}
	}
}
