// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cache"
)

// ─────────────────────────────────────────────────────────────────────────────
// The rule this file exists for: READ THE NEGOTIATED PROTOCOL FROM THE RESPONSE,
// NEVER FROM THE CONFIGURATION.
//
// A cell that asserted "HTTP/2.0" because it configured HTTP/2 would pass
// against a client that silently downgraded to HTTP/1.1 — the exact failure the
// negotiation rows exist to prevent (specs/PROTOCOLS.md §4.7 N1/N3/N5;
// docs/investigation/PROTO-010-http2-http3-client.md §6.2: "PROTO-014 owns
// fixing that; PROTO-017 owns asserting it from the response, not the
// configuration").
//
// Three response-borne surfaces are read, each with its own provenance:
//
//  1. the record       — the version read from the response the
//                        *exchange-performing* RoundTripper returned, at the
//                        moment it returned (§4.7 N1), surfaced through the
//                        client's ProtocolRecorder. It sits below every wrapper,
//                        so the cache's rebuilt response object and a locally
//                        synthesized 429 cannot forge it. This is the surface
//                        the transport rows established and the one this file
//                        extends to every cell.
//  2. the server echo  — the version the answering process wrote into the
//                        response BODY. It travels inside the response, it is
//                        authored by the peer, and it is the only oracle a
//                        client-side fallback cannot fabricate.
//  3. response.Proto   — the response object the caller received. It counts as
//                        an observation only while it agrees with an
//                        independent surface: the response cache rebuilds it
//                        with `Proto: req.Proto` (DF-041) and the rate limiter
//                        stamps the same value on a request that never reached
//                        the wire, so a disagreeing response object is marked
//                        UNRELIABLE — never believed, and never read as an
//                        observation of the version it happens to claim.
//
// A cell whose surfaces cannot observe the negotiated version is MARKED
// UNRELIABLE (a skip, which the harness reports as skipped — never as a pass)
// rather than allowed to pass on a missing observation. A cell that asserts the
// ABSENCE of an observation (no exchange took place, so no version was
// negotiated) is not a version claim: it passes on that assertion, and says so.
//
// Nothing here claims a speedup. These are correctness cells; WAN behaviour is
// unmeasured (specs/PROTOCOLS.md §8 O2 / PROTO-012 §8 O2).
//
// No cell calls t.Parallel(): the fixtures install a trust root in the
// process-global default transport.

// unreliableMarker is what a cell that could not observe the negotiated
// protocol puts in its skip message, so the gap is greppable in `go test -v`
// output and cannot be mistaken for a green version claim.
const unreliableMarker = "UNRELIABLE CELL (not counted as a pass)"

// markUnreliable marks the current cell as one that could not observe the
// negotiated version, and stops it before it can assert anything about a
// version it never saw. The cell is reported as skipped, never as passed.
func markUnreliable(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Skipf("%s: %s", unreliableMarker, fmt.Sprintf(format, args...))
}

// exchangeSurface is one place a version was read from, together with whether
// that read is an observation of the exchange at all.
type exchangeSurface struct {
	// Name identifies the surface in failure messages.
	Name string
	// Via says how the version was read, so a failure names the evidence
	// rather than only the value.
	Via string
	// Version is what the surface reported; empty when it reported nothing.
	Version string
	// Unreadable is the reason this surface must not be read as the negotiated
	// version of this exchange. A surface is readable only when it reported a
	// version AND recorded no reason to distrust it, so a surface left
	// unpopulated is treated as unobserved rather than as an empty claim.
	Unreadable string
}

// readable reports whether the surface is an observation of this exchange's
// negotiated version.
func (s exchangeSurface) readable() bool { return s.Unreadable == "" && s.Version != "" }

// String renders the surface for a failure message.
func (s exchangeSurface) String() string {
	switch {
	case s.readable():
		return fmt.Sprintf("%s (%s): %s", s.Name, s.Via, s.Version)
	case s.Unreadable != "":
		return fmt.Sprintf("%s (%s): unreadable — %s", s.Name, s.Via, s.Unreadable)
	default:
		return fmt.Sprintf("%s (%s): reported no version", s.Name, s.Via)
	}
}

