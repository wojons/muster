package auth

import (
	"testing"
	"time"
)

// ── Token.IsExpired ─────────────────────────────────────────

func TestToken_IsExpired_PastExpiry(t *testing.T) {
	tok := &Token{
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired an hour ago
	}
	if !tok.IsExpired() {
		t.Error("IsExpired should return true for token expired 1 hour ago")
	}
}

func TestToken_IsExpired_FutureExpiry(t *testing.T) {
	tok := &Token{
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(5 * time.Minute), // expires in 5 minutes — beyond 60s grace
	}
	if tok.IsExpired() {
		t.Error("IsExpired should return false for token expiring in 5 minutes")
	}
}

func TestToken_IsExpired_WithinGracePeriod(t *testing.T) {
	tok := &Token{
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(30 * time.Second), // expires in 30s — within 60s grace
	}
	if !tok.IsExpired() {
		t.Error("IsExpired should return true for token within 60s grace period")
	}
}

func TestToken_IsExpired_ExactlyAtGraceBoundary(t *testing.T) {
	tok := &Token{
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(60 * time.Second), // exactly at 60s threshold
	}
	// After(expiresAt) with 60s offset — current time + 60s > expiresAt → true
	if !tok.IsExpired() {
		t.Error("IsExpired should return true when current+60s == expiresAt")
	}
}

func TestToken_IsExpired_ZeroTime(t *testing.T) {
	tok := &Token{
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Time{}, // zero value = epoch
	}
	// time.Now().Add(60s) is always after epoch → always expired
	if !tok.IsExpired() {
		t.Error("IsExpired should return true for zero-time ExpiresAt")
	}
}

func TestToken_IsExpired_NilReceiver(t *testing.T) {
	var tok *Token
	// Calling IsExpired on nil *Token panics because it dereferences t.ExpiresAt.
	// This is standard Go behavior — nil receiver is fine, nil field access is not.
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_ = tok.IsExpired()
	}()
	if !panicked {
		t.Error("IsExpired on nil *Token should panic (dereferences t.ExpiresAt)")
	}
}

func TestToken_IsExpired_EmptyToken(t *testing.T) {
	tok := &Token{}
	if !tok.IsExpired() {
		t.Error("IsExpired should return true for Token with zero-value ExpiresAt")
	}
}
