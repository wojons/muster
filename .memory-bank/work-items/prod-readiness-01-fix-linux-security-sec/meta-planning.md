# Meta-Plan: Fix Linux Security (seccomp/SELinux)

axiom:trace work_item=prod-readiness-01-fix-linux-security-sec spec=specs/PLATFORM-LINUX.md,specs/LINUX-IMPLEMENTATION.md plan=.memory-bank/work-items/prod-readiness-01-fix-linux-security-sec/plan.md evidence=.memory-bank/work-items/prod-readiness-01-fix-linux-security-sec/verification.md

**Status**: Planning  
**Created**: 2026-05-28  
**Mode**: implement_feature  
**Verification Bar**: High  
**Parent**: `prod-readiness-01`

## 1) Intent

Fix the **genuine remaining gaps** in Linux security enforcement — seccomp BPF filtering correctness and SELinux context management — identified during the March 2026 dogfooding and confirmed by fresh audit (2026-05-28). The majority of seccomp and SELinux code is *implemented* but has specific correctness defects, missing edge-case handling, and untested integration paths.

### In Scope
- **Seccomp (4 known defects)**:
  - `LimitFileAccess` ignores its `allowedPaths` parameter — never iterates it, always denies all file syscalls
  - `ApplyToProcess` hardcoded to return "not supported" — document or implement ptrace-based apply
  - `lastFailedSyscall` / `lastFailedArch` are **never written** — `GetFailedSyscall` always returns "no failed syscall"
  - `toSyscallName` only maps 4 syscall numbers (0, 1, 2, 4) — expand to cover common Linux syscalls (x86_64 table)
- **SELinux (3 known defects)**:
  - `NewSELinux` checks `/selinux` path (deprecated since kernel 2.6.30) — should check `/sys/fs/selinux` or use `getenforce`
  - `GetFileContext` parses `ls -Z` output which is fragile and locale-dependent — improve parsing or use `stat`/`getfattr`
  - No integration test proves `SetContext` actually works on an enforcing system
- **Linux SecureStorage/Keychain**: `LinuxKeychain` is a complete no-op stub (all methods return nil/empty)
- **Integration tests**: Existing tests lack runtime verification (seccomp filter loading, SELinux context setting)
- **Spec alignment**: Code is at `pkg/platform/` but specs (`PLATFORM-LINUX.md`) describe `internal/platform/linux/` structure

### Out of Scope
- Windows/MacOS platform security changes
- Daemon lifecycle tests (covered by parent `prod-readiness-01`)
- Audit logging subsystem (covered by parent `prod-readiness-01`)
- New spec creation beyond inline documentation
- 12-language SDK ecosystem features
- Production deployment infrastructure

## 2) Contract Reconciliation

**Jira Reference**: N/A — non-Jira work item  
**Acceptance Criteria**:
1. `LimitFileAccess` correctly restricts to `allowedPaths` (not blanket deny)
2. `ApplyToProcess` either implemented (via ptrace) or explicitly documented as unsupported
3. `lastFailedSyscall` / `lastFailedArch` recording wired into `AllowSyscall`/`DenySyscall` failures
4. `toSyscallName` expanded to cover at least x86_64 common syscalls (Linux syscall table v5.10+)
5. `NewSELinux` uses modern SELinux detection (`/sys/fs/selinux` or `getenforce`)
6. `GetFileContext` parsing robust against locale/format variations
7. `LinuxKeychain` implements Secret Service D-Bus for secure credential storage
8. All integration tests pass on a Linux system with appropriate capabilities
9. At least one end-to-end test proves seccomp filter actually blocks a forbidden syscall
10. Go vet/lint/tests all pass

**Contracts/Specs Touched**:
- `specs/PLATFORM-LINUX.md` — Linux platform spec (2919 lines)
- `specs/LINUX-IMPLEMENTATION.md` — Linux implementation spec (85 lines)
- `specs/SECURITY-AUDIT-CHECKLIST.md` — Security audit requirements
- `specs/SECURITY-TESTING.md` — Security testing requirements

**Conflicts Discovered**:
- Code is in `pkg/platform/` package; spec `PLATFORM-LINUX.md` describes `internal/platform/linux/` package. Resolution: code location works for the Go toolchain; document divergence in inline comments.
- `NewSELinux` uses `/selinux` path that has been deprecated since kernel 2.6.30. Resolution: update to modern detection.

