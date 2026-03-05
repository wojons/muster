# Technical Decisions & Architecture Clarifications

## Executive Summary

**Language**: Go (golang) - Single binary, fast, excellent concurrency  
**WASM**: Wasmtime with wasmtime-go bindings  
**DSL**: Starlark (go.starlark.net) - Python-like, sandboxed  
**Distribution**: Bootstrap installer + self-update  
**Database**: sqlc/sqlx with golang-migrate (SQLite → MySQL → PostgreSQL)  
**Server**: Go reference server with pluggable storage (filesystem/S3)  

---

## 1. Language & Distribution Decision

### Selected: Go (golang)

**Why Go:**
- ✅ Single static binary (cross-compile trivially)
- ✅ Fast startup (critical for CLI)
- ✅ Excellent concurrency (multiplexing/proxy work)
- ✅ Great WASM hosting libraries (wasmtime-go)
- ✅ Fast compilation (LLM-assisted programming friendly)
- ✅ No runtime dependencies

**Why not Bun/Node:**
- Runtime dependency (Node version issues)
- Larger bundle size
- Slower startup
- Less control over binary

### Distribution: Bootstrap Installer Pattern

**How it works** (like `mise`, `volta`, `deno`):

```bash
# One-line installer
curl -fsSL https://install.openapi-cli.io | sh

# What the installer does:
# 1. Detect platform/arch (linux/amd64, darwin/arm64, etc.)
# 2. Download appropriate binary from GitHub releases
# 3. Verify checksum/signature
# 4. Install to ~/.local/bin or /usr/local/bin
# 5. Execute the binary
```

**Self-Update** (using creativeprojects/go-selfupdate):

```bash
# Manual update
openapi-cli update

# Auto-update on launch (configurable)
openapi-cli --auto-update

# Check without updating
openapi-cli update --check
```

**Version Pinning from Inventory:**
```json
{
  "cli_requirements": {
    "min_version": "1.4.0",
    "max_version": "2.0.0",
    "update_policy": "warn"  // warn | force | block
  }
}
```

**Enterprise Bundled Case:**
- Pre-ship the binary (no bootstrapper needed)
- Binary checks inventory for version requirements
- Can pin to specific version via config

---

## 2. WASM Runtime Selection

### Selected: Wasmtime with wasmtime-go

**Why Wasmtime:**
- ✅ Bytecode Alliance backed (Mozilla, Fastly, Intel, ARM)
- ✅ Best WASI support for capability grants
- ✅ wasmtime-go bindings maintained
- ✅ Explicit capability model
- ✅ Component Model support (future-proof)

**Capability Model Example:**

```go
import "github.com/bytecodealliance/wasmtime-go"

func runWASM(module []byte, caps CapabilityConfig) error {
    engine := wasmtime.NewEngine()
    store := wasmtime.NewStore(engine)
    
    // Create WASI config with explicit capabilities
    wasiConfig := wasmtime.NewWasiConfig()
    
    // File system access (disabled by default)
    if caps.Filesystem {
        wasiConfig.PreopenDir(caps.FSPath, "/data")
    }
    
    // Network access (disabled by default)
    // Modules can only use our HTTP client via host functions
    
    // Memory limits
    store.Limiter(
        int64(caps.MaxMemory),  // max memory
        -1,                      // max table entries
        int64(caps.MaxFSSize),  // max file size
        -1,                      // max files open
    )
    
    // Time limits (via context)
    ctx, cancel := context.WithTimeout(context.Background(), caps.MaxExecutionTime)
    defer cancel()
    
    // Link host functions (controlled HTTP access)
    linker := wasmtime.NewLinker(engine)
    linker.FuncWrap("env", "http_request", func(url, method, body string) string {
        // Routes through our HTTP client
        // Auth, rate limiting, fingerprinting all apply
        return makeControlledHTTPRequest(url, method, body)
    })
    
    // Instantiate and run
    instance, err := linker.Instantiate(store, module)
    // ... execute
}
```

**Capability Intersection:**
```
Server Requirements (inventory)
        ∩
User Configuration (~/.config/openapi-cli/capabilities.yaml)
        ∩
Command Line Flags (--allow-filesystem)
        ∩
Security Team Policy (/etc/security/openapi-cli/policy.yaml)
        =
Actual Capabilities Enforced at Runtime
```

---

## 3. DSL Selection

### Selected: Starlark (go.starlark.net)

