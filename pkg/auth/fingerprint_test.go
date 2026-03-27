package auth

import (
	"crypto/tls"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
)

func TestCalculateJA3_Nil(t *testing.T) {
	ja3 := CalculateJA3(nil)
	if ja3 != "" {
		t.Errorf("CalculateJA3(nil) = %v, want empty", ja3)
	}
}

func TestCalculateJA3_TLS12(t *testing.T) {
	tlsState := &tls.ConnectionState{
		Version: tls.VersionTLS12,
	}

	ja3 := CalculateJA3(tlsState)
	if ja3 == "" {
		t.Fatal("CalculateJA3() returned empty")
	}

	if len(ja3) == 0 {
		t.Fatal("CalculateJA3() returned empty result")
	}
}

func TestCalculateJA3_TLS13(t *testing.T) {
	tlsState := &tls.ConnectionState{
		Version: tls.VersionTLS13,
	}

	ja3 := CalculateJA3(tlsState)
	if ja3 == "" {
		t.Fatal("CalculateJA3() returned empty")
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"192.168.1.1, 10.0.0.1"},
			"X-Real-Ip":       []string{"10.0.0.2"},
		},
		RemoteAddr: "127.0.0.1:12345",
	}

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() = %v, want 192.168.1.1", ip)
	}
}

func TestClientIP_XRealIp(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Real-Ip": []string{"10.0.0.2"},
		},
		RemoteAddr: "127.0.0.1:12345",
	}

	ip := getClientIP(req)
	if ip != "10.0.0.2" {
		t.Errorf("getClientIP() = %v, want 10.0.0.2", ip)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "127.0.0.1:12345",
	}

	ip := getClientIP(req)
	if ip != "127.0.0.1" {
		t.Errorf("getClientIP() = %v, want 127.0.0.1 (port stripped)", ip)
	}
}

func TestFingerprintRequest_Nil(t *testing.T) {
	fp := FingerprintRequest(nil)
	if fp.IP != "" {
		t.Error("FingerprintRequest(nil) should have empty IP")
	}
}

func TestFingerprintRequest_Basic(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"User-Agent": []string{"TestAgent/1.0"},
			textproto.CanonicalMIMEHeaderKey("Accept"): []string{"application/json"},
		},
		RemoteAddr: "192.168.1.1:8080",
	}

	fp := FingerprintRequest(req)

	if fp.IP != "192.168.1.1" {
		t.Errorf("IP = %v, want 192.168.1.1 (port stripped)", fp.IP)
	}

	if fp.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %v, want TestAgent/1.0", fp.UserAgent)
	}

	if fp.Hash == "" {
		t.Error("Hash should not be empty")
	}

	if fp.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestCalculateHash(t *testing.T) {
	fp := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
	}

	hash := fp.calculateHash()
	if hash == "" {
		t.Error("calculateHash() returned empty")
	}

	if len(hash) != 64 {
		t.Errorf("calculateHash() returned hash of length %d, want 64", len(hash))
	}
}

func TestSimilarity_Identical(t *testing.T) {
	fp1 := &Fingerprint{
		Hash: "testhash",
	}
	fp2 := &Fingerprint{
		Hash: "testhash",
	}

	score := fp1.Similarity(fp2)
	if score != 1.0 {
		t.Errorf("Similarity() = %v, want 1.0", score)
	}
}

func TestSimilarity_Different(t *testing.T) {
	fp1 := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
		Hash:      "hash1",
	}
	fp2 := &Fingerprint{
		IP:        "10.0.0.1",
		UserAgent: "OtherAgent/2.0",
		Headers:   map[string]string{"Accept": "text/html"},
		Hash:      "hash2",
	}

	score := fp1.Similarity(fp2)
	if score > 0.5 {
		t.Errorf("Similarity() = %v, want <= 0.5", score)
	}
}

