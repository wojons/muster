# Muster

Turn any OpenAPI spec into a CLI, an MCP server, and a Go library. Instantly.

**Language:** Go 1.25+
**Binary:** `openapi-cli` (CLI, MCP server, daemon)
**Tests:** 44+ packages, `test_mode: full`

## Quick Commands

```bash
make build          # Build all binaries (CLI + MCP server + daemon)
make test-unit      # Run unit tests
make demo           # Run full integration demo (5 suites)
make vet            # Run go vet
make lint           # Run golangci-lint
make validate       # Full system validation
gitreins guard      # Pre-commit quality gates
```

## Project Structure

```
cmd/                  # Entry points (openapi-cli, mcp-server, daemon)
pkg/                  # Public libraries (26+ packages)
internal/             # Internal utilities
demo/                 # Demo application + test suites
specs/                # 280 specification files
```

## Build

```bash
go build ./cmd/openapi-cli/    # CLI + daemon
go build ./cmd/mcp-server/     # MCP server
go build ./cmd/daemon/         # Standalone daemon
```

Requires CGO for SQLite support. Cross-compiles for Linux, macOS, Windows.

## Testing

```bash
go test ./... -count=1                      # Full test suite
go test -short ./... -count=1               # Skip long-running tests
make test-unit                              # Unit tests (excludes stability)
make test-integration                       # Integration tests
make demo                                   # End-to-end demo
```

## Linting

```bash
golangci-lint run ./...                     # Full lint
make lint                                   # Via Makefile
```

Pre-existing: fieldalignment warning on `exploits/ratelimit/main.go` (demo exploit, not production).

## CI Status

Billing-blocked as of 2026-07-11. All workflows fail with "recent account payments have failed." Needs human with billing access to `wojons` org.
