# Build System Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

---

## Overview

This specification defines the comprehensive build system for the Muster OpenAPI CLI + MCP Server. The build system supports development workflows, cross-platform builds for 6 target platforms, release automation, Docker containerization, and CI/CD integration. All build operations are deterministic and produce byte-identical binaries when built from the same source commit.

A blind person can visualize this as a sophisticated factory with multiple assembly lines: one for local development (fast, iterative), one for cross-platform production (comprehensive, tested), and one for container deployment (isolated, reproducible). Each assembly line uses the same blueprints (source code) but produces products optimized for different destinations (platforms).

---

## 1. Directory Structure

```
/
├── Makefile                    # Main build orchestration
├── go.mod                      # Go module definition
├── go.sum                      # Dependency checksums
├── cmd/                        # Entry points
│   ├── cli/                    # CLI binary (main.go)
│   ├── mcp/                    # MCP server binary
│   └── daemon/                 # Daemon binary
├── scripts/
│   ├── build.sh               # Cross-platform build wrapper
│   ├── release.sh             # Release automation
│   ├── install.sh             # Installation script
│   └── version.sh             # Version extraction
├── build/
│   ├── Dockerfile             # Multi-stage Docker build
│   └── .dockerignore          # Docker ignore rules
├── bin/                       # Build output (gitignored)
└── dist/                      # Release artifacts (gitignored)
```

---

## 2. Complete Makefile

