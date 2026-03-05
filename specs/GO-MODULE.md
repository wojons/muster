# Go Module Specification

**Version:** 1.0.0  
**Last Updated:** 2026-02-28  
**Go Version:** 1.23.0

---

## Overview

This document defines the complete Go module specification for Muster, including all dependencies, versions, and build configurations.

---

## Complete go.mod File

```go
module github.com/muster/muster

go 1.23.0

require (
	// Core Framework Dependencies
	github.com/spf13/cobra v1.8.1
	github.com/spf13/viper v1.19.0
	github.com/getkin/kin-openapi v0.128.0
	go.starlark.net v0.0.0-20250205221240-492d3044cb0e
	github.com/bytecodealliance/wasmtime-go v28.0.0

	// Database Drivers & Tools
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/lib/pq v1.10.9
	github.com/go-sql-driver/mysql v1.8.1
	github.com/sqlc-dev/sqlc v1.28.0

	// HTTP Client
	github.com/go-resty/resty/v2 v2.16.2

	// Authentication & Security
	golang.org/x/oauth2 v0.26.0
	github.com/zalando/go-keyring v0.2.6

	// Cryptography
	golang.org/x/crypto v0.33.0
	filippo.io/age v1.2.1

	// Observability
	github.com/prometheus/client_golang v1.20.5
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
	go.opentelemetry.io/otel/sdk v1.34.0

	// CLI Utilities
	github.com/schollz/progressbar/v3 v3.18.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/fatih/color v1.18.0

	// Testing (indirect for production builds)
	github.com/stretchr/testify v1.10.0
	github.com/golang/mock v1.6.0
)

require (
	// Indirect Dependencies - Automatically managed by go mod tidy
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/yaml v0.3.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/magiconair/properties v1.8.9 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb4 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.61.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20250210185358-939b2ce775ac // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/term v0.29.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Tool dependencies for code generation
tool (
	github.com/sqlc-dev/sqlc/cmd/sqlc
	github.com/golang/mock/mockgen
)

// Build constraints and platform-specific configurations
// CGO is required for SQLite3 and keyring support
go 1.23.0

// Build tags for conditional compilation
// - sqlite: Enables SQLite database support (requires CGO)
// - keyring: Enables native credential storage (requires CGO on Linux/macOS)
// - wasm: Enables WebAssembly runtime support
// - metrics: Enables Prometheus metrics
// - tracing: Enables OpenTelemetry tracing
```

---

## Dependency Categories

### 1. Core Dependencies

#### CLI Framework
- **github.com/spf13/cobra v1.8.1**
  - Modern CLI framework for Go applications
  - Provides command structure, flags, and help generation
  - Powers the muster CLI interface

#### Configuration Management
- **github.com/spf13/viper v1.19.0**
  - Complete configuration solution
  - Supports JSON, YAML, TOML, environment variables
  - Configuration hierarchy and overrides

#### OpenAPI Parsing
- **github.com/getkin/kin-openapi v0.128.0**
  - OpenAPI 3.0 and 3.1 specification parser
  - Request/response validation
  - Used for API integration and code generation

#### DSL Runtime
- **go.starlark.net v0.0.0-20250205221240-492d3044cb0e**
  - Python-like configuration language
  - Sandboxed execution environment
  - Used for declarative configuration and scripting

#### WebAssembly Runtime
- **github.com/bytecodealliance/wasmtime-go v28.0.0**
  - High-performance WebAssembly runtime
  - Supports WASI and component model
  - Used for extensible plugin system

---

### 2. Database Dependencies

#### SQLite Driver
- **github.com/mattn/go-sqlite3 v1.14.24**
  - CGO-based SQLite driver
  - Full SQLite feature support
  - **Requires:** CGO enabled, build tag: `sqlite`

#### PostgreSQL Driver
- **github.com/lib/pq v1.10.9**
  - Pure Go PostgreSQL driver
  - SSL and advanced PostgreSQL features

#### MySQL Driver
- **github.com/go-sql-driver/mysql v1.8.1**
  - Pure Go MySQL driver
  - Connection pooling and prepared statements

#### SQL Code Generation
- **github.com/sqlc-dev/sqlc v1.28.0**
  - Generates type-safe Go code from SQL
  - Compile-time query validation
  - **Tool:** `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`

---

### 3. HTTP & Networking

#### HTTP Client
- **github.com/go-resty/resty/v2 v2.16.2**
  - Simple and extensible HTTP client
  - Automatic retries, middleware support
  - JSON/XML marshaling built-in

---

### 4. Authentication & Security

#### OAuth2
- **golang.org/x/oauth2 v0.26.0**
  - OAuth 2.0 client implementation
  - Support for all OAuth 2.0 flows
  - Token management and refresh

#### Credential Storage
- **github.com/zalando/go-keyring v0.2.6**
  - Cross-platform credential storage
  - Uses OS-native keyrings (macOS Keychain, Windows Credential Manager, Linux Secret Service)
  - **Requires:** CGO on Linux/macOS, build tag: `keyring`

#### Cryptography Extensions
- **golang.org/x/crypto v0.33.0**
  - Extended cryptographic primitives
  - Argon2, bcrypt, scrypt password hashing
  - SSH, OpenPGP implementations

