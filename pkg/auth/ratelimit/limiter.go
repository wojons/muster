// Package ratelimit provides rate limiting utilities.
// This file extends token_bucket.go with the TokenBucketLimiter type
// needed by pkg/auth/oauth2_ratelimit.go.
package ratelimit

import "time"

// TokenBucketLimiter wraps a TokenBucket for limiting API call rates.
type TokenBucketLimiter struct {
	bucket *TokenBucket
}

// NewTokenBucketLimiter creates a new token bucket limiter.
func NewTokenBucketLimiter(requestsPerSecond int, burstSize int) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		bucket: NewTokenBucket(burstSize, float64(requestsPerSecond)),
	}
}

// Allow checks if a request is allowed under the rate limit.
func (l *TokenBucketLimiter) Allow() bool {
	if l == nil || l.bucket == nil {
		return true
	}
	return l.bucket.Allow()
}

// Wait blocks until a request is allowed.
func (l *TokenBucketLimiter) Wait() {
	if l == nil || l.bucket == nil {
		return
	}
	for !l.bucket.Allow() {
		time.Sleep(100 * time.Millisecond)
	}
}

// Rate returns the configured rate in requests per second.
func (l *TokenBucketLimiter) Rate() float64 {
	if l == nil || l.bucket == nil {
		return 0
	}
	return l.bucket.fillRate
}
