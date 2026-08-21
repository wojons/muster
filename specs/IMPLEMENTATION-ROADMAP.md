# Implementation Roadmap

**Status**: ✅ Specifications Complete → 🚧 Ready for Implementation  
**Total Duration**: ~23 weeks (can parallelize)  
**Language**: Go (golang)  

## 📋 Quick Stats

- **Specifications**: 24 files, ~6,000 lines
- **Epics**: 7 (118 story points)
- **Implementation Phases**: 8
- **Team**: Recommended 2-3 developers

## 🎯 Architecture Overview

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                     ENTRY POINTS                            │
├──────────────┬──────────────┬───────────────────────────────┤
│   CLI Mode   │  MCP stdio   │  MCP Daemon (Unix/HTTP)       │
│              │              │                               │
│ Human use    │ AI agents    │ Multi-client, shared state    │
└──────┬───────┴──────┬───────┴───────────┬───────────────────┘
       │              │                   │
       └──────────────┼───────────────────┘
                      │
          ┌───────────▼───────────┐
          │      SHARED CORE      │
          │  • Inventory Manager  │
          │  • Config Hub         │
          │  • Security Layer     │
          │  • Execution Engine   │
          │  • Starlark DSL       │
          │  • Wasmtime Runtime   │
          └───────────┬───────────┘
                      │
          ┌───────────┼───────────┐
          ▼           ▼           ▼
    ┌──────────┐ ┌──────────┐ ┌──────────┐
    │ Inventory│ │   API    │ │   CDN    │
    │  Server  │ │  Server  │ │ (S3/R2)  │
    └──────────┘ └──────────┘ └──────────┘
```

### Three Deployment Modes

1. **Standalone CLI** (default)
   - Direct command execution
   - Auto-generated from OpenAPI
   - Single user, single process

2. **MCP stdio** (AI integration)
   - Claude Desktop, Cursor, etc.
   - Spawns process per AI session
   - Standard MCP protocol

3. **Daemon Mode** (enterprise)
   - Unix socket or HTTP
   - Multiple clients, one process
   - Shared rate limiting, WASM cache
   - Single TLS connection per user

## 📊 Implementation Phases

### Phase 1: Foundation (Weeks 1-3)
**Goal**: Basic inventory system and HTTP client

**Key Deliverables**:
- [ ] Go project structure
- [ ] HTTP client with auth, retry, rate limiting
- [ ] Inventory fetch and caching
- [ ] Version management (latest, last-known-good)

**Documents**:
- epic-01-inventory-system.md
- story-01-01-https-fetching.md
- story-01-04-version-management.md

**Dependencies**:
- `github.com/go-resty/resty/v2`
- `github.com/spf13/cobra`
- `github.com/spf13/viper`

**Acceptance Criteria**:
```bash
# Can fetch inventory
openapi-cli --host https://api.example.com --help

# Uses cache on second run
openapi-cli --host https://api.example.com users list

# Works offline with cached version
# (disconnect internet)
openapi-cli --host https://api.example.com users list
```

---

### Phase 2: Interface Generation (Weeks 4-6)
**Goal**: CLI and MCP from OpenAPI

**Key Deliverables**:
- [ ] OpenAPI parser (kin-openapi)
- [ ] CLI command generation (cobra)
- [ ] MCP stdio server (mcp-go-sdk)
- [ ] Tool registration from OpenAPI

**Documents**:
- epic-02-dual-interface.md
- story-02-01-path-mapping.md
- story-02-08-mcp-setup.md

**Dependencies**:
- `github.com/getkin/kin-openapi`
- `github.com/mcp-sdk/mcp-go`

**Acceptance Criteria**:
```bash
# CLI generates commands
openapi-cli --host https://api.example.com users list --limit 10

# MCP server starts
openapi-cli --mcp --host https://api.example.com
# (responds to MCP protocol on stdin/stdout)

# Tools registered match OpenAPI
# ListTools returns correct schema
```

---

### Phase 3: Security (Weeks 7-9)
**Goal**: Production-ready auth and signing

**Key Deliverables**:
- [ ] OS keychain integration (99designs/keyring)
- [ ] Multiple auth methods (OAuth2, JWT, mTLS)
- [ ] Inventory signing verification (GPG, SSH, TLS)
- [ ] Client fingerprinting

**Documents**:
- epic-07-security-layer.md
- TECHNICAL-DECISIONS.md (signing section)

**Dependencies**:
- `github.com/99designs/keyring`
- `golang.org/x/oauth2`
- `github.com/golang-jwt/jwt/v5`
- `golang.org/x/crypto`

**Acceptance Criteria**:
```bash
# Uses OS keychain
openapi-cli secret set api-token "my-token"

