package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAuthenticatedProxyServer(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)

	if server.socketPath != socketPath {
		t.Errorf("Expected socket path %s, got %s", socketPath, server.socketPath)
	}

	if server.tokenManager != tokenManager {
		t.Error("Expected token manager to be set")
	}

	if server.handler == nil {
		t.Error("Expected handler to be set")
	}
}

func TestWithTokenAuth_ValidToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	authHandler := withTokenAuth(handler, tokenManager)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenManager.GetToken())
	rr := httptest.NewRecorder()

	authHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	expected := `{"message":"success"}`
	if rr.Body.String() != expected {
		t.Errorf("Expected body %s, got %s", expected, rr.Body.String())
	}
}

func TestWithTokenAuth_MissingToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	authHandler := withTokenAuth(handler, tokenManager)

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No Authorization header
	rr := httptest.NewRecorder()

	authHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	body := rr.Body.String()
	if !contains(body, "Missing authorization header") {
		t.Errorf("Expected error message about missing header, got: %s", body)
	}
}

func TestWithTokenAuth_InvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	authHandler := withTokenAuth(handler, tokenManager)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-12345")
	rr := httptest.NewRecorder()

	authHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	body := rr.Body.String()
	if !contains(body, "Invalid or expired token") {
		t.Errorf("Expected error message about invalid token, got: %s", body)
	}
}

func TestWithTokenAuth_MalformedHeader(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	authHandler := withTokenAuth(handler, tokenManager)

	testCases := []struct {
		name   string
		header string
	}{
		{
			name:   "Missing Bearer prefix",
			header: tokenManager.GetToken(),
		},
		{
			name:   "Wrong prefix",
			header: "Basic " + tokenManager.GetToken(),
		},
		{
			name:   "Empty token",
			header: "Bearer ",
		},
		{
			name:   "Just whitespace",
			header: "Bearer    ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/test", nil)
			req.Header.Set("Authorization", tc.header)
			rr := httptest.NewRecorder()

			authHandler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestWithTokenAuth_HealthEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test.token")

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	authHandler := withTokenAuth(handler, tokenManager)

	healthEndpoints := []string{
		"/health",
		"/health/",
		"/health/live",
		"/health/ready",
		"/health/startup",
	}

	for _, endpoint := range healthEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest("GET", endpoint, nil)
			// No Authorization header for health checks
			rr := httptest.NewRecorder()

			authHandler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status %d for health endpoint, got %d", http.StatusOK, rr.Code)
			}
		})
	}
}

