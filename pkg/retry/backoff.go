package retry

import (
	"time"
)

// BackoffCalculator calculates the delay for a given attempt
type BackoffCalculator struct {
	config RetryConfig
	jitter *Jitter
}

// NewBackoffCalculator creates a new backoff calculator
func NewBackoffCalculator(config RetryConfig, jitter *Jitter) *BackoffCalculator {
	return &BackoffCalculator{
		config: config,
		jitter: jitter,
	}
}

// CalculateDelay computes backoff delay for attempt
func (bc *BackoffCalculator) CalculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch bc.config.BackoffStrategy {
	case BackoffLinear:
		delay = bc.linearBackoff(attempt)

	case BackoffExponential:
		delay = bc.exponentialBackoff(attempt)

	case BackoffDecorrelated:
		delay = bc.decorrelatedBackoff(attempt)

	default:
		delay = bc.exponentialBackoff(attempt)
	}

	// Apply jitter
	delay = bc.applyJitter(delay)

	// Clamp to max
	if delay > bc.config.MaxDelay {
		delay = bc.config.MaxDelay
	}

	return delay
}

// linearBackoff calculates linear backoff: delay = attempt * initialDelay
func (bc *BackoffCalculator) linearBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * bc.config.InitialDelay
}

// exponentialBackoff calculates exponential backoff: delay = initialDelay * 2^(attempt-1)
func (bc *BackoffCalculator) exponentialBackoff(attempt int) time.Duration {
	return bc.config.InitialDelay * time.Duration(1<<(uint(attempt-1)))
}

// decorrelatedBackoff calculates decorrelated jitter backoff
func (bc *BackoffCalculator) decorrelatedBackoff(attempt int) time.Duration {
	if attempt == 1 {
		return bc.config.InitialDelay
	}
	// Decorrelated jitter: delay = min(cap, random * 3)
	return bc.jitter.DecorrelatedJitter(
		bc.config.InitialDelay*time.Duration(attempt-1),
		bc.config.InitialDelay,
	)
}

// applyJitter applies the configured jitter strategy
func (bc *BackoffCalculator) applyJitter(delay time.Duration) time.Duration {
	switch bc.config.JitterStrategy {
	case JitterNone:
		return bc.jitter.NoJitter(delay)

	case JitterFull:
		return bc.jitter.FullJitter(delay)

	case JitterEqual:
		return bc.jitter.EqualJitter(delay)

	case JitterDecorr:
		// AWS-style decorrelated jitter
		return time.Duration(bc.jitter.rng.Int63n(int64(delay) * 3))

	default:
		return bc.jitter.FullJitter(delay)
	}
}
