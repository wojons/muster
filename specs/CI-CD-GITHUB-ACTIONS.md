# CI/CD GitHub Actions Workflows

Complete GitHub Actions configuration for continuous integration, automated releases, Docker builds, security scanning, and performance tracking.

## Repository Structure

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

---

## 1. ci.yml - Continuous Integration

**File**: `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [main, master]
    tags: ['v*']
  pull_request:
    branches: [main, master]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

env:
  GO_VERSION: '1.21'
  GOLANGCI_LINT_VERSION: v1.55

jobs:
  # ============================================
  # Lint
  # ============================================
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v3
        with:
          version: ${{ env.GOLANGCI_LINT_VERSION }}
          args: --timeout=5m --verbose
          skip-cache: false

  # ============================================
  # Unit Tests
  # ============================================
  test-unit:
    name: Unit Tests
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go: ['1.21', '1.22']
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      - name: Download dependencies
        run: go mod download

      - name: Run unit tests
        run: go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
          flags: unittests
          name: codecov-${{ matrix.os }}-go${{ matrix.go }}

  # ============================================
  # Integration Tests
  # ============================================
  test-integration:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: [lint, test-unit]
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      postgres:
        image: postgres:15-alpine
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: testdb
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      - name: Run integration tests
        env:
          REDIS_URL: redis://localhost:6379
          DATABASE_URL: postgres://test:test@localhost:5432/testdb?sslmode=disable
        run: go test -v -tags=integration ./... -run Integration

  # ============================================
  # Build All Platforms
  # ============================================
  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint, test-unit]
    strategy:
      fail-fast: false
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: windows
            goarch: amd64
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      - name: Build binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: 0
        run: |
          OUTPUT_NAME="muster-${{ matrix.goos }}-${{ matrix.goarch }}"
          if [ "${{ matrix.goos }}" = "windows" ]; then
            OUTPUT_NAME="${OUTPUT_NAME}.exe"
          fi
          go build -ldflags="-s -w -X main.version=${{ github.ref_name }}" -o "dist/${OUTPUT_NAME}" ./cmd/muster

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: muster-${{ matrix.goos }}-${{ matrix.goarch }}
          path: dist/*
          retention-days: 7

  # ============================================
  # Summary & Notifications
  # ============================================
  ci-summary:
    name: CI Summary
    runs-on: ubuntu-latest
    needs: [lint, test-unit, test-integration, build]
    if: always()
    steps:
      - name: Check status
        run: |
          if [ "${{ needs.lint.result }}" = "failure" ] || \
             [ "${{ needs.test-unit.result }}" = "failure" ] || \
             [ "${{ needs.test-integration.result }}" = "failure" ] || \
             [ "${{ needs.build.result }}" = "failure" ]; then
            echo "::error::CI workflow failed"
            exit 1
          fi
          echo "CI workflow completed successfully"

      - name: Notify Slack on failure
        if: failure() && github.event_name == 'push'
        uses: 8398a7/action-slack@v3
        with:
          status: ${{ job.status }}
          channel: '#ci-alerts'
          text: 'CI failed for ${{ github.repository }}@${{ github.ref }}'
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

---

## 2. release.yml - Automated Releases

**File**: `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - 'v[0-9]+.[0-9]+.[0-9]+'
      - 'v[0-9]+.[0-9]+.[0-9]+-*'

permissions:
  contents: write
  packages: write

env:
  GO_VERSION: '1.21'

