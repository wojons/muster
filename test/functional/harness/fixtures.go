// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockServer represents a mock HTTP server for testing.
type MockServer struct {
	*httptest.Server
	t *testing.T
}

// NewMockServer creates a new mock HTTP server.
func NewMockServer(t *testing.T, handler http.HandlerFunc) *MockServer {
	t.Helper()

	server := httptest.NewServer(handler)

	t.Cleanup(func() {
		server.Close()
	})

	return &MockServer{
		Server: server,
		t:      t,
	}
}

// HTTPClient returns an HTTP client configured for the mock server.
func (m *MockServer) HTTPClient() *http.Client {
	return &http.Client{}
}

// NewTestContext creates a test context with timeout.
func NewTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(func() {
		cancel()
	})

	return ctx
}

// WithMetric simulates metric recording for testing.
func WithMetric(state *State, name string, value interface{}) {
	state.SetMetric(name, value)
}
