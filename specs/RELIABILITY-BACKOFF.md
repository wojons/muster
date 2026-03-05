# Backoff Specification

## Overview

Backoff algorithms for retry strategies including linear, exponential, and jitter variants with configurable parameters.

## Core Components

### Backoff Interface

```go
// Backoff calculates delays between retry attempts
type Backoff interface {
    Next(attempt int) time.Duration
    Reset()
}

// BackoffConfig common backoff parameters
type BackoffConfig struct {
    BaseDelay  time.Duration `json:"base_delay"`
    MaxDelay   time.Duration `json:"max_delay"`
    Multiplier float64       `json:"multiplier"`
}
```

## Linear Backoff

```go
// LinearBackoff increases delay by fixed amount
type LinearBackoff struct {
    config BackoffConfig
    step   time.Duration
}

func NewLinearBackoff(config BackoffConfig) *LinearBackoff {
    return &LinearBackoff{
        config: config,
        step:   config.BaseDelay,
    }
}

func (b *LinearBackoff) Next(attempt int) time.Duration {
    delay := time.Duration(attempt) * b.config.BaseDelay
    return minDuration(delay, b.config.MaxDelay)
}

func (b *LinearBackoff) Reset() {
    // Stateless, nothing to reset
}

// LinearBackoffWithStep uses custom increment
type LinearBackoffWithStep struct {
    config BackoffConfig
    step   time.Duration
}

func (b *LinearBackoffWithStep) Next(attempt int) time.Duration {
    delay := b.config.BaseDelay + time.Duration(attempt-1)*b.step
    return minDuration(delay, b.config.MaxDelay)
}

func (b *LinearBackoffWithStep) Reset() {}
```

## Exponential Backoff

```go
// ExponentialBackoff doubles delay each attempt
type ExponentialBackoff struct {
    config      BackoffConfig
    current     time.Duration
    multiplier  float64
}

func NewExponentialBackoff(config BackoffConfig) *ExponentialBackoff {
    multiplier := config.Multiplier
    if multiplier == 0 {
        multiplier = 2.0
    }
    
    return &ExponentialBackoff{
        config:     config,
        current:    config.BaseDelay,
        multiplier: multiplier,
    }
}

func (b *ExponentialBackoff) Next(attempt int) time.Duration {
    delay := b.config.BaseDelay
    
    for i := 1; i < attempt; i++ {
        delay = time.Duration(float64(delay) * b.multiplier)
        if delay > b.config.MaxDelay {
            delay = b.config.MaxDelay
            break
        }
    }
    
    b.current = delay
    return delay
}

func (b *ExponentialBackoff) Reset() {
    b.current = b.config.BaseDelay
}

// ExponentialBackoffPrecise uses float64 for accuracy
type ExponentialBackoffPrecise struct {
    config BackoffConfig
    rng    *rand.Rand
}

func (b *ExponentialBackoffPrecise) Next(attempt int) time.Duration {
    delay := float64(b.config.BaseDelay) * math.Pow(b.config.Multiplier, float64(attempt-1))
    return minDuration(time.Duration(delay), b.config.MaxDelay)
}

func (b *ExponentialBackoffPrecise) Reset() {}
```

## Decorrelated Jitter

