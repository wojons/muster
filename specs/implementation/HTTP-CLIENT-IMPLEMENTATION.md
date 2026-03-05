# HTTP Client Implementation Specification

Production-ready Go HTTP client implementation with middleware system, authentication integration, rate limiting, circuit breaker support, and comprehensive error handling.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         HTTP Client                              │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Get()     │  │   Post()    │  │   Do()      │              │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘              │
│         └──────────────────┴──────────────────┘                  │
│                          │                                      │
│                          ▼                                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Middleware Chain (RoundTripper)             │   │
│  │  ┌─────────┐ → ┌───────────┐ → ┌─────────────┐ → ┌────┐ │   │
│  │  │ Logging │   │ RateLimit │   │ CircuitBrk  │   │Base│ │   │
│  │  └─────────┘   └───────────┘   └─────────────┘   └────┘ │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Base HTTP Transport                  │   │
│  │  - Connection pooling                                  │   │
│  │  - TLS configuration                                   │   │
│  │  - Timeout handling                                    │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 1. Core Interfaces

### File: `internal/core/http/client.go`

```go
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is the main HTTP client interface
type Client interface {
	// Do executes an HTTP request with full control
	Do(ctx context.Context, req *Request) (*Response, error)

	// Get is a convenience method for GET requests
	Get(ctx context.Context, url string, opts *RequestOptions) (*Response, error)

	// Post is a convenience method for POST requests with JSON body
	Post(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)

	// PostForm is a convenience method for POST requests with form data
	PostForm(ctx context.Context, url string, data url.Values, opts *RequestOptions) (*Response, error)

	// Patch is a convenience method for PATCH requests
	Patch(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)

	// Put is a convenience method for PUT requests
	Put(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error)

	// Delete is a convenience method for DELETE requests
	Delete(ctx context.Context, url string, opts *RequestOptions) (*Response, error)

	// SetAuthProvider sets the authentication provider
	SetAuthProvider(provider AuthProvider)

	// SetRateLimiter sets the rate limiter
	SetRateLimiter(limiter RateLimiter)

	// SetCircuitBreaker sets the circuit breaker
	SetCircuitBreaker(breaker CircuitBreaker)

	// AddMiddleware adds a middleware to the chain
	AddMiddleware(middleware Middleware)

	// SetBaseURL sets the base URL for all requests
	SetBaseURL(baseURL string) error

	// SetDefaultHeaders sets default headers for all requests
	SetDefaultHeaders(headers map[string]string)

	// Close releases all resources
	Close() error
}

// Request represents an HTTP request
type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body,omitempty"`

	// Internal fields
	ctx        context.Context
	skipAuth   bool
	skipRetry  bool
	skipRateLimit bool
	attempt    int
}

// Response represents an HTTP response
type Response struct {
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`

	// Metadata
	Duration      time.Duration `json:"duration"`
	RetryCount    int           `json:"retry_count"`
	RequestURL    string        `json:"request_url"`
	RequestMethod string        `json:"request_method"`
}

// RequestOptions configures a single request
type RequestOptions struct {
	Headers       map[string]string
	Query         url.Values
	Timeout       time.Duration
	SkipAuth      bool
	SkipRetry     bool
	SkipRateLimit bool
	ContentType   string
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
	URL        string
	Method     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s %s", e.StatusCode, e.Status, e.Method, e.URL)
}

// IsHTTPError checks if an error is an HTTPError
func IsHTTPError(err error) (*HTTPError, bool) {
	if err == nil {
		return nil, false
	}
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr, true
	}
	return nil, false
}

// IsRetryableError checks if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an HTTP error with retryable status code
	if httpErr, ok := IsHTTPError(err); ok {
		switch httpErr.StatusCode {
		case 408, 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}

	// Check for network errors
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"no such host",
		"temporary",
		"network is unreachable",
		"TLS handshake timeout",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

---

## 2. Authentication Integration

### File: `internal/core/http/auth.go`

```go
package http

import (
	"context"
	"net/http"
	"time"
)

// AuthProvider defines the interface for authentication mechanisms
// This matches the interface from AUTH-IMPLEMENTATION.md
type AuthProvider interface {
	// Authenticate performs the authentication flow
	Authenticate(ctx context.Context) (*Credentials, error)

	// Refresh updates expired credentials
	Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)

	// ApplyAuth modifies an HTTP request to include authentication
	ApplyAuth(req *http.Request, creds *Credentials) error

	// IsExpired checks if credentials have expired
	IsExpired(creds *Credentials) bool

	// Close cleans up resources
	Close() error
}

