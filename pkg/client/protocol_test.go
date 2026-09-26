// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/errors"
)

// These tests cover the HTTP/2 gate of specs/PROTOCOLS.md §6 order 9: the
// version a request actually used is recorded at the base transport and is
// read from there, never from a wrapper-built http.Response.
//
// Every version claim is checked against two oracles: what the server saw
// (r.Proto, recorded by the handler) and what the recorder reports. A test
// that only compared the client's own two surfaces could pass while both
// agreed on a forgery.
//
// No test here calls t.Parallel(): stubDefaultTransport swaps the
// process-global http.DefaultTransport for the duration of a test.

// wireProbe is the server side of the oracle: it records the HTTP version of
// every request it serves, plus a hit count and the request body length.
type wireProbe struct {
	proto   atomic.Value // string: last r.Proto seen
	hits    atomic.Int64
	bodyLen atomic.Int64
}

func (p *wireProbe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		p.proto.Store(r.Proto)
		body, _ := io.ReadAll(r.Body)
		p.bodyLen.Store(int64(len(body)))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"server_proto":%q,"method":%q}`, r.Proto, r.Method)
	})
}

// lastProto returns the version the server last saw, or "" if it served
// nothing.
func (p *wireProbe) lastProto() string {
	v, _ := p.proto.Load().(string)
	return v
}

// newH2Server starts an in-process loopback TLS server that offers h2 in ALPN.
func newH2Server(t *testing.T) (*httptest.Server, *wireProbe) {
	t.Helper()
	probe := &wireProbe{}
	s := httptest.NewUnstartedServer(probe.handler())
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, probe
}

// newHTTP11Server starts an in-process loopback TLS server that does not offer
// h2 — the reporting control of gate 9(c).
func newHTTP11Server(t *testing.T) (*httptest.Server, *wireProbe) {
	t.Helper()
	probe := &wireProbe{}
	s := httptest.NewUnstartedServer(probe.handler())
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, probe
}

// trustTLS returns a TLS config that trusts the test server's certificate.
func trustTLS(t *testing.T, s *httptest.Server) *tls.Config {
	t.Helper()
	tr, ok := s.Client().Transport.(*http.Transport)
	require.True(t, ok, "httptest TLS server client must use *http.Transport")
	require.NotNil(t, tr.TLSClientConfig, "httptest TLS client must carry a TLS config")
	return tr.TLSClientConfig.Clone()
}

// stubDefaultTransport points the process-global default transport at one that
// trusts s's certificate. DefaultTransport() (the client's default) clones the
// global when a client is constructed, so this is how a test gives a
// default-constructed client a trust root without reaching into the transport
// chain the option pipeline builds. The global is restored on cleanup.
func stubDefaultTransport(t *testing.T, s *httptest.Server) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = s.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// doRequest issues one request through the client and returns the response.
func doRequest(t *testing.T, c *Client, method, url string, body []byte) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	require.NoError(t, err)
	resp, err := c.Do(t.Context(), req)
	require.NoError(t, err)
	return resp
}

func TestDefaultTransport_ExplicitlyEnablesHTTP2(t *testing.T) {
	tr := DefaultTransport()
	require.NotNil(t, tr)
	assert.True(t, tr.ForceAttemptHTTP2,
		"the default transport must enable HTTP/2 explicitly, not by the nil-transport accident")

	// The returned transport is a clone: mutating it must not change the
	// process-global default.
	tr.ForceAttemptHTTP2 = false
	global, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, global.ForceAttemptHTTP2,
		"mutating the clone must not alias http.DefaultTransport")
}

func TestHTTP11Transport_DisablesHTTP2(t *testing.T) {
	tr := HTTP11Transport()
	require.NotNil(t, tr)
	assert.False(t, tr.ForceAttemptHTTP2, "the 1.1 transport must not attempt HTTP/2")
	assert.NotNil(t, tr.TLSNextProto, "a non-nil TLSNextProto map disables the h2 upgrade path")
	require.NotNil(t, tr.TLSClientConfig)
	assert.Equal(t, []string{"http/1.1"}, tr.TLSClientConfig.NextProtos,
		"the handshake must offer only http/1.1 so a peer cannot pick h2")
}

func TestNormalizeHTTPVersion(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: HTTPVersionAuto},
		{in: "auto", want: HTTPVersionAuto},
		{in: " AUTO ", want: HTTPVersionAuto},
		{in: "1.1", want: HTTPVersion11},
		{in: "2", want: HTTPVersion2},
		{in: "3", want: HTTPVersion3},
		{in: "2.0", wantErr: true},
		{in: "h2", wantErr: true},
	} {
		got, err := NormalizeHTTPVersion(tc.in)
		if tc.wantErr {
			require.Error(t, err, "input %q must be rejected", tc.in)
			var appErr *errors.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, errors.CodeValidation, appErr.Code())
			assert.Equal(t, tc.in, appErr.Details()["value"])
			continue
		}
		require.NoError(t, err, "input %q must be accepted", tc.in)
		assert.Equal(t, tc.want, got)
	}
}

// TestProtocol_DefaultClientNegotiatesHTTP2 is gate 9(a): the default client
// reaches an h2 server over h2, the server agrees, and the recorder reports it.
func TestProtocol_DefaultClientNegotiatesHTTP2(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP2, rec.Observed, "recorder must report the wire version")
	assert.Equal(t, SourceWire, rec.Source)
	assert.Equal(t, HTTPVersionAuto, rec.Requested)
	assert.True(t, rec.Satisfied, "auto is satisfied by an observed wire version")
	assert.Nil(t, rec.Fallback, "no cross-version fallback exists in this build")
	assert.Equal(t, WireVersionHTTP2, probe.lastProto(), "the server must have seen HTTP/2.0")
	assert.Equal(t, WireVersionHTTP2, resp.Proto, "with no wrapper the response agrees")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"server_proto":"HTTP/2.0"`)
}

