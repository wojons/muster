# Changelog

## [Unreleased]

## [v0.1.0] - 2026-08-07

### Added
- **CLI**: Dynamic command discovery from OpenAPI specs (`discover`, `request`, `generate`)
- **MCP Server**: Full MCP protocol — `tools/list`, `tools/call`, stdio transport
- **Daemon**: Unix socket proxy with lifecycle management
- **WASM plugins**: Sandboxed response transformation via wazero runtime
- **Starlark DSL**: Multi-step API workflows with `http_get/post/put/delete` builtins
- **Go library**: 26 public packages under `pkg/` — importable by other Go programs
- **Demo**: 5 mock services, 10 self-verifying demo scripts, one-command runner
- **Auth**: Bearer tokens, OAuth2, mTLS
- **Caching**: Multi-tier with memory and TTL
- **Rate limiting**: Token bucket with configurable limits
- **Cross-platform**: Linux, macOS, Windows

### Changed
- Forked from anomalyco/muster — renamed module to github.com/wojons/muster
- Switched WASM runtime from wasmtime to wazero (pure Go, no CGO)
- Renamed binaries: muster-cli → openapi-cli, muster-mcp → openapi-mcp, muster-daemon → openapi-daemon

### Removed
- Enterprise SaaS features (multi-tenancy, CRDT, 12-language SDK ecosystem)
- Ralph Wiggum Loop infrastructure
- Axiom pipeline traces