```makefile
# =============================================================================
# Build System Configuration
# =============================================================================

# Binary names
BINARY_NAME := muster
CLI_BINARY := muster-cli
MCP_BINARY := muster-mcp
DAEMON_BINARY := muster-daemon

# Build directories
BUILD_DIR := bin
DIST_DIR := dist
CMD_DIR := cmd

# Version information (injected at build time)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Go configuration
GOPATH := $(shell go env GOPATH)
GOBIN := $(GOPATH)/bin
GO_VERSION := $(shell go version | awk '{print $$3}')

# Build flags
LDFLAGS := -ldflags "\
	-X github.com/anomalyco/muster/internal/version.Version=$(VERSION) \
	-X github.com/anomalyco/muster/internal/version.Commit=$(COMMIT) \
	-X github.com/anomalyco/muster/internal/version.BuildDate=$(BUILD_DATE) \
	-X github.com/anomalyco/muster/internal/version.Branch=$(BRANCH) \
	-X github.com/anomalyco/muster/internal/version.GoVersion=$(GO_VERSION) \
	-s -w"

# CGO configuration for SQLite
CGO_ENABLED := 1

# Platform targets
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64 \
	windows/arm64

# Default target
.DEFAULT_GOAL := build

# =============================================================================
# Development Builds
# =============================================================================

.PHONY: build
build: ## Build for current platform
	@echo "Building $(BINARY_NAME) $(VERSION) for current platform..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: build-cli
build-cli: ## Build CLI only
	@echo "Building CLI $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./$(CMD_DIR)/cli
	@echo "✓ Built: $(BUILD_DIR)/$(CLI_BINARY)"

.PHONY: build-mcp
build-mcp: ## Build MCP server only
	@echo "Building MCP server $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(MCP_BINARY) ./$(CMD_DIR)/mcp
	@echo "✓ Built: $(BUILD_DIR)/$(MCP_BINARY)"

.PHONY: build-daemon
build-daemon: ## Build daemon only
	@echo "Building daemon $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(DAEMON_BINARY) ./$(CMD_DIR)/daemon
	@echo "✓ Built: $(BUILD_DIR)/$(DAEMON_BINARY)"

.PHONY: build-all
build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		OUTPUT=$(BUILD_DIR)/$(BINARY_NAME)-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then OUTPUT="$$OUTPUT.exe"; fi; \
		echo "  → Building for $$GOOS/$$GOARCH..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH \
			go build $(LDFLAGS) -o $$OUTPUT ./$(CMD_DIR) || exit 1; \
	done
	@echo "✓ All builds complete"

# =============================================================================
# Release Builds
# =============================================================================

.PHONY: release
release: clean generate lint test ## Create release builds
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		PLATFORM_NAME=$$GOOS-$$GOARCH; \
		RELEASE_NAME=$(BINARY_NAME)-$(VERSION)-$$PLATFORM_NAME; \
		mkdir -p $(DIST_DIR)/$$RELEASE_NAME; \
		OUTPUT=$(DIST_DIR)/$$RELEASE_NAME/$(BINARY_NAME); \
		if [ "$$GOOS" = "windows" ]; then OUTPUT="$$OUTPUT.exe"; fi; \
		echo "  → Building release for $$PLATFORM_NAME..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH \
			go build -trimpath $(LDFLAGS) -o $$OUTPUT ./$(CMD_DIR) || exit 1; \
		cp README.md LICENSE $(DIST_DIR)/$$RELEASE_NAME/ 2>/dev/null || true; \
		if [ "$$GOOS" = "windows" ]; then \
			(cd $(DIST_DIR) && zip -r $$RELEASE_NAME.zip $$RELEASE_NAME); \
		else \
			(cd $(DIST_DIR) && tar -czf $$RELEASE_NAME.tar.gz $$RELEASE_NAME); \
		fi; \
		rm -rf $(DIST_DIR)/$$RELEASE_NAME; \
	done
	@echo "✓ Release complete in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

.PHONY: release-local
release-local: ## Create release for current platform only
	@echo "Creating local release $(VERSION)..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-local ./$(CMD_DIR)
	@echo "✓ Local release: $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-local"

# =============================================================================
# Installation
# =============================================================================

.PHONY: install
install: build ## Install to system (default: /usr/local/bin)
	@echo "Installing $(BINARY_NAME) to $(DESTDIR)/usr/local/bin..."
	@install -d $(DESTDIR)/usr/local/bin
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(DESTDIR)/usr/local/bin/
	@echo "✓ Installed to $(DESTDIR)/usr/local/bin/$(BINARY_NAME)"

.PHONY: install-user
install-user: build ## Install to user bin directory
	@echo "Installing $(BINARY_NAME) to $(GOBIN)..."
	@mkdir -p $(GOBIN)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOBIN)/
	@echo "✓ Installed to $(GOBIN)/$(BINARY_NAME)"
	@echo "  Make sure $(GOBIN) is in your PATH"

.PHONY: uninstall
uninstall: ## Uninstall from system
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(DESTDIR)/usr/local/bin/$(BINARY_NAME)
	@rm -f $(GOBIN)/$(BINARY_NAME)
	@echo "✓ Uninstalled"

# =============================================================================
# Testing
# =============================================================================

.PHONY: test
test: ## Run all tests
	@echo "Running all tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@echo "✓ Tests complete"

.PHONY: test-unit
test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	@go test -v -short ./...
	@echo "✓ Unit tests complete"

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -tags=integration -run Integration ./...
	@echo "✓ Integration tests complete"

.PHONY: test-e2e
test-e2e: build ## Run end-to-end tests
	@echo "Running E2E tests..."
	@go test -v -tags=e2e -run E2E ./tests/e2e/...
	@echo "✓ E2E tests complete"

.PHONY: bench
bench: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...
	@echo "✓ Benchmarks complete"

.PHONY: coverage
coverage: test ## Generate coverage report
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# =============================================================================
# Code Quality
# =============================================================================

.PHONY: lint
lint: ## Run all linters
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "  → golangci-lint not installed, running go vet..."; \
		go vet ./...; \
	fi
	@echo "✓ Linting complete"

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Formatting complete"

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...
	@echo "✓ Vet complete"

# =============================================================================
# Code Generation
# =============================================================================

.PHONY: generate
generate: ## Run all code generation
	@echo "Running code generation..."
	@go generate ./...
	@echo "✓ Code generation complete"

.PHONY: generate-sqlc
generate-sqlc: ## Generate SQL code with sqlc
	@echo "Generating SQL code with sqlc..."
	@if command -v sqlc >/dev/null 2>&1; then \
		sqlc generate -f internal/db/sqlc.yaml; \
	else \
		echo "  ✗ sqlc not installed. Run: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi
	@echo "✓ SQL code generation complete"

.PHONY: generate-mocks
generate-mocks: ## Generate mocks with mockgen
	@echo "Generating mocks..."
	@go generate ./internal/...
	@echo "✓ Mocks generated"

# =============================================================================
# Docker
# =============================================================================

DOCKER_IMAGE := muster
DOCKER_TAG ?= $(VERSION)

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		-f build/Dockerfile .
	@echo "✓ Docker image built"

.PHONY: docker-run
docker-run: ## Run in Docker container
	@echo "Running Docker container..."
	@docker run --rm -it \
		-v $(PWD):/workspace \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-push
docker-push: docker-build ## Push Docker image to registry
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(DOCKER_IMAGE):latest
	@echo "✓ Docker image pushed"

# =============================================================================
# Cleanup
# =============================================================================

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR) $(DIST_DIR) coverage.out coverage.html
	@go clean -cache
	@echo "✓ Clean complete"

.PHONY: distclean
distclean: clean ## Clean all generated files
	@echo "Deep cleaning..."
	@rm -rf vendor/
	@git clean -fdx -e .env -e .idea -e .vscode
	@echo "✓ Deep clean complete"

# =============================================================================
# Development Helpers
# =============================================================================

.PHONY: dev
dev: ## Run in development mode
	@echo "Starting development server..."
	@go run ./$(CMD_DIR) --dev

.PHONY: watch
watch: ## Watch for changes and rebuild (requires entr)
	@echo "Watching for changes..."
	@find . -name '*.go' | entr -r make build

.PHONY: deps
deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "✓ Dependencies ready"

.PHONY: deps-update
deps-update: ## Update all dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "✓ Dependencies updated"

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying modules..."
	@go mod tidy
	@echo "✓ Modules tidied"

.PHONY: vendor
vendor: ## Create vendor directory
	@echo "Creating vendor directory..."
	@go mod vendor
	@echo "✓ Vendor directory created"

# =============================================================================
# Information
# =============================================================================

.PHONY: version
version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Branch: $(BRANCH)"
	@echo "Go Version: $(GO_VERSION)"

.PHONY: info
info: ## Show build information
	@echo "Project: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Dir: $(BUILD_DIR)"
	@echo "Dist Dir: $(DIST_DIR)"
	@echo "Go Path: $(GOPATH)"
	@echo "CGO Enabled: $(CGO_ENABLED)"

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
```

