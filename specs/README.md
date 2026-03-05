# Dynamic OpenAPI CLI & MCP Server - Specifications

This directory contains the BMAD Method specifications for building a dynamic CLI and MCP server that configures itself from a remote inventory.

## Project Overview

### Dual-Tool Architecture
A unified system providing **two interfaces** built on shared core functionality:

1. **CLI Tool** - For direct command-line usage, scripting, and automation
2. **MCP Server** - For AI assistant integration (Claude, Cursor, Copilot, etc.)

Both tools configure themselves identically from a remote inventory endpoint.

### Core Concept

Instead of installing and updating SDKs or MCP packages:
```bash
# CLI usage
openapi-cli --host https://api.example.com users list

# MCP usage (via npx)
npx openapi-mcp --host https://api.example.com
```

The tool:
1. Downloads inventory from `https://<host>/_inventory.json`
2. Caches multiple versions with "last known good" support
3. Generates CLI commands or MCP tools from OpenAPI spec
4. Supports WASM modules for advanced processing
5. Includes sandboxed DSL for simple workflows
6. Manages multiple auth methods (OAuth, JWT, mTLS, basic)
7. Integrates with OS keychain for secrets
8. Provides client-side rate limiting and retry logic

## Key Features

### Inventory System
- HTTPS-only remote configuration
- Dynamic inventory (different per user/role)
- Version management with caching
- "Last known good" fallback
- Offline mode support
- HTTP caching headers (CDN-compatible)

### Security
- mTLS support with client certificates
- OS keychain integration (macOS, Windows, Linux)
- Client fingerprinting (optional, configurable)
- Encrypted file storage
- Multiple simultaneous auth contexts
- Certificate pinning

### Advanced Execution
- WASM modules for custom processing
- Sandboxed DSL for workflows
- Client-side rate limiting
- Retry logic with exponential backoff
- SOCKS5 proxy support

## Documents

### Quick Reference
- **[EXECUTIVE-SUMMARY.md](EXECUTIVE-SUMMARY.md)** - One-page overview of the product
- **[QUICK-START.md](QUICK-START.md)** - Developer and user quick start guide
- **[ARCHITECTURE-VISUAL.md](ARCHITECTURE-VISUAL.md)** - Visual diagrams and data flows
- **[TECHNICAL-DECISIONS.md](TECHNICAL-DECISIONS.md)** - Detailed technical decisions and rationale

### Planning Artifacts
- **[PRD.md](planning-artifacts/PRD.md)** - Product Requirements (300+ lines)
- **[architecture.md](planning-artifacts/architecture.md)** - Technical Architecture (550+ lines)
- **[architecture-v2.md](planning-artifacts/architecture-v2.md)** - Updated architecture with technical decisions (600+ lines)

### Epics
1. **[Epic 1: Inventory System](planning-artifacts/epics/epic-01-inventory-system.md)** - Remote config management
2. **[Epic 2: Dual Interface](planning-artifacts/epics/epic-02-dual-interface.md)** - CLI + MCP implementation
3. **[Epic 3: Request Execution](planning-artifacts/epics/epic-03-request-execution.md)** - HTTP client and middleware
4. **[Epic 4: Module Registry](planning-artifacts/epics/epic-04-module-registry.md)** - WASM module system
5. **[Epic 5: Configuration](planning-artifacts/epics/epic-05-configuration.md)** - Config management
6. **[Epic 6: Workflows](planning-artifacts/epics/epic-06-workflows.md)** - Multi-step workflows
7. **[Epic 7: Security Layer](planning-artifacts/epics/epic-07-security-layer.md)** - Auth, secrets, crypto

### Stories (Sample Implementation)
See `planning-artifacts/stories/` for detailed stories including:
- HTTPS fetching with TLS/mTLS
- Version management
- MCP server setup
- And more...

## Architecture Highlights