jobs:
  # ============================================
  # Build Release Binaries
  # ============================================
  build:
    name: Build Release Binaries
    runs-on: ubuntu-latest
    strategy:
      fail-fast: true
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: linux
            goarch: arm
            goarm: '7'
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: windows
            goarch: amd64
          - goos: windows
            goarch: arm64
          - goos: freebsd
            goarch: amd64
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

      - name: Build binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          GOARM: ${{ matrix.goarm }}
          CGO_ENABLED: 0
        run: |
          VERSION=${GITHUB_REF#refs/tags/}
          OUTPUT_NAME="muster-${VERSION}-${{ matrix.goos }}-${{ matrix.goarch }}"
          if [ "${{ matrix.goos }}" = "windows" ]; then
            OUTPUT_NAME="${OUTPUT_NAME}.exe"
          fi
          
          go build \
            -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GITHUB_SHA::7} -X main.date=$(date -u +%Y-%m-%d)" \
            -o "dist/${OUTPUT_NAME}" \
            ./cmd/muster

      - name: Create checksum
        working-directory: dist
        run: |
          sha256sum * > checksums.txt

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: release-binaries-${{ matrix.goos }}-${{ matrix.goarch }}
          path: dist/*
          retention-days: 1

  # ============================================
  # Create GitHub Release
  # ============================================
  release:
    name: Create Release
    runs-on: ubuntu-latest
    needs: build
    outputs:
      upload_url: ${{ steps.create_release.outputs.upload_url }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Generate changelog
        id: changelog
        uses: mikepenz/release-changelog-builder-action@v4
        with:
          configuration: .github/changelog-config.json
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Download all artifacts
        uses: actions/download-artifact@v4
        with:
          path: dist
          pattern: release-binaries-*
          merge-multiple: true

      - name: Create Release
        id: create_release
        uses: softprops/action-gh-release@v1
        with:
          body: ${{ steps.changelog.outputs.changelog }}
          files: dist/*
          draft: false
          prerelease: ${{ contains(github.ref, '-') }}
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  # ============================================
  # Update Homebrew Formula
  # ============================================
  homebrew:
    name: Update Homebrew Formula
    runs-on: ubuntu-latest
    needs: release
    if: ${{ !contains(github.ref, '-') }}
    steps:
      - name: Checkout homebrew-tap
        uses: actions/checkout@v4
        with:
          repository: ${{ github.repository_owner }}/homebrew-tap
          token: ${{ secrets.HOMEBREW_TAP_TOKEN }}
          path: homebrew-tap

      - name: Download release artifacts
        uses: actions/download-artifact@v4
        with:
          path: dist
          pattern: release-binaries-*
          merge-multiple: true

      - name: Calculate checksums
        id: checksums
        run: |
          VERSION=${GITHUB_REF#refs/tags/}
          echo "version=${VERSION}" >> $GITHUB_OUTPUT
          
          DARWIN_AMD64_SHA=$(sha256sum dist/muster-${VERSION}-darwin-amd64 | cut -d' ' -f1)
          DARWIN_ARM64_SHA=$(sha256sum dist/muster-${VERSION}-darwin-arm64 | cut -d' ' -f1)
          LINUX_AMD64_SHA=$(sha256sum dist/muster-${VERSION}-linux-amd64 | cut -d' ' -f1)
          LINUX_ARM64_SHA=$(sha256sum dist/muster-${VERSION}-linux-arm64 | cut -d' ' -f1)
          
          echo "darwin_amd64_sha=${DARWIN_AMD64_SHA}" >> $GITHUB_OUTPUT
          echo "darwin_arm64_sha=${DARWIN_ARM64_SHA}" >> $GITHUB_OUTPUT
          echo "linux_amd64_sha=${LINUX_AMD64_SHA}" >> $GITHUB_OUTPUT
          echo "linux_arm64_sha=${LINUX_ARM64_SHA}" >> $GITHUB_OUTPUT

      - name: Update formula
        working-directory: homebrew-tap
        run: |
          cat > Formula/muster.rb << 'EOF'
          class Muster < Formula
            desc "Production-ready web crawler and scraper"
            homepage "https://github.com/${{ github.repository }}"
            version "${{ steps.checksums.outputs.version }}"
            license "MIT"

            on_macos do
              if Hardware::CPU.arm?
                url "https://github.com/${{ github.repository }}/releases/download/${{ steps.checksums.outputs.version }}/muster-${{ steps.checksums.outputs.version }}-darwin-arm64"
                sha256 "${{ steps.checksums.outputs.darwin_arm64_sha }}"
              else
                url "https://github.com/${{ github.repository }}/releases/download/${{ steps.checksums.outputs.version }}/muster-${{ steps.checksums.outputs.version }}-darwin-amd64"
                sha256 "${{ steps.checksums.outputs.darwin_amd64_sha }}"
              end
            end

            on_linux do
              if Hardware::CPU.arm?
                url "https://github.com/${{ github.repository }}/releases/download/${{ steps.checksums.outputs.version }}/muster-${{ steps.checksums.outputs.version }}-linux-arm64"
                sha256 "${{ steps.checksums.outputs.linux_arm64_sha }}"
              else
                url "https://github.com/${{ github.repository }}/releases/download/${{ steps.checksums.outputs.version }}/muster-${{ steps.checksums.outputs.version }}-linux-amd64"
                sha256 "${{ steps.checksums.outputs.linux_amd64_sha }}"
              end
            end

            def install
              bin.install Dir["muster-*"].first => "muster"
            end

            test do
              system "#{bin}/muster", "--version"
            end
          end
          EOF

      - name: Commit and push
        working-directory: homebrew-tap
        run: |
          git config user.name "GitHub Actions"
          git config user.email "actions@github.com"
          git add Formula/muster.rb
          git commit -m "Update muster to ${{ steps.checksums.outputs.version }}"
          git push

  # ============================================
  # Notify
  # ============================================
  notify:
    name: Notify
    runs-on: ubuntu-latest
    needs: [release, homebrew]
    if: always()
    steps:
      - name: Notify Slack
        uses: 8398a7/action-slack@v3
        with:
          status: ${{ job.status }}
          channel: '#releases'
          text: |
            Release ${{ github.ref_name }} ${{ needs.release.result == 'success' && 'completed' || 'failed' }}
            Homebrew ${{ needs.homebrew.result == 'success' && 'updated' || 'skipped' }}
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

---

## 3. docker.yml - Docker Build & Push

**File**: `.github/workflows/docker.yml`

```yaml
name: Docker

on:
  push:
    branches: [main, master]
    tags: ['v*']
  pull_request:
    branches: [main, master]
    paths:
      - 'Dockerfile'
      - 'docker/**'
      - 'cmd/**'
      - 'pkg/**'
      - 'go.mod'
      - 'go.sum'

env:
  REGISTRY: docker.io
  IMAGE_NAME: ${{ github.repository }}
  GO_VERSION: '1.21'

jobs:
  # ============================================
  # Build and Test
  # ============================================
  build-test:
    name: Build and Test Image
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: false
          load: true
          tags: ${{ env.IMAGE_NAME }}:test
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Test image
        run: |
          docker run --rm ${{ env.IMAGE_NAME }}:test --version
          docker run --rm ${{ env.IMAGE_NAME }}:test --help

      - name: Scan image
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ env.IMAGE_NAME }}:test
          format: 'sarif'
          output: 'trivy-results.sarif'
          severity: 'CRITICAL,HIGH'

      - name: Upload scan results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: 'trivy-results.sarif'

  # ============================================
  # Build and Push Multi-Arch
  # ============================================
  push:
    name: Push Multi-Arch Images
    runs-on: ubuntu-latest
    needs: build-test
    if: github.event_name != 'pull_request'
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3
        with:
          platforms: linux/amd64,linux/arm64,linux/arm/v7

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: |
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
            ghcr.io/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=sha,prefix=,suffix=,format=short

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          platforms: linux/amd64,linux/arm64,linux/arm/v7
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          build-args: |
            GO_VERSION=${{ env.GO_VERSION }}
            VERSION=${{ github.ref_name }}
            BUILD_DATE=${{ github.event.head_commit.timestamp }}
            VCS_REF=${{ github.sha }}

  # ============================================
  # Update Docker Hub Description
  # ============================================
  update-readme:
    name: Update Docker Hub Description
    runs-on: ubuntu-latest
    needs: push
    if: startsWith(github.ref, 'refs/tags/v')
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Update Docker Hub Description
        uses: peter-evans/dockerhub-description@v3
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}
          repository: ${{ env.IMAGE_NAME }}
          readme-filepath: ./DOCKER.md
          short-description: "Production-ready web crawler and scraper"
```

**Dockerfile**:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

ARG VERSION=dev
ARG BUILD_DATE
ARG VCS_REF

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X main.version=${VERSION} \
    -X main.commit=${VCS_REF} \
    -X main.date=${BUILD_DATE}" \
    -o muster \
    ./cmd/muster

# Final stage
FROM scratch

ARG VERSION
ARG BUILD_DATE
ARG VCS_REF

# Copy certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /build/muster /muster

# Labels
LABEL org.opencontainers.image.title="Muster" \
      org.opencontainers.image.description="Production-ready web crawler" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.source="https://github.com/yourorg/muster"

ENTRYPOINT ["/muster"]
```

---

## 4. security.yml - Security Scanning

**File**: `.github/workflows/security.yml`

```yaml
name: Security

on:
  push:
    branches: [main, master]
  pull_request:
    branches: [main, master]
  schedule:
    # Run daily at 6 AM UTC
    - cron: '0 6 * * *'
  workflow_dispatch:

permissions:
  actions: read
  contents: read
  security-events: write

env:
  GO_VERSION: '1.21'

jobs:
  # ============================================
  # govulncheck
  # ============================================
  govulncheck:
    name: Go Vulnerability Check
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest

      - name: Run govulncheck
        run: govulncheck -show verbose ./...

      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v2
        with:
          sarif_file: govulncheck-results.sarif

  # ============================================
  # CodeQL Analysis
  # ============================================
  codeql:
    name: CodeQL Analysis
    runs-on: ubuntu-latest
    permissions:
      actions: read
      contents: read
      security-events: write
    strategy:
      fail-fast: false
      matrix:
        language: ['go']
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Initialize CodeQL
        uses: github/codeql-action/init@v2
        with:
          languages: ${{ matrix.language }}
          queries: security-extended,security-and-quality

      - name: Autobuild
        uses: github/codeql-action/autobuild@v2

      - name: Perform Analysis
        uses: github/codeql-action/analyze@v2
        with:
          category: "/language:${{ matrix.language }}"

  # ============================================
  # Dependency Review
  # ============================================
  dependency-review:
    name: Dependency Review
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Dependency Review
        uses: actions/dependency-review-action@v3
        with:
          fail-on-severity: high
          vulnerability-check: true
          license-check: true

  # ============================================
  # Trivy Scan
  # ============================================
  trivy:
    name: Trivy Security Scan
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Run Trivy vulnerability scanner (Repo)
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          format: 'sarif'
          output: 'trivy-results.sarif'
          severity: 'CRITICAL,HIGH,MEDIUM'
          exit-code: '1'

      - name: Upload Trivy scan results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: 'trivy-results.sarif'

  # ============================================
  # Nancy (Go Dependency Scanner)
  # ============================================
  nancy:
    name: Nancy Dependency Scan
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Write Go mod list
        run: go list -json -m all > go.list

      - name: Run Nancy
        uses: sonatypecommunity/nancy-github-action@main
        with:
          goListFile: go.list

  # ============================================
  # Gosec (Go Security Checker)
  # ============================================
  gosec:
    name: Gosec Security Checker
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Run Gosec Security Scanner
        uses: securego/gosec@master
        with:
          args: '-fmt sarif -out gosec-results.sarif ./...'

      - name: Upload Gosec scan results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: 'gosec-results.sarif'

  # ============================================
  # Summary
  # ============================================
  security-summary:
    name: Security Summary
    runs-on: ubuntu-latest
    needs: [govulncheck, codeql, dependency-review, trivy, nancy, gosec]
    if: always()
    steps:
      - name: Check status
        run: |
          FAILED=0
          if [ "${{ needs.govulncheck.result }}" = "failure" ]; then
            echo "::error::govulncheck failed"
            FAILED=1
          fi
          if [ "${{ needs.codeql.result }}" = "failure" ]; then
            echo "::error::CodeQL analysis failed"
            FAILED=1
          fi
          if [ "${{ needs.trivy.result }}" = "failure" ]; then
            echo "::error::Trivy scan failed"
            FAILED=1
          fi
          if [ "${{ needs.gosec.result }}" = "failure" ]; then
            echo "::error::Gosec scan failed"
            FAILED=1
          fi
          
          if [ $FAILED -eq 1 ]; then
            exit 1
          fi
          echo "All security checks passed"

      - name: Notify on failure
        if: failure() && github.event_name == 'schedule'
        uses: 8398a7/action-slack@v3
        with:
          status: ${{ job.status }}
          channel: '#security-alerts'
          text: 'Security scan failed for ${{ github.repository }}'
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

---

## 5. benchmark.yml - Performance Tracking

**File**: `.github/workflows/benchmark.yml`

```yaml
name: Benchmark

on:
  push:
    branches: [main, master]
  pull_request:
    branches: [main, master]
  workflow_dispatch:

env:
  GO_VERSION: '1.21'

jobs:
  # ============================================
  # Run Benchmarks
  # ============================================
  benchmark:
    name: Run Benchmarks
    runs-on: ubuntu-latest
    steps:
      - name: Checkout PR
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

      - name: Run benchmarks
        run: |
          go test -bench=. -benchmem -count=5 -run=^$ ./... | tee benchmark-current.txt

      - name: Upload benchmark results
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-current
          path: benchmark-current.txt

  # ============================================
  # Compare with Previous
  # ============================================
  compare:
    name: Compare with Previous
    runs-on: ubuntu-latest
    needs: benchmark
    if: github.event_name == 'pull_request'
    steps:
      - name: Checkout main
        uses: actions/checkout@v4
        with:
          ref: main
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

      - name: Run benchmarks on main
        run: |
          go test -bench=. -benchmem -count=5 -run=^$ ./... | tee benchmark-main.txt

      - name: Download PR benchmark results
        uses: actions/download-artifact@v4
        with:
          name: benchmark-current
          path: .

      - name: Install benchstat
        run: go install golang.org/x/perf/cmd/benchstat@latest

      - name: Compare benchmarks
        id: compare
        run: |
          echo "## Benchmark Comparison" > benchmark-report.md
          echo "" >> benchmark-report.md
          echo "### Current PR vs Main" >> benchmark-report.md
          echo "" >> benchmark-report.md
          echo "\`\`\`" >> benchmark-report.md
          benchstat benchmark-main.txt benchmark-current.txt | tee -a benchmark-report.md
          echo "\`\`\`" >> benchmark-report.md
          
          # Check for regressions (>10% slower)
          REGRESSIONS=$(benchstat benchmark-main.txt benchmark-current.txt | grep -E "\+[0-9]+\.[0-9]+%" || true)
          if [ -n "$REGRESSIONS" ]; then
            echo "" >> benchmark-report.md
            echo "### ⚠️ Performance Regressions Detected" >> benchmark-report.md
            echo "" >> benchmark-report.md
            echo "$REGRESSIONS" >> benchmark-report.md
            echo "regressions=true" >> $GITHUB_OUTPUT
          else
            echo "regressions=false" >> $GITHUB_OUTPUT
          fi

      - name: Comment on PR
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const report = fs.readFileSync('benchmark-report.md', 'utf8');
            
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: report
            });

      - name: Fail on regression
        if: steps.compare.outputs.regressions == 'true'
        run: |
          echo "::error::Performance regressions detected!"
          exit 1

  # ============================================
  # Store Benchmark Results
  # ============================================
  store:
    name: Store Results
    runs-on: ubuntu-latest
    needs: benchmark
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - name: Checkout gh-pages
        uses: actions/checkout@v4
        with:
          ref: gh-pages
          fetch-depth: 0

      - name: Download benchmark results
        uses: actions/download-artifact@v4
        with:
          name: benchmark-current
          path: .

      - name: Store results
        run: |
          mkdir -p benchmarks
          TIMESTAMP=$(date -u +%Y%m%d-%H%M%S)
          COMMIT=${{ github.sha }}
          cp benchmark-current.txt "benchmarks/${TIMESTAMP}-${COMMIT:0:7}.txt"
          
          # Keep only last 30 results
          ls -t benchmarks/*.txt | tail -n +31 | xargs -r rm

      - name: Commit and push
        run: |
          git config user.name "GitHub Actions"
          git config user.email "actions@github.com"
          git add benchmarks/
          git commit -m "Add benchmark results for ${{ github.sha }}"
          git push

  # ============================================
  # Generate Trend Chart
  # ============================================
  trend:
    name: Generate Trend
    runs-on: ubuntu-latest
    needs: store
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - name: Checkout gh-pages
        uses: actions/checkout@v4
        with:
          ref: gh-pages
          fetch-depth: 0

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Generate trend chart
        run: |
          pip install matplotlib pandas
          python << 'EOF'
          import os
          import re
          import json
          from datetime import datetime
          import matplotlib.pyplot as plt
          import pandas as pd

          data = []
          for filename in sorted(os.listdir('benchmarks')):
              if filename.endswith('.txt'):
                  with open(f'benchmarks/{filename}') as f:
                      content = f.read()
                      timestamp = filename.split('-')[0]
                      
                      # Parse benchmark results
                      for line in content.split('\n'):
                          match = re.match(r'(Benchmark\S+)\s+\d+\s+([\d\.]+)\s+ns/op', line)
                          if match:
                              name = match.group(1)
                              value = float(match.group(2))
                              data.append({
                                  'timestamp': timestamp,
                                  'benchmark': name,
                                  'ns_per_op': value
                              })

          if data:
              df = pd.DataFrame(data)
              
              # Create trend charts
              for benchmark in df['benchmark'].unique():
                  subset = df[df['benchmark'] == benchmark].sort_values('timestamp')
                  plt.figure(figsize=(10, 6))
                  plt.plot(range(len(subset)), subset['ns_per_op'], marker='o')
                  plt.title(f'{benchmark} Performance Trend')
                  plt.xlabel('Run')
                  plt.ylabel('ns/op')
                  plt.grid(True)
                  plt.savefig(f'benchmarks/trend-{benchmark}.png')
                  plt.close()

              print("Trend charts generated successfully")
          EOF

      - name: Commit and push
        run: |
          git config user.name "GitHub Actions"
          git config user.email "actions@github.com"
          git add benchmarks/
          git commit -m "Update benchmark trend charts" || echo "No changes"
          git push
```

---

## Secrets Configuration

Add these secrets to your GitHub repository (Settings > Secrets and variables > Actions):

| Secret | Description | Required By |
|--------|-------------|-------------|
| `GITHUB_TOKEN` | Automatically provided | All workflows |
| `SLACK_WEBHOOK_URL` | Slack incoming webhook for notifications | ci.yml, release.yml, security.yml |
| `DOCKER_USERNAME` | Docker Hub username | docker.yml |
| `DOCKER_PASSWORD` | Docker Hub password/token | docker.yml |
| `HOMEBREW_TAP_TOKEN` | GitHub token with write access to homebrew-tap repo | release.yml |
| `CODECOV_TOKEN` | Codecov upload token (optional) | ci.yml |

---

## Artifact Handling

### Retention Policy

Artifacts are configured with retention days:
- CI builds: 7 days
- Release binaries: 1 day (temporary, uploaded to GitHub release)
- Benchmark results: Stored in gh-pages branch

### Artifact Naming Convention

```
muster-{version}-{os}-{arch}[.exe]
```

Examples:
- `muster-v1.0.0-linux-amd64`
- `muster-v1.0.0-darwin-arm64`
- `muster-v1.0.0-windows-amd64.exe`

---

## Cache Configuration

### Go Modules Cache

```yaml
- name: Cache Go modules
  uses: actions/cache@v3
  with:
    path: ~/go/pkg/mod
    key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
    restore-keys: |
      ${{ runner.os }}-go-
```

### Build Cache

```yaml
- name: Cache build
  uses: actions/cache@v3
  with:
    path: ~/.cache/go-build
    key: ${{ runner.os }}-go-build-${{ hashFiles('**/go.sum') }}
