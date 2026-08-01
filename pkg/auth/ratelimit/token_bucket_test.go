package ratelimit

import (
	"sync"
	"testing"
	"time"
)

// ── NewTokenBucket ──────────────────────────────────────────

func TestNewTokenBucket_Defaults(t *testing.T) {
	tb := NewTokenBucket(10, 5.0)
	if tb == nil {
		t.Fatal("NewTokenBucket returned nil")
	}
	if tb.capacity != 10 {
		t.Errorf("capacity = %d, want 10", tb.capacity)
	}
	if tb.fillRate != 5.0 {
		t.Errorf("fillRate = %f, want 5.0", tb.fillRate)
	}
	if tb.tokens != 10.0 {
		t.Errorf("initial tokens = %f, want 10.0", tb.tokens)
	}
}

func TestNewTokenBucket_CustomCapacityAndRate(t *testing.T) {
	tb := NewTokenBucket(50, 25.5)
	if tb.capacity != 50 {
		t.Errorf("capacity = %d, want 50", tb.capacity)
	}
	if tb.fillRate != 25.5 {
		t.Errorf("fillRate = %f, want 25.5", tb.fillRate)
	}
	if tb.tokens != 50.0 {
		t.Errorf("initial tokens = %f, want 50.0", tb.tokens)
	}
}

func TestNewTokenBucket_ZeroCapacity(t *testing.T) {
	tb := NewTokenBucket(0, 10.0)
	if tb.capacity != 0 {
		t.Errorf("capacity = %d, want 0", tb.capacity)
	}
	// With zero capacity, Allow should return false (no tokens)
	// First Allow always succeeds because initial tokens = capacity = 0, but AllowN(1) checks tokens >= 1 → false
	if tb.Allow() {
		t.Error("Allow with zero capacity should return false after initial burst")
	}
}

func TestNewTokenBucket_ZeroFillRate(t *testing.T) {
	tb := NewTokenBucket(5, 0)
	if tb.fillRate != 0 {
		t.Errorf("fillRate = %f, want 0", tb.fillRate)
	}
	// With zero fill rate, initial tokens = capacity (5)
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("Allow #%d should succeed with initial tokens", i+1)
		}
	}
	// Sixth should fail — zero fill rate, no refill
	if tb.Allow() {
		t.Error("Allow after exhausting tokens with zero fill rate should return false")
	}
}

// ── Allow ────────────────────────────────────────────────────

func TestTokenBucket_Allow_BurstThenBlock(t *testing.T) {
	tb := NewTokenBucket(3, 0) // 3 burst, no refill
	for i := 0; i < 3; i++ {
		if !tb.Allow() {
			t.Fatalf("Allow #%d should succeed", i+1)
		}
	}
	if tb.Allow() {
		t.Error("Allow after exhausting burst should return false")
	}
}

func TestTokenBucket_Allow_Refill(t *testing.T) {
	tb := NewTokenBucket(2, 100.0) // 100 tokens/second
	// Burn both tokens
	if !tb.Allow() {
		t.Fatal("first allow failed")
	}
	if !tb.Allow() {
		t.Fatal("second allow failed")
	}
	if tb.Allow() {
		t.Error("third allow should fail immediately after burning both tokens")
	}
	// Wait for refill
	time.Sleep(30 * time.Millisecond) // 100 t/s * 0.03s = 3 tokens refilled → capped at 2
	if !tb.Allow() {
		t.Error("allow after refill should succeed")
	}
}

// ── AllowN ───────────────────────────────────────────────────

func TestTokenBucket_AllowN_Valid(t *testing.T) {
	tb := NewTokenBucket(5, 0)
	if !tb.AllowN(3) {
		t.Error("AllowN(3) with 5 tokens should succeed")
	}
	if !tb.AllowN(2) {
		t.Error("AllowN(2) with 2 remaining tokens should succeed")
	}
	if tb.AllowN(1) {
		t.Error("AllowN(1) with 0 tokens should fail")
	}
}

func TestTokenBucket_AllowN_Zero(t *testing.T) {
	tb := NewTokenBucket(1, 0)
	_ = tb.Allow() // burn the single token
	// AllowN(0) should always succeed
	if !tb.AllowN(0) {
		t.Error("AllowN(0) should succeed even when empty")
	}
}

func TestTokenBucket_AllowN_Negative(t *testing.T) {
	tb := NewTokenBucket(1, 0)
	_ = tb.Allow() // burn the single token
	if !tb.AllowN(-1) {
		t.Error("AllowN(-1) should succeed even when empty")
	}
}

func TestTokenBucket_AllowN_ExceedsCapacity(t *testing.T) {
	tb := NewTokenBucket(3, 0)
	if tb.AllowN(5) {
		t.Error("AllowN(5) with capacity 3 should fail")
	}
	// Should still have all 3 tokens
	if !tb.AllowN(3) {
		t.Error("AllowN(3) should succeed — tokens not consumed by failed AllowN")
	}
}

