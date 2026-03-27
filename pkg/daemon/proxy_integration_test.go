package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestAuthenticatedSocket_FullLifecycle tests the complete token lifecycle
// from generation through cleanup with socket authentication
func TestAuthenticatedSocket_FullLifecycle(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-lifecycle-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-lifecycle-"+generateTestID(8)+".token")

	defer func() {
		_ = os.Remove(socketPath)
		_ = os.Remove(tokenPath)
	}()

	// Step 1: Create and generate token
	tokenManager := NewTokenManager(tokenPath)
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if !tokenManager.IsGenerated() {
		t.Fatal("Token should be generated")
	}

	token := tokenManager.GetToken()
	if len(token) != 43 {
		t.Fatalf("Token length should be 43, got %d", len(token))
	}

	// Step 2: Save token to file
	if err := tokenManager.Save(); err != nil {
		t.Fatalf("Failed to save token: %v", err)
	}

	// Step 3: Load token from file (simulating client reading)
	loadedToken, err := tokenManager.Load()
	if err != nil {
		t.Fatalf("Failed to load token: %v", err)
	}

	if loadedToken != token {
		t.Fatal("Loaded token should match generated token")
	}

	// Step 4: Start authenticated server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authenticated":true}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Step 5: Test unauthorized request
	client := createTestUnixSocketClient(socketPath)
	req, _ := http.NewRequest("GET", "http://localhost/protected", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthorized request, got %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()

	// Step 6: Test authorized request
	req, _ = http.NewRequest("GET", "http://localhost/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 200 for authorized request, got %d. Body: %s", resp.StatusCode, string(body))
	}
	defer func() { _ = resp.Body.Close() }()

	// Step 7: Stop server and cleanup
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	if err := tokenManager.Cleanup(); err != nil {
		t.Fatalf("Failed to cleanup token: %v", err)
	}

	if tokenManager.IsGenerated() {
		t.Error("Token should be cleared after cleanup")
	}

	if tokenManager.FileExists() {
		t.Error("Token file should be removed after cleanup")
	}
}

// TestAuthenticatedSocket_ConcurrentConnections tests multiple concurrent authenticated requests
func TestAuthenticatedSocket_ConcurrentConnections(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-concurrent-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-concurrent-"+generateTestID(8)+".token")

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
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(50 * time.Millisecond)

	// Run 50 concurrent requests
	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			client := createTestUnixSocketClient(socketPath)
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost/test/%d", id), nil)
			req.Header.Set("Authorization", "Bearer "+tokenManager.GetToken())

			resp, err := client.Do(req)
			if err != nil {
				errors <- fmt.Errorf("request %d failed: %v", id, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				errors <- fmt.Errorf("request %d returned status %d: %s", id, resp.StatusCode, string(body))
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors from 50 concurrent requests", errorCount)
	}
}

// TestAuthenticatedSocket_TokenRotation tests token rotation during runtime
func TestAuthenticatedSocket_TokenRotation(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-rotation-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-rotation-"+generateTestID(8)+".token")

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

	originalToken := tokenManager.GetToken()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authenticated":true}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(50 * time.Millisecond)

	// Test with original token
	client := createTestUnixSocketClient(socketPath)
	req, _ := http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer "+originalToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 with original token, got %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()

	// Rotate token - this invalidates the old token
	if err := tokenManager.Generate(); err != nil {
		t.Fatalf("Failed to rotate token: %v", err)
	}
	newToken := tokenManager.GetToken()

	// Old token should be rejected after rotation
	req, _ = http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer "+originalToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Original token should be rejected after rotation, got status %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()

	// New token should work
	req, _ = http.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("Authorization", "Bearer "+newToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("New token should work, got status %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()
}

// TestAuthenticatedSocket_MultipleEndpoints tests authentication across different endpoints
func TestAuthenticatedSocket_MultipleEndpoints(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-multi-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-multi-"+generateTestID(8)+".token")

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

	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"endpoint":"users"}`))
	})
	mux.HandleFunc("/api/items", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"endpoint":"items"}`))
	})
	mux.HandleFunc("/api/admin", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"endpoint":"admin"}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, mux, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(50 * time.Millisecond)

	endpoints := []string{"/api/users", "/api/items", "/api/admin"}
	client := createTestUnixSocketClient(socketPath)

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			// Without token - should fail
			req, _ := http.NewRequest("GET", "http://localhost"+endpoint, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 without token, got %d", resp.StatusCode)
			}
			defer func() { _ = resp.Body.Close() }()

			// With token - should succeed
			req, _ = http.NewRequest("GET", "http://localhost"+endpoint, nil)
			req.Header.Set("Authorization", "Bearer "+tokenManager.GetToken())
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected 200 with token, got %d. Body: %s", resp.StatusCode, string(body))
			}
			defer func() { _ = resp.Body.Close() }()
		})
	}
}