// exchangeObservation is everything one exchange's response says about the
// version that carried it.
type exchangeObservation struct {
	// Record is the version recorded from the response the exchange-performing
	// transport returned (§4.7 N1).
	Record exchangeSurface
	// ServerEcho is the version the peer reported inside the response body.
	ServerEcho exchangeSurface
	// ResponseProto is http.Response.Proto of the response the caller received.
	ResponseProto exchangeSurface
}

// surfaces returns the surfaces in authority order: the base-transport record
// first, then the peer's own echo, then the response object.
func (o exchangeObservation) surfaces() []exchangeSurface {
	return []exchangeSurface{o.Record, o.ServerEcho, o.ResponseProto}
}

// version returns the version the highest-authority readable surface observed,
// or "" when no surface observed one.
func (o exchangeObservation) version() string {
	for _, s := range o.surfaces() {
		if s.readable() {
			return s.Version
		}
	}
	return ""
}

// unreliableReason names every surface that could not observe the version and
// why, for a cell's failure or skip message.
func (o exchangeObservation) unreliableReason() string {
	var reasons []string
	for _, s := range o.surfaces() {
		if !s.readable() {
			reasons = append(reasons, s.String())
		}
	}
	return strings.Join(reasons, "; ")
}

// requireObservable stops the cell as UNRELIABLE when no surface observed the
// negotiated version, instead of letting a version claim pass on nothing.
func (o exchangeObservation) requireObservable(t *testing.T, want string) {
	t.Helper()
	if o.version() != "" {
		return
	}
	markUnreliable(t, "no surface could observe the negotiated version (expected %s): %s", want, o.unreliableReason())
}

// assertVersion is the row's assertion: every surface that can observe the
// negotiated version must agree with want. A cell that can observe nothing is
// marked unreliable and does not assert.
func (o exchangeObservation) assertVersion(t *testing.T, want string) {
	t.Helper()
	o.requireObservable(t, want)
	for _, s := range o.surfaces() {
		if !s.readable() {
			continue
		}
		assert.Equalf(t, want, s.Version, "%s must report the version the exchange used", s.String())
	}
}

// observeExchange reads every surface of one completed exchange. body is the
// response body the caller received, because the peer's own claim travels
// inside it.
func observeExchange(t *testing.T, c *Client, resp *http.Response, body []byte, method, url string) exchangeObservation {
	t.Helper()
	require.NotNil(t, resp, "an exchange that produced no response observed nothing")
	require.NotEmpty(t, resp.Proto, "the response must at least carry a version string")

	obs := exchangeObservation{}

	// 1. The record. It is fed at the base transport, so it is the response
	// version before any wrapper could rebuild the response object.
	obs.Record = exchangeSurface{Name: "record", Via: "read from the response the exchange-performing transport returned"}
	if rec, ok := c.ProtocolRecorder().For(method, url); ok {
		obs.Record.Version = rec.Observed
		switch {
		case rec.Source == SourceLocal:
			obs.Record.Unreadable = "the response was synthesized locally (source local): no exchange took place, so no version was negotiated"
		case rec.Observed == "":
			obs.Record.Unreadable = fmt.Sprintf("the record carries no version (source %q)", rec.Source)
		case rec.Source != SourceWire && rec.Source != SourceCache:
			obs.Record.Unreadable = fmt.Sprintf("unrecognised record source %q", rec.Source)
		}
	} else {
		obs.Record.Unreadable = "the recorder holds no record for this request"
	}

	// 2. The peer's own claim, carried inside the response body.
	obs.ServerEcho = exchangeSurface{Name: "server echo", Via: "the version the peer wrote into the response body"}
	var echo struct {
		ServerProto string `json:"server_proto"`
	}
	if err := json.Unmarshal(body, &echo); err != nil || echo.ServerProto == "" {
		obs.ServerEcho.Unreadable = "the response body carries no server_proto field"
	} else {
		obs.ServerEcho.Version = echo.ServerProto
	}

	// 3. The response object the caller received: an observation only while it
	// agrees with an independent surface.
	obs.ResponseProto = exchangeSurface{
		Name:    "response.Proto",
		Via:     "http.Response.Proto of the response the caller received",
		Version: resp.Proto,
	}
	var independent []exchangeSurface
	for _, s := range []exchangeSurface{obs.Record, obs.ServerEcho} {
		if s.readable() {
			independent = append(independent, s)
		}
	}
	switch {
	case len(independent) == 0:
		obs.ResponseProto.Unreadable = "no independent surface observed this exchange, so the response object's Proto cannot be confirmed"
	default:
		for _, s := range independent {
			if s.Version != obs.ResponseProto.Version {
				obs.ResponseProto.Unreadable = fmt.Sprintf(
					"the response object reports %s while the %s observed %s: it was rebuilt above the recorder (the cache's shape, DF-041), so its Proto is not an observation of this exchange",
					obs.ResponseProto.Version, s.Name, s.Version)
				break
			}
		}
	}

	return obs
}

