package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthRateLimiter_NewOAuthRateLimiter(t *testing.T) {
	rl := NewOAuthRateLimiter()

	require.NotNil(t, rl)
	assert.NotNil(t, rl.tokenLimiter)
	assert.NotNil(t, rl.deviceCodeLimiter)
	assert.NotNil(t, rl.failedAttempts)
	assert.Equal(t, 5, rl.maxFailedAttempts)
	assert.Equal(t, 15*time.Minute, rl.lockoutDuration)
}

func TestOAuthRateLimiter_AllowTokenRequest_Success(t *testing.T) {
	rl := NewOAuthRateLimiter()

	clientID := "test-client"

	// Should allow first request
	err := rl.AllowTokenRequest(clientID)
	assert.NoError(t, err)

	// Should allow multiple requests within limit
	for i := 0; i < 5; i++ {
		err := rl.AllowTokenRequest(clientID)
		assert.NoError(t, err)
	}
}

func TestOAuthRateLimiter_AllowTokenRequest_RateLimitExceeded(t *testing.T) {
	rl := NewOAuthRateLimiter()

	clientID := "test-client"

	// Exhaust the rate limit (capacity is 10)
	for i := 0; i < 15; i++ {
		_ = rl.AllowTokenRequest(clientID)
	}

	// Next request should be rate limited
	err := rl.AllowTokenRequest(clientID)
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

func TestOAuthRateLimiter_RecordFailedAttempt(t *testing.T) {
	rl := NewOAuthRateLimiter()

	identifier := "test-user"

	// Record failed attempts
	for i := 0; i < 5; i++ {
		rl.RecordFailedAttempt(identifier)
	}

	// Should have 5 failed attempts
	count := rl.GetFailedAttempts(identifier)
	assert.Equal(t, 5, count)
}

func TestOAuthRateLimiter_CheckAccountLockout_NotLocked(t *testing.T) {
	rl := NewOAuthRateLimiter()

	identifier := "test-user"

	// Less than max attempts
	for i := 0; i < 4; i++ {
		rl.RecordFailedAttempt(identifier)
	}

	err := rl.checkAccountLockout(identifier)
	assert.NoError(t, err)
}

func TestOAuthRateLimiter_CheckAccountLockout_Locked(t *testing.T) {
	rl := NewOAuthRateLimiter()

	identifier := "test-user"

	// Reach max attempts
	for i := 0; i < 5; i++ {
		rl.RecordFailedAttempt(identifier)
	}

	err := rl.checkAccountLockout(identifier)
	assert.ErrorIs(t, err, ErrAccountLocked)
}

func TestOAuthRateLimiter_RecordSuccessfulAttempt_ClearsFailedAttempts(t *testing.T) {
	rl := NewOAuthRateLimiter()

	identifier := "test-user"

	// Record some failed attempts
	rl.RecordFailedAttempt(identifier)
	rl.RecordFailedAttempt(identifier)

	// Record success
	rl.RecordSuccessfulAttempt(identifier)

	// Failed attempts should be cleared
	count := rl.GetFailedAttempts(identifier)
	assert.Equal(t, 0, count)
}

func TestOAuthRateLimiter_AllowDeviceCodePoll_Success(t *testing.T) {
	rl := NewOAuthRateLimiter()

	deviceCode := "test-device-code"

	// Should allow first poll
	err := rl.AllowDeviceCodePoll(deviceCode)
	assert.NoError(t, err)

	// Should allow second poll (capacity is 2)
	err = rl.AllowDeviceCodePoll(deviceCode)
	assert.NoError(t, err)
}

func TestOAuthRateLimiter_AllowDeviceCodePoll_RateLimitExceeded(t *testing.T) {
	rl := NewOAuthRateLimiter()

	deviceCode := "test-device-code"

	// Exhaust the rate limit
	_ = rl.AllowDeviceCodePoll(deviceCode)
	_ = rl.AllowDeviceCodePoll(deviceCode)
	_ = rl.AllowDeviceCodePoll(deviceCode)

	// Next poll should be rate limited
	err := rl.AllowDeviceCodePoll(deviceCode)
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

func TestOAuthRateLimiter_AllowCallbackRequest_Success(t *testing.T) {
	rl := NewOAuthRateLimiter()

	state := "test-state"

	// Should allow callback
	err := rl.AllowCallbackRequest(state)
	assert.NoError(t, err)
}

func TestOAuthRateLimiter_ResetFailedAttempts(t *testing.T) {
	rl := NewOAuthRateLimiter()

	identifier := "test-user"

	// Record failed attempts
	rl.RecordFailedAttempt(identifier)
	rl.RecordFailedAttempt(identifier)

	// Reset
	rl.ResetFailedAttempts(identifier)

	// Should have 0 failed attempts
	count := rl.GetFailedAttempts(identifier)
	assert.Equal(t, 0, count)
}

func TestOAuthRateLimiter_Configure(t *testing.T) {
	rl := NewOAuthRateLimiter()

	// Configure with custom values
	rl.Configure(10, 30*time.Minute)

	assert.Equal(t, 10, rl.maxFailedAttempts)
	assert.Equal(t, 30*time.Minute, rl.lockoutDuration)
}

func TestOAuthRateLimiter_AccountLockout_AutoClear(t *testing.T) {
	rl := NewOAuthRateLimiter()

	// Set short lockout for testing
	rl.lockoutDuration = 100 * time.Millisecond

	identifier := "test-user"

	// Lock the account
	for i := 0; i < 5; i++ {
		rl.RecordFailedAttempt(identifier)
	}

	// Verify locked
	err := rl.checkAccountLockout(identifier)
	assert.ErrorIs(t, err, ErrAccountLocked)

	// Wait for lockout to expire
	time.Sleep(150 * time.Millisecond)

	// Should be unlocked now
	err = rl.checkAccountLockout(identifier)
	assert.NoError(t, err)

	// Failed attempts should be cleared
	count := rl.GetFailedAttempts(identifier)
	assert.Equal(t, 0, count)
}

func TestOAuthRateLimiter_MultipleClients_Isolated(t *testing.T) {
	rl := NewOAuthRateLimiter()

	client1 := "client-1"
	client2 := "client-2"

	// Exhaust client1 rate limit
	for i := 0; i < 15; i++ {
		_ = rl.AllowTokenRequest(client1)
	}

	// Client1 should be rate limited
	err := rl.AllowTokenRequest(client1)
	assert.ErrorIs(t, err, ErrRateLimitExceeded)

	// Client2 should still work
	err = rl.AllowTokenRequest(client2)
	assert.NoError(t, err)
}

func TestOAuthRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewOAuthRateLimiter()

	done := make(chan bool, 100)

	// Spawn 100 goroutines accessing rate limiter concurrently
	for i := 0; i < 100; i++ {
		go func(id int) {
			clientID := "client-" + string(rune('A'+id%26))
			_ = rl.AllowTokenRequest(clientID)
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// No panics = test passed
}

// Test TokenManager rate limiting integration
func TestTokenManager_Refresh_RateLimiting(t *testing.T) {
	storage := &mockTokenStorageForRateLimit{
		tokens: make(map[string]*Token),
	}

	tm := NewTokenManager(storage, "https://example.com/token", "test-client", "test-secret")

	// Create a token with refresh token
	token := &Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
	}

	// Save token
	err := tm.Store("test-service", token)
	require.NoError(t, err)

	// Exhaust rate limit
	for i := 0; i < 20; i++ {
		_ = tm.rateLimiter.AllowTokenRequest("test-client")
	}

	// Try to refresh - should fail with rate limit
	_, err = tm.Refresh(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit")
}

// Test DeviceCodeFlow rate limiting integration
func TestDeviceCodeFlow_Poll_RateLimiting(t *testing.T) {
	flow := &DeviceCodeFlow{
		ClientID:    "test-client",
		DeviceURL:   "https://example.com/device",
		TokenURL:    "https://example.com/token",
		rateLimiter: NewOAuthRateLimiter(),
	}

	deviceCode := "test-device-code"

	// Exhaust rate limit
	for i := 0; i < 5; i++ {
		_ = flow.rateLimiter.AllowDeviceCodePoll(deviceCode)
	}

	// Next poll should be rate limited
	ctx := context.Background()
	_, err := flow.Poll(ctx, deviceCode, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit")
}

// Mock token storage for rate limit tests
type mockTokenStorageForRateLimit struct {
	tokens map[string]*Token
}

func (m *mockTokenStorageForRateLimit) Save(service string, token *Token) error {
	m.tokens[service] = token
	return nil
}

func (m *mockTokenStorageForRateLimit) Load(service string) (*Token, error) {
	token, ok := m.tokens[service]
	if !ok {
		return nil, nil
	}
	return token, nil
}

// Test CallbackServer rate limiting integration
func TestCallbackServer_HandleCallback_RateLimiting(t *testing.T) {
	stateStore := newStateStore()
	stateStore.Set("test-request", "test-state", 10*time.Minute)

	server := NewCallbackServer(8080, stateStore)

	// Exhaust rate limit
	for i := 0; i < 15; i++ {
		_ = server.rateLimiter.AllowCallbackRequest("test-request")
	}

	// Verify rate limit is exceeded
	err := server.rateLimiter.AllowCallbackRequest("test-request")
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}
