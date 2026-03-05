# Cryptographic Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

This specification defines the cryptographic standards and implementations used throughout the Muster system for secure communication, data integrity, and authentication. All cryptographic operations follow industry best practices and modern security standards.

### Security Principles

- **Defense in Depth**: Multiple layers of cryptographic protection
- **Modern Algorithms**: Use only NIST-approved, widely-audited algorithms
- **Key Management**: Secure key generation, storage, and rotation
- **Forward Secrecy**: Ensure past communications remain secure
- **Auditability**: All cryptographic operations are logged and auditable

## Supported Algorithms

### Symmetric Encryption

| Algorithm | Mode | Key Size | Use Case |
|-----------|------|----------|----------|
| AES-256 | GCM | 256-bit | Data encryption at rest |
| AES-256 | CTR | 256-bit | Stream encryption |
| ChaCha20-Poly1305 | AEAD | 256-bit | Mobile/low-power devices |

### Asymmetric Encryption

| Algorithm | Purpose | Key Size |
|-----------|---------|----------|
| RSA-OAEP | Key exchange | 4096-bit minimum |
| ECDH (P-384) | Key agreement | 384-bit curve |
| X25519 | Key agreement | 256-bit |

### Digital Signatures

| Algorithm | Purpose | Key Size |
|-----------|---------|----------|
| Ed25519 | WASM module signing | 256-bit |
| ECDSA (P-384) | API authentication | 384-bit curve |
| RSA-PSS | Legacy compatibility | 4096-bit |

### Hash Functions

| Algorithm | Output Size | Use Case |
|-----------|-------------|----------|
| SHA-256 | 256-bit | General purpose hashing |
| SHA-384 | 384-bit | High-security applications |
| SHA3-256 | 256-bit | Future-proof hashing |
| BLAKE2b | 256/512-bit | High-performance hashing |

## Key Management

### Key Hierarchy

```
Master Key (HSM-protected)
├── Data Encryption Keys (DEK)
│   ├── Database encryption keys
│   ├── File storage keys
│   └── Cache encryption keys
├── Key Encryption Keys (KEK)
│   ├── DEK wrapping keys
│   └── Backup encryption keys
└── Signing Keys
    ├── API authentication keys
    ├── WASM signing keys
    └── Certificate authority keys
```

### Key Generation

```go
package crypto

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/ed25519"
    "crypto/ecdsa"
    "crypto/elliptic"
    "io"
)

// GenerateEd25519KeyPair generates a new Ed25519 signing key pair
func GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
    return ed25519.GenerateKey(rand.Reader)
}

// GenerateRSAKeyPair generates a new RSA key pair
func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, error) {
    if bits < 4096 {
        bits = 4096
    }
    return rsa.GenerateKey(rand.Reader, bits)
}

// GenerateECDSAKeyPair generates a new ECDSA key pair on P-384 curve
func GenerateECDSAKeyPair() (*ecdsa.PrivateKey, error) {
    return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// GenerateSecureRandom generates cryptographically secure random bytes
func GenerateSecureRandom(length int) ([]byte, error) {
    bytes := make([]byte, length)
    if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
        return nil, err
    }
    return bytes, nil
}
```

### Key Storage

Keys MUST be stored using hardware security modules (HSM) or secure key management services:

```go
type KeyStorage interface {
    Store(keyID string, key []byte, keyType KeyType) error
    Retrieve(keyID string) ([]byte, KeyType, error)
    Delete(keyID string) error
    Rotate(keyID string) (string, error)
}

type KeyType int

const (
    KeyTypeSymmetric KeyType = iota
    KeyTypeAsymmetric
    KeyTypeSigning
)
```

### Key Rotation

```go
// KeyRotationPolicy defines rotation parameters
type KeyRotationPolicy struct {
    MaxAge        time.Duration // Maximum key lifetime
    MaxOperations int64         // Maximum operations before rotation
    Emergency     bool          // Force immediate rotation
}

// DefaultRotationPolicy returns standard rotation policy
func DefaultRotationPolicy() *KeyRotationPolicy {
    return &KeyRotationPolicy{
        MaxAge:        90 * 24 * time.Hour, // 90 days
        MaxOperations: 1_000_000,
        Emergency:     false,
    }
}
```

