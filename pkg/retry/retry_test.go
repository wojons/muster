package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrier_Execute_SuccessOnFirstAttempt(t *testing.T) {
	config := DefaultRetryConfig()
	retrier := NewRetrierWithSeed(config, 12345)

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		return "success", nil
	}

	result, err := retrier.Execute(context.Background(), fn)

	require.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 1, callCount)
}

func TestRetrier_Execute_SuccessAfterRetries(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
		RetryableErrors: []ErrorClass{ErrTransient},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("transient error")
		}
		return "success", nil
	}

	result, err := retrier.Execute(context.Background(), fn)

	require.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 3, callCount)
}

func TestRetrier_Execute_ExhaustedRetries(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
		RetryableErrors: []ErrorClass{ErrTransient},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	fn := func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("transient error")
	}

	result, err := retrier.Execute(context.Background(), fn)

	require.Error(t, err)
	assert.Nil(t, result)

	// The error should be RetryExhaustedError
	var retryErr *RetryExhaustedError
	require.True(t, errors.As(err, &retryErr), "Expected RetryExhaustedError but got: %T - %v", err, err)
	assert.Equal(t, 3, retryErr.Attempts)
}

func TestRetrier_Execute_PermanentError(t *testing.T) {
	config := DefaultRetryConfig()
	retrier := NewRetrierWithSeed(config, 12345)

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, &PermanentError{Cause: errors.New("permanent failure")}
	}

	result, err := retrier.Execute(context.Background(), fn)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, callCount)

	var permErr *PermanentError
	require.True(t, errors.As(err, &permErr))
}

func TestRetrier_Do_Success(t *testing.T) {
	config := DefaultRetryConfig()
	retrier := NewRetrierWithSeed(config, 12345)

	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	err := retrier.Do(context.Background(), fn)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestRetrier_Do_RetriesUntilSuccess(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
		RetryableErrors: []ErrorClass{ErrTransient},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("transient error")
		}
		return nil
	}

	err := retrier.Do(context.Background(), fn)

	require.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestRetrier_Do_ExhaustedRetries(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
		RetryableErrors: []ErrorClass{ErrTransient},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	fn := func() error {
		return errors.New("transient error")
	}

	err := retrier.Do(context.Background(), fn)

	require.Error(t, err)

	var retryErr *RetryExhaustedError
	require.True(t, errors.As(err, &retryErr))
	assert.Equal(t, 3, retryErr.Attempts)
}

func TestRetrier_Execute_ContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        500 * time.Millisecond,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
		RetryableErrors: []ErrorClass{ErrTransient},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		if callCount == 1 {
			// Cancel context immediately so the wait is interrupted
			cancel()
		}
		return nil, errors.New("transient error")
	}

	result, err := retrier.Execute(ctx, fn)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func TestBackoffCalculator_LinearBackoff(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffStrategy: BackoffLinear,
		JitterStrategy:  JitterNone,
	}
	jitter := NewJitter(12345)
	calc := NewBackoffCalculator(config, jitter)

	assert.Equal(t, 100*time.Millisecond, calc.CalculateDelay(1))
	assert.Equal(t, 200*time.Millisecond, calc.CalculateDelay(2))
	assert.Equal(t, 300*time.Millisecond, calc.CalculateDelay(3))
}

func TestBackoffCalculator_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        10 * time.Second,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
	}
	jitter := NewJitter(12345)
	calc := NewBackoffCalculator(config, jitter)

	assert.Equal(t, 100*time.Millisecond, calc.CalculateDelay(1))
	assert.Equal(t, 200*time.Millisecond, calc.CalculateDelay(2))
	assert.Equal(t, 400*time.Millisecond, calc.CalculateDelay(3))
	assert.Equal(t, 800*time.Millisecond, calc.CalculateDelay(4))
}

func TestBackoffCalculator_MaxDelayCap(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    1 * time.Second,
		MaxDelay:        2 * time.Second,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterNone,
	}
	jitter := NewJitter(12345)
	calc := NewBackoffCalculator(config, jitter)

	// After 3 attempts, exponential would be 4s, but max is 2s
	assert.Equal(t, 2*time.Second, calc.CalculateDelay(3))
	assert.Equal(t, 2*time.Second, calc.CalculateDelay(10))
}

