# Inventory Revocation & Emergency Response

## Overview

Mechanism for revoking compromised inventory versions, blacklisting malicious modules, and emergency kill-switch capabilities.

## 1. Revocation List Endpoint

### Endpoint Specification

```
GET https://inventory.example.com/_revocation.json
```

**Response Format:**

```json
{
  "updated_at": "2026-02-28T15:30:00Z",
  "revocations": {
    "inventories": [
      {
        "host": "api.example.com",
        "version": "1.2.3",
        "reason": "compromised_signing_key",
        "severity": "critical",
        "revoked_at": "2026-02-28T14:00:00Z"
      }
    ],
    "modules": [
      {
        "sha256": "abc123...",
        "reason": "malicious_code_detected",
        "severity": "critical",
        "revoked_at": "2026-02-28T13:00:00Z"
      }
    ],
    "fingerprints": [
      {
        "fingerprint": "fp_abc123...",
        "reason": "suspicious_activity",
        "severity": "high",
        "revoked_at": "2026-02-28T12:00:00Z"
      }
    ]
  },
  "emergency": {
    "enabled": false,
    "message": "Service temporarily disabled",
    "contact": "security@example.com"
  }
}
```

**HTTP Headers:**
- `Cache-Control: max-age=60` (frequently updated)
- `ETag` for conditional requests

### Client Behavior

```go
type RevocationChecker struct {
    client       *http.Client
    cache        *RevocationCache
    lastCheck    time.Time
    checkInterval time.Duration
}

func (rc *RevocationChecker) CheckRevocations(ctx context.Context) (*RevocationList, error) {
    // Check if cache is stale
    if time.Since(rc.lastCheck) < rc.checkInterval {
        return rc.cache.Get(), nil
    }
    
    // Fetch revocation list
    req, _ := http.NewRequestWithContext(ctx, "GET", rc.revocationURL, nil)
    req.Header.Set("If-None-Match", rc.cache.GetETag())
    
    resp, err := rc.client.Do(req)
    if err != nil {
        // Use cached list if fetch fails
        return rc.cache.Get(), nil
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNotModified {
        rc.lastCheck = time.Now()
        return rc.cache.Get(), nil
    }
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("revocation list fetch failed: %d", resp.StatusCode)
    }
    
    // Parse and cache
    var list RevocationList
    if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
        return nil, err
    }
    
    rc.cache.Set(&list, resp.Header.Get("ETag"))
    rc.lastCheck = time.Now()
    
    return &list, nil
}
```

## 2. Revocation Checks

### Periodic Checks

```go
func (ic *InventoryClient) StartRevocationWatcher(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            ic.checkRevocations(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (ic *InventoryClient) checkRevocations(ctx context.Context) {
    list, err := ic.revocationChecker.CheckRevocations(ctx)
    if err != nil {
        log.Error("Failed to check revocations", "error", err)
        return
    }
    
    // Check if current inventory is revoked
    for _, inv := range list.Revocations.Inventories {
        if inv.Host == ic.currentHost && inv.Version == ic.currentVersion {
            ic.handleRevokedInventory(inv)
        }
    }
    
    // Check if any cached modules are revoked
    for _, mod := range list.Revocations.Modules {
        if ic.moduleCache.Has(mod.SHA256) {
            ic.handleRevokedModule(mod)
        }
    }
    
    // Check emergency mode
    if list.Emergency.Enabled {
        ic.handleEmergencyMode(list.Emergency)
    }
}
```

### Immediate Enforcement

