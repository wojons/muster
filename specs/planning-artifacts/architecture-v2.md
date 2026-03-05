# Architecture Document v2: Technical Decisions Implemented

## Executive Summary

**Language**: Go (golang) - single static binary, fast compilation  
**WASM**: Wasmtime with wasmtime-go bindings  
**DSL**: Starlark (go.starlark.net) - Python-like, sandboxed  
**Server**: SQLite → MySQL/PostgreSQL + S3/CloudFront support  
**Distribution**: Bootstrap installer + self-update  
**Database**: sqlc + golang-migrate

## Updated Architecture Overview

### Three Deployment Modes

```
┌─────────────────────────────────────────────────────────────────┐
│                    DEPLOYMENT MODES                             │
├─────────────────┬──────────────────┬────────────────────────────┤
│   STANDALONE    │   HTTP SERVER    │   PROXY DAEMON             │
│   (Default)     │   (Multi-client) │   (Enterprise)             │
├─────────────────┼──────────────────┼────────────────────────────┤
│ • stdio MCP     │ • HTTP MCP       │ • Unix socket              │
│ • Direct API    │ • Multiple       │ • One TLS conn/user        │
│ • Self-contained│   clients        │ • All traffic proxied      │
│ • Per-session   │ • Shared cache   │ • Centralized audit        │
└─────────────────┴──────────────────┴────────────────────────────┘
```

### Core System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        ENTRY POINTS                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │  CLI Mode   │  │  MCP stdio  │  │  MCP HTTP / Daemon      │  │
│  │             │  │  Mode       │  │  Mode                   │  │
│  │ openapi-cli │  │ --mcp       │  │ --mcp-server --daemon   │  │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘  │
└─────────┼────────────────┼─────────────────────┼────────────────┘
          │                │                     │
          └────────────────┼─────────────────────┘
                           │
                           ▼
          ┌────────────────────────────────────┐
          │         SHARED CORE MODULES        │
          │  ┌──────────────────────────────┐  │
          │  │   INVENTORY MANAGER          │  │
          │  │  • Multi-source (URL/file)   │  │
          │  │  • HTTP caching (ETag/CC)    │  │
          │  │  • Multi-version storage     │  │
          │  │  • Offline fallback          │  │
          │  └──────────────────────────────┘  │
          │                                    │
          │  ┌──────────────────────────────┐  │
          │  │   CONFIGURATION HUB          │  │
          │  │  • CLI flags (highest)       │  │
          │  │  • Environment variables     │  │
          │  │  • Config files              │  │
          │  │  • Inventory defaults        │  │
          │  └──────────────────────────────┘  │
          │                                    │
          │  ┌──────────────────────────────┐  │
          │  │   SECURITY LAYER             │  │
          │  │  • Multi-auth (OAuth, mTLS)  │  │
          │  │  • OS keychain integration   │  │
          │  │  • Client fingerprinting     │  │
          │  │  • Inventory signing verify  │  │
          │  └──────────────────────────────┘  │
          │                                    │
          │  ┌──────────────────────────────┐  │
          │  │   EXECUTION ENGINE           │  │
          │  │  • HTTP client (Axios)       │  │
          │  │  • Middleware pipeline       │  │
          │  │  • Rate limiting             │  │
          │  │  • Retry logic               │  │
          │  └──────────────────────────────┘  │
          │                                    │
          │  ┌──────────────────────────────┐  │
          │  │   SANDBOXED EXECUTION        │  │
          │  │  • Starlark DSL (simple)     │  │
          │  │  • WASM runtime (complex)    │  │
          │  │  • Capability-based perms    │  │
          │  └──────────────────────────────┘  │
          └────────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          │                │                │
          ▼                ▼                ▼
   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
   │   Inventory  │ │   Target     │ │   Module     │
   │   Server     │ │   API        │ │   Registry   │
   └──────────────┘ └──────────────┘ └──────────────┘
