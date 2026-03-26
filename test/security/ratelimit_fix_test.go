package security

import (
	"sync"
	"testing"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/auth/ratelimit"
	"github.com/stretchr/testify/require"
)

// TestRateLimit_NegativeTokenBypass_Fixed tests that negative tokens cannot bypass rate limiting
func TestRateLimit_NegativeTokenBypass_Fixed(t *testing.T) {
	// Create a token bucket with capacity 10, fill rate 1 per second
	tb := ratelimit.NewTokenBucket(10, 1.0)

	initialTokens := tb.Tokens()
	require.Equal(t, float64(10), initialTokens, "Initial tokens should be 10")

	// Normal consumption
	allowed := tb.Allow(3)
	require.True(t, allowed, "Should allow 3 tokens")
	require.LessOrEqual(t, tb.Tokens(), float64(7.5), "Should have consumed ~3 tokens")

	// CRITICAL TEST: Attempt to bypass with negative tokens
	// This should NOT add tokens to the bucket
	tokensBefore := tb.Tokens()
	allowed = tb.Allow(-5)
	require.True(t, allowed, "Should allow request with negative tokens (consumes 0)")

	// Tokens should NOT increase (or should only increase by natural refill)
	tokensAfter := tb.Tokens()

	// Calculate expected natural refill (should be minimal since operations are fast)
	maxRefillTime := 1.0 * time.Second
	expectedMaxRefill := maxRefillTime.Seconds() * 1.0 // fill rate is 1.0

	// Tokens should not increase by more than natural refill
	maxExpectedTokens := tokensBefore + expectedMaxRefill
	require.LessOrEqual(t, tokensAfter, maxExpectedTokens,
		"Tokens should not increase from negative Allow() call (only natural refill)")

	// Verify tokens didn't jump by 5 (which would indicate the bypass worked)
	require.Less(t, tokensAfter, tokensBefore+4.0,
		"Tokens should not increase by 5 (negative token bypass prevented)")
}

// TestRateLimit_ZeroTokens tests that zero tokens consume nothing
func TestRateLimit_ZeroTokens(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	tokensBefore := tb.Tokens()
	allowed := tb.Allow(0)
	require.True(t, allowed, "Should allow request with 0 tokens")

	// Tokens should remain roughly the same (only natural refill)
	time.Sleep(10 * time.Millisecond) // Small delay for timing
	tokensAfter := tb.Tokens()

	// Should be within small refill amount
	require.Less(t, tokensAfter-tokensBefore, 0.1, "Zero tokens should consume nothing")
}

// TestRateLimit_LargeTokenRequest tests that requesting more tokens than capacity is handled
func TestRateLimit_LargeTokenRequest(t *testing.T) {
	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Request more tokens than capacity
	allowed := tb.Allow(100)
	require.False(t, allowed, "Should deny request for more tokens than capacity")

	// Tokens should remain unchanged
	require.Equal(t, float64(10), tb.Tokens(), "Tokens should not be consumed on denial")
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
			tb.Allow(-1)
			tb.Allow(-100)
		}()
	}

	wg.Wait()

	// Tokens should not exceed capacity (plus minimal refill)
	time.Sleep(10 * time.Millisecond)
	finalTokens := tb.Tokens()

	// Allow for small refill, but should not exceed capacity significantly
	require.LessOrEqual(t, finalTokens, float64(15),
		"Concurrent negative token attacks should not increase tokens beyond capacity")
}

// TestRateLimit_BucketLimiter_NegativeTokens tests TokenBucketLimiter with negative tokens
func TestRateLimit_BucketLimiter_NegativeTokens(t *testing.T) {
	config := ratelimit.BucketConfig{
		Capacity: 10,
		FillRate: 1.0,
	}
	limiter := ratelimit.NewTokenBucketLimiter(config)

	key := "test-key"

	// Initial allowance
	allowed := limiter.Allow(key, 3)
	require.True(t, allowed, "Should allow initial request")

	// Try negative token attack
	tokensBefore := limiter.GetBucket(key).Tokens()
	allowed = limiter.Allow(key, -5)
	require.True(t, allowed, "Should allow negative token request (consumes 0)")
	tokensAfter := limiter.GetBucket(key).Tokens()

	// Tokens should not increase significantly
	require.Less(t, tokensAfter-tokensBefore, 1.0,
		"Negative tokens should not add tokens to bucket")
}

// TestRateLimit_ExploitScenario reproduces the exploit scenario from exploits/ratelimit/main.go
func TestRateLimit_ExploitScenario(t *testing.T) {
	// This test proves the exploit is fixed in production code

	tb := ratelimit.NewTokenBucket(10, 1.0)

	// Step 1: Normal consumption
	allowed := tb.Allow(3)
	require.True(t, allowed)
	tokensAfterConsumption := tb.Tokens()

	// Step 2: Attacker tries negative tokens
	// In the FIXED version, this should NOT add tokens
	allowed = tb.Allow(-5)
	require.True(t, allowed, "Should allow (consumes 0 tokens)")

	// Step 3: Verify tokens didn't increase by 5
	tokensAfterAttack := tb.Tokens()

	// Calculate natural refill (should be minimal)
	naturalRefill := tokensAfterAttack - tokensAfterConsumption

	// Natural refill should be very small (operations are nearly instantaneous)
	// If the bypass worked, naturalRefill would be ~5.0
	require.Less(t, naturalRefill, 1.0,
		"Natural refill should be minimal; if negative bypass worked, would be ~5.0")

	// Step 4: Attacker tries to consume extra tokens
	// Should only be able to consume what's actually in the bucket
	consumedCount := 0
	for i := 0; i < 15; i++ {
		if tb.Allow(1) {
			consumedCount++
		}
	}

	// Should only be able to consume tokens that were actually in bucket
	// (approximately tokensAfterConsumption + minimal refill)
	maxExpectedConsumption := int(tokensAfterConsumption) + 2 // Allow 2 for refill
	require.LessOrEqual(t, consumedCount, maxExpectedConsumption,
		"Attacker should not be able to consume more than available tokens")
}
