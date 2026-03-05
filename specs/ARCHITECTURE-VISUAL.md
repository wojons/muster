# Architecture Visual Guide

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The Architecture Visual Guide provides comprehensive visual documentation of the OpenAPI CLI system architecture, including component relationships, data flows, state transitions, and deployment patterns. This specification serves as the definitive visual reference for understanding the system design.

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           USER / AI AGENT                               │
└───────────────┬───────────────────────────────────────┬─────────────────┘
                │                                       │
                │ CLI                                   │ MCP
                │                                       │
                ▼                                       ▼
┌───────────────────────────┐               ┌───────────────────────────┐
│     CLI Interface         │               │     MCP Interface         │
│  ┌─────────────────────┐  │               │  ┌─────────────────────┐  │
│  │ Argument Parser     │  │               │  │ MCP Protocol        │  │
│  │ Command Router      │  │               │  │ Tool Registry       │  │
│  │ Output Formatter    │  │               │  │ Request Handler     │  │
│  └─────────────────────┘  │               │  └─────────────────────┘  │
└───────────┬───────────────┘               └───────────┬───────────────┘
            │                                           │
            └───────────────────┬───────────────────────┘
                                │
                    ┌───────────▼───────────┐
                    │     SHARED CORE       │
                    └───────────┬───────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Inventory   │     │  Configuration  │     │    Security     │
│    Manager    │     │      Hub        │     │     Layer       │
├───────────────┤     ├─────────────────┤     ├─────────────────┤
│ • Fetch HTTPS │     │ • CLI flags     │     │ • Auth handlers │
│ • Cache       │     │ • Env vars      │     │ • mTLS          │
│ • Versions    │     │ • Config files  │     │ • Fingerprint   │
│ • Offline     │     │ • Inventory     │     │ • Secrets       │
└───────┬───────┘     └────────┬────────┘     └────────┬────────┘
        │                      │                       │
        └──────────────────────┼───────────────────────┘
                               │
                   ┌───────────▼───────────┐
                   │   Execution Engine    │
                   └───────────┬───────────┘
                               │
           ┌───────────────────┼───────────────────┐
           │                   │                   │
           ▼                   ▼                   ▼
    ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
    │ HTTP Client  │   │ WASM Runtime │   │ DSL Interpreter│
    ├──────────────┤   ├──────────────┤   ├──────────────┤
    │ • Requests   │   │ • Sandbox    │   │ • Workflows  │
    │ • Retries    │   │ • Modules    │   │ • Chaining   │
    │ • Rate limit │   │ • Transform  │   │ • Loops      │
    └────────┬─────┘   └──────────────┘   └──────────────┘
             │
             ▼
    ┌─────────────────┐
    │   Target API    │
    └─────────────────┘
```

## Startup Flow

```
User runs: openapi-cli --host https://api.example.com users list

    │
    ▼
┌────────────────────────────────────────────┐
│ 1. Parse CLI Arguments                     │
│    --host, --cert, --header, etc.          │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 2. Load Configuration                      │
│    - Environment variables                 │
│    - Config file (~/.config/...)           │
│    - Merge with CLI args                   │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 3. Fetch Inventory                         │
│    GET https://api.example.com/_inventory  │
│    - Check cache first (ETag)              │
│    - Download if needed                    │
│    - Validate JSON schema                  │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 4. Check Version Compatibility             │
│    - min_cli_version                       │
│    - max_cli_version                       │
│    - Warn/Error if incompatible            │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 5. Load Auth & Secrets                     │
│    - OS keychain                           │
│    - Encrypted files                       │
│    - Environment (if allowed)              │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 6. Generate Commands/Tools                 │
│    - Parse OpenAPI spec                    │
│    - Create command tree (CLI)             │
│    - Register tools (MCP)                  │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 7. Execute Command                         │
│    - Route to handler                      │
│    - Build request                         │
│    - Apply middleware                      │
│    - Send HTTP request                     │
│    - Format output                         │
└──────────────┬─────────────────────────────┘
               │
               ▼
        Display Result
```

## Request Execution Flow

```
Command: users list --limit 10

    │
    ▼
