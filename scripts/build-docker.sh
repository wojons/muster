#!/bin/bash
# =============================================================================
# Docker Build Script with Multi-Arch Support
# =============================================================================
# This script builds multi-architecture Docker images using Docker buildx
# Supports: linux/amd64, linux/arm64, linux/arm/v7
#
# Features:
#   - Multi-stage builds with layer caching
#   - Buildx support for multiple architectures
#   - GitHub Actions cache integration
#   - SBOM and provenance attestation
#   - Security scanning with Trivy
#   - Image size optimization
#
# Usage:
#   ./scripts/build-docker.sh [version] [registry]
#
# Examples:
#   ./scripts/build-docker.sh v1.0.0                    # Local build
#   ./scripts/build-docker.sh v1.0.0 ghcr.io/anomalyco # Push to registry
# =============================================================================

set -euo pipefail

# Configuration
VERSION="${1:-dev}"
REGISTRY="${2:-}"
IMAGE_NAME="muster"
# Added linux/arm/v7 for better compatibility
PLATFORMS="linux/amd64,linux/arm64,linux/arm/v7"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +%Y-%m-%d\ %H:%M:%S)]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date +%Y-%m-%d\ %H:%M:%S)] WARNING:${NC} $1"
}

info() {
    echo -e "${BLUE}[$(date +%Y-%m-%d\ %H:%M:%S)] INFO:${NC} $1"
}

error() {
    echo -e "${RED}[$(date +%Y-%m-%d\ %H:%M:%S)] ERROR:${NC} $1"
    exit 1
}

# Get git commit hash
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

log "Building Muster Docker image"
echo "  Version:    $VERSION"
echo "  Commit:     $GIT_COMMIT"
echo "  Build Date: $BUILD_DATE"
echo "  Platforms:  $PLATFORMS"

# Check if docker buildx is available
if ! docker buildx version &>/dev/null; then
    error "Docker buildx not available. Please install Docker Desktop or enable buildx"
fi

# Create a new buildx builder instance for multi-arch builds
BUILDER_NAME="muster-builder"
if docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
    log "Using existing buildx builder: $BUILDER_NAME"
else
    log "Creating new buildx builder: $BUILDER_NAME"
    docker buildx create --name "$BUILDER_NAME" --use --bootstrap || {
        warn "Failed to create builder with bootstrap, trying without..."
        docker buildx create --name "$BUILDER_NAME" --use
    }
fi

# Switch to the builder
docker buildx use "$BUILDER_NAME"

# Set up tags
TAGS=()
if [[ -z "$REGISTRY" ]]; then
    TAGS+=("-t" "${IMAGE_NAME}:${VERSION}")
    TAGS+=("-t" "${IMAGE_NAME}:latest")
else
    log "Building for registry: $REGISTRY"
    TAGS+=("-t" "${REGISTRY}/${IMAGE_NAME}:${VERSION}")
    TAGS+=("-t" "${REGISTRY}/${IMAGE_NAME}:latest")
fi

# Build arguments
BUILD_ARGS=(
    "--build-arg" "VERSION=$VERSION"
    "--build-arg" "GIT_COMMIT=$GIT_COMMIT"
    "--build-arg" "BUILD_DATE=$BUILD_DATE"
)

# Build options
BUILD_OPTS=("--platform" "$PLATFORMS")

# Enable layer caching with GitHub Actions cache
if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    info "Detected GitHub Actions environment - enabling cache"
    BUILD_OPTS+=("--cache-from" "type=gha")
    BUILD_OPTS+=("--cache-to" "type=gha,mode=max")
else
    # Local cache
    BUILD_OPTS+=("--cache-from" "type=local,src=/tmp/.buildx-cache")
    BUILD_OPTS+=("--cache-to" "type=local,dest=/tmp/.buildx-cache-new,mode=max")
fi

# Enable SBOM and provenance attestation (Docker BuildKit feature)
BUILD_OPTS+=("--sbom=true")
BUILD_OPTS+=("--provenance=true")

# Check if we should push
if [[ -n "$REGISTRY" ]]; then
    log "Pushing to registry: $REGISTRY"
    BUILD_OPTS+=("--push")
else
    log "Building locally (use with registry to push)"
    # Note: --load only works for single platform locally
    # For multi-platform local builds, we need to push to a registry
    if [[ "$PLATFORMS" == *","* ]]; then
        warn "Multi-platform builds require a registry. Building for current platform only."
        BUILD_OPTS+=("--load")
        # Override platforms to current only
        BUILD_OPTS=(${BUILD_OPTS[@]//"--platform" "$PLATFORMS"/})
        BUILD_OPTS+=("--platform" "linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')")
    else
        BUILD_OPTS+=("--load")
    fi
fi

# Build the multi-arch image
log "Starting multi-arch build..."
log "This may take several minutes for all platforms..."

docker buildx build \
    "${BUILD_OPTS[@]}" \
    "${TAGS[@]}" \
    "${BUILD_ARGS[@]}" \
    --progress=plain \
    --file Dockerfile \
    .

# Move cache
if [[ -d /tmp/.buildx-cache-new ]]; then
    rm -rf /tmp/.buildx-cache
    mv /tmp/.buildx-cache-new /tmp/.buildx-cache
fi

log "Build complete!"

# Show image info
if [[ -z "$REGISTRY" ]]; then
    log "Local images created:"
    docker images | grep "$IMAGE_NAME" || true
else
    log "Images pushed to: $REGISTRY/$IMAGE_NAME"
    info "Multi-arch image manifest:"
    docker buildx imagetools inspect "${REGISTRY}/${IMAGE_NAME}:${VERSION}" || true
fi

# Optional: Security scan with Trivy (if available)
if command -v trivy &> /dev/null && [[ -n "$REGISTRY" ]]; then
    log "Running security scan..."
    trivy image "${REGISTRY}/${IMAGE_NAME}:${VERSION}" || warn "Security scan completed with issues"
else
    info "Skipping security scan (Trivy not available or no registry)"
fi

log "Docker build script finished successfully"
