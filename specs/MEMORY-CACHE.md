# Memory Cache Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Caching

## Overview

Complete specification of **in-memory cache** (L1) implementation.

## 1. Implementation

```go
type MemoryCache struct {
    data     map[string]*entry
    maxSize  int64
    size     int64
    policy   string // "LRU", "LFU", "FIFO"
    mu       sync.RWMutex
}

type entry struct {
    value     Value
    size      int64
    accessed  time.Time
    frequency int
}

func NewMemoryCache(maxSize int64, policy string) *MemoryCache {
    return &MemoryCache{
        data:    make(map[string]*entry),
        maxSize: maxSize,
        policy:  policy,
    }
}
```

## 2. Operations

```go
func (c *MemoryCache) Get(ctx context.Context, key string) (Value, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    e, ok := c.data[key]
    if !ok {
        return Value{}, ErrNotFound
    }
    
    // Check expiration
    if time.Since(e.value.CreatedAt) > e.value.TTL {
        return Value{}, ErrNotFound
    }
    
    // Update access tracking
    e.accessed = time.Now()
    e.frequency++
    
    return e.value, nil
}

func (c *MemoryCache) Set(ctx context.Context, key string, value Value, ttl time.Duration) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    size := int64(len(value.Data))
    
    // Check if exceeds max entry size
    if size > c.maxSize/10 {
        return fmt.Errorf("entry too large for memory cache")
    }
    
    // Make room if necessary
    for c.size+size > c.maxSize {
        c.evict()
    }
    
    value.TTL = ttl
    c.data[key] = &entry{
        value:     value,
        size:      size,
        accessed:  time.Now(),
        frequency: 1,
    }
    c.size += size
    
    return nil
}
```

## 3. Eviction

```go
func (c *MemoryCache) evict() {
    if len(c.data) == 0 {
        return
    }
    
    var keyToEvict string
    
    switch c.policy {
    case "LRU":
        keyToEvict = c.findLRU()
    case "LFU":
        keyToEvict = c.findLFU()
    case "FIFO":
        keyToEvict = c.findFIFO()
    }
    
    if keyToEvict != "" {
        c.size -= c.data[keyToEvict].size
        delete(c.data, keyToEvict)
    }
}

func (c *MemoryCache) findLRU() string {
    var oldest time.Time
    var key string
    
    for k, v := range c.data {
        if key == "" || v.accessed.Before(oldest) {
            oldest = v.accessed
            key = k
        }
    }
    
    return key
}
```

## 4. Memory Limits

```go
const (
    DefaultMemoryCacheSize = 100 * 1024 * 1024 // 100MB
    MaxMemoryCacheSize     = 512 * 1024 * 1024 // 512MB
    MaxEntrySize           = 10 * 1024 * 1024  // 10MB
)
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
