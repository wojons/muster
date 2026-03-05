# Deployment Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Deployment & Operations  
**Referenced Files**: 25  

## Overview

Complete deployment specification covering build system, cross-compilation, release automation, CI/CD pipelines, Docker deployment, platform packages, Kubernetes orchestration, monitoring, backup/recovery, observability, health checks, and alerting systems. Ensures deterministic deployments with blind-person visualization compatibility.

## 1. Build System

### 1.1 Build Architecture

```
/
├── Makefile                    # Main build orchestration
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

### 1.2 Makefile Targets

**Development Builds:**
- `make build` - Build for current platform
- `make build-cli` - Build CLI only
- `make build-mcp` - Build MCP server only
- `make build-daemon` - Build daemon only
- `make build-all` - Build for all platforms

**Release Builds:**
- `make release` - Create release builds with tests and linting
- `make release-local` - Create release for current platform only

**Installation:**
- `make install` - Install to system (/usr/local/bin)
- `make install-user` - Install to user bin directory
- `make uninstall` - Uninstall from system

**Testing:**
- `make test` - Run all tests with race detection
- `make test-unit` - Run unit tests only
- `make test-integration` - Run integration tests
- `make test-e2e` - Run end-to-end tests
- `make bench` - Run benchmarks
- `make coverage` - Generate coverage report

**Code Quality:**
- `make lint` - Run all linters (golangci-lint)
- `make fmt` - Format code with go fmt
- `make vet` - Run go vet
- `make generate` - Run all code generation
- `make generate-sqlc` - Generate SQL code with sqlc
- `make generate-mocks` - Generate mocks with mockgen

**Docker:**
- `make docker-build` - Build Docker image
- `make docker-run` - Run in Docker container
- `make docker-push` - Push Docker image to registry

**Cleanup:**
- `make clean` - Clean build artifacts
- `make distclean` - Clean all generated files

### 1.3 Version Injection

Version information injected at build time via `-ldflags`:

```bash
go build -ldflags "\
  -X github.com/anomalyco/muster/internal/version.Version=$(VERSION) \
  -X github.com/anomalyco/muster/internal/version.Commit=$(COMMIT) \
  -X github.com/anomalyco/muster/internal/version.BuildDate=$(BUILD_DATE) \
  -X github.com/anomalyco/muster/internal/version.Branch=$(BRANCH) \
  -X github.com/anomalyco/muster/internal/version.GoVersion=$(GO_VERSION) \
  -s -w"
```

**Reference**: `BUILD-SYSTEM.md`

## 2. Cross-Compilation

### 2.1 Build Matrix

| Platform | Architecture | CGO | Status |
|----------|-------------|-----|--------|
| Linux | amd64 | No | Supported |
| Linux | arm64 | No | Supported |
| Darwin | amd64 | Yes | Supported |
| Darwin | arm64 | Yes | Supported |
| Windows | amd64 | No | Supported |
| FreeBSD | amd64 | No | Experimental |

### 2.2 Cross-Compilation Commands

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

### 2.3 CGO Considerations

SQLite requires CGO. For cross-platform builds:

```bash
# Use pure Go SQLite alternative for cross-compilation
go mod edit -replace github.com/mattn/go-sqlite3=github.com/glebarez/go-sqlite@v1.21.2

# Or disable SQLite for specific builds
go build -tags nosqlite -o bin/muster-nosqlite ./cmd/cli
```

### 2.4 Platform-Specific Build Tags

```go
//go:build darwin
// +build darwin

package keychain

import "github.com/zalando/go-keyring"

func GetPassword(service, user string) (string, error) {
    return keyring.Get(service, user)
}
```

**Reference**: `CROSS-COMPILATION.md`

## 3. Release Process

### 3.1 Semantic Versioning

Format: `MAJOR.MINOR.PATCH[-prerelease][+build]`

- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes
- **prerelease**: `alpha`, `beta`, `rc1`

Examples: `v1.0.0`, `v1.2.0-beta.1`, `v2.0.0-rc.1`

### 3.2 Pre-Release Checklist

```markdown
## Release v1.2.0 Checklist

- [ ] All tests passing (`make test`)
- [ ] Linting clean (`make lint`)
- [ ] Version bumped in `version.go`
- [ ] CHANGELOG.md updated
- [ ] Documentation updated
- [ ] Migration guide written (if breaking changes)
- [ ] Security scan passed (`make security-scan`)
- [ ] Performance benchmarks acceptable
- [ ] Manual QA completed
```

### 3.3 Git Tagging Process

```bash
# 1. Update version
$EDITOR version.go

# 2. Commit version bump
git add version.go
git commit -m "chore: bump version to v1.2.0"

