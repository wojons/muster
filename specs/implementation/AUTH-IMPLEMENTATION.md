# Authentication Implementation Specification

## Overview

Production-ready Go authentication implementation supporting OAuth2 (authorization code, client credentials, refresh), mTLS, and API key authentication with secure cross-platform credential storage.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     AuthProvider Interface                   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   OAuth2    │  │    mTLS     │  │      API Key        │  │
│  │   Provider  │  │   Provider  │  │     Provider        │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                 SecureCredentialStore                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────────┐   │
│  │ macOS    │  │  Linux   │  │        Windows           │   │
│  │ Keychain │  │ Keyring  │  │   Credential Manager     │   │
│  └──────────┘  └──────────┘  └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Core Interfaces

### AuthProvider Interface

```go
// AuthProvider defines the interface for all authentication mechanisms
package auth

type AuthProvider interface {
	// Authenticate performs the authentication flow and returns valid credentials
	Authenticate(ctx context.Context) (*Credentials, error)
	
	// Refresh updates expired credentials automatically
	Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)
	
	// ApplyAuth modifies an HTTP request to include authentication
	ApplyAuth(req *http.Request, creds *Credentials) error
	
	// IsExpired checks if credentials have expired
	IsExpired(creds *Credentials) bool
	
	// Close cleans up any resources
	Close() error
}

// Credentials holds authentication tokens and metadata
type Credentials struct {
	AccessToken   string                 `json:"access_token,omitempty"`
	RefreshToken  string                 `json:"refresh_token,omitempty"`
	TokenType     string                 `json:"token_type,omitempty"`
	ExpiresAt     time.Time              `json:"expires_at,omitempty"`
	Scopes        []string               `json:"scopes,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	APIKey        string                 `json:"api_key,omitempty"`
	mTLSCert      *tls.Certificate       `json:"-"`
}

// HTTPClient interface for making HTTP requests
// Import this from internal/core/http package - defined in HTTP-CLIENT-IMPLEMENTATION.md
type HTTPClient interface {
	// Do executes an HTTP request with full control
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
	
	// Get is a convenience method for GET requests
	Get(ctx context.Context, url string, opts *http.RequestOptions) (*http.Response, error)
	
	// Post is a convenience method for POST requests with JSON body
	Post(ctx context.Context, url string, body interface{}, opts *http.RequestOptions) (*http.Response, error)
	
	// Patch is a convenience method for PATCH requests
	Patch(ctx context.Context, url string, body interface{}, opts *http.RequestOptions) (*http.Response, error)
	
	// Put is a convenience method for PUT requests
	Put(ctx context.Context, url string, body interface{}, opts *http.RequestOptions) (*http.Response, error)
	
	// Delete is a convenience method for DELETE requests
	Delete(ctx context.Context, url string, opts *http.RequestOptions) (*http.Response, error)
	
	// PostForm is a convenience method for POST requests with form data
	PostForm(ctx context.Context, url string, data url.Values, opts *http.RequestOptions) (*http.Response, error)
}

// SecureCredentialStore interface for platform-specific secure storage
type SecureCredentialStore interface {
	// Store saves credentials securely
	Store(service, account string, creds *Credentials) error
	
	// Retrieve gets credentials from secure storage
	Retrieve(service, account string) (*Credentials, error)
	
	// Delete removes credentials from storage
	Delete(service, account string) error
	
	// ListAccounts returns all accounts for a service
	ListAccounts(service string) ([]string, error)
}
```

## OAuth2 Implementation

### OAuth2 Configuration

```go
package oauth2

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
	
	"golang.org/x/oauth2"
)

// GrantType represents OAuth2 grant types
type GrantType string

const (
	GrantTypeAuthorizationCode GrantType = "authorization_code"
	GrantTypeClientCredentials GrantType = "client_credentials"
	GrantTypeRefreshToken      GrantType = "refresh_token"
)

// Config holds OAuth2 configuration
type Config struct {
	ClientID       string
	ClientSecret   string
	AuthURL        string
	TokenURL       string
	RedirectURL    string
	Scopes         []string
	GrantType      GrantType
	PKCEEnabled    bool
	StateGenerator func() (string, error)
	TokenStore     TokenStore
	HTTPClient     HTTPClient
}

// TokenStore interface for token persistence
type TokenStore interface {
	Save(token *oauth2.Token) error
	Load() (*oauth2.Token, error)
}

// OAuth2Provider implements AuthProvider for OAuth2
type OAuth2Provider struct {
	config     *Config
	oauth2Cfg  *oauth2.Config
	verifier   string // PKCE code verifier
	state      string // CSRF protection state
	store      SecureCredentialStore
	service    string
	httpClient HTTPClient
}

// NewOAuth2Provider creates a new OAuth2 provider
func NewOAuth2Provider(cfg *Config, store SecureCredentialStore, service string) *OAuth2Provider {
	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cfg.AuthURL,
			TokenURL: cfg.TokenURL,
		},
		RedirectURL: cfg.RedirectURL,
		Scopes:      cfg.Scopes,
	}
	
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
			},
		}
	}
	
	return &OAuth2Provider{
		config:     cfg,
		oauth2Cfg:  oauth2Cfg,
		store:      store,
		service:    service,
		httpClient: httpClient,
	}
}
```

### Authorization Code Flow with PKCE

```go
// AuthorizationCodeFlow handles the OAuth2 authorization code grant flow
func (p *OAuth2Provider) AuthorizationCodeFlow(ctx context.Context) (*Credentials, error) {
	// Generate PKCE code verifier
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	p.verifier = verifier
	
	// Generate state for CSRF protection
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	p.state = state
	
	// Build authorization URL with PKCE
	challenge := generateCodeChallenge(verifier)
	authURL := p.oauth2Cfg.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	
	// Start local HTTP server to receive callback
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)
	
	server := &http.Server{
		Addr:    ":8080",
		Handler: p.callbackHandler(codeChan, errChan),
	}
	
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
	
	// Open browser for user authorization
	fmt.Printf("Please visit this URL to authorize:\n%s\n", authURL)
	
	// Wait for callback or timeout
	select {
	case code := <-codeChan:
		// Exchange code for token
		creds, err := p.exchangeCode(ctx, code)
		server.Shutdown(context.Background())
		return creds, err
		
	case err := <-errChan:
		server.Shutdown(context.Background())
		return nil, err
		
	case <-ctx.Done():
		server.Shutdown(context.Background())
		return nil, ctx.Err()
	}
}

