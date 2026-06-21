package ratelimit

import (
	"testing"
	"time"
)

// ── NewTokenBucketLimiter ────────────────────────────────────

func TestNewTokenBucketLimiter_Valid(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 5)
	if limiter == nil {
		t.Fatal("NewTokenBucketLimiter returned nil")
	}
	if limiter.bucket == nil {
		t.Fatal("limiter.bucket is nil")
	}
	if limiter.bucket.fillRate != 10.0 {
		t.Errorf("fillRate = %f, want 10.0", limiter.bucket.fillRate)
	}
	if limiter.bucket.capacity != 5 {
		t.Errorf("capacity = %d, want 5", limiter.bucket.capacity)
	}
}

func TestNewTokenBucketLimiter_ZeroRate(t *testing.T) {
	limiter := NewTokenBucketLimiter(0, 10)
	if limiter.bucket.fillRate != 0 {
		t.Errorf("fillRate = %f, want 0", limiter.bucket.fillRate)
	}
}

func TestNewTokenBucketLimiter_ZeroBurst(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 0)
	if limiter.bucket.capacity != 0 {
		t.Errorf("capacity = %d, want 0", limiter.bucket.capacity)
	}
}

// ── Allow ────────────────────────────────────────────────────

func TestTokenBucketLimiter_Allow_UnderLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 10)
	for i := 0; i < 10; i++ {
		if !limiter.Allow() {
			t.Fatalf("Allow #%d should succeed under limit", i+1)
		}
	}
}

func TestTokenBucketLimiter_Allow_Exhausted(t *testing.T) {
	limiter := NewTokenBucketLimiter(0, 3) // 3 burst, no refill
	for i := 0; i < 3; i++ {
		if !limiter.Allow() {
			t.Fatalf("Allow #%d should succeed", i+1)
		}
	}
	if limiter.Allow() {
		t.Error("Allow after exhausting burst should return false")
	}
}

func TestTokenBucketLimiter_Allow_NilLimiter(t *testing.T) {
	var limiter *TokenBucketLimiter
	if !limiter.Allow() {
		t.Error("Allow on nil limiter should return true")
	}
}

func TestTokenBucketLimiter_Allow_NilBucket(t *testing.T) {
	limiter := &TokenBucketLimiter{bucket: nil}
	if !limiter.Allow() {
		t.Error("Allow with nil bucket should return true")
	}
}

func TestTokenBucketLimiter_Allow_Refill(t *testing.T) {
	limiter := NewTokenBucketLimiter(500, 3)
	for i := 0; i < 3; i++ {
		_ = limiter.Allow()
	}
	if limiter.Allow() {
		t.Error("Allow immediately after burst should fail")
	}
	// Wait for refill of ~1 token (500 t/s, 3ms refills ~1.5 tokens, capped at capacity but only need 1)
	time.Sleep(5 * time.Millisecond)
	if !limiter.Allow() {
		t.Error("Allow after refill should succeed")
	}
}

// ── Wait ─────────────────────────────────────────────────────

func TestTokenBucketLimiter_Wait_UnderLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 10)
	done := make(chan bool)
	go func() {
		limiter.Wait()
		done <- true
	}()
	select {
	case <-done:
		// Expected — should not block
	case <-time.After(time.Second):
		t.Error("Wait should not block under limit")
	}
}

func TestTokenBucketLimiter_Wait_NilLimiter(t *testing.T) {
	var limiter *TokenBucketLimiter
	done := make(chan bool)
	go func() {
		limiter.Wait()
		done <- true
	}()
	select {
	case <-done:
		// Should not panic
	case <-time.After(time.Second):
		t.Error("Wait on nil limiter should not block")
	}
}

func TestTokenBucketLimiter_Wait_NilBucket(t *testing.T) {
	limiter := &TokenBucketLimiter{bucket: nil}
	done := make(chan bool)
	go func() {
		limiter.Wait()
		done <- true
	}()
	select {
	case <-done:
		// Should not panic
	case <-time.After(time.Second):
		t.Error("Wait with nil bucket should not block")
	}
}

func TestTokenBucketLimiter_Wait_EventuallyUnblocks(t *testing.T) {
	limiter := NewTokenBucketLimiter(500, 2)
	// Exhaust burst
	for i := 0; i < 2; i++ {
		_ = limiter.Allow()
	}

	done := make(chan bool, 1)
	go func() {
		limiter.Wait()
		done <- true
	}()

	// Wait should block briefly then unblock after refill
	select {
	case <-done:
		// Expected — unblocked
	case <-time.After(2 * time.Second):
		t.Error("Wait should eventually unblock after refill")
	}
}

// ── Rate ─────────────────────────────────────────────────────

func TestTokenBucketLimiter_Rate_Valid(t *testing.T) {
	limiter := NewTokenBucketLimiter(50, 10)
	if r := limiter.Rate(); r != 50.0 {
		t.Errorf("Rate() = %f, want 50.0", r)
	}
}

func TestTokenBucketLimiter_Rate_NilLimiter(t *testing.T) {
	var limiter *TokenBucketLimiter
	if r := limiter.Rate(); r != 0 {
		t.Errorf("Rate() on nil limiter = %f, want 0", r)
	}
}

func TestTokenBucketLimiter_Rate_NilBucket(t *testing.T) {
	limiter := &TokenBucketLimiter{bucket: nil}
	if r := limiter.Rate(); r != 0 {
		t.Errorf("Rate() with nil bucket = %f, want 0", r)
	}
}

func TestTokenBucketLimiter_Rate_ZeroRate(t *testing.T) {
	limiter := NewTokenBucketLimiter(0, 10)
	if r := limiter.Rate(); r != 0 {
		t.Errorf("Rate() = %f, want 0", r)
	}
}