// doRequestBody performs one request and reads its body: the version the
// answering process reports travels in that body, so it is part of the
// observation.
func doRequestBody(t *testing.T, c *Client, method, url string, body []byte) (*http.Response, []byte) {
	t.Helper()
	resp := doRequest(t, c, method, url, body)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp, raw
}

// forgingRoundTripper rebuilds every response with a claimed wire version — the
// exact shape pkg/client/cache.go's buildCachedResponse has (`Proto: req.Proto`,
// cache.go:110-126) and the rate limiter's synthetic 429 has too
// (ratelimit.go:50-64). Installed above the recorder, which is where both of
// those wrappers sit, it is what a cell reading the response OBJECT would
// believe.
type forgingRoundTripper struct {
	inner http.RoundTripper
	claim string
}

func (f *forgingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := f.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Proto = f.claim
	resp.ProtoMajor, resp.ProtoMinor = protoParts(f.claim)
	return resp, nil
}

// protoParts splits a wire version string the way net/http stores it.
func protoParts(proto string) (major, minor int) {
	parts := strings.SplitN(proto, "/", 2)
	if len(parts) == 2 {
		_, _ = fmt.Sscanf(parts[1], "%d.%d", &major, &minor)
	}
	return major, minor
}

// ── 1. HTTP/2, negotiated on a real h2 endpoint, observed from the response ──

// TestNegotiatedProtocol_HTTP2IsObservedFromTheResponse is coverage item 1: the
// request really is carried over HTTP/2, and every version claim comes from the
// response — the peer's own echo inside the body and the record read at the
// base transport. The client's configuration claims no version at all, which is
// asserted first so the claims below cannot have come from it.
func TestNegotiatedProtocol_HTTP2IsObservedFromTheResponse(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	require.Equal(t, HTTPVersionAuto, c.ProtocolRecorder().Policy(),
		"the configuration claims no version, so the claims below cannot come from it")

	for _, method := range []string{http.MethodGet, "PROPFIND"} {
		requestURL := s.URL + "/proto"
		resp, body := doRequestBody(t, c, method, requestURL, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", method)

		obs := observeExchange(t, c, resp, body, method, requestURL)
		obs.assertVersion(t, WireVersionHTTP2)

		assert.Equal(t, WireVersionHTTP2, probe.lastProto(),
			"%s: the answering server must agree it was HTTP/2.0", method)
		assert.Contains(t, string(body), `"server_proto":"HTTP/2.0"`,
			"%s: the peer's own echo travels in the response", method)
	}
}

// TestNegotiatedProtocol_ReportedVersionFollowsThePeerNotTheConfiguration is the
// anti-gaming control the row's rule implies: ONE client with ONE configuration
// talks to two peers that differ only in what they offer. A client reporting its
// configured version would report the same value twice; a client reading the
// response reports what each peer actually used.
func TestNegotiatedProtocol_ReportedVersionFollowsThePeerNotTheConfiguration(t *testing.T) {
	h2Server, h2Probe := newH2Server(t)
	h11Server, h11Probe := newHTTP11Server(t)

	// Both peers are self-signed fixtures, so one transport trusts both CAs and
	// the client is genuinely identical for the two legs.
	pool := x509.NewCertPool()
	pool.AddCert(h2Server.Certificate())
	pool.AddCert(h11Server.Certificate())

	c := NewClient(WithTransport(&http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}))
	require.Equal(t, HTTPVersionAuto, c.ProtocolRecorder().Policy())

	for _, tc := range []struct {
		name  string
		url   string
		probe *wireProbe
		want  string
	}{
		{name: "peer-offering-h2", url: h2Server.URL + "/proto", probe: h2Probe, want: WireVersionHTTP2},
		{name: "peer-offering-http-1-1-only", url: h11Server.URL + "/proto", probe: h11Probe, want: WireVersionHTTP11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doRequestBody(t, c, http.MethodGet, tc.url, nil)
			obs := observeExchange(t, c, resp, body, http.MethodGet, tc.url)
			obs.assertVersion(t, tc.want)
			assert.Equal(t, tc.want, tc.probe.lastProto(), "the peer's own observation is the oracle")
		})
	}
}

