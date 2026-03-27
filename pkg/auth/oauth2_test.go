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

func TestAuthorizationCodeFlow_Start_StoresState(t *testing.T) {
	flow := &AuthorizationCodeFlow{
		ClientID:    "test-client-id",
		AuthURL:     "https://example.com/auth",
		RedirectURL: "http://localhost:8080/callback",
		Scopes:      []string{"read"},
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if flow.State == "" {
		t.Fatal("Start() did not store state in flow.State")
	}

	if flow.State != req.State {
		t.Errorf("flow.State (%s) != req.State (%s)", flow.State, req.State)
	}
}

func TestAuthorizationCodeFlow_Exchange_ValidState(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "test-access-token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	}))
	defer mockServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		TokenURL:     mockServer.URL,
		RedirectURL:  "http://localhost:8080/callback",
		State:        "test-state-123",
	}

	resp, err := flow.Exchange("test-auth-code", "test-state-123")
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}

	if resp.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %v, want test-access-token", resp.AccessToken)
	}
}

func TestAuthorizationCodeFlow_Exchange_InvalidState(t *testing.T) {
	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		TokenURL:     "https://example.com/token",
		RedirectURL:  "http://localhost:8080/callback",
		State:        "expected-state",
	}

	_, err := flow.Exchange("test-auth-code", "attacker-state")
	if err == nil {
		t.Fatal("Exchange() should have failed with state mismatch")
	}

	if !contains(err.Error(), "state mismatch") {
		t.Errorf("Expected state mismatch error, got: %v", err)
	}
}

func TestAuthorizationCodeFlow_Exchange_EmptyState(t *testing.T) {
	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		TokenURL:     "https://example.com/token",
		RedirectURL:  "http://localhost:8080/callback",
		State:        "expected-state",
	}

	_, err := flow.Exchange("test-auth-code", "")
	if err == nil {
		t.Fatal("Exchange() should have failed with empty state")
	}

	if !contains(err.Error(), "state mismatch") {
		t.Errorf("Expected state mismatch error, got: %v", err)
	}
}

func TestAuthorizationCodeFlow_Integration_StartAndExchange(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "test-access-token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	}))
	defer mockServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      "https://example.com/auth",
		TokenURL:     mockServer.URL,
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"read"},
	}

	// Start the flow to generate state
	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if flow.State == "" {
		t.Fatal("Start() did not generate state")
	}

	// Simulate callback with correct state
	resp, err := flow.Exchange("test-auth-code", req.State)
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}

	if resp.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %v, want test-access-token", resp.AccessToken)
	}
}

