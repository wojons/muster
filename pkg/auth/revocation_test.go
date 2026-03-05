package auth

import (
	"context"
	"testing"
	"time"
)

func TestNewRevocationManager(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	if rm == nil {
		t.Fatal("NewRevocationManager() returned nil")
	}

	if rm.store == nil {
		t.Fatal("RevocationManager store is nil")
	}
}

func TestMemoryRevocationStore_Revoke(t *testing.T) {
	store := NewMemoryRevocationStore()

	token := "test-token"
	expiresAt := time.Now().Add(24 * time.Hour)

	err := store.Revoke(token, expiresAt)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	revoked, err := store.IsRevoked(token)
	if err != nil {
		t.Fatalf("IsRevoked() error = %v", err)
	}

	if !revoked {
		t.Error("IsRevoked() = false, want true")
	}
}

func TestMemoryRevocationStore_IsRevoked_NotFound(t *testing.T) {
	store := NewMemoryRevocationStore()

	revoked, err := store.IsRevoked("nonexistent-token")
	if err != nil {
		t.Fatalf("IsRevoked() error = %v", err)
	}

	if revoked {
		t.Error("IsRevoked() = true, want false for nonexistent token")
	}
}

func TestMemoryRevocationStore_IsRevoked_Expired(t *testing.T) {
	store := NewMemoryRevocationStore()

	token := "expired-token"
	expiresAt := time.Now().Add(-1 * time.Hour)

	store.Revoke(token, expiresAt)

	revoked, err := store.IsRevoked(token)
	if err != nil {
		t.Fatalf("IsRevoked() error = %v", err)
	}

	if revoked {
		t.Error("IsRevoked() = true, want false for expired token")
	}
}

func TestMemoryRevocationStore_Cleanup(t *testing.T) {
	store := NewMemoryRevocationStore()

	now := time.Now()

	store.Revoke("token1", now.Add(-2*time.Hour))
	store.Revoke("token2", now.Add(30*time.Minute))
	store.Revoke("token3", now.Add(-1*time.Hour))

	err := store.Cleanup(now)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	revoked1, _ := store.IsRevoked("token1")
	if revoked1 {
		t.Error("token1 should have been cleaned up")
	}

	revoked2, _ := store.IsRevoked("token2")
	if !revoked2 {
		t.Error("token2 should not have been cleaned up")
	}

	revoked3, _ := store.IsRevoked("token3")
	if revoked3 {
		t.Error("token3 should have been cleaned up")
	}
}

func TestRevocationManager_RevokeToken(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	token := "test-token"
	err := rm.RevokeToken(token)
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}

	revoked, err := store.IsRevoked(token)
	if err != nil {
		t.Fatalf("IsRevoked() error = %v", err)
	}

	if !revoked {
		t.Error("RevokeToken() did not revoke token")
	}
}

func TestRevocationManager_RevokeAPIKey(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	key := "api-key-12345"
	err := rm.RevokeAPIKey(key)
	if err != nil {
		t.Fatalf("RevokeAPIKey() error = %v", err)
	}
}

func TestRevocationManager_ValidateToken(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	token := "valid-token"

	err := rm.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
}

func TestRevocationManager_ValidateToken_Revoked(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	token := "revoked-token"
	store.Revoke(token, time.Now().Add(24*time.Hour))

	err := rm.ValidateToken(token)
	if err == nil {
		t.Fatal("ValidateToken() expected error for revoked token")
	}

	if err.Error() != "token has been revoked" {
		t.Errorf("ValidateToken() error = %v, want token has been revoked", err)
	}
}

func TestRevocationManager_CleanupExpired(t *testing.T) {
	store := NewMemoryRevocationStore()
	rm := NewRevocationManager(store)

	ctx := context.Background()

	err := rm.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
}

func TestHashKey(t *testing.T) {
	key := "test-key"
	hashed := hashKey(key)

	if hashed != key {
		t.Errorf("hashKey() = %v, want %v", hashed, key)
	}
}

func TestRevokedToken(t *testing.T) {
	err := &ErrTokenRevoked{Token: "test-token"}

	if err.Error() != "token has been revoked" {
		t.Errorf("ErrTokenRevoked.Error() = %v", err.Error())
	}

	if err.Token != "test-token" {
		t.Errorf("ErrTokenRevoked.Token = %v, want test-token", err.Token)
	}
}
