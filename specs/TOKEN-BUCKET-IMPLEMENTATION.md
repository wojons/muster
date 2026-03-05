# Token Bucket Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Rate Limiting

## Overview

Complete specification of **token bucket** algorithm implementation for rate limiting.

## 1. Algorithm

```go
type TokenBucket struct {
    capacity    int64         // Maximum tokens
    tokens      float64       // Current tokens
    fillRate    float64       // Tokens added per second
    lastUpdated time.Time
    mu          sync.Mutex
}

func NewTokenBucket(capacity int64, fillRate float64) *TokenBucket {
    return &TokenBucket{
        capacity:    capacity,
        tokens:      float64(capacity),
        fillRate:    fillRate,
        lastUpdated: time.Now(),
    }
}

func (tb *TokenBucket) Allow(tokens int64) bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()
    
    now := time.Now()
    elapsed := now.Sub(tb.lastUpdated).Seconds()
    tb.lastUpdated = now
    
    // Add tokens based on elapsed time
    tb.tokens += elapsed * tb.fillRate
    if tb.tokens > float64(tb.capacity) {
        tb.tokens = float64(tb.capacity)
    }
    
    // Check if enough tokens
    if tb.tokens >= float64(tokens) {
        tb.tokens -= float64(tokens)
        return true
    }
    
    return false
}
```

## 2. Rate Limiter

```go
type RateLimiter struct {
    buckets map[string]*TokenBucket
    mu      sync.RWMutex
    config  RateLimitConfig
}

type RateLimitConfig struct {
    DefaultCapacity int64
    DefaultRate     float64
}

func (rl *RateLimiter) Allow(key string, tokens int64) bool {
    rl.mu.RLock()
    bucket, exists := rl.buckets[key]
    rl.mu.RUnlock()
    
    if !exists {
        rl.mu.Lock()
        bucket = NewTokenBucket(rl.config.DefaultCapacity, rl.config.DefaultRate)
        rl.buckets[key] = bucket
        rl.mu.Unlock()
    }
    
    return bucket.Allow(tokens)
}

func (rl *RateLimiter) Wait(ctx context.Context, key string, tokens int64) error {
    for {
        if rl.Allow(key, tokens) {
            return nil
        }
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(100 * time.Millisecond):
            continue
        }
    }
}
```

## 3. Per-Host Rate Limiting

```go
func (rl *RateLimiter) LimitHost(host string) bool {
    return rl.Allow("host:"+host, 1)
}

func (rl *RateLimiter) LimitEndpoint(host, endpoint string) bool {
    return rl.Allow(fmt.Sprintf("endpoint:%s:%s", host, endpoint), 1)
}
```

## 4. Distributed Rate Limiting

```go
type RedisRateLimiter struct {
    client *redis.Client
    script *redis.Script
}

func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
    luaScript := `
        local key = KEYS[1]
        local capacity = tonumber(ARGV[1])
        local rate = tonumber(ARGV[2])
        local now = tonumber(ARGV[3])
        local requested = tonumber(ARGV[4])
        
        local bucket = redis.call('HMGET', key, 'tokens', 'last')
        local tokens = tonumber(bucket[1]) or capacity
        local last = tonumber(bucket[2]) or now
        
        local elapsed = now - last
        tokens = math.min(capacity, tokens + elapsed * rate)
        
        if tokens >= requested then
            tokens = tokens - requested
            redis.call('HMSET', key, 'tokens', tokens, 'last', now)
            redis.call('EXPIRE', key, 60)
            return 1
        else
            redis.call('HSET', key, 'last', now)
            return 0
        end
    `
    
    return &RedisRateLimiter{
        client: client,
        script: redis.NewScript(luaScript),
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
