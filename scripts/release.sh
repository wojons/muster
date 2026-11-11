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

# Create checksums — FAIL CLOSED: a missing/garbled artifact must stop the
# release, never publish an unverifiable archive (Carter r2 RELEASE-015).
echo "Creating checksums..."
cd "$DIST_DIR"
ARTIFACTS=$(ls *.tar.gz *.zip 2>/dev/null)
if [ -z "$ARTIFACTS" ]; then
    echo "Error: no release artifacts found in $DIST_DIR — refusing to continue" >&2
    exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    sha256sum $ARTIFACTS > checksums.txt
else
    shasum -a 256 $ARTIFACTS > checksums.txt
fi
echo "Checksums written for $(wc -l < checksums.txt) artifacts"
cd ..

# GitHub publishing is owned by ONE path: the CI workflow (goreleaser via
# softprops/action-gh-release). This script deliberately does NOT publish —
# the previous double-publish (script gh release create + workflow release)
# is the defect Carter r2 named. To publish from a machine, use goreleaser:
#   goreleaser release --clean
echo "Release $VERSION built. Publishing is owned by the CI workflow (goreleaser)."
echo "Artifacts in $DIST_DIR/"
