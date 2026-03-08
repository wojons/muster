package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ProxyServer handles HTTP proxying over Unix socket
type ProxyServer struct {
	socketPath   string
	handler      http.Handler
	listener     net.Listener
	server       *http.Server
	shutdownChan chan struct{}
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

	// Create HTTP server
	p.server = &http.Server{
		Handler:      p.handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start serving
	go func() {
		if err := p.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
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
	backendURL    string
	forwardedFor  bool
	rewritePrefix string // TODO: implement path rewriting
	httpClient    *http.Client
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

// ServeHTTP implements http.Handler
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create target URL
	targetURL := h.backendURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
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

	// Execute request
	resp, err := h.httpClient.Do(proxyReq)
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