# mTLS connection
openapi-cli --host https://api.example.com \
  --cert ./client.crt --key ./client.key

# Verifies inventory signature
# (configured trust settings)
```

---

### Phase 4: Scripting (Weeks 10-12)
**Goal**: Starlark DSL for workflows

**Key Deliverables**:
- [ ] Starlark integration (go.starlark.net)
- [ ] HTTP builtins (http.get, http.post, etc.)
- [ ] JSON handling (encode, decode, path)
- [ ] Resource limits (CPU, memory, steps)

**Documents**:
- epic-06-workflows.md
- TECHNICAL-DECISIONS.md (DSL section)

**Dependencies**:
- `go.starlark.net`

**Acceptance Criteria**:
```python
# Example workflow script
items = http.get("/items").json()

results = []
for item in items:
    r = http.post("/process", body={"id": item["id"]})
    results.append(r.json())

return {"processed": len(results)}
```

---

### Phase 5: WASM (Weeks 13-15)
**Goal**: Sandboxed WASM execution

**Key Deliverables**:
- [ ] Wasmtime integration (wasmtime-go)
- [ ] Capability model (explicit grants)
- [ ] Module loading (download, verify, cache)
- [ ] Host function registration

**Documents**:
- epic-04-module-registry.md
- story-04-01-registry-client.md

**Dependencies**:
- `github.com/bytecodealliance/wasmtime-go`

**Acceptance Criteria**:
```go
// WASM module with explicit capabilities
// No filesystem access (unless granted)
// No network (only through host functions)
// Memory and CPU limits enforced
```

---

### Phase 6: Enterprise Features (Weeks 16-18)
**Goal**: Daemon mode and connection management

**Key Deliverables**:
- [ ] Daemon mode (Unix socket, HTTP)
- [ ] Connection multiplexing
- [ ] Shared state (WASM cache, rate limits)
- [ ] Audit logging

**Architecture**:
```
Agent A ──┐
Agent B ──┼──► unix socket ──► daemon ──► API
Agent C ──┘   (one process)
```

**Benefits**:
- One WASM compilation per module
- Shared rate limit budget
- Single TLS connection per user
- Centralized audit logging

---

### Phase 7: Reference Server (Weeks 19-21)
**Goal**: Bundled server for self-hosting

**Key Deliverables**:
- [ ] Go server with database/sql
- [ ] Multi-driver (SQLite, MySQL, PostgreSQL)
- [ ] S3/CloudFront adapter
- [ ] Webhook proxy
- [ ] Credential provisioning

**Documents**:
- TECHNICAL-DECISIONS.md (server section)

**Dependencies**:
- `github.com/sqlc-dev/sqlc`
- `github.com/golang-migrate/migrate/v4`
- `github.com/lib/pq` (PostgreSQL)
- `github.com/go-sql-driver/mysql` (MySQL)
- `github.com/mattn/go-sqlite3` (SQLite)

**Deployment Options**:
1. SQLite (single binary, zero config)
2. MySQL/PostgreSQL (production)
3. S3/CloudFront (CDN, static files)

---

### Phase 8: Distribution (Weeks 22-23)
**Goal**: Release and distribution

**Key Deliverables**:
- [ ] Self-update (go-selfupdate)
- [ ] Bootstrap installer (shell/powershell)
- [ ] Documentation
- [ ] CI/CD pipeline
- [ ] Release automation

**Distribution Channels**:
1. **GitHub Releases** (binaries)
2. **Bootstrap Installer**:
   ```bash
   curl -fsSL https://install.openapi-cli.io | sh
   ```
3. **Package Managers** (Homebrew, etc.)
4. **Enterprise**: Pre-shipped binary

**Acceptance Criteria**:
```bash
# Install
curl -fsSL https://install.openapi-cli.io | sh

# Auto-update
openapi-cli update

