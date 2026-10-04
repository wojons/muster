// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PROTO-020: the proxy must forward the client-declared deadline to the
// outbound request (PROTOCOLS.md §4.3: every call honors its caller's ctx
// deadline; a layer may apply a default ONLY when the caller declared none).

// TestProxyHandler_CallerDeadlinePropagates proves the inbound request's
// context deadline reaches the proxy's OUTBOUND request unchanged — the hop
// neither drops it nor replaces it with the handler's default. (Context
// deadlines do not cross the HTTP wire, so the outbound transport is the
// layer that observes them; the wire itself only carries the consequences.)
func TestProxyHandler_CallerDeadlinePropagates(t *testing.T) {
	type observed struct {
		has bool
		dl  time.Time
	}
	seen := make(chan observed, 1)
	base := http.DefaultTransport
	capture := &captureRT{
		inner: base,
		observe: func(req *http.Request) {
			dl, ok := req.Context().Deadline()
			seen <- observed{has: ok, dl: dl}
		},
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	handler := NewProxyHandler(backend.URL)
	handler.httpClient = &http.Client{Transport: capture}

	inboundCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wantDeadline, hasDeadline := inboundCtx.Deadline()
	require.True(t, hasDeadline)

	req := httptest.NewRequest(http.MethodGet, "/slow", nil).WithContext(inboundCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "proxied request should complete within the caller deadline")

	select {
	case got := <-seen:
		require.True(t, got.has, "outbound request carried no deadline: the proxy dropped the caller's context")
		assert.WithinDuration(t, wantDeadline, got.dl, time.Second,
			"outbound layer must observe the caller's deadline, not a handler default")
	default:
		t.Fatal("proxy never issued an outbound request")
	}
}

// captureRT observes each outbound request before forwarding it.
type captureRT struct {
	inner   http.RoundTripper
	observe func(*http.Request)
}

func (c *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	c.observe(req)
	return c.inner.RoundTrip(req)
}

// TestProxyHandler_CallerDeadlineNotClampedByDefault proves a caller-set
// deadline is never overridden by the handler's private default: a backend
// slower than the configured default still completes when the caller's
// deadline allows it.
func TestProxyHandler_CallerDeadlineNotClampedByDefault(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// A handler whose private default (10ms) is far shorter than the
	// caller's deadline (2s): the caller deadline must win.
	handler := &ProxyHandler{
		backendURL:   backend.URL,
		forwardedFor: true,
		httpClient:   &http.Client{Timeout: 10 * time.Millisecond},
	}

	inboundCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil).WithContext(inboundCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"caller deadline (2s) must override the private default (10ms): got %s", rec.Body.String())
}

// TestProxyHandler_NoCallerDeadlineAppliesDefault proves the default still
// governs when the caller declared no deadline — the default is a fallback,
// not dead configuration.
func TestProxyHandler_NoCallerDeadlineAppliesDefault(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	handler := &ProxyHandler{
		backendURL:   backend.URL,
		forwardedFor: true,
		httpClient:   &http.Client{Timeout: 10 * time.Millisecond},
	}

	// No deadline on the inbound context: the 10ms default applies and the
	// request fails rather than hanging.
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code,
		"without a caller deadline the private default must still bound the request")
}

// TestProxyHandler_CallerCancellationPropagates proves cancellation (not
// only deadlines) rides the forwarded context.
func TestProxyHandler_CallerCancellationPropagates(t *testing.T) {
	sawCancel := make(chan struct{}, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		sawCancel <- struct{}{}
	}))
	defer backend.Close()

	handler := NewProxyHandler(backend.URL)

	inboundCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/hang", nil).WithContext(inboundCtx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-sawCancel:
		// The backend observed the propagated cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("backend never observed the caller's cancellation: the proxy dropped the caller's context")
	}
	<-done
}