```

## Technology Stack (Confirmed)

### Language & Runtime
- **Language**: Go (golang) 1.21+
- **Compilation**: `go build` → single static binary
- **Distribution**: Bootstrap installer + self-update
- **Advantages**:
  - Single binary with no runtime dependencies
  - Fast startup (critical for CLI)
  - Excellent concurrency (goroutines for multiplexing/proxy)
  - Cross-compile trivially
  - Fast compilation (LLM-friendly)
  - Great WASM hosting (wasmtime-go)

### Core Dependencies
```go
import (
    // CLI
    "github.com/spf13/cobra"          // CLI framework
    "github.com/spf13/viper"          // Config management
    
    // MCP
    "github.com/mcp-sdk/mcp-go"       // MCP SDK
    
    // WASM
    "github.com/bytecodealliance/wasmtime-go"  // WASM runtime
    
    // DSL
    "go.starlark.net"                 // Starlark interpreter
    
    // HTTP
    "github.com/go-resty/resty/v2"    // HTTP client
    
    // Secrets
    "github.com/99designs/keyring"    // OS keychain
    
    // Auth
    "golang.org/x/oauth2"             // OAuth2
    "github.com/golang-jwt/jwt/v5"    // JWT
    
    // Database
    "github.com/sqlc-dev/sqlc"        // Code generation
    "github.com/golang-migrate/migrate/v4"  // Migrations
)
```

### WASM Runtime: Wasmtime
**Why Wasmtime:**
- Production proven (Fastly, Microsoft, Shopify)
- WASI standard support
- Security-first design
- wasmtime-go bindings available
- Active development by Bytecode Alliance

**Sandboxing Features:**
- Capability-based security model
- Configurable resource limits (memory, CPU)
- No filesystem/network access by default
- Explicit permission grants only

### DSL: Starlark
**Why Starlark:**
- Python-like syntax (familiar)
- Deterministic execution
- Already sandboxed (designed for Bazel)
- Easy to extend with custom functions

**Extensions for our use case:**
```python
# HTTP primitives
result = http_get("/users/123")
result = http_post("/users", {"name": "John"})
result = http_patch("/users/123", {"email": "new@example.com"})
result = http_delete("/users/123")

# JSON handling
data = json_parse(result.body)
json_str = json_encode(data)

# Data extraction (JSONPath-style)
items = jsonpath(data, "$.items[*]")
name = jsonpath(data, "$.user.name")

# Control flow
for item in items:
    http_post("/process", {"id": item.id})

if data.status == "active":
    http_post("/notify", {"user": data.id})

# Response building
return {"processed": len(items), "data": data}
```

## Inventory System Deep Dive

### Inventory Schema (v2)

```typescript
interface Inventory {
  // Version info
  version: string;
  min_cli_version: string;
  max_cli_version: string;
  
  // OpenAPI spec location
  openapi: {
    source: 'embedded' | 'url';
    content?: string;        // If embedded
    url?: string;            // If URL
    sha256: string;
  };
  
  // Execution modules
  modules: {
    processors: ModuleRef[];
    validators: ModuleRef[];
    auth_handlers: ModuleRef[];
  };
  
  // Authentication config
  auth: {
    inventory_auth: AuthConfig;  // To fetch this inventory
    api_auth: AuthConfig;        // To call the API
    key_server?: KeyServerConfig;
  };
  
  // Client-side controls
  config: {
    rate_limiting: RateLimitConfig;
    retry: RetryConfig;
    fingerprinting: FingerprintConfig;
    capabilities: CapabilityConfig;
  };
  
  // Signing
  signatures: {
    method: 'tls' | 'gpg' | 'ssh';
    public_key?: string;
    signature: string;
  };
}

interface ModuleRef {
  name: string;
  url: string;
  sha256: string;
  signature: string;
  capabilities: string[];
}
```

### Caching Strategy

```
~/.cache/openapi-cli/
├── inventory/
│   └── <url-hash>/
│       ├── v1.0.0.json
│       ├── v1.1.0.json
│       ├── v1.2.0.json
│       ├── latest -> v1.2.0.json (symlink)
│       ├── known-good -> v1.1.0.json (symlink, user-defined)
│       └── current -> v1.2.0.json (symlink, last fetched)
├── modules/
│   ├── <sha256>.wasm
│   └── manifest.json
├── specs/
│   └── <sha256>.json
└── fingerprints/
    └── <host>.json
