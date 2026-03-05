# Contributing to Muster

Thank you for your interest in contributing to Muster!

## Getting Started

### Prerequisites

- **Go**: 1.21 or later
- **Git**: 2.30 or later
- **Docker**: For containerized development (optional)
- **Make**: For build automation

### Setting Up Your Development Environment

```bash
# Clone the repository
git clone https://github.com/anomalyco/muster.git
cd muster

# Install dependencies
go mod download

# Verify the installation
go run ./cmd/muster-cli --version
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...

# Run tests for a specific package
go test ./pkg/config

# Run tests with verbose output
go test -v ./...
```

### Building

```bash
# Build CLI binary
go build -o bin/muster-cli ./cmd/muster-cli

# Build MCP server binary
go build -o bin/muster-mcp ./cmd/muster-mcp

# Build for your platform
go build -o muster-linux ./cmd/muster-cli
```

## Development Workflow

### Code Style

We follow standard Go conventions:

- Use `gofmt` for formatting
- Use `golint` for linting
- Add comments for exported functions
- Write testable code

```bash
# Format code
go fmt ./...

# Run linter
golangci-lint run

# Fix linting issues
golangci-lint run --fix
```

### Testing

All contributions must include tests:

```go
func TestExample(t *testing.T) {
    result := FunctionUnderTest()
    assert.Equal(t, "expected", result)
}
```

Test coverage is required for new code:
- Aim for > 80% coverage
- Write unit tests for business logic
- Add integration tests for external interactions

### Documentation

- Update README.md for user-facing changes
- Add inline comments for complex code
- Document new public APIs
- Update the [documentation](docs/) for feature changes

## Adding Features

### 1. Create a Branch

```bash
# Create feature branch
git checkout -b feature/your-feature-name

# Or for bug fixes
git checkout -b fix/your-bug-fix
```

### 2. Write Code

- Write tests first (TDD)
- Implement the feature
- Ensure all tests pass
- Update documentation

### 3. Test Your Changes

```bash
# Run tests
go test ./...

# Run linter
golangci-lint run

# Test integration
go test -tags integration ./...
```

### 4. Commit Changes

```bash
git add .
git commit -m "Add feature: description of your changes"

# Use conventional commits:
# feat: add new feature
# fix: fix a bug
# docs: update documentation
# test: add or update tests
# refactor: code refactoring
```

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request via GitHub.

## Code Review Process

1. Automated checks must pass (CI/CD)
2. At least one maintainer approval required
3. All comments addressed
4. No merge conflicts

## Project Structure

```
.
├── cmd/                 # Main entry points
│   ├── muster-cli/      # CLI application
│   ├── muster-mcp/      # MCP server
│   └── muster-daemon/   # Background daemon
├── pkg/                 # Public APIs
│   ├── openapi/         # OpenAPI parsing
│   ├── wasm/            # WebAssembly runtime
│   ├── mcp/             # MCP protocol
│   └── ...
├── internal/            # Internal utilities
├── docs/                # Documentation
└── specs/               # Design specifications
```

## Design Principles

### Keep It Simple
- Avoid over-engineering
- Use standard libraries when possible
- Clear, readable code

### Security First
- Never log secrets
- Validate all inputs
- Follow security best practices
- Use constant-time comparisons for sensitive data

### Cross-Platform
- Code should work on Linux, macOS, and Windows
- Use Go's built-in cross-platform support
- Test on multiple platforms

### Backward Compatibility
- Don't break existing functionality
- Use deprecation warnings for API changes
- Document breaking changes

## Reporting Issues

When reporting bugs:

1. Search existing issues first
2. Use the issue templates
3. Include:
   - Steps to reproduce
   - Expected behavior
   - Actual behavior
   - Environment details (OS, Go version)
   - Logs or error messages

## Feature Requests

For new features:

1. Open an issue first to discuss
2. Explain the use case
3. Provide examples if possible
4. Consider the maintenance cost

## Getting Help

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/anomalyco/muster/issues)
- **Discussions**: [GitHub Discussions](https://github.com/anomalyco/muster/discussions)

## License

By contributing, you agree that your contributions will be licensed under the MIT License or Apache License 2.0, at your choice.

## Additional Resources

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
- [Muster Documentation](docs/)
