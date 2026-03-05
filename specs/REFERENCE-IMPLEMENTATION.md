# Reference Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Reference

## Overview

Complete specification of **reference implementation** for the OpenAPI CLI tool.

## 1. Project Structure

```
cmd/
  openapi-cli/       # Main CLI entry point
  mcp-server/        # MCP server entry point

pkg/
  wasm/              # WASM runtime
  mcp/               # MCP protocol
  auth/              # Authentication
  dsl/               # Starlark DSL
  cli/               # CLI framework
  cache/             # Caching
  config/            # Configuration
  openapi/           # OpenAPI parsing

internal/
  tests/             # Internal tests
```

## 2. Key Implementation Files

| Component | File | Lines |
|-----------|------|-------|
| WASM Runtime | pkg/wasm/runtime.go | ~500 |
| MCP Server | pkg/mcp/server.go | ~400 |
| Command Generator | pkg/cli/generator.go | ~600 |
| OAuth2 | pkg/auth/oauth2.go | ~350 |

## 3. Build Configuration

```yaml
# .goreleaser.yml
builds:
  - id: openapi-cli
    main: ./cmd/openapi-cli
    binary: openapi-cli
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
```

## 4. Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./...
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
