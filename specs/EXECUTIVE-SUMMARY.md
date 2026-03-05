# Executive Summary: Dynamic API Tool

## The Product

Two tools sharing one architecture:
1. **CLI** - `openapi-cli --host https://api.example.com`
2. **MCP Server** - `npx openapi-mcp --host https://api.example.com`

Both configure themselves dynamically from a remote inventory.

## Core Innovation

Instead of installing SDKs or MCP packages that go stale:
- Tool downloads config from `https://host/_inventory.json`
- Caches it locally (works offline)
- Generates interface from OpenAPI spec
- Downloads WASM modules for custom processing
- Always up-to-date, zero maintenance

## Key Features

### Startup
```bash
# CLI
openapi-cli --host https://api.example.com \
  --cert ./client.crt --key ./client.key \
  --header "Authorization: Bearer token"

# MCP (in claude_desktop_config.json)
{
  "mcpServers": {
    "my-api": {
      "command": "npx",
      "args": ["openapi-mcp", "--host", "https://api.example.com"],
      "env": {
        "OPENAPICLI_CERT_FILE": "./client.crt"
      }
    }
  }
}
```

### Inventory System
- **URL**: `https://<host>/_inventory.json` (configurable)
- **HTTPS mandatory** (TLS 1.2+)
- **Dynamic**: Different payloads per user/role
- **Cached**: Works offline, respects HTTP cache headers
- **Versioned**: Multiple versions stored, "last known good" support

### Configuration Sources (priority order)
1. CLI flags / MCP init params
2. Environment variables (`OPENAPICLI_*`)
3. Config file (`~/.config/openapi-cli/config.yaml`)
4. Inventory-provided defaults
5. Built-in defaults

### Security Features
- **mTLS**: Client certificates for auth
- **Multiple auth contexts**: Different creds for inventory vs API
- **OS keychain**: Secrets stored securely (not env vars)
- **Encrypted files**: With keys from inventory/key server
- **Client fingerprinting**: Optional, configurable, backend-controlled
- **SOCKS5 proxy**: Built-in support

### Advanced Processing
- **WASM modules**: Sandboxed execution for data processing
- **DSL**: Simple workflow language for multi-step operations
  ```yaml
  steps:
    - call: GET /users/{id}
    - for_each: $.posts
      steps:
        - call: GET /posts/{id}/comments
  ```

### Client-Side Controls
- **Rate limiting**: Per-instance and global limits
- **Retry logic**: Exponential backoff, circuit breaker
- **Request filtering**: Reduce payload size before sending

### Version Management
```bash
# Use specific version
openapi-cli --host https://api.example.com --version 1.2.3

# Use latest (default), fallback to cached if down
openapi-cli --host https://api.example.com --use-latest

# Use last known good version
openapi-cli --host https://api.example.com --use-known-good

# Mark current as known good
openapi-cli mark-good

# Create alias
openapi-cli alias create prod https://api.example.com --version 1.2.3

# Use alias
openapi-cli use prod
```

## Inventory JSON Structure

```json
{
  "version": "1.2.3",
  "min_cli_version": "0.5.0",
  "max_cli_version": "1.x",
  
  "openapi": {
    "source": "url",
    "url": "https://api.example.com/openapi.json",
    "sha256": "abc123..."
  },
  
  "modules": {
    "transform": {
      "url": "https://cdn.example.com/modules/transform.wasm",
      "sha256": "def456...",
      "signature": "ghi789..."
    }
  },
  
  "config": {
    "rate_limiting": {
      "requests_per_minute": 100
    },
    "retry": {
      "max_attempts": 3,
      "backoff": "exponential"
    },
    "fingerprinting": {
      "enabled": true,
      "regenerate_on_ip_change": true
    }
  },
  
  "auth": {
    "inventory": {
      "type": "bearer",
      "token_env": "INV_TOKEN"
    },
    "api": {
      "type": "oauth2",
      "flow": "client_credentials"
    }
  },
  
  "crypto": {
    "signing_key_id": "key-2024-01",
    "algorithm": "ES256"
  }
}
```

