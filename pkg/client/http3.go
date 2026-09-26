// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

// HTTP/3 (QUIC) client transport — the opt-in transport of policy "3".
//
// SUPPLY CHAIN. There is no HTTP/3 in the standard library, and none that is
// importable from golang.org/x/net (its http3 package is a //go:linkname shim
// with three unexported functions and no RoundTripper), so HTTP/3 requires a
// QUIC implementation from outside the stdlib. This file is the only non-test
// importer of github.com/quic-go/quic-go, pinned to **v0.63.0** (matrix row 16
// of specs/PROTOCOLS.md; decided in
// specs/implementation/PROTO-012-http2-http3-transport.md §4.2):
//
//   - http3.Transport IS an http.RoundTripper (measured in PROTO-010), so it
//     slots into the same option pipeline as DefaultTransport and
//     HTTP11Transport with no interface of its own;
//   - v0.63.0 grows this module's graph less than v0.59.1 and — the property
//     that keeps the decision small — perturbs no existing dependency version;
//   - its `go` floor (1.26.0) is below the repo's (1.26.6), it is MIT, and
//     govulncheck reported 0 reachable vulnerabilities.
//
// The HTTP/1.1 and HTTP/2 transports stay pure stdlib, so the h2 path gains no
// dependency from this pin. What the pin costs is linkage: measured against
// this repo's own CLI (PROTO-015, control = the pre-pin commit, same flags)
// `cmd/openapi-cli` grows +3,053,102 B default / +2,138,112 B stripped, and its
// package closure goes 516 → 544. The +306,802 B PROTO-012 §4.2 recorded was a
// *minimal* program and understates what this binary pays; the layout is
// recorded in specs/PROTOCOLS.md §7.

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// Reason classes reported in ProtocolFallback.Reason when an HTTP/3 attempt
// fails at the connection layer and HTTP/2 carries the request instead
// (PROTOCOLS.md §4.7 N3/N5). They name the *class*, not the rendered message,
// so a caller can branch on them. An HTTP status is never one of these: a peer
// that answers over HTTP/3 with an error has answered over HTTP/3.
const (
	// ReasonQUICConnectionRefused: the UDP path is closed (ICMP port
	// unreachable, or the peer's transport error CONNECTION_REFUSED).
	ReasonQUICConnectionRefused = "connection-refused"
	// ReasonQUICHandshakeTimeout: the QUIC handshake did not complete in
	// time (2 x the handshake idle timeout).
	ReasonQUICHandshakeTimeout = "handshake-timeout"
	// ReasonQUICIdleTimeout: nothing was heard from the peer before the
	// handshake idle timeout — the shape a middlebox that drops UDP
	// produces (measured in PROTO-010: "no recent network activity").
	ReasonQUICIdleTimeout = "idle-timeout"
	// ReasonQUICCryptoError: a TLS alert during the QUIC handshake
	// (CRYPTO_ERROR 0x1xx) — typically a peer that cannot do TLS 1.3, which
	// QUIC mandates, rather than a missing one.
	ReasonQUICCryptoError = "crypto-error"
	// ReasonQUICVersionNegotiation: no QUIC version in common with the peer.
	ReasonQUICVersionNegotiation = "version-negotiation"
	// ReasonQUICConnectionFailure: any other QUIC connection-layer failure
	// (a stateless reset, a peer transport error that is not
	// CONNECTION_REFUSED).
	ReasonQUICConnectionFailure = "quic-connection-failure"
)

// CRYPTO_ERROR transport error codes are 0x100 + the TLS alert (RFC 9000
// §20.1). A code at or above cryptoErrorCodeBase and below
// cryptoErrorCodeLimit is a TLS handshake failure, not a QUIC framing one.
const (
	cryptoErrorCodeBase  quic.TransportErrorCode = 0x100
	cryptoErrorCodeLimit quic.TransportErrorCode = 0x200
)

