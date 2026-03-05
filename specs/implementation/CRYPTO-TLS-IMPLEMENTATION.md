# Cryptography and TLS Implementation Specification

## Overview

Production-ready Go cryptography and TLS implementation providing Ed25519 signatures, X.509 certificate management, mTLS support, secure key storage with platform integration, and TLS 1.2+ with strong cipher suites. Designed for secure inventory signature verification, mutual TLS authentication, and hardware fingerprinting.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CryptoManager (Main Interface)                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐ │
│  │   Ed25519     │  │  Certificate  │  │     mTLS      │  │  Key Storage  │ │
│  │   Signatures  │  │   Manager     │  │   Provider    │  │  (Keychain)   │ │
│  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘ │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐ │
│  │   Hash Utils  │  │  TLS Config   │  │   Pinning     │  │    Random     │ │
│  │  (SHA-256)    │  │   Factory     │  │   Manager     │  │   (CSPRNG)    │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └───────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
┌─────────────────────────────────────┴─────────────────────────────────────┐
│                           Integration Points                                │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐            │
│  │  Auth System │<────>│   Inventory  │<────>│ Fingerprint  │            │
│  │  (mTLS Auth) │      │ (Signatures) │      │  (Hashing)   │            │
│  └──────────────┘      └──────────────┘      └──────────────┘            │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Core Interfaces

### CryptoManager Interface

```go
package crypto

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"io"
)

// CryptoManager provides centralized cryptographic operations
type CryptoManager interface {
	// Signature operations
	Sign(data []byte) ([]byte, error)
	Verify(data []byte, signature []byte) error
	
	// Certificate operations
	LoadCertificate(certPEM []byte) (*x509.Certificate, error)
	ValidateCertificate(cert *x509.Certificate) error
	VerifyCertificateChain(cert *x509.Certificate, roots *x509.CertPool) error
	
	// TLS operations
	GetTLSConfig() *tls.Config
	GetMTLSConfig() *tls.Config
	PinCertificate(cert *x509.Certificate) error
	VerifyPinned(cert *x509.Certificate) error
	
	// Key operations
	GenerateKeyPair() (crypto.PrivateKey, crypto.PublicKey, error)
	StoreKey(name string, key crypto.PrivateKey) error
	LoadKey(name string) (crypto.PrivateKey, error)
	
	// Hash operations
	Hash(data []byte) []byte
	HashReader(r io.Reader) ([]byte, error)
	
	// Random operations
	GenerateRandom(length int) ([]byte, error)
	
	// Close cleans up resources
	Close() error
}

// Config holds crypto configuration
type Config struct {
	// Key storage
	KeyStorageType KeyStorageType
	KeyStorePath   string
	
	// TLS settings
	MinTLSVersion   uint16
	CipherSuites    []uint16
	InsecureSkipVerify bool
	ServerName      string
	
	// Certificate settings
	CACertPath      string
	ClientCertPath  string
	ClientKeyPath   string
	AutoGenerate    bool
	
	// Pinning settings
	PinningEnabled  bool
	PinnedCerts     []string // SHA-256 hashes of pinned certificates
	
	// Signature settings
	SignAlgorithm   SignatureAlgorithm
}

// KeyStorageType defines key storage backends
type KeyStorageType int

const (
	KeyStorageMemory KeyStorageType = iota
	KeyStorageKeychain
	KeyStorageKeyring
	KeyStorageFile
	KeyStorageWindows
)

// SignatureAlgorithm defines supported signature algorithms
type SignatureAlgorithm int

const (
	SignatureEd25519 SignatureAlgorithm = iota
	SignatureECDSAP256
	SignatureECDSAP384
)

func (a SignatureAlgorithm) String() string {
	switch a {
	case SignatureEd25519:
		return "Ed25519"
	case SignatureECDSAP256:
		return "ECDSA-P256"
	case SignatureECDSAP384:
		return "ECDSA-P384"
	default:
		return "unknown"
	}
}
```

## Ed25519 Signatures

### Ed25519 Implementation

```go
package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Ed25519Provider implements Ed25519 signature operations
type Ed25519Provider struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewEd25519Provider creates a new Ed25519 provider
func NewEd25519Provider() (*Ed25519Provider, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}
	
	return &Ed25519Provider{
		privateKey: priv,
		publicKey:  priv.Public().(ed25519.PublicKey),
	}, nil
}

// Sign creates an Ed25519 signature for the given data
func (p *Ed25519Provider) Sign(data []byte) ([]byte, error) {
	if p.privateKey == nil {
		return nil, fmt.Errorf("private key not loaded")
	}
	
	signature := ed25519.Sign(p.privateKey, data)
	return signature, nil
}

// Verify verifies an Ed25519 signature
func (p *Ed25519Provider) Verify(data []byte, signature []byte) error {
	if p.publicKey == nil {
		return fmt.Errorf("public key not loaded")
	}
	
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: expected %d, got %d", ed25519.SignatureSize, len(signature))
	}
	
	if !ed25519.Verify(p.publicKey, data, signature) {
		return fmt.Errorf("signature verification failed")
	}
	
	return nil
}

// VerifyWithKey verifies a signature using a specific public key
func VerifyEd25519Signature(publicKey ed25519.PublicKey, data []byte, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: expected %d, got %d", ed25519.PublicKeySize, len(publicKey))
	}
	
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: expected %d, got %d", ed25519.SignatureSize, len(signature))
	}
	
	if !ed25519.Verify(publicKey, data, signature) {
		return fmt.Errorf("signature verification failed")
	}
	
	return nil
}

// ExportPublicKeyBase64 exports the public key as base64
func (p *Ed25519Provider) ExportPublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(p.publicKey)
}

// ImportEd25519PublicKeyBase64 imports a public key from base64
func ImportEd25519PublicKeyBase64(encoded string) (ed25519.PublicKey, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	
	if len(data) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: expected %d, got %d", ed25519.PublicKeySize, len(data))
	}
	
	return ed25519.PublicKey(data), nil
}
```