## Use Cases

### 1. Developer CLI Usage
```bash
# One-time setup
openapi-cli --host https://api.example.com --cert ./cert.pem

# Use generated commands
openapi-cli users list --limit 10
openapi-cli users get --id 123
openapi-cli posts create --title "Hello" --body "World"
```

### 2. AI Assistant via MCP
AI assistant automatically gets tools:
- `users_list` - List all users
- `users_get` - Get user by ID
- `posts_create` - Create a post

No manual configuration needed.

### 3. Enterprise Deployment
Company ships pre-configured package:
```bash
# Internal tool with pinned version
acme-cli --version 2.1.0

# Uses company's internal inventory
# Handles mTLS automatically
# Secrets from OS keychain
```

### 4. CI/CD Pipeline
```yaml
- name: Deploy
  run: |
    openapi-cli --host ${{ vars.API_HOST }} \
      --use-known-good \
      deployments create \
      --ref ${{ github.sha }}
```

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐
│   CLI Entry     │     │  MCP Server     │
│  (src/cli/)     │     │  (src/mcp/)     │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     │
         ┌───────────▼───────────┐
         │     SHARED CORE       │
         │  ┌─────────────────┐  │
         │  │ Inventory Mgr   │  │ ◄── Fetches & caches
         │  └─────────────────┘  │
         │  ┌─────────────────┐  │
         │  │ Config Hub      │  │ ◄── Merges all sources
         │  └─────────────────┘  │
         │  ┌─────────────────┐  │
         │  │ Security Layer  │  │ ◄── Auth, crypto, mTLS
         │  └─────────────────┘  │
         │  ┌─────────────────┐  │
         │  │ Execution Eng   │  │ ◄── HTTP, WASM, DSL
         │  └─────────────────┘  │
         └───────────────────────┘
```

## Technology Stack

- **Language**: Go (golang)
- **Compilation**: Single static binary (`go build`)
- **HTTP Client**: resty/v2
- **WASM Runtime**: Wasmtime (wasmtime-go bindings)
- **DSL**: Starlark (go.starlark.net)
- **Secrets**: 99designs/keyring (OS keychain)
- **MCP**: mcp-go-sdk
- **CLI**: Cobra
- **Database**: sqlc + golang-migrate (SQLite → MySQL → PostgreSQL)

## Security Model

1. **WASM Sandboxing**: Modules can't access filesystem/network unless explicitly allowed
2. **Code Signing**: All WASM modules signed with ECDSA
3. **mTLS**: Mutual authentication for sensitive environments
4. **Secret Isolation**: OS keychain or encrypted files, never in env vars
5. **Client Fingerprinting**: Optional, backend controls requirements
6. **HTTPS Only**: Inventory must be served over TLS

## Development Phases

### Phase 1: Foundation (Weeks 1-4)
- Inventory fetching and caching
- Basic CLI and MCP interfaces
- HTTP request execution
- Simple auth (Bearer, API Key)

### Phase 2: Security (Weeks 5-8)
- mTLS support
- OS keychain integration
- OAuth/JWT flows
- Client fingerprinting

### Phase 3: Advanced (Weeks 9-12)
- WASM module system
- DSL for workflows
- Rate limiting and retries
- SOCKS5 proxy

### Phase 4: Polish (Weeks 13-14)
- Performance optimization
- Documentation
- Distribution (npm, binaries, Docker)

## Success Criteria

- ✅ Works offline with cached inventory
- ✅ Zero-config for basic usage
- ✅ Both CLI and MCP from same codebase
- ✅ Secure secret handling
- ✅ Enterprise-ready (mTLS, auth, fingerprinting)

---

**Status**: Ready for implementation  
**Priority**: High  
**Target**: Production-ready in 14 weeks
