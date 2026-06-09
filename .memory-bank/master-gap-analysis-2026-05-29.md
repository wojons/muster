# Muster — Master Gap Analysis

**Generated**: 2026-05-29 | **Project**: /home/kara/projects/muster
**Implementation State**: 456 Go files, 233 test files, 28 pkg packages, 13 internal packages
**Overall Completeness**: ~80-85% for Go CLI MVP scope

---

## Analysis Methodology

Two sub-agents independently analyzed the project:
- **Agent A**: Read 60+ unverified spec files, cross-referenced against progress.md
- **Agent B**: Deep-verified all 19 functional requirement areas against actual code paths
- **Parent**: Verified codebase state directly (build, tests, package structure, key gaps)

Convergent findings confirmed by both agents and parent verification.

---

## Part 1: What EXISTS (Rebutting Stale Analysis)

The prior memory bank (progress.md, March 2026) claimed these were 0% unimplemented. They are now RESOLVED:

| Old Claim | Actual State |
|-----------|-------------|
| "Audit logging 0%" | **14 files** in internal/audit/ |
| "Telemetry empty" | **13 files** in internal/telemetry/ (full OpenTelemetry) |
| "Events handlers 0%" | **9 files** in pkg/events/ (w/ circuit breaker) |
| "Alias system 0%" | **9 files** in internal/alias/ |
| "Config watching missing" | **pkg/config/watcher.go** (WatchConfig/OnConfigChange) |
| "HTTP canonicalization 0%" | **pkg/httputil/canonical.go** |
| "OpenAPIExecutor stub" | **pkg/mcp/openapi_integration.go** — fully implemented |
| "Daemon tests 100% skipped" | **3 test files, 0 skips** |
| "Linux security 100% unimplemented" | **2 stubs** (test helper + Windows fallback) |
| "Keychain Linux broken" | **pkg/auth/keychain_secret_service_linux.go** exists |
| "Empty internal dirs" | **NONE** — all 13 internal dirs have files |

---

## Part 2: Critical Gaps (Must Fix)

### 🔴 CRITICAL

#### GAP-001: Plugin System — Not Implemented
- **Specs**: CLI-MARKET-PLUGINS.md (443 lines), PLUGIN-INTERFACE.md (593 lines), PLUGIN-LOADER.md
- **Current**: Zero plugin code. Only `PluginConfig` struct in config/types.go (unused).
- **Requirements**: Plugin interface, Loader, Registry, Manager, plugin commands (list/install/remove), hooks (PreCommand/PostRequest), configuration
- **Impact**: Entire feature area missing. No extensibility story.
- **Effort**: ~40h

#### GAP-002: Configuration System — Bifurcated
- **Spec**: 12 configuration sources merged in priority order, single env prefix (OPENAPICLI_*)
- **Current**: TWO separate viper systems:
  - `cmd/cli/main.go` uses `viper.New()` with `MUSTER` env prefix (lines 111-164)
  - `pkg/config/loader.go` uses `OPENAPI_CLI` prefix, different ConfigSources (only 4, not 12)
- **Impact**: CLI entry point ignores the config package until fallback. No unified config.
- **Effort**: ~8h

#### GAP-003: Database — SQLite Only
- **Spec**: PostgreSQL, MySQL, AND SQLite with dialect abstraction
- **Current**: pkg/repository/sql/queries.go defines DriverSQLite/DriverPostgreSQL/DriverMySQL constants, but only sqlite.go has an actual connection implementation. No postgres.go or mysql.go connection files.
- **Impact**: Spec promises multi-DB. Only SQLite delivered.
- **Effort**: ~20h

### 🟠 HIGH

#### GAP-004: Authentication Pipeline — Not Integrated
- **Issue**: pkg/auth/ has comprehensive primitives (OAuth2 578 lines, mTLS 206 lines, keychain, crypto, revocation, fingerprint), but they are NOT wired into the HTTP client request path.
- **Missing**: No `Authenticator` interface, no auth middleware in the request builder, no automatic credential application in generated command RunE handlers
- **Impact**: Auth exists as standalone package but requires manual wiring per-command — defeats the "dynamic API interaction" promise
- **Effort**: ~12h

#### GAP-005: Asymmetric Cryptography — Partial
- **Spec**: CRYPTO-IMPLEMENTATION.md (687 lines) — Ed25519 keygen/signing, RSA-OAEP 4096, ECDH P-384, X25519, SHA3-256, BLAKE2b, key hierarchy
- **Current**: pkg/auth/crypto.go (807 lines) — primarily AES-256-GCM for keychain encryption. Missing all asymmetric operations.
- **Impact**: Compliance gap for advanced security requirements
- **Effort**: ~15h

#### GAP-006: Runtime Sandbox (Kernel-Level) — Not Implemented
- **Spec**: RUNTIME-SANDBOX.md (453 lines) — cgroups v2, Linux namespaces, seccomp filters, process-level isolation
- **Current**: WASM-SANDBOX (pkg/wasm/sandbox.go, 412 lines) provides WASM-level sandboxing, but NO kernel-level containment
- **Impact**: WASM sandbox works at application layer, but no defense-in-depth via kernel containment
- **Effort**: ~25h

---

## Part 3: Medium Gaps

#### GAP-007: MCP LLM Sampling — Not Implemented
- **Spec**: MCP-MARKET-SAMPLING.md (243 lines) — CreateMessageRequest, SamplingMessage, ModelPreferences
- **Current**: No sampling handler exists. All other MCP handlers (tools, prompts, resources, roots) implemented.
- **Impact**: One missing MCP feature. Needed for Claude Desktop integration.
- **Effort**: ~8h

