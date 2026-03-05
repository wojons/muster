# Product Requirements Document: Dynamic OpenAPI CLI & MCP Server

## 1. Overview

### 1.1 Product Vision
A unified CLI and MCP server tool that dynamically configures itself from a remote inventory, ensuring users always have the latest API interface, WASM modules, and configuration without manual updates or version drift.

### 1.2 Dual-Tool Architecture
The system provides **two interfaces** built on shared core functionality:
1. **CLI Tool** - For direct command-line usage and scripting
2. **MCP Server** - For AI assistant integration (Claude, Cursor, etc.)

Both tools configure themselves identically from the same inventory source.

### 1.3 Problem Statement
- **MCP Distribution Pain**: npm packages require updates users forget to run
- **Configuration Drift**: Multiple environments need different auth/certs/configs
- **Curl Complexity**: Raw HTTP forces models to manage retries, headers, auth flows
- **Version Compatibility**: API changes break existing tools
- **Security Concerns**: mTLS, client fingerprinting, encrypted secrets needed
- **Offline Reliability**: Need to work when services are down

### 1.4 Solution
A self-configuring tool that:
- Downloads inventory from configurable HTTPS endpoint
- Caches multiple versions with "last known good" support
- Supports WASM modules for advanced processing
- Includes sandboxed DSL for simple workflows
- Manages multiple auth methods (OAuth, JWT, mTLS, basic)
- Integrates with OS keychain for secrets
- Provides client-side rate limiting and retry logic
- Supports client fingerprinting and backend crypto controls

## 2. Target Users

### 2.1 Primary Users
- **AI Coding Assistants** via MCP protocol
- **Developers** using CLI for automation
- **DevOps/SRE** for CI/CD pipeline integration
- **Security-conscious teams** requiring mTLS/client certs

### 2.2 User Personas
1. **The AI Assistant**: Needs reliable, deterministic API calls via MCP
2. **The Developer**: Wants CLI tool that auto-updates from company inventory
3. **The Security Engineer**: Needs mTLS, client fingerprinting, encrypted secrets
4. **The Platform Team**: Distributes standardized tools across organization

## 3. Core Requirements

### 3.1 Dual Interface Requirements

#### D1: CLI Interface
- Standalone binary with subcommand structure
- Full terminal interaction support
- Shell completion generation
- Scriptable (exit codes, JSON output)
- Progress indicators and spinners

#### D2: MCP Server Interface
- Implements Model Context Protocol
- Exposes tools based on OpenAPI operations
- Handles MCP lifecycle (initialize, invoke, shutdown)
- Can be run via npx without installation
- Supports MCP stdio and HTTP transports

### 3.2 Inventory System (Critical)

#### INV1: Inventory Endpoint
- Configurable URL (default: `https://<host>/_inventory.json`)
- HTTPS mandatory (security requirement)
- Supports dynamic generation (different per user/role)
- Returns JSON with:
  - OpenAPI specification (embedded or URL)
  - WASM module URLs and checksums
  - Configuration templates
  - CLI version requirements
  - Auth requirements

#### INV2: Inventory Contents
```json
{
  "version": "1.2.3",
  "min_cli_version": "0.5.0",
  "max_cli_version": "1.x",
  "openapi": {
    "source": "embedded" | "url",
    "content": "..." | "https://...",
    "sha256": "..."
  },
  "modules": {
    "processors": [
      {"name": "data-transform", "url": "...", "sha256": "...", "signature": "..."}
    ]
  },
  "config": {
    "rate_limiting": {"requests_per_minute": 100},
    "retry_policy": {"max_attempts": 3, "backoff": "exponential"},
    "fingerprinting": {"enabled": true, "regenerate_on_ip_change": true}
  },
  "auth": {
    "inventory_auth": {"type": "bearer", "token_env": "INV_TOKEN"},
    "api_auth": {"type": "oauth2", "flow": "client_credentials"}
  }
}
```

#### INV3: Version Management
- Download and cache multiple inventory versions
- Support "last known good" marker
- User can pin to specific version
- Automatic fallback to cached if server down
- Aliases for different configurations (e.g., `prod`, `staging`)
- CLI can enforce version requirements from inventory

