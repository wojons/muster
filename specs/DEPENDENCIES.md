# Dependencies Management Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

---

## Overview

This specification defines the dependencies management strategy for the Muster OpenAPI CLI + MCP Server. Dependencies are managed through Go modules with strict version pinning to ensure deterministic builds, reproducible deployments, and maintainable security updates. The system categorizes dependencies by function and maintains compatibility across all supported platforms (Linux, macOS, Windows) and architectures (amd64, arm64).

A blind person can visualize this as a meticulously organized library with each book (dependency) catalogued by category, edition (version), and shelf location (module path), with a complete index showing which books are essential for each feature of the system.

---

## 1. Dependency Categories

### 1.1 Core Dependencies

| Category | Purpose | Examples |
|----------|---------|----------|
| CLI Framework | Command-line interface construction | cobra, viper |
| HTTP Client | API communication | standard library net/http |
| Database | Data persistence | sqlc, database drivers |
| Authentication | Security & auth flows | OAuth2, mTLS libraries |
| WASM Runtime | Plugin execution | wazero |
| Starlark DSL | Script interpreter | go-starlark |
| Logging | Structured logging | zerolog |
| Testing | Test framework | testify, mockgen |

### 1.2 Platform-Specific Dependencies

| Platform | Dependencies | Notes |
|----------|--------------|-------|
| Linux | libsecret (keychain), systemd bindings | CGO required |
| macOS | Security framework bindings | Native integration |
| Windows | Windows Credential Manager APIs | Syscall-based |
| All | SQLite (CGO), PostgreSQL, MySQL drivers | Database connectivity |

---

## 2. Go Module Configuration

### 2.1 Module Definition

```go
// go.mod
module github.com/anomalyco/muster

go 1.22

require (
    // CLI Framework
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    
    // Database
    github.com/lib/pq v1.10.9
    github.com/go-sql-driver/mysql v1.7.1
    github.com/mattn/go-sqlite3 v1.14.22
    github.com/pressly/goose/v3 v3.19.2
    github.com/sqlc-dev/sqlc v1.25.0
    
    // Authentication
    golang.org/x/oauth2 v0.18.0
    
    // WASM Runtime
    github.com/tetratelabs/wazero v1.6.0
    
    // Starlark DSL
    go.starlark.net v0.0.0-20240123142251-f86470692795
    
    // Logging
    github.com/rs/zerolog v1.32.0
    
    // Metrics
    github.com/prometheus/client_golang v1.19.0
    
    // Testing
    github.com/stretchr/testify v1.9.0
    github.com/golang/mock/mockgen v1.6.0
)
```

### 2.2 Dependency Management Commands

```bash
# Download and verify dependencies
make deps
# Runs: go mod download && go mod verify

# Update all dependencies
make deps-update
# Runs: go get -u ./... && go mod tidy

# Create vendor directory for offline builds
make vendor
# Runs: go mod vendor

# Tidy module dependencies
make tidy
# Runs: go mod tidy
```

---

## 3. Version Pinning Strategy

### 3.1 Pinning Rules

| Dependency Type | Pinning Strategy | Rationale |
|-----------------|------------------|-----------|
| Core functionality | Exact version (v1.2.3) | Reproducibility |
| Security libraries | Exact version + checksum | Supply chain security |
| Testing libraries | Minor version (v1.2.x) | Flexibility for bug fixes |
| Indirect dependencies | Locked via go.sum | Transitive dependency stability |

### 3.2 go.sum Integrity

```
# go.sum entries include cryptographic checksums
github.com/spf13/cobra v1.8.0 h1:7aJaZOLd+AZ9DhT+0EmX4U/...=
github.com/spf13/cobra v1.8.0/go.mod h1:WXLWApfZ/O/az1R6/...=
```

The `go.sum` file must be committed to version control. Any modification to dependencies updates `go.sum` automatically via `go mod tidy`.

### 3.3 Version Update Workflow

