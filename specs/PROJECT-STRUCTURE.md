# Project Structure - openapi-cli

**GitHub**: github.com/openapi-cli/core  
**Language**: Go 1.21+  
**License**: MIT

---

## Complete Directory Tree

```
openapi-cli/
├── .github/
│   └── workflows/
│       ├── ci.yaml              # Continuous integration
│       ├── release.yaml         # Release automation
│       └── docker.yaml          # Docker builds
│
├── .agents/                     # AI agent configurations (optional)
│
├── api/                         # Public API definitions
│   └── proto/                   # Protocol buffer definitions
│       └── v1/
│           ├── inventory.proto
│           └── daemon.proto
│
├── bin/                         # Build artifacts (gitignored)
│   ├── openapi-cli              # CLI binary
│   ├── openapi-cli-mcp         # MCP server binary
│   └── openapi-cli-daemon      # Daemon binary
│
├── build/                       # Build scripts and assets
│   ├── scripts/
│   │   ├── build.sh            # Cross-platform build script
│   │   ├── release.sh          # Release automation
│   │   ├── install.sh          # Bootstrap installer
│   │   ├── sign.sh             # Binary signing
│   │   └── version.sh          # Version extraction
│   ├── wasm/                   # WASM build configurations
│   │   └── modules/
│   └── docker/
│       └── Dockerfile.*        # Multi-arch Dockerfiles
│
├── cmd/                         # Entry points (main packages)
│   ├── cli/
│   │   ├── main.go             # CLI entry point
│   │   └── commands/           # Built-in CLI commands
│   │       ├── version.go
│   │       ├── config.go
│   │       ├── cache.go
│   │       ├── alias.go
│   │       └── debug.go
│   │
│   ├── mcp/
│   │   ├── main.go             # MCP server entry point
│   │   └── stdio.go            # Stdio transport handler
│   │
│   └── daemon/
│       ├── main.go             # Daemon entry point
│       ├── server.go           # HTTP/gRPC server
│       └── socket.go           # Unix socket listener
│
├── configs/                     # Configuration templates
│   ├── openapi-cli.yaml        # Example config
│   ├── profiles.yaml            # Profile examples
│   └── systemd/                 # Systemd service files
│       ├── openapi-cli-daemon.service
│       └── openapi-cli-daemon.socket
│
├── deployments/                 # Deployment configurations
│   ├── docker-compose.yaml
│   ├── kubernetes/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── configmap.yaml
│   └── helm/
│       └── openapi-cli/
│
├── dist/                        # Distribution artifacts (gitignored)
│   ├── openapi-cli-darwin-amd64
│   ├── openapi-cli-darwin-arm64
│   ├── openapi-cli-linux-amd64
│   ├── openapi-cli-linux-arm64
│   ├── openapi-cli-windows-amd64.exe
│   └── checksums.txt
│
├── docs/                        # Documentation
│   ├── README.md
│   ├── CONTRIBUTING.md
│   ├── CHANGELOG.md
│   ├── SECURITY.md
│   ├── api/                     # API documentation
│   └── architecture/            # Architecture diagrams
│
├── examples/                    # Usage examples
│   ├── workflows/               # Starlark workflow examples
│   ├── configs/                 # Configuration examples
│   └── integrations/            # Third-party integrations
│
├── internal/                    # Private application code
│   │
│   ├── cli/                     # CLI-specific implementation
│   │   ├── commands/
│   │   │   ├── generated.go     # Auto-generated from OpenAPI
│   │   │   ├── executor.go      # Command execution logic
│   │   │   ├── formatter.go     # Output formatting
│   │   │   └── interactive.go   # Interactive prompts
│   │   ├── parser.go            # Argument parsing
│   │   ├── completer.go         # Shell completion
│   │   └── help.go              # Help text generation
│   │
│   ├── core/                    # Shared core components
│   │   │
│   │   ├── inventory/           # Inventory management
│   │   │   ├── manager.go         # Main interface
│   │   │   ├── fetcher.go         # HTTP fetching
│   │   │   ├── cache.go           # Cache operations
│   │   │   ├── validator.go       # Version validation
│   │   │   ├── types.go           # Data types
│   │   │   └── errors.go          # Error definitions
│   │   │
│   │   ├── config/              # Configuration management
│   │   │   ├── manager.go         # Config interface
│   │   │   ├── loader.go          # Config loading
│   │   │   ├── merger.go          # Config merging
│   │   │   ├── validator.go       # Config validation
│   │   │   ├── paths.go           # Path resolution
│   │   │   └── types.go           # Config types
│   │   │
│   │   ├── auth/                # Authentication
│   │   │   ├── manager.go         # Auth coordinator
│   │   │   ├── token.go           # Token management
│   │   │   ├── oauth.go           # OAuth2 flow
│   │   │   ├── jwt.go             # JWT handling
│   │   │   ├── mtls.go            # mTLS support
│   │   │   ├── keyring.go         # OS keyring integration
│   │   │   └── providers/         # Auth providers
│   │   │       ├── basic.go
│   │   │       ├── bearer.go
│   │   │       ├── oauth.go
│   │   │       └── apikey.go
│   │   │
│   │   ├── crypto/              # Cryptographic operations
│   │   │   ├── signer.go          # Signing interface
│   │   │   ├── gpg.go             # GPG verification
│   │   │   ├── ssh.go             # SSH verification
│   │   │   ├── x509.go            # X.509/TLS verification
│   │   │   ├── fingerprint.go     # Client fingerprinting
│   │   │   └── revocation.go      # Revocation checking
│   │   │
│   │   ├── http/                # HTTP client
│   │   │   ├── client.go          # HTTP client interface
│   │   │   ├── transport.go       # Transport layer
│   │   │   ├── middleware.go      # Middleware chain
│   │   │   ├── retry.go           # Retry logic
│   │   │   ├── ratelimit.go       # Rate limiting
│   │   │   ├── circuitbreaker.go  # Circuit breaker
│   │   │   └── metrics.go         # HTTP metrics
│   │   │
│   │   ├── dsl/                 # Starlark DSL
│   │   │   ├── engine.go          # Starlark engine
│   │   │   ├── builtins.go        # Built-in functions
│   │   │   ├── http.go            # HTTP builtins
│   │   │   ├── json.go            # JSON builtins
│   │   │   ├── limits.go          # Resource limits
│   │   │   └── sandbox.go         # Sandbox controls
│   │   │
│   │   ├── wasm/                # WASM runtime
│   │   │   ├── runtime.go         # WASM runtime interface
│   │   │   ├── module.go          # Module management
│   │   │   ├── executor.go        # Execution engine
│   │   │   ├── cache.go           # Module cache
│   │   │   ├── capabilities.go    # Capability grants
│   │   │   ├── host.go            # Host functions
│   │   │   └── limits.go          # Resource limits
│   │   │
│   │   ├── errors/              # Error handling
│   │   │   ├── errors.go          # Error types
│   │   │   ├── codes.go           # Error codes
│   │   │   └── recovery.go        # Panic recovery
│   │   │
│   │   ├── logging/             # Logging
│   │   │   ├── logger.go          # Logger interface
│   │   │   └── zerolog.go         # Zerolog implementation
│   │   │
│   │   ├── metrics/             # Metrics
│   │   │   ├── metrics.go         # Metrics interface
│   │   │   └── prometheus.go      # Prometheus implementation
│   │   │
│   │   └── version/             # Version management
│   │       ├── version.go         # Version checking
│   │       └── semver.go          # Semver parsing
│   │
│   ├── mcp/                     # MCP-specific implementation
│   │   ├── server.go              # MCP server
│   │   ├── transport.go           # Transport abstractions
│   │   ├── tools.go               # Tool registration
│   │   ├── resources.go           # Resource handling
│   │   ├── prompts.go             # Prompt templates
│   │   └── stdio.go               # Stdio transport
│   │
│   ├── daemon/                  # Daemon-specific implementation
│   │   ├── server.go              # Daemon HTTP server
│   │   ├── socket.go              # Unix socket handling
│   │   ├── proxy.go               # Connection proxy
│   │   ├── session.go             # Session management
│   │   ├── pool.go                # Connection pooling
│   │   └── api/                   # Daemon API handlers
│   │       ├── inventory.go
│   │       ├── execute.go
│   │       └── status.go
│   │
│   └── db/                      # Database layer
│       ├── store.go               # Database interface
│       ├── models.go              # Data models
│       │
│       ├── sqlite/                # SQLite implementation
│       │   ├── store.go
│       │   └── queries/
│       │       └── queries.sql
│       │
│       ├── postgres/              # PostgreSQL implementation
│       │   ├── store.go
│       │   └── queries/
│       │       └── queries.sql
│       │
│       ├── mysql/                 # MySQL implementation
│       │   ├── store.go
│       │   └── queries/
│       │       └── queries.sql
│       │
│       └── migrate/               # Migration management
│           ├── migrate.go
│           └── migrations/
│
├── migrations/                  # Database migrations
│   ├── 001_initial_schema.up.sql
│   ├── 001_initial_schema.down.sql
│   ├── 001_initial_schema.sqlite.sql
│   ├── 001_initial_schema.postgres.sql
│   ├── 001_initial_schema.mysql.sql
│   └── schema.sql               # Combined schema reference
│
├── pkg/                         # Public API (stable)
│   └── api/
│       ├── v1/
│       │   ├── types.go         # Public types
│       │   ├── client.go        # API client
│       │   └── errors.go        # Public errors
│       └── v2/                  # Future versions
│
├── queries/                     # SQL queries for sqlc
│   ├── inventory.sql
│   ├── alias.sql
│   ├── module.sql
│   ├── audit.sql
│   ├── fingerprint.sql
│   └── revocation.sql
│
├── scripts/                     # Development scripts
│   ├── setup.sh                 # Development setup
│   ├── lint.sh                  # Linting
│   ├── test.sh                  # Test runner
│   ├── coverage.sh              # Coverage report
│   └── generate.sh              # Code generation
│
├── server/                      # Reference server (separate binary)
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/                 # HTTP API handlers
│   │   ├── db/                  # Database layer
│   │   ├── storage/             # S3/CloudFront adapter
│   │   ├── auth/                # Server-side auth
│   │   ├── webhook/             # Webhook proxy
│   │   └── admin/               # Admin endpoints
│   └── migrations/
│
├── sqlc.yaml                    # sqlc configuration
│
├── test/                        # Test suites
│   ├── unit/                    # Unit tests
│   │   ├── core/
│   │   │   ├── inventory_test.go
│   │   │   ├── http_test.go
│   │   │   ├── auth_test.go
│   │   │   └── wasm_test.go
│   │   ├── cli/
│   │   ├── mcp/
│   │   └── daemon/
│   │
│   ├── integration/             # Integration tests
│   │   ├── api_test.go
│   │   ├── mcp_test.go
│   │   ├── daemon_test.go
│   │   └── database_test.go
│   │
│   ├── e2e/                     # End-to-end tests
│   │   └── scenarios/
│   │       ├── basic_usage_test.go
│   │       ├── cache_test.go
│   │       └── auth_test.go
│   │
│   ├── fixtures/                # Test fixtures
│   │   ├── openapi/
│   │   │   ├── petstore.yaml
│   │   │   └── github.yaml
│   │   ├── inventories/
│   │   │   └── sample.json
│   │   └── wasm/
│   │       ├── test.wasm
│   │       └── benchmark.wasm
│   │
│   ├── helpers/                 # Test helpers
│   │   ├── mock/
│   │   │   ├── http.go
│   │   │   └── store.go
│   │   ├── server.go
│   │   └── fixtures.go
│   │
│   └── benchmark/               # Performance benchmarks
│       └── performance_test.go
│
├── third_party/               # Vendored dependencies (if needed)
│
├── tools/                     # Development tools
│   ├── codegen/               # Code generation tools
│   └── mockgen/               # Mock generators
│
├── web/                       # Web assets (if applicable)
│   ├── static/
│   └── templates/
│
├── go.mod                     # Go module definition
├── go.sum                     # Go checksums
├── Makefile                   # Build automation
├── Dockerfile                 # Docker image
├── docker-compose.yml         # Development environment
├── LICENSE                    # MIT License
├── README.md                  # Project README
└── .gitignore                 # Git ignore rules
```

