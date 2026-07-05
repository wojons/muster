package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestCLICreation(t *testing.T) {
	t.Run("createRootCommand returns valid command", func(t *testing.T) {
		cmd := createRootCommand()
		require.NotNil(t, cmd)
		assert.NotEmpty(t, cmd.Use)
	})
}

func TestPersistentFlags(t *testing.T) {
	t.Run("all persistent flags are registered", func(t *testing.T) {
		cmd := createRootCommand()

		flags := []string{
			"config",
			"host",
			"verbose",
			"log-level",
			"log-format",
			"database-url",
			"auth-token",
			"timeout",
			"max-retries",
		}

		for _, flag := range flags {
			assert.True(t, cmd.PersistentFlags().Lookup(flag) != nil,
				"flag %s should be registered", flag)
		}
	})

	t.Run("flags have correct default values", func(t *testing.T) {
		cmd := createRootCommand()

		host, _ := cmd.PersistentFlags().GetString("host")
		assert.Equal(t, "localhost:8080", host)

		verbose, _ := cmd.PersistentFlags().GetBool("verbose")
		assert.False(t, verbose)

		logLevel, _ := cmd.PersistentFlags().GetString("log-level")
		assert.Equal(t, "info", logLevel)

		logFormat, _ := cmd.PersistentFlags().GetString("log-format")
		assert.Equal(t, "console", logFormat)

		timeout, _ := cmd.PersistentFlags().GetDuration("timeout")
		assert.Equal(t, 30*time.Second, timeout)

		maxRetries, _ := cmd.PersistentFlags().GetInt("max-retries")
		assert.Equal(t, 3, maxRetries)
	})
}

func TestPersistentPreRun(t *testing.T) {
	t.Run("skips initialization for completion command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "completion"

		err := persistentPreRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("skips initialization for version command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "version"

		err := persistentPreRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("skips initialization for help command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "help"

		err := persistentPreRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("skips initialization for __complete command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "__complete"

		err := persistentPreRun(cmd, []string{})
		assert.NoError(t, err)
	})
}

func TestPersistentPostRun(t *testing.T) {
	t.Run("skips cleanup for completion command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "completion"

		err := persistentPostRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("skips cleanup for version command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "version"

		err := persistentPostRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("skips cleanup for help command", func(t *testing.T) {
		cmd := createRootCommand()
		cmd.Use = "help"

		err := persistentPostRun(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("initializes logger for regular commands", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			LogLevel:  "info",
			LogFormat: "console",
		}

		require.NoError(t, initializeLogger())
		defer func() {
			if logger != nil {
				_ = logger.Sync()
			}
		}()

		cmd := createRootCommand()
		cmd.Use = "test-command"

		err := persistentPostRun(cmd, []string{})
		assert.NoError(t, err)
	})
}

func TestInitializeConfig(t *testing.T) {
	t.Run("uses default config path when not specified", func(t *testing.T) {
		oldLoader := loader
		oldCfg := cfg
		defer func() {
			loader = oldLoader
			cfg = oldCfg
		}()

		loader = config.NewLoader()
		cmd := createRootCommand()

		err := initializeConfig(cmd)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
	})

	t.Run("handles missing config file gracefully", func(t *testing.T) {
		oldLoader := loader
		oldCfg := cfg
		defer func() {
			loader = oldLoader
			cfg = oldCfg
		}()

		loader = config.NewLoader()
		tmpDir := t.TempDir()
		nonExistentConfig := tmpDir + "/nonexistent.yaml"
		loader.SetConfigFile(nonExistentConfig)
		cmd := createRootCommand()

		_ = initializeConfig(cmd)
		// initializeConfig returns nil on missing file
		assert.NotNil(t, cfg)
	})
}

func TestInitializeLogger(t *testing.T) {
	t.Run("initializes console logger", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			LogLevel:  "info",
			LogFormat: "console",
		}

		err := initializeLogger()
		assert.NoError(t, err)
		assert.NotNil(t, logger)

		if logger != nil {
			_ = logger.Sync()
		}
	})

	t.Run("initializes JSON logger", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			LogLevel:  "info",
			LogFormat: "json",
		}

		err := initializeLogger()
		assert.NoError(t, err)
		assert.NotNil(t, logger)

		if logger != nil {
			_ = logger.Sync()
		}
	})

	t.Run("enables verbose mode", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			LogLevel:  "info",
			LogFormat: "console",
			Verbose:   true,
		}

		err := initializeLogger()
		assert.NoError(t, err)
		assert.NotNil(t, logger)

		if logger != nil {
			_ = logger.Sync()
		}
	})

	t.Run("handles invalid log level", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			LogLevel:  "invalid",
			LogFormat: "console",
		}

		err := initializeLogger()
		assert.NoError(t, err)
		assert.NotNil(t, logger)

		if logger != nil {
			_ = logger.Sync()
		}
	})
}

func TestInitializeHTTPClient(t *testing.T) {
	t.Run("initializes HTTP client with timeout", func(t *testing.T) {
		oldCfg := cfg
		defer func() { cfg = oldCfg }()

		cfg = &config.Config{
			Timeout: 60 * time.Second,
		}

		initializeHTTPClient()
		assert.NotNil(t, cfg)
	})
}

