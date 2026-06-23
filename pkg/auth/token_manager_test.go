package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockTokenStorage implements TokenStorage for testing
type mockTokenStorage struct {
	tokens  map[string]*Token
	loadErr error // error to return from Load
	saveErr error // error to return from Save
}

func newMockTokenStorage() *mockTokenStorage {
	return &mockTokenStorage{tokens: make(map[string]*Token)}
}

func (m *mockTokenStorage) Save(service string, token *Token) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.tokens[service] = token
	return nil
}

func (m *mockTokenStorage) Load(service string) (*Token, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	t, ok := m.tokens[service]
	if !ok {
		return nil, nil
	}
	return t, nil
}

// TestNewTokenManager verifies the constructor sets all fields
func TestNewTokenManager(t *testing.T) {
	storage := newMockTokenStorage()
	tm := NewTokenManager(storage, "https://auth.example.com/token", "client-123", "secret-abc")

	if tm.storage != storage {
		t.Error("storage not set")
	}
	if tm.tokenURL != "https://auth.example.com/token" {
		t.Errorf("tokenURL = %q, want %q", tm.tokenURL, "https://auth.example.com/token")
	}
	if tm.clientID != "client-123" {
		t.Errorf("clientID = %q, want %q", tm.clientID, "client-123")
	}
	if tm.clientSecret != "secret-abc" {
		t.Errorf("clientSecret = %q, want %q", tm.clientSecret, "secret-abc")
	}
	if tm.client == nil {
		t.Error("client is nil")
	}
	if tm.client.Timeout != 30*time.Second {
		t.Errorf("client timeout = %v, want 30s", tm.client.Timeout)
	}
}

// TestTokenManager_Store_Success stores a token and verifies it can be loaded
func TestTokenManager_Store_Success(t *testing.T) {
	storage := newMockTokenStorage()
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	token := &Token{AccessToken: "tok-abc", TokenType: "Bearer"}
	err := tm.Store("github", token)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	loaded, _ := storage.Load("github")
	if loaded == nil {
		t.Fatal("token not stored")
	}
	if loaded.AccessToken != "tok-abc" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "tok-abc")
	}
}

// TestTokenManager_Store_Error returns storage error
func TestTokenManager_Store_Error(t *testing.T) {
	storage := newMockTokenStorage()
	storage.saveErr = http.ErrServerClosed
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	err := tm.Store("github", &Token{AccessToken: "tok-abc"})
	if err != http.ErrServerClosed {
		t.Errorf("expected storage error, got %v", err)
	}
}

// TestTokenManager_Get_ValidToken returns non-expired token without refresh
func TestTokenManager_Get_ValidToken(t *testing.T) {
	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken: "tok-valid",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	token, err := tm.Get("github")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if token.AccessToken != "tok-valid" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "tok-valid")
	}
}

// TestTokenManager_Get_LoadError returns error from storage
func TestTokenManager_Get_LoadError(t *testing.T) {
	storage := newMockTokenStorage()
	storage.loadErr = http.ErrServerClosed
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error from storage load")
	}
}

// TestTokenManager_Get_NilToken is a defensive test for nil token from storage
func TestTokenManager_Get_NilToken(t *testing.T) {
	storage := newMockTokenStorage()
	// No token stored — Load returns (nil, nil)
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error for nil token")
	}
}

// TestTokenManager_Get_ExpiredNoRefresh returns error for expired token without refresh token
func TestTokenManager_Get_ExpiredNoRefresh(t *testing.T) {
	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-expired",
		RefreshToken: "", // no refresh token
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired 1h ago
	}
	tm := NewTokenManager(storage, "https://auth.example.com/token", "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error for expired token with no refresh token")
	}
}

// TestTokenManager_Get_ExpiredWithRefresh refreshes expired token via httptest
func TestTokenManager_Get_ExpiredWithRefresh(t *testing.T) {
	// Set up a fake token endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "rt-old" {
			t.Errorf("refresh_token = %q, want rt-old", r.FormValue("refresh_token"))
		}

		resp := TokenResponse{
			AccessToken:  "tok-new",
			RefreshToken: "rt-new",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-expired",
		RefreshToken: "rt-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	tm := NewTokenManager(storage, server.URL, "id", "secret")

	token, err := tm.Get("github")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if token.AccessToken != "tok-new" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "tok-new")
	}
	if token.RefreshToken != "rt-new" {
		t.Errorf("RefreshToken = %q, want %q", token.RefreshToken, "rt-new")
	}

	// Verify the refreshed token was persisted
	saved, _ := storage.Load("github")
	if saved == nil || saved.AccessToken != "tok-new" {
		t.Error("refreshed token not persisted to storage")
	}
}

// TestTokenManager_Get_RefreshServerError returns error when refresh endpoint fails
func TestTokenManager_Get_RefreshServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-expired",
		RefreshToken: "rt-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	tm := NewTokenManager(storage, server.URL, "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error from refresh server error")
	}
}

// TestTokenManager_Get_RefreshBadJSON returns error for invalid refresh response
func TestTokenManager_Get_RefreshBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-expired",
		RefreshToken: "rt-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	tm := NewTokenManager(storage, server.URL, "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error for bad JSON refresh response")
	}
}

// TestTokenManager_Get_ExpiresWithinGraceWindow triggers refresh when token
// expires within the 60-second grace window.
func TestTokenManager_Get_ExpiresWithinGraceWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
			AccessToken:  "tok-refreshed-grace",
			RefreshToken: "rt-grace",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-about-to-expire",
		RefreshToken: "rt-grace-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(30 * time.Second), // expires in 30s, within 60s grace
	}
	tm := NewTokenManager(storage, server.URL, "id", "secret")

	token, err := tm.Get("github")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if token.AccessToken != "tok-refreshed-grace" {
		t.Errorf("AccessToken = %q, want tok-refreshed-grace", token.AccessToken)
	}
}

// TestTokenManager_Get_RefreshSaveError returns error when Save fails
// after a successful refresh — this demonstrates the atomicity gap.
func TestTokenManager_Get_RefreshSaveError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
			AccessToken:  "tok-refreshed-save-fail",
			RefreshToken: "rt-save-fail",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := newMockTokenStorage()
	storage.tokens["github"] = &Token{
		AccessToken:  "tok-expired",
		RefreshToken: "rt-old",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	// Save will fail — but the refresh already happened.
	// Get returns the Save error, losing the refreshed token.
	storage.saveErr = http.ErrAbortHandler

	tm := NewTokenManager(storage, server.URL, "id", "secret")

	_, err := tm.Get("github")
	if err == nil {
		t.Error("expected error from failed Save after refresh")
	}
}
