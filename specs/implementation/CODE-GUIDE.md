# Code Guide - Go Implementation Specifications

> STATUS: HISTORICAL — the file paths in this spec use an early layout that was never
> built for three of its four `### File:` headings. Only `cmd/openapi-cli/main.go` exists;
> CLI command generation is `pkg/generator/generator.go` and the MCP server is
> `pkg/mcp/server.go` (entry point `cmd/mcp-server/main.go`). The parenthetical note on
> each heading below gives its real location.

## Project Structure

```
openapi-cli/
├── cmd/
│   ├── cli/
│   │   └── main.go              # CLI entry point
│   ├── mcp/
│   │   └── main.go              # MCP server entry point
│   └── daemon/
│       └── main.go              # Daemon entry point
│
├── internal/
│   ├── core/
│   │   ├── inventory/           # Inventory management
│   │   ├── config/              # Configuration
│   │   ├── auth/                # Authentication
│   │   ├── crypto/              # Cryptography
│   │   ├── http/                # HTTP client
│   │   ├── dsl/                 # Starlark DSL
│   │   └── wasm/                # WASM runtime
│   │
│   ├── cli/
│   │   ├── commands/            # CLI commands
│   │   └── parser.go            # Argument parsing
│   │
│   ├── mcp/
│   │   ├── server.go            # MCP server
│   │   └── tools.go             # Tool generation
│   │
│   ├── daemon/
│   │   ├── server.go            # Daemon server
│   │   ├── socket.go            # Unix socket
│   │   └── proxy.go             # Connection proxy
│   │
│   └── db/
│       ├── store.go             # Database interface
│       └── sqlite/              # SQLite implementation
│
├── pkg/
│   └── api/                     # Public API types
│
├── server/
│   └── ...                      # Bundled server (separate)
│
└── docs/
    └── ...                      # Documentation
```

---

## 1. Core Interfaces

### Inventory Manager

```go
// internal/core/inventory/manager.go

package inventory

import (
    "context"
    "time"
)

// Manager handles inventory fetching, caching, and lifecycle
type Manager interface {
    // Fetch retrieves inventory from remote or cache
    Fetch(ctx context.Context, host string, opts FetchOptions) (*Inventory, error)
    
    // GetCached returns cached inventory without network call
    GetCached(ctx context.Context, host string, version string) (*Inventory, error)
    
    // ListVersions returns all cached versions for a host
    ListVersions(ctx context.Context, host string) ([]VersionInfo, error)
    
    // MarkGood marks a version as "known good"
    MarkGood(ctx context.Context, host string, version string) error
    
    // GetKnownGood returns the user-marked "known good" version
    GetKnownGood(ctx context.Context, host string) (*Inventory, error)
    
    // CheckForUpdates checks if newer version available
    CheckForUpdates(ctx context.Context, host string) (*UpdateInfo, error)
    
    // ClearCache removes all cached data for a host
    ClearCache(ctx context.Context, host string) error
    
    // Close releases resources
    Close() error
}

// FetchOptions controls fetch behavior
type FetchOptions struct {
    Version       string        // Specific version to fetch (empty = latest)
    ForceRefresh  bool          // Bypass cache
    Timeout       time.Duration // Request timeout
    ClientVersion string        // Current CLI version
    Fingerprint   string        // Client fingerprint
}

// Inventory represents the configuration from server
type Inventory struct {
    Version       string          `json:"version"`
    MinCLIVersion string          `json:"min_cli_version"`
    MaxCLIVersion string          `json:"max_cli_version"`
    UpdatedAt     time.Time       `json:"updated_at"`
    OpenAPI       OpenAPISource   `json:"openapi"`
    Modules       ModuleConfig    `json:"modules"`
    Config        SystemConfig    `json:"config"`
    Auth          AuthConfig      `json:"auth"`
    Signatures    Signatures      `json:"signatures"`
    
    // Metadata
    Host       string    `json:"-"`
    ETag       string    `json:"-"`
    CachedAt   time.Time `json:"-"`
    CacheUntil time.Time `json:"-"`
}

// VersionInfo describes a cached version
type VersionInfo struct {
    Version   string    `json:"version"`
    FetchedAt time.Time `json:"fetched_at"`
    IsKnownGood bool    `json:"is_known_good"`
    SizeBytes int64     `json:"size_bytes"`
}

// UpdateInfo describes available updates
type UpdateInfo struct {
    CurrentVersion string `json:"current_version"`
    LatestVersion  string `json:"latest_version"`
    HasUpdate      bool   `json:"has_update"`
    IsRequired     bool   `json:"is_required"` // Breaking changes
}
```

