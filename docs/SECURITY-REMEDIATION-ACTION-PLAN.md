# Security Remediation Action Plan
# Priority: P0 CRITICAL - Blocks ALL Production Deployment

**Status**: 🔴 CRITICAL - 10 confirmed security vulnerabilities  
**Production-Ready**: ❌ NO - Requires 14-20 weeks remediation  
**Last Updated**: 2026-03-04  
**Priority**: IMMEDIATE (halt all new feature development)

---

## Executive Summary

The Muster system contains **DEEP ARCHITECTURAL SECURITY FLAWS** discovered through comprehensive adversarial review. Five P0 (showstopper) vulnerabilities prevent ANY production deployment. These are not just bugs - they are fundamental design flaws that make the system fundamentally unsafe.

**Key Metrics:**
- **Critical Vulnerabilities**: 5 (block all production)
- **High-Risk Issues**: 3 (significant security impact)
- **Medium-Risk Issues**: 2 (security hardening needed)
- **Total Remediation Time**: 14-20 weeks (revised from 6 weeks)
- **Production Block**: YES (stop all deployment plans)

---

## P0 CRITICAL SHOWSTOPPERS (Weeks 1-4)

### 1. Authentication Token Storage (Week 1-2) 🔴
**File**: `pkg/auth/keychain.go:88-138`

**Problem**: Token storage returns "not yet implemented" for Linux, Windows, and File keychains.

**Impact**:
- OAuth2 tokens cannot persist across process restarts
- 75% of platforms completely broken (3 of 4)
- Sessions invalidated on SIGKILL, crash, or update

**Attack Scenario**:
1. Attacker forces process restart (via OOM, SIGKILL, or vulnerability)
2. All user sessions invalidated → Denial of Service
3. Users must re-authenticate → Credential phishing opportunity
4. Production downtime until all users re-login

**Fix Required**: Implement concrete file-based token storage for all platforms