func TestCalculateFingerprintHash(t *testing.T) {
	hash := CalculateFingerprintHash(
		"192.168.1.1",
		"TestAgent/1.0",
		map[string]string{"Accept": "application/json"},
	)

	if hash == "" {
		t.Error("CalculateFingerprintHash() returned empty")
	}

	if len(hash) != 64 {
		t.Errorf("CalculateFingerprintHash() returned hash of length %d, want 64", len(hash))
	}
}

func TestNewFingerprint(t *testing.T) {
	fp := NewFingerprint(
		"192.168.1.1",
		"TestAgent/1.0",
		map[string]string{"Accept": "application/json"},
	)

	if fp == nil {
		t.Fatal("NewFingerprint() returned nil")
	}

	if fp.IP != "192.168.1.1" {
		t.Errorf("IP = %v, want 192.168.1.1", fp.IP)
	}

	if fp.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %v, want TestAgent/1.0", fp.UserAgent)
	}

	if fp.Hash == "" {
		t.Error("Hash should not be empty")
	}
}

func TestFingerprint_String(t *testing.T) {
	fp := &Fingerprint{
		IP:   "192.168.1.1",
		Hash: "testhash",
	}

	str := fp.String()
	if !strings.Contains(str, "192.168.1.1") {
		t.Errorf("String() = %v, should contain IP", str)
	}

	if !strings.Contains(str, "testhash") {
		t.Errorf("String() = %v, should contain hash", str)
	}
}

func TestTLSFingerprint_String(t *testing.T) {
	tlsInfo := &TLSFingerprint{
		JA3Hash: "ja3hash",
	}

	str := tlsInfo.String()
	if !strings.Contains(str, "ja3hash") {
		t.Errorf("String() = %v, should contain ja3hash", str)
	}
}

func TestFingerprint_calculateHash_Deterministic(t *testing.T) {
	fp := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
	}

	hash1 := fp.calculateHash()
	hash2 := fp.calculateHash()

	if hash1 != hash2 {
		t.Error("calculateHash() should be deterministic")
	}
}

func TestCalculateJA3_Deterministic(t *testing.T) {
	tlsState := &tls.ConnectionState{
		Version: tls.VersionTLS12,
	}

	ja3_1 := CalculateJA3(tlsState)
	ja3_2 := CalculateJA3(tlsState)

	if ja3_1 != ja3_2 {
		t.Error("CalculateJA3() should be deterministic")
	}
}

func TestHMACFingerprint_CollisionResistance_SameContent(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, userAgent, headers)
	hash2 := CalculateFingerprintHash(ip, userAgent, headers)

	if hash1 != hash2 {
		t.Error("Fingerprint hash should be deterministic for same content")
	}
}