// ErrHTTP3RequiresTLS is returned when policy "3" is asked to carry a
// cleartext request. QUIC mandates TLS 1.3 (PROTOCOLS.md §4.7 N5), and the
// HTTP/2 fallback cannot help either — the stdlib has no h2c, so a cleartext
// endpoint would settle at HTTP/1.1, which is not a fallback target this
// policy accepts. Client.Do maps it to CodeProtocolMismatch with reason
// "plaintext-endpoint".
var ErrHTTP3RequiresTLS = errors.New("http3: QUIC requires TLS 1.3 and the request is cleartext")

// HTTP3Capable is implemented by a transport that can attempt HTTP/3. The
// client's policy "3" accepts such a transport (and a bare *http3.Transport)
// as the answer to its requirement; anything else is refused before the
// exchange rather than silently carried over another version (N5).
//
// It is exported so a caller that wraps one of this package's HTTP/3
// transports (to add logging or metrics) keeps being recognised.
type HTTP3Capable interface {
	SupportsHTTP3() bool
}

// HTTP3Transport returns a transport that speaks HTTP/3 only — no fallback.
// The QUIC connection is dialled with quic-go's defaults unless
// WithHTTP3QUICConfig says otherwise, and TLS is verified against the system
// roots (SSL_CERT_FILE included) unless WithHTTP3TLS says otherwise.
//
// Use it with WithTransport to require HTTP/3 and *not* fall back:
//
//	client.NewClient(client.WithHTTPVersion("3"), client.WithTransport(client.HTTP3Transport()))
//
// The policy "3" default (WithHTTPVersion on its own) is
// HTTP3FallbackTransport instead, whose reported fallback is what the opt-in
// policy promises (N4). Close the transport when the client is gone to release
// its UDP socket.
func HTTP3Transport(opts ...HTTP3Option) *http3.Transport {
	cfg := newHTTP3Config(opts...)
	return newHTTP3Transport(cfg.tls, cfg.quic)
}

// HTTP3FallbackTransport returns the transport policy "3" installs: HTTP/3 is
// attempted first and, when QUIC cannot be reached, the request is retried
// over HTTP/2 and the fallback is recorded (PROTOCOLS.md §4.7 N4/N5) — an
// HTTP/3 request that quietly becomes HTTP/2 is the outcome this transport
// exists to prevent.
//
// Only a connection-layer failure (see the ReasonQUIC* classes) falls back. An
// HTTP status, a caller cancellation or a request-body error is returned to the
// caller unchanged: none of them is evidence that the peer cannot do HTTP/3.
//
// rec receives the fallback. It is normally the client's own recorder, which
// the option pipeline passes here; a nil recorder still transports, it just
// reports nothing.
//
// The fallback is a clone of the client's default HTTP/2-capable transport, so
// HTTP/3 dials with the same TLS trust that transport uses unless
// WithHTTP3TLS overrides it. Close the returned transport when the client is
// gone to release its UDP socket.
func HTTP3FallbackTransport(rec *ProtocolRecorder, opts ...HTTP3Option) *HTTP3FallbackRoundTripper {
	cfg := newHTTP3Config(opts...)
	t := &HTTP3FallbackRoundTripper{
		fallback: DefaultTransport(),
		recorder: rec,
	}
	tlsConf := cfg.tls
	if tlsConf == nil {
		tlsConf = tlsConfigOf(t.fallback)
	}
	t.preferred = newHTTP3Transport(tlsConf, cfg.quic)
	return t
}

// HTTP3FallbackRoundTripper is the HTTP/3-preferred transport: QUIC first,
// HTTP/2 as a reported fallback. Its zero value is not usable; build it with
// HTTP3FallbackTransport.
type HTTP3FallbackRoundTripper struct {
	preferred *http3.Transport
	fallback  http.RoundTripper
	recorder  *ProtocolRecorder
}

// SupportsHTTP3 marks the transport as one policy "3" can use.
func (t *HTTP3FallbackRoundTripper) SupportsHTTP3() bool { return true }

// Close releases the HTTP/3 transport's UDP socket and its QUIC connections.
// The HTTP/2 fallback transport needs no release.
func (t *HTTP3FallbackRoundTripper) Close() error {
	if t == nil || t.preferred == nil {
		return nil
	}
	return t.preferred.Close()
}

