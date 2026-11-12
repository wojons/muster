package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/log"
)

// Logger defines the interface for daemon logging.
type Logger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// AuditHook records an event after a daemon request has been served. The
// interface keeps the controller independent of the audit package while still
// wiring the production request path to the configured hook.
type AuditHook interface {
	LogCommand(ctx context.Context, command, actorID string, success bool, details map[string]string)
}

// ControllerConfig holds configuration for the daemon controller.
type ControllerConfig struct {
	PidFilePath string
	SocketPath  string
	Version     string
	Logger      Logger
	Handler     http.Handler
	// AuditHook receives one command event for each request served by the
	// controller. Keeping the interface here lets the controller own the real
	// request seam without importing the audit implementation.
	AuditHook AuditHook
	// AuthTokens, when non-empty, makes the socket require a client token
	// (Authorization: Bearer <token>) before any request reaches the handler.
	// MUSTER-001: a daemon that INJECTS an upstream credential must gate its
	// own socket, otherwise every local caller is silently authenticated as
	// the daemon's upstream identity.
	AuthTokens []string
}

// DaemonStatus holds the current daemon status.
type DaemonStatus struct {
	Running   bool
	PID       int
	StartTime time.Time
	Version   string
}

// Controller manages the daemon lifecycle and Unix socket proxy.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1
type Controller struct {
	config      ControllerConfig
	pidFile     string
	socketPath  string
	version     string
	logger      Logger
	handler     http.Handler
	tokenPath   string
	proxyServer *ProxyServer
	running     bool
	startTime   time.Time
	mu          sync.RWMutex
}

// NewController creates a new daemon controller.
func NewController(config ControllerConfig) (*Controller, error) {
	return &Controller{
		config:     config,
		pidFile:    config.PidFilePath,
		socketPath: config.SocketPath,
		version:    config.Version,
		logger:     config.Logger,
		handler:    config.Handler,
		tokenPath:  config.SocketPath + ".token",
	}, nil
}

// Start starts the daemon process.
func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("daemon is already running")
	}

	c.running = true
	c.startTime = time.Now()

	if c.logger != nil {
		c.logger.Info("daemon started", "pid", os.Getpid())
	}

	return nil
}

// Stop stops the daemon process.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("daemon is not running")
	}

	if c.proxyServer != nil {
		if err := c.proxyServer.Stop(); err != nil {
			if c.logger != nil {
				c.logger.Warn("proxy server stop error", "error", err.Error())
			}
		}
		c.proxyServer = nil
	}

	c.running = false

	if c.logger != nil {
		c.logger.Info("daemon stopped")
	}

	return nil
}

// Restart restarts the daemon process.
func (c *Controller) Restart() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		if c.proxyServer != nil {
			if err := c.proxyServer.Stop(); err != nil {
				if c.logger != nil {
					c.logger.Warn("proxy server stop error during restart", "error", err.Error())
				}
			}
			c.proxyServer = nil
		}
		c.running = false
	}

	c.running = true
	c.startTime = time.Now()

	if c.logger != nil {
		c.logger.Info("daemon restarted")
	}

	return nil
}

// Status returns the current daemon status.
func (c *Controller) Status() (DaemonStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return DaemonStatus{
		Running:   c.running,
		PID:       os.Getpid(),
		StartTime: c.startTime,
		Version:   c.version,
	}, nil
}

// GetTokenPath returns the path to the token file.
func (c *Controller) GetTokenPath() string {
	return c.tokenPath
}

// SetupSignalHandlers sets up OS signal handling for graceful shutdown.
func (c *Controller) SetupSignalHandlers() {}