**Why Starlark:**
- ✅ Official Go implementation (used by Bazel)
- ✅ Python-like syntax (familiar)
- ✅ Deterministic execution (no concurrency, no I/O by default)
- ✅ Sandboxed (no filesystem, no network, no OS access)
- ✅ Easy to extend with Go builtins

**Go Integration:**

```go
import "go.starlark.net/starlark"
import "go.starlark.net/starlarkstruct"

func executeDSL(script string, ctx ExecutionContext) (starlark.Value, error) {
    // Create thread with limits
    thread := &starlark.Thread{
        Name: "workflow",
        Load: nil, // Disallow imports
    }
    
    // Set execution steps limit
    thread.SetMaxExecutionSteps(100000)
    
    // Register HTTP builtins
    builtins := starlark.StringDict{
        "http": starlarkstruct.FromStringDict(
            starlarkstruct.Default, 
            starlark.StringDict{
                "get": starlark.NewBuiltin("http_get", httpGetBuiltin),
                "post": starlark.NewBuiltin("http_post", httpPostBuiltin),
                "patch": starlark.NewBuiltin("http_patch", httpPatchBuiltin),
                "delete": starlark.NewBuiltin("http_delete", httpDeleteBuiltin),
            },
        ),
        "json": starlarkstruct.FromStringDict(
            starlarkstruct.Default,
            starlark.StringDict{
                "encode": starlark.NewBuiltin("json_encode", jsonEncodeBuiltin),
                "decode": starlark.NewBuiltin("json_decode", jsonDecodeBuiltin),
                "path": starlark.NewBuiltin("json_path", jsonPathBuiltin), // JSONPath support
            },
        ),
    }
    
    // Parse and execute
    globals, err := starlark.ExecFile(thread, "workflow.star", script, builtins)
    if err != nil {
        return nil, err
    }
    
    // Get return value
    return globals["result"], nil
}

// HTTP builtin implementation
func httpGetBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path string
    if err := starlark.UnpackArgs("http.get", args, kwargs, "path", &path); err != nil {
        return nil, err
    }
    
    // Route through our HTTP client
    // Auth, rate limiting, retry all apply automatically
    resp, err := httpClient.Get(path)
    if err != nil {
        return nil, err
    }
    
    // Return as Starlark dict
    return responseToStarlarkDict(resp), nil
}
```

**Example DSL Script:**

```python
# Workflow: Fetch user and their posts
user = http.get("/users/123").json()

posts = http.get("/users/123/posts").json()

results = []
for post in posts:
    # Process each post
    processed = http.post("/process", body={"id": post["id"]})
    results.append({
        "title": post["title"],
        "status": processed.json()["status"]
    })

# Return final result
result = {
    "user": user["name"],
    "posts_processed": len(results),
    "details": results
}
```

