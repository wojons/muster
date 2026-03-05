# Goose Migrations Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Database

## Overview

Complete specification of **Goose migration** integration for database versioning.

## 1. Migration Files

```sql
-- migrations/001_initial_schema.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key VARCHAR(255) UNIQUE NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS inventories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url VARCHAR(512) UNIQUE NOT NULL,
    data JSON NOT NULL,
    etag VARCHAR(255),
    fetched_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS configs;
DROP TABLE IF EXISTS inventories;
```

## 2. Goose Integration

```go
func runGooseMigrations(db *sql.DB, migrationsDir string) error {
    if err := goose.SetDialect("sqlite3"); err != nil {
        return err
    }
    
    return goose.Up(db, migrationsDir)
}

func createGooseMigration(name, migrationsDir string) error {
    timestamp := time.Now().Format("20060102150405")
    filename := fmt.Sprintf("%s_%s.sql", timestamp, name)
    filepath := filepath.Join(migrationsDir, filename)
    
    template := `-- +goose Up
-- +goose StatementBegin
-- Your migration here
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Your rollback here
-- +goose StatementEnd
`
    return os.WriteFile(filepath, []byte(template), 0644)
}
```

## 3. Migration Status

```go
func getMigrationStatus(db *sql.DB, migrationsDir string) (*MigrationStatus, error) {
    records, err := goose.GetDBVersion(db)
    if err != nil {
        return nil, err
    }
    
    migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
    if err != nil {
        return nil, err
    }
    
    return &MigrationStatus{
        Current:   records,
        Latest:    int64(len(migrations)),
        Pending:   int64(len(migrations)) - records,
    }, nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
