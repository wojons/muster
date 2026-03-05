package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateSecureState(t *testing.T) {
	state := generateSecureState()
	if state == "" {
		t.Fatal("generateSecureState() returned empty")
	}
	if len(state) == 0 {
		t.Fatal("generateSecureState() returned state with length 0")
	}
}

func TestGenerateCodeVerifier(t *testing.T) {
	verifier := generateCodeVerifier()
	if verifier == "" {
		t.Fatal("generateCodeVerifier() returned empty")
	}
	if len(verifier) < 32 {
		t.Fatalf("generateCodeVerifier() returned short verifier: %d", len(verifier))
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := generateCodeVerifier()
	challenge := generateCodeChallenge(verifier)

	if challenge == "" {
		t.Fatal("generateCodeChallenge() returned empty")
	}

	if challenge == verifier {
		t.Fatal("generateCodeChallenge() should not equal verifier")
	}
}

func TestAuthorizationCodeFlow_Start(t *testing.T) {
	flow := &AuthorizationCodeFlow{
		ClientID:    "test-client-id",
		AuthURL:     "https://example.com/auth",
		RedirectURL: "http://localhost:8080/callback",
		Scopes:      []string{"read", "write"},
		PKCE:        false,
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if req.URL == "" {
		t.Fatal("Start() returned empty URL")
	}

	if req.State == "" {
		t.Fatal("Start() returned empty state")
	}

	if !contains(req.URL, "client_id=test-client-id") {
		t.Error("Start() URL missing client_id")
	}

	if !contains(req.URL, "scope=read+write") {
		t.Error("Start() URL missing scope")
	}
}

func TestAuthorizationCodeFlow_Start_PKCE(t *testing.T) {
	flow := &AuthorizationCodeFlow{
		ClientID:    "test-client-id",
		AuthURL:     "https://example.com/auth",
		RedirectURL: "http://localhost:8080/callback",
		Scopes:      []string{"read"},
		PKCE:        true,
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if req.URL == "" {
		t.Fatal("Start() returned empty URL")
	}

	if !contains(req.URL, "code_challenge=") {
		t.Error("Start() URL missing code_challenge for PKCE")
	}

	if !contains(req.URL, "code_challenge_method=S256") {
		t.Error("Start() URL missing code_challenge_method for PKCE")
	}

	if flow.codeVerifier == "" {
		t.Fatal("Start() did not set codeVerifier for PKCE")
	}
}

func TestDeviceCodeFlow_Start(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that makes HTTP request in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"device_code": "test-device-code",
			"user_code": "ABCD-EFGH",
			"verification_uri": "https://example.com/verify",
			"expires_in": 1800,
			"interval": 5
		}`))
	}))
	defer mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID:  "test-client-id",
		DeviceURL: mockServer.URL,
		TokenURL:  "https://example.com/token",
		Scopes:    []string{"read"},
	}

	resp, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if resp == nil {
		t.Fatal("Start() returned nil response")
	}

	if resp.DeviceCode != "test-device-code" {
		t.Errorf("DeviceCode = %v, want test-device-code", resp.DeviceCode)
	}

	if resp.UserCode != "ABCD-EFGH" {
		t.Errorf("UserCode = %v, want ABCD-EFGH", resp.UserCode)
	}
}

func TestTokenManager(t *testing.T) {
	storage := &mockStorage{}
	tm := NewTokenManager(storage, "https://example.com/token", "test-client-id", "test-client-secret")

	token := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "read write",
	}

	err := tm.Store("test-service", token)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	retrieved, err := tm.Get("test-service")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.AccessToken != token.AccessToken {
		t.Errorf("Get() AccessToken = %v, want %v", retrieved.AccessToken, token.AccessToken)
	}
}

func TestCallbackServer(t *testing.T) {
	server := NewCallbackServer(18080)

	url := server.Start()
	if url == "" {
		t.Fatal("Start() returned empty URL")
	}

	if !contains(url, "localhost:18080") {
		t.Errorf("Start() URL = %v, want localhost:18080", url)
	}
}

func TestCallbackServer_WaitCode(t *testing.T) {
	server := NewCallbackServer(18081)
	server.codeChan = make(chan string, 1)
	server.errChan = make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(10 * time.Millisecond)
		server.codeChan <- "test-code"
	}()

	code, err := server.WaitCode(ctx)
	if err != nil {
		t.Fatalf("WaitCode() error = %v", err)
	}

	if code != "test-code" {
		t.Errorf("WaitCode() = %v, want test-code", code)
	}
}

type mockStorage struct {
	token *Token
}

func (m *mockStorage) Save(service string, token *Token) error {
	m.token = token
	return nil
}

func (m *mockStorage) Load(service string) (*Token, error) {
	return m.token, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