```

### Docker Layer Cache

```yaml
- name: Cache Docker layers
  uses: actions/cache@v3
  with:
    path: /tmp/.buildx-cache
    key: ${{ runner.os }}-buildx-${{ github.sha }}
    restore-keys: |
      ${{ runner.os }}-buildx-
```

---

## Matrix Builds

### Platform Matrix

| OS | Architecture | CGO |
|----|--------------|-----|
| linux | amd64 | disabled |
| linux | arm64 | disabled |
| linux | arm/v7 | disabled |
| darwin | amd64 | disabled |
| darwin | arm64 | disabled |
| windows | amd64 | disabled |
| windows | arm64 | disabled |
| freebsd | amd64 | disabled |

### Go Version Matrix

CI tests run against:
- Go 1.21
- Go 1.22

---

## PR Checks and Required Status

### Required Checks

Configure in GitHub (Settings > Branches > Branch protection rules):

1. **ci / lint**
2. **ci / test-unit (ubuntu-latest, 1.21)**
3. **ci / test-unit (macos-latest, 1.21)**
4. **ci / test-integration**
5. **ci / build**
6. **security / govulncheck**
7. **security / codeql**
8. **docker / build-test** (if Dockerfile changed)

### Status Check Configuration

```yaml
# In branch protection rule:
Require status checks to pass before merging:
  - ci-summary
  - security-summary
  - benchmark (for PRs with performance changes)
