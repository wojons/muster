# Changelog

## [Unreleased]

### Fixed
- **Build**: Docker release images are now stamped with the real version,
  commit and build date. The Dockerfile's single-quoted `-ldflags` never let
  the shell expand `${VERSION}`, so images reported the literal token
  `${VERSION} (unknown)` (or ` ()` when the build args were unset);
  `docker-build.yml` also never passed build args at all. Empty stamps are
  additionally normalized in `internal/version.Get`, so `version` output can
  never be blank. Bare `go build` binaries still report `dev (unknown)` by
  design; `make` builds stamp via the Makefile as before. Also re-included
  `pkg/auth/credential.go` and `pkg/auth/credential_meta.go` in the Docker
  build context — `.dockerignore`'s `**/*credential*` rule had silently
  stripped them, breaking Docker builds at HEAD.

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
