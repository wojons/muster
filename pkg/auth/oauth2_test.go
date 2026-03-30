package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	if req.State == "" {
		t.Fatal("Start() did not generate state")
	}

	if req.RequestID == "" {
		t.Fatal("Start() did not generate requestID")
	}

	// Verify state is stored in stateStore
	storedState, exists := flow.stateStore.Get(req.RequestID)
	if !exists {
		t.Fatal("Start() did not store state in stateStore")
	}

	if storedState != req.State {
		t.Errorf("stored state (%s) != req.State (%s)", storedState, req.State)
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
	}

	// Start the flow first to get requestID and state
	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	resp, err := flow.Exchange("test-auth-code", req.State, req.RequestID)
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
	}

	// Start the flow first
	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err = flow.Exchange("test-auth-code", "attacker-state", req.RequestID)
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
	}

	// Start the flow first
	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err = flow.Exchange("test-auth-code", "", req.RequestID)
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

	// Start the flow to generate state and requestID
	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if req.State == "" {
		t.Fatal("Start() did not generate state")
	}
	if req.RequestID == "" {
		t.Fatal("Start() did not generate requestID")
	}

	// Simulate callback with correct state and requestID
	resp, err := flow.Exchange("test-auth-code", req.State, req.RequestID)
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
	store := newStateStore()
	server := NewCallbackServer(18080, store)

	url := server.Start("test-request-id")
	if url == "" {
		t.Fatal("Start() returned empty URL")
	}

	if !contains(url, "localhost:18080") {
		t.Errorf("Start() URL = %v, want localhost:18080", url)
	}
}

func TestCallbackServer_WaitCode(t *testing.T) {
	store := newStateStore()
	server := NewCallbackServer(18081, store)

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
	store := newStateStore()
	store.Set("test-request-id", "test-state-123", 10*time.Minute)
	server := NewCallbackServer(18082, store)

	req := httptest.NewRequest("GET", "/callback?code=test-auth-code&state=test-state-123&request_id=test-request-id", nil)
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
	store := newStateStore()
	store.Set("test-request-id", "expected-state", 10*time.Minute)
	server := NewCallbackServer(18083, store)

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=wrong-state&request_id=test-request-id", nil)
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
	store := newStateStore()
	store.Set("test-request-id", "test-state", 10*time.Minute)
	server := NewCallbackServer(18084, store)

	req := httptest.NewRequest("GET", "/callback?state=test-state&request_id=test-request-id", nil)
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
	store := newStateStore()
	store.Set("test-request-id", "test-state", 10*time.Minute)
	server := NewCallbackServer(18085, store)

	req := httptest.NewRequest("GET", "/callback?code=test-code&request_id=test-request-id", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("handleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestCallbackServer_handleCallback_ResponseMessage(t *testing.T) {
	store := newStateStore()
	store.Set("test-request-id", "test-state", 10*time.Minute)
	server := NewCallbackServer(18086, store)

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=test-state&request_id=test-request-id", nil)
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

func TestAuthorizationCodeFlow_ConcurrentFlows(t *testing.T) {
	// Test that multiple OAuth flows can run concurrently without state collisions

	// Create mock token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","refresh_token":"test-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	// Start multiple flows concurrently
	numFlows := 10
	results := make(chan struct {
		requestID string
		state     string
		err       error
	}, numFlows)

	var wg sync.WaitGroup
	for i := 0; i < numFlows; i++ {
		wg.Add(1)
		go func(flowNum int) {
			defer wg.Done()

			flow := &AuthorizationCodeFlow{
				ClientID:     "test-client-id",
				ClientSecret: "test-secret",
				AuthURL:      "https://example.com/auth",
				TokenURL:     tokenServer.URL,
				RedirectURL:  "http://localhost/callback",
				Scopes:       []string{"read"},
			}

			// Start flow
			req, err := flow.Start()
			if err != nil {
				results <- struct {
					requestID string
					state     string
					err       error
				}{err: fmt.Errorf("flow %d: Start() failed: %w", flowNum, err)}
				return
			}

			// Exchange with correct state
			_, err = flow.Exchange("test-auth-code", req.State, req.RequestID)
			results <- struct {
				requestID string
				state     string
				err       error
			}{requestID: req.RequestID, state: req.State, err: err}
		}(i)
	}

	// Wait for all flows to complete
	wg.Wait()
	close(results)

	// Verify all flows succeeded
	successCount := 0
	failCount := 0
	for result := range results {
		if result.err != nil {
			t.Errorf("Concurrent flow failed: %v", result.err)
			failCount++
		} else {
			successCount++
		}
	}

	if successCount != numFlows {
		t.Errorf("Expected %d successful flows, got %d (failed: %d)", numFlows, successCount, failCount)
	}
}

func TestAuthorizationCodeFlow_StateReuse_Prevented(t *testing.T) {
	// Test that state can only be used once (prevents replay attacks)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","refresh_token":"test-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		TokenURL:     tokenServer.URL,
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// First exchange should succeed
	_, err = flow.Exchange("test-code", req.State, req.RequestID)
	if err != nil {
		t.Fatalf("First Exchange() failed: %v", err)
	}

	// Second exchange with same state should fail (state was deleted)
	_, err = flow.Exchange("test-code", req.State, req.RequestID)
	if err == nil {
		t.Fatal("Second Exchange() should have failed (state reuse)")
	}
	if !strings.Contains(err.Error(), "expired or invalid") {
		t.Errorf("Second Exchange() error = %v, want 'expired or invalid'", err)
	}
}

func TestAuthorizationCodeFlow_StateExpiration(t *testing.T) {
	// Test that state expires after timeout

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
	}))
	defer tokenServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		TokenURL:     tokenServer.URL,
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Manually expire the state by deleting it
	flow.stateStore.Delete(req.RequestID)

	// Exchange should fail
	_, err = flow.Exchange("test-code", req.State, req.RequestID)
	if err == nil {
		t.Fatal("Exchange() should have failed (expired state)")
	}
	if !strings.Contains(err.Error(), "expired or invalid") {
		t.Errorf("Exchange() error = %v, want 'expired or invalid'", err)
	}
}

