package auth

import (
	"sync"
	"testing"
	"time"

	"github.com/wojons/muster/pkg/auth/ratelimit"
)

// ── NewOAuthRateLimiter ─────────────────────────────────────

func TestNewOAuthRateLimiter_Defaults(t *testing.T) {
	rl := NewOAuthRateLimiter()
	if rl == nil {
		t.Fatal("NewOAuthRateLimiter returned nil")
	}
	if rl.tokenLimiter == nil {
		t.Error("tokenLimiter is nil")
	}
	if rl.deviceCodeLimiter == nil {
		t.Error("deviceCodeLimiter is nil")
	}
	if rl.failedAttempts == nil {
		t.Error("failedAttempts map is nil")
	}
	if rl.maxFailedAttempts != 5 {
		t.Errorf("maxFailedAttempts = %d, want 5", rl.maxFailedAttempts)
	}
	if rl.lockoutDuration != 15*time.Minute {
		t.Errorf("lockoutDuration = %v, want 15m", rl.lockoutDuration)
	}
}

func TestNewOAuthRateLimiter_TokenLimiterRate(t *testing.T) {
	rl := NewOAuthRateLimiter()
	if r := rl.tokenLimiter.Rate(); r != 10.0 {
		t.Errorf("tokenLimiter rate = %f, want 10.0", r)
	}
	if r := rl.deviceCodeLimiter.Rate(); r != 2.0 {
		t.Errorf("deviceCodeLimiter rate = %f, want 2.0", r)
	}
}

// ── AllowTokenRequest ───────────────────────────────────────

func TestAllowTokenRequest_UnderLimit(t *testing.T) {
	rl := NewOAuthRateLimiter()
	for i := 0; i < 10; i++ {
		if err := rl.AllowTokenRequest("client-" + string(rune('a'+i))); err != nil {
			t.Fatalf("AllowTokenRequest #%d should succeed under limit, got: %v", i+1, err)
		}
	}
}

func TestAllowTokenRequest_OverLimit(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Burn the burst (10 tokens, all different clients — no lockout)
	for i := 0; i < 10; i++ {
		_ = rl.AllowTokenRequest("client-" + string(rune('a'+i)))
	}
	// 11th should fail with rate limit
	err := rl.AllowTokenRequest("client-k")
	if err == nil {
		t.Error("AllowTokenRequest after exhausting burst should return error")
	}
	if err != ErrRateLimitExceeded {
		t.Errorf("AllowTokenRequest error = %v, want ErrRateLimitExceeded", err)
	}
}

func TestAllowTokenRequest_AccountLocked(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(3, 15*time.Minute) // Lock after 3 fails

	// Record 3 failed attempts
	for i := 0; i < 3; i++ {
		rl.RecordFailedAttempt("locked-user")
	}

	// Should be locked
	err := rl.AllowTokenRequest("locked-user")
	if err != ErrAccountLocked {
		t.Errorf("AllowTokenRequest on locked account: err = %v, want ErrAccountLocked", err)
	}
}

func TestAllowTokenRequest_NotLockedUnderThreshold(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(5, 15*time.Minute)

	// Record 4 fails — one below threshold
	for i := 0; i < 4; i++ {
		rl.RecordFailedAttempt("user")
	}

	// Should NOT be locked (still has rate limit tokens)
	err := rl.AllowTokenRequest("user")
	if err == ErrAccountLocked {
		t.Error("AllowTokenRequest should not return ErrAccountLocked under threshold")
	}
}

func TestAllowTokenRequest_LockoutExpired(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(2, 50*time.Millisecond) // Short lockout

	// Record 2 fails → lock
	for i := 0; i < 2; i++ {
		rl.RecordFailedAttempt("temp-user")
	}

	err := rl.AllowTokenRequest("temp-user")
	if err != ErrAccountLocked {
		t.Fatalf("expected locked, got: %v", err)
	}

	// Wait for lockout to expire
	time.Sleep(100 * time.Millisecond)

	// Should be unlocked now
	err = rl.AllowTokenRequest("temp-user")
	if err == ErrAccountLocked {
		t.Error("AllowTokenRequest should succeed after lockout expires")
	}
}

// ── RecordFailedAttempt ─────────────────────────────────────

func TestRecordFailedAttempt_IncrementsCount(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user1")
	rl.RecordFailedAttempt("user1")
	rl.RecordFailedAttempt("user1")

	if count := rl.GetFailedAttempts("user1"); count != 3 {
		t.Errorf("GetFailedAttempts = %d, want 3", count)
	}
}