## Encryption Operations

### AES-256-GCM

```go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

// EncryptAESGCM encrypts data using AES-256-GCM
func EncryptAESGCM(plaintext []byte, key []byte) ([]byte, []byte, error) {
    if len(key) != 32 {
        return nil, nil, fmt.Errorf("key must be 32 bytes for AES-256")
    }
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, nil, err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, nil, err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts data using AES-256-GCM
func DecryptAESGCM(ciphertext []byte, key []byte) ([]byte, error) {
    if len(key) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes for AES-256")
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
        return nil, fmt.Errorf("ciphertext too short")
    }
    
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    return gcm.Open(nil, nonce, ciphertext, nil)
}
```

### ChaCha20-Poly1305

```go
import (
    "golang.org/x/crypto/chacha20poly1305"
)

// EncryptChaCha20 encrypts data using ChaCha20-Poly1305
func EncryptChaCha20(plaintext []byte, key []byte) ([]byte, []byte, error) {
    if len(key) != 32 {
        return nil, nil, fmt.Errorf("key must be 32 bytes for ChaCha20")
    }
    
    aead, err := chacha20poly1305.New(key)
    if err != nil {
        return nil, nil, err
    }
    
    nonce := make([]byte, aead.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, nil, err
    }
    
    ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
    return ciphertext, nonce, nil
}
```

## Digital Signatures

### Ed25519 Signing

```go
// SignEd25519 creates an Ed25519 signature
func SignEd25519(message []byte, privateKey ed25519.PrivateKey) ([]byte, error) {
    if len(privateKey) != ed25519.PrivateKeySize {
        return nil, fmt.Errorf("invalid private key size")
    }
    
    signature := ed25519.Sign(privateKey, message)
    return signature, nil
}

// VerifyEd25519 verifies an Ed25519 signature
func VerifyEd25519(message []byte, signature []byte, publicKey ed25519.PublicKey) bool {
    if len(publicKey) != ed25519.PublicKeySize {
        return false
    }
    
    return ed25519.Verify(publicKey, message, signature)
}

// SignStringEd25519 signs a string message and returns base64-encoded signature
func SignStringEd25519(message string, privateKey ed25519.PrivateKey) (string, error) {
    sig, err := SignEd25519([]byte(message), privateKey)
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyStringEd25519 verifies a base64-encoded signature
func VerifyStringEd25519(message string, signatureB64 string, publicKey ed25519.PublicKey) (bool, error) {
    signature, err := base64.StdEncoding.DecodeString(signatureB64)
    if err != nil {
        return false, err
    }
    
    return VerifyEd25519([]byte(message), signature, publicKey), nil
}
```

### WASM Module Signing

```go
// WasmModuleSignature represents a signed WASM module
type WasmModuleSignature struct {
    Algorithm string `json:"algorithm"`
    PublicKey string `json:"public_key"`
    Signature string `json:"signature"`
}

// WasmModule represents a WASM module with signature
type WasmModule struct {
    Name      string              `json:"name"`
    URL       string              `json:"url"`
    SHA256    string              `json:"sha256"`
    Signature WasmModuleSignature `json:"signature"`
    Version   string              `json:"version"`
}

// SignWasmModule signs a WASM module
func SignWasmModule(module *WasmModule, privateKey ed25519.PrivateKey) error {
    // Create canonical message: name:sha256:url:version
    message := fmt.Sprintf("%s:%s:%s:%s", 
        module.Name, 
        module.SHA256, 
        module.URL, 
        module.Version,
    )
    
    signature, err := SignStringEd25519(message, privateKey)
    if err != nil {
        return err
    }
    
    module.Signature = WasmModuleSignature{
        Algorithm: "Ed25519",
        PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
        Signature: signature,
    }
    
    return nil
}

// VerifyWasmModule verifies a WASM module signature
func VerifyWasmModule(module *WasmModule) error {
    if module.Signature.Algorithm != "Ed25519" {
        return fmt.Errorf("unsupported signature algorithm: %s", module.Signature.Algorithm)
    }
    
    message := fmt.Sprintf("%s:%s:%s:%s",
        module.Name,
        module.SHA256,
        module.URL,
        module.Version,
    )
    
    publicKey, err := base64.StdEncoding.DecodeString(module.Signature.PublicKey)
    if err != nil {
        return fmt.Errorf("invalid public key: %w", err)
    }
    
    valid, err := VerifyStringEd25519(message, module.Signature.Signature, publicKey)
    if err != nil {
        return fmt.Errorf("signature verification error: %w", err)
    }
    
    if !valid {
        return fmt.Errorf("signature verification failed")
    }
    
    return nil
}
```

