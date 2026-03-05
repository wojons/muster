# Database Migrations Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Database

## Overview

Complete specification of **database migrations** for schema evolution.

## 1. Migration Structure

```go
type Migration struct {
    Version   int64
    Name      string
    Up        string
    Down      string
    Timestamp time.Time
}

type MigrationManager struct {
    db        *sql.DB
    tableName string
}
```

## 2. Migration Table

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL,
    name VARCHAR(255) NOT NULL
);
```

## 3. Migration Runner

```go
func (mm *MigrationManager) Migrate(migrations []Migration) error {
    // Ensure migrations table exists
    if err := mm.createMigrationsTable(); err != nil {
        return err
    }
    
    // Get current version
    currentVersion, err := mm.getCurrentVersion()
    if err != nil {
        return err
    }
    
    // Apply pending migrations
    for _, migration := range migrations {
        if migration.Version <= currentVersion {
            continue
        }
        
        if err := mm.applyMigration(migration); err != nil {
            return fmt.Errorf("migration %d failed: %w", migration.Version, err)
        }
    }
    
    return nil
}

func (mm *MigrationManager) applyMigration(m Migration) error {
    tx, err := mm.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Run migration
    if _, err := tx.Exec(m.Up); err != nil {
        return fmt.Errorf("up migration failed: %w", err)
    }
    
    // Record migration
    _, err = tx.Exec(
        "INSERT INTO schema_migrations (version, applied_at, name) VALUES (?, ?, ?)",
        m.Version, time.Now(), m.Name,
    )
    if err != nil {
        return fmt.Errorf("failed to record migration: %w", err)
    }
    
    return tx.Commit()
}
```

## 4. Rollback

```go
func (mm *MigrationManager) Rollback(targetVersion int64) error {
    currentVersion, err := mm.getCurrentVersion()
    if err != nil {
        return err
    }
    
    if targetVersion >= currentVersion {
        return nil
    }
    
    // Get migrations to rollback
    migrations, err := mm.getMigrationsSince(targetVersion)
    if err != nil {
        return err
    }
    
    // Rollback in reverse order
    for i := len(migrations) - 1; i >= 0; i-- {
        if err := mm.rollbackMigration(migrations[i]); err != nil {
            return fmt.Errorf("rollback of migration %d failed: %w", migrations[i].Version, err)
        }
    }
    
    return nil
}
```

## 5. Migration Files

```go
// migrations/001_initial_schema.go
var Migration001 = Migration{
    Version: 1,
    Name:    "initial_schema",
    Up: `
        CREATE TABLE IF NOT EXISTS configs (
            id INTEGER PRIMARY KEY,
            key VARCHAR(255) UNIQUE NOT NULL,
            value TEXT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS inventories (
            id INTEGER PRIMARY KEY,
            url VARCHAR(512) UNIQUE NOT NULL,
            data JSON NOT NULL,
            fetched_at TIMESTAMP NOT NULL
        );
    `,
    Down: `
        DROP TABLE IF EXISTS configs;
        DROP TABLE IF EXISTS inventories;
    `,
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