// TestProtocol_WebDAVVerbNegotiatesHTTP2 is the PROPFIND half of gate 9(a).
func TestProtocol_WebDAVVerbNegotiatesHTTP2(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	resp := doRequest(t, c, "PROPFIND", s.URL+"/dav", nil)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, WireVersionHTTP2, c.Protocol().Observed)
	assert.Equal(t, WireVersionHTTP2, probe.lastProto())
}

// TestProtocol_LargePUTArrivesIntact is gate 9(b): a 1 MiB body reaches the
// server byte-for-byte over h2.
func TestProtocol_LargePUTArrivesIntact(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	payload := bytes.Repeat([]byte("muster"), 1024*1024/6) // 1 MiB (rounded down to a multiple of 6)
	require.Len(t, payload, 1048572)

	c := NewClient()
	resp := doRequest(t, c, http.MethodPut, s.URL+"/f", payload)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int64(len(payload)), probe.bodyLen.Load(), "the server must observe the whole body")
	assert.Equal(t, WireVersionHTTP2, c.Protocol().Observed)
	assert.Equal(t, WireVersionHTTP2, probe.lastProto())
}

// TestProtocol_HTTP11OnlyServerIsReported is gate 9(c): a peer that offers only
// HTTP/1.1 still serves the request under the default policy, and the fact is
// reported — the version comes from the wire, never from configuration.
func TestProtocol_HTTP11OnlyServerIsReported(t *testing.T) {
	s, probe := newHTTP11Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, 200, resp.StatusCode, "an HTTP/1.1-only peer must still be served")
	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP11, rec.Observed)
	assert.Equal(t, SourceWire, rec.Source)
	assert.True(t, rec.Satisfied)
	assert.Equal(t, WireVersionHTTP11, probe.lastProto())
}

