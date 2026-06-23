package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenManager manages OAuth2 tokens with automatic refresh.
type TokenManager struct {
	storage     TokenStorage
	tokenURL    string
	clientID    string
	clientSecret string
	client      *http.Client
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(storage TokenStorage, tokenURL, clientID, clientSecret string) *TokenManager {
	return &TokenManager{
		storage:      storage,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Store saves a token to persistent storage.
func (tm *TokenManager) Store(service string, token *Token) error {
	return tm.storage.Save(service, token)
}

// Get retrieves a token, auto-refreshing if expired.
func (tm *TokenManager) Get(service string) (*Token, error) {
	token, err := tm.storage.Load(service)
	if err != nil {
		return nil, fmt.Errorf("load token: %w", err)
	}

	if token == nil {
		return nil, fmt.Errorf("token not found for service %q", service)
	}

	if !token.IsExpired() {
		return token, nil
	}

	// Token is expired — attempt refresh
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("token expired and no refresh token available")
	}

	refreshed, err := tm.refresh(token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	// Store the refreshed token
	if err := tm.storage.Save(service, refreshed); err != nil {
		return nil, fmt.Errorf("save refreshed token: %w", err)
	}

	return refreshed, nil
}

// refresh POSTs to the token endpoint with grant_type=refresh_token.
func (tm *TokenManager) refresh(refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", tm.clientID)
	form.Set("client_secret", tm.clientSecret)

	req, err := http.NewRequest("POST", tm.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}