// ── 2. HTTP/3, on the same standard, with the unavailable case reported ──

// TestNegotiatedProtocol_HTTP3IsObservedFromTheResponse is coverage item 2:
// HTTP/3 is negotiated and used, asserted from the response on both surfaces.
//
// The peer is deliberately DUAL-STACK — an HTTP/3 server on UDP and an
// HTTP/2-capable TLS server on TCP at the same port number — because that is
// what makes the cell sharp in both directions:
//
//   - when HTTP/3 works, the response says HTTP/3.0 and the cell asserts it;
//   - when this environment cannot reach QUIC at all, the request lands on the
//     TCP leg and the cell MUST find the reported fallback {from,to,reason} —
//     it asserts that and then marks itself unreliable, so an unobserved
//     HTTP/3 never reads as a green HTTP/3 claim;
//   - when a client silently skips HTTP/3 (the failure this row exists to
//     prevent), the request also lands on the TCP leg, but with no fallback
//     reported — and the cell fails with exactly that message instead of being
//     excused as an environment gap.
func TestNegotiatedProtocol_HTTP3IsObservedFromTheResponse(t *testing.T) {
	probe := &wireProbe{}
	fixture := newDualStackFixture(t, probe.handler())
	stubGlobalTransport(t, loopbackTLS(fixture.pool))

	c := NewClient(WithHTTPVersion(HTTPVersion3))
	t.Cleanup(func() { closeBaseTransport(t, c) })

	const target = "/proto"
	requestURL := fixture.url + target
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, requestURL, nil)
	require.NoError(t, err)

	resp, err := c.Do(t.Context(), req)
	if err != nil {
		// Neither leg answered, so nothing observed the exchange and this cell
		// cannot claim a version: mark it, never pass it.
		if reason, isConnectionFailure := quicConnectionFailureReason(err); isConnectionFailure {
			markUnreliable(t, "neither the HTTP/3 nor the HTTP/2 leg of the fixture is reachable in this environment (%s): %v", reason, err)
		}
		require.NoError(t, err, "a transport failure that is not a connection-layer gap is a defect, not an environment gap")
	}
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	obs := observeExchange(t, c, resp, body, http.MethodGet, requestURL)
	if obs.version() != WireVersionHTTP3 {
		// HTTP/3 was not observed. The policy promises a reported fallback
		// (§4.7 N4/N5), so assert that rather than passing on an HTTP/2
		// response: a silent HTTP/2 request is the outcome this row exists to
		// prevent.
		rec := c.Protocol()
		require.NotNil(t, rec.Fallback,
			"HTTP/3 was not observed and no fallback was reported: the downgrade would be silent")
		assert.Equal(t, WireVersionHTTP3, rec.Fallback.From)
		assert.Equal(t, obs.version(), rec.Fallback.To,
			"the fallback's To must be the version the response was carried on")
		assertConnectionLayerReason(t, rec.Fallback.Reason)
		markUnreliable(t, "HTTP/3 was not observed in this environment (the attempt failed at the connection layer: %s); the reported fallback %s → %s was asserted instead",
			rec.Fallback.Reason, rec.Fallback.From, rec.Fallback.To)
	}

	obs.requireObservable(t, "HTTP/3")
	obs.assertVersion(t, WireVersionHTTP3)
	assert.Equal(t, WireVersionHTTP3, probe.lastProto(), "the answering server must agree it was HTTP/3.0")
	assert.Contains(t, string(body), `"server_proto":"HTTP/3.0"`, "the peer's own echo travels in the response")
}

