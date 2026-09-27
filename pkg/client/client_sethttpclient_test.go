// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cache"
)

// These tests pin DF-042: SetHTTPClient must not silently discard the
// middleware the option pipeline installed on the client it replaces. The
// WithCache and WithRateLimiter wrappers (and the protocol recorder below
// them) wrap the transport of the *http.Client present when the options ran;
// a bare assignment of a fresh *http.Client used to drop all of them on the
// floor. After the fix the chain carried over sits above whatever transport
// the replacement client brings, so the wrappers keep operating.

// TestSetHTTPClient_PreservesCache proves caching still operates after the
// underlying client is replaced: the second GET is served from the cache, not
// re-fetched over the wire through the replacement client.
func TestSetHTTPClient_PreservesCache(t *testing.T) {
	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	c := NewClient(WithCache(cacheStore, time.Minute))

	first := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = first.Body.Close()
	require.Equal(t, int64(1), probe.hits.Load(), "the first GET must reach the server")

	c.SetHTTPClient(&http.Client{Timeout: 30 * time.Second})

	second := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	defer func() { _ = second.Body.Close() }()

	assert.Equal(t, http.StatusOK, second.StatusCode)
	assert.Equal(t, int64(1), probe.hits.Load(),
		"after SetHTTPClient the second GET must be served from the cache, not the wire")
	assert.Equal(t, SourceCache, c.Protocol().Source,
		"the cache wrapper must still report cache-served responses")
}

// TestSetHTTPClient_PreservesRateLimiter proves rate limiting still operates
// after the underlying client is replaced: with a burst of 1 already spent,
// the next request is refused locally with 429 instead of reaching the server
// through the replacement client.
func TestSetHTTPClient_PreservesRateLimiter(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	c := NewClient(WithRateLimiter(1, 1))
	transport := c.HTTPClient().Transport.(*RateLimitRoundTripper)
	t.Cleanup(transport.Stop)

	first := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = first.Body.Close()
	require.Equal(t, int64(1), probe.hits.Load(), "the first GET must spend the burst token")

	c.SetHTTPClient(&http.Client{Timeout: 30 * time.Second})

	second, err := c.Do(context.Background(), mustRequest(t, http.MethodGet, s.URL+"/x"))
	require.NoError(t, err)
	defer func() { _ = second.Body.Close() }()

	assert.Equal(t, http.StatusTooManyRequests, second.StatusCode,
		"after SetHTTPClient the limiter must still refuse the request locally")
	assert.Equal(t, int64(1), probe.hits.Load(),
		"a refused request must not reach the server through the replacement client")
}

// TestSetHTTPClient_CarriesChainWithBothWrappers proves the whole middleware
// stack — limiter above cache above the recorder — is carried over as the
// same live value, not rebuilt and not dropped: the replacement client's
// transport is the very RateLimitRoundTripper the options installed, with the
// cache wrapper below it.
func TestSetHTTPClient_CarriesChainWithBothWrappers(t *testing.T) {
	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	c := NewClient(WithCache(cacheStore, time.Minute), WithRateLimiter(10, 5))
	transport := c.HTTPClient().Transport.(*RateLimitRoundTripper)
	t.Cleanup(transport.Stop)

	c.SetHTTPClient(&http.Client{Timeout: 30 * time.Second})

	replaced, ok := c.HTTPClient().Transport.(*RateLimitRoundTripper)
	require.True(t, ok, "the option-installed chain must survive SetHTTPClient")
	assert.Same(t, transport, replaced, "the carried chain is the installed wrapper, not a copy")

	cacheRT, ok := replaced.inner.(*CacheRoundTripper)
	require.True(t, ok, "the cache wrapper stays below the limiter on the carried chain")
	assert.Same(t, c.protocol, cacheRT.observer, "the recorder stays wired to the wrappers")
}

// TestSetHTTPClient_NilClearsChain pins the deliberate opt-out: a nil
// replacement removes every wrapper, the recorder included, and the client
// degrades to a plain HTTP client — the only way to strip the chain.
func TestSetHTTPClient_NilClearsChain(t *testing.T) {
	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	c := NewClient(WithCache(cacheStore, time.Minute), WithRateLimiter(10, 5))
	transport := c.HTTPClient().Transport.(*RateLimitRoundTripper)
	t.Cleanup(transport.Stop)

	c.SetHTTPClient(nil)

	assert.Nil(t, c.HTTPClient().Transport, "a nil replacement strips the whole chain")

	s, probe := newH2Server(t)
	stubDefaultTransport(t, s)
	resp := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = resp.Body.Close()
	assert.Equal(t, int64(1), probe.hits.Load(), "the request still reaches the server")
	assert.Empty(t, c.Protocol().Source, "no recorder, so no source is reported")
}

// countingTransport is a replacement client's own transport: it counts the
// exchanges it performs, which is the oracle that a request really went
// through the caller's transport.
type countingTransport struct {
	inner      http.RoundTripper
	roundTrips int
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.roundTrips++
	return t.inner.RoundTrip(req)
}

// TestSetHTTPClient_CallerTransportSitsBelowTheChain proves the replacement
// client's own transport becomes the exchange performer BELOW the carried
// middleware — the limiter still refuses, and the requests that pass are
// served by the caller's transport and then cached.
func TestSetHTTPClient_CallerTransportSitsBelowTheChain(t *testing.T) {
	probe := &wireProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cacheStore := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = cacheStore.Close() })

	c := NewClient(WithCache(cacheStore, time.Minute), WithRateLimiter(100, 5))
	transport := c.HTTPClient().Transport.(*RateLimitRoundTripper)
	t.Cleanup(transport.Stop)

	counting := &countingTransport{inner: s.Client().Transport}
	c.SetHTTPClient(&http.Client{Transport: counting, Timeout: 30 * time.Second})

	first := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()

	assert.Equal(t, 1, counting.roundTrips, "the exchange must be performed by the caller's transport")
	assert.Equal(t, int64(1), probe.hits.Load())

	second := doRequest(t, c, http.MethodGet, s.URL+"/x", nil)
	_ = second.Body.Close()

	assert.Equal(t, http.StatusOK, second.StatusCode)
	assert.Equal(t, 1, counting.roundTrips, "a cache hit must not re-exchange through the caller's transport")
	assert.Equal(t, int64(1), probe.hits.Load(), "a cache hit must not re-reach the server")
	assert.Equal(t, SourceCache, c.Protocol().Source)
}
