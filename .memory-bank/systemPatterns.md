# System Patterns

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      OpenAPI CLI + MCP Server                    │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────┐   │
│  │   CLI Mode  │  │  MCP Mode   │  │    Daemon Mode       │   │
│  │  (cobra)    │  │  (stdio)    │  │  (unix socket)       │   │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬───────────┘   │
│         │                │                    │               │
│         └────────────────┼────────────────────┘               │
│                          │                                     │
│  ┌───────────────────────┴───────────────────────┐             │
│  │             Core Application Layer             │             │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │             │
│  │  │ Command  │ │  OpenAPI │ │   Config     │  │             │
│  │  │ Generator│ │  Parser  │ │   Manager    │  │             │
│  │  └────┬─────┘ └────┬─────┘ └──────┬───────┘  │             │
│  │       └────────────┼──────────────┘          │             │
│  │                    │                         │             │
│  │  ┌─────────────────┴──────────────────┐      │             │
│  │  │      Execution Layer                │      │             │
│  │  │  ┌──────────┐ ┌──────────┐        │      │             │
│  │  │  │  WASM    │ │ Starlark │        │      │             │
│  │  │  │ Runtime  │ │   DSL    │        │      │             │
│  │  │  └──────────┘ └──────────┘        │      │             │
│  │  └────────────────────────────────────┘      │             │
│  └──────────────────────────────────────────────┘             │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                 Infrastructure Layer                     │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │   │
│  │  │  Auth    │ │  Cache   │ │  Rate    │ │ Database │   │   │
│  │  │(oauth2,  │ │(memory,  │ │ Limiter  │ │(sqlite,  │   │   │
│  │  │  mTLS)   │ │  redis)  │ │          │ │  pg)     │   │   │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Design Patterns

### 1. Clean Architecture

```
┌──────────────┐
│   Delivery   │  ← CLI, MCP, HTTP handlers
│   Layer      │
├──────────────┤
│  Application │  ← Use cases, command handlers
│    Layer     │
├──────────────┤
│   Domain     │  ← Business logic, entities
│    Layer     │
├──────────────┤
│ Infrastructure│ ← DB, HTTP, external services
└──────────────┘
```

### 2. Command Pattern

All CLI commands implement:
```go
type Command interface {
    Name() string
    Execute(ctx context.Context, args []string) error
}
```

### 3. Plugin Architecture

WASM plugins use capability-based security:
```go
type Capability string
const (
    CapabilityFileRead    Capability = "file:read"
    CapabilityNetworkHTTP Capability = "network:http"
)
```

### 4. Multi-Mode Support

Three deployment modes share core logic:
```go
type Mode interface {
    Start() error
    Stop() error
    Execute(cmd Command) error
}
```

## Key Technical Decisions

### 1. Go Language
- **Why**: Single binary, cross-compile, fast startup
- **Trade-off**: More verbose than Python, but better performance

### 2. Cobra + Viper
- **Why**: Industry standard for Go CLI tools
- **Pattern**: Command tree with persistent flags

### 3. Wasmtime 8.0.0
- **Why**: Deterministic, cross-language runtime
- **Critical**: All 12 SDKs use same version

### 4. Starlark DSL
- **Why**: Python-like syntax, sandboxed, deterministic
- **Use**: Simple automation scripts

### 5. SQLite Default
- **Why**: Zero-config, embedded, fast
- **Migration**: Support PostgreSQL/MySQL via config

## Component Relationships

### Command Generation Flow
```
OpenAPI Spec → Parser → Command Tree → Cobra → CLI
                    ↓
               MCP Registration
                    ↓
               Claude Desktop
```

### Authentication Flow
```
User Request → Auth Manager → Keychain/OAuth2/mTLS
                    ↓
               Token Store
                    ↓
               API Request
```

### WASM Execution Flow
```
WASM Module → Loader → Validator → Runtime → Sandbox
                                  ↓
                            Host Functions
                                  ↓
                            File/Network Access
```

