# Tech Context

## Technology Stack

### Core Language
- **Go** 1.21+
  - Reason: Single binary, fast startup, excellent concurrency
  - Compiler: Official Go toolchain
  - Modules: Enabled, proxy: https://proxy.golang.org

### Build Tools
- **Make** - Build orchestration
- **GoReleaser** - Multi-platform releases
- **Docker** - Container builds
- **golangci-lint** - Code quality

### CLI Framework
- **Cobra** 1.8+
  - Command tree structure
  - Flag parsing
  - Shell completion generation
  - Help text generation

### Configuration
- **Viper** 1.18+
  - Multi-format support (YAML, JSON, TOML)
  - Environment variable binding
  - Configuration watching
  - Default values

### WASM Runtime
- **Wasmtime** 8.0.0 (critical version)
  - All 12 language SDKs use this exact version
  - Deterministic execution
  - WASI support
  - Capability-based security

### DSL Engine
- **Starlark** (go.starlark.net)
  - Python-like syntax
  - Sandboxed execution
  - Deterministic evaluation
  - No recursion, no globals

### OpenAPI Parsing
- **kin-openapi** v0.125+
  - OpenAPI 3.0 support
  - Schema validation
  - Reference resolution

### Database
- **SQLite** (default)
  - Zero configuration
  - Embedded
  - Fast for local usage
- **PostgreSQL** (production)
  - Concurrent access
  - Advanced features
- **MySQL** (optional)
  - Enterprise environments

### Caching
- **Memory** - In-process LRU cache
- **Redis** - Distributed caching
  - `github.com/redis/go-redis` v9

### Authentication
- **OAuth2**
  - `golang.org/x/oauth2`
  - PKCE support
  - Device flow
- **mTLS**
  - Standard library `crypto/tls`
  - Certificate management
- **Keychain**
  - macOS: `security` command
  - Linux: Secret Service API
  - Windows: Credential Manager

### HTTP Client
- Standard library `net/http`
- Custom transport for:
  - Unix sockets
  - mTLS
  - Proxy support

### Testing
- **Standard library** `testing`
- **Testify** - Assertions and mocks
- **Ginkgo/Gomega** - BDD style (optional)

### Logging
- **Structured logging**
  - `log/slog` (Go 1.21+)
  - Or `github.com/rs/zerolog`

### Interactive UI
- **Bubble Tea** - TUI framework
  - Progress bars
  - Spinners
  - Interactive prompts
- **Lipgloss** - Styling

## Development Environment

### Required Tools
```bash
# Go
brew install go  # macOS
apt install golang-go  # Ubuntu

# Make
brew install make

# Docker
brew install docker

# golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

### Recommended Tools
```bash
# GoReleaser
brew install goreleaser

# Air (hot reload)
go install github.com/cosmtrek/air@latest

# Delve (debugger)
go install github.com/go-delve/delve/cmd/dlv@latest
```

### IDE Setup
- **VS Code** + Go extension
- **GoLand** (JetBrains)
- **Vim/Neovim** + LSP

## Build Configuration

### Makefile Targets
```makefile
# Development
make build          # Build binary
make test           # Run tests
make lint           # Run linters
make fmt            # Format code

# Release
make release        # Create release
make docker-build   # Build container

# Utilities
make clean          # Clean artifacts
make install        # Install locally
```

### Go Module
```go
module github.com/openapi-cli/openapi-cli

go 1.21

require (
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0
    github.com/getkin/kin-openapi v0.125.0
    go.starlark.net v0.0.0-20240123142251-f86470692795
    github.com/bytecodealliance/wasmtime-go v8.0.0
)
```

## Platform Support

### Target Platforms
- **Linux**: amd64, arm64
- **macOS**: amd64, arm64 (Apple Silicon)
- **Windows**: amd64, arm64

### Platform-Specific Code
```go
//go:build darwin
// +build darwin

