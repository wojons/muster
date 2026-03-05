# Package Organization Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Project Structure

## Overview

Complete specification for **package organization** defining the directory structure, package hierarchy, and code organization principles for the OpenAPI CLI project. This specification ensures consistent, maintainable, and scalable codebase organization following Go best practices and domain-driven design principles.

## 1. Directory Structure

### 1.1 Root Layout

```
openapi-cli/
├── cmd/                          # Application entry points
│   ├── openapi-cli/             # CLI binary
│   │   └── main.go
│   ├── daemon/                  # Daemon binary
│   │   └── main.go
│   └── mcp-server/              # MCP server binary
│       └── main.go
├── pkg/                          # Public library packages
│   ├── api/                     # Public API types
│   ├── auth/                    # Authentication
│   ├── cache/                   # Caching
│   ├── client/                  # HTTP clients
│   ├── config/                  # Configuration
│   ├── daemon/                  # Daemon functionality
│   ├── dsl/                     # Starlark DSL
│   ├── inventory/               # Inventory management
│   ├── mcp/                     # MCP protocol
│   ├── openapi/                 # OpenAPI parsing
│   ├── plugin/                  # Plugin system
│   ├── rate/                    # Rate limiting
│   └── wasm/                    # WASM runtime
├── internal/                     # Private implementation
│   ├── builtin/                 # Built-in commands
│   ├── commands/                # Command implementations
│   ├── database/                # Database layer
│   ├── middleware/              # HTTP middleware
│   ├── platform/                # Platform-specific code
│   ├── rendering/               # Output formatting
│   ├── storage/                 # Storage backends
│   └── telemetry/               # Metrics/tracing
├── api/                          # API definitions
│   ├── proto/                   # Protocol Buffer files
│   └── openapi/                 # OpenAPI specs
├── web/                          # Web console assets
│   ├── static/                  # Static files
│   ├── templates/               # HTML templates
│   └── src/                     # Source assets
├── scripts/                      # Build and utility scripts
├── configs/                      # Configuration examples
├── deployments/                  # Deployment configs
├── docs/                         # Documentation
├── specs/                        # Specifications
└── test/                         # Integration tests
    ├── e2e/                     # End-to-end tests
    ├── fixtures/                # Test data
    └── integration/             # Integration tests
```

### 1.2 Package by Layer

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│  cmd/openapi-cli  cmd/daemon  cmd/mcp-server               │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Interface Layer                          │
│  pkg/mcp  pkg/daemon  internal/builtin                     │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Application Services                     │
│  pkg/api  pkg/auth  pkg/cache  pkg/config                  │
│  pkg/inventory  pkg/openapi  pkg/plugin                    │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Domain Services                          │
│  pkg/client  pkg/dsl  pkg/wasm  internal/commands          │
│  internal/database  internal/middleware                    │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Infrastructure                           │
│  internal/platform  internal/rendering                     │
│  internal/storage  internal/telemetry                      │
└─────────────────────────────────────────────────────────────┘
```

## 2. Package Organization Rules

### 2.1 Package Naming Conventions

| Package Type | Naming Rule | Example |
|--------------|-------------|---------|
| Domain package | Singular noun | `auth`, `cache`, `plugin` |
| Utility package | Describes function | `httputil`, `stringutil` |
| Interface package | Protocol + suffix | `mcp`, `openapi` |
| Internal package | Descriptive | `builtin`, `middleware` |
| Test package | Same as code + `_test` | `auth_test`, `cache_test` |

### 2.2 Package Visibility

```go
// pkg/ packages are PUBLIC API
// Can be imported by external projects
// Must maintain backward compatibility
package auth

// internal/ packages are PRIVATE
// Cannot be imported by external projects
// Can change without notice
package builtin
```

### 2.3 Package Size Guidelines

| Metric | Target | Maximum |
|--------|--------|---------|
| Files per package | 5-10 | 20 |
| Lines per file | 200-500 | 1000 |
| Public symbols | 5-15 | 30 |
| Dependencies | 3-8 | 15 |

## 3. Core Packages Specification

### 3.1 cmd/openapi-cli

```go
// cmd/openapi-cli/main.go
// Application entry point for CLI binary

