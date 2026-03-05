# Active Context - Updated 2026-03-05 (Phase 32 Adversarial Review Complete)

## Current Focus

**Phase**: Phase 32 Adversarial Review ✅ COMPLETE
**Status**: 🔴 NOT PRODUCTION-READY - 6 vulnerabilities require remediation (1 CRITICAL, 2 HIGH, 3 MEDIUM)
**Production-Ready**: ❌ NO - Requires 2-3 days critical fix work + 1 week external audit
**Next Action**: Implement Week 1 critical fixes (WASM memory limits, symlink bypass, network policy, rate limiting)
**Remediation Progress**: 2/8 vulnerabilities FIXED ✅ (Authentication Key Derivation, DNS Rebinding), 6/8 vulnerabilities REMAINING ❌

## Critical Findings (2026-03-05 - VERIFIED FINDINGS)

### ✅ FIXED Critical Vulnerabilities (2/9):

1. **Authentication - Weak Key Derivation** 🔴 CRITICAL → ✅ FIXED
   - **Issue**: Key derived from public information (service + account, NO salt)
   - **Fix Applied**: HMAC-based key derivation with 100,000 iterations and master key
   - **Status**: ✅ FIXED - Now requires secret master key (OS keychain or file-based)

2. **Network Security - DNS Rebinding/SSRF** 🔴 CRITICAL → ✅ FIXED
   - **Issue**: validateNetworkURL() blocks IP literals but NOT domain names
   - **Fix Applied**: Added DNS resolution with timeout, validates resolved IPs
   - **Status**: ✅ FIXED - Blocks private networks, metadata service, IPv6 addresses

### ⚠️ REMAINING Vulnerabilities (7 remaining):

### 🔴 P0 CRITICAL Vulnerabilities (REMAINING - NEED REMEDIATION):

1. **WASM Sandbox - Missing Memory Limits** 🔴 CRITICAL → ⏳ NOT YET FIXED
    - wasmtime.NewEngine() created WITHOUT memory constraints
    - No MaxMemory enforcement, no WithFuel (CPU limits)
    - Location: pkg/wasm/runtime.go:65-70 (Execute function)
    - Impact: Host system memory exhaustion, Denial-of-Service
    - **Fix Required**: Configure engine with WithMaxMemory, WithFuel, WithMaxFileDescriptors
    - **Est: 6 hours**

### 🔶 P1 HIGH Vulnerabilities (REMAINING - NEED REMEDIATION):

1. **File Access - Symlink Bypass** 🔶 HIGH → ⏳ NOT YET FIXED
    - validateFilePath() doesn't call filepath.EvalSymlinks
    - Symlinks can escape workspace sandbox
    - Location: pkg/dsl/builtins.go:518-544 (validateFilePath)
    - Impact: Arbitrary file read/write via symlink, workspace escape
    - **Fix Required**: Add filepath.EvalSymlinks() call before path validation
    - **Est: 4 hours**

2. **WASM Sandbox - Network Policy Missing** 🔶 HIGH → ⏳ NOT YET FIXED
    - NetworkPolicy defined but never integrated into runtime
    - WASM modules have unrestricted network access (if network capability granted)
    - Location: pkg/wasm/types.go:69-107 (NetworkPolicy struct)
    - Impact: Unrestricted network access, SSRF possible
    - **Fix Required**: Integrate NetworkPolicy validation into Runtime.Execute
    - **Est: 6 hours**

### ⚠️ P2 MEDIUM Vulnerabilities (REMAINING - NEED REMEDIATION):

1. **Rate Limiting - Negative Token Bypass** ⚠️ MEDIUM → ⏳ NOT YET FIXED
    - No validation that token parameter is positive
    - Negative tokens ADD to bucket instead of subtract
    - Location: pkg/auth/ratelimit/tokenbucket.go:26-45 (Allow function)
    - Impact: Complete bypass of rate limiting, resource exhaustion
    - **Fix Required**: Validate tokens > 0 before processing
    - **Est: 1 hour**

