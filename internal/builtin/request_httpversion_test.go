package builtin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/errors"
)

// These tests are the integration half of the HTTP/2 work (specs/PROTOCOLS.md
// §6 order 9): they drive the CLI's request handler — config value, client
// options, wire exchange, diagnostics — and assert against the server's own
// observation rather than the client's claim alone.

// versionProbe records what the server saw for every request it served.
type versionProbe struct {
	proto atomic.Value // string
	hits  atomic.Int64
}

func (p *versionProbe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		p.proto.Store(r.Proto)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"server_proto":%q}`, r.Proto)
	})
}

func (p *versionProbe) lastProto() string {
	v, _ := p.proto.Load().(string)
	return v
}

// startH2Fixture starts an in-process loopback TLS server offering h2.
func startH2Fixture(t *testing.T, probe *versionProbe) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(probe.handler())
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

// trustDefaultTransportFor points the process-global default transport at one
// that trusts s's certificate. pkg/client's default transport clones the global
// when a client is constructed, so this is how the CLI's internally built
// client is given a trust root. Restored on cleanup.
func trustDefaultTransportFor(t *testing.T, s *httptest.Server) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = s.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// captureStdio runs fn with os.Stdout and os.Stderr redirected to pipes,
// returning what each stream received. It keeps the verbose diagnostics out of
// the test output and makes the stdout/stderr split assertable (stdout payload
// fidelity is frozen by PROTOCOLS.md §4.4/§5.3).
func captureStdio(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	errR, errW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout, os.Stderr = outW, errW

	runErr := fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	os.Stdout, os.Stderr = origOut, origErr

	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	return string(outBytes), string(errBytes), runErr
}

// TestHandleRequest_HTTPVersionReportsHTTP2 is the CLI-level h2 cell: the
// default policy negotiates HTTP/2 against an h2 peer and the verbose surface
// reports the version that carried the request, from the record, on stderr.
func TestHandleRequest_HTTPVersionReportsHTTP2(t *testing.T) {
	probe := &versionProbe{}
	s := startH2Fixture(t, probe)
	trustDefaultTransportFor(t, s)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "auto"}}
	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.NoError(t, err)

	assert.Equal(t, "HTTP/2.0", probe.lastProto(), "the server must have seen HTTP/2.0")
	assert.Contains(t, stderr, "HTTP: HTTP/2.0 (wire, requested auto)",
		"the verbose diagnostic must report the version the wire used")
	assert.NotContains(t, stdout, "HTTP:", "diagnostics belong on stderr, never in the payload")
	assert.Contains(t, stdout, `"server_proto":"HTTP/2.0"`, "stdout carries the response body")
}

// TestHandleRequest_HTTPVersionReportsHTTP11Only is the reporting control: a
// peer offering only HTTP/1.1 still serves the request under the default
// policy, and the version reported comes from the wire.
func TestHandleRequest_HTTPVersionReportsHTTP11Only(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(nil, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.NoError(t, err)

	assert.Equal(t, "HTTP/1.1", probe.lastProto())
	assert.Contains(t, stderr, "HTTP: HTTP/1.1 (wire, requested auto)")
	assert.Contains(t, stdout, `"server_proto":"HTTP/1.1"`)
}

// TestHandleRequest_HTTPVersionRequiredPolicyFails covers the policy cell: a
// required HTTP/2 that the endpoint cannot provide is a coded failure naming
// requested, observed and reason — not a downgrade.
func TestHandleRequest_HTTPVersionRequiredPolicyFails(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "2"}}
	stdout, _, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.Error(t, err)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, "2", appErr.Details()["requested"])
	assert.Equal(t, "HTTP/1.1", appErr.Details()["observed"])
	assert.Equal(t, "plaintext-endpoint", appErr.Details()["reason"])
	assert.Empty(t, stdout, "a failed request must not print a payload")
}

// TestHandleRequest_HTTPVersionUnknownValueRejected: the policy value is
// validated before anything is sent.
func TestHandleRequest_HTTPVersionUnknownValueRejected(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "h2"}}
	_, _, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeValidation, appErr.Code())
	assert.Equal(t, int64(0), probe.hits.Load(), "no request may be sent for an invalid policy")
}

// TestRequestCommand_HTTPVersionFlagOverridesConfig drives the real cobra
// command: the flag must win over the configured policy in both directions.
func TestRequestCommand_HTTPVersionFlagOverridesConfig(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	// Config requires HTTP/2, the flag asks for auto: the flag wins and the
	// request succeeds over the plaintext endpoint.
	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "2"}}
	cmd := newRequestCommand(cfg)
	cmd.SetArgs([]string{"GET", s.URL + "/x", "--http-version", "auto", "-v"})
	_, stderr, err := captureStdio(t, func() error { return cmd.ExecuteContext(t.Context()) })
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Request.HTTPVersion, "the flag must override the configured policy")
	assert.Contains(t, stderr, "HTTP: HTTP/1.1 (wire, requested auto)")

	// The other direction: auto configured, HTTP/2 required by the flag, and
	// the cleartext endpoint cannot satisfy it.
	cfg2 := &config.Config{Request: config.RequestConfig{HTTPVersion: "auto"}}
	cmd2 := newRequestCommand(cfg2)
	cmd2.SetArgs([]string{"GET", s.URL + "/x", "--http-version", "2", "-v"})
	_, _, err = captureStdio(t, func() error { return cmd2.ExecuteContext(t.Context()) })
	require.Error(t, err)
	assert.Equal(t, "2", cfg2.Request.HTTPVersion)
	assert.True(t, errors.IsError(err, errors.CodeProtocolMismatch))
	assert.Contains(t, err.Error(), "requested 2, observed HTTP/1.1 (plaintext-endpoint)")
}

// TestRequestCommandHasHTTPVersionFlag pins the flag's existence and default:
// an empty value means "use the configured policy".
func TestRequestCommandHasHTTPVersionFlag(t *testing.T) {
	cmd := newRequestCommand(&config.Config{})
	flag := cmd.Flags().Lookup("http-version")
	require.NotNil(t, flag, "the request verb must expose --http-version")
	assert.Equal(t, "", flag.DefValue)
}

// ── HTTP/3 half: §6 order 10 (board row PROTO-015) ──────────────────────────
//
// PROTO-014's CLI cell asserted that policy "3" failed with
// http3-transport-not-installed. HTTP/3 exists now, so these cells assert what
// replaced it: the request goes over HTTP/3 when the peer speaks it, and when
// it does not, it is carried over HTTP/2 *with the fallback on the diagnostic
// surface* — the CLI-level proof that a caller can see the downgrade.

// TestHandleRequest_HTTPVersion3ReportsHTTP3 is gate 10(a) at the CLI level:
// policy "3" reaches an HTTP/3-only peer, and both the server's echo and the
// stderr diagnostic say HTTP/3.0.
func TestHandleRequest_HTTPVersion3ReportsHTTP3(t *testing.T) {
	probe := &versionProbe{}
	url, pool := startH3Fixture(t, probe)
	trustDefaultTransportWithPool(t, pool)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "3"}}
	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", url+"/probe", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.NoError(t, err)

	assert.Equal(t, "HTTP/3.0", probe.lastProto(), "the server must have seen HTTP/3.0")
	assert.Contains(t, stderr, "HTTP: HTTP/3.0 (wire, requested 3)",
		"the diagnostic must name the version the wire used")
	assert.NotContains(t, stderr, "fallback", "nothing fell back, so nothing may be reported")
	assert.NotContains(t, stdout, "HTTP:", "diagnostics belong on stderr")
	assert.Contains(t, stdout, `"server_proto":"HTTP/3.0"`)
}

// TestHandleRequest_HTTPVersion3FallsBackToHTTP2AndReportsIt is gate 10(d) at
// the CLI level: a TCP-only peer means no QUIC, so the request completes over
// HTTP/2 and the verbose line reports the fallback, its direction and its
// connection-layer class. Success without that line would be the silent
// downgrade the row exists to prevent.
func TestHandleRequest_HTTPVersion3FallsBackToHTTP2AndReportsIt(t *testing.T) {
	probe := &versionProbe{}
	s := startH2Fixture(t, probe)
	trustDefaultTransportFor(t, s)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "3"}}
	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/probe", nil, nil, "", true, 30*time.Second, "", "", "")
	})
	require.NoError(t, err, "the request must still be served after the fallback")

	assert.Equal(t, "HTTP/2.0", probe.lastProto(), "the h2 peer carried it")
	require.Contains(t, stderr, "HTTP: HTTP/2.0 (wire, requested 3, fallback HTTP/3.0→HTTP/2.0: ",
		"the fallback must be on the diagnostic surface")
	assert.Regexp(t, `fallback HTTP/3\.0→HTTP/2\.0: (connection-refused|handshake-timeout|idle-timeout|crypto-error|version-negotiation|quic-connection-failure)`, stderr)
	assert.Contains(t, stdout, `"server_proto":"HTTP/2.0"`)
}

// TestHandleRequest_HTTPVersion3OnPlaintextIsRefused is gate 10(f) at the CLI
// level: a cleartext endpoint can satisfy no HTTP/3 policy — QUIC mandates
// TLS 1.3 and the stdlib has no h2c — and nothing is sent.
func TestHandleRequest_HTTPVersion3OnPlaintextIsRefused(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "3"}}
	stdout, _, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/probe", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.Error(t, err)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, "3", appErr.Details()["requested"])
	assert.Equal(t, "plaintext-endpoint", appErr.Details()["reason"])
	assert.Equal(t, int64(0), probe.hits.Load(), "a refused request must not reach the peer")
	assert.Empty(t, stdout)
}

// startH3Fixture starts an in-process HTTP/3 server on a UDP loopback socket —
// the h3 analogue of httptest (PROTOCOLS.md §4.6). QUIC mandates TLS 1.3, so
// the fixture carries a certificate and returns the pool that trusts it.
func startH3Fixture(t *testing.T, probe *versionProbe) (url string, pool *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "muster builtin h3 fixture"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool = x509.NewCertPool()
	pool.AddCert(leaf)

	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err, "an in-process HTTP/3 server needs a UDP loopback socket")

	srv := &http3.Server{
		Handler: probe.handler(),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
			MinVersion:   tls.VersionTLS13,
		},
	}
	go func() { _ = srv.Serve(pc) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = pc.Close()
	})

	return "https://" + pc.LocalAddr().String(), pool
}

// trustDefaultTransportWithPool points the process-global default transport at
// one that trusts pool. pkg/client's default transport clones the global, and
// its HTTP/3 transport dials with that same TLS configuration, so one stub
// gives the CLI's internally built client the trust root for both versions.
func trustDefaultTransportWithPool(t *testing.T, pool *x509.CertPool) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}
	t.Cleanup(func() { http.DefaultTransport = orig })
}