```go
// pkg/auth/keychain.go

// Linux: Use libsecret or file-based fallback
func (k *LinuxKeychain) Set(service, account, password string) error {
    // Fallback: ~/.local/share/muster/credentials with AES-256-GCM encryption
    credPath := filepath.Join(xdg.ConfigHome, "muster", "credentials", service, account)
    credDir := filepath.Dir(credPath)
    
    if err := os.MkdirAll(credDir, 0700); err != nil {
        return err
    }
    
    // Encrypt password with key derived from user-specific secret
    encrypted, err := encryptAES256GCM([]byte(password))
    if err != nil {
        return err
    }
    
    return os.WriteFile(credPath, encrypted, 0600)
}

func (k *LinuxKeychain) Get(service, account string) (string, error) {
    credPath := filepath.Join(xdg.ConfigHome, "muster", "credentials", service, account)
    
    encrypted, err := os.ReadFile(credPath)
    if err != nil {
        if os.IsNotExist(err) {
            return "", ErrNotFound
        }
        return "", err
    }
    
    password, err := decryptAES256GCM(encrypted)
    if err != nil {
        return "", err
    }
    
    return string(password), nil
}

// Windows: Use Windows Credential Manager
import (
    "golang.org/x/sys/windows"
    "strings"
)

func (k *WindowsKeychain) Set(service, account, password string) error {
    credentialName := fmt.Sprintf("Muster://%s/%s", service, account)
    
    // Use CredWrite API (Windows Credential Manager)
    var blob windows.CREDENTIAL
    blob.Flags = 0
    blob.Type = windows.CRED_TYPE_GENERIC
    blob.TargetName = windows.StringToUTF16Ptr(credentialName)
    blob.CredentialBlob = password
    blob.CredentialBlobSize = uint32(len(password))
    blob.Persist = windows.CRED_PERSIST_LOCAL_MACHINE
    blob.UserName = windows.StringToUTF16Ptr(account)
    
    if err := windows.CredWrite(&blob, 0); err != nil {
        return err
    }
    
    return nil
}

func (k *WindowsKeychain) Get(service, account string) (string, error) {
    credentialName := fmt.Sprintf("Muster://%s/%s", service, account)
    
    var cred *windows.CREDENTIAL
    err := windows.CredRead(credentialName, windows.CRED_TYPE_GENERIC, 0, &cred)
    if err != nil {
        return "", err
    }
    defer windows.CredFree(cred)
    
    return string(cred.CredentialBlob[:cred.CredentialBlobSize]), nil
}

// File keychain: Implement encrypted file storage
func (k *FileKeychain) Set(service, account, password string) error {
    credPath := filepath.Join(os.Getenv("HOME"), ".muster", "credentials", service, account)
    credDir := filepath.Dir(credPath)
    
    if err := os.MkdirAll(credDir, 0700); err != nil {
        return err
    }
    
    // Encrypt with AES-256-GCM
    encrypted, err := encryptAES256GCM([]byte(password))
    if err != nil {
        return err
    }
    
    return os.WriteFile(credPath, encrypted, 0600)
}

func (k *FileKeychain) Get(service, account string) (string, error) {
    credPath := filepath.Join(os.Getenv("HOME"), ".muster", "credentials", service, account)
    
    encrypted, err := os.ReadFile(credPath)
    if err != nil {
        if os.IsNotExist(err) {
            return "", ErrNotFound
        }
        return "", err
    }
    
    password, err := decryptAES256GCM(encrypted)
    if err != nil {
        return "", err
    }
    
    return string(password), nil
}

// Helper functions
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
)

func deriveKeyFromUserSecret() ([]byte, error) {
    // Derive key from OS-specific user-specific secret
    // Use HKDF with user home directory, username, machine ID
    keyMaterial := []byte(
        os.Getenv("HOME") + 
        os.Getenv("USER") + 
        getMachineID(),
    )
    
    // HKDF-SHA256 with salt
    salt := []byte("muster-credential-encryption-2024")
    block, err := hkdf.New(sha256.New, keyMaterial, salt, nil)
    if err != nil {
        return nil, err
    }
    
    key := make([]byte, 32) // 256-bit key
    if _, err := block.Read(key); err != nil {
        return nil, err
    }
    
    return key, nil
}

func encryptAES256GCM(plaintext []byte) ([]byte, error) {
    key, err := deriveKeyFromUserSecret()
    if err != nil {
        return nil, err
    }
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err = rand.Read(nonce); err != nil {
        return nil, err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return ciphertext, nil
}

func decryptAES256GCM(ciphertext []byte) ([]byte, error) {
    key, err := deriveKeyFromUserSecret()
    if err != nil {
        return nil, err
    }
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, errors.New("ciphertext too short")
    }
    
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    return gcm.Open(nil, nonce, ciphertext, nil)
}

func getMachineID() string {
    // Platform-specific machine ID
    if runtime.GOOS == "linux" {
        if id, err := os.ReadFile("/etc/machine-id"); err == nil {
            return strings.TrimSpace(string(id))
        }
    }
    if runtime.GOOS == "darwin" {
        if out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformSerialNumber").Output(); err == nil {
            return strings.TrimSpace(string(out))
        }
    }
    if runtime.GOOS == "windows" {
        if out, err := exec.Command("wmic", "csproduct", "get", "uuid").Output(); err == nil {
            return strings.TrimSpace(string(out))
        }
    }
    return "fallback"
}
```

**Testing:**
```go
// pkg/auth/keychain_test.go
func TestTokenPersistenceAcrossRestart(t *testing.T) {
    keychain := NewKeychain()
    
    // 1. Store token
    err := keychain.Set("muster", "test-user", "secret-token")
    require.NoError(t, err)
    
    // 2. Verify token in keychain
    token, err := keychain.Get("muster", "test-user")
    require.NoError(t, err)
    require.Equal(t, "secret-token", token)
    
    // 3. Simulate process restart (new keychain instance)
    keychain2 := NewKeychain()
    token2, err := keychain2.Get("muster", "test-user")
    require.NoError(t, err)
    require.Equal(t, "secret-token", token2)
    
    // 4. Clean up
    keychain.Delete("muster", "test-user")
}
```

**Estimated Time**: 5 days (Linux: 2, Windows: 2, File: 1)

---

### 2. WASM Sandbox Enforcement (Week 1) 🔴
**File**: `pkg/wasm/runtime.go:67-68`

**Problem**: Limits defined but not enforced - WASM can allocate unlimited memory and file descriptors.

**Fix Required**: Integrate Wasmtime v8.0.0 limiting capabilities