package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/openapi-cli/pkg/config"
    "github.com/openapi-cli/internal/builtin"
    "github.com/spf13/cobra"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

func run() error {
    ctx := context.Background()
    
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }
    
    // Build root command
    rootCmd := builtin.NewRootCommand(cfg)
    
    // Execute
    return rootCmd.ExecuteContext(ctx)
}
```

### 3.2 pkg/api

```go
// pkg/api/types.go
// Public API types used across packages

package api

import (
    "time"
)

// ErrorResponse represents API error
type ErrorResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

// Operation represents an OpenAPI operation
type Operation struct {
    ID          string            `json:"id"`
    Method      string            `json:"method"`
    Path        string            `json:"path"`
    Summary     string            `json:"summary,omitempty"`
    Description string            `json:"description,omitempty"`
    Tags        []string          `json:"tags,omitempty"`
    Parameters  []Parameter       `json:"parameters,omitempty"`
    RequestBody *RequestBody      `json:"requestBody,omitempty"`
    Responses   map[string]Response `json:"responses,omitempty"`
}

// Parameter represents an operation parameter
type Parameter struct {
    Name        string `json:"name"`
    In          string `json:"in"` // query, path, header, cookie
    Required    bool   `json:"required,omitempty"`
    Description string `json:"description,omitempty"`
    Schema      *Schema `json:"schema,omitempty"`
}
```

### 3.3 pkg/auth

```go
// pkg/auth/auth.go
// Authentication abstractions

package auth

import (
    "context"
    "net/http"
)

// Provider is the interface for authentication providers
type Provider interface {
    // Name returns provider identifier
    Name() string
    
    // AuthenticateRequest modifies request with auth credentials
    AuthenticateRequest(ctx context.Context, req *http.Request) error
    
    // Refresh updates credentials if expired
    Refresh(ctx context.Context) error
    
    // IsExpired checks if credentials need refresh
    IsExpired() bool
    
    // Revoke invalidates credentials
    Revoke(ctx context.Context) error
}

// Registry manages multiple auth providers
type Registry struct {
    providers map[string]Provider
    mu        sync.RWMutex
}

// Register adds a provider to the registry
func (r *Registry) Register(name string, provider Provider) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.providers[name] = provider
}

// Get retrieves a provider by name
func (r *Registry) Get(name string) (Provider, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    p, ok := r.providers[name]
    return p, ok
}
```

### 3.4 pkg/cache

```go
// pkg/cache/cache.go
// Caching abstractions

package cache

import (
    "context"
    "time"
)

// Cache is the interface for cache implementations
type Cache interface {
    // Get retrieves value by key
    Get(ctx context.Context, key string) ([]byte, error)
    
    // Set stores value with optional TTL
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    
    // Delete removes key from cache
    Delete(ctx context.Context, key string) error
    
    // Exists checks if key exists
    Exists(ctx context.Context, key string) (bool, error)
    
    // Clear removes all entries
    Clear(ctx context.Context) error
    
    // Close releases resources
    Close() error
}

// MultiTier implements layered caching
type MultiTier struct {
    tiers []Cache // Ordered: L1 (fast) to Ln (slow)
}

// Get attempts to retrieve from each tier
func (m *MultiTier) Get(ctx context.Context, key string) ([]byte, error) {
    for i, tier := range m.tiers {
        val, err := tier.Get(ctx, key)
        if err == nil {
            // Populate faster tiers
            for j := 0; j < i; j++ {
                m.tiers[j].Set(ctx, key, val, 0)
            }
            return val, nil
        }
    }
    return nil, ErrNotFound
}
```

### 3.5 pkg/config

```go
// pkg/config/config.go
// Configuration management

package config

type Config struct {
    // Core settings
    Version   string    `yaml:"version"`
    Profile   string    `yaml:"profile"`
    
    // Paths
    ConfigDir string    `yaml:"-"` // Computed
    CacheDir  string    `yaml:"-"` // Computed
    DataDir   string    `yaml:"-"` // Computed
    
    // Feature toggles
    Features  Features  `yaml:"features"`
    
    // Service configurations
    Auth      AuthConfig      `yaml:"auth"`
    Cache     CacheConfig     `yaml:"cache"`
    Daemon    DaemonConfig    `yaml:"daemon"`
    Inventory InventoryConfig `yaml:"inventory"`
    MCP       MCPConfig       `yaml:"mcp"`
    Plugin    PluginConfig    `yaml:"plugin"`
}