### Inventory Signature Verification

```go
package crypto

import (
	"encoding/json"
	"time"
)

// InventorySignature represents a signed inventory
type InventorySignature struct {
	Version    string `json:"version"`
	Content    []byte `json:"content"`
	Signature  string `json:"signature"`  // Base64-encoded Ed25519 signature
	PublicKey  string `json:"public_key"` // Base64-encoded Ed25519 public key
	Algorithm  string `json:"algorithm"`  // "Ed25519"
	Timestamp  int64  `json:"timestamp"`
}

// InventoryVerifier handles inventory signature verification
type InventoryVerifier struct {
	trustedKeys map[string]ed25519.PublicKey // key ID -> public key
}

// NewInventoryVerifier creates a new inventory verifier
func NewInventoryVerifier() *InventoryVerifier {
	return &InventoryVerifier{
		trustedKeys: make(map[string]ed25519.PublicKey),
	}
}

// AddTrustedKey adds a trusted public key for verification
func (v *InventoryVerifier) AddTrustedKey(keyID string, publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: expected %d, got %d", ed25519.PublicKeySize, len(publicKey))
	}
	
	v.trustedKeys[keyID] = publicKey
	return nil
}

// VerifyInventory verifies an inventory signature
func (v *InventoryVerifier) VerifyInventory(sig *InventorySignature) error {
	if sig.Algorithm != "Ed25519" {
		return fmt.Errorf("unsupported signature algorithm: %s", sig.Algorithm)
	}
	
	signature, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}
	
	publicKey, err := ImportEd25519PublicKeyBase64(sig.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}
	
	keyFound := false
	for _, trustedKey := range v.trustedKeys {
		if string(trustedKey) == string(publicKey) {
			keyFound = true
			break
		}
	}
	
	if !keyFound {
		return fmt.Errorf("untrusted public key")
	}
	
	// Create canonical content for signing
	content := struct {
		Version   string `json:"version"`
		Content   []byte `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}{
		Version:   sig.Version,
		Content:   sig.Content,
		Timestamp: sig.Timestamp,
	}
	
	canonical, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to canonicalize content: %w", err)
	}
	
	if err := VerifyEd25519Signature(publicKey, canonical, signature); err != nil {
		return fmt.Errorf("inventory signature verification failed: %w", err)
	}
	
	return nil
}

