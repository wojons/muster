#!/bin/bash
# Build WASM modules from WAT source files
# Requires: wat2wasm (from WABT toolkit)

set -e

echo "Building WASM modules..."

# Check for wat2wasm
if ! command -v wat2wasm &> /dev/null; then
    echo "ERROR: wat2wasm not found"
    echo ""
    echo "Install WABT toolkit:"
    echo "  macOS:   brew install wabt"
    echo "  Linux:   apt-get install wabt"
    echo "  Or from: https://github.com/WebAssembly/wabt"
    exit 1
fi

# Build hello.wasm
echo "Building hello.wasm..."
wat2wasm hello.wat -o hello.wasm
echo "✓ hello.wasm built"

# Build transform.wasm
echo "Building transform.wasm..."
wat2wasm transform.wat -o transform.wasm
echo "✓ transform.wasm built"

# Build validate.wasm
echo "Building validate.wasm..."
wat2wasm validate.wat -o validate.wasm
echo "✓ validate.wasm built"

echo ""
echo "All WASM modules built successfully!"
ls -lh *.wasm