func TestTokenBucket_AllowN_NonPositive(t *testing.T) {
	tb := NewTokenBucket(0, 0)
	if !tb.AllowN(0) {
		t.Error("AllowN(0) should succeed")
	}
	if !tb.AllowN(-5) {
		t.Error("AllowN(-5) should succeed")
	}
}

// ── Tokens ───────────────────────────────────────────────────

func TestTokenBucket_Tokens_Initial(t *testing.T) {
	tb := NewTokenBucket(10, 0)
	if tok := tb.Tokens(); tok < 9.9 || tok > 10.1 {
		t.Errorf("Tokens() = %f, want ~10.0", tok)
	}
}

func TestTokenBucket_Tokens_AfterAllow(t *testing.T) {
	tb := NewTokenBucket(5, 0)
	_ = tb.AllowN(2)
	if tok := tb.Tokens(); tok < 2.9 || tok > 3.1 {
		t.Errorf("Tokens() after AllowN(2) = %f, want ~3.0", tok)
	}
}

func TestTokenBucket_Tokens_RefillReflected(t *testing.T) {
	tb := NewTokenBucket(10, 100.0) // 100 tokens/second
	for i := 0; i < 10; i++ {
		_ = tb.Allow()
	}
	// Tokens ~0 now
	time.Sleep(50 * time.Millisecond) // 5 tokens refilled
	// Refill is time-based (elapsed * fillRate). Under system load the
	// sleep overruns, so MORE than 5.0 tokens accrue — assert lower bound
	// only. Capacity cap guarantees the upper bound.
	if tok := tb.Tokens(); tok < 4.5 || tok > float64(tb.capacity) {
		t.Errorf("Tokens() after 50ms refill = %f, want >= 4.5 and <= capacity %d", tok, tb.capacity)
	}
}

// ── SetRate ──────────────────────────────────────────────────

func TestTokenBucket_SetRate(t *testing.T) {
	tb := NewTokenBucket(5, 10.0)
	tb.SetRate(50.0)
	if tb.fillRate != 50.0 {
		t.Errorf("fillRate after SetRate(50) = %f, want 50.0", tb.fillRate)
	}
}

func TestTokenBucket_SetRate_Zero(t *testing.T) {
	tb := NewTokenBucket(5, 10.0)
	tb.SetRate(0)
	if tb.fillRate != 0 {
		t.Errorf("fillRate after SetRate(0) = %f, want 0", tb.fillRate)
	}
	// Exhaust tokens, then verify no refill
	for i := 0; i < 5; i++ {
		_ = tb.Allow()
	}
	time.Sleep(50 * time.Millisecond)
	if tb.Allow() {
		t.Error("Allow after SetRate(0) and exhaustion should return false")
	}
}

func TestTokenBucket_SetRate_Negative(t *testing.T) {
	tb := NewTokenBucket(5, 10.0)
	tb.SetRate(-5.0) // Degenerate but should not panic
	if tb.fillRate != -5.0 {
		t.Errorf("fillRate after SetRate(-5) = %f, want -5.0", tb.fillRate)
	}
}

// ── Concurrent Access ────────────────────────────────────────

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := NewTokenBucket(100, 50.0)
	var wg sync.WaitGroup
	allowed := make([]bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			allowed[idx] = tb.Allow()
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, ok := range allowed {
		if ok {
			successCount++
		}
	}
	// With capacity 100 and 100 goroutines, all should succeed
	if successCount != 100 {
		t.Errorf("concurrent allows: %d/100 succeeded (want 100)", successCount)
	}
}

func TestTokenBucket_ConcurrentAllowN(t *testing.T) {
	tb := NewTokenBucket(50, 0)
	var wg sync.WaitGroup
	allowed := make([]bool, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			allowed[idx] = tb.AllowN(1)
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, ok := range allowed {
		if ok {
			successCount++
		}
	}
	if successCount != 50 {
		t.Errorf("concurrent AllowN: %d/50 succeeded (want 50)", successCount)
	}
}

func TestTokenBucket_ConcurrentTokensAndAllow(t *testing.T) {
	tb := NewTokenBucket(100, 100.0)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				tb.Allow()
			}
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = tb.Tokens()
			}
		}()
	}
	wg.Wait()
	// Should not panic — just verify thread safety
}

func TestTokenBucket_ConcurrentSetRate(t *testing.T) {
	tb := NewTokenBucket(50, 10.0)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tb.SetRate(float64(10))
		}()
	}
	wg.Wait()
	// Should not panic under concurrent SetRate
}

// ── Token Bucket Exhaustion with Real Time ───────────────────

func TestTokenBucket_ExhaustionAndRefill(t *testing.T) {
	tb := NewTokenBucket(5, 100.0)
	// Exhaust
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("Allow #%d failed", i+1)
		}
	}
	if tb.Allow() {
		t.Error("Allow after exhaustion should fail")
	}
	// Wait for refill of 1 token (at 100 t/s, 15ms = 1.5 tokens → capped at capacity but only 1 fits)
	time.Sleep(15 * time.Millisecond)
	if !tb.Allow() {
		t.Error("Allow after 15ms refill should succeed")
	}
}
