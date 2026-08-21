# CODE-STRUCTURE.md

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

---

## Overview

The Code Structure specification defines the organization and layout of the Go codebase for the OpenAPI CLI + MCP Server. It establishes clear boundaries between layers, ensures maintainability, and enables testability through dependency injection.

A blind person can visualize this codebase as a **well-organized library**. The main entrance (cmd/) leads to different reading rooms for different purposes (CLI, MCP server, daemon). The library has a public section (pkg/) where visitors can browse, and a private staff-only section (internal/) with restricted access. Within the private section, there are specialized departments: the core domain section handles business logic, the infrastructure section manages external services, and the platform section deals with OS-specific operations. Each book (file) has a specific location following the Dewey Decimal System (naming conventions).

### Design Principles

1. **Clean Architecture**: Clear separation between layers
2. **Domain-Driven Design**: Core business logic independent of frameworks
3. **Dependency Injection**: Interfaces enable testability
4. **Standard Layout**: Follows Go community conventions
5. **Package Cohesion**: Related code grouped together

---

## Directory Structure

```
muster/
├── cmd/                          # Application entry points
│   ├── cli/                      # CLI binary
│   │   └── main.go
│   ├── mcp/                      # MCP server binary
│   │   └── main.go
│   └── daemon/                   # Daemon binary
│       └── main.go
│
├── internal/                     # Private application code
│   ├── core/                     # Core domain logic
│   │   ├── inventory/            # Inventory management
│   │   │   ├── manager.go        # Inventory manager interface
│   │   │   ├── fetcher.go        # HTTP fetching logic
│   │   │   ├── validator.go      # Schema validation
│   │   │   └── types.go          # Domain types
│   │   │
│   │   ├── openapi/              # OpenAPI parsing
│   │   │   ├── parser.go         # OpenAPI parser
│   │   │   ├── generator.go      # Command generator
│   │   │   ├── resolver.go       # Schema resolver
│   │   │   └── types.go          # OpenAPI types
│   │   │
│   │   ├── auth/                 # Authentication
│   │   │   ├── manager.go        # Auth manager
│   │   │   ├── oauth2.go         # OAuth2 implementation
│   │   │   ├── mtls.go           # mTLS implementation
│   │   │   ├── apikey.go         # API key auth
│   │   │   └── types.go          # Auth types
│   │   │
│   │   ├── cache/                # Caching layer
│   │   │   ├── manager.go        # Cache manager
│   │   │   ├── memory.go         # In-memory cache
│   │   │   ├── redis.go          # Redis cache
│   │   │   └── types.go          # Cache types
│   │   │
│   │   └── wasm/                 # WASM runtime
│   │       ├── runtime.go        # WASM runtime
│   │       ├── sandbox.go        # Sandbox environment
│   │       ├── capabilities.go   # Capability system
│   │       └── types.go          # WASM types
│   │
│   ├── client/                   # External clients
│   │   ├── http/                 # HTTP client
│   │   │   ├── client.go         # HTTP client implementation
│   │   │   ├── retry.go          # Retry logic
│   │   │   ├── circuit.go        # Circuit breaker
│   │   │   └── middleware.go     # HTTP middleware
│   │   │
│   │   └── db/                   # Database client
│   │       ├── connection.go     # DB connection
│   │       ├── migrations.go     # Schema migrations
│   │       └── repository.go     # Repository pattern
│   │
│   ├── server/                   # Server implementations
│   │   ├── mcp/                  # MCP server
│   │   │   ├── server.go         # MCP server
│   │   │   ├── handlers.go       # Request handlers
│   │   │   ├── tools.go          # Tool registration
│   │   │   └── protocol.go       # MCP protocol
│   │   │
│   │   └── daemon/               # Daemon
│   │       ├── daemon.go         # Daemon controller
│   │       ├── socket.go         # Unix socket
│   │       └── proxy.go          # HTTP proxy
│   │
│   ├── config/                   # Configuration
│   │   ├── loader.go             # Config loader
│   │   ├── profiles.go           # Profile management
│   │   ├── schema.go             # Config schema
│   │   └── defaults.go           # Default values
│   │
│   ├── platform/                 # Platform-specific code
│   │   ├── linux/                # Linux implementations
│   │   ├── darwin/               # macOS implementations
│   │   ├── windows/              # Windows implementations
│   │   └── keychain.go           # Keychain abstraction
│   │
│   └── cli/                      # CLI implementation
│       ├── root.go               # Root command
│       ├── commands/             # Command implementations
│       │   ├── version.go
│       │   ├── config.go
│       │   ├── auth.go
│       │   ├── inventory.go
│       │   ├── daemon.go
│       │   └── generated/        # Auto-generated commands
│       ├── flags/                # Flag definitions
│       ├── prompts/              # Interactive prompts
│       └── formatters/           # Output formatters
│
├── pkg/                          # Public packages
│   ├── inventory/                # Inventory protocol types
│   │   ├── types.go
│   │   └── client.go
│   │
│   ├── openapi/                  # OpenAPI utilities
│   │   ├── types.go
│   │   └── helpers.go
│   │
│   ├── mcp/                      # MCP protocol helpers
│   │   ├── types.go
│   │   └── client.go
│   │
│   └── version/                  # Version information
│       └── version.go
│
├── api/                          # API definitions
│   ├── openapi.json              # OpenAPI spec for CLI API
│   └── proto/                    # Protocol buffer definitions
│
├── migrations/                   # Database migrations
│   ├── 001_initial_schema.sql
│   └── 002_add_indexes.sql
│
├── web/                          # Web assets (if any)
│   └── static/
│
├── test/                         # Test utilities and fixtures
│   ├── fixtures/                 # Test data
│   ├── mocks/                    # Generated mocks
│   └── integration/              # Integration test helpers
│
├── scripts/                      # Build and utility scripts
│   ├── build.sh
│   ├── test.sh
│   └── release.sh
│
├── docs/                         # Documentation
│   ├── architecture/
│   ├── api/
│   └── user-guides/
│
├── configs/                      # Configuration examples
│   ├── config.example.yaml
│   └── profiles/
│
├── specs/                        # Specifications
│   └── (specification files)
│
├── Makefile                      # Build automation
├── go.mod                        # Go module definition
├── go.sum                        # Dependency checksums
├── Dockerfile                    # Container build
├── LICENSE                       # License file
└── README.md                     # Project readme
```