---

## Directory Purposes

### Root Level

| Directory | Purpose | Visibility |
|-----------|---------|------------|
| `cmd/` | Application entry points (main packages) | Public |
| `internal/` | Private application code | Private |
| `pkg/` | Public API for external use | Public (stable) |
| `api/` | API definitions (protobuf, OpenAPI) | Public |
| `test/` | Test suites and fixtures | Internal |
| `server/` | Reference implementation server | Separate binary |
| `configs/` | Configuration templates | Documentation |
| `deployments/` | Deployment configs (K8s, Docker) | DevOps |
| `docs/` | Documentation | Documentation |
| `examples/` | Usage examples | Documentation |
| `scripts/` | Development scripts | Development |
| `build/` | Build scripts and assets | Build |
| `migrations/` | Database migrations | Database |
| `queries/` | SQL queries for sqlc | Database |

### Internal Structure

| Directory | Purpose |
|-----------|---------|
| `internal/core/` | Shared business logic (inventory, auth, http, wasm, dsl) |
| `internal/cli/` | CLI-specific code (commands, parsing, formatting) |
| `internal/mcp/` | MCP server implementation |
| `internal/daemon/` | Daemon mode implementation |
| `internal/db/` | Database abstraction layer |

---

## File Organization by Type

### Entry Points (main packages)

