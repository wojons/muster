# Cache Package Examples

This document provides comprehensive examples for using the `pkg/cache` package, which provides multi-tier caching with memory, disk, and Redis backends.

## Overview

The `cache` package implements a three-tier caching system:
1. **L1 Cache**: In-memory LRU cache (fastest)
2. **L2 Cache**: Disk cache (persistent)
3. **L3 Cache**: Redis (distributed)

## Basic Usage

### Memory Cache

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/wojons/muster/pkg/cache"
)

func main() {
    ctx := context.Background()

    // Create memory cache
    memCache, err := cache.NewMemoryCache(&cache.MemoryOptions{
        MaxSize:      1000,
        TTL:          5 * time.Minute,
        EvictionPolicy: cache.EvictionLRU,
    })
    if err != nil {
        panic(err)
    }

    // Set value
    err = memCache.Set(ctx, "user:123", []byte(`{"name": "John", "email": "john@example.com"}`))
    if err != nil {
        panic(err)
    }

    // Get value
    value, err := memCache.Get(ctx, "user:123")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Cached value: %s\n", string(value))

    // Delete value
    err = memCache.Delete(ctx, "user:123")
    if err != nil {
        panic(err)
    }
}
```

### Disk Cache

```go
func diskCacheExample(ctx context.Context) {
    // Create disk cache
    diskCache, err := cache.NewDiskCache(&cache.DiskOptions{
        Directory: "/tmp/muster-cache",
        MaxSize:   100 * cache.MB,
        TTL:       time.Hour,
    })
    if err != nil {
        panic(err)
    }
    defer diskCache.Close()

    // Set value
    value := []byte(`{"data": "example"}`)
    err = diskCache.Set(ctx, "key", value)
    if err != nil {
        panic(err)
    }

    // Get value
    retrieved, err := diskCache.Get(ctx, "key")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Retrieved: %s\n", string(retrieved))
}
```

### Redis Cache

```go
func redisCacheExample(ctx context.Context) {
    // Create Redis cache
    redisCache, err := cache.NewRedisCache(&cache.RedisOptions{
        Addr:     "localhost:6379",
        Password: "", // No password
        DB:       0,
        Prefix:   "muster:",
        TTL:      time.Hour,
        PoolSize: 10,
    })
    if err != nil {
        panic(err)
    }
    defer redisCache.Close()

    // Set value
    err = redisCache.Set(ctx, "api:response:123", []byte(`{"result": "success"}`))
    if err != nil {
        panic(err)
    }

    // Get value
    value, err := redisCache.Get(ctx, "api:response:123")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Redis value: %s\n", string(value))
}
```

## Multi-Tier Cache

### Using Cache Manager

```go
func cacheManagerExample(ctx context.Context) {
    // Create multi-tier cache
    manager, err := cache.NewManager(&cache.ManagerConfig{
        Memory: &cache.MemoryOptions{
            MaxSize:        1000,
            TTL:            5 * time.Minute,
            EvictionPolicy: cache.EvictionLRU,
        },
        Disk: &cache.DiskOptions{
            Directory: "/tmp/muster-cache",
            MaxSize:   1 * cache.GB,
            TTL:       time.Hour,
        },
        Redis: &cache.RedisOptions{
            Addr:     "localhost:6379",
            Prefix:   "muster:",
            TTL:      24 * time.Hour,
            PoolSize: 10,
        },
        Strategy: cache.StrategyWriteThrough,
    })
    if err != nil {
        panic(err)
    }
    defer manager.Close()

    // Set value (writes to all tiers)
    err = manager.Set(ctx, "user:profile:123", []byte(`{"name": "John"}`))
    if err != nil {
        panic(err)
    }

    // Get value (reads from memory, falls back to disk, then Redis)
    value, err := manager.Get(ctx, "user:profile:123")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Cached profile: %s\n", string(value))

    // Delete from all tiers
    err = manager.Delete(ctx, "user:profile:123")
    if err != nil {
        panic(err)
    }
}
```

### Cache-Aside Pattern

```go
type Service struct {
    cache   cache.Cache
    db      *Database
}

