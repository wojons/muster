package retry

import (
	"math/rand"
	"time"
)

// Jitter provides deterministic jitter for testing
type Jitter struct {
	rng *rand.Rand
}

// NewJitter creates a new jitter instance with the given seed
func NewJitter(seed int64) *Jitter {
	return &Jitter{rng: rand.New(rand.NewSource(seed))}
}

// NewJitterWithTime creates a new jitter instance seeded with current time
func NewJitterWithTime() *Jitter {
	return &Jitter{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// FullJitter returns random value in [0, duration)
func (j *Jitter) FullJitter(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
	return time.Duration(j.rng.Int63n(int64(duration)))
}

// EqualJitter returns random value in [duration/2, duration)
func (j *Jitter) EqualJitter(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
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

// NoJitter returns the duration unchanged (no jitter applied)
func (j *Jitter) NoJitter(duration time.Duration) time.Duration {
	return duration
}