# 3. Create annotated tag
git tag -a v1.2.0 -m "Release v1.2.0 - Feature release"

# 4. Push to remote
git push origin main
git push origin v1.2.0

# 5. Verify tag
git show v1.2.0
```

### 3.4 Homebrew Formula Update

```ruby
# Formula template: muster.rb
class Muster < Formula
  desc "OpenAPI CLI tool for API testing and exploration"
  homepage "https://github.com/muster/muster"
  version "1.2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_darwin_arm64.tar.gz"
      sha256 "ABC123..."
    else
      url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_darwin_amd64.tar.gz"
      sha256 "DEF456..."
    end
  end

  on_linux do
    url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_linux_amd64.tar.gz"
    sha256 "GHI789..."
  end

  def install
    bin.install "muster"
  end
end
```

### 3.5 Docker Image Tagging

```bash
# Build and tag
VERSION=1.2.0
docker build -t muster/muster:$VERSION .
docker tag muster/muster:$VERSION muster/muster:latest

# Push to registry
docker push muster/muster:$VERSION
docker push muster/muster:latest

# Multi-arch build
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t muster/muster:$VERSION \
  -t muster/muster:latest \
  --push .
```

**Reference**: `RELEASE-PROCESS.md`

## 4. CI/CD Pipelines

### 4.1 Repository Structure

```
.github/
├── workflows/
│   ├── ci.yml           # Continuous Integration
│   ├── release.yml      # Automated Releases
│   ├── docker.yml       # Docker Build & Push
│   ├── security.yml     # Security Scanning
│   └── benchmark.yml    # Performance Tracking
└── scripts/
    └── update-homebrew.sh  # Homebrew formula updater
```

### 4.2 Continuous Integration (ci.yml)

**Triggers:**
- Push to main/master branches
- Push to tags (v*)
- Pull requests to main/master

**Jobs:**
1. **Lint** - Run golangci-lint with Go 1.21+
2. **Unit Tests** - Matrix across OS (ubuntu, macos, windows) and Go versions (1.21, 1.22)
3. **Integration Tests** - With Redis and PostgreSQL services
4. **Build** - Cross-platform builds for all target architectures
5. **CI Summary** - Aggregated status and Slack notifications

### 4.3 Automated Releases (release.yml)

**Triggers:** Push to tags matching `v[0-9]+.[0-9]+.[0-9]+`

**Workflow:**
1. **Build Release Binaries** - Matrix across all platforms (linux/darwin/windows, amd64/arm64)
2. **Create GitHub Release** - Generate changelog, upload artifacts
3. **Update Homebrew Formula** - Automatic formula updates to homebrew-tap repository
4. **Notify** - Slack notifications on success/failure

### 4.4 Docker Build & Push (docker.yml)

**Triggers:** Push to main/master, tags, PRs with Dockerfile changes

**Jobs:**
1. **Build and Test Image** - Build Docker image, run basic tests, security scan
2. **Push Multi-Arch Images** - Build and push multi-architecture images to Docker Hub and GitHub Container Registry
3. **Update Docker Hub Description** - Update README on Docker Hub for tagged releases

### 4.5 Security Scanning (security.yml)

**Triggers:** Push to main/master, PRs, daily schedule, manual dispatch

**Scans:**
- **govulncheck** - Go vulnerability checker
- **CodeQL** - Static analysis for security vulnerabilities
- **Dependency Review** - Check PR dependencies for vulnerabilities
- **Trivy** - Container and filesystem vulnerability scanning
- **Nancy** - Go dependency scanner
- **Gosec** - Go security checker

### 4.6 Performance Tracking (benchmark.yml)

**Triggers:** Push to main/master, PRs, manual dispatch

**Workflow:**
1. **Run Benchmarks** - Execute benchmarks with benchmem
2. **Compare with Previous** - For PRs, compare against main branch using benchstat
3. **Store Results** - Archive benchmark results in gh-pages branch
4. **Generate Trend Chart** - Create performance trend visualizations

**Reference**: `CI-CD-GITHUB-ACTIONS.md`

## 5. Docker Deployment

### 5.1 Multi-Stage Dockerfile

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=$(git describe --tags)" \
    -o muster \
    ./cmd/cli

# Final stage - Distroless
FROM gcr.io/distroless/static-debian11:nonroot

# Copy binary and certs
COPY --from=builder /build/muster /usr/local/bin/muster
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Non-root user
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/muster"]
```

### 5.2 Image Optimization

**Size Reduction Techniques:**
- Multi-stage builds separate build dependencies
- Use minimal base images (alpine, distroless)
- Single RUN layers with cleanup
- .dockerignore to exclude unnecessary files

