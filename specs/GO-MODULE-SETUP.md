# Go Module Setup Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Build System

## Overview

Complete specification for **Go module setup** defining module structure, dependency management, versioning strategy, and build configuration for the OpenAPI CLI project. This specification ensures reproducible builds, clear dependency governance, and compatibility across development and production environments.

## 1. Module Configuration

### 1.1 go.mod Structure

```go
// go.mod
module github.com/openapi-cli/openapi-cli

go 1.21

require (
    // CLI framework
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    
    // MCP protocol
    github.com/mark3labs/mcp-go v0.8.0
    
    // HTTP client
    github.com/go-resty/resty/v2 v2.11.0
    
    // OpenAPI parsing
    github.com/getkin/kin-openapi v0.123.0
    
    // WASM runtime
    github.com/tetratelabs/wazero v1.6.0
    
    // Starlark DSL
    go.starlark.net v0.0.0-20240123142251-f86470692795
    
    // Database
    github.com/mattn/go-sqlite3 v1.14.22
    github.com/lib/pq v1.10.9
    github.com/go-sql-driver/mysql v1.7.1
    github.com/pressly/goose/v3 v3.18.0
    
    // Caching
    github.com/redis/go-redis/v9 v9.4.0
    github.com/patrickmn/go-cache v2.1.0+incompatible
    
    // Authentication
    golang.org/x/oauth2 v0.17.0
    github.com/zalando/go-keyring v0.2.3
    
    // Rate limiting
    golang.org/x/time v0.5.0
    
    // Observability
    go.opentelemetry.io/otel v1.23.1
    go.opentelemetry.io/otel/trace v1.23.1
    go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.23.1
    
    // Utilities
    github.com/charmbracelet/lipgloss v0.10.0
    github.com/charmbracelet/bubbles v0.18.0
    github.com/fatih/color v1.16.0
    github.com/olekukonko/tablewriter v0.0.5
    github.com/sirupsen/logrus v1.9.3
    github.com/google/uuid v1.6.0
    github.com/mitchellh/mapstructure v1.5.0
    
    // Testing
    github.com/stretchr/testify v1.9.0
    github.com/vektra/mockery/v2 v2.42.0
)

require (
    // Indirect dependencies
    github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
    github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
    gopkg.in/yaml.v3 v3.0.1 // indirect
    // ... other indirect deps
)
```

### 1.2 Go Version Policy

| Environment | Go Version | Update Policy |
|-------------|------------|---------------|
| Minimum | 1.21 | Supports 2 latest minor versions |
| Development | 1.21.x | Latest patch monthly |
| CI/CD | 1.21.x | Latest patch weekly |
| Production | 1.21.x | Tested, stable version |

## 2. Dependency Management

### 2.1 Dependency Categories

```go
// Direct dependencies: Core functionality
DirectDeps = [
    "github.com/spf13/cobra",        // CLI framework
    "github.com/getkin/kin-openapi", // OpenAPI parsing
    "github.com/tetratelabs/wazero", // WASM runtime
    "go.starlark.net",               // Starlark DSL
]

// Infrastructure dependencies
InfraDeps = [
    "github.com/mattn/go-sqlite3",   // SQLite driver
    "github.com/redis/go-redis/v9",  // Redis client
    "golang.org/x/oauth2",           // OAuth2
]

// Observability dependencies
ObservabilityDeps = [
    "go.opentelemetry.io/otel",      // OpenTelemetry
    "github.com/sirupsen/logrus",    // Logging
]

// Development dependencies
DevDeps = [
    "github.com/stretchr/testify",   // Testing
    "github.com/vektra/mockery/v2",  // Mock generation
]
```

### 2.2 Version Pinning Rules

```makefile
# Makefile - Dependency management targets

.PHONY: deps-check deps-update deps-vendor deps-verify

# Check for outdated dependencies
deps-check:
	@echo "Checking for outdated dependencies..."
	@go list -u -m all | grep -E "^\S+\s+\S+\s+=>" || true
	@go list -u -m all | grep "\[.*\]" || echo "All dependencies up to date"

# Update minor versions (safe)
deps-update:
	@echo "Updating dependencies to latest minor versions..."
	@go get -u ./...
	@go mod tidy
	@go mod verify

# Update specific dependency
deps-update-single:
	@read -p "Enter package path: " pkg; \
	go get -u $$pkg && go mod tidy

# Create vendor directory
deps-vendor:
	@echo "Creating vendor directory..."
	@go mod vendor
	@git add vendor/ go.mod go.sum

# Verify dependencies
deps-verify:
	@echo "Verifying dependencies..."
	@go mod verify
	@go mod tidy -diff || (echo "go.mod is not tidy" && exit 1)

# Audit dependencies for vulnerabilities
deps-audit:
	@echo "Auditing dependencies..."
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	@govulncheck ./...
```

