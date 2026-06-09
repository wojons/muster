# Meta-Plan: Audit Logging Implementation (audit-logging-01)

axiom:trace work_item=audit-logging-01 spec=specs/AUDIT-LOGGING.md,specs/EVENTS-AUDIT.md plan=.memory-bank/work-items/audit-logging-01/plan.md impl=internal/audit/ test=internal/audit/ evidence=.memory-bank/work-items/audit-logging-01/verification.md

**Status**: Not started  
**Created**: 2026-05-28  
**Mode**: implement_feature  
**Verification Bar**: Standard  

## 1) Intent

Complete the audit logging subsystem by wiring the existing `audit.Hook` into CLI and MCP entry points, and adding database-backed audit storage via the existing migration.

### Background (code audit findings)

The audit package (`internal/audit/`) is already substantially complete, contrary to the initial "no logging implementation" assumption. Audit code audit found 12 source files:

| File | Status | Purpose |
|---|---|---|
| `types.go` | ✅ Complete (230 lines) | AuditEvent, AuditActor, AuditResource, AuditResult, AuditContext, AuditChange, AuditLevel, AuditCategory |
| `config.go` | ✅ Complete (114 lines) | AuditConfig with Validate(), DefaultAuditConfig |
| `logger.go` | ✅ Complete (211 lines) | AuditLogger with Log(), LogAsync(), Flush(), Close(), hash chain |
| `redaction.go` | ✅ Complete (212 lines) | Redactor with recursive JSON path redaction, MandatoryRedactionRules |
| `storage.go` | ✅ Complete (538 lines) | MemoryStorage, FileStorage with rotation, Query |
| `hooks.go` | ✅ Complete (202 lines) | Hook struct with LogCommand, LogToolCall, LogAuth, Close |
| `config_test.go` | ✅ Exists | Config validation tests |
| `types_test.go` | ✅ Exists | Type validation tests |
| `logger_test.go` | ✅ Exists | Logger unit tests |
| `logger_integration_test.go` | ✅ Exists | Integration test |
| `storage_test.go` | ✅ Exists | Storage tests |
| `redaction_test.go` | ✅ Exists | Redaction tests |

**Remaining gaps:**
1. **Hook not wired into CLI entry points** — `cmd/` entry points (CLI, MCP server, daemon) don't create or use the `audit.Hook`. The `Hook` struct is ready with `LogCommand()`, `LogToolCall()`, `LogAuth()` methods.
2. **Database-backed audit storage** — Migration `00005_audit.sql` creates the `audit` table, but there's no `DatabaseStorage` implementation of `AuditStorage`. Only `MemoryStorage` and `FileStorage` exist.
3. **Verify all spec requirements are met** — Cross-reference `specs/AUDIT-LOGGING.md` and `specs/EVENTS-AUDIT.md` against existing implementation.

### In Scope
- Wire `audit.Hook` into CLI entry point(s) — create hook on startup, call `LogCommand` on command execution
- Wire `audit.Hook` into MCP server entry point — call `LogToolCall` on tool invocations
- Implement `DatabaseStorage` (SQLite-backed) using the existing `00005_audit.sql` schema
- Run existing audit tests to confirm they pass
- Verify all spec requirements are covered

### Out of Scope
- New audit event types beyond what specs define
- Audit log viewer/query CLI commands
- S3-backed storage (listed in config but not spec-required)
- Log encryption at rest (encryption key field exists but is optional)
- Distributed audit log aggregation

## 2) Contract Reconciliation

**Jira Reference**: N/A — non-Jira work item  
**Acceptance Criteria**:
1. `audit.Hook` is created and used in CLI command execution path
2. `audit.Hook` is created and used in MCP server tool invocation path
3. `DatabaseStorage` (SQLite-backed) implements `AuditStorage` interface
4. All existing `internal/audit/...` tests pass
5. Audit events include credential redaction (already implemented)
6. Spec requirements from `AUDIT-LOGGING.md` and `EVENTS-AUDIT.md` are satisfied

**Contracts/Specs Touched**:
- `specs/AUDIT-LOGGING.md` — audit event types, logger structure, log rotation
- `specs/EVENTS-AUDIT.md` — audit config, credential redaction, event types, hash chain
- `migrations/00005_audit.sql` — existing SQLite schema for audit table
- `internal/audit/` — all existing source files

