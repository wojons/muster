# Build Specification

## Overview

Complete build system including Makefile, CI/CD pipeline, release automation, and distribution.

## 1. Project Structure

```
openapi-cli/
├── Makefile
├── Dockerfile
├── .github/
│   └── workflows/
│       ├── ci.yaml
│       ├── release.yaml
│       └── docker.yaml
├── scripts/
│   ├── build.sh
│   ├── release.sh
│   ├── install.sh
│   └── sign.sh
├── dist/
│   └── ...
├── bin/
│   └── ...
└── go.mod
```

## 2. Makefile

```makefile
# Makefile

# Variables
BINARY_NAME=openapi-cli
VERSION=$(shell git describe --tags --always --dirty)
COMMIT=$(shell git rev-parse --short HEAD)
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Platforms
PLATFORMS=darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build build-all test clean install uninstall release docker

# Default target
all: build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/cli
	go build $(LDFLAGS) -o bin/$(BINARY_NAME)-mcp ./cmd/mcp
	go build $(LDFLAGS) -o bin/$(BINARY_NAME)-daemon ./cmd/daemon

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1) \
		GOARCH=$$(echo $$platform | cut -d/ -f2) \
		go build $(LDFLAGS) -o dist/$(BINARY_NAME)-$$(echo $$platform | tr / -) ./cmd/cli; \
		echo "Built: dist/$(BINARY_NAME)-$$(echo $$platform | tr / -)"; \
	done

# Development build (faster)
dev:
	go build -o bin/$(BINARY_NAME) ./cmd/cli

# Install dependencies
deps:
	go mod download
	go mod tidy

# Generate code (mocks, sqlc, etc.)
generate:
	go generate ./...
	go run github.com/sqlc-dev/sqlc/cmd/sqlc generate
	go run github.com/vektra/mockery/v2 --all --inpackage

# Run tests
test:
	go test -v -short ./...

test-all:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

# Benchmark
bench:
	go test -bench=. -benchmem ./...

# Lint
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run

# Format
fmt:
	go fmt ./...
	go run mvdan.cc/gofumpt -w .

# Security scan
security:
	go run github.com/securego/gosec/v2/cmd/gosec ./...
	go run github.com/govulncheck/govulncheck ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf dist/
	rm -f coverage.out
	rm -f coverage.html

# Install locally
install: build
	cp bin/$(BINARY_NAME) $(GOPATH)/bin/
	cp bin/$(BINARY_NAME)-mcp $(GOPATH)/bin/
	@echo "Installed to $(GOPATH)/bin/"

uninstall:
	rm -f $(GOPATH)/bin/$(BINARY_NAME)
	rm -f $(GOPATH)/bin/$(BINARY_NAME)-mcp

# Create release binaries
release: clean
	@echo "Creating release $(VERSION)..."
	mkdir -p dist
	for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1) \
		GOARCH=$$(echo $$platform | cut -d/ -f2) \
		OUTPUT=dist/$(BINARY_NAME)-$$(echo $$platform | tr / -)-$(VERSION); \
		go build $(LDFLAGS) -o $$OUTPUT ./cmd/cli; \
		upx -q $$OUTPUT 2>/dev/null || true; \
	done
	@echo "Release binaries created in dist/"

# Sign release binaries
sign: release
	@scripts/sign.sh

# Create Docker image
docker:
	docker build -t openapi-cli:$(VERSION) .
	docker tag openapi-cli:$(VERSION) openapi-cli:latest

# Docker push
docker-push: docker
	docker push openapi-cli:$(VERSION)
	docker push openapi-cli:latest

# Database migrations
migrate-up:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "sqlite3://$(HOME)/.local/share/openapi-cli/cache.db" up

migrate-down:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "sqlite3://$(HOME)/.local/share/openapi-cli/cache.db" down

migrate-create:
	@read -p "Migration name: " name; \
	go run github.com/golang-migrate/migrate/v4/cmd/migrate create -ext sql -dir migrations -seq $$name

# Development tools
setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/vektra/mockery/v2@latest
	go install mvdan.cc/gofumpt@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Help
help:
	@echo "Available targets:"
	@echo "  build       - Build for current platform"
	@echo "  build-all   - Build for all platforms"
	@echo "  dev         - Fast development build"
	@echo "  test        - Run unit tests"
	@echo "  test-all    - Run all tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  lint        - Run linters"
	@echo "  fmt         - Format code"
	@echo "  security    - Run security scans"
	@echo "  clean       - Clean build artifacts"
	@echo "  install     - Install locally"
	@echo "  release     - Create release binaries"
	@echo "  docker      - Build Docker image"
	@echo "  setup       - Install dev tools"
```

