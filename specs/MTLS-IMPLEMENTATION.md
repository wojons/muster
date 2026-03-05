# mTLS Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Authentication

## Overview

Complete specification of **mutual TLS (mTLS)** authentication for secure service-to-service communication.

## 1. Certificate Management

```go
type CertificateManager struct {
    certPath string
    keyPath  string
    caPath   string
}

type Certificate struct {
    Cert       *x509.Certificate
    PrivateKey *rsa.PrivateKey
    PEM        []byte
}

func (cm *CertificateManager) Load() (*tls.Config, error) {
    // Load client certificate
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
    
    // Load CA certificate
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
    }, nil
}
```

## 2. TLS Configuration

```go
type TLSConfig struct {
    MinVersion   uint16
    MaxVersion   uint16
    CipherSuites []uint16
    ClientAuth   tls.ClientAuthType
}

func DefaultTLSConfig() *tls.Config {
    return &tls.Config{
        MinVersion: tls.VersionTLS12,
        MaxVersion: tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
            tls.TLS_AES_128_GCM_SHA256,
        },
        PreferServerCipherSuites: true,
    }
}
```

## 3. Certificate Validation

```go
func (cm *CertificateManager) Validate(cert *x509.Certificate) error {
    // Check expiration
    if time.Now().After(cert.NotAfter) {
        return errors.New("certificate has expired")
    }
    
    if time.Now().Before(cert.NotBefore) {
        return errors.New("certificate not yet valid")
    }
    
    // Check key usage
    if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
        return errors.New("certificate missing digital signature key usage")
    }
    
    // Verify hostname
    if err := cert.VerifyHostname("expected-hostname"); err != nil {
        return fmt.Errorf("hostname verification failed: %w", err)
    }
    
    return nil
}
```

## 4. HTTP Client with mTLS

```go
func CreateMTLSClient(certManager *CertificateManager) (*http.Client, error) {
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
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
