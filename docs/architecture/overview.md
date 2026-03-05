# Architecture Overview

High-level system architecture and design principles for OpenAPI CLI.

## System Overview

OpenAPI CLI is a modular command-line tool that bridges OpenAPI specifications with CLI interactions, MCP server functionality, and extensible scripting.

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI Frontend                         │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐│
│  │   Cobra CLI   │ │  Interactive  │ │   Output Formatters  ││
│  │   Framework   │ │   Prompts    │ │    (json/table)     ││
│  └──────┬───────┘ └──────┬───────┘ └──────────┬───────────┘│
│         │                  │                      │         │
└─────────┼──────────────────┼──────────────────────┼─────────┘
          │                  │                      │
          ▼                  ▼                      ▼
┌─────────────────────────────────────────────────────────────┐
│                   Command Generator Layer                    │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐│
│  │  OpenAPI     │ │   Request    │ │    Response          ││
│  │   Parser     │ │    Builder   │ │     Handler          ││
│  └──────┬───────┘ └──────┬───────┘ └──────────┬───────────┘│
│         │                  │                      │         │
└─────────┼──────────────────┼──────────────────────┼─────────┘
          │                  │                      │
          ▼                  ▼                      ▼
┌─────────────────────────────────────────────────────────────┐
│                      Business Logic Layer                    │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐│
│  │   Inventory  │ │   Inventory  │ │    Cache             ││
│  │   Manager    │ │   Discovery  │ │    Manager           ││
│  └──────┬───────┘ └──────┬───────┘ └──────────┬───────────┘│
└─────────┼──────────────────┼──────────────────────┼─────────┘
          │                  │                      │
          ▼                  ▼                      ▼
┌─────────────────────────────────────────────────────────────┐
│                     Service Layer                            │
│  ┌──────────────┐ ┌──────────────────────────────────────┐ │
│  │   HTTP       │ │         MCP Server                    │ │
│  │   Client     │ │         (JSON-RPC)                    │ │
│  └──────┬───────┘ └──────────────────────┬───────────────┘ │
└─────────┼──────────────────────────────────┼────────────────┘
          │                                  │
          ▼                                  ▼
┌─────────────────────────────────────────────────────────────┐
│                  Platform Services                           │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐         │
│  │  WASM        │ │  Starlark    │ │  Daemon      │         │
│  │  Runtime     │ │  DSL Engine  │ │  Controller  │         │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘         │
└─────────┼──────────────────┼──────────────────┼─────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                   Infrastructure                             │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐         │
│  │   Auth       │ │   Config     │ │   Logging    │         │
│  │   Manager    │ │   Manager    │ │   & Audit    │         │
│  └──────────────┘ └──────────────┘ └──────────────┘         │
└─────────────────────────────────────────────────────────────┘
```

## Component Descriptions

### CLI Frontend

**Cobra Framework**: Provides the command-line structure, argument parsing, and built-in commands.

**Interactive Prompts**: `charmbracelet/bubbletea` for interactive CLI experiences with user-friendly prompts.

**Output Formatters**: Supports JSON, YAML, table, text output formats with color highlighting.

### Command Generator Layer

**OpenAPI Parser**: Uses `github.com/getkin/kin-openapi` to parse and validate OpenAPI 3.0/3.1 specifications.

**Request Builder**: Constructs HTTP requests from CLI arguments with parameter serialization and body formatting.

**Response Handler**: Processes and formats API responses with content-type detection and error handling.

### Business Logic Layer

**Inventory Manager**: Manages API endpoint discovery, caching, and retrieval with deduplication and validation.

**Inventory Discovery**: Dynamic endpoint discovery from OpenAPI specs with tag-based grouping and filtering.

**Cache Manager**: Multi-tier caching (memory, Redis) with TTL-based eviction and cache invalidation.

### Service Layer

**HTTP Client**: Configurable HTTP client with retry logic, timeout handling, rate limiting, and connection pooling.

**MCP Server**: Model Context Protocol server over stdio with tool generation from OpenAPI specs and MCP 2024-11-05 compliance.

### Platform Services

**WASM Runtime**: `tetratelabs/wazero` Go-native WebAssembly runtime with capability-based sandboxing.

**Starlark DSL**: `go.starlark.net` for embedded scripting with Go-Starlark value conversion and built-in functions.

**Daemon Controller**: Background process management with Unix sockets and graceful shutdown.

### Infrastructure

**Auth Manager**: OAuth2, mTLS, API key support with platform-native credential storage (keychain, Secret Service, Credential Manager).

**Config Manager**: Viper-based configuration with file, environment variable, and command-line flag support.

**Logging**: Structured logging (`uber-go/zap`) with configurable levels and output formats.

## Data Flow

### CLI Request Flow

```
User Input
    ↓
CLI Frontend (Cobra)
    ↓
Command Generator
    ↓
Business Logic (Inventory)
    ↓
Service Layer (HTTP Client)
    ↓
API Response
    ↓
Response Handler
    ↓
Output Formatter
    ↓