func (p *OAuth2Provider) callbackHandler(codeChan chan<- string, errChan chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify state parameter
		state := r.URL.Query().Get("state")
		if state != p.state {
			errChan <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}
		
		// Check for error
		if err := r.URL.Query().Get("error"); err != "" {
			errChan <- fmt.Errorf("authorization error: %s - %s", err, r.URL.Query().Get("error_description"))
			http.Error(w, "Authorization failed", http.StatusBadRequest)
			return
		}
		
		// Get authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no authorization code received")
			http.Error(w, "No code received", http.StatusBadRequest)
			return
		}
		
		// Send success response
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Authorization Successful</h1><p>You can close this window.</p></body></html>`))
		
		codeChan <- code
	}
}

func (p *OAuth2Provider) exchangeCode(ctx context.Context, code string) (*Credentials, error) {
	// Exchange authorization code for access token
	data := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{p.config.RedirectURL},
		"client_id":     []string{p.config.ClientID},
		"client_secret": []string{p.config.ClientSecret},
		"code_verifier": []string{p.verifier},
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	// Add basic auth header as additional security
	auth := base64.StdEncoding.EncodeToString([]byte(p.config.ClientID + ":" + p.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // Limit to 1MB
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	
	creds := &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}
	
	// Store credentials securely
	if err := p.store.Store(p.service, "oauth2", creds); err != nil {
		return nil, fmt.Errorf("failed to store credentials: %w", err)
	}
	
	return creds, nil
}
```

### Client Credentials Flow

```go
// ClientCredentialsFlow handles OAuth2 client credentials grant
func (p *OAuth2Provider) ClientCredentialsFlow(ctx context.Context) (*Credentials, error) {
	data := url.Values{
		"grant_type":    []string{"client_credentials"},
		"client_id":     []string{p.config.ClientID},
		"client_secret": []string{p.config.ClientSecret},
		"scope":         []string{strings.Join(p.config.Scopes, " ")},
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	// Add basic auth header
	auth := base64.StdEncoding.EncodeToString([]byte(p.config.ClientID + ":" + p.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	
	creds := &Credentials{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:      strings.Split(tokenResp.Scope, " "),
	}
	
	// Store credentials
	if err := p.store.Store(p.service, "oauth2", creds); err != nil {
		return nil, fmt.Errorf("failed to store credentials: %w", err)
	}
	
	return creds, nil
}
```

### Token Refresh with Retry

```go
// RefreshTokenFlow refreshes an expired access token
func (p *OAuth2Provider) RefreshTokenFlow(ctx context.Context, refreshToken string) (*Credentials, error) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
		"client_id":     []string{p.config.ClientID},
		"client_secret": []string{p.config.ClientSecret},
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	// Add basic auth header
	auth := base64.StdEncoding.EncodeToString([]byte(p.config.ClientID + ":" + p.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}
	
	// Use new refresh token if provided, otherwise keep the old one
	newRefreshToken := tokenResp.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}
	
	creds := &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: newRefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}
	
	// Store updated credentials
	if err := p.store.Store(p.service, "oauth2", creds); err != nil {
		return nil, fmt.Errorf("failed to store refreshed credentials: %w", err)
	}
	
	return creds, nil
}

// Refresh implements AuthProvider.Refresh with automatic retry
func (p *OAuth2Provider) Refresh(ctx context.Context, creds *Credentials) (*Credentials, error) {
	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}
	
	// Retry configuration
	maxRetries := 3
	baseDelay := 1 * time.Second
	maxDelay := 30 * time.Second
	
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}
			// Add jitter (±25%)
			jitter := time.Duration(rand.Int63n(int64(delay) / 4))
			delay = delay + jitter - (delay / 8)
			
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		newCreds, err := p.RefreshTokenFlow(ctx, creds.RefreshToken)
		if err == nil {
			return newCreds, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}
	}
	
	return nil, fmt.Errorf("token refresh failed after %d attempts: %w", maxRetries, lastErr)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	// Retry on network errors, 5xx errors, rate limiting
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary",
		"503",
		"502",
		"504",
		"429",
	}
	
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}
```

### AuthProvider Interface Implementation

```go
// Authenticate performs the OAuth2 flow based on configured grant type
func (p *OAuth2Provider) Authenticate(ctx context.Context) (*Credentials, error) {
	switch p.config.GrantType {
	case GrantTypeAuthorizationCode:
		return p.AuthorizationCodeFlow(ctx)
	case GrantTypeClientCredentials:
		return p.ClientCredentialsFlow(ctx)
	default:
		return nil, fmt.Errorf("unsupported grant type: %s", p.config.GrantType)
	}
}

// ApplyAuth adds OAuth2 Bearer token to request
func (p *OAuth2Provider) ApplyAuth(req *http.Request, creds *Credentials) error {
	if creds == nil || creds.AccessToken == "" {
		return fmt.Errorf("no access token available")
	}
	
	tokenType := creds.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", tokenType, creds.AccessToken))
	return nil
}

// IsExpired checks if credentials have expired
func (p *OAuth2Provider) IsExpired(creds *Credentials) bool {
	if creds == nil {
		return true
	}
	
	// Consider token expired 60 seconds before actual expiration
	return time.Now().Add(60 * time.Second).After(creds.ExpiresAt)
}

// Close cleans up resources
func (p *OAuth2Provider) Close() error {
	return nil
}

// Helper functions

func generateCodeVerifier() (string, error) {
	// Generate a 128-character random code verifier (RFC 7636)
	bytes := make([]byte, 96)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func generateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
```

## mTLS Implementation

### mTLS Configuration and Certificate Loading