func TestIsHealthEndpoint(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/health/", true},
		{"/health/live", true},
		{"/health/ready", true},
		{"/health/startup", true},
		{"/api/test", false},
		{"/api/health", false},
		{"/health/extra", false},
		{"/", false},
		{"/api", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isHealthEndpoint(tc.path)
			if result != tc.expected {
				t.Errorf("isHealthEndpoint(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestProxyServer_Authentication_Integration(t *testing.T) {
	// Use /tmp with short name to avoid macOS socket path length limit (104 chars)
	socketPath := filepath.Join("/tmp", "muster-test-"+generateRandomString(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-test-"+generateRandomString(8)+".token")

	// Cleanup after test
	defer func() {
		_ = os.Remove(socketPath)
		_ = os.Remove(tokenPath)
	}()

	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	if err := tokenManager.Save(); err != nil {
		t.Fatalf("Failed to save token: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"authenticated"}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		_ = server.Stop()
	}()

	time.Sleep(50 * time.Millisecond)

	// Test 1: Request without token should fail
	t.Run("Unauthorized without token", func(t *testing.T) {
		client := createUnixSocketClient(socketPath)
		req, _ := http.NewRequest("GET", "http://localhost/api/test", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}
	})

	// Test 2: Request with valid token should succeed
	t.Run("Authorized with token", func(t *testing.T) {
		client := createUnixSocketClient(socketPath)
		req, _ := http.NewRequest("GET", "http://localhost/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenManager.GetToken())
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, resp.StatusCode, string(body))
		}
	})

	// Test 3: Health endpoint should not require token
	t.Run("Health without token", func(t *testing.T) {
		client := createUnixSocketClient(socketPath)
		req, _ := http.NewRequest("GET", "http://localhost/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
	})
}

func TestNewProxyServer_WithoutAuth(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	server := NewProxyServer(socketPath, handler)

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		_ = server.Stop()
	}()

	time.Sleep(50 * time.Millisecond)

	// Without authentication, requests should succeed without token
	client := createUnixSocketClient(socketPath)
	req, _ := http.NewRequest("GET", "http://localhost/api/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

// Helper functions

func createUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// generateRandomString generates a random hex string of specified length
func generateRandomString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
	}
	return hex.EncodeToString(bytes)[:length]
}

// TestSanitizeProxyPath tests the path sanitization function
func TestSanitizeProxyPath(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_path",
			input:    "",
			expected: "/",
		},
		{
			name:     "normal_path",
			input:    "/api/users",
			expected: "/api/users",
		},
		{
			name:     "path_with_trailing_slash",
			input:    "/api/users/",
			expected: "/api/users", // filepath.Clean removes trailing slash
		},
		{
			name:     "path_with_double_dot",
			input:    "/api/../admin",
			expected: "", // Should be rejected
		},
		{
			name:     "path_with_multiple_double_dots",
			input:    "/api/../../etc/passwd",
			expected: "", // Should be rejected
		},
		{
			name:     "path_with_null_byte",
			input:    "/api/users\x00.txt",
			expected: "", // Should be rejected
		},
		{
			name:     "path_with_newline",
			input:    "/api/users\n",
			expected: "", // Should be rejected
		},
		{
			name:     "path_with_carriage_return",
			input:    "/api/users\r",
			expected: "", // Should be rejected
		},
		{
			name:     "path_without_leading_slash",
			input:    "api/users",
			expected: "/api/users",
		},
		{
			name:     "complex_valid_path",
			input:    "/api/v1/users/123/profile",
			expected: "/api/v1/users/123/profile",
		},
		{
			name:     "path_with_single_dot",
			input:    "/api/./users",
			expected: "/api/users", // Single dot is OK, gets cleaned
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeProxyPath(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeProxyPath(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestProxyHandler_RequestSmuggling tests that request smuggling attacks are prevented
func TestProxyHandler_RequestSmuggling(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"path":%q, "method":%q}`, r.URL.Path, r.Method)
	}))
	defer backend.Close()

	handler := NewProxyHandler(backend.URL)

	testCases := []struct {
		name           string
		path           string
		query          string
		expectedStatus int
		description    string
	}{
		{
			name:           "normal_request",
			path:           "/api/users",
			query:          "",
			expectedStatus: http.StatusOK,
			description:    "Normal path should work",
		},
		{
			name:           "path_with_query",
			path:           "/api/users",
			query:          "limit=10",
			expectedStatus: http.StatusOK,
			description:    "Path with query string should work",
		},
		{
			name:           "path_traversal_attempt",
			path:           "/api/../admin",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			description:    "Path traversal should be rejected",
		},
		{
			name:           "double_path_traversal",
			path:           "/api/../../etc/passwd",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			description:    "Double path traversal should be rejected",
		},
		// Skip null byte test - httptest.NewRequest panics with control characters
		// The sanitizeProxyPath function is tested separately in TestSanitizeProxyPath
		// Newline and carriage return tests removed - httptest.NewRequest rejects control characters
		// These are tested in TestSanitizeProxyPath instead
		{
			name:           "encoded_path_traversal",
			path:           "/api/%2e%2e/admin",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			description:    "Encoded traversal should be rejected by sanitizeProxyPath",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.query != "" {
				req.URL.RawQuery = tc.query
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d. Body: %s",
					tc.description, tc.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestProxyHandler_URLValidation tests that the proxy validates target URLs
func TestProxyHandler_URLValidation(t *testing.T) {
	// Test with invalid backend URL
	t.Run("invalid_backend_url", func(t *testing.T) {
		handler := NewProxyHandler("://invalid-url")
		req := httptest.NewRequest("GET", "/api/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		// Should fail with bad gateway when trying to parse invalid URL
		if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusBadGateway {
			t.Errorf("Expected status 500 or 502 for invalid backend URL, got %d", rr.Code)
		}
	})

	// Test that scheme changes are rejected
	t.Run("scheme_change_rejected", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		handler := NewProxyHandler(backend.URL)
		req := httptest.NewRequest("GET", "/api/test", nil)
		// Try to inject a different scheme (should be prevented by url.JoinPath)
		req.URL.Scheme = "file"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		// Should succeed because url.JoinPath preserves the backend scheme
		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}

// TestProxyHandler_PathSanitization_Integration tests path sanitization in integration
func TestProxyHandler_PathSanitization_Integration(t *testing.T) {
	requestSmugglingAttempts := []string{
		"/api/../../../etc/passwd",
		"/api/..\\..\\..\\windows\\system32\\config\\sam",
		// Removed control character tests - httptest.NewRequest rejects them
		"/....//....//....//etc/passwd",
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Backend should only receive clean paths
		if len(r.URL.Path) > 100 {
			t.Errorf("Backend received suspiciously long path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer backend.Close()

	handler := NewProxyHandler(backend.URL)

	for i, attempt := range requestSmugglingAttempts {
		t.Run(fmt.Sprintf("attempt_%d", i+1), func(t *testing.T) {
			req := httptest.NewRequest("GET", attempt, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			// All smuggling attempts should be rejected
			if rr.Code != http.StatusBadRequest {
				t.Errorf("Request smuggling attempt %d (%s) should be rejected with 400, got %d. Body: %s",
					i+1, attempt, rr.Code, rr.Body.String())
			}
		})
	}
}
