# Repository Package Examples

This document provides comprehensive examples for using the `pkg/repository` package, which handles database operations and persistence.

## Overview

The `repository` package provides a SQLite-based persistence layer with support for migrations, transactions, and high-performance queries.

## Initialization

### Basic Setup

```go
package main

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/openapi-cli/openapi-cli/pkg/repository"
)

func main() {
    // Create repository in working directory
    dbPath := filepath.Join(os.TempDir(), "muster.db")
    repos, err := repository.NewRepository(dbPath)
    if err != nil {
        panic(err)
    }
    defer repos.Close()

    fmt.Println("Repository initialized successfully")
}
```

### With Custom Configuration

```go
func createRepository() (*repository.SQLiteManager, error) {
    opts := &repository.Options{
        CacheSize:      10000,
        WALMode:        true,
        BusyTimeout:   5000,
        ForeignKeys:    true,
        JournalMode:   repository.JournalModeWAL,
        Synchronous:   repository.SynchronousNormal,
        Cache:         repository.CacheShared,
        TempStore:     repository.TempStoreMemory,
        Migrations:    true,
        AutoMigrate:   true,
    }

    dbPath := filepath.Join(os.TempDir(), "muster.db")
    repos, err := repository.NewRepositoryWithOptions(dbPath, opts)
    if err != nil {
        return nil, err
    }
    return repos, nil
}
```

## Entity Operations

### Defining an Entity

```go
// User represents a user in the system
type User struct {
    ID       int64  `db:"id" json:"id"`
    Username string `db:"username" json:"username"`
    Email    string `db:"email" json:"email"`
    Created  int64  `db:"created_at" json:"created_at"`
}

// TableName returns the table name for the User entity
func (u *User) TableName() string {
    return "users"
}
```

### Creating Entities

```go
func createUserExample(ctx context.Context, repos *repository.SQLiteManager) error {
    user := &User{
        Username: "john_doe",
        Email:    "john@example.com",
        Created:  time.Now().Unix(),
    }

    id, err := repos.Create(ctx, user)
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }

    fmt.Printf("Created user with ID: %d\n", id)
    return nil
}
```

### Querying Entities

```go
func queryUserExample(ctx context.Context, repos *repository.SQLiteManager) error {
    // Find by ID
    user, err := repos.FindByID(ctx, &User{}, 1)
    if err != nil {
        return fmt.Errorf("failed to find user: %w", err)
    }

    if user != nil {
        fmt.Printf("Found user: %+v\n", user)
    }

    // Find with conditions
    users, err := repos.Find(ctx, &User{}, repository.WithWhere("username = ?", "john_doe"))
    if err != nil {
        return fmt.Errorf("failed to find users: %w", err)
    }

    fmt.Printf("Found %d users\n", len(users))

    // List with pagination
    page, err := repos.List(ctx, &User{}, repository.WithLimit(10), repository.WithOffset(0))
    if err != nil {
        return fmt.Errorf("failed to list users: %w", err)
    }

    fmt.Printf("Listing %d users\n", len(page))
    return nil
}
```

### Updating Entities

```go
func updateUserExample(ctx context.Context, repos *repository.SQLiteManager) error {
    user, err := repos.FindByID(ctx, &User{}, 1)
    if err != nil {
        return fmt.Errorf("failed to find user: %w", err)
    }

    if user == nil {
        return fmt.Errorf("user not found")
    }

    u := user.(*User)
    u.Email = "new.email@example.com"

    err = repos.Update(ctx, u)
    if err != nil {
        return fmt.Errorf("failed to update user: %w", err)
    }

    fmt.Println("User updated successfully")
    return nil
}
```

### Deleting Entities

```go
func deleteUserExample(ctx context.Context, repos *repository.SQLiteManager) error {
    err := repos.Delete(ctx, &User{}, 1)
    if err != nil {
        return fmt.Errorf("failed to delete user: %w", err)
    }

    fmt.Println("User deleted successfully")
    return nil
}
```

## Transaction Operations

### Basic Transaction

```go
func transactionExample(ctx context.Context, repos *repository.SQLiteManager) error {
    err := repos.Transaction(ctx, func(tx *repository.Transaction) error {
        // Get repository with transaction
        txRepo := tx.Repository()

        // Create user
        user := &User{
            Username: "transaction_user",
            Email:    "transaction@example.com",
            Created:  time.Now().Unix(),
        }

        id, err := txRepo.Create(ctx, user)
        if err != nil {
            return err // Transaction will rollback
        }

        // Update cache (fictional example)
        err = updateCache(id)
        if err != nil {
            return err // Transaction will rollback
        }

        fmt.Printf("Transaction successful, created user %d\n", id)
        return nil
    })

    if err != nil {
        return fmt.Errorf("transaction failed: %w", err)
    }

    fmt.Println("Transaction completed successfully")
    return nil
}

func updateCache(id int64) error {
    // Fictional cache update
    return nil
}
```

### Nested Transactions (Savepoints)

```go
func nestedTransactionExample(ctx context.Context, repos *repository.SQLiteManager) error {
    err := repos.Transaction(ctx, func(tx *repository.Transaction) error {
        txRepo := tx.Repository()

        // Create first user
        user1 := &User{
            Username: "outer_transaction",
            Email:    "outer@example.com",
            Created:  time.Now().Unix(),
        }

        id1, err := txRepo.Create(ctx, user1)
        if err != nil {
            return err
        }

        // Inner transaction (savepoint)
        err = tx.Transaction(ctx, func(innerTx *repository.Transaction) error {
            innerRepo := innerTx.Repository()

            user2 := &User{
                Username: "inner_transaction",
                Email:    "inner@example.com",
                Created:  time.Now().Unix(),
            }

            _, err := innerRepo.Create(ctx, user2)
            if err != nil {
                return err
            }

            // This only rolls back to savepoint if it fails
            return nil
        })

        if err != nil {
            // Inner transaction failed, but outer continues
            fmt.Printf("Inner transaction failed, but outer continuing: %v\n", err)
        }

        fmt.Printf("Outer transaction created user %d\n", id1)
        return nil
    })

    if err != nil {
        return fmt.Errorf("outer transaction failed: %w", err)
    }

    return nil
}
```