2. **File Access - System Files Not Blocked** ⚠️ MEDIUM → ⏳ NOT YET FIXED
    - validateFilePath has no explicit system file blocklist
    - If workspace is root, all files accessible
    - Location: pkg/dsl/builtins.go:518-546
    - Impact: Sensitive file access if workspace misconfigured
    - **Fix Required**: Add blocklist of restricted paths (/etc/, /root/, ~/.ssh/)
    - **Est: 2 hours**

3. **File Access - Workspace Can Be Root** ⚠️ MEDIUM → ⏳ NOT YET FIXED
    - defaultWorkspaceDir can be current working directory
    - No enforcement that workspace is not root
    - Location: pkg/dsl/builtins.go:524-527
    - Impact: Sandbox bypass if run from root
    - **Fix Required**: Validate workspace cannot be root directory
    - **Est: 1 hour**

4. **WASM Sandbox - File Descriptor Limits** ⚠️ MEDIUM → ⏳ NOT YET FIXED
    - MaxFileDescriptors defined but never enforced
    - No call to wasmtime.WithMaxFileDescriptors
    - Location: pkg/wasm/types.go:36-42 (SandboxLimits)
    - Impact: File descriptor exhaustion, DoS
    - **Fix Required**: Configure engine with WithMaxFileDescriptors
    - **Est: 2 hours**

### ⚠️ P3 LOW Vulnerabilities (VERIFIED - NEED REMEDIATION):

1. **Network Security - IPv6 Private Networks** ⚠️ LOW
    - IPv6 localhost (::1) and private networks (fd00::/8) not blocked
    - Location: pkg/dsl/builtins.go:561-599
    - Impact: IPv6 SSRF possible
    - Fix: Add IPv6 CIDRs to blockedNetworks

2. **Authentication - Error Handling** ⚠️ LOW
    - crypto/rand.Read() returns error that's ignored
    - Location: pkg/auth/oauth2.go:52, oauth2.go:58
    - Impact: Low potential for weak randomness if system RNG fails
    - Fix: Check error and propagate or panic

### ✏️ Remediation Progress (UPDATED):

**Week 1 Status (CRITICAL Fixes):**
- ✅ Authentication Key Derivation - FIXED (HMAC + master key)
- ✅ DNS Rebuilding/SSRF - FIXED (DNS resolution + validation)
- ⏳ WASM Memory Limits - NOT STARTED (6 hours remaining)
- ⏳ Symlink Bypass - NOT STARTED (4 hours remaining)
- ⏳ Rate Limit Negative Token - NOT STARTED (1 hour remaining)
- ⏳ System File Blocking - NOT STARTED (2 hours remaining)
- ⏳ Workspace Root Check - NOT STARTED (1 hour remaining)
- ⏳ WASM Network Policy - NOT STARTED (6 hours remaining)
- ⏳ File Descriptor Limits - NOT STARTED (2 hours remaining)

**Total Estimated Work Remaining:** ~22 hours (~3 days for critical/high fixes)

**Verified Security Posture Improvement:**
- Before: 9 vulnerabilities (3 CRITICAL, 2 HIGH, 4 MEDIUM)
- After: 7 vulnerabilities (1 CRITICAL, 3 HIGH, 4 MEDIUM)
- Net Change: 2 CRITICAL vulnerabilities fixed

### 📋 Integration Test Status (UPDATED):

## Critical Findings (2026-03-03)

### Integration Test Reality Check (Week 1 Remediation)
- **Semi-Real (20%)**: 2/10 tests build binaries + make HTTP requests ✅
- **Fake/Mock (50%)**: 5/10 tests manually implement services in test code ⚠️ (documented)
- **Unit Only (10%)**: 1/10 tests validate config but don't execute (WASM) ⚠️
- **Silent Failures**: Already fixed or misidentified (no current issues) ✅