## 3. Go Module

```go
// go.mod
module github.com/openapi-cli/core

go 1.21

require (
    // CLI
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0
    github.com/knadh/koanf v1.5.4
    
    // MCP
    github.com/mcp-sdk/mcp-go v0.5.0
    
    // WASM
    github.com/bytecodealliance/wasmtime-go v14.0.0
    
    // DSL
    go.starlark.net v0.0.0-20231121155337-90ade8b19d09
    
    // HTTP
    github.com/go-resty/resty/v2 v2.11.0
    
    // Database
    github.com/sqlc-dev/sqlc v1.24.0
    github.com/golang-migrate/migrate/v4 v4.17.0
    github.com/mattn/go-sqlite3 v1.14.19
    github.com/lib/pq v1.10.9
    github.com/go-sql-driver/mysql v1.7.1
    
    // Secrets
    github.com/99designs/keyring v1.2.2
    
    // Auth
    golang.org/x/oauth2 v0.15.0
    github.com/golang-jwt/jwt/v5 v5.2.0
    
    // Signing
    golang.org/x/crypto v0.17.0
    
    // Config
    github.com/knadh/koanf v2.0.0
    
    // Observability
    github.com/prometheus/client_golang v1.18.0
    go.opentelemetry.io/otel v1.21.0
    go.opentelemetry.io/otel/exporters/jaeger v1.17.0
    github.com/rs/zerolog v1.31.0
    
    // Utilities
    github.com/hashicorp/go-version v1.6.0
    github.com/creativeprojects/go-selfupdate v1.1.0
    github.com/charmbracelet/log v0.3.1
    github.com/charmbracelet/spinner v1.1.0
    github.com/google/uuid v1.5.0
    
    // Testing
    github.com/stretchr/testify v1.8.4
)
```

## 4. CI/CD Pipeline

```yaml
# .github/workflows/ci.yaml

name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v3
        with:
          version: latest
          args: --timeout=5m

  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.20', '1.21']
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
      
      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
      
      - name: Download dependencies
        run: go mod download
      
      - name: Run tests
        run: go test -race -coverprofile=coverage.out ./...
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: coverage.out

  build:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Build
        run: make build
      
      - name: Build all platforms
        run: make build-all
      
      - name: Upload artifacts
        uses: actions/upload-artifact@v3
        with:
          name: binaries
          path: dist/

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Gosec Security Scanner
        uses: securego/gosec@master
        with:
          args: '-no-fail -fmt sarif -out results.sarif ./...'
      
      - name: Run govulncheck
        uses: golang/govulncheck-action@v1
```

```yaml
# .github/workflows/release.yaml

name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Build release binaries
        run: make release
      
      - name: Sign binaries
        run: scripts/sign.sh
        env:
          SIGNING_KEY: ${{ secrets.SIGNING_KEY }}
      
      - name: Generate checksums
        run: |
          cd dist
          sha256sum * > checksums.txt
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/*
          body_path: CHANGELOG.md
          draft: false
          prerelease: ${{ contains(github.ref, 'beta') || contains(github.ref, 'alpha') }}
```

```yaml
# .github/workflows/docker.yaml

name: Docker

on:
  push:
    branches: [main]
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      
      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}
      
      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: openapi-cli/core
          tags: |
            type=ref,event=branch
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
      
      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          platforms: linux/amd64,linux/arm64
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

## 5. Dockerfile

```dockerfile
# Dockerfile

# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openapi-cli ./cmd/cli
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openapi-cli-mcp ./cmd/mcp

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN adduser -D -s /bin/sh openapi-cli