```

**HTTP Caching:**
- Respects `Cache-Control`, `ETag`, `Last-Modified`
- Background refresh (check for updates without blocking)
- CDN-compatible (CloudFront, etc.)

## Permission & Capability System

### Capability Model

```typescript
interface Capabilities {
  // HTTP access
  http: boolean;
  http_hosts: string[];        // Wildcards allowed: "*.example.com"
  http_methods: string[];      // ["GET", "POST"]
  
  // Filesystem access
  filesystem: boolean;
  fs_read_paths: string[];     // ["/data/allowed/*"]
  fs_write_paths: string[];
  
  // Network access (direct, not via HTTP client)
  network: boolean;
  network_hosts: string[];
  network_ports: number[];
  
  // Environment variables
  env_vars: string[];          // Whitelist
  
  // Resource limits
  max_memory: string;          // "128MB"
  max_cpu_time: string;        // "30s"
  max_execution_time: string;  // "60s"
}
```

### Permission Sources (Priority Order)

1. **Server-Required Minimum** (inventory defines)
   ```json
   {
     "required_capabilities": {
       "http": true,
       "filesystem": true
     }
   }
   ```

2. **User-Granted** (CLI flags)
   ```bash
   openapi-cli --allow-filesystem --allow-path=/tmp/data
   ```

3. **Interactive Prompt**
   ```
   Module "data-processor" requests filesystem access.
   Allow? [y/N/details]
   ```

**Security Rule**: Most restrictive wins (intersection of all sources)

## Authentication System

### Supported Methods

```typescript
type AuthMethod = 
  | { type: 'bearer'; token: string }
  | { type: 'api_key'; key: string; location: 'header' | 'query'; name: string }
  | { type: 'basic'; username: string; password: string }
  | { type: 'oauth2'; 
      flow: 'client_credentials' | 'authorization_code' | 'device_code';
      client_id: string;
      client_secret?: string;
      token_url: string;
      scopes: string[];
    }
  | { type: 'jwt'; 
      token: string;
      refresh_token?: string;
      refresh_url?: string;
    }
  | { type: 'mtls'; 
      cert_file: string;
      key_file: string;
      ca_file?: string;
    }
  | { type: 'hmac'; 
      key_id: string;
      algorithm: 'sha256' | 'sha512';
      headers: string[];
    };
```

### Secret Storage Hierarchy

1. **OS Keychain** (preferred)
   - macOS: Keychain
   - Windows: Credential Manager
   - Linux: Secret Service (DBus)

2. **Encrypted File**
   - AES-256-GCM encrypted
   - Key from inventory or key server
   - Stored at `~/.config/openapi-cli/secrets/`

3. **Environment Variable** (discouraged for prod)
   - `OPENAPICLI_API_TOKEN`
   - `OPENAPICLI_CLIENT_SECRET`

4. **CLI Flag** (dev only, never logged)
   - `--token`, `--api-key`

## MCP Server Modes

### Mode 1: stdio (Default)

```typescript
// src/mcp/stdio.ts
export async function runStdioServer(config: Config) {
  const server = new Server({
    name: 'openapi-cli-mcp',
    version: config.inventory.version
  });
  
  server.setRequestHandler(ListToolsRequestSchema, async () => {
    return { tools: generateToolsFromOpenAPI(config.inventory.openapi) };
  });
  
  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    return executeTool(request.params.name, request.params.arguments);
  });
  
  const transport = new StdioServerTransport();
  await server.connect(transport);
}
```

**Use case**: Claude Desktop, Cursor, direct AI integration

### Mode 2: HTTP Server

```bash
# Start HTTP MCP server
openapi-cli --mcp-server --port 8080 --host https://api.example.com

# Connect via HTTP
 curl http://localhost:8080/mcp/tools/list
 curl -X POST http://localhost:8080/mcp/tools/call \
   -d '{"name": "users_list", "arguments": {"limit": 10}}'
```

**Use case**: Multiple clients, shared state, single auth context

### Mode 3: Proxy Daemon (Enterprise)

```bash
# Start daemon with Unix socket
openapi-cli --daemon --socket /var/run/openapi-cli.sock --host https://api.example.com

# Multiple CLIs connect via socket
openapi-cli --via-socket /var/run/openapi-cli.sock users list
openapi-cli --via-socket /var/run/openapi-cli.sock posts create --title "Hello"

