package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cache"
)

// countingRoundTripper counts how many times RoundTrip was called.
// Used to verify that cached responses don't trigger upstream calls.
type countingRoundTripper struct {
	count int
	body  string
	ct    string
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.count++
	ct := c.ct
	if ct == "" {
		ct = "application/json"
	}
	body := c.body
	if body == "" {
		body = `{"ok":true}`
	}
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Header:        http.Header{"Content-Type": []string{ct}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
}

// TestCacheRoundTripper_GetCachesAndReturnsCached verifies that the first
// GET request reaches the inner transport, and the second GET to the same
// URL is served from cache without calling the inner transport.
func TestCacheRoundTripper_GetCachesAndReturnsCached(t *testing.T) {
	counter := &countingRoundTripper{body: `{"ok":true}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// First GET: should call inner
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, `{"ok":true}`, string(body))
	assert.Equal(t, 1, counter.count)

	// Second GET (same URL): should use cache
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp2, err := transport.RoundTrip(req2)
	require.NoError(t, err)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	assert.Equal(t, `{"ok":true}`, string(body2))
	assert.Equal(t, 1, counter.count, "second call should not hit inner transport")
}

// TestCacheRoundTripper_CachedContentType verifies that the Content-Type
// header is cached along with the response body and returned on cache hits.
func TestCacheRoundTripper_CachedContentType(t *testing.T) {
	counter := &countingRoundTripper{body: `{"data":"x"}`, ct: "application/json"}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// First GET caches the response
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, 1, counter.count)

	// Second GET served from cache with correct Content-Type
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp2, err := transport.RoundTrip(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, "application/json", resp2.Header.Get("Content-Type"))
	assert.Equal(t, 1, counter.count, "second call should not hit inner transport")
}

// TestCacheRoundTripper_PostBypassesCache verifies that POST requests
// always pass through to the inner transport and are never cached.
func TestCacheRoundTripper_PostBypassesCache(t *testing.T) {
	counter := &countingRoundTripper{body: `{"created":true}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// First POST
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api", strings.NewReader("data"))
	_, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 1, counter.count)

	// Second POST: should NOT use cache
	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/api", strings.NewReader("data"))
	_, err = transport.RoundTrip(req2)
	require.NoError(t, err)
	assert.Equal(t, 2, counter.count, "POST should always bypass cache")
}

// TestCacheRoundTripper_DifferentURLsNotConfused verifies that cache keys
// are URL-specific and different URLs don't interfere with each other.
func TestCacheRoundTripper_DifferentURLsNotConfused(t *testing.T) {
	counter := &countingRoundTripper{body: `{"ok":true}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// GET /api/1
	req1 := httptest.NewRequest(http.MethodGet, "http://example.com/api/1", nil)
	_, err := transport.RoundTrip(req1)
	require.NoError(t, err)

	// GET /api/2 — different URL should also reach inner
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/api/2", nil)
	_, err = transport.RoundTrip(req2)
	require.NoError(t, err)
	assert.Equal(t, 2, counter.count, "different URLs should each hit the inner transport")
}

// TestCacheRoundTripper_TTLExpiry verifies that expired cache entries
// trigger a fresh request to the inner transport.
func TestCacheRoundTripper_TTLExpiry(t *testing.T) {
	counter := &countingRoundTripper{body: `{"ok":true}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	// Very short TTL
	transport := NewCacheRoundTripper(counter, c, 1*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	_, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 1, counter.count)

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	// Should miss cache and call inner again
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	_, err = transport.RoundTrip(req2)
	require.NoError(t, err)
	assert.Equal(t, 2, counter.count, "expired TTL should trigger a new upstream call")
}

// TestWithCache_ClientOption verifies that WithCache wraps the HTTP client
// transport with a CacheRoundTripper.
func TestWithCache_ClientOption(t *testing.T) {
	c := cache.NewMemoryCache(100)
	defer c.Close()

	cl := NewClient(WithCache(c, 5*time.Minute))
	transport := cl.HTTPClient().Transport
	require.NotNil(t, transport)
	_, ok := transport.(*CacheRoundTripper)
	assert.True(t, ok, "transport should be *CacheRoundTripper")
}

// TestWithCache_NilCacheNoOp verifies that WithCache with a nil cache is
// a no-op and does not wrap the transport.
func TestWithCache_NilCacheNoOp(t *testing.T) {
	cl := NewClient(WithCache(nil, 5*time.Minute))
	_, ok := cl.HTTPClient().Transport.(*CacheRoundTripper)
	assert.False(t, ok, "transport should NOT be wrapped when cache is nil")
}

// TestCacheRoundTripper_InnerTransportError verifies that errors from the
// inner transport are propagated to the caller.
func TestCacheRoundTripper_InnerTransportError(t *testing.T) {
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(&errorRoundTripper{}, c, 1*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	_, err := transport.RoundTrip(req)
	assert.Error(t, err)
}

type errorRoundTripper struct{}

func (e *errorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, http.ErrHandlerTimeout
}

// TestCacheRoundTripper_Non2xxNotCached verifies that non-2xx responses
// (e.g., 404) are NOT cached and each request hits the inner transport.
func TestCacheRoundTripper_Non2xxNotCached(t *testing.T) {
	counter := &statusRoundTripper{status: http.StatusNotFound, body: "not found"}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// First GET returns 404
	req := httptest.NewRequest(http.MethodGet, "http://example.com/missing", nil)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, 1, counter.count)

	// Second GET should NOT be cached (404s aren't cached)
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/missing", nil)
	resp2, err := transport.RoundTrip(req2)
	require.NoError(t, err)
	_ = resp2.Body.Close()
	assert.Equal(t, 2, counter.count, "404 responses should not be cached")
}

type statusRoundTripper struct {
	count  int
	status int
	body   string
}

func (s *statusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.count++
	return &http.Response{
		Status:        http.StatusText(s.status),
		StatusCode:    s.status,
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Body:          io.NopCloser(strings.NewReader(s.body)),
		ContentLength: int64(len(s.body)),
	}, nil
}

// TestCacheRoundTripper_ContextCancellation verifies that the cache layer
// does not panic when the request context is already cancelled.
func TestCacheRoundTripper_ContextCancellation(t *testing.T) {
	counter := &countingRoundTripper{body: `{"ok":true}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	req = req.WithContext(ctx)

	// Should not panic
	resp, err := transport.RoundTrip(req)
	// Either err is non-nil (context cancelled) or nil (cached from a previous
	// test or context propagation). Both are acceptable — we're testing no-panic.
	_ = resp
	_ = err
}

// TestCacheRoundTripper_WithNilInnerTransport verifies that passing nil as
// the inner transport falls back to http.DefaultTransport.
func TestCacheRoundTripper_WithNilInnerTransport(t *testing.T) {
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(nil, c, 1*time.Minute)
	require.NotNil(t, transport)
	assert.NotNil(t, transport.inner)
}

// TestCacheRoundTripper_ResponseShape verifies the shape of cached responses:
// correct status code, body, and Content-Length header.
func TestCacheRoundTripper_ResponseShape(t *testing.T) {
	counter := &countingRoundTripper{body: `{"data":"test"}`}
	c := cache.NewMemoryCache(100)
	defer c.Close()
	transport := NewCacheRoundTripper(counter, c, 1*time.Minute)

	// First request caches
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	body1, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, `{"data":"test"}`, string(body1))

	// Second request served from cache
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp2, err := transport.RoundTrip(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, "200 OK", resp2.Status)
	assert.Greater(t, resp2.ContentLength, int64(0), "Content-Length should be set")

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"data":"test"}`, string(body2))
}
