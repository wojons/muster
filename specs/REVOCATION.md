# Token Revocation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Security

## Overview

Complete specification of **token revocation** for OAuth2 and API key management.

## 1. Revocation Architecture

```go
type RevocationManager struct {
    store RevocationStore
}

type RevocationStore interface {
    Revoke(token string, expiresAt time.Time) error
    IsRevoked(token string) (bool, error)
    Cleanup(before time.Time) error
}
```

## 2. Revocation Methods

### 2.1 OAuth2 Token Revocation

```go
func (rm *RevocationManager) RevokeOAuth2Token(token, tokenType, revocationURL string) error {
    data := url.Values{
        "token":           {token},
        "token_type_hint": {tokenType},
    }
    
    resp, err := http.PostForm(revocationURL, data)
    if err != nil {
        return fmt.Errorf("revocation request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("revocation failed: %d", resp.StatusCode)
    }
    
    // Also mark as revoked locally
    return rm.store.Revoke(token, time.Now().Add(24*time.Hour))
}
```

### 2.2 API Key Revocation

```go
func (rm *RevocationManager) RevokeAPIKey(key string) error {
    return rm.store.Revoke(hashKey(key), time.Now().Add(365*24*time.Hour))
}
```

## 3. Revocation Store

```go
type MemoryRevocationStore struct {
    revoked map[string]time.Time
    mu      sync.RWMutex
}

func (s *MemoryRevocationStore) Revoke(token string, expiresAt time.Time) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    s.revoked[token] = expiresAt
    return nil
}

func (s *MemoryRevocationStore) IsRevoked(token string) (bool, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    expiresAt, exists := s.revoked[token]
    if !exists {
        return false, nil
    }
    
    // Auto-cleanup expired entries
    if time.Now().After(expiresAt) {
        return false, nil
    }
    
    return true, nil
}
```

## 4. Redis Revocation Store

```go
type RedisRevocationStore struct {
    client *redis.Client
}

func (s *RedisRevocationStore) Revoke(token string, expiresAt time.Time) error {
    ttl := time.Until(expiresAt)
    if ttl <= 0 {
        ttl = 24 * time.Hour
    }
    
    return s.client.Set(
        context.Background(),
        "revoked:"+token,
        "1",
        ttl,
    ).Err()
}

func (s *RedisRevocationStore) IsRevoked(token string) (bool, error) {
    exists, err := s.client.Exists(
        context.Background(),
        "revoked:"+token,
    ).Result()
    
    return exists > 0, err
}
```

## 5. Token Validation with Revocation

```go
func (rm *RevocationManager) ValidateToken(token string) error {
    // Check if revoked
    revoked, err := rm.store.IsRevoked(token)
    if err != nil {
        return fmt.Errorf("revocation check failed: %w", err)
    }
    
    if revoked {
        return errors.New("token has been revoked")
    }
    
    return nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
