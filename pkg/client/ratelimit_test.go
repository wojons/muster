package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRateLimiter_AllowsWithinLimit verifies that requests within the
// token bucket's burst limit are allowed through to the upstream server.
func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// rate=100, burst=5: starts with 5 tokens, fast refill.
	client := NewClient(WithRateLimiter(100, 5))
	transport := client.HTTPClient().Transport.(*RateLimitRoundTripper)
	defer transport.Stop()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		resp, err := client.Get(ctx, server.URL+"/test")
		require.NoError(t, err, "request %d should not error", i+1)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should be allowed", i+1)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// TestRateLimiter_BlocksExcess verifies that requests exceeding the
// available tokens receive a 429 Too Many Requests response.
func TestRateLimiter_BlocksExcess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// rate=10, burst=1: starts with 1 token, refills at 10 tokens/sec.
	// Two back-to-back requests — the second arrives before a refill tick.
	client := NewClient(WithRateLimiter(10, 1))
	transport := client.HTTPClient().Transport.(*RateLimitRoundTripper)
	defer transport.Stop()

	ctx := context.Background()

	// First request consumes the initial token.
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Second request should be rate-limited (no tokens available yet).
	resp, err = client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "rate limit exceeded")
	resp.Body.Close()
}

// TestRateLimiter_Disabled verifies that when WithRateLimiter is not used,
// all requests pass through without any rate limiting — existing behavior
// is unchanged.
func TestRateLimiter_Disabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// No rate limiter configured — default disabled.
	client := NewClient()

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		resp, err := client.Get(ctx, server.URL+"/test")
		require.NoError(t, err, "request %d should not error", i+1)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// TestRateLimiter_WithRateZero verifies that calling WithRateLimiter with
// a non-positive rate leaves the transport unwrapped (no-op).
func TestRateLimiter_WithRateZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// rate=0 means disabled — no transport wrapping occurs.
	client := NewClient(WithRateLimiter(0, 5))

	// Verify the transport was NOT wrapped (rate <= 0 is no-op).
	_, ok := client.HTTPClient().Transport.(*RateLimitRoundTripper)
	assert.False(t, ok, "transport should NOT be wrapped when rate <= 0")

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		resp, err := client.Get(ctx, server.URL+"/test")
		require.NoError(t, err, "request %d should not error", i+1)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// TestRateLimiter_Concurrent verifies that the rate limiter is safe for
// concurrent use under the token bucket's mutex protection.
func TestRateLimiter_Concurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Burst of 10 allows all 10 concurrent goroutines to succeed.
	client := NewClient(WithRateLimiter(1000, 10))
	transport := client.HTTPClient().Transport.(*RateLimitRoundTripper)
	defer transport.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(ctx, server.URL+"/test")
			if err != nil {
				errs <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				errs <- assert.AnError // unexpected status
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent request failed: %v", err)
		}
	}
}

// TestRateLimiter_429ResponseBody verifies the shape of the 429 response:
// correct status code, readable body, and Content-Length header.
func TestRateLimiter_429ResponseShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// rate=1, burst=1: only one request is allowed before the bucket empties.
	client := NewClient(WithRateLimiter(1, 1))
	transport := client.HTTPClient().Transport.(*RateLimitRoundTripper)
	defer transport.Stop()

	ctx := context.Background()

	// Consume the single token.
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Get rate-limited.
	resp, err = client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "429 Too Many Requests", resp.Status)
	assert.Greater(t, resp.ContentLength, int64(0), "Content-Length should be set")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "rate limit exceeded")
}

// ── NewRateLimitRoundTripper ──────────────────────────────────────────

func TestNewRateLimitRoundTripper_ValidRateAndBurst(t *testing.T) {
	inner := http.DefaultTransport
	rt := NewRateLimitRoundTripper(inner, 100, 20)
	require.NotNil(t, rt)
	assert.NotNil(t, rt.inner, "inner transport should not be nil")
	assert.NotNil(t, rt.limiter, "limiter should not be nil")

	// Verify the inner is the one we passed
	assert.Same(t, inner, rt.inner)

	rt.Stop()
}

func TestNewRateLimitRoundTripper_ZeroRate(t *testing.T) {
	// Zero rate is valid — no tokens are added after initial burst
	inner := http.DefaultTransport
	rt := NewRateLimitRoundTripper(inner, 0, 10)
	require.NotNil(t, rt)
	assert.NotNil(t, rt.limiter)

	rt.Stop()
}

func TestNewRateLimitRoundTripper_ZeroBurst(t *testing.T) {
	// Zero burst is valid — no tokens initially, none can accumulate
	inner := http.DefaultTransport
	rt := NewRateLimitRoundTripper(inner, 10, 0)
	require.NotNil(t, rt)
	assert.NotNil(t, rt.limiter)

	rt.Stop()
}

func TestNewRateLimitRoundTripper_NilInner(t *testing.T) {
	// nil inner should fall back to http.DefaultTransport
	rt := NewRateLimitRoundTripper(nil, 10, 5)
	require.NotNil(t, rt)
	assert.NotNil(t, rt.inner, "inner should default to DefaultTransport when nil is passed")

	rt.Stop()
}

// ── RoundTrip ─────────────────────────────────────────────────────────

func TestRoundTrip_PassesThroughResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "pass-through")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// rate=100, burst=10 ensures the request is allowed
	rt := NewRateLimitRoundTripper(http.DefaultTransport, 100, 10)
	defer rt.Stop()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "pass-through", resp.Header.Get("X-Test"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"ok"}`, string(body))
}

func TestRoundTrip_BlocksWhenRateExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// rate=0, burst defaults to 1 → 1 initial token, then no refill
	rt := NewRateLimitRoundTripper(http.DefaultTransport, 0, 0)
	defer rt.Stop()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	require.NoError(t, err)

	// First request consumes the single initial token
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Second request — no tokens left, burst=1 was consumed
	resp2, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp2.StatusCode)
	assert.Equal(t, "429 Too Many Requests", resp2.Status)

	body, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "rate limit exceeded")
	assert.Equal(t, int64(len(body)), resp2.ContentLength)
}

func TestRoundTrip_429ResponseBodyContents(t *testing.T) {
	// Verify the 429 response body is readable and contains a clear message.
	// rate=0, burst defaults to 1 → consume it first, then test 429.
	rt := NewRateLimitRoundTripper(http.DefaultTransport, 0, 0)
	defer rt.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Consume the initial token
	req1, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp1, _ := rt.RoundTrip(req1)
	if resp1 != nil {
		resp1.Body.Close()
	}

	// Now no tokens left — next request gets 429
	req2, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp, err := rt.RoundTrip(req2)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "rate limit exceeded\n", string(body))
}

func TestRoundTrip_ProtoPreservedOn429(t *testing.T) {
	// Verify proto fields are preserved in the synthetic 429 response
	rt := NewRateLimitRoundTripper(http.DefaultTransport, 0, 0)
	defer rt.Stop()

	// Consume the initial token first
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req1, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp1, _ := rt.RoundTrip(req1)
	if resp1 != nil {
		resp1.Body.Close()
	}

	// Now make the 429-bound request with proto fields set
	req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	require.NoError(t, err)
	req.Proto = "HTTP/2.0"
	req.ProtoMajor = 2
	req.ProtoMinor = 0

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "HTTP/2.0", resp.Proto)
	assert.Equal(t, 2, resp.ProtoMajor)
	assert.Equal(t, 0, resp.ProtoMinor)
}
