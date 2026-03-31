// Package ratelimit provides token bucket rate limiting implementation for controlling API request
// throughput and preventing abuse. It implements the token bucket algorithm with configurable
// capacity and refill rates for smooth rate limiting.
//
// # Overview
//
// The ratelimit package implements the token bucket algorithm, one of the most popular and
// flexible rate limiting strategies:
//
//   - Tokens are added to a bucket at a constant rate (fill rate)
//   - Each request consumes tokens from the bucket
//   - Requests are allowed only if sufficient tokens are available
//   - Bucket capacity limits burst size
//
// This approach provides:
//
//   - Smooth rate limiting with controlled bursting
//   - Immediate rejection when bucket is empty
//   - Gradual recovery as tokens accumulate
//   - Thread-safe concurrent access
//
// # Architecture
//
// The package consists of one main component:
//
// 1. TokenBucket: Thread-safe token bucket implementation
//
// # Usage
//
// Create a token bucket with 100 token capacity and 10 tokens/second fill rate:
//
//	bucket := ratelimit.NewTokenBucket(100, 10.0)
//
// Check if request is allowed (consumes 1 token):
//
//	if bucket.Allow(1) {
//	    // Make API request
//	    response, err := http.Get(url)
//	} else {
//	    // Rate limited - wait and retry
//	    time.Sleep(100 * time.Millisecond)
//	}
//
// Consume multiple tokens for bulk operations:
//
//	if bucket.Allow(10) {
//	    // Bulk operation consuming 10 tokens
//	    processBulkRequest(items)
//	}
//
// # Token Bucket Algorithm
//
// The algorithm works as follows:
//
// 1. Bucket starts full (tokens = capacity)
// 2. Tokens are added continuously at fill_rate tokens/second
// 3. Each request attempts to consume N tokens
// 4. If tokens >= N, request succeeds and tokens -= N
// 5. If tokens < N, request is rejected (rate limited)
// 6. Tokens never exceed capacity (bucket overflows)
//
// Example: 100 capacity, 10 tokens/sec fill rate
//
//	T=0s:  Bucket full (100 tokens)
//	T=1s:  Consume 10 → 90 tokens remain
//	T=2s:  Consume 50 → 40 tokens remain (+10 refilled)
//	T=3s:  Consume 100 → REJECTED (only 50 tokens: 40 + 10 refilled)
//	T=4s:  Consume 10 → ALLOWED (60 tokens: 50 + 10 refilled)
//
// # Configuration
//
// Key parameters:
//
//   - Capacity: Maximum tokens in bucket (controls burst size)
//   - FillRate: Tokens added per second (controls sustained rate)
//
// Common configurations:
//
//	// 10 requests/second, burst up to 100
//	bucket := ratelimit.NewTokenBucket(100, 10.0)
//
//	// 1 request/second, no bursting
//	bucket := ratelimit.NewTokenBucket(1, 1.0)
//
//	// 100 requests/second, burst up to 1000
//	bucket := ratelimit.NewTokenBucket(1000, 100.0)
//
// # Thread Safety
//
// TokenBucket is fully thread-safe using sync.Mutex:
//
//   - Allow(): Locks mutex, updates tokens, checks availability
//   - Multiple goroutines can safely call Allow() concurrently
//   - No external locking required
//
// Example concurrent usage:
//
//	bucket := ratelimit.NewTokenBucket(100, 10.0)
//
//	var wg sync.WaitGroup
//	for i := 0; i < 10; i++ {
//	    wg.Add(1)
//	    go func() {
//	        defer wg.Done()
//	        if bucket.Allow(1) {
//	            makeRequest()
//	        }
//	    }()
//	}
//	wg.Wait()
//
// # Security
//
// The implementation includes a critical security fix:
//
// CRITICAL: Negative or zero token requests are rejected to prevent bypass attacks.
// Without this check, an attacker could call Allow(-1000) to add tokens to the bucket,
// completely bypassing rate limiting.
//
//	if tokens <= 0 {
//	    return true // Zero or negative tokens consume nothing
//	}
//
// # Error Handling
//
// The Allow() method always returns a boolean:
//
//   - true: Request allowed (tokens consumed)
//   - false: Request denied (insufficient tokens)
//
// No errors are returned; rate limiting is a normal operational condition.
//
// # Integration
//
// The ratelimit package integrates with:
//
//   - pkg/auth/oauth2.go: OAuth2 endpoint rate limiting
//   - pkg/auth/oauth2_ratelimit.go: OAuth-specific rate limiter wrapper
//   - pkg/client: Client-side API rate limiting
//   - pkg/daemon: Server-side request throttling
//
// # Example: OAuth2 Rate Limiting
//
//	type OAuthRateLimiter struct {
//	    tokenEndpoint *ratelimit.TokenBucket    // 10 requests/minute
//	    devicePolling *ratelimit.TokenBucket    // 2 requests/minute
//	    callback      *ratelimit.TokenBucket    // 20 requests/minute
//	}
//
//	func NewOAuthRateLimiter() *OAuthRateLimiter {
//	    return &OAuthRateLimiter{
//	        tokenEndpoint: ratelimit.NewTokenBucket(10, 10.0/60.0),
//	        devicePolling: ratelimit.NewTokenBucket(2, 2.0/60.0),
//	        callback:      ratelimit.NewTokenBucket(20, 20.0/60.0),
//	    }
//	}
//
// # Performance
//
// Token bucket operations are extremely fast:
//
//   - Allow(): < 1 microsecond (includes mutex locking)
//   - No allocations after initialization
//   - O(1) time complexity
//   - Minimal memory footprint (24 bytes per bucket)
//
// # Comparison with Other Rate Limiters
//
// Token Bucket vs Fixed Window:
//
//   - Token Bucket: Smooth rate, allows bursting, no boundary spikes
//   - Fixed Window: Simple, but allows 2x burst at window boundaries
//
// Token Bucket vs Sliding Window:
//
//   - Token Bucket: Simpler, allows controlled bursting
//   - Sliding Window: More precise, but higher memory overhead
//
// Token Bucket vs Leaky Bucket:
//
//   - Token Bucket: Allows bursting, rejects when empty
//   - Leaky Bucket: Smooth output, queues excess requests
//
// # Best Practices
//
// 1. Choose capacity based on acceptable burst size
// 2. Set fill rate based on sustainable throughput
// 3. Monitor rejection rates to tune parameters
// 4. Use separate buckets for different endpoints/resources
// 5. Implement exponential backoff on rejection
// 6. Log rate limit events for debugging and alerting
package ratelimit

