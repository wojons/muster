# Ralph Wiggum Loop - Task Queue

## 🔄 PENDING IMPLEMENTATION TASKS

**Start here. Execute in order. Mark [x] when complete.**

### Phase 1: Production Blockers (CRITICAL)
- [x] **IMPL-001: Credential Redaction** - ✅ COMPLETED as part of AUDIT-001. Redactor implemented in pkg/audit/logger.go with mandatory credential redaction (password, token, api_key, secret, credit_card). Spec: specs/EVENTS-AUDIT.md. Impact: CRITICAL security compliance.
- [ ] **IMPL-002: HTTP Retry with Backoff** - Create pkg/retry/ package with Retrier, exponential backoff, jitter. Spec: specs/RELIABILITY-RETRY.md. Impact: HIGH production reliability.

### Phase 2: High Priority Features
- [ ] **IMPL-003: CLI Flag Parsing** - Create internal/cli/flags/ with typed validation (URL/IP/email). Spec: specs/CLI-FLAG-PARSING.md.
- [ ] **IMPL-004: Command Aliases** - Create internal/alias/ with Manager, Expander, YAML persistence. Spec: specs/CLI-MARKET-ALIASES.md.
- [ ] **IMPL-005: Event Handler Registry** - Create pkg/events/ with HandlerRegistry + middleware. Spec: specs/EVENTS-HANDLERS.md.
- [ ] **IMPL-006: Daemon Config Hot Reload** - Create internal/daemon/config/ with Manager, FileWatcher, ReloadCh. Spec: specs/DAEMON-CONFIGURATION.md.

### Phase 3: Medium Priority Features
- [ ] **IMPL-007: MCP Roots Protocol** - Implement RootManager with roots/list handler. Spec: specs/MCP-MARKET-ROOTS.md.
- [ ] **IMPL-008: Filesystem Utilities** - Create pkg/fsutil/ with EnsureDir, LockFile, etc. Spec: specs/OS-FILESYSTEM.md.
- [ ] **IMPL-009: HTTP Canonicalization** - Implement CanonicalizeURL, CanonicalizeHeaders. Spec: specs/HTTP-CANONICALIZATION.md.
- [ ] **IMPL-010: Viper Advanced** - Add BindPFlag, WatchConfig, SetProfile. Spec: specs/VIPER-INTEGRATION.md.

### Phase 4: Platform Security (Optional)
- [ ] **IMPL-011: Platform Security Stubs** - Complete or document seccomp/selinux/networkmanager stubs. Spec: specs/PLATFORM-LINUX.md.

---

## Audit Status (2026-03-22)

**Completeness**: 78% (Integrated Level)
**Data Flows**: 7/7 Documented

---

## P0: BLOCKING PRODUCTION

- [x] **AUDIT-001: Implement AuditLogger** (COMPLETED)
  - ✅ Create pkg/audit/logger.go with AuditLogger struct
  - ✅ Schema exists (migrations/00005_audit.sql), integrated
  - ✅ Add credential redaction (password, token, api_key, secret)
  - ✅ File: pkg/audit/logger.go (new)
  - ✅ Impact: CRITICAL (security compliance)

## P1: HIGH PRIORITY

- [ ] **AUDIT-002: Add Distributed Tracing** (3-4 days)
  - internal/telemetry/ directory is empty
  - Add OpenTelemetry integration
  - Traces for MCP, WASM, HTTP client
  - File: internal/telemetry/tracer.go (new)
  - Impact: HIGH (observability)

- [ ] **AUDIT-003: Create Event Handler Registry** (2-3 days)
  - Create pkg/events/ package
  - HandlerRegistry + middleware pattern
  - File: pkg/events/registry.go (new)
  - Impact: HIGH (extensibility)

## P2: MEDIUM PRIORITY

- [ ] **AUDIT-004: Complete Platform Security Stubs** (4-6 hours)
  - pkg/platform/seccomp.go - 14 TODO stubs
  - pkg/platform/selinux.go - 5 TODO stubs
  - pkg/platform/networkmanager.go - 5 TODO stubs
  - Either complete or document as deferred
  - Impact: MEDIUM (Linux hardening)

- [ ] **AUDIT-005: Complete Package Validation** (2-3 hours)
  - packaging/validation.go - 22 TODO stubs
  - Complete or simplify implementation
  - Impact: MEDIUM (deployment)

## IMPLEMENTATION TASKS (From Gap Analysis)

- [x] **IMPL-001: Credential Redaction** (COMPLETED)
  - ✅ Redactor struct implemented in pkg/audit/logger.go
  - ✅ Mandatory redaction patterns: password, token, api_key, secret, credit_card
  - ✅ Integrated into AuditLogger for automatic sanitization

- [ ] **IMPL-002: HTTP Retry with Backoff** (2-3 days)
  - Create pkg/retry/ package
  - Retrier with exponential backoff + jitter
  - Effort: 2-3 days | Impact: HIGH (production reliability)

- [ ] **IMPL-003: CLI Flag Parsing** (3-4 days)
  - Create internal/cli/flags/ package
  - Typed validation (URL/IP/email)

- [ ] **IMPL-004: Command Aliases** (2-3 days)
  - Create internal/alias/ package
  - Manager + Expander + YAML persistence

- [ ] **IMPL-005: Event Handlers** (2-3 days)
  - Create pkg/events/ package
  - HandlerRegistry + middleware

- [ ] **IMPL-006: Daemon Config Hot Reload** (2-3 days)
  - Create internal/daemon/config/
  - Manager + FileWatcher + ReloadCh

- [ ] **IMPL-007: MCP Roots Protocol** (1-2 days)
  - Implement MCP roots protocol extension

- [ ] **IMPL-008: Filesystem Utilities** (1-2 days)
  - Common filesystem operations

- [ ] **IMPL-009: HTTP Canonicalization** (4-6 hours)
  - URL canonicalization utilities

- [ ] **IMPL-010: Viper Advanced** (1-2 days)
  - Advanced Viper configuration patterns

---

## EXECUTION ORDER

**Phase 1: Production Blockers (P0)**
1. AUDIT-001: AuditLogger

**Phase 2: Observability (P1)**
2. AUDIT-002: Distributed Tracing
3. AUDIT-003: Event Handler Registry

**Phase 3: Quality (P2)**
4. AUDIT-004: Platform Security
5. AUDIT-005: Package Validation

**Phase 4: Implementation**
6. IMPL-001 through IMPL-010

---

## Total Effort: 20-30 days