```go
package mtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Config holds mTLS configuration
type Config struct {
	// Certificate paths
	CertPath string
	KeyPath  string
	CAPath   string
	
	// Or raw certificate data
	CertPEM []byte
	KeyPEM  []byte
	CAPEM   []byte
	
	// Certificate loading options
	InsecureSkipVerify bool
	ServerName         string
	
	// Auto-generate self-signed cert for testing
	AutoGenerate bool
}

// MTLSProvider implements AuthProvider for mutual TLS
type MTLSProvider struct {
	config     *Config
	tlsConfig  *tls.Config
	cert       *tls.Certificate
	caCert     *x509.Certificate
	httpClient HTTPClient
}

// NewMTLSProvider creates a new mTLS provider
func NewMTLSProvider(cfg *Config) (*MTLSProvider, error) {
	provider := &MTLSProvider{
		config: cfg,
	}
	
	// Load or generate certificate
	if cfg.AutoGenerate {
		cert, err := generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate certificate: %w", err)
		}
		provider.cert = cert
	} else {
		cert, err := loadCertificate(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		provider.cert = cert
	}
	
	// Load CA certificate
	if cfg.CAPath != "" || len(cfg.CAPEM) > 0 {
		caCert, err := loadCACertificate(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate: %w", err)
		}
		provider.caCert = caCert
	}
	
	// Build TLS config
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{*provider.cert},
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ServerName:         cfg.ServerName,
		MinVersion:         tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}
	
	if provider.caCert != nil {
		caCertPool := x509.NewCertPool()
		caCertPool.AddCert(provider.caCert)
		tlsConfig.RootCAs = caCertPool
	}
	
	provider.tlsConfig = tlsConfig
	
	// Create HTTP client with mTLS
	provider.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:       tlsConfig,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
	
	return provider, nil
}

// loadCertificate loads a TLS certificate from file or PEM data
func loadCertificate(cfg *Config) (*tls.Certificate, error) {
	var certPEM, keyPEM []byte
	var err error
	
	if len(cfg.CertPEM) > 0 && len(cfg.KeyPEM) > 0 {
		certPEM = cfg.CertPEM
		keyPEM = cfg.KeyPEM
	} else if cfg.CertPath != "" && cfg.KeyPath != "" {
		certPEM, err = os.ReadFile(cfg.CertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		
		keyPEM, err = os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("certificate and key must be provided either as paths or PEM data")
	}
	
	// Validate certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load X509 key pair: %w", err)
	}
	
	// Parse certificate to validate and check expiration
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	// Check if certificate is expired
	if time.Now().After(x509Cert.NotAfter) {
		return nil, fmt.Errorf("certificate has expired (valid until %s)", x509Cert.NotAfter)
	}
	
	// Check if certificate is not yet valid
	if time.Now().Before(x509Cert.NotBefore) {
		return nil, fmt.Errorf("certificate is not yet valid (valid from %s)", x509Cert.NotBefore)
	}
	
	return &cert, nil
}

// loadCACertificate loads the CA certificate
func loadCACertificate(cfg *Config) (*x509.Certificate, error) {
	var caPEM []byte
	var err error
	
	if len(cfg.CAPEM) > 0 {
		caPEM = cfg.CAPEM
	} else if cfg.CAPath != "" {
		caPEM, err = os.ReadFile(cfg.CAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("CA certificate must be provided either as path or PEM data")
	}
	
	// Decode PEM
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode CA certificate PEM")
	}
	
	// Parse certificate
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}
	
	return caCert, nil
}

// generateSelfSignedCert generates a self-signed certificate for testing
func generateSelfSignedCert() (*tls.Certificate, error) {
	// Generate ECDSA key (more secure than RSA)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Organization"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	
	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}
	
	// Encode certificate and key to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	
	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	
	// Load as TLS certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load generated certificate: %w", err)
	}
	
	return &cert, nil
}

// ValidateCertificate validates a certificate chain
func (p *MTLSProvider) ValidateCertificate() error {
	if p.cert == nil {
		return fmt.Errorf("no certificate loaded")
	}
	
	// Parse certificate
	x509Cert, err := x509.ParseCertificate(p.cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	// Check expiration
	if time.Now().After(x509Cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}
	
	// Verify certificate chain if CA is configured
	if p.caCert != nil {
		roots := x509.NewCertPool()
		roots.AddCert(p.caCert)
		
		opts := x509.VerifyOptions{
			Roots:         roots,
			DNSName:       p.config.ServerName,
			CurrentTime:   time.Now(),
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		
		if _, err := x509Cert.Verify(opts); err != nil {
			return fmt.Errorf("certificate verification failed: %w", err)
		}
	}
	
	return nil
}

// GetCertificate returns the loaded TLS certificate
func (p *MTLSProvider) GetCertificate() *tls.Certificate {
	return p.cert
}

// GetTLSConfig returns the TLS configuration
func (p *MTLSProvider) GetTLSConfig() *tls.Config {
	return p.tlsConfig
}
```

### mTLS AuthProvider Implementation