func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    // Try cache first
    value, err := s.cache.Get(ctx, key)
    if err == nil {
        var user User
        if err := json.Unmarshal(value, &user); err == nil {
            return &user, nil
        }
    }

    // Cache miss, fetch from database
    user, err := s.db.FindUser(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch user: %w", err)
    }

    // Store in cache for next time
    data, err := json.Marshal(user)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal user: %w", err)
    }

    if err := s.cache.Set(ctx, key, data); err != nil {
        // Log error but don't fail the request
        log.Printf("Failed to cache user: %v", err)
    }

    return user, nil
}
```

### Write-Through Pattern

```go
func (s *Service) UpdateUser(ctx context.Context, user *User) error {
    // Update database
    if err := s.db.UpdateUser(ctx, user); err != nil {
        return err
    }

    // Update cache
    key := fmt.Sprintf("user:%d", user.ID)
    data, err := json.Marshal(user)
    if err != nil {
        return err
    }

    return s.cache.Set(ctx, key, data)
}
```

### Write-Behind Pattern

```go
func (s *Service) UpdateUserAsync(ctx context.Context, user *User) error {
    // Update cache immediately
    key := fmt.Sprintf("user:%d", user.ID)
    data, err := json.Marshal(user)
    if err != nil {
        return err
    }

    if err := s.cache.Set(ctx, key, data); err != nil {
        return err
    }

    // Queue database update
    s.updateQueue <- user

    return nil
}

func (s *Service) backgroundUpdater() {
    for user := range s.updateQueue {
        ctx := context.Background()
        if err := s.db.UpdateUser(ctx, user); err != nil {
            log.Printf("Failed to update user %d: %v", user.ID, err)
        }
    }
}
```

## Advanced Features

### Cache Invalidation

```go
func invalidationExample(ctx context.Context, manager cache.Manager) {
    // Invalidate specific key
    manager.Delete(ctx, "user:123")

    // Invalidate pattern (e.g., all user cache entries)
    manager.Invalidate(ctx, "user:*")

    // Flush entire cache
    manager.Flush(ctx)
}
```

### Cache Statistics

```go
func statsExample() cache.Manager {
    manager, _ := cache.NewManager(&cache.ManagerConfig{})

    // Get statistics
    stats := manager.Stats()

    fmt.Printf("Hits: %d\n", stats.Hits)
    fmt.Printf("Misses: %d\n", stats.Misses)
    fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate())
    fmt.Printf("Size: %d\n", stats.Size)
    fmt.Printf("Evictions: %d\n", stats.Evictions)

    return manager
}
```

### Cache Warming

```go
func warmCache(ctx context.Context, manager cache.Manager, db *Database) error {
    // Get all frequently accessed users
    users, err := db.GetPopularUsers(ctx)
    if err != nil {
        return err
    }

    // Populate cache
    for _, user := range users {
        key := fmt.Sprintf("user:%d", user.ID)
        data, err := json.Marshal(user)
        if err != nil {
            continue
        }

        manager.Set(ctx, key, data)
    }

    return nil
}
```

### TTL and Expiration

```go
func ttlExample(ctx context.Context, manager cache.Manager) {
    // Set with specific TTL
    data := []byte("expiring data")
    err := manager.SetWithTTL(ctx, "temp:data", data, 30*time.Second)
    if err != nil {
        panic(err)
    }

    // Extend TTL
    err = manager.ExtendTTL(ctx, "temp:data", 60*time.Second)
    if err != nil {
        panic(err)
    }

    // Get TTL
    ttl, err := manager.GetTTL(ctx, "temp:data")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Time until expiration: %v\n", ttl)
}
```

## Error Handling

### Graceful Degradation

```go
type ResilientCache struct {
    primary cache.Cache
    fallback cache.Cache
}

func (c *ResilientCache) Get(ctx context.Context, key string) ([]byte, error) {
    // Try primary cache
    value, err := c.primary.Get(ctx, key)
    if err == nil {
        return value, nil
    }

    // Fall back to secondary cache
    value, err = c.fallback.Get(ctx, key)
    if err == nil {
        return value, nil
    }

    // Both caches failed
    return nil, cache.ErrCacheMiss
}
```

### Circuit Breaker

```go
type CircuitBreakerCache struct {
    cache         cache.Cache
    breaker       *circuit.Breaker
    fallbackCache cache.Cache
}