```
cmd/
├── cli/main.go          # CLI: Human users, direct execution
├── mcp/main.go          # MCP: AI integration (stdio)
└── daemon/main.go       # Daemon: Multi-client, shared state
```

**Rule**: Each `main.go` should be minimal - delegate to `internal/<mode>/`

### Core Interfaces

```
internal/core/
├── inventory/manager.go    # InventoryManager interface
├── http/client.go          # HTTPClient interface
├── wasm/runtime.go         # WASMRuntime interface
├── dsl/engine.go           # DSLEngine interface
├── config/manager.go       # ConfigManager interface
└── auth/manager.go         # AuthManager interface
```

### Platform-Specific Code

```
internal/
├── platform/
│   ├── unix/
│   │   ├── socket_unix.go      # Unix-specific socket handling
│   │   └── keyring_unix.go     # Unix keyring
│   ├── windows/
│   │   ├── socket_windows.go   # Windows named pipes
│   │   └── keyring_windows.go  # Windows credential manager
│   └── darwin/
│       └── keyring_darwin.go   # macOS Keychain
```

**Pattern**: Use build tags: `//go:build unix`

### Database Implementations

```
internal/db/
├── store.go              # Interface: Store
├── sqlite/
│   ├── store.go          # Implementation
│   └── queries/          # SQL queries
├── postgres/
│   ├── store.go
│   └── queries/
└── mysql/
    ├── store.go
    └── queries/
```

