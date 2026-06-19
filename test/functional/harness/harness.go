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
