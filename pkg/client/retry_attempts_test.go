package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// QA-MUSTER-044: the retry implementation (WithRetryConfig, doWithRetry) was
// documented but unreachable, MUSTER_MAX_RETRIES was parsed and never read,
// and Retry-After was ignored. These tests pin the three pieces that make the
// wiring honest: the attempt ceiling resolves from the environment, a
// Retry-After response both surfaces the value and delays the next attempt,
// and an exhausted retry reports the attempts it made.

func TestMaxAttemptsFromEnv(t *testing.T) {
	t.Run("unset uses the default", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "")
		// An empty value is present but unparseable: the default wins.
		assert.Equal(t, DefaultMaxAttempts, MaxAttemptsFromEnv())
	})

	t.Run("explicit values are used verbatim", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "5")
		assert.Equal(t, 5, MaxAttemptsFromEnv())
	})

	t.Run("zero disables retries", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "0")
		assert.Equal(t, 1, MaxAttemptsFromEnv())
	})

	t.Run("negative is clamped to one attempt", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "-2")
		assert.Equal(t, 1, MaxAttemptsFromEnv())
	})

	t.Run("garbage falls back to the default", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "not-a-number")
		assert.Equal(t, DefaultMaxAttempts, MaxAttemptsFromEnv())
	})
}

func TestRetryConfigFor(t *testing.T) {
	assert.Equal(t, 3, RetryConfigFor(3).MaxAttempts)
	assert.Equal(t, 1, RetryConfigFor(0).MaxAttempts, "an attempt ceiling below 1 is clamped to a single attempt")
	assert.Equal(t, "exponential", RetryConfigFor(2).BackoffType)
}

func TestRetryAfterDelay(t *testing.T) {
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)

	tests := []struct {
		name      string
		header    string
		wantOK    bool
		wantMin   time.Duration
		wantExact bool
		want      time.Duration
	}{
		{name: "absent", header: "", wantOK: false},
		{name: "seconds", header: "3", wantOK: true, wantExact: true, want: 3 * time.Second},
		{name: "zero seconds", header: "0", wantOK: true, wantExact: true, want: 0},
		{name: "negative", header: "-5", wantOK: false},
		{name: "garbage", header: "soon", wantOK: false},
		{name: "http date", header: future, wantOK: true, wantMin: time.Second},
		{name: "past http date", header: time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tc.header != "" {
				resp.Header.Set("Retry-After", tc.header)
			}
			got, ok := retryAfterDelay(resp)
			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				return
			}
			if tc.wantExact {
				assert.Equal(t, tc.want, got)
				return
			}
			assert.GreaterOrEqual(t, got, tc.wantMin)
		})
	}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	require.NoError(t, w.Close())
	captured, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(captured)
}

// TestDoWithRetry_HonoursRetryAfter proves a 429 carrying Retry-After both
// reaches stderr and sets the floor for the wait before the next attempt.
func TestDoWithRetry_HonoursRetryAfter(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&count, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(WithRetryConfig(RetryConfigFor(2)))
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/limited", nil)
	require.NoError(t, err)

	var (
		resp  *http.Response
		doErr error
	)
	start := time.Now()
	stderr := captureStderr(t, func() {
		resp, doErr = c.Do(context.Background(), req)
	})
	elapsed := time.Since(start)

	require.NoError(t, doErr)
	require.NotNil(t, resp)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&count), "one 429 then one success")
	assert.Contains(t, stderr, "honouring Retry-After: 1s",
		"the Retry-After value must be surfaced on stderr")
	assert.Contains(t, stderr, "retrying",
		"stderr must say the request is being retried")
	assert.GreaterOrEqual(t, elapsed, time.Second,
		"the wait must honour Retry-After rather than the short default backoff")
}

// TestDoWithRetry_ExhaustionReturnsLastResponse proves what an exhausted
// STATUS ceiling does: the retrier stops, and the caller still receives the
// final response so its own 4xx/5xx handling (error text, body rendering) is
// exactly what it would be without retries. The attempts are on stderr.
func TestDoWithRetry_ExhaustionReturnsLastResponse(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(WithRetryConfig(RetryConfigFor(2)))
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/down", nil)
	require.NoError(t, err)

	var (
		resp  *http.Response
		doErr error
	)
	stderr := captureStderr(t, func() {
		resp, doErr = c.Do(context.Background(), req)
	})

	require.NoError(t, doErr, "an exhausted status ceiling is not an error: it is a response")
	require.NotNil(t, resp)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&count), "the ceiling is the number of attempts, not retries")
	assert.Contains(t, stderr, "attempt 2 of 2, no attempts left")
}

// TestDoWithRetry_TransportExhaustionErrors is the other half: with no
// response to hand back, the ceiling exhausts into an error naming the
// attempts made.
func TestDoWithRetry_TransportExhaustionErrors(t *testing.T) {
	// A listener that is closed before the request: connect fails every time.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := NewClient(WithRetryConfig(RetryConfigFor(2)))
	req, err := http.NewRequest(http.MethodGet, url+"/gone", nil)
	require.NoError(t, err)

	var doErr error
	_ = captureStderr(t, func() {
		_, doErr = c.Do(context.Background(), req)
	})

	require.Error(t, doErr)
	assert.Contains(t, doErr.Error(), "request failed after 2 attempts")
}

// TestDoWithoutRetrier_SingleAttempt is the backward-compatible half: a client
// built without WithRetryConfig still performs exactly one exchange.
func TestDoWithoutRetrier_SingleAttempt(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/down", nil)
	require.NoError(t, err)

	resp, doErr := c.Do(context.Background(), req)
	require.NoError(t, doErr)
	require.NotNil(t, resp)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
}
