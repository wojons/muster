# Development Setup

Setup guide for contributing to OpenAPI CLI.

## Prerequisites

### Required Tools

- **Go**: 1.26.10 or later (the version in `go.mod`)
- **A C compiler**: `gcc` or `clang` — SQLite support is a CGO package
  (`github.com/mattn/go-sqlite3`) and `make` builds with `CGO_ENABLED=1`
- **Git**: 2.30 or later
- **Docker**: For containerized development (optional)
- **Make**: For build automation

### Recommended Tools

- **VS Code** or **GoLand** - IDE
- **golangci-lint** - Linter
- **go test** - Testing
- **dlv** - Debugger

## Cloning the Repository

```bash
# Clone the repository
git clone https://github.com/wojons/muster.git
cd muster

# Verify Go modules
go mod verify
```

## Local Development

### Build from Source

```bash
# Build binary
go build -o openapi-cli ./cmd/openapi-cli

# Run tests
go test ./...

# Run with debug
./openapi-cli --debug version
```

### Makefile Commands

```bash
# Build all binaries
make build

# Run tests
make test

# Run linting
make lint

# Run race detector
make race

# Format code
make fmt

# Run all checks
make check
```

### Running Locally

```bash
# Start OpenAPI CLI
go run ./cmd/openapi-cli --help

# Start MCP server
go run ./cmd/openapi-cli mcp-server --spec openapi.yaml

# Test with example spec
openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json
```

## IDE Setup

### VS Code

Recommended extensions:

1. **Go** - Google's official Go extension
2. **GitLens** - Git integration
3. **Docker** - Container support

**Settings**:

```json
{
  "go.inferGopath": false,
  "go.useLanguageServer": true,
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "workspace",
  "go.testTimeout": "30s"
}
```

### GoLand

1. Open project directory
2. Go will auto-detect Go module
3. Configure Go SDK in Settings
4. Enable "Go Modules Integration"

## Testing

### Running Tests

```bash
# All tests
go test ./...

# Package-specific tests
go test ./pkg/...

# With coverage
go test ./... -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out

# Verbose output
go test ./... -v
```

### Writing Tests

```go
package mypackage

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    // Setup
    input := "test"

    // Execute
    result, err := MyFunction(input)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, "expected", result)
}

func TestMyFunction_ErrorCase(t *testing.T) {
    // Test error handling
    _, err := MyFunction("")
    assert.Error(t, err)
}
```

### Test Organization

```
pkg/
  mypackage/
    mypackage.go
    mypackage_test.go
    mypackage_integration_test.go  // Integration tests
```

### Mocking External Dependencies

```go
import "github.com/stretchr/testify/mock"

type MockHTTPClient struct {
    mock.Mock
}

func (m *MockHTTPClient) Get(url string) (*Response, error) {
    args := m.Called(url)
    return args.Get(0).(*Response), args.Error(1)
}

// Use in tests
func TestWithMock(t *testing.T) {
    mockClient := new(MockHTTPClient)
    mockClient.On("Get", "http://test.com").Return(&Response{}, nil)

    // Test code...
}
```

## Code Style

### Naming Conventions

- **Packages**: Lowercase, single word, no underscores
- **Exported**: PascalCase (`MyFunction`)
- **Private**: camelCase (`myFunction`)
- **Constants**: PascalCase or all caps (`MaxRetries` or `MAX_RETRIES`)
- **Interfaces**: `-er` suffix (`Reader`, `Writer`)

### File Organization

```go
// Package comment (one file per package)
package mypackage

// Order in file:
// 1. Package imports
// 2. Constants
// 3. Variables
// 4. Types
// 5. Functions
```

### Error Handling

```go
// Always check errors
result, err := doSomething()
if err != nil {
    return fmt.Errorf("doSomething: %w", err)
}

// Wrap errors with context
if err != nil {
    return fmt.Errorf("initializing %s: %w", configName, err)
}
```

### GoDoc Comments

```go
// DoSomething performs an operation and returns result.
// If the operation fails, an error is returned.
//
// Parameters:
//   input - The input to process
//
// Returns:
//   The result and any error encountered
func DoSomething(input string) (string, error) {
    // Implementation
}
```

## Debugging

### Using dlv

```bash
# Debug tests
dlv test ./pkg/mypackage

# Debug main function
dlv debug ./cmd/openapi-cli

# Attach to running process
dlv attach <pid>
```

### Logging

```go
import "go.uber.org/zap"

logger, _ := zap.NewProduction()
defer logger.Sync()

logger.Info("Processing request",
    zap.String("method", "GET"),
    zap.String("path", "/pet/1"),
)
```

## Performance Profiling

### CPU Profiling

```go
import (
    "os"
    "runtime/pprof"
)

func main() {
    f, _ := os.Create("cpu.prof")
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()

    // Run code to profile
}

// Analyze with:
// go tool pprof cpu.prof
```

### Memory Profiling

```go
f, _ := os.Create("mem.prof")
defer f.Close()

pprof.WriteHeapProfile(f)

// Analyze with:
// go tool pprof mem.prof
```

## Build Tags

```go
//go:build !windows

package mypackage

// Windows-specific code would be in:
//go:build windows
package mypackage
```

## Continuous Integration

### GitHub Actions

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test ./...
```

### Pre-commit Hooks

```bash
# Install pre-commit
pre-commit install

# Run checks
pre-commit run --all-files
```

## Documentation

### Code Comments

- Exported functions, types, and constants must have comments
- Comments should explain "why" not "what"
- Use complete sentences

### Examples in GoDoc

```go
// Example usage:
//
//   result, err := NewAPI().Get("/users/1")
//   if err != nil {
//       log.Fatal(err)
//   }
//   fmt.Println(result.Name)
```

## Release Process

### Version Bumping

```bash
# Update version in code
# Update CHANGELOG.md
# Tag release
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### Publishing

GoReleaser will automatically:
- Build binaries for all platforms
- Create GitHub releases
- Update Homebrew/Scoop formulas

## Contributing

### Guidelines

1. **Small, focused commits** - One change per commit
2. **Clear commit messages** - Use imperative mood
3. **Test coverage** - Maintain >80% coverage
4. **Documentation** - Update docs for user-facing changes
5. **Code review** - All PRs require review

### Commit Message Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

Types:
- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation
- `style` - Formatting
- `refactor` - Code change
- `test` - Tests
- `chore` - Build process

### Pull Request Process

1. Fork repository
2. Create branch (`git checkout -b feature/cool-thing`)
3. Commit changes (`git commit -m 'feat: add cool thing'`)
4. Push to branch (`git push origin feature/cool-thing`)
5. Create Pull Request

PR must:
- Pass all CI checks
- Include tests
- Update documentation
- Be reviewed by a maintainer

## Troubleshooting

### Go Module Issues

```bash
# Update dependencies
go mod tidy

# Verify integrity
go mod verify

# Clean module cache
go clean -modcache
```

### Test Failures

```bash
# Run specific test
go test -run TestMyFunction ./pkg/mypackage

# Run with race detector
go test -race ./...

# Run with race and timeout
go test -race -timeout 5m ./...
```

## Resources

- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Go Wiki](https://github.com/golang/go/wiki)
- [Project Issues](https://github.com/wojons/muster/issues)
- [Project Discussions](https://github.com/wojons/muster/discussions)
