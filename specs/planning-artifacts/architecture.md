# Architecture Document: Dynamic OpenAPI CLI & MCP Server

## 1. System Overview

### 1.1 Dual-Interface Architecture

The system provides **two entry points** sharing a common core:

```
┌─────────────────────────────────────────────────────────────────┐
│                      User/AI Agent                              │
└─────────────┬──────────────────────────────┬────────────────────┘
              │                              │
              │ CLI                          │ MCP Protocol
              ▼                              ▼
┌──────────────────────────┐  ┌──────────────────────────────────┐
│     CLI Entry Point      │  │     MCP Server Entry Point       │
│  (src/cli/index.ts)      │  │  (src/mcp/index.ts)              │
│                          │  │                                  │
│  • Argument parsing      │  │  • MCP lifecycle mgmt            │
│  • Command routing       │  │  • Tool registration             │
│  • Terminal output       │  │  • Request handling              │
└──────────┬───────────────┘  └──────────┬───────────────────────┘
           │                             │
           └──────────────┬──────────────┘
                          │
                          ▼
           ┌──────────────────────────────┐
           │      SHARED CORE             │
           │  ┌────────────────────────┐  │
           │  │    Inventory Manager   │  │
           │  │  • Fetch & cache       │  │
           │  │  • Version control     │  │
           │  │  • Offline fallback    │  │
           │  └────────────────────────┘  │
           │                              │
           │  ┌────────────────────────┐  │
           │  │   Configuration Hub    │  │
           │  │  • Merge configs       │  │
           │  │  • Secrets mgmt        │  │
           │  │  • Profile switching   │  │
           │  └────────────────────────┘  │
           │                              │
           │  ┌────────────────────────┐  │
           │  │   Execution Engine     │  │
           │  │  • HTTP client         │  │
           │  │  • WASM runtime        │  │
           │  │  • DSL interpreter     │  │
           │  └────────────────────────┘  │
           │                              │
           │  ┌────────────────────────┐  │
           │  │   Security Layer       │  │
           │  │  • Auth handlers       │  │
           │  │  • Crypto ops          │  │
           │  │  • Fingerprinting      │  │
           │  └────────────────────────┘  │
           └──────────────────────────────┘
                          │
           ┌──────────────┼──────────────┐
           ▼              ▼              ▼
    ┌──────────┐  ┌──────────┐  ┌──────────┐
    │ Inventory│  │   API    │  │  Module  │
    │  Server  │  │  Server  │  │ Registry │
    └──────────┘  └──────────┘  └──────────┘
```

### 1.2 Core Principles

1. **Single Source of Truth**: Inventory JSON defines everything
2. **Cache-First**: Work offline when possible
3. **Security-First**: mTLS, sandboxing, encrypted secrets
4. **Zero-Config**: Sensible defaults, auto-discovery
5. **Dual Interface**: Same core, different presentations

## 2. Component Architecture

### 2.1 Inventory Manager (`/src/core/inventory/`)

**Responsibility**: Fetch, cache, and manage inventory versions

```typescript
interface InventoryManager {
  // Fetch inventory from URL
  fetch(url: string, options: FetchOptions): Promise<Inventory>;
  
  // Get cached inventory (fallback if offline)
  getCached(alias?: string): Promise<Inventory | null>;
  
  // Mark version as "known good"
  markGood(version: string): void;
  
  // List available versions
  listVersions(): VersionInfo[];
  
  // Check for updates (background)
  checkForUpdates(): Promise<UpdateInfo | null>;
}

interface Inventory {
  version: string;
  min_cli_version: string;
  max_cli_version: string;
  openapi: OpenAPISource;
  modules: ModuleRegistry;
  config: SystemConfig;
  auth: AuthConfig;
  crypto?: CryptoConfig;
}
```

**Key Features**:
- HTTP caching with ETag/Last-Modified
- Multiple version storage
- Alias management (prod, staging, etc.)
- Background refresh
- Offline mode detection

### 2.2 Configuration Hub (`/src/core/config/`)

**Responsibility**: Merge configs from all sources

```typescript
interface ConfigurationHub {
  // Load and merge all config sources
  load(context: ConfigContext): Promise<Configuration>;
  
  // Get effective config for operation
  get(operation?: string): ConfigSnapshot;
  
  // Manage profiles
  saveProfile(name: string, config: Partial<Configuration>): void;
  loadProfile(name: string): Configuration;
  
  // Secrets management
  setSecret(key: string, value: string, provider?: SecretProvider): void;
  getSecret(key: string): Promise<string>;
}

interface Configuration {
  host: string;
  inventory_url: string;
  tls: TLSConfig;
  auth: AuthContext[];
  rate_limiting: RateLimitConfig;
  retry: RetryConfig;
  fingerprinting: FingerprintConfig;
  proxy?: ProxyConfig;
}
```

**Config Hierarchy** (highest priority first):
1. CLI flags / MCP init params
2. Environment variables
3. User config file (~/.config/openapi-cli/)
4. Inventory-provided config
5. Built-in defaults

### 2.3 Execution Engine (`/src/core/execution/`)

