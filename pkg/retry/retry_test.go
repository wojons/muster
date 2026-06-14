package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultRetryConfig
// ---------------------------------------------------------------------------

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts: expected 3, got %d", cfg.MaxAttempts)
	}
	if cfg.BackoffType != "exponential" {
		t.Fatalf("BackoffType: expected 'exponential', got %q", cfg.BackoffType)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Fatalf("InitialDelay: expected 100ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Fatalf("MaxDelay: expected 30s, got %v", cfg.MaxDelay)
	}
	if !cfg.Jitter {
		t.Fatal("Jitter: expected true")
	}
}

// ---------------------------------------------------------------------------
// NewRetrier normalization
// ---------------------------------------------------------------------------

func TestNewRetrier_NormalizesZeroValues(t *testing.T) {
	r := NewRetrier(RetryConfig{})

	if r.config.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts: expected 1 (normalized from 0), got %d", r.config.MaxAttempts)
	}
	if r.config.BackoffType != "constant" {
		t.Fatalf("BackoffType: expected 'constant' (normalized from empty), got %q", r.config.BackoffType)
	}
	if r.config.InitialDelay != 100*time.Millisecond {
		t.Fatalf("InitialDelay: expected 100ms (normalized from 0), got %v", r.config.InitialDelay)
	}
	if r.config.MaxDelay != 30*time.Second {
		t.Fatalf("MaxDelay: expected 30s (normalized from 0), got %v", r.config.MaxDelay)
	}
}

func TestNewRetrier_NormalizesNegativeMaxAttempts(t *testing.T) {
	r := NewRetrier(RetryConfig{MaxAttempts: -5, BackoffType: "constant", InitialDelay: time.Millisecond, MaxDelay: time.Second})

	if r.config.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts: expected 1 (normalized from -5), got %d", r.config.MaxAttempts)
	}
}

func TestNewRetrier_PreservesValidValues(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  7,
		BackoffType:  "exponential",
		InitialDelay: time.Second,
		MaxDelay:     10 * time.Second,
		Jitter:       true,
	})

	if r.config.MaxAttempts != 7 {
		t.Fatalf("MaxAttempts: expected 7, got %d", r.config.MaxAttempts)
	}
	if r.config.BackoffType != "exponential" {
		t.Fatalf("BackoffType: expected 'exponential', got %q", r.config.BackoffType)
	}
	if r.config.InitialDelay != time.Second {
		t.Fatalf("InitialDelay: expected 1s, got %v", r.config.InitialDelay)
	}
	if r.config.MaxDelay != 10*time.Second {
		t.Fatalf("MaxDelay: expected 10s, got %v", r.config.MaxDelay)
	}
	if !r.config.Jitter {
		t.Fatal("Jitter: expected true")
	}
}

// ---------------------------------------------------------------------------
// Retrier.Execute — success paths
// ---------------------------------------------------------------------------

func TestRetrier_Execute_FirstAttemptSuccess(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		BackoffType:  "constant",
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     1 * time.Millisecond,
		Jitter:       false,
	})
	ctx := context.Background()

	var callCount int
	result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		callCount++
		return "hello", nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %v", result)
	}
	if callCount != 1 {
		t.Fatalf("expected fn called exactly once, got %d calls", callCount)
	}
}

func TestRetrier_Execute_SuccessOnNthAttempt(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  5,
		BackoffType:  "constant",
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     1 * time.Millisecond,
		Jitter:       false,
	})
	ctx := context.Background()

	var callCount int
	result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, fmt.Errorf("attempt %d failed", callCount)
		}
		return "success", nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got %v", result)
	}
	if callCount != 3 {
		t.Fatalf("expected fn called 3 times, got %d calls", callCount)
	}
}

// ---------------------------------------------------------------------------
// Retrier.Execute — exhaustion
// ---------------------------------------------------------------------------