## Determinism Patterns

### 1. Canonical NaN
All floating-point operations use canonical NaN representation

### 2. Sorted Iteration
Map iteration always sorted by key

### 3. Fixed Seeds
Random operations use deterministic seeds

### 4. Time Freezing
Tests use fixed timestamps

## Security Patterns

### 1. Defense in Depth
- Input validation at boundaries
- Sandboxed execution for WASM
- Least-privilege for capabilities

### 2. Secure Defaults
- No plaintext credential storage
- TLS 1.2+ required
- Rate limiting enabled

### 3. Audit Everything
- All auth attempts logged
- Config changes tracked
- Admin actions recorded

## Testing Patterns

### 1. Table-Driven Tests
```go
func TestX(t *testing.T) {
    tests := []struct{
        name string
        input string
        want string
    }{
        {"valid", "input", "output"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := X(tt.input)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### 2. Golden Files
Expected outputs stored in `testdata/` directories

### 3. Integration Tests
End-to-end tests with real (test) services

## Error Handling

### 1. Structured Errors
```go
type AppError struct {
    Code    ErrorCode
    Message string
    Cause   error
}
```

### 2. Error Wrapping
```go
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

### 3. User Messages
Technical errors converted to user-friendly messages at CLI boundary

## Performance Patterns

### 1. Lazy Loading
Resources loaded on first use

### 2. Connection Pooling
Database and HTTP connections reused

### 3. Caching Strategy
- L1: In-memory (fast, limited)
- L2: Disk (persistent)
- L3: Redis (distributed)

### 4. Streaming
Large responses processed as streams

## Configuration Hierarchy

```
Defaults → Config File → Environment → Flags
   ↑            ↑            ↑          ↑
 Lowest      Low         Medium      Highest
```

## Critical Implementation Paths

### 1. Startup
```
main() → Load Config → Init Logger → Register Commands → Start Loop
```

### 2. Command Execution
```
User Input → Parse → Validate → Execute → Format Output
```

### 3. MCP Request
```
JSON-RPC → Route → Execute → JSON Response
```

### 4. WASM Call
```
Load Module → Validate → Instantiate → Execute → Return
```

## File Organization

```
cmd/
  ├── openapi-cli/     # Main entry
  └── mcp-server/      # MCP entry

pkg/
  ├── cli/             # CLI framework
  ├── wasm/            # WASM runtime
  ├── mcp/             # MCP protocol
  ├── auth/            # Authentication
  ├── dsl/             # Starlark DSL
  ├── cache/           # Caching layer
  ├── config/          # Configuration
  └── openapi/         # OpenAPI parsing

internal/
  └── tests/           # Test utilities
```

## Dependencies

### Must-Have
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration
- `github.com/bytecodealliance/wasmtime-go` - WASM runtime
- `go.starlark.net/starlark` - Starlark interpreter

### Should-Have
- `github.com/getkin/kin-openapi` - OpenAPI parsing
- `github.com/charmbracelet/bubbletea` - Interactive UI
- `github.com/redis/go-redis` - Redis client

### Optional
- `github.com/prometheus/client_golang` - Metrics
- `github.com/golang-migrate/migrate` - Database migrations

## Version Compatibility

| Component | Version | Constraint |
|-----------|---------|------------|
| Go | 1.21+ | Build requirement |
| Wasmtime | 8.0.0 | Determinism |
| Cobra | 1.8+ | CLI framework |
| Viper | 1.18+ | Config management |

## Migration Strategy

1. **New Projects**: Start with latest versions
2. **Existing**: Test thoroughly before upgrading
3. **Breaking Changes**: Update specs first, then code

## Documentation References

- Architecture: `specs/ARCHITECTURE-VISUAL.md`
- Implementation: `specs/IMPLEMENTATION-MAPPING.md`
- Reference: `specs/REFERENCE-IMPLEMENTATION.md`
- Security: `specs/SECURITY-AUDIT-CHECKLIST.md`
