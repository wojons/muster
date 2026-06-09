// Package retry provides configurable retry logic with multiple backoff strategies.
package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	MaxAttempts int

	// BackoffType specifies the backoff strategy: "constant", "linear", or "exponential".
	BackoffType string

	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration

	// Jitter enables random jitter on retry delays.
	Jitter bool
}

// Retrier executes operations with retry logic.
type Retrier struct {
	config RetryConfig
}

// RetryExhaustedError is returned when all retry attempts have been exhausted.
type RetryExhaustedError struct {
	Attempts int
	LastErr  error
}

// Error implements the error interface.
func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("retry exhausted after %d attempts: %v", e.Attempts, e.LastErr)
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		BackoffType:  "exponential",
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Jitter:       true,
	}
}

// NewRetrier creates a new Retrier with the given configuration.
func NewRetrier(config RetryConfig) *Retrier {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 1
	}
	if config.BackoffType == "" {
		config.BackoffType = "constant"
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	return &Retrier{config: config}
}

// Execute runs the given function with retry logic.
// The function should return (result, error). If the error is nil, the result is returned.
// If all attempts are exhausted, a RetryExhaustedError is returned.
func (r *Retrier) Execute(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt < r.config.MaxAttempts-1 {
			delay := r.calculateDelay(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, &RetryExhaustedError{
		Attempts: r.config.MaxAttempts,
		LastErr:  lastErr,
	}
}

// calculateDelay computes the delay for a given retry attempt number (0-indexed).
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch r.config.BackoffType {
	case "constant":
		delay = r.config.InitialDelay
	case "linear":
		delay = r.config.InitialDelay * time.Duration(attempt+1)
	case "exponential":
		delay = r.config.InitialDelay * time.Duration(int64(math.Pow(2, float64(attempt))))
	default:
		delay = r.config.InitialDelay
	}

	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}

	if r.config.Jitter {
		jitter := time.Duration(rand.Int63n(int64(delay) / 2))
		delay = delay/2 + jitter
	}

	return delay
}
