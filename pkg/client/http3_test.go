// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	stderrors "errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/errors"
)

// These tests cover the HTTP/3 gate of specs/PROTOCOLS.md §6 order 10 on an
// in-process http3.Server over a UDP loopback socket — the h3 analogue of
// httptest (§4.6). QUIC mandates TLS 1.3, so every fixture carries a real
// certificate; a bare listener cannot work.
//
// The two properties the gate exists for are asserted explicitly: the version
// is read from the server's own observation as well as from the record, and a
// request that could not go over HTTP/3 either fails or reports the fallback —
// never quietly succeeds as HTTP/2.
//
// No test here calls t.Parallel(): the fixtures install a trust root in the
// process-global default transport.

// Compile-time assertions: a quic-go transport is what the policy-attachment
// point requires, with no new interface.
var (
	_ http.RoundTripper = (*http3.Transport)(nil)
	_ http.RoundTripper = (*HTTP3FallbackRoundTripper)(nil)
	_ HTTP3Capable      = (*HTTP3FallbackRoundTripper)(nil)
)

// h3Fixture is an in-process HTTP/3 server on a UDP loopback socket plus the
// trust root a client needs to verify it.
type h3Fixture struct {
	url  string
	pool *x509.CertPool
}

// newH3Fixture starts an HTTP/3 fixture serving handler.
func newH3Fixture(t *testing.T, handler http.Handler) *h3Fixture {
	t.Helper()
	cert, pool := loopbackCertificate(t)

	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err, "the h3 fixture needs a UDP loopback socket")

	srv := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		},
	}
	go func() { _ = srv.Serve(pc) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = pc.Close()
	})

	return &h3Fixture{url: "https://" + pc.LocalAddr().String(), pool: pool}
}

// loopbackCertificate returns a server certificate for 127.0.0.1 and the pool
// that trusts it. The HTTP/2 fixtures get their certificate from httptest;
// this one is explicit because the HTTP/3 server is a plain UDP listener.
func loopbackCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "muster h3 test fixture"},
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

	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// stubGlobalTransport points the process-global default transport at one with
// the given TLS configuration, which is how a default-constructed client gets
// a trust root (DefaultTransport clones the global). The HTTP/3 transport
// inherits that same TLS configuration, so one stub covers both versions.
// Restored on cleanup.
func stubGlobalTransport(t *testing.T, tlsConf *tls.Config) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: tlsConf, ForceAttemptHTTP2: true}
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// loopbackTLS is the trust configuration a fixture's client needs.
func loopbackTLS(pool *x509.CertPool) *tls.Config {
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// shortHandshakeQUIC bounds an HTTP/3 attempt that is deliberately
// unreachable, so the test proves the classification instead of waiting out
// quic-go's default five-second handshake idle timeout.
func shortHandshakeQUIC() *quic.Config {
	return &quic.Config{HandshakeIdleTimeout: 500 * time.Millisecond}
}

// assertConnectionLayerReason fails the test unless reason is one of the QUIC
// connection-layer classes and not an HTTP status.
func assertConnectionLayerReason(t *testing.T, reason string) {
	t.Helper()
	switch reason {
	case ReasonQUICConnectionRefused, ReasonQUICHandshakeTimeout, ReasonQUICIdleTimeout,
		ReasonQUICCryptoError, ReasonQUICVersionNegotiation, ReasonQUICConnectionFailure:
	default:
		t.Fatalf("fallback reason %q is not a QUIC connection-layer class", reason)
	}
	assert.NotContains(t, reason, "HTTP/", "an HTTP status is never a fallback reason")
}

// TestHTTP3_Policy3CarriesTheRequestOverHTTP3 is gate 10(a): the version
// recorded on the client and seen by the server is HTTP/3.0, for a plain GET
// and for the WebDAV verbs, with request bodies intact.
func TestHTTP3_Policy3CarriesTheRequestOverHTTP3(t *testing.T) {
	probe := &wireProbe{}
	f := newH3Fixture(t, probe.handler())
	stubGlobalTransport(t, loopbackTLS(f.pool))

	c := NewClient(WithHTTPVersion(HTTPVersion3))
	t.Cleanup(func() { closeBaseTransport(t, c) })

	for _, tc := range []struct {
		method string
		body   []byte
	}{
		{method: http.MethodGet},
		{method: http.MethodPut, body: []byte("0123456789")},
		{method: "PROPFIND", body: []byte("<propfind/>")},
		{method: http.MethodDelete},
	} {
		resp := doRequest(t, c, tc.method, f.url+"/dav", tc.body)
		assert.Equal(t, 200, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = resp.Body.Close()

		rec := c.Protocol()
		assert.Equal(t, WireVersionHTTP3, rec.Observed, "%s must be carried over HTTP/3", tc.method)
		assert.Equal(t, SourceWire, rec.Source)
		assert.Equal(t, HTTPVersion3, rec.Requested)
		assert.True(t, rec.Satisfied, "%s: HTTP/3 is what the policy preferred", tc.method)
		assert.Nil(t, rec.Fallback, "%s: nothing fell back", tc.method)
		assert.Equal(t, WireVersionHTTP3, resp.Proto)

		// The server's own observation is the oracle: a client claim the
		// server disagrees with is a forgery, not a measurement.
		assert.Equal(t, WireVersionHTTP3, probe.lastProto(), "%s: the server must have seen HTTP/3.0", tc.method)
		assert.Equal(t, int64(len(tc.body)), probe.bodyLen.Load(), "%s: the body must arrive intact", tc.method)
		assert.Contains(t, string(body), `"server_proto":"HTTP/3.0"`)
	}
}

// TestHTTP3_Policy3FallsBackToHTTP2AndReportsIt is gate 10(d): with QUIC
// unreachable the request completes over HTTP/2 and the record says so —
// fallback{from,to,reason}, classified as a connection-layer failure. This is
// the cell that makes a silent downgrade impossible to mistake for a success.
func TestHTTP3_Policy3FallsBackToHTTP2AndReportsIt(t *testing.T) {
	// A TLS peer that speaks h2 over TCP, with nothing listening on UDP at
	// that port: exactly the "no QUIC on this path" shape.
	s, probe := newH2Server(t)
	stubGlobalTransport(t, trustTLS(t, s))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, 200, resp.StatusCode, "the request must still be served")
	assert.Equal(t, WireVersionHTTP2, probe.lastProto(), "the h2 peer must have carried it")

	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP2, rec.Observed)
	assert.Equal(t, SourceWire, rec.Source)
	assert.Equal(t, HTTPVersion3, rec.Requested)
	assert.False(t, rec.Satisfied, "the preferred version was not the one used")

	require.NotNil(t, rec.Fallback, "a fallback must be reported, never silently performed")
	assert.Equal(t, WireVersionHTTP3, rec.Fallback.From)
	assert.Equal(t, WireVersionHTTP2, rec.Fallback.To)
	assertConnectionLayerReason(t, rec.Fallback.Reason)

	// The same record is addressable by method and URL, so a caller that kept
	// the recorder can find it after the fact.
	byRequest, ok := c.ProtocolRecorder().For(http.MethodGet, s.URL+"/x")
	require.True(t, ok)
	require.NotNil(t, byRequest.Fallback)
	assert.Equal(t, rec.Fallback.Reason, byRequest.Fallback.Reason)
}