**Security Guarantees:**
- No filesystem access (unless Go builtin explicitly provides it)
- No network access except through controlled HTTP client
- No imports (can't load external code)
- Execution steps limit (prevents infinite loops)
- Memory limits enforced by Go runtime

---

## 4. Database Architecture

### Selected: sqlc + golang-migrate

**Schema Management:**

```sql
-- migrations/001_initial_schema.sql
CREATE TABLE inventories (
    id TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    version TEXT NOT NULL,
    content JSONB NOT NULL,
    sha256 TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host, version)
);

CREATE TABLE modules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sha256 TEXT NOT NULL UNIQUE,
    content BYTEA NOT NULL,
    signature TEXT,
    downloaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE aliases (
    name TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    version TEXT,
    config JSONB
);
```

**Go Code Generation with sqlc:**

```yaml
# sqlc.yaml
version: "2"
sql:
  - schema: "migrations/"
    queries: "queries/"
    engine: "postgresql"
    gen:
      go:
        package: "db"
        out: "internal/db"
```

**Query Example:**

```sql
-- queries/inventory.sql
-- name: GetInventory :one
SELECT * FROM inventories 
WHERE host = $1 AND version = $2;

-- name: ListCachedVersions :many
SELECT version, created_at FROM inventories 
WHERE host = $1 
ORDER BY created_at DESC;

-- name: SaveInventory :exec
INSERT INTO inventories (id, host, version, content, sha256)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (host, version) DO UPDATE SET
    content = EXCLUDED.content,
    sha256 = EXCLUDED.sha256;
```

**Multi-Driver Support:**

```go
type Store interface {
    GetInventory(ctx context.Context, host, version string) (Inventory, error)
    ListCachedVersions(ctx context.Context, host string) ([]Version, error)
    SaveInventory(ctx context.Context, inv Inventory) error
    // ...
}

// SQLite implementation
func NewSQLiteStore(dbPath string) (Store, error) {
    db, err := sql.Open("sqlite3", dbPath)
    // ...
}

// PostgreSQL implementation
func NewPostgresStore(connString string) (Store, error) {
    db, err := sql.Open("postgres", connString)
    // ...
}
```

**Migration Tooling:**

```bash
# Using golang-migrate
migrate -database "sqlite3://~/.local/share/openapi-cli/db.sqlite" -path migrations up
migrate -database "postgres://user:pass@localhost/db" -path migrations up
```

---

## 5. Storage Abstraction

### Interface Design

```go
type Storage interface {
    // Inventory
    GetInventory(ctx context.Context, host string, version string) (*Inventory, error)
    SaveInventory(ctx context.Context, inv *Inventory) error
    
    // Modules (WASM, DSL scripts)
    GetModule(ctx context.Context, sha256 string) ([]byte, error)
    SaveModule(ctx context.Context, sha256 string, content []byte) error
    
    // Static assets
    GetAsset(ctx context.Context, path string) ([]byte, error)
}

// Implementations:
// - FileSystemStorage (default, local files)
// - S3Storage (AWS S3, R2)
// - CachedStorage (wrapper with local cache)
```

**CDN-Optimized Deployment:**

```
Dynamic Config Server (Go)
├── Handles auth
├── Issues credentials
├── Generates inventory JSON
└── Uploads to S3

CDN (CloudFront/Cloudflare)
├── Serves inventory.json
├── Serves WASM blobs (content-addressed)
├── Serves DSL scripts
└── Cache headers for fast global delivery

Client (openapi-cli)
├── Fetches from CDN
├── Verifies signatures
├── Caches locally
└── Executes
```

**Content-Addressed Storage:**

```go
// Modules stored by SHA256 hash
func (s *S3Storage) SaveModule(ctx context.Context, content []byte) (string, error) {
    hash := sha256.Sum256(content)
    key := fmt.Sprintf("modules/%x", hash)
    
    _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
        Bucket: &s.bucket,
        Key:    &key,
        Body:   bytes.NewReader(content),
        // Immutable content = infinite cache
        CacheControl: aws.String("public, max-age=31536000, immutable"),
    })
    
    return key, err
}
```

---

## 6. MCP Transport & Connection Model

### Three Modes, One Binary

**Mode 1: stdio (Default MCP)**

```bash
# Claude Desktop config
{
  "mcpServers": {
    "my-api": {
      "command": "/usr/local/bin/openapi-cli",
      "args": ["--mcp", "--host", "https://api.example.com"]
    }
  }
}
```

Go implementation:

```go
func runMCPStdio(config Config) error {
    server := mcp.NewServer(
        mcp.WithName("openapi-cli-mcp"),
        mcp.WithVersion(config.Inventory.Version),
    )
    
    // Register tools from OpenAPI
    for _, op := range config.Inventory.OpenAPI.Operations {
        server.RegisterTool(mcp.Tool{
            Name:        op.OperationID,
            Description: op.Description,
            InputSchema: op.InputSchema,
            Handler:     makeToolHandler(op),
        })
    }
    
    // Connect via stdio
    transport := stdio.NewTransport()
    return server.Serve(transport)
}
```

**Mode 2: Local Daemon + Unix Socket/HTTP**

```bash
# Start daemon
openapi-cli --daemon --socket /var/run/openapi-cli.sock --host https://api.example.com

# Or HTTP mode
openapi-cli --daemon --port 8080 --host https://api.example.com
```

Architecture:

```
Agent A ──┐
Agent B ──┼──► unix socket ──► daemon ──► rate limiter ──► API
Agent C ──┘                    (one instance)
                               
Benefits:
- One WASM compilation per module
- Shared rate limit budget
- Single TLS connection to API
- Centralized audit logging
```

Go implementation:

```go
type Daemon struct {
    config      Config
    httpClient  *http.Client
    wasmCache   map[string]wasmtime.Module
    rateLimiter *RateLimiter
    authManager *AuthManager
}

func (d *Daemon) ServeUnixSocket(socketPath string) error {
    listener, err := net.Listen("unix", socketPath)
    if err != nil {
        return err
    }
    defer os.Remove(socketPath)
    
    // Set permissions (only owner can connect)
    os.Chmod(socketPath, 0600)
    
    return d.serve(listener)
}

func (d *Daemon) handleConnection(conn net.Conn) {
    defer conn.Close()
    
    // Read MCP request
    decoder := json.NewDecoder(conn)
    encoder := json.NewEncoder(conn)
    
    for {
        var req MCPRequest
        if err := decoder.Decode(&req); err != nil {
            return
        }
        
        resp := d.processRequest(req)
        encoder.Encode(resp)
    }
}
```

**Mode 3: CLI (Human/Script Use)**

```bash
# Auto-generated commands from OpenAPI
openapi-cli users list --limit 10
openapi-cli users get --id 123
openapi-cli posts create --title "Hello"
```

**Connection Limit Enforcement:**

```go
// Security policy: max 1 upstream connection per user
type ConnectionLimiter struct {
    mu          sync.Mutex
    connections map[string]net.Conn // user -> connection
}

func (cl *ConnectionLimiter) Acquire(userID string) (net.Conn, error) {
    cl.mu.Lock()
    defer cl.mu.Unlock()
    
    if existing, ok := cl.connections[userID]; ok {
        // Reject or queue
        return nil, fmt.Errorf("max connections reached for user %s", userID)
    }
    
    // Create new connection
    conn, err := cl.dialUpstream()
    if err != nil {
        return nil, err
    }
    
    cl.connections[userID] = conn
    return conn, nil
}
```

---

## 7. Signing & Verification

### Supported Methods

```go
type SignatureVerifier interface {
    Verify(data []byte, signature []byte) error
}

// TLS-based (certificate pinning)
type TLSPinnedVerifier struct {
    pinnedCert *x509.Certificate
}

// GPG/PGP
type GPGVerifier struct {
    keyRing openpgp.EntityList
}

// SSH key signing
type SSHVerifier struct {
    publicKey ssh.PublicKey
}
```

**Configuration:**

```yaml
# ~/.config/openapi-cli/trust.yaml
trusted_keys:
  - host: api.example.com
    method: tls
    pin: sha256//abc123...  # Certificate pinning
    
  - host: api.enterprise.com
    method: gpg
    key: |
      -----BEGIN PGP PUBLIC KEY BLOCK-----
      ...
      -----END PGP PUBLIC KEY BLOCK-----
      
  - host: api.legacy.com
    method: ssh
    key: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...

# Trust on First Use (TOFU) for development
tofu: true
```

**Verification Flow:**

```go
func verifyInventory(inv []byte, sig InventorySignature, trustConfig TrustConfig) error {
    switch sig.Method {
    case "tls":
        // Already verified by TLS connection + optional pinning
        return verifyTLSPin(inv, sig, trustConfig)
        
    case "gpg":
        return verifyGPG(inv, sig.Signature, trustConfig.GPGKey)
        
    case "ssh":
        return verifySSH(inv, sig.Signature, trustConfig.SSHKey)
        
    default:
        return fmt.Errorf("unknown signing method: %s", sig.Method)
    }
}

// WASM blobs are content-addressed
// If inventory is signed, referenced WASM modules are implicitly trusted
func verifyWASM(module []byte, expectedHash string) error {
    hash := sha256.Sum256(module)
    actualHash := hex.EncodeToString(hash[:])
    if actualHash != expectedHash {
        return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
    }
    return nil
}
```

---

## 8. Revised Build Order

### Phase 1: Core Foundation (Weeks 1-3)
1. **Go project setup** (mod, structure, CI)
2. **HTTP client** with:
   - Auth (basic → API key → JWT → OAuth2)
   - Retry with exponential backoff
   - Rate limiting
   - Fingerprinting (stub)
3. **Inventory fetch + cache**:
   - HTTP caching (ETag, Cache-Control)
   - Version storage (latest, last-known-good, pinned)
   - Alias management

### Phase 2: Interface Generation (Weeks 4-6)
4. **OpenAPI parser** (using kin-openapi or similar)
5. **CLI command generation** (using cobra or urfave/cli)
6. **MCP stdio server** (using mcp-go-sdk)
7. **Tool registration** from OpenAPI spec

### Phase 3: Security (Weeks 7-9)
8. **Signing verification** (GPG, SSH, TLS pinning)
9. **OS keychain integration** (using 99designs/keyring)
10. **mTLS support** (client certificates)
11. **Client fingerprinting** (with configurable policies)

### Phase 4: Scripting (Weeks 10-12)
12. **Starlark integration** (go.starlark.net)
13. **HTTP builtins** (http.get, http.post, etc.)
14. **Resource limits** (CPU, memory, execution steps)
15. **Simple workflow execution**

### Phase 5: WASM (Weeks 13-15)
16. **Wasmtime integration** (wasmtime-go)
17. **Capability model** (explicit grants)
18. **Module loading** (download, verify, cache)
19. **Host function registration** (controlled HTTP access)

### Phase 6: Enterprise Features (Weeks 16-18)
20. **Daemon mode** (unix socket, HTTP)
21. **Connection multiplexing**
22. **Shared state** (WASM cache, rate limits)
23. **Audit logging**

### Phase 7: Reference Server (Weeks 19-21)
24. **Go server** with database/sql
25. **Multi-driver support** (SQLite, MySQL, PostgreSQL)
26. **S3/CloudFront adapter**
27. **Webhook proxy**
28. **Credential provisioning**

### Phase 8: Distribution (Weeks 22-23)
29. **Self-update** (go-selfupdate)
30. **Bootstrap installer** (shell/powershell)
31. **Documentation**
32. **Release automation**

**Total Timeline**: ~23 weeks (can parallelize some phases)

---

## 9. Go Dependencies

```go
// go.mod
module github.com/openapi-cli/core

go 1.21

require (
    // CLI
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0
    
    // MCP
    github.com/mcp-sdk/mcp-go v0.5.0
    
    // WASM
    github.com/bytecodealliance/wasmtime-go v14.0.0
    
    // DSL
    go.starlark.net v0.0.0-20231121155337-90ade8b19d09
    
    // HTTP
    github.com/go-resty/resty/v2 v2.11.0
    
    // Database
    github.com/sqlc-dev/sqlc v1.24.0
    github.com/golang-migrate/migrate/v4 v4.17.0
    github.com/lib/pq v1.10.9        // PostgreSQL
    github.com/go-sql-driver/mysql v1.7.1  // MySQL
    github.com/mattn/go-sqlite3 v1.14.19   // SQLite
    
    // Secrets
    github.com/99designs/keyring v1.2.2
    
    // Auth
    golang.org/x/oauth2 v0.15.0
    github.com/golang-jwt/jwt/v5 v5.2.0
    
    // Signing
    golang.org/x/crypto v0.17.0  // GPG, SSH
    
    // Config
    github.com/knadh/koanf v2.0.0
    
    // Utilities
    github.com/hashicorp/go-version v1.6.0
    github.com/creativeprojects/go-selfupdate v1.1.0
    github.com/charmbracelet/log v0.3.1
    github.com/charmbracelet/spinner v1.1.0
)
```

---

## 8a. Key Server (Credential Provisioning)

### Design

The inventory provides credentials in one of two ways:

**Option 1: Direct Blob (Inventory contains encrypted credentials)**
```json
{
  "auth": {
    "api_auth": {
      "type": "encrypted_blob",
      "blob": "base64encrypted...",
      "algorithm": "aes-256-gcm"
    }
  }
}
```

**Option 2: Key Server (Separate credential endpoint)**
```json
{
  "auth": {
    "api_auth": {
      "type": "key_server",
      "endpoint": "https://keys.example.com/credentials",
      "auth": {"type": "bearer", "token_env": "KEY_SERVER_TOKEN"}
    }
  }
}
```

**Key Server Protocol:**

```go
// Client fetches credentials from key server
type KeyServerClient struct {
    Endpoint string
    Auth     AuthConfig
}

func (k *KeyServerClient) FetchCredentials(ctx context.Context) (*Credentials, error) {
    // Authenticate to key server
    // Receive encrypted credentials
    // Decrypt with key from OS keychain or key file
    return &Credentials{
        Token: decryptedToken,
        Expires: expiryTime,
    }, nil
}
```

**Use Cases:**
- High-security environments: credentials never in inventory
- Rotation: key server issues short-lived tokens
- Audit: key server logs all credential access
- Multi-tenant: different creds per user/role

**Implementation:**
- Configurable: inventory owner chooses direct vs key server
- Key server is optional component in bundled server
- Can be separate service entirely
- Client supports both modes transparently

---

## 8b. Webhook Proxy

### Design

When API providers need to send webhooks to clients behind NAT/firewall:

```
API Server ──► Webhook ──► Reference Server ──► Client
                (public)      (proxy)            (via WebSocket/SSE)
```

**Protocol:**

1. Client establishes persistent connection to server (WebSocket/SSE)
2. Server provides webhook URL: `https://hooks.example.com/webhook/{client-id}`
3. API provider sends webhooks to that URL
4. Server forwards to client via persistent connection
5. Client responds, server proxies response back

**Go Implementation:**

```go
type WebhookProxy struct {
    clients map[string]*ClientConnection
}

func (p *WebhookProxy) RegisterClient(clientID string, conn *websocket.Conn) {
    p.clients[clientID] = &ClientConnection{
        ID: clientID,
        Conn: conn,
        LastActivity: time.Now(),
    }
}

func (p *WebhookProxy) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    clientID := extractClientID(r.URL.Path)
    
    client, ok := p.clients[clientID]
    if !ok {
        http.Error(w, "Client not connected", http.StatusNotFound)
        return
    }
    
    // Forward to client
    body, _ := io.ReadAll(r.Body)
    err := client.Conn.WriteJSON(WebhookMessage{
        Headers: r.Header,
        Body: body,
    })
    
    // Wait for response
    var response WebhookResponse
    client.Conn.ReadJSON(&response)
    
    // Proxy back to caller
    w.WriteHeader(response.StatusCode)
    w.Write(response.Body)
}
```

**Security:**
- Client must maintain connection (heartbeat)
- Server validates webhook signatures
- Rate limiting per client
- Audit logging

**Deployment Options:**
- Enabled: Full proxy functionality
- Disabled: Client must expose public endpoint
- Community: Others can implement custom proxies

---

## 8c. WASM Security Rationale

### Why WASM Instead of JavaScript Sidecar

**Original Problem:**
> "WASM will be more secure and less prone to crashing than just sidecar injecting JavaScript or something and running it"

**Comparison:**

| Aspect | JS Sidecar | WASM (Wasmtime) |
|--------|-----------|-----------------|
| **Sandboxing** | V8 isolates (complex) | WASI capabilities (explicit) |
| **Crash Isolation** | Process crash = all JS | Module crash = isolated |
| **Memory Safety** | GC (unpredictable pauses) | Linear memory (controlled) |
| **Startup Time** | V8 init (~100ms) | Near instant |
| **Bundle Size** | V8 runtime (~20MB) | Wasmtime (~15MB) |
| **Determinism** | JS semantics vary | Deterministic by design |
| **Auditability** | Hard to audit JS | WASM is inspectable |

**Why Not JavaScript:**
1. **Security Surface**: Node.js/V8 has huge attack surface (filesystem, network, child_process)
2. **Sandbox Complexity**: vm2 and similar have had security vulnerabilities
3. **Resource Control**: Hard to limit memory/CPU in V8
4. **Non-Determinism**: JS has undefined behavior, floating point quirks
5. **Dependency Hell**: npm packages, supply chain attacks

**Why WASM (Wasmtime specifically):**
1. **Explicit Capabilities**: Nothing allowed unless explicitly granted
2. **Memory Safety**: Linear memory with bounds checking
3. **Resource Limits**: CPU, memory enforced at runtime
4. **Deterministic**: Same input = same output, every time
5. **Language Agnostic**: Rust, Go, C++ can all compile to WASM
6. **Production Proven**: Used by Fastly, Cloudflare, Shopify

**The DSL Complement:**
- WASM: Complex logic, computation, custom algorithms
- Starlark DSL: Simple glue, endpoint chaining, basic scripting
- User chooses based on complexity

**Enterprise Security:**
```yaml
# Security team can mandate WASM only
security_policy:
  allow_dsl: true
  allow_wasm: true
  allow_js: false  # Explicitly disabled
  wasm_capabilities:
    http: true
    filesystem: false  # No file access
    max_memory: "64MB"
```

---

## Summary

**Language**: Go - single binary, fast, great concurrency  
**WASM**: Wasmtime - production-ready, explicit capabilities  
**DSL**: Starlark - sandboxed, Python-like, easy to extend  
**Distribution**: Bootstrap installer + self-update  
**Database**: sqlc + golang-migrate (SQLite → PostgreSQL)  
**Storage**: Pluggable (filesystem/S3) with CDN support  
**MCP**: Three modes (stdio, daemon socket, daemon HTTP)  
**Signing**: TLS, GPG, SSH - configurable by server  

This architecture provides maximum flexibility while maintaining security and ease of deployment.