// StartWithSocket starts the daemon with a custom socket handler, creating and starting
// a real ProxyServer that listens on the Unix socket.
func (c *Controller) StartWithSocket(handler http.Handler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("daemon is already running")
	}

	// Remove any stale socket file
	if _, err := os.Stat(c.socketPath); err == nil {
		if err := os.Remove(c.socketPath); err != nil {
			return fmt.Errorf("remove stale socket %s: %w", c.socketPath, err)
		}
	}

	// Ensure parent directory exists
	socketDir := filepath.Dir(c.socketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	// Create default handler if nil
	if handler == nil {
		handler = createDefaultHandler()
	}

	// Audit the actual socket request path, after the handler has produced its
	// response. This keeps audit tied to real daemon work rather than a startup
	// probe or a test-only call.
	handler = withAuditHook(handler, c.config.AuditHook)
	c.handler = handler

	// Create and start the proxy server. When the controller was given client
	// tokens, the socket requires them (MUSTER-001): a daemon that injects an
	// upstream credential must not authenticate callers implicitly.
	var proxy *ProxyServer
	if len(c.config.AuthTokens) > 0 {
		tokenManager := NewTokenManager()
		for _, token := range c.config.AuthTokens {
			tokenManager.AddToken(token)
		}
		proxy = NewAuthenticatedProxyServer(c.socketPath, handler, tokenManager)
	} else {
		proxy = NewProxyServer(c.socketPath, handler)
	}
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("start proxy server: %w", err)
	}
	c.proxyServer = proxy

	c.running = true
	c.startTime = time.Now()

	if c.logger != nil {
		c.logger.Info("proxy server started", "socket", c.socketPath)
	}

	return nil
}

// Shutdown gracefully shuts down the proxy server and cleans up resources.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.proxyServer != nil {
		if err := c.proxyServer.Stop(); err != nil {
			if c.logger != nil {
				c.logger.Warn("proxy server stop error", "error", err.Error())
			}
		}
		c.proxyServer = nil
	}

	c.running = false
	return nil
}

// Kill immediately stops the proxy server without graceful shutdown.
func (c *Controller) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("daemon is not running")
	}

	if c.proxyServer != nil {
		if c.proxyServer.listener != nil {
			_ = c.proxyServer.listener.Close()
		}
		if err := os.Remove(c.socketPath); err != nil && !os.IsNotExist(err) {
			if c.logger != nil {
				c.logger.Warn("remove socket error", "error", err.Error(), "path", c.socketPath)
			}
		}
		c.proxyServer = nil
	}

	c.running = false
	return nil
}

// IsRunning returns whether the daemon is currently running.
func (c *Controller) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// ServeHTTP implements http.Handler by delegating to the proxy server's handler.
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		http.Error(w, "no handler configured", http.StatusInternalServerError)
	}
}

// Handler returns the current HTTP handler.
func (c *Controller) Handler() http.Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.handler
}

// ProxyServer handles HTTP proxying over Unix socket
type ProxyServer struct {
	socketPath   string
	handler      http.Handler
	listener     net.Listener
	server       *http.Server
	shutdownChan chan struct{}
	tokenManager *TokenManager
	mu           sync.RWMutex
}

// NewProxyServer creates a new HTTP proxy server
func NewProxyServer(socketPath string, handler http.Handler) *ProxyServer {
	if handler == nil {
		handler = createDefaultHandler()
	}

	return &ProxyServer{
		socketPath:   socketPath,
		handler:      handler,
		shutdownChan: make(chan struct{}),
	}
}

// NewAuthenticatedProxyServer creates a new HTTP proxy server with token authentication
func NewAuthenticatedProxyServer(socketPath string, handler http.Handler, tokenManager *TokenManager) *ProxyServer {
	if handler == nil {
		handler = createDefaultHandler()
	}

	// Wrap handler with authentication middleware
	authHandler := withTokenAuth(handler, tokenManager)

	return &ProxyServer{
		socketPath:   socketPath,
		handler:      authHandler,
		tokenManager: tokenManager,
		shutdownChan: make(chan struct{}),
	}
}

func withAuditHook(next http.Handler, hook AuditHook) http.Handler {
	if hook == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		hook.LogCommand(r.Context(), "daemon.request", "daemon", recorder.Status() < http.StatusInternalServerError, map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": fmt.Sprintf("%d", recorder.Status()),
		})
	})
}