# Copy binaries
COPY --from=builder /build/bin/openapi-cli /usr/local/bin/
COPY --from=builder /build/bin/openapi-cli-mcp /usr/local/bin/

# Set ownership
RUN chown openapi-cli:openapi-cli /usr/local/bin/openapi-cli*

# Switch to non-root user
USER openapi-cli

# Set entrypoint
ENTRYPOINT ["openapi-cli"]
CMD ["--help"]

# Labels
LABEL org.opencontainers.image.title="OpenAPI CLI"
LABEL org.opencontainers.image.description="Dynamic CLI for OpenAPI specifications"
LABEL org.opencontainers.image.source="https://github.com/openapi-cli/core"
```

## 6. Installation Script

```bash
#!/bin/bash
# scripts/install.sh

set -e

REPO="openapi-cli/core"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $OS in
        linux)
            PLATFORM="linux"
            ;;
        darwin)
            PLATFORM="darwin"
            ;;
        mingw*|msys*|cygwin*)
            PLATFORM="windows"
            INSTALL_DIR="/usr/bin"
            ;;
        *)
            echo "Unsupported OS: $OS"
            exit 1
            ;;
    esac
    
    case $ARCH in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            echo "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    echo "${PLATFORM}-${ARCH}"
}

# Get latest version
get_latest_version() {
    curl -s https://api.github.com/repos/${REPO}/releases/latest | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/'
}

# Download binary
download_binary() {
    local version=$1
    local platform=$2
    
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${version}/openapi-cli-${platform}-${version}"
    
    echo "Downloading ${version} for ${platform}..."
    curl -fsSL -o /tmp/openapi-cli "${DOWNLOAD_URL}"
    
    # Verify checksum if available
    curl -fsSL -o /tmp/checksums.txt "https://github.com/${REPO}/releases/download/${version}/checksums.txt"
    
    if command -v shasum &> /dev/null; then
        EXPECTED=$(grep "openapi-cli-${platform}-${version}$" /tmp/checksums.txt | awk '{print $1}')
        ACTUAL=$(shasum -a 256 /tmp/openapi-cli | awk '{print $1}')
        
        if [ "$EXPECTED" != "$ACTUAL" ]; then
            echo "Checksum verification failed!"
            exit 1
        fi
    fi
}

# Install binary
install_binary() {
    chmod +x /tmp/openapi-cli
    
    if [ -w "$INSTALL_DIR" ]; then
        mv /tmp/openapi-cli "$INSTALL_DIR/openapi-cli"
    else
        echo "Installing to $INSTALL_DIR (requires sudo)..."
        sudo mv /tmp/openapi-cli "$INSTALL_DIR/openapi-cli"
    fi
    
    echo "Installed to $INSTALL_DIR/openapi-cli"
}

# Main
main() {
    PLATFORM=$(detect_platform)
    VERSION=${1:-$(get_latest_version)}
    
    echo "Installing OpenAPI CLI ${VERSION} for ${PLATFORM}..."
    
    download_binary "$VERSION" "$PLATFORM"
    install_binary
    
    # Verify installation
    if command -v openapi-cli &> /dev/null; then
        echo "Success! Version: $(openapi-cli --version)"
    else
        echo "Installation complete!"
        echo "Please ensure $INSTALL_DIR is in your PATH"
    fi
}

main "$@"
```

## 7. Release Script

```bash
#!/bin/bash
# scripts/release.sh

set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v1.2.3"
    exit 1
fi

# Validate version format
if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
    echo "Invalid version format. Expected: vX.Y.Z"
    exit 1
fi

# Check for uncommitted changes
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Uncommitted changes detected"
    exit 1
fi

# Update CHANGELOG
echo "Updating CHANGELOG..."
git-chglog -o CHANGELOG.md --next-tag $VERSION

# Commit changelog
git add CHANGELOG.md
git commit -m "chore(release): prepare $VERSION"

# Create tag
echo "Creating tag $VERSION..."
git tag -a $VERSION -m "Release $VERSION"

# Push
echo "Pushing to origin..."
git push origin main
git push origin $VERSION