User Output
```

### MCP Server Flow

```
MCP Client Request (stdio)
    ↓
JSON-RPC Parser
    ↓
Handler Registry (tools/list, tools/call)
    ↓
OpenAPI Spec Parser
    ↓
Tool Generation & Execution
    ↓
API Request (HTTP Client)
    ↓
JSON-RPC Response
    ↓
MCP Client Response
```

### WASM Module Execution Flow

```
Load .wasm File
    ↓
Validation & Security Check
    ↓
Wazero Runtime Initialization
    ↓
Capability Check
    ↓
Module Execution (with limits)
    ↓
Resource Cleanup
    ↓
Result
```

## Communication Layers

### CLI to API

```
openapi-cli pet find-id --petId 1
    ↓
Command parsing
    ↓
OpenAPI spec lookup (/pet/{petId}, GET)
    ↓
Request building (GET https://api.example.com/v1/pet/1)
    ↓
HTTP request execution
    ↓
Response formatting
    ↓
Table/json output
```

### MCP to CLI

```
AI Assistant → MCP Client (JSON-RPC)
    ↓
MCP Server (stdio)
    ↓
/tools/list - Returns tool definitions from spec
    ↓
/tools/call - Executes API calls
    ↓
Returns JSON response with content
```

## Security Architecture

### Authentication Flow

```
Client Request
    ↓
Auth Token Validation
    ↓
Credential Storage Check (keychain/Secret Service)
    ↓
Token Refresh (if expired)
    ↓
Signed API Request
    ↓
API Response
```

### WASM Sandbox

```
WASM Module Load
    ↓
Capability Check (allowed domains, filesystem access)
    ↓
Resource Limits (memory, CPU, time)
    ↓
Execution in Sandbox
    ↓
Result Validation
    ↓
Response Return
```

## Error Handling Strategy

### Layered Error Handling

1. **CLI Layer**: User-facing error messages with exit codes
2. **Business Logic**: Contextual errors with retry logic
3. **Service Layer**: HTTP errors with status codes
4. **Infrastructure**: System errors with stack traces

### Error Propagation

```
Error Condition
    ↓
Error Wrapping (with context)
    ↓
Log Error (structured)
    ↓
User-Facing Message (if CLI)
    ↓
Appropriate Exit Code
```

## Performance Characteristics

### Caching Strategy

- **L1**: Memory cache (LRU, 1000 items)
- **L2**: Network cache (Redis optional)
- **TTL**: 3600 seconds default
- **Invalidation**: Manual or TTL-based

### Rate Limiting

- **Algorithm**: Token bucket
- **Default**: 10 requests/second
- **Burst**: 100 requests
- **Per-endpoint**: Configurable

### Concurrency

- **Max concurrent**: 100 requests
- **Connection pooling**: 10 connections per host
- **Timeouts**: Default 30 seconds

## Extension Points

### Custom Formatters

```go
type Formatter interface {
    Format(response []byte, contentType string) ([]byte, error)
    Supports(contentType string) bool
}
```

### Custom Auth Providers

```go
type AuthProvider interface {
    Authenticate() (string, error)
    Refresh() (string, error)
    Validate(token string) bool
}
```

### WASM Modules

Load and execute custom WASM modules:

```bash
openapi-cli wasm run module.wasm --function process
```

## Configuration Architecture

### Configuration Hierarchy

1. **Defaults**: Built-in default values
2. **Global Config**: `~/.config/openapi-cli/config.yaml`
3. **Project Config**: `./openapi-cli.yaml`
4. **Environment Variables**: `MUSTER_*`
5. **Command-line Flags**: Override all

### Profile Support

```yaml
profiles:
  development:
    base_url: https://dev.example.com
    log_level: debug
  production:
    base_url: https://api.example.com
    log_level: error
```

## Trade-offs and Design Decisions

### Go vs. Rust

**Decision**: Go was chosen for the implementation due to:

- **Maturity**: Strong ecosystem for CLI tools
- **Cross-platform**: First-class support
- **Wazero**: Pure Go WASM runtime (no CGO dependency)
- **Cobra**: Battle-tested CLI framework

### WASM Runtime Selection

**Decision**: `wazero` over `wasmer-go` due to:

- **Pure Go**: No CGO requirement
- **Performance**: Comparable performance
- **Simplicity**: Easier embedding

### Starlark for DSL

**Decision**: Starlark (Python-like) due to:

- **Familiarity**: Python-like syntax
- **Safety**: Sandboxed by design
- **Integration**: Good Go bindings

## Future Extensions

### Planned Features

- **GraphQL Support**: GraphQL spec parsing and CLI generation
- **Plugin System**: Extensible command plugins
- **Web UI**: Browser-based API testing interface
- **OpenTelemetry**: Distributed tracing integration

### API Versioning

MCP server will follow semver with protocol compatibility guarantees.

## References

- [OpenAPI Specification](https://swagger.io/specification/)
- [MCP Protocol](https://modelcontextprotocol.io/)
- [Wazero Documentation](https://wazero.io/)
- [Cobra Documentation](https://github.com/spf13/cobra)
