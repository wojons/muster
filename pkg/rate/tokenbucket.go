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
// burst is the maximum accumulated tokens.
func NewTokenBucket(ratePerSec, burst int) *TokenBucket {
	return &TokenBucket{
		TokenBucket: ratelimit.NewTokenBucket(burst, float64(ratePerSec)),
	}
}

// Stop is a no-op — the underlying implementation has no background goroutine.
func (tb *TokenBucket) Stop() {}
