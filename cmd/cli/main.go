// Package main provides the CLI entry point for the muster tool.
// It handles initialization, configuration loading, command dispatch,
// error handling, and graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/wojons/muster/internal/audit"
	builtpkg "github.com/wojons/muster/internal/builtin"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	ExitSuccess       = 0
	ExitGeneralError  = 1
	ExitConfigError   = 2
	ExitDatabaseError = 3
	ExitAuthError     = 4
	ExitSignal        = 130
	ExitPanic         = 137
)

var (
	cfg       *config.Config
	logger    *zap.Logger
	auditHook *audit.Hook
	auditFlag bool
	loader    *config.Loader
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			handlePanic(r)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := setupSignalHandling(cancel)

	if err := runCLI(ctx, sigChan); err != nil {
		exitCode := handleError(err)
		os.Exit(exitCode)
	}

	os.Exit(ExitSuccess)
}

func runCLI(ctx context.Context, sigChan <-chan os.Signal) error {
	loader = config.NewLoader()
	rootCmd := createRootCommand()

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return persistentPreRun(cmd, args)
	}

	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		return persistentPostRun(cmd, args)
	}

	done := make(chan error, 1)
	go func() {
		done <- rootCmd.ExecuteContext(ctx)
	}()

	select {
	case err := <-done:
		return err
	case sig := <-sigChan:
		return fmt.Errorf("received signal: %v", sig)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func createRootCommand() *cobra.Command {
	addPersistentFlags := func(cmd *cobra.Command) {
		cmd.PersistentFlags().StringP("config", "c", "", "Config file path (default: $HOME/.muster/config.yaml)")
		cmd.PersistentFlags().StringP("host", "H", "localhost:8080", "Server host address")
		cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
		cmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error, fatal)")
		cmd.PersistentFlags().String("log-format", "console", "Log format (json, console)")
		cmd.PersistentFlags().String("database-url", "", "Database connection URL")
		cmd.PersistentFlags().String("auth-token", "", "Authentication token")
		cmd.PersistentFlags().Duration("timeout", 30*time.Second, "Request timeout")
		cmd.PersistentFlags().Int("max-retries", 3, "Maximum number of retries for failed requests")
		cmd.PersistentFlags().BoolVar(&auditFlag, "audit", false, "Enable audit logging (also set MUSTER_AUDIT_ENABLED=1)")
	}

	// Initialize audit hook (opt-in via --audit flag or MUSTER_AUDIT_ENABLED env var)
	auditConfig := audit.DefaultConfig()
	if auditFlag {
		auditConfig.Enabled = true
	}
	auditHook = audit.NewHook(auditConfig)

	// Create a minimal config for builtin commands that expect one
	// (the real config is loaded in persistentPreRun via loader.Load())
	initialCfg := &config.Config{Version: "1.0.0"}
	root := builtpkg.NewRootCommandWithAudit(initialCfg, auditHook)

	addPersistentFlags(root)

	return root
}

func persistentPreRun(cmd *cobra.Command, args []string) error {
	// Skip config loading for meta-commands
	cmdName := cmd.Name()
	if cmdName == "completion" || cmdName == "version" || cmdName == "help" || cmdName == "__complete" {
		return nil
	}

	if err := initializeConfig(cmd); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	if err := initializeLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	initializeHTTPClient()

	logger.Info("muster CLI initialized",
		zap.String("version", version.Get().Short()),
		zap.String("command", cmd.Name()),
	)

	return nil
}

func persistentPostRun(cmd *cobra.Command, args []string) error {
	cmdName := cmd.Name()
	if cmdName == "completion" || cmdName == "version" || cmdName == "help" || cmdName == "__complete" {
		return nil
	}

	if logger != nil {
		logger.Info("cleaning up resources")
		_ = logger.Sync()
	}

	// Close audit hook to flush pending events
	if auditHook != nil {
		if err := auditHook.Close(); err != nil {
			if logger != nil {
				logger.Warn("audit: close error", zap.Error(err))
			}
		}
	}

	return nil
}

