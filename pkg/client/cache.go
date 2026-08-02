// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wojons/muster/pkg/cache"
)

// CacheRoundTripper wraps an http.RoundTripper and caches GET responses.
// Non-GET requests pass through to the inner transport without caching.
// Cache keys are formatted as "GET:<url>". Response Content-Type headers
// are stored separately as "HEADER:<cacheKey>:Content-Type" so cached
// responses are served with the correct content type.
//
// Zero-value is not usable; use NewCacheRoundTripper.
type CacheRoundTripper struct {
	inner http.RoundTripper
	cache cache.Cache
	ttl   time.Duration
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
// inner transport and caches the response (for 2xx status codes). Cached
// responses include the Content-Type header.
// For non-GET requests: passes through to inner transport without caching.
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
		contentType := r.getCachedContentType(ctx, cacheKey)
		return r.buildCachedResponse(data, contentType, req), nil
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

		return r.buildCachedResponse(body, resp.Header.Get("Content-Type"), req), nil
	}

	return resp, nil
}

// getCachedContentType retrieves the cached Content-Type header value.
func (r *CacheRoundTripper) getCachedContentType(ctx context.Context, cacheKey string) string {
	headerKey := "HEADER:" + cacheKey + ":Content-Type"
	if data, found, err := r.cache.Get(ctx, headerKey); err == nil && found {
		return string(data)
	}
	return ""
}

// buildCachedResponse constructs an http.Response from cached body bytes.
func (r *CacheRoundTripper) buildCachedResponse(body []byte, contentType string, req *http.Request) *http.Response {
	header := make(http.Header)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}

	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

// WithCache returns a ClientOption that wraps the HTTP client's transport
// with a caching layer. GET requests to the same URL will be served from
// the cache within the TTL duration. Non-GET requests are never cached.
//
// When c is nil, the option is a no-op and caching is disabled — existing
// client behavior is unchanged.
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
		cl.httpClient.Transport = NewCacheRoundTripper(transport, c, ttl)
	}
}
