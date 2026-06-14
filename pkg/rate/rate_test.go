// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package rate

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenBucket_New validates that NewTokenBucket creates a usable bucket
// with the correct initial capacity (burst).
func TestTokenBucket_New(t *testing.T) {
	tb := NewTokenBucket(10, 5)
	defer tb.Stop()

	// A fresh bucket with burst=5 should allow exactly 5 requests.
	require.NotNil(t, tb)
	for i := 0; i < 5; i++ {
		assert.True(t, tb.Allow(), "request %d should be allowed (burst)", i+1)
	}
	// The 6th request should be denied (bucket empty).
	assert.False(t, tb.Allow(), "request beyond burst should be denied")
}

// TestTokenBucket_ZeroRate creates a bucket with rate=0, meaning no tokens
// are added over time (the bucket stays at initial burst capacity but never
// refills beyond that).
func TestTokenBucket_ZeroRate(t *testing.T) {
	tb := NewTokenBucket(0, 3)
	defer tb.Stop()

	// Burst of 3 allows all 3 initial tokens.
	for i := 0; i < 3; i++ {
		assert.True(t, tb.Allow(), "initial token %d should be available", i+1)
	}
	// No refill happens, so all subsequent requests should be denied.
	assert.False(t, tb.Allow(), "no refill — should be denied")
	assert.False(t, tb.Allow(), "still no refill — should be denied")
}

// TestTokenBucket_Refill verifies that the bucket refills over time at the
// configured rate. With rate=1000 and burst=1, one token refills quickly.
func TestTokenBucket_Refill(t *testing.T) {
	// Use a high refill rate so the token arrives within the test's time budget.
	tb := NewTokenBucket(1000, 1)
	defer tb.Stop()

	// Consume the initial token.
	assert.True(t, tb.Allow())
	assert.False(t, tb.Allow(), "no token yet — should be denied")

	// Wait long enough for at least one token to refill.
	// At 1000 tokens/sec, 1 token arrives every 1ms.
	time.Sleep(15 * time.Millisecond)

	assert.True(t, tb.Allow(), "token should have been refilled")
}

// TestTokenBucket_StopIsSafe verifies that calling Stop() multiple times
// does not panic or deadlock, and that remaining tokens can still be consumed.
func TestTokenBucket_StopIsSafe(t *testing.T) {
	tb := NewTokenBucket(10, 3)

	// Call Stop multiple times — must not panic.
	tb.Stop()
	tb.Stop()
	tb.Stop()
}

// TestTokenBucket_ConcurrentAccess verifies that Allow() is safe for
// concurrent access. Uses rate=0 so no refill occurs during the race.
func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := NewTokenBucket(0, 100)
	defer tb.Stop()

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// Spawn 200 goroutines all calling Allow(). The first 100 should
	// succeed, the next 100 should fail (no refill with rate=0).
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- tb.Allow()
		}()
	}

	wg.Wait()
	close(allowed)

	var allowedCount, deniedCount int
	for ok := range allowed {
		if ok {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	// With rate=0 and burst=100, exactly 100 tokens are available.
	// No refill happens during the race, so the count is deterministic.
	assert.Equal(t, 100, allowedCount, "exactly burst-sized tokens should be allowed")
	assert.Equal(t, 100, deniedCount, "remaining calls should be denied")
}

// TestTokenBucket_ZeroBurst creates a bucket with burst=0, which should
// allow no requests.
func TestTokenBucket_ZeroBurst(t *testing.T) {
	tb := NewTokenBucket(10, 0)
	defer tb.Stop()

	for i := 0; i < 5; i++ {
		assert.False(t, tb.Allow(), "zero-burst bucket should always deny")
	}
}

// TestTokenBucket_HighBurst verifies that a high burst value allows exactly
// that many requests before denying.
func TestTokenBucket_HighBurst(t *testing.T) {
	tb := NewTokenBucket(100, 1000)
	defer tb.Stop()

	for i := 0; i < 1000; i++ {
		require.True(t, tb.Allow(), "request %d within burst should be allowed", i+1)
	}
	assert.False(t, tb.Allow(), "burst exhausted — should deny")
}

// TestTokenBucket_SequentialAllowConsistency verifies that Allow() returns
// consistent results for trivial sequential consumption.
func TestTokenBucket_SequentialAllowConsistency(t *testing.T) {
	tb := NewTokenBucket(10, 3)
	defer tb.Stop()

	results := make([]bool, 6)
	for i := 0; i < 6; i++ {
		results[i] = tb.Allow()
	}

	assert.Equal(t, []bool{true, true, true, false, false, false}, results,
		"expected: 3 allowed (burst), then 3 denied")
}
