# Certificate & Key Rotation

## Overview

Procedures for rotating mTLS certificates, inventory signing keys, and key server keys without breaking clients.

## 1. mTLS Certificate Rotation

### Rotation Strategy: Overlapping Validity

```
Timeline:
Day 0   Day 30  Day 60  Day 90
  |-------|-------|-------|
Cert A: |-----------------| (valid for 90 days)
            |-----------------| Cert B (issued at day 30)
                |-----------------| Cert C (issued at day 60)
```

**Principle**: Always have at least two valid certificates during rotation.

### Client Implementation

```go
type MTLSConfig struct {
    CurrentCert *tls.Certificate
    NextCert    *tls.Certificate  // Pre-fetched rotation cert
    RenewBefore time.Duration      // Renew 30 days before expiry
}

func (m *MTLSConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
    // Check if current cert is expiring soon
    if m.needsRenewal() && m.NextCert != nil {
        // Atomically swap to next cert
        m.CurrentCert = m.NextCert
        m.NextCert = nil
        
        log.Info("Rotated to new mTLS certificate",
            "not_after", m.CurrentCert.Leaf.NotAfter)
    }
    
    return m.CurrentCert, nil
}

func (m *MTLSConfig) needsRenewal() bool {
    if m.CurrentCert == nil || m.CurrentCert.Leaf == nil {
        return true
    }
    
    return time.Until(m.CurrentCert.Leaf.NotAfter) < m.RenewBefore
}
```

### Automatic Renewal

```go
func (ic *InventoryClient) StartCertRenewalWatcher(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour) // Check daily
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            ic.checkAndRenewCerts(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (ic *InventoryClient) checkAndRenewCerts(ctx context.Context) {
    // Check mTLS cert
    if ic.mtlsConfig.needsRenewal() {
        log.Info("mTLS certificate expiring soon, fetching new cert")
        
        newCert, err := ic.fetchNewCertificate(ctx)
        if err != nil {
            log.Error("Failed to fetch new certificate", "error", err)
            return
        }
        
        // Validate new certificate
        if err := ic.validateCertificate(newCert); err != nil {
            log.Error("New certificate validation failed", "error", err)
            return
        }
        
        // Stage for rotation
        ic.mtlsConfig.NextCert = newCert
        
        audit.Log("CERTIFICATE_STAGED", map[string]interface{}{
            "type": "mtls",
            "not_after": newCert.Leaf.NotAfter,
        })
    }
}
```

### Server-Side Certificate Provisioning

```go
// Inventory provides both current and next certificates
type CertificateResponse struct {
    Current CertificateData `json:"current"`
    Next    *CertificateData `json:"next,omitempty"` // Staged for rotation
}

type CertificateData struct {
    Certificate string    `json:"certificate"` // PEM encoded
    Key         string    `json:"key"`         // PEM encoded (encrypted)
    NotBefore   time.Time `json:"not_before"`
    NotAfter    time.Time `json:"not_after"`
    Serial      string    `json:"serial"`
}

// Client decrypts key using OS keychain
func (ic *InventoryClient) decryptKey(encryptedKey string) ([]byte, error) {
    keyEncryptionKey, err := ic.secretManager.Get("key_encryption_key")
    if err != nil {
        return nil, err
    }
    
    return crypto.DecryptAES(encryptedKey, keyEncryptionKey)
}
```

## 2. Inventory Signing Key Rotation

### Key Hierarchy

```
Root Key (offline, HSM)
    ↓
Intermediate Key (online, rotated quarterly)
    ↓
Inventory Signing Key (rotated monthly)
```

### Rotation Procedure

```go
type SigningKeyRotation struct {
    OldKey      *SigningKey // Still valid during rotation
    NewKey      *SigningKey // Newly activated
    ValidFrom   time.Time
    ValidUntil  time.Time
    GracePeriod time.Duration // 7 days
}

type SigningKey struct {
    ID        string    `json:"id"`
    PublicKey []byte    `json:"public_key"`
    Algorithm string    `json:"algorithm"` // ecdsa-p256, ed25519
    NotBefore time.Time `json:"not_before"`
    NotAfter  time.Time `json:"not_after"`
}
```

### Client Handling