**Conflicts Discovered**: 
- `specs/AUDIT-LOGGING.md` defines a simpler `AuditEvent` with flat `User`/`Action`/`Resource`/`Result` strings, while the actual implementation uses structured `AuditActor`/`AuditResource`/`AuditResult` types. The implementation is more detailed — no conflict, the spec is a simplified view.
- `specs/EVENTS-AUDIT.md` uses `context` struct for storing metadata; the implementation is consistent with this.

## 3) Decision Points

| Decision | Default | What Changes With Other Choice |
|---|---|---|
| DatabaseStorage — use raw SQL or repository pattern? | Raw SQL with `database/sql` (matches migration) | Repository = more testable but adds abstraction |
| Hook initialization — global singleton or per-command? | Per-command (create in `main()` and pass) | Singleton = simpler but less testable |
| Audit enablement default | Off by default (`MUSTER_AUDIT_ENABLED=1` to enable) | On by default = more secure but adds overhead |
| DatabaseStorage: synchronous or buffered writes? | Buffered (matches existing FileStorage pattern) | Sync = stronger durability but higher latency |

## 4) Risks and Blast Radius

| Risk | Severity | Mitigation |
|---|---|---|
| CLI startup delay from audit init if DB unavailable | Low | Fail gracefully — log warning, disable audit |
| MCP server audit logging adds latency to tool calls | Low | Buffered writes; configurable buffer/flush |
| DatabaseStorage migration mismatch | Medium | Verify against `00005_audit.sql` schema |
| Audit package tests already pass — wiring doesn't break them | Low | `go test -v ./internal/audit/...` after each change |

## 5) Verification Design

### Required Verifiers
- `@qa-axiom` — run `go test -v ./internal/audit/...` and confirm all pass
- `@spec-verifier-axiom` — verify spec alignment
- `@trace-auditor-axiom` — verify trace completeness

### 5a) Test Value Assessment

| What spec requirement does each test verify? | Real code path exercised? | What breaks if deleted? |
|---|---|---|
| `logger_test.go` — AuditLogger.Log() creates events with correct fields | `AuditLogger.Log()` → redaction → hash chain → buffer | No unit-level logger validation |
| `logger_integration_test.go` — AuditLogger writes to storage end-to-end | Full path: `Log()` → buffer → `Flush()` → `FileStorage.Write()` | Integration regression undetected |
| `config_test.go` — AuditConfig.Validate() rejects bad configs | `AuditConfig.Validate()` | Bad configs accepted silently |
| `storage_test.go` — FileStorage.Write/Query/Rotate work correctly | `FileStorage` write → rotate → query chain | Storage errors undetected |
| `redaction_test.go` — Redactor correctly redacts sensitive fields | `Redactor.RedactEvent()` → JSON marshal → path traversal → unmarshal | Credential leak regression |
| `types_test.go` — AuditEvent.Validate() rejects invalid events | `AuditEvent.Validate()` | Invalid events accepted |

**Anti-pattern check**: No fake executors, no raw HTTP, no coverage padding, no weak assertions.

### 5b) Working Backwards Assessment

**User's destination**: An operator runs a CLI command like `muster inventory fetch` or makes an MCP tool call like `read_inventory`, and a tamper-resistant audit event with credential redaction is recorded to SQLite or file storage.

**Walking skeleton**: CLI startup → `audit.NewHook(audit.DefaultConfigEnabled())` → execute command → `hook.LogCommand()` → event buffered → flushed to `FileStorage` → verify via log file.

## 6) Ambiguity Assessment

| Assumption | How to Verify | Impact if Wrong |
|---|---|---|
| A1: Existing audit tests pass | `go test -v ./internal/audit/...` | Low — gap is wiring, not implementation |
| A2: CLI and MCP entry points are identifiable | Check `cmd/` directory structure | Low — standard Cobra commands |
| A3: DatabaseStorage should match migration exactly | Read `00005_audit.sql` columns | Medium — schema mismatch breaks queries |
| A4: Audit is opt-in via env var | Check `DefaultConfig()` uses `MUSTER_AUDIT_ENABLED` | Low — aligns with expected pattern |

**Ambiguity Rating**: Low — the core implementation exists, gap is wiring.

**Missing Inputs**: None.
