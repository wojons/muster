# Cache Policies Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Caching

## Overview

Complete specification of **cache policies** for different data types.

## Policy Architecture

```mermaid
graph TB
    subgraph TimeBased[Time-Based Policies]
        T1[OpenAPI Spec<br/>1h - 7d]
        T2[WASM Module<br/>1d - 30d]
        T3[Inventory<br/>5m - 24h]
        T4[Auth Token<br/>5m - 1h]
    end

    subgraph SizeBased[Size-Based Policies]
        S1[Memory<br/>100MB LRU]
        S2[Disk<br/>1GB LRU]
    end

    subgraph Validation[Validation Strategies]
        V1[ETag]
        V2[Last-Modified]
        V3[Checksum]
    end

    Policy[Cache Policy System] --> TimeBased
    Policy --> SizeBased
    Policy --> Validation

    style Policy fill:#87cefa
    style TimeBased fill:#ffebcd
    style SizeBased fill:#ffe484
    style Validation fill:#ffa07a
```

## 1. Policy Types

### 1.1 Time-Based Policies

```go
type TTLPolicy struct {
    DefaultTTL time.Duration
    MaxTTL     time.Duration
    MinTTL     time.Duration
}

var DefaultPolicies = map[string]TTLPolicy{
    "openapi_spec": {
        DefaultTTL: 24 * time.Hour,
        MaxTTL:     7 * 24 * time.Hour,
        MinTTL:     1 * time.Hour,
    },
    "wasm_module": {
        DefaultTTL: 7 * 24 * time.Hour,
        MaxTTL:     30 * 24 * time.Hour,
        MinTTL:     24 * time.Hour,
    },
    "inventory": {
        DefaultTTL: 1 * time.Hour,
        MaxTTL:     24 * time.Hour,
        MinTTL:     5 * time.Minute,
    },
    "auth_token": {
        DefaultTTL: 55 * time.Minute, // Refresh before expiry
        MaxTTL:     1 * time.Hour,
        MinTTL:     5 * time.Minute,
    },
}
```

### 1.2 Size-Based Policies

```go
type SizePolicy struct {
    MaxSize       int64
    EvictionPolicy string // "LRU", "LFU", "FIFO"
}

var SizePolicies = map[string]SizePolicy{
    "memory": {
        MaxSize:        100 * 1024 * 1024, // 100MB
        EvictionPolicy: "LRU",
    },
    "disk": {
        MaxSize:        1024 * 1024 * 1024, // 1GB
        EvictionPolicy: "LRU",
    },
}
```

## 2. Cache-Control Headers

```go
type CacheControl struct {
    MaxAge       int  `json:"max_age"`       // seconds
    NoCache      bool `json:"no_cache"`
    NoStore      bool `json:"no_store"`
    MustRevalidate bool `json:"must_revalidate"`
    ETag         string `json:"etag"`
    LastModified time.Time `json:"last_modified"`
}

func ParseCacheControl(header string) CacheControl {
    cc := CacheControl{}
    
    for _, directive := range strings.Split(header, ",") {
        directive = strings.TrimSpace(directive)
        
        switch {
        case directive == "no-cache":
            cc.NoCache = true
        case directive == "no-store":
            cc.NoStore = true
        case directive == "must-revalidate":
            cc.MustRevalidate = true
        case strings.HasPrefix(directive, "max-age="):
            age, _ := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
            cc.MaxAge = age
        }
    }
    
    return cc
}
```

## 3. Validation Strategies

```mermaid
flowchart TD
    Start[Validating Cache Entry] --> GetMeta[Get Entry Metadata]
    GetMeta --> CheckStrategy{Strategy?}
    CheckStrategy -->|ETag| ValidateETag[Validate ETag]
    CheckStrategy -->|Last-Mod| ValidateLM[Validate Last-Modified]
    CheckStrategy -->|Checksum| ValidateCS[Validate Checksum]

    ValidateETag --> CheckETag{ETag Match?}
    ValidateLM --> CheckLM{Modified?}
    ValidateCS --> CheckCS{Checksum Match?}

    CheckETag -->|Yes| Valid[Valid Cache]
    CheckETag -->|No| Invalid[Invalid Cache]
    CheckLM -->|No| Valid
    CheckLM -->|Yes| Invalid
    CheckCS -->|Yes| Valid
    CheckCS -->|No| Invalid

    Valid --> Success[Return true]
    Invalid --> Fail[Return false]
    Fail --> Error[Return error]

    style Start fill:#b5e8ad
    style Valid fill:#98fb98
    style Invalid fill:#ff6b6b
    style Success fill:#87cefa
    style Fail fill:#ffb6c1
    style Error fill:#808080
```

```go
type ValidationStrategy string

const (
    ValidationETag     ValidationStrategy = "etag"
    ValidationLastMod  ValidationStrategy = "last-modified"
    ValidationChecksum ValidationStrategy = "checksum"
)

func (c *Cache) Validate(ctx context.Context, key string, strategy ValidationStrategy) (bool, error) {
    entry, err := c.GetMetadata(ctx, key)
    if err != nil {
        return false, err
    }
    
    switch strategy {
    case ValidationETag:
        return c.validateETag(ctx, key, entry.ETag)
    case ValidationLastMod:
        return c.validateLastModified(ctx, key, entry.LastModified)
    case ValidationChecksum:
        return c.validateChecksum(ctx, key, entry.Checksum)
    }
    
    return false, fmt.Errorf("unknown validation strategy: %s", strategy)
}
```

## 4. Policy Enforcement

```mermaid
flowchart TD
    Start[Set Cache Entry] --> CheckPolicy{Policy Exists?}
    CheckPolicy -->|No| Error[Unknown Policy]
    CheckPolicy -->|Yes| GetTTL[Get TTL Policy]

    GetTTL --> CheckSize{Entry Size OK?}
    CheckSize -->|Too Large| ErrorSize[Entry Too Large]
    CheckSize -->|OK| Apply[Apply Size Limits]

    Apply --> CheckTTL{TTL in Range?}
    CheckTTL -->|No| Clamp[Clamp to Min/Max]
    CheckTTL -->|Yes| Set[Set with Policy]

    Clamp --> Set
    Set --> Store[Store in Cache]

    Error --> Fail[Return Error]
    ErrorSize --> Fail
    Fail --> End[End]
    Store --> End

    style Start fill:#b5e8ad
    style CheckPolicy fill:#ffe484
    style GetTTL fill:#ffe484
    style CheckSize fill:#ffa07a
    style CheckTTL fill:#ffa07a
    style Error fill:#ff6b6b
    style ErrorSize fill:#ff6b6b
    style Apply fill:#87cefa
    style Clamp fill:#87cefa
    style Set fill:#87cefa
    style Store fill:#98fb98
    style Fail fill:#ffb6c1
    style End fill:#e6e6fa
```

```go
func (c *Cache) SetWithPolicy(ctx context.Context, key string, value Value, policy string) error {
    ttlPolicy, ok := DefaultPolicies[policy]
    if !ok {
        return fmt.Errorf("unknown policy: %s", policy)
    }
    
    // Apply size limits
    if value.Metadata.Size > c.maxEntrySize {
        return fmt.Errorf("entry too large: %d bytes", value.Metadata.Size)
    }
    
    return c.Set(ctx, key, value, ttlPolicy.DefaultTTL)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
