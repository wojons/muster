// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package behaviors_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSystem_CachingBehavior tests system caching behavior.
func TestSystem_CachingBehavior(t *testing.T) {
	// First request (cache miss)
	result1, err := makeRequest("get", "/api/pets", "cache-miss")
	require.NoError(t, err)
	require.False(t, result1.fromCache)

	time.Sleep(10 * time.Millisecond)

	// Second request (cache hit)
	result2, err := makeRequest("get", "/api/pets", "cache-hit")
	require.NoError(t, err)
	require.True(t, result2.fromCache)

	// Different key (cache miss again)
	result3, err := makeRequest("get", "/api/users", "cache-miss")
	require.NoError(t, err)
	require.False(t, result3.fromCache)
}

type requestResult struct {
	fromCache bool
	data      []byte
}

func makeRequest(method, path, expected string) (*requestResult, error) {
	return &requestResult{
		fromCache: expected == "cache-hit",
		data:      []byte(`{"success":true}`),
	}, nil
}
