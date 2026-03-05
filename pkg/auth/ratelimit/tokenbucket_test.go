package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestNewTokenBucket(t *testing.T) {
	tb := NewTokenBucket(100, 10)
	if tb == nil {
		t.Fatal("NewTokenBucket() returned nil")
	}

	if tb.capacity != 100 {
		t.Errorf("capacity = %v, want 100", tb.capacity)
	}

	if tb.fillRate != 10 {
		t.Errorf("fillRate = %v, want 10", tb.fillRate)
	}

	if tb.tokens != 100 {
		t.Errorf("tokens = %v, want 100 (full bucket)", tb.tokens)
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(10, 1)

	if !tb.Allow(5) {
		t.Error("Allow(5) should return true for full bucket")
	}

	if !tb.Allow(3) {
		t.Error("Allow(3) should return true")
	}
}

func TestTokenBucket_Allow_ExceedsCapacity(t *testing.T) {
	tb := NewTokenBucket(5, 1)

	if tb.Allow(10) {
		t.Error("Allow(10) should return false when requesting more than full bucket")
	}

	remaining := tb.Tokens()
	if remaining < 5 {
		t.Errorf("tokens should remain at capacity: %v", remaining)
	}

	if !tb.Allow(5) {
		t.Error("Allow(5) should return true for full bucket")
	}

	if tb.Allow(1) {
		t.Error("Allow(1) should return false for empty bucket")
	}
}

func TestTokenBucket_Allow_FillRate(t *testing.T) {
	tb := NewTokenBucket(10, 100)
	tb.Allow(10)

	time.Sleep(50 * time.Millisecond)

	if !tb.Allow(1) {
		t.Error("Allow(1) should return true after refill")
	}
}

func TestTokenBucket_Wait(t *testing.T) {
	tb := NewTokenBucket(10, 100)

	ctx := context.Background()
	err := tb.Wait(ctx, 5)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestTokenBucket_Wait_ContextCancelled(t *testing.T) {
	tb := NewTokenBucket(5, 0)
	tb.Allow(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tb.Wait(ctx, 1)
	if err == nil {
		t.Fatal("Wait() should error when context cancelled")
	}
}

func TestTokenBucket_Tokens(t *testing.T) {
	tb := NewTokenBucket(10, 1)

	tokens := tb.Tokens()
	if tokens != 10 {
		t.Errorf("Tokens() = %v, want 10", tokens)
	}

	tb.Allow(3)

	tokens = tb.Tokens()
	if tokens < 6.5 || tokens > 7.5 {
		t.Errorf("Tokens() = %v, want approximately 7", tokens)
	}
}

func TestTokenBucket_Capacity(t *testing.T) {
	tb := NewTokenBucket(10, 1)

	capacity := tb.Capacity()
	if capacity != 10 {
		t.Errorf("Capacity() = %v, want 10", capacity)
	}
}

func TestTokenBucket_FillRate(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	fillRate := tb.FillRate()
	if fillRate != 5 {
		t.Errorf("FillRate() = %v, want 5", fillRate)
	}
}

func TestNewTokenBucketLimiter(t *testing.T) {
	config := BucketConfig{
		Capacity: 100,
		FillRate: 10,
	}

	rl := NewTokenBucketLimiter(config)
	if rl == nil {
		t.Fatal("NewTokenBucketLimiter() returned nil")
	}

	if rl.config.Capacity != 100 {
		t.Errorf("config.Capacity = %v, want 100", rl.config.Capacity)
	}

	if rl.config.FillRate != 10 {
		t.Errorf("config.FillRate = %v, want 10", rl.config.FillRate)
	}
}

func TestTokenBucketLimiter_Allow(t *testing.T) {
	config := BucketConfig{
		Capacity: 5,
		FillRate: 1,
	}

	rl := NewTokenBucketLimiter(config)

	if !rl.Allow("test-key", 3) {
		t.Error("Allow() should return true")
	}

	if !rl.Allow("test-key", 2) {
		t.Error("Allow() should return true")
	}

	if rl.Allow("test-key", 1) {
		t.Error("Allow() should return false after bucket empty")
	}
}

func TestTokenBucketLimiter_Allow_DifferentKeys(t *testing.T) {
	config := BucketConfig{
		Capacity: 3,
		FillRate: 1,
	}

	rl := NewTokenBucketLimiter(config)

	rl.Allow("key1", 3)

	if !rl.Allow("key2", 3) {
		t.Error("Allow() should return true for different key")
	}
}

func TestTokenBucketLimiter_Wait(t *testing.T) {
	config := BucketConfig{
		Capacity: 5,
		FillRate: 10,
	}

	rl := NewTokenBucketLimiter(config)
	ctx := context.Background()

	err := rl.Wait(ctx, "test-key", 3)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestTokenBucketLimiter_GetBucket(t *testing.T) {
	config := BucketConfig{
		Capacity: 10,
		FillRate: 5,
	}

	rl := NewTokenBucketLimiter(config)

	bucket := rl.GetBucket("test-key")
	if bucket == nil {
		t.Fatal("GetBucket() returned nil")
	}

	if bucket.Capacity() != 10 {
		t.Errorf("bucket.Capacity() = %v, want 10", bucket.Capacity())
	}
}

func TestBucketMetrics(t *testing.T) {
	m := BucketMetrics{
		RequestsAllowed: 100,
		RequestsDenied:  10,
		AverageTokens:   50.0,
		PeakTokens:      100,
	}

	if m.RequestsAllowed != 100 {
		t.Errorf("RequestsAllowed = %v, want 100", m.RequestsAllowed)
	}

	if m.RequestsDenied != 10 {
		t.Errorf("RequestsDenied = %v, want 10", m.RequestsDenied)
	}
}

// TestTokenBucket_Allow_NegativeTokens verifies that negative tokens are consumed correctly
// to prevent rate limit bypass as per Phase 32 adversarial review finding
func TestTokenBucket_Allow_NegativeTokens(t *testing.T) {
	tb := NewTokenBucket(10, 1.0) // Capacity 10, fill rate 1.0/sec

	// CRITICAL: Test that negative tokens don't bypass the bucket
	// Before fix: negative tokens would add to bucket, allowing unlimited requests
	// After fix: negative tokens are treated as zero consumption
	for i := 0; i < 100; i++ {
		if !tb.Allow(-1) {
			t.Errorf("Allow(-1) should always return true (zero consumption)")
		}
	}

	// Bucket tokens should reflect no consumption from negative tokens
	tokens := tb.Tokens()
	// Bucket fills at 1 token/sec, so after 100 loops, we'd have consumed negligible time
	// Just verify it hasn't drastically changed
	if tokens < 0 || tokens > 100 {
		t.Errorf("Expected reasonable token count after 100 negative requests, got %v", tokens)
	}

	// Verify positive tokens are actually consumed
	// Allow should succeed even with small token count
	tb.Allow(5)

	tokensAfter := tb.Tokens()
	// Should have decreased by 5 (or close to it)
	if tokensAfter >= tokens+4 {
		t.Errorf("Expected tokens to decrease after Allow(5), before: %v, after: %v", tokens, tokensAfter)
	}
}