---

## Import Path Conventions

### Module Path

```
github.com/openapi-cli/core
```

### Import Organization

```go
// Standard library
import (
    "context"
    "fmt"
    "time"
)

// Third-party imports
import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    "github.com/bytecodealliance/wasmtime-go"
)

// Internal imports (grouped by depth)
import (
    "github.com/openapi-cli/core/internal/core/inventory"
    "github.com/openapi-cli/core/internal/core/config"
    "github.com/openapi-cli/core/internal/core/http"
    "github.com/openapi-cli/core/internal/db"
    "github.com/openapi-cli/core/pkg/api/v1"
)
```

### Alias Conventions

```go
import (
    coreconfig "github.com/openapi-cli/core/internal/core/config"  // Avoid 'config' conflict
    dbconfig "github.com/openapi-cli/core/internal/db/config"      // Database config
)
```

---

## Build Artifact Locations

### Binaries

| Binary | Source | Output Path |
|--------|--------|-------------|
| `openapi-cli` | `cmd/openapi-cli/` | `bin/openapi-cli` |
| `openapi-cli-mcp` | `cmd/mcp/` | `bin/openapi-cli-mcp` |
| `openapi-cli-daemon` | `cmd/daemon/` | `bin/openapi-cli-daemon` |

### Cross-Platform Builds

