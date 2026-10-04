// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

// Package client provides HTTP client functionality with OpenTelemetry tracing support.
//
// This package implements a traced HTTP client that automatically creates spans
// for all HTTP requests, including detailed attributes for method, URL, status code,
// and error conditions.
//
// Basic Usage:
//
//	client := client.NewClient()
//	resp, err := client.Get(ctx, "https://api.example.com/data")
//
// With Tracing:
//
//	tracer := telemetry.NewTracer(tp.Tracer("http-client"))
//	client := client.NewClientWithTracer(tracer)
//	resp, err := client.Get(ctx, "https://api.example.com/data")
//	// Automatically creates spans for the request
//
// With Retry:
//
//	config := retry.DefaultRetryConfig()
//	config.MaxAttempts = 5
//	client := client.NewClient(client.WithRetryConfig(config))
//	resp, err := client.Get(ctx, "https://api.example.com/data")
//	// Automatically retries on transient failures
package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wojons/muster/internal/telemetry"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/retry"
	"go.opentelemetry.io/otel/trace"
)

// Client is an HTTP client with tracing and retry support.
type Client struct {
	httpClient *http.Client
	tracer     *telemetry.Tracer
	retrier    *retry.Retrier
	baseURL    string
	headers    map[string]string

	// protocol records the HTTP transport version every exchange used, and
	// httpVersion is the policy in force (PROTOCOLS.md §4.7). baseTransport is
	// the value that performs the exchange; h3Capable is true when that value
	// can attempt HTTP/3 (WithHTTPVersion("3") installs one), which is what
	// makes the "3" policy satisfiable; http3Options are the settings the
	// policy's HTTP/3 transport is (re-)built with. chain is the top of the
	// transport chain the option pipeline assembled — wrapper options such as
	// WithCache and WithRateLimiter above the recorder link — kept so
	// SetHTTPClient can carry the whole chain onto a replacement client
	// (DF-042).
	protocol      *ProtocolRecorder
	baseTransport http.RoundTripper
	h3Capable     bool
	httpVersion   string
	http3Options  []HTTP3Option
	chain         http.RoundTripper
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL sets the base URL for all requests.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHeader adds a default header to all requests.
func WithHeader(key, value string) ClientOption {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithRetryConfig sets the retry configuration for the client.
// When configured, HTTP requests will be automatically retried on transient failures.
//
// Example:
//
//	config := retry.DefaultRetryConfig()
//	config.MaxAttempts = 5
//	client := client.NewClient(client.WithRetryConfig(config))
func WithRetryConfig(config retry.RetryConfig) ClientOption {
	return func(c *Client) {
		c.retrier = retry.NewRetrier(config)
	}
}

// NewClient creates a new HTTP client with tracing support.
//
// The client always installs a base transport and a protocol recorder
// (PROTOCOLS.md §4.7 N2): the default transport is HTTP/2-capable by
// construction rather than by relying on the nil-transport fallback to
// http.DefaultTransport, and the recorder sits below any wrapper option so it
// observes what the network actually did. Use WithTransport to install another
// transport and WithHTTPVersion to select a version policy.
//
// Example:
//
//	client := client.NewClient()
//	resp, err := client.Get(ctx, "https://api.example.com/data")
func NewClient(opts ...ClientOption) *Client {
	rec := NewProtocolRecorder()
	base := DefaultTransport()
	link := newProtocolTransport(base, rec)
	c := &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: link,
		},
		headers:       make(map[string]string),
		protocol:      rec,
		baseTransport: base,
		chain:         link,
		httpVersion:   HTTPVersionAuto,
	}
	rec.SetPolicy(HTTPVersionAuto)

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// NewClientWithTracer creates a new HTTP client with tracing.
//
// Example:
//
//	tracer := telemetry.NewTracer(tp.Tracer("http-client"))
//	client := client.NewClientWithTracer(tracer)
func NewClientWithTracer(tracer *telemetry.Tracer, opts ...ClientOption) *Client {
	c := NewClient(opts...)
	c.tracer = tracer
	return c
}

// SetTracer sets the tracer for the client.
func (c *Client) SetTracer(tracer *telemetry.Tracer) {
	c.tracer = tracer
}

// Do executes an HTTP request with tracing and optional retry logic.
//
// The transport version policy is enforced here, once per request: a policy
// this build cannot honour is rejected before the exchange, and a required
// version the peer did not use is reported as CodeProtocolMismatch after it
// (PROTOCOLS.md §4.7 N5). The check runs after the retry loop so a version
// mismatch — which is deterministic — is never retried.
//
// A transport-level failure that carries a coded meaning is translated here
// too: an HTTP/3 policy on a cleartext endpoint is a CodeProtocolMismatch with
// reason "plaintext-endpoint", and a transport timeout is CodeTimeout (§4.2).
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.protocolPreflight(); err != nil {
		return nil, err
	}

	var (
		resp *http.Response
		err  error
	)
	if c.retrier != nil {
		resp, err = c.doWithRetry(ctx, req)
	} else {
		resp, err = c.doOnce(ctx, req)
	}
	if err != nil {
		if mapped := c.transportError(err); mapped != nil {
			return nil, mapped
		}
		return nil, err
	}

	if mismatch := c.protocolOutcome(req); mismatch != nil {
		// The exchange happened; the caller gets the coded failure instead of
		// a response that silently used another version.
		_ = resp.Body.Close()
		return nil, mismatch
	}

	return resp, nil
}