func TestRecordFailedAttempt_NewIdentifier(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("new-user")
	if count := rl.GetFailedAttempts("new-user"); count != 1 {
		t.Errorf("GetFailedAttempts after first fail = %d, want 1", count)
	}
}

func TestRecordFailedAttempt_MultipleIdentifiers(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("alice")
	rl.RecordFailedAttempt("bob")
	rl.RecordFailedAttempt("alice")

	if count := rl.GetFailedAttempts("alice"); count != 2 {
		t.Errorf("alice count = %d, want 2", count)
	}
	if count := rl.GetFailedAttempts("bob"); count != 1 {
		t.Errorf("bob count = %d, want 1", count)
	}
}

// ── RecordSuccessfulAttempt ─────────────────────────────────

func TestRecordSuccessfulAttempt_ClearsTracking(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user")
	rl.RecordFailedAttempt("user")

	if count := rl.GetFailedAttempts("user"); count != 2 {
		t.Fatalf("expected 2 failed attempts, got %d", count)
	}

	rl.RecordSuccessfulAttempt("user")
	if count := rl.GetFailedAttempts("user"); count != 0 {
		t.Errorf("GetFailedAttempts after success = %d, want 0", count)
	}
}

func TestRecordSuccessfulAttempt_Nonexistent(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Should not panic
	rl.RecordSuccessfulAttempt("no-such-user")
	// Verify no side effects
	if count := rl.GetFailedAttempts("no-such-user"); count != 0 {
		t.Errorf("GetFailedAttempts = %d, want 0", count)
	}
}

// ── AllowDeviceCodePoll ─────────────────────────────────────

func TestAllowDeviceCodePoll_UnderLimit(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Device code limiter: 2 burst
	if err := rl.AllowDeviceCodePoll("dc-1"); err != nil {
		t.Errorf("AllowDeviceCodePoll #1: %v", err)
	}
	if err := rl.AllowDeviceCodePoll("dc-2"); err != nil {
		t.Errorf("AllowDeviceCodePoll #2: %v", err)
	}
}

func TestAllowDeviceCodePoll_OverLimit(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Burn the 2-token burst
	_ = rl.AllowDeviceCodePoll("dc-1")
	_ = rl.AllowDeviceCodePoll("dc-2")
	// Third should fail
	err := rl.AllowDeviceCodePoll("dc-3")
	if err != ErrRateLimitExceeded {
		t.Errorf("AllowDeviceCodePoll #3 error = %v, want ErrRateLimitExceeded", err)
	}
}

// ── AllowCallbackRequest ────────────────────────────────────

func TestAllowCallbackRequest_UnderLimit(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Uses tokenLimiter (10 burst) — same pool, need different limiter state
	// Fresh limiter: 10 burst available
	for i := 0; i < 5; i++ {
		if err := rl.AllowCallbackRequest("state-" + string(rune('a'+i))); err != nil {
			t.Fatalf("AllowCallbackRequest #%d: %v", i+1, err)
		}
	}
}

func TestAllowCallbackRequest_OverLimit(t *testing.T) {
	rl := &OAuthRateLimiter{
		tokenLimiter:      ratelimit.NewTokenBucketLimiter(0, 3), // 3 burst, no refill
		deviceCodeLimiter: ratelimit.NewTokenBucketLimiter(10, 10),
		failedAttempts:    make(map[string]*failedAttemptTracker),
		maxFailedAttempts: 5,
		lockoutDuration:   15 * time.Minute,
		cleanupInterval:   5 * time.Minute,
	}
	for i := 0; i < 3; i++ {
		_ = rl.AllowCallbackRequest("s")
	}
	err := rl.AllowCallbackRequest("s")
	if err != ErrRateLimitExceeded {
		t.Errorf("AllowCallbackRequest exhausted: err = %v, want ErrRateLimitExceeded", err)
	}
}

// ── GetFailedAttempts ───────────────────────────────────────

func TestGetFailedAttempts_NotTracked(t *testing.T) {
	rl := NewOAuthRateLimiter()
	if count := rl.GetFailedAttempts("unknown"); count != 0 {
		t.Errorf("GetFailedAttempts = %d, want 0", count)
	}
}

func TestGetFailedAttempts_AfterMultipleFails(t *testing.T) {
	rl := NewOAuthRateLimiter()
	for i := 0; i < 4; i++ {
		rl.RecordFailedAttempt("u")
	}
	if count := rl.GetFailedAttempts("u"); count != 4 {
		t.Errorf("GetFailedAttempts = %d, want 4", count)
	}
}

// ── ResetFailedAttempts ─────────────────────────────────────

