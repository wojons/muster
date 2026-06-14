package rate

import (
	"testing"
	"time"
)

func TestNewTokenBucket(t *testing.T) {
	tb := NewTokenBucket(10, 5)
	if tb == nil {
		t.Fatal("NewTokenBucket returned nil")
	}
	// Initial state: bucket starts full (burst capacity)
	if !tb.Allow() {
		t.Error("expected Allow() to return true on a full bucket")
	}
}

func TestTokenBucket_Allow_Exhaustion(t *testing.T) {
	// ratePerSec=1, burst=1 — bucket holds 1 token, refills at 1/sec
	tb := NewTokenBucket(1, 1)

	// First call consumes the single initial token
	if !tb.Allow() {
		t.Fatal("expected Allow() to return true on a full bucket")
	}

	// Second call should fail — bucket exhausted
	if tb.Allow() {
		t.Error("expected Allow() to return false on exhausted bucket")
	}
}

func TestTokenBucket_Allow_Refill(t *testing.T) {
	tb := NewTokenBucket(100, 1) // 100 tokens/sec, burst=1

	// Exhaust the bucket
	if !tb.Allow() {
		t.Fatal("expected Allow() to return true on a full bucket")
	}
	if tb.Allow() {
		t.Fatal("expected Allow() to return false on exhausted bucket")
	}

	// Wait ~20ms to let the bucket refill (100 tokens/sec → ~2 tokens in 20ms)
	time.Sleep(20 * time.Millisecond)

	if !tb.Allow() {
		t.Error("expected Allow() to return true after refill")
	}
}

func TestTokenBucket_Stop_NoPanic(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// Stop is documented as a no-op — must not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop() panicked: %v", r)
		}
	}()
	tb.Stop()
}

func TestTokenBucket_Allow_BurstCapacity(t *testing.T) {
	// burst=3 means we can consume 3 tokens immediately
	tb := NewTokenBucket(10, 3)

	for i := 0; i < 3; i++ {
		if !tb.Allow() {
			t.Errorf("expected Allow() call %d to return true within burst", i+1)
		}
	}

	// 4th call should fail (burst exhausted)
	if tb.Allow() {
		t.Error("expected Allow() to return false after burst exhausted")
	}
}package rate