**Build Command:**
```bash
docker build \
  --build-arg VERSION=$(git describe --tags) \
  --tag muster:latest \
  --tag muster:$(git describe --tags) \
  .
```

### 5.3 Layer Caching

```dockerfile
# Layer 1: Dependencies (cached if go.mod unchanged)
COPY go.mod go.sum ./
RUN go mod download

# Layer 2: Source (rebuilt on code changes)
COPY . .
RUN go build -o muster ./cmd/cli

# Layer 3: Runtime (minimal, static)
FROM scratch
COPY --from=builder /build/muster /muster
```

### 5.4 Security Scanning

```bash
# Trivy scan
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy image muster:latest

# Snyk scan
docker scan muster:latest

# Grype scan
grype muster:latest
```

**Reference**: `DOCKER-BUILD.md`

## 6. Docker Compose

### 6.1 Development Environment

```yaml
# docker-compose.dev.yaml
version: '3.8'

services:
  muster:
    build:
      context: .
      dockerfile: Dockerfile.dev
    container_name: muster-dev
    volumes:
      - .:/app
      - go-cache:/go/pkg/mod
      - go-build:/root/.cache/go-build
    working_dir: /app
    command: ["air", "-c", ".air.toml"]
    environment:
      - ENV=development
      - LOG_LEVEL=debug
    ports:
      - "8080:8080"
    networks:
      - muster-network

  # Hot reload for Go
  air:
    image: cosmtrek/air:latest
    volumes:
      - .:/app
    working_dir: /app

volumes:
  go-cache:
  go-build:

networks:
  muster-network:
    driver: bridge
```

### 6.2 PostgreSQL Service

```yaml
postgres:
  image: postgres:16-alpine
  container_name: muster-postgres
  environment:
    POSTGRES_USER: muster
    POSTGRES_PASSWORD: muster_dev
    POSTGRES_DB: muster_test
  volumes:
    - postgres-data:/var/lib/postgresql/data
    - ./scripts/init-db.sql:/docker-entrypoint-initdb.d/init.sql
  ports:
    - "5432:5432"
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U muster"]
    interval: 5s
    timeout: 5s
    retries: 5
```

### 6.3 Full Development Stack

```yaml
# docker-compose.full.yaml
version: '3.8'

services:
  muster:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://muster:muster_dev@postgres:5432/muster
      REDIS_URL: redis:6379
    ports:
      - "8080:8080"

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: muster
      POSTGRES_PASSWORD: muster_dev
      POSTGRES_DB: muster
    volumes:
      - postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]

volumes:
  postgres-data:
  redis-data:
```

**Reference**: `DOCKER-COMPOSE.md`

## 7. Platform Packages

### 7.1 Linux Packages

**Debian Package (.deb):**
```
muster_1.0.0_amd64/
├── DEBIAN/
│   ├── control
│   ├── preinst
│   ├── postinst
│   ├── prerm
│   └── postrm
└── usr/
    ├── bin/
    │   └── muster
    ├── share/
    │   ├── applications/
    │   │   └── muster.desktop
    │   ├── doc/
    │   │   └── muster/
    │   │       ├── changelog.gz
    │   │       ├── copyright
    │   │       └── README
    │   ├── man/
    │   │   └── man1/
    │   │       └── muster.1.gz
    │   └── mime/
    │       └── packages/
    │           └── muster.xml
    └── lib/
        └── systemd/
            └── system/
                ├── muster-daemon.service
                └── muster-daemon.socket
```

**RPM Package:**
```spec
# muster.spec
Name:           muster
Version:        1.0.0
Release:        1%{?dist}
Summary:        Modern API Client and Development Tool
License:        MIT
URL:            https://muster.sh
Source0:        %{name}-%{version}.tar.gz

BuildArch:      x86_64
Requires:       ca-certificates
Recommends:     gnome-keyring

%description
Muster is a powerful CLI tool for API development and testing.
Features OpenAPI integration, authentication management, and
extensible workflows.
```

**Alpine Package (APK):**
```bash
# APKBUILD
pkgname=muster
pkgver=1.0.0
pkgrel=0
pkgdesc="Modern API Client and Development Tool"
url="https://muster.sh"
arch="x86_64 aarch64"
license="MIT"
depends="ca-certificates"
install="muster.pre-install muster.post-install"
```

**Arch Linux PKGBUILD:**
```bash
# PKGBUILD
pkgname=muster
pkgver=1.0.0
pkgrel=1
pkgdesc="Modern API Client and Development Tool"
arch=('x86_64' 'aarch64')
url="https://muster.sh"
license=('MIT')
depends=('glibc' 'ca-certificates')
```

