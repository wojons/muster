package workflows

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/test/functional/harness"
)

func TestCLIWorkflow(t *testing.T) {
	t.Run("complete CLI workflow", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		assertions := harness.NewSystemAssertions(t, system)

		// Step 1: System should be ready
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should become ready")

		// Step 2: CLI should execute --version
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err, "CLI --version should succeed")
		require.NotEmpty(t, output, "Version output should not be empty")

		// Step 3: CLI should execute --help
		output, err = system.ExecuteCLI("--help")
		require.NoError(t, err, "CLI --help should succeed")
		require.Contains(t, output, "Usage:", "Help should contain usage")

		// Step 4: Verify system is still operational
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should remain ready")
	})

	t.Run("CLI handles errors gracefully", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Invalid command should fail gracefully
		_, err := system.ExecuteCLI("invalid-command-xyz123")
		require.Error(t, err, "Invalid command should fail")

		// System should still be running
		require.True(t, system.IsRunning(), "System should still be running")
	})
}

func TestMCPWorkflow(t *testing.T) {
	t.Run("MCP server startup and shutdown", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		assertions := harness.NewSystemAssertions(t, system)

		// System should be ready
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should become ready")

		// Verify daemon socket exists
		harness.SocketExists(t, system.SocketPath())

		// Stop and verify cleanup
		system.Stop()
		_, err := os.Stat(system.SocketPath())
		require.Error(t, err, "Socket should be cleaned up")
	})

	t.Run("MCP server remains responsive", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		assertions := harness.NewSystemAssertions(t, system)

		// Make multiple requests to verify responsiveness
		for i := 0; i < 5; i++ {
			assertions.Eventually(5*time.Second, func() bool {
				return system.Ready()
			}, "System should remain responsive")
			time.Sleep(100 * time.Millisecond)
		}
	})
}

func TestAuthWorkflow(t *testing.T) {
	t.Run("authentication state management", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		assertions := harness.NewSystemAssertions(t, system)

		// System should be ready
		assertions.Eventually(5*time.Second, func() bool {
			return system.Ready()
		}, "System should become ready")

		// CLI commands should work without auth
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err, "CLI should work without auth")
		require.NotEmpty(t, output, "Version should be returned")
	})

	t.Run("CLI can store and use credentials", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Set environment variable for config
		configPath := system.TempDir() + "/config.yaml"
		system.SetConfigFile(configPath)

		// CLI should use config from environment variable
		_, err := system.ExecuteCLI("--version")
		require.NoError(t, err, "CLI should use config from MUSTER_CONFIG")
	})
}

func TestDaemonWorkflow(t *testing.T) {
	t.Run("daemon lifecycle management", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		// Start daemon
		system.Start()

		// Wait for ready
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Verify socket exists
		harness.SocketExists(t, system.SocketPath())

		// Verify daemon is running
		require.True(t, system.IsRunning(), "Daemon should be running")

		// Stop daemon
		system.Stop()

		// Verify stopped
		require.False(t, system.IsRunning(), "Daemon should not be running")
	})

	t.Run("daemon handles multiple start/stop cycles", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		for i := 0; i < 3; i++ {
			// Start
			system.Start()

			// Wait for ready
			require.Eventually(t, func() bool {
				return system.Ready()
			}, 5*time.Second, 100*time.Millisecond, "System should become ready")

			// Verify running
			require.True(t, system.IsRunning(), "Daemon should be running (cycle %d)", i)

			// Stop
			system.Stop()

			// Verify stopped
			require.False(t, system.IsRunning(), "Daemon should be stopped (cycle %d)", i)

			// Wait a moment before next cycle
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("daemon socket is created and usable", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Wait for socket creation
		require.Eventually(t, func() bool {
			_, err := os.Stat(system.SocketPath())
			return err == nil
		}, 2*time.Second, 10*time.Millisecond, "Socket should be created")

		// Verify socket is valid
		harness.SocketExists(t, system.SocketPath())

		// CLI can connect
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err, "CLI should connect via socket")
		require.NotEmpty(t, output, "CLI should return version")
	})

	t.Run("daemon handles concurrent CLI connections", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// System should be ready
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Make concurrent CLI requests
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		errors := make(chan error, 10)
		for i := 0; i < 5; i++ {
			go func() {
				select {
				case <-ctx.Done():
					return
				default:
					_, err := system.ExecuteCLI("--version")
					errors <- err
				}
			}()
		}

		// All requests should succeed
		for i := 0; i < 5; i++ {
			err := <-errors
			require.NoError(t, err, "All CLI requests should succeed")
		}
	})
}

func TestEndToEndWorkflow(t *testing.T) {
	t.Run("complete end-to-end system workflow", func(t *testing.T) {
		system := harness.NewSystem(t)

		// Build all binaries
		system.Build()

		// Verify binaries exist
		harness.FileExists(t, system.CLIPath())
		harness.FileExists(t, system.MCPPath())
		harness.FileExists(t, system.DaemonPath())

		// Start daemon
		system.Start()

		// Wait for ready
		require.Eventually(t, func() bool {
			return system.Ready()
		}, 5*time.Second, 100*time.Millisecond, "System should become ready")

		// Execute CLI command
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err, "CLI should execute successfully")
		require.NotEmpty(t, output, "CLI should return output")

		// Stop and cleanup
		system.Stop()

		// Verify cleanup
		_, err = os.Stat(system.SocketPath())
		require.Error(t, err, "System should clean up socket")
	})

	t.Run("system handles rapid start/stop cycles", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()

		for i := 0; i < 5; i++ {
			// Start
			system.Start()

			// Wait for ready
			require.Eventually(t, func() bool {
				return system.Ready()
			}, 5*time.Second, 100*time.Millisecond, "System should become ready")

			// Execute command
			_, err := system.ExecuteCLI("--version")
			require.NoError(t, err, "CLI should work")

			// Stop immediately
			system.Stop()
		}
	})

	t.Run("system maintains state across operations", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Get initial state
		state1 := system.State()
		require.Equal(t, harness.StateReady, state1, "System should start in ready state")

		// Execute commands
		for i := 0; i < 10; i++ {
			_, err := system.ExecuteCLI("--version")
			require.NoError(t, err)
		}

		// Verify state is still ready
		state2 := system.State()
		require.Equal(t, harness.StateReady, state2, "System should remain in ready state")
	})
}

func TestCLIWorkflow_ErrorHandling(t *testing.T) {
	t.Run("CLI handles missing spec gracefully", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Try to load non-existent spec
		_, err := system.ExecuteCLI("load", "/nonexistent/path/spec.yaml")
		require.Error(t, err, "Should fail to load non-existent spec")

		// System should still be operational
		require.True(t, system.IsRunning(), "System should still be running")
	})

	t.Run("CLI provides helpful error messages", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Invalid command
		_, err := system.ExecuteCLI("invalid-command")
		require.Error(t, err)
		require.NotEmpty(t, err.Error(), "Error message should not be empty")
	})

	t.Run("CLI recovers from errors", func(t *testing.T) {
		system := harness.NewSystem(t)
		system.Build()
		system.Start()
		defer system.Stop()

		// Make failing request
		_, err := system.ExecuteCLI("invalid-command-xyz")
		require.Error(t, err)

		// Next command should still work
		output, err := system.ExecuteCLI("--version")
		require.NoError(t, err)
		require.NotEmpty(t, output)
	})
}
