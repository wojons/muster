package harness

import (
	"context"
	"testing"
	"time"

	"github.com/wojons/muster/pkg/config"
	"github.com/stretchr/testify/require"
)

// TestHarness provides system-wide test isolation and state management
type TestHarness struct {
	Context   context.Context
	Cancel    context.CancelFunc
	Config    *config.Config
	StartTime time.Time
	Resources []interface{}
}

// New creates a new test harness with isolated context and resources
func New(t *testing.T) *TestHarness {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	cfg := &config.Config{
		Version: "1.0.0",
		Cache: config.CacheConfig{
			Enabled: true,
			Type:    "memory",
			TTL:     5 * time.Minute,
		},
	}

	return &TestHarness{
		Context:   ctx,
		Cancel:    cancel,
		Config:    cfg,
		StartTime: time.Now(),
		Resources: make([]interface{}, 0),
	}
}

// Cleanup releases all resources held by the harness
func (h *TestHarness) Cleanup(t *testing.T) {
	for _, resource := range h.Resources {
		if cleanup, ok := resource.(interface{ Cleanup() }); ok {
			cleanup.Cleanup()
		}
	}
	h.Cancel()
}

// State provides a simple test state tracker for functional tests.
type State struct {
	state       string
	initialized bool
	ready       bool
	err         error
	metrics     map[string]interface{}
}

// NewState creates a new State starting in "stopped" state.
func NewState() *State {
	return &State{
		state:   "stopped",
		metrics: make(map[string]interface{}),
	}
}

// GetState returns the current state string.
func (s *State) GetState() string {
	return s.state
}

// SetInitialized marks the system as initialized.
func (s *State) SetInitialized(v bool) {
	s.initialized = v
	if v {
		s.state = "initializing"
	}
}

// SetReady marks the system as ready.
func (s *State) SetReady(v bool) {
	s.ready = v
	if v {
		s.state = "ready"
	}
}

// SetError sets an error on the system.
func (s *State) SetError(err error) {
	s.err = err
	if err != nil {
		s.state = "error"
	}
}

// SetMetric sets a metric value.
func (s *State) SetMetric(key string, value interface{}) {
	if s.metrics == nil {
		s.metrics = make(map[string]interface{})
	}
	s.metrics[key] = value
}

// System provides a simple test system wrapper for functional tests.
type System struct {
	State
	t      *testing.T
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSystem creates a new System for testing.
func NewSystem(t *testing.T) *System {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	return &System{
		State:  State{state: "stopped"},
		t:      t,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Cleanup cleans up the system resources.
func (s *System) Cleanup(t *testing.T) {
	s.cancel()
}

// Context returns the system context.
func (s *System) Context() context.Context {
	return s.ctx
}

// Initialize initializes the system and transitions to ready.
func (s *System) Initialize(t *testing.T) error {
	s.SetInitialized(true)
	s.SetReady(true)
	return nil
}

// IsReady returns whether the system is ready.
func (s *System) IsReady() bool {
	return s.ready
}

// AssertReady asserts that the system is ready for operations
func (h *TestHarness) AssertReady(t *testing.T) {
	require.NotNil(t, h.Context)
	require.NotNil(t, h.Config)
	require.False(t, h.Context.Err() != nil)
}

// AssertExecutionTime verifies operations complete within expected time
func (h *TestHarness) AssertExecutionTime(t *testing.T, maxDuration time.Duration, checkpoint time.Time) {
	elapsed := time.Since(checkpoint)
	require.LessOrEqual(t, elapsed, maxDuration,
		"operation took %v, expected <= %v", elapsed, maxDuration)
}
