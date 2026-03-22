package retry

import (
	"time"
)

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffStrategy BackoffType   `json:"backoff_strategy"`
	JitterStrategy  JitterType    `json:"jitter_strategy"`
	RetryableErrors []ErrorClass  `json:"retryable_errors"`
	NonRetryable    []ErrorClass  `json:"non_retryable_errors"`
	ContextTimeout  time.Duration `json:"context_timeout,omitempty"`
}

// BackoffType represents the backoff strategy type
type BackoffType string

const (
	BackoffLinear       BackoffType = "linear"
	BackoffExponential  BackoffType = "exponential"
	BackoffDecorrelated BackoffType = "decorrelated"
)

// JitterType represents the jitter strategy type
type JitterType string

const (
	JitterNone   JitterType = "none"
	JitterFull   JitterType = "full"
	JitterEqual  JitterType = "equal"
	JitterDecorr JitterType = "decorrelated"
)

// DefaultRetryConfig provides sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		BackoffStrategy: BackoffExponential,
		JitterStrategy:  JitterFull,
		RetryableErrors: []ErrorClass{
			ErrTransient, ErrTimeout, ErrUnavailable,
		},
	}
}