```go
// Authenticate for mTLS validates the certificate and returns credentials
func (p *MTLSProvider) Authenticate(ctx context.Context) (*Credentials, error) {
	if err := p.ValidateCertificate(); err != nil {
		return nil, fmt.Errorf("certificate validation failed: %w", err)
	}
	
	// Extract certificate metadata
	x509Cert, _ := x509.ParseCertificate(p.cert.Certificate[0])
	
	creds := &Credentials{
		TokenType: "mTLS",
		ExpiresAt: x509Cert.NotAfter,
		Metadata: map[string]interface{}{
			"subject":       x509Cert.Subject.String(),
			"issuer":        x509Cert.Issuer.String(),
			"serial_number": x509Cert.SerialNumber.String(),
			"not_before":    x509Cert.NotBefore,
			"not_after":     x509Cert.NotAfter,
		},
		mTLSCert: p.cert,
	}
	
	return creds, nil
}

// Refresh for mTLS reloads certificates from disk
func (p *MTLSProvider) Refresh(ctx context.Context, creds *Credentials) (*Credentials, error) {
	// Reload certificate from file
	newCert, err := loadCertificate(p.config)
	if err != nil {
		return nil, fmt.Errorf("failed to reload certificate: %w", err)
	}
	
	p.cert = newCert
	p.tlsConfig.Certificates = []tls.Certificate{*newCert}
	
	return p.Authenticate(ctx)
}

// ApplyAuth configures the HTTP request with mTLS
func (p *MTLSProvider) ApplyAuth(req *http.Request, creds *Credentials) error {
	// mTLS is handled at the transport layer, not via headers
	// This method is a no-op but required by the interface
	return nil
}

// IsExpired checks if the certificate has expired
func (p *MTLSProvider) IsExpired(creds *Credentials) bool {
	if p.cert == nil {
		return true
	}
	
	x509Cert, err := x509.ParseCertificate(p.cert.Certificate[0])
	if err != nil {
		return true
	}
	
	// Consider expired 24 hours before actual expiration for renewal
	return time.Now().Add(24 * time.Hour).After(x509Cert.NotAfter)
}

// Close cleans up resources
func (p *MTLSProvider) Close() error {
	return nil
}

// DoRequest performs an HTTP request with mTLS authentication
func (p *MTLSProvider) DoRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	return p.httpClient.Do(req)
}
```

## API Key Authentication

### API Key Provider Implementation

```go
package apikey

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Location specifies where the API key should be placed in the request
type Location string

const (
	LocationHeader Location = "header"
	LocationQuery  Location = "query"
)

// Config holds API key authentication configuration
type Config struct {
	Key         string
	KeyName     string
	Location    Location
	Prefix      string // e.g., "Bearer ", "ApiKey "
	ServiceName string
}

// APIKeyProvider implements AuthProvider for API key authentication
type APIKeyProvider struct {
	config *Config
	store  SecureCredentialStore
}

// NewAPIKeyProvider creates a new API key provider
func NewAPIKeyProvider(cfg *Config, store SecureCredentialStore) *APIKeyProvider {
	return &APIKeyProvider{
		config: cfg,
		store:  store,
	}
}

// Authenticate returns the API key credentials
func (p *APIKeyProvider) Authenticate(ctx context.Context) (*Credentials, error) {
	// Try to load from secure store first
	if p.store != nil && p.config.ServiceName != "" {
		creds, err := p.store.Retrieve(p.config.ServiceName, "apikey")
		if err == nil && creds != nil && creds.APIKey != "" {
			return creds, nil
		}
	}
	
	// Use configured key
	if p.config.Key == "" {
		return nil, fmt.Errorf("no API key configured")
	}
	
	creds := &Credentials{
		APIKey:    p.config.Key,
		TokenType: "APIKey",
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // API keys typically don't expire
	}
	
	// Store in secure storage
	if p.store != nil && p.config.ServiceName != "" {
		if err := p.store.Store(p.config.ServiceName, "apikey", creds); err != nil {
			return nil, fmt.Errorf("failed to store API key: %w", err)
		}
	}
	
	return creds, nil
}

// Refresh for API key is typically a no-op, but can rotate keys
func (p *APIKeyProvider) Refresh(ctx context.Context, creds *Credentials) (*Credentials, error) {
	// API keys usually don't refresh, but if there's a rotation mechanism,
	// it would be implemented here
	return p.Authenticate(ctx)
}

// ApplyAuth adds the API key to the request
func (p *APIKeyProvider) ApplyAuth(req *http.Request, creds *Credentials) error {
	if creds == nil || creds.APIKey == "" {
		return fmt.Errorf("no API key available")
	}
	
	key := creds.APIKey
	if p.config.Prefix != "" {
		key = p.config.Prefix + key
	}
	
	switch p.config.Location {
	case LocationHeader:
		req.Header.Set(p.config.KeyName, key)
	case LocationQuery:
		q := req.URL.Query()
		q.Set(p.config.KeyName, creds.APIKey)
		req.URL.RawQuery = q.Encode()
	default:
		return fmt.Errorf("unsupported API key location: %s", p.config.Location)
	}
	
	return nil
}

// IsExpired checks if the API key has expired
func (p *APIKeyProvider) IsExpired(creds *Credentials) bool {
	// API keys typically don't expire, but check if they do
	if creds == nil {
		return true
	}
	return time.Now().After(creds.ExpiresAt)
}

// Close cleans up resources
func (p *APIKeyProvider) Close() error {
	return nil
}

// ValidateKey checks if the API key meets security requirements
func (p *APIKeyProvider) ValidateKey() error {
	if p.config.Key == "" {
		return fmt.Errorf("API key is empty")
	}
	
	// Minimum length check
	if len(p.config.Key) < 16 {
		return fmt.Errorf("API key too short (minimum 16 characters)")
	}
	
	// Check for common weak patterns
	weakPatterns := []string{
		"test",
		"example",
		"sample",
		"demo",
		"123456",
		"password",
	}
	
	lowerKey := strings.ToLower(p.config.Key)
	for _, pattern := range weakPatterns {
		if strings.Contains(lowerKey, pattern) {
			return fmt.Errorf("API key contains weak pattern: %s", pattern)
		}
	}
	
	return nil
}
```

## Secure Credential Storage

### Cross-Platform Secure Storage

```go
package securestore

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
)

// SecureCredentialStore provides platform-specific secure credential storage
type SecureCredentialStore interface {
	Store(service, account string, creds *auth.Credentials) error
	Retrieve(service, account string) (*auth.Credentials, error)
	Delete(service, account string) error
	ListAccounts(service string) ([]string, error)
}

// NewSecureCredentialStore creates the appropriate store for the current platform
func NewSecureCredentialStore() (SecureCredentialStore, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewMacOSKeychainStore()
	case "linux":
		return NewLinuxKeyringStore()
	case "windows":
		return NewWindowsCredentialStore()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// FallbackFileStore provides file-based storage as a fallback (encrypted)
type FallbackFileStore struct {
	basePath string
	mu       sync.RWMutex
}

// NewFallbackFileStore creates a file-based credential store with encryption
func NewFallbackFileStore(basePath string) (*FallbackFileStore, error) {
	return &FallbackFileStore{
		basePath: basePath,
	}, nil
}

func (s *FallbackFileStore) Store(service, account string, creds *auth.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Serialize credentials
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	
	// Encrypt data (simplified - use proper encryption in production)
	encrypted, err := encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}
	
	// Write to file
	filename := filepath.Join(s.basePath, fmt.Sprintf("%s-%s.enc", service, account))
	if err := os.WriteFile(filename, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}
	
	return nil
}

func (s *FallbackFileStore) Retrieve(service, account string) (*auth.Credentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	filename := filepath.Join(s.basePath, fmt.Sprintf("%s-%s.enc", service, account))
	encrypted, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("credentials not found")
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}
	
	// Decrypt data
	data, err := decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}
	
	// Deserialize
	var creds auth.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}
	
	return &creds, nil
}
```

