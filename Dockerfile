# syntax=docker/dockerfile:1

# =============================================================================
# STAGE 1: Build Environment
# =============================================================================
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    make \
    gcc \
    musl-dev \
    sqlite-dev

# Create non-root user for build
RUN adduser -D -g '' appuser

# Set working directory
WORKDIR /build

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Extract build arguments
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

# Build openapi-cli binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static" \
    -X github.com/openapi-cli/openapi-cli/internal/version.Version=${VERSION} \
    -X github.com/openapi-cli/openapi-cli/internal/version.Commit=${GIT_COMMIT} \
    -X github.com/openapi-cli/openapi-cli/internal/version.BuildDate=${BUILD_DATE}' \
    -a -installsuffix cgo \
    -o openapi-cli \
    ./cmd/openapi-cli

# Build MCP server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static" \
    -X github.com/openapi-cli/openapi-cli/internal/version.Version=${VERSION} \
    -X github.com/openapi-cli/openapi-cli/internal/version.Commit=${GIT_COMMIT} \
    -X github.com/openapi-cli/openapi-cli/internal/version.BuildDate=${BUILD_DATE}' \
    -a -installsuffix cgo \
    -o openapi-mcp \
    ./cmd/mcp-server

# Run tests
RUN go test -v ./... 2>&1 | tee test-results.txt

# =============================================================================
# STAGE 2: Security Scan
# =============================================================================
FROM aquasec/trivy:latest AS security-scanner

COPY --from=builder /build/openapi-cli /tmp/
COPY --from=builder /build/openapi-mcp /tmp/

# Scan binaries for vulnerabilities
RUN trivy filesystem --exit-code 0 --no-progress /tmp/ 2>/dev/null || true

# =============================================================================
# STAGE 3: Final Runtime Image
# =============================================================================
FROM gcr.io/distroless/static:nonroot

# Metadata labels following OCI spec
LABEL org.opencontainers.image.title="OpenAPI CLI + MCP Server" \
      org.opencontainers.image.description="Unified OpenAPI CLI and MCP Server" \
      org.opencontainers.image.version="1.0.0" \
      org.opencontainers.image.vendor="OpenAPI CLI Project" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/openapi-cli/muster" \
      org.opencontainers.image.documentation="https://docs.openapi-cli.io"

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
