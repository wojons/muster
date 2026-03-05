# Retry Specification

## Overview

Comprehensive retry mechanism with configurable strategies, exponential backoff, jitter, and intelligent error classification.

## Core Components

### Retry Configuration

```go
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

type BackoffType string

const (
    BackoffLinear     BackoffType = "linear"
    BackoffExponential BackoffType = "exponential"
    BackoffDecorrelated BackoffType = "decorrelated"
)

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
```

### Retry Executor

```go
// Retrier executes operations with retry logic
type Retrier struct {
    config RetryConfig
    rng    *rand.Rand
}

func NewRetrier(config RetryConfig) *Retrier {
    return &Retrier{
        config: config,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

// Execute runs fn with retry logic
func (r *Retrier) Execute(ctx context.Context, fn RetryableFunc) (Result, error) {
    var lastErr error
    
    for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
        if attempt > 1 {
            delay := r.calculateDelay(attempt)
            
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

func (r *Retrier) shouldRetry(err error, attempt int) bool {
    if attempt >= r.config.MaxAttempts {
        return false
    }
    
    // Check permanent marker
    if errors.Is(err, ErrPermanent) {
        return false
    }
    
    // Check error classification
    errClass := ClassifyError(err)
    
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
```

## Exponential Backoff

```go
// calculateDelay computes backoff delay for attempt
func (r *Retrier) calculateDelay(attempt int) time.Duration {
    var delay time.Duration
    
    switch r.config.BackoffStrategy {
    case BackoffLinear:
        delay = time.Duration(attempt) * r.config.InitialDelay
        
    case BackoffExponential:
        delay = r.config.InitialDelay * time.Duration(1<<(attempt-1))
        
    case BackoffDecorrelated:
        // Decorrelated jitter: delay = min(cap, random * 3)
        if attempt == 1 {
            delay = r.config.InitialDelay
        } else {
            delay = time.Duration(r.rng.Int63n(int64(delay)*3)) + r.config.InitialDelay
        }
    }
    
    // Apply jitter
    delay = r.applyJitter(delay)
    
    // Clamp to max
    if delay > r.config.MaxDelay {
        delay = r.config.MaxDelay
    }
    
    return delay
}

func (r *Retrier) applyJitter(delay time.Duration) time.Duration {
    switch r.config.JitterStrategy {
    case JitterNone:
        return delay
        
    case JitterFull:
        // Random between 0 and delay
        return time.Duration(r.rng.Int63n(int64(delay)))
        
    case JitterEqual:
        // Random between delay/2 and delay
        half := delay / 2
        return half + time.Duration(r.rng.Int63n(int64(half)))
        
    case JitterDecorr:
        // AWS-style decorrelated jitter
        return time.Duration(r.rng.Int63n(int64(delay)*3))
    }
    
    return delay
}
```

## Jitter Strategies

```go
// Jitter provides deterministic jitter for testing
type Jitter struct {
    rng *rand.Rand
}

func NewJitter(seed int64) *Jitter {
    return &Jitter{rng: rand.New(rand.NewSource(seed))}
}

// FullJitter returns random value in [0, duration)
func (j *Jitter) FullJitter(duration time.Duration) time.Duration {
    return time.Duration(j.rng.Int63n(int64(duration)))
}

// EqualJitter returns random value in [duration/2, duration)
func (j *Jitter) EqualJitter(duration time.Duration) time.Duration {
    half := duration / 2
    return half + time.Duration(j.rng.Int63n(int64(half)))
}

// DecorrelatedJitter returns random value bounded by 3*duration
func (j *Jitter) DecorrelatedJitter(prevDelay, baseDelay time.Duration) time.Duration {
    max := prevDelay * 3
    if max < baseDelay {
        max = baseDelay
    }
    return time.Duration(j.rng.Int63n(int64(max)))
}
```

## Retryable Error Detection

```go
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

// ErrorClassifier determines retry behavior from errors
type ErrorClassifier struct {
    rules []ClassificationRule
}

type ClassificationRule struct {
    Match   func(error) bool
    Class   ErrorClass
    Retry   bool
}

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

func (ec *ErrorClassifier) Classify(err error) ErrorClass {
    for _, rule := range ec.rules {
        if rule.Match(err) {
            return rule.Class
        }
    }
    return ErrPermanent
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
    var he *net.Error
    return errors.As(err, &he) && (*he).Timeout()
}

func isThrottled(err error) bool {
    var re *RateLimitError
    return errors.As(err, &re)
}
```

### Error Types

```go
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

var ErrPermanent = errors.New("permanent error")
```

## Usage Examples

```go
// Basic retry
retrier := NewRetrier(DefaultRetryConfig())
result, err := retrier.Execute(ctx, func(ctx context.Context) (Result, error) {
    return client.Call(ctx, request)
})

// With custom config
config := RetryConfig{
    MaxAttempts:     5,
    InitialDelay:    500 * time.Millisecond,
    MaxDelay:        60 * time.Second,
    BackoffStrategy: BackoffExponential,
    JitterStrategy:  JitterEqual,
}

retrier := NewRetrier(config)
```