// TestProtocol_HandBuiltTransportDropsToHTTP11 is gate 9(d): the negative cell
// that lets the h2 assertions fail. A hand-built transport without
// ForceAttemptHTTP2 loses HTTP/2 against a server that offers it.
func TestProtocol_HandBuiltTransportDropsToHTTP11(t *testing.T) {
	s, probe := newH2Server(t)

	c := NewClient(WithTransport(&http.Transport{TLSClientConfig: trustTLS(t, s)}))
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, WireVersionHTTP11, c.Protocol().Observed,
		"a hand-built transport must really lose HTTP/2, or gate 9(a) proves nothing")
	assert.Equal(t, WireVersionHTTP11, probe.lastProto())
}

// TestProtocol_Policy2AgainstHTTP11PeerFails is gate 9(e): a required version
// the peer did not use is a coded failure, not a silent downgrade.
func TestProtocol_Policy2AgainstHTTP11PeerFails(t *testing.T) {
	s, _ := newHTTP11Server(t)
	stubDefaultTransport(t, s)

	c := NewClient(WithHTTPVersion(HTTPVersion2))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.URL+"/x", nil)
	require.NoError(t, err)

	resp, err := c.Do(t.Context(), req)
	require.Error(t, err)
	assert.Nil(t, resp)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, HTTPVersion2, appErr.Details()["requested"])
	assert.Equal(t, WireVersionHTTP11, appErr.Details()["observed"])
	assert.Equal(t, "server-offered-http/1.1", appErr.Details()["reason"])
	assert.Contains(t, err.Error(), "PROTOCOL_MISMATCH")
}

// TestProtocol_Policy2OnPlaintextFails covers the plaintext row of the §4.7 N5
// table: cleartext can never satisfy a required 2.
func TestProtocol_Policy2OnPlaintextFails(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	c := NewClient(WithHTTPVersion(HTTPVersion2))
	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, s.URL+"/x"))

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, ReasonPlaintextEndpoint, appErr.Details()["reason"])
	assert.Equal(t, WireVersionHTTP11, appErr.Details()["observed"],
		"a cleartext exchange is HTTP/1.1 by construction")
	assert.Equal(t, WireVersionHTTP11, probe.lastProto())
}

// TestProtocol_Policy11ForcesHTTP11AgainstAnH2Peer proves version selection is
// transport selection: policy 1.1 suppresses h2 even where the peer offers it.
func TestProtocol_Policy11ForcesHTTP11AgainstAnH2Peer(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient(WithHTTPVersion(HTTPVersion11))
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = resp.Body.Close() }()

	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP11, rec.Observed)
	assert.True(t, rec.Satisfied)
	assert.Equal(t, WireVersionHTTP11, probe.lastProto(), "the h2-capable peer must have served 1.1")
}

// TestProtocol_Policy3WithoutHTTP3TransportFails pins the boundary PROTO-015
// takes over: asking for HTTP/3 in a build without an HTTP/3 transport fails
// loudly and sends nothing.
func TestProtocol_Policy3WithoutHTTP3TransportFails(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient(WithHTTPVersion(HTTPVersion3))
	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, s.URL+"/x"))

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, HTTPVersion3, appErr.Details()["requested"])
	assert.Equal(t, ReasonHTTP3TransportMissing, appErr.Details()["reason"])
	assert.Equal(t, int64(0), probe.hits.Load(), "no request may be sent when the transport is missing")
}

// TestProtocol_CacheHitReportsStoredWireVersion is gate 9(f): with the cache
// enabled, a 2xx GET reports the wire version on both the miss and the hit.
// The cached response object's own Proto is not the truth source (DF-041), so
// this cell reads the recorder.
func TestProtocol_CacheHitReportsStoredWireVersion(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	c := NewClient(WithCache(cacheStore, time.Minute))

	first := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = first.Body.Close()
	missRecord := c.Protocol()
	assert.Equal(t, WireVersionHTTP2, missRecord.Observed)
	assert.Equal(t, SourceWire, missRecord.Source, "a cache miss is served by the wire exchange")

	second := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = second.Body.Close()
	hitRecord := c.Protocol()

	assert.Equal(t, int64(1), probe.hits.Load(), "the second GET must be served from cache")
	assert.Equal(t, WireVersionHTTP2, hitRecord.Observed,
		"a cache hit must report the version the entry was carried on")
	assert.Equal(t, SourceCache, hitRecord.Source)
	assert.True(t, hitRecord.Satisfied)
}

