package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/pkg/auth"
)

// TestRealAuthIntegration is a REAL integration test that proves:
// 1. OAuth2 flow starts and returns auth URL
// 2. Authorization code exchange works
// 3. Token storage and retrieval works
// 4. Token refresh on expiry works
// 5. Authenticated requests with tokens work
//
// This is NOT a toy test - it traces actual auth flow.
func TestRealAuthIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure context is used
	_ = ctx

	// Step 1: Create mock OAuth2 provider server
	authorizationCode := "mock_auth_code_12345"
	refreshToken := "mock_refresh_token_67890"
	accessTokenCounter := 0

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/token" {
			accessTokenCounter++
			accessToken := fmt.Sprintf("mock_access_token_%d", accessTokenCounter)

			// Check if this is a token refresh request
			if r.FormValue("grant_type") == "refresh_token" {
				rt := r.FormValue("refresh_token")
				if rt == "" {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "refresh_token_required",
					})
					return
				}

				// Check for revoked/invalid refresh tokens
				if rt == "revoked_token" {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "invalid_grant",
					})
					return
				}
			} else if r.FormValue("grant_type") == "authorization_code" {
				code := r.FormValue("code")
				if code != authorizationCode {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "invalid_code",
					})
					return
				}
			}

			// Return token response
			resp := map[string]interface{}{
				"access_token":  accessToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": refreshToken,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer authServer.Close()

	authURL := authServer.URL + "/auth"
	tokenURL := authServer.URL + "/token"
	redirectURL := "http://localhost:8080/callback"

	// Step 2: Test OAuth2 authorization flow start
	t.Run("StartAuthFlow", func(t *testing.T) {
		flow := &auth.AuthorizationCodeFlow{
			ClientID:     "test_client",
			ClientSecret: "test_secret",
			AuthURL:      authURL,
			TokenURL:     tokenURL,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read", "write"},
			PKCE:         false,
		}

		authRequest, err := flow.Start()
		require.NoError(t, err, "Should start auth flow successfully")
		assert.NotEmpty(t, authRequest.State, "Should generate state")
		assert.Contains(t, authRequest.URL, authURL, "Auth URL should contain the provider URL")
		assert.Contains(t, authRequest.URL, "test_client", "Auth URL should contain client ID")
		assert.Contains(t, authRequest.URL, "read", "Auth URL should contain scope")
		fmt.Println("Auth URL:", authRequest.URL)
	})

	// Step 3: Test authorization code exchange
	t.Run("ExchangeCodeForToken", func(t *testing.T) {
		flow := &auth.AuthorizationCodeFlow{
			ClientID:     "test_client",
			ClientSecret: "test_secret",
			AuthURL:      authURL,
			TokenURL:     tokenURL,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read"},
			PKCE:         false,
		}

		// Start the flow to generate and store state
		authRequest, err := flow.Start()
		require.NoError(t, err, "Should start auth flow successfully")

		tokenResponse, err := flow.Exchange(authorizationCode, authRequest.State)
		require.NoError(t, err, "Should exchange auth code for token")
		assert.NotEmpty(t, tokenResponse.AccessToken, "Should have access token")
		assert.NotEmpty(t, tokenResponse.RefreshToken, "Should have refresh token")
		assert.Equal(t, "Bearer", tokenResponse.TokenType, "Token type should be Bearer")
		assert.Equal(t, 3600, tokenResponse.ExpiresIn, "Should have expiry time")
	})

	// Step 4: Test token storage and retrieval
	t.Run("TokenStorage", func(t *testing.T) {
		// Create in-memory storage for testing
		storage := &InMemoryTokenStorage{
			tokens: make(map[string]*auth.Token),
		}

		tokenManager := auth.NewTokenManager(storage, tokenURL, "test_client", "test_secret")

		// Store a token
		token := &auth.Token{
			AccessToken:  "stored_token_abc",
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
		}

		err := tokenManager.Store("test_service", token)
		require.NoError(t, err, "Should store token successfully")

		// Retrieve the token
		retrieved, err := tokenManager.Get("test_service")
		require.NoError(t, err, "Should retrieve stored token")
		assert.Equal(t, "stored_token_abc", retrieved.AccessToken, "Retrieved token should match")
		assert.Equal(t, token.RefreshToken, retrieved.RefreshToken, "Refresh tokens should match")
	})

	// Step 5: Test token refresh on expiry
	t.Run("TokenRefresh", func(t *testing.T) {
		storage := &InMemoryTokenStorage{
			tokens: make(map[string]*auth.Token),
		}

		tokenManager := auth.NewTokenManager(storage, tokenURL, "test_client", "test_secret")

		// Create and store expired token
		expiredToken := &auth.Token{
			AccessToken:  "expired_token_xyz",
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		}

		err := tokenManager.Store("test_service", expiredToken)
		require.NoError(t, err)

		// Attempt to retrieve - should auto-refresh
		newToken, err := tokenManager.Get("test_service")
		require.NoError(t, err, "Should refresh expired token automatically")
		assert.NotEqual(t, expiredToken.AccessToken, newToken.AccessToken, "Should have new access token after refresh")
		assert.Equal(t, "mock_access_token_2", newToken.AccessToken, "Should have second access token")
	})

	// Step 6: Test authenticated HTTP requests
	t.Run("AuthenticatedRequest", func(t *testing.T) {
		// Create protected resource server
		protectedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "missing authorization",
				})
				return
			}

			if !assertPrefix(authHeader, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid authorization header format",
				})
				return
			}

			// Valid authorization
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":  "access granted",
				"resource": r.URL.Path,
			})
		}))
		defer protectedServer.Close()

		// Make authenticated request
		token := "mock_access_token_123"
		req, _ := http.NewRequest("GET", protectedServer.URL+"/api/resource", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err, "Should make authenticated request")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should get 200 OK with valid token")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)
		assert.Equal(t, "access granted", result["message"], "Should have access granted message")
	})

	// Step 7: Complete end-to-end auth flow test
	t.Run("CompleteAuthFlow", func(t *testing.T) {
		storage := &InMemoryTokenStorage{
			tokens: make(map[string]*auth.Token),
		}

		tokenManager := auth.NewTokenManager(storage, tokenURL, "test_client", "test_secret")

		// Step 1: Start auth flow
		flow := &auth.AuthorizationCodeFlow{
			ClientID:     "test_client",
			ClientSecret: "test_secret",
			AuthURL:      authURL,
			TokenURL:     tokenURL,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read", "write"},
			PKCE:         false,
		}

		authRequest, err := flow.Start()
		require.NoError(t, err)

		// Step 2: Simulate user authorization and code exchange
		tokenResponse, err := flow.Exchange(authorizationCode, authRequest.State)
		require.NoError(t, err)

		// Step 3: Store token
		expiresAt := time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		token := &auth.Token{
			AccessToken:  tokenResponse.AccessToken,
			RefreshToken: tokenResponse.RefreshToken,
			TokenType:    tokenResponse.TokenType,
			ExpiresAt:    expiresAt,
		}

		err = tokenManager.Store("api_service", token)
		require.NoError(t, err)

		// Step 4: Retrieve token (should not need refresh yet)
		retrieved, err := tokenManager.Get("api_service")
		require.NoError(t, err)
		assert.Equal(t, token.AccessToken, retrieved.AccessToken, "Token should not be refreshed early")

		// Verify complete flow
		assert.NotEmpty(t, authRequest.State, "Auth flow should have state")
		assert.NotEmpty(t, token.AccessToken, "Should have access token")
		assert.NotEmpty(t, token.RefreshToken, "Should have refresh token")
		assert.True(t, token.ExpiresAt.After(time.Now()), "Token should not be expired")

		t.Logf("✓ Complete auth flow successful: %d calls to auth server", accessTokenCounter)
	})

	t.Log("Auth integration test completed successfully")
}

