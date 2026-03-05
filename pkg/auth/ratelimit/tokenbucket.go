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
