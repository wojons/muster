package states

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

func TestSystemState_Initialization(t *testing.T) {
	t.Run("system starts in stopped state", func(t *testing.T) {
		system := harness.NewSystem(t)

		assertions := harness.NewSystemAssertions(t, system)
		assertions.State(harness.StateStopped)
	})

	t.Run("system initializes correctly", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// System should be ready
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Daemon should be running
		require.True(t, system.IsRunning(), "Daemon should be running")

		// State should be ready
		require.Equal(t, harness.StateReady, system.State())
	})

	t.Run("system transitions correctly", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		// Start in stopped state
		require.Equal(t, harness.StateStopped, sm.Current())

		// Transition to initializing
		err := sm.Transition(harness.StateInitializing, "system init")
		require.NoError(t, err)
		require.Equal(t, harness.StateInitializing, sm.Current())

		// Transition to ready
		err = sm.Transition(harness.StateReady, "system ready")
		require.NoError(t, err)
		require.Equal(t, harness.StateReady, sm.Current())

		// Can't transition to invalid state
		err = sm.Transition(harness.StateReady, "invalid transition")
		require.Error(t, err)
	})

	t.Run("system can be stopped and restarted", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		assertions := harness.NewSystemAssertions(t, system)

		// Start the system
		system.Start()
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should become ready")
		assertions.State(harness.StateReady)

		// Stop the system
		system.Stop()

		// Verify stopped state
		assertions.State(harness.StateStopped)
		assertions.NotStarted()

		// Can restart
		system.Start()
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should become ready after restart")
	})
}

func TestSystemState_Cleanup(t *testing.T) {
	t.Run("cleanup removes temp directory", func(t *testing.T) {
		system := harness.NewSystem(t)
		tempDir := system.TempDir()

		system.Build()
		system.Start()

		// Verify temp directory exists
		harness.DirExists(t, tempDir)

		system.Stop()

		// Verify temp directory removed
		_, err := os.Stat(tempDir)
		require.Error(t, err)
		require.Equal(t, os.ErrNotExist, err)
	})

	t.Run("cleanup removes socket file", func(t *testing.T) {
		system := harness.NewSystem(t)
		socketPath := system.SocketPath()

		system.Build()
		system.Start()

		// Wait for socket creation
		require.Eventually(t, func() bool {
			_, err := os.Stat(socketPath)
			return err == nil
		}, 2*time.Second, 10*time.Millisecond, "Socket should be created")

		harness.SocketExists(t, socketPath)

		system.Stop()

		// Verify socket removed
		_, err := os.Stat(socketPath)
		require.Error(t, err)
	})
}

func TestSystemState_StateTransitions(t *testing.T) {
	t.Run("valid state transitions", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		transitions := []struct {
			from  harness.State
			to    harness.State
			valid bool
		}{
			{harness.StateStopped, harness.StateInitializing, true},
			{harness.StateInitializing, harness.StateReady, true},
			{harness.StateReady, harness.StateReady, true},
			{harness.StateReady, harness.StateExecuting, true},
			{harness.StateReady, harness.StateStopping, true},
			{harness.StateExecuting, harness.StateReady, true},
			{harness.StateExecuting, harness.StateError, true},
			{harness.StateError, harness.StateStopped, true},
			{harness.StateInitializing, harness.StateError, true},
			{harness.StateReady, harness.StateStopped, false},          // Invalid
			{harness.StateExecuting, harness.StateInitializing, false}, // Invalid
		}

		for _, tt := range transitions {
			t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
				// Reset to starting state
				sm.Reset()

				// Navigate to starting state
				if tt.from != harness.StateStopped {
					if tt.from != harness.StateInitializing {
						err := sm.Transition(harness.StateInitializing, "setup")
						require.NoError(t, err)
					}
					if tt.from != harness.StateReady && tt.from != harness.StateInitializing {
						err := sm.Transition(harness.StateReady, "setup")
						require.NoError(t, err)
					}
					if tt.from == harness.StateReady && tt.to != harness.StateReady {
						if tt.from != harness.StateReady {
							// Already there
						}
					}
				}

				// Attempt transition
				err := sm.Transition(tt.to, "test")

				if tt.valid {
					require.NoError(t, err, "Valid transition should succeed")
					require.Equal(t, tt.to, sm.Current())
				} else {
					require.Error(t, err, "Invalid transition should fail")
					require.Equal(t, tt.from, sm.Current())
				}
			})
		}
	})

	t.Run("state transition history", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		// Make several transitions
		sm.MustTransition(t, harness.StateInitializing, "init")
		sm.MustTransition(t, harness.StateReady, "ready")
		sm.MustTransition(t, harness.StateExecuting, "execute")
		sm.MustTransition(t, harness.StateReady, "complete")

		// Verify history captured
		history := sm.History()
		require.Len(t, history, 4)

		// Verify transition sequence
		require.Equal(t, harness.StateStopped, history[0].From)
		require.Equal(t, harness.StateInitializing, history[1].From)
		require.Equal(t, harness.StateReady, history[2].From)
		require.Equal(t, harness.StateExecuting, history[3].From)
	})
}

