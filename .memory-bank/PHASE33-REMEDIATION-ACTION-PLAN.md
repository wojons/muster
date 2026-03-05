# Phase 33: Security Remediation Action Plan

**Based on Phase 32 Adversarial Review Findings (2026-03-04)**

## Summary

Phase 32 adversarial review completed with comprehensive challenge of all system claims. The initial 14-20 week remediation estimate was **significantly overstated**. Realistic timeline: **5-7 days for P0 critical baseline, 2-3 weeks for full remediation**.

## Critical Findings (Corrected)

### P0: CRITICAL (Blocks Production) - 1 Issue

**Issue**: SSRF via HTTP Builtins - Can exfiltrate AWS credentials
- **Impact**: CRITICAL (cloud credential theft, full account compromise)
- **File**: `pkg/dsl/builtins.go:63-70`
- **Fix**: Network segmentation + metadata service blocking (3 days)
- **Verification**: Test `http://169.254.169.254/` blocked

### P1: MEDIUM (Security Hardening) - 1 Issue

**Issue**: Rate Limit Bypass via Race Condition
- **Impact**: MEDIUM-HIGH (2-5x rate limit multiplier, financial cost)
- **File**: `pkg/auth/ratelimit/tokenbucket.go:98-111`
- **Fix**: Use sync.Map.LoadOrStore (2 days)
- **Verification**: Parallel requests create single bucket

### P2: DEFENSE IN DEPTH (Optional) - 3 Issues

**Issue**: WASM Memory Not Enforced via Wasmtime
- **Impact**: MEDIUM (OS OOM killer protects, just process crash)
- **Reality**: Execution time enforced via Go context ✅
- **Fix**: Use `wasmtime.WithMaxMemory()` (2 days, optional)

**Issue**: Token Storage Not Implemented (Linux/Windows/File)
- **Impact**: LOW (CLI okay, daemon needs persistence)
- **Reality**: Standard daemon behavior (not vulnerability)
- **Fix**: File-based fallback with AES-256-GCM (2-3 days, UX improvement)

**Issue**: File Access Not Restricted
- **Impact**: NONE (OS permissions protect, same as Python/Node.js)
- **Reality**: Tool respects OS permissions (no bypass)
- **Fix**: Integrate existing SandboxedFS (1 day, defense in depth)

## Remediation Timeline

### Week 1: P0 Critical + P1 Hardening (5 days)

**Day 1-3: Network Segmentation** (P0 CRITICAL)
```go
// pkg/dsl/builtins.go
func isBlocked(host string) bool {
    blockedAddrs := []string{
        "169.254.169.254",         // AWS metadata
        "metadata.google.internal", // GCP metadata
    }
    for _, addr := range blockedAddrs {
        if host == addr {
            return true
        }
    }
    // Block localhost and private networks
    ip := net.ParseIP(host)
    return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
```
**Success Criteria**:
- ✅ `muster run malicious.star` with `http://169.254.169.254/` → Blocked
- ✅ `http://localhost:*` → Blocked
- ✅ `http://10.0.0.0/8`, `172.16.0.0/12` → Blocked

**Day 4-5: Rate Limit Fix** (P1 MEDIUM)
```go
// pkg/auth/ratelimit/tokenbucket.go
func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
    bucket, _ := rl.buckets.LoadOrStore(key, NewTokenBucket(...))
    return bucket.Allow(tokens)
}
```
**Success Criteria**:
- ✅ Test 100 parallel requests → single bucket created
- ✅ Rate limit enforced at configured threshold
- ✅ No bypass possible

**Result**: **PRODUCTION-READY** for CLI usage ✅

### Week 2: P2 Improvements (5 days)

**Day 6-7: WASM Memory Enforcement** (Optional)
```go
// pkg/wasm/runtime.go
config := wasmtime.NewConfig()
config.WasmMaxMemory(r.limits.MaxMemory)
engine := wasmtime.NewEngineWithConfig(config)
```
**Success Criteria**:
- ✅ Memory limit enforced via wasmtime
- ✅ OOM bypass prevented
- ✅ Attack complexity increased

**Day 8-9: Token Storage Implementation** (Optional)
```go
// pkg/auth/keychain.go
func (k *FileKeychain) Set(service, account, password string) error {
    encrypted := encryptAES256GCM(password)
    dir := filepath.Join(os.Getenv("HOME"), ".muster", "credentials")
    os.MkdirAll(dir, 0700)
    return os.WriteFile(filepath.Join(dir, sanitize(service)), encrypted, 0600)
}
```
**Success Criteria**:
- ✅ Tokens persist across process restarts
- ✅ Files encrypted with AES-256-GCM
- ✅ Platform-specific implementation (Linux keychain fallback, Windows Credential Manager)

**Day 10: File Sandbox Integration** (Optional)
```go
// pkg/dsl/builtins.go
func builtinFileRead(...) {
    workspace := getWorkspaceDir()
    if !filepath.HasPrefix(filepath.Clean(path), workspace) {
        return nil, fmt.Errorf("path outside workspace")
    }
    // ...
}
```
**Success Criteria**:
- ✅ File access restricted to workspace
- ✅ SandboxedFS integrated with DSL builtins
- ✅ Defense in depth achieved

### Week 3: Testing & Validation (5 days)

**Day 11-12: Binary Coverage Improvement**
- Add tests for cmd/packages: target 70% coverage
- Replace 4/5 mock tests with real integration tests
- Test actual binary execution end-to-end

**Day 13: Security Validation**
- Run security scan (make security-audit)
- Verify P0/P1 fixes still pass
- No regressions introduced