### Binary Coverage Crisis (Week 1 Priority 1) ✅
- **cmd/openapi-cli: 0.0% → 55.6%** ✅ SIGNIFICANT PROGRESS
- cmd/mcp-server: 11.1% (unchanged)
- cmd/daemon: 38.9% (unchanged)
- **Average: 35.2%** (was 16.7%, improved by 118%)
- Target: 70%+ coverage (in progress)

### Security Issues (Week 1 Fixes) ✅
- **1 CRITICAL:** Path traversal vulnerability FIXED ✅ (key sanitization added)
- **1 CRITICAL:** WASM file permissions FIXED ✅ (0644 → 0600, 23 instances)
- **1 HIGH:** Unix socket permissions ✅ (0600 already enforced)
- **1 HIGH:** WASM timeout test ✅ (type comparison corrected)
- **1 MEDIUM:** Keychain stubs (Linux/Windows not implemented) ⏸️ (Week 2)
- OAuth2 implementation secure (uses crypto/rand ✅)

### Production Readiness (VERIFIED - UPDATED):
- Previous Claim: "All P0 CRITICAL vulnerabilities were FALSE" ❌ INCORRECT
- Adversarial Review Assessment: 3 CRITICAL, 2 HIGH, 4 MEDIUM vulnerabilities ✅ VERIFIED
- CORRECTED Assessment: System NOT production-ready until CRITICAL vulnerabilities fixed
- Actual Status: 9 security vulnerabilities require remediation
- Remaining: Week 1 critical fixes (DNS SSRF, WASM limits, auth key derivation)

### Verified Remediation Timeline:
- **COMPLETED (2026-03-05)**: Adversarial review by Devils-Advocate, RedTeam, Trust-But-Verify
- **WEEK 1 PENDING**: Critical vulnerability fixes (5 days estimated)
  - Fix DNS rebinding SSRF (8 hours)
  - Add WASM memory limits (6 hours)
  - Strengthen auth key derivation (8 hours)
  - Fix symlink bypass (4 hours)
  - Fix negative token bypass (1 hour)
  - Add system file blocking (2 hours)
  - Enforce workspace not root (1 hour)
- **WEEK 2 PENDING**: External security audit and penetration testing
- **ESTIMATED TOTAL**: 2-3 days for critical fixes + 1 week external validation
- Status: NOT PRODUCTION-READY until all CRITICAL vulnerabilities fixed

### Binary Coverage Status (VERIFIED via go test -cover ./cmd/...)
- cmd/openapi-cli: 55.6% ✅ (IMPROVED from 0%, 14.4% gap to 70% target)
- cmd/mcp-server: 11.1% ❌ (58.9% gap to 70% target)
- cmd/daemon: 38.9% ⚠️ (31.1% gap to 70% target)
- Average: 35.2% ❌ (34.8% gap to 70% target)
- Status: NOT PRODUCTION-READY (major gaps remain)

## Immediate Action Items (Phase 32 Adversarial Review)

1. **HALT ALL NEW FEATURE DEVELOPMENT** 🔴 CRITICAL
    - System fundamentally unsafe for production
    - Focus 100% on security remediation
    - Assign dedicated security engineering resources

2. **IMPLEMENT TOKEN STORAGE** 🔴 CRITICAL (Week 1-2 Priority 1)
    - Linux: Use libsecret or ~/.local/share/muster/credentials
    - Windows: Use Windows Credential Manager API
    - File-based fallback: Encrypt with AES-256-GCM
    - Status: NOT STARTED - 50% of platforms broken

3. **ENFORCE WASM SANDBOX LIMITS** 🔴 CRITICAL (Week 1-2 Priority 2)
    - Use wasmtime.WithMaxMemory()
    - Use wasmtime.WithFuel() (execution time)
    - Enforce MaxFileDescriptors
    - Status: NOT STARTED - Limits defined but not enforced

