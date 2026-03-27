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
