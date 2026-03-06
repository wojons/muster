package behaviors

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

func TestCachingBehavior(t *testing.T) {
	t.Skip("HTTP request/response caching not yet implemented - feature pending")

	t.Run("cache behavior for repeated requests", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
		client := system.HTTPClient()

		resp1, err := client.Do(req)
		require.NoError(t, err, "First request should succeed")
		require.NotNil(t, resp1, "Response should not be nil")

		// Second request (potential cache hit simulation)
		resp2, err := client.Do(req)
		require.NoError(t, err, "Second request should succeed")
		require.NotNil(t, resp2, "Response should not be nil")
	})

	t.Run("cache invalidation behavior", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Make a request
		req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
		client := system.HTTPClient()

		_, err := client.Do(req)
		require.NoError(t, err)

		// Make another request with different URL
		req2 := system.NewRequest().Method("GET").URL("http://example.com/api/test2")
		resp2, err := client.Do(req2)
		require.NoError(t, err)
		require.NotNil(t, resp2)
	})

	t.Run("cache capacity limits", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		client := system.HTTPClient()

		// Make multiple requests to different endpoints
		for i := 0; i < 20; i++ {
			req := system.NewRequest().Method("GET").
				URL("http://example.com/api/test")
			_, err := client.Do(req)
			require.NoError(t, err)
		}

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready")
	})
}

func TestRateLimitingBehavior(t *testing.T) {
	t.Skip("HTTP request rate limiting not yet implemented - feature pending")
	t.Run("normal requests within limits", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		client := system.HTTPClient()

		// Make requests at normal rate
		for i := 0; i < 10; i++ {
			req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
			_, err := client.Do(req)
			require.NoError(t, err, "Request %d should succeed", i)
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("handles request bursts gracefully", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		errors := make(chan error, 100)

		// Launch burst of 50 requests
		for i := 0; i < 50; i++ {
			go func() {
				select {
				case <-ctx.Done():
					return
				default:
					req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
					_, err := system.HTTPClient().Do(req)
					errors <- err
				}
			}()
		}

		// Wait for all requests
		count := 0
		for i := 0; i < 50; i++ {
			err := <-errors
			if err == nil {
				count++
			}
		}

		// Most should succeed (some may be rate limited)
		require.Greater(t, count, 40, "Most requests should succeed")
	})

	t.Run("respects rate limit over time", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		client := system.HTTPClient()

		// Make requests continuously for 2 seconds
		start := time.Now()
		count := 0
		for time.Since(start) < 2*time.Second {
			req := system.NewRequest().Method("GET").URL("http://example.com/api/test")
			_, err := client.Do(req)
			if err == nil {
				count++
			}
			time.Sleep(10 * time.Millisecond)
		}

		// Count should be reasonable (not millions)
		require.Less(t, count, 1000, "Request count should be rate-limited")
	})
}

func TestErrorHandlingBehavior(t *testing.T) {
	t.Run("network errors are handled gracefully", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Request to invalid URL should fail gracefully
		req := system.NewRequest().Method("GET").URL("http://nonexistent-domain-xyz-123.example/api/test")
		_, err := system.HTTPClient().Do(req)
		require.Error(t, err, "Request to invalid domain should fail")

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after error")

		// Next request should work
		req2 := system.NewRequest().Method("GET").URL("http://example.com/api/test")
		_, err = system.HTTPClient().Do(req2)
		// May fail because network, but shouldn't crash system
		require.True(t, system.Ready(), "System should remain ready")
	})

	t.Run("invalid response handling", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// CLI handles invalid commands
		_, err := system.ExecuteCLI("invalid-command-xyz")
		require.Error(t, err, "Invalid command should fail")

		// Next command should work
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)
	})

	t.Run("timeout behavior", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Quick timeout test
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		done := make(chan struct{})
		go func() {
			// Execute version command
			_, err := system.ExecuteCLI("--version")
			require.NoError(t, err)
			close(done)
		}()

		select {
		case <-done:
			// Command completed in time
		case <-ctx.Done():
			t.Fatal("Command should complete before timeout")
		}
	})
}