**Responsibility**: Execute API requests with middleware pipeline

```typescript
interface ExecutionEngine {
  execute(request: Request, context: ExecutionContext): Promise<Response>;
  registerMiddleware(middleware: Middleware): void;
}

interface ExecutionContext {
  config: Configuration;
  inventory: Inventory;
  operation?: Operation;
  fingerprint: string;
  retryCount: number;
}
```

**Middleware Pipeline**:
1. **Fingerprint Middleware**: Add client fingerprint header
2. **Auth Middleware**: Inject auth headers/tokens
3. **Rate Limit Middleware**: Enforce client-side rate limits
4. **Retry Middleware**: Handle retries with backoff
5. **WASM Pre-processor**: Transform request via WASM
6. **HTTP Client**: Send request
7. **WASM Post-processor**: Transform response via WASM
8. **Validation Middleware**: Validate response schema
9. **Cache Middleware**: Cache responses

### 2.4 WASM Runtime (`/src/core/wasm/`)

**Responsibility**: Sandboxed execution of WASM modules

```typescript
interface WASMRuntime {
  // Load and instantiate module
  load(moduleRef: ModuleRef): Promise<WASMInstance>;
  
  // Call exported function
  call(instance: WASMInstance, func: string, args: any[]): Promise<any>;
  
  // Manage memory limits
  setMemoryLimit(bytes: number): void;
  
  // Set execution timeout
  setTimeout(ms: number): void;
}

interface WASMInstance {
  exports: Record<string, Function>;
  memory: WebAssembly.Memory;
  sandbox: SandboxCapabilities;
}
```

**Sandbox Restrictions**:
- No filesystem access (unless explicitly granted)
- No network access (unless explicitly granted)
- Memory limit: 128MB default
- Execution timeout: 30 seconds default
- Can only access provided host functions

**Host Functions Provided**:
- `log(message: string)` - Logging
- `get_config(key: string)` - Read config values
- `crypto_hash(data: bytes, algorithm: string)` - Hashing
- `crypto_sign(data: bytes, key_id: string)` - Signing

### 2.5 DSL Interpreter (`/src/core/dsl/`)

**Responsibility**: Execute workflow scripts

```typescript
interface DSLInterpreter {
  // Parse and validate workflow
  parse(source: string): Workflow;
  
  // Execute workflow
  execute(workflow: Workflow, context: ExecutionContext): Promise<any>;
}

interface Workflow {
  name: string;
  steps: Step[];
}

type Step = 
  | { type: 'call'; method: string; path: string; body?: any; extract: Record<string, string> }
  | { type: 'for_each'; array: string; as: string; steps: Step[] }
  | { type: 'if'; condition: string; then: Step[]; else?: Step[] }
  | { type: 'return'; value: any };
```

**Language Features**:
- Variables from previous steps
- JSONPath for data extraction
- Simple conditionals
- Loops over arrays
- HTTP method calls
- Return values

**Security**:
- No arbitrary code execution
- No file system access
- No environment access (except whitelisted)
- Execution timeout
- Memory limits

### 2.6 Security Layer (`/src/core/security/`)

**Responsibility**: Authentication, crypto, fingerprinting

#### 2.6.1 Authentication Handlers

```typescript
interface AuthHandler {
  type: string;
  authenticate(context: ExecutionContext): Promise<AuthHeaders>;
  refresh?(context: ExecutionContext): Promise<void>;
}

// Implementations:
// - BearerAuthHandler
// - APIKeyAuthHandler  
// - BasicAuthHandler
// - OAuth2Handler
// - JWTHandler
// - MTLSHandler
```

#### 2.6.2 Client Fingerprinting

```typescript
interface FingerprintGenerator {
  generate(context: ExecutionContext): string;
  regenerate(): string;
  validate(fingerprint: string): boolean;
}

// Components:
// - Hardware info hash (if available)
// - IP address (if allowed)
// - Timestamp
// - Random component
// - Signed by server-provided key
```

#### 2.6.3 Secret Management

```typescript
interface SecretManager {
  // OS keychain integration
  getFromKeychain(service: string, account: string): Promise<string>;
  setInKeychain(service: string, account: string, value: string): Promise<void>;
  
  // Encrypted file storage
  getFromFile(path: string, key: string): Promise<string>;
  setInFile(path: string, value: string, key: string): Promise<void>;
  
  // In-memory (session only)
  setSession(key: string, value: string): void;
  getSession(key: string): string | undefined;
}
```

## 3. Data Flows

### 3.1 Initialization Flow

```
┌─────────────┐
│  User runs  │
│  CLI/MCP    │
└──────┬──────┘
       │
       ▼
┌──────────────────────────┐
│ 1. Parse CLI args /      │
│    MCP init params       │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 2. Load user config file │
│    (~/.config/...)       │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 3. Load env vars         │
│    (OPENAPICLI_*)        │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 4. Fetch inventory from  │
│    configured URL        │
│    (with caching)        │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 5. Check CLI version     │
│    requirements          │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 6. Generate CLI commands │
│    or MCP tools          │
└──────┬───────────────────┘
       │
       ▼
┌──────────────────────────┐
│ 7. Ready for commands    │
└──────────────────────────┘
```