### HTTP Client

```go
// internal/core/http/client.go

package http

import (
    "context"
    "net/http"
    "time"
)

// Client is the HTTP client interface
type Client interface {
    // Do executes an HTTP request with middleware
    Do(ctx context.Context, req *Request) (*Response, error)
    
    // Get is a convenience method for GET requests
    Get(ctx context.Context, path string, opts RequestOptions) (*Response, error)
    
    // Post is a convenience method for POST requests
    Post(ctx context.Context, path string, body interface{}, opts RequestOptions) (*Response, error)
    
    // SetAuth sets authentication for subsequent requests
    SetAuth(auth AuthProvider)
    
    // SetRateLimiter sets rate limiting configuration
    SetRateLimiter(limiter RateLimiter)
    
    // Close releases resources
    Close() error
}

// Request represents an HTTP request
type Request struct {
    Method  string            `json:"method"`
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
    Body    []byte            `json:"body,omitempty"`
    
    // Internal
    ctx context.Context
}

// Response represents an HTTP response
type Response struct {
    StatusCode int               `json:"status_code"`
    Headers    map[string]string `json:"headers"`
    Body       []byte            `json:"body"`
    
    // Metadata
    Duration   time.Duration `json:"duration"`
    RetryCount int           `json:"retry_count"`
}

// RequestOptions configures a request
type RequestOptions struct {
    Headers         map[string]string
    Timeout         time.Duration
    SkipRetry       bool
    SkipRateLimit   bool
}

// AuthProvider provides authentication
type AuthProvider interface {
    Apply(req *Request) error
    Refresh(ctx context.Context) error
    IsExpired() bool
}

// RateLimiter controls request rate
type RateLimiter interface {
    Allow(ctx context.Context, key string) error
    GetRemaining(key string) int
    GetResetTime(key string) time.Time
}
```

### WASM Runtime

```go
// internal/core/wasm/runtime.go

package wasm

import (
    "context"
    "time"
)

// Runtime executes WASM modules
type Runtime interface {
    // LoadModule loads a WASM module into the runtime
    LoadModule(ctx context.Context, spec ModuleSpec) (Module, error)
    
    // GetModule returns a cached module
    GetModule(hash string) (Module, bool)
    
    // UnloadModule removes a module from cache
    UnloadModule(hash string) error
    
    // Execute runs a function in a loaded module
    Execute(ctx context.Context, moduleHash string, function string, input []byte) (*ExecutionResult, error)
    
    // SetCapability sets a capability grant
    SetCapability(cap Capability)
    
    // Close releases all resources
    Close() error
}

// Module represents a loaded WASM module
type Module interface {
    // Hash returns the content hash
    Hash() string
    
    // Name returns the module name
    Name() string
    
    // Exports returns available functions
    Exports() []string
    
    // MemoryUsage returns current memory usage
    MemoryUsage() int64
    
    // Close unloads the module
    Close() error
}

// ModuleSpec describes a module to load
type ModuleSpec struct {
    Name        string
    Content     []byte
    SHA256      string
    Signature   string
    Capabilities []string
}

// ExecutionResult is the result of WASM execution
type ExecutionResult struct {
    Output   []byte        `json:"output"`
    Duration time.Duration `json:"duration"`
    MemoryUsed int64       `json:"memory_used"`
}

// Capability controls what a module can do
type Capability struct {
    HTTP         bool     `json:"http"`
    HTTPHosts    []string `json:"http_hosts"`
    Filesystem   bool     `json:"filesystem"`
    FSReadPaths  []string `json:"fs_read_paths"`
    FSWritePaths []string `json:"fs_write_paths"`
    MaxMemory    int64    `json:"max_memory"`    // bytes
    MaxCPUTime   time.Duration `json:"max_cpu_time"`
}
```

### DSL Engine

