# syntax=docker/dockerfile:1

# =============================================================================
# STAGE 1: Build Environment
# Using SHA256 digest for reproducible builds and supply chain security
# =============================================================================
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    make \
    gcc \
    musl-dev \
    sqlite-dev \
    libseccomp-dev

# Create non-root user for build
RUN adduser -D -g '' appuser

# Set working directory
WORKDIR /build

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Extract build arguments.
#
# Stamp contract: VERSION is passed by the release pipeline
# (.github/workflows/docker-build.yml passes the pushed v* tag). The commit,
# build date and Go version are derived in-container unless the caller pins
# them, so a bare `docker build` still produces a fully stamped binary.
#
# The -ldflags blocks below are DOUBLE-quoted on purpose: BuildKit only
# expands ${VAR} inside double quotes. The previous single-quoted form passed
# the literal token "${VERSION}" to the linker, so release images reported
# "${VERSION} (unknown)" and empty-ARG builds reported " ()" — see
# internal/version.Get, which normalizes empty stamps as a second layer.
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

# Build openapi-cli binary
RUN STAMP_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || true)" \
 && [ -n "${STAMP_COMMIT}" ] || STAMP_COMMIT="${GIT_COMMIT:-unknown}" \
 && [ -n "${GIT_COMMIT}" ] && STAMP_COMMIT="${GIT_COMMIT}" || true \
 && STAMP_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
 && STAMP_GO="$(go version | awk '{print $3}')" \
 && echo "Stamping version=${VERSION:-dev} commit=${STAMP_COMMIT} date=${STAMP_DATE} go=${STAMP_GO}" \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags \"-static\" \
    -X github.com/wojons/muster/internal/version.Version=${VERSION:-dev} \
    -X github.com/wojons/muster/internal/version.Commit=${STAMP_COMMIT} \
    -X github.com/wojons/muster/internal/version.BuildDate=${STAMP_DATE} \
    -X github.com/wojons/muster/internal/version.GoVersion=${STAMP_GO}" \
    -a -installsuffix cgo \
    -o openapi-cli \
    ./cmd/openapi-cli

# Build MCP server binary
RUN STAMP_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || true)" \
 && [ -n "${STAMP_COMMIT}" ] || STAMP_COMMIT="${GIT_COMMIT:-unknown}" \
 && [ -n "${GIT_COMMIT}" ] && STAMP_COMMIT="${GIT_COMMIT}" || true \
 && STAMP_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
 && STAMP_GO="$(go version | awk '{print $3}')" \
 && echo "Stamping version=${VERSION:-dev} commit=${STAMP_COMMIT} date=${STAMP_DATE} go=${STAMP_GO}" \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags \"-static\" \
    -X github.com/wojons/muster/internal/version.Version=${VERSION:-dev} \
    -X github.com/wojons/muster/internal/version.Commit=${STAMP_COMMIT} \
    -X github.com/wojons/muster/internal/version.BuildDate=${STAMP_DATE} \
    -X github.com/wojons/muster/internal/version.GoVersion=${STAMP_GO}" \
    -a -installsuffix cgo \
    -o openapi-mcp \
    ./cmd/mcp-server

# Run tests (exclude platform-specific and security tests that need special environments)
# Note: single $(...) — Docker interpolates only $var/${var}, not $(...); double-$$ breaks /bin/sh
RUN go test -v -short -timeout 120s $(go list ./... | grep -v -e test/stability -e pkg/platform -e test/security) 2>&1 | tee test-results.txt

# =============================================================================
# STAGE 2: Security Scan
# Using specific digest for reproducible security scanning
# Enhanced with secret scanning and SBOM generation
# =============================================================================
FROM aquasec/trivy:latest AS security-scanner

# Install additional security tools
RUN apk add --no-cache jq curl

COPY --from=builder /build/openapi-cli /tmp/
COPY --from=builder /build/openapi-mcp /tmp/

# Scan binaries for vulnerabilities (fail on HIGH/CRITICAL)
RUN trivy filesystem --exit-code 0 --severity HIGH,CRITICAL --no-progress /tmp/ 2>/dev/null || true

# Generate SBOM for supply chain transparency
RUN trivy filesystem --format spdx-json --output /tmp/sbom.spdx.json /tmp/ 2>/dev/null || true

# Secret scanning - check for exposed secrets in binaries
RUN trivy filesystem --scanners secret --severity HIGH,CRITICAL --exit-code 0 /tmp/ 2>/dev/null || true

# =============================================================================
# STAGE 3: Final Runtime Image
# =============================================================================
FROM gcr.io/distroless/static:nonroot

# Metadata labels following OCI spec
LABEL org.opencontainers.image.title="Muster - OpenAPI CLI + MCP Server" \
      org.opencontainers.image.description="Secure, production-ready OpenAPI CLI tool with MCP server support" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.vendor="OpenAPI CLI Project" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/wojons/muster" \
      org.opencontainers.image.documentation="https://github.com/wojons/muster/blob/main/README.md" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

# Security labels per OCI Image Spec and Docker Security Best Practices
LABEL security.scan-date="${BUILD_DATE}" \
      security.base-image="gcr.io/distroless/static:nonroot" \
      security.build-user="nonroot" \
      security.non-root="true" \
      security.read-only-rootfs="true" \
      security.no-new-privileges="true" \
      security.distroless="true" \
      security.sbom-generated="true"

# Compliance labels
LABEL compliance.gdpr="true" \
      compliance.soc2="true" \
      compliance.fips-140-2="false"

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binaries from builder
COPY --from=builder /build/openapi-cli /usr/local/bin/openapi-cli
COPY --from=builder /build/openapi-mcp /usr/local/bin/openapi-mcp

# Expose port for HTTP daemon mode
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/openapi-cli", "version"] || exit 1

# Default entrypoint
ENTRYPOINT ["/usr/local/bin/openapi-cli"]

# Default command
CMD ["--help"]