func TestRetrier_Execute_AllAttemptsExhausted(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		BackoffType:  "constant",
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     1 * time.Millisecond,
		Jitter:       false,
	})
	ctx := context.Background()

	sentinelErr := fmt.Errorf("always fails")
	var callCount int
	result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, sentinelErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}

	var retryErr *RetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected *RetryExhaustedError, got %T: %v", err, err)
	}
	if retryErr.Attempts != 3 {
		t.Fatalf("expected Attempts=3, got %d", retryErr.Attempts)
	}
	if retryErr.LastErr != sentinelErr {
		t.Fatalf("expected LastErr=%v, got %v", sentinelErr, retryErr.LastErr)
	}
	if callCount != 3 {
		t.Fatalf("expected fn called 3 times, got %d calls", callCount)
	}
}

// ---------------------------------------------------------------------------
// Retrier.Execute — context cancellation
// ---------------------------------------------------------------------------

func TestRetrier_Execute_ContextCancelBeforeFirstAttempt(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		BackoffType:  "constant",
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     1 * time.Millisecond,
		Jitter:       false,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any call

	var callCount int
	result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		callCount++
		return "result", nil
	})

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if callCount != 0 {
		t.Fatalf("expected fn never called, got %d calls", callCount)
	}
}

func TestRetrier_Execute_ContextCancelBetweenAttempts(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  5,
		BackoffType:  "constant",
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Jitter:       false,
	})
	ctx, cancel := context.WithCancel(context.Background())

	var callCount int
	errCh := make(chan error, 1)

	go func() {
		_, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
			callCount++
			return nil, fmt.Errorf("fail")
		})
		errCh <- err
	}()

	// Give enough time for the first attempt to run and enter the sleep.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
		// The function should have been called at least once (first attempt failed),
		// but not all 5 times (context was cancelled during sleep).
		if callCount >= 5 {
			t.Fatalf("fn called %d times, expected <5 due to cancellation", callCount)
		}
		if callCount < 1 {
			t.Fatalf("fn called %d times, expected >=1", callCount)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Execute to return")
	}
}

func TestRetrier_Execute_ContextDeadlineExceeded(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		BackoffType:  "constant",
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Jitter:       false,
	})

	// Context that expires in 5ms — well before the 50ms retry delay finishes.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("fail")
	})

	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// RetryExhaustedError.Error()
// ---------------------------------------------------------------------------

func TestRetryExhaustedError_Error(t *testing.T) {
	err := &RetryExhaustedError{
		Attempts: 5,
		LastErr:  fmt.Errorf("connection refused"),
	}
	msg := err.Error()
	expected := "retry exhausted after 5 attempts: connection refused"
	if msg != expected {
		t.Fatalf("expected %q, got %q", expected, msg)
	}
}

func TestRetryExhaustedError_Error_NilLastErr(t *testing.T) {
	err := &RetryExhaustedError{
		Attempts: 3,
		LastErr:  nil,
	}
	msg := err.Error()
	expected := "retry exhausted after 3 attempts: <nil>"
	if msg != expected {
		t.Fatalf("expected %q, got %q", expected, msg)
	}
}

// ---------------------------------------------------------------------------
// calculateDelay — deterministic tests (jitter disabled)
// ---------------------------------------------------------------------------

func TestCalculateDelay_Constant(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "constant",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       false,
	})
	for attempt := 0; attempt < 5; attempt++ {
		d := r.calculateDelay(attempt)
		expected := 100 * time.Millisecond
		if d != expected {
			t.Fatalf("attempt %d: expected %v, got %v", attempt, expected, d)
		}
	}
}

func TestCalculateDelay_Linear(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "linear",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       false,
	})
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},  // 100ms * 1
		{1, 200 * time.Millisecond},  // 100ms * 2
		{2, 300 * time.Millisecond},  // 100ms * 3
		{3, 400 * time.Millisecond},  // 100ms * 4
		{10, 1100 * time.Millisecond}, // 100ms * 11
	}
	for _, tc := range cases {
		d := r.calculateDelay(tc.attempt)
		if d != tc.expected {
			t.Fatalf("attempt %d: expected %v, got %v", tc.attempt, tc.expected, d)
		}
	}
}

func TestCalculateDelay_Exponential(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "exponential",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       false,
	})
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},  // 100ms * 2^0 = 100ms
		{1, 200 * time.Millisecond},  // 100ms * 2^1 = 200ms
		{2, 400 * time.Millisecond},  // 100ms * 2^2 = 400ms
		{3, 800 * time.Millisecond},  // 100ms * 2^3 = 800ms
		{4, 1600 * time.Millisecond}, // 100ms * 2^4 = 1600ms
	}
	for _, tc := range cases {
		d := r.calculateDelay(tc.attempt)
		if d != tc.expected {
			t.Fatalf("attempt %d: expected %v, got %v", tc.attempt, tc.expected, d)
		}
	}
}