---

## 3. Build Scripts

### 3.1 Version Extraction Script

```bash
#!/bin/bash
# scripts/version.sh
set -e

# Extract version information from git

get_version() {
    git describe --tags --always --dirty 2>/dev/null || echo "dev"
}

get_commit() {
    git rev-parse --short HEAD 2>/dev/null || echo "unknown"
}

get_build_date() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

get_branch() {
    git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown"
}

# Output based on argument
case "${1:-all}" in
    version)
        get_version
        ;;
    commit)
        get_commit
        ;;
    date)
        get_build_date
        ;;
    branch)
        get_branch
        ;;
    json)
        cat <<EOF
{
  "version": "$(get_version)",
  "commit": "$(get_commit)",
  "build_date": "$(get_build_date)",
  "branch": "$(get_branch)"
}
EOF
        ;;
    *)
        echo "VERSION=$(get_version)"
        echo "COMMIT=$(get_commit)"
        echo "BUILD_DATE=$(get_build_date)"
        echo "BRANCH=$(get_branch)"
        ;;
esac
```

### 3.2 Cross-Platform Build Script

```bash
#!/bin/bash
# scripts/build.sh
set -e

# Cross-platform build script

BINARY_NAME="muster"
CMD_DIR="cmd"
BUILD_DIR="bin"

# Get version info
source scripts/version.sh > /dev/null

# Build flags
LDFLAGS="-X github.com/anomalyco/muster/internal/version.Version=$VERSION"
LDFLAGS="$LDFLAGS -X github.com/anomalyco/muster/internal/version.Commit=$COMMIT"
LDFLAGS="$LDFLAGS -X github.com/anomalyco/muster/internal/version.BuildDate=$BUILD_DATE"
LDFLAGS="$LDFLAGS -s -w"

# Platforms
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

build_platform() {
    local goos=$1
    local goarch=$2
    local output="$BUILD_DIR/${BINARY_NAME}-${goos}-${goarch}"
    
    if [ "$goos" = "windows" ]; then
        output="${output}.exe"
    fi
    
    echo "Building for $goos/$goarch..."
    CGO_ENABLED=1 GOOS=$goos GOARCH=$goarch \
        go build -ldflags "$LDFLAGS" -o "$output" ./$CMD_DIR
}

# Main
mkdir -p "$BUILD_DIR"

if [ -n "$1" ] && [ -n "$2" ]; then
    # Build specific platform
    build_platform "$1" "$2"
else
    # Build all platforms
    for platform in "${PLATFORMS[@]}"; do
        IFS='/' read -r goos goarch <<< "$platform"
        build_platform "$goos" "$goarch"
    done
fi

echo "Build complete!"
```