## 3) Decision Points

| Decision | Default | What Changes With Other Choice |
|---|---|---|
| Seccomp `ApplyToProcess`: implement ptrace or document unsupported? | Document as "not supported — use Apply() on same process" | ptrace impl = high complexity, security risk; not worth it for CLI tool |
| Seccomp `LimitFileAccess`: path-prefix allow or full path allow? | Path-prefix allow (matches dir) | Full path = more secure but requires exact path |
| SELinux detection: check `/sys/fs/selinux` or `getenforce`? | Both: try `getenforce` first, fall back to stat | Only one = misses edge cases |
| `LinuxKeychain`: use libsecret D-Bus or exec `secret-tool`? | Use D-Bus directly (same pattern as NetworkManager) | `secret-tool` = simpler but depends on GNOME; D-Bus = broader |
| `toSyscallName`: manual table or turn it into a debug-only feature? | Manual table (20 most common syscalls) | Feature removal = simpler but loses diagnostic |

## 4) Risks and Blast Radius

| Risk | Severity | Mitigation |
|---|---|---|
| Seccomp filter changes could break process on deny-by-default profiles | High | All changes additive (adding allowed syscalls); test with `CheckSeccompSupport()` |
| SELinux detection change from `/selinux` to `/sys/fs/selinux` breaks on older systems | Medium | Fall back to old path if `/sys/fs/selinux` doesn't exist |
| D-Bus Secret Service implementation could hang if service not running | Medium | Add timeout context; fall back to FileKeychain |
| Integration tests require root/capabilities for seccomp filter loading | Medium | Pre-check `CheckSeccompSupport()` and skip if unavailable |
| Changes to `pkg/platform/` affect all platform consumers | Low | All changes are within Linux-only build-tagged files |

## 5) Verification Design

### Required Verifiers
- `@qa-axiom` — verify all tests pass
- `@spec-verifier-axiom` — verify alignment with PLATFORM-LINUX.md spec
- `@trace-auditor-axiom` — verify trace markers present in all changed files
- `@security-review-axiom` — verify seccomp/SELinux security posture

### Required Checks (pre-execution)
- [x] meta-planning.md written
- [ ] plan.md written
- [ ] plan.yaml written
- [ ] At least one `axiom:trace work_item=prod-readiness-01-fix-linux-security-sec ...` line present

### 5a) Test Value Assessment

| Test | Spec-to-Test Mapping | Real Code Path | Deletion Test |
|---|---|---|---|
| TestSeccompFilter_LimitFileAccess | PLATFORM-LINUX.md §4.3 file restrictions | `LimitFileAccess()` → `DenySyscall()` for file ops | No proof path restrictions work |
| TestSeccompFilter_LastFailedSyscall | PLATFORM-LINUX.md §6 seccomp diagnostics | `recordFailedSyscall()` → `GetFailedSyscall()` | No diagnostic feedback on failures |
| TestSELinux_ModernDetection | PLATFORM-LINUX.md §6.2 SELinux | `NewSELinux()` → `/sys/fs/selinux` stat | Old `/selinux` path still used |
| TestLinuxKeychain_SecretService | LINUX-IMPLEMENTATION.md §2 | `LinuxSecretService.Store/Get/Delete` via D-Bus | No Linux keychain at all |
| TestSeccomp_E2E_RuntimeBlocks | SECURITY-TESTING.md | `Apply()` → `fork()` → test blocked syscall ↔ `VerifySyscall` | No proof filter actually blocks |

**Anti-pattern check**: All tests exercise real code paths (not mocks), have negative test cases, and prove behavioral outcomes.

## 6) Ambiguity Assessment

| Assumption | How to Verify | Impact if Wrong |
|---|---|---|
| A1: libseccomp library installed on CI/build system | Check `pkg-config --libs libseccomp` | Medium — seccomp tests skip; need Docker build image |
| A2: D-Bus session bus available on test system | `dbus-send --session --print-reply ...` | Medium — Secret Service tests skip; test with mock |
| A3: SELinux enforcing system available for integration test | `getenforce` returns `Enforcing` | Low — integration test skips gracefully |
| A4: Existing tests still pass after changes | `go test -v -count=1 ./pkg/platform/...` | High — regression indicates breaking change |

**Ambiguity Rating**: Low — code exists, defects are well-understood, scope is targeted.

**Missing Inputs**: None.

<!-- meta-plan-type: standard -->