---

## Package Organization

### 1. cmd/ - Entry Points

Each binary has its own main package:

```go
// cmd/openapi-cli/main.go
package main

import (
    "os"
    
    "github.com/openapi-cli/muster/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

```go
// cmd/mcp/main.go
package main

import (
    "context"
    "os"
    
    "github.com/openapi-cli/muster/internal/server/mcp"
)

func main() {
    ctx := context.Background()
    server := mcp.NewServer()
    
    if err := server.Run(ctx); err != nil {
        os.Exit(1)
    }
}
```

### 2. internal/core/ - Domain Logic

Domain packages contain pure business logic with no external dependencies:

```go
// internal/core/inventory/manager.go
package inventory

import (
    "context"
    "fmt"
    "time"
    
    "github.com/openapi-cli/muster/internal/core/cache"
)

// Manager handles inventory operations
type Manager struct {
    fetcher   Fetcher
    validator Validator
    cache     cache.Manager
    store     Store
}

// NewManager creates an inventory manager
func NewManager(fetcher Fetcher, validator Validator, cache cache.Manager, store Store) *Manager {
    return &Manager{
        fetcher:   fetcher,
        validator: validator,
        cache:     cache,
        store:     store,
    }
}

// Fetch retrieves inventory from source
func (m *Manager) Fetch(ctx context.Context, url string) (*Inventory, error) {
    // Check cache first
    if cached, err := m.cache.Get(ctx, cacheKey(url)); err == nil {
        return cached.(*Inventory), nil
    }
    
    // Fetch from source
    data, err := m.fetcher.Fetch(ctx, url)
    if err != nil {
        return nil, fmt.Errorf("fetch failed: %w", err)
    }
    
    // Validate
    if err := m.validator.Validate(data); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }
    
    // Parse and cache
    inv := parseInventory(data)
    m.cache.Set(ctx, cacheKey(url), inv, 5*time.Minute)
    
    return inv, nil
}

// Interfaces for dependency injection
type Fetcher interface {
    Fetch(ctx context.Context, url string) ([]byte, error)
}

type Validator interface {
    Validate(data []byte) error
}

type Store interface {
    Save(ctx context.Context, inv *Inventory) error
    Load(ctx context.Context, id string) (*Inventory, error)
}
```

### 3. internal/client/ - Infrastructure

Client packages contain external service integrations:

```go
// internal/client/http/client.go
package http