┌────────────────────────────────────────────┐
│ 1. Match Command to Operation              │
│    "users list" → GET /users               │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 2. Validate Arguments                      │
│    --limit 10: number, valid               │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 3. Build Request                           │
│    URL: https://api.example.com/users      │
│    Query: ?limit=10                        │
│    Headers: Authorization, X-Client-Id     │
└──────────────┬─────────────────────────────┘
               │
               ▼
┌────────────────────────────────────────────┐
│ 4. Apply Middleware Chain                  │
│                                            │
│    ┌───────────────────────────────────┐   │
│    │ a. Add Fingerprint                │   │
│    │    X-Client-Fingerprint: abc123   │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ b. Inject Auth                    │   │
│    │    Authorization: Bearer token    │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ c. Check Rate Limit               │   │
│    │    Token bucket: allow/deny       │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ d. Pre-process (WASM)             │   │
│    │    Transform request if needed    │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ e. Sign Request (Crypto)          │   │
│    │    Add HMAC/ECDSA signature       │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ f. Send HTTP Request              │   │
│    │    (with mTLS if configured)      │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ g. Handle Response                │   │
│    │    - Retry on 5xx                 │   │
│    │    - Circuit breaker              │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ h. Post-process (WASM)            │   │
│    │    Transform response if needed   │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ i. Validate Schema                │   │
│    │    Check against OpenAPI          │   │
│    └──────────────┬────────────────────┘   │
│                   │                        │
│    ┌──────────────▼────────────────────┐   │
│    │ j. Format Output                  │   │
│    │    JSON/Table/YAML                │   │
│    └───────────────────────────────────┘   │
└──────────────┬─────────────────────────────┘
               │
               ▼
        Display Result
```

## Cache Structure

```
~/.cache/oa/
│
├── inventory/
│   └── <sha256-of-url>/
│       ├── v1.0.0.json           # Version 1.0.0
│       ├── v1.1.0.json           # Version 1.1.0
│       ├── v1.2.0.json           # Version 1.2.0
│       ├── current.json          # Symlink to latest
│       └── known-good.json       # Symlink to user-marked
│
├── modules/
│   ├── abc123def456.wasm         # WASM module (by hash)
│   ├── def789abc012.js           # JS module (by hash)
│   └── manifest.json             # Module metadata
│
├── specs/
│   └── <sha256-of-url>.json      # Cached OpenAPI specs
│
├── fingerprints/
│   └── <host>.json               # Client fingerprints
│
└── responses/                    # Optional response cache
    └── <request-hash>.json

~/.config/oa/
│
├── config.yaml                   # Main config
├── profiles/
│   ├── prod.yaml                 # Production profile
│   └── staging.yaml              # Staging profile
└── aliases/
    └── <alias-name>.json         # Alias definitions
```

## Component Dependencies

```
                    ┌──────────────────┐
                    │   CLI / MCP      │
                    └────────┬─────────┘
                             │
               ┌──────────────┼──────────────┐
               │              │              │
               ▼              ▼              ▼
     ┌──────────────────────────────────────────────────┐
     │              CORE MODULES                        │
     │  ┌───────────┐  ┌───────────┐  ┌───────────┐   │
     │  │ Inventory │  │  Config   │  │  Security │   │
     │  │  Manager  │  │    Hub    │  │   Layer   │   │
     │  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘   │
     │        │              │              │         │
     │        └──────────────┼──────────────┘         │
     │                       │                        │
     │              ┌────────▼────────┐               │
     │              │Execution Engine │               │
     │              └────────┬────────┘               │
     │                       │                        │
     │        ┌──────────────┼──────────────┐        │
     │        │              │              │        │
     │   ┌────▼────┐   ┌────▼────┐   ┌────▼────┐   │
     │   │  HTTP   │   │  WASM   │   │   DSL   │   │
     │   │ Client  │   │ Runtime │   │Interp.  │   │
     │   └────┬────┘   └─────────┘   └─────────┘   │
     │        │                                      │
     └────────┼──────────────────────────────────────┘
              │
              ▼
     ┌──────────────────┐
     │   External APIs  │
     │   (HTTPS only)   │
     └──────────────────┘
```

## Data Flow Summary

```
┌──────────────┐
│  User Input  │
└──────┬───────┘
       │
       │ CLI flags / MCP init
       ▼
┌──────────────┐
│    Config    │
│   (merged)   │
└──────┬───────┘
       │
       │ Fetch inventory
       ▼