// transportError translates the transport failures that have a coded meaning
// into the AppError the contract names (PROTOCOLS.md §4.2/§4.7 N5), or returns
// nil when the error passes through untouched:
//
//   - policy "3" against a cleartext endpoint: QUIC mandates TLS 1.3 and the
//     stdlib has no h2c, so no permitted version can carry the request —
//     CodeProtocolMismatch with reason "plaintext-endpoint" (rather than the
//     HTTP/3 transport's own "unsupported protocol scheme").
//   - a context deadline: CodeTimeout, per §4.2's "any transport timeout"
//     row. Cancellation is not a timeout and is left as it is.
func (c *Client) transportError(err error) error {
	switch {
	case stderrors.Is(err, ErrHTTP3RequiresTLS):
		return protocolMismatch(HTTPVersion3, "", ReasonPlaintextEndpoint)
	case stderrors.Is(err, context.DeadlineExceeded):
		return errors.Wrap(err, errors.CodeTimeout, "http: request timed out").
			WithDetail("requested", c.httpVersion)
	}
	return nil
}

// protocolPreflight rejects a policy this build cannot honour, before any
// request goes out: an unknown version (CodeValidation) and policy "3" without
// an HTTP/3-capable transport (CodeProtocolMismatch, N5/N8). It never rejects a
// policy the caller did not require.
func (c *Client) protocolPreflight() error {
	version := c.httpVersion
	switch version {
	case "", HTTPVersionAuto, HTTPVersion11, HTTPVersion2:
		return nil
	case HTTPVersion3:
		if c.h3Capable {
			// The installed transport can attempt HTTP/3 (WithHTTPVersion("3")
			// installs HTTP3FallbackTransport; a caller may install one, or a
			// bare http3.Transport, itself). Satisfaction is reported per
			// request, and a fallback, when it happens, is reported by the
			// transport that performed it.
			return nil
		}
		return protocolMismatch(HTTPVersion3, "", ReasonHTTP3TransportMissing)
	default:
		return errors.New(errors.CodeValidation,
			fmt.Sprintf("invalid http version %q: want auto, 1.1, 2 or 3", version)).
			WithDetail("value", version)
	}
}

// protocolOutcome reports the required-version failure for the exchange just
// performed, or nil when there is nothing to report. "auto" never fails on
// version; policy "3" never fails here either, because a version other than
// HTTP/3 under a "3" preference is a fallback (PROTO-015), not a mismatch. A
// request that never reached the wire (a local refusal such as the rate
// limiter's 429, or a cache-served response) used no version, so there is no
// downgrade to report — the record says source local/cache with no version.
func (c *Client) protocolOutcome(req *http.Request) error {
	if c.protocol == nil {
		return nil
	}
	rec := c.protocol.Last()
	if rec.Method != req.Method || req.URL == nil || rec.URL != req.URL.String() {
		// No observation for this request: nothing to report. (This is the
		// shape left by SetHTTPClient, which discards the recorder with the
		// rest of the option-installed chain — forbidden for version
		// selection, defect DF-042.)
		return nil
	}
	switch rec.Requested {
	case HTTPVersion11, HTTPVersion2:
	default:
		return nil
	}
	if rec.Observed == "" || rec.Satisfied {
		return nil
	}

	reason := serverOfferedReason(rec.Observed)
	if req.URL != nil && req.URL.Scheme == "http" && rec.Observed == WireVersionHTTP11 {
		// Cleartext has no h2c in the stdlib and QUIC requires TLS 1.3, so a
		// plaintext endpoint can never satisfy a required 2 or 3 (N5).
		reason = ReasonPlaintextEndpoint
	}
	return protocolMismatch(rec.Requested, rec.Observed, reason)
}