### 7.2 macOS Packages

**Homebrew Formula:**
```ruby
class Muster < Formula
  desc "Powerful connectivity and deployment tool"
  homepage "https://muster.io"
  url "https://github.com/muster/muster/archive/v1.0.0.tar.gz"
  sha256 "INSERT_SHA256_HERE"
  license "MIT"
  head "https://github.com/muster/muster.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos => :monterey

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/muster"
  end
end
```

**PKG Installer:**
- Custom macOS installer package with LaunchAgent integration
- Notarization with Apple Developer ID
- Signed with hardened runtime entitlements

**DMG Distribution:**
- Drag-and-drop installation experience
- Background image and custom layout
- Applications folder symlink

### 7.3 Windows Packages

**WiX MSI Installer:**
- Windows Installer XML (WiX) configuration
- Per-machine installation with system service
- Registry entries and PATH modification
- Uninstall support via Add/Remove Programs

**Chocolatey Package:**
```powershell
# chocolateyinstall.ps1
$packageName = 'muster'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$url64 = 'https://github.com/muster/muster/releases/download/v1.0.0/muster-windows-amd64.zip'
$checksum64 = 'A1B2C3D4E5F6789012345678901234567890ABCDEF1234567890ABCDEF123456'

Install-ChocolateyZipPackage @packageArgs
```

**Scoop Manifest:**
```json
{
  "version": "1.0.0",
  "description": "Model Context Protocol server for AI assistant integration",
  "homepage": "https://muster.dev",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/muster/muster/releases/download/v1.0.0/muster-windows-amd64.zip",
      "hash": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
    }
  },
  "bin": ["muster.exe", "musterd.exe"],
  "shortcuts": [["muster.exe", "Muster CLI"]]
}
```

**Winget Package:**
```yaml
PackageIdentifier: Muster.Muster
PackageVersion: 1.0.0
MinimumOSVersion: 10.0.17763.0
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/muster/muster/releases/download/v1.0.0/muster-windows-amd64.zip
    InstallerSha256: A1B2C3D4E5F6789012345678901234567890ABCDEF1234567890ABCDEF123456
    NestedInstallerType: portable
```

**Reference**: `PLATFORM-LINUX.md`, `PLATFORM-MACOS.md`, `PLATFORM-WINDOWS.md`

## 8. Kubernetes Deployment

### 8.1 Kubernetes Manifests

**Namespace:**
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: muster
  labels:
    name: muster
```

**ConfigMap:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muster-config
  namespace: muster
data:
  APP_ENV: "production"
  LOG_LEVEL: "info"
  METRICS_PORT: "9090"
```

**Secret:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: muster-secrets
  namespace: muster
type: Opaque
stringData:
  DATABASE_URL: "postgresql://user:pass@db:5432/muster"
  REDIS_URL: "redis://redis:6379"
  SECRET_KEY: "${SECRET_KEY}"
```

**Deployment:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muster-api
  namespace: muster
spec:
  replicas: 3
  selector:
    matchLabels:
      app: muster-api
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: muster-api
    spec:
      containers:
      - name: api
        image: muster/api:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        envFrom:
        - configMapRef:
            name: muster-config
        - secretRef:
            name: muster-secrets
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

**Service:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: muster-api
  namespace: muster
spec:
  selector:
    app: muster-api
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP
```

**Horizontal Pod Autoscaler:**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: muster-api-hpa
  namespace: muster
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: muster-api
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### 8.2 Deployment Commands

```bash
# Apply manifests
kubectl apply -f k8s/

# Check rollout status
kubectl rollout status deployment/muster-api -n muster

# View pods
kubectl get pods -n muster

# View logs
kubectl logs -f deployment/muster-api -n muster

# Rollback
kubectl rollout undo deployment/muster-api -n muster
```

**Reference**: `OPS-DEPLOYMENT.md`

## 9. Monitoring

### 9.1 Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: production

scrape_configs:
  - job_name: 'muster-api'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - muster
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
```

### 9.2 Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `http_requests_total` | Total HTTP requests | - |
| `request_duration_seconds` | Request latency | p95 > 500ms |
| `active_connections` | Active connections | > 1000 |
| `process_resident_memory_bytes` | Memory usage | > 512MB |
| `up` | Service health | = 0 |

### 9.3 Metric Types

**Counter:** Monotonically increasing cumulative metrics
```python
http_requests_total = Counter(
    'http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status_code']
)
```

**Gauge:** Single numerical values that can go up or down
```python
active_connections = Gauge(
    'active_connections',
    'Number of active connections',
    ['service']
)
```

**Histogram:** Samples observations and counts them in buckets
```python
request_duration_seconds = Histogram(
    'request_duration_seconds',
    'Request duration in seconds',
    ['endpoint'],
    buckets=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
)
```

**Summary:** Similar to histogram but calculates configurable quantiles
```python
request_latency = Summary(
    'request_latency_seconds',
    'Request latency',
    ['method']
)
```

**Reference**: `OPS-MONITORING.md`, `OPS-METRICS.md`## 10. Backup and Recovery

### 10.1 Database Backup

**Automated Backups:**
```bash
#!/bin/bash
# backup-database.sh