```go
// internal/core/dsl/engine.go

package dsl

import (
    "context"
)

// Engine executes Starlark scripts
type Engine interface {
    // Execute runs a script with the given context
    Execute(ctx context.Context, script string, env Environment) (*ExecutionResult, error)
    
    // Validate checks if a script is syntactically valid
    Validate(script string) error
    
    // RegisterBuiltin registers a Go function as a Starlark builtin
    RegisterBuiltin(name string, fn BuiltinFunc)
    
    // SetMaxSteps sets the maximum execution steps
    SetMaxSteps(max int64)
    
    // SetMaxMemory sets the maximum memory usage
    SetMaxMemory(max int64)
}

// Environment provides context for script execution
type Environment struct {
    Variables map[string]interface{} `json:"variables"`
    HTTPClient HTTPClient           `json:"-"` // Injected
    Logger     Logger               `json:"-"` // Injected
}

// ExecutionResult is the result of script execution
type ExecutionResult struct {
    Result    interface{} `json:"result"`
    Steps     int64       `json:"steps"`
    MemoryUsed int64      `json:"memory_used"`
}

// BuiltinFunc is a Go function exposed to Starlark
type BuiltinFunc func(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

// HTTPClient is the interface for HTTP operations from DSL
type HTTPClient interface {
    Get(url string) (*HTTPResponse, error)
    Post(url string, body map[string]interface{}) (*HTTPResponse, error)
    Patch(url string, body map[string]interface{}) (*HTTPResponse, error)
    Delete(url string) (*HTTPResponse, error)
}

// HTTPResponse represents an HTTP response in DSL
type HTTPResponse struct {
    StatusCode int                 `json:"status_code"`
    Body       interface{}         `json:"body"`
    Headers    map[string]string   `json:"headers"`
}

// Logger provides logging from DSL
type Logger interface {
    Info(msg string)
    Warn(msg string)
    Error(msg string)
}
```

### Configuration

```go
// internal/core/config/config.go

package config

import (
    "time"
)

// Manager handles configuration loading and merging
type Manager interface {
    // Load loads configuration from all sources
    Load() (*Configuration, error)
    
    // Get returns the current effective configuration
    Get() *Configuration
    
    // Reload forces a configuration reload
    Reload() error
    
    // Save persists configuration to disk
    Save(config *Configuration) error
    
    // Watch watches for configuration changes
    Watch(callback func(*Configuration))
}

// Configuration is the complete effective configuration
type Configuration struct {
    // Core settings
    Host          string        `json:"host" yaml:"host"`
    InventoryURL  string        `json:"inventory_url" yaml:"inventory_url"`
    
    // TLS settings
    TLS TLSConfig `json:"tls" yaml:"tls"`
    
    // Auth settings
    Auth AuthSettings `json:"auth" yaml:"auth"`
    
    // Cache settings
    Cache CacheConfig `json:"cache" yaml:"cache"`
    
    // Execution settings
    RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`
    Retry     RetryConfig     `json:"retry" yaml:"retry"`
    
    // Proxy settings
    Proxy ProxyConfig `json:"proxy" yaml:"proxy"`
    
    // Logging
    LogLevel string `json:"log_level" yaml:"log_level"`
    
    // Fingerprints
    Fingerprinting FingerprintConfig `json:"fingerprinting" yaml:"fingerprinting"`
    
    // Profiles
    Profiles map[string]Profile `json:"profiles" yaml:"profiles"`
    CurrentProfile string `json:"current_profile" yaml:"current_profile"`
}

// TLSConfig contains TLS settings
type TLSConfig struct {
    CertFile string `json:"cert_file" yaml:"cert_file"`
    KeyFile  string `json:"key_file" yaml:"key_file"`
    CAFile   string `json:"ca_file" yaml:"ca_file"`
    Insecure bool   `json:"insecure" yaml:"insecure"` // dev only
}

// AuthSettings contains authentication configuration
type AuthSettings struct {
    Token      string            `json:"token,omitempty" yaml:"token,omitempty"`
    APIKey     string            `json:"api_key,omitempty" yaml:"api_key,omitempty"`
    OAuth      OAuthConfig       `json:"oauth,omitempty" yaml:"oauth,omitempty"`
    mTLS       bool              `json:"mtls" yaml:"mtls"`
}

// OAuthConfig contains OAuth settings
type OAuthConfig struct {
    ClientID     string   `json:"client_id" yaml:"client_id"`
    ClientSecret string   `json:"client_secret,omitempty" yaml:"client_secret,omitempty"`
    TokenURL     string   `json:"token_url" yaml:"token_url"`
    Scopes       []string `json:"scopes" yaml:"scopes"`
}

