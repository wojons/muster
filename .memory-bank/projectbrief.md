# Muster - Project Brief

**Purpose**: Dynamic OpenAPI client & MCP CLI tool
**Type**: Go CLI application
**Language**: Go 1.25+
**Architecture**: Integrated system with cmd/, pkg/, internal/ packages

## Core Requirements
1. Parse OpenAPI specifications
2. Generate CLI commands automatically
3. MCP server integration
4. WASM plugin support
5. Daemon with Unix socket proxy

## Non-Requirements
- 12-language SDK ecosystem (enterprise feature)
- Multi-tenancy (enterprise SaaS)
- CRDT state sync (advanced feature)
- Linux-only runtime (must be cross-platform)

## Entry Points
- `cmd/openapi-cli` - Main CLI (55.6% coverage)
- `cmd/mcp-server` - MCP server (72.0% coverage)
- `cmd/daemon` - Background daemon (56.9% coverage)

## Testing Status (Fresh 2026-03-08)
- pkg/auth: Tests PASS
- pkg/dsl: Tests PASS
- pkg/mcp: Tests PASS
- cmd/*: All 3 binaries exist, tests running
- Security Posture: HIGH (verified fresh)
