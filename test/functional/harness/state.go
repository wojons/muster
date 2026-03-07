// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"sync"
)

// State represents the system state.
type State struct {
	mu          sync.RWMutex
	initialized bool
	ready       bool
	error       error
	metrics     map[string]interface{}
}

// NewState creates a new state instance.
func NewState() *State {
	return &State{
		metrics: make(map[string]interface{}),
	}
}

// GetState returns the current system state.
func (s *State) GetState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.error != nil {
		return "error"
	}
	if s.ready {
		return "ready"
	}
	if s.initialized {
		return "initializing"
	}
	return "stopped"
}

// SetInitialized marks the system as initialized.
func (s *State) SetInitialized(initialized bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialized = initialized
}

// SetReady marks the system as ready.
func (s *State) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = ready
}

// SetError records an error state.
func (s *State) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.error = err
}

// GetError returns the last error, if any.
func (s *State) GetError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.error
}

// SetMetric records a metric value.
func (s *State) SetMetric(name string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics[name] = value
}

// GetMetric retrieves a metric value.
func (s *State) GetMetric(name string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.metrics[name]
	return val, ok
}