#### INV4: Caching Strategy
- HTTP caching headers support (ETag, Cache-Control, Last-Modified)
- CDN-compatible
- Configurable cache limits (size, age)
- Offline mode using cached data
- Background refresh (check for updates without blocking)

### 3.3 Configuration System

#### CFG1: Configuration Hierarchy (low to high priority)
1. Built-in defaults
2. Inventory-provided config
3. Config file (`~/.config/openapi-cli/config.yaml`)
4. Environment variables
5. CLI flags / MCP initialization params

#### CFG2: Configuration Sources
- **CLI Flags**: `--host`, `--cert`, `--key`, `--header`, `--inventory-url`
- **Environment Variables**: `OPENAPICLI_HOST`, `OPENAPICLI_CERT_FILE`, etc.
- **Config File**: YAML/JSON with profiles
- **Inventory**: Server-provided configuration

#### CFG3: Security Configuration
- mTLS support (cert and key files)
- Client certificate chains
- CA bundle configuration
- SOCKS5 proxy support
- Skip TLS verification (dev only, with warnings)

#### CFG4: Auth Methods
Multiple auth contexts supported simultaneously:
- **Inventory Auth**: To fetch the inventory itself
- **API Auth**: To call the actual API endpoints
- **Types Supported**:
  - Bearer tokens
  - API keys (header or query)
  - Basic auth
  - OAuth 2.0 (client credentials, auth code, device flow)
  - JWT (with refresh)
  - mTLS (client certificates)

### 3.4 WASM Module System

#### WASM1: Module Loading
- Download WASM modules from inventory URLs
- Verify integrity (SHA256 checksum)
- Verify authenticity (code signing with ECDSA)
- Cache modules locally
- Lazy loading (only when needed)

#### WASM2: Execution Environment
- Sandboxed WASM runtime (Wasmtime)
- No filesystem access (unless explicitly allowed)
- No network access (unless explicitly allowed)
- Memory limits enforced
- Execution timeout

#### WASM3: Use Cases
- Request/response transformation
- Complex data processing
- Custom authentication flows
- Request signing
- Data validation beyond JSON Schema

### 3.5 DSL for Workflows

#### DSL1: Language Design
- Simple, declarative syntax
- Sandboxed execution
- No arbitrary code execution
- Limited to: variables, conditionals, loops over arrays, HTTP calls

#### DSL2: Example Workflow
```yaml
workflow:
  name: "fetch-user-posts"
  steps:
    - call: GET /users/{userId}
      extract:
        userName: "$.name"
    
    - for_each: "$.posts"
      as: post
      steps:
        - call: GET /posts/{post.id}/comments
          extract:
            comments: "$"
    
    - return:
        user: "{userName}"
        all_comments: "{comments}"
```

#### DSL3: Security
- No file system access
- No environment variable access (except whitelisted)
- No network calls except to configured API
- Execution timeout (30 seconds default)
- Memory limits

### 3.6 Client-Side Controls

#### CTRL1: Rate Limiting
- Per-instance rate limiting
- Global rate limiting across all instances
- Configurable limits (requests per minute/hour)
- Token bucket algorithm
- Respects server-side rate limits (429 responses)

#### CTRL2: Retry Logic
- Configurable max attempts
- Exponential backoff with jitter
- Circuit breaker pattern
- Retry on specific status codes only
- Per-endpoint retry policies

#### CTRL3: Request Filtering
- Client-side filtering of responses
- Field selection (reduce payload size)
- Pagination handling
- Batch request optimization

### 3.7 Security Features