// newDualStackFixture starts one loopback endpoint that answers over HTTP/3
// (UDP) and over HTTP/2 (TCP) on the same port number, sharing one self-signed
// certificate and one handler. QUIC mandates TLS 1.3, so the certificate is a
// real one (PROTOCOLS.md §4.6's h3 pattern); the TCP leg advertises h2 and
// http/1.1 in ALPN so a fallback can genuinely be served.
//
// The port is taken by the UDP listener first and the TCP listener then binds
// the same number, which the two namespaces allow: this is what lets a cell tell
// "QUIC is unreachable here" (fallback served) from "the client never attempted
// QUIC" (also served, but with nothing reported).
func newDualStackFixture(t *testing.T, handler http.Handler) *h3Fixture {
	t.Helper()
	cert, pool := loopbackCertificate(t)

	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err, "the HTTP/3 leg needs a UDP loopback socket")
	port := pc.LocalAddr().(*net.UDPAddr).Port

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err, "the HTTP/2 leg needs a TCP listener on the same port number")

	h3Server := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		},
	}
	go func() { _ = h3Server.Serve(pc) }()

	h2Server := &http.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2", "http/1.1"},
		},
	}
	go func() { _ = h2Server.ServeTLS(ln, "", "") }()

	t.Cleanup(func() {
		_ = h3Server.Close()
		_ = pc.Close()
		_ = h2Server.Close()
		_ = ln.Close()
	})

	return &h3Fixture{url: "https://" + pc.LocalAddr().String(), pool: pool}
}

// ── 3. The fallback is the deliverable: the caller can SEE it happened ──

// TestNegotiatedProtocol_Policy3FallbackIsVisibleToTheCaller is coverage item 3.
// The peer is TCP-only (h2 over TCP, nothing on UDP): policy "3" completes over
// HTTP/2, and the caller is told — fallback{from,to,reason}, classified as a
// connection-layer failure, with Satisfied false. A twin leg against the SAME
// peer under the default policy reports no fallback at all, so the signal is
// about this exchange and not a constant the caller learns to ignore.
func TestNegotiatedProtocol_Policy3FallbackIsVisibleToTheCaller(t *testing.T) {
	s, probe := newH2Server(t)
	stubGlobalTransport(t, trustTLS(t, s))

	c := NewClient(
		WithHTTPVersion(HTTPVersion3),
		WithHTTP3Options(WithHTTP3QUICConfig(shortHandshakeQUIC())),
	)
	t.Cleanup(func() { closeBaseTransport(t, c) })

	const target = "/proto"
	requestURL := s.URL + target
	resp, body := doRequestBody(t, c, http.MethodGet, requestURL, nil)

	require.Equal(t, http.StatusOK, resp.StatusCode, "the request must still be served")
	assert.Equal(t, WireVersionHTTP2, probe.lastProto(), "the TCP-only peer carried it over HTTP/2")

	obs := observeExchange(t, c, resp, body, http.MethodGet, requestURL)
	obs.assertVersion(t, WireVersionHTTP2)
	assert.NotEqual(t, WireVersionHTTP3, obs.version(), "no surface may claim HTTP/3: it was attempted and was not used")

	// The response alone is indistinguishable from a plain HTTP/2 exchange —
	// nothing in the payload says a fallback happened — which is exactly why a
	// library caller reads the record.
	assert.NotContains(t, string(body), "fallback")

	rec := c.Protocol()
	assert.Equal(t, HTTPVersion3, rec.Requested)
	assert.False(t, rec.Satisfied, "the preferred version was not the one used")
	require.NotNil(t, rec.Fallback, "a silent HTTP/2 request is the outcome this row exists to prevent")
	assert.Equal(t, WireVersionHTTP3, rec.Fallback.From)
	assert.Equal(t, WireVersionHTTP2, rec.Fallback.To)
	assertConnectionLayerReason(t, rec.Fallback.Reason)

	// A caller that kept the recorder can still find the record after the fact.
	byRequest, ok := c.ProtocolRecorder().For(http.MethodGet, requestURL)
	require.True(t, ok)
	require.NotNil(t, byRequest.Fallback)
	assert.Equal(t, rec.Fallback.From, byRequest.Fallback.From)
	assert.Equal(t, rec.Fallback.To, byRequest.Fallback.To)
	assert.Equal(t, rec.Fallback.Reason, byRequest.Fallback.Reason)

	// Negative control: the same peer, a policy that prefers no version. The
	// fallback report above is therefore a signal about the HTTP/3 attempt, not
	// a always-present field.
	plain := NewClient()
	plainResp, plainBody := doRequestBody(t, plain, http.MethodGet, requestURL, nil)
	plainObs := observeExchange(t, plain, plainResp, plainBody, http.MethodGet, requestURL)
	plainObs.assertVersion(t, WireVersionHTTP2)

	plainRecord := plain.Protocol()
	assert.Nil(t, plainRecord.Fallback, "an auto request attempts no preferred version, so there is nothing to fall back from")
	assert.True(t, plainRecord.Satisfied)
}

