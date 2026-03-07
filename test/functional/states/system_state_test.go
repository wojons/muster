// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package states_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

// TestSystem_StateTransitions tests system state transitions.
func TestSystem_StateTransitions(t *testing.T) {
	state := harness.NewState()

	// System starts in "stopped" state
	assert.Equal(t, "stopped", state.GetState())

	// Initialize transitions to "initializing"
	state.SetInitialized(true)
	assert.Equal(t, "initializing", state.GetState())

	// After init, transitions to "ready"
	state.SetReady(true)
	assert.Equal(t, "ready", state.GetState())

	// Error transitions to "error" state
	state.SetError(assert.AnError)
	assert.Equal(t, "error", state.GetState())

	// Clear error returns to "ready"
	state.SetError(nil)
	state.SetReady(true)
	assert.Equal(t, "ready", state.GetState())
}

// TestSystem_CacheState tests cache state transitions.
func TestSystem_CacheState(t *testing.T) {
	state := harness.NewState()

	// Initial state: no cache
	cacheState := "empty"
	assert.Equal(t, "empty", cacheState)

	// First request: cache miss
	state.SetMetric("cache_hits", 0)
	cacheState = "miss"
	assert.Equal(t, "miss", cacheState)

	// Subsequent request: cache hit
	state.SetMetric("cache_hits", 1)
	cacheState = "hit"
	assert.Equal(t, "hit", cacheState)
}

// TestSystem_AuthState tests authentication state transitions.
func TestSystem_AuthState(t *testing.T) {
	state := harness.NewState()

	// Initial state: not authenticated
	authState := "unauthenticated"
	assert.Equal(t, "unauthenticated", authState)

	// After OAuth2 flow: authenticated
	state.SetMetric("auth_token", "token123")
	authState = "authenticated"
	assert.Equal(t, "authenticated", authState)

	// Token expiry: needs refresh
	state.SetError(assert.AnError)
	authState = "needs_refresh"
	assert.Equal(t, "needs_refresh", authState)
}