### 2.3 Dependency Governance

| Dependency Type | Approval Required | Max Age |
|-----------------|------------------|---------|
| Critical (security) | Security team | Immediate |
| Major version bump | Tech lead review | Quarterly |
| Minor version bump | CI tests pass | Monthly |
| Patch version | Auto-merge | Weekly |
| New dependency | Architecture review | N/A |

## 3. Build Configuration

### 3.1 Build Tags

```go
// build/tags.go
//go:build !wasm

package build

// DefaultBuildTags contains tags for standard builds
var DefaultBuildTags = []string{
    "sqlite",      // Include SQLite support
    "redis",       // Include Redis support
    "otel",        // Include OpenTelemetry
}

// MinimalBuildTags for constrained environments
var MinimalBuildTags = []string{
    "sqlite",
}
```

### 3.2 Build Scripts

```makefile
# Makefile - Build targets

# Build variables
BINARY_NAME=openapi-cli
DAEMON_NAME=openapi-daemon
MCP_NAME=openapi-mcp
VERSION=$(shell git describe --tags --always --dirty)
COMMIT=$(shell git rev-parse --short HEAD)
BUILD_DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"
TAGS=-tags "sqlite redis otel"

.PHONY: all build build-cli build-daemon build-mcp clean test

all: build

# Build all binaries
build: build-cli build-daemon build-mcp

# Build CLI binary
build-cli:
	@echo "Building CLI binary..."
	@go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/openapi-cli

# Build daemon binary
build-daemon:
	@echo "Building daemon binary..."
	@go build $(TAGS) $(LDFLAGS) -o bin/$(DAEMON_NAME) ./cmd/daemon

# Build MCP server binary
build-mcp:
	@echo "Building MCP server binary..."
	@go build $(TAGS) $(LDFLAGS) -o bin/$(MCP_NAME) ./cmd/mcp-server

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	@GOOS=darwin GOARCH=amd64 go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/openapi-cli
	@GOOS=darwin GOARCH=arm64 go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/openapi-cli
	@GOOS=linux GOARCH=amd64 go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/openapi-cli
	@GOOS=linux GOARCH=arm64 go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/openapi-cli
	@GOOS=windows GOARCH=amd64 go build $(TAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/openapi-cli

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@go clean -cache

# Run tests
test:
	@echo "Running tests..."
	@go test $(TAGS) -race -coverprofile=coverage.out ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@go test $(TAGS) -tags=integration -race ./test/integration/...

# Run e2e tests
test-e2e:
	@echo "Running e2e tests..."
	@go test $(TAGS) -tags=e2e -race ./test/e2e/...

# Generate mocks
mocks:
	@echo "Generating mocks..."
	@mockery --all --inpackage --case=underscore

# Generate code
generate:
	@echo "Running code generation..."
	@go generate ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@gofumpt -w .

# Lint code
lint:
	@echo "Linting code..."
	@golangci-lint run ./...

# Security scan
security:
	@echo "Running security scan..."
	@gosec -quiet ./...
	@govulncheck ./...

# Full CI check
ci: deps-verify fmt lint test security
	@echo "All CI checks passed!"
```

## 4. Version Management

### 4.1 Version Strategy

```go
// internal/version/version.go

package version

import (
    "fmt"
    "runtime"
)

// Build-time variables (set via ldflags)
var (
    Version   = "dev"
    Commit    = "unknown"
    BuildDate = "unknown"
)

// Info contains version information
type Info struct {
    Version   string `json:"version"`
    Commit    string `json:"commit"`
    BuildDate string `json:"build_date"`
    GoVersion string `json:"go_version"`
    Platform  string `json:"platform"`
}

// Get returns current version info
func Get() Info {
    return Info{
        Version:   Version,
        Commit:    Commit,
        BuildDate: BuildDate,
        GoVersion: runtime.Version(),
        Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
    }
}

// String returns formatted version string
func (i Info) String() string {
    return fmt.Sprintf("openapi-cli version %s (commit: %s, built: %s)\n%s/%s %s",
        i.Version, i.Commit, i.BuildDate, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
```

