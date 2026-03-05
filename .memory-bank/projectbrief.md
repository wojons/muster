# Project Brief: OpenAPI CLI + MCP Server

## Project Overview

**Name**: OpenAPI CLI + MCP Server  
**Type**: Developer Tool / CLI Application  
**Language**: Go  
**Status**: Specifications Complete, Ready for Implementation  
**Goal**: Generate 100% production-ready code from Bible-standard specifications

## Core Purpose

A unified tool that dominates both CLI and MCP markets by providing:
- Dynamic command generation from OpenAPI specifications
- WASM module execution for custom processing
- Starlark DSL for simple workflows
- Cross-platform authentication with OS keychain integration
- Three deployment modes: CLI, MCP stdio server, Unix socket daemon

## Target Users

- Developers building API clients
- DevOps engineers managing API infrastructure
- AI assistants (Claude, GPT) needing structured API access
- Teams wanting consistent API tooling across platforms

## Key Features

1. **Dynamic CLI Generation** - Automatically generates CLI commands from any OpenAPI spec
2. **MCP Server Mode** - Serves commands via Model Context Protocol to Claude Desktop
3. **WASM Plugin System** - Supports custom logic via sandboxed WASM modules
4. **Multi-Auth Support** - OAuth2, API Key, mTLS, Basic auth
5. **Cross-Platform** - Works identically on Linux, macOS, Windows
6. **Self-Updating** - Fetches inventory updates automatically

## Success Criteria

| Metric | Target |
|--------|--------|
| CLI Commands Generated | 100% from OpenAPI |
| MCP Tool Coverage | 100% of CLI commands |
| Platform Support | Linux, macOS, Windows |
| Performance | <100ms command generation |
| Code Coverage | >80% |
| Determinism | Cross-language identical behavior |

## Project Structure

```
├── cmd/
│   ├── openapi-cli/       # Main CLI entry
│   └── mcp-server/        # MCP server entry
├── pkg/
│   ├── wasm/              # WASM runtime (Wasmtime 8.0.0)
│   ├── mcp/               # MCP protocol implementation
│   ├── auth/              # OAuth2, mTLS, keychain
│   ├── dsl/               # Starlark interpreter
│   ├── cli/               # CLI framework (Cobra)
│   ├── cache/             # Multi-tier caching
│   ├── config/            # Configuration management
│   └── openapi/           # OpenAPI parsing
├── specs/                 # 98 specification files
├── memory-bank/           # Project documentation (this)
└── .ralph/               # Ralph Wiggum Loop state
```

## Bible Standard Compliance

This project follows Bible Standard specifications which require:
- **Deterministic output** - Identical code generation across AI systems
- **Zero manual work** - 100% automated from specs
- **Complete specifications** - No ambiguity, all edge cases covered
- **Autonomous validation** - Self-testing and verification

## Current Status

- ✅ 98 specification files completed
- ✅ Ralph Wiggum Loop configured
- 🔄 Ready for autonomous implementation

## Next Milestone

Begin Phase 1 implementation: Project setup and core infrastructure