#### Modern Encryption
- **filippo.io/age v1.2.1**
  - Modern encryption tool with simple API
  - X25519 + ChaCha20-Poly1305
  - Passphrase-based encryption support

---

### 5. Observability

#### Metrics
- **github.com/prometheus/client_golang v1.20.5**
  - Prometheus instrumentation library
  - Metrics exposition in Prometheus format
  - Build tag: `metrics`

#### Distributed Tracing
- **go.opentelemetry.io/otel v1.34.0**
  - OpenTelemetry Go SDK
  - Tracing, metrics, and logging
  - Vendor-neutral observability
  - Build tag: `tracing`

---

### 6. CLI Utilities

#### Progress Bars
- **github.com/schollz/progressbar/v3 v3.18.0**
  - Terminal progress bars
  - Customizable themes and styles

#### Table Formatting
- **github.com/olekukonko/tablewriter v0.0.5**
  - ASCII table generation
  - Alignment, padding, auto-wrap

#### Terminal Colors
- **github.com/fatih/color v1.18.0**
  - Cross-platform terminal color output
  - Supports Windows terminals

---

### 7. Testing Dependencies

#### Test Framework
- **github.com/stretchr/testify v1.10.0**
  - Assertion library for Go tests
  - Mock generation, test suites
  - Used only in test builds

#### Mock Generation
- **github.com/golang/mock v1.6.0**
  - Mocking framework for Go interfaces
  - **Tool:** `go install github.com/golang/mock/mockgen@latest`

---

## Build Constraints

### CGO Requirements

Certain features require CGO to be enabled:

```bash
# Enable CGO for all features
export CGO_ENABLED=1

# Linux: Install build dependencies for keyring
sudo apt-get install libsecret-1-dev  # Debian/Ubuntu
sudo yum install libsecret-devel      # RHEL/CentOS/Fedora

# macOS: No additional dependencies required

# Windows: No additional dependencies required for keyring
```

### Build Tags

| Tag | Description | Dependencies |
|-----|-------------|--------------|
| `sqlite` | Enable SQLite support | go-sqlite3 |
| `keyring` | Enable credential storage | go-keyring |
| `wasm` | Enable WebAssembly runtime | wasmtime-go |
| `metrics` | Enable Prometheus metrics | client_golang |
| `tracing` | Enable OpenTelemetry | otel |
| `noplugins` | Disable plugin system | - |

### Usage Examples

```bash
# Full build with all features
go build -tags "sqlite keyring wasm metrics tracing"

# Minimal build without optional features
go build -tags "noplugins"

# Build with only SQLite support
go build -tags "sqlite"

# Run tests with specific tags
go test -tags "sqlite keyring" ./...
```

---

## Tool Dependencies

Install these tools for development:

```bash
# SQL code generation
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Mock generation
go install github.com/golang/mock/mockgen@latest

# Development utilities
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## Platform-Specific Notes

### macOS
- All features fully supported
- Keyring uses native Keychain
- No additional dependencies

### Linux
- Install `libsecret-1-dev` for keyring support
- All features supported with CGO
- SQLite3 requires CGO

### Windows
- Keyring uses Windows Credential Manager
- All features supported
- Use WSL for Unix-like development

### FreeBSD/OpenBSD
- Limited keyring support
- SQLite3 works with CGO
- WebAssembly fully supported

---

## Optional Dependencies

These dependencies can be excluded for smaller builds:

| Feature | Dependencies | Size Impact | When to Exclude |
|---------|--------------|-------------|-----------------|
| SQLite | go-sqlite3 | ~2MB | Using PostgreSQL/MySQL only |
| Keyring | go-keyring | ~500KB | Using env-based credentials |
| WASM Runtime | wasmtime-go | ~5MB | Not using plugins |
| Metrics | client_golang | ~1MB | Not monitoring with Prometheus |
| Tracing | otel | ~2MB | Not using distributed tracing |
| Progress Bars | progressbar | ~200KB | CI/batch mode builds |

### Minimal Build Example

```go
// +build noplugins nosqlite nokeyring nometrics notracing

package main

// Minimal build excludes:
// - WebAssembly runtime
// - SQLite support
// - Native credential storage
// - Prometheus metrics
// - OpenTelemetry tracing
```

---

## Version Management

### Updating Dependencies

```bash
# Check for updates
go list -u -m all

# Update specific dependency
go get -u github.com/spf13/cobra

# Update all dependencies
go get -u ./...
go mod tidy

# Update to specific version
go get github.com/spf13/cobra@v1.9.0
```

### Compatibility

- **Minimum Go Version:** 1.23.0
- **Tested Go Versions:** 1.23.x, 1.24.x
- **Supported Platforms:** Linux, macOS, Windows, FreeBSD

---

## Security Considerations

1. **Keep dependencies updated:** Run `go list -u -m all` regularly
2. **Audit dependencies:** Use `go list -m -json all | nancy sleuth`
3. **Vulnerability scanning:** Run `govulncheck ./...`
4. **Minimal builds:** Exclude unused dependencies for attack surface reduction

---

## Quick Reference

### Initialize Module

```bash
go mod init github.com/muster/muster
```

### Download Dependencies

```bash
go mod download
```

### Verify Dependencies

```bash
go mod verify
```

### Vendor Dependencies (Optional)

```bash
go mod vendor
```

### Clean Module Cache

```bash
go clean -modcache
```