```go
func (ic *InventoryClient) handleRevokedInventory(rev RevokedInventory) {
    log.Error("INVENTORY REVOKED",
        "host", rev.Host,
        "version", rev.Version,
        "reason", rev.Reason,
        "severity", rev.Severity,
    )
    
    switch rev.Severity {
    case "critical":
        // Immediately disable all operations
        ic.disableOperations(fmt.Sprintf(
            "CRITICAL: Inventory %s@%s has been revoked. Reason: %s. " +
            "All operations disabled for security. Contact: security@example.com",
            rev.Host, rev.Version, rev.Reason,
        ))
        
    case "high":
        // Block new operations, allow in-flight to complete
        ic.blockNewOperations(fmt.Sprintf(
            "WARNING: Inventory %s@%s has been revoked. Reason: %s. " +
            "New operations blocked. Update required.",
            rev.Host, rev.Version, rev.Reason,
        ))
        
    case "medium", "low":
        // Show warning but allow operations
        ic.showWarning(fmt.Sprintf(
            "NOTICE: Inventory %s@%s has been flagged. Reason: %s. " +
            "Please update when convenient.",
            rev.Host, rev.Version, rev.Reason,
        ))
    }
}

func (ic *InventoryClient) handleRevokedModule(rev RevokedModule) {
    log.Error("MODULE REVOKED",
        "sha256", rev.SHA256,
        "reason", rev.Reason,
    )
    
    // Remove from cache
    ic.moduleCache.Delete(rev.SHA256)
    
    // Block future use
    ic.revokedModules[rev.SHA256] = rev
}

func (ic *InventoryClient) handleEmergencyMode(emerg EmergencyMode) {
    log.Error("EMERGENCY MODE ACTIVE",
        "message", emerg.Message,
        "contact", emerg.Contact,
    )
    
    ic.disableOperations(fmt.Sprintf(
        "EMERGENCY: %s. Contact: %s",
        emerg.Message, emerg.Contact,
    ))
}
```

## 3. Kill Switch

### Emergency Kill Switch

```go
type KillSwitch struct {
    Enabled   bool      `json:"enabled"`
    Message   string    `json:"message"`
    Contact   string    `json:"contact"`
    Timestamp time.Time `json:"timestamp"`
    Signature string    `json:"signature"` // Signed by offline key
}

func (ic *InventoryClient) checkKillSwitch(ctx context.Context) error {
    // Fetch kill switch status
    resp, err := ic.client.Get(ctx, ic.killSwitchURL)
    if err != nil {
        // Don't fail on network error - use cached state
        return nil
    }
    defer resp.Body.Close()
    
    var ks KillSwitch
    if err := json.NewDecoder(resp.Body).Decode(&ks); err != nil {
        return err
    }
    
    // Verify signature
    if !ic.verifyKillSwitchSignature(ks) {
        return fmt.Errorf("invalid kill switch signature")
    }
    
    if ks.Enabled {
        return fmt.Errorf("KILL SWITCH ACTIVE: %s (Contact: %s)",
            ks.Message, ks.Contact)
    }
    
    return nil
}

func (ic *InventoryClient) verifyKillSwitchSignature(ks KillSwitch) bool {
    // Verify against embedded public key
    // This should use offline/backup key, separate from inventory signing
    data := fmt.Sprintf("%t|%s|%s|%d",
        ks.Enabled, ks.Message, ks.Contact, ks.Timestamp.Unix())
    
    return crypto.Verify(
        ic.killSwitchPublicKey,
        []byte(data),
        []byte(ks.Signature),
    )
}
```

### Offline Key Storage

```go
// Embedded in binary at build time
// Separate from inventory signing keys
var killSwitchPublicKey = []byte(`
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
-----END PUBLIC KEY-----
`)

// Private key stored offline (HSM, air-gapped)
// Only used in emergency
```

## 4. Client-Side Revocation Cache

```go
type RevocationCache struct {
    mu       sync.RWMutex
    list     *RevocationList
    etag     string
    fetchedAt time.Time
    ttl      time.Duration
}

func (rc *RevocationCache) Set(list *RevocationList, etag string) {
    rc.mu.Lock()
    defer rc.mu.Unlock()
    
    rc.list = list
    rc.etag = etag
    rc.fetchedAt = time.Now()
}

func (rc *RevocationCache) Get() *RevocationList {
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    return rc.list
}

func (rc *RevocationCache) IsStale() bool {
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    return time.Since(rc.fetchedAt) > rc.ttl
}

// Persistent storage for offline operation
func (rc *RevocationCache) Save(path string) error {
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    data, err := json.Marshal(rc.list)
    if err != nil {
        return err
    }
    
    return os.WriteFile(path, data, 0600)
}

func (rc *RevocationCache) Load(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    
    var list RevocationList
    if err := json.Unmarshal(data, &list); err != nil {
        return err
    }
    
    rc.Set(&list, "")
    return nil
}
```