// PreferredTransport returns the HTTP/3 transport this one attempts first, for
// a caller that needs to inspect or close it.
func (t *HTTP3FallbackRoundTripper) PreferredTransport() *http3.Transport {
	if t == nil {
		return nil
	}
	return t.preferred
}

// RoundTrip attempts HTTP/3, and on a connection-layer failure retries the
// request over HTTP/2 and records the fallback.
func (t *HTTP3FallbackRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		// A malformed request is the HTTP/3 transport's to report; there is
		// nothing here to classify as a connection failure.
		return t.preferred.RoundTrip(req)
	}
	if req.URL.Scheme != "https" {
		return nil, ErrHTTP3RequiresTLS
	}

	resp, err := t.preferred.RoundTrip(req)
	if err == nil {
		return resp, nil
	}

	reason, isConnectionFailure := quicConnectionFailureReason(err)
	if !isConnectionFailure {
		return nil, err
	}

	retry, replayErr := replayRequest(req)
	if replayErr != nil {
		// The attempt failed before the peer was reached, but the request
		// cannot be replayed, so no fallback is possible. Report the original
		// failure (still unwrappable) and name why the fallback was skipped.
		return nil, fmt.Errorf("http3: no HTTP/2 fallback (%v): %w", replayErr, err)
	}

	fallbackResp, fallbackErr := t.fallback.RoundTrip(retry)
	if fallbackErr != nil {
		// Both transports failed. The HTTP/2 error is the one that describes
		// what the caller can act on.
		return nil, fallbackErr
	}

	// The version that actually carried the request, read from the response
	// the exchange-performing transport returned — never assumed to be
	// HTTP/2.0 because it was asked for.
	t.recorder.noteFallback(WireVersionHTTP3, fallbackResp.Proto, reason)
	return fallbackResp, nil
}

// HTTP3Option configures an HTTP/3 transport (HTTP3Transport,
// HTTP3FallbackTransport, and the one policy "3" installs via
// WithHTTP3Options).
type HTTP3Option func(*http3Config)

// http3Config collects the HTTP/3 transport settings.
type http3Config struct {
	tls  *tls.Config
	quic *quic.Config
}

// newHTTP3Config applies opts in order.
func newHTTP3Config(opts ...HTTP3Option) *http3Config {
	cfg := &http3Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// WithHTTP3TLS sets the TLS configuration the HTTP/3 transport dials with:
// trust roots for a private CA, a client certificate, or a version cap. The
// configuration is cloned, and its NextProtos are replaced with "h3" by the
// transport. A nil configuration keeps the default, which is the TLS
// configuration of the client's HTTP transport (and the system roots when that
// one has none).
func WithHTTP3TLS(cfg *tls.Config) HTTP3Option {
	return func(c *http3Config) { c.tls = cfg }
}

// WithHTTP3QUICConfig sets the QUIC parameters of the HTTP/3 transport — the
// handshake and idle timeouts, keep-alive, the flow-control windows. A nil
// configuration keeps quic-go's defaults, and the configuration is cloned.
//
// It is the knob for a path known to drop UDP: shortening
// quic.Config.HandshakeIdleTimeout bounds how long an unreachable
// HTTP/3 attempt waits before the reported fallback to HTTP/2 happens.
func WithHTTP3QUICConfig(cfg *quic.Config) HTTP3Option {
	return func(c *http3Config) { c.quic = cfg }
}

// newHTTP3Transport builds the quic-go transport from resolved settings.
//
// QUIC mandates TLS 1.3 (RFC 9001 §4.2), so the version floor is stated rather
// than inherited: a peer offering only TLS 1.2 fails with CRYPTO_ERROR 0x150
// instead of being negotiated down. quic-go replaces NextProtos with "h3"
// itself.
func newHTTP3Transport(tlsConf *tls.Config, quicConf *quic.Config) *http3.Transport {
	if tlsConf == nil {
		tlsConf = &tls.Config{}
	} else {
		tlsConf = tlsConf.Clone()
	}
	if tlsConf.MinVersion < tls.VersionTLS13 {
		tlsConf.MinVersion = tls.VersionTLS13
	}

	if quicConf != nil {
		quicConf = quicConf.Clone()
	}

	return &http3.Transport{
		TLSClientConfig: tlsConf,
		QUICConfig:      quicConf,
	}
}

// tlsConfigOf returns the TLS configuration a transport dials with, so an
// HTTP/3 dial trusts what the HTTP transport trusts. A transport without one
// returns nil, which means the system roots — the same default the HTTP
// transport falls back to.
func tlsConfigOf(rt http.RoundTripper) *tls.Config {
	if tr, ok := rt.(*http.Transport); ok && tr.TLSClientConfig != nil {
		return tr.TLSClientConfig
	}
	return nil
}

// replayRequest returns a request the HTTP/2 fallback can send: the same
// request, with its body re-read through GetBody because the failed HTTP/3
// attempt may have consumed it (quic-go closes the body of a request whose
// round trip failed).
//
// A request whose body cannot be re-read cannot be replayed — retrying it
// could send a truncated body to the peer, which is worse than reporting the
// HTTP/3 failure — so the caller is told instead.
func replayRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("the request body cannot be replayed (no Request.GetBody)")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("re-read the request body: %w", err)
	}
	clone.Body = body
	return clone, nil
}