// Credentials holds authentication tokens
type Credentials struct {
	AccessToken   string                 `json:"access_token,omitempty"`
	RefreshToken  string                 `json:"refresh_token,omitempty"`
	TokenType     string                 `json:"token_type,omitempty"`
	ExpiresAt     time.Time              `json:"expires_at,omitempty"`
	Scopes        []string               `json:"scopes,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	APIKey        string                 `json:"api_key,omitempty"`
}

// authManager wraps AuthProvider with automatic refresh logic
type authManager struct {
	provider AuthProvider
	creds    *Credentials
	mu       sync.RWMutex
}

// newAuthManager creates a new authentication manager
func newAuthManager(provider AuthProvider) *authManager {
	return &authManager{
		provider: provider,
	}
}

// ensureValidCredentials ensures credentials are valid, refreshing if needed
func (am *authManager) ensureValidCredentials(ctx context.Context) error {
	am.mu.RLock()
	creds := am.creds
	am.mu.RUnlock()

	// No credentials - authenticate
	if creds == nil {
		return am.authenticate(ctx)
	}

	// Check if expired - refresh if possible
	if am.provider.IsExpired(creds) {
		return am.refresh(ctx, creds)
	}

	return nil
}

// authenticate performs initial authentication
func (am *authManager) authenticate(ctx context.Context) error {
	creds, err := am.provider.Authenticate(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	am.mu.Lock()
	am.creds = creds
	am.mu.Unlock()

	return nil
}

// refresh attempts to refresh credentials
func (am *authManager) refresh(ctx context.Context, oldCreds *Credentials) error {
	if oldCreds == nil || oldCreds.RefreshToken == "" {
		// No refresh token - re-authenticate
		return am.authenticate(ctx)
	}

	newCreds, err := am.provider.Refresh(ctx, oldCreds)
	if err != nil {
		// Refresh failed - try full re-authentication
		return am.authenticate(ctx)
	}

	am.mu.Lock()
	am.creds = newCreds
	am.mu.Unlock()

	return nil
}

// applyAuth applies authentication to an HTTP request
func (am *authManager) applyAuth(req *http.Request) error {
	am.mu.RLock()
	creds := am.creds
	am.mu.RUnlock()

	if creds == nil {
		return fmt.Errorf("no credentials available")
	}

	return am.provider.ApplyAuth(req, creds)
}

// close releases resources
func (am *authManager) close() error {
	if am.provider != nil {
		return am.provider.Close()
	}
	return nil
}
```

---

## 3. Rate Limiter Integration

### File: `internal/core/http/ratelimit.go`

```go
package http

import (
	"context"
	"fmt"
	"time"

	"github.com/openapi-cli/core/internal/core/ratelimit"
)

// RateLimiter defines the interface for rate limiting
// This matches the interface from RATE-LIMITING-IMPLEMENTATION.md
type RateLimiter interface {
	// Allow checks if a request is allowed for the given key
	Allow(ctx context.Context, key string) (*ratelimit.Result, error)

	// Close releases resources
	Close() error
}

// rateLimitMiddleware wraps requests with rate limiting
type rateLimitMiddleware struct {
	limiter   RateLimiter
	keyFunc   func(*http.Request) string
}

// newRateLimitMiddleware creates rate limiting middleware
func newRateLimitMiddleware(limiter RateLimiter, keyFunc func(*http.Request) string) *rateLimitMiddleware {
	if keyFunc == nil {
		keyFunc = defaultRateLimitKey
	}
	return &rateLimitMiddleware{
		limiter: limiter,
		keyFunc: keyFunc,
	}
}

// RoundTrip implements http.RoundTripper
func (m *rateLimitMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	key := m.keyFunc(req)

	result, err := m.limiter.Allow(req.Context(), key)
	if err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	if !result.Allowed {
		return nil, &RateLimitError{
			Limit:      result.Limit,
			Remaining:  result.Remaining,
			ResetTime:  result.ResetTime,
			RetryAfter: result.RetryAfter,
		}
	}

	return m.next.RoundTrip(req)
}

// RateLimitError represents a rate limit exceeded error
type RateLimitError struct {
	Limit      uint64
	Remaining  uint64
	ResetTime  time.Time
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded: retry after %v", e.RetryAfter)
}

// defaultRateLimitKey generates a default rate limit key from request
func defaultRateLimitKey(req *http.Request) string {
	return fmt.Sprintf("%s:%s", req.Method, req.Host)
}
```

---

## 4. Circuit Breaker Integration

### File: `internal/core/http/circuitbreaker.go`

```go
package http

import (
	"fmt"
	"net/http"

	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

// CircuitBreaker defines the interface for circuit breaker pattern
// This matches the interface from CIRCUIT-BREAKER-IMPLEMENTATION.md
type CircuitBreaker interface {
	// Execute runs a function with circuit breaker protection
	Execute(func() error) error

	// State returns the current circuit breaker state
	State() circuitbreaker.State

	// Close releases resources
	Close() error
}

// circuitBreakerMiddleware wraps requests with circuit breaker
type circuitBreakerMiddleware struct {
	breaker CircuitBreaker
}

// newCircuitBreakerMiddleware creates circuit breaker middleware
func newCircuitBreakerMiddleware(breaker CircuitBreaker) *circuitBreakerMiddleware {
	return &circuitBreakerMiddleware{
		breaker: breaker,
	}
}

// RoundTrip implements http.RoundTripper
func (m *circuitBreakerMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	execErr := m.breaker.Execute(func() error {
		resp, err = m.next.RoundTrip(req)
		if err != nil {
			return err
		}

		// Treat 5xx errors as failures
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return nil
	})

	if execErr != nil {
		if execErr == circuitbreaker.ErrCircuitOpen {
			return nil, &CircuitBreakerError{
				State: m.breaker.State(),
				URL:   req.URL.String(),
			}
		}
		return nil, execErr
	}

	return resp, err
}

// CircuitBreakerError represents an error when circuit is open
type CircuitBreakerError struct {
	State circuitbreaker.State
	URL   string
}

func (e *CircuitBreakerError) Error() string {
	return fmt.Sprintf("circuit breaker is %s for %s", e.State, e.URL)
}

// IsCircuitBreakerError checks if an error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	var cbErr *CircuitBreakerError
	return errors.As(err, &cbErr)
}
```

---

## 5. Middleware System

### File: `internal/core/http/middleware.go`

```go
package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/sirupsen/logrus"
)

// Middleware is a function that wraps an http.RoundTripper
type Middleware func(http.RoundTripper) http.RoundTripper

// MiddlewareChain manages a chain of middleware
type MiddlewareChain struct {
	middlewares []Middleware
	base        http.RoundTripper
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain(base http.RoundTripper) *MiddlewareChain {
	if base == nil {
		base = http.DefaultTransport
	}
	return &MiddlewareChain{
		middlewares: make([]Middleware, 0),
		base:        base,
	}
}

// Add adds a middleware to the chain
func (c *MiddlewareChain) Add(middleware Middleware) {
	c.middlewares = append(c.middlewares, middleware)
}

// Build returns the final RoundTripper with all middleware applied
func (c *MiddlewareChain) Build() http.RoundTripper {
	transport := c.base
	// Apply middleware in reverse order so first added executes first
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		transport = c.middlewares[i](transport)
	}
	return transport
}

// loggingMiddleware logs HTTP requests and responses
type loggingMiddleware struct {
	logger  *logrus.Logger
	level   logrus.Level
	next    http.RoundTripper
}

// newLoggingMiddleware creates logging middleware
func newLoggingMiddleware(logger *logrus.Logger, level logrus.Level) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return &loggingMiddleware{
			logger: logger,
			level:  level,
			next:   next,
		}
	}
}

// RoundTrip implements http.RoundTripper
func (m *loggingMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Log request
	if m.logger.IsLevelEnabled(m.level) {
		reqDump, _ := httputil.DumpRequestOut(req, false)
		m.logger.Logf(m.level, "HTTP Request: %s", string(reqDump))
	}

	resp, err := m.next.RoundTrip(req)
	duration := time.Since(start)

	// Log response
	if err != nil {
		m.logger.WithError(err).Errorf("HTTP Request failed after %v: %s %s", duration, req.Method, req.URL)
	} else {
		m.logger.Logf(m.level, "HTTP Response: %d %s in %v", resp.StatusCode, resp.Status, duration)
	}

	return resp, err
}

// retryMiddleware handles request retries with exponential backoff
type retryMiddleware struct {
	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
	shouldRetry func(error) bool
	next        http.RoundTripper
}

// RetryConfig configures the retry middleware
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// defaultRetryConfig returns sensible defaults
func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    30 * time.Second,
	}
}

// newRetryMiddleware creates retry middleware
func newRetryMiddleware(config RetryConfig, shouldRetry func(error) bool) Middleware {
	if shouldRetry == nil {
		shouldRetry = IsRetryableError
	}
	return func(next http.RoundTripper) http.RoundTripper {
		return &retryMiddleware{
			maxRetries:  config.MaxAttempts,
			baseDelay:   config.BaseDelay,
			maxDelay:    config.MaxDelay,
			shouldRetry: shouldRetry,
			next:        next,
		}
	}
}

// RoundTrip implements http.RoundTripper
func (m *retryMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	// Clone request body for retries
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}

	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		// Create new request for each attempt
		reqClone := req.Clone(req.Context())
		if len(bodyBytes) > 0 {
			reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err = m.next.RoundTrip(reqClone)

		// Success - return immediately
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		// Check if we should retry
		if attempt < m.maxRetries && m.shouldRetry(err) {
			delay := calculateBackoff(attempt, m.baseDelay, m.maxDelay)
			select {
			case <-time.After(delay):
				continue
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}

		// Don't retry - return result
		return resp, err
	}

	return resp, err
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Exponential backoff: baseDelay * 2^attempt
	delay := baseDelay * time.Duration(1<<uint(attempt))

	// Cap at maxDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter: ±25%
	jitter := time.Duration(float64(delay) * 0.25)
	if jitter > 0 {
		// Use current nanosecond as simple jitter source
		ns := time.Now().UnixNano() % int64(jitter*2)
		delay = delay - jitter + time.Duration(ns)
	}

	return delay
}

// headerMiddleware manages default and per-request headers
type headerMiddleware struct {
	defaultHeaders map[string]string
	next           http.RoundTripper
}

// newHeaderMiddleware creates header management middleware
func newHeaderMiddleware(defaultHeaders map[string]string) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return &headerMiddleware{
			defaultHeaders: defaultHeaders,
			next:           next,
		}
	}
}

// RoundTrip implements http.RoundTripper
func (m *headerMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	// Apply default headers
	for key, value := range m.defaultHeaders {
		if req.Header.Get(key) == "" {
			req.Header.Set(key, value)
		}
	}

	return m.next.RoundTrip(req)
}

// tracingMiddleware adds distributed tracing headers
type tracingMiddleware struct {
	next http.RoundTripper
}

// newTracingMiddleware creates tracing middleware
func newTracingMiddleware() Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return &tracingMiddleware{next: next}
	}
}

// RoundTrip implements http.RoundTripper
func (m *tracingMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add trace ID if not present
	traceID := req.Header.Get("X-Request-ID")
	if traceID == "" {
		traceID = generateTraceID()
		req.Header.Set("X-Request-ID", traceID)
	}

	return m.next.RoundTrip(req)
}

// generateTraceID generates a unique trace ID
func generateTraceID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(8))
}

// randomString generates a random string of given length
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
```

---

## 6. Client Implementation

### File: `internal/core/http/client_impl.go`

```go
package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// client is the main HTTP client implementation
type client struct {
	// Configuration
	baseURL        string
	defaultHeaders map[string]string
	defaultTimeout time.Duration

	// Dependencies
	httpClient     *http.Client
	authManager    *authManager
	rateLimiter    RateLimiter
	circuitBreaker CircuitBreaker
	logger         *logrus.Logger

	// Middleware
	middlewareChain *MiddlewareChain

	// State
	mu     sync.RWMutex
	closed bool
}

// ClientOptions configures the HTTP client
type ClientOptions struct {
	// Base URL for all requests (optional)
	BaseURL string

	// Default headers applied to all requests
	DefaultHeaders map[string]string

	// Default timeout for requests
	DefaultTimeout time.Duration

	// TLS configuration
	TLSConfig *tls.Config

	// Connection pool settings
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	// Retry configuration
	Retry RetryConfig

	// Logger for request/response logging
	Logger *logrus.Logger

	// Disable following redirects
	DisableRedirects bool
}

// DefaultClientOptions returns sensible defaults
func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		DefaultTimeout:      30 * time.Second,
		DefaultHeaders:      make(map[string]string),
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		Retry:               defaultRetryConfig(),
		Logger:              logrus.New(),
	}
}

// NewClient creates a new HTTP client
func NewClient(opts ClientOptions) (Client, error) {
	if opts.DefaultTimeout == 0 {
		opts.DefaultTimeout = 30 * time.Second
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 100
	}
	if opts.MaxIdleConnsPerHost == 0 {
		opts.MaxIdleConnsPerHost = 10
	}
	if opts.IdleConnTimeout == 0 {
		opts.IdleConnTimeout = 90 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = logrus.New()
	}

	// Create base transport with connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       opts.TLSConfig,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		IdleConnTimeout:       opts.IdleConnTimeout,
		DisableKeepAlives:     false,
	}

	// Build middleware chain
	chain := NewMiddlewareChain(transport)

	// Add retry middleware
	chain.Add(newRetryMiddleware(opts.Retry, nil))

	// Add header middleware
	chain.Add(newHeaderMiddleware(opts.DefaultHeaders))

	// Add tracing middleware
	chain.Add(newTracingMiddleware())

	// Add logging middleware
	chain.Add(newLoggingMiddleware(opts.Logger, logrus.DebugLevel))

	httpClient := &http.Client{
		Transport: chain.Build(),
		Timeout:   opts.DefaultTimeout,
	}

	if opts.DisableRedirects {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	c := &client{
		baseURL:         opts.BaseURL,
		defaultHeaders:  opts.DefaultHeaders,
		defaultTimeout:  opts.DefaultTimeout,
		httpClient:    httpClient,
		middlewareChain: chain,
		logger:        opts.Logger,
	}

	return c, nil
}

// Do executes an HTTP request
func (c *client) Do(ctx context.Context, req *Request) (*Response, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	start := time.Now()

	// Build URL
	fullURL := req.URL
	if c.baseURL != "" && !isAbsoluteURL(req.URL) {
		base, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		rel, err := url.Parse(req.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid request URL: %w", err)
		}
		fullURL = base.ResolveReference(rel).String()
	}

	// Create HTTP request
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Apply headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Apply authentication if not skipped
	if !req.skipAuth && c.authManager != nil {
		if err := c.authManager.ensureValidCredentials(ctx); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
		if err := c.authManager.applyAuth(httpReq); err != nil {
			return nil, fmt.Errorf("failed to apply auth: %w", err)
		}
	}

	// Execute request
	httpResp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 10*1024*1024)) // Limit to 10MB
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Convert headers
	headers := make(map[string]string)
	for key, values := range httpResp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	resp := &Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       headers,
		Body:          body,
		Duration:      duration,
		RetryCount:    req.attempt,
		RequestURL:    fullURL,
		RequestMethod: req.Method,
	}

	// Handle error responses
	if httpResp.StatusCode >= 400 {
		return resp, &HTTPError{
			StatusCode: httpResp.StatusCode,
			Status:     httpResp.Status,
			Body:       body,
			URL:        fullURL,
			Method:     req.Method,
		}
	}

	return resp, nil
}

// Get performs a GET request
func (c *client) Get(ctx context.Context, url string, opts *RequestOptions) (*Response, error) {
	req := &Request{
		Method: "GET",
		URL:    url,
	}

	if opts != nil {
		req.Headers = opts.Headers
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// Post performs a POST request with JSON body
func (c *client) Post(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error) {
	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
	}

	headers := make(map[string]string)
	if opts != nil && opts.Headers != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
	}
	if opts == nil || opts.ContentType == "" {
		headers["Content-Type"] = "application/json"
	} else {
		headers["Content-Type"] = opts.ContentType
	}

	req := &Request{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    bodyBytes,
	}

	if opts != nil {
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// PostForm performs a POST request with form data
func (c *client) PostForm(ctx context.Context, url string, data url.Values, opts *RequestOptions) (*Response, error) {
	body := data.Encode()

	headers := make(map[string]string)
	if opts != nil && opts.Headers != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"

	req := &Request{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    []byte(body),
	}

	if opts != nil {
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// Patch performs a PATCH request
func (c *client) Patch(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error) {
	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
	}

	headers := make(map[string]string)
	if opts != nil && opts.Headers != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
	}
	headers["Content-Type"] = "application/json"

	req := &Request{
		Method:  "PATCH",
		URL:     url,
		Headers: headers,
		Body:    bodyBytes,
	}

	if opts != nil {
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// Put performs a PUT request
func (c *client) Put(ctx context.Context, url string, body interface{}, opts *RequestOptions) (*Response, error) {
	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
	}

	headers := make(map[string]string)
	if opts != nil && opts.Headers != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
	}
	headers["Content-Type"] = "application/json"

	req := &Request{
		Method:  "PUT",
		URL:     url,
		Headers: headers,
		Body:    bodyBytes,
	}

	if opts != nil {
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// Delete performs a DELETE request
func (c *client) Delete(ctx context.Context, url string, opts *RequestOptions) (*Response, error) {
	req := &Request{
		Method: "DELETE",
		URL:    url,
	}

	if opts != nil {
		req.Headers = opts.Headers
		req.skipAuth = opts.SkipAuth
		req.skipRetry = opts.SkipRetry
	}

	return c.Do(ctx, req)
}

// SetAuthProvider sets the authentication provider
func (c *client) SetAuthProvider(provider AuthProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authManager = newAuthManager(provider)
}

// SetRateLimiter sets the rate limiter
func (c *client) SetRateLimiter(limiter RateLimiter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rateLimiter = limiter

	// Add rate limit middleware if not already present
	if limiter != nil {
		c.middlewareChain.Add(newRateLimitMiddleware(limiter, defaultRateLimitKey))
		// Rebuild client with new middleware
		c.httpClient.Transport = c.middlewareChain.Build()
	}
}

// SetCircuitBreaker sets the circuit breaker
func (c *client) SetCircuitBreaker(breaker CircuitBreaker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.circuitBreaker = breaker

	// Add circuit breaker middleware if not already present
	if breaker != nil {
		c.middlewareChain.Add(newCircuitBreakerMiddleware(breaker))
		// Rebuild client with new middleware
		c.httpClient.Transport = c.middlewareChain.Build()
	}
}

// AddMiddleware adds a middleware to the chain
func (c *client) AddMiddleware(middleware Middleware) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewareChain.Add(middleware)
	// Rebuild client with new middleware
	c.httpClient.Transport = c.middlewareChain.Build()
}

// SetBaseURL sets the base URL for all requests
func (c *client) SetBaseURL(baseURL string) error {
	if baseURL != "" {
		if _, err := url.Parse(baseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}
	c.mu.Lock()
	c.baseURL = baseURL
	c.mu.Unlock()
	return nil
}

// SetDefaultHeaders sets default headers for all requests
func (c *client) SetDefaultHeaders(headers map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.defaultHeaders = headers

	// Rebuild header middleware
	c.middlewareChain = NewMiddlewareChain(c.httpClient.Transport)
	c.middlewareChain.Add(newHeaderMiddleware(headers))
	c.httpClient.Transport = c.middlewareChain.Build()
}

// Close releases all resources
func (c *client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Close authentication manager
	if c.authManager != nil {
		c.authManager.close()
	}

	// Close rate limiter
	if c.rateLimiter != nil {
		c.rateLimiter.Close()
	}

	// Close circuit breaker
	if c.circuitBreaker != nil {
		c.circuitBreaker.Close()
	}

	// Close idle connections
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}

	return nil
}

// isAbsoluteURL checks if a URL is absolute
func isAbsoluteURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.IsAbs()
}
```

---

## 7. TLS Configuration

### File: `internal/core/http/tls.go`

```go
package http

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfigBuilder builds TLS configurations
type TLSConfigBuilder struct {
	certFile       string
	keyFile        string
	caFile         string
	insecure       bool
	serverName     string
	minVersion     uint16
	cipherSuites   []uint16
}

// NewTLSConfigBuilder creates a new TLS config builder
func NewTLSConfigBuilder() *TLSConfigBuilder {
	return &TLSConfigBuilder{
		minVersion: tls.VersionTLS12,
		cipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

// WithClientCert adds a client certificate (mTLS)
func (b *TLSConfigBuilder) WithClientCert(certFile, keyFile string) *TLSConfigBuilder {
	b.certFile = certFile
	b.keyFile = keyFile
	return b
}

// WithCA adds a CA certificate
func (b *TLSConfigBuilder) WithCA(caFile string) *TLSConfigBuilder {
	b.caFile = caFile
	return b
}

// WithInsecureSkipVerify disables certificate verification (dev only)
func (b *TLSConfigBuilder) WithInsecureSkipVerify(insecure bool) *TLSConfigBuilder {
	b.insecure = insecure
	return b
}

// WithServerName sets the expected server name
func (b *TLSConfigBuilder) WithServerName(serverName string) *TLSConfigBuilder {
	b.serverName = serverName
	return b
}

// Build creates the TLS configuration
func (b *TLSConfigBuilder) Build() (*tls.Config, error) {
	config := &tls.Config{
		MinVersion:         b.minVersion,
		CipherSuites:       b.cipherSuites,
		InsecureSkipVerify: b.insecure,
		ServerName:         b.serverName,
	}

	// Load client certificate for mTLS
	if b.certFile != "" && b.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(b.certFile, b.keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate
	if b.caFile != "" {
		caCert, err := os.ReadFile(b.caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.RootCAs = caCertPool
	}

	return config, nil
}

// CreateMTLSConfig creates an mTLS configuration
func CreateMTLSConfig(certPEM, keyPEM, caPEM []byte, serverName string) (*tls.Config, error) {
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_AES_256_GCM_SHA384,
		},
		ServerName: serverName,
	}

	// Load client certificate
	if len(certPEM) > 0 && len(keyPEM) > 0 {
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate
	if len(caPEM) > 0 {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		config.RootCAs = caCertPool
	}

	return config, nil
}
```

---

## 8. Complete Integration Example

### File: `internal/core/http/example.go`

```go
package http

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openapi-cli/core/internal/core/auth"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
	"github.com/openapi-cli/core/internal/core/ratelimit"
	"github.com/sirupsen/logrus"
)

// Example demonstrates complete HTTP client usage with all integrations
func Example() {
	ctx := context.Background()

	// 1. Create HTTP client with defaults
	client, err := NewClient(ClientOptions{
		BaseURL:        "https://api.example.com",
		DefaultTimeout: 30 * time.Second,
		DefaultHeaders: map[string]string{
			"Accept":       "application/json",
			"User-Agent":   "openapi-cli/1.0.0",
		},
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Second,
			MaxDelay:    30 * time.Second,
		},
		Logger: logrus.New(),
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 2. Set up authentication (OAuth2 example)
	authProvider, err := auth.NewOAuth2Provider(&auth.OAuth2Config{
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		TokenURL:     "https://auth.example.com/oauth/token",
		Scopes:       []string{"read", "write"},
	})
	if err != nil {
		log.Fatalf("Failed to create auth provider: %v", err)
	}
	client.SetAuthProvider(authProvider)

	// 3. Set up rate limiting
	backend := ratelimit.NewMemoryBackend(ratelimit.MemoryBackendOptions{
		DefaultRate: ratelimit.Rate{
			RPS:   10,  // 10 requests per second
			Burst: 20,  // Allow bursts up to 20
		},
	})
	limiter, _ := ratelimit.NewLimiter(ratelimit.Config{
		Backend:     backend,
		DefaultRate: ratelimit.Rate{RPS: 10, Burst: 20},
	})
	client.SetRateLimiter(limiter)

	// 4. Set up circuit breaker
	cb, _ := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold:    5,
		SuccessThreshold:    3,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
	})
	client.SetCircuitBreaker(cb)

	// 5. Make requests

	// GET request
	resp, err := client.Get(ctx, "/users/123", nil)
	if err != nil {
		if httpErr, ok := IsHTTPError(err); ok {
			log.Printf("HTTP Error: %d %s", httpErr.StatusCode, httpErr.Status)
		} else if rateErr, ok := err.(*RateLimitError); ok {
			log.Printf("Rate limited, retry after: %v", rateErr.RetryAfter)
		} else if cbErr, ok := IsCircuitBreakerError(err); ok {
			log.Printf("Circuit breaker open: %s", cbErr.State)
		} else {
			log.Printf("Request failed: %v", err)
		}
	} else {
		fmt.Printf("Response: %d, Body: %s\n", resp.StatusCode, string(resp.Body))
	}

	// POST request with JSON body
	newUser := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}
	resp, err = client.Post(ctx, "/users", newUser, nil)
	if err != nil {
		log.Printf("POST failed: %v", err)
	} else {
		fmt.Printf("Created user: %s\n", string(resp.Body))
	}

	// PATCH request
	update := map[string]interface{}{
		"name": "Jane Doe",
	}
	resp, err = client.Patch(ctx, "/users/123", update, nil)
	if err != nil {
		log.Printf("PATCH failed: %v", err)
	}

	// DELETE request
	resp, err = client.Delete(ctx, "/users/123", nil)
	if err != nil {
		log.Printf("DELETE failed: %v", err)
	} else {
		fmt.Printf("Deleted user: %d\n", resp.StatusCode)
	}

	// Request with options
	resp, err = client.Get(ctx, "/sensitive-data", &RequestOptions{
		SkipAuth:    false,
		SkipRetry:   true,
		SkipRateLimit: false,
		Headers: map[string]string{
			"X-Custom-Header": "value",
		},
	})
	if err != nil {
		log.Printf("Request failed: %v", err)
	}
}

// ExampleWithMTLS demonstrates mTLS usage
func ExampleWithMTLS() {
	ctx := context.Background()

	// Create TLS configuration with mTLS
	tlsConfig, err := CreateMTLSConfig(
		[]byte(clientCertPEM),
		[]byte(clientKeyPEM),
		[]byte(caCertPEM),
		"api.example.com",
	)
	if err != nil {
		log.Fatalf("Failed to create TLS config: %v", err)
	}

	// Create client with mTLS
	client, err := NewClient(ClientOptions{
		BaseURL:        "https://api.example.com",
		TLSConfig:      tlsConfig,
		DefaultTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Make request - authentication is handled via mTLS certificates
	resp, err := client.Get(ctx, "/secure-endpoint", nil)
	if err != nil {
		log.Printf("Request failed: %v", err)
	} else {
		fmt.Printf("Response: %s\n", string(resp.Body))
	}
}

// ExampleWithCircuitBreaker demonstrates circuit breaker patterns
func ExampleWithCircuitBreaker() {
	ctx := context.Background()

	// Create circuit breaker with hooks
	cb, _ := circuitbreaker.NewWithHooks(
		circuitbreaker.Config{
			FailureThreshold:    3,
			SuccessThreshold:    2,
			Timeout:             10 * time.Second,
			HalfOpenMaxRequests: 2,
		},
		circuitbreaker.Hooks{
			OnStateChange: func(from, to circuitbreaker.State) {
				log.Printf("Circuit breaker: %s -> %s", from, to)
			},
			OnFailure: func(err error) {
				log.Printf("Request failed: %v", err)
			},
			OnSuccess: func() {
				log.Println("Request succeeded")
			},
			OnReject: func() {
				log.Println("Request rejected - circuit open")
			},
		},
	)

	client, _ := NewClient(DefaultClientOptions())
	client.SetCircuitBreaker(cb)
	defer client.Close()

	// Make requests - circuit breaker will monitor and protect
	for i := 0; i < 10; i++ {
		_, err := client.Get(ctx, "/unreliable-endpoint", nil)
		if err != nil {
			if IsCircuitBreakerError(err) {
				log.Println("Circuit is open, not making request")
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

var (
	clientCertPEM = `-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----`

	clientKeyPEM = `-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----`

	caCertPEM = `-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----`
)
```

---

## 9. Testing

### File: `internal/core/http/client_test.go`

```go
package http

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
)

func TestClientGet(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/test", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "success"})
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(DefaultClientOptions())
	require.NoError(t, err)
	defer client.Close()

	// Make request
	resp, err := client.Get(context.Background(), server.URL+"/test", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "success")
}

func TestClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		json.Unmarshal(body, &data)
		assert.Equal(t, "test", data["name"])

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, _ := NewClient(DefaultClientOptions())
	defer client.Close()

	resp, err := client.Post(context.Background(), server.URL, map[string]string{"name": "test"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestClientRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
		},
	})
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 3, attempts)
}

func TestHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid input"}`))
	}))
	defer server.Close()

	client, _ := NewClient(DefaultClientOptions())
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL, nil)
	require.Error(t, err)

	httpErr, ok := IsHTTPError(err)
	require.True(t, ok)
	assert.Equal(t, 400, httpErr.StatusCode)
	assert.Contains(t, string(httpErr.Body), "invalid input")
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"timeout", fmt.Errorf("operation timeout"), true},
		{"HTTP 500", &HTTPError{StatusCode: 500}, true},
		{"HTTP 400", &HTTPError{StatusCode: 400}, false},
		{"random error", fmt.Errorf("something else"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRetryableError(tt.err))
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		base     time.Duration
		max      time.Duration
		expected time.Duration
	}{
		{0, 1 * time.Second, 30 * time.Second, 1 * time.Second},
		{1, 1 * time.Second, 30 * time.Second, 2 * time.Second},
		{2, 1 * time.Second, 30 * time.Second, 4 * time.Second},
		{5, 1 * time.Second, 30 * time.Second, 30 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		actual := calculateBackoff(tt.attempt, tt.base, tt.max)
		// Allow 25% jitter
		min := time.Duration(float64(tt.expected) * 0.75)
		max := time.Duration(float64(tt.expected) * 1.25)
		assert.True(t, actual >= min && actual <= max,
			"Expected %v ±25%, got %v", tt.expected, actual)
	}
}

func TestClientWithBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/users", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{
		BaseURL: server.URL,
	})
	defer client.Close()

	resp, err := client.Get(context.Background(), "/api/users", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestClientClose(t *testing.T) {
	client, _ := NewClient(DefaultClientOptions())

	err := client.Close()
	require.NoError(t, err)

	// Should return error if closed again
	err = client.Close()
	require.NoError(t, err) // Idempotent close
}

func BenchmarkClientGet(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(DefaultClientOptions())
	defer client.Close()

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			client.Get(ctx, server.URL, nil)
		}
	})
}
```

---

## 10. Package Documentation

### File: `internal/core/http/doc.go`

```go
// Package http provides a production-ready HTTP client with comprehensive features:
//
// Features:
//   - Request/Response types with full metadata
//   - Middleware system for extensibility
//   - Authentication integration (OAuth2, API Key, mTLS)
//   - Rate limiting support
//   - Circuit breaker pattern
//   - Automatic retries with exponential backoff
//   - Connection pooling and TLS configuration
//   - Context-based timeouts and cancellation
//   - Header management (default and per-request)
//   - Comprehensive error handling
//
// Basic Usage:
//
//	client, err := http.NewClient(http.DefaultClientOptions())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	resp, err := client.Get(ctx, "https://api.example.com/users", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// With Authentication:
//
//	authProvider, _ := auth.NewOAuth2Provider(config)
//	client.SetAuthProvider(authProvider)
//
// With Rate Limiting:
//
//	backend := ratelimit.NewMemoryBackend(opts)
//	limiter, _ := ratelimit.NewLimiter(config)
//	client.SetRateLimiter(limiter)
//
// With Circuit Breaker:
//
//	cb, _ := circuitbreaker.New(config)
//	client.SetCircuitBreaker(cb)
//
// The client integrates with:
//   - AuthProvider from AUTH-IMPLEMENTATION.md
//   - RateLimiter from RATE-LIMITING-IMPLEMENTATION.md
//   - CircuitBreaker from CIRCUIT-BREAKER-IMPLEMENTATION.md
//
package http
```

---

## Summary

This HTTP client implementation provides:

1. **Core HTTP Client** (`client.go`, `client_impl.go`)
   - Complete Do(), Get(), Post(), Patch(), Put(), Delete() methods
   - Request/Response structs with full metadata
   - Context-based timeouts and cancellation

2. **Middleware System** (`middleware.go`)
   - RoundTripper-based middleware chain
   - Built-in logging, retry, header, and tracing middleware
   - Extensible for custom middleware

3. **Authentication Integration** (`auth.go`)
   - AuthProvider interface matching AUTH-IMPLEMENTATION.md
   - Automatic token refresh with thread-safe credential management

4. **Rate Limiting Integration** (`ratelimit.go`)
   - RateLimiter interface matching RATE-LIMITING-IMPLEMENTATION.md
   - Automatic rate limit checking before requests

5. **Circuit Breaker Integration** (`circuitbreaker.go`)
   - CircuitBreaker interface matching CIRCUIT-BREAKER-IMPLEMENTATION.md
   - HTTP middleware that monitors responses and triggers circuit breaker

6. **Retry Logic** (`middleware.go`)
   - Exponential backoff with jitter
   - Configurable max attempts and delays
   - Automatic retry on retryable errors

7. **TLS Configuration** (`tls.go`)
   - mTLS support with client certificates
   - Custom CA certificate support
   - TLS version and cipher suite configuration

8. **Connection Pooling** (in `client_impl.go`)
   - HTTP transport with configurable pool settings
   - Connection reuse and keep-alive

9. **Error Handling** (`client.go`)
   - HTTPError type with status code checking
   - IsRetryableError() for determining retry eligibility
   - Circuit breaker and rate limit error types

10. **Production-Ready Features**
    - Thread-safe operations with sync.RWMutex
    - Resource cleanup with Close() method
    - Request timeout handling
    - Response body size limits
    - Comprehensive logging support

## Deterministic Guarantees

This HTTP client implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic HTTP Request Formation

HTTP requests must be formed identically across platforms:

- **Header canonicalization**: Use `textproto.CanonicalMIMEHeaderKey()` for header names
- **Header value normalization**: Trim whitespace, collapse multiple spaces
- **URL parameter ordering**: Sorted lexicographically (see `API-SPECIFICATION.md` section 3)
- **Body serialization**: Deterministic JSON serialization (sorted keys, no extra whitespace)

### 2. Deterministic Retry Behavior

Retry logic must produce identical retry sequences:

- **Jitter seeds**: Use deterministic seed for random number generation in test mode
- **Backoff calculation**: Use integer milliseconds, identical across platforms
- **Retry decision**: Same error → same retry decision across platforms

```go
// internal/determinism/retry.go