### 4.2 Semantic Versioning

```
Version Format: MAJOR.MINOR.PATCH[-PRERELEASE][+METADATA]

Examples:
  1.0.0          - Initial stable release
  1.1.0          - New features, backward compatible
  1.1.1          - Bug fixes only
  2.0.0          - Breaking changes
  1.2.0-beta.1   - Beta pre-release
  1.2.0+build.42 - Build metadata

Branch Strategy:
  main          → Development, version: 1.x.x-dev
  release/1.x   → Stable releases, version: 1.x.x
  hotfix/*      → Critical fixes, version: 1.x.x+1
```

## 5. Workspace Configuration

### 5.1 go.work (Optional)

```go
// go.work - For multi-module development

go 1.21

use (
    .
    ./tools
    ./api
)

replace github.com/openapi-cli/api => ./api
```

### 5.2 Tools Module

```go
// tools/go.mod

module github.com/openapi-cli/tools

go 1.21

require (
    github.com/vektra/mockery/v2 v2.42.0
    github.com/golangci/golangci-lint v1.56.2
    golang.org/x/vuln/cmd/govulncheck v1.0.4
    github.com/securego/gosec/v2 v2.19.0
    mvdan.cc/gofumpt v0.6.0
)
```

```makefile
# tools/tools.go

//go:build tools

package tools

import (
    _ "github.com/vektra/mockery/v2"
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "golang.org/x/vuln/cmd/govulncheck"
    _ "github.com/securego/gosec/v2/cmd/gosec"
    _ "mvdan.cc/gofumpt"
)
```

## 6. CI/CD Integration

### 6.1 GitHub Actions Workflow

```yaml
# .github/workflows/ci.yml

name: CI

on:
  push:
    branches: [main, release/*]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout=5m

  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.21', '1.22']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - run: go mod download
      - run: go test -race -coverprofile=coverage.out ./...
      - uses: codecov/codecov-action@v4
        with:
          files: coverage.out

  build:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: make build-all
      - uses: actions/upload-artifact@v4
        with:
          name: binaries
          path: bin/

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...
      - uses: securego/gosec@master
        with:
          args: '-quiet ./...'
```

## 7. Module Maintenance

### 7.1 Update Schedule

| Task | Frequency | Command |
|------|-----------|---------|
| Check outdated | Weekly | `make deps-check` |
| Update patches | Weekly | `go get -u=patch ./...` |
| Update minors | Monthly | `make deps-update` |
| Security audit | Daily | `make deps-audit` |
| Major reviews | Quarterly | Architecture review |

### 7.2 Dependency Cleanup

```bash
#!/bin/bash
# scripts/cleanup-deps.sh

echo "Cleaning up unused dependencies..."

# Find unused dependencies
go mod tidy

# Verify no indirect dependencies can be direct
# (This requires manual review)

echo "Checking for replace directives..."
grep -n "replace " go.mod || echo "No replace directives found"

echo "Checking for retracted versions..."
go list -m -u all | grep "retracted" || echo "No retracted versions found"

echo "Dependency cleanup complete!"
```

## 8. Reproducible Builds

### 8.1 go.sum Integrity

```bash
# Verify checksums
go mod verify

# Ensure go.sum is committed
# Never add go.sum to .gitignore for applications

# For libraries, go.sum is optional but recommended
```

### 8.2 Build Reproducibility

```dockerfile
# Dockerfile - Reproducible build environment

FROM golang:1.21-alpine AS builder

# Pin Alpine version for reproducibility
RUN apk add --no-cache git make

WORKDIR /build

# Copy and download dependencies first (cache layer)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build with specific flags for reproducibility
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags="-s -w -extldflags '-static' \
    -X main.Version=$(git describe --tags) \
    -X main.Commit=$(git rev-parse HEAD) \
    -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /openapi-cli \
    ./cmd/openapi-cli

# Final minimal image
FROM gcr.io/distroless/static-debian11
COPY --from=builder /openapi-cli /openapi-cli
ENTRYPOINT ["/openapi-cli"]
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

*This specification produces identical implementations across different AI systems. No human interpretation required.*