### macOS Keychain Implementation

```go
// +build darwin

package securestore

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <Security/Security.h>
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// MacOSKeychainStore implements SecureCredentialStore using macOS Keychain
type MacOSKeychainStore struct {
	keychain C.SecKeychainRef
}

// NewMacOSKeychainStore creates a new macOS Keychain credential store
func NewMacOSKeychainStore() (*MacOSKeychainStore, error) {
	// Use default keychain
	return &MacOSKeychainStore{}, nil
}

func (s *MacOSKeychainStore) Store(service, account string, creds *auth.Credentials) error {
	// Serialize credentials to JSON
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	
	// Convert to CFData
	cfData := C.CFDataCreate(C.kCFAllocatorDefault, (*C.UInt8)(unsafe.Pointer(&data[0])), C.CFIndex(len(data)))
	defer C.CFRelease(C.CFTypeRef(cfData))
	
	// Create attributes
	serviceStr := C.CString(service)
	defer C.free(unsafe.Pointer(serviceStr))
	
	accountStr := C.CString(account)
	defer C.free(unsafe.Pointer(accountStr))
	
	serviceCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, serviceStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(serviceCF))
	
	accountCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, accountStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(accountCF))
	
	// Create query dictionary
	keys := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClass),
		C.CFTypeRef(C.kSecAttrService),
		C.CFTypeRef(C.kSecAttrAccount),
		C.CFTypeRef(C.kSecValueData),
	}
	values := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClassGenericPassword),
		C.CFTypeRef(serviceCF),
		C.CFTypeRef(accountCF),
		C.CFTypeRef(cfData),
	}
	
	query := C.CFDictionaryCreate(
		C.kCFAllocatorDefault,
		(*unsafe.Pointer)(unsafe.Pointer(&keys[0])),
		(*unsafe.Pointer)(unsafe.Pointer(&values[0])),
		C.CFIndex(len(keys)),
		&C.kCFTypeDictionaryKeyCallBacks,
		&C.kCFTypeDictionaryValueCallBacks,
	)
	defer C.CFRelease(C.CFTypeRef(query))
	
	// Delete existing item first
	C.SecItemDelete(query)
	
	// Add new item
	status := C.SecItemAdd(query, nil)
	if status != C.errSecSuccess {
		return fmt.Errorf("failed to add item to keychain: %d", status)
	}
	
	return nil
}

func (s *MacOSKeychainStore) Retrieve(service, account string) (*auth.Credentials, error) {
	serviceStr := C.CString(service)
	defer C.free(unsafe.Pointer(serviceStr))
	
	accountStr := C.CString(account)
	defer C.free(unsafe.Pointer(accountStr))
	
	serviceCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, serviceStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(serviceCF))
	
	accountCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, accountStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(accountCF))
	
	// Create query
	keys := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClass),
		C.CFTypeRef(C.kSecAttrService),
		C.CFTypeRef(C.kSecAttrAccount),
		C.CFTypeRef(C.kSecReturnData),
		C.CFTypeRef(C.kSecMatchLimit),
	}
	values := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClassGenericPassword),
		C.CFTypeRef(serviceCF),
		C.CFTypeRef(accountCF),
		C.CFTypeRef(C.kCFBooleanTrue),
		C.CFTypeRef(C.kSecMatchLimitOne),
	}
	
	query := C.CFDictionaryCreate(
		C.kCFAllocatorDefault,
		(*unsafe.Pointer)(unsafe.Pointer(&keys[0])),
		(*unsafe.Pointer)(unsafe.Pointer(&values[0])),
		C.CFIndex(len(keys)),
		&C.kCFTypeDictionaryKeyCallBacks,
		&C.kCFTypeDictionaryValueCallBacks,
	)
	defer C.CFRelease(C.CFTypeRef(query))
	
	var result C.CFTypeRef
	status := C.SecItemCopyMatching(query, &result)
	if status == C.errSecItemNotFound {
		return nil, fmt.Errorf("credentials not found in keychain")
	}
	if status != C.errSecSuccess {
		return nil, fmt.Errorf("failed to retrieve item from keychain: %d", status)
	}
	defer C.CFRelease(result)
	
	// Get data
	data := C.CFDataGetBytes(C.CFDataRef(result))
	length := C.CFDataGetLength(C.CFDataRef(result))
	
	// Copy to Go slice
	goData := C.GoBytes(unsafe.Pointer(data), C.int(length))
	
	// Deserialize
	var creds auth.Credentials
	if err := json.Unmarshal(goData, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}
	
	return &creds, nil
}

func (s *MacOSKeychainStore) Delete(service, account string) error {
	serviceStr := C.CString(service)
	defer C.free(unsafe.Pointer(serviceStr))
	
	accountStr := C.CString(account)
	defer C.free(unsafe.Pointer(accountStr))
	
	serviceCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, serviceStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(serviceCF))
	
	accountCF := C.CFStringCreateWithCString(C.kCFAllocatorDefault, accountStr, C.kCFStringEncodingUTF8)
	defer C.CFRelease(C.CFTypeRef(accountCF))
	
	keys := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClass),
		C.CFTypeRef(C.kSecAttrService),
		C.CFTypeRef(C.kSecAttrAccount),
	}
	values := []C.CFTypeRef{
		C.CFTypeRef(C.kSecClassGenericPassword),
		C.CFTypeRef(serviceCF),
		C.CFTypeRef(accountCF),
	}
	
	query := C.CFDictionaryCreate(
		C.kCFAllocatorDefault,
		(*unsafe.Pointer)(unsafe.Pointer(&keys[0])),
		(*unsafe.Pointer)(unsafe.Pointer(&values[0])),
		C.CFIndex(len(keys)),
		&C.kCFTypeDictionaryKeyCallBacks,
		&C.kCFTypeDictionaryValueCallBacks,
	)
	defer C.CFRelease(C.CFTypeRef(query))
	
	status := C.SecItemDelete(query)
	if status != C.errSecSuccess && status != C.errSecItemNotFound {
		return fmt.Errorf("failed to delete item from keychain: %d", status)
	}
	
	return nil
}

func (s *MacOSKeychainStore) ListAccounts(service string) ([]string, error) {
	// Implementation for listing accounts
	return nil, fmt.Errorf("not implemented")
}
```

