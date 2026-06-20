// Package rate provides token-bucket rate limiting types.
// Thin adapter over pkg/auth/ratelimit with a compatible API surface.
package rate

import (
	"github.com/wojons/muster/pkg/auth/ratelimit"
)

// TokenBucket implements token-bucket rate limiting.
type TokenBucket struct {
	*ratelimit.TokenBucket
}

// NewTokenBucket creates a new token bucket.
// ratePerSec is the number of tokens added per second.
// burst is the maximum accumulated tokens. If burst <= 0, defaults to 1.
func NewTokenBucket(ratePerSec, burst int) *TokenBucket {
	if burst <= 0 {
		burst = 1
	}
	return &TokenBucket{
		TokenBucket: ratelimit.NewTokenBucket(burst, float64(ratePerSec)),
	}
}

// Stop prevents further token refill by setting the fill rate to zero.
func (tb *TokenBucket) Stop() {
	tb.TokenBucket.SetRate(0)
}
