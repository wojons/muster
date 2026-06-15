package daemon

import (
	"strings"
	"sync"
	"testing"
)

// --- sanitizeProxyPath tests ---

func TestSanitizeProxyPath_NormalPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty root", "", "/"},
		{"root path", "/", "/"},
		{"simple path", "/tasks", "/tasks"},
		{"nested path", "/tasks/123", "/tasks/123"},
		{"deep path", "/a/b/c/d", "/a/b/c/d"},
		{"trailing slash", "/tasks/", "/tasks"},
		{"double slash", "/tasks//123", "/tasks/123"},
		{"missing leading slash", "tasks", "/tasks"},
		{"dot segment", "/tasks/./123", "/tasks/123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeProxyPath(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeProxyPath(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestSanitizeProxyPath_RejectedPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"null byte", "/tasks\x00secret"},
		{"newline", "/tasks\nsecret"},
		{"carriage return", "/tasks\rsecret"},
		{"CRLF", "/tasks\r\nsecret"},
		{"path traversal ..", "/../etc/passwd"},
		{"embedded traversal", "/tasks/../secret"},
		{"deep traversal", "/a/b/../../c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeProxyPath(tc.input)
			if result != "" {
				t.Errorf("sanitizeProxyPath(%q) = %q, want empty string (rejected)", tc.input, result)
			}
		})
	}
}

// --- isHopHeader tests ---

func TestIsHopHeader_HopHeaders(t *testing.T) {
	hopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, h := range hopHeaders {
		t.Run(h, func(t *testing.T) {
			if !isHopHeader(h) {
				t.Errorf("isHopHeader(%q) = false, want true", h)
			}
		})
	}
}

func TestIsHopHeader_NonHopHeaders(t *testing.T) {
	nonHopHeaders := []string{
		"Content-Type",
		"Content-Length",
		"Authorization",
		"Accept",
		"User-Agent",
		"Host",
		"",
	}
	for _, h := range nonHopHeaders {
		t.Run(h, func(t *testing.T) {
			if isHopHeader(h) {
				t.Errorf("isHopHeader(%q) = true, want false", h)
			}
		})
	}
}

func TestIsHopHeader_CaseSensitive(t *testing.T) {
	if isHopHeader("connection") {
		t.Error("isHopHeader should be case-sensitive; 'connection' should be false")
	}
	if isHopHeader("CONNECTION") {
		t.Error("isHopHeader should be case-sensitive; 'CONNECTION' should be false")
	}
}

// --- isHealthEndpoint tests ---

func TestIsHealthEndpoint_True(t *testing.T) {
	healthPaths := []string{"/health", "/health/", "/health/live", "/health/ready", "/health/startup"}
	for _, p := range healthPaths {
		t.Run(p, func(t *testing.T) {
			if !isHealthEndpoint(p) {
				t.Errorf("isHealthEndpoint(%q) = false, want true", p)
			}
		})
	}
}

func TestIsHealthEndpoint_False(t *testing.T) {
	nonHealthPaths := []string{"/tasks", "/api/v1", "/health/foo", "", "/"}
	for _, p := range nonHealthPaths {
		t.Run(p, func(t *testing.T) {
			if isHealthEndpoint(p) {
				t.Errorf("isHealthEndpoint(%q) = true, want false", p)
			}
		})
	}
}

// --- TokenManager tests ---

func TestTokenManager_NewTokenManager(t *testing.T) {
	tm := NewTokenManager()
	if tm == nil {
		t.Fatal("NewTokenManager() returned nil")
	}
	if len(tm.tokens) != 0 {
		t.Errorf("expected empty token store, got %d tokens", len(tm.tokens))
	}
}

func TestTokenManager_AddAndValidate(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("secret-123")
	if !tm.Validate("secret-123") {
		t.Error("Validate should return true for added token")
	}
	if tm.Validate("wrong-token") {
		t.Error("Validate should return false for unknown token")
	}
}

func TestTokenManager_RemoveToken(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("temp-token")
	if !tm.Validate("temp-token") {
		t.Fatal("token should be valid before removal")
	}
	tm.RemoveToken("temp-token")
	if tm.Validate("temp-token") {
		t.Error("token should be invalid after removal")
	}
}

func TestTokenManager_RemoveNonExistent(t *testing.T) {
	tm := NewTokenManager()
	tm.RemoveToken("never-added") // should not panic
	if tm.Validate("never-added") {
		t.Error("token should not exist")
	}
}

func TestTokenManager_ValidateFromHeader(t *testing.T) {
	tm := NewTokenManager()
	tm.AddToken("bearer-token-abc")

	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"valid Bearer", "Bearer bearer-token-abc", true},
		{"wrong token", "Bearer wrong", false},
		{"missing Bearer prefix", "bearer-token-abc", false},
		{"empty header", "", false},
		{"Basic auth", "Basic YWxhZGRpbjpvcGVuIHNlc2FtZQ==", false},
		{"Bearer with empty token", "Bearer ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tm.ValidateFromHeader(tc.header)
			if result != tc.expected {
				t.Errorf("ValidateFromHeader(%q) = %v, want %v", tc.header, result, tc.expected)
			}
		})
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	tm := NewTokenManager()
	var wg sync.WaitGroup
	n := 100

	// Concurrent adds
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "token-" + strings.Repeat("x", id%10)
			tm.AddToken(key)
		}(i)
	}

	// Concurrent validates
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tm.Validate("non-existent")
		}()
	}

	wg.Wait()
	// If we get here without deadlock/panic, the test passes
}