func TestHMACFingerprint_CollisionResistance_DifferentIP(t *testing.T) {
	userAgent := "TestAgent/1.0"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash("192.168.1.1", userAgent, headers)
	hash2 := CalculateFingerprintHash("192.168.1.2", userAgent, headers)

	if hash1 == hash2 {
		t.Error("Different IPs should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_DifferentUserAgent(t *testing.T) {
	ip := "192.168.1.1"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, "Agent/1.0", headers)
	hash2 := CalculateFingerprintHash(ip, "Agent/2.0", headers)

	if hash1 == hash2 {
		t.Error("Different UserAgents should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_DifferentHeader(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"

	hash1 := CalculateFingerprintHash(ip, userAgent, map[string]string{"Accept": "application/json"})
	hash2 := CalculateFingerprintHash(ip, userAgent, map[string]string{"Accept": "text/html"})

	if hash1 == hash2 {
		t.Error("Different headers should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_AdditionalHeader(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"
	baseHeaders := map[string]string{"Accept": "application/json"}
	extraHeaders := map[string]string{"Accept": "application/json", "Accept-Language": "en-US"}

	hash1 := CalculateFingerprintHash(ip, userAgent, baseHeaders)
	hash2 := CalculateFingerprintHash(ip, userAgent, extraHeaders)

	if hash1 == hash2 {
		t.Error("Additional headers should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_CaseInsensitiveHeaders(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"

	hash1 := CalculateFingerprintHash(ip, userAgent, map[string]string{"Accept": "application/json"})
	hash2 := CalculateFingerprintHash(ip, userAgent, map[string]string{"accept": "application/json"})

	// Headers are case-insensitive in HTTP, but our map keys are case-sensitive
	// This test documents the current behavior
	if hash1 == hash2 {
		t.Error("Header key case should affect hash (documented limitation)")
	}
}

func TestHMACFingerprint_CollisionResistance_EmptyVsNil(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"

	hash1 := CalculateFingerprintHash(ip, userAgent, nil)
	hash2 := CalculateFingerprintHash(ip, userAgent, map[string]string{})

	// In Go, nil and empty maps behave identically when iterated
	// This is expected behavior - both produce the same hash
	if hash1 != hash2 {
		t.Error("Nil and empty headers should produce the same hash (expected behavior)")
	}
}

func TestHMACFingerprint_CollisionResistance_OrderDependent(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "TestAgent/1.0"

	headers1 := map[string]string{
		"Accept":          "application/json",
		"Accept-Language": "en-US",
	}
	headers2 := map[string]string{
		"Accept-Language": "en-US",
		"Accept":          "application/json",
	}

	hash1 := CalculateFingerprintHash(ip, userAgent, headers1)
	hash2 := CalculateFingerprintHash(ip, userAgent, headers2)

	// Map iteration order is not guaranteed in Go, but the hash should still be
	// based on the same key-value pairs. This test documents the behavior.
	_ = hash1
	_ = hash2
	t.Logf("Hash1: %s, Hash2: %s (map order may vary)", hash1, hash2)
}

func TestHMACFingerprint_CollisionResistance_LengthVariation(t *testing.T) {
	ip1 := "192.168.1.1"
	ip2 := "192.168.1.10"
	userAgent := "TestAgent/1.0"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip1, userAgent, headers)
	hash2 := CalculateFingerprintHash(ip2, userAgent, headers)

	if hash1 == hash2 {
		t.Error("Different length IPs should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_SpecialCharacters(t *testing.T) {
	ip := "192.168.1.1"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, "Agent with spaces/1.0", headers)
	hash2 := CalculateFingerprintHash(ip, "Agent-with-dashes/1.0", headers)

	if hash1 == hash2 {
		t.Error("Special characters in UserAgent should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_Unicode(t *testing.T) {
	ip := "192.168.1.1"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, "Agent/1.0", headers)
	hash2 := CalculateFingerprintHash(ip, "エージェント/1.0", headers)

	if hash1 == hash2 {
		t.Error("Unicode characters should produce different hashes")
	}
}

func TestHMACFingerprint_CollisionResistance_NullByte(t *testing.T) {
	ip := "192.168.1.1"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, "Agent/1.0", headers)
	hash2 := CalculateFingerprintHash(ip, "Agent\x00/1.0", headers)

	if hash1 == hash2 {
		t.Error("Null byte injection should produce different hash")
	}
}

func TestHMACFingerprint_CollisionResistance_NewlineInjection(t *testing.T) {
	ip := "192.168.1.1"
	headers := map[string]string{"Accept": "application/json"}

	hash1 := CalculateFingerprintHash(ip, "Agent/1.0", headers)
	hash2 := CalculateFingerprintHash(ip, "Agent\n/1.0", headers)

	if hash1 == hash2 {
		t.Error("Newline injection should produce different hash")
	}
}

func TestHMACFingerprint_CollisionResistance_HeaderInjection(t *testing.T) {
	ip := "192.168.1.1"
	userAgent := "Agent/1.0"

	hash1 := CalculateFingerprintHash(ip, userAgent, map[string]string{"Accept": "application/json"})
	hash2 := CalculateFingerprintHash(ip, userAgent, map[string]string{"Accept": "application/json\r\nX-Injected: header"})

	if hash1 == hash2 {
		t.Error("Header injection should produce different hash")
	}
}

func TestHMACFingerprint_AttemptedCollision_MultipleAttributes(t *testing.T) {
	hash1 := CalculateFingerprintHash(
		"192.168.1.1",
		"Agent/1.0",
		map[string]string{"Accept": "application/json", "Language": "en"},
	)

	hash2 := CalculateFingerprintHash(
		"192.168.1.2",
		"Agent/2.0",
		map[string]string{"Accept": "text/html", "Language": "fr"},
	)

	if hash1 == hash2 {
		t.Error("Changing multiple attributes should not accidentally collide")
	}
}

func TestFingerprint_WithTLSInfo(t *testing.T) {
	fp1 := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
		TLSInfo: &TLSFingerprint{
			JA3Hash: "ja3hash123",
		},
	}

	fp2 := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
		TLSInfo: &TLSFingerprint{
			JA3Hash: "ja3hash456",
		},
	}

	hash1 := fp1.calculateHash()
	hash2 := fp2.calculateHash()

	if hash1 == hash2 {
		t.Error("Different TLS fingerprints should produce different hashes")
	}
}

func TestFingerprint_WithNilTLSInfo(t *testing.T) {
	fp1 := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
		TLSInfo:   nil,
	}

	fp2 := &Fingerprint{
		IP:        "192.168.1.1",
		UserAgent: "TestAgent/1.0",
		Headers:   map[string]string{"Accept": "application/json"},
		TLSInfo:   nil,
	}

	hash1 := fp1.calculateHash()
	hash2 := fp2.calculateHash()

	if hash1 != hash2 {
		t.Error("Same fingerprints with nil TLS should produce identical hashes")
	}
}

func TestGetFingerprintSecret_NotNil(t *testing.T) {
	secret := getFingerprintSecret()
	if secret == nil {
		t.Error("Fingerprint secret should not be nil")
	}
	if len(secret) == 0 {
		t.Error("Fingerprint secret should not be empty")
	}
}

func TestNewHMAC_NotNil(t *testing.T) {
	h := newHMAC()
	if h == nil {
		t.Error("HMAC should not be nil")
	}
}

func TestIsTrustedProxy_Localhost(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 localhost", "127.0.0.1", true},
		{"IPv6 localhost", "::1", true},
		{"Private IPv4 10.x", "10.0.0.1", true},
		{"Private IPv4 172.16.x", "172.16.0.1", true},
		{"Private IPv4 192.168.x", "192.168.1.1", true},
		{"Public IPv4", "8.8.8.8", false},
		{"Public IPv4 different range", "1.2.3.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTrustedProxy(tt.ip)
			if got != tt.want {
				t.Errorf("IsTrustedProxy(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestAddTrustedProxy(t *testing.T) {
	testIP := "203.0.113.1"

	if IsTrustedProxy(testIP) {
		t.Skipf("Test IP %s is already trusted, skipping", testIP)
	}

	AddTrustedProxy(testIP)

	if !IsTrustedProxy(testIP) {
		t.Errorf("AddTrustedProxy(%q) failed - IP should be trusted", testIP)
	}
}

func TestRemoveTrustedProxy(t *testing.T) {
	testIP := "203.0.113.2"

	AddTrustedProxy(testIP)
	if !IsTrustedProxy(testIP) {
		t.Skipf("Could not add test IP %s", testIP)
	}

	RemoveTrustedProxy(testIP)

	if IsTrustedProxy(testIP) {
		t.Errorf("RemoveTrustedProxy(%q) failed - IP should not be trusted", testIP)
	}
}

func TestGetClientIP_UntrustedProxy_IgnoresXForwardedFor(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"192.168.1.1"},
		},
		RemoteAddr: "8.8.8.8:12345",
	}

	ip := getClientIP(req)
	if ip != "8.8.8.8" {
		t.Errorf("getClientIP() from untrusted proxy = %v, want 8.8.8.8 (ignoring X-Forwarded-For)", ip)
	}
}

func TestGetClientIP_TrustedProxy_UsesXForwardedFor(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"192.168.1.1, 10.0.0.1"},
		},
		RemoteAddr: "127.0.0.1:12345",
	}

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() from trusted proxy = %v, want 192.168.1.1 (first in X-Forwarded-For)", ip)
	}
}