// Protocol returns the record for the exchange this client performed most
// recently (PROTOCOLS.md §4.7 N3), including the policy in force. It is the
// surface product code reads instead of http.Response.Proto, which the cache
// and the local rate-limit refusal rebuild. On a cache-served response the
// record reports source "cache" with the version stored with the cache entry;
// a response with no wire exchange reports no version at all, never a default
// of HTTP/1.1.
func (c *Client) Protocol() ProtocolRecord {
	if c == nil || c.protocol == nil {
		return ProtocolRecord{}
	}
	return c.protocol.Last()
}

// ProtocolRecorder returns the recorder that collects this client's
// observations, for callers that need a per-request lookup
// (ProtocolRecorder.For) rather than the most recent exchange.
func (c *Client) ProtocolRecorder() *ProtocolRecorder {
	if c == nil {
		return nil
	}
	return c.protocol
}

// BaseTransport returns the transport that performs the exchange, below the
// protocol recorder and every wrapper option. It is nil only for a Client not
// built by NewClient.
func (c *Client) BaseTransport() http.RoundTripper {
	if c == nil {
		return nil
	}
	return c.baseTransport
}

// doOnce executes a single HTTP request with tracing.
func (c *Client) doOnce(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Create span if tracer is configured
	var span *telemetry.Span
	if c.tracer != nil {
		ctx, span = c.tracer.Start(ctx, "http.request",
			telemetry.WithSpanKind(trace.SpanKindClient),
			telemetry.WithAttributes(),
		)
		defer span.End()

		// Add request attributes
		span.SetAttribute("http.method", req.Method)
		span.SetAttribute("http.url", req.URL.String())
		span.SetAttribute("http.host", req.URL.Host)
		span.SetAttribute("http.scheme", req.URL.Scheme)
		span.SetAttribute("http.target", req.URL.Path)
	}

	// Apply default headers
	for key, value := range c.headers {
		if req.Header.Get(key) == "" {
			req.Header.Set(key, value)
		}
	}

	// The caller's context deadline governs the exchange (PROTOCOLS.md
	// §4.3: no layer sets a default timeout that overrides the caller's).
	// The stdlib enforces http.Client.Timeout as the shorter of the caller
	// deadline and the configured default, which would clamp a caller
	// deadline the default happens to exceed — so a deadline-carrying
	// caller runs on a per-request copy with the private Timeout disarmed.
	// Deadline-less callers keep the default unchanged (the field remains
	// its storage, and the zero value means no bound). Cancellation and
	// budget metadata on the caller's context ride along with WithContext.
	callClient := c.httpClient
	if _, hasDeadline := ctx.Deadline(); hasDeadline && callClient.Timeout > 0 {
		hc := *callClient
		hc.Timeout = 0
		callClient = &hc
	}

	// Execute the request with context
	resp, err := callClient.Do(req.WithContext(ctx))

	// Record response or error
	if err != nil {
		if span != nil {
			span.RecordError(err)
			span.SetAttributeBool("http.error", true)
			span.SetAttribute("error.message", err.Error())
		}
		return nil, err
	}

	if span != nil {
		span.SetAttributeInt("http.status_code", int64(resp.StatusCode))
		span.SetAttributeInt("http.response_content_length", resp.ContentLength)
		span.SetAttribute("http.response_content_type", resp.Header.Get("Content-Type"))

		// Mark as error for 4xx and 5xx status codes
		if resp.StatusCode >= 400 {
			span.SetAttributeBool("http.error", true)
			span.SetAttribute("http.error_class", fmt.Sprintf("%dxx", resp.StatusCode/100))
		} else {
			span.SetOK()
		}
	}

	return resp, nil
}