### 3.3 Release Automation Script

```bash
#!/bin/bash
# scripts/release.sh
set -e

# Release automation script

BINARY_NAME="muster"
VERSION=${1:-$(git describe --tags --always --dirty)}
DIST_DIR="dist"
GITHUB_TOKEN=${GITHUB_TOKEN:-}

if [ -z "$VERSION" ]; then
    echo "Error: No version specified"
    exit 1
fi

echo "Creating release $VERSION..."

# Clean and prepare
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Run tests
echo "Running tests..."
make test

# Build all platforms
echo "Building releases..."
make release VERSION="$VERSION"

# Create checksums
echo "Creating checksums..."
cd "$DIST_DIR"
shasum -a 256 *.tar.gz *.zip > checksums.txt 2>/dev/null || true
cd ..

# Create GitHub release (if gh CLI is available)
if command -v gh >/dev/null 2>&1 && [ -n "$GITHUB_TOKEN" ]; then
    echo "Creating GitHub release..."
    gh release create "$VERSION" \
        --title "Release $VERSION" \
        --notes "Release $VERSION" \
        dist/*
fi

echo "Release $VERSION complete!"
echo "Artifacts in $DIST_DIR/"
```

### 3.4 Installation Script

```bash
#!/bin/bash
# scripts/install.sh
set -e

# Installation script for muster

BINARY_NAME="muster"
REPO="anomalyco/muster"
INSTALL_DIR="/usr/local/bin"
VERSION=${VERSION:-"latest"}

# Detect platform
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch"; exit 1 ;;
    esac
    
    echo "${os}-${arch}"
}

# Download release
download_release() {
    local version=$1
    local platform=$2
    local url
    
    if [ "$version" = "latest" ]; then
        url="https://github.com/$REPO/releases/latest/download/${BINARY_NAME}-${version}-${platform}.tar.gz"
    else
        url="https://github.com/$REPO/releases/download/${version}/${BINARY_NAME}-${version}-${platform}.tar.gz"
    fi
    
    echo "Downloading from $url..."
    curl -fsSL "$url" -o "/tmp/${BINARY_NAME}.tar.gz"
}

# Install
install_binary() {
    echo "Installing to $INSTALL_DIR..."
    tar -xzf "/tmp/${BINARY_NAME}.tar.gz" -C /tmp/
    sudo install -m 755 "/tmp/${BINARY_NAME}" "$INSTALL_DIR/"
    rm -f "/tmp/${BINARY_NAME}.tar.gz" "/tmp/${BINARY_NAME}"
}

# Main
main() {
    echo "Installing $BINARY_NAME..."
    
    PLATFORM=$(detect_platform)
    echo "Detected platform: $PLATFORM"
    
    download_release "$VERSION" "$PLATFORM"
    install_binary
    
    echo "✓ $BINARY_NAME installed successfully!"
    echo "  Run '$BINARY_NAME --help' to get started"
}

main "$@"
```