#### SEC1: Secret Management
- OS keychain integration (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- Encrypted file storage
- Keys fetched from key server or inventory
- Secrets never logged or exposed in errors
- Automatic secret rotation support

#### SEC2: Client Fingerprinting
- Generate unique client fingerprint
- Can be disabled via config
- Backend controls fingerprint requirements
- Regenerate on IP change (configurable)
- Include in request headers

#### SEC3: Crypto Operations
- Backend can provide crypto keys and algorithms
- Request signing with HMAC or ECDSA
- Challenge-response authentication
- Token refresh automation
- Certificate pinning

#### SEC4: Backend Controls
Inventory server can configure:
- Required fingerprinting rules
- Crypto key rotation schedules
- Auth token lifetimes
- Client-side rate limits
- Required client versions

### 3.8 Output & Logging

#### OUT1: Output Formats
- JSON (machine-readable, default)
- Table (human-readable)
- YAML
- Raw HTTP response
- jq-compatible filtering
- Silent mode (errors only)

#### OUT2: Logging
- Debug logging to stderr
- Request/response logging (with sensitive data redaction)
- Performance metrics
- Audit logging for security events

## 4. CLI Commands

### 4.1 Core Commands
```bash
# Initialize with inventory
openapi-cli --host https://api.example.com

# Use specific version
openapi-cli --host https://api.example.com --version 1.2.3

# Use alias
openapi-cli use prod  # Use previously configured 'prod' alias

# Mark current as known good
openapi-cli mark-good

# List cached versions
openapi-cli versions --cached

# Force refresh
openapi-cli refresh

# Show current inventory info
openapi-cli info
```

### 4.2 Dynamic Commands (from OpenAPI)
After initialization, CLI exposes API operations as commands:
```bash
openapi-cli users list --limit 10
openapi-cli users get --id 123
openapi-cli posts create --title "Hello" --body "World"
```

## 5. MCP Server Usage

### 5.1 Running via npx
```bash
npx openapi-mcp --host https://api.example.com
```

### 5.2 Configuration File
```json
{
  "mcpServers": {
    "my-api": {
      "command": "npx",
      "args": ["openapi-mcp", "--host", "https://api.example.com"],
      "env": {
        "OPENAPICLI_AUTH_TOKEN": "..."
      }
    }
  }
}
```

### 5.3 Tools Exposed
MCP server exposes each OpenAPI operation as a tool with:
- Name derived from operationId
- Parameters from OpenAPI schema
- Description from OpenAPI docs

## 6. Non-Functional Requirements

### 6.1 Performance
- CLI startup: < 1 second (cached), < 3 seconds (fresh download)
- MCP initialization: < 2 seconds
- Request overhead: < 50ms vs raw HTTP
- WASM load time: < 500ms

### 6.2 Reliability
- Works offline with cached inventory
- Graceful degradation when services unavailable
- Automatic retry with exponential backoff
- Circuit breaker for failing endpoints

### 6.3 Security
- All remote communication over HTTPS
- WASM sandboxed execution
- Secrets in OS keychain or encrypted files
- No arbitrary code execution
- Code signing verification

### 6.4 Compatibility
- Node.js 18+, standalone binaries
- macOS, Linux, Windows
- MCP stdio and HTTP transports
- SOCKS5 proxy support

## 7. Success Metrics

### 7.1 Adoption
- Downloads per week
- Active MCP connections
- Unique inventory endpoints accessed

### 7.2 Reliability
- Cache hit rate
- Offline functionality usage
- Retry success rate

### 7.3 Performance
- Average startup time
- Request latency vs baseline
- WASM execution time

## 8. Constraints & Assumptions

### 8.1 Constraints
- Inventory endpoint must use HTTPS
- WASM runtime required for advanced features
- Network access required for first run

### 8.2 Assumptions
- Target APIs follow REST/OpenAPI conventions
- Inventory server is maintained by API provider
- Users have basic CLI familiarity

## 9. Timeline

### Phase 1: Core (4-6 weeks)
- Inventory fetching and caching
- Basic CLI generation from OpenAPI
- HTTP request execution
- Simple auth (Bearer, API Key)

### Phase 2: Security (3-4 weeks)
- mTLS support
- OS keychain integration
- OAuth/JWT flows
- Client fingerprinting

### Phase 3: Advanced (4-6 weeks)
- WASM module system
- DSL for workflows
- Rate limiting and retry logic
- MCP server interface

### Phase 4: Polish (2-3 weeks)
- Performance optimization
- Documentation
- Distribution packaging

## 10. Open Questions

1. Should we support custom DSL syntax or use existing language (Lua, JavaScript subset)?
2. How do we handle WASM module versioning and compatibility?
3. What's the governance model for the public module registry?
4. Should we provide a hosted inventory service or focus on self-hosted?
5. How do we handle API versioning in the inventory?

---

**Document Status**: Draft v2  
**Last Updated**: 2026-02-28  
**Author**: Lexi
