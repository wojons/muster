// Package rate provides a token bucket rate limiter with configurable
// rate and burst limits. It uses a background goroutine with a time.Ticker
// for periodic token refill, and is safe for concurrent use.
//
// Usage:
//
//	limiter := rate.NewTokenBucket(10, 5) // 10 tokens/sec, burst of 5
//	if limiter.Allow() {
//	    // handle request
//	}
//	limiter.Stop() // clean up when done
package rate

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter. Tokens are added
// at the configured rate (tokens per second) up to the burst limit.
// The bucket starts full with burst tokens available.
//
// All public methods are safe for concurrent use.
type TokenBucket struct {
	mu       sync.Mutex
	rate     int           // tokens per second
	burst    int           // maximum accumulated tokens
	tokens   float64       // current token count
	ticker   *time.Ticker  // periodic refill ticker (nil when rate == 0)
	done     chan struct{} // signals the refill goroutine to stop
	stopOnce sync.Once     // ensures Stop is idempotent
}

// NewTokenBucket creates a new TokenBucket with the given rate (tokens per
// second) and burst size. The bucket starts with burst tokens available.
//
// If rate is 0, no tokens are ever added (the bucket is only usable for its
// initial tokens). If burst is less than 1, it defaults to 1.
func NewTokenBucket(rate int, burst int) *TokenBucket {
	if burst <= 0 {
		burst = 1
	}
	if rate < 0 {
		rate = 0
	}

	tb := &TokenBucket{
		rate:   rate,
		burst:  burst,
		tokens: float64(burst),
		done:   make(chan struct{}),
	}

	if rate > 0 {
		tb.ticker = time.NewTicker(100 * time.Millisecond)
		go tb.refillLoop()
	}

	return tb
}

// refillLoop runs in a background goroutine and adds tokens at the
// configured rate. It reads the current rate from tb.rate under the
// mutex so that SetRate updates take effect on the next tick.
func (tb *TokenBucket) refillLoop() {
	const tickInterval = 100 * time.Millisecond
	for {
		select {
		case <-tb.ticker.C:
			tb.mu.Lock()
			tokensPerTick := float64(tb.rate) * tickInterval.Seconds()
			tb.tokens += tokensPerTick
			if tb.tokens > float64(tb.burst) {
				tb.tokens = float64(tb.burst)
			}
			tb.mu.Unlock()
		case <-tb.done:
			return
		}
	}
}

// Allow consumes one token. Returns true if the token was consumed,
// or false if the bucket is empty (rate limited).
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN consumes n tokens. Returns true if enough tokens were available
// and consumed, or false if rate limited. Non-positive values of n return
// true without consuming any tokens.
func (tb *TokenBucket) AllowN(n int) bool {
	if n <= 0 {
		return true
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// Tokens returns the current number of tokens in the bucket. This is useful
// for observability and monitoring.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// SetRate updates the token refill rate at runtime. The new rate takes
// effect on the next tick of the background refill goroutine. Setting
// rate to 0 means no new tokens are added (remaining tokens can still
// be consumed).
func (tb *TokenBucket) SetRate(rate int) {
	if rate < 0 {
		rate = 0
	}

	tb.mu.Lock()
	tb.rate = rate
	tb.mu.Unlock()
}

// Stop stops the background refill goroutine and releases resources.
// After Stop, no more tokens are added, but existing tokens can still
// be consumed. Safe to call multiple times.
func (tb *TokenBucket) Stop() {
	tb.stopOnce.Do(func() {
		if tb.ticker != nil {
			tb.ticker.Stop()
		}
		close(tb.done)
	})
}
