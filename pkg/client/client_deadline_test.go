// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PROTO-020: a caller's context deadline is the authority (PROTOCOLS.md
// §4.3). It must reach the outbound layer intact, and the client's private
// default timeout must apply ONLY when the caller declared no deadline.

// capturingTransport records the deadline observed at the outbound layer.
type capturingTransport struct {
	inner       http.RoundTripper
	observed    chan time.Time
	sawDeadline chan bool
}

func (c *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	dl, ok := req.Context().Deadline()
	if c.sawDeadline != nil {
		c.sawDeadline <- ok
	}
	if ok && c.observed != nil {
		c.observed <- dl
	}
	return c.inner.RoundTrip(req)
}

// TestClient_CallerDeadlineReachesOutboundLayer proves the context deadline
// survives to the transport that performs the exchange.
func TestClient_CallerDeadlineReachesOutboundLayer(t *testing.T) {
	sawDeadline := make(chan bool, 1)
	observed := make(chan time.Time, 1)
	base := DefaultTransport()
	capture := &capturingTransport{
		inner:       base,
		observed:    observed,
		sawDeadline: sawDeadline,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(WithTransport(capture))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wantDeadline, hasDeadline := ctx.Deadline()
	require.True(t, hasDeadline)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	require.NoError(t, err)

	resp, err := c.Do(ctx, req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case ok := <-sawDeadline:
		require.True(t, ok, "outbound layer saw no deadline on the request context")
		gotDeadline := <-observed
		assert.WithinDuration(t, wantDeadline, gotDeadline, time.Second,
			"outbound layer must observe the caller's deadline")
	case <-time.After(2 * time.Second):
		t.Fatal("outbound transport was never consulted")
	}
}

// TestClient_CallerDeadlineNotClampedByDefault proves the client's private
// default timeout does not override a caller-set deadline: the request
// completes when it finishes within the caller's deadline, even though the
// configured default is far shorter.
func TestClient_CallerDeadlineNotClampedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(WithTimeout(20 * time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/slow", nil)
	require.NoError(t, err)

	resp, err := c.Do(ctx, req)
	require.NoError(t, err, "caller deadline (2s) must override the private default (20ms)")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestClient_NoCallerDeadlineUsesDefaultTimeout proves the default still
// bounds a deadline-less caller — the fallback is not removed, it is
// demoted to what the contract names it: a default for absent deadlines.
func TestClient_NoCallerDeadlineUsesDefaultTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(WithTimeout(20 * time.Millisecond))

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/slow", nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.Error(t, err, "without a caller deadline the client default must bound the request")
	if resp != nil {
		_ = resp.Body.Close()
	}
}