func (c *CircuitBreakerCache) Get(ctx context.Context, key string) ([]byte, error) {
    if c.breaker.IsOpen() {
        return c.fallbackCache.Get(ctx, key)
    }

    value, err := c.cache.Get(ctx, key)
    if err != nil && errors.Is(err, context.DeadlineExceeded) {
        c.breaker.RecordFailure()
        return c.fallbackCache.Get(ctx, key)
    }

    c.breaker.RecordSuccess()
    return value, nil
}
```

## Best Practices

### 1. Use Appropriate Cache Sizes

```go
// Bad: Too small
memCache, _ := cache.NewMemoryCache(&cache.MemoryOptions{
    MaxSize: 10, // Too small
    TTL:     5 * time.Minute,
})

// Good: Sufficient size
memCache, _ := cache.NewMemoryCache(&cache.MemoryOptions{
    MaxSize: 1000, // Sufficient
    TTL:     5 * time.Minute,
})
```

### 2. Set Appropriate TTLs

```go
// Static data: Long TTL
cache.Set(ctx, "config", data, 24*time.Hour)

// User data: Medium TTL
cache.Set(ctx, "user:123", data, 5*time.Minute)

// Temporary data: Short TTL
cache.Set(ctx, "temp", data, time.Minute)
```

### 3. Monitor Cache Performance

```go
func monitorCache(manager cache.Manager) {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        stats := manager.Stats()
        log.Printf("Cache stats - Hits: %d, Misses: %d, Hit Rate: %.2f%%",
            stats.Hits, stats.Misses, stats.HitRate())
    }
}
```

### 4. Handle Cache Failures Gracefully

```go
func getUserWithCache(ctx context.Context, cache cache.Cache, db *Database, id int64) (*User, error) {
    var user *User

    // Try cache
    if value, err := cache.Get(ctx, fmt.Sprintf("user:%d", id)); err == nil {
        if err := json.Unmarshal(value, &user); err == nil {
            return user, nil
        }
    }

    // Cache miss or error
    user, err = db.FindUser(ctx, id)
    if err != nil {
        return nil, err
    }

    // Try to cache (don't fail if it doesn't work)
    if data, err := json.Marshal(user); err == nil {
        _ = cache.Set(ctx, fmt.Sprintf("user:%d", id), data)
    }

    return user, nil
}
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/wojons/muster/pkg/cache"
)

type APIClient struct {
    cache cache.Manager
}

func NewAPIClient() (*APIClient, error) {
    manager, err := cache.NewManager(&cache.ManagerConfig{
        Memory: &cache.MemoryOptions{
            MaxSize:        1000,
            TTL:            5 * time.Minute,
            EvictionPolicy: cache.EvictionLRU,
        },
        Disk: &cache.DiskOptions{
            Directory: "/tmp/muster-cache",
            MaxSize:   1 * cache.GB,
            TTL:       time.Hour,
        },
        Redis: &cache.RedisOptions{
            Addr:     "localhost:6379",
            Prefix:   "muster:",
            TTL:      24 * time.Hour,
            PoolSize: 10,
        },
        Strategy: cache.Strat egyWriteThrough,
    })

    return &APIClient{cache: manager}, err
}

func (c *APIClient) GetAPIResponse(ctx context.Context, endpoint string) ([]byte, error) {
    key := fmt.Sprintf("api:%s", endpoint)

    // Try cache first
    if value, err := c.cache.Get(ctx, key); err == nil {
        return value, nil
    }

    // Cache miss - call API
    response, err := c.callAPI(ctx, endpoint)
    if err != nil {
        return nil, err
    }

    // Cache the response
    if err := c.cache.Set(ctx, key, response); err != nil {
        log.Printf("Failed to cache response: %v", err)
    }

    return response, nil
}

func (c *APIClient) Close() error {
    return c.cache.Close()
}

func main() {
    ctx := context.Background()

    // Create client
    client, err := NewAPIClient()
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // Make API call (with caching)
    response, err := client.GetAPIResponse(ctx, "/users/123")
    if err != nil {
        panic(err)
    }

    fmt.Printf("API response: %s\n", string(response))
}
```