func TestResetFailedAttempts_Clears(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user")
	rl.RecordFailedAttempt("user")
	rl.ResetFailedAttempts("user")
	if count := rl.GetFailedAttempts("user"); count != 0 {
		t.Errorf("GetFailedAttempts after reset = %d, want 0", count)
	}
}

func TestResetFailedAttempts_Nonexistent(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Should not panic
	rl.ResetFailedAttempts("ghost")
}

// ── Configure ───────────────────────────────────────────────

func TestConfigure_UpdatesSettings(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(3, 30*time.Second)

	if rl.maxFailedAttempts != 3 {
		t.Errorf("maxFailedAttempts = %d, want 3", rl.maxFailedAttempts)
	}
	if rl.lockoutDuration != 30*time.Second {
		t.Errorf("lockoutDuration = %v, want 30s", rl.lockoutDuration)
	}
}

func TestConfigure_ZeroMaxFailed(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(0, 15*time.Minute)

	// Any failed attempt triggers lockout
	rl.RecordFailedAttempt("user")
	err := rl.AllowTokenRequest("user")
	if err != ErrAccountLocked {
		t.Errorf("expected ErrAccountLocked after 0 max-failed, got: %v", err)
	}
}

func TestConfigure_ZeroLockoutDuration(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(2, 0) // Zero lockout — expires instantly

	for i := 0; i < 2; i++ {
		rl.RecordFailedAttempt("user")
	}

	err := rl.AllowTokenRequest("user")
	if err == ErrAccountLocked {
		t.Error("expected unlocked with zero lockout duration")
	}
}

// ── Concurrent Access ───────────────────────────────────────

func TestOAuthRateLimiter_ConcurrentFailedAttempts(t *testing.T) {
	rl := NewOAuthRateLimiter()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.RecordFailedAttempt("concurrent-user")
		}()
	}
	wg.Wait()

	count := rl.GetFailedAttempts("concurrent-user")
	if count != 50 {
		t.Errorf("concurrent RecordFailedAttempt: count = %d, want 50", count)
	}
}

func TestOAuthRateLimiter_ConcurrentRecordAndReset(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Pre-populate some data
	for i := 0; i < 10; i++ {
		rl.RecordFailedAttempt("shared")
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.RecordFailedAttempt("shared")
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rl.GetFailedAttempts("shared")
		}()
	}
	wg.Wait()
	// Should not panic under concurrent access
}

func TestOAuthRateLimiter_ConcurrentAllowAndRecord(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(10, 15*time.Minute)

	var wg sync.WaitGroup
	// 10 goroutines recording failed attempts
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rl.RecordFailedAttempt("mixed-user")
		}(i)
	}
	// 5 goroutines checking AllowTokenRequest
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rl.AllowTokenRequest("mixed-user")
		}()
	}
	// 5 goroutines on different user (no lockout contention)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rl.AllowTokenRequest("clean-user")
		}()
	}
	wg.Wait()
	// Should not panic
}

// ── Edge Cases ──────────────────────────────────────────────

func TestAllowTokenRequest_RefillAfterWait(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Burn the burst (10 tokens) across different clients
	for i := 0; i < 10; i++ {
		_ = rl.AllowTokenRequest("c" + string(rune('a'+i)))
	}
	// Should be rate-limited now
	err := rl.AllowTokenRequest("fresh-client")
	if err != ErrRateLimitExceeded {
		t.Fatalf("expected rate limited, got: %v", err)
	}

	// Wait for refill — at 10 t/s, 150ms refills ~1.5 tokens → capped at 1
	time.Sleep(150 * time.Millisecond)

	err = rl.AllowTokenRequest("refilled-client")
	if err != nil {
		t.Errorf("AllowTokenRequest after refill: %v", err)
	}
}

func TestRecordFailedAttempt_ExactThreshold(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(3, 15*time.Minute)

	// 2 fails — not locked
	for i := 0; i < 2; i++ {
		rl.RecordFailedAttempt("user")
	}
	if rl.GetFailedAttempts("user") != 2 {
		t.Fatalf("expected 2 failures, got %d", rl.GetFailedAttempts("user"))
	}
	err := rl.AllowTokenRequest("user")
	if err != ErrAccountLocked {
		// Should not be locked at count=2 with threshold=3
	}

	// 3rd fail — now locked
	rl.RecordFailedAttempt("user")
	if count := rl.GetFailedAttempts("user"); count != 3 {
		t.Fatalf("expected 3 failures, got %d", count)
	}
	err = rl.AllowTokenRequest("user")
	if err != ErrAccountLocked {
		t.Errorf("expected locked at threshold, got: %v", err)
	}
}