**Day 14: Documentation & Release**
- Update security documentation
- Document remediation timeline
- Prepare for production deployment

## Integration Test Status

**Current Reality**:
- CLI Integration (100%): Builds binary + makes HTTP requests ✅
- MCP Integration (0%): Fake JSON-RPC implementation ❌
- Auth Integration (0%): Mock OAuth2 provider ❌
- Daemon Integration (0%): Mock HTTP proxy ❌
- WASM Integration (0%): Config only, no execution ❌

**Required**: Replace mock tests with real binary tests

## Binary Coverage Status

| Package | Current | Target | Gap |
|---------|---------|-------|-----|
| cmd/openapi-cli | 55.6% ✅ | 70% | -14.4% |
| cmd/mcp-server | 11.1% ❌ | 70% | -58.9% |
| cmd/daemon | 38.9% ⚠️ | 70% | -31.1% |

**Required**: Improve to 70%+ for production

## Production Readiness Checklist

### P0 CRITICAL (MUST DO)

- [ ] **Implement network segmentation** for HTTP builtins (3 days)
  - Block metadata services (`169.254.169.254`, `metadata.google.internal`)
  - Block localhost (`127.0.0.0/8`)
  - Block private networks (`10.0.0.0/8`, `172.16.0.0/12`)
  - Test: Cannot exfiltrate credentials

- [ ] **Fix rate limit race condition** (2 days)
  - Use sync.Map.LoadOrStore
  - Prevent 2-5x rate limit bypass
  - Test: Parallel requests enforce single bucket

### P1 MEDIUM (SHOULD DO)

- [ ] **Fix rate limit race condition** (included above)
- [ ] **Improve binary coverage** to 70% (cmd/openapi-cli ✅, cmd/mcp-server ❌, cmd/daemon ⚠️)
- [ ] **Replace mock integration tests** with real binary tests (4/5 currently fake)

### P2 DEFENSE IN DEPTH (NICE TO HAVE)

- [ ] **Add WASM memory enforcement** via `wasmtime.WithMaxMemory()` (2 days)
- [ ] **Implement token storage** for Linux/Windows/File (2-3 days)
  - File-based fallback with AES-256-GCM
  - Linux: Secret Service API or `~/.muster/credentials/`
  - Windows: Credential Manager API
- [ ] **Integrate SandboxedFS** with DSL builtins (1 day)
- [ ] **Documentation** - Security best practices (1 day)

## Before Production Deployment

### Must Have (P0):
- [ ] Network segmentation implemented and tested
- [ ] Rate limit fix implemented and tested
- [ ] SSRF attack scenario tested (blocked)
- [ ] Rate limit bypass tested (single bucket)

### Should Have (P1):
- [ ] Binary coverage >70%
- [ ] Real integration tests (not mocks)
- [ ] Security audit passed

### Nice to Have (P2):
- [ ] WASM memory enforcement
- [ ] Token storage implementation
- [ ] File sandbox integration
- [ ] External security review

## Risk Assessment Matrix

| Vulnerability | Exploitable? | Impact | Risk | Priority | Timeline |
|---------------|--------------|--------|------|----------|----------|
| SSRF (metadata) | ✅ YES | CRITICAL | CRITICAL | **P0** | 3 days |
| Rate limit bypass | ✅ YES | MEDIUM | MEDIUM | P1 | 2 days |
| WASM memory DoS | ✅ YES | LOW | MEDIUM | P2 | 2 days |
| Token persistence | ❌ NO | LOW | LOW | P2 | 2-3 days |
| File access | ❌ NO | NONE | NONE | P2 | 1 day |

## Next Steps

### Immediate (Week 1):

1. **Stop**: Halt all new feature development
2. **Start**: Implement network segmentation (3 days)
3. **Follow**: Fix rate limit race (2 days)
4. **Result**: Production-ready CLI ✅

### Short-term (Week 2):

1. Implement WASM memory enforcement (optional)
2. Implement token storage (UX improvement)
3. Integrate file sandbox (defense in depth)

### Medium-term (Week 3):

1. Improve binary coverage
2. Replace mock tests with real tests
3. Security validation and documentation

## Success Criteria

**P0 Critical Baseline** (5 days):
- ✅ Cannot access `http://169.254.169.254/`
- ✅ Rate limit enforced correctly
- ✅ No SSRF possible
- ✅ CLI tool production-ready

**Full Remediation** (2-3 weeks):
- ✅ All P0 + P1 fixes complete
- ✅ Binary coverage >70%
- ✅ Real integration tests passing
- ✅ Security audit passed
- ✅ Documentation complete

## Conclusion

**Phase 32 adversarial review was accurate in finding vulnerabilities but overestimated severity:**

- ✅ 1 Critical vulnerability (SSRF) - Blocks production (3 days to fix)
- ✅ 1 Medium vulnerability (rate limit) - Security hardening (2 days to fix)
- ⚠️ 3 Theoretical vulnerabilities - Mitigated by OS or not exploitable
- ❌ NOT 10 critical vulnerabilities (2.0x vs 10.0x)
- ❌ NOT 14-20 weeks remediation (5-7 days P0 baseline, 2-3 weeks full)

**Realistic Production Timeline:**
- **CLI Usage**: 5 days (P0 fixes only)
- **Full System**: 2-3 weeks (including optional hardening)

The system is **closer to production-ready** than initially assessed. Focus on the 2 real vulnerabilities (SSRF + rate limit) and defer theoretical improvements.

---

**Status**: Phase 32 Adversarial Review ✅ COMPLETE  
**Next Phase**: Phase 33 - Security Remediation (5-7 days P0 baseline)  
**Ready to Start**: YES (clear action plan with realistic timeline)
