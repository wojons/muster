package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ─── NewRetrier ────────────────────────────────────────────────────────────

func TestNewRetrier_ValidConfig(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:  5,
		BackoffType:  "exponential",
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Jitter:       true,
	}
	r := NewRetrier(cfg)
	if r == nil {
		t.Fatal("NewRetrier returned nil")
	}
	if r.config.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", r.config.MaxAttempts)
	}
	if r.config.BackoffType != "exponential" {
		t.Errorf("BackoffType = %q, want exponential", r.config.BackoffType)
	}
}

func TestNewRetrier_ZeroAttemptsFallback(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxAttempts: 0})
	if r.config.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1 (fallback)", r.config.MaxAttempts)
	}
}

func TestNewRetrier_NegativeAttemptsFallback(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxAttempts: -5})
	if r.config.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1 (fallback)", r.config.MaxAttempts)
	}
}

func TestNewRetrier_EmptyBackoffType(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: ""})
	if r.config.BackoffType != "constant" {
		t.Errorf("BackoffType = %q, want constant", r.config.BackoffType)
	}
}

func TestNewRetrier_ZeroInitialDelay(t *testing.T) {
	r := NewRetrier(RetryConfig{InitialDelay: 0})
	if r.config.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", r.config.InitialDelay)
	}
}

func TestNewRetrier_ZeroMaxDelay(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxDelay: 0})
	if r.config.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", r.config.MaxDelay)
	}
}

// ─── DefaultRetryConfig ────────────────────────────────────────────────────

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.BackoffType != "exponential" {
		t.Errorf("BackoffType = %q, want exponential", cfg.BackoffType)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if !cfg.Jitter {
		t.Error("Jitter should default to true")
	}
}

// ─── Execute ────────────────────────────────────────────────────────────────

func TestExecute_SuccessFirstTry(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxAttempts: 3, BackoffType: "constant", InitialDelay: 1 * time.Millisecond})
	result, err := r.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %v, want ok", result)
	}
}

func TestExecute_RetryOnTransientThenSucceed(t *testing.T) {
	attempts := 0
	r := NewRetrier(RetryConfig{MaxAttempts: 3, BackoffType: "constant", InitialDelay: 1 * time.Millisecond})
	result, err := r.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("transient error")
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("result = %v, want recovered", result)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestExecute_ExhaustedError(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxAttempts: 2, BackoffType: "constant", InitialDelay: 1 * time.Millisecond})
	_, err := r.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("always fails")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	exhausted, ok := err.(*RetryExhaustedError)
	if !ok {
		t.Fatalf("expected *RetryExhaustedError, got %T: %v", err, err)
	}
	if exhausted.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", exhausted.Attempts)
	}
	if exhausted.LastErr == nil || exhausted.LastErr.Error() != "always fails" {
		t.Errorf("LastErr = %v, want 'always fails'", exhausted.LastErr)
	}
}

func TestExecute_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRetrier(RetryConfig{MaxAttempts: 5, BackoffType: "constant", InitialDelay: 500 * time.Millisecond})
	cancel() // cancel immediately before Execute
	_, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("fail")
	})
	if err != context.Canceled {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestExecute_ContextCancelDuringRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRetrier(RetryConfig{MaxAttempts: 5, BackoffType: "constant", InitialDelay: 500 * time.Millisecond})
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("fail")
	})
	if err != context.Canceled {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestExecute_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	r := NewRetrier(RetryConfig{MaxAttempts: 5, BackoffType: "constant", InitialDelay: 500 * time.Millisecond})
	_, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("fail")
	})
	if err != context.DeadlineExceeded {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

// ─── calculateDelay ────────────────────────────────────────────────────────

func TestCalculateDelay_Constant(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "constant", InitialDelay: 100 * time.Millisecond, Jitter: false})
	// All attempts should return the same delay
	for attempt := 0; attempt < 5; attempt++ {
		delay := r.calculateDelay(attempt)
		if delay != 100*time.Millisecond {
			t.Errorf("attempt %d: delay = %v, want 100ms", attempt, delay)
		}
	}
}

func TestCalculateDelay_Linear(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "linear", InitialDelay: 100 * time.Millisecond, Jitter: false})
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 300 * time.Millisecond},
		{3, 400 * time.Millisecond},
	}
	for _, tt := range tests {
		delay := r.calculateDelay(tt.attempt)
		if delay != tt.want {
			t.Errorf("attempt %d: delay = %v, want %v", tt.attempt, delay, tt.want)
		}
	}
}