// ── 4. The response object is not an observation when it disagrees ──

// TestNegotiatedProtocol_WrapperBuiltResponseIsNotAnObservation pins the trap
// the rule exists for, as a cell that can fail: a wrapper above the recorder
// rebuilds the response claiming HTTP/2.0 while the wire used HTTP/1.1 (the
// cache's shape, DF-041). A cell that read the negotiated version from the
// response OBJECT passes here and is wrong; the record, taken at the
// exchange-performing transport, cannot be forged from above, and the
// disagreeing response object is marked unreliable rather than believed.
func TestNegotiatedProtocol_WrapperBuiltResponseIsNotAnObservation(t *testing.T) {
	s, probe := newHTTP11Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	c.HTTPClient().Transport = &forgingRoundTripper{inner: c.HTTPClient().Transport, claim: WireVersionHTTP2}

	const target = "/proto"
	requestURL := s.URL + target
	resp, body := doRequestBody(t, c, http.MethodGet, requestURL, nil)

	// What a naive response-object-reading cell would assert on, and what the
	// wire actually did.
	assert.Equal(t, WireVersionHTTP2, resp.Proto,
		"the wrapper's claim is what a response-object-reading cell would believe")
	assert.Equal(t, WireVersionHTTP11, probe.lastProto(), "the exchange was HTTP/1.1")

	rec := c.Protocol()
	require.Equal(t, SourceWire, rec.Source)
	require.Equal(t, WireVersionHTTP11, rec.Observed,
		"the record is read from the response the exchange-performing transport returned, below the wrapper that rebuilt it")

	obs := observeExchange(t, c, resp, body, http.MethodGet, requestURL)
	assert.False(t, obs.ResponseProto.readable(), "a response rebuilt above the recorder is not an observation")
	assert.Contains(t, obs.ResponseProto.Unreadable, "rebuilt")
	assert.Equal(t, WireVersionHTTP2, obs.ResponseProto.Version,
		"the unreliability is what the surface claimed, reported rather than hidden")
	obs.assertVersion(t, WireVersionHTTP11)

	assert.NotEqual(t, obs.ResponseProto.Version, probe.lastProto(),
		"asserting the response object alone would have passed while the wire used HTTP/1.1")
}