// quicConnectionFailureReason classifies err as a QUIC connection-layer
// failure and returns the class a ProtocolFallback reports (PROTOCOLS.md §4.7
// N5: "a QUIC connection-layer failure (dial refused / handshake timeout /
// CRYPTO_ERROR) is a fallback reason").
//
// Anything else is not a fallback reason. An HTTP status is not, because the
// peer answered over HTTP/3. A caller deadline or cancellation is not: the
// policy's preference was not what failed. A request-body or header error is
// not, because another transport would fail the same way.
func quicConnectionFailureReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	var idleTimeout *quic.IdleTimeoutError
	if errors.As(err, &idleTimeout) {
		// The handshake heard nothing at all — what a UDP-dropping path
		// produces (measured: "timeout: no recent network activity").
		return ReasonQUICIdleTimeout, true
	}
	var handshakeTimeout *quic.HandshakeTimeoutError
	if errors.As(err, &handshakeTimeout) {
		return ReasonQUICHandshakeTimeout, true
	}
	var versionError *quic.VersionNegotiationError
	if errors.As(err, &versionError) {
		return ReasonQUICVersionNegotiation, true
	}
	var statelessReset *quic.StatelessResetError
	if errors.As(err, &statelessReset) {
		return ReasonQUICConnectionFailure, true
	}
	var transportErr *quic.TransportError
	if errors.As(err, &transportErr) {
		switch {
		case transportErr.ErrorCode >= cryptoErrorCodeBase && transportErr.ErrorCode < cryptoErrorCodeLimit:
			return ReasonQUICCryptoError, true
		case transportErr.ErrorCode == quic.ConnectionRefused:
			return ReasonQUICConnectionRefused, true
		}
		return ReasonQUICConnectionFailure, true
	}

	// A closed UDP port on the path reports itself as ECONNREFUSED on the
	// socket read, below the QUIC layer.
	if errors.Is(err, syscall.ECONNREFUSED) {
		return ReasonQUICConnectionRefused, true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.ECONNREFUSED) {
		return ReasonQUICConnectionRefused, true
	}

	return "", false
}

// SupportsHTTP3 reports whether rt is a transport policy "3" accepts as its
// HTTP/3 transport — this package's HTTP/3 transports, or a bare quic-go
// *http3.Transport. Anything else does not make the client able to attempt
// HTTP/3, and a nil transport certainly does not.
func SupportsHTTP3(rt http.RoundTripper) bool {
	if rt == nil {
		return false
	}
	switch t := rt.(type) {
	case HTTP3Capable:
		return t.SupportsHTTP3()
	case *http3.Transport:
		return true
	}
	return false
}