```go
// DecorrelatedJitter AWS-style decorrelated backoff
type DecorrelatedJitter struct {
    config  BackoffConfig
    prev    time.Duration
    rng     *rand.Rand
}

func NewDecorrelatedJitter(config BackoffConfig) *DecorrelatedJitter {
    return &DecorrelatedJitter{
        config: config,
        prev:   config.BaseDelay,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

// Next returns random value between base and 3*previous
func (b *DecorrelatedJitter) Next(attempt int) time.Duration {
    max := 3 * b.prev
    if max < b.config.BaseDelay {
        max = b.config.BaseDelay
    }
    
    // Random in [base, max]
    range_ := max - b.config.BaseDelay
    delay := b.config.BaseDelay + time.Duration(b.rng.Int63n(int64(range_)+1))
    
    b.prev = delay
    return minDuration(delay, b.config.MaxDelay)
}

func (b *DecorrelatedJitter) Reset() {
    b.prev = b.config.BaseDelay
}

// EqualJitter half fixed, half random
type EqualJitter struct {
    config BackoffConfig
    rng    *rand.Rand
}

func NewEqualJitter(config BackoffConfig) *EqualJitter {
    return &EqualJitter{
        config: config,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (b *EqualJitter) Next(attempt int) time.Duration {
    exp := b.config.BaseDelay
    for i := 1; i < attempt; i++ {
        exp *= 2
        if exp > b.config.MaxDelay {
            exp = b.config.MaxDelay
            break
        }
    }
    
    // Half fixed, half jitter
    half := exp / 2
    jitter := time.Duration(b.rng.Int63n(int64(half)))
    
    return half + jitter
}

func (b *EqualJitter) Reset() {}

// FullJitter completely random up to exponential
type FullJitter struct {
    config BackoffConfig
    rng    *rand.Rand
}

func NewFullJitter(config BackoffConfig) *FullJitter {
    return &FullJitter{
        config: config,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (b *FullJitter) Next(attempt int) time.Duration {
    exp := b.config.BaseDelay
    for i := 1; i < attempt; i++ {
        exp *= 2
        if exp > b.config.MaxDelay {
            exp = b.config.MaxDelay
            break
        }
    }
    
    return time.Duration(b.rng.Int63n(int64(exp)))
}

func (b *FullJitter) Reset() {}
```

### Utility Functions

```go
func minDuration(a, b time.Duration) time.Duration {
    if a < b {
        return a
    }
    return b
}

func maxDuration(a, b time.Duration) time.Duration {
    if a > b {
        return a
    }
    return b
}

// BackoffFactory creates backoff instances
type BackoffFactory struct {
    config BackoffConfig
    rng    *rand.Rand
}

func NewBackoffFactory(config BackoffConfig) *BackoffFactory {
    return &BackoffFactory{
        config: config,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (f *BackoffFactory) Create(algorithm string) Backoff {
    switch algorithm {
    case "linear":
        return NewLinearBackoff(f.config)
    case "exponential":
        return NewExponentialBackoff(f.config)
    case "decorrelated":
        return NewDecorrelatedJitter(f.config)
    case "equal":
        return NewEqualJitter(f.config)
    case "full":
        return NewFullJitter(f.config)
    default:
        return NewExponentialBackoff(f.config)
    }
}
```

## Comparison

| Algorithm | Jitter | Behavior | Use Case |
|-----------|--------|----------|----------|
| Linear | None | Fixed increment | Predictable rate limiting |
| Exponential | None | Doubles each time | Aggressive backoff |
| Full Jitter | Max | Random [0, exp] | High contention |
| Equal Jitter | Half | Fixed + random | Balanced approach |
| Decorrelated | Bounded | [base, 3*prev] | AWS recommended |

## Usage Examples

```go
// Simple exponential
backoff := NewExponentialBackoff(BackoffConfig{
    BaseDelay: 100 * time.Millisecond,
    MaxDelay:  30 * time.Second,
})

for attempt := 1; attempt <= maxAttempts; attempt++ {
    delay := backoff.Next(attempt)
    time.Sleep(delay)
}

// With jitter
jitterBackoff := NewDecorrelatedJitter(BackoffConfig{
    BaseDelay: 100 * time.Millisecond,
    MaxDelay:  60 * time.Second,
})

// Factory pattern
factory := NewBackoffFactory(BackoffConfig{
    BaseDelay:  100 * time.Millisecond,
    MaxDelay:   30 * time.Second,
    Multiplier: 2.0,
})

expBackoff := factory.Create("exponential")
decorrBackoff := factory.Create("decorrelated")
```
