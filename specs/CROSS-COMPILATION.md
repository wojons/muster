# Cross-Compilation Guide

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Build Matrix

| Platform | Architecture | CGO | Status |
|----------|-------------|-----|--------|
| Linux | amd64 | No | Supported |
| Linux | arm64 | No | Supported |
| Darwin | amd64 | Yes | Supported |
| Darwin | arm64 | Yes | Supported |
| Windows | amd64 | No | Supported |
| FreeBSD | amd64 | No | Experimental |

---

## Basic Cross-Compilation

```bash
# Linux AMD64 (no CGO)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/muster-linux-amd64 ./cmd/cli

# Linux ARM64 (no CGO)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/muster-linux-arm64 ./cmd/cli

# macOS AMD64 (with CGO for keychain)
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -o bin/muster-darwin-amd64 ./cmd/cli

# macOS ARM64 (with CGO for keychain)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -o bin/muster-darwin-arm64 ./cmd/cli

# Windows AMD64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/muster-windows-amd64.exe ./cmd/cli
```

---

## CGO Considerations for SQLite

SQLite requires CGO. For cross-platform builds:

```bash
# Use pure Go SQLite alternative for cross-compilation
go mod edit -replace github.com/mattn/go-sqlite3=github.com/glebarez/go-sqlite@v1.21.2

# Or disable SQLite for specific builds
go build -tags nosqlite -o bin/muster-nosqlite ./cmd/cli
```

---

## Platform-Specific Build Tags

```go
//go:build darwin
// +build darwin

package keychain

import "github.com/zalando/go-keyring"

func GetPassword(service, user string) (string, error) {
    return keyring.Get(service, user)
}
```

```go
//go:build !darwin
// +build !darwin

package keychain

func GetPassword(service, user string) (string, error) {
    return "", ErrUnsupportedPlatform
}
```

---

## Makefile Targets

```makefile
.PHONY: build-all build-linux build-darwin build-windows

BINARY_NAME=muster
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -s -w"

build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/cli
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/cli

build-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/cli
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/cli

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/cli
```

---

## Docker-Based Cross-Compilation

```dockerfile
# Dockerfile.build
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o muster ./cmd/cli

FROM scratch
COPY --from=builder /build/muster /muster
ENTRYPOINT ["/muster"]
```

```bash
# Build for multiple platforms with Docker
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 -t muster:latest .
```

---

## Release Matrix

| Asset | Platform | Filename |
|-------|----------|----------|
| Linux x64 | linux/amd64 | muster_1.0.0_linux_amd64.tar.gz |
| Linux ARM64 | linux/arm64 | muster_1.0.0_linux_arm64.tar.gz |
| macOS x64 | darwin/amd64 | muster_1.0.0_darwin_amd64.tar.gz |
| macOS ARM64 | darwin/arm64 | muster_1.0.0_darwin_arm64.tar.gz |
| Windows x64 | windows/amd64 | muster_1.0.0_windows_amd64.zip |
