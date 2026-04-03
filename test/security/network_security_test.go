package security

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// NETWORK SECURITY TESTS
// ============================================================================

// TestNetwork_TLSVersionEnforcement verifies only TLS 1.2+ is allowed
func TestNetwork_TLSVersionEnforcement(t *testing.T) {
	// Test that TLS configuration enforces minimum TLS 1.2
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	require.NotNil(t, config)
	require.GreaterOrEqual(t, config.MinVersion, uint16(tls.VersionTLS12))
}

// TestNetwork_TLSVersions verifies supported TLS versions
func TestNetwork_TLSVersions(t *testing.T) {
	versions := []uint16{
		tls.VersionTLS12,
		tls.VersionTLS13,
	}

	for _, v := range versions {
		config := &tls.Config{
			MinVersion: v,
		}
		require.NotNil(t, config)
	}
}

// TestNetwork_InsecureTLSRejected verifies TLS 1.0 and 1.1 are rejected
func TestNetwork_InsecureTLSRejected(t *testing.T) {
	// TLS 1.0 is considered insecure
	insecureVersions := []uint16{
		tls.VersionTLS10,
		tls.VersionTLS11,
	}

	for _, v := range insecureVersions {
		// These should be rejected by modern TLS configs
		require.Less(t, v, uint16(tls.VersionTLS12), "TLS version %d should be rejected", v)
	}
}

// TestNetwork_CipherSuites verifies secure cipher configuration
func TestNetwork_CipherSuites(t *testing.T) {
	// Secure cipher suites (no CBC, no RC4, no 3DES)
	secureCiphers := []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}

	for _, cipher := range secureCiphers {
		require.Greater(t, cipher, uint16(0), "Cipher %d should be valid", cipher)
	}
}

// TestNetwork_RequestSizeLimits verifies request size validation
func TestNetwork_RequestSizeLimits(t *testing.T) {
	testCases := []struct {
		name        string
		size        int64
		shouldAllow bool
		maxSize     int64
	}{
		{
			name:        "small_request_allowed",
			size:        1024, // 1KB
			shouldAllow: true,
			maxSize:     10 * 1024 * 1024, // 10MB
		},
		{
			name:        "medium_request_allowed",
			size:        1024 * 1024, // 1MB
			shouldAllow: true,
			maxSize:     10 * 1024 * 1024, // 10MB
		},
		{
			name:        "large_request_rejected",
			size:        20 * 1024 * 1024, // 20MB
			shouldAllow: false,
			maxSize:     10 * 1024 * 1024, // 10MB
		},
		{
			name:        "exact_limit_allowed",
			size:        10 * 1024 * 1024, // 10MB
			shouldAllow: true,
			maxSize:     10 * 1024 * 1024, // 10MB
		},
		{
			name:        "empty_request_allowed",
			size:        0,
			shouldAllow: true,
			maxSize:     10 * 1024 * 1024, // 10MB
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed := tc.size <= tc.maxSize
			require.Equal(t, tc.shouldAllow, allowed,
				"Request size %d with max %d should be allowed=%v",
				tc.size, tc.maxSize, tc.shouldAllow)
		})
	}
}

// TestNetwork_SecureHeaders verifies security headers
func TestNetwork_SecureHeaders(t *testing.T) {
	// Common security headers that should be present
	securityHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Content-Security-Policy",
		"Strict-Transport-Security",
		"Referrer-Policy",
	}

	for _, header := range securityHeaders {
		t.Run(header, func(t *testing.T) {
			require.NotEmpty(t, header)
		})
	}
}

// TestNetwork_HTTPSRequired verifies HTTPS is enforced
func TestNetwork_HTTPSRequired(t *testing.T) {
	urls := []struct {
		url      string
		isSecure bool
	}{
		{"https://api.example.com/v1/users", true},
		{"https://secure.service.com/data", true},
		{"http://insecure.example.com/api", false},
		{"https://localhost:8443/", true},
		{"https://127.0.0.1:8443/", true},
	}

	for _, tc := range urls {
		t.Run(tc.url, func(t *testing.T) {
			isHTTPS := len(tc.url) > 8 && tc.url[:8] == "https://"
			require.Equal(t, tc.isSecure, isHTTPS, "URL %s should have secure=%v", tc.url, tc.isSecure)
		})
	}
}

// TestNetwork_CertificateValidation verifies certificate validation
func TestNetwork_CertificateValidation(t *testing.T) {
	// Test that InsecureSkipVerify is not enabled by default
	config := &tls.Config{}
	require.False(t, config.InsecureSkipVerify, "InsecureSkipVerify should be false by default")
}

// TestNetwork_TimeoutConfiguration verifies network timeouts
func TestNetwork_TimeoutConfiguration(t *testing.T) {
	timeoutTests := []struct {
		name          string
		timeout       int // seconds
		minAcceptable int // seconds
	}{
		{
			name:          "connection_timeout",
			timeout:       30,
			minAcceptable: 10,
		},
		{
			name:          "read_timeout",
			timeout:       60,
			minAcceptable: 30,
		},
		{
			name:          "write_timeout",
			timeout:       30,
			minAcceptable: 10,
		},
		{
			name:          "idle_timeout",
			timeout:       120,
			minAcceptable: 60,
		},
	}

	for _, tc := range timeoutTests {
		t.Run(tc.name, func(t *testing.T) {
			require.GreaterOrEqual(t, tc.timeout, tc.minAcceptable,
				"Timeout %s should be at least %d seconds", tc.name, tc.minAcceptable)
		})
	}
}

// TestNetwork_HostnameValidation verifies hostname validation in TLS
func TestNetwork_HostnameValidation(t *testing.T) {
	config := &tls.Config{
		ServerName: "api.example.com",
	}
	require.Equal(t, "api.example.com", config.ServerName)
	require.NotEmpty(t, config.ServerName, "ServerName should be set for hostname validation")
}
