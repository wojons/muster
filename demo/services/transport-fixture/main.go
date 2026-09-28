// Command transport-fixture is the loopback HTTP fixture the transport demo
// legs use (demo/demos/19-http2-transport.sh and 20-http3-transport.sh).
//
// It serves three loopback endpoints with one self-signed certificate:
//
//	H2 — advertises h2 and http/1.1 in ALPN (TCP)
//	H1 — advertises http/1.1 only (TCP)
//	H3 — HTTP/3 over QUIC (UDP); QUIC mandates TLS 1.3, so this endpoint is
//	     UDP-only and has no TCP listener beside it
//
// The TCP endpoints deliberately have no QUIC listener on their port, which is
// what the reported-fallback leg needs: policy "3" against H2 cannot complete
// over HTTP/3 and must say so.
//
// Every endpoint echoes the request's version, method, path and body length as
// JSON, which is the server-side oracle the demos compare the client's own
// report against: a client claim that the server does not agree with is a
// forgery, not a measurement.
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

	"github.com/quic-go/quic-go/http3"
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

	h2URL, h2Port, err := serve(certPath, keyPath, []string{"h2", "http/1.1"})
	if err != nil {
		log.Fatalf("transport-fixture: start the h2 endpoint: %v", err)
	}
	h1URL, h1Port, err := serve(certPath, keyPath, []string{"http/1.1"})
	if err != nil {
		log.Fatalf("transport-fixture: start the http/1.1 endpoint: %v", err)
	}
	h3URL, err := serveH3(certPath, keyPath, []int{h2Port, h1Port})
	if err != nil {
		log.Fatalf("transport-fixture: start the HTTP/3 endpoint: %v", err)
	}

	fmt.Printf("CERT %s\n", certPath)
	fmt.Printf("H2 %s\n", h2URL)
	fmt.Printf("H1 %s\n", h1URL)
	fmt.Printf("H3 %s\n", h3URL)
	fmt.Println("READY")

	// Serve until the caller kills the process.
	select {}
}

// serve starts a loopback TLS server whose ALPN list is nextProtos and returns
// its base URL and port. ServeTLS installs HTTP/2 support whenever the ALPN
// list offers h2, so the endpoint's version capability is exactly what
// nextProtos says.
func serve(certPath, keyPath string, nextProtos []string) (url string, port int, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, err
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

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return "", 0, fmt.Errorf("listener address is %T, want *net.TCPAddr", ln.Addr())
	}
	return "https://" + ln.Addr().String(), tcpAddr.Port, nil
}

// serveH3 starts an HTTP/3 endpoint on a UDP loopback socket and returns its
// base URL. avoid lists port numbers the socket must not take, so the TCP
// endpoints never end up with a QUIC listener beside them — that is the
// condition the reported-fallback leg exercises.
func serveH3(certPath, keyPath string, avoid []int) (string, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return "", err
	}
	srv := &http3.Server{
		Handler: http.HandlerFunc(echoHandler),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		},
	}

	for attempt := 0; attempt < 50; attempt++ {
		pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			return "", err
		}
		addr := pc.LocalAddr().String()
		udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
		if !ok {
			_ = pc.Close()
			return "", fmt.Errorf("udp listener address is %T, want *net.UDPAddr", pc.LocalAddr())
		}
		if containsPort(avoid, udpAddr.Port) {
			_ = pc.Close()
			continue
		}
		go func() {
			if serveErr := srv.Serve(pc); serveErr != nil {
				log.Printf("transport-fixture: serve h3: %v", serveErr)
			}
		}()
		return "https://" + addr, nil
	}
	return "", errors.New("no UDP port free of the TCP endpoints")
}

// containsPort reports whether port is in ports.
func containsPort(ports []int, port int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
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
