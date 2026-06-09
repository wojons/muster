# Muster — Master Work Item List

**Generated**: 2026-05-29 | **Source**: master-gap-analysis-2026-05-29.md
**Convention**: Work items ordered by dependency (earlier items unblock later ones)

---

## Phase 1: Foundation Fixes (unblock everything else)

### WI-001: Unify Configuration System
- **Gap**: GAP-002 (CRITICAL)
- **Goal**: Single viper instance, single env prefix (OPENAPICLI_*), all sources merged
- **Specs**: CONFIGURATION-MANAGEMENT.md, VIPER-INTEGRATION.md, CONFIGURATION-SCHEMA.md
- **Tasks**:
  1. Consolidate cmd/cli/main.go and pkg/config/loader.go into single config system
  2. Add remaining config sources (inventory, profiles, env, CLI flags → single merge)
  3. Wire config into all entry points uniformly (CLI, MCP, Daemon)
  4. Verify env prefix consistency (OPENAPICLI_* everywhere)
- **Dependencies**: None
- **Effort**: 8h
- **Priority**: P0 — everything depends on config

### WI-002: Integrate Auth Pipeline
- **Gap**: GAP-004 (HIGH)
- **Goal**: Wire pkg/auth/ primitives into pkg/client/ and pkg/request/ 
- **Specs**: AUTH-IMPLEMENTATION.md, API-AUTHENTICATION.md
- **Tasks**:
  1. Define `Authenticator` interface (ApplyAuth(ctx, req) error)
  2. Create OAuth2, mTLS, API Key, Basic auth providers implementing interface
  3. Wire authenticator selection into HTTP client middleware chain
  4. Auto-apply auth in generator's `createRunHandler()` 
  5. Wire into MCP OpenAPIExecutor
- **Dependencies**: None
- **Effort**: 12h
- **Priority**: P0 — needed for dynamic API interaction

---

## Phase 2: Core Capability Gaps

### WI-003: Implement Plugin System
- **Gap**: GAP-001 (CRITICAL)
- **Goal**: Full CLI-MARKET-PLUGINS.md implementation
- **Specs**: CLI-MARKET-PLUGINS.md, PLUGIN-INTERFACE.md, PLUGIN-LOADER.md
- **Tasks**:
  1. Define Plugin interface (Metadata, Initialize, RegisterCommands, Cleanup)
  2. Implement Loader (plugin.Open for Go, exec for external, WASM loader)
  3. Implement Registry (Register, Get, List, Unload with mutex)
  4. Implement Manager (load from plugin dirs, auto-discover, dependency resolution)
  5. Add plugin CLI commands (plugin list, install, remove, info, enable, disable)
  6. Implement HookManager (PreCommand, PostCommand, PreRequest, PostResponse)
  7. Add plugin configuration (~/.muster/plugins.yaml)
  8. Add security controls (signature verification, capability limiting)
- **Dependencies**: WI-001 (config)
- **Effort**: 40h
- **Priority**: P1

### WI-004: Add PostgreSQL & MySQL Support
- **Gap**: GAP-003 (CRITICAL)
- **Goal**: Add postgres.go + mysql.go connection implementations
- **Specs**: DATABASE-SCHEMA.md, DATABASE-CONNECTION.md, DATABASE-QUERIES.md
- **Tasks**:
  1. Implement postgres.go (connection, pool, query dialect using $N placeholders)
  2. Implement mysql.go (connection, pool, query dialect using ? placeholders)
  3. Add connection string templates and validation per driver
  4. Add migration driver support for PostgreSQL and MySQL
  5. Integration tests for each backend
- **Dependencies**: None
- **Effort**: 20h
- **Priority**: P1

### WI-005: Wire WASM into CLI/DSL Execution
- **Gap**: GAP-012 (MEDIUM)
- **Goal**: WASM runtime as alternative execution backend
- **Specs**: WASM-RUNTIME.md, STARLARK-DSL.md, DSL-ENGINE.md
- **Tasks**:
  1. Add WASM execution option to generator's createRunHandler()
  2. Add DSL wasm_call() builtin function
  3. Add request pre-processing / response post-processing WASM hooks
  4. Ensure capability isolation across execution paths
- **Dependencies**: None
- **Effort**: 8h
- **Priority**: P2

---

## Phase 3: MCP & Daemon Completeness

### WI-006: Implement MCP LLM Sampling
- **Gap**: GAP-007 (MEDIUM)
- **Goal**: MCP sampling/createMessage support
- **Specs**: MCP-MARKET-SAMPLING.md (243 lines)
- **Tasks**:
  1. Implement CreateMessageRequest/SamplingMessage types
  2. Add sampling/createMessage handler
  3. Implement ModelPreferences, SamplingCapability
  4. Register in MCP server lifecycle
- **Dependencies**: None
- **Effort**: 8h
- **Priority**: P2

### WI-007: Daemon Fixes
- **Gap**: GAP-011 (MEDIUM)
- **Goal**: Log rotation, correct socket path, config hot-reload
- **Specs**: DAEMON-CONFIGURATION.md, DAEMON-LIFECYCLE.md
- **Tasks**:
  1. Implement log rotation (size-based via lumberjack or custom)
  2. Fix socket path to ~/.local/share/openapi-cli/daemon.sock
  3. Wire config file watching into daemon (actual reload, not just signal catch)
  4. Add daemon config schema (ServerConfig, DatabaseConfig, Logging, Metrics, Tracing)
- **Dependencies**: WI-001 (config)
- **Effort**: 6h
- **Priority**: P2

---

## Phase 4: Integration & Pipeline

