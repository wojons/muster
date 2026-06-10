package daemon

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProxyHandler_RateLimit_BlocksExcess verifies that a ProxyHandler
// created with NewProxyHandlerWithRateLimit returns 429 when the burst
// is exhausted. Uses a tight limit (burst=1, rate=1) so two back-to-back
// requests produce a 429 on the second request.
func TestProxyHandler_RateLimit_BlocksExcess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	handler := NewProxyHandlerWithRateLimit(backend.URL, 1, 1)

	// First request: consumes the single token.
	req1 := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	assert.Equal(t, http.StatusOK, rec1.Code, "first request should succeed (token available)")
	body1, _ := io.ReadAll(rec1.Result().Body)
	assert.Equal(t, "ok", string(body1))
	rec1.Result().Body.Close()

	// Second request: should be rate-limited (no tokens left).
	req2 := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusTooManyRequests, rec2.Code, "second request should return 429")
	body2, _ := io.ReadAll(rec2.Result().Body)
	assert.Contains(t, string(body2), "rate limit exceeded")
	rec2.Result().Body.Close()
}

// TestProxyHandler_RateLimit_Disabled verifies that a ProxyHandler created
// with ratePerSec <= 0 does NOT rate-limit requests — existing behavior is
// preserved (disabled path).
func TestProxyHandler_RateLimit_Disabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	// ratePerSec=0 means disabled
	handler := NewProxyHandlerWithRateLimit(backend.URL, 0, 5)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed when rate limiting is disabled", i+1)
		rec.Result().Body.Close()
	}
}

// TestProxyHandler_RateLimit_ExistingConstructorStillWorks verifies that
// the original NewProxyHandler constructor is unchanged and does not apply
// any rate limiting.
func TestProxyHandler_RateLimit_ExistingConstructorStillWorks(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	handler := NewProxyHandler(backend.URL)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "existing constructor should not rate-limit")
		rec.Result().Body.Close()
	}
}