# Version check
openapi-cli --version
```

## 🏗️ Project Structure

```
openapi-cli/
├── cmd/
│   ├── cli/                    # CLI entry point
│   │   └── main.go
│   ├── mcp/                    # MCP server entry
│   │   └── main.go
│   └── daemon/                 # Daemon entry
│       └── main.go
│
├── internal/
│   ├── core/                   # Shared core
│   │   ├── inventory/         # Inventory management
│   │   ├── config/            # Configuration
│   │   ├── auth/              # Authentication
│   │   ├── crypto/            # Cryptographic ops
│   │   ├── http/              # HTTP client
│   │   ├── dsl/               # Starlark integration
│   │   └── wasm/              # Wasmtime runtime
│   │
│   ├── cli/                    # CLI-specific
│   │   ├── commands/          # Generated commands
│   │   └── parser.go
│   │
│   ├── mcp/                    # MCP-specific
│   │   ├── server.go          # MCP server
│   │   └── tools.go           # Tool generation
│   │
│   └── daemon/                 # Daemon-specific
│       ├── server.go          # Daemon server
│       ├── socket.go          # Unix socket
│       └── proxy.go           # Connection proxy
│
├── pkg/                        # Public API (optional)
│   └── api/
│
├── server/                     # Reference server
│   ├── cmd/server/
│   ├── internal/
│   │   ├── api/               # HTTP API
│   │   ├── db/                # Database layer
│   │   ├── storage/           # S3 adapter
│   │   └── auth/              # Auth server
│   └── migrations/            # SQL migrations
│
├── docs/                       # Documentation
├── scripts/                    # Build scripts
├── Makefile
├── go.mod
└── README.md
```

## 📦 Go Dependencies

### Core
```go
github.com/spf13/cobra          // CLI framework
github.com/spf13/viper          // Config management
github.com/mcp-sdk/mcp-go       // MCP SDK
github.com/bytecodealliance/wasmtime-go  // WASM runtime
go.starlark.net                 // Starlark DSL
github.com/go-resty/resty/v2    // HTTP client
github.com/99designs/keyring    // OS keychain
github.com/knadh/koanf          // Config loading
```

### Database
```go
github.com/sqlc-dev/sqlc        // Code generation
github.com/golang-migrate/migrate/v4  // Migrations
github.com/lib/pq               // PostgreSQL
github.com/go-sql-driver/mysql  // MySQL
github.com/mattn/go-sqlite3     // SQLite
```

### Auth & Crypto
```go
golang.org/x/oauth2             // OAuth2
github.com/golang-jwt/jwt/v5    // JWT
golang.org/x/crypto             // GPG, SSH
```

### Utilities
```go
github.com/hashicorp/go-version // Version parsing
github.com/creativeprojects/go-selfupdate  // Self-update
github.com/charmbracelet/log    // Logging
github.com/charmbracelet/spinner // UI
```

## ✅ Definition of Done (Per Phase)

Each phase is "done" when:

1. **All code implemented** with tests
2. **Documentation updated**
3. **Acceptance criteria met**
4. **Both CLI and MCP tested**
5. **Code review completed**
6. **CI/CD passing**

## 🎯 Success Metrics

- **Build time**: < 30 seconds
- **Binary size**: < 50MB (with WASM runtime)
- **Startup time**: < 500ms (cached), < 3s (fresh)
- **Test coverage**: > 80%
- **Memory usage**: < 256MB typical

## 🚀 Getting Started

### Prerequisites
- Go 1.21+
- Make
- Docker (optional, for testing)

### Development Setup

```bash
# Clone repo
git clone https://github.com/openapi-cli/core.git
cd core

# Install dependencies
make deps

# Run tests
make test

# Build binary
make build

# Run locally
./bin/openapi-cli --host https://api.example.com --help

# Run MCP mode
./bin/openapi-cli --mcp --host https://api.example.com
```

### First Task (Good for Onboarding)

Implement `inventory fetch` command:

```go
// cmd/openapi-cli/commands/inventory.go
package commands

import (
    "github.com/spf13/cobra"
    "github.com/openapi-cli/core/internal/core/inventory"
)

var inventoryCmd = &cobra.Command{
    Use:   "inventory",
    Short: "Manage inventory",
}

var fetchCmd = &cobra.Command{
    Use:   "fetch [host]",
    Short: "Fetch inventory from host",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        host := args[0]
        
        inv, err := inventory.Fetch(cmd.Context(), host)
        if err != nil {
            return err
        }
        
        cmd.Printf("Fetched inventory version %s\n", inv.Version)
        return nil
    },
}

func init() {
    inventoryCmd.AddCommand(fetchCmd)
}
```

## 📞 Next Steps

1. **Read**: Start with EXECUTIVE-SUMMARY.md
2. **Setup**: Follow QUICK-START.md for dev environment
3. **Decisions**: Review TECHNICAL-DECISIONS.md
4. **Implement**: Start Phase 1 (epic-01-inventory-system.md)
5. **Iterate**: Test both CLI and MCP after each phase

## 📚 Key Documents

| Document | Purpose |
|----------|---------|
| INDEX.md | Navigation guide |
| EXECUTIVE-SUMMARY.md | One-page overview |
| TECHNICAL-DECISIONS.md | Why Go, Wasmtime, Starlark |
| architecture-v2.md | Complete technical design |
| PRD.md | Product requirements |
| QUICK-START.md | Developer setup |

---

**Ready to start implementing!** 🚀

Start with Phase 1: Epic 1 (Inventory System)