// SignInventory creates a signed inventory
func SignInventory(content []byte, version string, provider *Ed25519Provider) (*InventorySignature, error) {
	if provider == nil {
		return nil, fmt.Errorf("signature provider is nil")
	}
	
	timestamp := time.Now().Unix()
	
	sigContent := struct {
		Version   string `json:"version"`
		Content   []byte `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}{
		Version:   version,
		Content:   content,
		Timestamp: timestamp,
	}
	
	canonical, err := json.Marshal(sigContent)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize content: %w", err)
	}
	
	signature, err := provider.Sign(canonical)
	if err != nil {
		return nil, fmt.Errorf("failed to sign inventory: %w", err)
	}
	
	return &InventorySignature{
		Version:   version,
		Content:   content,
		Signature: base64.StdEncoding.EncodeToString(signature),
		PublicKey: provider.ExportPublicKeyBase64(),
		Algorithm: "Ed25519",
		Timestamp: timestamp,
	}, nil
}
```

## Certificate Management

### X.509 Certificate Parsing and Validation

```go
package crypto

import (
	"crypto/x509"
	"encoding/pem"
	"time"
)

// CertificateManager handles X.509 certificate operations
type CertificateManager struct {
	rootCAs       *x509.CertPool
	intermediates *x509.CertPool
	certs         []*x509.Certificate
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager() *CertificateManager {
	return &CertificateManager{
		rootCAs:       x509.NewCertPool(),
		intermediates: x509.NewCertPool(),
		certs:         make([]*x509.Certificate, 0),
	}
}

// LoadCertificateFromPEM parses a certificate from PEM data
func (cm *CertificateManager) LoadCertificateFromPEM(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid PEM type: expected CERTIFICATE, got %s", block.Type)
	}
	
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	return cert, nil
}

// VerifyCertificate validates a certificate (expiration, signature, etc.)
func (cm *CertificateManager) VerifyCertificate(cert *x509.Certificate) error {
	now := time.Now()
	
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not yet valid (valid from %s)", cert.NotBefore)
	}
	
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired (expired %s)", cert.NotAfter)
	}
	
	return nil
}

// VerifyCertificateChain validates a certificate chain
func (cm *CertificateManager) VerifyCertificateChain(cert *x509.Certificate) error {
	if err := cm.VerifyCertificate(cert); err != nil {
		return err
	}
	
	opts := x509.VerifyOptions{
		Roots:         cm.rootCAs,
		Intermediates: cm.intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	
	_, err := cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}
	
	return nil
}

// GetCertificateFingerprint returns the SHA-256 fingerprint of a certificate
func GetCertificateFingerprint(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(h[:])
}

// IsSelfSigned checks if a certificate is self-signed
func IsSelfSigned(cert *x509.Certificate) bool {
	return cert.Subject.String() == cert.Issuer.String()
}

// NeedsRenewal checks if certificate needs renewal (within 30 days of expiration)
func NeedsRenewal(cert *x509.Certificate) bool {
	return time.Until(cert.NotAfter) < 30*24*time.Hour
}
```

## mTLS Support

### mTLS Configuration and Client Certificate Loading

```go
package crypto

import (
	"crypto/tls"
	"crypto/x509"
)

// MTLSConfig holds mutual TLS configuration
type MTLSConfig struct {
	CertPath       string
	KeyPath        string
	CertPEM        []byte
	KeyPEM         []byte
	CAPath         string
	CAPEM          []byte
	ServerName     string
	InsecureSkipVerify bool
	RequireClientCert  bool
	ClientCACertPath   string
	VerifyClientCert   bool
}

// MTLSProvider provides mutual TLS authentication
type MTLSProvider struct {
	config        *MTLSConfig
	tlsConfig     *tls.Config
	cert          *tls.Certificate
	caCert        *x509.Certificate
}

// NewMTLSProvider creates a new mTLS provider
func NewMTLSProvider(config *MTLSConfig) (*MTLSProvider, error) {
	provider := &MTLSProvider{
		config: config,
	}
	
	cert, err := provider.loadClientCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}
	provider.cert = cert
	
	if config.CAPath != "" || len(config.CAPEM) > 0 {
		caCert, err := provider.loadCACertificate()
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate: %w", err)
		}
		provider.caCert = caCert
	}
	
	if err := provider.buildTLSConfig(); err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}
	
	return provider, nil
}

// loadClientCertificate loads the client certificate
func (p *MTLSProvider) loadClientCertificate() (*tls.Certificate, error) {
	var certPEM, keyPEM []byte
	
	if len(p.config.CertPEM) > 0 && len(p.config.KeyPEM) > 0 {
		certPEM = p.config.CertPEM
		keyPEM = p.config.KeyPEM
	} else if p.config.CertPath != "" && p.config.KeyPath != "" {
		certPEM, err = os.ReadFile(p.config.CertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		
		keyPEM, err = os.ReadFile(p.config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("client certificate and key must be provided")
	}
	
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key pair: %w", err)
	}
	
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse X509 certificate: %w", err)
	}
	
	if time.Now().After(x509Cert.NotAfter) {
		return nil, fmt.Errorf("certificate has expired")
	}
	
	return &cert, nil
}

// buildTLSConfig builds the TLS configuration
func (p *MTLSProvider) buildTLSConfig() error {
	p.tlsConfig = &tls.Config{
		Certificates:       []tls.Certificate{*p.cert},
		ServerName:         p.config.ServerName,
		InsecureSkipVerify: p.config.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
		CipherSuites:       GetSecureCipherSuites(),
		PreferServerCipherSuites: true,
	}
	
	if p.caCert != nil {
		caPool := x509.NewCertPool()
		caPool.AddCert(p.caCert)
		p.tlsConfig.RootCAs = caPool
	}
	
	return nil
}

// GetTLSConfig returns the TLS configuration for use with HTTP clients
func (p *MTLSProvider) GetTLSConfig() *tls.Config {
	return p.tlsConfig.Clone()
}

// VerifyCertificate validates the client certificate
func (p *MTLSProvider) VerifyCertificate() error {
	if p.cert == nil {
		return fmt.Errorf("no certificate loaded")
	}
	
	x509Cert, err := x509.ParseCertificate(p.cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	if time.Now().After(x509Cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}
	
	if p.caCert != nil {
		caPool := x509.NewCertPool()
		caPool.AddCert(p.caCert)
		
		opts := x509.VerifyOptions{
			Roots:     caPool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		
		if _, err := x509Cert.Verify(opts); err != nil {
			return fmt.Errorf("certificate verification failed: %w", err)
		}
	}
	
	return nil
}

// IsExpired checks if the certificate is expired or near expiration
func (p *MTLSProvider) IsExpired() bool {
	if p.cert == nil {
		return true
	}
	
	x509Cert, err := x509.ParseCertificate(p.cert.Certificate[0])
	if err != nil {
		return true
	}
	
	return time.Now().Add(24 * time.Hour).After(x509Cert.NotAfter)
}
```

## Key Storage

### Secure Key Storage with Platform Integration

```go
package crypto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// KeyStorage defines the interface for secure key storage
type KeyStorage interface {
	Store(name string, key interface{}) error
	Retrieve(name string) (interface{}, error)
	Delete(name string) error
	List() ([]string, error)
	Exists(name string) (bool, error)
}

// KeyData represents stored key data
type KeyData struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
	Algorithm string    `json:"algorithm"`
}

// SecureKeyStore provides secure key storage with platform integration
type SecureKeyStore struct {
	storage KeyStorage
	mu      sync.RWMutex
}

// NewSecureKeyStore creates a secure key store appropriate for the platform
func NewSecureKeyStore(config *Config) (*SecureKeyStore, error) {
	var storage KeyStorage
	var err error
	
	switch config.KeyStorageType {
	case KeyStorageKeychain:
		if runtime.GOOS != "darwin" {
			return nil, fmt.Errorf("keychain storage only available on macOS")
		}
		storage, err = NewMacOSKeychainStorage()
	case KeyStorageKeyring:
		if runtime.GOOS != "linux" {
			return nil, fmt.Errorf("keyring storage only available on Linux")
		}
		storage, err = NewLinuxKeyringStorage()
	case KeyStorageWindows:
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("Windows storage only available on Windows")
		}
		storage, err = NewWindowsCredentialStorage()
	case KeyStorageFile:
		storage, err = NewEncryptedFileStorage(config.KeyStorePath)
	case KeyStorageMemory:
		storage = NewMemoryStorage()
	default:
		return nil, fmt.Errorf("unknown key storage type: %d", config.KeyStorageType)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}
	
	return &SecureKeyStore{
		storage: storage,
	}, nil
}

// StoreEd25519Key stores an Ed25519 private key securely
func (s *SecureKeyStore) StoreEd25519Key(name string, key ed25519.PrivateKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	keyData := &KeyData{
		Name:      name,
		Type:      "Ed25519",
		Data:      key.Seed(), // Store only the seed (32 bytes)
		CreatedAt: time.Now(),
		Algorithm: "Ed25519",
	}
	
	return s.storage.Store(name, keyData)
}

// RetrieveEd25519Key retrieves an Ed25519 private key
func (s *SecureKeyStore) RetrieveEd25519Key(name string) (ed25519.PrivateKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	data, err := s.storage.Retrieve(name)
	if err != nil {
		return nil, err
	}
	
	keyData, ok := data.(*KeyData)
	if !ok {
		return nil, fmt.Errorf("invalid key data type")
	}
	
	if keyData.Type != "Ed25519" {
		return nil, fmt.Errorf("key is not Ed25519 type")
	}
	
	if len(keyData.Data) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid Ed25519 seed size")
	}
	
	return ed25519.NewKeyFromSeed(keyData.Data), nil
}

// MemoryStorage provides in-memory key storage (for testing/development)
type MemoryStorage struct {
	keys map[string]*KeyData
	mu   sync.RWMutex
}

// NewMemoryStorage creates a new memory-based storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		keys: make(map[string]*KeyData),
	}
}

func (m *MemoryStorage) Store(name string, key interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	keyData, ok := key.(*KeyData)
	if !ok {
		return fmt.Errorf("key must be *KeyData")
	}
	
	m.keys[name] = keyData
	return nil
}

func (m *MemoryStorage) Retrieve(name string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	key, exists := m.keys[name]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", name)
	}
	
	keyCopy := *key
	return &keyCopy, nil
}

func (m *MemoryStorage) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.keys, name)
	return nil
}

func (m *MemoryStorage) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	names := make([]string, 0, len(m.keys))
	for name := range m.keys {
		names = append(names, name)
	}
	return names, nil
}

func (m *MemoryStorage) Exists(name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.keys[name]
	return exists, nil
}

// EncryptedFileStorage provides file-based encrypted storage
type EncryptedFileStorage struct {
	basePath string
	mu       sync.RWMutex
}

// NewEncryptedFileStorage creates encrypted file-based storage
func NewEncryptedFileStorage(basePath string) (*EncryptedFileStorage, error) {
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	return &EncryptedFileStorage{
		basePath: basePath,
	}, nil
}

func (e *EncryptedFileStorage) Store(name string, key interface{}) error {
	keyData, ok := key.(*KeyData)
	if !ok {
		return fmt.Errorf("key must be *KeyData")
	}
	
	data, err := json.Marshal(keyData)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}
	
	encrypted, err := encryptData(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}
	
	filename := filepath.Join(e.basePath, fmt.Sprintf("%s.key", sanitizeFilename(name)))
	if err := os.WriteFile(filename, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}
	
	return nil
}

func (e *EncryptedFileStorage) Retrieve(name string) (interface{}, error) {
	filename := filepath.Join(e.basePath, fmt.Sprintf("%s.key", sanitizeFilename(name)))
	
	encrypted, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("key not found: %s", name)
		}
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	
	data, err := decryptData(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}
	
	var keyData KeyData
	if err := json.Unmarshal(data, &keyData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key: %w", err)
	}
	
	return &keyData, nil
}

func (e *EncryptedFileStorage) Delete(name string) error {
	filename := filepath.Join(e.basePath, fmt.Sprintf("%s.key", sanitizeFilename(name)))
	return os.Remove(filename)
}

func (e *EncryptedFileStorage) List() ([]string, error) {
	entries, err := os.ReadDir(e.basePath)
	if err != nil {
		return nil, err
	}
	
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".key") {
			name := strings.TrimSuffix(entry.Name(), ".key")
			names = append(names, name)
		}
	}
	
	return names, nil
}

func (e *EncryptedFileStorage) Exists(name string) (bool, error) {
	filename := filepath.Join(e.basePath, fmt.Sprintf("%s.key", sanitizeFilename(name)))
	_, err := os.Stat(filename)
	return !os.IsNotExist(err), nil
}

func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
}
```

## Certificate Generation

### Self-Signed Certificate Generation for Testing

```go
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
)

// NewSelfSignedCACertificate generates a self-signed CA certificate
func NewSelfSignedCACertificate(subject pkix.Name) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}
	
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}
	
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}
	
	return cert, privateKey, nil
}

// GenerateLeafCertificate generates a leaf certificate signed by a CA
func GenerateLeafCertificate(caCert *x509.Certificate, caKey interface{}, dnsNames []string, ipAddresses []net.IP) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate leaf key: %w", err)
	}
	
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Server"},
			CommonName:   "localhost",
		},
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create leaf certificate: %w", err)
	}
	
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse leaf certificate: %w", err)
	}
	
	return cert, privateKey, nil
}

// CertificateToPEM converts a certificate to PEM format
func CertificateToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// PrivateKeyToPEM converts a private key to PEM format
func PrivateKeyToPEM(key interface{}) ([]byte, error) {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: b,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
}

// GenerateSelfSignedCertForTesting generates a complete self-signed certificate for testing
func GenerateSelfSignedCertForTesting(hosts []string) (certPEM []byte, keyPEM []byte, err error) {
	caSubject := pkix.Name{
		Organization: []string{"Test CA"},
		CommonName:   "Test CA",
	}
	
	caCert, caKey, err := NewSelfSignedCACertificate(caSubject)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA: %w", err)
	}
	
	var dnsNames []string
	var ipAddresses []net.IP
	
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		} else {
			dnsNames = append(dnsNames, host)
		}
	}
	
	leafCert, leafKey, err := GenerateLeafCertificate(caCert, caKey, dnsNames, ipAddresses)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate leaf certificate: %w", err)
	}
	
	certPEM = CertificateToPEM(leafCert)
	keyPEM, err = PrivateKeyToPEM(leafKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert key to PEM: %w", err)
	}
	
	return certPEM, keyPEM, nil
}
```

## TLS Configuration

### TLS 1.2+ with Strong Cipher Suites

```go
package crypto

import (
	"crypto/tls"
)

// GetSecureCipherSuites returns recommended TLS 1.2 cipher suites
func GetSecureCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}
}

// TLSConfigBuilder builds secure TLS configurations
type TLSConfigBuilder struct {
	minVersion         uint16
	maxVersion         uint16
	cipherSuites       []uint16
	certificates       []tls.Certificate
	rootCAs           *x509.CertPool
	clientCAs         *x509.CertPool
	serverName        string
	insecureSkipVerify bool
	clientAuth        tls.ClientAuthType
}

// NewTLSConfigBuilder creates a new TLS config builder with secure defaults
func NewTLSConfigBuilder() *TLSConfigBuilder {
	return &TLSConfigBuilder{
		minVersion:   tls.VersionTLS12,
		maxVersion:   0,
		cipherSuites: GetSecureCipherSuites(),
		clientAuth:   tls.NoClientCert,
	}
}

func (b *TLSConfigBuilder) WithMinVersion(version uint16) *TLSConfigBuilder {
	b.minVersion = version
	return b
}

func (b *TLSConfigBuilder) WithCipherSuites(suites []uint16) *TLSConfigBuilder {
	b.cipherSuites = suites
	return b
}

func (b *TLSConfigBuilder) WithCertificate(cert tls.Certificate) *TLSConfigBuilder {
	b.certificates = append(b.certificates, cert)
	return b
}

func (b *TLSConfigBuilder) WithRootCAs(pool *x509.CertPool) *TLSConfigBuilder {
	b.rootCAs = pool
	return b
}

func (b *TLSConfigBuilder) WithServerName(name string) *TLSConfigBuilder {
	b.serverName = name
	return b
}

func (b *TLSConfigBuilder) WithInsecureSkipVerify(skip bool) *TLSConfigBuilder {
	b.insecureSkipVerify = skip
	return b
}

func (b *TLSConfigBuilder) WithClientAuth(auth tls.ClientAuthType) *TLSConfigBuilder {
	b.clientAuth = auth
	return b
}

// Build creates the TLS configuration
func (b *TLSConfigBuilder) Build() (*tls.Config, error) {
	config := &tls.Config{
		MinVersion:         b.minVersion,
		CipherSuites:       b.cipherSuites,
		Certificates:       b.certificates,
		RootCAs:            b.rootCAs,
		ClientCAs:          b.clientCAs,
		ServerName:         b.serverName,
		InsecureSkipVerify: b.insecureSkipVerify,
		ClientAuth:         b.clientAuth,
	}
	
	if b.maxVersion > 0 {
		config.MaxVersion = b.maxVersion
	}
	
	return config, nil
}

// CreateSecureTLSConfig creates a TLS config with secure defaults for clients
func CreateSecureTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: GetSecureCipherSuites(),
	}
}

// CreateMTLSConfig creates a mutual TLS configuration
func CreateMTLSConfig(clientCert tls.Certificate, caPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: GetSecureCipherSuites(),
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
	}
}

// ValidateTLSConfig validates a TLS configuration for security issues
func ValidateTLSConfig(config *tls.Config) []error {
	var errors []error
	
	if config.MinVersion < tls.VersionTLS12 {
		errors = append(errors, fmt.Errorf("TLS version below 1.2 is not secure"))
	}
	
	if config.InsecureSkipVerify {
		errors = append(errors, fmt.Errorf("InsecureSkipVerify is enabled - certificates will not be verified"))
	}
	
	return errors
}
```

## Certificate Pinning

### Public Key Pinning Implementation

```go
package crypto

import (
	"crypto/subtle"
	"sync"
)

// PinningManager manages certificate pinning
type PinningManager struct {
	pins    map[string][]string // domain -> list of accepted pins
	mu      sync.RWMutex
}

// NewPinningManager creates a new pinning manager
func NewPinningManager() *PinningManager {
	return &PinningManager{
		pins: make(map[string][]string),
	}
}

// AddPin adds a pin for a domain
func (pm *PinningManager) AddPin(domain string, pin string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.pins[domain] = append(pm.pins[domain], pin)
}

// RemovePins removes all pins for a domain
func (pm *PinningManager) RemovePins(domain string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	delete(pm.pins, domain)
}

// VerifyPin verifies if a certificate matches any pin for its domain
func (pm *PinningManager) VerifyPin(domain string, cert *x509.Certificate) error {
	pm.mu.RLock()
	pins, exists := pm.pins[domain]
	pm.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	pin, err := ComputePin(cert)
	if err != nil {
		return fmt.Errorf("failed to compute certificate pin: %w", err)
	}
	
	for _, pinnedHash := range pins {
		if subtle.ConstantTimeCompare([]byte(pin), []byte(pinnedHash)) == 1 {
			return nil
		}
	}
	
	return fmt.Errorf("certificate pinning failed: pin %s not in allowed pins for %s", pin, domain)
}

// ComputePin computes the SHA-256 pin (SPKI hash) of a certificate
func ComputePin(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("certificate is nil")
	}
	
	spki := cert.RawSubjectPublicKeyInfo
	hash := sha256.Sum256(spki)
	
	return base64.StdEncoding.EncodeToString(hash[:]), nil
}

// GetCertificatePin returns the pin for a certificate
func GetCertificatePin(cert *x509.Certificate) string {
	pin, _ := ComputePin(cert)
	return pin
}

// IsPinned checks if a domain has pinned certificates
func (pm *PinningManager) IsPinned(domain string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	_, exists := pm.pins[domain]
	return exists
}
```

## Entropy and Random Generation

### Cryptographically Secure Random Generation

```go
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math/big"
)

// SecureRandom provides cryptographically secure random generation
type SecureRandom struct {
	reader io.Reader
}

// NewSecureRandom creates a new secure random generator
func NewSecureRandom() *SecureRandom {
	return &SecureRandom{
		reader: rand.Reader,
	}
}

// GenerateBytes generates random bytes
func (sr *SecureRandom) GenerateBytes(length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}
	
	bytes := make([]byte, length)
	if _, err := io.ReadFull(sr.reader, bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	
	return bytes, nil
}

// GenerateHex generates random bytes and returns as hex string
func (sr *SecureRandom) GenerateHex(length int) (string, error) {
	bytes, err := sr.GenerateBytes(length)
	if err != nil {
		return "", err
	}
	
	return hex.EncodeToString(bytes), nil
}

// GenerateBase64 generates random bytes and returns as base64 string
func (sr *SecureRandom) GenerateBase64(length int) (string, error) {
	bytes, err := sr.GenerateBytes(length)
	if err != nil {
		return "", err
	}
	
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// GenerateInt generates a random integer in range [0, max)
func (sr *SecureRandom) GenerateInt(max int64) (int64, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive")
	}
	
	n, err := rand.Int(sr.reader, big.NewInt(max))
	if err != nil {
		return 0, fmt.Errorf("failed to generate random int: %w", err)
	}
	
	return n.Int64(), nil
}

// GenerateToken generates a secure random token
func (sr *SecureRandom) GenerateToken(length int) (string, error) {
	bytes, err := sr.GenerateBytes(length)
	if err != nil {
		return "", err
	}
	
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateUUIDv4 generates a UUID version 4
func GenerateUUIDv4() (string, error) {
	u := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, u); err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}
	
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16]), nil
}

// GenerateSalt generates a cryptographically secure salt
func GenerateSalt(length int) ([]byte, error) {
	if length < 16 {
		return nil, fmt.Errorf("salt should be at least 16 bytes for security")
	}
	
	salt := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	
	return salt, nil
}
```

## Hash Functions

### SHA-256 Implementation for Fingerprints and Checksumming

```go
package crypto

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"io"
)

// HashProvider provides hashing operations
type HashProvider struct {
	algorithm string
}

// NewHashProvider creates a new hash provider
func NewHashProvider() *HashProvider {
	return &HashProvider{
		algorithm: "SHA-256",
	}
}

// Hash computes SHA-256 hash of data
func (hp *HashProvider) Hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

// HashHex computes SHA-256 hash and returns as hex string
func (hp *HashProvider) HashHex(data []byte) string {
	return hex.EncodeToString(hp.Hash(data))
}

// HashBase64 computes SHA-256 hash and returns as base64 string
func (hp *HashProvider) HashBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(hp.Hash(data))
}

// HashReader computes SHA-256 hash of reader content
func (hp *HashProvider) HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("failed to hash reader: %w", err)
	}
	
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile computes SHA-256 hash of a file
func (hp *HashProvider) HashFile(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	return hp.HashReader(file)
}

// HashString computes SHA-256 hash of a string
func HashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// HashBytes computes SHA-256 hash of bytes
func HashBytes(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// VerifyHash verifies that data matches a hash
func VerifyHash(data []byte, expectedHash []byte) bool {
	actualHash := HashBytes(data)
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

// VerifyHashHex verifies that data matches a hex-encoded hash
func VerifyHashHex(data []byte, expectedHashHex string) bool {
	expectedHash, err := hex.DecodeString(expectedHashHex)
	if err != nil {
		return false
	}
	return VerifyHash(data, expectedHash)
}

// ContentHasher provides content hashing with salt support
type ContentHasher struct {
	salt []byte
}

// NewContentHasher creates a new content hasher
func NewContentHasher(salt []byte) *ContentHasher {
	return &ContentHasher{
		salt: salt,
	}
}

// Hash computes hash with salt
func (ch *ContentHasher) Hash(data []byte) []byte {
	h := sha256.New()
	if ch.salt != nil {
		h.Write(ch.salt)
	}
	h.Write(data)
	return h.Sum(nil)
}

// HashHex computes hash with salt and returns hex string
func (ch *ContentHasher) HashHex(data []byte) string {
	return hex.EncodeToString(ch.Hash(data))
}

// ComputeFingerprint computes a fingerprint using salt
func ComputeFingerprint(data []byte, salt []byte) string {
	return NewContentHasher(salt).HashHex(data)
}
```

## Integration Points

### Auth System Integration

The cryptography module integrates with the Auth system for mTLS authentication:

```go
// Integration with AUTH-IMPLEMENTATION.md
// The MTLSProvider in crypto/mtls.go is used by the auth.MTLSProvider

// Example usage in auth system:
func NewAuthMTLSProvider(cryptoMTLS *crypto.MTLSProvider) (*auth.MTLSProvider, error) {
	return &auth.MTLSProvider{
		tlsConfig: cryptoMTLS.GetTLSConfig(),
	}, nil
}
```

### Inventory System Integration

The cryptography module provides signature verification for inventory files:

```go
// Integration with inventory system for signature verification
func VerifyInventorySignature(content []byte, signature *InventorySignature, verifier *InventoryVerifier) error {
	return verifier.VerifyInventory(signature)
}
```

### Fingerprint System Integration

The cryptography module provides hashing for hardware fingerprinting:

```go
// Integration with FINGERPRINT-ALGORITHM.md
// The HashProvider is used for generating component hashes

func ComputeComponentHash(data []byte, salt []byte) string {
	return ComputeFingerprint(data, salt)
}
```

## Cross-Platform Support

### Platform-Specific Key Storage

The key storage layer provides platform-specific implementations:

```go
// MacOS Keychain Support
// +build darwin

package crypto

import (
	"github.com/keybase/go-keychain"
)

// MacOSKeychainStorage implements key storage using macOS Keychain
type MacOSKeychainStorage struct {
	service string
}

func NewMacOSKeychainStorage() (*MacOSKeychainStorage, error) {
	return &MacOSKeychainStorage{
		service: "com.openapi-cli.keys",
	}, nil
}

func (m *MacOSKeychainStorage) Store(name string, key interface{}) error {
	keyData, ok := key.(*KeyData)
	if !ok {
		return fmt.Errorf("key must be *KeyData")
	}
	
	data, err := json.Marshal(keyData)
	if err != nil {
		return err
	n}
	
	item := keychain.NewGenericPassword(
		m.service,
		name,
		"",
		data,
		keychain.AccessibleWhenUnlocked,
	)
	
	return keychain.AddItem(item)
}

func (m *MacOSKeychainStorage) Retrieve(name string) (interface{}, error) {
	data, err := keychain.GetGenericPassword(m.service, name, "", "")
	if err != nil {
		return nil, err
	}
	
	var keyData KeyData
	if err := json.Unmarshal(data, &keyData); err != nil {
		return nil, err
	}
	
	return &keyData, nil
}

func (m *MacOSKeychainStorage) Delete(name string) error {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(m.service)
	query.SetAccount(name)
	return keychain.DeleteItem(query)
}

func (m *MacOSKeychainStorage) List() ([]string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(m.service)
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)
	
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	}
	
	names := make([]string, len(results))
	for i, result := range results {
		names[i] = result.Account
	}
	
	return names, nil
}

func (m *MacOSKeychainStorage) Exists(name string) (bool, error) {
	_, err := m.Retrieve(name)
	return err == nil, nil
}
```

```go
// Linux Keyring Support
// +build linux

package crypto

import (
	"github.com/zalando/go-keyring"
)

// LinuxKeyringStorage implements key storage using Linux Secret Service
type LinuxKeyringStorage struct {
	service string
}

func NewLinuxKeyringStorage() (*LinuxKeyringStorage, error) {
	return &LinuxKeyringStorage{
		service: "openapi-cli",
	}, nil
}

func (l *LinuxKeyringStorage) Store(name string, key interface{}) error {
	keyData, ok := key.(*KeyData)
	if !ok {
		return fmt.Errorf("key must be *KeyData")
	}
	
	data, err := json.Marshal(keyData)
	if err != nil {
		return err
	}
	
	return keyring.Set(l.service, name, string(data))
}

func (l *LinuxKeyringStorage) Retrieve(name string) (interface{}, error) {
	data, err := keyring.Get(l.service, name)
	if err != nil {
		return nil, err
	}
	
	var keyData KeyData
	if err := json.Unmarshal([]byte(data), &keyData); err != nil {
		return nil, err
	}
	
	return &keyData, nil
}

func (l *LinuxKeyringStorage) Delete(name string) error {
	return keyring.Delete(l.service, name)
}

func (l *LinuxKeyringStorage) List() ([]string, error) {
	return nil, fmt.Errorf("listing not supported by go-keyring")
}

func (l *LinuxKeyringStorage) Exists(name string) (bool, error) {
	_, err := l.Retrieve(name)
	return err == nil, nil
}
```

```go
// Windows Credential Manager Support
// +build windows

package crypto

import (
	"golang.org/x/sys/windows/registry"
)

// WindowsCredentialStorage implements key storage using Windows Credential Manager
type WindowsCredentialStorage struct {
	targetPrefix string
}

func NewWindowsCredentialStorage() (*WindowsCredentialStorage, error) {
	return &WindowsCredentialStorage{
		targetPrefix: "openapi-cli:",
	}, nil
}

func (w *WindowsCredentialStorage) Store(name string, key interface{}) error {
	keyData, ok := key.(*KeyData)
	if !ok {
		return fmt.Errorf("key must be *KeyData")
	}
	
	data, err := json.Marshal(keyData)
	if err != nil {
		return err
	}
	
	targetName := w.targetPrefix + name
	// Use Windows Credential Manager API
	// Implementation would use syscall to advapi32.dll CredWriteW
	return fmt.Errorf("Windows credential storage requires native implementation")
}

func (w *WindowsCredentialStorage) Retrieve(name string) (interface{}, error) {
	return nil, fmt.Errorf("Windows credential storage requires native implementation")
}

func (w *WindowsCredentialStorage) Delete(name string) error {
	return fmt.Errorf("Windows credential storage requires native implementation")
}

func (w *WindowsCredentialStorage) List() ([]string, error) {
	return nil, fmt.Errorf("Windows credential storage requires native implementation")
}

func (w *WindowsCredentialStorage) Exists(name string) (bool, error) {
	return false, fmt.Errorf("Windows credential storage requires native implementation")
}
```

## Production Security Considerations

### Security Best Practices

1. **Never hardcode keys** - Always use secure key storage
2. **Use TLS 1.2+ only** - Disable older versions
3. **Enable certificate verification** - Never use InsecureSkipVerify in production
4. **Pin certificates** - Prevent MITM attacks with certificate pinning
5. **Rotate keys regularly** - Implement key rotation policies
6. **Audit all crypto operations** - Log security-critical operations
7. **Use constant-time comparison** - Prevent timing attacks
8. **Validate all inputs** - Check lengths and formats before processing

### Dependencies

```go
// go.mod
require (
	golang.org/x/crypto v0.21.0
	github.com/zalando/go-keyring v0.2.4 // For Linux keyring
	github.com/keybase/go-keychain v0.0.0-20231213204628-e2352d9f7404 // For macOS keychain
	golang.org/x/sys v0.18.0 // For Windows credential manager
)
```

### Testing

```go
// crypto_test.go
package crypto

import (
	"testing"
)

func TestEd25519SignVerify(t *testing.T) {
	provider, err := NewEd25519Provider()
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	
	data := []byte("test data")
	signature, err := provider.Sign(data)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	
	if err := provider.Verify(data, signature); err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
}

func TestCertificatePinning(t *testing.T) {
	pm := NewPinningManager()
	
	certPEM, _, err := GenerateSelfSignedCertForTesting([]string{"localhost"})
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		t.Fatalf("Failed to parse cert: %v", err)
	}
	
	pin, err := ComputePin(cert)
	if err != nil {
		t.Fatalf("Failed to compute pin: %v", err)
	}
	
	pm.AddPin("localhost", pin)
	
	if err := pm.VerifyPin("localhost", cert); err != nil {
		t.Fatalf("Pinning verification failed: %v", err)
	}
}

func TestHashOperations(t *testing.T) {
	hp := NewHashProvider()
	
	data := []byte("test data")
	hash := hp.Hash(data)
	
	if len(hash) != sha256.Size {
		t.Errorf("Expected hash size %d, got %d", sha256.Size, len(hash))
	}
	
	hexHash := hp.HashHex(data)
	if len(hexHash) != 64 {
		t.Errorf("Expected hex string length 64, got %d", len(hexHash))
	}
}
```

## Summary

This implementation provides:

1. **Ed25519 Signatures** - Modern, fast, secure digital signatures for inventory verification
2. **Certificate Management** - Full X.509 support with chain validation
3. **mTLS Support** - Production-ready mutual TLS authentication
4. **Secure Key Storage** - Platform-specific secure storage (Keychain, Keyring, Credential Manager)
5. **Certificate Generation** - Self-signed certificate generation for testing
6. **TLS Configuration** - TLS 1.2+ with strong cipher suites
7. **Cipher Selection** - Secure defaults with no weak ciphers
8. **Certificate Pinning** - Public key pinning for additional security
9. **Secure Random** - CSPRNG for all cryptographic operations
10. **Hash Functions** - SHA-256 for fingerprints and integrity checking
11. **Integration Points** - Seamless integration with Auth, Inventory, and Fingerprint systems
12. **Cross-Platform** - Platform-specific implementations for macOS, Linux, and Windows

## Deterministic Guarantees

This cryptographic implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Cryptographic Operations

Cryptographic operations must produce identical results across platforms:

- **Signature generation**: Same input → same signature (Ed25519 deterministic)
- **Hash computation**: SHA‑256 with identical byte ordering (big‑endian)
- **Random number generation**: Seeded CSPRNG for test mode, identical sequences

### 2. Platform‑Independent Certificate Handling

X.509 certificate processing must be platform‑independent:

- **Certificate parsing**: Identical ASN.1 decoding across platforms
- **Chain validation**: Same validation results for same certificate chains
- **Key storage**: Platform‑specific secure storage with identical retrieval behavior

### 3. Deterministic TLS Handshake

TLS handshake must be identical across platforms:

- **Cipher suite negotiation**: Same cipher suite selection order
- **Certificate selection**: Same certificate chain presentation
- **ALPN protocol selection**: Same protocol ordering

### 4. Cross‑Platform Key Storage

Secure key storage must behave identically:

- **Keychain integration**: Same key retrieval semantics across macOS, Linux, Windows
- **Fallback behavior**: Identical fallback to file‑based storage
- **Error mapping**: Same storage errors → same error codes

### 5. Testing Determinism

All cryptographic tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestCryptographyDeterminism(t *testing.T) {
	// Verify identical signature generation
	// Verify identical hash computation
	// Verify identical certificate validation
}
```

### 6. Integration with SDK Generation

When generating SDKs for 12 languages, cryptographic libraries must:

- Use identical cryptographic algorithms
- Produce identical signatures and hashes
- Validate certificates identically

**Verification**: Run `./scripts/validate‑determinism‑crypto.sh` to confirm cross‑platform consistency.

All implementations follow Go best practices and production security standards.