// TestProtocol_CacheEntryWithoutVersionReportsNone covers the N3 rule that an
// entry written before version recording existed reports no version at all —
// never a default of HTTP/1.1.
func TestProtocol_CacheEntryWithoutVersionReportsNone(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })
	// Seed a body without the companion version entry, the shape an entry
	// written by an older build has.
	require.NoError(t, cacheStore.Set(t.Context(), "GET:"+s.URL+"/x", []byte(`{"ok":true}`), time.Minute))

	c := NewClient(WithCache(cacheStore, time.Minute))
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = resp.Body.Close()

	rec := c.Protocol()
	assert.Equal(t, SourceCache, rec.Source)
	assert.Empty(t, rec.Observed, "an entry with no stored version must report none")
	assert.False(t, rec.Satisfied)
	assert.Equal(t, int64(0), probe.hits.Load())
}

// TestProtocol_LocalRefusalReportsNoVersion covers the rate limiter's
// synthesized 429: no exchange, therefore no version — and no mismatch error
// even under a required policy that was being satisfied, because nothing was
// negotiated for the refused request.
func TestProtocol_LocalRefusalReportsNoVersion(t *testing.T) {
	plainProbe := &wireProbe{}
	plain := httptest.NewServer(plainProbe.handler())
	t.Cleanup(plain.Close)

	h2, _ := newH2Server(t)
	stubDefaultTransport(t, h2)

	for _, tc := range []struct {
		policy string
		url    string
	}{
		{HTTPVersionAuto, plain.URL + "/x"},
		{HTTPVersion11, plain.URL + "/x"},
		{HTTPVersion2, h2.URL + "/x"},
	} {
		// Transport options come before the wrapper options (N2).
		c := NewClient(WithHTTPVersion(tc.policy), WithRateLimiter(1, 1))

		// The first request spends the burst token and satisfies the policy.
		first, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, tc.url))
		require.NoError(t, err, "policy %q", tc.policy)
		require.True(t, c.Protocol().Satisfied, "policy %q first request", tc.policy)
		_ = first.Body.Close()

		refused, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, tc.url))
		require.NoError(t, err, "a local refusal is not a version mismatch (policy %q)", tc.policy)
		defer func() { _ = refused.Body.Close() }()
		assert.Equal(t, http.StatusTooManyRequests, refused.StatusCode, "policy %q", tc.policy)

		rec := c.Protocol()
		assert.Equal(t, SourceLocal, rec.Source, "policy %q", tc.policy)
		assert.Empty(t, rec.Observed, "a locally refused request has no negotiated version (policy %q)", tc.policy)
		assert.False(t, rec.Satisfied)
	}
}

// TestProtocol_UnknownPolicyIsRejected: an unrecognised policy is a validation
// error, never a silent "auto".
func TestProtocol_UnknownPolicyIsRejected(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	c := NewClient(WithHTTPVersion("2.0"))
	_, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, s.URL+"/x"))

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeValidation, appErr.Code())
	assert.Equal(t, int64(0), probe.hits.Load())
}

// TestProtocolRecorder_ForLookupAndNilSafety covers the per-request lookup and
// the nil-receiver contract (a client without a recorder must not panic).
func TestProtocolRecorder_ForLookupAndNilSafety(t *testing.T) {
	s, _ := newH2Server(t)
	stubDefaultTransport(t, s)

	c := NewClient()
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = resp.Body.Close()

	rec, ok := c.ProtocolRecorder().For(http.MethodGet, s.URL+"/x")
	require.True(t, ok)
	assert.Equal(t, WireVersionHTTP2, rec.Observed)

	_, ok = c.ProtocolRecorder().For(http.MethodGet, s.URL+"/never-requested")
	assert.False(t, ok)

	var nilRecorder *ProtocolRecorder
	assert.Equal(t, "", nilRecorder.Policy())
	assert.Equal(t, ProtocolRecord{}, nilRecorder.Last())
	_, ok = nilRecorder.For(http.MethodGet, s.URL+"/x")
	assert.False(t, ok)
	nilRecorder.SetPolicy(HTTPVersion2) // must not panic
}