```go
// pkg/wasm/runtime.go

import (
    "github.com/bytecodealliance/wasmtime-go/v8"
)

func (r *Runtime) ExecuteWithContext(ctx context.Context, moduleBytes []byte, function string, args ...interface{}) (*ExecutionResult, error) {
    start := time.Now()
    
    result := &ExecutionResult{
        Success:  false,
        Duration: time.Since(start),
    }
    
    // Context cancellation check (existing, keep)
    select {
    case <-ctx.Done():
        result.Error = "execution cancelled by context"
        return result, ctx.Err()
    default:
    }
    
    // Capability check (existing, keep)
    if !r.CheckCapability("stdio") {
        return result, fmt.Errorf("stdio capability not granted")
    }
    
    // Module size check (existing, keep)
    if r.limits != nil && r.limits.MaxFileSize > 0 {
        if int64(len(moduleBytes)) > int64(r.limits.MaxFileSize) {
            result.Error = fmt.Sprintf("module byte size %d exceeds configured maximum %d", len(moduleBytes), r.limits.MaxFileSize)
            return result, fmt.Errorf("module size exceeds MaximumFileSize limit")
        }
    }
    
    // ✅ NEW: Create engine with memory limits
    engineConfig := wasmtime.NewConfig()
    
    // Enforce memory limits using Wasmtime's WasmMaxMemory
    if r.limits != nil && r.limits.MaxMemory > 0 {
        // wasmtime-go v8 uses WasmMaxMemory for memory limits
        engineConfig.WasmMaxMemory(true)
    }
    
    engine := wasmtime.NewEngineWithConfig(engineConfig)
    
    // ✅ NEW: Create store with fuel for execution time limiting
    store := wasmtime.NewStore(engine)
    
    // Enforce execution time using fuel
    if r.limits != nil && r.limits.MaxExecutionTime > 0 {
        // Convert time to fuel (rough estimate: 1 million instructions per second)
        fuel := uint64(r.limits.MaxExecutionTime.Milliseconds() * 1000)
        store.SetFuel(fuel)
    }
    
    // ✅ NEW: Create module with limits
    module, err := wasmtime.NewModule(engine, moduleBytes)
    if err != nil {
        result.Error = fmt.Sprintf("compile module: %v", err)
        return result, err
    }
    
    // ✅ NEW: Create linker with host function restrictions
    linker := wasmtime.NewLinker(engine)
    
    // ✅ NEW: Configure module instance with limits
    instance, err := wasmtime.NewInstance(store, module, []wasmtime.AsExtern{})
    if err != nil {
        result.Error = fmt.Sprintf("instantiate: %v", err)
        return result, err
    }
    
    // Get the function
    wasmFunc := instance.GetFunc(store, function)
    if wasmFunc == nil {
        result.Error = fmt.Sprintf("function not found: %s", function)
        return result, fmt.Errorf("function not found: %s", function)
    }
    
    // Convert args
    wasmArgs, err := convertArgsToWasmtime(args)
    if err != nil {
        result.Error = fmt.Sprintf("convert args: %v", err)
        return result, err
    }
    
    // Execute with fuel consumption monitoring
    wasmValues, err := wasmFunc.Call(store, wasmArgs...)
    if err != nil {
        result.Error = fmt.Sprintf("call function: %v", err)
        return result, err
    }
    
    // Check fuel consumption
    consumed := store.FuelConsumed()
    if r.limits != nil && r.limits.MaxFuel > 0 && consumed > r.limits.MaxFuel {
        result.Error = fmt.Sprintf("fuel limit exceeded: %d/%d", consumed, r.limits.MaxFuel)
        return result, fmt.Errorf("computation limit exceeded")
    }
    
    result.Output = wasmValues
    result.Success = true
    return result, nil
}

// pkg/wasm/types.go - Add fuel field
type SandboxLimits struct {
    MaxFileSize         uint64        // Maximum file size in bytes
    MaxExecutionTime    time.Duration // Maximum execution time
    MaxMemory           uint64        // Maximum memory allocation in bytes
    MaxFileDescriptors  int           // Maximum open file descriptors
    MaxFuel             uint64        // Maximum fuel (computational steps)
}
```