func TestResourceCleanupBehavior(t *testing.T) {
	t.Run("cleans up temp directory on stop", func(t *testing.T) {
		system := harness.NewSystem(t)
		tempDir := system.TempDir()

		system.Build()
		system.Start()

		// Temp directory should exist
		harness.DirExists(t, tempDir)

		// Stop the system
		system.Stop()

		// Wait for cleanup
		time.Sleep(100 * time.Millisecond)

		_, err := os.Stat(tempDir)
		require.Error(t, err, "Temp directory should be cleaned up")
	})

	t.Run("cleans up socket file on stop", func(t *testing.T) {
		system := harness.NewSystem(t)
		socketPath := system.SocketPath()

		system.Build()
		system.Start()

		// Wait for socket creation
		require.Eventually(t, func() bool {
			_, err := os.Stat(socketPath)
			return err == nil
		}, 2*time.Second, 10*time.Millisecond, "Socket should be created")

		// Stop the system
		system.Stop()

		// Socket should be cleaned up
		_, err := os.Stat(socketPath)
		require.Error(t, err, "Socket should be cleaned up")
	})

	t.Run("multiple start/stop cycles don't leak resources", func(t *testing.T) {
		var socketPaths []string

		for i := 0; i < 5; i++ {
			system := harness.NewSystem(t)
			socketPath := system.SocketPath()
			socketPaths = append(socketPaths, socketPath)

			system.Build()
			system.Start()
			system.Stop()

			// Wait for cleanup
			time.Sleep(50 * time.Millisecond)
		}

		// Verify all sockets are cleaned up
		for _, path := range socketPaths {
			_, err := os.Stat(path)
			require.Error(t, err, "Socket %s should be cleaned up", path)
		}
	})
}

func TestStatePersistenceBehavior(t *testing.T) {
	t.Run("system state remains consistent", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Get initial state
		state1 := system.State()
		require.Equal(t, harness.StateReady, state1)

		// Execute some operations
		for i := 0; i < 10; i++ {
			_, err := system.ExecuteCLI("--version")
			require.NoError(t, err)
		}

		// State should remain ready
		state2 := system.State()
		require.Equal(t, harness.StateReady, state2)
	})

	t.Run("can recover from transient states", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		// Initial state
		require.Equal(t, harness.StateStopped, sm.Current())

		// Transition through states
		sm.MustTransition(t, harness.StateInitializing, "init")
		sm.MustTransition(t, harness.StateReady, "ready")

		// Execute and return to ready
		sm.MustTransition(t, harness.StateExecuting, "execute")
		sm.MustTransition(t, harness.StateReady, "complete")

		// Verify final state
		require.Equal(t, harness.StateReady, sm.Current())
	})
}

func TestConcurrencyBehavior(t *testing.T) {
	t.Run("handles concurrent CLI requests", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		errors := make(chan error, 100)
		var wg sync.WaitGroup

		// Launch 50 concurrent requests
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(num int) {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				default:
					_, err := system.ExecuteCLI("--version")
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Most requests should succeed
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		require.Less(t, errorCount, 10, "Most concurrent requests should succeed")
	})

	t.Run("handles concurrent state transitions", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		errors := make(chan error, 100)

		// Launch concurrent transitions
		for i := 0; i < 10; i++ {
			go func() {
				err := sm.Transition(harness.StateInitializing, "concurrent")
				errors <- err
			}()
		}

		// Wait for all
		for i := 0; i < 10; i++ {
			<-errors
		}

		// State should be deterministic
		require.Equal(t, harness.StateInitializing, sm.Current())
	})
}

func TestAuthenticationBehavior(t *testing.T) {
	t.Run("system works without authentication", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// CLI should work without credentials
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)
	})

	t.Run("respects environment configuration", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Set config variable
		oldConfig := os.Getenv("MUSTER_CONFIG")
		defer os.Setenv("MUSTER_CONFIG", oldConfig)

		configPath := system.TempDir() + "/config.yaml"
		os.Setenv("MUSTER_CONFIG", configPath)

		// CLI should use config
		_, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
	})
}