package keychain
// macOS implementation
```

### Cross-Compilation
```bash
# Linux
GOOS=linux GOARCH=amd64 go build

# macOS
GOOS=darwin GOARCH=arm64 go build

# Windows
GOOS=windows GOARCH=amd64 go build
```

## Dependencies

### Direct Dependencies
| Package | Version | Purpose |
|---------|---------|---------|
| cobra | 1.8+ | CLI framework |
| viper | 1.18+ | Configuration |
| wasmtime-go | 8.0.0 | WASM runtime |
| starlark | latest | DSL interpreter |
| kin-openapi | 0.125+ | OpenAPI parsing |
| go-redis | 9+ | Redis client |
| testify | 1.9+ | Testing utilities |
| zerolog | 1.31+ | Structured logging |

### Indirect Dependencies
Managed by `go mod`, see `go.sum`

### Vendor Strategy
- **Default**: Use Go modules
- **Enterprise**: Vendor dependencies with `go mod vendor`
- **Air-gapped**: Pre-downloaded vendor directory

## External Services

### Optional Services
- **Redis**: For distributed caching
- **PostgreSQL**: For production database
- **Prometheus**: For metrics collection
- **Jaeger**: For distributed tracing

### API Dependencies
- **Inventory Server**: Updates and modules
- **OAuth2 Providers**: GitHub, Google, Azure
- **Package Registries**: npm, PyPI, etc.

## Security Considerations

### Secrets Management
- Credentials stored in OS keychain
- Never committed to version control
- Rotated automatically

### TLS Configuration
- Minimum version: TLS 1.2
- Preferred: TLS 1.3
- Cipher suites: Only secure algorithms

### WASM Security
- Sandboxed execution
- Capability-based access
- Resource limits enforced

## Performance Targets

### Startup Time
- Cold start: <100ms
- Warm start: <50ms

### Runtime Performance
- Command generation: <100ms
- WASM execution: <1s for simple modules
- API request: <500ms (with caching)

### Resource Usage
- Memory: <512MB typical
- Disk: <100MB binary + cache
- CPU: Minimal idle, burst during execution

## Monitoring

### Metrics
- Request latency
- Error rates
- Cache hit/miss ratios
- WASM execution time

### Logging
- Structured JSON format
- Log levels: DEBUG, INFO, WARN, ERROR
- Rotation: Daily, keep 30 days

### Health Checks
- `/health` - Basic liveness
- `/ready` - Readiness probe
- `/metrics` - Prometheus metrics

## Development Workflow

### Local Development
```bash
# Clone
git clone https://github.com/openapi-cli/openapi-cli
cd openapi-cli

# Install dependencies
go mod download

# Build
make build

# Test
make test

# Run
./bin/openapi-cli --help
```

### Testing
```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...

# Coverage
go test -cover ./...

# Race detection
go test -race ./...
```

### Debugging
```bash
# Start debugger
dlv debug ./cmd/openapi-cli

# Or use Air for hot reload
air
```

## CI/CD

### GitHub Actions
- **Test**: On PR, run tests
- **Lint**: Code quality checks
- **Build**: Multi-platform builds
- **Release**: Automated releases

### Release Process
1. Tag version: `git tag v1.0.0`
2. Push tag: `git push origin v1.0.0`
3. GitHub Actions builds releases
4. Artifacts uploaded to GitHub Releases

## Documentation

### Code Documentation
- All exported functions documented
- Examples in `_test.go` files
- Architecture Decision Records (ADRs)

### User Documentation
- README.md
- docs/ directory
- Inline help (`--help`)

## Constraints

### Technical Constraints
- Go 1.21+ required
- Wasmtime 8.0.0 exact version
- Deterministic execution required

### Business Constraints
- MIT License
- Open source
- Community contributions welcome

## Migration Path

### From Existing Tools
- Import OpenAPI specs
- Migrate authentication config
- Gradual adoption

### Version Upgrades
- Semantic versioning
- Migration guides
- Deprecation warnings
