# Meta-Plan: Muster Production Readiness

axiom:trace work_item=prod-readiness-01 spec=specs/00-PRD.md plan=.memory-bank/work-items/prod-readiness-01/plan.md doc=.memory-bank/soul.md evidence=.memory-bank/work-items/prod-readiness-01/verification.md

**Status**: Planning  
**Created**: 2026-05-28  
**Mode**: implement_feature  
**Verification Bar**: High  

## 1) Intent

Get Muster production-ready by closing the real remaining gaps in Linux security (seccomp/SELinux), daemon testing, keychain, and audit logging. The March 2026 dogfooding report claimed 100% unimplemented for several areas, but a fresh code audit (2026-05-28) found that many of those "missing" features actually exist. This work item targets the genuine remaining gaps, not the outdated report.

### In Scope
- Fix genuine seccomp gaps: `LimitFileAccess` ignores `allowedPaths`, `ApplyToProcess` hardcoded failure, `lastFailedSyscall` never written to, expand `toSyscallName` beyond 4 syscalls
- Verify SELinux implementation works end-to-end (code exists but flagged "REQUIRES: Complete implementation")
- Verify Linux Secret Service keychain works end-to-end (implementation exists but needs integration proof)
- Verify daemon tests run and pass (all tests now run unconditionally — confirm they pass)
- Verify `internal/audit/` subsystem is wired into the application (audit logging code exists but may not be called from any entry point)
- Ensure cross-platform keychain fallback behavior is correct and documented
- End-to-end demonstration that all four areas are functional

### Out of Scope
- New feature development beyond the four targeted areas
- 12-language SDK ecosystem (Enterprise SaaS scope, not Go CLI)
- Production deployment infrastructure (Kubernetes, Docker Compose expansion, package managers)
- Distributed tracing (OpenTelemetry)
- Comprehensive config management (Viper hot-reload, file watching)
- Command alias system
- New spec creation beyond what's needed to document fixes

## 2) Contract Reconciliation

**Jira Reference**: N/A — non-Jira work item  
**Acceptance Criteria** (from user request):
1. Linux security (seccomp/selinux) — real gaps fixed, not just stubs removed
2. Daemon tests — confirmed passing, no skipped tests
3. Keychain — Linux Secret Service works end-to-end, Windows fallback is clear
4. Audit logging — wired into request paths and demonstrable
5. End-to-end functionality proven

**Contracts/Specs Touched**:
- `specs/00-PRD.md` — North Star product requirements
- `specs/07-Mission-North-Star.md` — Mission & North Star (operator doctrine)
- `.memory-bank/soul.md` — Soul / hardening priorities

**Conflicts Discovered**: 
- March 2026 dogfooding report claims conflicts with current code state. The report says Linux security is 100% unimplemented — actual code has substantial implementations. Resolution: the report is stale; work from actual code state.

## 3) Decision Points

| Decision | Default | What Changes With Other Choice |
|---|---|---|
| Seccomp: fix LimitFileAccess or remove allowedPaths param? | Fix it — iterate allowedPaths in deny loop | Remove param = simpler but less secure |
| Seccomp: implement ApplyToProcess via ptrace or document as not supported? | Document as "not supported, use ApplyProfile" | ptrace implementation = complex, brittle |
| SELinux: rewrite with native C bindings or verify current shell-based approach? | Verify current approach; rewrite only if broken | Rewrite = more reliable but higher effort |
| Audit logging: wire into entry points or verify subsystem only? | Wire into at least CLI and MCP entry points | Subsystem-only = code exists but not used |
| Keychain: implement libsecret for master key or keep file-based? | Keep file-based with clear documentation | libsecret = more secure but adds dependency |

## 4) Risks and Blast Radius

| Risk | Severity | Mitigation |
|---|---|---|
| Seccomp changes could break process isolation if filter rules are wrong | High | Apply Profile-based testing; validate with seccomp-tools |
| SELinux context changes could break file access on SELinux-enabled systems | Medium | Only touch context get/set, not policy management |
| Audit logging wiring adds overhead to every request | Low | Configurable enable/disable; use buffered logger |
| Keychain changes could break existing credential storage | High | Add migration helper; preserve FileKeychain fallback |
| Daemon test changes could expose race conditions | Medium | Run with `-race` flag; keep changes minimal |

## 5) Verification Design

### Required Verifiers
- `@qa-axiom` — verify all tests pass, including daemon tests
- `@spec-verifier-axiom` — verify changes align with spec contracts
- `@trace-auditor-axiom` — verify trace completeness
- `@security-review-axiom` — verify seccomp/SELinux/keychain security posture

### 5a) Working Backwards Assessment

**User's destination**: An operator runs `muster daemon start` on Linux with SELinux enforcing and seccomp active, credentials are stored securely via Secret Service, and all operations produce audit events.

**Walking Skeleton (Phase 0)**: Build → Start daemon → Make a request → Verify audit event emitted → Verify seccomp profile applied → Stop daemon.

**Connection points to verify**:
- Daemon start → seccomp filter applied → verified via `/proc/<pid>/status`
- CLI request → audit logger → event written to storage
- Credential store → Secret Service → retrieve → use in request
- Daemon stop → graceful shutdown → audit events flushed

### 5b) Test Value Assessment

| What spec requirement does each test verify? | Real code path exercised? | What breaks if deleted? |
|---|---|---|
| Seccomp profile application blocks forbidden syscalls | `seccomp.go:Apply()` → libseccomp `filter.Load()` | No automated seccomp validation |
| SELinux context retrieval on enforcing system | `selinux.go:GetContext()` → `/proc/self/attr/current` | No SELinux integration verification |
| Secret Service keychain stores/retrieves credentials | `keychain_secret_service_linux.go:Set()/Get()` | Linux credential storage unverified |
| Audit logger emits events through CLI entry point | `audit/logger.go:Log()` → FileStorage | No proof audit trail exists |
| Daemon lifecycle: start → serve → stop → cleanup | `cmd/daemon` → Controller → proxy | Daemon regressions undetected |

**Anti-pattern check**: No fake executor bypass, no raw HTTP instead of adapter, no coverage padding files, no source inspection instead of behavioral testing, no weak assertions.

## 6) Ambiguity Assessment

| Assumption | How to Verify | Impact if Wrong |
|---|---|---|
| A1: SELinux implementation (shell-based) works on enforcing systems | Run `getenforce` and test context operations on a system with SELinux enforcing | Medium — may need C binding rewrite |
| A2: Secret Service keychain backend (`internal/platform/linux`) D-Bus service is available | Test on a system with `gnome-keyring` or `kwallet` running | High — fallback to FileKeychain but less secure |
| A3: Daemon tests pass without modification | Run `go test -v ./cmd/daemon/...` | Low — tests already run unconditionally |
| A4: `internal/audit/` can be wired without API changes | Trace audit entry points | Medium — may need to add audit params to existing functions |
| A5: Seccomp libseccomp is available on build system | Check for `libseccomp-dev` package | High — seccomp tests will skip without it |

**Ambiguity Rating**: Low — code exists for all four areas, gaps are well-understood, and the scope is verification + targeted fixes.

**Missing Inputs**: No missing inputs. All four areas have code to work from.

<!-- axiom:trace work_item=prod-readiness-01 spec=specs/00-PRD.md,specs/07-Mission-North-Star.md doc=.memory-bank/soul.md -->
