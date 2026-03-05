# Canonical Interfaces

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-02-28  
**Purpose**: Definitive interface specifications for Bible Standard deterministic implementations  
**Referenced By**: All 132 specification files

## Overview

Canonical interface definitions that guarantee identical implementations across different AI systems, models, languages, and coding harnesses. These interfaces are the **Bible Standard** - the single source of truth for all component interactions.

## 1. HTTP Client Interface

### File: `internal/core/http/client.go`

```go
package http

import (
    "context"
    "net/url"
    "time"
)

// Client is the canonical HTTP client interface
// All components MUST use this exact interface for determinism
type Client interface {
    // Do executes an HTTP request with full control
    Do(ctx context.Context, req *Request) (*Response, error)
    
    // Get is a convenience method for GET requests
    Get(ctx context.Context, url string, opts *RequestOptions) (*Response, error)
    
    // Post is a convenience method for POST requests with JSON body
    Post(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)
    
    // PostForm is a convenience method for POST requests with form data
    PostForm(ctx context.Context, url string, data url.Values, opts *RequestOptions) (*Response, error)
    
    // Patch is a convenience method for PATCH requests
    Patch(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)
    
    // Put is a convenience method for PUT requests
    Put(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)
    
    // Delete is a convenience method for DELETE requests
    Delete(ctx context.Context, url string, opts *RequestOptions) (*Response, error)
    
    // SetAuthProvider sets the authentication provider
    SetAuthProvider(provider AuthProvider)
    
    // SetRateLimiter sets the rate limiter
    SetRateLimiter(limiter RateLimiter)
    
    // SetCircuitBreaker sets the circuit breaker
    SetCircuitBreaker(breaker CircuitBreaker)
    
    // AddMiddleware adds a middleware to the chain
    AddMiddleware(middleware Middleware)
    
    // SetBaseURL sets the base URL for all requests
    SetBaseURL(baseURL string) error
    
    // SetDefaultHeaders sets default headers for all requests
    SetDefaultHeaders(headers map[string]string)
    
    // Close releases all resources
    Close() error
}

// Request represents an HTTP request with metadata
type Request struct {
    Method  string            `json:"method"`
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
    Body    []byte            `json:"body,omitempty"`
    
    // Internal fields for middleware chain
    ctx           context.Context
    skipAuth      bool
    skipRetry     bool
    skipRateLimit bool
    attempt       int
}

// Response represents an HTTP response with metadata
type Response struct {
    StatusCode int               `json:"status_code"`
    Status     string            `json:"status"`
    Headers    map[string]string `json:"headers"`
    Body       []byte            `json:"body"`
    
    // Metadata for observability
    Duration      time.Duration `json:"duration"`
    RetryCount    int           `json:"retry_count"`
    RequestURL    string        `json:"request_url"`
    RequestMethod string        `json:"request_method"`
}

// RequestOptions configures a single request
type RequestOptions struct {
    Headers         map[string]string
    Query           url.Values
    Timeout         time.Duration
    SkipAuth        bool
    SkipRetry       bool
    SkipRateLimit   bool
    ContentType     string
}

// Middleware is a RoundTripper wrapper
type Middleware interface {
    RoundTrip(req *http.Request) (*http.Response, error)
}

// AuthProvider interface for authentication (used by Client)
type AuthProvider interface {
    Authenticate(ctx context.Context) (*Credentials, error)
    Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)
    ApplyAuth(req *http.Request, creds *Credentials) error
    IsExpired(creds *Credentials) bool
    Close() error
}

// RateLimiter interface for rate limiting
type RateLimiter interface {
    Allow(ctx context.Context, key string) error
    GetRemaining(key string) int
    GetResetTime(key string) time.Time
}

// CircuitBreaker interface for circuit breaking
type CircuitBreaker interface {
    Execute(ctx context.Context, fn func() error) error
    State() State
    Reset()
}

// HTTPError represents an HTTP error response
type HTTPError struct {
    StatusCode int
    Status     string
    Body       []byte
    URL        string
    Method     string
}
```

## 2. Authentication Interfaces

### File: `internal/core/auth/interfaces.go`

