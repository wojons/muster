# WASM Module Examples

This directory contains example WebAssembly (WASM) modules for the Muster demo.

## Modules

### hello.wasm
Simple greeting module that demonstrates basic WASM loading and execution.

**Features:**
- Takes a name input
- Returns greeting text
- Demonstrates module loading

**Source**: `hello.wat`
**Build**: `wat2wasm hello.wat -o hello.wasm`

---

### transform.wasm
Response-transformation plugin for `openapi-cli request --wasm-plugin`.

**Features:**
- Reads the response body from stdin, writes the transformed body to stdout
- Wraps non-empty bodies in a deterministic JSON envelope:
  `{"hello":"world"}` → `{"wasm":true,"payload":{"hello":"world"}}`
- Empty stdin produces empty stdout (an empty body is never turned into output)

**Runtime contract**: plugins run as WASI *commands* (`pkg/wasm/module.go`), so
the module must define `_start` and use `fd_read`/`fd_write`. A module that
exports only a helper function is instantiated but runs nothing, which makes the
interceptor replace the response body with an empty one.

**Source**: `transform.wat`
**Build**: `wat2wasm transform.wat -o transform.wasm`

---

### validate.wasm
Request validation module for security checks.

**Features:**
- Validates input data
- Checks for required fields
- Enforces type constraints

**Source**: `validate.wat`
**Build**: `wat2wasm validate.wat -o validate.wasm`

---

## Building WASM Modules

### Prerequisites

You need the WebAssembly Binary Toolkit (WABT):

```bash
# macOS
brew install wabt

# Linux (Ubuntu/Debian)
sudo apt-get install wabt

# Or build from source
git clone https://github.com/WebAssembly/wabt.git
cd wabt
mkdir build && cd build
cmake ..
make -j
sudo make install
```

### Build All Modules

```bash
cd demo/wasm-modules
./build.sh
```

This will compile all `.wat` files to `.wasm` binaries.

---

## Using WASM Modules

Once built, WASM modules can be used by the Muster CLI:

```bash
# Generate commands with WASM support
muster-cli generate demo/openapi-specs/petstore.yaml

# Execute WASM module
muster-cli wasm execute demo/wasm-modules/hello.wasm --arg "World"
```

---

## WASM Testing

The demo includes WASM integration tests:

```bash
# Run WASM integration tests
go test -v ./demo/test-suites -run TestWASM
```

These tests validate:
- WASM runtime creation
- Capability enforcement
- Resource limits
- Sandbox isolation
- Network policies

**Note**: The WASM runtime is fully functional. The module binaries are provided for demonstration purposes.

---

## Creating Custom WASM Modules

Use the provided WAT files as templates:

```wat
(module
  ;; Import host functions
  (import "env" "log" (func $log (param i32 i32)))

  ;; Define memory
  (memory 1)

  ;; Export functions
  (export "my_function" (func $my_function))

  ;; Implementation
  (func $my_function (param i32) (result i32)
    ;; Your code here
    i32.const 0  ;; Return value
  )
)
```

Compile with: `wat2wasm custom.wat -o custom.wasm`