```go
func (ic *InventoryClient) verifyInventorySignature(inv []byte, sig Signature) error {
    // Try new key first
    if err := ic.verifyWithKey(inv, sig, ic.signingKeys.NewKey); err == nil {
        return nil
    }
    
    // Fall back to old key during rotation
    if ic.signingKeys.OldKey != nil {
        if err := ic.verifyWithKey(inv, sig, ic.signingKeys.OldKey); err == nil {
            log.Warn("Inventory signed with deprecated key",
                "key_id", ic.signingKeys.OldKey.ID)
            return nil
        }
    }
    
    return fmt.Errorf("signature verification failed with all keys")
}

func (ic *InventoryClient) updateSigningKeys(keys SigningKeyRotation) {
    ic.signingKeys = keys
    
    // Schedule cleanup of old key after grace period
    if keys.OldKey != nil {
        time.AfterFunc(keys.GracePeriod, func() {
            if time.Now().After(keys.OldKey.NotAfter) {
                ic.signingKeys.OldKey = nil
                log.Info("Removed deprecated signing key",
                    "key_id", keys.OldKey.ID)
            }
        })
    }
}
```

### Public Key Distribution

```json
{
  "signing_keys": {
    "current": {
      "id": "key-2024-03",
      "public_key": "-----BEGIN PUBLIC KEY-----...",
      "algorithm": "ecdsa-p256",
      "not_before": "2024-03-01T00:00:00Z",
      "not_after": "2024-04-01T00:00:00Z"
    },
    "previous": {
      "id": "key-2024-02",
      "public_key": "-----BEGIN PUBLIC KEY-----...",
      "not_after": "2024-03-15T00:00:00Z"  // Grace period
    }
  }
}
```

## 3. Key Server Key Rotation

### Rotation Flow

```
1. Generate new key pair
2. Distribute new public key to clients (via inventory)
3. Re-encrypt all credentials with new key
4. Support both keys for grace period
5. Retire old key
```

### Client-Side Key Handling

```go
type KeyServerClient struct {
    CurrentKeyID string
    Keys         map[string][]byte // key_id -> decryption key
}

func (ksc *KeyServerClient) decryptCredential(encryptedData []byte, keyID string) ([]byte, error) {
    key, ok := ksc.Keys[keyID]
    if !ok {
        // Try to fetch key from key server
        key, err := ksc.fetchKey(ctx, keyID)
        if err != nil {
            return nil, fmt.Errorf("key %s not available: %w", keyID, err)
        }
        ksc.Keys[keyID] = key
    }
    
    return crypto.DecryptAES(encryptedData, key)
}

func (ksc *KeyServerClient) handleKeyRotation(newKeyID string, newKey []byte) {
    // Store new key
    ksc.Keys[newKeyID] = newKey
    ksc.CurrentKeyID = newKeyID
    
    // Mark old keys for cleanup after grace period
    for keyID := range ksc.Keys {
        if keyID != newKeyID {
            ksc.scheduleKeyCleanup(keyID, 7*24*time.Hour)
        }
    }
}
```

## 4. Trust On First Use (TOFU) with Rotation

### Initial Trust

```go
type TOFUConfig struct {
    TrustedKeys     map[string]*SigningKey
    UntrustedPolicy string // "prompt", "reject", "accept_once"
}

func (ic *InventoryClient) handleUnknownKey(key *SigningKey) error {
    switch ic.tofuConfig.UntrustedPolicy {
    case "reject":
        return fmt.Errorf("unknown signing key: %s (rejected by policy)", key.ID)
        
    case "accept_once":
        // Accept but don't persist
        return nil
        
    case "prompt":
        // Interactive approval (CLI only)
        if ic.isInteractive {
            approved := ic.promptUser(fmt.Sprintf(
                "Unknown signing key: %s\n" +
                "Algorithm: %s\n" +
                "Valid from: %s\n" +
                "Valid until: %s\n\n" +
                "Trust this key? [y/N/show]",
                key.ID, key.Algorithm, key.NotBefore, key.NotAfter,
            ))
            
            if approved {
                ic.tofuConfig.TrustedKeys[key.ID] = key
                return ic.saveTrustedKeys()
            }
        }
        return fmt.Errorf("unknown signing key rejected")
    }
    
    return fmt.Errorf("unknown TOFU policy: %s", ic.tofuConfig.UntrustedPolicy)
}
```

### Key Pinning with Rotation

```yaml
# ~/.config/openapi-cli/trust.yaml
trusted_keys:
  - host: api.example.com
    pinned_keys:
      - id: key-2024-03
        public_key: "base64..."
      - id: key-2024-02  # Previous key (grace period)
        public_key: "base64..."
    auto_accept_rotations: true  # Accept rotations from same host
    require_manual_approval: false
```

## 5. Rotation Monitoring

### Metrics

```go
// Certificate expiry monitoring
certExpiryGauge := prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "openapi_cli_cert_expiry_days",
        Help: "Days until certificate expiry",
    },
    []string{"type", "host"},
)

// Rotation events
rotationCounter := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "openapi_cli_key_rotations_total",
        Help: "Total number of key rotations",
    },
    []string{"type", "status"},
)

// Alert when cert expires in < 7 days
certExpiryGauge.WithLabelValues("mtls", "api.example.com").Set(daysUntilExpiry)
if daysUntilExpiry < 7 {
    alert("Certificate expiring soon: %.1f days", daysUntilExpiry)
}
```

