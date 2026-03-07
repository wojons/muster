# Integration Test Suite

**Version:** 1.1.0  
**Last Updated:** 2026-03-03  
**Status:** ✅ IMPLEMENTED - 6 test suites passing in demo/

---

## Quick Start

```bash
# Run all integration tests
cd demo && bash test-suites/run-all.sh

# Individual test suites
./demo/test-suites/01-cli-generation.sh
./demo/test-suites/02-mcp-flow.sh
./demo/test-suites/03-wasm-execution.sh
./demo/test-suites/04-auth-flows.sh
./demo/test-suites/05-daemon-socket.sh
./demo/test-suites/06-full-stack.sh
```

---

## Test Suites Overview

| Test Suite | Proves | Status |
|------------|--------|--------|
| 01-cli-generation | OpenAPI Parser → Command Generator | ✅ PASSING |
| 02-mcp-flow | CLI → MCP Server → Tool Registration | ✅ PASSING |
| 03-wasm-execution | MCP Server → WASM Runtime → Execution | ✅ PASSING |
| 04-auth-flows | Authentication → Protected Endpoints | ✅ PASSING |
| 05-daemon-socket | Daemon → Socket → Proxy → Response | ✅ PASSING |
| 06-full-stack | Complete pipeline end-to-end | ✅ PASSING |

---

---

## Integration Test Setup

```go
//go:build integration

package cache_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/openapi-cli/openapi-cli/pkg/cache"
)

func TestMain(m *testing.M) {
    // Global setup
    setupIntegrationTests()
    code := m.Run()
    // Global teardown
    teardownIntegrationTests()
    os.Exit(code)
}
```

---

## Database Setup/Teardown

```go
// testutil/db.go
package testutil

import (
    "context"
    "database/sql"
    "fmt"
    "testing"
    
    _ "github.com/lib/pq"
    "github.com/pressly/goose/v3"
)

type TestDB struct {
    DB       *sql.DB
    Database string
}

func NewTestDB(t *testing.T) *TestDB {
    t.Helper()
    
    // Connect to template database
    connStr := "postgres://openapi:openapi_test@localhost:5432/postgres?sslmode=disable"
    adminDB, err := sql.Open("postgres", connStr)
    require.NoError(t, err)
    defer adminDB.Close()
    
    // Create unique test database
    dbName := fmt.Sprintf("openapi_test_%d", time.Now().UnixNano())
    _, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
    require.NoError(t, err)
    
    // Connect to test database
    testConnStr := fmt.Sprintf("postgres://openapi:openapi_test@localhost:5432/%s?sslmode=disable", dbName)
    db, err := sql.Open("postgres", testConnStr)
    require.NoError(t, err)
    
    // Run migrations
    err = goose.Up(db, "../../migrations")
    require.NoError(t, err)
    
    t.Cleanup(func() {
        db.Close()
        // Drop test database
        adminDB, _ := sql.Open("postgres", connStr)
        adminDB.Exec(fmt.Sprintf("DROP DATABASE %s", dbName))
        adminDB.Close()
    })
    
    return &TestDB{DB: db, Database: dbName}
}
```

### Usage

```go
func TestUserRepository_Create(t *testing.T) {
    testDB := testutil.NewTestDB(t)
    repo := NewUserRepository(testDB.DB)
    
    user := &User{Name: "Alice", Email: "alice@example.com"}
    err := repo.Create(context.Background(), user)
    
    require.NoError(t, err)
    assert.NotZero(t, user.ID)
    
    // Verify in database
    var count int
    err = testDB.DB.QueryRow("SELECT COUNT(*) FROM users WHERE id = $1", user.ID).Scan(&count)
    require.NoError(t, err)
    assert.Equal(t, 1, count)
}
```

---

## Testcontainers for Dependencies

```go
package cache_test

import (
    "context"
    "testing"
    
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
)

func TestCache_WithPostgresContainer(t *testing.T) {
    ctx := context.Background()
    
    container, err := postgres.Run(ctx,
        "postgres:16-alpine",
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("testuser"),
        postgres.WithPassword("testpass"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections"),
        ),
    )
    require.NoError(t, err)
    defer container.Terminate(ctx)
    
    // Get connection string
    connStr, err := container.ConnectionString(ctx)
    require.NoError(t, err)
    
    // Use in tests
    db, err := sql.Open("postgres", connStr)
    require.NoError(t, err)
    defer db.Close()
    
    // Run tests...
}
```

### Redis Container

```go
func TestCache_WithRedis(t *testing.T) {
    ctx := context.Background()
    
    req := testcontainers.ContainerRequest{
        Image:        "redis:7-alpine",
        ExposedPorts: []string{"6379/tcp"},
        WaitingFor:   wait.ForLog("Ready to accept connections"),
    }
    
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    require.NoError(t, err)
    defer container.Terminate(ctx)
    
    // Get endpoint
    endpoint, err := container.Endpoint(ctx, "")
    require.NoError(t, err)
    
    // Connect to Redis
    client := redis.NewClient(&redis.Options{Addr: endpoint})
    // Run tests...
}
```

---

## Test Isolation

```go
func TestUserService_Transaction(t *testing.T) {
    testDB := testutil.NewTestDB(t)
    
    // Each test gets isolated transaction
    tx, err := testDB.DB.Begin()
    require.NoError(t, err)
    defer tx.Rollback() // Always rollback
    
    service := NewUserService(tx)
    
    // Create user
    user, err := service.CreateUser(context.Background(), "Alice")
    require.NoError(t, err)
    
    // Verify (will be rolled back after test)
    assert.Equal(t, "Alice", user.Name)
}
```

---

## Makefile Targets

```makefile
.PHONY: test-integration test-integration-ci

test-integration:
	docker-compose -f docker-compose.test.yaml up -d
	go test -v -tags=integration -count=1 ./...
	docker-compose -f docker-compose.test.yaml down

test-integration-ci:
	go test -v -tags=integration -count=1 -short ./...
```