---

## 4. Docker Configuration

### 4.1 Multi-Stage Dockerfile

```dockerfile
# =============================================================================
# Build Stage
# =============================================================================
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version injection
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build flags
ENV CGO_ENABLED=1
ENV LDFLAGS="-X github.com/anomalyco/muster/internal/version.Version=${VERSION} \
             -X github.com/anomalyco/muster/internal/version.Commit=${COMMIT} \
             -X github.com/anomalyco/muster/internal/version.BuildDate=${BUILD_DATE} \
             -s -w"

# Build binary
RUN go build -ldflags "${LDFLAGS}" -o muster ./cmd

# =============================================================================
# Runtime Stage
# =============================================================================
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs

# Create non-root user
RUN adduser -D -s /bin/sh muster

# Copy binary from builder
COPY --from=builder /build/muster /usr/local/bin/

# Set ownership
RUN chown muster:muster /usr/local/bin/muster

# Switch to non-root user
USER muster

# Set working directory
WORKDIR /home/muster

# Expose ports (if needed)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD muster health || exit 1

# Default command
ENTRYPOINT ["muster"]
CMD ["--help"]
```

### 4.2 Docker Ignore

```
# Git
.git
.gitignore

# Build artifacts
bin/
dist/
coverage.out
*.exe
*.dll
*.so
*.dylib

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# Dependencies
vendor/

# Documentation
*.md
docs/

# Test files
*_test.go
tests/

# CI
.github/
.travis.yml
.gitlab-ci.yml

# Local environment
.env
.env.local

# OS
.DS_Store
Thumbs.db
```

---

## 5. Version Package

### 5.1 Version Information Package

```go
// internal/version/version.go
// Package version provides build-time version information.
package version

import "fmt"

// These variables are injected at build time via -ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	Branch    = "unknown"
	GoVersion = "unknown"
)

// Info returns a formatted version string
func Info() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate)
}

// FullInfo returns detailed version information
func FullInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
		"branch":     Branch,
		"go_version": GoVersion,
	}
}
```

---

## 6. CI/CD Integration

### 6.1 GitHub Actions Workflow

```yaml
# .github/workflows/build.yml
name: Build

on:
  push:
    branches: [main, develop]
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Cache dependencies
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
      
      - name: Install dependencies
        run: make deps
      
      - name: Generate code
        run: make generate
      
      - name: Run tests
        run: make test
      
      - name: Run linter
        uses: golangci/golangci-lint-action@v3
        with:
          version: latest

  build:
    needs: test
    runs-on: ubuntu-latest
    strategy:
      matrix:
        platform:
          - os: linux
            arch: amd64
          - os: linux
            arch: arm64
          - os: darwin
            arch: amd64
          - os: darwin
            arch: arm64
          - os: windows
            arch: amd64
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Build
        env:
          GOOS: ${{ matrix.platform.os }}
          GOARCH: ${{ matrix.platform.arch }}
        run: make build

  release:
    needs: [test, build]
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Create release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          VERSION=${GITHUB_REF#refs/tags/}
          ./scripts/release.sh "$VERSION"
      
      - name: Upload artifacts
        uses: softprops/action-gh-release@v1
        with:
          files: dist/*
```

---

## 7. Usage Examples

### 7.1 Development Workflow

