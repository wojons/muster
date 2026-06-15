package daemon

import (
	"sync"
	"testing"
)

func TestTokenManager_MultipleTokens(t *testing.T) {
	tm := NewTokenManager()
	tokens := []string{"alpha", "beta", "gamma", "delta"}
	for _, tok := range tokens {
		tm.AddToken(tok)
	}
	for _, tok := range tokens {
		if !tm.Validate(tok) {
			t.Errorf("Validate(%q) = false, want true", tok)
		}
	}
	if tm.Validate("epsilon") {
		t.Error("Validate should return false for unregistered token")
	}
}

func TestTokenManager_RemoveAllTokens(t *testing.T) {
	tm := NewTokenManager()
	tokens := []string{"a", "b", "c"}
	for _, tok := range tokens {
		tm.AddToken(tok)
	}
	for _, tok := range tokens {
		tm.RemoveToken(tok)
	}
	for _, tok := range tokens {
		if tm.Validate(tok) {
			t.Errorf("Validate(%q) = true after removal, want false", tok)
		}
	}
}

func TestTokenManager_EmptyToken(t *testing.T) {
	tm := NewTokenManager()
	if tm.Validate("") {
		t.Error("Validate should return false for empty string")
	}
	tm.AddToken("")
	if !tm.Validate("") {
		t.Error("Validate should return true after adding empty string")
	}
}

func TestTokenManager_RaceConditions(t *testing.T) {
	tm := NewTokenManager()
	var wg sync.WaitGroup
	iterations := 50

	// Add, validate, and remove concurrently from many goroutines
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(grp int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch i % 3 {
				case 0:
					tm.AddToken("race-token")
				case 1:
					_ = tm.Validate("race-token")
				case 2:
					tm.RemoveToken("race-token")
				}
			}
		}(g)
	}
	wg.Wait()
	// No deadlock or panic = pass
}

func TestIsHopHeader_EdgeCases(t *testing.T) {
	// Exact match required — substrings should NOT match
	if isHopHeader("Connection-Close") {
		t.Error("isHopHeader should require exact match")
	}
	if isHopHeader("Transfer-Encoding-Extra") {
		t.Error("isHopHeader should require exact match")
	}
	// Space-prefixed — should NOT match
	if isHopHeader(" Connection") {
		t.Error("isHopHeader should not match space-prefixed headers")
	}
}

func TestSanitizeProxyPath_EdgeCases(t *testing.T) {
	// Path with only slashes
	result := sanitizeProxyPath("////")
	if result != "/" {
		t.Errorf("sanitizeProxyPath(\"////\") = %q, want \"/\"", result)
	}

	// Path with dots in legitimate names
	result = sanitizeProxyPath("/file.txt")
	if result != "/file.txt" {
		t.Errorf("sanitizeProxyPath(%q) = %q, want %q", "/file.txt", result, "/file.txt")
	}

	// Path with dots as directory names (not ..)
	result = sanitizeProxyPath("/config/.hidden/file")
	if result != "/config/.hidden/file" {
		t.Errorf("sanitizeProxyPath(%q) = %q, want %q", "/config/.hidden/file", result, "/config/.hidden/file")
	}
}

func TestIsHealthEndpoint_SubstringMatch(t *testing.T) {
	// /health-check should NOT be a health endpoint
	if isHealthEndpoint("/health-check") {
		t.Error("isHealthEndpoint should not match /health-check")
	}
	if isHealthEndpoint("/healthy") {
		t.Error("isHealthEndpoint should not match /healthy")
	}
}