// CacheConfig contains cache settings
type CacheConfig struct {
    Dir         string        `json:"dir" yaml:"dir"`
    MaxSize     int64         `json:"max_size" yaml:"max_size"`         // bytes
    MaxAge      time.Duration `json:"max_age" yaml:"max_age"`           // duration
    TTL         time.Duration `json:"ttl" yaml:"ttl"`                   // cache TTL
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
    Enabled          bool          `json:"enabled" yaml:"enabled"`
    RequestsPerMin   int           `json:"requests_per_min" yaml:"requests_per_min"`
    Burst            int           `json:"burst" yaml:"burst"`
}

// RetryConfig contains retry settings
type RetryConfig struct {
    MaxAttempts  int           `json:"max_attempts" yaml:"max_attempts"`
    InitialDelay time.Duration `json:"initial_delay" yaml:"initial_delay"`
    MaxDelay     time.Duration `json:"max_delay" yaml:"max_delay"`
    Backoff      string        `json:"backoff" yaml:"backoff"` // "linear", "exponential"
}

// ProxyConfig contains proxy settings
type ProxyConfig struct {
    URL      string `json:"url" yaml:"url"`
    SOCKS5   bool   `json:"socks5" yaml:"socks5"`
}

// FingerprintConfig contains fingerprinting settings
type FingerprintConfig struct {
    Enabled              bool     `json:"enabled" yaml:"enabled"`
    RegenerateOnIPChange bool     `json:"regenerate_on_ip_change" yaml:"regenerate_on_ip_change"`
    Components           []string `json:"components" yaml:"components"` // "ip", "hardware", "random"
}

// Profile is a named configuration set
type Profile struct {
    Host     string            `json:"host" yaml:"host"`
    Version  string            `json:"version,omitempty" yaml:"version,omitempty"`
    Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
    Auth     AuthSettings      `json:"auth,omitempty" yaml:"auth,omitempty"`
}
```

---

## 2. CLI Implementation

### File: `cmd/openapi-cli/main.go`

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/openapi-cli/core/internal/cli"
    "github.com/openapi-cli/core/internal/core/config"
    "github.com/openapi-cli/core/internal/core/inventory"
    "github.com/spf13/cobra"
)

var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
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
    cfgManager, err := config.NewManager(config.Options{
        ConfigDir: getConfigDir(),
    })
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }
    
    cfg, err := cfgManager.Load()
    if err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }
    
    // Create root command
    rootCmd := &cobra.Command{
        Use:   "openapi-cli",
        Short: "Dynamic CLI for OpenAPI specifications",
        Long: `A CLI tool that dynamically generates commands from OpenAPI specifications.
        
Get started:
  openapi-cli --host https://api.example.com --help
  
For more information, visit https://openapi-cli.io`,
        Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
    }
    
    // Add global flags
    rootCmd.PersistentFlags().String("host", "", "API host URL")
    rootCmd.PersistentFlags().String("config", "", "Config file path")
    rootCmd.PersistentFlags().String("profile", "", "Configuration profile")
    rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
    rootCmd.PersistentFlags().Bool("use-known-good", false, "Use last known good version")
    
    // Add built-in commands
    rootCmd.AddCommand(newVersionCommand())
    rootCmd.AddCommand(newUpdateCommand())
    rootCmd.AddCommand(newAliasCommand(cfgManager))
    rootCmd.AddCommand(newCacheCommand())
    rootCmd.AddCommand(newConfigCommand(cfgManager))
    rootCmd.AddCommand(newDebugCommand())
    
    // If host is specified, load inventory and generate dynamic commands
    host, _ := rootCmd.Flags().GetString("host")
    if host != "" {
        invManager, err := inventory.NewManager(inventory.Options{
            Host:       host,
            Config:     cfg,
            CacheDir:   getCacheDir(),
            HTTPClient: nil, // Will create default
        })
        if err != nil {
            return fmt.Errorf("failed to create inventory manager: %w", err)
        }
        defer invManager.Close()
        
        // Fetch inventory
        inv, err := invManager.Fetch(ctx, host, inventory.FetchOptions{
            ClientVersion: version,
        })
        if err != nil {
            return fmt.Errorf("failed to fetch inventory: %w", err)
        }
        
        // Generate dynamic commands from OpenAPI
        dynamicCmds, err := cli.GenerateCommands(inv, invManager)
        if err != nil {
            return fmt.Errorf("failed to generate commands: %w", err)
        }
        
        rootCmd.AddCommand(dynamicCmds...)
    }
    
    return rootCmd.Execute()
}

func getConfigDir() string {
    if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
        return filepath.Join(dir, "openapi-cli")
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".config", "openapi-cli")
}

