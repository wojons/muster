# Cache Manager Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Caching

## Overview

Complete specification of **cache manager** for coordinating multi-tier caching.

## 1. Manager Structure

```go
type CacheManager struct {
    tiers      []CacheTier
    policies   map[string]Policy
    metrics    *CacheMetrics
}

type CacheTier struct {
    Name      string
    Cache     Cache
    Priority  int // 1 = highest (memory)
    MaxSize   int64
}

type Policy struct {
    TTL       time.Duration
    Tiers     []string // Which tiers to use
    Priority  int      // Promotion priority
}
```

## 2. Tiered Operations

```go
func (cm *CacheManager) Get(ctx context.Context, key string) (Value, error) {
    // Try each tier in priority order
    for _, tier := range cm.tiers {
        value, err := tier.Cache.Get(ctx, key)
        if err == nil {
            // Promote to higher tiers if found in lower tier
            cm.promote(ctx, key, value, tier.Priority)
            cm.metrics.Hit(tier.Name)
            return value, nil
        }
    }
    
    cm.metrics.Miss()
    return Value{}, ErrNotFound
}

func (cm *CacheManager) Set(ctx context.Context, key string, value Value, policyName string) error {
    policy, ok := cm.policies[policyName]
    if !ok {
        return fmt.Errorf("unknown policy: %s", policyName)
    }
    
    // Set in specified tiers
    for _, tierName := range policy.TTL {
        for _, tier := range cm.tiers {
            if tier.Name == tierName {
                tier.Cache.Set(ctx, key, value, policy.TTL)
            }
        }
    }
    
    return nil
}

func (cm *CacheManager) promote(ctx context.Context, key string, value Value, fromTier int) {
    // Promote to all tiers with higher priority
    for _, tier := range cm.tiers {
        if tier.Priority < fromTier {
            tier.Cache.Set(ctx, key, value, time.Minute)
        }
    }
}
```

## 3. Policy Configuration

```go
var DefaultPolicies = map[string]Policy{
    "openapi_spec": {
        TTL:      24 * time.Hour,
        Tiers:    []string{"memory", "disk", "redis"},
        Priority: 1,
    },
    "inventory": {
        TTL:      1 * time.Hour,
        Tiers:    []string{"memory", "disk"},
        Priority: 2,
    },
    "auth_token": {
        TTL:      55 * time.Minute,
        Tiers:    []string{"memory"},
        Priority: 1,
    },
}
```

## 4. Metrics

```go
type CacheMetrics struct {
    hits      map[string]int64
    misses    int64
    evictions map[string]int64
    mu        sync.RWMutex
}

func (m *CacheMetrics) Hit(tier string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.hits[tier]++
}

func (m *CacheMetrics) GetHitRate() float64 {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    totalHits := int64(0)
    for _, hits := range m.hits {
        totalHits += hits
    }
    
    total := totalHits + m.misses
    if total == 0 {
        return 0
    }
    
    return float64(totalHits) / float64(total)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
