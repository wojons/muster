#!/bin/bash
# Build Debian (.deb) package for Muster

set -e

VERSION=${1:-1.0.0}
ARCH=${2:-amd64}
PKG_DIR="muster_${VERSION}_${ARCH}"

# Create package structure
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/usr/share/doc/muster"
mkdir -p "$PKG_DIR/etc/muster"

# Copy binary (should be built first)
if [ ! -f "dist/muster-linux-$ARCH" ]; then
  echo "Error: dist/muster-linux-$ARCH not found. Build binaries first with:"
  echo "  go build -o dist/muster-linux-$ARCH ./cmd/openapi-cli"
  exit 1
fi

cp dist/muster-linux-$ARCH "$PKG_DIR/usr/bin/muster"
chmod 755 "$PKG_DIR/usr/bin/muster"

# Copy control file and scripts
cat > "$PKG_DIR/DEBIAN/control" <<EOF
Package: muster
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Depends: libc6 (>= 2.31), ca-certificates
Recommends: gnome-keyring | kwallet-cli
Suggests: muster-completion
Maintainer: Muster Team <team@muster.sh>
Description: Modern API Client and Development Tool
 Muster is a powerful CLI tool for API development and testing.
 Features OpenAPI integration, authentication management, and
 extensible workflows.
Homepage: https://muster.sh
EOF

cp packaging/deb/preinst "$PKG_DIR/DEBIAN/"
cp packaging/deb/postinst "$PKG_DIR/DEBIAN/"
cp packaging/deb/prerm "$PKG_DIR/DEBIAN/"
cp packaging/deb/postrm "$PKG_DIR/DEBIAN/"
chmod 755 "$PKG_DIR/DEBIAN/"{preinst,postinst,prerm,postrm}

# Create control directory metadata
dpkg-gencontrol -p$PKG_DIR -P"$PKG_DIR" -c"$PKG_DIR/DEBIAN/control"

# Build package
dpkg-deb --build "$PKG_DIR"

echo "Package built: ${PKG_DIR}.deb"