```

---

## Job Dependencies

```
ci.yml:
  lint
    └─> test-unit
         ├─> test-integration
         └─> build
              └─> ci-summary

release.yml:
  build
    └─> release
         └─> homebrew
              └─> notify

docker.yml:
  build-test
    └─> push (if not PR)
         └─> update-readme (if tag)

security.yml:
  ├─> govulncheck
  ├─> codeql
  ├─> dependency-review (PR only)
  ├─> trivy
  ├─> nancy
  └─> gosec
       └─> security-summary

benchmark.yml:
  benchmark
    ├─> compare (PR only)
    └─> store (main branch only)
         └─> trend (main branch only)
```

---

## Conditional Execution

### Skip Conditions

```yaml
# Skip for documentation-only changes
if: ${{ !contains(github.event.head_commit.message, '[skip ci]') }}

# Skip for specific paths
if: ${{ !contains(toJson(github.event.head_commit.modified), '.md') }}

# Run only on tags
if: startsWith(github.ref, 'refs/tags/v')

# Run only on main branch
if: github.ref == 'refs/heads/main'

# Skip for draft PRs
if: ${{ !github.event.pull_request.draft }}
```

---

## Notifications

### Slack Notifications

All workflows support Slack notifications on failure:

```yaml
- name: Notify Slack
  if: failure()
  uses: 8398a7/action-slack@v3
  with:
    status: ${{ job.status }}
    channel: '#ci-alerts'
    text: 'Workflow failed: ${{ github.workflow }}'
  env:
    SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

