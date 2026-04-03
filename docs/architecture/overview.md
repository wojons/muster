# Architecture Overview

High-level system architecture and design principles for OpenAPI CLI.

## System Overview

OpenAPI CLI is a modular command-line tool that bridges OpenAPI specifications with CLI interactions, MCP server functionality, and extensible scripting.

### Core Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLI Frontend                                    │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐ │
│  │   Cobra CLI  │ │  Interactive │ │   Output     │ │   Shell Completion   │ │
│  │   Framework  │ │   Prompts    │ │  Formatters  │ │      Generators      │ │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────────┬───────────┘ │
│         │                │                │                  │             │
└─────────┼────────────────┼────────────────┼──────────────────┼─────────────┘
          │                │                │                  │
          ▼                ▼                ▼                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Command Generator Layer                              │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐         │
│  │   OpenAPI    │ │   Request    │ │   Response   │ │   Command    │       │
│  │   Parser     │ │   Builder    │ │   Handler    │ │   Store      │         │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘       │
│         │                │                │                │             │
└─────────┼────────────────┼────────────────┼────────────────┼───────────────┘
          │                │                │                │
          ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Business Logic Layer                               │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│  │   Inventory  │ │   Inventory  │ │    Cache     │ │   Events     │       │
│  │   Manager    │ │   Discovery  │ │   Manager    │ │  Dispatcher  │         │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘       │
│         │                │                │                │             │
└─────────┼────────────────┼────────────────┼────────────────┼───────────────┘
          │                │                │                │
          ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Service Layer                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│  │     HTTP     │ │     MCP      │ │   Webhook    │ │    Rate      │       │
│  │    Client    │ │    Server    │ │    Client    │ │   Limiter    │         │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘       │
│         │                │                │                │             │
└─────────┼────────────────┼────────────────┼────────────────┼───────────────┘
          │                │                │                │
          ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Platform Services                                  │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│  │     WASM     │ │   Starlark   │ │    Daemon    │ │   Tenant     │       │
│  │   Runtime    │ │  DSL Engine  │ │  Controller  │ │  Isolation   │         │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘       │
│         │                │                │                │             │
└─────────┼────────────────┼────────────────┼────────────────┼───────────────┘
          │                │                │                │
          ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Infrastructure                                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐│
│  │   Auth   │ │  Config  │ │ Logging  │ │ Metrics  │ │  Audit  │ │ Crypto ││
│  │ Manager  │ │ Manager  │ │ & Audit  │ │  Export  │ │ Storage │ │ Engine ││
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └───┬────┘│
│       │            │            │            │            │            │    │
└───────┼────────────┼────────────┼────────────┼────────────┼────────────┼────┘
        │            │            │            │            │            │
        ▼            ▼            ▼            ▼            ▼            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Observability & Compliance                            │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│  │  Distributed │ │  Compliance  │ │   Metrics  │ │     Log      │       │
│  │   Tracing    │ │ Framework    │ │    RED/    │ │   Alerting   │         │
│  │              │ │              │ │    USE     │ │              │         │
│  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘       │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Component Descriptions

### CLI Frontend

**Cobra Framework**: Provides the command-line structure, argument parsing, and built-in commands with context-aware help.

**Interactive Prompts**: `charmbracelet/bubbletea` for interactive CLI experiences with user-friendly prompts and validation.

**Output Formatters**: Supports JSON, YAML, table, text output formats with color highlighting and custom templates.

**Shell Completion Generators**: Automated shell completion for Bash, Zsh, Fish, and PowerShell with dynamic completion based on OpenAPI specs.

### Command Generator Layer

**OpenAPI Parser**: Uses `github.com/getkin/kin-openapi` to parse and validate OpenAPI 3.0/3.1 specifications with full schema validation.

**Request Builder**: Constructs HTTP requests from CLI arguments with parameter serialization, body formatting, and validation.

**Response Handler**: Processes and formats API responses with content-type detection, streaming support (SSE, NDJSON), and error handling.

**Command Store**: Persistent storage for OpenAPI specs with versioning, metadata, and fast lookup.

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

**Auth Manager**: OAuth2 (PKCE, Device Code, Refresh), mTLS (X.509 certificates), API key support with platform-native credential storage (keychain, Secret Service, Credential Manager).

**Config Manager**: Viper-based configuration with file, environment variable, and command-line flag support, profile-based settings.

**Logging & Audit**: Structured logging (`uber-go/zap`) with configurable levels, correlation IDs, sensitive data redaction, and audit trail with hash chain.

**Metrics Export**: Prometheus-compatible metrics with RED/USE methodology, cardinality control, and SLO-based alerting.

**Audit Storage**: File-based and memory-based audit log storage with pagination and retention policies.

**Crypto Engine**: Comprehensive cryptographic toolkit including RSA, ECDSA, AES-GCM, ChaCha20-Poly1305, PBKDF2, Argon2id, HMAC, HKDF.

### Observability & Compliance

**Distributed Tracing**: OpenTelemetry integration with Jaeger/Zipkin exporters, trace context propagation, and span management.

**Compliance Framework**: Multi-framework compliance support (Bible Standard, OpenAPI, MCP, OAuth2, TLS, GDPR, HIPAA, PCI DSS, FIPS 140-3) with evidence collection and report generation.

**Metrics (RED/USE)**: Standardized metrics following RED (Rate, Errors, Duration) and USE (Utilization, Saturation, Errors) patterns.

**Log Alerting**: Rule-based log alerting with webhook, Slack, and PagerDuty integrations.

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