func getCacheDir() string {
    if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
        return filepath.Join(dir, "openapi-cli")
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".cache", "openapi-cli")
}
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/generator/generator.go`.

### File: `internal/cli/generator.go` (real: `pkg/generator/generator.go`)

```go
package cli

import (
    "fmt"
    "strings"
    
    "github.com/openapi-cli/core/internal/core/inventory"
    "github.com/spf13/cobra"
)

// GenerateCommands creates cobra commands from OpenAPI spec
func GenerateCommands(inv *inventory.Inventory, invMgr inventory.Manager) ([]*cobra.Command, error) {
    // Parse OpenAPI spec
    spec, err := parseOpenAPI(inv.OpenAPI)
    if err != nil {
        return nil, fmt.Errorf("failed to parse OpenAPI: %w", err)
    }
    
    var commands []*cobra.Command
    
    // Group operations by tag
    groups := groupOperationsByTag(spec.Operations)
    
    for tag, ops := range groups {
        groupCmd := &cobra.Command{
            Use:   strings.ToLower(tag),
            Short: fmt.Sprintf("Operations for %s", tag),
        }
        
        for _, op := range ops {
            cmd := createCommand(op, invMgr)
            groupCmd.AddCommand(cmd)
        }
        
        commands = append(commands, groupCmd)
    }
    
    return commands, nil
}

func createCommand(op Operation, invMgr inventory.Manager) *cobra.Command {
    cmd := &cobra.Command{
        Use:   op.OperationID,
        Short: op.Summary,
        Long:  op.Description,
        RunE: func(cmd *cobra.Command, args []string) error {
            // Execute the operation
            return executeOperation(cmd.Context(), op, invMgr)
        },
    }
    
    // Add flags for parameters
    for _, param := range op.Parameters {
        switch param.Type {
        case "string":
            cmd.Flags().String(param.Name, "", param.Description)
        case "integer":
            cmd.Flags().Int(param.Name, 0, param.Description)
        case "boolean":
            cmd.Flags().Bool(param.Name, false, param.Description)
        }
        
        if param.Required {
            cmd.MarkFlagRequired(param.Name)
        }
    }
    
    return cmd
}

func executeOperation(ctx context.Context, op Operation, invMgr inventory.Manager) error {
    // TODO: Implement operation execution
    return nil
}
```

---

## 3. MCP Server Implementation

> STATUS: HISTORICAL — this path was never built. Real implementation: `cmd/mcp-server/main.go`.

### File: `cmd/mcp/main.go` (real: `cmd/mcp-server/main.go`)

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/openapi-cli/core/internal/mcp"
    "github.com/openapi-cli/core/internal/core/config"
    "github.com/openapi-cli/core/internal/core/inventory"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

func run() error {
    ctx := context.Background()
    
    // Get host from environment or flag
    host := os.Getenv("OPENAPICLI_HOST")
    if host == "" {
        return fmt.Errorf("OPENAPICLI_HOST environment variable required")
    }
    
    // Load configuration
    cfgManager, err := config.NewManager(config.Options{
        ConfigDir: getConfigDir(),
    })
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }
    
    cfg, err := cfgManager.Load()
    if err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }
    
    // Create inventory manager
    invManager, err := inventory.NewManager(inventory.Options{
        Host:     host,
        Config:   cfg,
        CacheDir: getCacheDir(),
    })
    if err != nil {
        return fmt.Errorf("failed to create inventory manager: %w", err)
    }
    defer invManager.Close()
    
    // Fetch inventory
    inv, err := invManager.Fetch(ctx, host, inventory.FetchOptions{
        ClientVersion: "mcp-1.0.0",
    })
    if err != nil {
        return fmt.Errorf("failed to fetch inventory: %w", err)
    }
    
    // Create and start MCP server
    server, err := mcp.NewServer(mcp.Options{
        Inventory:      inv,
        InventoryManager: invManager,
    })
    if err != nil {
        return fmt.Errorf("failed to create MCP server: %w", err)
    }
    
    // Start stdio transport
    transport := mcp.NewStdioTransport()
    return server.Serve(transport)
}
```

> STATUS: HISTORICAL — this path was never built. Real implementation: `pkg/mcp/server.go`.

### File: `internal/mcp/server.go` (real: `pkg/mcp/server.go`)

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/mcp-sdk/mcp-go/mcp"
    "github.com/mcp-sdk/mcp-go/server"
    "github.com/openapi-cli/core/internal/core/inventory"
)

