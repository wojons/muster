// Package ratelimit provides token bucket rate limiting.
package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	capacity  int
	fillRate  float64 // tokens per second
	tokens    float64
	lastFill  time.Time
	mu        sync.Mutex
}

// NewTokenBucket creates a new token bucket with the given capacity and fill rate.
// capacity is the maximum number of tokens the bucket can hold.
// fillRate is the number of tokens added per second.
func NewTokenBucket(capacity int, fillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity: capacity,
		fillRate: fillRate,
		tokens:   float64(capacity),
		lastFill: time.Now(),
	}
}

// Allow checks if a single token can be consumed.
// Returns true if allowed, false if rate limited.
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if n tokens can be consumed.
// Returns true if allowed, false if rate limited.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastFill).Seconds()
	tb.tokens += elapsed * tb.fillRate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.lastFill = now

	// Check if enough tokens available
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}
