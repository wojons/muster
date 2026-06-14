package auth

import (
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// OAuthRateLimiter tests
// ---------------------------------------------------------------------------

func TestNewOAuthRateLimiter(t *testing.T) {
	rl := NewOAuthRateLimiter()
	if rl == nil {
		t.Fatal("NewOAuthRateLimiter() returned nil")
	}
}

func TestOAuthRateLimiter_AllowTokenRequest_BurstCapacity(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Token bucket has capacity 10; first 10 should always succeed
	for i := 0; i < 10; i++ {
		if err := rl.AllowTokenRequest("client-1"); err != nil {
			t.Fatalf("request %d should be allowed, got: %v", i+1, err)
		}
	}
	// 11th request must be rate limited
	if err := rl.AllowTokenRequest("client-1"); !errors.Is(err, ErrRateLimitExceeded) {
		t.Fatalf("expected ErrRateLimitExceeded, got: %v", err)
	}
}

func TestOAuthRateLimiter_AllowTokenRequest_DifferentClientsShareBucket(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// The token bucket is global, not per-clientID. Exhaust it from two clients.
	for i := 0; i < 10; i++ {
		var err error
		if i%2 == 0 {
			err = rl.AllowTokenRequest("client-a")
		} else {
			err = rl.AllowTokenRequest("client-b")
		}
		if err != nil {
			t.Fatalf("request %d should be allowed, got: %v", i+1, err)
		}
	}
	// A third client should also be rate limited because the bucket is shared.
	if err := rl.AllowTokenRequest("client-c"); !errors.Is(err, ErrRateLimitExceeded) {
		t.Fatalf("expected ErrRateLimitExceeded for client-c, got: %v", err)
	}
}

func TestOAuthRateLimiter_AccountLockout(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(3, 15*time.Minute) // lock after 3 failures

	for i := 0; i < 3; i++ {
		rl.RecordFailedAttempt("user-1")
	}

	if got := rl.GetFailedAttempts("user-1"); got != 3 {
		t.Fatalf("expected 3 failed attempts, got: %d", got)
	}

	// AllowTokenRequest checks lockout before the rate limiter.
	if err := rl.AllowTokenRequest("user-1"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked, got: %v", err)
	}
}

func TestOAuthRateLimiter_SuccessfulAttemptClearsLockout(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(3, 15*time.Minute)

	for i := 0; i < 3; i++ {
		rl.RecordFailedAttempt("user-1")
	}

	// Confirm locked before success.
	if err := rl.AllowTokenRequest("user-1"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked before success, got: %v", err)
	}

	rl.RecordSuccessfulAttempt("user-1")

	// Should no longer be locked (but may still be rate limited if bucket empty).
	if got := rl.GetFailedAttempts("user-1"); got != 0 {
		t.Fatalf("expected 0 failed attempts after success, got: %d", got)
	}
	err := rl.AllowTokenRequest("user-1")
	if errors.Is(err, ErrAccountLocked) {
		t.Fatal("account should not be locked after RecordSuccessfulAttempt")
	}
}

func TestOAuthRateLimiter_AllowDeviceCodePoll(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Device code bucket has capacity 2.
	if err := rl.AllowDeviceCodePoll("code-1"); err != nil {
		t.Fatalf("first poll should be allowed, got: %v", err)
	}
	if err := rl.AllowDeviceCodePoll("code-1"); err != nil {
		t.Fatalf("second poll should be allowed, got: %v", err)
	}
	// Third poll exceeds capacity.
	if err := rl.AllowDeviceCodePoll("code-1"); !errors.Is(err, ErrRateLimitExceeded) {
		t.Fatalf("expected ErrRateLimitExceeded on third poll, got: %v", err)
	}
}

func TestOAuthRateLimiter_AllowCallbackRequest(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Callback uses the same token bucket as token requests (capacity 10).
	for i := 0; i < 10; i++ {
		if err := rl.AllowCallbackRequest("state-1"); err != nil {
			t.Fatalf("callback %d should be allowed, got: %v", i+1, err)
		}
	}
	if err := rl.AllowCallbackRequest("state-1"); !errors.Is(err, ErrRateLimitExceeded) {
		t.Fatalf("expected ErrRateLimitExceeded on 11th callback, got: %v", err)
	}
}

func TestOAuthRateLimiter_GetFailedAttempts_Unknown(t *testing.T) {
	rl := NewOAuthRateLimiter()
	if got := rl.GetFailedAttempts("nonexistent"); got != 0 {
		t.Fatalf("expected 0 for unknown user, got: %d", got)
	}
}

func TestOAuthRateLimiter_GetFailedAttempts_TracksCount(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user-1")
	rl.RecordFailedAttempt("user-1")
	rl.RecordFailedAttempt("user-1")

	if got := rl.GetFailedAttempts("user-1"); got != 3 {
		t.Fatalf("expected 3 failed attempts, got: %d", got)
	}
}

func TestOAuthRateLimiter_ResetFailedAttempts(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user-1")
	rl.RecordFailedAttempt("user-1")

	rl.ResetFailedAttempts("user-1")

	if got := rl.GetFailedAttempts("user-1"); got != 0 {
		t.Fatalf("expected 0 after reset, got: %d", got)
	}
}

func TestOAuthRateLimiter_RecordSuccessfulAttempt_OnlyClearsMatching(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.RecordFailedAttempt("user-1")
	rl.RecordFailedAttempt("user-1")
	rl.RecordFailedAttempt("user-2")

	rl.RecordSuccessfulAttempt("user-1") // clears only user-1

	if got := rl.GetFailedAttempts("user-1"); got != 0 {
		t.Fatalf("expected 0 for user-1, got: %d", got)
	}
	if got := rl.GetFailedAttempts("user-2"); got != 1 {
		t.Fatalf("expected 1 for user-2, got: %d", got)
	}
}

func TestOAuthRateLimiter_RecordSuccessfulAttempt_NoEffectOnUnknown(t *testing.T) {
	rl := NewOAuthRateLimiter()
	// Should not panic or hang for unknown identifier.
	rl.RecordSuccessfulAttempt("never-failed")
}

func TestOAuthRateLimiter_Configure(t *testing.T) {
	rl := NewOAuthRateLimiter()
	rl.Configure(10, 30*time.Minute)

	// With maxFailed=10, need 10 attempts before lockout.
	for i := 0; i < 10; i++ {
		rl.RecordFailedAttempt("user-1")
	}

	if got := rl.GetFailedAttempts("user-1"); got != 10 {
		t.Fatalf("expected 10 failed attempts, got: %d", got)
	}

	if err := rl.AllowTokenRequest("user-1"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Token revocation tests (using mockRevocationStore from oauth2_test.go)
// ---------------------------------------------------------------------------

func TestMockRevocationStore_TokenNotRevoked(t *testing.T) {
	store := newMockRevocationStore()
	revoked, err := store.IsRevoked("token-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked {
		t.Fatal("token should not be revoked")
	}
}

func TestMockRevocationStore_TokenRevoked(t *testing.T) {
	store := newMockRevocationStore()
	store.revoke("token-123")

	revoked, err := store.IsRevoked("token-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revoked {
		t.Fatal("token should be revoked")
	}
}

func TestMockRevocationStore_MultipleTokens(t *testing.T) {
	store := newMockRevocationStore()
	store.revoke("bad-token")

	bad, _ := store.IsRevoked("bad-token")
	if !bad {
		t.Fatal("bad-token should be revoked")
	}

	good, _ := store.IsRevoked("good-token")
	if good {
		t.Fatal("good-token should NOT be revoked")
	}

	store.revoke("another-bad")
	stillBad, _ := store.IsRevoked("bad-token")
	if !stillBad {
		t.Fatal("bad-token should still be revoked after revoking another")
	}
}

func TestErrorRevocationStore_ReturnsError(t *testing.T) {
	store := &errorRevocationStore{}
	revoked, err := store.IsRevoked("any-token")
	if err == nil {
		t.Fatal("expected an error from errorRevocationStore")
	}
	if revoked {
		t.Fatal("should return false on error")
	}
}

func TestErrorRevocationStore_ErrorMessage(t *testing.T) {
	store := &errorRevocationStore{}
	_, err := store.IsRevoked("token")
	const expected = "database connection failed"
	if err.Error() != expected {
		t.Fatalf("expected %q, got: %q", expected, err.Error())
	}
}

// ---------------------------------------------------------------------------
// Keychain constructor and interface satisfaction tests
// ---------------------------------------------------------------------------

func TestNewFileKeychain_NonNil(t *testing.T) {
	kc := NewFileKeychain()
	if kc == nil {
		t.Fatal("NewFileKeychain() returned nil")
	}
}

func TestFileKeychain_SatisfiesKeychainInterface(t *testing.T) {
	// Compile-time check: *FileKeychain implements Keychain.
	var _ Keychain = (*FileKeychain)(nil)
}

func TestNewLinuxKeychain_NonNil(t *testing.T) {
	kc := NewLinuxKeychain()
	if kc == nil {
		t.Fatal("NewLinuxKeychain() returned nil")
	}
}

func TestLinuxKeychain_SatisfiesKeychainInterface(t *testing.T) {
	var _ Keychain = (*LinuxKeychain)(nil)
}

func TestNewWindowsKeychain_NonNil(t *testing.T) {
	kc := NewWindowsKeychain()
	if kc == nil {
		t.Fatal("NewWindowsKeychain() returned nil")
	}
}

func TestWindowsKeychain_SatisfiesKeychainInterface(t *testing.T) {
	var _ Keychain = (*WindowsKeychain)(nil)
}

func TestMacOSKeychain_SatisfiesKeychainInterface(t *testing.T) {
	var _ Keychain = (*MacOSKeychain)(nil)
}

func TestNewKeychain_Factory(t *testing.T) {
	kc, err := NewKeychain()
	if err != nil {
		t.Fatalf("NewKeychain() returned error: %v", err)
	}
	if kc == nil {
		t.Fatal("NewKeychain() returned nil")
	}
}

// ---------------------------------------------------------------------------
// sanitizeKeychainParam tests
// ---------------------------------------------------------------------------

func TestSanitizeKeychainParam_Valid(t *testing.T) {
	cases := []struct {
		param string
		name  string
	}{
		{"my-service", "service"},
		{"my_account-123", "account"},
		{"simple", "service"},
		{"a", "account"},
	}
	for _, tc := range cases {
		if err := sanitizeKeychainParam(tc.param, tc.name); err != nil {
			t.Errorf("valid param %q (%s) should pass, got: %v", tc.param, tc.name, err)
		}
	}
}

func TestSanitizeKeychainParam_Empty(t *testing.T) {
	if err := sanitizeKeychainParam("", "service"); err == nil {
		t.Fatal("empty param should fail")
	}
}

func TestSanitizeKeychainParam_DangerousChars(t *testing.T) {
	dangerous := []struct {
		char  string
		input string
	}{
		{"$", "$HOME"},
		{"`", "`id`"},
		{"(", "(x)"},
		{")", "(x)"},
		{"|", "a|b"},
		{"&", "a&b"},
		{";", "a;b"},
		{"<", "<file"},
		{">", "file>"},
		{"\\", "a\\b"},
		{"\"", "\"string\""},
		{"'", "'string'"},
		{"*", "a*b"},
		{"?", "a?b"},
		{"[", "[x]"},
		{"]", "[x]"},
		{"{", "{x}"},
		{"}", "{x}"},
		{"!", "!cmd"},
		{"#", "#comment"},
		{"~", "~user"},
	}
	for _, tc := range dangerous {
		if err := sanitizeKeychainParam(tc.input, "service"); err == nil {
			t.Errorf("input %q (contains %s) should be rejected", tc.input, tc.char)
		}
	}
}

func TestSanitizeKeychainParam_Newline(t *testing.T) {
	if err := sanitizeKeychainParam("a\nb", "service"); err == nil {
		t.Fatal("newline should be rejected")
	}
	if err := sanitizeKeychainParam("a\rb", "service"); err == nil {
		t.Fatal("carriage return should be rejected")
	}
	if err := sanitizeKeychainParam("a\tb", "service"); err == nil {
		t.Fatal("tab should be rejected")
	}
}

func TestSanitizeKeychainParam_InjectionPatterns(t *testing.T) {
	if err := sanitizeKeychainParam("../etc", "service"); err == nil {
		t.Fatal("path traversal '../' should be rejected")
	}
	if err := sanitizeKeychainParam("..\\etc", "service"); err == nil {
		t.Fatal("path traversal '..\\' should be rejected")
	}
	if err := sanitizeKeychainParam("$((", "service"); err == nil {
		t.Fatal("shell injection '$((' should be rejected")
	}
}

// ---------------------------------------------------------------------------
// sanitizeFilename tests
// ---------------------------------------------------------------------------

func TestSanitizeFilename_AllowsSafeNames(t *testing.T) {
	cases := []string{
		"my-service",
		"my_account",
		"simple",
		"a",
		"test123",
		"ABC",
		"a-b_c",
	}
	for _, input := range cases {
		got := sanitizeFilename(input)
		if got != input {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", input, got, input)
		}
	}
}

func TestSanitizeFilename_RejectsPathSeparators(t *testing.T) {
	if got := sanitizeFilename("hello/world"); got != "dangerous_chars_rejected" {
		t.Fatalf("expected 'dangerous_chars_rejected', got: %q", got)
	}
	if got := sanitizeFilename("hello\\world"); got != "dangerous_chars_rejected" {
		t.Fatalf("expected 'dangerous_chars_rejected', got: %q", got)
	}
	if got := sanitizeFilename("hello\x00world"); got != "dangerous_chars_rejected" {
		t.Fatalf("expected 'dangerous_chars_rejected', got: %q", got)
	}
}

func TestSanitizeFilename_RejectsPathTraversal(t *testing.T) {
	if got := sanitizeFilename(".."); got != "path_traversal_rejected" {
		t.Fatalf("expected 'path_traversal_rejected', got: %q", got)
	}
	if got := sanitizeFilename("../etc"); got != "path_traversal_rejected" {
		t.Fatalf("expected 'path_traversal_rejected', got: %q", got)
	}
	if got := sanitizeFilename("foo..bar"); got != "path_traversal_rejected" {
		t.Fatalf("expected 'path_traversal_rejected', got: %q", got)
	}
}

func TestSanitizeFilename_ReplacesSpecialChars(t *testing.T) {
	got := sanitizeFilename("hello.world@example#test!")
	// All special characters become underscores.
	for _, c := range got {
		if !(('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-' || c == '_') {
			t.Fatalf("unexpected character %q in sanitized output %q", c, got)
		}
	}
}

// ---------------------------------------------------------------------------
// SecretService and Windows keychain stub tests (platform-independent)
// ---------------------------------------------------------------------------

func TestNewSecretServiceKeychain_NonNilOrError(t *testing.T) {
	kc, err := NewSecretServiceKeychain()
	// On non-linux this returns (nil, ErrSecretServiceNotAvailable).
	// On linux it may succeed or fail depending on the D-Bus availability.
	// Either way the contract is: non-nil with no error, or nil with an error.
	if kc == nil && err == nil {
		t.Fatal("contract violation: both keychain and error are nil")
	}
	if kc != nil && err != nil {
		t.Fatalf("contract violation: keychain is non-nil but error is %v", err)
	}
}

func TestSecretServiceKeychain_SatisfiesInterface(t *testing.T) {
	var _ interface {
		Set(service, account string, data []byte) error
		Get(service, account string) ([]byte, error)
		Delete(service, account string) error
		List(service string) ([]Account, error)
	} = (*SecretServiceKeychain)(nil)
}

func TestNewWindowsKeychainNative_NonNil(t *testing.T) {
	kc, err := NewWindowsKeychainNative()
	if err != nil {
		t.Fatalf("NewWindowsKeychainNative() should not error: %v", err)
	}
	if kc == nil {
		t.Fatal("NewWindowsKeychainNative() returned nil")
	}
}

func TestWindowsKeychainNative_Set_ReturnsErrSecretServiceNotAvailable(t *testing.T) {
	kc, _ := NewWindowsKeychainNative()
	if err := kc.Set("s", "a", "p"); err != ErrSecretServiceNotAvailable {
		t.Fatalf("expected ErrSecretServiceNotAvailable, got: %v", err)
	}
}

func TestWindowsKeychainNative_Get_ReturnsErrSecretServiceNotAvailable(t *testing.T) {
	kc, _ := NewWindowsKeychainNative()
	_, err := kc.Get("s", "a")
	if err != ErrSecretServiceNotAvailable {
		t.Fatalf("expected ErrSecretServiceNotAvailable, got: %v", err)
	}
}

func TestWindowsKeychainNative_Delete_ReturnsErrSecretServiceNotAvailable(t *testing.T) {
	kc, _ := NewWindowsKeychainNative()
	if err := kc.Delete("s", "a"); err != ErrSecretServiceNotAvailable {
		t.Fatalf("expected ErrSecretServiceNotAvailable, got: %v", err)
	}
}

func TestWindowsKeychainNative_List_ReturnsEmpty(t *testing.T) {
	kc, _ := NewWindowsKeychainNative()
	accounts, err := kc.List("s")
	if err != nil {
		t.Fatalf("List should not error on stub: %v", err)
	}
	if accounts != nil {
		t.Fatal("expected nil accounts list from stub")
	}
}
