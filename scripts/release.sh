#!/bin/bash
set -e

# Release automation script

BINARY_NAME="muster"
VERSION=${1:-$(git describe --tags --always --dirty)}
DIST_DIR="dist"
GITHUB_TOKEN=${GITHUB_TOKEN:-}

if [ -z "$VERSION" ]; then
    echo "Error: No version specified"
    exit 1
fi

echo "Creating release $VERSION..."

# Clean and prepare
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Run tests
echo "Running tests..."
make test

# Build all platforms
echo "Building releases..."
make release VERSION="$VERSION"

# Create checksums
echo "Creating checksums..."
cd "$DIST_DIR"
shasum -a 256 *.tar.gz *.zip > checksums.txt 2>/dev/null || true
cd ..

# Create GitHub release (if gh CLI is available)
if command -v gh >/dev/null 2>&1 && [ -n "$GITHUB_TOKEN" ]; then
    echo "Creating GitHub release..."
    gh release create "$VERSION" \
        --title "Release $VERSION" \
        --notes "Release $VERSION" \
        dist/*
fi

echo "Release $VERSION complete!"
echo "Artifacts in $DIST_DIR/"