┌──────────────┐
│  Inventory   │
│   (cached)   │
└──────┬───────┘
       │
       │ Parse OpenAPI
       ▼
┌──────────────┐
│  Operations  │
│  (commands)  │
└──────┬───────┘
       │
       │ Execute with middleware
       ▼
┌──────────────┐
│    Result    │
└──────────────┘
```

## State Machine: Command Execution

```
                         ┌──────────────┐
                         │    IDLE      │
                         └──────┬───────┘
                                │ Parse
                                ▼
                         ┌──────────────┐
                         │   PARSED     │
                         └──────┬───────┘
                                │ Validate
                                ▼
                   ┌────────────────────────┐
                   │      VALIDATED         │
                   └───────────┬────────────┘
                               │ Load Config
                               ▼
                    ┌──────────────────────┐
                    │   CONFIG LOADED      │
                    └──────────┬───────────┘
                               │ Fetch Inventory
                               ▼
                    ┌──────────────────────┐
                    │  INVENTORY READY     │
                    └──────────┬───────────┘
                               │ Check Versions
                               ▼
                    ┌──────────────────────┐
                    │  VERSION CHECKED     │
                    └──────────┬───────────┘
                               │ Load Auth
                               ▼
                    ┌──────────────────────┐
                    │   AUTHENTICATED      │
                    └──────────┬───────────┘
                               │ Build Request
                               ▼
                    ┌──────────────────────┐
                    │  REQUEST BUILT       │
                    └──────────┬───────────┘
                               │ Apply Middleware
                               ▼
                    ┌──────────────────────┐
        ┌──────────│  REQUEST PROCESSED   │
        │          └──────────┬───────────┘
   Failure│                   │ Send Request
        │          ┌──────────▼───────────┐
        └─────────►│   EXECUTING          │
                   └──────────┬───────────┘
                              │ Receive Response
                              ▼
                   ┌──────────────────────┐
                   │   RESPONSE RECEIVED  │
                   └──────────┬───────────┘
                              │ Process Response
                              ▼
                   ┌──────────────────────┐
                   │ RESPONSE PROCESSED   │
                   └──────────┬───────────┘
                              │ Format Output
                              ▼
                   ┌──────────────────────┐
                   │     COMPLETE         │
                   └──────────────────────┘
```

## Security Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    SECURITY LAYERS                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Layer 5: Application                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • Command validation  • Rate limiting              │   │
│  │ • Input sanitization  • Output encoding            │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│  Layer 4: Transport                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • TLS 1.3             • Certificate pinning          │   │
│  │ • mTLS                • Cipher suite enforcement     │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│  Layer 3: Authentication                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • OAuth 2.0           • API keys                     │   │
│  │ • JWT validation      • Basic auth                   │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│  Layer 2: Authorization                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • Role-based access   • Scope validation             │   │
│  │ • Permission checks   • Resource ownership           │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│  Layer 1: Secrets                                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • OS keychain         • Encrypted storage            │   │
│  │ • Environment vars    • Secure memory                │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Module Loading Flow

```
┌─────────────────────────────────────────────────────────┐
│              MODULE LOADING PROCESS                     │
└─────────────────────────────────────────────────────────┘

1. DISCOVERY
   Inventory references module by URL + SHA256
   │
   ▼
2. CACHE CHECK
   Check ~/.cache/oa/modules/<sha256>.wasm
   │
   ├── Cache Hit ──► 8. LOAD
   │
   └── Cache Miss ──► 3. DOWNLOAD

3. DOWNLOAD
   Fetch from registry/repository
   │
   ▼
4. VERIFY
   Calculate SHA256, verify against inventory
   │
   ├── Match ──► 5. STORE
   │
   └── Mismatch ──► ERROR

5. STORE
   Save to cache directory
   │
   ▼
6. INSTANTIATE
   Create WASM runtime instance
   │
   ▼
7. INITIALIZE
   Call module _init() function
   │
   ▼
8. LOAD
   Module ready for execution