## Hash Functions

### SHA-256 Hashing

```go
import (
    "crypto/sha256"
    "encoding/hex"
)

// HashSHA256 computes SHA-256 hash
func HashSHA256(data []byte) []byte {
    hash := sha256.Sum256(data)
    return hash[:]
}

// HashSHA256Hex computes SHA-256 hash and returns hex string
func HashSHA256Hex(data []byte) string {
    return hex.EncodeToString(HashSHA256(data))
}

// HashSHA256File computes SHA-256 hash of file contents
func HashSHA256File(filepath string) (string, error) {
    file, err := os.Open(filepath)
    if err != nil {
        return "", err
    }
    defer file.Close()
    
    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", err
    }
    
    return hex.EncodeToString(hasher.Sum(nil)), nil
}
```

### HMAC

```go
import (
    "crypto/hmac"
    "crypto/sha256"
)

// ComputeHMAC computes HMAC-SHA256
func ComputeHMAC(key []byte, data []byte) []byte {
    mac := hmac.New(sha256.New, key)
    mac.Write(data)
    return mac.Sum(nil)
}

// VerifyHMAC verifies HMAC-SHA256
func VerifyHMAC(key []byte, data []byte, expectedMAC []byte) bool {
    actualMAC := ComputeHMAC(key, data)
    return hmac.Equal(actualMAC, expectedMAC)
}
```

## Transport Layer Security

### TLS Configuration

```go
import (
    "crypto/tls"
)

// SecureTLSConfig returns a secure TLS configuration
func SecureTLSConfig() *tls.Config {
    return &tls.Config{
        MinVersion: tls.VersionTLS13,
        CurvePreferences: []tls.CurveID{
            tls.X25519,
            tls.CurveP384,
        },
        CipherSuites: []uint16{
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
        },
        PreferServerCipherSuites: true,
    }
}

// MutualTLSConfig returns a TLS configuration with client certificate validation
func MutualTLSConfig(caCert []byte) (*tls.Config, error) {
    certPool := x509.NewCertPool()
    if !certPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("failed to parse CA certificate")
    }
    
    config := SecureTLSConfig()
    config.ClientCAs = certPool
    config.ClientAuth = tls.RequireAndVerifyClientCert
    
    return config, nil
}
```

## Password Hashing

### Argon2id

```go
import (
    "golang.org/x/crypto/argon2"
)

// Argon2Params defines Argon2id parameters
type Argon2Params struct {
    Memory      uint32
    Iterations  uint32
    Parallelism uint8
    SaltLength  uint32
    KeyLength   uint32
}

// DefaultArgon2Params returns recommended Argon2id parameters
func DefaultArgon2Params() *Argon2Params {
    return &Argon2Params{
        Memory:      64 * 1024, // 64 MB
        Iterations:  3,
        Parallelism: 4,
        SaltLength:  16,
        KeyLength:   32,
    }
}

// HashPassword hashes a password using Argon2id
func HashPassword(password string, params *Argon2Params) (string, error) {
    if params == nil {
        params = DefaultArgon2Params()
    }
    
    salt, err := GenerateSecureRandom(int(params.SaltLength))
    if err != nil {
        return "", err
    }
    
    hash := argon2.IDKey(
        []byte(password),
        salt,
        params.Iterations,
        params.Memory,
        params.Parallelism,
        params.KeyLength,
    )
    
    // Encode to modular crypt format
    b64Salt := base64.RawStdEncoding.EncodeToString(salt)
    b64Hash := base64.RawStdEncoding.EncodeToString(hash)
    
    encodedHash := fmt.Sprintf(
        "$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
        argon2.Version,
        params.Memory,
        params.Iterations,
        params.Parallelism,
        b64Salt,
        b64Hash,
    )
    
    return encodedHash, nil
}

// VerifyPassword verifies a password against an Argon2id hash
func VerifyPassword(password string, encodedHash string) (bool, error) {
    // Parse the encoded hash
    parts := strings.Split(encodedHash, "$")
    if len(parts) != 6 {
        return false, fmt.Errorf("invalid hash format")
    }
    
    var version int
    _, err := fmt.Sscanf(parts[2], "v=%d", &version)
    if err != nil {
        return false, err
    }
    
    if version != argon2.Version {
        return false, fmt.Errorf("incompatible argon2 version")
    }
    
    params := &Argon2Params{}
    _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", 
        &params.Memory, &params.Iterations, &params.Parallelism)
    if err != nil {
        return false, err
    }
    
    salt, err := base64.RawStdEncoding.DecodeString(parts[4])
    if err != nil {
        return false, err
    }
    params.SaltLength = uint32(len(salt))
    
    hash, err := base64.RawStdEncoding.DecodeString(parts[5])
    if err != nil {
        return false, err
    }
    params.KeyLength = uint32(len(hash))
    
    // Compute hash with same parameters
    computedHash := argon2.IDKey(
        []byte(password),
        salt,
        params.Iterations,
        params.Memory,
        params.Parallelism,
        params.KeyLength,
    )
    
    return subtle.ConstantTimeCompare(hash, computedHash) == 1, nil
}
```

