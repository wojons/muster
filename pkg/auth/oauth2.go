package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AuthorizationCodeFlow struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
	State        string
	PKCE         bool
	codeVerifier string
}

type AuthRequest struct {
	URL   string
	State string
}

type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

func generateSecureState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (f *AuthorizationCodeFlow) Start() (*AuthRequest, error) {
	state := generateSecureState()

	params := url.Values{
		"client_id":     {f.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {f.RedirectURL},
		"scope":         {strings.Join(f.Scopes, " ")},
		"state":         {state},
	}

	if f.PKCE {
		codeVerifier := generateCodeVerifier()
		params.Set("code_challenge", generateCodeChallenge(codeVerifier))
		params.Set("code_challenge_method", "S256")
		f.codeVerifier = codeVerifier
	}

	authURL := f.AuthURL + "?" + params.Encode()

	return &AuthRequest{
		URL:   authURL,
		State: state,
	}, nil
}

func (f *AuthorizationCodeFlow) Exchange(code string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":     {f.ClientID},
		"client_secret": {f.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {f.RedirectURL},
	}

	if f.PKCE {
		data.Set("code_verifier", f.codeVerifier)
	}

	resp, err := http.PostForm(f.TokenURL, data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result TokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Error != "" {
		return &result, fmt.Errorf("oauth error: %s", result.Error)
	}

	return &result, nil
}

type DeviceCodeFlow struct {
	ClientID  string
	DeviceURL string
	TokenURL  string
	Scopes    []string
}

func (f *DeviceCodeFlow) Start() (*DeviceAuthResponse, error) {
	data := url.Values{
		"client_id": {f.ClientID},
		"scope":     {strings.Join(f.Scopes, " ")},
	}

	resp, err := http.PostForm(f.DeviceURL, data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result DeviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (f *DeviceCodeFlow) Poll(ctx context.Context, deviceCode string, interval int) (*TokenResponse, error) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			data := url.Values{
				"client_id":   {f.ClientID},
				"device_code": {deviceCode},
				"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			}

			resp, err := http.PostForm(f.TokenURL, data)
			if err != nil {
				return nil, err
			}

			var result TokenResponse
			_ = json.NewDecoder(resp.Body).Decode(&result)
			_ = resp.Body.Close()

			if result.Error == "" {
				return &result, nil
			}

			if result.Error != "authorization_pending" && result.Error != "slow_down" {
				return nil, fmt.Errorf("oauth error: %s", result.Error)
			}
		}
	}
}

type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

type TokenStorage interface {
	Save(service string, token *Token) error
	Load(service string) (*Token, error)
}

type TokenManager struct {
	storage      TokenStorage
	tokenURL     string
	clientID     string
	clientSecret string
}

func NewTokenManager(storage TokenStorage, tokenURL, clientID, clientSecret string) *TokenManager {
	return &TokenManager{
		storage:      storage,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (tm *TokenManager) Store(service string, token *Token) error {
	return tm.storage.Save(service, token)
}

func (tm *TokenManager) Get(service string) (*Token, error) {
	token, err := tm.storage.Load(service)
	if err != nil {
		return nil, err
	}

	if time.Now().After(token.ExpiresAt) && token.RefreshToken != "" {
		newToken, err := tm.Refresh(token)
		if err != nil {
			return nil, err
		}
		_ = tm.Store(service, newToken)
		return newToken, nil
	}

	return token, nil
}

func (tm *TokenManager) Refresh(token *Token) (*Token, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Perform refresh with the refresh token
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {token.RefreshToken},
	}

	if tm.clientID != "" {
		data.Set("client_id", tm.clientID)
	}
	if tm.clientSecret != "" {
		data.Set("client_secret", tm.clientSecret)
	}

	resp, err := http.PostForm(tm.tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	var result TokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("token refresh failed: %s", result.Error)
	}

	// Calculate new expiry time
	expiresAt := time.Now()
	if result.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}

	return &Token{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
		ExpiresAt:    expiresAt,
	}, nil
}

type CallbackServer struct {
	port     int
	state    string
	codeChan chan string
	errChan  chan error
}

func NewCallbackServer(port int) *CallbackServer {
	return &CallbackServer{
		port:     port,
		codeChan: make(chan string, 1),
		errChan:  make(chan error, 1),
	}
}

func (s *CallbackServer) Start() string {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	addr := fmt.Sprintf("localhost:%d", s.port)
	go func() { _ = http.ListenAndServe(addr, mux) }()

	return fmt.Sprintf("http://%s/callback", addr)
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if state != s.state {
		s.errChan <- fmt.Errorf("invalid state")
		http.Error(w, "Invalid state", 400)
		return
	}

	s.codeChan <- code
	_, _ = fmt.Fprintln(w, "Authorization successful. You can close this window.")
}

func (s *CallbackServer) WaitCode(ctx context.Context) (string, error) {
	select {
	case code := <-s.codeChan:
		return code, nil
	case err := <-s.errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