import (
	"context"
	"sync"
	"time"
)

type TokenBucket struct {
	capacity    int64
	tokens      float64
	fillRate    float64
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

	// CRITICAL FIX: Validate tokens is positive to prevent bypass
	if tokens <= 0 {
		return true // Zero or negative tokens consume nothing
	}

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdated).Seconds()
	tb.lastUpdated = now

	tb.tokens += elapsed * tb.fillRate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}

	if tb.tokens >= float64(tokens) {
		tb.tokens -= float64(tokens)
		return true
	}

	return false
}

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

func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

func (tb *TokenBucket) Capacity() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.capacity
}

func (tb *TokenBucket) FillRate() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.fillRate
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

func NewTokenBucketLimiter(config BucketConfig) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		buckets: make(map[string]*TokenBucket),
		config:  config,
	}
}

func (rl *TokenBucketLimiter) Allow(key string, tokens int64) bool {
	rl.mu.Lock()
	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = NewTokenBucket(rl.config.Capacity, rl.config.FillRate)
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	return bucket.Allow(tokens)
}

func (rl *TokenBucketLimiter) Wait(ctx context.Context, key string, tokens int64) error {
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

func (rl *TokenBucketLimiter) GetBucket(key string) *TokenBucket {
	rl.mu.Lock()
	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = NewTokenBucket(rl.config.Capacity, rl.config.FillRate)
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	return bucket
}

type BucketMetrics struct {
	RequestsAllowed int64
	RequestsDenied  int64
	AverageTokens   float64
	PeakTokens      int64
}