// TestNegotiatedProtocol_CacheHitIsObservedThroughTheRecord runs the same
// disagreement through the real cache rather than a synthetic wrapper: with the
// cache enabled the second 2xx GET is served from the entry and the response
// object is rebuilt. The version claim comes from the record — the wire version
// stored with the entry (§4.7 N3) — while the rebuilt object is either agreed
// with (once DF-041 is fixed) or marked unreliable, never read as an HTTP/1.1
// observation.
func TestNegotiatedProtocol_CacheHitIsObservedThroughTheRecord(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })
	c := NewClient(WithCache(cacheStore, time.Minute))

	const target = "/proto"
	requestURL := s.URL + target

	first, firstBody := doRequestBody(t, c, http.MethodGet, requestURL, nil)
	firstObs := observeExchange(t, c, first, firstBody, http.MethodGet, requestURL)
	firstObs.assertVersion(t, WireVersionHTTP2)
	require.Equal(t, SourceWire, c.Protocol().Source, "a cache miss is served by the wire exchange")

	second, secondBody := doRequestBody(t, c, http.MethodGet, requestURL, nil)
	require.Equal(t, int64(1), probe.hits.Load(), "the second GET must be served from the cache")

	rec := c.Protocol()
	require.Equal(t, SourceCache, rec.Source)
	require.Equal(t, WireVersionHTTP2, rec.Observed, "the version stored with the entry is the observation")

	obs := observeExchange(t, c, second, secondBody, http.MethodGet, requestURL)
	if obs.ResponseProto.readable() {
		assert.Equal(t, WireVersionHTTP2, obs.ResponseProto.Version,
			"if the cache stops rebuilding Proto (DF-041) the surfaces must agree")
	} else {
		assert.Contains(t, obs.ResponseProto.Unreadable, "rebuilt")
		assert.NotEqual(t, WireVersionHTTP2, obs.ResponseProto.Version,
			"the rebuilt object's value is not the wire version, which is why it is not read as one")
	}
	obs.assertVersion(t, WireVersionHTTP2)
}

// ── 5. Unobservable cells are classified and marked ──

// TestNegotiatedProtocol_ObservationSurfacesAreClassified pins how the three
// exchange classes are classified. Two of the three have NO observation of a
// negotiated version, which is what makes the marking rule load-bearing: a cell
// asking either of them for a version would be marked, not passed.
func TestNegotiatedProtocol_ObservationSurfacesAreClassified(t *testing.T) {
	t.Run("wire-exchange-is-observable-on-every-surface", func(t *testing.T) {
		s, _ := newH2Server(t)
		stubDefaultTransport(t, s)

		c := NewClient()
		requestURL := s.URL + "/proto"
		resp, body := doRequestBody(t, c, http.MethodGet, requestURL, nil)

		obs := observeExchange(t, c, resp, body, http.MethodGet, requestURL)
		assert.True(t, obs.Record.readable(), "the base-transport record observed the wire")
		assert.True(t, obs.ServerEcho.readable(), "the peer's echo is inside the response")
		assert.True(t, obs.ResponseProto.readable(), "with no wrapper the response object agrees")
		assert.Equal(t, WireVersionHTTP2, obs.version())
		assert.Empty(t, obs.unreliableReason(), "nothing needed to be marked on a bare wire exchange")
	})

	t.Run("cache-served-response-is-observable-through-the-record-only", func(t *testing.T) {
		s, probe := newH2Server(t)
		stubDefaultTransport(t, s)

		cacheStore := cache.NewMemoryCache(100)
		t.Cleanup(func() { _ = cacheStore.Close() })
		c := NewClient(WithCache(cacheStore, time.Minute))

		requestURL := s.URL + "/proto"
		first, firstBody := doRequestBody(t, c, http.MethodGet, requestURL, nil)
		observeExchange(t, c, first, firstBody, http.MethodGet, requestURL).assertVersion(t, WireVersionHTTP2)

		resp, body := doRequestBody(t, c, http.MethodGet, requestURL, nil)
		require.Equal(t, int64(1), probe.hits.Load(), "the second GET is a cache hit")

		obs := observeExchange(t, c, resp, body, http.MethodGet, requestURL)
		assert.True(t, obs.Record.readable(), "the version stored with the entry is an observation")
		assert.Equal(t, WireVersionHTTP2, obs.Record.Version)
		assert.False(t, obs.ResponseProto.readable(),
			"the response object is rebuilt by the cache (DF-041) and is not an observation")
		assert.Equal(t, WireVersionHTTP2, obs.version())
	})

	t.Run("locally-refused-request-has-no-negotiated-version", func(t *testing.T) {
		s, probe := newH2Server(t)
		stubDefaultTransport(t, s)

		// Transport options precede the wrapper options (§4.7 N2).
		c := NewClient(WithHTTPVersion(HTTPVersionAuto), WithRateLimiter(1, 1))

		requestURL := s.URL + "/proto"
		first, firstBody := doRequestBody(t, c, http.MethodGet, requestURL, nil)
		observeExchange(t, c, first, firstBody, http.MethodGet, requestURL).assertVersion(t, WireVersionHTTP2)

		refused, refusedBody := doRequestBody(t, c, http.MethodGet, requestURL, nil)
		require.Equal(t, http.StatusTooManyRequests, refused.StatusCode, "the second request is refused locally")
		require.Equal(t, int64(1), probe.hits.Load(), "the refused request reached no peer")

		obs := observeExchange(t, c, refused, refusedBody, http.MethodGet, requestURL)
		assert.Empty(t, obs.version(), "a locally refused request negotiated no version")
		assert.Equal(t, WireVersionHTTP11, obs.ResponseProto.Version,
			"the synthesized 429 stamps the outgoing request's version")
		assert.False(t, obs.ResponseProto.readable(),
			"that stamp is not evidence of anything: no exchange took place")
		assert.Contains(t, obs.unreliableReason(), "synthesized locally")
		assert.Contains(t, obs.unreliableReason(), "cannot be confirmed")
	})
}