// TestHTTP3_FallbackReplaysTheRequestBody: a body-carrying request that falls
// back must send the body over HTTP/2 — a replay that dropped it would be a
// worse outcome than the failed HTTP/3 attempt.
func TestHTTP3_FallbackReplaysTheRequestBody(t *testing.T) {
	s, probe := newH2Server(t)
	stubGlobalTransport(t, trustTLS(t, s))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	payload := []byte("webdav-put-payload")
	resp := doRequest(t, c, http.MethodPut, s.URL+"/dav/file", payload)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int64(len(payload)), probe.bodyLen.Load(), "the replay must carry the whole body")
	require.NotNil(t, c.Protocol().Fallback)
	assert.Equal(t, WireVersionHTTP3, c.Protocol().Fallback.From)
}

// TestHTTP3_PrereadableBodyWithoutGetBodyIsNotReplayed: when the body cannot be
// re-read, the HTTP/3 failure is reported (still unwrappable) and nothing is
// sent over HTTP/2 — a truncated body would be worse than the error.
func TestHTTP3_PrereadableBodyWithoutGetBodyIsNotReplayed(t *testing.T) {
	s, probe := newH2Server(t)
	stubGlobalTransport(t, trustTLS(t, s))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, s.URL+"/x", io.NopCloser(strings.NewReader("body")))
	require.NoError(t, err)
	require.Nil(t, req.GetBody, "this is the unreplayable shape")

	_, err = c.Do(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no HTTP/2 fallback")
	assert.Equal(t, int64(0), probe.hits.Load(), "nothing may be sent over HTTP/2 without a replayable body")
}

// TestHTTP3_HTTPStatusIsNotAFallbackReason: a peer that answers over HTTP/3
// with an error has answered over HTTP/3 — the response is returned as it is,
// and no fallback is reported.
func TestHTTP3_HTTPStatusIsNotAFallbackReason(t *testing.T) {
	f := newH3Fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	stubGlobalTransport(t, loopbackTLS(f.pool))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	resp := doRequest(t, c, http.MethodGet, f.url+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP3, rec.Observed)
	assert.True(t, rec.Satisfied)
	assert.Nil(t, rec.Fallback, "an HTTP status must never be reported as a version fallback")
}