1. **Security Updates** (Critical):
   ```bash
   # Check for vulnerabilities
   go list -json -m all | nancy sleuth
   
   # Update vulnerable dependency
   go get github.com/example/pkg@latest
   go mod tidy
   ```

2. **Feature Updates** (Scheduled):
   ```bash
   # Quarterly dependency updates
   go get -u ./...
   go mod tidy
   make test  # Full test suite
   ```

3. **Emergency Patches**:
   ```bash
   # Specific version for hotfix
   go get github.com/example/pkg@v1.2.3-patch1
   go mod tidy
   ```

---

## 4. CGO Dependencies

### 4.1 CGO-Enabled Dependencies

```makefile
# Makefile configuration
CGO_ENABLED := 1

# Build with CGO for SQLite support
build:
    CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
```

### 4.2 Platform Build Requirements

| Platform | Required Packages | Installation |
|----------|-------------------|--------------|
| Linux | gcc, musl-dev, sqlite-dev | `apk add --no-cache gcc musl-dev sqlite-dev` (Alpine) |
| macOS | Xcode Command Line Tools | `xcode-select --install` |
| Windows | MinGW-w64 or MSYS2 | Chocolatey: `choco install mingw` |

### 4.3 Cross-Compilation with CGO

```bash
# Linux to Linux (same architecture)
CGO_ENABLED=1 go build -o muster-linux-amd64 ./cmd

# Linux to macOS (requires osxcross)
CC=o64-clang CXX=o64-clang++ CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build

# Linux to Windows (requires mingw)
CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build
```

---

## 5. Dependency Security

### 5.1 Vulnerability Scanning

```yaml
# .github/workflows/security.yml
name: Security Scan

on: [push, pull_request]

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Nancy vulnerability scanner
        uses: sonatypecommunity/nancy-github-action@main
        
      - name: Run govulncheck
        uses: golang/govulncheck-action@v1
```

### 5.2 Supply Chain Protection

| Control | Implementation |
|---------|----------------|
| Checksum verification | `go mod verify` on every build |
| Private registry | GOPROXY=direct for internal packages |
| Module proxy | Corporate proxy with caching |
| Signed commits | GPG-signed dependency updates |

### 5.3 License Compliance

```bash
# Check licenses of all dependencies
go-licenses check ./...

# Generate license report
go-licenses csv ./... > licenses.csv
```

Approved licenses: MIT, Apache-2.0, BSD-3-Clause, ISC  
Forbidden licenses: GPL-3.0, AGPL-3.0 (incompatible with commercial distribution)

---

## 6. Database Driver Dependencies

### 6.1 Supported Databases

| Database | Driver | CGO | Connection String |
|----------|--------|-----|-------------------|
| SQLite | mattn/go-sqlite3 | Yes | `file:/path/to/db.sqlite` |
| PostgreSQL | lib/pq | No | `postgres://user:pass@host/db` |
| MySQL | go-sql-driver/mysql | No | `user:pass@tcp(host)/db` |

### 6.2 Build Tags for Database Selection

```go
// +build sqlite

package db

import (
    _ "github.com/mattn/go-sqlite3"
)
```

```bash
# Build with specific database support
go build -tags=sqlite ./...
go build -tags=postgres ./...
go build -tags=mysql ./...
```

---

## 7. Development Dependencies

### 7.1 Required Tools

| Tool | Purpose | Installation |
|------|---------|--------------|
| golangci-lint | Linting | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| sqlc | SQL code generation | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| mockgen | Mock generation | `go install github.com/golang/mock/mockgen@latest` |
| goimports | Import formatting | `go install golang.org/x/tools/cmd/goimports@latest` |
| delve | Debugging | `go install github.com/go-delve/delve/cmd/dlv@latest` |

### 7.2 Tools Management

```go
// tools.go (build-tagged file)
//go:build tools
// +build tools

package tools

import (
    _ "github.com/golang/mock/mockgen"
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
```