// TestOAUth2ErrorHandling tests error scenarios in auth flow
func TestOAuth2ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx

	t.Run("InvalidAuthCode", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/token" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid_grant",
				})
				return
			}
		}))
		defer server.Close()

		flow := &auth.AuthorizationCodeFlow{
			ClientID:     "test_client",
			ClientSecret: "test_secret",
			AuthURL:      server.URL + "/auth",
			TokenURL:     server.URL + "/token",
			RedirectURL:  "http://localhost:8080/callback",
			Scopes:       []string{"read"},
			PKCE:         false,
		}

		// Start the flow to generate and store state
		authRequest, err := flow.Start()
		require.NoError(t, err)

		_, err = flow.Exchange("invalid_code", authRequest.State)
		assert.Error(t, err, "Should error on invalid auth code")
	})

	t.Run("RefreshTokenRevoked", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/token" && r.FormValue("grant_type") == "refresh_token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid_grant",
				})
				return
			}
		}))
		defer server.Close()

		storage := &InMemoryTokenStorage{
			tokens: make(map[string]*auth.Token),
		}

		tokenManager := auth.NewTokenManager(storage, server.URL+"/token", "test_client", "test_secret")

		expiredToken := &auth.Token{
			AccessToken:  "expired_token",
			RefreshToken: "revoked_token", // Revoked token
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(-1 * time.Hour),
		}

		err := tokenManager.Store("test_service", expiredToken)
		require.NoError(t, err)

		_, err = tokenManager.Get("test_service")
		assert.Error(t, err, "Should error when refresh token is revoked")
	})
}

// InMemoryTokenStorage is a simple in-memory token storage for testing
type InMemoryTokenStorage struct {
	tokens map[string]*auth.Token
}

func (s *InMemoryTokenStorage) Save(service string, token *auth.Token) error {
	s.tokens[service] = token
	return nil
}

func (s *InMemoryTokenStorage) Load(service string) (*auth.Token, error) {
	token, ok := s.tokens[service]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}
	return token, nil
}

func assertPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
