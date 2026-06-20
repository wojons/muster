package rate

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenBucket_Allow_BasicAllowDeny tests that a bucket with rate=5, burst=5
// allows the first 5 calls and denies the 6th.
func TestTokenBucket_Allow_BasicAllowDeny(t *testing.T) {
	tb := NewTokenBucket(5, 5)
	defer tb.Stop()

	for i := 0; i < 5; i++ {
		assert.True(t, tb.Allow(), "Allow %d should succeed (burst tokens available)", i+1)
	}

	assert.False(t, tb.Allow(), "6th Allow should be rate limited")
}

// TestTokenBucket_Allow_Burst tests that burst is respected: with rate=1, burst=10,
// 10 rapid calls succeed and the 11th is denied.
func TestTokenBucket_Allow_Burst(t *testing.T) {
	tb := NewTokenBucket(1, 10)
	defer tb.Stop()

	for i := 0; i < 10; i++ {
		assert.True(t, tb.Allow(), "Allow %d in burst should succeed", i+1)
	}

	assert.False(t, tb.Allow(), "11th Allow should be rate limited (burst exhausted)")
}

// TestTokenBucket_Allow_Refill tests that tokens are refilled over time.
// With rate=10, burst=5: exhaust tokens, wait for a tick, verify a token appears.
func TestTokenBucket_Allow_Refill(t *testing.T) {
	tb := NewTokenBucket(10, 5)
	defer tb.Stop()

	// Exhaust all burst tokens
	for i := 0; i < 5; i++ {
		require.True(t, tb.Allow(), "Allow %d should exhaust tokens", i+1)
	}

	// Wait for the ticker to fire at least once (ticker fires every 100ms)
	time.Sleep(150 * time.Millisecond)

	// Should have refilled at least 1 token (rate 10/sec × 0.1s tick = 1 token/tick)
	assert.GreaterOrEqual(t, tb.Tokens(), float64(1),
		"Should have refilled at least 1 token after waiting")
	assert.True(t, tb.Allow(),
		"Should allow after refill")

	// Next immediate call should fail (only 1 token was refilled and consumed)
	assert.False(t, tb.Allow(),
		"Should be rate limited again after consuming refilled token")
}

// TestTokenBucket_AllowN tests consuming multiple tokens at once.
func TestTokenBucket_AllowN(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	defer tb.Stop()

	// Consume 3 tokens
	assert.True(t, tb.AllowN(3), "AllowN(3) should succeed")

	// Try to consume 8 tokens (only 7 left)
	assert.False(t, tb.AllowN(8), "AllowN(8) should fail (only 7 remaining)")

	// Consume remaining 7 tokens
	assert.True(t, tb.AllowN(7), "AllowN(7) should consume remaining tokens")

	// Bucket should be empty
	assert.False(t, tb.Allow(), "Allow should fail on empty bucket")
}

// TestTokenBucket_AllowN_NonPositive tests that AllowN with n <= 0 returns true
// without consuming tokens.
func TestTokenBucket_AllowN_NonPositive(t *testing.T) {
	tb := NewTokenBucket(5, 5)
	defer tb.Stop()

	// Zero and negative n should succeed without consuming tokens
	assert.True(t, tb.AllowN(0), "AllowN(0) should succeed")
	assert.True(t, tb.AllowN(-1), "AllowN(-1) should succeed")

	// All 5 burst tokens should still be available
	for i := 0; i < 5; i++ {
		assert.True(t, tb.Allow(), "Allow %d should still have burst tokens", i+1)
	}

	// Now bucket should be empty
	assert.False(t, tb.Allow(), "Should fail after consuming all tokens")
}

// TestTokenBucket_Tokens verifies Tokens() returns accurate counts.
func TestTokenBucket_Tokens(t *testing.T) {
	tb := NewTokenBucket(1, 10)
	defer tb.Stop()

	// Initial tokens should equal burst
	initial := tb.Tokens()
	assert.InDelta(t, float64(10), initial, 0.5,
		"Initial tokens should approximately equal burst")

	// After consuming 3 tokens
	tb.AllowN(3)
	after3 := tb.Tokens()
	assert.InDelta(t, initial-3, after3, 0.5,
		"After AllowN(3), tokens should decrease by 3")

	// After consuming 1 more
	tb.Allow()
	after4 := tb.Tokens()
	assert.InDelta(t, after3-1, after4, 0.5,
		"After Allow(), tokens should decrease by 1")
}