### 3.2 Request Execution Flow

```
┌────────────────────────────────────────┐
│ 1. User invokes command/tool           │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 2. Parse arguments against OpenAPI     │
│    schema                              │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 3. Build request (URL, headers, body)  │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 4. Apply middleware chain:             │
│    - Add fingerprint                   │
│    - Inject auth                       │
│    - Check rate limits                 │
│    - Pre-process via WASM              │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 5. Send HTTP request                   │
│    (with mTLS if configured)           │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 6. Handle response                     │
│    - Retry if needed                   │
│    - Post-process via WASM             │
│    - Validate schema                   │
│    - Format output                     │
└──────┬─────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────┐
│ 7. Return result to user               │
└────────────────────────────────────────┘
```

## 4. Technology Stack

### 4.1 Runtime
- **Primary**: Node.js 18+ (LTS)
- **WASM Runtime**: Wasmtime (via @bytecodealliance/wasmtime)
- **Target**: Native binaries via `pkg`

### 4.2 Core Dependencies
```json
{
  "@bytecodealliance/wasmtime": "^14.x",
  "@modelcontextprotocol/sdk": "^0.5.x",
  "axios": "^1.x",
  "commander": "^11.x",
  "keytar": "^7.x",
  "js-yaml": "^4.x",
  "ajv": "^8.x",
  "chalk": "^5.x",
  "ora": "^7.x",
  "jsonpath-plus": "^7.x"
}
```

### 4.3 Development
- **Language**: TypeScript 5.x
- **Build**: esbuild (fast bundling)
- **Testing**: Vitest
- **Linting**: ESLint + Prettier

## 5. Storage & Caching

### 5.1 Cache Structure
```
~/.cache/openapi-cli/
├── inventory/
│   ├── <hash-of-url>/
│   │   ├── v1.0.0.json
│   │   ├── v1.1.0.json
│   │   ├── current.json (symlink)
│   │   └── known-good.json (symlink)
│   └── ...
├── modules/
│   ├── <module-hash>.wasm
│   └── <module-hash>.js
├── specs/
│   └── <spec-hash>.json
└── fingerprints/
    └── <host>.json
```

### 5.2 Configuration Storage
```
~/.config/openapi-cli/
├── config.yaml
├── profiles/
│   ├── prod.yaml
│   └── staging.yaml
├── secrets/
│   └── (encrypted files)
└── aliases/
    └── (alias definitions)
```

## 6. MCP Protocol Implementation

### 6.1 Server Capabilities
```typescript
const serverCapabilities = {
  tools: {
    listChanged: true
  },
  logging: {}
};
```

### 6.2 Tool Generation
Each OpenAPI operation becomes an MCP tool:

```typescript
interface MCPTool {
  name: string;              // operationId or generated
  description: string;       // OpenAPI description
  inputSchema: JSONSchema;   // From OpenAPI parameters/body
}

// Example:
{
  name: "users_get_by_id",
  description: "Get a user by their ID",
  inputSchema: {
    type: "object",
    properties: {
      id: { type: "string", description: "User ID" }
    },
    required: ["id"]
  }
}
```

### 6.3 Lifecycle
1. **Initialize**: Load inventory, generate tools
2. **Tools/List**: Return generated tool definitions
3. **Tools/Call**: Execute via ExecutionEngine
4. **Shutdown**: Cleanup

## 7. Security Architecture

### 7.1 Threat Model
- **Attacker controls**: Inventory server, WASM modules, network
- **Protect against**: Code injection, secret theft, MITM
- **Assume**: User's machine is secure

### 7.2 Defenses
1. **WASM Sandboxing**: Prevents arbitrary code execution
2. **Code Signing**: Verifies module authenticity
3. **mTLS**: Mutual authentication for sensitive APIs
4. **Secret Isolation**: OS keychain, encrypted files
5. **Certificate Pinning**: Prevent rogue CAs
6. **Input Validation**: JSON Schema validation

### 7.3 Secrets Flow
```
Inventory requests secret
       │
       ▼
Check OS keychain first ───────┐
       │                       │
       ▼                       │
Check encrypted file ──────────┤
       │                       │
       ▼                       │
Prompt user (CLI only) ────────┤
       │                       │
       ▼                       │
Store in keychain ◄────────────┘
```

## 8. Deployment & Distribution

### 8.1 Distribution Channels
1. **npm**: `npm install -g @openapi-cli/core`
2. **npx**: `npx @openapi-cli/core --host ...`
3. **Binary**: GitHub releases for major platforms
4. **Docker**: `docker run openapi-cli`

### 8.2 Version Compatibility
- CLI checks inventory's `min_cli_version` and `max_cli_version`
- Warns if outside range
- Can force use with `--force-version`

### 8.3 Self-Hosted Registry
Enterprise users can:
- Host private inventory server
- Use internal module registry
- Pin to specific CLI versions
- Configure via environment

---

**Document Status**: Draft v2  
**Last Updated**: 2026-02-28  
**Author**: Lexi
