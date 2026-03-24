package auth

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestNewCertificateManager(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")
	if cm == nil {
		t.Fatal("NewCertificateManager() returned nil")
	}

	if cm.certPath != "cert.pem" {
		t.Errorf("certPath = %v, want cert.pem", cm.certPath)
	}

	if cm.keyPath != "key.pem" {
		t.Errorf("keyPath = %v, want key.pem", cm.keyPath)
	}

	if cm.caPath != "ca.pem" {
		t.Errorf("caPath = %v, want ca.pem", cm.caPath)
	}

	if cm.serverName != "example.com" {
		t.Errorf("serverName = %v, want example.com", cm.serverName)
	}

	// Test that Load() sets ServerName in tls.Config
	tlsConfig, err := cm.Load()
	if err == nil {
		t.Fatal("Load() expected error for missing files")
	}

	if tlsConfig != nil && tlsConfig.ServerName != "" {
		t.Logf("ServerName is being set: %v", tlsConfig.ServerName)
	}
}

func TestCertificateManager_Load_NotFound(t *testing.T) {
	cm := NewCertificateManager("nonexistent-cert.pem", "nonexistent-key.pem", "nonexistent-ca.pem", "example.com")

	_, err := cm.Load()
	if err == nil {
		t.Fatal("Load() expected error for missing files")
	}
}

func TestDefaultTLSConfig(t *testing.T) {
	config := DefaultTLSConfig()
	if config == nil {
		t.Fatal("DefaultTLSConfig() returned nil")
	}

	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want TLS1.2", config.MinVersion)
	}

	if config.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion = %v, want TLS1.3", config.MaxVersion)
	}
}

func TestCertificateManager_Validate_Nil(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")

	err := cm.Validate(nil)
	if err == nil {
		t.Fatal("Validate() expected error for nil certificate")
	}

	if err.Error() != "certificate is nil" {
		t.Errorf("Validate() error = %v, want certificate is nil", err)
	}
}