func deterministicJitter(base time.Duration, seed int64) time.Duration {
	rng := rand.New(rand.NewSource(seed))
	jitter := rng.Int63n(int64(base / 2))
	return base + time.Duration(jitter)
}
```

### 3. Platform‑Independent Network Behavior

Network operations must behave identically across platforms:

- **DNS resolution**: Use `net.LookupIP` with consistent ordering (IPv4 before IPv6)
- **TCP connection ordering**: Connect to first resolved IP address
- **TLS handshake**: Identical cipher suite negotiation order
- **HTTP/2 negotiation**: Same ALPN protocol selection

### 4. Concurrent Request Ordering

Concurrent requests must be processed with deterministic ordering:

- **Connection pool scheduling**: FIFO queue for connection reuse
- **Rate limit token bucket**: Deterministic token allocation ordering
- **Circuit breaker state transitions**: Identical state machine across platforms

### 5. Time Zone Independence

All timestamps in HTTP headers must use UTC:

- **Date header**: RFC 1123 format with GMT timezone
- **Expires header**: UTC‑based expiration
- **Retry‑After header**: Seconds (integer) not absolute timestamps

### 6. Error Mapping Determinism

HTTP errors must map to identical error types across platforms:

- **Status code mapping**: Same status → same error type
- **Error message formatting**: Identical format strings
- **Error wrapping**: Same error chain structure

### 7. Testing Determinism

All HTTP client tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestHTTPClientDeterminism(t *testing.T) {
	// Verify identical request formation
	// Verify identical retry sequences
	// Verify identical error mapping
}
```

### 8. Integration with SDK Generation

When generating SDKs for 12 languages, HTTP clients must:

- Produce identical HTTP requests (byte‑for‑byte)
- Handle identical HTTP responses (parsing, error detection)
- Implement identical retry and circuit breaker logic

**Verification**: Run `./scripts/validate‑determinism‑http‑client.sh` to confirm cross‑platform consistency.

All implementations include complete source code, not stubs, and integrate seamlessly with the existing authentication, rate limiting, and circuit breaker specifications.