### WI-008: Rate Limiter Integration
- **Gap**: GAP-013 (MEDIUM)
- **Goal**: Wire token bucket into HTTP client middleware
- **Tasks**:
  1. Add RateLimitMiddleware to pkg/client/
  2. Auto-configure per-host buckets from spec rate limit info
  3. Add rate-limiting status to CLI output and MCP tool responses
- **Dependencies**: WI-001 (config)
- **Effort**: 4h
- **Priority**: P2

### WI-009: Unified Command & Auth State
- **Gap**: GAP-008 + GAP-009 (MEDIUM)
- **Goal**: Shared command interface + shared auth state
- **Specs**: UNIFIED-COMMANDS.md, UNIFIED-AUTH.md
- **Tasks**:
  1. Define Command interface (Execute(ctx, CommandInput) (CommandResult, error))
  2. Create CommandRegistry (Register, Get, List, ExecuteByName)
  3. Refactor CLI cobra commands to implement Command interface
  4. Refactor MCP tool handlers to implement Command interface
  5. Implement AuthManager with shared state between CLI and MCP
  6. Implement AuthSyncer for credential propagation
- **Dependencies**: WI-002 (auth pipeline)
- **Effort**: 16h
- **Priority**: P3

---

## Phase 5: Hardening & Operations

### WI-010: Improve Database Connection Management
- **Gap**: GAP-010 (MEDIUM)
- **Goal**: Connection pool, health checks
- **Specs**: DATABASE-CONNECTION.md
- **Tasks**:
  1. Implement PoolConfig (open/idle/lifetime/retry)
  2. Add HealthChecker with liveness probe
  3. Add pool metrics (active/idle/wait/inUse)
  4. Add connection retry with exponential backoff
- **Dependencies**: WI-004 (multi-DB)
- **Effort**: 10h
- **Priority**: P3

### WI-011: Asymmetric Cryptography
- **Gap**: GAP-005 (HIGH)
- **Goal**: Ed25519, RSA-OAEP, ECDH support
- **Specs**: CRYPTO-IMPLEMENTATION.md
- **Tasks**:
  1. Implement Ed25519 key generation and signing/verification
  2. Add RSA-OAEP 4096-bit key generation/encryption/decryption
  3. Add ECDH P-384 and X25519 key exchange
  4. Add SHA3-256 and BLAKE2b hash functions
  5. Implement key hierarchy (Master → DEK → KEK → Signing)
- **Dependencies**: WI-002 (auth pipeline)
- **Effort**: 15h
- **Priority**: P3

### WI-012: Performance Benchmark Suite
- **Gap**: GAP-014 (MEDIUM)
- **Goal**: Systematic performance validation
- **Specs**: BENCHMARK-SUITE.md, PERFORMANCE-REQUIREMENTS.md
- **Tasks**:
  1. Add OpenAPI parsing benchmarks (<100ms for 100 operations)
  2. Add WASM load/execute benchmarks (<50ms for 1MB module)
  3. Add DSL execution benchmarks (<10ms for simple scripts)
  4. Add HTTP request benchmarks (local <10ms, remote <1s)
  5. Add database query benchmarks (<1ms for indexed SELECT)
  6. Add regression testing with p95/p99 tracking
  7. Add load testing framework (100 concurrent users)
- **Dependencies**: None
- **Effort**: 8h
- **Priority**: P3

---

## Phase 6: Advanced Features (nice-to-have)

### WI-013: WebSocket Webhook Protocol
- **Gap**: GAP-015 (MEDIUM)
- **Goal**: WebSocket streaming + bidirectional communication
- **Specs**: API-WEBHOOK-PROTOCOL.md
- **Tasks**:
  1. Add WebSocket connection management
  2. Implement auto-reconnection with exponential backoff
  3. Add heartbeat/ping-pong keepalive
  4. Implement message acknowledgment
  5. Add event filtering subscriptions
- **Dependencies**: None
- **Effort**: 10h
- **Priority**: P4

### WI-014: Kernel-Level Runtime Sandbox
- **Gap**: GAP-006 (HIGH)
- **Goal**: cgroups v2 + Linux namespaces + seccomp
- **Specs**: RUNTIME-SANDBOX.md
- **Tasks**:
  1. Implement cgroups v2 memory/CPU/IO limits
  2. Add Linux namespace isolation (mount, PID, network, UTS, IPC)
  3. Implement seccomp filter profiles (allow-listed syscalls)
  4. Create ResourceLimiter with CPU/memory/IO/PID quotas
- **Dependencies**: None (Linux only)
- **Effort**: 25h
- **Priority**: P4

### WI-015: Cross-Platform Package Distribution
- **Gap**: GAP-019 (LOW)
- **Goal**: Homebrew, npm, PyPI, Scoop, Chocolatey, apt/yum repos
- **Effort**: 12h
- **Priority**: P4

---

## Execution Order

```
Phase 1 (Foundation):    WI-001 → WI-002
Phase 2 (Core):          WI-003, WI-004, WI-005  [parallel]
Phase 3 (MCP/Daemon):    WI-006, WI-007  [parallel]
Phase 4 (Integration):   WI-008 → WI-009
Phase 5 (Hardening):     WI-010 → WI-011, WI-012  [WI-010 blocks 011, WI-012 parallel]
Phase 6 (Advanced):      WI-013, WI-014, WI-015  [all parallel]
```

**Total estimated effort**: ~170h across 15 work items

**Dependency chain**: WI-001 → WI-002 → WI-009 → WI-011
                           → WI-003
                           → WI-007

---

## Quick-Start (Phase 1)

Begin with WI-001 (Unify Config, 8h) and WI-002 (Auth Pipeline, 12h).
These unblock everything else.
