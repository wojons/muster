package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// clearStateStore removes all entries from the package-level stateStore.
func clearStateStore() {
	stateStore.Range(func(key, _ any) bool {
		stateStore.Delete(key)
		return true
	})
}

// newTestFlow returns an AuthorizationCodeFlow with test values.
func newTestFlow() *AuthorizationCodeFlow {
	return &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      "https://auth.example.com/authorize",
		TokenURL:     "https://auth.example.com/token",
		RedirectURL:  "https://app.example.com/callback",
		Scopes:       []string{"read", "write"},
	}
}

// TestAuthorizationCodeFlow_Start verifies the Start method.
func TestAuthorizationCodeFlow_Start(t *testing.T) {
	t.Run("valid flow", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if req == nil {
			t.Fatal("Start() returned nil request")
		}

		// State should be 32 hex characters (16 bytes)
		if len(req.State) != 32 {
			t.Errorf("State length = %d, want 32", len(req.State))
		}
		if req.RequestID != req.State {
			t.Errorf("RequestID = %q, want %q", req.RequestID, req.State)
		}

		// Parse the URL and verify parameters
		parsed, err := url.Parse(req.URL)
		if err != nil {
			t.Fatalf("URL parse error: %v", err)
		}
		q := parsed.Query()
		if q.Get("response_type") != "code" {
			t.Errorf("response_type = %q, want \"code\"", q.Get("response_type"))
		}
		if q.Get("client_id") != "test-client-id" {
			t.Errorf("client_id = %q, want \"test-client-id\"", q.Get("client_id"))
		}
		if q.Get("redirect_uri") != "https://app.example.com/callback" {
			t.Errorf("redirect_uri = %q, want \"https://app.example.com/callback\"", q.Get("redirect_uri"))
		}
		if q.Get("state") != req.State {
			t.Errorf("state = %q, want %q", q.Get("state"), req.State)
		}
		if q.Get("scope") != "read write" {
			t.Errorf("scope = %q, want \"read write\"", q.Get("scope"))
		}
	})

	t.Run("no scopes", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		f.Scopes = nil
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		parsed, err := url.Parse(req.URL)
		if err != nil {
			t.Fatalf("URL parse error: %v", err)
		}
		if parsed.Query().Get("scope") != "" {
			t.Errorf("scope should be empty when Scopes is nil, got %q", parsed.Query().Get("scope"))
		}
	})

	t.Run("empty scopes", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		f.Scopes = []string{}
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		parsed, err := url.Parse(req.URL)
		if err != nil {
			t.Fatalf("URL parse error: %v", err)
		}
		if parsed.Query().Get("scope") != "" {
			t.Errorf("scope should be empty when Scopes is empty, got %q", parsed.Query().Get("scope"))
		}
	})

	t.Run("single scope", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		f.Scopes = []string{"profile"}
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		parsed, err := url.Parse(req.URL)
		if err != nil {
			t.Fatalf("URL parse error: %v", err)
		}
		if parsed.Query().Get("scope") != "profile" {
			t.Errorf("scope = %q, want \"profile\"", parsed.Query().Get("scope"))
		}
	})

	t.Run("invalid auth URL", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		f.AuthURL = "://invalid"
		req, err := f.Start()
		if err == nil {
			t.Error("Start() should return error for invalid URL")
		}
		if req != nil {
			t.Error("Start() should return nil request on error")
		}
	})

	t.Run("state uniqueness", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req1, err := f.Start()
		if err != nil {
			t.Fatalf("first Start() error = %v", err)
		}
		clearStateStore()
		req2, err := f.Start()
		if err != nil {
			t.Fatalf("second Start() error = %v", err)
		}
		if req1.State == req2.State {
			t.Error("two Start() calls should produce different states (probabilistic)")
		}
	})
}