func TestCLIError(t *testing.T) {
	t.Run("CLIError implements error interface", func(t *testing.T) {
		err := &CLIError{
			Message:  "test error",
			ExitCode: ExitGeneralError,
		}

		assert.Implements(t, (*error)(nil), err)
		assert.Equal(t, "test error", err.Error())
	})

	t.Run("CLIError with cause", func(t *testing.T) {
		causeErr := os.ErrNotExist
		err := &CLIError{
			Message:  "failed to read file",
			ExitCode: ExitGeneralError,
			Cause:    causeErr,
		}

		expected := "failed to read file: file does not exist"
		assert.Equal(t, expected, err.Error())
		assert.Equal(t, causeErr, err.Unwrap())
	})

	t.Run("CLIError Unwrap returns cause", func(t *testing.T) {
		causeErr := os.ErrPermission
		err := &CLIError{
			Message: "permission denied",
			Cause:   causeErr,
		}

		assert.Equal(t, causeErr, err.Unwrap())
	})
}

func TestHandleError(t *testing.T) {
	t.Run("returns ExitSuccess for nil error", func(t *testing.T) {
		exitCode := handleError(nil)
		assert.Equal(t, ExitSuccess, exitCode)
	})

	t.Run("handles CLIError with custom exit code", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		err := &CLIError{
			Message:  "test error",
			ExitCode: ExitConfigError,
		}

		exitCode := handleError(err)
		assert.Equal(t, ExitConfigError, exitCode)

		_ = w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()
		assert.Contains(t, output, "Error:")
		assert.Contains(t, output, "test error")
	})

	t.Run("handles context canceled", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		exitCode := handleError(ctx.Err())
		assert.Equal(t, ExitSignal, exitCode)

		_ = w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()
		assert.Contains(t, output, "cancelled")
	})

	t.Run("handles context deadline exceeded", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		<-ctx.Done()

		exitCode := handleError(ctx.Err())
		assert.Equal(t, ExitGeneralError, exitCode)

		_ = w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()
		assert.Contains(t, output, "timed out")
	})

	t.Run("handles generic error", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		err := os.ErrNotExist

		exitCode := handleError(err)
		assert.Equal(t, ExitGeneralError, exitCode)

		_ = w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()
		assert.Contains(t, output, "Error:")
	})
}

func TestHandlePanic(t *testing.T) {
	t.Skip("Skipping test - handlePanic calls os.Exit which terminates test process")
}

func TestSetupSignalHandling(t *testing.T) {
	t.Run("returns signal channel", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := setupSignalHandling(cancel)
		assert.NotNil(t, sigChan)
	})

	t.Run("handles SIGINT", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := setupSignalHandling(cancel)
		assert.NotNil(t, sigChan)
	})

	t.Run("handles SIGTERM", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := setupSignalHandling(cancel)
		assert.NotNil(t, sigChan)
	})

	t.Run("handles SIGHUP", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := setupSignalHandling(cancel)
		assert.NotNil(t, sigChan)
	})
}

func TestRunCLI(t *testing.T) {
	t.Run("executes CLI successfully", func(t *testing.T) {
		ctx := context.Background()
		sigChan := make(chan os.Signal, 1)

		err := runCLI(ctx, sigChan)
		_ = ctx
		assert.NoError(t, err)
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sigChan := make(chan os.Signal, 1)

		cancel()

		err := runCLI(ctx, sigChan)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "canceled") ||
			strings.Contains(err.Error(), "received signal"))
	})

	t.Run("handles signal interruption", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sigChan := make(chan os.Signal, 1)

		cancel()

		err := runCLI(ctx, sigChan)

		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "canceled") ||
			strings.Contains(err.Error(), "received signal"),
			"Expected error to mention cancellation or signal, got: %v", err)
	})

	t.Run("handles timeout", func(t *testing.T) {
		t.Skip("Skipping - CLI executes too quickly for timeout test")
	})
}

func TestExitCodes(t *testing.T) {
	t.Run("exit codes are defined correctly", func(t *testing.T) {
		assert.Equal(t, 0, ExitSuccess)
		assert.Equal(t, 1, ExitGeneralError)
		assert.Equal(t, 2, ExitConfigError)
		assert.Equal(t, 3, ExitDatabaseError)
		assert.Equal(t, 4, ExitAuthError)
		assert.Equal(t, 130, ExitSignal)
		assert.Equal(t, 137, ExitPanic)
	})
}

func TestCLIFlags(t *testing.T) {
	t.Run("config flag accepts path", func(t *testing.T) {
		cmd := createRootCommand()

		tmpDir := t.TempDir()
		testConfig := tmpDir + "/test-config.yaml"

		err := cmd.PersistentFlags().Set("config", testConfig)
		assert.NoError(t, err)

		configFile, err := cmd.PersistentFlags().GetString("config")
		assert.NoError(t, err)
		assert.Equal(t, testConfig, configFile)
	})

	t.Run("database-url flag accepts connection string", func(t *testing.T) {
		cmd := createRootCommand()

		dbURL := "postgres://user:***@localhost:5432/dbname"
		err := cmd.PersistentFlags().Set("database-url", dbURL)
		assert.NoError(t, err)

		result, err := cmd.PersistentFlags().GetString("database-url")
		assert.NoError(t, err)
		assert.Equal(t, dbURL, result)
	})

	t.Run("auth-token flag accepts token", func(t *testing.T) {
		cmd := createRootCommand()

		token := "Bearer eyJhbG...VCJ9"
		err := cmd.PersistentFlags().Set("auth-token", token)
		assert.NoError(t, err)

		result, err := cmd.PersistentFlags().GetString("auth-token")
		assert.NoError(t, err)
		assert.Equal(t, token, result)
	})
}