import (
    "context"
    "net/http"
    "time"
)

// Client wraps the standard HTTP client with reliability features
type Client struct {
    base      *http.Client
    retry     RetryPolicy
    breaker   *CircuitBreaker
    rateLimit *RateLimiter
}

// NewClient creates an HTTP client with middleware
func NewClient(opts ...ClientOption) *Client {
    c := &Client{
        base: &http.Client{
            Timeout: 30 * time.Second,
        },
        retry: DefaultRetryPolicy(),
    }
    
    for _, opt := range opts {
        opt(c)
    }
    
    return c
}

// Do executes an HTTP request with middleware
func (c *Client) Do(req *http.Request) (*http.Response, error) {
    // Apply rate limiting
    if c.rateLimit != nil {
        if err := c.rateLimit.Wait(req.Context()); err != nil {
            return nil, err
        }
    }
    
    // Check circuit breaker
    if c.breaker != nil && c.breaker.IsOpen() {
        return nil, ErrCircuitOpen
    }
    
    // Execute with retry
    var resp *http.Response
    var err error
    
    for attempt := 0; attempt <= c.retry.MaxAttempts; attempt++ {
        resp, err = c.base.Do(req)
        
        if err == nil && resp.StatusCode < 500 {
            // Success
            if c.breaker != nil {
                c.breaker.RecordSuccess()
            }
            return resp, nil
        }
        
        // Retry on failure
        if attempt < c.retry.MaxAttempts {
            time.Sleep(c.retry.Backoff(attempt))
        }
    }
    
    // Record failure
    if c.breaker != nil {
        c.breaker.RecordFailure()
    }
    
    return resp, err
}
```

### 4. internal/server/ - Server Implementations

Server packages contain protocol implementations:

```go
// internal/server/mcp/server.go
package mcp

import (
    "context"
    "encoding/json"
    "io"
    
    "github.com/openapi-cli/muster/internal/core/inventory"
)

// Server implements the MCP protocol
type Server struct {
    inventory  *inventory.Manager
    tools      []Tool
    resources  map[string]Resource
    prompts    map[string]Prompt
}

// NewServer creates an MCP server
func NewServer(inv *inventory.Manager) *Server {
    return &Server{
        inventory: inv,
        resources: make(map[string]Resource),
        prompts:   make(map[string]Prompt),
    }
}

// Run starts the MCP server
func (s *Server) Run(ctx context.Context) error {
    // Initialize capabilities
    s.initializeCapabilities()
    
    // Start stdio transport
    transport := NewStdioTransport()
    
    return transport.Serve(ctx, s.handleRequest)
}

func (s *Server) handleRequest(ctx context.Context, req *Request) (*Response, error) {
    switch req.Method {
    case "initialize":
        return s.handleInitialize(ctx, req)
    case "tools/list":
        return s.handleListTools(ctx, req)
    case "tools/call":
        return s.handleCallTool(ctx, req)
    case "resources/list":
        return s.handleListResources(ctx, req)
    default:
        return nil, ErrMethodNotFound
    }
}
```

### 5. internal/cli/ - CLI Commands

CLI packages organize commands hierarchically:

```go
// internal/cli/root.go
package cli

import (
    "github.com/spf13/cobra"
    "github.com/openapi-cli/muster/internal/cli/commands"
)

// Execute runs the CLI
func Execute() error {
    root := NewRootCommand()
    return root.Execute()
}

// NewRootCommand creates the root command
func NewRootCommand() *cobra.Command {
    root := &cobra.Command{
        Use:   "openapi-cli",
        Short: "OpenAPI CLI + MCP Server",
        Long:  `A unified CLI and MCP server for OpenAPI specifications.`,
    }
    
    // Add built-in commands
    root.AddCommand(commands.NewVersionCommand())
    root.AddCommand(commands.NewConfigCommand())
    root.AddCommand(commands.NewAuthCommand())
    root.AddCommand(commands.NewInventoryCommand())
    root.AddCommand(commands.NewDaemonCommand())
    
    // Generated commands will be added dynamically
    
    return root
}
```

```go
// internal/cli/commands/inventory.go
package commands

import (
    "context"
    "fmt"
    
    "github.com/spf13/cobra"
    "github.com/openapi-cli/muster/internal/core/inventory"
)