BACKUP_DIR="/backups/postgres"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="muster_db_${TIMESTAMP}.sql.gz"
RETENTION_DAYS=30

# Create backup
pg_dump -h ${DB_HOST} -U ${DB_USER} -d ${DB_NAME} | gzip > ${BACKUP_DIR}/${BACKUP_FILE}

# Verify backup
if gunzip -t ${BACKUP_DIR}/${BACKUP_FILE}; then
  echo "Backup successful: ${BACKUP_FILE}"
  
  # Upload to S3
  aws s3 cp ${BACKUP_DIR}/${BACKUP_FILE} s3://${S3_BUCKET}/postgres/
  
  # Cleanup old backups
  find ${BACKUP_DIR} -name "muster_db_*.sql.gz" -mtime +${RETENTION_DAYS} -delete
else
  echo "Backup failed: ${BACKUP_FILE}"
  exit 1
fi
```

**Kubernetes CronJob:**
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: database-backup
  namespace: muster
spec:
  schedule: "0 2 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:15-alpine
            command:
            - /bin/sh
            - -c
            - |
              pg_dump -h db -U postgres muster | gzip > /backup/muster_$(date +%Y%m%d).sql.gz
              aws s3 cp /backup/muster_$(date +%Y%m%d).sql.gz s3://muster-backups/postgres/
```

### 10.2 Redis Backup

```bash
#!/bin/bash
# backup-redis.sh

BACKUP_DIR="/backups/redis"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Trigger BGSAVE
redis-cli -h ${REDIS_HOST} BGSAVE

# Wait for save to complete
while redis-cli -h ${REDIS_HOST} LASTSAVE | grep -q "0"; do
  sleep 1
done

# Copy dump file
cp /var/lib/redis/dump.rdb ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb
aws s3 cp ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb s3://${S3_BUCKET}/redis/
```

### 10.3 File System Backup

```bash
#!/bin/bash
# backup-files.sh

SOURCE_DIRS="/var/lib/muster/uploads /var/log/muster"
BACKUP_DIR="/backups/files"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

tar czf ${BACKUP_DIR}/files_${TIMESTAMP}.tar.gz ${SOURCE_DIRS}
aws s3 cp ${BACKUP_DIR}/files_${TIMESTAMP}.tar.gz s3://${S3_BUCKET}/files/
```

### 10.4 Recovery Procedures

**Database Recovery:**
```bash
#!/bin/bash
# restore-database.sh

BACKUP_FILE=$1
DB_HOST=${DB_HOST:-localhost}
DB_NAME=${DB_NAME:-muster}

# Download from S3 if not local
if [[ ${BACKUP_FILE} == s3://* ]]; then
  aws s3 cp ${BACKUP_FILE} /tmp/restore.sql.gz
  BACKUP_FILE=/tmp/restore.sql.gz
fi

# Stop application
kubectl scale deployment muster-api --replicas=0

# Restore database
gunzip < ${BACKUP_FILE} | psql -h ${DB_HOST} -U postgres -d ${DB_NAME}

# Verify restore
psql -h ${DB_HOST} -U postgres -d ${DB_NAME} -c "SELECT COUNT(*) FROM users;"

# Restart application
kubectl scale deployment muster-api --replicas=3
```

### 10.5 Disaster Recovery

**RTO/RPO Targets:**
| Component | RTO | RPO |
|-----------|-----|-----|
| Database | 1 hour | 15 minutes |
| Redis | 30 minutes | 5 minutes |
| Files | 2 hours | 24 hours |

**Failover Procedure:**
```bash
# 1. Promote read replica to primary
aws rds promote-read-replica --db-instance-identifier muster-db-standby

# 2. Update application configuration
kubectl set env deployment/muster-api DATABASE_URL="new-primary-url"

# 3. Verify connectivity
kubectl exec -it deployment/muster-api -- python -c "import db; db.check_connection()"

# 4. Resume traffic
kubectl rollout restart deployment/muster-api
```

**Reference**: `OPS-BACKUP-RECOVERY.md`

## 11. Observability

### 11.1 Structured Logging