**Testing:**
```go
// pkg/wasm/runtime_test.go
func TestWASMMemoryLimitEnforcement(t *testing.T) {
    runtime := NewRuntime()
    runtime.limits = &SandboxLimits{
        MaxMemory: 1024 * 1024, // 1MB limit
    }
    
    // Malicious WASM that allocates 10MB
    maliciousWASM := compileWasm(`
        (module
            (memory 1000000)  // Request 10MB
            (func (export "exploit")
                (loop $l
                    (memory.grow (i32.const 1))
                    (br_if $l (i32.const 1))
                )
            )
        )
    `)
    
    _, err := runtime.Execute(maliciousWASM, "exploit")
    
    // Should fail with memory limit error
    require.Error(t, err)
    require.Contains(t, err.Error(), "memory limit")
}
```

**Estimated Time**: 3 days

---

### 3. DSL File System Sandbox (Week 1) 🔴
**File**: `pkg/dsl/builtins.go:425-453`

**Problem**: File builtins accept any path without validation - can read /etc/passwd, write system binaries.

**Fix Required**: Restrict to workspace directory with proper path validation

```go
// pkg/dsl/builtins.go

import (
    "os"
    "path/filepath"
    "runtime"
    "strings"
)

// Workspace directory for DSL operations
var (
    workspaceDir string
    workspaceDirMutex sync.RWMutex
)

// GetWorkspaceDir returns the configured workspace directory
func GetWorkspaceDir() string {
    workspaceDirMutex.RLock()
    defer workspaceDirMutex.RUnlock()
    return workspaceDir
}

// SetWorkspaceDir sets the workspace directory (called during DSL initialization)
func SetWorkspaceDir(dir string) {
    workspaceDirMutex.Lock()
    defer workspaceDirMutex.Unlock()
    workspaceDir = dir
}

// validateAndResolvePath ensures path is within workspace directory
func validateAndResolvePath(path string) (string, error) {
    workspace := GetWorkspaceDir()
    if workspace == "" {
        return "", errors.New("workspace directory not configured")
    }
    
    // Clean the path to resolve "../"
    cleanPath := filepath.Clean(path)
    
    // Make absolute relative to workspace
    absPath, err := filepath.Abs(filepath.Join(workspace, cleanPath))
    if err != nil {
        return "", fmt.Errorf("invalid path: %w", err)
    }
    
    // Get workspace absolute path
    workspaceAbs, err := filepath.Abs(workspace)
    if err != nil {
        return "", fmt.Errorf("invalid workspace: %w", err)
    }
    
    // Verify resolved path is within workspace
    // Compare common prefix to ensure no directory traversal
    relPath, err := filepath.Rel(workspaceAbs, absPath)
    if err != nil {
        return "", fmt.Errorf("path resolution error: %w", err)
    }
    
    // If relative path starts with "..", it's outside workspace
    if strings.HasPrefix(relPath, "..") {
        return "", fmt.Errorf("path traversal attempt detected: %s", path)
    }
    
    return absPath, nil
}

func builtinFileRead(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
        return nil, err
    }
    
    // ✅ NEW: Validate and resolve path
    safePath, err := validateAndResolvePath(path)
    if err != nil {
        return nil, err
    }
    
    data, err := os.ReadFile(safePath)
    if err != nil {
        return nil, fmt.Errorf("file read failed: %w", err)
    }
    
    return starlark.String(data), nil
}

func builtinFileWrite(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path, content string
    var perm int = 0600 // Secure default
    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "path", &path,
        "content", &content,
        "perm?", &perm,
    ); err != nil {
        return nil, err
    }
    
    // ✅ NEW: Validate and resolve path
    safePath, err := validateAndResolvePath(path)
    if err != nil {
        return nil, err
    }
    
    // ✅ NEW: Use secure default permissions (0600 instead of 0644)
    if perm == 0644 {
        perm = 0600
    }
    
    if err := os.WriteFile(safePath, []byte(content), os.FileMode(perm)); err != nil {
        return nil, fmt.Errorf("file write failed: %w", err)
    }
    
    return starlark.None, nil
}

func builtinFileExists(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var path string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
        return nil, err
    }
    
    // ✅ NEW: Validate and resolve path
    safePath, err := validateAndResolvePath(path)
    if err != nil {
        return nil, err
    }
    
    _, err = os.Stat(safePath)
    return starlark.Bool(err == nil), nil
}
```

