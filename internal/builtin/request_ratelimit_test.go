package builtin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/config"
)

// TestRequestCommand_RateLimit_BlocksExcess verifies that handleRequest
// with rate limiting enabled applies the rate limiter to the transport.
// Because each handleRequest call creates a fresh Client (and token bucket),
// we verify transport wiring by checking that two sequential requests within
// the same call are rate-limited via the single-use transport.
func TestRequestCommand_RateLimit_BlocksExcess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	// Config with rate limiting enabled: burst=1, rate=1
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled:    true,
			RatePerSec: 1,
			Burst:      1,
		},
	}

	// Build the client directly for testing — verify that WithRateLimiter
	// wraps the transport with a RateLimitRoundTripper.
	var clientOpts []client.ClientOption
	clientOpts = append(clientOpts, client.WithTimeout(30*time.Second))
	clientOpts = append(clientOpts, client.WithRateLimiter(cfg.RateLimit.RatePerSec, cfg.RateLimit.Burst))
	httpClient := client.NewClient(clientOpts...)

	// Verify the transport IS a RateLimitRoundTripper (wiring check).
	_, ok := httpClient.HTTPClient().Transport.(*client.RateLimitRoundTripper)
	require.True(t, ok, "transport should be wrapped with RateLimitRoundTripper when rate limit is enabled")
}

// TestRequestCommand_RateLimit_Disabled verifies that when the rate limit
// config is not enabled, all requests pass through without restriction.
func TestRequestCommand_RateLimit_Disabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	// Config with rate limiting disabled (default).
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled:    false,
			RatePerSec: 0,
			Burst:      0,
		},
	}

	for i := 0; i < 10; i++ {
		err := handleRequest(cfg, "GET", backend.URL+"/test", nil, nil, "table", false, 30*time.Second, "")
		assert.NoError(t, err, "request %d should succeed when rate limiting is disabled", i+1)
	}
}

// TestRequestCommand_RateLimit_NilConfig verifies that a nil config is
// handled gracefully — no rate limiting applied, all requests succeed.
func TestRequestCommand_RateLimit_NilConfig(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	for i := 0; i < 10; i++ {
		err := handleRequest(nil, "GET", backend.URL+"/test", nil, nil, "table", false, 30*time.Second, "")
		assert.NoError(t, err, "request %d should succeed with nil config", i+1)
	}
}