### Shared Core Components
```
┌─────────────────────────────────────────────┐
│           SHARED CORE                       │
│  ┌───────────────────────────────────────┐ │
│  │ Inventory Manager                     │ │
│  │  • Fetch & cache from HTTPS          │ │
│  │  • Version control                    │ │
│  │  • Offline fallback                   │ │
│  └───────────────────────────────────────┘ │
│  ┌───────────────────────────────────────┐ │
│  │ Configuration Hub                     │ │
│  │  • CLI flags → Env vars → Files       │ │
│  │  • Secrets (keychain, encrypted)      │ │
│  │  • Profile switching                   │ │
│  └───────────────────────────────────────┘ │
│  ┌───────────────────────────────────────┐ │
│  │ Execution Engine                      │ │
│  │  • HTTP client with middleware        │ │
│  │  • WASM runtime (sandboxed)          │ │
│  │  • DSL interpreter                    │ │
│  └───────────────────────────────────────┘ │
│  ┌───────────────────────────────────────┐ │
│  │ Security Layer                        │ │
│  │  • Auth handlers (OAuth, JWT, mTLS)   │ │
│  │  • Client fingerprinting              │ │
│  │  • Crypto operations                  │ │
│  └───────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### Configuration Hierarchy
1. CLI flags / MCP init params
2. Environment variables (`OPENAPICLI_*`)
3. Config file (`~/.config/openapi-cli/config.yaml`)
4. Inventory-provided config
5. Built-in defaults

### Inventory Schema
```json
{
  "version": "1.2.3",
  "min_cli_version": "0.5.0",
  "max_cli_version": "1.x",
  "openapi": {
    "source": "url",
    "content": "https://api.example.com/openapi.json"
  },
  "modules": {
    "processors": [
      {"name": "transform", "url": "...", "sha256": "..."}
    ]
  },
  "config": {
    "rate_limiting": {"requests_per_minute": 100},
    "fingerprinting": {"enabled": true}
  },
  "auth": {
    "inventory_auth": {"type": "bearer"},
    "api_auth": {"type": "oauth2"}
  }
}
```

## Technology Stack (Confirmed)

- **Language**: Go (golang)
- **Compilation**: Single static binary, cross-platform
- **Distribution**: Bootstrap installer + self-update
- **CLI Framework**: Cobra
- **MCP SDK**: mcp-go-sdk
- **WASM Runtime**: Wasmtime (wasmtime-go) - explicit capabilities
- **DSL**: Starlark (go.starlark.net) - sandboxed
- **HTTP Client**: resty/v2
- **Secrets**: 99designs/keyring (OS keychain)
- **Database**: sqlc + golang-migrate (SQLite → MySQL → PostgreSQL)
- **Storage**: Pluggable (filesystem/S3)
- **Auth**: oauth2, jwt, mTLS
- **Signing**: GPG, SSH, TLS pinning

**Why Go?**
- Single static binary (no runtime dependencies)
- Fast compilation (LLM-friendly)
- Excellent concurrency (multiplexing/proxy work)
- Cross-compile trivially
- Fast startup (critical for CLI)

**Distribution:**
- Bootstrap installer (like mise/volta/deno)
- Self-update via go-selfupdate
- Pre-ship binary for enterprise (no installer needed)

## BMAD Method

These specifications follow the [BMAD Method](https://bmad-method.org/) for AI-driven development.

### Phases
1. **Analysis** ✅ - Problem identified, solution proposed
2. **Planning** ✅ - PRD complete with requirements
3. **Solutioning** ✅ - Architecture and epics defined
4. **Implementation** ⏳ - Ready for development

## Usage for AI Agents

When implementing this project:
1. **Start here**: Read [EXECUTIVE-SUMMARY.md](EXECUTIVE-SUMMARY.md) for high-level context
2. **Understand flows**: Review [ARCHITECTURE-VISUAL.md](ARCHITECTURE-VISUAL.md) for visual diagrams
3. **Deep dive**: Read [PRD.md](planning-artifacts/PRD.md) for detailed requirements
4. **Technical design**: Study [architecture.md](planning-artifacts/architecture.md)
5. **Implement**: Start with Epic 1 (Inventory System), work through stories
6. **Test**: Verify both CLI and MCP interfaces work correctly

## Distribution

- **npm**: `npm install -g @openapi-cli/core`
- **npx**: `npx @openapi-cli/core --host ...`
- **Binary**: Native binaries via GitHub releases
- **Docker**: Container image available

## Key Decisions

1. **Runtime**: Bun (compiles to single binary, includes runtime)
2. **HTTPS mandatory** for inventory (security)
3. **Dual interface** (CLI + MCP) sharing core
4. **WASM runtime**: Wasmtime (production-ready, WASI support)
5. **DSL**: Starlark (sandboxed, Python-like, extensible)
6. **OS keychain** for secrets (not env vars)
7. **Client fingerprinting** optional and configurable
8. **Offline-first** with smart caching
9. **Version pinning** supported for stability
10. **Flexible permissions**: Server-defined + user-granted capabilities
11. **Three MCP modes**: stdio (default), HTTP server, Unix socket proxy
12. **Bundled server**: SQLite → MySQL/PostgreSQL + S3/CloudFront support

## Timeline

- **Phase 1**: Core (Inventory, basic CLI/MCP, HTTP) - 4-6 weeks
- **Phase 2**: Security (mTLS, OAuth, secrets, fingerprinting) - 3-4 weeks
- **Phase 3**: Advanced (WASM, DSL, rate limiting) - 4-6 weeks
- **Phase 4**: Polish (performance, docs, distribution) - 2-3 weeks

## Contributors

- **Lexi** - Product vision, architecture
- **Dimitri** - Architecture input
- **Ahanaf** - Technical input

---

**Status**: Specifications complete, ready for implementation  
**Last Updated**: 2026-02-28