// TestTokenBucket_SetRate verifies that changing the rate at runtime takes effect.
func TestTokenBucket_SetRate(t *testing.T) {
	tb := NewTokenBucket(100, 10)
	defer tb.Stop()

	// Consume all initial tokens
	for i := 0; i < 10; i++ {
		require.True(t, tb.Allow(), "Allow %d should succeed", i+1)
	}

	// Set rate to 0 (stop refill)
	tb.SetRate(0)

	// Wait for any tick that might try to refill
	time.Sleep(150 * time.Millisecond)

	// Should have no tokens (rate was set to 0 before any refill tick could add tokens)
	tokens := tb.Tokens()
	assert.Less(t, tokens, float64(1),
		"With rate=0, should have no refilled tokens (got %f)", tokens)
}

// TestTokenBucket_Stop_Idempotent verifies that Stop() can be called
// multiple times without panicking.
func TestTokenBucket_Stop_Idempotent(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// Stop twice - second call should be a no-op
	tb.Stop()
	tb.Stop()
}

// TestTokenBucket_ZeroRate verifies that a bucket created with rate=0
// allows initial burst but never refills.
func TestTokenBucket_ZeroRate(t *testing.T) {
	tb := NewTokenBucket(0, 5)
	defer tb.Stop()

	// Can consume initial burst
	for i := 0; i < 5; i++ {
		assert.True(t, tb.Allow(), "Allow %d with rate=0 should succeed", i+1)
	}

	// No more tokens and no refill
	assert.False(t, tb.Allow(), "Allow with empty bucket and rate=0 should fail")
}

// TestTokenBucket_ConcurrentAccess spawns 10 goroutines each calling
// Allow() 10 times and verifies no race condition and correct accounting.
// Run with -race to detect data races.
func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := NewTokenBucket(5, 50)
	defer tb.Stop()

	const numGoroutines = 10
	const callsPerGoroutine = 10
	totalCalls := numGoroutines * callsPerGoroutine

	var wg sync.WaitGroup
	results := make(chan bool, totalCalls)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				results <- tb.Allow()
			}
		}()
	}

	wg.Wait()
	close(results)

	totalAllowed := 0
	totalDenied := 0
	for r := range results {
		if r {
			totalAllowed++
		} else {
			totalDenied++
		}
	}

	t.Logf("Concurrent access: allowed=%d denied=%d total=%d",
		totalAllowed, totalDenied, totalCalls)

	assert.Equal(t, totalCalls, totalAllowed+totalDenied,
		"All %d attempts must be accounted for", totalCalls)

	// At minimum the burst capacity should be allowed
	assert.GreaterOrEqual(t, totalAllowed, 50,
		"At least burst (50) should be allowed among %d concurrent calls", totalCalls)
}

// TestTokenBucket_DefaultBurst verifies that a zero or negative burst
// defaults to 1.
func TestTokenBucket_DefaultBurst(t *testing.T) {
	// Zero burst defaults to 1
	tb1 := NewTokenBucket(1, 0)
	defer tb1.Stop()
	assert.True(t, tb1.Allow(), "Burst default 1: first call should succeed")
	assert.False(t, tb1.Allow(), "Burst default 1: second call should fail")

	// Negative burst defaults to 1
	tb2 := NewTokenBucket(1, -5)
	defer tb2.Stop()
	assert.True(t, tb2.Allow(), "Burst defaulted from -5: first call should succeed")
	assert.False(t, tb2.Allow(), "Burst defaulted from -5: second call should fail")
}

// TestTokenBucket_Stop_PreventsRefill verifies that after Stop(), no new
// tokens are added to the bucket.
func TestTokenBucket_Stop_PreventsRefill(t *testing.T) {
	tb := NewTokenBucket(100, 10)

	// Consume initial burst
	for i := 0; i < 10; i++ {
		require.True(t, tb.Allow(), "Allow %d before Stop should succeed", i+1)
	}

	// Stop the bucket
	tb.Stop()

	// Wait for any potential tick
	time.Sleep(200 * time.Millisecond)

	// No tokens should have been added (Stop stopped the goroutine)
	assert.Less(t, tb.Tokens(), float64(1),
		"After Stop, no new tokens should be added")
}
