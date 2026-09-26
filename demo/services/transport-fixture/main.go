// Command transport-fixture is the loopback HTTP fixture the transport demo
// legs use (demo/demos/19-http2-transport.sh; the HTTP/3 leg of PROTO-015 will
// reuse it).
//
// It serves two loopback TLS endpoints with one self-signed certificate:
//
//	H2 — advertises h2 and http/1.1 in ALPN
//	H1 — advertises http/1.1 only
//
// Both echo the request's version, method, path and body length as JSON, which
// is the server-side oracle the demo compares the client's own report against:
// a client claim that the server does not agree with is a forgery, not a
// measurement.
//
// The certificate is written next to the addresses so the caller can trust it
// (the CLI reads SSL_CERT_FILE). Addresses and the certificate path are printed
// as `KEY value` lines, then a final READY line.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	certDir := flag.String("cert-dir", "", "directory to write cert.pem/key.pem into (default: a new temporary directory)")
	flag.Parse()

	dir := *certDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "transport-fixture")
		if err != nil {
			log.Fatalf("transport-fixture: temp dir: %v", err)
		}
		dir = tmp
		defer func() { _ = os.RemoveAll(tmp) }()
	}

	certPEM, keyPEM, err := selfSignedCertificate()
	if err != nil {
		log.Fatalf("transport-fixture: generate certificate: %v", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		log.Fatalf("transport-fixture: write %s: %v", certPath, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		log.Fatalf("transport-fixture: write %s: %v", keyPath, err)
	}

	h2URL, err := serve(certPath, keyPath, []string{"h2", "http/1.1"})
	if err != nil {
		log.Fatalf("transport-fixture: start the h2 endpoint: %v", err)
	}
	h1URL, err := serve(certPath, keyPath, []string{"http/1.1"})
	if err != nil {
		log.Fatalf("transport-fixture: start the http/1.1 endpoint: %v", err)
	}

	fmt.Printf("CERT %s\n", certPath)
	fmt.Printf("H2 %s\n", h2URL)
	fmt.Printf("H1 %s\n", h1URL)
	fmt.Println("READY")

	// Serve until the caller kills the process.
	select {}
}

// serve starts a loopback TLS server whose ALPN list is nextProtos and returns
// its base URL. ServeTLS installs HTTP/2 support whenever the ALPN list offers
// h2, so the endpoint's version capability is exactly what nextProtos says.
func serve(certPath, keyPath string, nextProtos []string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	srv := &http.Server{
		Handler:           http.HandlerFunc(echoHandler),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: nextProtos,
		},
	}

	go func() {
		if serveErr := srv.ServeTLS(ln, certPath, keyPath); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("transport-fixture: serve %v: %v", nextProtos, serveErr)
		}
	}()

	return "https://" + ln.Addr().String(), nil
}

// echoHandler reports what the server actually saw.
func echoHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"server_proto": r.Proto,
		"method":       r.Method,
		"path":         r.URL.Path,
		"body_len":     len(body),
	})
}

// selfSignedCertificate returns a certificate/key pair for 127.0.0.1 and
// localhost. QUIC mandates TLS 1.3 and the CLI verifies the peer, so a bare
// listener is not an option.
func selfSignedCertificate() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "muster transport fixture"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