4. **RESTRICT FILE SYSTEM ACCESS** 🔴 CRITICAL (Week 1 Priority 3)
    - Restrict DSL builtins to workspace directory
    - Validate all file paths
    - Implement proper path traversal protection
    - Status: NOT STARTED - Arbitrary access enabled

5. **ADD NETWORK SEGMENTATION** 🔴 CRITICAL (Week 2 Priority 1)
    - Block localhost, private networks, metadata service
    - Implement URL allowlist
    - Add DNS pinning
    - Status: NOT STARTED - SSRF possible

**Previous State (2026-03-03 16:00)**:
- cmd/openapi-cli: 0.0% ❌
- cmd/mcp-server: 0.0% ❌
- cmd/daemon: 0.0% ❌
- **Average: 0%**

**Current State (2026-03-04)**:
- cmd/openapi-cli: 55.6% ✅ (11 new tests added - good progress)
- cmd/mcp-server: 11.1% ❌ (NO progress, 58.9% gap to target)
- cmd/daemon: 38.9% ⚠️ (above start but below target)
- **Average: 35.2%** (50% of Week 1 target met)

**Impact**: Binary coverage significantly improved - cmd/openapi-cli entry points now tested

## Next Steps (Phase 32 Remediation - 14-20 Weeks)

**Weeks 1-2: P0 CRITICAL SHOWSTOPPERS** 🔴 BLOCKS PRODUCTION:
1. Implement token storage for Linux/Windows/MacOS (showstopper bug)
2. Enforce WASM sandbox limits (memory, execution time, file descriptors)
3. Restrict DSL file system access to workspace directory
4. Add network segmentation with URL allowlisting

**Weeks 3-4: P0 CRITICAL (Continued)**:
5. Add circuit breakers everywhere (prevent cascade failures)
6. Conduct fuzz testing for WASM enforcement
7. Add proper panic recovery for goroutines

**Weeks 5-8: P1 HIGH**:
8. Fix rate limit race condition bypass
9. Implement credential hashing (SHA256)
10. Replace mock integration tests with real binary tests
11. Improve binary coverage to 70%+

**Weeks 9-12: P2 MEDIUM**:
12. Add deadlock detection
13. Fix cache consistency
14. Add comprehensive integration tests
15. Validate all security fixes with external audit

**Weeks 13-14: P3 VALIDATION**:
16. External security audit
17. Penetration testing
18. Load testing
19. Documentation updates

- Week 2: Real integration test implementation
- Week 3: Binary coverage improvement
- Week 4: End-to-end workflow validation
- Week 5: Security hardening and audit
- Week 6: Documentation accuracy (stop inflating coverage claims)

## Documentation Updates Required

- progress.md: ✅ Updated with critical vulnerability findings
- activeContext.md: ✅ Updated with realistic remediation timeline
- phase-32-adversarial-review.md: ✅ Comprehensive review with 17 new vulnerabilities
- Phase descriptions: Remove "100% Complete" and "Production Ready" claims
- Progress bar: Replace percentages with actual vulnerability counts

## Success Criteria (Before Production)

[ ] cmd/mcp-server coverage >55% (currently 11.1%, 58.9% gap)
[ ] cmd/daemon coverage >55% (currently 38.9%, 16.1% gap)
[ ] cmd/openapi-cli has REAL functionality tests (not just help/version)
[ ] Binary coverage average >70% (currently 35.2%, 34.8% gap)
[ ] At least 2/4 mock tests replaced with real integration

✅ 10/10 integration tests are real (not mocks)
✅ cmd/* coverage > 70%
✅ 0 HIGH/CRITICAL vulnerabilities
✅ End-to-end workflow validated
✅ Demo proves actual system works

## Resources

- Adversarial review: .memory-bank/phase-32-adversarial-review.md
- Integration tests: demo/test-suites/*.go
- Binary status: go test -cover ./cmd/...