### Alerts

```yaml
alerts:
  - name: CertificateExpiringSoon
    condition: cert_expiry_days < 7
    severity: warning
    message: "Certificate expires in {{ $value }} days"
    
  - name: CertificateExpired
    condition: cert_expiry_days < 0
    severity: critical
    message: "Certificate has expired!"
    
  - name: RotationFailed
    condition: rotation_status == "failed"
    severity: warning
    message: "Key rotation failed: {{ $reason }}"
```

## 6. Emergency Rotation

### Compromised Key Response

```go
func (ic *InventoryClient) handleCompromisedKey(compromisedKeyID string) {
    log.Error("COMPROMISED KEY DETECTED",
        "key_id", compromisedKeyID,
        "action", "emergency_rotation",
    )
    
    // Immediately invalidate compromised key
    delete(ic.signingKeys, compromisedKeyID)
    
    // Force inventory refresh
    ic.inventoryCache.Clear()
    
    // Fetch new keys from well-known endpoint
    newKeys, err := ic.fetchKeysFromWellKnownEndpoint()
    if err != nil {
        log.Error("Failed to fetch new keys", "error", err)
        // Fall back to manual key input
        return
    }
    
    // Verify new keys against out-of-band source
    if !ic.verifyKeysOutOfBand(newKeys) {
        log.Error("New keys failed out-of-band verification")
        return
    }
    
    // Update trusted keys
    ic.updateSigningKeys(newKeys)
    
    audit.Log("EMERGENCY_KEY_ROTATION", map[string]interface{}{
        "compromised_key": compromisedKeyID,
        "new_key": newKeys.Current.ID,
        "timestamp": time.Now(),
    })
}
```

### Well-Known Key Endpoint

```
GET https://api.example.com/.well-known/openapi-cli/keys.json
```

**Purpose**: Emergency key distribution separate from inventory.

```json
{
  "keys": [
    {
      "id": "emergency-2024-03",
      "public_key": "...",
      "not_before": "2024-03-01T00:00:00Z",
      "source": "out-of-band-verified"
    }
  ],
  "revoked": ["compromised-key-id"]
}
```

## 7. Testing Rotation

```go
func TestCertificateRotation(t *testing.T) {
    // Create certs with overlapping validity
    oldCert := generateCert(notAfter: time.Now().Add(30*24*time.Hour))
    newCert := generateCert(notBefore: time.Now(), notAfter: time.Now().Add(90*24*time.Hour))
    
    config := &MTLSConfig{
        CurrentCert: oldCert,
        NextCert:    newCert,
        RenewBefore: 7 * 24 * time.Hour,
    }
    
    // Before renewal - should use old cert
    cert, _ := config.GetCertificate(nil)
    assert.Equal(t, oldCert, cert)
    
    // Simulate time passing (past renewal threshold)
    config.CurrentCert.Leaf.NotAfter = time.Now().Add(6 * 24 * time.Hour)
    
    // After renewal - should use new cert
    cert, _ = config.GetCertificate(nil)
    assert.Equal(t, newCert, cert)
}

func TestSigningKeyRotation(t *testing.T) {
    oldKey := &SigningKey{ID: "key-2024-02", NotAfter: time.Now().Add(7 * 24 * time.Hour)}
    newKey := &SigningKey{ID: "key-2024-03", NotAfter: time.Now().Add(30 * 24 * time.Hour)}
    
    rotation := SigningKeyRotation{
        OldKey:      oldKey,
        NewKey:      newKey,
        GracePeriod: 7 * 24 * time.Hour,
    }
    
    client := NewTestClient(rotation)
    
    // Should accept both keys during rotation
    assert.NoError(t, client.verifyWithKey(oldKey))
    assert.NoError(t, client.verifyWithKey(newKey))
    
    // After grace period, old key should be rejected
    time.Sleep(rotation.GracePeriod + time.Second)
    assert.Error(t, client.verifyWithKey(oldKey))
}
```

## Summary

- **mTLS Certs**: Overlapping validity, auto-renew 30 days before expiry
- **Signing Keys**: Grace period (7 days), accept both old and new
- **Key Server Keys**: Graceful transition, cleanup after rotation
- **TOFU**: Configurable policy, interactive approval option
- **Monitoring**: Expiry alerts, rotation events tracked
- **Emergency**: Well-known key endpoint, out-of-band verification

This ensures continuous operation during key rotations, with security maintained throughout the transition.