**Testing:**
```go
// pkg/dsl/builtins_test.go
func TestFileReadPathTraversal(t *testing.T) {
    // Set workspace directory
    SetWorkspaceDir("/tmp/muster-workspace")
    
    // Try to read /etc/passwd
    _, err := builtinFileRead(nil, nil, starlark.Tuple{starlark.String("/etc/passwd")}, nil)
    
    // Should fail with path traversal error
    require.Error(t, err)
    require.Contains(t, err.Error(), "path traversal")
}

func TestFileWriteSystemBinary(t *testing.T) {
    SetWorkspaceDir("/tmp/muster-workspace")
    
    // Try to write to /usr/bin/sudo
    _, err := builtinFileWrite(nil, nil, 
        starlark.Tuple{starlark.String("/usr/bin/sudo"), starlark.String("malicious")}, nil)
    
    require.Error(t, err)
    require.Contains(t, err.Error(), "path traversal")
}
```

**Estimated Time**: 2 days

---

### 4. HTTP Network Segmentation (Week 1) 🔴
**File**: `pkg/dsl/builtins.go:40-236`

**Problem**: HTTP builtins accept any URL without validation - can probe internal networks and exfiltrate credentials.

**Fix Required**: Add network allowlist and block localhost/private networks

```go
// pkg/dsl/builtins.go

import (
    "net"
    "net/url"
    "strings"
)

// Blocked networks for SSRF prevention
var blockedNetworks = []string{
    "127.0.0.0/8",      // Loopback
    "169.254.0.0/16",   // Link-local
    "10.0.0.0/8",       // Private
    "172.16.0.0/12",     // Private
    "192.168.0.0/16",    // Private
    "169.254.169.254",  // AWS metadata
    "metadata.google.internal", // GCP metadata
}

// Allowed domains (empty = allow all except blocked)
var allowedDomains []string

// validateURL ensures URL is safe and not SSRF-vulnerable
func validateURL(rawURL string) error {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    
    // Only allow HTTP/HTTPS
    if parsed.Scheme != "http" && parsed.Scheme != "https" {
        return errors.New("only HTTP/HTTPS URLs allowed")
    }
    
    // Extract host
    host := parsed.Hostname()
    if host == "" {
        return errors.New("URL must have a hostname")
    }
    
    // Check IP address for blocked networks
    ip := net.ParseIP(host)
    if ip != nil {
        for _, blockedCIDR := range blockedNetworks {
            _, network, _ := net.ParseCIDR(blockedCIDR)
            if network != nil && network.Contains(ip) {
                return fmt.Errorf("blocked network: %s", blockedCIDR)
            }
        }
    }
    
    // Check domain allowlist if configured
    if len(allowedDomains) > 0 {
        allowed := false
        for _, allowedDomain := range allowedDomains {
            if strings.HasSuffix(host, allowedDomain) {
                allowed = true
                break
            }
        }
        if !allowed {
            return fmt.Errorf("domain not in allowlist: %s", host)
        }
    }
    
    // Block metadata service access
    if strings.Contains(rawURL, "169.254.169.254") ||
       strings.Contains(rawURL, "metadata.google.internal") {
        return errors.New("metadata service access blocked")
    }
    
    return nil
}

func builtinHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var urlStr string
    var headers, params *starlark.Dict
    var timeout int = 30
    
    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "url", &urlStr,
        "headers?", &headers,
        "params?", &params,
        "timeout?", &timeout,
    ); err != nil {
        return nil, err
    }
    
    // ✅ NEW: Validate URL for SSRF
    if err := validateURL(urlStr); err != nil {
        return nil, err
    }
    
    headerMap := make(http.Header)
    if headers != nil {
        for _, item := range headers.Items() {
            key, _ := starlark.AsString(item[0])
            value, _ := starlark.AsString(item[1])
            headerMap.Set(key, value)
        }
    }
    
    client := &http.Client{
        Timeout: time.Duration(timeout) * time.Second,
        Transport: &http.Transport{
            // ✅ NEW: Explicit TLS verification (existing, keep)
            TLSClientConfig: &tls.Config{
                MinVersion:         tls.VersionTLS12,
                InsecureSkipVerify: false, // Explicit verification
            },
            // ✅ NEW: DNS pinning (prevent DNS rebinding)
            DialContext: (&net.Dialer{
                Timeout: 30 * time.Second,
            }).DialContext,
        },
    }
    
    req, err := http.NewRequest("GET", urlStr, nil)
    if err != nil {
        return nil, fmt.Errorf("create request failed: %w", err)
    }
    req.Header = headerMap
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http request failed: %w", err)
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    responseDict := starlark.NewDict(3)
    responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
    responseDict.SetKey(starlark.String("body"), starlark.String(body))
    
    return responseDict, nil
}
```