#### GAP-008: Unified Auth State — Not Implemented
- **Spec**: UNIFIED-AUTH.md (332 lines) — Shared auth state between CLI and MCP, AuthManager, CredentialStore, AuthSyncer
- **Current**: CLI and MCP manage auth independently. No shared state.
- **Effort**: ~10h

#### GAP-009: Unified Command Interface — Not Implemented
- **Spec**: UNIFIED-COMMANDS.md (296 lines) — Command interface with Execute(ctx, CommandInput), Registry
- **Current**: CLI uses Cobra, MCP uses tool handlers. No shared command abstraction.
- **Effort**: ~10h

#### GAP-010: Database Connection Pool — Partial
- **Spec**: DATABASE-CONNECTION.md (2867 lines) — PoolConfig (25/5/30s), TransactionManager, HealthChecker, read replicas, retry
- **Current**: Basic sql.DB wrapping. No connection pool management, health checks, or replica support.
- **Effort**: ~10h

#### GAP-011: Daemon Improvements
- **Missing**: Log rotation, correct socket path (uses /tmp/ vs spec's ~/.local/share/.../), SIGHUP config reload is signal-only (no actual file re-reading)
- **Effort**: ~6h

#### GAP-012: WASM Integration into CLI/DSL
- **Issue**: WASM runtime works standalone but is not wired as an alternative execution backend for generated commands or DSL scripts
- **Effort**: ~8h

#### GAP-013: Rate Limiter Integration
- **Issue**: Token bucket exists (pkg/auth/ratelimit/) but not wired into HTTP client request path
- **Effort**: ~4h

#### GAP-014: Performance Benchmark Suite — Incomplete
- **Current**: 3 benchmark files (request, cache, auth)
- **Spec**: BENCHMARK-SUITE.md (901 lines) — 42 benchmarks, load testing, systematic regression
- **Effort**: ~8h

#### GAP-015: WebSocket Webhook Protocol — Missing
- **Current**: HTTP webhook client only
- **Spec**: API-WEBHOOK-PROTOCOL.md (693 lines) — WebSocket streaming, reconnection, heartbeat
- **Effort**: ~10h

---

## Part 4: Low Priority / Nice-to-Have

- GAP-016: CLI-FLAG-PARSING.md (702 lines) — comprehensive type-aware validators beyond basic BindPFlag
- GAP-017: CONCURRENT-MAP-SPEC.md (708 lines) — specialized concurrent map (Go sync.Map used as substitute)
- GAP-018: UNIFIED-STATE.md — shared CLI/MCP state tracking
- GAP-019: Full package manager support (Homebrew, npm, PyPI, Scoop — only Debian exists)
- GAP-020: Enterprise operations (alerting, full CI/CD, k8s manifests)

---

## Part 5: What Works (Validated)

| Area | Status | Notes |
|------|--------|-------|
| OpenAPI Parsing | ✅ Complete | kin-openapi, ExtractOperations() |
| Command Generation | ✅ Complete | cobra commands, 4 naming strategies, 4 op styles |
| CLI Buit-in Commands | ✅ Exceeds | 11 commands vs 7 spec-required |
| MCP Server | ✅ Complete | Tools, prompts, resources, roots, lifecycle |
| MCP OpenAPIExecutor | ✅ Complete | NOT a stub — full Execute() in openapi_integration.go |
| WASM Runtime | ✅ Complete | wasmtime-go, sandbox, capabilities, syscall filter |
| Starlark DSL | ✅ Complete | 18 builtins, security hardening |
| Daemon Proxy | ✅ Complete | Unix socket, proxy, token auth, health checks |
| Caching | ✅ Complete | Memory + Redis + Tiered |
| Rate Limiting | ✅ Complete | Token bucket (standalone, not wired) |
| Telemetry | ✅ Complete | OpenTelemetry (13 files) |
| Audit Logging | ✅ Complete | 14 files, SQLite storage, credential redaction |
| Events System | ✅ Complete | Handler registry w/ circuit breaker |
| Shell Completion | ✅ Complete | Bash, zsh, fish, PowerShell |
| Interactive Prompts | ✅ Complete | Input, Confirm, Select, MultiSelect |
| Output Formatting | ✅ Complete | JSON, YAML, Table, CSV, Raw, Template |
| Platform Support | ✅ Complete | Linux, macOS, Windows (paths, services) |
| Revocation | ✅ Complete | CRL-like, store, tests |
| Fingerprinting | ✅ Complete | Client fingerprint + IP detection |
| Webhook (HTTP) | ✅ Complete | HTTP webhook client |
| Alias System | ✅ Complete | 9 files in internal/alias/ |
| Multi-tenancy | ✅ Complete | pkg/tenant/ context propagation |
| Validation Suite | ✅ Complete | 5-component external validation, 16/16 tests pass |

---

## Summary Statistics

- **Total functional requirement areas**: 19
- **Fully working**: 13 (68%)
- **Partially working**: 4 (21%) — Auth, Config, DB, Daemon
- **Not working**: 1 (5%) — Plugin System
- **Not in scope**: 1 (5%) — SDK Generation (correctly omitted)

- **Remaining effort (core gaps only)**: ~100-120 hours
- **Remaining effort (all gaps)**: ~150-180 hours
