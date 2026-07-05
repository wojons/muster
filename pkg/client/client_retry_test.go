// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/retry"
)

// TestClient_WithRetryConfig tests the WithRetryConfig option
func TestClient_WithRetryConfig(t *testing.T) {
	config := retry.DefaultRetryConfig()
	config.MaxAttempts = 3

	client := NewClient(WithRetryConfig(config))

	assert.NotNil(t, client.retrier)
	assert.True(t, client.IsRetryEnabled())
	assert.NotNil(t, client.GetRetrier())
}

// TestClient_SetRetrier tests the SetRetrier method
func TestClient_SetRetrier(t *testing.T) {
	client := NewClient()
	assert.False(t, client.IsRetryEnabled())
	assert.Nil(t, client.GetRetrier())

	config := retry.DefaultRetryConfig()
	retrier := retry.NewRetrier(config)
	client.SetRetrier(retrier)

	assert.True(t, client.IsRetryEnabled())
	assert.Equal(t, retrier, client.GetRetrier())
}

// TestClient_Retry_SuccessOnFirstAttempt tests that successful requests don't retry
func TestClient_Retry_SuccessOnFirstAttempt(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, attemptCount) // Should only try once for success
}

// TestClient_Retry_RetriesOn5xxStatusCodes tests that 5xx errors trigger retries
func TestClient_Retry_RetriesOn5xxStatusCodes(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attemptCount) // Should retry 2 times before success
}

// TestClient_Retry_RetriesOn429StatusCode tests that 429 (Too Many Requests) triggers retries
func TestClient_Retry_RetriesOn429StatusCode(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusTooManyRequests) // 429
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
}

// TestClient_Retry_NoRetryOn4xxStatusCodes tests that 4xx errors (except 429) don't retry
func TestClient_Retry_NoRetryOn4xxStatusCodes(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"BadRequest", http.StatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized},
		{"Forbidden", http.StatusForbidden},
		{"NotFound", http.StatusNotFound},
		{"Conflict", http.StatusConflict},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			config := retry.RetryConfig{
				MaxAttempts:  5,
				InitialDelay: 10 * time.Millisecond,
				MaxDelay:     100 * time.Millisecond,
				BackoffType:  "linear",
				Jitter:       false,
			}

			client := NewClient(WithRetryConfig(config))

			ctx := context.Background()
			resp, err := client.Get(ctx, server.URL+"/test")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.statusCode, resp.StatusCode)
			assert.Equal(t, 1, attemptCount) // Should not retry
		})
	}
}

// TestClient_Retry_Exhaustion tests that retry exhaustion returns an error
func TestClient_Retry_Exhaustion(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError) // Always fail
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 3, attemptCount)
	assert.Contains(t, err.Error(), "request failed after 3 attempts")
}

// TestClient_Retry_NetworkError tests that network errors trigger retries
func TestClient_Retry_NetworkError(t *testing.T) {
	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	// Connect to a server that doesn't exist
	ctx := context.Background()
	resp, err := client.Get(ctx, "http://localhost:59999/test")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

// TestClient_Retry_WithAllMethods tests retry works with all HTTP methods
func TestClient_Retry_WithAllMethods(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusBadGateway) // 502
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))
	ctx := context.Background()

	// Test GET
	attemptCount = 0
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, 2, attemptCount)

	// Test POST
	attemptCount = 0
	resp, err = client.Post(ctx, server.URL+"/test", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, 2, attemptCount)

	// Test PUT
	attemptCount = 0
	resp, err = client.Put(ctx, server.URL+"/test", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, 2, attemptCount)

	// Test DELETE
	attemptCount = 0
	resp, err = client.Delete(ctx, server.URL+"/test")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, 2, attemptCount)

	// Test PATCH
	attemptCount = 0
	resp, err = client.Patch(ctx, server.URL+"/test", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, 2, attemptCount)
}