type Features struct {
    WASM      bool `yaml:"wasm"`
    DSL       bool `yaml:"dsl"`
    Cache     bool `yaml:"cache"`
    Telemetry bool `yaml:"telemetry"`
}

// Load reads configuration from files and environment
func Load() (*Config, error) {
    cfg := &Config{
        Version: "1.0.0",
    }
    
    // Set default paths
    cfg.setDefaultPaths()
    
    // Load from file
    if err := cfg.loadFromFile(); err != nil {
        return nil, err
    }
    
    // Override from environment
    cfg.loadFromEnv()
    
    return cfg, nil
}
```

### 3.6 pkg/plugin

```go
// pkg/plugin/plugin.go
// Plugin system interface

package plugin

import (
    "context"
)

// Plugin is the interface for all plugins
type Plugin interface {
    Metadata() Metadata
    Initialize(ctx context.Context, config map[string]interface{}) error
    Shutdown(ctx context.Context) error
    Health(ctx context.Context) HealthStatus
    Capabilities() Capabilities
}

// Manager handles plugin lifecycle
type Manager struct {
    loader    *Loader
    registry  *Registry
    instances map[string]*Instance
}

// Load discovers and loads all plugins
func (m *Manager) Load(ctx context.Context) error {
    plugins, err := m.loader.Discover(ctx)
    if err != nil {
        return err
    }
    
    for _, p := range plugins {
        if err := m.loader.Validate(ctx, p); err != nil {
            continue
        }
        
        instance, err := m.loader.Load(ctx, p)
        if err != nil {
            continue
        }
        
        if err := m.loader.Activate(ctx, instance); err != nil {
            continue
        }
        
        m.instances[instance.ID] = instance
    }
    
    return nil
}
```

### 3.7 internal/builtin

```go
// internal/builtin/root.go
// Built-in commands

package builtin

import (
    "github.com/openapi-cli/pkg/config"
    "github.com/spf13/cobra"
)

// NewRootCommand creates the root cobra command
func NewRootCommand(cfg *config.Config) *cobra.Command {
    root := &cobra.Command{
        Use:   "openapi-cli",
        Short: "OpenAPI-powered CLI and MCP server",
        Long: `A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.`,
        Version: cfg.Version,
    }
    
    // Add built-in commands
    root.AddCommand(newVersionCommand())
    root.AddCommand(newConfigCommand(cfg))
    root.AddCommand(newAuthCommand(cfg))
    root.AddCommand(newInventoryCommand(cfg))
    root.AddCommand(newDaemonCommand(cfg))
    root.AddCommand(newCompletionCommand())
    root.AddCommand(newHelpCommand())
    
    return root
}
```

### 3.8 internal/database

```go
// internal/database/db.go
// Database abstraction

package database

import (
    "context"
    "database/sql"
    
    _ "github.com/mattn/go-sqlite3"
    _ "github.com/lib/pq"
)

// DB wraps sql.DB with domain operations
type DB struct {
    *sql.DB
    driver string
}

// Config for database connection
type Config struct {
    Driver   string // sqlite, postgres, mysql
    DSN      string
    MaxConns int
}

// Open creates a new database connection
func Open(cfg Config) (*DB, error) {
    sqlDB, err := sql.Open(cfg.Driver, cfg.DSN)
    if err != nil {
        return nil, err
    }
    
    if cfg.MaxConns > 0 {
        sqlDB.SetMaxOpenConns(cfg.MaxConns)
    }
    
    if err := sqlDB.Ping(); err != nil {
        return nil, err
    }
    
    return &DB{DB: sqlDB, driver: cfg.Driver}, nil
}

// Migrate runs database migrations
func (db *DB) Migrate(ctx context.Context) error {
    // Implementation
    return nil
}
```

## 4. Import Rules

### 4.1 Allowed Imports

```go
// pkg packages can import:
// - Standard library
// - Other pkg packages (same or lower layer)
// - Third-party libraries

package auth