```
dist/
├── openapi-cli-darwin-amd64          # macOS Intel
├── openapi-cli-darwin-arm64          # macOS Apple Silicon
├── openapi-cli-linux-amd64           # Linux x64
├── openapi-cli-linux-arm64           # Linux ARM64
├── openapi-cli-windows-amd64.exe     # Windows x64
└── checksums.txt                     # SHA256 checksums
```

### Build Commands

```bash
# Development build
make build

# All platforms
make build-all

# Release build
make release

# Docker image
make docker
```

---

## Configuration File Locations

### User Configuration

| Platform | Location |
|----------|----------|
| Linux | `~/.config/openapi-cli/config.yaml` |
| macOS | `~/.config/openapi-cli/config.yaml` |
| Windows | `%APPDATA%\openapi-cli\config.yaml` |

**XDG Compliance**: Respects `XDG_CONFIG_HOME` environment variable

### Configuration Files

```
~/.config/openapi-cli/
├── config.yaml              # Main configuration
├── profiles.yaml            # Named profiles
├── aliases.yaml             # Inventory aliases
└── keys/                    # Key storage
    ├── client.crt
    └── client.key
```

### System Configuration (Daemon)

| Platform | Location |
|----------|----------|
| Linux | `/etc/openapi-cli/config.yaml` |
| macOS | `/etc/openapi-cli/config.yaml` |
| Windows | `C:\ProgramData\openapi-cli\config.yaml` |

---

## Cache Directory Locations

### User Cache

| Platform | Location |
|----------|----------|
| Linux | `~/.cache/openapi-cli/` |
| macOS | `~/Library/Caches/openapi-cli/` |
| Windows | `%LOCALAPPDATA%\openapi-cli\cache\` |

**XDG Compliance**: Respects `XDG_CACHE_HOME` environment variable

### Cache Structure

```
~/.cache/openapi-cli/
├── inventories/             # Cached inventory JSON
│   ├── api.example.com/
│   │   ├── latest.json
│   │   └── v1.2.3.json
│   └── github.com/
│
├── modules/                 # Cached WASM modules
│   ├── abc123.wasm
│   └── def456.wasm
│
├── specs/                   # Cached OpenAPI specs
│   └── sha256/
│       └── abc123.yaml
│
└── db/                      # SQLite cache database
    └── cache.db
```

### Data Directory (State)

| Platform | Location |
|----------|----------|
| Linux | `~/.local/share/openapi-cli/` |
| macOS | `~/Library/Application Support/openapi-cli/` |
| Windows | `%LOCALAPPDATA%\openapi-cli\data\` |

**XDG Compliance**: Respects `XDG_DATA_HOME` environment variable

```
~/.local/share/openapi-cli/
├── known-good/              # User-marked good versions
│   └── api.example.com.json
│
├── audit/                   # Audit logs
│   └── 2024-01-01.log
│
└── state.json               # Daemon state
```

---

## Platform-Specific Code Locations

### Build Tags

```go
// internal/platform/unix/socket_unix.go
//go:build unix
// +build unix

package platform

func CreateUnixSocket(path string) error {
    // Unix implementation
}
```

```go
// internal/platform/windows/socket_windows.go
//go:build windows
// +build windows

package platform

func CreateNamedPipe(name string) error {
    // Windows implementation
}
```

### Platform Files

| Feature | Unix | Windows | Darwin |
|---------|------|---------|--------|
| Socket | `socket_unix.go` | `socket_windows.go` | Uses Unix |
| Keyring | `keyring_unix.go` | `keyring_windows.go` | `keyring_darwin.go` |
| Paths | `paths_unix.go` | `paths_windows.go` | Uses Unix |

---

## Testing Structure

### Test File Naming

```
internal/core/inventory/manager.go      # Implementation
internal/core/inventory/manager_test.go # Unit tests
test/unit/core/inventory_test.go        # Additional unit tests
test/integration/api_test.go            # Integration tests
test/e2e/scenarios/basic_usage_test.go # E2E tests
```

### Test Organization

```
test/
├── unit/                    # Fast, isolated unit tests
│   ├── core/
│   ├── cli/
│   └── mocks/
│
├── integration/             # Component integration
│   ├── api_test.go
│   ├── mcp_test.go
│   └── database_test.go
│
├── e2e/                     # End-to-end scenarios
│   └── scenarios/
│       ├── basic_usage_test.go
│       ├── cache_test.go
│       └── auth_test.go
│
├── fixtures/                # Test data
│   ├── openapi/
│   ├── inventories/
│   └── wasm/
│
├── helpers/                 # Test utilities
│   ├── mock/
│   ├── server.go
│   └── fixtures.go
│
└── benchmark/               # Performance tests
    └── performance_test.go