func initializeConfig(cmd *cobra.Command) error {
	// Check for explicit --config flag
	configFile, _ := cmd.Flags().GetString("config")
	if configFile != "" {
		loader.SetConfigFile(configFile)
	} else {
		// Default to $HOME/.muster/config.yaml (non-fatal if missing)
		home, err := os.UserHomeDir()
		if err == nil {
			defaultPath := filepath.Join(home, ".muster", "config.yaml")
			if _, err := os.Stat(defaultPath); err == nil {
				loader.SetConfigFile(defaultPath)
			}
		}
	}

	// Load config from all sources
	var loadErr error
	cfg, loadErr = loader.Load()
	if loadErr != nil && configFile != "" {
		return fmt.Errorf("load config: %w", loadErr)
	}
	// If no config file exists, start with defaults
	if cfg == nil {
		cfg = &config.Config{
			Version:    "1.0.0",
			Host:       "localhost:8080",
			LogLevel:   "info",
			LogFormat:  "console",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		}
	}

	// Apply CLI flags as final overrides (highest priority)
	if err := loader.BindPFlags(cmd); err != nil {
		return fmt.Errorf("bind flags: %w", err)
	}
	if err := loader.ApplyCLIFlags(cmd, cfg); err != nil {
		return fmt.Errorf("apply cli flags: %w", err)
	}

	// Set computed paths
	if cfg.ConfigDir == "" {
		home, _ := os.UserHomeDir()
		cfg.ConfigDir = filepath.Join(home, ".config", "muster")
		cfg.CacheDir = filepath.Join(home, ".cache", "muster")
		cfg.DataDir = filepath.Join(home, ".local", "share", "muster")
	}

	return nil
}

func initializeLogger() error {
	var zapCfg zap.Config

	logFormat := cfg.LogFormat
	if cfg.Logging.Format != "" {
		logFormat = cfg.Logging.Format
	}

	if logFormat == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logLevel := cfg.LogLevel
	if cfg.Logging.Level != "" {
		logLevel = cfg.Logging.Level
	}

	level, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	if logFormat == "json" {
		zapCfg.DisableCaller = true
	}

	if cfg.Verbose {
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	logger, err = zapCfg.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}

	return nil
}

func initializeHTTPClient() {
	_ = &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	// TODO: Store in global or pass to commands that need it
}

func setupSignalHandling(cancel context.CancelFunc) <-chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigChan {
			if logger != nil {
				logger.Info("received signal",
					zap.String("signal", sig.String()),
				)
			}

			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				cancel()
				return
			case syscall.SIGHUP:
				if logger != nil {
					logger.Info("reloading configuration")
				}
			}
		}
	}()

	return sigChan
}

type CLIError struct {
	Message  string
	ExitCode int
	Cause    error
}

func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *CLIError) Unwrap() error {
	return e.Cause
}

func handleError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		fmt.Fprintf(os.Stderr, "Error: %s\n", cliErr.Error())
		if logger != nil {
			logger.Error("command failed",
				zap.String("error", cliErr.Error()),
				zap.Int("exit_code", cliErr.ExitCode),
			)
		}
		return cliErr.ExitCode
	}

	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "Operation cancelled")
		return ExitSignal
	}

	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "Operation timed out")
		return ExitGeneralError
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	if logger != nil {
		logger.Error("unexpected error", zap.Error(err))
	}

	return ExitGeneralError
}

func handlePanic(r interface{}) {
	stack := debug.Stack()

	fmt.Fprintf(os.Stderr, "\nFatal Error: %v\n\n", r)
	fmt.Fprintln(os.Stderr, "Stack trace:")
	fmt.Fprintln(os.Stderr, string(stack))

	if logger != nil {
		logger.Fatal("panic recovered",
			zap.Any("panic", r),
			zap.String("stack", string(stack)),
		)
		_ = logger.Sync()
	}

	os.Exit(ExitPanic)
}