import (
    "context"           // stdlib: allowed
    "fmt"               // stdlib: allowed
    
    "github.com/openapi-cli/pkg/cache"     // same layer: allowed
    "github.com/openapi-cli/pkg/config"    // same layer: allowed
    // "github.com/openapi-cli/internal/database"  // NOT ALLOWED
    
    "github.com/spf13/cobra"  // third-party: allowed
)
```

### 4.2 Import Restrictions

```
Layer Dependencies:
  cmd/*        → pkg/*, internal/*
  pkg/*        → pkg/* (same or below), stdlib, third-party
  internal/*   → pkg/*, internal/* (same or below), stdlib, third-party
  api/         → stdlib only (no dependencies)
  test/        → all packages

Forbidden:
  - Circular imports
  - pkg importing internal
  - Higher layer importing lower layer directly
```

## 5. File Organization

### 5.1 File Naming

| File Type | Pattern | Example |
|-----------|---------|---------|
| Main implementation | `package.go` | `auth.go`, `cache.go` |
| Types/structs | `types.go` | `types.go` |
| Interfaces | `interfaces.go` | `interfaces.go` |
| Utilities | `util.go` | `stringutil.go` |
| Tests | `package_test.go` | `auth_test.go` |
| Integration tests | `package_integration_test.go` | `auth_integration_test.go` |
| Build constraints | `package_os.go` | `auth_linux.go` |
| Generated code | `package_gen.go` | `types_gen.go` |

### 5.2 Internal Package Structure

```go
// pkg/auth/
├── auth.go              # Main implementation
├── types.go             # Public types
├── interfaces.go        # Public interfaces
├── providers/           # Provider implementations
│   ├── oauth2.go
│   ├── apikey.go
│   ├── mtls.go
│   └── basic.go
├── store/               # Credential storage
│   ├── keychain.go
│   ├── file.go
│   └── memory.go
├── auth_test.go         # Unit tests
└── auth_integration_test.go  # Integration tests
```

## 6. Testing Organization

### 6.1 Test Structure

```
test/
├── e2e/                     # End-to-end tests
│   ├── cli_test.go
│   ├── daemon_test.go
│   └── mcp_test.go
├── fixtures/                # Test data
│   ├── openapi/
│   ├── wasm/
│   └── responses/
├── integration/             # Integration tests
│   ├── auth_integration_test.go
│   ├── cache_integration_test.go
│   └── database_integration_test.go
└── helpers/                 # Test utilities
    ├── mock_cache.go
    ├── mock_auth.go
    └── testserver.go
```

### 6.2 Test Package Rules

```go
// Unit tests: same package
package auth

import "testing"

func TestProvider_Authenticate(t *testing.T) {
    // Test implementation
}

// Integration tests: separate _test package
package auth_test

import (
    "testing"
    "github.com/openapi-cli/pkg/auth"
)

func TestAuthFlow_Integration(t *testing.T) {
    // Test implementation
}
```

## 7. Build Tags

### 7.1 Platform-Specific Code

```go
// internal/platform/pidfile_unix.go
//go:build unix

package platform

import "syscall"

func isProcessRunning(pid int) bool {
    process, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    return process.Signal(syscall.Signal(0)) == nil
}
```

```go
// internal/platform/pidfile_windows.go
//go:build windows

package platform

import "syscall"

func isProcessRunning(pid int) bool {
    process, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
    if err != nil {
        return false
    }
    syscall.CloseHandle(process)
    return true
}
```

### 7.2 Feature Tags

```go
// pkg/wasm/runtime_enabled.go
//go:build wasm

package wasm

func init() {
    RegisterRuntime("wazero", NewWazeroRuntime)
}
```

## 8. Documentation

### 8.1 Package Documentation

```go
// Package auth provides authentication abstractions and implementations.
//
// The auth package supports multiple authentication methods:
//   - OAuth2 (authorization code, client credentials, device flow)
//   - API Key (header, query, cookie)
//   - mTLS (client certificates)
//   - Basic authentication
//
// Usage:
//
//     provider, err := auth.NewOAuth2Provider(config)
//     if err != nil {
//         return err
//     }
//     
//     if err := provider.AuthenticateRequest(ctx, req); err != nil {
//         return err
//     }
//
package auth
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

*This specification produces identical implementations across different AI systems. No human interpretation required.*