// doWithRetry executes an HTTP request with retry logic.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	result, err := c.retrier.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		resp, err := c.doOnce(ctx, req)
		if err != nil {
			return nil, err
		}

		// Check for retryable status codes (5xx and certain 4xx)
		if shouldRetryStatusCode(resp.StatusCode) {
			// Close the body to prevent resource leaks during retries
			_ = resp.Body.Close()
			return nil, &retryableError{statusCode: resp.StatusCode}
		}

		return resp, nil
	})

	if err != nil {
		// Check if it's a retry exhausted error and preserve the last response info
		if retryErr, ok := err.(*retry.RetryExhaustedError); ok {
			return nil, fmt.Errorf("request failed after %d attempts: %w", retryErr.Attempts, retryErr.LastErr)
		}
		return nil, err
	}

	res, ok := result.(*http.Response)
	if !ok {
		return nil, fmt.Errorf("request: unexpected result type %T", result)
	}
	return res, nil
}

// shouldRetryStatusCode determines if an HTTP status code should trigger a retry.
func shouldRetryStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// retryableError represents an HTTP error that can be retried.
type retryableError struct {
	statusCode int
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable HTTP error: status code %d", e.statusCode)
}

// Get performs an HTTP GET request with tracing.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fullURL(url), nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Post performs an HTTP POST request with tracing.
func (c *Client) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.fullURL(url), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(ctx, req)
}

// Put performs an HTTP PUT request with tracing.
func (c *Client) Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.fullURL(url), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(ctx, req)
}

// Delete performs an HTTP DELETE request with tracing.
func (c *Client) Delete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.fullURL(url), nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Patch performs an HTTP PATCH request with tracing.
func (c *Client) Patch(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.fullURL(url), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(ctx, req)
}

// fullURL returns the full URL for a request.
func (c *Client) fullURL(url string) string {
	if c.baseURL != "" {
		return c.baseURL + url
	}
	return url
}

// HTTPClient returns the underlying HTTP client.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// SetHTTPClient sets the underlying HTTP client.
//
// The transport chain the option pipeline built — the middleware the wrapper
// options installed (WithCache, WithRateLimiter) above the protocol recorder
// — is carried over: the replacement client's transport becomes the exchange
// performer below the chain, so cache and rate limiting keep operating
// exactly as before the swap (defect DF-042; PROTOCOLS.md §4.7 N2's "wrapper
// options sit above the recorder"). A replacement with no transport dials
// through http.DefaultTransport, the same fallback the wrapper options use.
//
// A nil httpClient strips the chain — wrappers and recorder alike — and the
// client carries on with a bare default HTTP client. This is the only way to
// remove installed middleware. A later SetHTTPClient gives the replacement a
// fresh recorder-only chain.
func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient == nil {
		// A nil field itself would make the next Do panic, so the strip
		// installs a bare client instead of no client.
		c.httpClient = &http.Client{}
		c.chain = nil
		return
	}
	if c.chain == nil {
		// The chain was stripped (or the Client was not built by NewClient):
		// the replacement gets a fresh recorder-only chain instead of
		// inheriting middleware that no longer exists.
		c.chain = newProtocolTransport(httpClient.Transport, c.protocol)
		httpClient.Transport = c.chain
		c.httpClient = httpClient
		return
	}
	if c.chain != httpClient.Transport {
		// Re-bind the base of the carried chain to the replacement's
		// transport, leaving every wrapper above it in place.
		inner := httpClient.Transport
		if inner == nil {
			inner = http.DefaultTransport
		}
		base := c.chain
		for {
			if pt, ok := base.(*protocolTransport); ok {
				pt.inner = inner
				c.baseTransport = inner
				break
			}
			switch w := base.(type) {
			case *CacheRoundTripper:
				base = w.inner
			case *RateLimitRoundTripper:
				base = w.inner
			default:
				// Reached the top without finding the recorder link: the
				// chain is not one this client can carry, so the replacement
				// keeps its own transport untouched.
				c.httpClient = httpClient
				return
			}
		}
		httpClient.Transport = c.chain
	}
	c.httpClient = httpClient
}

// SetRetrier sets the retry configuration for the client.
// This allows dynamic configuration of retry behavior after client creation.
//
// Example:
//
//	client := client.NewClient()
//	config := retry.DefaultRetryConfig()
//	config.MaxAttempts = 5
//	client.SetRetrier(retry.NewRetrier(config))
func (c *Client) SetRetrier(retrier *retry.Retrier) {
	c.retrier = retrier
}

// GetRetrier returns the current retrier configuration, or nil if not configured.
func (c *Client) GetRetrier() *retry.Retrier {
	return c.retrier
}

// IsRetryEnabled returns true if retry is configured for this client.
func (c *Client) IsRetryEnabled() bool {
	return c.retrier != nil
}