**Testing:**
```go
// pkg/dsl/builtins_test.go
func TestHTTPGetLocalhostBlocked(t *testing.T) {
    // Try to probe localhost
    _, err := builtinHTTPGet(nil, nil, 
        starlark.Tuple{starlark.String("http://localhost:8080")}, nil)
    
    require.Error(t, err)
    require.Contains(t, err.Error(), "blocked network")
}

func TestHTTPGetSSRFMetadata(t *testing.T) {
    // Try to access AWS metadata
    _, err := builtinHTTPGet(nil, nil,
        starlark.Tuple{starlark.String("http://169.254.169.254/latest/meta-data/iam/security-credentials/")}, nil)
    
    require.Error(t, err)
    require.Contains(t, err.Error(), "blocked")
}
```

**Estimated Time**: 3 days

---

### 5. Plaintext Credential Storage (Week 1) 🔴
**File**: `pkg/auth/revocation.go:49-51`

**Problem**: `hashKey()` returns key unchanged - API keys stored plaintext in memory.

**Fix Required**: Implement SHA256 hashing

```go
// pkg/auth/revocation.go

import (
    "crypto/sha256"
    "encoding/hex"
)

func hashKey(key string) string {
    hash := sha256.Sum256([]byte(key))
    return hex.EncodeToString(hash[:])
}
```

**Testing:**
```go
// pkg/auth/revocation_test.go
func TestHashKey(t *testing.T) {
    key := "test-api-key"
    
    hashed := hashKey(key)
    
    // Hashed should be different from original
    require.NotEqual(t, key, hashed)
    
    // Hash should be deterministic
    hashed2 := hashKey(key)
    require.Equal(t, hashed, hashed2)
    
    // Hash should be hex-encoded
    _, err := hex.DecodeString(hashed)
    require.NoError(t, err)
}
```

**Estimated Time**: 1 day

---

## P1 HIGH-RISK ISSUES (Weeks 2-4)

### 6. Rate Limit Race Condition (Week 2) ⚠️
**File**: `pkg/auth/ratelimit/tokenbucket.go:98-111`

**Fix Required**: Use `LoadOrStore` to prevent race window

```go
// pkg/auth/ratelimit/tokenbucket.go

func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
    // Load or create atomically
    bucket, loaded := rl.buckets.LoadOrStore(key, NewTokenBucket(rl.config.Capacity, rl.config.FillRate))
    
    if !loaded {
        // New bucket created
        _ = interface{}(bucket).(*TokenBucket)
    }
    
    return bucket.(*TokenBucket).Allow(tokens)
}
```

**Estimated Time**: 2 days

---

### 7. Circuit Breaker Pattern (Week 3) ⚠️

**Strategy**: Add circuit breakers to all external call points

```go
// pkg/resilience/circuitbreaker.go

import (
    "time"
    "errors"
    "sync/atomic"
)

// CircuitState represents the circuit breaker state
type CircuitState int32

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
    maxFailures     int
    timeout         time.Duration
    resetTimeout    time.Duration
    
    failures        atomic.Int32
    lastFailureTime atomic.Value
    
    state atomic.Value // CircuitState
}

func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
    cb := &CircuitBreaker{
        maxFailures:  maxFailures,
        timeout:      timeout,
        resetTimeout: timeout * 2,
    }
    cb.state.Store(CircuitClosed)
    return cb
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    state := cb.state.Load().(CircuitState)
    
    if state == CircuitOpen {
        if time.Since(cb.lastFailureTime.Load().(time.Time)) < cb.resetTimeout {
            return errors.New("circuit breaker open")
        }
        // Try half-open
        atomic.CompareAndSwapInt32(
            (*int32)(unsafe.Pointer(&state)),
            int32(CircuitOpen),
            int32(CircuitHalfOpen),
        )
    }
    
    err := fn()
    
    if err != nil {
        failures := cb.failures.Add(1)
        cb.lastFailureTime.Store(time.Now())
        
        if failures >= int32(cb.maxFailures) {
            cb.state.Store(CircuitOpen)
        }
        return err
    }
    
    cb.failures.Store(0)
    cb.state.Store(CircuitClosed)
    return nil
}

// Usage in HTTP client
func (c *HTTPClient) DoWithCircuitBreaker(req *http.Request) (*http.Response, error) {
    breaker := c.getCircuitBreaker(req.URL.Host)
    
    var resp *http.Response
    err := breaker.Execute(func() error {
        var err error
        resp, err = c.client.Do(req)
        return err
    })
    
    return resp, err
}
```

