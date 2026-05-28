# Muster — Project Brief

axiom:trace work_item=muster-bootstrap-01

**Purpose**: Dynamic OpenAPI client & MCP CLI tool that generates CLI commands from OpenAPI specs instantly.
**Type**: Go CLI application
**Language**: Go 1.25+
**Architecture**: Integrated system with `cmd/` (entry points), `pkg/` (public APIs, 26+ packages), `internal/` (private utilities)

## Goals

1. Parse any OpenAPI specification and generate typed CLI commands automatically
2. Provide an MCP server for AI agent integration
3. Support WASM plugin modules for extensibility
4. Background daemon with Unix socket proxy
5. Cross-platform: macOS, Linux, Windows (Linux security currently in progress)

## Non-Goals

- 12-language SDK ecosystem (enterprise feature, deferred)
- Multi-tenancy (enterprise SaaS, deferred)
- CRDT state sync (advanced feature, deferred)
- Linux-only runtime (must build cross-platform)

## Definitions

- **Done**: Code compiles, unit tests pass, integration tests pass, new feature is demonstrable via `make demo`, spec is updated with realized-by links, trace markers present near behavior boundaries.
- **Ready for review**: All of "Done" plus PR template filled, CI green, memory bank updated.
- **Blocked**: Required upstream dependency missing, permission/access gap, or unresolved spec ambiguity.

## Key Integrations

- Build: Makefile + Go 1.25+ + CGO_ENABLED=1 (SQLite)
- CI: GitHub Actions (`.github/`)
- Specs: 276 specification files in `specs/`
- Memory Bank: `.memory-bank/`
- Axiom config: `.axiom/`
- Agent config: `.opencode/`

## Repo Conventions

- Specs: `specs/`
- Memory Bank: `.memory-bank/`
- Axiom config: `.axiom/`
- Agent config: `.opencode/`
- Commits: conventional commits with trace footers
- Testing: unit + integration + E2E required per AGENTS.md

## Current State (from dogfooding validation 2026-03-22)

- Completeness level: FUNCTIONALIS (Level 2/5) — NOT production ready
- Critical gaps: Linux security (100% unimplemented), daemon tests (100% skipped), cross-platform keychain (66% broken)
- Binary builds: 3/3 compile
- MCP server protocol: functional
- OpenAPI parsing: 85%+ spec coverage