---

## 8. Dependency Resolution

### 8.1 Minimal Version Selection (MVS)

Go uses MVS algorithm for dependency resolution:

```
Project requires:
  - A@v1.2.0
  - B@v1.1.0 (which requires A@v1.1.0)

MVS Resolution:
  - Selects A@v1.2.0 (maximum required version)
  - B is satisfied (needs >= v1.1.0)
```

### 8.2 Replace Directives

```go
// go.mod - Use local development version
replace github.com/anomalyco/muster/pkg/inventory => ./pkg/inventory

// Use fork with bugfix
replace github.com/example/pkg => github.com/anomalyco/pkg-fork v1.2.3-fix
```

**Warning**: Replace directives are ignored when module is used as dependency. Use only for local development.

---

## 9. Dependency Audit

### 9.1 Regular Audit Schedule

| Frequency | Action | Responsible |
|-----------|--------|-------------|
| Weekly | Automated vulnerability scan | CI/CD Pipeline |
| Monthly | License compliance check | Legal/DevOps |
| Quarterly | Dependency update review | Engineering Team |
| Annually | Full dependency audit | Security Team |

### 9.2 Audit Checklist

- [ ] All dependencies have acceptable licenses
- [ ] No known CVEs in dependency tree
- [ ] go.sum checksums match upstream
- [ ] No unused dependencies (checked via `go mod tidy`)
- [ ] CGO dependencies are justified and documented
- [ ] Replace directives are removed before release

---

## 10. Implementation Examples

### 10.1 Adding a New Dependency

```bash
# 1. Add dependency
go get github.com/new/dependency@v1.0.0

# 2. Verify checksum
go mod verify

# 3. Run tests
make test

# 4. Check license
go-licenses check github.com/new/dependency

# 5. Update documentation
# Add to this spec in appropriate section

# 6. Commit changes
git add go.mod go.sum
git commit -m "deps: add new/dependency@v1.0.0 for feature X"
```

### 10.2 Removing a Dependency

```bash
# 1. Remove imports from source code
# 2. Tidy modules
go mod tidy

# 3. Verify no references remain
grep -r "github.com/old/dependency" .

# 4. Commit changes
git add go.mod go.sum
git commit -m "deps: remove old/dependency, no longer needed"
```

### 10.3 Updating a Dependency

```bash
# 1. Update to specific version
go get github.com/example/pkg@v1.3.0

# 2. Check for breaking changes
# Review CHANGELOG, run tests

# 3. Run full test suite
make test

# 4. Security scan
make security-scan

# 5. Commit with clear message
git add go.mod go.sum
git commit -m "deps: upgrade example/pkg v1.2.0 → v1.3.0"
```

---

## 11. Troubleshooting

### 11.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `go: inconsistent vendoring` | Vendor directory out of sync | `rm -rf vendor && go mod vendor` |
| `cannot find module` | Missing proxy or network | Check GOPROXY, `go env GOPROXY` |
| `checksum mismatch` | Corrupted go.sum | `go clean -modcache && go mod download` |
| `CGO compilation failed` | Missing C compiler | Install gcc/Clang for platform |
| `version conflict` | Multiple versions required | Use `go mod graph` to analyze |

### 11.2 Module Cache Management

```bash
# Clean module cache
go clean -modcache

# Verify cache integrity
go mod verify

# Download all dependencies
go mod download

# Vendor for offline builds
go mod vendor
```

---

## 12. References

- **go.mod Reference**: https://go.dev/ref/mod
- **Module Proxy**: https://proxy.golang.org
- **Vulnerability DB**: https://pkg.go.dev/vuln
- **License Scanner**: https://github.com/google/go-licenses

---

**Bible Standard Compliance**: ✅  
This specification is deterministic and implementation-agnostic. All dependency management operations produce identical results across platforms and build environments.

**END OF SPECIFICATION**
