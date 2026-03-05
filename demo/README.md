# Demo Application

This directory contains a complete demonstration of the OpenAPI CLI + MCP Server system. It proves all components work together.

## Quick Start

```bash
# Run the full demo
make demo

# Or run individual parts:
./demo/test-suites/01-cli-generation.sh
./demo/test-suites/02-mcp-flow.sh
./demo/test-suites/03-wasm-execution.sh
```

## Structure

```
demo/
├── openapi-specs/       # Test API specifications
│   ├── petstore.yaml    # Standard pet store API
│   ├── auth-api.yaml    # OAuth2/mTLS auth examples
│   └── simple.yaml      # Minimal test API
├── wasm-modules/        # Example WASM modules
│   ├── hello.wasm       # Simple greeting module
│   ├── transform.wasm   # Data transformation
│   └── validate.wasm    # Request validation
├── scripts/             # Starlark DSL scripts
│   ├── hello.star       # Basic script
│   ├── workflow.star    # Multi-step workflow
│   └── transform.star   # Data processing
├── test-suites/         # Integration tests
│   ├── 01-cli-generation.sh
│   ├── 02-mcp-flow.sh
│   ├── 03-wasm-execution.sh
│   ├── 04-auth-flows.sh
│   ├── 05-daemon-socket.sh
│   └── 06-full-stack.sh
└── resources/           # Supporting files
    └── test-server.go   # Mock API server
```

## Integration Test Scenarios

### 1. CLI Generation (01-cli-generation.sh)
**Proves**: OpenAPI Parser → Command Generator
- Parse petstore.yaml
- Generate CLI commands
- Validate command structure

### 2. MCP Flow (02-mcp-flow.sh)
**Proves**: CLI → MCP Server → Tool Registration
- Start MCP server
- Register tools from spec
- Execute tool call

### 3. WASM Execution (03-wasm-execution.sh)
**Proves**: MCP Server → WASM Runtime → Execution
- Load WASM module
- Execute in sandbox
- Return result

### 4. Auth Flows (04-auth-flows.sh)
**Proves**: Authentication → Protected Endpoints
- OAuth2 flow
- mTLS authentication
- Token management

### 5. Daemon Socket (05-daemon-socket.sh)
**Proves**: Daemon → Socket → Proxy → Response
- Start daemon
- Connect via socket
- Route request

### 6. Full Stack (06-full-stack.sh)
**Proves**: Complete pipeline
- Load spec → Parse → Generate → Execute → Validate

## Running Tests

```bash
# All integration tests
make test-integration

# Individual test suites
./demo/test-suites/01-cli-generation.sh
./demo/test-suites/02-mcp-flow.sh
# etc.

# End-to-end tests
make test-e2e
```

## Mock Server

For testing without external dependencies:

```bash
# Start mock server
go run demo/resources/test-server.go &

# Run tests against mock
./demo/test-suites/run-all.sh --mock
```

## Verification

Before claiming completion, verify:

- [ ] `make demo` completes successfully
- [ ] All 6 test suites pass
- [ ] No errors in logs
- [ ] Can exercise ALL major features
- [ ] Components connect properly