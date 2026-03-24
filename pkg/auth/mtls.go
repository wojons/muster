package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
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
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	var certPEM bytes.Buffer
	err = pem.Encode(&certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode certificate PEM: %w", err)
	}

	return &Certificate{
		Cert:       cert,
		PrivateKey: priv,
		PEM:        certPEM.Bytes(),
	}, nil
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