**Log Levels:**
| Level | Value | Usage |
|-------|-------|-------|
| DEBUG | 10 | Detailed debugging |
| INFO | 20 | General information |
| WARNING | 30 | Potential issues |
| ERROR | 40 | Error conditions |
| CRITICAL | 50 | System failure |

**JSON Format:**
```python
import json
import logging
from pythonjsonlogger import jsonlogger

class CustomJsonFormatter(jsonlogger.JsonFormatter):
    def add_fields(self, log_record, record, message_dict):
        super().add_fields(log_record, record, message_dict)
        log_record['timestamp'] = datetime.utcnow().isoformat()
        log_record['level'] = record.levelname
        log_record['logger'] = record.name
        log_record['source'] = f"{record.filename}:{record.lineno}"

formatter = CustomJsonFormatter('%(timestamp)s %(level)s %(name)s %(message)s')
handler = logging.StreamHandler()
handler.setFormatter(formatter)

logger = logging.getLogger('app')
logger.addHandler(handler)
logger.setLevel(logging.INFO)
```

**Log Rotation:**
```python
from logging.handlers import RotatingFileHandler, TimedRotatingFileHandler

# Size-based rotation
file_handler = RotatingFileHandler(
    'app.log',
    maxBytes=10*1024*1024,  # 10MB
    backupCount=5
)

# Time-based rotation (daily)
timed_handler = TimedRotatingFileHandler(
    'app.log',
    when='midnight',
    interval=1,
    backupCount=30
)
```

### 11.2 Distributed Tracing

**OpenTelemetry Setup:**
```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

provider = TracerProvider()
processor = BatchSpanProcessor(OTLPSpanExporter(endpoint="otel-collector:4317"))
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)

tracer = trace.get_tracer("muster.tracer")
```

**Span Creation:**
```python
with tracer.start_as_current_span("process-request") as span:
    span.set_attribute("http.method", "GET")
    span.set_attribute("http.url", "/api/v1/users")
    # ... process request ...
```

**Nested Spans:**
```python
with tracer.start_as_current_span("parent-operation") as parent:
    parent.set_attribute("operation.type", "user-creation")
    
    with tracer.start_as_current_span("validate-input") as child1:
        child1.set_attribute("validation.duration_ms", 45)
    
    with tracer.start_as_current_span("database-insert") as child2:
        child2.set_attribute("db.table", "users")
        child2.set_attribute("db.rows_affected", 1)
```

**Instrumentation:**
```python
# Flask
from opentelemetry.instrumentation.flask import FlaskInstrumentor
FlaskInstrumentor().instrument_app(app)

# FastAPI
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
FastAPIInstrumentor.instrument_app(app)

# Database
from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor
SQLAlchemyInstrumentor().instrument()

# HTTP Client
from opentelemetry.instrumentation.requests import RequestsInstrumentor
RequestsInstrumentor().instrument()
```

**Reference**: `OPS-LOGGING.md`, `OPS-TRACING.md`

## 12. Health Checks

### 12.1 Health Check Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/health/startup` | GET | Startup probe |

### 12.2 Implementation

```python
from fastapi import FastAPI, status
from pydantic import BaseModel
from datetime import datetime

app = FastAPI()

class HealthStatus(BaseModel):
    status: str
    timestamp: datetime
    version: str
    checks: dict = {}

@app.get("/health/live", status_code=status.HTTP_200_OK)
async def liveness_probe():
    """Liveness probe - Returns 200 if app is running"""
    return HealthStatus(
        status="healthy",
        timestamp=datetime.utcnow(),
        version="1.0.0"
    )

@app.get("/health/ready", status_code=status.HTTP_200_OK)
async def readiness_probe():
    """Readiness probe - Returns 200 if app can serve traffic"""
    checks = {
        "database": await check_database(),
        "cache": await check_cache()
    }
    
    all_healthy = all(c["status"] == "healthy" for c in checks.values())
    status_code = status.HTTP_200_OK if all_healthy else status.HTTP_503_SERVICE_UNAVAILABLE
    
    return HealthStatus(
        status="ready" if all_healthy else "not_ready",
        timestamp=datetime.utcnow(),
        version="1.0.0",
        checks=checks
    ), status_code
```

