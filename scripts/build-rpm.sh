#!/bin/bash
# Build RPM package for Muster

set -e

VERSION=${1:-1.0.0}

# Create RPM build structure
mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Copy source files
if [ ! -f "dist/muster-linux-amd64" ]; then
  echo "Error: dist/muster-linux-amd64 not found. Build binary first with:"
  echo "  go build -o dist/muster-linux-amd64 ./cmd/openapi-cli"
  exit 1
fi

cp dist/muster-linux-amd64 ~/rpmbuild/SOURCES/
cp packaging/rpm/muster.spec ~/rpmbuild/SPECS/

# Create tarball
tar czf ~/rpmbuild/SOURCES/muster-${VERSION}.tar.gz -C . \
  --exclude='.git' --exclude='dist' --exclude='bin' --exclude='.ralph' --exclude='tmp' \
  internal/platform linux pkg/cmdstore packaging scripts charts k8s configs debian \
  desktop systemd docs migrations test api web .memory-bank .agents specs \
  AGENTS.md CONTRIBUTING.md LICENSE Makefile README.md go.mod go.sum

# Build RPM
rpmbuild -bb ~/rpmbuild/SPECS/muster.spec

echo "RPM built: ~/rpmbuild/RPMS/x86_64/muster-${VERSION}-1.x86_64.rpm"