// TestAuthenticatedSocket_InvalidTokens tests various invalid token scenarios
func TestAuthenticatedSocket_InvalidTokens(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-invalid-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-invalid-"+generateTestID(8)+".token")

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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(50 * time.Millisecond)

	testCases := []struct {
		name       string
		authHeader string
		expectCode int
	}{
		{
			name:       "Empty header",
			authHeader: "",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Missing Bearer prefix",
			authHeader: tokenManager.GetToken(),
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Wrong prefix Basic",
			authHeader: "Basic " + tokenManager.GetToken(),
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Completely wrong token",
			authHeader: "Bearer invalid-token-12345",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Bearer with empty token",
			authHeader: "Bearer ",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Bearer with whitespace only",
			authHeader: "Bearer    ",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Valid token",
			authHeader: "Bearer " + tokenManager.GetToken(),
			expectCode: http.StatusOK,
		},
	}

	client := createTestUnixSocketClient(socketPath)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://localhost/test", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tc.expectCode {
				t.Errorf("Expected status %d, got %d", tc.expectCode, resp.StatusCode)
			}
		})
	}
}

// TestAuthenticatedSocket_ConnectionPersistence tests persistent connections
func TestAuthenticatedSocket_ConnectionPersistence(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-persist-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-persist-"+generateTestID(8)+".token")

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

	requestCount := 0
	var mu sync.Mutex

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		count := requestCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"request":%d}`, count)
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(50 * time.Millisecond)

	client := createTestUnixSocketClient(socketPath)
	token := tokenManager.GetToken()

	// Make multiple requests over the same connection
	for i := 1; i <= 5; i++ {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost/test/%d", i), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d. Body: %s", i, resp.StatusCode, string(body))
		}
	}

	mu.Lock()
	finalCount := requestCount
	mu.Unlock()

	if finalCount != 5 {
		t.Errorf("Expected 5 requests processed, got %d", finalCount)
	}
}

// TestAuthenticatedSocket_CleanupOnStop tests that token is cleaned up when server stops
func TestAuthenticatedSocket_CleanupOnStop(t *testing.T) {
	socketPath := filepath.Join("/tmp", "muster-cleanup-"+generateTestID(8)+".sock")
	tokenPath := filepath.Join("/tmp", "muster-cleanup-"+generateTestID(8)+".token")

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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	server := NewAuthenticatedProxyServer(socketPath, handler, tokenManager)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Verify socket and token file exist
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatal("Socket file should exist after start")
	}
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Fatal("Token file should exist after start")
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// Note: Stop() removes the socket but doesn't clean up the token
	// Token cleanup is the responsibility of the controller/token manager
	if err := tokenManager.Cleanup(); err != nil {
		t.Fatalf("Failed to cleanup token: %v", err)
	}

	// Verify cleanup
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file should be removed after stop")
	}
	if tokenManager.FileExists() {
		t.Error("Token file should be removed after cleanup")
	}
}

// Helper functions

func createTestUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func generateTestID(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
	}
	return hex.EncodeToString(bytes)[:length]
}
