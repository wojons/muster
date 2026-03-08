# System Patterns

## Architecture
```
cmd/             # Entry points (3)
├── openapi-cli  # Main CLI
├── mcp-server   # MCP protocol
└── daemon       # Background process

pkg/             # Public APIs (26+ packages)
internal/        # Private utilities
specs/           # 227 specification files
demo/            # Working examples
```

## Key Patterns
1. **Integrated System**: Every component must connect (unit + integration + E2E tests)
2. **Security by Design**: Multiple layers (keychain encryption, rate limiting, DoS protection)
3. **Plugin System**: WASM modules for extensibility
4. **Starlark DSL**: Configurable execution
5. **Multi-tier Caching**: Memory + Redis layers

## Component Connections
- OpenAPI Parser → Command Generator
- CLI → MCP Server
- MCP Server → WASM Runtime
- Auth → Protected Endpoints
- Daemon → Socket → Proxy

## Critical Paths
1. Spec parsing: pkg/openapi → cmd/openapi-cli
2. WASM execution: pkg/wasm → Wasmtime runtime
3. Auth: pkg/auth → OS keychains
4. Caching: pkg/cache → memory + Redis
