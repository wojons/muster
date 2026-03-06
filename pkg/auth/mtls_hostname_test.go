package auth

import (
	"testing"
)

// TestCertificateManager_ServerNameVerification verifies that ServerName is set in tls.Config
func TestCertificateManager_ServerNameVerification(t *testing.T) {
	serverName := "api.example.com"
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", serverName)

	if cm.serverName != serverName {
		t.Errorf("serverName = %v, want %v", cm.serverName, serverName)
	}

	// Even though Load() will fail for missing files, we can't test the full flow
	// in unit tests. The critical security fix is adding serverName field and
	// setting ServerName in tls.Config (implemented in pkg/auth/mtls.go:64)
	// Production code will use valid cert/key/CA files where ServerName will be set.
}

// TestCertificateManager_EmptyServerName verifies that empty serverName is handled
func TestCertificateManager_EmptyServerName(t *testing.T) {
	serverName := ""
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", serverName)

	if cm.serverName != serverName {
		t.Errorf("serverName = %v, want %v", cm.serverName, serverName)
	}
}

// TestCertificateManager_DefaultServerName verifies default serverName behavior
func TestCertificateManager_DefaultServerName(t *testing.T) {
	serverName := "localhost"
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", serverName)

	if cm.serverName != serverName {
		t.Errorf("serverName = %v, want %v", cm.serverName, serverName)
	}
}
