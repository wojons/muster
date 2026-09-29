// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wojons/muster/pkg/cache"
)

// CacheRoundTripper wraps an http.RoundTripper and caches GET responses.
// Non-GET requests pass through to the inner transport without caching.
// Cache keys are formatted as "GET:<url>".
//
// An entry stores the response body under the cache key and the rest of the
// response — its protocol version, its status and its end-to-end headers —
// under companion keys prefixed "HEADER:<cacheKey>:" (see cachedResponseMeta).
// A cache hit is therefore served the response the exchange delivered instead
// of a rebuilt approximation of it: the version it reports is the one the wire
// used and every header the origin sent is present (DF-041). Connection-scoped
// headers are never part of a stored entry, because they describe the
// connection the response arrived on rather than the response itself
// (RFC 7230 §6.1, RFC 7234 §3).
//
// Zero-value is not usable; use NewCacheRoundTripper.
type CacheRoundTripper struct {
	inner    http.RoundTripper
	cache    cache.Cache
	ttl      time.Duration
	observer *ProtocolRecorder
}

// SetProtocolRecorder points the cache at the client's protocol recorder so a
// cache-served response is reported as source "cache" with the version stored
// with the entry (PROTOCOLS.md §4.7 N3). It is optional: a directly
// constructed cache round tripper without one still caches, it just reports
// nothing.
func (r *CacheRoundTripper) SetProtocolRecorder(rec *ProtocolRecorder) {
	r.observer = rec
}

// NewCacheRoundTripper creates a caching round tripper that wraps the given
// inner transport. If inner is nil, http.DefaultTransport is used.
// The ttl parameter controls how long cached responses are stored.
//
// The caller retains ownership of the cache and is responsible for calling
// Close on it when no longer needed.
func NewCacheRoundTripper(inner http.RoundTripper, c cache.Cache, ttl time.Duration) *CacheRoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &CacheRoundTripper{
		inner: inner,
		cache: c,
		ttl:   ttl,
	}
}

// RoundTrip executes the request through the cache layer.
// For GET requests: checks cache first, and on cache miss forwards to the
// inner transport and caches the response (for 2xx status codes). An entry
// keeps the body, the status, the protocol version and every end-to-end header
// the response carried, so a later hit is served the response the exchange
// delivered. For non-GET requests: passes through to inner transport without
// caching.
//
// The caller must call resp.Body.Close() on the returned response.
func (r *CacheRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only cache GET requests
	if req.Method != http.MethodGet {
		return r.inner.RoundTrip(req)
	}

	ctx := req.Context()
	cacheKey := "GET:" + req.URL.String()

	// Check cache
	if data, found, err := r.cache.Get(ctx, cacheKey); err == nil && found {
		entry := r.loadCachedEntry(ctx, cacheKey, data)
		// The version the entry was written with — empty for an entry written
		// before version recording existed, which is reported as no version at
		// all rather than as HTTP/1.1 (PROTOCOLS.md §4.7 N3).
		r.observe(req, SourceCache, entry.version())
		return entry.response(), nil
	}

	// Cache miss: forward request to inner transport
	resp, err := r.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Only cache 2xx responses
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("cache: read response body: %w", readErr)
		}

		// Cache the response body
		if setErr := r.cache.Set(ctx, cacheKey, body, r.ttl); setErr != nil {
			// Log would go here; don't fail the request over cache errors
		}

		// Cache Content-Type header
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			headerKey := "HEADER:" + cacheKey + ":Content-Type"
			_ = r.cache.Set(ctx, headerKey, []byte(ct), r.ttl)
		}

		// Cache the wire version the response arrived on, so a later hit can
		// report it without a network exchange. resp.Proto is truthful at this
		// layer: the inner transport is the exchange that produced it.
		if resp.Proto != "" {
			_ = r.cache.Set(ctx, versionKey(cacheKey), []byte(resp.Proto), r.ttl)
		}

		// Cache the response envelope: the status, the protocol version and
		// the end-to-end headers, with the connection-scoped ones removed. A
		// later hit is served those instead of a rebuilt 200/HTTP/1.1 with a
		// lone Content-Type (DF-041).
		r.storeEnvelope(ctx, cacheKey, resp, len(body))

		// A miss is served by the exchange itself: the response the inner
		// transport returned, with the body that was just read re-attached. A
		// cache miss is not a reason to hand the caller less than the wire
		// delivered, and the hop-by-hop headers are left in place — a direct
		// client would deliver them too; only a stored copy drops them.
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}

	return resp, nil
}

// observe reports a response the cache served without a network exchange.
func (r *CacheRoundTripper) observe(req *http.Request, source, version string) {
	if r.observer == nil {
		return
	}
	r.observer.note(req.Method, req.URL.String(), source, version)
}

