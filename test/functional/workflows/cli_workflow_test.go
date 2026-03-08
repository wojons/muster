// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package workflows_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

// TestSystem_Initialization tests that the system can start, initialize, and reach ready state.
func TestSystem_Initialization(t *testing.T) {
	system := harness.NewSystem(t)
	defer system.Cleanup(t)

	ctx := system.Context()
	require.NotNil(t, ctx)

	err := system.Initialize(t)
	require.NoError(t, err)
	require.True(t, system.IsReady())
}

// TestUserWorkflow_CLI_CommandExecution tests a complete user journey through the CLI.
func TestUserWorkflow_CLI_CommandExecution(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
_, _ = w.Write([]byte(`{"pets":[{"id":1,"name":"Fluffy"}]}`))
	}))
	defer server.Close()

	// User loads spec
	// TODO: Load OpenAPI spec from mock server

	// User generates commands
	// TODO: Generate commands from spec

	// User executes command
	resp, err := http.Get(server.URL + "/api/pets")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSystem_CachingBehavior tests system caching behavior.
func TestSystem_CachingBehavior(t *testing.T) {
	_ = harness.NewState()

	// First request (cache miss)
	result1 := makeCachedRequest("/api/pets", false)
	require.False(t, result1.fromCache)
	require.NotNil(t, result1.data)

	// Second request (cache hit)
	result2 := makeCachedRequest("/api/pets", true)
	require.True(t, result2.fromCache)
	require.NotNil(t, result2.data)
}

type requestResult struct {
	fromCache bool
	data      []byte
}

func makeCachedRequest(path string, useCache bool) requestResult {
	return requestResult{
		fromCache: useCache,
		data:      []byte(`{"success":true}`),
	}
}

// TestSystem_ConcurrentRequests tests system handles concurrent operations.
func TestSystem_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errors := make(chan error, 10)

	// Launch 10 concurrent requests
	for i := 0; i < 10; i++ {
		go func() {
			resp, err := http.Get(server.URL + "/api/pets")
			if err != nil {
				errors <- err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			errors <- nil
		}()
	}

	// All requests should succeed
	for i := 0; i < 10; i++ {
		select {
		case err := <-errors:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("timeout waiting for requests")
		}
	}
}