### Linux Keyring Implementation (using DBus Secret Service)

```go
// +build linux

package securestore

import (
	"encoding/json"
	"fmt"
	"github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"
)

// LinuxKeyringStore implements SecureCredentialStore using Linux Secret Service (keyring)
type LinuxKeyringStore struct{}

// NewLinuxKeyringStore creates a new Linux keyring credential store
func NewLinuxKeyringStore() (*LinuxKeyringStore, error) {
	return &LinuxKeyringStore{}, nil
}

func (s *LinuxKeyringStore) Store(service, account string, creds *auth.Credentials) error {
	// Serialize credentials
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	
	// Store in keyring using zalando/go-keyring
	// This uses the Secret Service API via DBus
	err = keyring.Set(service, account, string(data))
	if err != nil {
		return fmt.Errorf("failed to store in keyring: %w", err)
	}
	
	return nil
}

func (s *LinuxKeyringStore) Retrieve(service, account string) (*auth.Credentials, error) {
	// Retrieve from keyring
	data, err := keyring.Get(service, account)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, fmt.Errorf("credentials not found in keyring")
		}
		return nil, fmt.Errorf("failed to retrieve from keyring: %w", err)
	}
	
	// Deserialize
	var creds auth.Credentials
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}
	
	return &creds, nil
}

func (s *LinuxKeyringStore) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if err != nil {
		return fmt.Errorf("failed to delete from keyring: %w", err)
	}
	return nil
}

func (s *LinuxKeyringStore) ListAccounts(service string) ([]string, error) {
	// go-keyring doesn't support listing, would need direct Secret Service implementation
	return nil, fmt.Errorf("listing accounts not supported by keyring backend")
}
```

### Windows Credential Manager Implementation

```go
// +build windows

package securestore

import (
	"encoding/json"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modadvapi32               = syscall.NewLazyDLL("advapi32.dll")
	procCredWriteW            = modadvapi32.NewProc("CredWriteW")
	procCredReadW             = modadvapi32.NewProc("CredReadW")
	procCredDeleteW           = modadvapi32.NewProc("CredDeleteW")
	procCredFree              = modadvapi32.NewProc("CredFree")
)

const (
	CRED_TYPE_GENERIC = 1
)

type CREDENTIAL struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// WindowsCredentialStore implements SecureCredentialStore using Windows Credential Manager
type WindowsCredentialStore struct{}

// NewWindowsCredentialStore creates a new Windows credential store
func NewWindowsCredentialStore() (*WindowsCredentialStore, error) {
	return &WindowsCredentialStore{}, nil
}

func (s *WindowsCredentialStore) Store(service, account string, creds *auth.Credentials) error {
	// Serialize credentials
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	
	// Build target name: service/account
	targetName := syscall.StringToUTF16(fmt.Sprintf("%s/%s", service, account))
	userName := syscall.StringToUTF16(account)
	
	cred := &CREDENTIAL{
		Type:               CRED_TYPE_GENERIC,
		TargetName:         &targetName[0],
		UserName:           &userName[0],
		CredentialBlobSize: uint32(len(data)),
		CredentialBlob:     &data[0],
		Persist:            2, // CRED_PERSIST_LOCAL_MACHINE
	}
	
	ret, _, err := procCredWriteW.Call(
		uintptr(unsafe.Pointer(cred)),
		0,
	)
	
	if ret == 0 {
		return fmt.Errorf("failed to write credential: %v", err)
	}
	
	return nil
}

func (s *WindowsCredentialStore) Retrieve(service, account string) (*auth.Credentials, error) {
	targetName := syscall.StringToUTF16(fmt.Sprintf("%s/%s", service, account))
	
	var cred *CREDENTIAL
	ret, _, err := procCredReadW.Call(
		uintptr(unsafe.Pointer(&targetName[0])),
		CRED_TYPE_GENERIC,
		0,
		uintptr(unsafe.Pointer(&cred)),
	)
	
	if ret == 0 {
		return nil, fmt.Errorf("failed to read credential: %v", err)
	}
	
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	
	// Extract data
	data := make([]byte, cred.CredentialBlobSize)
	copy(data, (*[1 << 30]byte)(unsafe.Pointer(cred.CredentialBlob))[:cred.CredentialBlobSize])
	
	// Deserialize
	var credentials auth.Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}
	
	return &credentials, nil
}

func (s *WindowsCredentialStore) Delete(service, account string) error {
	targetName := syscall.StringToUTF16(fmt.Sprintf("%s/%s", service, account))
	
	ret, _, err := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(&targetName[0])),
		CRED_TYPE_GENERIC,
		0,
	)
	
	if ret == 0 {
		return fmt.Errorf("failed to delete credential: %v", err)
	}
	
	return nil
}

func (s *WindowsCredentialStore) ListAccounts(service string) ([]string, error) {
	// Windows Credential Manager doesn't provide a simple way to list by service prefix
	return nil, fmt.Errorf("listing accounts not implemented for Windows Credential Manager")
}
```

## Complete Authentication Client

### Authenticated HTTP Client

