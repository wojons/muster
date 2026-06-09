# Plan: Fix Linux Security (seccomp/SELinux)

**Parent**: `prod-readiness-01`  
**Created**: 2026-05-28  
**Cursor**: phase-1 / task-1-1 / step-1-1-1

> **Mission**: Make seccomp BPF filtering correct, SELinux detection modern, and Linux keychain functional — proving each with runtime integration tests — without breaking existing functionality.

## AC → Verification Mapping

| AC | Description | Verification | Phase |
|---|---|---|---|
| AC-1 | `LimitFileAccess` restricts to `allowedPaths` | `TestSeccompFilter_LimitFileAccess_RespectsPaths` | P1 |
| AC-2 | `ApplyToProcess` documented as unsupported | Code comment + doc | P1 |
| AC-3 | `lastFailedSyscall` recording wired in | `TestSeccomp_RecordFailedSyscall` | P1 |
| AC-4 | `toSyscallName` covers 20+ syscalls | Code review + unit test | P1 |
| AC-5 | `NewSELinux` uses modern detection | `TestSELinux_ModernDetection` | P2 |
| AC-6 | `GetFileContext` parsing robust | `TestSELinux_GetFileContext_Robust` | P2 |
| AC-7 | `LinuxKeychain` uses Secret Service D-Bus | `TestLinuxKeychain_SecretService` | P3 |
| AC-8 | All integration tests pass | `make test-integration` | P4 |
| AC-9 | E2E test proves seccomp blocks forbidden syscall | `TestSeccomp_E2E_RuntimeBlocks` | P4 |
| AC-10 | `go vet` and `make lint` pass | CI checks | P4 |

## Phases

### Phase 1: Seccomp Defect Fixes (4 tasks, 8 steps)

**Task 1.1 — Fix `LimitFileAccess` allowedPaths handling**
- Step 1.1.1: Audit `LimitFileAccess` — confirm `allowedPaths` is declared but never read
- Step 1.1.2: Rewrite `LimitFileAccess` to iterate `allowedPaths`, building a path-prefix allowlist via seccomp arg filter (or use path-based deny/allow pattern via BPF)
- Step 1.1.3: Add `TestSeccompFilter_LimitFileAccess_RespectsPaths` — verifies files outside allowed paths are blocked
- *Verification*: `go test -v -run TestSeccompFilter_LimitFileAccess ./pkg/platform/`

**Task 1.2 — `ApplyToProcess` disposition**
- Step 1.2.1: Replace hardcoded failure message with explicit documentation comment explaining why ptrace apply is unsupported
- Step 1.2.2: Add a doc note linking to alternative: use `Apply()` on the target process if it's a child
- *Verification*: Code review + doc comment

**Task 1.3 — Wire `lastFailedSyscall` / `lastFailedArch` recording**
- Step 1.3.1: Add `recordFailedSyscall(num uintptr, arch uintptr)` that writes to `lastFailedSyscall`/`lastFailedArch` under `failedMutex`
- Step 1.3.2: Call `recordFailedSyscall` from `AllowSyscall`/`DenySyscall` when filter creation/rule addition fails
- Step 1.3.3: Add `TestSeccomp_RecordFailedSyscall` unit test
- *Verification*: `go test -v -run TestSeccomp_RecordFailedSyscall ./pkg/platform/`

**Task 1.4 — Expand `toSyscallName`**
- Step 1.4.1: Expand mapping to cover 20+ common x86_64 syscalls (read, write, open, close, stat, mmap, brk, execve, clone, fork, socket, connect, accept, bind, listen, ioctl, fcntl, epoll, poll, exit)
- Step 1.4.2: Add fallback `fmt.Sprintf("syscall_%d", num)` for unmapped numbers
- Step 1.4.3: Verify `toSyscallName` covers the syscalls used in profiles
- *Verification*: Code review + existing `TestSeccompFilter_SyscallNumberConversion`

### Phase 2: SELinux Fixes (2 tasks, 4 steps)

**Task 2.1 — Modernize SELinux detection**
- Step 2.1.1: Replace `/selinux` stat check with: (1) `getenforce` command, (2) fallback to `/sys/fs/selinux` stat, (3) final fallback to `/selinux`
- Step 2.1.2: Add `TestSELinux_ModernDetection` unit test
- *Verification*: `go test -v -run TestSELinux_ModernDetection ./pkg/platform/`

**Task 2.2 — Robust `GetFileContext` parsing**
- Step 2.2.1: Replace `ls -Z` parsing with `stat --printf='%C'` (more portable, avoids locale issues) or `getfattr -d -m security.selinux`
- Step 2.2.2: Add `TestSELinux_GetFileContext_Robust` with locale-invariant capture
- *Verification*: `go test -v -run TestSELinux_GetFileContext_Robust ./pkg/platform/`

### Phase 3: Linux Keychain (SecureStorage) (1 task, 3 steps)

**Task 3.1 — Implement `LinuxKeychain` via Secret Service D-Bus**
- Step 3.1.1: Implement `Store()` — open D-Bus session to `org.freedesktop.secrets`, create item with marshalled credential, store in "muster" collection (see pattern from NetworkManager's D-Bus usage)
- Step 3.1.2: Implement `Retrieve()` / `Delete()` / `List()` — search items by attributes, return/serialize credential data
- Step 3.1.3: Add `TestLinuxKeychain_SecretService` unit + integration test (skip if D-Bus unavailable)
- *Verification*: `go test -v -run TestLinuxKeychain ./pkg/platform/`

### Phase 4: Integration & Validation (2 tasks, 4 steps)

**Task 4.1 — Integration tests and E2E proof**
- Step 4.1.1: Add `TestSeccomp_E2E_RuntimeBlocks` — create filter, apply, attempt blocked syscall, verify `ENOSYS`
- Step 4.1.2: Add integration test for SELinux context round-trip on enforcing system
- *Verification*: `make test-integration`

**Task 4.2 — Full validation pass**
- Step 4.2.1: Run `go test -v -count=1 ./pkg/platform/...` and confirm all tests pass
- Step 4.2.2: Run `make lint`, `make vet`, and `go build ./...`
- *Verification*: All CI checks green

## Rollback Notes
- Each file change is a single, reversible edit
- If seccomp filter behavior breaks, revert specific method rather than entire file
- LinuxKeychain: FileKeychain fallback preserves backward compatibility

## Evidence Paths
- `.memory-bank/work-items/prod-readiness-01-fix-linux-security-sec/verification.md` (rolling)
- `.memory-bank/work-items/prod-readiness-01-fix-linux-security-sec/runs/<RUN_ID>/verification.md` (immutable snapshots)
