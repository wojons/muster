#!/bin/bash
set -e

# Cross-platform build script

BINARY_NAME="muster"
MCP_BINARY_NAME="muster-mcp"
DAEMON_BINARY_NAME="muster-daemon"
BUILD_DIR="bin"

# Resolve paths so the script can be invoked from any directory.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# Get version info
source "$SCRIPT_DIR/version.sh" > /dev/null
VERSION="${VERSION:-$(get_version)}"
COMMIT="${COMMIT:-$(get_commit)}"
BUILD_DATE="${BUILD_DATE:-$(get_build_date)}"

# Build flags
LDFLAGS="-X github.com/wojons/muster/internal/version.Version=$VERSION"
LDFLAGS="$LDFLAGS -X github.com/wojons/muster/internal/version.Commit=$COMMIT"
LDFLAGS="$LDFLAGS -X github.com/wojons/muster/internal/version.BuildDate=$BUILD_DATE"
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

build_binary() {
    local entrypoint=$1
    local output=$2

    echo "  → Building $entrypoint as $output..."
    CGO_ENABLED="${CGO_ENABLED:-1}" GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags "$LDFLAGS" -o "$output" "$entrypoint"
}

build_platform() {
    local goos=$1
    local goarch=$2
    local suffix="$goos-$goarch"
    local cli_output="$BUILD_DIR/${BINARY_NAME}-${suffix}"
    local mcp_output="$BUILD_DIR/${MCP_BINARY_NAME}-${suffix}"
    local daemon_output="$BUILD_DIR/${DAEMON_BINARY_NAME}-${suffix}"

    if [ "$goos" = "windows" ]; then
        cli_output="${cli_output}.exe"
        mcp_output="${mcp_output}.exe"
        daemon_output="${daemon_output}.exe"
    fi

    echo "Building for $goos/$goarch..."
    GOOS="$goos"
    GOARCH="$goarch"
    build_binary "./cmd/openapi-cli" "$cli_output"
    build_binary "./cmd/mcp-server" "$mcp_output"
    build_binary "./cmd/daemon" "$daemon_output"
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