## Secure Random Generation

```go
// SecureRandomGenerator provides cryptographically secure random values
type SecureRandomGenerator struct {
    reader io.Reader
}

// NewSecureRandomGenerator creates a new secure random generator
func NewSecureRandomGenerator() *SecureRandomGenerator {
    return &SecureRandomGenerator{
        reader: rand.Reader,
    }
}

// GenerateBytes generates random bytes
func (g *SecureRandomGenerator) GenerateBytes(length int) ([]byte, error) {
    bytes := make([]byte, length)
    if _, err := io.ReadFull(g.reader, bytes); err != nil {
        return nil, err
    }
    return bytes, nil
}

// GenerateHex generates random hex string
func (g *SecureRandomGenerator) GenerateHex(length int) (string, error) {
    bytes, err := g.GenerateBytes(length)
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(bytes), nil
}

// GenerateBase64 generates random base64 string
func (g *SecureRandomGenerator) GenerateBase64(length int) (string, error) {
    bytes, err := g.GenerateBytes(length)
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(bytes), nil
}

// GenerateUUID generates a random UUID v4
func (g *SecureRandomGenerator) GenerateUUID() (string, error) {
    bytes, err := g.GenerateBytes(16)
    if err != nil {
        return "", err
    }
    
    // Set version (4) and variant (RFC 4122)
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80
    
    return fmt.Sprintf(
        "%x-%x-%x-%x-%x",
        bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16],
    ), nil
}
```

## Compliance and Auditing

### Cryptographic Audit Log

```go
// CryptoOperation represents a logged cryptographic operation
type CryptoOperation struct {
    Timestamp   time.Time              `json:"timestamp"`
    Operation   string                 `json:"operation"`
    Algorithm   string                 `json:"algorithm"`
    KeyID       string                 `json:"key_id,omitempty"`
    Success     bool                   `json:"success"`
    Error       string                 `json:"error,omitempty"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CryptoAuditor logs cryptographic operations
type CryptoAuditor struct {
    logger *log.Logger
}

// LogOperation logs a cryptographic operation
func (a *CryptoAuditor) LogOperation(op *CryptoOperation) {
    op.Timestamp = time.Now().UTC()
    
    data, _ := json.Marshal(op)
    a.logger.Printf("[CRYPTO] %s", string(data))
}
```

### Compliance Standards

This specification ensures compliance with:

- **NIST SP 800-57**: Key Management Guidelines
- **NIST SP 800-131A**: Cryptographic Algorithm Transitions
- **FIPS 140-2/140-3**: Cryptographic Module Validation
- **PCI DSS**: Payment Card Industry Data Security Standard

---

**Bible Standard Compliance**: This specification adheres to the Muster Bible Standard for cryptographic security, key management, and auditability. All implementations must use approved algorithms and follow secure key management practices.
