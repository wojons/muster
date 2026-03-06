# Architecture Visual Guide

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The Architecture Visual Guide provides comprehensive visual documentation of the OpenAPI CLI system architecture, including component relationships, data flows, state transitions, and deployment patterns. This specification serves as the definitive visual reference for understanding the system design.

## System Overview

```mermaid
graph TB
    User["USER / AI AGENT"] --> CLI1["CLI Interface"]
    User --> MCP1["MCP Interface"]

    subgraph "CLI Interface"
        CLI_ArgParse["Argument Parser"]
        CLI_CmdRouter["Command Router"]
        CLI_OutputFmt["Output Formatter"]
    end

    subgraph "MCP Interface"
        MCP_Protocol["MCP Protocol"]
        MCP_Tools["Tool Registry"]
        MCP_Handler["Request Handler"]
    end

    subgraph "Shared Core"
        InventoryMgr["Inventory Manager<br/>• Fetch HTTPS<br/>• Cache<br/>• Versions<br/>• Offline"]
        ConfigHub["Configuration Hub<br/>• CLI flags<br/>• Env vars<br/>• Config files<br/>• Inventory"]
        SecurityLayer["Security Layer<br/>• Auth handlers<br/>• mTLS<br/>• Fingerprint<br/>• Secrets"]
    end

    subgraph "Execution Engine"
        HTTPClient["HTTP Client<br/>• Requests<br/>• Retries<br/>• Rate limit"]
        WASMRuntime["WASM Runtime<br/>• Sandbox<br/>• Modules<br/>• Transform"]
        DSLInterpreter["DSL Interpreter<br/>• Workflows<br/>• Chaining<br/>• Loops"]
    end

    TargetAPI["Target API"]

    User -->|CLI| CLI1
    User -->|MCP| MCP1
    CLI1 -->|Commands| InventoryMgr
    MCP1 -->|Tools| InventoryMgr

    InventoryMgr -->|Config| ConfigHub
    InventoryMgr -->|Auth| SecurityLayer

    SecurityLayer -->|Execute| HTTPClient
    SecurityLayer -->|Execute| WASMRuntime
    SecurityLayer -->|Execute| DSLInterpreter

    HTTPClient -->|Requests| TargetAPI
```

## Startup Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Config
    participant Inventory
    participant Auth
    participant Generator
    participant Executor

    User->>CLI: Run command<br/>openapi-cli --host https://api.example.com users list
    CLI->>CLI: 1. Parse CLI Arguments<br/>--host, --cert, --header, etc.

    CLI->>Config: 2. Load Configuration
    Config-->>CLI: • Environment variables<br/>• Config file (~/.config/...)<br/>• Merge with CLI args

    CLI->>Inventory: 3. Fetch Inventory<br/>GET https://api.example.com/_inventory
    alt Cache Hit (ETag match)
        Inventory-->>CLI: Return cached inventory
    else Cache Miss
        Inventory->>Inventory: • Check cache first (ETag)<br/>• Download if needed<br/>• Validate JSON schema
        Inventory-->>CLI: Return fetched inventory
    end

    CLI->>Inventory: 4. Check Version Compatibility<br/>• min_cli_version<br/>• max_cli_version
    alt Incompatible
        Inventory-->>CLI: Warn/Error if incompatible
    end

    CLI->>Auth: 5. Load Auth & Secrets<br/>• OS keychain<br/>• Encrypted files<br/>• Environment (if allowed)
    Auth-->>CLI: Load credentials

    CLI->>Generator: 6. Generate Commands/Tools<br/>• Parse OpenAPI spec<br/>• Create command tree (CLI)<br/>• Register tools (MCP)
    Generator-->>CLI: Command tree ready

    CLI->>Executor: 7. Execute Command<br/>• Route to handler<br/>• Build request<br/>• Apply middleware<br/>• Send HTTP request<br/>• Format output
    Executor-->>User: Display Result