# Or use as MCP server
# MCP config connects to socket instead of spawning process
```

**Architecture:**
```
┌──────────┐    ┌──────────┐    ┌──────────┐
│  CLI 1   │    │  CLI 2   │    │  MCP     │
└────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │
     └───────────────┼───────────────┘
                     │ Unix Socket
                     ▼
            ┌─────────────────┐
            │  Proxy Daemon   │
            │                 │
            │ • One TLS conn  │◄─── Single connection
            │   to backend    │     to API server
            │ • All auth      │
            │ • WASM cache    │
            │ • Rate limiting │
            └─────────────────┘
```

**Security Benefits:**
- One TLS connection per user (auditable)
- Centralized credential management
- Shared WASM compilation cache
- Single point for rate limiting

## Bundled Server Architecture

### Components

```
server/
├── src/
│   ├── api/                    # HTTP API
│   │   ├── inventory.ts       # Inventory endpoints
│   │   ├── modules.ts         # Module registry
│   │   └── webhook.ts         # Webhook proxy
│   │
│   ├── db/                     # Database layer
│   │   ├── index.ts          # Multi-driver support
│   │   ├── sqlite.ts         # SQLite (default)
│   │   ├── mysql.ts          # MySQL driver
│   │   └── postgres.ts       # PostgreSQL driver
│   │
│   ├── storage/               # Object storage
│   │   ├── s3.ts             # AWS S3
│   │   ├── r2.ts             # Cloudflare R2
│   │   └── filesystem.ts     # Local files
│   │
│   └── auth/                  # Authentication
│       ├── oauth.ts          # OAuth 2.0 server
│       ├── jwt.ts            # JWT issuance
│       └── mtls.ts           # mTLS cert management
│
├── migrations/                # SQL migrations
├── static/                    # Static file templates
└── package.json
```

### Deployment Options

**Option 1: SQLite (Single binary)**
```bash
# Zero-config deployment
./openapi-server
# Uses SQLite database in ~/.local/share/openapi-server/
```

**Option 2: MySQL/PostgreSQL**
```bash
# Production deployment
DATABASE_URL="mysql://user:pass@host/db" ./openapi-server
```

**Option 3: S3/CloudFront (Static)**
```bash
# Generate static files
openapi-server generate-static --output ./static-site

# Upload to S3
aws s3 sync ./static-site s3://my-inventory-bucket

# Serve via CloudFront
# No server required!
```

**Option 4: Hybrid**
- Static inventory on S3/CloudFront
- Dynamic auth on server
- Module registry on CDN

## Build & Distribution

### Development Workflow

```bash
# Install dependencies
bun install

# Development mode
bun run dev -- --host https://api.example.com

# Run tests
bun test

# Build for production
bun run build

# Compile to single binary
bun build --compile --outfile openapi-cli src/cli.ts
```

### Distribution Channels

**1. GitHub Releases (Binaries)**
```yaml
# .github/workflows/release.yml
- Build for: macOS (x64, arm64), Linux (x64, arm64), Windows (x64)
- Upload to GitHub Releases
- Provide install script: curl -sSL https://install.openapi-cli.io | sh
```

**2. Bootstrap Installer (like mise/volta)**
```bash
# One-line installer
curl -fsSL https://install.openapi-cli.io | sh

# What it does:
# 1. Detect platform/arch
# 2. Download binary from GitHub releases
# 3. Verify checksum
# 4. Install to ~/.local/bin or /usr/local/bin
```

**Self-Update**
```bash
openapi-cli update          # Manual update
openapi-cli --auto-update   # Auto-update on launch (configurable)
```

**3. Homebrew**
```bash
brew install openapi-cli
```

**4. Docker**
```bash
docker run -it openapi-cli --host https://api.example.com
```

## Summary

This architecture provides:
- ✅ **Flexibility**: Multiple deployment modes, configurable permissions
- ✅ **Security**: Sandboxed execution, OS keychain, mTLS
- ✅ **Scalability**: SQLite → PostgreSQL, S3/CDN support
- ✅ **Developer Experience**: Go with fast compilation, single binary
- ✅ **Enterprise Ready**: Proxy daemon, audit logging, centralized auth

Next step: Implementation Phase 1 (Inventory + basic CLI)
