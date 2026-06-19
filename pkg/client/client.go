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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wojons/muster/internal/telemetry"
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
// Example:
//
//	client := client.NewClient()
//	resp, err := client.Get(ctx, "https://api.example.com/data")
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers: make(map[string]string),
	}

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
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// If retrier is configured, use retry logic
	if c.retrier != nil {
		return c.doWithRetry(ctx, req)
	}
	return c.doOnce(ctx, req)
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

	// Execute the request with context
	resp, err := c.httpClient.Do(req.WithContext(ctx))

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

	return result.(*http.Response), nil
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
func (c *Client) SetHTTPClient(httpClient *http.Client) {
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