// TestProtocol_SequencesIncrease pins the per-exchange sequence the record
// carries, which is what lets a caller tell two exchanges apart.
func TestProtocol_SequencesIncrease(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	c := NewClient()
	first := doRequest(t, c, http.MethodGet, s.URL+"/a", nil)
	_ = first.Body.Close()
	seqA := c.Protocol().Sequence

	second := doRequest(t, c, http.MethodGet, s.URL+"/b", nil)
	_ = second.Body.Close()
	seqB := c.Protocol().Sequence
	assert.Greater(t, seqB, seqA)

	_, ok := c.ProtocolRecorder().For(http.MethodGet, s.URL+"/a")
	assert.True(t, ok, "an earlier exchange stays addressable by method and URL")
}

// TestWithTransport_MustPrecedeWrapperOptions documents the pipeline order: a
// transport installed first is wrapped by the cache, while installing one after
// a wrapper discards that wrapper (the same hazard SetHTTPClient has, DF-042).
func TestWithTransport_MustPrecedeWrapperOptions(t *testing.T) {
	s, _ := newH2Server(t)
	trusted := func() http.RoundTripper {
		return &http.Transport{TLSClientConfig: trustTLS(t, s), ForceAttemptHTTP2: true}
	}

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	ordered := NewClient(WithTransport(trusted()), WithCache(cacheStore, time.Minute))
	_, isCache := ordered.HTTPClient().Transport.(*CacheRoundTripper)
	assert.True(t, isCache, "a transport installed before the cache is wrapped by it")

	reversed := NewClient(WithCache(cacheStore, time.Minute), WithTransport(trusted()))
	_, isCache = reversed.HTTPClient().Transport.(*CacheRoundTripper)
	assert.False(t, isCache, "installing a transport after the cache discards the cache wrapper")
}

// TestWithTransport_NilKeepsDefaultTransport: a nil transport option is a
// no-op, so the default (HTTP/2-capable) transport stays installed.
func TestWithTransport_NilKeepsDefaultTransport(t *testing.T) {
	c := NewClient(WithTransport(nil))
	base, ok := c.BaseTransport().(*http.Transport)
	require.True(t, ok)
	assert.True(t, base.ForceAttemptHTTP2)
}

// mustRequest builds a request for the tests, failing on error.
func mustRequest(t *testing.T, method, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	require.NoError(t, err)
	return req
}

// TestProtocol_RecordIsNotReadFromTheResponseObject pins the row's trap: on a
// cache hit the record and the rebuilt response object can disagree, and the
// record is the one that follows the wire. The assertion is made against the
// server-side oracle, so it holds whether or not DF-041 has been fixed.
func TestProtocol_RecordIsNotReadFromTheResponseObject(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })
	c := NewClient(WithCache(cacheStore, time.Minute))

	first := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = first.Body.Close()
	second := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = second.Body.Close() }()

	require.Equal(t, int64(1), probe.hits.Load(), "the second GET must be a cache hit")

	rec := c.Protocol()
	assert.Equal(t, WireVersionHTTP2, probe.lastProto(), "the oracle: the exchange used HTTP/2")
	assert.Equal(t, WireVersionHTTP2, rec.Observed, "the record must follow the wire")
	assert.True(t, strings.HasPrefix(rec.Observed, "HTTP/"))

	if second.Proto != rec.Observed {
		// Documented divergence (DF-041), not a failure: the rebuilt response
		// object reports a different version than the one the wire used. This
		// is exactly why the record, not resp.Proto, is the truth source.
		t.Logf("DF-041 divergence on a cache hit: resp.Proto=%q record.Observed=%q",
			second.Proto, rec.Observed)
	}
}
