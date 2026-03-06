package auth

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"
)

type CertificateManager struct {
	certPath   string
	keyPath    string
	caPath     string
	serverName string // Server name for hostname verification
}

type Certificate struct {
	Cert       *x509.Certificate
	PrivateKey interface{}
	PEM        []byte
}

func NewCertificateManager(certPath, keyPath, caPath, serverName string) *CertificateManager {
	return &CertificateManager{
		certPath:   certPath,
		keyPath:    keyPath,
		caPath:     caPath,
		serverName: serverName,
	}
}

func (cm *CertificateManager) Load() (*tls.Config, error) {
	certPEM, err := os.ReadFile(cm.certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(cm.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}

	caPEM, err := os.ReadFile(cm.caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		ServerName:   cm.serverName, // CRITICAL SECURITY: Enable hostname verification
	}, nil
}

type TLSConfig struct {
	MinVersion   uint16
	MaxVersion   uint16
	CipherSuites []uint16
	ClientAuth   tls.ClientAuthType
}

func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAndVerifyClientCert,
	}
}

func (cm *CertificateManager) Validate(cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	now := time.Now()

	if now.After(cert.NotAfter) {
		return errors.New("certificate has expired")
	}

	if now.Before(cert.NotBefore) {
		return errors.New("certificate not yet valid")
	}

	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("certificate missing digital signature key usage")
	}

	return nil
}

func CreateMTLSClient(certManager *CertificateManager) (*http.Client, error) {
	if certManager == nil {
		return nil, errors.New("certificate manager is nil")
	}

	tlsConfig, err := certManager.Load()
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

func GenerateSelfSignedCert(commonName string) (*Certificate, error) {
	return nil, errors.New("self-signed cert generation not yet implemented")
}

func LoadCertificate(path string) (*Certificate, error) {
	certPEM, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("certificate file not found")
		}
		return nil, err
	}

	return &Certificate{PEM: certPEM}, nil
}

func SaveCertificate(cert *Certificate, path string) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	if len(cert.PEM) == 0 {
		return errors.New("certificate PEM is empty")
	}

	return os.WriteFile(path, cert.PEM, 0644)
}