## 5. Operational Procedures

### Revocation Workflow

```
1. Security incident detected
   ↓
2. Decision to revoke inventory/module
   ↓
3. Add to revocation list
   ↓
4. Sign revocation list with offline key
   ↓
5. Deploy to revocation endpoint
   ↓
6. Clients check within 5 minutes
   ↓
7. Revoked inventory/modules blocked
```

### Emergency Response

```go
// Emergency override (for testing/recovery)
func (ic *InventoryClient) EmergencyOverride(adminKey string) error {
    // Verify admin key (different from signing keys)
    if !ic.verifyAdminKey(adminKey) {
        return fmt.Errorf("invalid admin key")
    }
    
    // Temporarily disable revocation checks
    ic.revocationChecker.Disable()
    
    // Log override
    audit.Log("EMERGENCY_OVERRIDE", map[string]interface{}{
        "timestamp": time.Now(),
        "admin_key_hash": hash(adminKey),
    })
    
    return nil
}
```

## 6. User Experience

### Revocation Notifications

```bash
# Critical revocation - operations blocked
$ openapi-cli users list
ERROR: CRITICAL SECURITY ALERT
The current inventory (api.example.com@1.2.3) has been revoked.
Reason: compromised_signing_key
All operations have been disabled.

Contact: security@example.com
Status: https://status.example.com

To check for updates:
  openapi-cli update --check

# High severity - new operations blocked
WARNING: Inventory api.example.com@1.2.3 has been revoked.
Reason: security_patch_required
New operations blocked. Please update:
  openapi-cli update

# Medium severity - warning only
NOTICE: Inventory api.example.com@1.2.3 has been flagged.
Reason: deprecated_api_usage
Please update when convenient:
  openapi-cli update
```

### Audit Logging

```go
// All revocation events logged
audit.Log("REVOCATION_CHECK", map[string]interface{}{
    "timestamp": time.Now(),
    "host": ic.currentHost,
    "version": ic.currentVersion,
    "revocations_found": len(revoked),
    "action_taken": action,
})
```

## 7. Testing

```go
func TestRevocationHandling(t *testing.T) {
    // Test critical revocation
    t.Run("critical_revocation", func(t *testing.T) {
        mockRevocationList := &RevocationList{
            Revocations: Revocations{
                Inventories: []RevokedInventory{
                    {
                        Host:     "test.example.com",
                        Version:  "1.0.0",
                        Reason:   "test_revocation",
                        Severity: "critical",
                    },
                },
            },
        }
        
        client := NewTestClient(mockRevocationList)
        err := client.ExecuteCommand("users list")
        
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "revoked")
    })
    
    // Test kill switch
    t.Run("kill_switch", func(t *testing.T) {
        mockKillSwitch := KillSwitch{
            Enabled: true,
            Message: "Emergency maintenance",
        }
        
        client := NewTestClientWithKillSwitch(mockKillSwitch)
        err := client.ExecuteCommand("users list")
        
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "KILL SWITCH")
    })
}
```

## Summary

- **Revocation List**: Periodic checks (5 min), cached for offline
- **Severity Levels**: Critical (block), High (block new), Medium/Low (warn)
- **Kill Switch**: Emergency stop with offline key signature
- **Client Behavior**: Automatic enforcement, clear error messages
- **Audit**: All revocation events logged
- **Override**: Admin key for emergency recovery

This ensures compromised inventories/modules can be quickly revoked, protecting all clients automatically.