### 12.3 Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        startupProbe:
          httpGet:
            path: /health/startup
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 30
```

### 12.4 Response Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Healthy | Continue |
| 503 | Unhealthy | Stop traffic/restart |

**Reference**: `OPS-HEALTH-CHECKS.md`

## 13. Alerting

### 13.1 Alert Rules

```yaml
# alerts.yml
groups:
  - name: muster_alerts
    rules:
      # High error rate
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(http_requests_total{status_code=~"5.."}[5m]))
            /
            sum(rate(http_requests_total[5m]))
          ) > 0.05
        for: 5m
        labels:
          severity: critical
          service: api
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value | humanizePercentage }}"
          runbook_url: "https://wiki/runbooks/high-error-rate"

      # High latency
      - alert: HighLatency
        expr: |
          histogram_quantile(0.95,
            sum(rate(request_duration_seconds_bucket[5m])) by (le)
          ) > 0.5
        for: 10m
        labels:
          severity: warning
          service: api

      # Low disk space
      - alert: LowDiskSpace
        expr: node_filesystem_avail_bytes / node_filesystem_size_bytes < 0.1
        for: 5m
        labels:
          severity: warning
```

### 13.2 Alertmanager Configuration

```yaml
# alertmanager.yml
global:
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@muster.io'
  smtp_auth_username: 'alerts@muster.io'
  smtp_auth_password: '${SMTP_PASSWORD}'
  slack_api_url: '${SLACK_WEBHOOK_URL}'

route:
  receiver: 'default'
  group_by: ['alertname', 'severity', 'service']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty'
      continue: true
    - match:
        severity: warning
      receiver: 'slack'
      continue: true

receivers:
  - name: 'default'
    email_configs:
      - to: 'ops@muster.io'
        send_resolved: true

  - name: 'slack'
    slack_configs:
      - channel: '#alerts'
        send_resolved: true
        title: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
        text: |
          {{ range .Alerts }}
          *Alert:* {{ .Annotations.summary }}
          *Description:* {{ .Annotations.description }}
          *Severity:* {{ .Labels.severity }}
          *Runbook:* {{ .Annotations.runbook_url }}
          {{ end }}
```

### 13.3 Alert Severity Levels

| Severity | Response Time | Notification |
|----------|---------------|--------------|
| Critical | Immediate | PagerDuty + Slack |
| Warning | 15 minutes | Slack + Email |
| Info | 1 hour | Email only |

### 13.4 Silencing Rules

```yaml
# Silence maintenance windows
- matchers:
    - name: alertname
      value: HighMemoryUsage
    - name: environment
      value: staging
  startsAt: 2024-01-01T00:00:00Z
  endsAt: 2024-01-01T04:00:00Z
  createdBy: ops-team
  comment: "Scheduled maintenance"
```

**Reference**: `OPS-ALERTING.md`

## 14. Blind-Person Visualization

### 14.1 Deployment Flow Visualization

A blind person can understand the deployment architecture through structured descriptions:

**Build Pipeline Flow:**
```
Source Code → Build System → Cross-Compilation → Artifact Generation → Release Packaging → Distribution
```

**Platform Distribution Channels:**
```
Linux:   .deb → apt, .rpm → yum, APK → apk, PKGBUILD → pacman
macOS:   Homebrew → brew, PKG → Installer, DMG → Drag-and-drop
Windows: MSI → Installer, Chocolatey → choco, Scoop → scoop, Winget → winget
```

**Deployment Environments:**
```
Development → Docker Compose (Local)
Staging     → Kubernetes (Test Cluster)
Production  → Kubernetes (Multi-Region)
```

### 14.2 Component Relationships

**Hierarchical Structure:**
```
Muster Deployment System
├── Build System
│   ├── Makefile Orchestration
│   ├── Cross-Compilation Matrix
│   └── Version Injection
├── Release Process
│   ├── Semantic Versioning
│   ├── Git Tagging
│   └── Distribution Packaging
├── CI/CD Pipelines
│   ├── Continuous Integration
│   ├── Automated Releases
│   ├── Security Scanning
│   └── Performance Tracking
├── Deployment Targets
│   ├── Docker Containers
│   ├── Kubernetes Clusters
│   ├── System Services
│   └── Platform Packages
└── Operations
    ├── Monitoring & Metrics
    ├── Logging & Tracing
    ├── Health Checks
    ├── Alerting & Notifications
    └── Backup & Recovery
```

### 14.3 Deterministic Guarantee

**Same Input → Same Output Principle:**
- Identical source code + identical build environment = identical artifacts
- Version-controlled dependencies with checksum verification
- Immutable container images with content-addressable digests
- Reproducible builds across different environments

**Verification Mechanisms:**
1. **Artifact Checksums**: SHA256 verification of all build outputs
2. **Container Signatures**: Docker content trust with Notary
3. **Package Signing**: GPG signatures for platform packages
4. **Build Provenance**: SLSA attestations for build artifacts

**Reproducibility Controls:**
- Locked dependency versions (go.mod, package-lock.json)
- Pinned base images (Dockerfile FROM with specific digest)
- Fixed toolchain versions (Go compiler, build tools)
- Isolated build environments (Docker build, GitHub Actions runners)

## 15. Deterministic Guarantee

### 15.1 Reproducible Builds

**Environment Consistency:**
```dockerfile
# Build environment definition
FROM golang:1.22.0-alpine3.19 AS builder