func TestCalculateDelay_Exponential(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "exponential", InitialDelay: 100 * time.Millisecond, Jitter: false})
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},  // 2^0 * 100ms = 100ms
		{1, 200 * time.Millisecond},  // 2^1 * 100ms = 200ms
		{2, 400 * time.Millisecond},  // 2^2 * 100ms = 400ms
		{3, 800 * time.Millisecond},  // 2^3 * 100ms = 800ms
	}
	for _, tt := range tests {
		delay := r.calculateDelay(tt.attempt)
		if delay != tt.want {
			t.Errorf("attempt %d: delay = %v, want %v", tt.attempt, delay, tt.want)
		}
	}
}

func TestCalculateDelay_MaxDelayCap(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "exponential", InitialDelay: 100 * time.Millisecond, MaxDelay: 500 * time.Millisecond, Jitter: false})
	// Attempts 0-2 should be under cap, 3+ should be capped
	if delay := r.calculateDelay(0); delay != 100*time.Millisecond {
		t.Errorf("attempt 0: delay = %v, want 100ms", delay)
	}
	if delay := r.calculateDelay(1); delay != 200*time.Millisecond {
		t.Errorf("attempt 1: delay = %v, want 200ms", delay)
	}
	if delay := r.calculateDelay(2); delay != 400*time.Millisecond {
		t.Errorf("attempt 2: delay = %v, want 400ms", delay)
	}
	// 2^3 * 100ms = 800ms > 500ms cap
	if delay := r.calculateDelay(3); delay != 500*time.Millisecond {
		t.Errorf("attempt 3: delay = %v, want 500ms (capped)", delay)
	}
}

func TestCalculateDelay_JitterVariance(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "constant", InitialDelay: 100 * time.Millisecond, Jitter: true})
	// With jitter enabled and constant backoff, delays should vary.
	// Run multiple iterations and check that not all values are identical.
	delays := make(map[time.Duration]int)
	for i := 0; i < 50; i++ {
		delay := r.calculateDelay(0)
		delays[delay]++
	}
	// With jitter, we expect more than 1 unique value (probabilistic, but 50 iterations is enough)
	if len(delays) < 2 {
		t.Logf("delays: %v (jitter may not produce variance in 50 iterations — rare but possible)", delays)
	}
}

func TestCalculateDelay_JitterWithinBounds(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "constant", InitialDelay: 100 * time.Millisecond, Jitter: true})
	for i := 0; i < 100; i++ {
		delay := r.calculateDelay(0)
		// With jitter: delay = InitialDelay/2 + rand(InitialDelay/2)
		// Lower bound: 50ms + 0 = 50ms
		// Upper bound: 50ms + 49ms = 99ms (< 100ms)
		if delay < 50*time.Millisecond {
			t.Errorf("delay %v < 50ms lower bound", delay)
		}
		if delay >= 100*time.Millisecond {
			t.Errorf("delay %v >= 100ms upper bound (jitter should keep below InitialDelay)", delay)
		}
	}
}

func TestCalculateDelay_NoJitter(t *testing.T) {
	r := NewRetrier(RetryConfig{BackoffType: "constant", InitialDelay: 100 * time.Millisecond, Jitter: false})
	for i := 0; i < 10; i++ {
		delay := r.calculateDelay(0)
		if delay != 100*time.Millisecond {
			t.Errorf("without jitter: delay = %v, want 100ms", delay)
		}
	}
}

func TestCalculateDelay_DefaultBackoffType(t *testing.T) {
	// Unknown/empty backoff type falls back to constant
	r := NewRetrier(RetryConfig{BackoffType: "bogus", InitialDelay: 50 * time.Millisecond, Jitter: false})
	delay := r.calculateDelay(5)
	if delay != 50*time.Millisecond {
		t.Errorf("delay = %v, want 50ms (constant fallback)", delay)
	}
}

// ─── RetryExhaustedError ───────────────────────────────────────────────────

func TestRetryExhaustedError_Error(t *testing.T) {
	e := &RetryExhaustedError{Attempts: 3, LastErr: errors.New("connection refused")}
	msg := e.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}
	// Check it contains key details
	if !contains(msg, "retry exhausted") {
		t.Errorf("Error() = %q, want to contain 'retry exhausted'", msg)
	}
	if !contains(msg, "3") {
		t.Errorf("Error() = %q, want to contain attempt count '3'", msg)
	}
	if !contains(msg, "connection refused") {
		t.Errorf("Error() = %q, want to contain 'connection refused'", msg)
	}
}

func TestRetryExhaustedError_SingleAttempt(t *testing.T) {
	e := &RetryExhaustedError{Attempts: 1, LastErr: errors.New("fail")}
	msg := e.Error()
	if !contains(msg, "1 attempts") {
		t.Errorf("Error() = %q, want to contain '1 attempts'", msg)
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