func TestJitter_FullJitter(t *testing.T) {
	jitter := NewJitter(12345)

	duration := 100 * time.Millisecond
	result := jitter.FullJitter(duration)

	// Should be between 0 and duration
	assert.True(t, result >= 0)
	assert.True(t, result < duration)
}

func TestJitter_EqualJitter(t *testing.T) {
	jitter := NewJitter(12345)

	duration := 100 * time.Millisecond
	result := jitter.EqualJitter(duration)

	// Should be between duration/2 and duration
	assert.True(t, result >= duration/2)
	assert.True(t, result < duration)
}

func TestJitter_DecorrelatedJitter(t *testing.T) {
	jitter := NewJitter(12345)

	prevDelay := 100 * time.Millisecond
	baseDelay := 50 * time.Millisecond
	result := jitter.DecorrelatedJitter(prevDelay, baseDelay)

	// Should be between 0 and max(prevDelay*3, baseDelay)
	maxDelay := prevDelay * 3
	assert.True(t, result >= 0)
	assert.True(t, result < maxDelay)
}

func TestErrorClassifier_Classify(t *testing.T) {
	classifier := DefaultClassifier()

	tests := []struct {
		name     string
		err      error
		expected ErrorClass
	}{
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			expected: ErrTimeout,
		},
		{
			name:     "temporary error",
			err:      &temporaryError{},
			expected: ErrTransient,
		},
		{
			name:     "unknown error defaults to transient",
			err:      errors.New("some error"),
			expected: ErrTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetrier_shouldRetry(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		RetryableErrors: []ErrorClass{ErrTransient, ErrTimeout},
		NonRetryable:    []ErrorClass{ErrInvalid},
	}
	retrier := NewRetrierWithSeed(config, 12345)

	tests := []struct {
		name     string
		attempt  int
		err      error
		expected bool
	}{
		{
			name:     "max attempts reached",
			attempt:  3,
			err:      errors.New("error"),
			expected: false,
		},
		{
			name:     "permanent marker error",
			attempt:  1,
			err:      ErrPermanentMarker,
			expected: false,
		},
		{
			name:     "permanent error type",
			attempt:  1,
			err:      &PermanentError{Cause: errors.New("permanent")},
			expected: false,
		},
		{
			name:     "retryable error class",
			attempt:  1,
			err:      context.DeadlineExceeded,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retrier.shouldRetry(tt.err, tt.attempt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper types for testing

type temporaryError struct{}

func (e *temporaryError) Error() string   { return "temporary error" }
func (e *temporaryError) Temporary() bool { return true }

func TestRetryExhaustedError_Error(t *testing.T) {
	err := &RetryExhaustedError{
		Attempts: 3,
		LastErr:  errors.New("last error"),
	}

	assert.Contains(t, err.Error(), "retry exhausted after 3 attempts")
	assert.Contains(t, err.Error(), "last error")
}

func TestRetryExhaustedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	err := &RetryExhaustedError{
		Attempts: 3,
		LastErr:  originalErr,
	}

	assert.Equal(t, originalErr, err.Unwrap())
}

func TestPermanentError_Error(t *testing.T) {
	cause := errors.New("cause error")
	err := &PermanentError{Cause: cause}

	assert.Contains(t, err.Error(), "permanent error")
	assert.Contains(t, err.Error(), "cause error")
}

func TestPermanentError_Unwrap(t *testing.T) {
	cause := errors.New("cause error")
	err := &PermanentError{Cause: cause}

	assert.Equal(t, cause, err.Unwrap())
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, BackoffExponential, config.BackoffStrategy)
	assert.Equal(t, JitterFull, config.JitterStrategy)
	assert.Contains(t, config.RetryableErrors, ErrTransient)
	assert.Contains(t, config.RetryableErrors, ErrTimeout)
	assert.Contains(t, config.RetryableErrors, ErrUnavailable)
}

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{
		RetryAfter: 5 * time.Second,
	}

	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Contains(t, err.Error(), "5s")
}
