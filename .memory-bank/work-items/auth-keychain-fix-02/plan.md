# Plan: Fix Keychain Key Derivation on Linux

**Work Item**: auth-keychain-fix-02  
**Status**: Not started  
**Goal**: Wire D-Bus Secret Service into Linux master key path, replace hardcoded application secret, fix failing tests.

## Mission

Two changes to `pkg/auth/keychain.go` with one root cause: `getMasterKeyFromLinuxKeychain()` ignores the existing `SecretServiceKeyring` implementation and uses a hardcoded application secret. Fix both and make all tests pass.

## AC → Verification Mapping

| Acceptance Criteria | Verification |
|---|---|
| AC1: `getMasterKeyFromLinuxKeychain()` tries Secret Service first | Code review + unit test with mock |
| AC2: `getApplicationSecret()` returns runtime-generated value, not hardcoded | Code review + confirm value changes across test runs |
| AC3: All `pkg/auth/...` tests pass | `go test -v ./pkg/auth/...` |
| AC4: Backward compatible — stored credentials remain decryptable | Existing tests exercise round-trip Set/Get |
| AC5: Secret Service master key path works on Linux with D-Bus | `go test -run TestSecretServiceKeychain ./pkg/auth/` |

## Phases

### Phase 1: Diagnose and fix failing tests
**Status**: Not started  
**Task**: Run failing tests, diagnose "cipher: message authentication failed" root cause, apply fix

- **Step 1.1**: Run failing tests to capture exact error output  
  *Verification*: `go test -v -run 'TestWindowsKeychain_Set|TestFileKeychain_SecureMasterKeyGeneration' ./pkg/auth/ 2>&1 | tee /tmp/auth-fail-output.txt`  
  *Evidence path*: `/tmp/auth-fail-output.txt`

- **Step 1.2**: Root-cause analysis — trace encrypt/decrypt path in `keychain.go`  
  *Verification*: Identify whether hardcoded secret, PBKDF2 salt mismatch, or iteration count mismatch causes the auth tag failure  
  *Evidence path*: Documented in verification.md

- **Step 1.3**: Fix `getApplicationSecret()` → generate runtime secret on first use, store alongside master key  
  *Verification*: Code review — `getApplicationSecret()` reads from stored config, generates if absent  
  *Rollback*: Revert the function to return the hardcoded constant

- **Step 1.4**: Fix consistency in `encryptMasterKeyFile()` / `decryptMasterKeyFile()` to use the same secret  
  *Verification*: Both functions call the same `getApplicationSecret()` source  
  *Evidence path*: Tests pass

### Phase 2: Wire Secret Service into master key path
**Status**: Not started

- **Step 2.1**: Update `getMasterKeyFromLinuxKeychain()` to try D-Bus Secret Service first  
  *Verification*: Code calls `SecretServiceKeyring.Get("muster-master-key", "master-key")` before falling back to file  
  *Spec ref*: `internal/platform/linux/keyring_secret_service.go`

- **Step 2.2**: Add `setMasterKeyOnLinuxKeychain()` for storing newly generated master keys to Secret Service  
  *Verification*: On first key generation, store to both Secret Service and file fallback  
  *Rollback*: Remove the Secret Service call, keep file-only path

- **Step 2.3**: Ensure fallback path (file-based) remains functional when Secret Service unavailable  
  *Verification*: Mock D-Bus failure → confirm file fallback works  
  *Evidence path*: Integration test output

### Phase 3: Verification
**Status**: Not started

- **Step 3.1**: Run full `pkg/auth/...` test suite  
  *Verification*: `go test -v -count=1 ./pkg/auth/...` all pass  
  *Evidence path*: logged output

- **Step 3.2**: Add trace markers to changed function boundaries  
  *Verification*: grep for `axiom:trace` in modified files  
  *Evidence path*: grep output

- **Step 3.3**: Verify backward compatibility with old-format keys  
  *Verification*: `go test -v -run TestFileKeychain ./pkg/auth/` passes  
  *Evidence path*: test output
