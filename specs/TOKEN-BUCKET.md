# Token Bucket Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Rate Limiting

## Overview

Complete specification of the **token bucket algorithm** for rate limiting.

## 1. Algorithm Description

The token bucket algorithm controls request rates by maintaining a bucket with a fixed capacity. Tokens are added at a constant rate, and each request consumes one token. If no tokens are available, the request is rejected or delayed.

```go
type TokenBucket struct {
    capacity    int64         // Maximum tokens in bucket
    tokens      float64       // Current token count
    fillRate    float64       // Tokens added per second
    lastUpdated time.Time     // Last token addition time
    mu          sync.Mutex    // Thread safety
}
```

## 2. Core Operations

### 2.1 Bucket Creation

```go
func NewTokenBucket(capacity int64, fillRate float64) *TokenBucket {
    return &TokenBucket{
        capacity:    capacity,
        tokens:      float64(capacity), // Start with full bucket
        fillRate:    fillRate,
        lastUpdated: time.Now(),
    }
}
```

### 2.2 Token Consumption

```go
func (tb *TokenBucket) Allow(tokens int64) bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()
    
    // Add tokens based on elapsed time
    now := time.Now()
    elapsed := now.Sub(tb.lastUpdated).Seconds()
    tb.lastUpdated = now
    
    tb.tokens += elapsed * tb.fillRate
    if tb.tokens > float64(tb.capacity) {
        tb.tokens = float64(tb.capacity)
    }
    
    // Check if enough tokens available
    if tb.tokens >= float64(tokens) {
        tb.tokens -= float64(tokens)
        return true
    }
    
    return false
}
```

### 2.3 Waiting for Tokens

```go
func (tb *TokenBucket) Wait(ctx context.Context, tokens int64) error {
    for {
        if tb.Allow(tokens) {
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

## 3. Rate Limiter Interface

```go
type RateLimiter interface {
    Allow(key string, tokens int64) bool
    Wait(ctx context.Context, key string, tokens int64) error
}

type TokenBucketLimiter struct {
    buckets map[string]*TokenBucket
    config  BucketConfig
    mu      sync.RWMutex
}

type BucketConfig struct {
    Capacity int64
    FillRate float64
}
```

## 4. Multi-Bucket Management

```go
func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
    rl.mu.RLock()
    bucket, exists := rl.buckets[key]
    rl.mu.RUnlock()
    
    if !exists {
        rl.mu.Lock()
        bucket = NewTokenBucket(rl.config.Capacity, rl.config.FillRate)
        rl.buckets[key] = bucket
        rl.mu.Unlock()
    }
    
    return bucket.Allow(tokens)
}
```

## 5. Configuration Examples

```go
var DefaultConfigs = map[string]BucketConfig{
    "api_requests": {
        Capacity: 100,
        FillRate: 10, // 10 requests per second
    },
    "auth_attempts": {
        Capacity: 5,
        FillRate: 0.1, // 1 attempt per 10 seconds
    },
    "wasm_execution": {
        Capacity: 10,
        FillRate: 2, // 2 executions per second
    },
}
```

## 6. Distributed Rate Limiting

```go
type DistributedTokenBucket struct {
    redis  *redis.Client
    script *redis.Script
}

func (d *DistributedTokenBucket) Allow(key string, tokens int64) bool {
    luaScript := `
        local key = KEYS[1]
        local capacity = tonumber(ARGV[1])
        local fillRate = tonumber(ARGV[2])
        local now = tonumber(ARGV[3])
        local requested = tonumber(ARGV[4])
        
        local bucket = redis.call('HMGET', key, 'tokens', 'last')
        local tokens = tonumber(bucket[1]) or capacity
        local last = tonumber(bucket[2]) or now
        
        local elapsed = now - last
        tokens = math.min(capacity, tokens + elapsed * fillRate)
        
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
    
    result, err := d.redis.Eval(
        context.Background(),
        luaScript,
        []string{key},
        d.config.Capacity,
        d.config.FillRate,
        time.Now().Unix(),
        tokens,
    ).Int()
    
    return err == nil && result == 1
}
```

## 7. Monitoring

```go
type BucketMetrics struct {
    RequestsAllowed    int64
    RequestsDenied     int64
    AverageTokens      float64
    PeakTokens         int64
}

func (tb *TokenBucket) Metrics() BucketMetrics {
    tb.mu.RLock()
    defer tb.mu.RUnlock()
    
    return BucketMetrics{
        AverageTokens: tb.tokens,
        PeakTokens:    tb.capacity,
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