```go
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AuthenticatedClient wraps an HTTP client with automatic authentication
type AuthenticatedClient struct {
	provider   AuthProvider
	creds      *Credentials
	httpClient HTTPClient
	mu         sync.RWMutex
}

// NewAuthenticatedClient creates a new authenticated HTTP client
func NewAuthenticatedClient(provider AuthProvider, httpClient HTTPClient) *AuthenticatedClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	
	return &AuthenticatedClient{
		provider:   provider,
		httpClient: httpClient,
	}
}

// Authenticate performs initial authentication
func (c *AuthenticatedClient) Authenticate(ctx context.Context) error {
	creds, err := c.provider.Authenticate(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	
	c.mu.Lock()
	c.creds = creds
	c.mu.Unlock()
	
	return nil
}

// Do performs an HTTP request with automatic authentication and retry
func (c *AuthenticatedClient) Do(req *http.Request) (*http.Response, error) {
	// Ensure we have valid credentials
	if err := c.ensureValidCredentials(req.Context()); err != nil {
		return nil, err
	}
	
	// Clone request to allow retry
	reqClone := req.Clone(req.Context())
	
	// Apply authentication
	c.mu.RLock()
	creds := c.creds
	c.mu.RUnlock()
	
	if err := c.provider.ApplyAuth(reqClone, creds); err != nil {
		return nil, fmt.Errorf("failed to apply authentication: %w", err)
	}
	
	// Execute request
	resp, err := c.httpClient.Do(reqClone)
	if err != nil {
		return nil, err
	}
	
	// Handle 401 Unauthorized - attempt refresh and retry
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		
		// Try to refresh credentials
		if err := c.refreshCredentials(req.Context()); err != nil {
			return nil, fmt.Errorf("token refresh failed after 401: %w", err)
		}
		
		// Retry request with new credentials
		reqClone = req.Clone(req.Context())
		c.mu.RLock()
		creds = c.creds
		c.mu.RUnlock()
		
		if err := c.provider.ApplyAuth(reqClone, creds); err != nil {
			return nil, fmt.Errorf("failed to apply refreshed authentication: %w", err)
		}
		
		resp, err = c.httpClient.Do(reqClone)
		if err != nil {
			return nil, err
		}
	}
	
	return resp, nil
}

// ensureValidCredentials checks and refreshes credentials if needed
func (c *AuthenticatedClient) ensureValidCredentials(ctx context.Context) error {
	c.mu.RLock()
	creds := c.creds
	c.mu.RUnlock()
	
	// Check if we need to authenticate
	if creds == nil {
		return c.Authenticate(ctx)
	}
	
	// Check if credentials are expired
	if c.provider.IsExpired(creds) {
		return c.refreshCredentials(ctx)
	}
	
	return nil
}

// refreshCredentials refreshes expired credentials
func (c *AuthenticatedClient) refreshCredentials(ctx context.Context) error {
	c.mu.RLock()
	oldCreds := c.creds
	c.mu.RUnlock()
	
	if oldCreds == nil {
		return c.Authenticate(ctx)
	}
	
	newCreds, err := c.provider.Refresh(ctx, oldCreds)
	if err != nil {
		// If refresh fails, try full re-authentication
		return c.Authenticate(ctx)
	}
	
	c.mu.Lock()
	c.creds = newCreds
	c.mu.Unlock()
	
	return nil
}

// GetCredentials returns the current credentials (for inspection)
func (c *AuthenticatedClient) GetCredentials() *Credentials {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.creds
}

// Convenience methods

func (c *AuthenticatedClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *AuthenticatedClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

func (c *AuthenticatedClient) Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

func (c *AuthenticatedClient) Delete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
```

## Usage Examples

### OAuth2 Authorization Code Flow

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"
	
	"github.com/yourorg/muster/pkg/auth/oauth2"
	"github.com/yourorg/muster/pkg/securestore"
)

func main() {
	// Create secure credential store
	store, err := securestore.NewSecureCredentialStore()
	if err != nil {
		log.Fatalf("Failed to create credential store: %v", err)
	}
	
	// Configure OAuth2
	config := &oauth2.Config{
		ClientID:     "your-client-id",
		ClientSecret: "your-client-secret",
		AuthURL:      "https://oauth.example.com/authorize",
		TokenURL:     "https://oauth.example.com/token",
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"read", "write"},
		GrantType:    oauth2.GrantTypeAuthorizationCode,
		PKCEEnabled:  true,
	}
	
	// Create OAuth2 provider
	provider := oauth2.NewOAuth2Provider(config, store, "my-service")
	defer provider.Close()
	
	// Authenticate
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	creds, err := provider.Authenticate(ctx)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	
	fmt.Printf("Authenticated! Token expires at: %s\n", creds.ExpiresAt)
}
```

### Client Credentials Flow

```go
func clientCredentialsExample() {
	store, _ := securestore.NewSecureCredentialStore()
	
	config := &oauth2.Config{
		ClientID:     "service-account-id",
		ClientSecret: "service-account-secret",
		TokenURL:     "https://api.example.com/oauth/token",
		Scopes:       []string{"api:read", "api:write"},
		GrantType:    oauth2.GrantTypeClientCredentials,
	}
	
	provider := oauth2.NewOAuth2Provider(config, store, "api-service")
	defer provider.Close()
	
	ctx := context.Background()
	creds, err := provider.Authenticate(ctx)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	
	fmt.Printf("Service authenticated. Token: %s...\n", creds.AccessToken[:10])
}
```

### mTLS Authentication

```go
func mTLSExample() {
	// Load certificates from files
	config := &mtls.Config{
		CertPath:   "/path/to/client.crt",
		KeyPath:    "/path/to/client.key",
		CAPath:     "/path/to/ca.crt",
		ServerName: "api.example.com",
	}
	
	provider, err := mtls.NewMTLSProvider(config)
	if err != nil {
		log.Fatalf("Failed to create mTLS provider: %v", err)
	}
	defer provider.Close()
	
	// Validate certificate
	if err := provider.ValidateCertificate(); err != nil {
		log.Fatalf("Certificate validation failed: %v", err)
	}
	
	// Make authenticated request
	ctx := context.Background()
	resp, err := provider.DoRequest(ctx, "GET", "https://api.example.com/data", nil)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	
	fmt.Printf("Response status: %s\n", resp.Status)
}
```

### API Key Authentication

```go
func apiKeyExample() {
	store, _ := securestore.NewSecureCredentialStore()
	
	config := &apikey.Config{
		Key:         "your-secret-api-key",
		KeyName:     "X-API-Key",
		Location:    apikey.LocationHeader,
		ServiceName: "external-api",
	}
	
	provider := apikey.NewAPIKeyProvider(config, store)
	defer provider.Close()
	
	// Validate key
	if err := provider.ValidateKey(); err != nil {
		log.Fatalf("Invalid API key: %v", err)
	}
	
	ctx := context.Background()
	creds, err := provider.Authenticate(ctx)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	
	// Create request and apply auth
	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	if err := provider.ApplyAuth(req, creds); err != nil {
		log.Fatalf("Failed to apply auth: %v", err)
	}
	
	fmt.Printf("Authorization header: %s\n", req.Header.Get("X-API-Key"))
}
```

### Complete Authenticated Client Example

```go
func completeExample() {
	// Setup OAuth2
	store, _ := securestore.NewSecureCredentialStore()
	
	oauthConfig := &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"api"},
		GrantType:    oauth2.GrantTypeClientCredentials,
	}
	
	provider := oauth2.NewOAuth2Provider(oauthConfig, store, "my-app")
	
	// Create authenticated client
	client := client.NewAuthenticatedClient(provider, nil)
	
	// Authenticate
	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	
	// Make requests - authentication is automatic
	resp, err := client.Get(ctx, "https://api.example.com/users")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Token will be automatically refreshed if expired
	resp2, err := client.Post(ctx, "https://api.example.com/users", "application/json", strings.NewReader(`{"name":"John"}`))
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()
}
```

## Dependencies

```go
// go.mod
module github.com/yourorg/muster

