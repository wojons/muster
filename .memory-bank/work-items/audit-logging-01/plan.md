# Plan: Audit Logging Implementation

**Work Item**: audit-logging-01  
**Status**: Not started  
**Goal**: Wire audit logging into CLI/MCP entry points, add database-backed storage.

## Mission

The `internal/audit/` package is functionally complete (AuditLogger, Redactor, FileStorage, Hook with LogCommand/LogToolCall/LogAuth). The remaining work is: (1) create `DatabaseStorage` backed by the existing SQLite migration, (2) wire the Hook into CLI and MCP entry points so audit events are actually emitted when commands run.

## AC → Verification Mapping

| Acceptance Criteria | Verification |
|---|---|
| AC1: `audit.Hook` wired into CLI command path | Code review — `cmd/` creates Hook, calls `LogCommand` |
| AC2: `audit.Hook` wired into MCP server tool path | Code review — MCP creates Hook, calls `LogToolCall` |
| AC3: `DatabaseStorage` implements `AuditStorage` | `go build ./internal/audit/` |
| AC4: All existing `internal/audit/...` tests pass | `go test -v ./internal/audit/...` |
| AC5: Credential redaction active | Verified by existing redaction tests |
| AC6: Spec requirements satisfied | Cross-reference AUDIT-LOGGING.md and EVENTS-AUDIT.md |

## Phases

### Phase 1: Implement DatabaseStorage
**Status**: Not started

- **Step 1.1**: Check migration schema (`00005_audit.sql`) and define `DatabaseStorage` struct  
  *Verification*: Schema columns: id, action, entity_type, entity_id, user_id, data, ip_address, user_agent, created_at  
  *Evidence path*: `migrations/00005_audit.sql`

- **Step 1.2**: Implement `DatabaseStorage` with Write/Query/Close methods matching `AuditStorage` interface  
  *Verification*: `go build ./internal/audit/` succeeds  
  *Rollback*: Comment out DatabaseStorage, keep FileStorage

- **Step 1.3**: Add unit tests for DatabaseStorage  
  *Verification*: `go test -v -run DatabaseStorage ./internal/audit/` passes  
  *Evidence path*: test output

### Phase 2: Wire Hook into CLI entry points
**Status**: Not started

- **Step 2.1**: Identify CLI entry point and add audit Hook creation on startup  
  *Verification*: Hook is created in `cmd/` root command or `main()`  
  *Spec ref*: `internal/audit/hooks.go`

- **Step 2.2**: Call `hook.LogCommand()` in command execution path  
  *Verification*: Command execution triggers `hook.LogCommand()` with command name, actor, success status  
  *Rollback*: Remove hook wiring, keep standalone audit package

- **Step 2.3**: Call `hook.Close()` on shutdown to flush pending events  
  *Verification*: Deferred `hook.Close()` in root command

### Phase 3: Wire Hook into MCP entry points
**Status**: Not started

- **Step 3.1**: Add Hook creation in MCP server startup  
  *Verification*: MCP `main()` or server setup creates Hook  
  *Spec ref*: `internal/audit/hooks.go#LogToolCall`

- **Step 3.2**: Call `hook.LogToolCall()` on tool invocations  
  *Verification*: Each tool handler calls `hook.LogToolCall()` with tool name, actor ID, success status  
  *Rollback*: Remove MCP hook wiring

### Phase 4: Verification
**Status**: Not started

- **Step 4.1**: Run all audit tests  
  *Verification*: `go test -v -count=1 ./internal/audit/...` all pass  
  *Evidence path*: logged output

- **Step 4.2**: Cross-reference spec requirements  
  *Verification*: Checklist of `AUDIT-LOGGING.md` and `EVENTS-AUDIT.md` requirements  
  *Evidence path*: verification.md

- **Step 4.3**: Add trace markers  
  *Verification*: `axiom:trace` lines in new/updated files  
  *Evidence path*: grep output