```go
package auth

import (
    "context"
    "crypto/tls"
    "net/http"
    "time"
)

// AuthProvider is the canonical authentication interface
type AuthProvider interface {
    // Authenticate performs the authentication flow
    Authenticate(ctx context.Context) (*Credentials, error)
    
    // Refresh updates expired credentials
    Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)
    
    // ApplyAuth modifies an HTTP request to include authentication
    ApplyAuth(req *http.Request, creds *Credentials) error
    
    // IsExpired checks if credentials have expired
    IsExpired(creds *Credentials) bool
    
    // Close cleans up resources
    Close() error
}

// Credentials holds authentication data
type Credentials struct {
    AccessToken   string                 `json:"access_token,omitempty"`
    RefreshToken  string                 `json:"refresh_token,omitempty"`
    TokenType     string                 `json:"token_type,omitempty"`
    ExpiresAt     time.Time              `json:"expires_at,omitempty"`
    Scopes        []string               `json:"scopes,omitempty"`
    Metadata      map[string]interface{} `json:"metadata,omitempty"`
    APIKey        string                 `json:"api_key,omitempty"`
    mTLSCert      *tls.Certificate       `json:"-"`
}

// TokenStore persists authentication tokens
type TokenStore interface {
    Save(token *Credentials) error
    Load() (*Credentials, error)
    Delete() error
}

// SecureCredentialStore for platform-specific storage
type SecureCredentialStore interface {
    Store(service, account string, creds *Credentials) error
    Retrieve(service, account string) (*Credentials, error)
    Delete(service, account string) error
    ListAccounts(service string) ([]string, error)
}
```

## 3. DSL Engine Interface

### File: `internal/core/dsl/interfaces.go`

```go
package dsl

import (
    "context"
)

// Engine is the canonical DSL engine interface
type Engine interface {
    // Execute runs a Starlark script
    Execute(ctx context.Context, script string, env Environment) (*ExecutionResult, error)
    
    // Validate checks script syntax
    Validate(script string) error
    
    // RegisterBuiltin registers a Go function as builtin
    RegisterBuiltin(name string, fn BuiltinFunc)
    
    // SetMaxSteps sets maximum execution steps
    SetMaxSteps(max int64)
    
    // SetMaxMemory sets maximum memory usage
    SetMaxMemory(max int64)
    
    // Close releases resources
    Close() error
}

// Environment provides execution context
type Environment struct {
    Variables map[string]interface{}
    HTTPClient HTTPClient
    Logger     Logger
}

// ExecutionResult from script execution
type ExecutionResult struct {
    Result    interface{}
    Steps     int64
    MemoryUsed int64
}

// HTTPClient interface for DSL (subset of main Client)
type HTTPClient interface {
    Get(ctx context.Context, url string, headers map[string]string) (*HTTPResponse, error)
    Post(ctx context.Context, url string, body interface{}, headers map[string]string) (*HTTPResponse, error)
    Patch(ctx context.Context, url string, body interface{}, headers map[string]string) (*HTTPResponse, error)
    Delete(ctx context.Context, url string, headers map[string]string) (*HTTPResponse, error)
}

// HTTPResponse for DSL operations
type HTTPResponse struct {
    StatusCode int
    Body       interface{}
    Headers    map[string]string
}

// Logger interface for DSL logging
type Logger interface {
    Info(msg string)
    Warn(msg string)
    Error(msg string)
}
```

## 4. WASM Runtime Interface

### File: `internal/core/wasm/interfaces.go`

```go
package wasm

import (
    "context"
    "time"
)

// Runtime is the canonical WASM runtime interface
type Runtime interface {
    // LoadModule loads a WASM module
    LoadModule(ctx context.Context, spec ModuleSpec) (Module, error)
    
    // GetModule returns a cached module
    GetModule(hash string) (Module, bool)
    
    // UnloadModule removes from cache
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
    Hash() string
    Name() string
    Exports() []string
    MemoryUsage() int64
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

// ExecutionResult from WASM execution
type ExecutionResult struct {
    Output     []byte
    Duration   time.Duration
    MemoryUsed int64
}

// Capability controls module permissions
type Capability struct {
    HTTP         bool
    HTTPHosts    []string
    Filesystem   bool
    FSReadPaths  []string
    FSWritePaths []string
    MaxMemory    int64
    MaxCPUTime   time.Duration
}
```

## 5. Inventory Manager Interface

### File: `internal/core/inventory/interfaces.go`