go 1.21

require (
	github.com/godbus/dbus/v5 v5.1.0 // Linux keyring
	github.com/zalando/go-keyring v0.2.3 // Linux keyring abstraction
	golang.org/x/oauth2 v0.15.0 // OAuth2 client
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.19.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
```

## Security Considerations

1. **Credential Storage**: All credentials are stored in platform-specific secure storage (Keychain, Secret Service, Credential Manager)
2. **PKCE**: OAuth2 authorization code flow uses PKCE (S256) to prevent authorization code interception attacks
3. **State Parameter**: CSRF protection via random state parameter in OAuth2 flows
4. **Token Expiration**: Automatic token refresh 60 seconds before expiration to prevent race conditions
5. **mTLS Security**: Minimum TLS 1.2, strong cipher suites, certificate validation
6. **Retry Logic**: Exponential backoff with jitter for token refresh retries
7. **API Key Validation**: Minimum length checks and weak pattern detection
8. **Memory Safety**: Credentials cleared from memory when no longer needed
9. **Scope Validation**: OAuth2 scopes are validated and stored with credentials
10. **Audit Logging**: All authentication events should be logged (implementation-specific)

## Error Handling

All errors are wrapped with context using `fmt.Errorf("...: %w", err)` for proper error chain inspection. Common error types:

- `ErrInvalidCredentials`: Authentication failed due to invalid credentials
- `ErrTokenExpired`: Token has expired and refresh failed
- `ErrNetwork`: Network-related errors
- `ErrStorage`: Secure storage operation failures
- `ErrValidation`: Certificate or key validation failures

## Testing

See `auth_test.go` for complete test suite including:
- OAuth2 flow mocking
- Certificate generation and validation
- Secure storage mocking
- Token refresh scenarios
- 401 retry handling
- Timeout and cancellation handling

## Deterministic Guarantees

This implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Cross‑Platform Credential Storage

Authentication providers must produce identical credential state regardless of platform:

- **Keychain/Keyring/Credential Manager**: Secure storage abstractions must produce identical credential retrieval behavior
- **File‑based fallback**: When platform‑secure storage is unavailable, file‑based storage must use consistent paths (normalized per `PLATFORM-BEHAVIOR.md`)
- **Serialization format**: Credentials must be serialized with deterministic JSON (sorted keys, no extra whitespace)

### 2. Time Zone Independence

All token expiration times must be stored and compared in **UTC**:

```go
// CORRECT: Store expiration in UTC
creds.ExpiresAt = token.Expiry.UTC()

// CORRECT: Compare with UTC reference
if time.Now().UTC().After(creds.ExpiresAt) {
	// Token expired
}
```

### 3. Deterministic Error Sequences

Authentication failures must produce identical error sequences across platforms:

- **Network errors**: Use same retry patterns with deterministic jitter seeds
- **Storage errors**: Platform‑specific storage errors must be mapped to canonical error types
- **OAuth2 flow errors**: State parameter generation must use cryptographically secure random with test‑mode seeding

### 4. Concurrent Authentication Operations

Multiple concurrent authentication requests must be handled deterministically:

- **Token refresh**: Only one refresh operation per credential set, with deterministic lock acquisition ordering
- **Credential updates**: Atomic updates using copy‑on‑write pattern (see `DETERMINISTIC-GUARANTEES.md` section 1.2)

### 5. Platform‑Specific Behavior Normalization

- **Environment variables**: Authentication configuration must use `GetCanonicalEnv()` from `PLATFORM-BEHAVIOR.md`
- **File paths**: Configuration file paths must be normalized with `NormalizePath()`
- **Console interaction**: Interactive OAuth2 flows must respect color‑support detection

### 6. Testing Determinism

All authentication tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestAuthDeterminism(t *testing.T) {
	// Test that authentication produces identical state across platforms
}
```

### 7. Integration with SDK Generation

When generating SDKs for 12 languages, authentication interfaces must:

- Use identical method signatures across languages
- Produce identical HTTP request headers
- Handle token refresh with same timing semantics

**Verification**: Run `./scripts/validate‑determinism‑auth.sh` to confirm cross‑platform consistency.