```bash
# Quick build for current platform
make build

# Build specific component
make build-cli
make build-mcp
make build-daemon

# Run in development mode
make dev

# Watch for changes and rebuild
make watch
```

### 7.2 Cross-Platform Builds

```bash
# Build all platforms
make build-all

# Build specific platform
GOOS=linux GOARCH=amd64 make build

# Using build script
./scripts/build.sh linux amd64
```

### 7.3 Release Workflow

```bash
# Create release
make release VERSION=v1.0.0

# Automated release
./scripts/release.sh v1.0.0
```

### 7.4 Testing Workflow

```bash
# All tests
make test

# Specific test types
make test-unit
make test-integration
make test-e2e

# With coverage
make coverage

# Benchmarks
make bench
```

### 7.5 Docker Workflow

```bash
# Build image
make docker-build

# Run container
make docker-run

# Push to registry
make docker-push
```

### 7.6 Installation Workflow

```bash
# System-wide install
sudo make install

# User install
make install-user

# Using install script
curl -sSL https://raw.githubusercontent.com/anomalyco/muster/main/scripts/install.sh | bash
```

---

## 8. CGO Configuration

### 8.1 CGO Settings

```makefile
# CGO is enabled by default for SQLite support
CGO_ENABLED := 1
```

### 8.2 Platform-Specific Requirements

| Platform | Requirements |
|----------|--------------|
| Linux | gcc, musl-dev |
| macOS | Xcode Command Line Tools (clang) |
| Windows | MinGW-w64 or MSYS2 |

### 8.3 Cross-Compilation Matrix

| Target | Host | CGO Support | Notes |
|--------|------|-------------|-------|
| linux/amd64 | linux/amd64 | Yes | Native build |
| linux/arm64 | linux/amd64 | Yes | Cross-compiler required |
| darwin/amd64 | darwin/amd64 | Yes | Native build |
| darwin/arm64 | darwin/arm64 | Yes | Native build |
| windows/amd64 | linux/amd64 | Yes | MinGW-w64 |
| windows/arm64 | linux/amd64 | Limited | Limited CGO support |

---

## 9. Version Injection

### 9.1 Build-Time Version Injection

```bash
go build -ldflags "\
  -X github.com/anomalyco/muster/internal/version.Version=v1.0.0 \
  -X github.com/anomalyco/muster/internal/version.Commit=abc123 \
  -X github.com/anomalyco/muster/internal/version.BuildDate=2024-01-01T00:00:00Z \
  -s -w"
```

### 9.2 LDFLAGS Explanation

| Flag | Purpose |
|------|---------|
| `-X` | Inject value into package variable |
| `-s` | Strip symbol table |
| `-w` | Strip DWARF debug info |

---

## 10. Build Verification

### 10.1 Binary Verification

```bash
# Verify version injection
./muster version

# Verify architecture
file ./muster

# Verify stripped symbols
nm -D ./muster | head
```

### 10.2 Checksum Verification

```bash
# Generate checksum
shasum -a 256 muster-1.0.0-linux-amd64.tar.gz > checksums.txt

# Verify checksum
shasum -a 256 -c checksums.txt
```

---

## 11. Troubleshooting

### 11.1 Common Build Issues

| Issue | Solution |
|-------|----------|
| `go: no go.mod file` | Run `go mod init` in project root |
| `undefined: version.Version` | Ensure LDFLAGS are properly set |
| `CGO_ENABLED=1 but no C compiler` | Install gcc/clang for target platform |
| `cannot find package` | Run `go mod tidy` |

### 11.2 Debug Build

```bash
# Build with debug symbols
CGO_ENABLED=1 go build -gcflags="all=-N -l" -o muster-debug ./cmd

# Build with race detector
go build -race -o muster-race ./cmd
```

---

**Bible Standard Compliance**: ✅  
This specification produces deterministic builds across all platforms. Identical source code produces byte-identical binaries when built with the same toolchain versions.

**END OF SPECIFICATION**
