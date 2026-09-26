// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/wojons/muster/pkg/errors"
)

// HTTP transport version policies (specs/PROTOCOLS.md §4.7 N4). "auto" is the
// default and never fails on version; "1.1" and "2" REQUIRE the named version;
// "3" prefers HTTP/3 and may fall back to HTTP/2, always reported.
const (
	HTTPVersionAuto = "auto"
	HTTPVersion11   = "1.1"
	HTTPVersion2    = "2"
	HTTPVersion3    = "3"
)

// Wire version strings, spelled exactly as net/http reports them in
// http.Response.Proto and as they are reported in ProtocolRecord.Observed.
const (
	WireVersionHTTP11 = "HTTP/1.1"
	WireVersionHTTP2  = "HTTP/2.0"
	WireVersionHTTP3  = "HTTP/3.0"
)

// Provenance of a response, reported in ProtocolRecord.Source.
const (
	// SourceWire means the version was observed on an exchange that reached
	// the network.
	SourceWire = "wire"
	// SourceCache means the response was served from the response cache. Its
	// version is the one stored with the cache entry, or "" when the entry
	// carries none — never a default of HTTP/1.1.
	SourceCache = "cache"
	// SourceLocal means the response was synthesized locally (no exchange
	// took place), so no version was negotiated.
	SourceLocal = "local"
)

// Reasons carried in the details of a CodeProtocolMismatch / fallback report.
const (
	// ReasonHTTP3TransportMissing is reported for policy "3" when the
	// installed transport cannot attempt HTTP/3 — a caller replaced the
	// HTTP/3 transport policy "3" installs with one that is not HTTP/3
	// capable. Asking for HTTP/3 fails loudly instead of quietly using
	// HTTP/2, which is the one outcome the policy exists to prevent. The
	// connection-layer classes a *reported fallback* carries are the
	// ReasonQUIC* constants in http3.go.
	ReasonHTTP3TransportMissing = "http3-transport-not-installed"
	// ReasonPlaintextEndpoint is reported when a required version cannot be
	// met because the URL is cleartext http://: the stdlib has no h2c and
	// QUIC requires TLS 1.3 (PROTOCOLS.md §4.7 N5).
	ReasonPlaintextEndpoint = "plaintext-endpoint"
	// reasonServerOfferedPrefix begins the reason reported when the peer
	// offered a different version than the policy required, e.g.
	// "server-offered-http/1.1".
	reasonServerOfferedPrefix = "server-offered-"
)

// serverOfferedReason names the wire version a peer offered instead of the one
// a policy required.
func serverOfferedReason(observed string) string {
	return reasonServerOfferedPrefix + strings.ToLower(observed)
}

// protocolMismatch builds the coded failure a required version that could not
// be used reports (PROTOCOLS.md §4.7 N8). It is never used for a version the
// policy did not require.
func protocolMismatch(requested, observed, reason string) *errors.AppError {
	shown := observed
	if shown == "" {
		shown = "none"
	}
	return errors.New(errors.CodeProtocolMismatch,
		fmt.Sprintf("http transport version mismatch: requested %s, observed %s (%s)", requested, shown, reason)).
		WithDetail("requested", requested).
		WithDetail("observed", observed).
		WithDetail("reason", reason)
}

// ProtocolFallback records that a preferred version was attempted and another
// carried the request (PROTOCOLS.md §4.7 N3). Only explicit fallback code sets
// it: no transport falls back on its own (N6). The policy-"3" fallback
// (HTTP3FallbackTransport in http3.go) is what fills it today.
//
// To is the version the successful attempt actually reported — read from its
// response, never assumed to be HTTP/2.0 because that was the intent. Reason
// is one of the connection-layer classes (ReasonQUIC*), by definition: an
// HTTP status is never a fallback reason.
type ProtocolFallback struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// ProtocolRecord is the HTTP transport version record every HTTP-family
// execution reports (PROTOCOLS.md §4.7 N3): what the wire used, where the
// response came from, which policy was in force, whether the policy was met,
// and — when a preferred version was attempted and another was used — the
// fallback that occurred.
//
// Observed is never guessed: it is empty when no exchange took place, and for
// a cache-served response it is the version stored with the cache entry.
//
// Satisfied reports whether the policy's preferred version was the one used.
// A policy-"3" request that fell back to HTTP/2 therefore succeeds with
// Satisfied false *and* Fallback set — the two together say "carried, over the
// other version, and here is why" (N4/N5). A policy "3" failure is never
// reported as a fallback, and a fallback is never reported as success with no
// signal.
type ProtocolRecord struct {
	Observed  string            `json:"observed"`
	Source    string            `json:"source"`
	Requested string            `json:"requested"`
	Satisfied bool              `json:"satisfied"`
	Fallback  *ProtocolFallback `json:"fallback,omitempty"`
	Method    string            `json:"method,omitempty"`
	URL       string            `json:"url,omitempty"`
	Sequence  uint64            `json:"sequence,omitempty"`
}