```

---

## Generated Code Locations

### sqlc Generated

```
internal/db/sqlite/
├── db.go              # Generated: database interface
├── models.go          # Generated: struct definitions
└── queries.sql.go     # Generated: query functions
```

### Mocks

```
test/mocks/
├── mock_http_client.go    # Generated by mockery
├── mock_store.go
└── mock_inventory.go
```

### Protocol Buffers

```
internal/db/pb/            # Generated from api/proto/
├── inventory.pb.go
├── inventory_grpc.pb.go
└── daemon.pb.go
```

---

## One-Command Directory Creation

For Ralph Loop - create entire directory structure:

```bash
#!/bin/bash
# scripts/init-project.sh

set -e

PROJECT_ROOT="${1:-openapi-cli}"

echo "Creating openapi-cli project structure at: $PROJECT_ROOT"

mkdir -p "$PROJECT_ROOT"
cd "$PROJECT_ROOT"

# Root level directories
mkdir -p {cmd/{cli,mcp,daemon},internal/{cli,core/{inventory,config,auth,crypto,http,dsl,wasm,errors,logging,metrics,version},mcp,daemon,db/{sqlite,postgres,mysql,migrate}},pkg/api/v1,api/proto/v1,server/{cmd/server,internal/{api,db,storage,auth,webhook,admin}},migrations,queries,test/{unit/{core,cli,mcp,daemon,mocks},integration,e2e/scenarios,fixtures/{openapi,inventories,wasm},helpers/{mock},benchmark},configs/systemd,deployments/{kubernetes,helm/openapi-cli},docs/{api,architecture},examples/{workflows,configs,integrations},scripts,build/{scripts,wasm/modules,docker},tools/{codegen,mockgen},web/{static,templates},bin,dist,third_party,.github/workflows}

# Create initial files
touch \
    cmd/openapi-cli/main.go \
    cmd/mcp/main.go \
    cmd/daemon/main.go \
    go.mod \
    go.sum \
    Makefile \
    Dockerfile \
    docker-compose.yml \
    sqlc.yaml \
    LICENSE \
    README.md \
    .gitignore

echo "Project structure created successfully!"
echo ""
echo "Next steps:"
echo "  1. cd $PROJECT_ROOT"
echo "  2. go mod init github.com/openapi-cli/core"
echo "  3. make setup"
echo "  4. make build"
```

Run with:
```bash
chmod +x scripts/init-project.sh
./scripts/init-project.sh /path/to/openapi-cli
```

---

## Key File Locations Summary

| Type | Path |
|------|------|
| **Main Entry** | `cmd/openapi-cli/main.go` |
| **MCP Entry** | `cmd/mcp/main.go` |
| **Daemon Entry** | `cmd/daemon/main.go` |
| **Core Interfaces** | `internal/core/*/manager.go` |
| **CLI Commands** | `internal/cli/commands/*.go` |
| **Database Interface** | `internal/db/store.go` |
| **Migrations** | `migrations/*.sql` |
| **SQL Queries** | `queries/*.sql` |
| **Config Template** | `configs/openapi-cli.yaml` |
| **Build Script** | `Makefile` |
| **Dockerfile** | `Dockerfile` |
| **CI/CD** | `.github/workflows/*.yaml` |
| **Tests** | `test/{unit,integration,e2e}/**/*_test.go` |
| **Fixtures** | `test/fixtures/` |

---

**Version**: 1.0  
**Last Updated**: 2024-01-15  
**Author**: Ralph Loop Reference
