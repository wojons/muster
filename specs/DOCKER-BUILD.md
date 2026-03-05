# Docker Build Specification

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Multi-Stage Dockerfile

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
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

---

## Image Optimization

### Size Reduction Techniques

```dockerfile
# Use minimal base image
FROM alpine:3.19

# Single RUN layer
RUN apk add --no-cache ca-certificates && \
    rm -rf /var/cache/apk/*

# Multi-stage build separates build dependencies
# Result: ~20MB vs ~300MB

# Use .dockerignore
echo "
.git
*.md
*_test.go
dist/
bin/
.dockerignore
Dockerfile
" > .dockerignore
```

### Build Command

```bash
# Build optimized image
docker build \
  --build-arg VERSION=$(git describe --tags) \
  --tag muster:latest \
  --tag muster:$(git describe --tags) \
  .

# Verify size
docker images | grep muster
```

---

## Layer Caching

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

### Cache Mounts (BuildKit)

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.23-alpine

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o muster ./cmd/cli
```

---

## Security Scanning

```bash
# Trivy scan
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy image muster:latest

# Snyk scan
docker scan muster:latest

# Grype scan
grype muster:latest
```

### Dockerfile Security Best Practices

```dockerfile
# Use non-root user
RUN adduser -D -u 1000 muster
USER muster

# Read-only root filesystem
FROM scratch
COPY --from=builder /build/muster /muster
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# No shell, minimal attack surface
ENTRYPOINT ["/muster"]
```

---

## Image Variants

### Alpine Variant

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/muster /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/muster"]
# Size: ~25MB
```

### Distroless Variant (Recommended)

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/muster /usr/local/bin/
USER nonroot
ENTRYPOINT ["/usr/local/bin/muster"]
# Size: ~20MB
```

### Debug Variant

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache ca-certificates bash curl
COPY --from=builder /build/muster /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/muster"]
# Size: ~30MB, includes shell for debugging
```

---

## Build Automation

```bash
#!/bin/bash
# build-docker.sh

VERSION=${1:-latest}
PLATFORMS="linux/amd64,linux/arm64"

docker buildx create --use --name muster-builder || true

docker buildx build \
  --platform $PLATFORMS \
  --tag muster/muster:$VERSION \
  --tag muster/muster:latest \
  --push \
  .
```