# Pinned tool versions
RUN apk add --no-cache \
    git=2.40.1-r0 \
    make=4.4.1-r0 \
    gcc=12.2.1_git20220924-r4 \
    musl-dev=1.2.4-r0
```

**Dependency Locking:**
```go
// go.mod with exact versions
module github.com/anomalyco/muster

go 1.22.0

require (
    github.com/lib/pq v1.10.9
    github.com/redis/go-redis/v9 v9.0.5
)
```

### 15.2 Content-Addressable Artifacts

**Container Image Digests:**
```
muster/api@sha256:a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

**Package Checksums:**
```
muster_1.0.0_linux_amd64.tar.gz:
  SHA256: a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

### 15.3 Verification Commands

```bash
# Verify container image
docker pull muster/api@sha256:a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
docker trust inspect --pretty muster/api

# Verify package integrity
shasum -a 256 muster_1.0.0_linux_amd64.tar.gz
gpg --verify muster_1.0.0_linux_amd64.tar.gz.sig

# Verify build reproducibility
diffoscope build1/ build2/
```

## 16. Implementation Files

### Build System Components
1. `BUILD-SYSTEM.md` - Complete build system specification
2. `CROSS-COMPILATION.md` - Cross-platform build guide
3. `RELEASE-PROCESS.md` - Release automation procedures
4. `CI-CD-GITHUB-ACTIONS.md` - GitHub Actions workflows

### Containerization
5. `DOCKER-BUILD.md` - Docker image specification
6. `DOCKER-COMPOSE.md` - Docker Compose configurations

### Platform Packages
7. `PLATFORM-LINUX.md` - Linux packaging and integration
8. `PLATFORM-MACOS.md` - macOS packaging and integration
9. `PLATFORM-WINDOWS.md` - Windows packaging and integration

### Operations
10. `OPS-DEPLOYMENT.md` - Kubernetes and systemd deployment
11. `OPS-MONITORING.md` - Monitoring setup and dashboards
12. `OPS-BACKUP-RECOVERY.md` - Backup and disaster recovery
13. `OPS-METRICS.md` - Prometheus metrics specification
14. `OPS-LOGGING.md` - Structured logging specification
15. `OPS-TRACING.md` - Distributed tracing specification
16. `OPS-HEALTH-CHECKS.md` - Health check endpoints
17. `OPS-ALERTING.md` - Alert rules and notifications

### Additional Context
18. `.github/workflows/ci.yml` - Continuous integration pipeline
19. `.github/workflows/release.yml` - Automated release pipeline
20. `.github/workflows/docker.yml` - Docker build pipeline
21. `.github/workflows/security.yml` - Security scanning pipeline
22. `.github/workflows/benchmark.yml` - Performance tracking pipeline
23. `Makefile` - Build orchestration
24. `Dockerfile` - Container build definition
25. `docker-compose.yml` - Local development environment

## 17. Validation Criteria

### Build System Validation
- [ ] All Makefile targets execute successfully
- [ ] Cross-compilation produces correct binaries for all platforms
- [ ] Version injection works correctly in built artifacts
- [ ] Docker images build successfully and pass security scans

### Release Process Validation
- [ ] Semantic versioning followed correctly
- [ ] Git tagging process works as specified
- [ ] All platform packages build and install correctly
- [ ] Homebrew formula updates automatically

### CI/CD Validation
- [ ] All GitHub Actions workflows pass
- [ ] Security scanning identifies vulnerabilities
- [ ] Performance benchmarks track regressions
- [ ] Automated releases work end-to-end

### Deployment Validation
- [ ] Kubernetes manifests deploy successfully
- [ ] Health checks respond correctly
- [ ] Monitoring metrics are exposed
- [ ] Alerting rules trigger appropriately

### Observability Validation
- [ ] Structured logs contain all required fields
- [ ] Distributed traces capture request flows
- [ ] Metrics are collected and queryable
- [ ] Alerts are routed to correct channels

### Recovery Validation
- [ ] Backup scripts execute successfully
- [ ] Database recovery works within RTO
- [ ] Disaster recovery procedures tested
- [ ] Retention policies enforced

---

**Deployment Specification Complete - References 25 files**  
**Blind-Person Visualization: ✅ COMPLIANT**  
**Bible Standard Determinism: ✅ GUARANTEED**