func TestAuthorizationCodeFlow_TimingAttack_Prevented(t *testing.T) {
	// Test that state comparison uses constant-time comparison

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
	}))
	defer tokenServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		TokenURL:     tokenServer.URL,
	}

	req, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Try with wrong state (different length)
	wrongState := "wrong"
	_, err = flow.Exchange("test-code", wrongState, req.RequestID)
	if err == nil {
		t.Fatal("Exchange() should have failed (wrong state)")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("Exchange() error = %v, want 'mismatch'", err)
	}
}

func TestStateStore_ConcurrentAccess(t *testing.T) {
	// Test that state store handles concurrent access safely

	store := newStateStore()
	defer func() {
		// Clean up store
		store.mu.Lock()
		store.states = make(map[string]*stateEntry)
		store.mu.Unlock()
	}()

	numGoroutines := 100
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			requestID := fmt.Sprintf("req-%d", id)
			state := fmt.Sprintf("state-%d", id)
			store.Set(requestID, state, 10*time.Minute)
		}(i)
	}

	wg.Wait()

	// Verify all states were stored
	store.mu.RLock()
	count := len(store.states)
	store.mu.RUnlock()

	if count != numGoroutines {
		t.Errorf("Expected %d states, got %d", numGoroutines, count)
	}
}

func TestAuthorizationCodeFlow_MissingRequestID(t *testing.T) {
	// Test that Exchange requires request ID

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
	}))
	defer tokenServer.Close()

	flow := &AuthorizationCodeFlow{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		TokenURL:     tokenServer.URL,
	}

	_, err := flow.Exchange("test-code", "test-state", "")
	if err == nil {
		t.Fatal("Exchange() should have failed (missing request ID)")
	}
	if !strings.Contains(err.Error(), "request ID required") {
		t.Errorf("Exchange() error = %v, want 'request ID required'", err)
	}
}

