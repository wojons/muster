package daemon

import (
	"fmt"
	"testing"
)

func TestNewTokenManager(t *testing.T) {
	tm := NewTokenManager()
	if tm == nil {
		t.Fatal("NewTokenManager() returned nil")
	}
	if tm.tokens == nil {
		t.Fatal("tokens map is nil")
	}
}

func TestTokenManager_AddToken(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("token-1")
	if !tm.tokens["token-1"] {
		t.Fatal("token should exist in map after AddToken")
	}
}

func TestTokenManager_AddToken_Multiple(t *testing.T) {
	tm := NewTokenManager()
	for i := 0; i < 5; i++ {
		tm.AddToken(fmt.Sprintf("token-%d", i))
	}
	for i := 0; i < 5; i++ {
		if !tm.tokens[fmt.Sprintf("token-%d", i)] {
			t.Fatalf("token-%d should exist after AddToken", i)
		}
	}
}

func TestTokenManager_RemoveToken(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("token-1")
	tm.RemoveToken("token-1")
	if tm.tokens["token-1"] {
		t.Fatal("token should be removed from map")
	}
}

func TestTokenManager_RemoveToken_NonExistent(t *testing.T) {
	tm := NewTokenManager()
	// Should not panic
	tm.RemoveToken("nonexistent")
	// Map should still be valid
	if tm.tokens == nil {
		t.Fatal("tokens map should not be nil after removing non-existent key")
	}
}

func TestTokenManager_Validate(t *testing.T) {
	tm := NewTokenManager()

	if tm.Validate("") {
		t.Fatal(`Validate("") should return false`)
	}
	if tm.Validate("bad-token") {
		t.Fatal("Validate(bad-token) should return false")
	}

	tm.AddToken("good-token")
	if !tm.Validate("good-token") {
		t.Fatal("Validate(good-token) should return true after AddToken")
	}

	tm.RemoveToken("good-token")
	if tm.Validate("good-token") {
		t.Fatal("Validate should return false after RemoveToken")
	}
}

func TestTokenManager_ValidateFromHeader(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("my-secret-token")

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"valid Bearer", "Bearer my-secret-token", true},
		{"missing Bearer prefix", "my-secret-token", false},
		{"empty header", "", false},
		{"lowercase bearer", "bearer my-secret-token", false},
		{"invalid token in Bearer", "Bearer wrong-token", false},
		{"Bear only prefix", "Bear my-secret-token", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tm.ValidateFromHeader(tt.header)
			if got != tt.want {
				t.Errorf("ValidateFromHeader(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	tm := NewTokenManager()
	const numGoroutines = 50
	done := make(chan struct{}, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			token := fmt.Sprintf("token-%d", id)
			tm.AddToken(token)
			tm.Validate(token)
			tm.RemoveToken(token)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