// ProtocolRecorder collects the HTTP transport version observed for the
// exchanges a client performed. It is fed at the base transport — below
// WithRateLimiter and WithCache — because that is the only layer that sees
// what the network actually did; the version a wrapper-built response reports
// through http.Response.Proto is not a truth source (PROTOCOLS.md §4.7 N1).
//
// A zero-value ProtocolRecorder is usable and safe (it behaves like an empty
// recorder), and every method tolerates a nil receiver.
type ProtocolRecorder struct {
	mu        sync.Mutex
	policy    string
	sequence  uint64
	last      ProtocolRecord
	byRequest map[string]ProtocolRecord
	// pending is a fallback a transport performed and has not yet been able
	// to attach: the version that carried the request can only be recorded by
	// the layer directly above the transfer that performed it, which runs
	// after the fallback transport returns. note() consumes it.
	pending *ProtocolFallback
}

// NewProtocolRecorder creates an empty recorder with the default policy.
func NewProtocolRecorder() *ProtocolRecorder {
	return &ProtocolRecorder{
		policy:    HTTPVersionAuto,
		byRequest: make(map[string]ProtocolRecord),
	}
}

// SetPolicy records the version policy in force (N4). It is reported on every
// subsequent record.
func (r *ProtocolRecorder) SetPolicy(version string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = version
}

// Policy returns the version policy in force.
func (r *ProtocolRecorder) Policy() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.policy
}

// Last returns the most recent record, or the zero record when nothing has
// been observed yet.
func (r *ProtocolRecorder) Last() ProtocolRecord {
	if r == nil {
		return ProtocolRecord{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

// For returns the most recent record for a method and URL pair. The boolean is
// false when that request has never been observed.
func (r *ProtocolRecorder) For(method, url string) (ProtocolRecord, bool) {
	if r == nil {
		return ProtocolRecord{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byRequest[requestKey(method, url)]
	return rec, ok
}

// note records one observation. observed is the wire version the response was
// carried on, or "" when no exchange took place. source is one of SourceWire,
// SourceCache or SourceLocal.
//
// A fallback a transport reported since the last observation is attached to
// this record and cleared: it belongs to the exchange being recorded, which is
// the one that reached the peer (PROTOCOLS.md §4.7 N3/N5).
func (r *ProtocolRecorder) note(method, url, source, observed string) ProtocolRecord {
	if r == nil {
		return ProtocolRecord{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	rec := ProtocolRecord{
		Observed:  observed,
		Source:    source,
		Requested: r.policy,
		Satisfied: policySatisfied(r.policy, source, observed),
		Fallback:  r.pending,
		Method:    method,
		URL:       url,
		Sequence:  r.sequence,
	}
	r.pending = nil
	r.last = rec
	r.byRequest[requestKey(method, url)] = rec
	return rec
}

// noteFallback records that a preferred version was attempted, failed at the
// connection layer, and another version carried the request (PROTOCOLS.md
// §4.7 N3/N5). The fallback is attached to the next observation recorded —
// the exchange that actually happened — so a transport calls this after the
// fallback response came back and before returning it.
//
// A client's recorder follows its most recent exchange unless the caller keeps
// a per-request lookup (ProtocolRecorder.For), which is why the CLI and MCP
// build one client per request (§4.7 N3); a long-lived client multiplexing
// versions concurrently would need the context-carried shape §8 O4 defers.
func (r *ProtocolRecorder) noteFallback(from, to, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = &ProtocolFallback{From: from, To: to, Reason: reason}
}

// requestKey is the lookup key of the per-request record map. Method and URL
// together identify one exchange.
func requestKey(method, url string) string {
	return method + " " + url
}

// policySatisfied reports whether an observation met the policy in force
// (N3/N4). A response with no observed version can never satisfy a policy;
// "auto" is satisfied by any observed version from the wire.
func policySatisfied(policy, source, observed string) bool {
	if source == "" || observed == "" {
		return false
	}
	switch policy {
	case "", HTTPVersionAuto:
		return true
	case HTTPVersion11:
		return observed == WireVersionHTTP11
	case HTTPVersion2:
		return observed == WireVersionHTTP2
	case HTTPVersion3:
		return observed == WireVersionHTTP3
	}
	return false
}

// protocolTransport sits at the base of a client's transport chain. It wraps
// the transport that performs the exchange, so the version is read from the
// response net/http actually received — before any wrapper may rebuild it.
type protocolTransport struct {
	inner http.RoundTripper
	rec   *ProtocolRecorder
}

// newProtocolTransport wraps an exchange-performing transport with the
// recorder. A nil inner falls back to the default transport.
func newProtocolTransport(inner http.RoundTripper, rec *ProtocolRecorder) *protocolTransport {
	if inner == nil {
		inner = DefaultTransport()
	}
	return &protocolTransport{inner: inner, rec: rec}
}

// RoundTrip performs the exchange and records the version it was carried on.
func (t *protocolTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.rec.note(req.Method, req.URL.String(), SourceWire, resp.Proto)
	return resp, nil
}