func TestCalculateDelay_DefaultBackoffType(t *testing.T) {
	// An unrecognized BackoffType falls through to the default case,
	// which uses InitialDelay — same as constant.
	r := NewRetrier(RetryConfig{
		BackoffType:  "unknown",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       false,
	})
	for attempt := 0; attempt < 3; attempt++ {
		d := r.calculateDelay(attempt)
		expected := 100 * time.Millisecond
		if d != expected {
			t.Fatalf("attempt %d: expected %v, got %v", attempt, expected, d)
		}
	}
}

// ---------------------------------------------------------------------------
// calculateDelay — max delay cap
// ---------------------------------------------------------------------------

func TestCalculateDelay_MaxDelayCap(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "exponential",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Jitter:       false,
	})
	// attempt 3: 100ms * 2^3 = 800ms, capped at 500ms
	d := r.calculateDelay(3)
	expected := 500 * time.Millisecond
	if d != expected {
		t.Fatalf("expected %v (capped from 800ms), got %v", expected, d)
	}
}

func TestCalculateDelay_MaxDelayCap_Linear(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "linear",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     250 * time.Millisecond,
		Jitter:       false,
	})
	// attempt 4: 100ms * 5 = 500ms, capped at 250ms
	d := r.calculateDelay(4)
	expected := 250 * time.Millisecond
	if d != expected {
		t.Fatalf("expected %v (capped from 500ms), got %v", expected, d)
	}
}

func TestCalculateDelay_NoCapWhenBelowMax(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "exponential",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Jitter:       false,
	})
	// attempt 5: 100ms * 32 = 3200ms, well below 10s cap
	d := r.calculateDelay(5)
	expected := 3200 * time.Millisecond
	if d != expected {
		t.Fatalf("expected %v, got %v", expected, d)
	}
}

// ---------------------------------------------------------------------------
// calculateDelay — jitter
// ---------------------------------------------------------------------------

func TestCalculateDelay_JitterRange(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "constant",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       true,
	})
	// With jitter: delay = InitialDelay/2 + rand(0, InitialDelay/2)
	// So the result is in [InitialDelay/2, InitialDelay).
	const target = 100 * time.Millisecond
	const half = 50 * time.Millisecond

	for i := 0; i < 200; i++ {
		d := r.calculateDelay(0)
		if d < half {
			t.Fatalf("iteration %d: delay %v is below minimum %v", i, d, half)
		}
		if d >= target {
			t.Fatalf("iteration %d: delay %v is at or above maximum %v", i, d, target)
		}
	}
}

func TestCalculateDelay_JitterVaries(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "constant",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       true,
	})
	// Collect multiple values and verify they are not all identical.
	first := r.calculateDelay(0)
	allSame := true
	for i := 1; i < 30; i++ {
		if r.calculateDelay(0) != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("jitter should produce varying delays over 30 calls, but all values were the same")
	}
}

func TestCalculateDelay_JitterWithMaxCap(t *testing.T) {
	r := NewRetrier(RetryConfig{
		BackoffType:  "exponential",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     150 * time.Millisecond,
		Jitter:       true,
	})
	// attempt 1: 100ms * 2 = 200ms, capped at 150ms.
	// After jitter: min = 75ms, max < 150ms.
	const min = 75 * time.Millisecond
	const max = 150 * time.Millisecond

	for i := 0; i < 100; i++ {
		d := r.calculateDelay(1)
		if d < min {
			t.Fatalf("iteration %d: delay %v below minimum %v", i, d, min)
		}
		if d >= max {
			t.Fatalf("iteration %d: delay %v at or above maximum %v", i, d, max)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkRetrier_Execute_Success(b *testing.B) {
	r := NewRetrier(DefaultRetryConfig())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Every invocation succeeds on the first attempt, so no delays are incurred.
		result, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
			return "result", nil
		})
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		if result != "result" {
			b.Fatalf("unexpected result: %v", result)
		}
	}
}
