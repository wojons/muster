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
	if ip != "127.0.0.1:12345" {
		t.Errorf("getClientIP() = %v, want 127.0.0.1:12345", ip)
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

	if fp.IP != "192.168.1.1:8080" {
		t.Errorf("IP = %v, want 192.168.1.1:8080", fp.IP)
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