// TestAuthorizationCodeFlow_Exchange verifies the Exchange method.
func TestAuthorizationCodeFlow_Exchange(t *testing.T) {
	t.Run("successful exchange", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Set up httptest server that returns a valid token response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Errorf("Content-Type = %q, want \"application/x-www-form-urlencoded\"", ct)
			}
			resp := TokenResponse{
				AccessToken:  "access-token-123",
				RefreshToken: "refresh-token-456",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		f.TokenURL = server.URL
		tr, err := f.Exchange("auth-code-xyz", req.State, req.RequestID)
		if err != nil {
			t.Fatalf("Exchange() error = %v", err)
		}
		if tr.AccessToken != "access-token-123" {
			t.Errorf("AccessToken = %q, want \"access-token-123\"", tr.AccessToken)
		}
		if tr.RefreshToken != "refresh-token-456" {
			t.Errorf("RefreshToken = %q, want \"refresh-token-456\"", tr.RefreshToken)
		}
		if tr.TokenType != "Bearer" {
			t.Errorf("TokenType = %q, want \"Bearer\"", tr.TokenType)
		}
		if tr.ExpiresIn != 3600 {
			t.Errorf("ExpiresIn = %d, want 3600", tr.ExpiresIn)
		}
	})

	t.Run("invalid state - not found", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		// Don't call Start() — stateStore is empty
		_, err := f.Exchange("code", "nonexistent-state", "nonexistent-state")
		if err == nil {
			t.Error("Exchange() should return error for unknown state")
		}
		if !strings.Contains(err.Error(), "state not found") {
			t.Errorf("error = %v, want \"state not found\"", err)
		}
	})

	t.Run("invalid state - client ID mismatch", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Change client ID so validation fails
		f.ClientID = "different-client-id"
		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("Exchange() should return error for client ID mismatch")
		}
		if !strings.Contains(err.Error(), "client ID mismatch") {
			t.Errorf("error = %v, want \"client ID mismatch\"", err)
		}
	})

	t.Run("state consumed after exchange", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := TokenResponse{AccessToken: "tok", TokenType: "Bearer"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()
		f.TokenURL = server.URL

		_, err = f.Exchange("code", req.State, req.RequestID)
		if err != nil {
			t.Fatalf("first Exchange() error = %v", err)
		}

		// Second Exchange with same state should fail (state was deleted)
		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("second Exchange() should return error because state was consumed")
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid_grant"))
		}))
		defer server.Close()
		f.TokenURL = server.URL

		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("Exchange() should return error for non-200 response")
		}
		if !strings.Contains(err.Error(), "status 400") {
			t.Errorf("error = %v, want to contain \"status 400\"", err)
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not-json"))
		}))
		defer server.Close()
		f.TokenURL = server.URL

		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("Exchange() should return error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "parse token response") {
			t.Errorf("error = %v, want to contain \"parse token response\"", err)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Use a closed server to simulate connection refused
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close()
		f.TokenURL = server.URL

		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("Exchange() should return error for connection refused")
		}
		if !strings.Contains(err.Error(), "token request failed") {
			t.Errorf("error = %v, want to contain \"token request failed\"", err)
		}
	})

	t.Run("custom client used when set", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := TokenResponse{AccessToken: "custom-tok", TokenType: "Bearer"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()
		f.TokenURL = server.URL

		// Set a custom client
		f.client = server.Client()

		tr, err := f.Exchange("code", req.State, req.RequestID)
		if err != nil {
			t.Fatalf("Exchange() with custom client error = %v", err)
		}
		if tr.AccessToken != "custom-tok" {
			t.Errorf("AccessToken = %q, want \"custom-tok\"", tr.AccessToken)
		}
	})

	t.Run("500 error response", func(t *testing.T) {
		defer clearStateStore()
		f := newTestFlow()
		req, err := f.Start()
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()
		f.TokenURL = server.URL

		_, err = f.Exchange("code", req.State, req.RequestID)
		if err == nil {
			t.Error("Exchange() should return error for 500 response")
		}
		if !strings.Contains(err.Error(), "status 500") {
			t.Errorf("error = %v, want to contain \"status 500\"", err)
		}
	})
}

// TestAuthorizationCodeFlow_Start_WithoutScopes tests Start without the Scopes field.
func TestAuthorizationCodeFlow_Start_WithoutScopes(t *testing.T) {
	defer clearStateStore()
	f := &AuthorizationCodeFlow{
		ClientID:    "id",
		AuthURL:     "https://example.com/auth",
		RedirectURL: "https://example.com/cb",
	}
	req, err := f.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if req.State == "" {
		t.Error("State should not be empty")
	}
	parsed, _ := url.Parse(req.URL)
	if parsed.Query().Get("scope") != "" {
		t.Errorf("scope should be empty for nil Scopes, got %q", parsed.Query().Get("scope"))
	}
}

// TestAuthorizationCodeFlow_ConcurrentStart tests Start() from multiple goroutines.
func TestAuthorizationCodeFlow_ConcurrentStart(t *testing.T) {
	defer clearStateStore()
	f := newTestFlow()

	var wg sync.WaitGroup
	states := make(map[string]bool)
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := f.Start()
			if err != nil {
				t.Errorf("Start() error = %v", err)
				return
			}
			mu.Lock()
			states[req.State] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(states) != 10 {
		t.Errorf("concurrent Start() produced %d unique states in stateStore, want 10", len(states))
	}
}

// TestAuthorizationCodeFlow_Exchange_EmptyCode tests Exchange with empty code.
func TestAuthorizationCodeFlow_Exchange_EmptyCode(t *testing.T) {
	defer clearStateStore()
	f := newTestFlow()
	req, _ := f.Start()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the code is empty in the request body
		_ = r.ParseForm()
		if r.Form.Get("code") != "" {
			t.Error("expected empty code in form")
		}
		resp := TokenResponse{AccessToken: "tok", TokenType: "Bearer"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	f.TokenURL = server.URL

	tr, err := f.Exchange("", req.State, req.RequestID)
	if err != nil {
		t.Fatalf("Exchange() with empty code error = %v", err)
	}
	if tr.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want \"tok\"", tr.AccessToken)
	}
}
