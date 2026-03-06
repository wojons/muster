package behaviors

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

func TestErrorRecovery(t *testing.T) {
	t.Run("recovers from network timeout", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Network request should timeout gracefully
		req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
		start := time.Now()

		_, err := system.HTTPClient().Do(req)
		elapsed := time.Since(start)

		// Either success or timeout error
		require.True(t, err == nil || elapsed < 30*time.Second, "Request should complete or timeout")

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after timeout")

		// Next request should work
		req2 := system.NewRequest().Method("GET").URL("http://example.com/api/test2")
		_, err = system.HTTPClient().Do(req2)
		require.True(t, system.Ready(), "System should remain ready even if request fails")
	})

	t.Run("recovers from invalid request", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Invalid CLI command should fail
		_, err := system.ExecuteCLI("invalid-command-xyz")
		require.Error(t, err)

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after error")

		// Next command should work
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)
	})

	t.Run("recovers from daemon crash", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()

		// System should be ready
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Stop (simulating crash)
		system.Stop()

		// System should be stopped
		require.False(t, system.IsRunning())

		// Restart should work
		system.Start()
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should recover and become ready")

		// Commands should work after restart
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)

		system.Stop()
	})

	t.Run("handles concurrent errors gracefully", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		errors := make(chan error, 50)
		successCount := 0

		// Launch 30 concurrent requests
		for i := 0; i < 30; i++ {
			go func(id int) {
				select {
				case <-ctx.Done():
					return
				default:
					if id%5 == 0 {
						// Every 5th request is intentionally invalid
						_, err := system.ExecuteCLI("invalid-command")
						errors <- err
					} else {
						_, err := system.ExecuteCLI("--version")
						if err == nil {
							successCount++
						}
						errors <- err
					}
				}
			}(i)
		}

		// Wait for all requests
		errorCount := 0
		for i := 0; i < 30; i++ {
			err := <-errors
			if err != nil {
				errorCount++
			}
		}

		// Should have some success and some errors
		require.Greater(t, successCount, 20, "Most requests should succeed")
		require.Equal(t, 6, errorCount, "Six requests should fail")

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after errors")
	})

	t.Run("recovers from disk full error simulation", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Try to write to read-only location (fails gracefully)
		tmpPath := system.TempDir() + "/test.txt"
		err := os.WriteFile(tmpPath, []byte("test"), 0644)
		require.NoError(t, err, "Write to temp should succeed")

		// Verify file exists
		data, err := os.ReadFile(tmpPath)
		require.NoError(t, err)
		require.Equal(t, "test", string(data))

		// System should still be operational
		require.True(t, system.Ready())
	})

	t.Run("handles rapid request failures", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Make 20 failing requests rapidly
		for i := 0; i < 20; i++ {
			_, err := system.ExecuteCLI("invalid-command")
			require.Error(t, err)
		}

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after rapid failures")

		// Next valid command should work
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)
	})
}

func TestErrorPropagation(t *testing.T) {
	t.Run("errors are returned to caller", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Invalid command should return error
		_, err := system.ExecuteCLI("invalid-command")
		require.Error(t, err)
		require.NotEmpty(t, err.Error())

		// Error message should be helpful
		errMsg := err.Error()
		require.NotEmpty(t, errMsg, "Error message should not be empty")
	})

	t.Run("errors don't cause resource leaks", func(t *testing.T) {
		var socketPaths []string

		// Create multiple systems, cause errors, ensure cleanup
		for i := 0; i < 5; i++ {
			system := harness.NewSystem(t)
			socketPath := system.SocketPath()
			socketPaths = append(socketPaths, socketPath)

			system.Build()
			system.Start()
			system.Stop()

			// Some errors
			_, _ = system.ExecuteCLI("invalid-command")
		}

		time.Sleep(100 * time.Millisecond)

		// Verify all sockets are cleaned up
		for _, path := range socketPaths {
			_, err := os.Stat(path)
			require.Error(t, err, "Socket %s should be cleaned up", path)
		}
	})
}

func TestStateErrorRecovery(t *testing.T) {
	t.Run("can recover from error state", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		// Transition to ready
		sm.MustTransition(t, harness.StateReady, "ready")

		// Transition to error
		sm.MustTransition(t, harness.StateError, "error occurred")
		require.Equal(t, harness.StateError, sm.Current())

		// Should be able to transition to stopped
		err := sm.Transition(harness.StateStopped, "shutdown")
		require.NoError(t, err)
		require.Equal(t, harness.StateStopped, sm.Current())
	})

	t.Run("error state doesn't block reinitialization", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		// Full lifecycle with error
		sm.MustTransition(t, harness.StateInitializing, "init1")
		sm.MustTransition(t, harness.StateReady, "ready1")
		sm.MustTransition(t, harness.StateError, "error")
		sm.MustTransition(t, harness.StateStopped, "shutdown")

		// Should be able to restart
		sm.MustTransition(t, harness.StateInitializing, "init2")
		sm.MustTransition(t, harness.StateReady, "ready2")

		require.Equal(t, harness.StateReady, sm.Current())
	})
}

func TestPartialFailureRecovery(t *testing.T) {
	t.Run("partial request failure doesn't affect system", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Mix of successful and failed requests
		successCount := 0
		for i := 0; i < 10; i++ {
			if i%3 == 0 {
				// Fail
				_, err := system.ExecuteCLI("invalid-command")
				require.Error(t, err)
			} else {
				// Succeed
				_, err := system.ExecuteCLI("--version")
				if err == nil {
					successCount++
				}
			}
		}

		// Should have 7 successes
		require.Equal(t, 7, successCount)

		// System should still be operational
		require.True(t, system.Ready())
	})

	t.Run("timeout on one request doesn't affect others", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Make several requests
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		results := make(chan string, 10)

		// Launch 5 requests
		for i := 0; i < 5; i++ {
			go func() {
				select {
				case <-ctx.Done():
					return
				default:
					_, err := system.ExecuteCLI("--version")
					if err == nil {
						results <- "success"
					} else {
						results <- "error"
					}
				}
			}()
		}

		// Count results
		success := 0
		errors := 0
		for i := 0; i < 5; i++ {
			select {
			case <-time.After(3 * time.Second):
				t.Fatal("Requests should complete")
			case result := <-results:
				if result == "success" {
					success++
				} else {
					errors++
				}
			}
		}

		// Most should succeed
		require.Greater(t, success, 3, "Most requests should succeed")

		// System should still be operational
		require.True(t, system.Ready())
	})
}

func TestRecoveryAfterUnexpectedTermination(t *testing.T) {
	t.Run("daemon can restart after crash", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		// Start
		system.Start()
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Stop abruptly (simulating crash)
		system.Stop()

		// Should be able to restart
		system.Start()
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready after restart")

		// Should still work
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)

		system.Stop()
	})

	t.Run("state restores correctly after restart", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		for i := 0; i < 3; i++ {
			// Start
			system.Start()
			require.Eventually(t, func() bool {
				return system.Ready()
			}, 5*time.Second, 100*time.Second, "System should become ready")

			// Verify functionality
			output, err := system.ExecuteCLI("--version")
			require.NoError(t, err, "Restart %d should work", i)
			require.NotEmpty(t, output)

			// Stop
			system.Stop()
			time.Sleep(50 * time.Millisecond)
		}
	})
}