```

## Deployment Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    DEPLOYMENT PATTERNS                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Pattern 1: Local Binary                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  User Machine                                       │   │
│  │  ┌─────────────────┐    ┌─────────────────┐        │   │
│  │  │  oa-cli binary  │───▶│   Target API    │        │   │
│  │  └─────────────────┘    └─────────────────┘        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Pattern 2: Container                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Docker Container                                   │   │
│  │  ┌─────────────────┐    ┌─────────────────┐        │   │
│  │  │  oa-cli         │───▶│   Target API    │        │   │
│  │  │  + config       │    └─────────────────┘        │   │
│  │  └─────────────────┘                               │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Pattern 3: CI/CD Integration                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  CI/CD Pipeline                                     │   │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────────────┐   │   │
│  │  │  Build  │──▶│  Test   │──▶│  Deploy (oa-cli)│   │   │
│  │  └─────────┘   └─────────┘   └─────────────────┘   │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Pattern 4: MCP Server                                      │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  MCP Host (Claude, etc.)                            │   │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────────────┐   │   │
│  │  │  Agent  │──▶│  oa-mcp │───▶│   Target API    │   │   │
│  │  └─────────┘   └─────────┘   └─────────────────┘   │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Key Design Decisions

1. **Dual Interface**: CLI and MCP share 90%+ code through the Shared Core
2. **Inventory-First**: Everything starts with inventory.json - no hardcoded API knowledge
3. **Cache Everything**: System works offline with cached inventories and specs
4. **Security Built-In**: mTLS, keychain integration, and fingerprinting by default
5. **Extensible**: WASM modules for custom request/response transformation
6. **Zero Config**: Sensible defaults minimize initial setup requirements

## Integration Points

```
┌────────────────────────────────────────────────────────────┐
│                      EXTERNAL SYSTEMS                      │
├──────────────┬──────────────┬──────────────┬───────────────┤
│   Inventory  │   Module     │    Auth      │     Key       │
│   Server     │   Registry   │   Servers    │    Server     │
├──────────────┼──────────────┼──────────────┼───────────────┤
│ HTTPS only   │ HTTPS + sig  │ OAuth/JWT    │ HTTPS         │
│ ETag support │ CDN ready    │ mTLS         │ cert-based    │
│ Dynamic gen  │ Versioned    │ Rotatable    │ HSM support   │
│ JSON format  │ WASM/JS      │ OIDC/OAuth2  │ HashiCorp     │
└──────────────┴──────────────┴──────────────┴───────────────┘
```

## Performance Characteristics

| Component | Target Latency | Throughput | Notes |
|-----------|---------------|------------|-------|
| Inventory Fetch | < 500ms | N/A | With cache: < 10ms |
| Command Generation | < 100ms | N/A | Depends on spec size |
| Request Execution | < 100ms | 1000 RPS | Without network |
| WASM Module Load | < 50ms | N/A | From cache |
| Cache Lookup | < 5ms | 10000 RPS | Local filesystem |

## Error Handling Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    ERROR HANDLING                           │
└─────────────────────────────────────────────────────────────┘

Error Type           │  Recovery Action           │  User Impact
─────────────────────┼────────────────────────────┼────────────────
Network Timeout      │  Retry with backoff        │  Warning message
HTTP 4xx             │  Display error details     │  Full error info
HTTP 5xx             │  Retry + circuit breaker   │  Retry attempt info
Validation Failed    │  Show validation errors    │  Correct input help
Auth Failed          │  Re-prompt for credentials │  Auth error details
Version Mismatch     │  Suggest upgrade/downgrade │  Compatibility info
Cache Miss           │  Fetch from remote         │  (transparent)
WASM Error           │  Skip middleware + warn    │  Warning message
```

---

## Bible Standard Compliance

**Genesis 1:1** - *"In the beginning God created the heavens and the earth."*

This specification follows the Bible Standard for software documentation, ensuring clarity, completeness, and divine order in technical communication. All architectural diagrams and system designs shall be documented with precision, honoring the Creator who established order from chaos.

**Compliance Checklist:**
- [x] Status clearly marked as COMPLETE
- [x] Version number specified (1.0.0)
- [x] Last updated date included
- [x] Overview provides clear context
- [x] System architecture diagrams
- [x] Data flow documentation
- [x] State machine diagrams
- [x] Security architecture
- [x] Deployment patterns
- [x] Performance characteristics
- [x] Bible Standard footer included

*"Whatever you do, work at it with all your heart, as working for the Lord."* - Colossians 3:23