```

## Request Execution Flow

```mermaid
sequenceDiagram
    participant User
    participant Router as Command Router
    participant Builder as Request Builder
    participant Middleware as Middleware Chain
    participant WASM as WASM Runtime
    participant API as Target API

    User->>Router: users list --limit 10
    Router->>Router: 1. Match Command to Operation<br/>"users list" → GET /users

    Router->>Builder: 2. Validate Arguments<br/>--limit 10: number, valid

    Builder->>Builder: 3. Build Request<br/>URL: https://api.example.com/users<br/>Query: ?limit=10<br/>Headers: Authorization, X-Client-Id

    Builder->>Middleware: 4. Apply Middleware Chain

    Middleware->>Middleware: a. Add Fingerprint<br/>X-Client-Fingerprint: abc123

    Middleware->>Middleware: b. Inject Auth<br/>Authorization: Bearer token

    Middleware->>Middleware: c. Check Rate Limit<br/>Token bucket: allow/deny

    alt Pre-process needed
        Middleware->>WASM: d. Pre-process (WASM)<br/>Transform request if needed
        WASM-->>Middleware: Transformed request
    end

    Middleware->>Middleware: e. Sign Request (Crypto)<br/>Add HMAC/ECDSA signature

    Middleware->>API: f. Send HTTP Request<br/>(with mTLS if configured)

    API-->>Middleware: HTTP Response<br/>• Retry on 5xx<br/>• Circuit breaker

    alt Post-process needed
        Middleware->>WASM: h. Post-process (WASM)<br/>Transform response if needed
        WASM-->>Middleware: Transformed response
    end

    Middleware->>Middleware: i. Validate Schema<br/>Check against OpenAPI

    Middleware->>Middleware: j. Format Output<br/>JSON/Table/YAML

    Middleware-->>User: Display Result
```

## Cache Structure

```mermaid
graph TB
    subgraph "Cache Directory (~/.cache/oa/)"
        Cache["~/.cache/oa/"]
        InventoryDir["inventory/"]
        ModulesDir["modules/"]
        SpecsDir["specs/"]
        FingerprintsDir["fingerprints/"]
        ResponsesDir["responses/"]

        InventorySHA["<sha256-of-url>/"]
        InventoryVersions["v1.0.0.json"]
        InventoryV11["v1.1.0.json"]
        InventoryV12["v1.2.0.json"]
        InventoryCurrent["current.json"]
        InventoryKnown["known-good.json"]

        ModulesWASM["abc123def456.wasm"]
        ModulesJS["def789abc012.js"]
        ModulesManifest["manifest.json"]

        SpecsCache["<sha256-of-url>.json"]
        FingerprintsHost["<host>.json"]
        ResponsesHash["<request-hash>.json"]
    end

    subgraph "Config Directory (~/.config/oa/)"
        ConfigDir["~/.config/oa/"]
        ConfigFile["config.yaml"]
        ProfilesDir["profiles/"]
        AliasesDir["aliases/"]

        ProfileProd["prod.yaml"]
        ProfileStaging["staging.yaml"]
        AliasFile["<alias-name>.json"]
    end

    Cache --> InventoryDir
    Cache --> ModulesDir
    Cache --> SpecsDir
    Cache --> FingerprintsDir
    Cache --> ResponsesDir

    InventoryDir --> InventorySHA
    InventorySHA --> InventoryVersions
    InventorySHA --> InventoryV11
    InventorySHA --> InventoryV12
    InventorySHA --> InventoryCurrent
    InventorySHA --> InventoryKnown

    ModulesDir --> ModulesWASM
    ModulesDir --> ModulesJS
    ModulesDir --> ModulesManifest

    SpecsDir --> SpecsCache
    FingerprintsDir --> FingerprintsHost
    ResponsesDir --> ResponsesHash

    ConfigDir --> ConfigFile
    ConfigDir --> ProfilesDir
    ConfigDir --> AliasesDir

    ProfilesDir --> ProfileProd
    ProfilesDir --> ProfileStaging
    AliasesDir --> AliasFile
```

## Component Dependencies

```mermaid
graph TB
    CLI_MCP["CLI / MCP"]

    subgraph "Core Modules"
        InventoryMgr["Inventory Manager"]
        ConfigHub["Config Hub"]
        SecurityLayer["Security Layer"]
    end

    ExecutionEngine["Execution Engine"]
    HTTPClient["HTTP Client"]
    WASMRuntime["WASM Runtime"]
    DSLInterpreter["DSL Interpreter"]

    ExternalAPIs["External APIs<br/>(HTTPS only)"]

    CLI_MCP --> InventoryMgr
    CLI_MCP --> ConfigHub
    CLI_MCP --> SecurityLayer

    InventoryMgr --> ExecutionEngine
    ConfigHub --> ExecutionEngine
    SecurityLayer --> ExecutionEngine

    ExecutionEngine --> HTTPClient
    ExecutionEngine --> WASMRuntime
    ExecutionEngine --> DSLInterpreter

    HTTPClient --> ExternalAPIs
