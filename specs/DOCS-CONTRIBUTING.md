# Contributing Guidelines

Thank you for your interest in contributing to Muster!

## Development Setup

### Prerequisites

- Go 1.21 or later
- Git
- Make

### Clone Repository

```bash
git clone https://github.com/your-org/muster.git
cd muster
```

### Install Dependencies

```bash
make deps
```

### Build

```bash
make build
```

### Run Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific test
make test TEST=TestAuth
```

## Code Style

### Go Code

We follow standard Go conventions:

- Use `gofmt` for formatting
- Use `golint` for linting
- Use `go vet` for static analysis

```bash
# Format code
make fmt

# Run linter
make lint

# Run all checks
make check
```

### Naming Conventions

- **Packages**: lowercase, single word
- **Files**: snake_case.go
- **Types**: PascalCase
- **Functions**: PascalCase (exported), camelCase (unexported)
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE

### Documentation

- Add comments for exported functions/types
- Include examples in package documentation
- Update README.md for user-facing changes

## Testing

### Test Structure

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "valid input",
            input:    "test",
            expected: "TEST",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := FunctionName(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Test Coverage

- Maintain >80% code coverage
- Test error cases
- Test edge cases
- Add integration tests for complex features

## Pull Request Process

1. **Fork** the repository
2. **Create branch**: `git checkout -b feature/your-feature`
3. **Make changes** with tests
4. **Run checks**: `make check`
5. **Commit** with descriptive message:
   ```
   feat: add support for X
   
   - Implements feature X
   - Adds tests
   - Updates documentation
   ```
6. **Push** to your fork
7. **Create PR** with description

### PR Requirements

- Clear description of changes
- Link to related issue
- Tests included
- Documentation updated
- No linting errors
- All tests passing

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation only
- `style:` - Formatting changes
- `refactor:` - Code refactoring
- `test:` - Adding tests
- `chore:` - Maintenance tasks

## Code Review

- All PRs require at least one review
- Address review comments promptly
- Keep PRs focused and small
- Rebase on main before merging

## Release Process

1. Update version in `version.go`
2. Update `CHANGELOG.md`
3. Create tag: `git tag v1.2.3`
4. Push tag: `git push origin v1.2.3`
5. CI will create release automatically

## Community

- Be respectful and constructive
- Help others in discussions
- Follow the Code of Conduct