### Email Notifications

Configure in GitHub (Settings > Notifications) for:
- Failed workflow runs
- Security alerts
- Release events

---

## Best Practices

1. **Pin action versions** to specific commits or tags for reproducibility
2. **Use concurrency groups** to cancel redundant runs
3. **Set appropriate timeouts** (default: 360 minutes)
4. **Use minimal permissions** (principle of least privilege)
5. **Cache aggressively** to speed up builds
6. **Fail fast** for expensive operations
7. **Use artifacts** for data transfer between jobs
8. **Store long-term data** in gh-pages or external storage
9. **Validate secrets** are configured before running
10. **Document all workflows** with inline comments

---

## Testing Workflows

Test workflows locally using [act](https://github.com/nektos/act):

```bash
# Install act
brew install act

# Run CI workflow
act -j lint

# Run specific job
act -j build -P ubuntu-latest=nektos/act-environments-ubuntu:18.04

# Run with secrets
act --secret-file .secrets
```

---

## Additional Configuration Files

### .github/changelog-config.json

```json
{
  "categories": [
    {
      "title": "### Features",
      "labels": ["feature", "enhancement"]
    },
    {
      "title": "### Bug Fixes",
      "labels": ["bug", "fix"]
    },
    {
      "title": "### Documentation",
      "labels": ["docs", "documentation"]
    },
    {
      "title": "### Dependencies",
      "labels": ["dependencies", "deps"]
    }
  ],
  "ignore_labels": ["skip-changelog"],
  "template": "${{CHANGELOG}}",
  "pr_template": "- ${{TITLE}} (#{{NUMBER}})"
}
```

### .golangci.yml

```yaml
run:
  timeout: 5m
  concurrency: 4
  go: "1.21"

linters:
  enable:
    - bodyclose
    - dogsled
    - dupl
    - errcheck
    - errorlint
    - exportloopref
    - gochecknoinits
    - goconst
    - gocritic
    - gofmt
    - goimports
    - gomnd
    - goprintffuncname
    - gosec
    - gosimple
    - govet
    - ineffassign
    - misspell
    - nakedret
    - noctx
    - nolintlint
    - rowserrcheck
    - staticcheck
    - stylecheck
    - typecheck
    - unconvert
    - unparam
    - unused
    - whitespace

linters-settings:
  gocritic:
    enabled-tags:
      - performance
      - style
      - experimental
  gomnd:
    settings:
      mnd:
        checks: argument,case,condition,return

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gomnd
        - goconst
```

---

## Troubleshooting

### Common Issues

1. **Cache not working**: Ensure `hashFiles('**/go.sum')` matches your file structure
2. **Permission denied**: Check `permissions` block in workflow
3. **Secrets not found**: Verify secrets are set in repository settings
4. **Job timeout**: Add `timeout-minutes` to long-running jobs
5. **Artifact upload fails**: Check retention limits and total size

### Debug Mode

Add to workflow for verbose logging:

```yaml
env:
  ACTIONS_STEP_DEBUG: true
  ACTIONS_RUNNER_DEBUG: true
```