## Advanced Queries

### Custom Queries

```go
func customQueryExample(ctx context.Context, repos *repository.SQLiteManager) error {
    // Use raw SQL for complex queries
    var results []User
    err := repos.Query(ctx, &results, `
        SELECT * FROM users 
        WHERE created_at > ? 
        ORDER BY created_at DESC 
        LIMIT ?
    `, time.Now().AddDate(-30, 0, 0).Unix(), 50)

    if err != nil {
        return fmt.Errorf("query failed: %w", err)
    }

    fmt.Printf("Found %d users created in last 30 days\n", len(results))
    return nil
}
```

### Aggregations

```go
func aggregationExample(ctx context.Context, repos *repository.SQLiteManager) error {
    // Count users
    count, err := repos.Count(ctx, &User{})
    if err != nil {
        return fmt.Errorf("count failed: %w", err)
    }

    fmt.Printf("Total users: %d\n", count)

    // Count with conditions
    recentCount, err := repos.Count(ctx, &User{}, repository.WithWhere("created_at > ?", time.Now().AddDate(-7, 0, 0).Unix()))
    if err != nil {
        return fmt.Errorf("recent count failed: %w", err)
    }

    fmt.Printf("Users created in last 7 days: %d\n", recentCount)
    return nil
}
```

## Bulk Operations

### Bulk Insert

```go
func bulkInsertExample(ctx context.Context, repos *repository.SQLiteManager) error {
    users := []interface{}{
        &User{Username: "user1", Email: "user1@example.com", Created: time.Now().Unix()},
        &User{Username: "user2", Email: "user2@example.com", Created: time.Now().Unix()},
        &User{Username: "user3", Email: "user3@example.com", Created: time.Now().Unix()},
    }

    err := repos.BulkInsert(ctx, users)
    if err != nil {
        return fmt.Errorf("bulk insert failed: %w", err)
    }

    fmt.Printf("Inserted %d users\n", len(users))
    return nil
}
```

### Bulk Update

```go
func bulkUpdateExample(ctx context.Context, repos *repository.SQLiteManager) error {
    users, err := repos.Find(ctx, &User{})
    if err != nil {
        return fmt.Errorf("find failed: %w", err)
    }

    for _, interfaceUser := range users {
        user := interfaceUser.(*User)
        user.Email = "updated_" + user.Email
    }

    err = repos.BulkUpdate(ctx, users)
    if err != nil {
        return fmt.Errorf("bulk update failed: %w", err)
    }

    fmt.Printf("Updated %d users\n", len(users))
    return nil
}
```

## Indexes and Optimization

### Creating Indexes

```go
func createIndexesExample(ctx context.Context, repos *repository.SQLiteManager) error {
    // Create index on username for faster lookups
    err := repos.CreateIndex(ctx, "users", "idx_username", "username")
    if err != nil {
        return fmt.Errorf("create index failed: %w", err)
    }

    fmt.Println("Index created successfully")
    return nil
}
```

## Error Handling

### Handling Errors

```go
func errorHandlingExample(ctx context.Context, repos *repository.SQLiteManager) {
    _, err := repos.FindByID(ctx, &User{}, 999)
    if err != nil {
        if repository.IsNotFoundError(err) {
            fmt.Println("User not found")
        } else if repository.IsDuplicateError(err) {
            fmt.Println("Duplicate key error")
        } else {
            fmt.Printf("Other error: %v\n", err)
        }
    }
}
```

## Best Practices

### 1. Always Close Connections

```go
repos, err := repository.NewRepository(dbPath)
if err != nil {
    return err
}
defer repos.Close()
```

### 2. Use Context Properly

```go
func withTimeout(ctx context.Context, repos *repository.SQLiteManager) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    _, err := repos.Find(ctx, &User{})
    return err
}
```

### 3. Use Transactions for Multi-Step Operations

```go
err := repos.Transaction(ctx, func(tx *repository.Transaction) error {
    // Multiple operations
    return nil
})
```

### 4. Handle Errors Gracefully

```go
if repository.IsNotFoundError(err) {
    // Handle not found
} else if repository.IsDuplicateError(err) {
    // Handle duplicate
} else {
    // Handle other errors
}
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/openapi-cli/openapi-cli/pkg/repository"
)

type User struct {
    ID       int64  `db:"id" json:"id"`
    Username string `db:"username" json:"username"`
    Email    string `db:"email" json:"email"`
    Created  int64  `db:"created_at" json:"created_at"`
}

func (u *User) TableName() string {
    return "users"
}

func main() {
    ctx := context.Background()
    dbPath := filepath.Join(os.TempDir(), "muster.db")

    // Initialize repository
    repos, err := repository.NewRepository(dbPath)
    if err != nil {
        panic(err)
    }
    defer repos.Close()

    // Create user
    user := &User{
        Username: "example_user",
        Email:    "user@example.com",
        Created:  time.Now().Unix(),
    }

    id, err := repos.Create(ctx, user)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Created user with ID: %d\n", id)

    // Query user
    found, err := repos.FindByID(ctx, &User{}, id)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found user: %+v\n", found)

    // List users
    users, err := repos.List(ctx, &User{}, repository.WithLimit(10))
    if err != nil {
        panic(err)
    }

    fmt.Printf("Total users: %d\n", len(users))
}
```