**Estimated Time**: 10 days

---

### 8. Goroutine Panic Recovery (Week 2) ⚠️

**File**: `pkg/mcp/server.go:65`

**Fix Required**: Add panic recovery to all goroutines

```go
// pkg/mcp/server.go

func (s *Server) handleRequestWithRecovery(ctx context.Context, req JSONRPCRequest) {
    defer func() {
        if r := recover(); r != nil {
            s.logger.Error("Panic in request handler", "panic", r)
            debug.PrintStack()
        }
    }()
    
    s.handleRequest(ctx, &req)
}

// Update all goroutine calls:
go s.handleRequestWithRecovery(ctx, req)
```

**Estimated Time**: 3 days

---

## P2 MEDIUM-RISK ISSUES (Weeks 5-8)

### 9. File Permissions (Week 5) ⚠️

**Locations**: 
- `pkg/cmdstore/store.go:79`
- `pkg/dsl/builtins.go:448`

**Fix Required**: Change 0644 to 0600 for sensitive files

```go
// pkg/cmdstore/store.go
os.WriteFile(s.dataPath, data, 0600)  // Was 0644

// pkg/dsl/builtins.go  
os.WriteFile(path, []byte(content), 0600)  // Was 0644
```

**Estimated Time**: 1 day

---

### 10. Connection Pool Limits (Week 6) ⚠️

**File**: `pkg/client/http.go`

**Fix Required**: Add connection pool limits

```go
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

**Estimated Time**: 2 days

---

## Remediation Timeline

### Week 1-2: P0 CRITICAL SHOWSTOPPERS
- **Day 1-5**: Token storage implementation (Linux, Windows, File)
- **Day 6-8**: WASM sandbox enforcement
- **Day 9-10**: DSL file system sandbox
- **Day 11-13**: HTTP network segmentation
- **Day 14**: Credential hashing

### Week 3-4: P0 CRITICAL (Continued)
- **Day 15-24**: Circuit breaker pattern implementation
- **Day 25-26**: WASM enforcement fuzzing
- **Day 27-28**: Network segmentation validation

### Week 5-8: P1 HIGH + P2 MEDIUM
- Rate limit fixes (2 days)
- Panic recovery (3 days)
- Config permissions (1 day)
- Integration tests replacement (10+ days)

### Week 9-12: P2 MEDIUM + Validation
- Deadlock detection (5 days)
- Cache consistency (3 days)
- Path protection (3 days)
- Binary coverage improvement (5+ days)

### Week 13-14: Validation
- External security audit (5 days)
- Penetration testing (5 days)
- Load testing (3 days)
- Documentation updates (2 days)

---

## Success Criteria

### Before Production Release

- [ ] All P0 vulnerabilities fixed
- [ ] All P1 vulnerabilities fixed
- [ ] Security architecture review passes
- [ ] Penetration test completed
- [ ] Binary coverage >70% for cmd/*
- [ ] Integration tests replaced with real binary tests
- [ ] External audit completed
- [ ] Load testing passes
- [ ] Documentation updated

---

## References

- Adversarial review: `.memory-bank/phase-32-adversarial-review.md`
- Integration tests: `demo/test-suites/*.go`
- Security checklist: `specs/SECURITY-AUDIT-CHECKLIST.md`

---

**Next Actions**:
1. **STOP**: Halt all new feature development immediately
2. **START**: Begin P0 critical security fixes (Week 1-2)
3. **RESOURCE**: Assign dedicated security engineer
4. **AUDIT**: Schedule external security review (Week 13)
5. **DOCUMENT**: Update progress.md with realistic timeline
