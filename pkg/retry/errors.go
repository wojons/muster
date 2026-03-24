package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrorClass represents the classification of an error for retry decisions
type ErrorClass string

const (
	ErrTransient    ErrorClass = "transient"
	ErrPermanent    ErrorClass = "permanent"
	ErrTimeout      ErrorClass = "timeout"
	ErrUnavailable  ErrorClass = "unavailable"
	ErrThrottled    ErrorClass = "throttled"
	ErrInvalid      ErrorClass = "invalid"
	ErrUnauthorized ErrorClass = "unauthorized"
)

// PermanentError wraps non-retryable errors
type PermanentError struct {
	Cause error
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent error: %v", e.Cause)
}

func (e *PermanentError) Unwrap() error { return e.Cause }

// RetryExhaustedError indicates all retries failed
type RetryExhaustedError struct {
	Attempts int
	LastErr  error
}

func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("retry exhausted after %d attempts: %v", e.Attempts, e.LastErr)
}

func (e *RetryExhaustedError) Unwrap() error { return e.LastErr }

// ErrPermanentMarker is a marker error for permanent failures
var ErrPermanentMarker = errors.New("permanent error")

// ClassificationRule defines a rule for classifying errors
type ClassificationRule struct {
	Match func(error) bool
	Class ErrorClass
	Retry bool
}

// ErrorClassifier determines retry behavior from errors
type ErrorClassifier struct {
	rules []ClassificationRule
}

// DefaultClassifier creates a new error classifier with default rules
func DefaultClassifier() *ErrorClassifier {
	return &ErrorClassifier{
		rules: []ClassificationRule{
			{Match: isTimeout, Class: ErrTimeout, Retry: true},
			{Match: isTemporary, Class: ErrTransient, Retry: true},
			{Match: isUnavailable, Class: ErrUnavailable, Retry: true},
			{Match: isThrottled, Class: ErrThrottled, Retry: true},
			{Match: isUnauthorized, Class: ErrUnauthorized, Retry: false},
			{Match: isInvalid, Class: ErrInvalid, Retry: false},
		},
	}
}

// Classify classifies an error according to the rules
func (ec *ErrorClassifier) Classify(err error) ErrorClass {
	for _, rule := range ec.rules {
		if rule.Match(err) {
			return rule.Class
		}
	}
	// Default to transient for unknown errors (safer to retry)
	return ErrTransient
}

func isTimeout(err error) bool {
	var te interface{ Timeout() bool }
	if errors.As(err, &te) {
		return te.Timeout()
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func isTemporary(err error) bool {
	var te interface{ Temporary() bool }
	if errors.As(err, &te) {
		return te.Temporary()
	}
	return false
}

func isUnavailable(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

// RateLimitError represents a rate limit error
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %v", e.RetryAfter)
}

func isThrottled(err error) bool {
	var re *RateLimitError
	return errors.As(err, &re)
}

func isUnauthorized(err error) bool {
	// Check for unauthorized errors
	if err == nil {
		return false
	}
	return false // Placeholder - specific implementation would check error codes
}

func isInvalid(err error) bool {
	// Check for invalid input errors
	if err == nil {
		return false
	}
	return false // Placeholder - specific implementation would check error codes
}