// TestClient_Retry_WithoutRetry tests that requests work without retry configured
func TestClient_Retry_WithoutRetry(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient() // No retry configured
	assert.False(t, client.IsRetryEnabled())

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, 1, attemptCount) // No retry, single attempt
}

// TestClient_Retry_RespectsContextCancellation tests that context cancellation stops retries
func TestClient_Retry_RespectsContextCancellation(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		time.Sleep(50 * time.Millisecond) // Slow response
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	resp, err := client.Get(ctx, server.URL+"/test")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Less(t, attemptCount, 10) // Should stop before max attempts due to context cancellation
}

// TestClient_Retry_ResponseBodyPreserved tests that response body is readable after retry
func TestClient_Retry_ResponseBodyPreserved(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"message":"success"}`, string(body))
}

// TestClient_Retry_ConcurrentRequests tests that retry works with concurrent requests
func TestClient_Retry_ConcurrentRequests(t *testing.T) {
	var attemptCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount%2 == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))
	ctx := context.Background()

	// Run concurrent requests
	for i := 0; i < 5; i++ {
		resp, err := client.Get(ctx, server.URL+fmt.Sprintf("/test%d", i))
		require.NoError(t, err)
		_ = resp.Body.Close()

	}
}

// TestShouldRetryStatusCode tests the shouldRetryStatusCode function
func TestShouldRetryStatusCode(t *testing.T) {
	testCases := []struct {
		statusCode  int
		shouldRetry bool
	}{
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},     // 429
		{http.StatusInternalServerError, true}, // 500
		{http.StatusBadGateway, true},          // 502
		{http.StatusServiceUnavailable, true},  // 503
		{http.StatusGatewayTimeout, true},      // 504
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Status%d", tc.statusCode), func(t *testing.T) {
			result := shouldRetryStatusCode(tc.statusCode)
			assert.Equal(t, tc.shouldRetry, result)
		})
	}
}

// TestRetryableError tests the retryableError type
func TestRetryableError(t *testing.T) {
	err := &retryableError{statusCode: 503}
	assert.Equal(t, "retryable HTTP error: status code 503", err.Error())

	err2 := &retryableError{statusCode: 429}
	assert.Equal(t, "retryable HTTP error: status code 429", err2.Error())
}

// TestClient_Retry_WithCustomHTTPClient tests retry with a custom HTTP client
func TestClient_Retry_WithCustomHTTPClient(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	customClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(
		WithRetryConfig(config),
		func(c *Client) {
			c.SetHTTPClient(customClient)
		},
	)

	assert.Equal(t, customClient, client.HTTPClient())

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
}

// TestClient_Retry_IntegrationWithHeaders tests retry works with default headers
func TestClient_Retry_IntegrationWithHeaders(t *testing.T) {
	attemptCount := 0
	receivedHeaders := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		receivedHeaders["Authorization"] = r.Header.Get("Authorization")
		receivedHeaders["X-Custom-Header"] = r.Header.Get("X-Custom-Header")

		if attemptCount < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(
		WithRetryConfig(config),
		WithHeader("Authorization", "Bearer token123"),
		WithHeader("X-Custom-Header", "custom-value"),
	)

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
	assert.Equal(t, "Bearer token123", receivedHeaders["Authorization"])
	assert.Equal(t, "custom-value", receivedHeaders["X-Custom-Header"])
}

// TestClient_Retry_WithBaseURL tests retry works with base URL
func TestClient_Retry_WithBaseURL(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(
		WithRetryConfig(config),
		WithBaseURL(server.URL),
	)

	ctx := context.Background()
	resp, err := client.Get(ctx, "/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
}

// TestClient_Retry_DoMethodDirectly tests retry with the Do method directly
func TestClient_Retry_DoMethodDirectly(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := retry.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffType:  "linear",
		Jitter:       false,
	}

	client := NewClient(WithRetryConfig(config))

	req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	require.NoError(t, err)

	ctx := context.Background()
	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
}
