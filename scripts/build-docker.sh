#!/bin/bash
# scripts/build-docker.sh

set -euo pipefail

VERSION=${VERSION:-"1.0.0"}
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
REGISTRY=${REGISTRY:-"ghcr.io/openapi-cli"}
IMAGE_NAME=${IMAGE_NAME:-"openapi-cli"}

# Build arguments
BUILD_ARGS=(
    "--build-arg VERSION=${VERSION}"
    "--build-arg GIT_COMMIT=${GIT_COMMIT}"
    "--build-arg BUILD_DATE=${BUILD_DATE}"
)

# Platforms
PLATFORMS="linux/amd64,linux/arm64"

echo "Building Docker image..."
echo "  Version: ${VERSION}"
echo "  Commit: ${GIT_COMMIT}"
echo "  Date: ${BUILD_DATE}"
echo "  Platforms: ${PLATFORMS}"
echo "  Registry: ${REGISTRY}"
echo "  Image: ${IMAGE_NAME}"

# Build multi-platform image
docker buildx build \
    --platform "${PLATFORMS}" \
    "${BUILD_ARGS[@]}" \
    --tag "${REGISTRY}/${IMAGE_NAME}:${VERSION}" \
    --tag "${REGISTRY}/${IMAGE_NAME}:latest" \
    --push \
    --provenance=true \
    --sbom=true \
    .

echo "Build complete!"
echo "  Image: ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
