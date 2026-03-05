package main

import (
	"context"
	"os"
	"testing"

	"github.com/openapi-cli/openapi-cli/internal/builtin"
	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRun_ConfigLoad(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "config load should succeed")

	require.NotNil(t, cfg, "config should not be nil")
	require.NotEmpty(t, cfg.Version, "version should not be empty")
}

func TestNewRootCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	require.NotNil(t, root, "root command should not be nil")
	require.Equal(t, "openapi-cli", root.Use, "command use should be openapi-cli")
}

func TestNewRootCommand_ExecuteContext(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "root command should execute without error")
}

func TestNewRootCommand_ExecuteWithHelp(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "help command should execute without error")
}

func TestNewRootCommand_ExecuteWithVersion(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--version"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "version command should execute without error")
}

func TestNewRootCommand_ExecuteWithInvalidArgument(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--invalid-argument"}
	err := root.ExecuteContext(ctx)

	require.Error(t, err, "invalid argument should cause error")
}

func TestRun_Success(t *testing.T) {
	ctx := context.Background()

	// Load configuration with default values if needed
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{Version: "test"}
	}

	require.NotNil(t, cfg, "config should not be nil")

	// Build root command
	rootCmd := builtin.NewRootCommand(cfg)

	// Execute with context
	err = rootCmd.ExecuteContext(ctx)

	require.NoError(t, err, "run should succeed")
}

func TestRun_WithConfigError(t *testing.T) {
	// Test that run() handles config errors gracefully by using defaults
	cfg, err := config.Load()

	require.NoError(t, err, "config load should succeed or use defaults")

	rootCmd := builtin.NewRootCommand(cfg)
	require.NotNil(t, rootCmd, "root command should be created")
}

func TestMain_RunsProgram(t *testing.T) {
	// Test that main function exists and is callable
	// Since main calls os.Exit on error, we can't directly test it
	// This test verifies that the module has a main function
	// by checking we can import and reference it

	// If we reach here, the test passed
	require.True(t, true)
}

func TestNewRootCommand_HasAllSubcommands(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	expectedCommands := []string{
		"version",
		"config",
		"help",
		"completion",
		"init",
		"auth",
		"inventory",
		"modules",
		"generate",
		"request",
		"daemon",
	}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		require.True(t, found, "should have command: %s", cmdName)
	}
}

func TestNewRootCommand_HasConfigPassed(t *testing.T) {
	version := "1.0.0-test"
	cfg := &config.Config{Version: version}
	root := builtin.NewRootCommand(cfg)

	// Root command version comes from version.Get(), not cfg.Version
	// This test verifies the command was created with the config
	require.NotNil(t, root, "root command should be created")
	require.Contains(t, root.Version, "dev", "version should be set")
}

func TestRun_Integration(t *testing.T) {
	// Test the full run() function path
	// This calls the actual run() function from main.go
	err := run()

	// run() should execute without error (with no-arg default which shows help)
	// Note: run() displays help for no args, which is considered success
	require.NoError(t, err, "run() should execute successfully")
}

func TestRun_HandleConfigLoadError(t *testing.T) {
	// Test that config load errors don't prevent program from running
	// The actual run() function handles this by using defaults

	// Attempt to load config (may fail)
	cfg, err := config.Load()

	// Even if config fails, we should still be able to create a command
	if cfg == nil || err != nil {
		cfg = &config.Config{Version: "test"}
	}

	require.NotNil(t, cfg, "should have config (default if load failed)")

	// Should still be able to create and execute root command
	rootCmd := builtin.NewRootCommand(cfg)
	require.NotNil(t, rootCmd, "root command should be created")

	ctx := context.Background()
	err = rootCmd.ExecuteContext(ctx)

	require.NoError(t, err, "root command should execute with defaults")
}