func TestGetClientIP_UntrustedProxy_IgnoresXRealIP(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Real-Ip": []string{"192.168.1.1"},
		},
		RemoteAddr: "8.8.8.8:12345",
	}

	ip := getClientIP(req)
	if ip != "8.8.8.8" {
		t.Errorf("getClientIP() from untrusted proxy = %v, want 8.8.8.8 (ignoring X-Real-IP)", ip)
	}
}

func TestGetClientIP_TrustedProxy_UsesXRealIP(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Real-Ip": []string{"192.168.1.1"},
		},
		RemoteAddr: "127.0.0.1:12345",
	}

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() from trusted proxy = %v, want 192.168.1.1 (from X-Real-IP)", ip)
	}
}

func TestGetClientIP_NoProxyHeaders(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "192.168.1.1:12345",
	}

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() = %v, want 192.168.1.1", ip)
	}
}

func TestGetClientIP_NilRequest(t *testing.T) {
	ip := getClientIP(nil)
	if ip != "" {
		t.Errorf("getClientIP(nil) = %v, want empty string", ip)
	}
}

func TestGetClientIP_RemoteAddrWithoutPort(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "192.168.1.1",
	}

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() = %v, want 192.168.1.1", ip)
	}
}

func TestGetClientIP_IPv6Address(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "[::1]:12345",
	}

	ip := getClientIP(req)
	if ip != "::1" {
		t.Errorf("getClientIP() = %v, want ::1", ip)
	}
}