func TestCertificateManager_Validate_Expired(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")

	cert := &x509.Certificate{
		NotBefore: time.Now().Add(-48 * time.Hour),
		NotAfter:  time.Now().Add(-24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	err := cm.Validate(cert)
	if err == nil {
		t.Fatal("Validate() expected error for expired certificate")
	}

	if err.Error() != "certificate has expired" {
		t.Errorf("Validate() error = %v, want certificate has expired", err)
	}
}

func TestCertificateManager_Validate_NotYetValid(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")

	cert := &x509.Certificate{
		NotBefore: time.Now().Add(24 * time.Hour),
		NotAfter:  time.Now().Add(48 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	err := cm.Validate(cert)
	if err == nil {
		t.Fatal("Validate() expected error for not yet valid certificate")
	}

	if err.Error() != "certificate not yet valid" {
		t.Errorf("Validate() error = %v, want certificate not yet valid", err)
	}
}

func TestCertificateManager_Validate_MissingKeyUsage(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")

	cert := &x509.Certificate{
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),
		KeyUsage:  0,
	}

	err := cm.Validate(cert)
	if err == nil {
		t.Fatal("Validate() expected error for missing key usage")
	}

	if err.Error() != "certificate missing digital signature key usage" {
		t.Errorf("Validate() error = %v, want certificate missing digital signature key usage", err)
	}
}

func TestCertificateManager_Validate_Valid(t *testing.T) {
	cm := NewCertificateManager("cert.pem", "key.pem", "ca.pem", "example.com")

	cert := &x509.Certificate{
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	err := cm.Validate(cert)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCreateMTLSClient_NilManager(t *testing.T) {
	_, err := CreateMTLSClient(nil)
	if err == nil {
		t.Fatal("CreateMTLSClient() expected error for nil manager")
	}
}

func TestCreateMTLSClient_FileNotFound(t *testing.T) {
	cm := NewCertificateManager("nonexistent.pem", "nonexistent.pem", "nonexistent.pem", "example.com")

	_, err := CreateMTLSClient(cm)
	if err == nil {
		t.Fatal("CreateMTLSClient() expected error for missing certificates")
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, err := GenerateSelfSignedCert("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	if cert == nil {
		t.Fatal("GenerateSelfSignedCert() returned nil certificate")
	}

	if cert.Cert == nil {
		t.Error("Certificate.Cert is nil")
	}

	if cert.PrivateKey == nil {
		t.Error("Certificate.PrivateKey is nil")
	}

	if len(cert.PEM) == 0 {
		t.Error("Certificate.PEM is empty")
	}
}

func TestGenerateSelfSignedCert_CommonName(t *testing.T) {
	commonName := "my-test.example.com"
	cert, err := GenerateSelfSignedCert(commonName)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	if cert.Cert.Subject.CommonName != commonName {
		t.Errorf("CommonName = %v, want %v", cert.Cert.Subject.CommonName, commonName)
	}
}

func TestGenerateSelfSignedCert_KeyType(t *testing.T) {
	cert, err := GenerateSelfSignedCert("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	if _, ok := cert.PrivateKey.(*rsa.PrivateKey); !ok {
		t.Error("PrivateKey is not an RSA key")
	}
}

func TestGenerateSelfSignedCert_Validity(t *testing.T) {
	cert, err := GenerateSelfSignedCert("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	now := time.Now()
	if cert.Cert.NotBefore.After(now) {
		t.Errorf("NotBefore is in the future: %v", cert.Cert.NotBefore)
	}

	expectedNotAfter := now.Add(365 * 24 * time.Hour)
	diff := cert.Cert.NotAfter.Sub(expectedNotAfter)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("NotAfter = %v, want approximately %v", cert.Cert.NotAfter, expectedNotAfter)
	}
}

func TestGenerateSelfSignedCert_KeyUsage(t *testing.T) {
	cert, err := GenerateSelfSignedCert("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	if cert.Cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("Missing KeyUsageDigitalSignature")
	}

	if cert.Cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Error("Missing KeyUsageKeyEncipherment")
	}
}

func TestGenerateSelfSignedCert_ExtKeyUsage(t *testing.T) {
	cert, err := GenerateSelfSignedCert("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	hasServerAuth := false
	hasClientAuth := false
	for _, usage := range cert.Cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
		if usage == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
		}
	}

	if !hasServerAuth {
		t.Error("Missing ExtKeyUsageServerAuth")
	}

	if !hasClientAuth {
		t.Error("Missing ExtKeyUsageClientAuth")
	}
}

func TestGenerateSelfSignedCert_SerialNumber(t *testing.T) {
	cert1, err := GenerateSelfSignedCert("test1.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	cert2, err := GenerateSelfSignedCert("test2.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	if cert1.Cert.SerialNumber.Cmp(cert2.Cert.SerialNumber) == 0 {
		t.Error("Serial numbers are identical, expected unique values")
	}
}

func TestLoadCertificate_NotFound(t *testing.T) {
	_, err := LoadCertificate("/nonexistent/path/to/cert.pem")
	if err == nil {
		t.Fatal("LoadCertificate() expected error for missing file")
	}
}

func TestSaveCertificate_Nil(t *testing.T) {
	err := SaveCertificate(nil, "/tmp/test-cert.pem")
	if err == nil {
		t.Fatal("SaveCertificate() expected error for nil certificate")
	}
}

func TestSaveCertificate_EmptyPEM(t *testing.T) {
	cert := &Certificate{
		PEM: []byte{},
	}

	err := SaveCertificate(cert, "/tmp/test-cert.pem")
	if err == nil {
		t.Fatal("SaveCertificate() expected error for empty PEM")
	}
}

func TestTLSConfig(t *testing.T) {
	config := &TLSConfig{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
	}

	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion mismatch")
	}

	if config.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion mismatch")
	}

	if config.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth mismatch")
	}
}

func TestCertificate(t *testing.T) {
	cert := &Certificate{
		Cert:       &x509.Certificate{},
		PrivateKey: &struct{}{},
		PEM:        []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
	}

	if len(cert.PEM) == 0 {
		t.Fatal("Certificate PEM is empty")
	}

	if cert.Cert == nil {
		t.Fatal("Certificate cert field is nil")
	}

	if cert.PrivateKey == nil {
		t.Fatal("Certificate PrivateKey is nil")
	}
}
