// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"context"
	"testing"
)

// System represents the complete system under test.
type System struct {
	ctx          context.Context
	cleanupFuncs []func()
}

// NewSystem creates a new system instance for testing.
func NewSystem(t *testing.T) *System {
	t.Helper()

	ctx := context.Background()

	return &System{
		ctx:          ctx,
		cleanupFuncs: make([]func(), 0),
	}
}

// Initialize initializes the system.
func (s *System) Initialize(t *testing.T) error {
	t.Helper()

	// TODO: Initialize actual system components
	// - Config loader
	// - Logger
	// - Cache
	// - MCP server
	// - WASM runtime

	return nil
}

// Cleanup calls all registered cleanup functions.
func (s *System) Cleanup(t *testing.T) {
	t.Helper()

	// Call cleanup functions in reverse order (LIFO)
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		s.cleanupFuncs[i]()
	}
}

// RegisterCleanup adds a cleanup function to be called during Cleanup.
func (s *System) RegisterCleanup(fn func()) {
	s.cleanupFuncs = append(s.cleanupFuncs, fn)
}

// Context returns the system context.
func (s *System) Context() context.Context {
	return s.ctx
}

// IsReady returns true if the system is ready for use.
func (s *System) IsReady() bool {
	// TODO: Check actual system state
	return true
}