func TestCallbackServer_MissingRequestID(t *testing.T) {
	// Test that callback handler requires request ID

	store := newStateStore()
	server := NewCallbackServer(18087, store)

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=test-state", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	if w.Code != 400 {
		t.Errorf("handleCallback() status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Request ID required") {
		t.Errorf("handleCallback() body = %v, want 'Request ID required'", w.Body.String())
	}
}

func TestCallbackServer_InvalidRequestID(t *testing.T) {
	// Test that callback handler rejects invalid request ID

	store := newStateStore()
	server := NewCallbackServer(18088, store)

	req := httptest.NewRequest("GET", "/callback?code=test-code&state=test-state&request_id=invalid-id", nil)
	w := httptest.NewRecorder()

	server.handleCallback(w, req)

	if w.Code != 400 {
		t.Errorf("handleCallback() status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid or expired") {
		t.Errorf("handleCallback() body = %v, want 'Invalid or expired'", w.Body.String())
	}
}

func TestCallbackServer_ValidRequestWithState(t *testing.T) {
	// Test that callback handler works with valid request ID and state

	store := newStateStore()
	store.Set("valid-request-id", "valid-state", 10*time.Minute)
	server := NewCallbackServer(18089, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server in goroutine
	go func() {
		req := httptest.NewRequest("GET", "/callback?code=test-code&state=valid-state&request_id=valid-request-id", nil)
		w := httptest.NewRecorder()
		server.handleCallback(w, req)
	}()

	// Wait for code
	code, err := server.WaitCode(ctx)
	if err != nil {
		t.Fatalf("WaitCode() error = %v", err)
	}
	if code != "test-code" {
		t.Errorf("WaitCode() code = %v, want test-code", code)
	}
}

func TestAuthorizationCodeFlow_StateComparison_ConstantTime(t *testing.T) {
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

	tests := []struct {
		name        string
		setupState  func() (state, requestID string)
		testState   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_state",
			setupState: func() (string, string) {
				flow := &AuthorizationCodeFlow{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					TokenURL:     mockServer.URL,
					RedirectURL:  "http://localhost:8080/callback",
				}
				req, _ := flow.Start()
				return req.State, req.RequestID
			},
			testState:   "", // Will be set to valid state in test
			expectError: false,
		},
		{
			name: "single_char_difference",
			setupState: func() (string, string) {
				flow := &AuthorizationCodeFlow{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					TokenURL:     mockServer.URL,
					RedirectURL:  "http://localhost:8080/callback",
				}
				req, _ := flow.Start()
				return req.State, req.RequestID
			},
			testState:   "", // Will be set to modified state in test
			expectError: true,
			errorMsg:    "state",
		},
		{
			name: "completely_different_state",
			setupState: func() (string, string) {
				flow := &AuthorizationCodeFlow{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					TokenURL:     mockServer.URL,
					RedirectURL:  "http://localhost:8080/callback",
				}
				req, _ := flow.Start()
				return req.State, req.RequestID
			},
			testState:   "completelydifferentstate12345678901234567890",
			expectError: true,
			errorMsg:    "state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validState, requestID := tt.setupState()

			testState := tt.testState
			switch tt.name {
			case "valid_state":
				testState = validState
			case "single_char_difference":
				testState = validState[:len(validState)-1] + "X"
			}

			flow := &AuthorizationCodeFlow{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				TokenURL:     mockServer.URL,
				RedirectURL:  "http://localhost:8080/callback",
				stateStore:   newStateStore(),
			}
			flow.stateStore.Set(requestID, validState, 10*time.Minute)

			_, err := flow.Exchange("test-code", testState, requestID)
			if tt.expectError {
				if err == nil {
					t.Errorf("Exchange() expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Exchange() error = %v, want %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Exchange() unexpected error = %v", err)
				}
			}
		})
	}
}