// cachedResponseMeta is the envelope stored beside a cached response body: the
// parts of a 2xx response that are not its body. It is additive — an entry
// written before it existed is still readable through the pre-envelope keys,
// and no key that existed before changes meaning.
//
// Header is the stored header map: every end-to-end header the response
// carried, with the hop-by-hop set removed and Content-Length normalized to the
// stored body's length (see storableHeaders).
type cachedResponseMeta struct {
	Proto      string              `json:"proto,omitempty"`
	ProtoMajor int                 `json:"proto_major,omitempty"`
	ProtoMinor int                 `json:"proto_minor,omitempty"`
	Status     string              `json:"status,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Header     map[string][]string `json:"header,omitempty"`
}

// metaKey is the cache key under which an entry's response envelope is stored.
func metaKey(cacheKey string) string {
	return "HEADER:" + cacheKey + ":Response-Meta"
}

// versionKey is the cache key under which the wire version of a cached
// response is stored, alongside the body and the Content-Type header.
func versionKey(cacheKey string) string {
	return "HEADER:" + cacheKey + ":HTTP-Version"
}

// storeEnvelope writes the response envelope of a cache entry. A failure here
// is not fatal and does not fail the request: the entry then serves the way an
// entry written before the envelope existed does (body, Content-Type and
// version only).
func (r *CacheRoundTripper) storeEnvelope(ctx context.Context, cacheKey string, resp *http.Response, bodyLen int) {
	meta, err := json.Marshal(cachedResponseMeta{
		Proto:      resp.Proto,
		ProtoMajor: resp.ProtoMajor,
		ProtoMinor: resp.ProtoMinor,
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Header:     storableHeaders(resp.Header, bodyLen),
	})
	if err != nil {
		return
	}
	_ = r.cache.Set(ctx, metaKey(cacheKey), meta, r.ttl)
}

// cachedEntry is one cache entry as reconstructed from the store: the stored
// body bytes plus what the entry says about the response they came from. meta
// is nil for an entry written before the envelope existed; such an entry
// carries only its body, its Content-Type and its version.
type cachedEntry struct {
	body              []byte
	meta              *cachedResponseMeta
	legacyContentType string
	legacyVersion     string
}

// loadCachedEntry reconstructs an entry from the store. The pre-envelope keys
// are read only when the envelope cannot answer: a hit on an entry written by
// this build costs one extra lookup (the envelope), not four.
func (r *CacheRoundTripper) loadCachedEntry(ctx context.Context, cacheKey string, body []byte) cachedEntry {
	entry := cachedEntry{body: body, meta: r.getCachedMeta(ctx, cacheKey)}
	if entry.meta == nil || entry.meta.Proto == "" {
		entry.legacyVersion = r.getCachedHTTPVersion(ctx, cacheKey)
	}
	if entry.meta == nil || entry.meta.Header == nil {
		entry.legacyContentType = r.getCachedContentType(ctx, cacheKey)
	}
	return entry
}

// version is the wire version the entry was written with: the envelope's, or
// the pre-envelope version key for an older entry. Empty means the entry
// carries no version at all, which is reported as such and never filled in
// with a default (PROTOCOLS.md §4.7 N3).
func (e cachedEntry) version() string {
	if e.meta != nil && e.meta.Proto != "" {
		return e.meta.Proto
	}
	return e.legacyVersion
}

// response rebuilds the http.Response a cache hit is served with. It carries
// the protocol version, the status and the end-to-end headers the entry was
// stored with — rebuilding a 2xx response as HTTP/1.1 with a lone Content-Type
// header would forge both (DF-041).
//
// ContentLength is the stored body's length, which is the only figure the cache
// can vouch for. A legacy entry that recorded neither a status nor a header map
// is served as it always was: 200 OK, its stored Content-Type, and its stored
// version.
func (e cachedEntry) response() *http.Response {
	header := make(http.Header)
	proto, major, minor := "", 0, 0
	status, statusCode := "200 OK", http.StatusOK

	if e.meta != nil {
		for name, values := range e.meta.Header {
			header[name] = append([]string(nil), values...)
		}
		proto, major, minor = e.meta.Proto, e.meta.ProtoMajor, e.meta.ProtoMinor
		if e.meta.Status != "" {
			status = e.meta.Status
		}
		if e.meta.StatusCode != 0 {
			statusCode = e.meta.StatusCode
		}
	}
	if header.Get("Content-Type") == "" && e.legacyContentType != "" {
		header.Set("Content-Type", e.legacyContentType)
	}
	if proto == "" {
		proto = e.legacyVersion
	}
	if proto != "" && major == 0 {
		// net/http spells the version "HTTP/<major>.<minor>"; an entry stored
		// without the parts still tells them.
		if maj, min, ok := http.ParseHTTPVersion(proto); ok {
			major, minor = maj, min
		}
	}

	return &http.Response{
		Status:        status,
		StatusCode:    statusCode,
		Proto:         proto,
		ProtoMajor:    major,
		ProtoMinor:    minor,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
	}
}

// hopByHopHeaders are the connection-scoped headers a cache must not store
// (RFC 7230 §6.1, RFC 7234 §3): they describe the connection the response
// arrived on, so replaying them on a later, different connection would be
// wrong. Names are http.Header's canonical spelling.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// storableHeaders returns the header map to store for a cached response: every
// end-to-end header the response carried, minus the hop-by-hop set and minus
// the field names the response's own Connection header lists (RFC 7230 §6.1),
// with Content-Length normalized to the length of the stored body so the
// entry's bookkeeping cannot contradict the bytes it serves.
//
// Names are canonicalized the way http.Header expects, and compared
// case-insensitively: a response built by hand may spell them any way, and a
// header that is dropped must not survive under another spelling, while a
// header that is kept must be found by Header.Get whatever spelling it arrived
// with.
func storableHeaders(h http.Header, bodyLen int) http.Header {
	out := make(http.Header, len(h))
	for name, values := range h {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "" {
			continue
		}
		if hopByHopHeaders[canonical] {
			continue
		}
		out[canonical] = append(out[canonical], values...)
	}

	// A field named by the Connection header is hop-by-hop for this response
	// even when it is not on the static list.
	for name, values := range h {
		if !strings.EqualFold(name, "Connection") {
			continue
		}
		for _, value := range values {
			for _, listed := range strings.Split(value, ",") {
				if listed = strings.TrimSpace(listed); listed != "" {
					deleteHeaderFold(out, listed)
				}
			}
		}
	}

	if hasHeaderFold(out, "Content-Length") {
		deleteHeaderFold(out, "Content-Length")
		out.Set("Content-Length", strconv.Itoa(bodyLen))
	}
	return out
}

// deleteHeaderFold removes a header by name, whatever spelling the map holds.
func deleteHeaderFold(h http.Header, name string) {
	for existing := range h {
		if strings.EqualFold(existing, name) {
			delete(h, existing)
		}
	}
}

// hasHeaderFold reports whether a header is present under any spelling.
func hasHeaderFold(h http.Header, name string) bool {
	for existing := range h {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
}

// getCachedContentType retrieves the cached Content-Type header value.
func (r *CacheRoundTripper) getCachedContentType(ctx context.Context, cacheKey string) string {
	headerKey := "HEADER:" + cacheKey + ":Content-Type"
	if data, found, err := r.cache.Get(ctx, headerKey); err == nil && found {
		return string(data)
	}
	return ""
}

// getCachedHTTPVersion retrieves the wire version a cached entry was written
// with. An entry written before version recording existed has none, and the
// empty string is returned — never a default of HTTP/1.1 (PROTOCOLS.md §4.7 N3).
func (r *CacheRoundTripper) getCachedHTTPVersion(ctx context.Context, cacheKey string) string {
	if data, found, err := r.cache.Get(ctx, versionKey(cacheKey)); err == nil && found {
		return string(data)
	}
	return ""
}

// getCachedMeta retrieves the response envelope of a cached entry, or nil when
// the entry has none (written before the envelope existed) or it cannot be
// read. A nil envelope degrades to the pre-envelope behaviour rather than
// failing the hit.
func (r *CacheRoundTripper) getCachedMeta(ctx context.Context, cacheKey string) *cachedResponseMeta {
	data, found, err := r.cache.Get(ctx, metaKey(cacheKey))
	if err != nil || !found || len(data) == 0 {
		return nil
	}
	var meta cachedResponseMeta
	if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr != nil {
		return nil
	}
	return &meta
}

// WithCache returns a ClientOption that wraps the HTTP client's transport
// with a caching layer. GET requests to the same URL will be served from
// the cache within the TTL duration. Non-GET requests are never cached.
//
// When c is nil, the option is a no-op and caching is disabled — existing
// client behavior is unchanged.
//
// The cache records the wire version of each entry and reports a cache-served
// response to the client's ProtocolRecorder (source "cache"), so a caller
// reading the recorder learns which version carried the original exchange. The
// recorded value is authoritative: it is taken at the base transport, below
// this wrapper, and is not affected by the response object this layer rebuilds
// (PROTOCOLS.md §4.7 N1/N3).
//
// The caller retains ownership of the cache and is responsible for calling
// Close on it when no longer needed.
//
// Example:
//
//	cache := cache.NewMemoryCache(1000)
//	client := client.NewClient(client.WithCache(cache, 5*time.Minute))
func WithCache(c cache.Cache, ttl time.Duration) ClientOption {
	return func(cl *Client) {
		if c == nil {
			return // disabled — no wrapping, existing behavior unchanged
		}

		transport := cl.httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		cacheRT := NewCacheRoundTripper(transport, c, ttl)
		cacheRT.SetProtocolRecorder(cl.protocol)
		cl.httpClient.Transport = cacheRT
		cl.chain = cacheRT // the option-installed chain now tops at the cache (DF-042)
	}
}