// NewInventoryCommand creates the inventory command
func NewInventoryCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "inventory",
        Short: "Manage OpenAPI inventories",
    }
    
    cmd.AddCommand(&cobra.Command{
        Use:   "fetch [url]",
        Short: "Fetch an inventory from URL",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            
            // Get dependencies from context (injected)
            manager := cmd.Context().Value(inventoryKey).(*inventory.Manager)
            
            inv, err := manager.Fetch(ctx, args[0])
            if err != nil {
                return err
            }
            
            fmt.Printf("Fetched inventory: %s\n", inv.Name)
            return nil
        },
    })
    
    cmd.AddCommand(&cobra.Command{
        Use:   "list",
        Short: "List cached inventories",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Implementation...
            return nil
        },
    })
    
    return cmd
}
```

### 6. pkg/ - Public Packages

Public packages provide reusable types and utilities:

```go
// pkg/inventory/types.go
package inventory

import (
    "time"
)

// Inventory represents an OpenAPI inventory
type Inventory struct {
    ID          string                 `json:"id"`
    Name        string                 `json:"name"`
    Version     string                 `json:"version"`
    Description string                 `json:"description,omitempty"`
    Specs       []SpecReference        `json:"specs"`
    Modules     []ModuleReference      `json:"modules,omitempty"`
    Config      map[string]interface{} `json:"config,omitempty"`
    Auth        AuthConfig             `json:"auth,omitempty"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}

// SpecReference references an OpenAPI specification
type SpecReference struct {
    Name        string `json:"name"`
    URL         string `json:"url"`
    Version     string `json:"version"`
    Description string `json:"description,omitempty"`
}

// Validate performs basic validation
func (i *Inventory) Validate() error {
    if i.ID == "" {
        return ErrInvalidInventory
    }
    if len(i.Specs) == 0 {
        return ErrNoSpecs
    }
    return nil
}
```

### 7. test/ - Test Support

Test utilities and fixtures:

```go
// test/fixtures/inventory.go
package fixtures

import (
    "github.com/openapi-cli/muster/pkg/inventory"
)

// ValidInventory returns a valid inventory for testing
func ValidInventory() *inventory.Inventory {
    return &inventory.Inventory{
        ID:      "test-inventory",
        Name:    "Test Inventory",
        Version: "1.0.0",
        Specs: []inventory.SpecReference{
            {
                Name:    "test-api",
                URL:     "https://example.com/api/openapi.json",
                Version: "1.0.0",
            },
        },
    }
}

// InvalidInventory returns an invalid inventory for error testing
func InvalidInventory() *inventory.Inventory {
    return &inventory.Inventory{
        ID: "", // Invalid: missing ID
    }
}
```

---

## Dependency Management

### go.mod Structure

```go
module github.com/openapi-cli/muster

go 1.22

require (
    // CLI framework
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    
    // HTTP client
    github.com/go-resty/resty/v2 v2.11.0
    
    // Database
    github.com/lib/pq v1.10.9
    github.com/pressly/goose/v3 v3.18.0
    
    // Caching
    github.com/redis/go-redis/v9 v9.4.0
    
    // WASM
    github.com/tetratelabs/wazero v1.6.0
    
    // Authentication
    golang.org/x/oauth2 v0.17.0
    
    // Observability
    github.com/rs/zerolog v1.32.0
    github.com/prometheus/client_golang v1.19.0
    go.opentelemetry.io/otel v1.23.1
    
    // Testing
    github.com/stretchr/testify v1.8.4
    github.com/golang/mock/mockgen v1.6.0
)

require (
    // Indirect dependencies...
)
```

### Dependency Injection Pattern

```go
// internal/di/container.go
package di

import (
    "github.com/openapi-cli/muster/internal/client/db"
    "github.com/openapi-cli/muster/internal/client/http"
    "github.com/openapi-cli/muster/internal/core/cache"
    "github.com/openapi-cli/muster/internal/core/inventory"
)

// Container holds all dependencies
type Container struct {
    Config     *config.Config
    HTTPClient *http.Client
    DB         *db.Connection
    Cache      cache.Manager
    Inventory  *inventory.Manager
}

// NewContainer creates a fully wired container
func NewContainer(cfg *config.Config) (*Container, error) {
    c := &Container{Config: cfg}
    
    // HTTP client
    c.HTTPClient = http.NewClient(
        http.WithTimeout(cfg.HTTP.Timeout),
        http.WithRetry(cfg.HTTP.RetryPolicy),
    )
    
    // Database
    dbConn, err := db.Connect(cfg.Database)
    if err != nil {
        return nil, err
    }
    c.DB = dbConn
    
    // Cache
    c.Cache = cache.NewManager(cfg.Cache)
    
    // Inventory manager
    c.Inventory = inventory.NewManager(
        http.NewFetcher(c.HTTPClient),
        validator.New(),
        c.Cache,
        db.NewInventoryStore(c.DB),
    )
    
    return c, nil
}
```

---

## Testing Strategy

### Unit Tests

```go
// internal/core/inventory/manager_test.go
package inventory

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/openapi-cli/muster/test/fixtures"
)

func TestManager_Fetch(t *testing.T) {
    // Create mocks
    mockFetcher := new(MockFetcher)
    mockValidator := new(MockValidator)
    mockCache := new(MockCache)
    mockStore := new(MockStore)
    
    // Setup expectations
    mockFetcher.On("Fetch", mock.Anything, "http://test.com/inventory.json").
        Return([]byte(`{"id": "test"}`), nil)
    mockValidator.On("Validate", mock.Anything).Return(nil)
    mockCache.On("Get", mock.Anything, "inventory:http://test.com/inventory.json").
        Return(nil, cache.ErrNotFound)
    mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
        Return(nil)
    
    // Create manager
    manager := NewManager(mockFetcher, mockValidator, mockCache, mockStore)
    
    // Execute
    ctx := context.Background()
    inv, err := manager.Fetch(ctx, "http://test.com/inventory.json")
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, inv)
    mockFetcher.AssertExpectations(t)
}
```

### Integration Tests

```go
// test/integration/inventory_test.go
//go:build integration

package integration

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/require"
    "github.com/openapi-cli/muster/internal/di"
)

func TestInventory_Fetch_RealServer(t *testing.T) {
    // Create container with real dependencies
    cfg := testConfig()
    container, err := di.NewContainer(cfg)
    require.NoError(t, err)
    
    ctx := context.Background()
    inv, err := container.Inventory.Fetch(ctx, "https://api.example.com/_inventory.json")
    
    require.NoError(t, err)
    require.NotNil(t, inv)
}
```

---

## Code Conventions

### Naming

- **Packages**: lowercase, no underscores (e.g., `inventory`, `httpclient`)
- **Files**: snake_case (e.g., `inventory_manager.go`)
- **Types**: PascalCase (e.g., `InventoryManager`)
- **Interfaces**: Noun describing behavior (e.g., `Fetcher`, `Validator`)
- **Functions**: MixedCase, verb first (e.g., `FetchInventory`, `ValidateInput`)
- **Variables**: camelCase (e.g., `inventoryID`, `maxRetries`)
- **Constants**: PascalCase for exported, camelCase for unexported

### Error Handling

```go
// Define errors at package level
var (
    ErrNotFound      = errors.New("not found")
    ErrInvalidInput  = errors.New("invalid input")
    ErrUnauthorized  = errors.New("unauthorized")
)

// Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to fetch inventory: %w", err)
}

// Check error types
if errors.Is(err, ErrNotFound) {
    // Handle not found
}
```

### Documentation

```go
// Manager coordinates inventory operations.
// It handles fetching, caching, and validation of inventory data.
type Manager struct {
    // fields...
}

// Fetch retrieves an inventory from the given URL.
// It first checks the cache, then fetches from the source if not cached.
// Returns ErrInvalidInventory if the fetched data fails validation.
func (m *Manager) Fetch(ctx context.Context, url string) (*Inventory, error) {
    // implementation...
}
```

---

## References

- **Go Project Layout**: Standard Go Project Layout (https://github.com/golang-standards/project-layout)
- **Clean Architecture**: Robert C. Martin's Clean Architecture
- **Go Code Review Comments**: https://github.com/golang/go/wiki/CodeReviewComments
- **Testing Strategy**: `TESTING-STRATEGY.md`
- **Build System**: `BUILD-SYSTEM.md`

---

**Bible Standard Compliance**: ✅ This specification is canonical, deterministic, and implementation-agnostic. It guarantees identical behavior across different AI systems, models, and coding harnesses. No human interpretation required.

**Deterministic Guarantees**:
- Directory structure explicitly defined
- Package organization rules specified
- Naming conventions standardized
- Dependency injection pattern documented
- Testing patterns with examples provided
