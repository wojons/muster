package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context) (interface{}, error)

// Retrier executes operations with retry logic
type Retrier struct {
	config     RetryConfig
	classifier *ErrorClassifier
	jitter     *Jitter
	backoff    *BackoffCalculator
}

// NewRetrier creates a new retrier with the given configuration
func NewRetrier(config RetryConfig) *Retrier {
	jitter := NewJitterWithTime()
	return &Retrier{
		config:     config,
		classifier: DefaultClassifier(),
		jitter:     jitter,
		backoff:    NewBackoffCalculator(config, jitter),
	}
}

// NewRetrierWithSeed creates a new retrier with a deterministic seed (for testing)
func NewRetrierWithSeed(config RetryConfig, seed int64) *Retrier {
	jitter := NewJitter(seed)
	return &Retrier{
		config:     config,
		classifier: DefaultClassifier(),
		jitter:     jitter,
		backoff:    NewBackoffCalculator(config, jitter),
	}
}

// Execute runs fn with retry logic
func (r *Retrier) Execute(ctx context.Context, fn RetryableFunc) (interface{}, error) {
	var lastErr error

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		var delay time.Duration

		if attempt > 1 {
			delay = r.backoff.CalculateDelay(attempt)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue to next attempt
			}
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if !r.shouldRetry(err, attempt) {
			// If we've exhausted retries, don't wrap - let it fall through to RetryExhaustedError
			if attempt >= r.config.MaxAttempts {
				break
			}
			return nil, &PermanentError{Cause: err}
		}

		if attempt < r.config.MaxAttempts {
			r.logRetry(attempt, err, delay)
		}
	}

	return nil, &RetryExhaustedError{
		Attempts: r.config.MaxAttempts,
		LastErr:  lastErr,
	}
}

// Do executes a function with retry logic, returning only an error.
// This is a convenience method for operations that don't return a value.
func (r *Retrier) Do(ctx context.Context, fn func() error) error {
	_, err := r.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, fn()
	})
	return err
}

// shouldRetry determines if an error should be retried
func (r *Retrier) shouldRetry(err error, attempt int) bool {
	if attempt >= r.config.MaxAttempts {
		return false
	}

	// Check permanent marker
	if errors.Is(err, ErrPermanentMarker) {
		return false
	}

	// Check if error is a PermanentError type
	var permErr *PermanentError
	if errors.As(err, &permErr) {
		return false
	}

	// Check error classification
	errClass := r.classifier.Classify(err)

	// Check non-retryable first
	for _, nr := range r.config.NonRetryable {
		if errClass == nr {
			return false
		}
	}

	// Check retryable
	for _, rc := range r.config.RetryableErrors {
		if errClass == rc {
			return true
		}
	}

	// Default: retry on transient errors
	return errClass == ErrTransient
}

// logRetry logs retry attempts (can be customized)
func (r *Retrier) logRetry(attempt int, err error, delay time.Duration) {
	// In production, this would use the logging system
	// For now, we keep it simple
	_ = fmt.Sprintf("Retry attempt %d after %v due to: %v", attempt, delay, err)
}

// SetClassifier allows setting a custom error classifier
func (r *Retrier) SetClassifier(classifier *ErrorClassifier) {
	r.classifier = classifier
}
