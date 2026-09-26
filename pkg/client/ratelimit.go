// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"io"
	"net/http"
	"strings"

	"github.com/wojons/muster/pkg/rate"
)

// RateLimitRoundTripper wraps an http.RoundTripper and applies token-bucket
// rate limiting to outgoing requests. When the rate limit is exceeded, the
// transport returns HTTP 429 Too Many Requests instead of forwarding the
// request to the inner transport.
//
// Zero-value is not usable; use NewRateLimitRoundTripper.
type RateLimitRoundTripper struct {
	inner    http.RoundTripper
	limiter  *rate.TokenBucket
	observer *ProtocolRecorder
}

// SetProtocolRecorder points the rate limiter at the client's protocol
// recorder so a locally refused request is reported as source "local" with no
// version (PROTOCOLS.md §4.7 N3): no exchange took place, so there is no
// negotiated version to claim.
func (r *RateLimitRoundTripper) SetProtocolRecorder(rec *ProtocolRecorder) {
	r.observer = rec
}

// NewRateLimitRoundTripper creates a rate-limiting round tripper that wraps
// the given inner transport. ratePerSec is the number of tokens added per
// second; burst is the maximum accumulated tokens. When ratePerSec is 0,
// no tokens are added after the initial burst — the bucket is effectively
// a one-time allowance of burst requests.
//
// The caller should call Stop on the returned transport when done to
// release the background refill goroutine.
func NewRateLimitRoundTripper(inner http.RoundTripper, ratePerSec, burst int) *RateLimitRoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &RateLimitRoundTripper{
		inner:   inner,
		limiter: rate.NewTokenBucket(ratePerSec, burst),
	}
}

// RoundTrip executes the request through the rate limiter. If the token
// bucket has a token available, the request is forwarded to the inner
// transport. Otherwise, an HTTP 429 Too Many Requests response is returned
// immediately without contacting the upstream server.
//
// The returned 429 response includes a body with a clear error message
// and a Content-Length header. The caller must call resp.Body.Close().
func (r *RateLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !r.limiter.Allow() {
		// Report the refusal before synthesizing the response: no exchange
		// happened, so this request has no negotiated version. The Proto field
		// below is the response object's local framing metadata (req.Proto),
		// not an observed version — it must never be read as one (PROTOCOLS.md
		// §4.7 N1/N3; DF-041 covers the response-object fidelity fix).
		if r.observer != nil {
			r.observer.note(req.Method, req.URL.String(), SourceLocal, "")
		}
		body := "rate limit exceeded\n"
		return &http.Response{
			Status:        "429 Too Many Requests",
			StatusCode:    http.StatusTooManyRequests,
			Proto:         req.Proto,
			ProtoMajor:    req.ProtoMajor,
			ProtoMinor:    req.ProtoMinor,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
		}, nil
	}
	return r.inner.RoundTrip(req)
}

// Stop stops the underlying token bucket's background refill goroutine.
// After Stop, no more tokens are added to the bucket, but existing tokens
// can still be consumed. Safe to call multiple times.
func (r *RateLimitRoundTripper) Stop() {
	r.limiter.Stop()
}

// WithRateLimiter returns a ClientOption that wraps the HTTP client's
// transport with rate limiting. The limiter allows up to burst requests
// at any instant and refills at ratePerSec tokens per second.
//
// When ratePerSec is less than or equal to 0, the option is a no-op and
// rate limiting is disabled — existing client behavior is unchanged.
//
// Example:
//
//	client := client.NewClient(client.WithRateLimiter(10, 5))
//	// Allows bursts of 5, sustained at 10 req/s
func WithRateLimiter(ratePerSec, burst int) ClientOption {
	return func(c *Client) {
		if ratePerSec <= 0 {
			return // disabled — no wrapping, existing behavior unchanged
		}

		transport := c.httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		limitRT := NewRateLimitRoundTripper(transport, ratePerSec, burst)
		limitRT.SetProtocolRecorder(c.protocol)
		c.httpClient.Transport = limitRT
	}
}
