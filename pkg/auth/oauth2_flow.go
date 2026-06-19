package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// stateStore maps state values to client IDs for CSRF protection.
var stateStore sync.Map

// AuthorizationCodeFlow implements the OAuth2 Authorization Code grant.
type AuthorizationCodeFlow struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
	PKCE         bool
	client       *http.Client
}

// Start initiates the authorization flow and returns an AuthRequest.
func (f *AuthorizationCodeFlow) Start() (*AuthRequest, error) {
	// Generate random state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Store state → clientID mapping
	stateStore.Store(state, f.ClientID)

	// Build authorization URL
	authURL, err := url.Parse(f.AuthURL)
	if err != nil {
		return nil, fmt.Errorf("parse auth URL: %w", err)
	}

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", f.ClientID)
	q.Set("redirect_uri", f.RedirectURL)
	q.Set("state", state)
	if len(f.Scopes) > 0 {
		q.Set("scope", strings.Join(f.Scopes, " "))
	}
	authURL.RawQuery = q.Encode()

	return &AuthRequest{
		URL:       authURL.String(),
		State:     state,
		RequestID: state, // RequestID is the same as state for simplicity
	}, nil
}

// Exchange exchanges an authorization code for a token.
func (f *AuthorizationCodeFlow) Exchange(code, state, requestID string) (*TokenResponse, error) {
	// Validate state
	storedClientID, ok := stateStore.Load(state)
	if !ok {
		return nil, fmt.Errorf("invalid state: state not found")
	}
	if storedClientID != f.ClientID {
		return nil, fmt.Errorf("invalid state: client ID mismatch")
	}

	// Clean up state after validation
	stateStore.Delete(state)

	// Build token request
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", f.ClientID)
	form.Set("client_secret", f.ClientSecret)
	form.Set("redirect_uri", f.RedirectURL)

	client := f.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest("POST", f.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &tr, nil
}
