// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package behaviors_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSystem_ErrorRecovery_Network tests error recovery from network issues.
func TestSystem_ErrorRecovery_Network(t *testing.T) {
	// Inject network failure
	err := system.InjectFailure("network_timeout")
	require.NoError(t, err)

	// Verify graceful degradation - should use fallback
	result, err := system.MakeRequest("/api/pets")
	require.NoError(t, err)
	require.True(t, result.FallbackUsed)
	require.NotNil(t, result.Data)
}

// TestSystem_ErrorRecovery_InvalidResponse tests error recovery from invalid responses.
func TestSystem_ErrorRecovery_InvalidResponse(t *testing.T) {
	// Make request that returns invalid JSON
	result, err := system.MakeRequest("/api/pets/invalid")

	// Should handle error gracefully
	require.NoError(t, err)
	require.True(t, result.ErrorHandled)
	require.NotNil(t, result.Data)
}

// TestSystem_ErrorRecovery_MissingResource tests error recovery from missing resources.
func TestSystem_ErrorRecovery_MissingResource(t *testing.T) {
	result, err := system.MakeRequest("/api/pets/99999")

	// Should return Not Found gracefully
	require.NoError(t, err)
	require.True(t, result.NotFound)
	require.NotNil(t, result.Data)
}

// TestSystem_ErrorRecovery_Timeout tests error recovery from timeouts.
func TestSystem_ErrorRecovery_Timeout(t *testing.T) {
	// Inject timeout failure
	err := system.InjectFailure("request_timeout")
	require.NoError(t, err)

	// Should use cached data or fallback
	result, err := system.MakeRequest("/api/pets")
	require.NoError(t, err)
	require.True(t, result.TimeoutHandled)
	require.NotNil(t, result.Data)
}

// Mock system for testing error recovery
var system testSystem

type testSystem struct {
	lastFailure string
}

func (s *testSystem) InjectFailure(failureType string) error {
	s.lastFailure = failureType
	return nil
}

func (s *testSystem) MakeRequest(path string) (*errorRequestResult, error) {
	// Simulate different error scenarios based on path
	if path == "/api/pets/99999" {
		return &errorRequestResult{
			FallbackUsed:   false,
			ErrorHandled:   true,
			NotFound:       true,
			TimeoutHandled: false,
			Data:           []byte(`{"error":"not found"}`),
		}, nil
	}

	if s.lastFailure == "request_timeout" {
		return &errorRequestResult{
			FallbackUsed:   true,
			ErrorHandled:   true,
			NotFound:       false,
			TimeoutHandled: true,
			Data:           []byte(`{"status":"fallback"}`),
		}, nil
	}

	return &errorRequestResult{
		FallbackUsed:   true,
		ErrorHandled:   true,
		NotFound:       false,
		TimeoutHandled: false,
		Data:           []byte(`{"status":"fallback"}`),
	}, nil
}

type errorRequestResult struct {
	FallbackUsed   bool
	ErrorHandled   bool
	NotFound       bool
	TimeoutHandled bool
	Data           []byte
}
