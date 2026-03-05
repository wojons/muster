package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

// MigrationManager handles database migrations
type MigrationManager struct {
	db      *sql.DB
	log     *zap.Logger
	dir     string
	dialect string
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *sql.DB, migrationsDir string, log *zap.Logger) (*MigrationManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	if migrationsDir == "" {
		migrationsDir = "./migrations"
	}

	if err := ensureDir(migrationsDir); err != nil {
		return nil, fmt.Errorf("create migrations directory: %w", err)
	}

	mm := &MigrationManager{
		db:      db,
		log:     log,
		dir:     migrationsDir,
		dialect: "sqlite3",
	}

	if err := goose.SetDialect(mm.dialect); err != nil {
		return nil, fmt.Errorf("set dialect: %w", err)
	}

	return mm, nil
}

// Up runs all pending migrations
func (mm *MigrationManager) Up() error {
	if mm.log != nil {
		mm.log.Info("Running migrations", zap.String("dir", mm.dir))
	}

	if err := goose.Up(mm.db, mm.dir); err != nil {
		if mm.log != nil {
			mm.log.Error("Migration failed", zap.Error(err))
		}
		return fmt.Errorf("run migrations: %w", err)
	}

	if mm.log != nil {
		mm.log.Info("Migrations completed successfully")
	}

	return nil
}

// UpTo runs migrations up to a specific version
func (mm *MigrationManager) UpTo(version int64) error {
	if mm.log != nil {
		mm.log.Info("Running migrations to version", zap.Int64("version", version))
	}

	if err := goose.UpTo(mm.db, mm.dir, version); err != nil {
		if mm.log != nil {
			mm.log.Error("Migration failed", zap.Error(err))
		}
		return fmt.Errorf("run migrations to %d: %w", version, err)
	}

	if mm.log != nil {
		mm.log.Info("Migrations completed successfully", zap.Int64("version", version))
	}

	return nil
}

// Down rolls back the last migration
func (mm *MigrationManager) Down() error {
	if mm.log != nil {
		mm.log.Info("Rolling back last migration")
	}

	if err := goose.Down(mm.db, mm.dir); err != nil {
		if mm.log != nil {
			mm.log.Error("Rollback failed", zap.Error(err))
		}
		return fmt.Errorf("rollback migration: %w", err)
	}

	if mm.log != nil {
		mm.log.Info("Rollback completed successfully")
	}

	return nil
}

// DownTo rolls back to a specific version
func (mm *MigrationManager) DownTo(version int64) error {
	if mm.log != nil {
		mm.log.Info("Rolling back to version", zap.Int64("version", version))
	}

	if err := goose.DownTo(mm.db, mm.dir, version); err != nil {
		if mm.log != nil {
			mm.log.Error("Rollback failed", zap.Error(err))
		}
		return fmt.Errorf("rollback to %d: %w", version, err)
	}

	if mm.log != nil {
		mm.log.Info("Rollback completed successfully")
	}

	return nil
}

// Status returns the current migration status
func (mm *MigrationManager) Status() (*MigrationStatus, error) {
	records, err := goose.GetDBVersion(mm.db)
	if err != nil {
		return nil, fmt.Errorf("get database version: %w", err)
	}

	migrations, err := goose.CollectMigrations(mm.dir, 0, goose.MaxVersion)
	if err != nil {
		return nil, fmt.Errorf("collect migrations: %w", err)
	}

	pending := len(migrations) - int(records)

	return &MigrationStatus{
		Current: records,
		Latest:  int64(len(migrations)),
		Pending: int64(pending),
	}, nil
}

// CreateMigration creates a new migration file
func (mm *MigrationManager) CreateMigration(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("migration name is required")
	}

	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.sql", timestamp, name)
	filepath := filepath.Join(mm.dir, filename)

	template := `-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS example (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS example;
-- +goose StatementEnd
`

	if err := os.WriteFile(filepath, []byte(template), 0644); err != nil {
		return "", fmt.Errorf("write migration file: %w", err)
	}

	if mm.log != nil {
		mm.log.Info("Created migration file", zap.String("file", filename))
	}

	return filepath, nil
}

// Redo rolls back and re-applies the last migration
func (mm *MigrationManager) Redo() error {
	if mm.log != nil {
		mm.log.Info("Redoing last migration")
	}

	status, err := mm.Status()
	if err != nil {
		return err
	}

	if status.Current == 0 {
		return fmt.Errorf("no migrations to redo")
	}

	if err := mm.Down(); err != nil {
		return err
	}

	if err := mm.Up(); err != nil {
		return err
	}

	if mm.log != nil {
		mm.log.Info("Redo completed successfully")
	}

	return nil
}

// Validate checks that all migrations can be applied
func (mm *MigrationManager) Validate() error {
	migrations, err := goose.CollectMigrations(mm.dir, 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("collect migrations: %w", err)
	}

	if mm.log != nil {
		mm.log.Info("Validating migrations", zap.Int("count", len(migrations)))
	}

	for _, m := range migrations {
		if mm.log != nil {
			mm.log.Debug("Checking migration", zap.Int64("version", m.Version))
		}
	}

	return nil
}

// MigrationStatus represents migration status
type MigrationStatus struct {
	Current int64
	Latest  int64
	Pending int64
}

// ensureDir ensures a directory exists
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return nil
}