func TestGetXForwardedFor_SpoofingAttempt_FromExternalIP(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"10.0.0.1"},
		},
		RemoteAddr: "203.0.113.50:8080",
	}

	ip := getClientIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("getClientIP() should reject spoofed X-Forwarded-For from external IP, got %v", ip)
	}
}

func TestGetXForwardedFor_SpoofingAttempt_PrivateNetwork(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"10.0.0.1"},
		},
		RemoteAddr: "192.168.1.100:8080",
	}

	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("getClientIP() should accept X-Forwarded-For from private network, got %v", ip)
	}
}

func TestFingerprintRequest_XForwardedForSpoofing(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"1.2.3.4"},
			"User-Agent":      []string{"TestAgent/1.0"},
		},
		RemoteAddr: "8.8.8.8:12345",
	}

	fp := FingerprintRequest(req)

	if fp.IP != "8.8.8.8" {
		t.Errorf("Fingerprint IP = %v, want 8.8.8.8 (spoofed header ignored)", fp.IP)
	}
}

func TestFingerprintRequest_XForwardedForFromTrustedProxy(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-For": []string{"1.2.3.4"},
			"User-Agent":      []string{"TestAgent/1.0"},
		},
		RemoteAddr: "127.0.0.1:12345",
	}

	fp := FingerprintRequest(req)

	if fp.IP != "1.2.3.4" {
		t.Errorf("Fingerprint IP = %v, want 1.2.3.4 (from trusted proxy)", fp.IP)
	}
}

func TestExtractIP_NoPort(t *testing.T) {
	result := extractIP("192.168.1.1")
	if result != "192.168.1.1" {
		t.Errorf("extractIP() = %v, want 192.168.1.1", result)
	}
}

func TestExtractIP_WithPort(t *testing.T) {
	result := extractIP("192.168.1.1:8080")
	if result != "192.168.1.1" {
		t.Errorf("extractIP() = %v, want 192.168.1.1", result)
	}
}

func TestExtractIP_IPv6(t *testing.T) {
	result := extractIP("[::1]:8080")
	if result != "::1" {
		t.Errorf("extractIP() = %v, want ::1", result)
	}
}