// withTokenAuth wraps a handler with token authentication middleware
// Excludes health check endpoints from authentication
func withTokenAuth(next http.Handler, tokenManager *TokenManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for health check endpoints
		if isHealthEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Global().Warn().
				Str("event", "auth_failure").
				Str("reason", "missing_header").
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Msg("Unauthorized access attempt: missing authorization header")
			http.Error(w, `{"error":"Unauthorized","message":"Missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		// Validate token
		if !tokenManager.ValidateFromHeader(authHeader) {
			log.Global().Warn().
				Str("event", "auth_failure").
				Str("reason", "invalid_token").
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Msg("Unauthorized access attempt: invalid or expired token")
			http.Error(w, `{"error":"Unauthorized","message":"Invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed with request
		next.ServeHTTP(w, r)
	})
}

// isHealthEndpoint checks if the path is a health check endpoint
// Health endpoints are excluded from authentication
func isHealthEndpoint(path string) bool {
	// Normalize path
	path = strings.TrimSuffix(path, "/")

	switch path {
	case "/health", "/health/live", "/health/ready", "/health/startup":
		return true
	default:
		return false
	}
}

// Start begins listening on the Unix socket
func (p *ProxyServer) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove socket if it exists
	if _, err := os.Stat(p.socketPath); err == nil {
		_ = os.Remove(p.socketPath)
	}

	// Ensure parent directory exists
	socketDir := filepath.Dir(p.socketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}

	// Set restrictive permissions (owner only)
	if err := os.Chmod(p.socketPath, 0600); err != nil {
		return fmt.Errorf("set socket permissions: %w", err)
	}

	p.listener = listener

	// Create HTTP server. Keep a local reference for the serving goroutine;
	// p.server may be replaced by a subsequent Start call.
	server := &http.Server{
		Handler:      p.handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	p.server = server

	// Start serving
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Global().Error().Err(err).Msg("Proxy server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the server
func (p *ProxyServer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	if p.listener != nil {
		_ = p.listener.Close()
	}

	if err := os.Remove(p.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket: %w", err)
	}

	return nil
}

// SocketPath returns the socket path
func (p *ProxyServer) SocketPath() string {
	return p.socketPath
}

// IsRunning checks if the server is running
func (p *ProxyServer) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.server == nil {
		return false
	}

	select {
	case <-p.shutdownChan:
		return false
	default:
		return true
	}
}

// createDefaultHandler creates a default handler that returns JSON responses
func createDefaultHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","message":"Muster daemon is running"}`)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy"}`)
	})

	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"alive","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ready","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	mux.HandleFunc("/health/startup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"started","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	return mux
}

// ProxyHandler creates a handler that proxies requests to a backend URL
type ProxyHandler struct {
	backendURL   string
	forwardedFor bool
	httpClient   *http.Client
	// injectHeader/injectValue carry the UPSTREAM credential the daemon holds
	// on the caller's behalf (MUSTER-001). Empty means no injection — client
	// headers pass through untouched, exactly as before.
	injectHeader string
	injectValue  string
	// metrics, when non-nil, receives one observation per proxied request
	// (muster_daemon_proxy_requests_total / _duration_seconds, spec §3.1).
	// It is an interface so pkg/daemon does not depend on the monitoring
	// implementation and tests can pass a bare recorder.
	metrics ProxyMetrics
}

// ProxyMetrics is the recording surface the proxy writes to. It matches
// monitoring.MetricRecorder.RecordDaemonProxyRequest, so the daemon wires its
// explicit-registry recorder here without pkg/daemon importing internal/.
// endpoint is the request path as received; the recorder is responsible for
// reducing it to a bounded label (spec §6 forbids unbounded path labels).
type ProxyMetrics interface {
	RecordDaemonProxyRequest(endpoint, method string, status int, duration time.Duration)
}

// SetMetrics attaches the recording surface. Nil (the default) disables
// instrumentation, so every existing constructor keeps today's behaviour.
func (h *ProxyHandler) SetMetrics(m ProxyMetrics) {
	h.metrics = m
}

// statusRecorder captures the status code the handler actually sent, for the
// status_class label. A handler that never calls WriteHeader still sends 200,
// so Status() reports 200 in that case rather than 0.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader records the first status code sent (later calls are ignored by
// net/http itself) and forwards it unchanged.
func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

// Write records an implicit 200 for a handler that writes a body without
// calling WriteHeader first.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer when it supports flushing, so
// wrapping the ResponseWriter cannot disable streaming responses.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap exposes the wrapped writer to http.ResponseController.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Status returns the status code sent, defaulting to 200 (net/http's implicit
// status for a handler that wrote nothing).
func (r *statusRecorder) Status() int {
	if !r.wroteHeader {
		return http.StatusOK
	}
	return r.status
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(backendURL string) *ProxyHandler {
	return &ProxyHandler{
		backendURL:   backendURL,
		forwardedFor: true,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewProxyHandlerWithAuth creates a proxy handler that injects an upstream
// credential on every proxied request. Callers then need neither the upstream
// credential nor the upstream base URL: they speak to the Unix socket and the
// daemon supplies the credential (MUSTER-001). The header is applied AFTER the
// client's headers are copied, so a caller cannot override or weaken it.
//
// headerName == "" disables injection (identical to NewProxyHandler).
func NewProxyHandlerWithAuth(backendURL, headerName, headerValue string) *ProxyHandler {
	handler := NewProxyHandler(backendURL)
	handler.injectHeader = strings.TrimSpace(headerName)
	handler.injectValue = headerValue
	return handler
}

// NewProxyHandlerWithRateLimit creates a new proxy handler with token-bucket
// rate limiting applied to outgoing proxy requests. ratePerSec is the number
// of tokens added per second; burst is the maximum accumulated tokens.
// When ratePerSec is <= 0, rate limiting is disabled.
func NewProxyHandlerWithRateLimit(backendURL string, ratePerSec, burst int) *ProxyHandler {
	transport := http.DefaultTransport
	if ratePerSec > 0 {
		transport = client.NewRateLimitRoundTripper(transport, ratePerSec, burst)
	}
	return &ProxyHandler{
		backendURL:   backendURL,
		forwardedFor: true,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// NewProxyHandlerWithCache creates a new proxy handler that caches GET
// responses from the backend. c is the cache implementation; ttl controls
// how long cached responses are stored. The caller retains ownership of
// the cache and is responsible for calling Close when no longer needed.
//
// This composes with NewProxyHandlerWithRateLimit: callers may create a
// ProxyHandler directly and compose their own transport chain.
func NewProxyHandlerWithCache(backendURL string, c cache.Cache, ttl time.Duration) *ProxyHandler {
	transport := client.NewCacheRoundTripper(http.DefaultTransport, c, ttl)
	return &ProxyHandler{
		backendURL:   backendURL,
		forwardedFor: true,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// ServeHTTP implements http.Handler
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Instrument the whole request: the wrapper captures the status actually
	// sent (implicit 200 included) and the deferred recorder observes exactly
	// one point per proxied call, on every return path — including the
	// validation failures below, which are the failures an operator wants to
	// see as 4xx/5xx rather than as silence (spec §3.1).
	started := time.Now()
	status := &statusRecorder{ResponseWriter: w}
	w = status
	// endpoint stays empty until the path passes validation, so an
	// unreachable/invalid backend is reported against "/" rather than an
	// attacker-chosen path.
	endpoint := ""
	defer func() {
		if h.metrics == nil {
			return
		}
		h.metrics.RecordDaemonProxyRequest(endpoint, r.Method, status.Status(), time.Since(started))
	}()

	// Parse backend URL
	backend, err := url.Parse(h.backendURL)
	if err != nil {
		http.Error(w, "Invalid backend URL", http.StatusInternalServerError)
		return
	}

	// Sanitize and validate path components
	cleanedPath := sanitizeProxyPath(r.URL.Path)
	if cleanedPath == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	endpoint = cleanedPath

	// Use url.JoinPath for proper path joining (Go 1.20+)
	// This handles path separators, escaping, and prevents smuggling
	targetURL, err := url.JoinPath(h.backendURL, cleanedPath)
	if err != nil {
		http.Error(w, "Failed to build target URL", http.StatusBadGateway)
		return
	}

	// Append query string if present
	if r.URL.RawQuery != "" {
		if !strings.Contains(targetURL, "?") {
			targetURL += "?" + r.URL.RawQuery
		} else {
			targetURL += "&" + r.URL.RawQuery
		}
	}

	// Validate final URL to prevent scheme/host changes
	targetParsed, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusBadGateway)
		return
	}

	// Ensure target URL maintains the same scheme and host as backend
	if targetParsed.Scheme != "" && targetParsed.Scheme != backend.Scheme {
		http.Error(w, "Invalid target URL scheme", http.StatusBadGateway)
		return
	}
	if targetParsed.Host != "" && targetParsed.Host != backend.Host {
		http.Error(w, "Invalid target URL host", http.StatusBadGateway)
		return
	}

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusBadGateway)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		// Skip hop-by-hop headers
		if isHopHeader(key) {
			continue
		}

		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Add X-Forwarded-For header
	if h.forwardedFor && r.RemoteAddr != "" {
		proxyReq.Header.Add("X-Forwarded-For", r.RemoteAddr)
	}

	// Injected upstream credential (MUSTER-001). Applied after the client's
	// headers so the daemon's credential is authoritative: a caller cannot
	// replace or downgrade it. Set (not Add) — the upstream sees exactly one.
	if h.injectHeader != "" {
		proxyReq.Header.Set(h.injectHeader, h.injectValue)
	}

	// Forward the caller's context to the outbound hop: its deadline and
	// cancellation govern the backend request (PROTOCOLS.md §4.3 — the
	// proxy must not override a caller-set deadline, and every hop stays
	// aware of the caller's budget). The handler's default timeout applies
	// only when the caller declared no deadline; when one exists, the
	// private Client.Timeout is disarmed on a per-request copy so the
	// stdlib's shorter-of-two enforcement cannot clamp the caller.
	outCtx := r.Context()
	outClient := h.httpClient
	if _, hasDeadline := outCtx.Deadline(); hasDeadline && outClient.Timeout > 0 {
		hc := *outClient
		hc.Timeout = 0
		outClient = &hc
	} else if outClient.Timeout > 0 {
		var cancel context.CancelFunc
		outCtx, cancel = context.WithTimeout(outCtx, outClient.Timeout)
		defer cancel()
	}
	proxyReq = proxyReq.WithContext(outCtx)

	// Execute request
	resp, err := outClient.Do(proxyReq)
	if err != nil {
		http.Error(w, "Failed to execute proxy request", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy response headers
	for key, values := range resp.Header {
		if isHopHeader(key) {
			continue
		}

		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, _ = io.Copy(w, resp.Body)
}

// sanitizeProxyPath sanitizes and validates a path component for proxying
// Prevents path traversal, request smuggling, and other attacks
func sanitizeProxyPath(path string) string {
	// Reject empty paths
	if path == "" {
		return "/"
	}

	// Reject null bytes (could be used for path truncation)
	if strings.Contains(path, "\x00") {
		return ""
	}

	// Reject paths with newlines or carriage returns (HTTP smuggling)
	if strings.ContainsAny(path, "\r\n") {
		return ""
	}

	// Check for path traversal attempts BEFORE cleaning
	// Split by / and check each component
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == ".." {
			return ""
		}
	}

	// Clean the path to resolve . and normalize separators.
	// Use path.Clean (forward-slash semantics) not filepath.Clean —
	// filepath.Clean converts / to \ on Windows, corrupting URL paths.
	cleaned := pathpkg.Clean(path)

	// Double-check after cleaning (belt and suspenders)
	if strings.Contains(cleaned, "..") {
		return ""
	}

	// Ensure path starts with /
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}

	return cleaned
}

// isHopHeader checks if a header is hop-by-hop
func isHopHeader(header string) bool {
	switch header {
	case "Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade":
		return true
	}
	return false
}