echo "Release $VERSION created!"
echo "CI/CD will build and publish artifacts."
```

## 8. Self-Update Integration

```go
// internal/core/update/updater.go

package update

import (
    "context"
    "fmt"
    "os"
    "runtime"
)

// Updater handles self-updates
type Updater struct {
    source Source
}

// Source provides update information
type Source interface {
    GetLatestVersion(ctx context.Context) (VersionInfo, error)
    DownloadAsset(ctx context.Context, version string) ([]byte, error)
}

// VersionInfo describes an available version
type VersionInfo struct {
    Version     string
    URL         string
    PublishedAt string
    Notes       string
}

// Check checks for available updates
func (u *Updater) Check(ctx context.Context, currentVersion string) (*VersionInfo, error) {
    latest, err := u.source.GetLatestVersion(ctx)
    if err != nil {
        return nil, err
    }
    
    current, _ := semver.Parse(currentVersion)
    latestVer, _ := semver.Parse(latest.Version)
    
    if latestVer.LessThanOrEqual(current) {
        return nil, nil // No update available
    }
    
    return &latest, nil
}

// Update performs the self-update
func (u *Updater) Update(ctx context.Context, version string) error {
    // Download new binary
    data, err := u.source.DownloadAsset(ctx, version)
    if err != nil {
        return fmt.Errorf("failed to download: %w", err)
    }
    
    // Get current executable path
    exePath, err := os.Executable()
    if err != nil {
        return fmt.Errorf("failed to get executable path: %w", err)
    }
    
    // Create temp file
    tmpFile := exePath + ".tmp"
    if err := os.WriteFile(tmpFile, data, 0755); err != nil {
        return fmt.Errorf("failed to write temp file: %w", err)
    }
    
    // Replace current binary
    if err := os.Rename(tmpFile, exePath); err != nil {
        return fmt.Errorf("failed to replace binary: %w", err)
    }
    
    return nil
}

// GitHubSource implements Source for GitHub releases
type GitHubSource struct {
    Repo string
}

func (g *GitHubSource) GetLatestVersion(ctx context.Context) (VersionInfo, error) {
    // Implementation using GitHub API
    // GET https://api.github.com/repos/{repo}/releases/latest
    // ...
}
```

---

## Deterministic Guarantees

This build specification must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Build Outputs

Build processes must produce identical outputs across platforms:

- **Binary reproducibility**: Same source → same binary hash across build environments
- **Cross‑compilation**: Identical Go compiler flags across platforms
- **Dependency versioning**: Pinned dependency versions for deterministic builds

### 2. Platform‑Independent CI/CD Pipelines

CI/CD pipelines must behave identically across platforms:

- **GitHub Actions**: Same workflow execution on Linux, macOS, Windows runners
- **Docker builds**: Platform‑independent Dockerfile instructions
- **Artifact generation**: Identical artifact naming and structure

### 3. Deterministic Release Process

Release automation must be deterministic:

- **Version tagging**: Semantic versioning with deterministic changelog generation
- **Asset creation**: Identical binary names and checksums
- **Distribution**: Same distribution channels with identical metadata

### 4. Cross‑Platform Self‑Update Mechanism

Self‑update mechanism must work identically across platforms:

- **Executable path resolution**: Use `NormalizePath()` from `PLATFORM-BEHAVIOR.md`
- **File permissions**: Use `DefaultFilePerm()` from `PLATFORM-BEHAVIOR.md`
- **Update verification**: Deterministic signature verification

### 5. Testing Determinism

All build system tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestBuildDeterminism(t *testing.T) {
	// Verify identical build outputs
	// Verify identical CI/CD pipeline behavior
	// Verify identical release artifact generation
}
```

### 6. Integration with SDK Generation

When generating SDKs for 12 languages, build systems must:

- Produce identical package artifacts
- Use identical versioning schemes
- Implement identical release processes

**Verification**: Run `./scripts/validate‑determinism‑build.sh` to confirm cross‑platform consistency.

This build specification provides complete Makefile, CI/CD pipelines, Docker configuration, and release automation for Ralph Loop.