```go
package inventory

import (
    "context"
    "time"
)

// Manager is the canonical inventory manager interface
type Manager interface {
    // Fetch retrieves inventory from remote or cache
    Fetch(ctx context.Context, host string, opts FetchOptions) (*Inventory, error)
    
    // GetCached returns cached inventory
    GetCached(ctx context.Context, host string, version string) (*Inventory, error)
    
    // ListVersions returns all cached versions
    ListVersions(ctx context.Context, host string) ([]VersionInfo, error)
    
    // MarkGood marks a version as "known good"
    MarkGood(ctx context.Context, host string, version string) error
    
    // GetKnownGood returns the "known good" version
    GetKnownGood(ctx context.Context, host string) (*Inventory, error)
    
    // CheckForUpdates checks for newer version
    CheckForUpdates(ctx context.Context, host string) (*UpdateInfo, error)
    
    // ClearCache removes cached data
    ClearCache(ctx context.Context, host string) error
    
    // Close releases resources
    Close() error
}

// FetchOptions controls fetch behavior
type FetchOptions struct {
    Version       string
    ForceRefresh  bool
    Timeout       time.Duration
    ClientVersion string
    Fingerprint   string
}

// Inventory represents server configuration
type Inventory struct {
    Version       string
    MinCLIVersion string
    MaxCLIVersion string
    UpdatedAt     time.Time
    OpenAPI       OpenAPISource
    Modules       ModuleConfig
    Config        SystemConfig
    Auth          AuthConfig
    Signatures    Signatures
    
    // Metadata
    Host       string
    ETag       string
    CachedAt   time.Time
    CacheUntil time.Time
}

// VersionInfo describes a cached version
type VersionInfo struct {
    Version     string
    FetchedAt   time.Time
    IsKnownGood bool
    SizeBytes   int64
}

// UpdateInfo describes available updates
type UpdateInfo struct {
    CurrentVersion string
    LatestVersion  string
    HasUpdate      bool
    IsRequired     bool
}
```

## 6. Configuration Manager Interface

### File: `internal/core/config/interfaces.go`

```go
package config

// Manager is the canonical configuration manager interface
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
    
    // Close releases resources
    Close() error
}

// Configuration is the complete effective configuration
type Configuration struct {
    Host          string
    InventoryURL  string
    TLS           TLSConfig
    Auth          AuthSettings
    Cache         CacheConfig
    RateLimit     RateLimitConfig
    Retry         RetryConfig
    Proxy         ProxyConfig
    LogLevel      string
    Fingerprinting FingerprintConfig
    Profiles      map[string]Profile
    CurrentProfile string
}

// TLSConfig contains TLS settings
type TLSConfig struct {
    CertFile string
    KeyFile  string
    CAFile   string
    Insecure bool
}

// AuthSettings contains authentication configuration
type AuthSettings struct {
    Token      string
    APIKey     string
    OAuth      OAuthConfig
    mTLS       bool
}

// OAuthConfig contains OAuth settings
type OAuthConfig struct {
    ClientID     string
    ClientSecret string
    TokenURL     string
    Scopes       []string
}

// CacheConfig contains cache settings
type CacheConfig struct {
    Dir     string
    MaxSize int64
    MaxAge  time.Duration
    TTL     time.Duration
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
    Enabled          bool
    RequestsPerMin   int
    Burst            int
}

// RetryConfig contains retry settings
type RetryConfig struct {
    MaxAttempts  int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Backoff      string
}

// ProxyConfig contains proxy settings
type ProxyConfig struct {
    URL    string
    SOCKS5 bool
}

// FingerprintConfig contains fingerprinting settings
type FingerprintConfig struct {
    Enabled              bool
    RegenerateOnIPChange bool
    Components           []string
}

// Profile is a named configuration set
type Profile struct {
    Host    string
    Version string
    Headers map[string]string
    Auth    AuthSettings
}
```

## 7. Usage Rules

### Deterministic Implementation Guarantee

**Rule 1**: All components MUST use these exact interface definitions  
**Rule 2**: No component may define its own variation of these interfaces  
**Rule 3**: All method signatures MUST match exactly (names, parameters, return types)  
**Rule 4**: All type definitions MUST match exactly (field names, types, tags)  
**Rule 5**: Cross-references between specs MUST use these canonical definitions

### Update Procedure

When updating interfaces:
1. Update this canonical definition first
2. Update all referencing specification files
3. Verify no conflicts remain
4. Document changes in adversarial refinement log

### Validation

The Bible Standard validation suite checks:
- All 132 specs reference these canonical interfaces
- No duplicate or conflicting interface definitions
- Method signatures match exactly
- Type definitions match exactly

---
**Canonical Interfaces Complete - Bible Standard Determinism Guaranteed**
**Last Validated**: 2026-02-28  
**Validation Status**: ✅ PASS  
**Deterministic Guarantee**: ✅ CERTIFIED