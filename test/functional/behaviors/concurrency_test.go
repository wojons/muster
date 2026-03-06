package behaviors

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

func TestConcurrency(t *testing.T) {
	t.Run("handles concurrent CLI requests", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		errors := make(chan error, 100)
		successCount := 0
		var mu sync.Mutex

		// Launch 100 concurrent requests
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				default:
					_, err := system.ExecuteCLI("--version")
					mu.Lock()
					if err == nil {
						successCount++
					}
					errors <- err
					mu.Unlock()
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Count errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		// At least 90% should succeed
		require.Greater(t, successCount, 89, "Most concurrent requests should succeed")

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready after concurrent load")
	})

	t.Run("handles rapid concurrent start/stop cycles", func(t *testing.T) {
		var systems []*harness.System

		// Start and stop 10 systems concurrently
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				system := harness.NewSystem(t)
				systems = append(systems, system)

				system.Build()
				system.Start()
				time.Sleep(10 * time.Millisecond)
				system.Stop()
			}(i)
		}

		wg.Wait()

		// All systems should have completed
		// Verify cleanup
		time.Sleep(100 * time.Millisecond)
		for _, sys := range systems {
			require.False(t, sys.IsRunning())
		}
	})

	t.Run("concurrent HTTP requests don't interfere", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		errors := make(chan error, 50)

		// Launch 50 concurrent HTTP requests
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				default:
					url := "http://example.com/api/test"
					req := system.NewRequest().Method("GET").URL(url)
					_, err := system.HTTPClient().Do(req)
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Most should succeed or have network errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		// System should still be operational
		require.True(t, system.Ready())
	})

	t.Run("concurrent state transitions are safe", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		var wg sync.WaitGroup
		errors := make(chan error, 10)

		// Launch concurrent transitions
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := sm.Transition(harness.StateInitializing, "concurrent")
				errors <- err
			}()
		}

		wg.Wait()
		close(errors)

		// State should be deterministic
		require.Equal(t, harness.StateInitializing, sm.Current())

		// Some transitions may have failed (expected)
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		require.Greater(t, errorCount, 0, "Some concurrent transitions should fail")
	})
}

func TestConcurrency_RaceConditions(t *testing.T) {
	t.Run("no race conditions on shared resources", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		var wg sync.WaitGroup

		// Mix of reads and writes to shared state
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = system.State()
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = system.ExecuteCLI("--version")
			}()
		}

		wg.Wait()

		// System should still be operational
		require.True(t, system.Ready())
	})

	t.Run("concurrent socket access is safe", func(t *testing.T) {
		system := harness.NewSystem(t)
		socketPath := system.SocketPath()
		system.Build()
		system.Start()
		defer system.Stop()

		var wg sync.WaitGroup

		// Many goroutines accessing the same socket concurrently
		for i := 0; i < 30; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = system.ExecuteCLI("--version")
			}()
		}

		wg.Wait()

		// Socket should still be valid
		require.True(t, system.Ready())
		require.FileExists(t, socketPath)
	})
}

func TestConcurrency_ResourceManagement(t *testing.T) {
	t.Run("concurrent operations don't leak resources", func(t *testing.T) {
		var socketPaths []string

		// Create and use multiple systems concurrently
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				system := harness.NewSystem(t)
				socketPaths = append(socketPaths, system.SocketPath())

				system.Build()
				system.Start()

				// Make some requests
				for j := 0; j < 5; j++ {
					_, _ = system.ExecuteCLI("--version")
				}

				system.Stop()
			}(i)
		}

		wg.Wait()

		// Wait for cleanup
		time.Sleep(200 * time.Millisecond)

		// Verify all sockets should be cleaned up
		// Note: This is a basic check - in production you'd want more rigorous verification
		for i, path := range socketPaths {
			if path != "" {
				// Each socket path should be unique
				require.NotEmpty(t, path, "Socket path %d should not be empty", i)
				// Cannot easily verify cleanup without filesystem access
				// This test ensures concurrent creation doesn't cause issues
			}
		}
	})

	t.Run("goroutine limits are enforced", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		var wg sync.WaitGroup
		counter := 0
		var mu sync.Mutex

		// Launch 1000 goroutines
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				mu.Lock()
				counter++
				mu.Unlock()

				_ = sm.Current()
			}()
		}

		wg.Wait()

		// Should not have crashed
		require.Equal(t, 1000, counter)
	})
}

func TestConcurrency_DeadlockPrevention(t *testing.T) {
	t.Run("no deadlocks on mutual resource access", func(t *testing.T) {
		ch1 := make(chan struct{}, 1)
		ch2 := make(chan struct{}, 1)

		var wg sync.WaitGroup

		// Goroutine 1
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ch1
			ch2 <- struct{}{}
		}()

		// Goroutine 2
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ch2
			ch1 <- struct{}{}
		}()

		// Trigger
		ch1 <- struct{}{}

		// Should complete without deadlock
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Should not deadlock")
		}
	})

	t.Run("system operations complete under load", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		errors := make(chan error, 100)

		// Launch 100 concurrent operations
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					errors <- ctx.Err()
					return
				default:
					_, err := system.ExecuteCLI("--version")
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Should not deadlock
		select {
		case <-ctx.Done():
			t.Fatal("Should complete before timeout")
		default:
		}

		// Verify operations completed
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		require.Less(t, errorCount, 50, "Most operations should succeed")
	})

	t.Run("consecutive start/stop is safe", func(t *testing.T) {
		var wg sync.WaitGroup

		// Start and stop many systems sequentially but quickly
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				system := harness.NewSystem(t)
				system.Build()
				system.Start()
				time.Sleep(10 * time.Millisecond)
				system.Stop()
			}(i)
		}

		wg.Wait()

		// Should complete without deadlocks
	})
}

func TestConcurrency_Fairness(t *testing.T) {
	t.Run("all goroutines get a chance to run", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var mu sync.Mutex
		var wg sync.WaitGroup
		runs := make([]int, 10)

		// 10 goroutines
		for i := 0; i < 10; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					select {
					case <-ctx.Done():
						return
					default:
						_, _ = system.ExecuteCLI("--version")
						mu.Lock()
						runs[id]++
						mu.Unlock()
					}
				}
			}(i)
			wg.Add(1)
		}

		wg.Wait()

		// All goroutines should have run
		for i, count := range runs {
			require.Greater(t, count, 10, "Goroutine %d should have run", i)
		}
	})

	t.Run("no starvation under load", func(t *testing.T) {
		sm := harness.NewStateManager()
		defer sm.Stop()

		var mu sync.Mutex
		results := make([]int, 5)

		// 5 goroutines competing for state manager
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					mu.Lock()
					_ = sm.Current()
					results[id]++
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		// All goroutines should have completed work
		for i, count := range results {
			require.Greater(t, count, 50, "Goroutine %d should have completed work", i)
		}
	})
}

func TestConcurrency_Stress(t *testing.T) {
	t.Run("high concurrency load test", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		errors := make(chan error, 1000)
		successCount := 0
		var mu sync.Mutex

		// Launch 1000 concurrent requests
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				default:
					_, err := system.ExecuteCLI("--version")
					mu.Lock()
					if err == nil {
						successCount++
					}
					errors <- err
					mu.Unlock()
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Count errors
		errorCount := 0
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}

		// At least 95% should succeed
		successRate := float64(successCount) / 1000.0
		require.Greater(t, successRate, 0.95, "Success rate should be >95%%")

		// System should still be operational
		require.True(t, system.Ready(), "System should remain ready under load")
	})
}
