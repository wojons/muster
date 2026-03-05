#!/bin/bash
set -e

# Installation script for muster

BINARY_NAME="muster"
REPO="anomalyco/muster"
INSTALL_DIR="/usr/local/bin"
VERSION=${VERSION:-"latest"}

# Detect platform
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch"; exit 1 ;;
    esac
    
    echo "${os}-${arch}"
}

# Download release
download_release() {
    local version=$1
    local platform=$2
    local url
    
    if [ "$version" = "latest" ]; then
        url="https://github.com/$REPO/releases/latest/download/${BINARY_NAME}-${version}-${platform}.tar.gz"
    else
        url="https://github.com/$REPO/releases/download/${version}/${BINARY_NAME}-${version}-${platform}.tar.gz"
    fi
    
    echo "Downloading from $url..."
    curl -fsSL "$url" -o "/tmp/${BINARY_NAME}.tar.gz"
}

# Install
install_binary() {
    echo "Installing to $INSTALL_DIR..."
    tar -xzf "/tmp/${BINARY_NAME}.tar.gz" -C /tmp/
    sudo install -m 755 "/tmp/${BINARY_NAME}" "$INSTALL_DIR/"
    rm -f "/tmp/${BINARY_NAME}.tar.gz" "/tmp/${BINARY_NAME}"
}

# Main
main() {
    echo "Installing $BINARY_NAME..."
    
    PLATFORM=$(detect_platform)
    echo "Detected platform: $PLATFORM"
    
    download_release "$VERSION" "$PLATFORM"
    install_binary
    
    echo "✓ $BINARY_NAME installed successfully!"
    echo "  Run '$BINARY_NAME --help' to get started"
}

main "$@"