func TestSystemState_WaitForState(t *testing.T) {
	t.Run("waits for state change", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		go func() {
			time.Sleep(100 * time.Millisecond)
			sm.Transition(harness.StateReady, "become ready")
		}()

		// Wait for ready state
		state := sm.WaitForState(ctx, harness.StateReady)
		require.Equal(t, harness.StateReady, <-state)
	})

	t.Run("times out if state never reached", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Wait for state that never comes
		_ = sm.WaitForState(ctx, harness.StateExecuting)

		// Context should be cancelled
		select {
		case <-ctx.Done():
			require.Equal(t, context.DeadlineExceeded, ctx.Err())
		default:
			t.Fatal("Should have timed out")
		}
	})
}

func TestSystemState_StateSnapshots(t *testing.T) {
	t.Run("captures state snapshot", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		sm.MustTransition(t, harness.StateInitializing, "init")
		sm.MustTransition(t, harness.StateReady, "ready")

		snap := sm.Capture()

		require.Equal(t, harness.StateReady, snap.State)
		require.False(t, snap.Timestamp.IsZero())
		require.NotNil(t, snap.Metadata)
		require.NotNil(t, snap.Metrics)
	})
}

func TestSystemState_StateCounting(t *testing.T) {
	t.Run("counts state visits", func(t *testing.T) {
		sc := harness.NewStateCounter()

		sc.Increment(harness.StateReady)
		sc.Increment(harness.StateReady)
		sc.Increment(harness.StateExecuting)

		require.Equal(t, 2, sc.Count(harness.StateReady))
		require.Equal(t, 1, sc.Count(harness.StateExecuting))
		require.Equal(t, 0, sc.Count(harness.StateStopped))
		require.Equal(t, 3, sc.Total())
	})

	t.Run("snapshot preserves counts", func(t *testing.T) {
		sc := harness.NewStateCounter()

		sc.Increment(harness.StateReady)
		sc.Increment(harness.StateReady)
		sc.Increment(harness.StateExecuting)

		snap := sc.Snapshot()

		require.Equal(t, 2, snap[harness.StateReady])
		require.Equal(t, 1, snap[harness.StateExecuting])
		require.Equal(t, 0, snap[harness.StateStopped])

		// Verify snapshot is independent
		sc.Reset()
		sc.Increment(harness.StateStopped)

		require.Equal(t, 3, snap[harness.StateReady]+snap[harness.StateExecuting])
	})
}

func TestSystemState_ConcurrentTransitions(t *testing.T) {
	t.Run("handles concurrent transitions safely", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		errors := make(chan error, 10)

		// Launch multiple goroutines trying to transition
		for i := 0; i < 5; i++ {
			go func() {
				err := sm.Transition(harness.StateInitializing, "concurrent")
				errors <- err
			}()
		}

		// Wait for all to complete
		for i := 0; i < 5; i++ {
			<-errors
		}

		// State should be deterministic
		require.Equal(t, harness.StateInitializing, sm.Current())
	})
}

func TestSystemState_ErrorHandling(t *testing.T) {
	t.Run("state persists after error", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		sm.MustTransition(t, harness.StateReady, "ready")

		// Try invalid transition
		err := sm.Transition(harness.StateStopped, "invalid")
		require.Error(t, err)

		// State should remain unchanged
		require.Equal(t, harness.StateReady, sm.Current())
	})

	t.Run("can transition from error state", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		sm.MustTransition(t, harness.StateInitializing, "init")
		sm.MustTransition(t, harness.StateError, "error occurred")

		require.Equal(t, harness.StateError, sm.Current())

		// Should be able to transition to stopped
		err := sm.Transition(harness.StateStopped, "shutdown")
		require.NoError(t, err)
		require.Equal(t, harness.StateStopped, sm.Current())
	})
}