// Server implements the MCP protocol
type Server struct {
    mcpServer          *server.MCPServer
    inventory          *inventory.Inventory
    inventoryManager   inventory.Manager
}

// Options for creating a server
type Options struct {
    Inventory          *inventory.Inventory
    InventoryManager   inventory.Manager
}

// NewServer creates a new MCP server
func NewServer(opts Options) (*Server, error) {
    s := &Server{
        inventory:        opts.Inventory,
        inventoryManager: opts.InventoryManager,
    }
    
    // Create MCP server
    s.mcpServer = server.NewMCPServer(
        "openapi-cli",
        opts.Inventory.Version,
        server.WithResourceCapabilities(true, true),
        server.WithLogging(),
    )
    
    // Register tools from OpenAPI
    if err := s.registerTools(); err != nil {
        return nil, fmt.Errorf("failed to register tools: %w", err)
    }
    
    return s, nil
}

// Serve starts serving requests on the given transport
func (s *Server) Serve(transport server.Transport) error {
    return s.mcpServer.Serve(transport)
}

func (s *Server) registerTools() error {
    // Parse OpenAPI spec
    spec, err := parseOpenAPI(s.inventory.OpenAPI)
    if err != nil {
        return err
    }
    
    // Register each operation as a tool
    for _, op := range spec.Operations {
        tool := mcp.Tool{
            Name:        op.OperationID,
            Description: op.Description,
            InputSchema: generateInputSchema(op),
        }
        
        s.mcpServer.AddTool(tool, s.createToolHandler(op))
    }
    
    return nil
}

func (s *Server) createToolHandler(op Operation) server.ToolHandlerFunc {
    return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // Execute the operation
        result, err := s.executeOperation(ctx, op, request.Params.Arguments)
        if err != nil {
            return nil, fmt.Errorf("operation failed: %w", err)
        }
        
        // Format result
        content, err := json.MarshalIndent(result, "", "  ")
        if err != nil {
            return nil, fmt.Errorf("failed to marshal result: %w", err)
        }
        
        return &mcp.CallToolResult{
            Content: []mcp.Content{
                mcp.TextContent{
                    Type: "text",
                    Text: string(content),
                },
            },
        }, nil
    }
}

func (s *Server) executeOperation(ctx context.Context, op Operation, args map[string]interface{}) (interface{}, error) {
    // TODO: Implement operation execution
    return nil, nil
}
```

---

## Deterministic Guarantees

This code guide must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Interface Definitions

Interface definitions must enable identical implementations across platforms:

- **Method signatures**: Identical parameter ordering and types
- **Error definitions**: Same error types and error chain structure
- **Concurrency patterns**: Deterministic concurrent access patterns (see `DETERMINISTIC-GUARANTEES.md` section 1.2)

### 2. Platform‑Independent Struct Layouts

Struct layouts must be platform‑independent:

- **Field ordering**: Deterministic field serialization order (JSON tags)
- **Memory alignment**: Platform‑aware padding for cross‑language compatibility
- **Zero values**: Deterministic zero value initialization

### 3. Deterministic Function Signatures

Function signatures must be deterministic:

- **Parameter ordering**: Consistent parameter ordering across similar functions
- **Return type consistency**: Same return types for same operations
- **Context propagation**: Identical context parameter position (always first)

### 4. Cross‑Language Code Generation

When generating SDKs for 12 languages, code guides must produce:

- **Identical interfaces**: Same method signatures across languages
- **Identical structs**: Same data models with identical field mapping
- **Identical error handling**: Same error types and propagation

### 5. Testing Determinism

All code guide examples must be tested for determinism:

```go
//go:build linux || darwin || windows
func TestCodeGuideDeterminism(t *testing.T) {
	// Verify identical interface implementations
	// Verify identical struct serialization
	// Verify identical function behavior
}
```

### 6. Integration with SDK Generation

The code guide serves as the blueprint for 12 language SDK generation:

- **Interface mapping**: Deterministic mapping of Go interfaces to language‑native interfaces
- **Type conversion**: Deterministic type conversion rules (Go → language‑native)
- **Error translation**: Deterministic error type mapping

**Verification**: Run `./scripts/validate‑determinism‑code‑guide.sh` to confirm cross‑platform consistency.

This code guide provides complete interface definitions, struct layouts, and function signatures for Ralph Loop implementation.
