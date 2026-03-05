#!/bin/bash
set -e

# Cross-platform build script

BINARY_NAME="muster"
CMD_DIR="cmd"
BUILD_DIR="bin"

# Get version info
source scripts/version.sh > /dev/null

# Build flags
LDFLAGS="-X github.com/anomalyco/muster/internal/version.Version=$VERSION"
LDFLAGS="$LDFLAGS -X github.com/anomalyco/muster/internal/version.Commit=$COMMIT"
LDFLAGS="$LDFLAGS -X github.com/anomalyco/muster/internal/version.BuildDate=$BUILD_DATE"
LDFLAGS="$LDFLAGS -s -w"

# Platforms
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

build_platform() {
    local goos=$1
    local goarch=$2
    local output="$BUILD_DIR/${BINARY_NAME}-${goos}-${goarch}"
    
    if [ "$goos" = "windows" ]; then
        output="${output}.exe"
    fi
    
    echo "Building for $goos/$goarch..."
    CGO_ENABLED=1 GOOS=$goos GOARCH=$goarch \
        go build -ldflags "$LDFLAGS" -o "$output" ./$CMD_DIR
}

# Main
mkdir -p "$BUILD_DIR"

if [ -n "$1" ] && [ -n "$2" ]; then
    # Build specific platform
    build_platform "$1" "$2"
else
    # Build all platforms
    for platform in "${PLATFORMS[@]}"; do
        IFS='/' read -r goos goarch <<< "$platform"
        build_platform "$goos" "$goarch"
    done
fi

echo "Build complete!"