// TestNegotiatedProtocol_UnreliableCellIsMarkedNotPassed pins the mechanism
// coverage item 5 requires: a cell that cannot observe the negotiated version
// reports as SKIPPED (never as a pass) and does not fail the suite, while a cell
// that did observe one asserts normally.
func TestNegotiatedProtocol_UnreliableCellIsMarkedNotPassed(t *testing.T) {
	unobservable := exchangeObservation{
		Record:        exchangeSurface{Name: "record", Unreadable: "the recorder holds no record for this request"},
		ServerEcho:    exchangeSurface{Name: "server echo", Unreadable: "the response body carries no server_proto field"},
		ResponseProto: exchangeSurface{Name: "response.Proto", Unreadable: "no independent surface observed this exchange"},
	}
	require.Empty(t, unobservable.version())

	var markedSkipped, markedFailed bool
	parentOK := t.Run("cell-with-no-observation-is-marked", func(t *testing.T) {
		defer func() {
			markedSkipped = t.Skipped()
			markedFailed = t.Failed()
		}()
		unobservable.assertVersion(t, WireVersionHTTP3)
		t.Error("assertVersion must stop the cell before it asserts a version it never observed")
	})
	require.True(t, parentOK, "a marked cell must not fail the run")
	require.True(t, markedSkipped, "a marked cell must report as skipped, never as passed")
	require.False(t, markedFailed)

	observable := exchangeObservation{
		Record: exchangeSurface{Name: "record", Via: "base transport", Version: WireVersionHTTP2},
	}
	var assertedSkipped, assertedFailed bool
	parentOK = t.Run("cell-with-an-observation-asserts", func(t *testing.T) {
		defer func() {
			assertedSkipped = t.Skipped()
			assertedFailed = t.Failed()
		}()
		observable.assertVersion(t, WireVersionHTTP2)
	})
	require.True(t, parentOK)
	require.False(t, assertedFailed, "a cell that observed the version asserts normally")
	require.False(t, assertedSkipped)

	// The marker is what a reader or a grader greps for in `go test -v` output.
	assert.Contains(t, unreliableMarker, "UNRELIABLE")
}
