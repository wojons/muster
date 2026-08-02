package auth

import (
	"errors"
	"sync"
	"time"

	"github.com/wojons/muster/pkg/auth/ratelimit"
)

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded, please try again later")
	// ErrAccountLocked is returned when account is temporarily locked due to failed attempts
	ErrAccountLocked = errors.New("account temporarily locked due to too many failed attempts, please try again in 15 minutes")
)

// OAuthRateLimiter provides rate limiting for OAuth2 endpoints
type OAuthRateLimiter struct {
	// Token endpoint rate limiter (token bucket: 10 requests/minute per client)
	tokenLimiter *ratelimit.TokenBucketLimiter

	// Device code polling rate limiter (token bucket: 2 requests/minute per device code)
	deviceCodeLimiter *ratelimit.TokenBucketLimiter

	// Failed attempt tracker for account lockout (sliding window)
	failedAttempts map[string]*failedAttemptTracker
	failedMu       sync.RWMutex

	// Configuration
	maxFailedAttempts int           // Number of failed attempts before lockout
	lockoutDuration   time.Duration // Duration of account lockout
	cleanupInterval   time.Duration // Interval for cleaning up old trackers
}

type failedAttemptTracker struct {
	count    int
	firstTry time.Time
	lastTry  time.Time
	mu       sync.RWMutex
}

// NewOAuthRateLimiter creates a new OAuth rate limiter with default configuration
func NewOAuthRateLimiter() *OAuthRateLimiter {
	rl := &OAuthRateLimiter{
		tokenLimiter:      ratelimit.NewTokenBucketLimiter(10, 10),
		deviceCodeLimiter: ratelimit.NewTokenBucketLimiter(2, 2),
		failedAttempts:    make(map[string]*failedAttemptTracker),
		maxFailedAttempts: 5, // Lock after 5 failed attempts
		lockoutDuration:   15 * time.Minute,
		cleanupInterval:   5 * time.Minute,
	}

	// Start cleanup goroutine
	go rl.cleanupExpired()

	return rl
}

// AllowTokenRequest checks if a token request should be allowed
func (rl *OAuthRateLimiter) AllowTokenRequest(clientID string) error {
	// Check account lockout first
	if err := rl.checkAccountLockout(clientID); err != nil {
		return err
	}

	// Check rate limit
	if !rl.tokenLimiter.Allow() {
		return ErrRateLimitExceeded
	}

	return nil
}

// RecordFailedAttempt records a failed authentication attempt
func (rl *OAuthRateLimiter) RecordFailedAttempt(identifier string) {
	rl.failedMu.Lock()
	defer rl.failedMu.Unlock()

	tracker, exists := rl.failedAttempts[identifier]
	if !exists {
		tracker = &failedAttemptTracker{}
		rl.failedAttempts[identifier] = tracker
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	now := time.Now()
	tracker.count++
	tracker.lastTry = now
	if tracker.firstTry.IsZero() {
		tracker.firstTry = now
	}
}

// RecordSuccessfulAttempt clears failed attempt tracking on success
func (rl *OAuthRateLimiter) RecordSuccessfulAttempt(identifier string) {
	rl.failedMu.Lock()
	defer rl.failedMu.Unlock()

	delete(rl.failedAttempts, identifier)
}

// checkAccountLockout checks if an account is locked due to too many failed attempts
func (rl *OAuthRateLimiter) checkAccountLockout(identifier string) error {
	rl.failedMu.RLock()
	tracker, exists := rl.failedAttempts[identifier]
	rl.failedMu.RUnlock()

	if !exists {
		return nil
	}

	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	// Check if lockout period has expired. A zero/negative lockout
	// duration means "no lockout" (expires instantly) — checked
	// explicitly because coarse OS clocks (Windows ~15.6ms tick) can
	// make time.Since() return 0 for back-to-back attempts, which
	// would keep `0 > 0` false and leave the account locked forever.
	if rl.lockoutDuration <= 0 || time.Since(tracker.lastTry) > rl.lockoutDuration {
		// Auto-clear after lockout period
		rl.failedMu.Lock()
		delete(rl.failedAttempts, identifier)
		rl.failedMu.Unlock()
		return nil
	}

	// Check if max attempts exceeded
	if tracker.count >= rl.maxFailedAttempts {
		return ErrAccountLocked
	}

	return nil
}

// AllowDeviceCodePoll checks if a device code poll request should be allowed
func (rl *OAuthRateLimiter) AllowDeviceCodePoll(deviceCode string) error {
	if !rl.deviceCodeLimiter.Allow() {
		return ErrRateLimitExceeded
	}
	return nil
}

// AllowCallbackRequest checks if a callback request should be allowed
func (rl *OAuthRateLimiter) AllowCallbackRequest(state string) error {
	if !rl.tokenLimiter.Allow() {
		return ErrRateLimitExceeded
	}
	return nil
}

// cleanupExpired removes old failed attempt trackers
func (rl *OAuthRateLimiter) cleanupExpired() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.failedMu.Lock()
		now := time.Now()
		for id, tracker := range rl.failedAttempts {
			tracker.mu.RLock()
			// Remove if last attempt was more than lockout duration ago
			if now.Sub(tracker.lastTry) > rl.lockoutDuration*2 {
				delete(rl.failedAttempts, id)
			}
			tracker.mu.RUnlock()
		}
		rl.failedMu.Unlock()
	}
}

// GetFailedAttempts returns the number of failed attempts for an identifier
func (rl *OAuthRateLimiter) GetFailedAttempts(identifier string) int {
	rl.failedMu.RLock()
	defer rl.failedMu.RUnlock()

	tracker, exists := rl.failedAttempts[identifier]
	if !exists {
		return 0
	}

	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	return tracker.count
}

// ResetFailedAttempts resets failed attempt count for an identifier
func (rl *OAuthRateLimiter) ResetFailedAttempts(identifier string) {
	rl.failedMu.Lock()
	defer rl.failedMu.Unlock()

	delete(rl.failedAttempts, identifier)
}

// Configure allows customization of rate limiting parameters
func (rl *OAuthRateLimiter) Configure(maxFailed int, lockoutDur time.Duration) {
	rl.maxFailedAttempts = maxFailed
	rl.lockoutDuration = lockoutDur
}