// TestHTTP3_OnlyTransportAgainstTCPOnlyEndpointFails is the second half of gate
// 10(c): a transport that speaks HTTP/3 only does not fall back to TCP on its
// own, so the request fails. That is the measured proof that the reported
// fallback is client code and not a transport property.
func TestHTTP3_OnlyTransportAgainstTCPOnlyEndpointFails(t *testing.T) {
	s, _ := newH2Server(t)
	stubGlobalTransport(t, trustTLS(t, s))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithTransport(HTTP3Transport(
			WithHTTP3TLS(trustTLS(t, s)),
			WithHTTP3QUICConfig(shortHandshakeQUIC()),
		)),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, s.URL+"/x"))
	require.Error(t, err, "an HTTP/3-only transport must fail against a TCP-only peer")
	rec := c.Protocol()
	assert.Empty(t, rec.Observed, "no exchange took place, so no version may be reported")
	assert.Nil(t, rec.Fallback, "the transport itself never falls back")
}

// TestHTTP2_TransportAgainstH3OnlyEndpointFails is the first half of gate
// 10(c): an h2 transport sends no HTTP/3 and fails against an endpoint whose
// only listener is QUIC.
func TestHTTP2_TransportAgainstH3OnlyEndpointFails(t *testing.T) {
	f := newH3Fixture(t, http.NotFoundHandler())
	stubGlobalTransport(t, loopbackTLS(f.pool))

	c := NewClient(WithHTTPVersion(HTTPVersion2), WithTimeout(3*time.Second))
	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, f.url+"/x"))
	require.Error(t, err, "an endpoint with only a UDP listener must not answer over TCP")
	assert.Equal(t, HTTPVersion2, c.ProtocolRecorder().Policy(), "the policy in force is still reported")
	assert.Empty(t, c.Protocol().Observed)
	assert.Zero(t, c.Protocol().Sequence, "no exchange took place, so nothing was observed")
}

// TestHTTP3_TLS12CapFailsWithCryptoError is gate 10(e): QUIC mandates TLS 1.3,
// so a TLS-1.2-capped client fails with CRYPTO_ERROR rather than hanging.
func TestHTTP3_TLS12CapFailsWithCryptoError(t *testing.T) {
	f := newH3Fixture(t, http.NotFoundHandler())
	stubGlobalTransport(t, loopbackTLS(f.pool))

	capped := &tls.Config{RootCAs: f.pool, MaxVersion: tls.VersionTLS12}
	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithTransport(HTTP3Transport(
			WithHTTP3TLS(capped),
			WithHTTP3QUICConfig(shortHandshakeQUIC()),
		)),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	start := time.Now()
	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, f.url+"/x"))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 5*time.Second, "the cap must fail, not hang")
	assert.Contains(t, err.Error(), "CRYPTO_ERROR")

	reason, ok := quicConnectionFailureReason(err)
	require.True(t, ok, "a TLS handshake failure is a connection-layer failure: %v", err)
	assert.Equal(t, ReasonQUICCryptoError, reason)
}

// TestHTTP3_PlaintextIsRefusedNotDowngraded is gate 10(f): a cleartext endpoint
// can satisfy no HTTP/3 policy — QUIC mandates TLS 1.3 and the stdlib has no
// h2c — and it is refused before anything is sent.
func TestHTTP3_PlaintextIsRefusedNotDowngraded(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, s.URL+"/x"))

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, HTTPVersion3, appErr.Details()["requested"])
	assert.Equal(t, ReasonPlaintextEndpoint, appErr.Details()["reason"])
	assert.Equal(t, "", appErr.Details()["observed"], "no exchange took place, so no version was observed")
	assert.Equal(t, int64(0), probe.hits.Load(), "a refused request must not be sent")
}

// TestHTTP3_DeadlineIsCodeTimeout is gate 10(b): a context deadline over the
// HTTP/3 path surfaces as CodeTimeout (§4.2's transport-timeout row), not as a
// raw transport error.
func TestHTTP3_DeadlineIsCodeTimeout(t *testing.T) {
	release := make(chan struct{})
	f := newH3Fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { close(release) })
	stubGlobalTransport(t, loopbackTLS(f.pool))

	c := NewClient(WithHTTPVersion(HTTPVersion3))
	t.Cleanup(func() { closeBaseTransport(t, c) })

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	_, err := c.Do(ctx, mustRequest(t, http.MethodGet, f.url+"/slow"))
	require.Error(t, err)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr, "a deadline is a coded failure: %v", err)
	assert.Equal(t, errors.CodeTimeout, appErr.Code())
}

// TestHTTP3_TransportMustBeHTTP3Capable pins the capability rule the policy's
// preflight depends on.
func TestHTTP3_TransportMustBeHTTP3Capable(t *testing.T) {
	assert.True(t, SupportsHTTP3(HTTP3Transport()))
	assert.True(t, SupportsHTTP3(HTTP3FallbackTransport(nil)))
	assert.False(t, SupportsHTTP3(&http.Transport{ForceAttemptHTTP2: true}))
	assert.False(t, SupportsHTTP3(nil))
	assert.False(t, SupportsHTTP3(DefaultTransport()))
}

// closeBaseTransport releases a transport that owns a UDP socket, so tests do
// not leak listeners between cases.
func closeBaseTransport(t *testing.T, c *Client) {
	t.Helper()
	if closer, ok := c.BaseTransport().(io.Closer); ok {
		if err := closer.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
			t.Logf("closing the base transport: %v", err)
		}
	}
}