```

## Data Flow Summary

```mermaid
flowchart LR
    Start([User Input]) --> Config[Config<br/>merged]
    Config -->|Fetch inventory| Inventory[Inventory<br/>cached]
    Inventory -->|Parse OpenAPI| Operations[Operations<br/>commands]
    Operations -->|Execute with middleware| Result[Result]
```

## State Machine: Command Execution

```mermaid
stateDiagram-v2
    [*] --> IDLE

    IDLE --> PARSED: Parse
    PARSED --> VALIDATED: Validate
    VALIDATED --> CONFIG_LOADED: Load Config
    CONFIG_LOADED --> INVENTORY_READY: Fetch Inventory
    INVENTORY_READY --> AUTHENTICATED: Load Auth
    AUTHENTICATED --> REQUEST_BUILT: Build Request
    REQUEST_BUILT --> REQUEST_PROCESSED: Apply Middleware

    REQUEST_PROCESSED --> EXECUTING: Send Request
    REQUEST_PROCESSED -[right]-> FAILURE: Failure

    EXECUTING --> RESPONSE_RECEIVED: Receive Response
    RESPONSE_RECEIVED --> RESPONSE_PROCESSED: Process Response
    RESPONSE_PROCESSED --> COMPLETE: Format Output

    COMPLETE --> [*]
    FAILURE --> [*]
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

```mermaid
graph TB
    subgraph "Security Layers"
        Layer5["Layer 5: Application<br/>• Command validation<br/>• Rate limiting<br/>• Input sanitization<br/>• Output encoding"]

        Layer4["Layer 4: Transport<br/>• TLS 1.3<br/>• Certificate pinning<br/>• mTLS<br/>• Cipher suite enforcement"]

        Layer3["Layer 3: Authentication<br/>• OAuth 2.0<br/>• API keys<br/>• JWT validation<br/>• Basic auth"]

        Layer2["Layer 2: Authorization<br/>• Role-based access<br/>• Scope validation<br/>• Permission checks<br/>• Resource ownership"]

        Layer1["Layer 1: Secrets<br/>• OS keychain<br/>• Encrypted storage<br/>• Environment vars<br/>• Secure memory"]
    end

    ExternalAPI["External APIs"]

    Layer5 --> ExternalAPI
    Layer5 --> Layer4
    Layer4 --> Layer3
    Layer3 --> Layer2
    Layer2 --> Layer1
```

## Module Loading Flow

```mermaid
flowchart TD
    Start([Module Loading Process]) --> Discovery[1. Discovery<br/>Inventory references module by URL + SHA256]

    Discovery --> CacheCheck{2. Cache Check}

    CacheCheck -->|Cache Hit| Load[8. Load<br/>Module ready for execution]

    CacheCheck -->|Cache Miss| Download[3. Download<br/>Fetch from registry/repository]

    Download --> Verify{4. Verify<br/>Calculate SHA256<br/>verify against inventory}

    Verify -->|Match| Store[5. Store<br/>Save to cache directory]
    Verify -->|Mismatch| Error[ERROR]

    Store --> Instantiate[6. Instantiate<br/>Create WASM runtime instance]
    Instantiate --> Initialize[7. Initialize<br/>Call module _init function]
    Initialize --> Load

    Load --> End([End])
    Error --> End
```

## Deployment Architecture

```mermaid
graph TB
    subgraph "Pattern 1: Local Binary"
        UserMachine1["User Machine"]
        OACLI1["oa-cli binary"]
        TargetAPI1["Target API"]
        UserMachine1 --> OACLI1
        OACLI1 --> TargetAPI1
    end

    subgraph "Pattern 2: Container"
        DockerContainer{"Docker Container"}
        OACLI2["oa-cli<br/>+ config"]
        TargetAPI2["Target API"]
        DockerContainer --> OACLI2
        OACLI2 --> TargetAPI2
    end

    subgraph "Pattern 3: CI/CD Integration"
        CIPipeline["CI/CD Pipeline"]
        Build["Build"]
        Test["Test"]
        Deploy1["Deploy (oa-cli)"]
        CIPipeline --> Build
        Build --> Test
        Test --> Deploy1
    end

    subgraph "Pattern 4: MCP Server"
        MCPHost["MCP Host<br/>(Claude, etc.)"]
        Agent["Agent"]
        OAMCP["oa-mcp"]
        TargetAPI3["Target API"]
        MCPHost --> Agent
        Agent --> OAMCP
        OAMCP --> TargetAPI3
    end
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