func TestDeviceCodeFlow_Start(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that makes HTTP request in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
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

func TestDeviceCodeFlow_Poll_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "test-access-token",
			"token_type": "Bearer",
			"expires_in": 3600,
			"refresh_token": "test-refresh-token"
		}`))
	}))
	defer mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := flow.Poll(ctx, "test-device-code", 1)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if token == nil {
		t.Fatal("Poll() returned nil token")
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %v, want test-access-token", token.AccessToken)
	}

	if token.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %v, want test-refresh-token", token.RefreshToken)
	}
}

func TestDeviceCodeFlow_Poll_AuthorizationPending(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if callCount == 1 {
			_, _ = w.Write([]byte(`{"error": "authorization_pending"}`))
		} else {
			_, _ = w.Write([]byte(`{
				"access_token": "test-access-token",
				"token_type": "Bearer",
				"expires_in": 3600
			}`))
		}
	}))
	defer mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := flow.Poll(ctx, "test-device-code", 1)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if token == nil {
		t.Fatal("Poll() returned nil token after retry")
	}

	if callCount < 2 {
		t.Errorf("Expected at least 2 calls (retry), got %d", callCount)
	}
}

func TestDeviceCodeFlow_Poll_SlowDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if callCount <= 2 {
			_, _ = w.Write([]byte(`{"error": "slow_down"}`))
		} else {
			_, _ = w.Write([]byte(`{
				"access_token": "test-access-token",
				"token_type": "Bearer",
				"expires_in": 3600
			}`))
		}
	}))
	defer mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := flow.Poll(ctx, "test-device-code", 1)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if token == nil {
		t.Fatal("Poll() returned nil token after slow_down")
	}
}

func TestDeviceCodeFlow_Poll_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "authorization_pending"}`))
	}))
	defer mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := flow.Poll(ctx, "test-device-code", 1)
	if err == nil {
		t.Fatal("Poll() should have failed with context cancellation")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestDeviceCodeFlow_Poll_NetworkError(t *testing.T) {
	mockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mockServer.Close()

	flow := &DeviceCodeFlow{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := flow.Poll(ctx, "test-device-code", 1)
	if err == nil {
		t.Fatal("Poll() should have failed with network error")
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

func TestTokenManager_Refresh_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "new-access-token",
			"token_type": "Bearer",
			"expires_in": 3600,
			"refresh_token": "new-refresh-token"
		}`))
	}))
	defer mockServer.Close()

	storage := &mockStorage{}
	tm := NewTokenManager(storage, mockServer.URL, "test-client-id", "test-client-secret")

	oldToken := &Token{
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}

	newToken, err := tm.Refresh(oldToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if newToken == nil {
		t.Fatal("Refresh() returned nil token")
	}

	if newToken.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %v, want new-access-token", newToken.AccessToken)
	}

	if newToken.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %v, want new-refresh-token", newToken.RefreshToken)
	}

	if newToken.ExpiresAt.Before(time.Now()) {
		t.Error("Refresh() returned token with past expiry")
	}
}

func TestTokenManager_Refresh_NoRefreshToken(t *testing.T) {
	storage := &mockStorage{}
	tm := NewTokenManager(storage, "https://example.com/token", "test-client-id", "test-client-secret")

	oldToken := &Token{
		AccessToken:  "old-access-token",
		RefreshToken: "",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	_, err := tm.Refresh(oldToken)
	if err == nil {
		t.Fatal("Refresh() should fail when no refresh token available")
	}

	if !contains(err.Error(), "no refresh token") {
		t.Errorf("Expected 'no refresh token' error, got: %v", err)
	}
}

func TestTokenManager_Refresh_ErrorResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	defer mockServer.Close()

	storage := &mockStorage{}
	tm := NewTokenManager(storage, mockServer.URL, "test-client-id", "test-client-secret")

	oldToken := &Token{
		AccessToken:  "old-access-token",
		RefreshToken: "invalid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}

	_, err := tm.Refresh(oldToken)
	if err == nil {
		t.Fatal("Refresh() should fail with error response")
	}

	if !contains(err.Error(), "invalid_grant") {
		t.Errorf("Expected 'invalid_grant' error, got: %v", err)
	}
}

func TestTokenManager_Refresh_NetworkError(t *testing.T) {
	mockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mockServer.Close()

	storage := &mockStorage{}
	tm := NewTokenManager(storage, mockServer.URL, "test-client-id", "test-client-secret")

	oldToken := &Token{
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}

	_, err := tm.Refresh(oldToken)
	if err == nil {
		t.Fatal("Refresh() should fail with network error")
	}
}

func TestTokenManager_Get_AutoRefresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "refreshed-access-token",
			"token_type": "Bearer",
			"expires_in": 3600,
			"refresh_token": "refreshed-refresh-token"
		}`))
	}))
	defer mockServer.Close()

	storage := &mockStorage{}
	tm := NewTokenManager(storage, mockServer.URL, "test-client-id", "test-client-secret")

	expiredToken := &Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "old-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	_ = storage.Save("test-service", expiredToken)

	token, err := tm.Get("test-service")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if token == nil {
		t.Fatal("Get() returned nil token")
	}

	if token.AccessToken != "refreshed-access-token" {
		t.Errorf("AccessToken = %v, want refreshed-access-token", token.AccessToken)
	}
}

func TestTokenManager_Get_NoAutoRefresh(t *testing.T) {
	storage := &mockStorage{}
	tm := NewTokenManager(storage, "https://example.com/token", "test-client-id", "test-client-secret")

	validToken := &Token{
		AccessToken:  "valid-access-token",
		RefreshToken: "",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	_ = storage.Save("test-service", validToken)

	token, err := tm.Get("test-service")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if token == nil {
		t.Fatal("Get() returned nil token")
	}

	if token.AccessToken != "valid-access-token" {
		t.Errorf("AccessToken = %v, want valid-access-token", token.AccessToken)
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

func TestCallbackServer_handleCallback_Success(t *testing.T) {
	server := NewCallbackServer(18082)
	server.state = "test-state-123"

	req := httptest.NewRequest("GET", "/callback?code=test-auth-code&state=test-state-123", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("handleCallback() status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	select {
	case code := <-server.codeChan:
		if code != "test-auth-code" {
			t.Errorf("handleCallback() code = %v, want test-auth-code", code)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handleCallback() did not send code to channel")
	}
}

func TestCallbackServer_handleCallback_InvalidState(t *testing.T) {
	server := NewCallbackServer(18083)
	server.state = "expected-state"

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=wrong-state", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("handleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}

	select {
	case err := <-server.errChan:
		if err == nil || !contains(err.Error(), "state") {
			t.Errorf("handleCallback() error = %v, want 'state' error", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handleCallback() did not send error to channel")
	}
}

func TestCallbackServer_handleCallback_MissingCode(t *testing.T) {
	server := NewCallbackServer(18084)
	server.state = "test-state"

	req := httptest.NewRequest("GET", "/callback?state=test-state", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("handleCallback() status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	select {
	case code := <-server.codeChan:
		if code != "" {
			t.Errorf("handleCallback() code = %v, want empty string", code)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handleCallback() did not send code to channel")
	}
}

func TestCallbackServer_handleCallback_MissingState(t *testing.T) {
	server := NewCallbackServer(18085)
	server.state = "test-state"

	req := httptest.NewRequest("GET", "/callback?code=test-code", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("handleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestCallbackServer_handleCallback_ResponseMessage(t *testing.T) {
	server := NewCallbackServer(18086)
	server.state = "test-state"

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=test-state", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	body := w.Body.String()
	if !contains(body, "Authorization successful") {
		t.Errorf("handleCallback() response = %v, want 'Authorization successful'", body)
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
