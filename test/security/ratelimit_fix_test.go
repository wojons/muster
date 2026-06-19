package security

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/auth/ratelimit"
)

// TestRateLimit_NegativeTokenBypass_Fixed tests that negative tokens cannot bypass rate limiting
func TestRateLimit_NegativeTokenBypass_Fixed(t *testing.T) {
	// Create a token bucket with capacity 10, fill rate 1 per second
	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Consume 5 tokens
	allowed := tb.AllowN(5)
	require.True(t, allowed, "Should allow 5 tokens")

	// CRITICAL: Attempt to bypass with negative tokens — should not add tokens
	allowed = tb.AllowN(-5)
	require.True(t, allowed, "Should allow negative tokens (consumes 0)")

	// After negative bypass attempt, we should still have ~5 tokens (plus tiny refill)
	// AllowN(6) should succeed because 5 remaining + refill ≈ 5+
	// But AllowN(11) should fail (capacity is 10)
	allowed = tb.AllowN(11)
	require.False(t, allowed, "Should deny 11 tokens — negative didn't add tokens")
}

// TestRateLimit_ZeroTokens tests that zero tokens consume nothing
func TestRateLimit_ZeroTokens(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		require.True(t, tb.Allow(), "Should allow token %d", i+1)
	}

	// Zero tokens should be allowed (consumes nothing)
	allowed := tb.AllowN(0)
	require.True(t, allowed, "Should allow request with 0 tokens")

	// Bucket should still be empty — next Allow should fail
	allowed = tb.Allow()
	require.False(t, allowed, "Should deny after bucket emptied")
}

// TestRateLimit_LargeTokenRequest tests that requesting more tokens than capacity is handled
func TestRateLimit_LargeTokenRequest(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Request more tokens than capacity
	allowed := tb.AllowN(100)
	require.False(t, allowed, "Should deny request for more tokens than capacity")

	// Normal request should still work (capacity wasn't consumed)
	allowed = tb.Allow()
	require.True(t, allowed, "Should allow normal request after denied large request")
}

// TestRateLimit_ConcurrentNegativeTokenAttack tests concurrent negative token attacks
func TestRateLimit_ConcurrentNegativeTokenAttack(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Try negative token attack
			tb.AllowN(-1)
			tb.AllowN(-100)
		}()
	}

	wg.Wait()

	// After concurrent negative attacks, the bucket should NOT have more tokens than capacity
	// AllowN(11) should fail (capacity is 10, and negative didn't add tokens)
	allowed := tb.AllowN(11)
	require.False(t, allowed,
		"Concurrent negative token attacks should not increase tokens beyond capacity")
}

// TestRateLimit_TokenBucketLimiter tests TokenBucketLimiter with the current API
func TestRateLimit_TokenBucketLimiter(t *testing.T) {
	limiter := ratelimit.NewTokenBucketLimiter(5, 5) // 5 req/s, burst 5

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		allowed := limiter.Allow()
		require.True(t, allowed, "Should allow request %d", i+1)
	}

	// 6th request should be denied (burst exhausted)
	allowed := limiter.Allow()
	require.False(t, allowed, "Should deny after burst exhausted")

	// limiter.Wait() should not panic
	// (not calling it here because it blocks for 100ms+)
}

// TestRateLimit_ExploitScenario reproduces the exploit scenario
func TestRateLimit_ExploitScenario(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Step 1: Normal consumption — use 5 tokens
	allowed := tb.AllowN(5)
	require.True(t, allowed, "Should allow 5 tokens")

	// Step 2: Attacker tries negative tokens — should NOT add tokens
	allowed = tb.AllowN(-5)
	require.True(t, allowed, "Should allow negative (consumes 0)")

	// Step 3: Verify attacker can't consume more than actually available
	// After using 5, we have ~5 left (plus tiny refill). AllowN(10) should fail.
	allowed = tb.AllowN(10)
	require.False(t, allowed,
		"Attacker should not be able to consume more than available tokens")
}
