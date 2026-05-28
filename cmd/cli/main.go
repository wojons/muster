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
	"strings"
	"syscall"
	"time"

	builtpkg "github.com/wojons/muster/internal/builtin"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	cfg    *appConfig
	logger *zap.Logger
	_      *http.Client // httpClient reserved for future use
)

type appConfig struct {
	ConfigFile  string        `mapstructure:"config_file"`
	Host        string        `mapstructure:"host"`
	Verbose     bool          `mapstructure:"verbose"`
	LogLevel    string        `mapstructure:"log_level"`
	LogFormat   string        `mapstructure:"log_format"`
	DatabaseURL string        `mapstructure:"database_url"`
	AuthToken   string        `mapstructure:"auth_token"`
	Timeout     time.Duration `mapstructure:"timeout"`
	MaxRetries  int           `mapstructure:"max_retries"`
	Profiles    []string      `mapstructure:"profiles"`
}

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
	configMgr, err := config.Load()
	if err != nil {
		loader := config.NewLoader()
		configMgr, _ = loader.Load()
	}

	root := builtpkg.NewRootCommand(configMgr)

	addPersistentFlags(root)

	return root
}

func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", "", "Config file path (default: $HOME/.muster/config.yaml)")
	_ = viper.BindPFlag("config_file", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().StringP("host", "H", "localhost:8080", "Server host address")
	_ = viper.BindPFlag("host", cmd.PersistentFlags().Lookup("host"))

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	_ = viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	cmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error, fatal)")
	_ = viper.BindPFlag("log_level", cmd.PersistentFlags().Lookup("log-level"))

	cmd.PersistentFlags().String("log-format", "console", "Log format (json, console)")
	_ = viper.BindPFlag("log_format", cmd.PersistentFlags().Lookup("log-format"))

	cmd.PersistentFlags().String("database-url", "", "Database connection URL")
	_ = viper.BindPFlag("database_url", cmd.PersistentFlags().Lookup("database-url"))

	cmd.PersistentFlags().String("auth-token", "", "Authentication token")
	_ = viper.BindPFlag("auth_token", cmd.PersistentFlags().Lookup("auth-token"))

	cmd.PersistentFlags().Duration("timeout", 30*time.Second, "Request timeout")
	_ = viper.BindPFlag("timeout", cmd.PersistentFlags().Lookup("timeout"))

	cmd.PersistentFlags().Int("max-retries", 3, "Maximum number of retries for failed requests")
	_ = viper.BindPFlag("max_retries", cmd.PersistentFlags().Lookup("max-retries"))
}

func persistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "completion" || cmd.Name() == "version" || cmd.Name() == "help" || cmd.Name() == "__complete" {
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
	if cmd.Name() == "completion" || cmd.Name() == "version" || cmd.Name() == "help" || cmd.Name() == "__complete" {
		return nil
	}

	logger.Info("cleaning up resources")

	if logger != nil {
		_ = logger.Sync()
	}

	return nil
}

func initializeConfig(cmd *cobra.Command) error {
	viper.SetEnvPrefix("MUSTER")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configFile := viper.GetString("config_file")
	if configFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configFile = filepath.Join(home, ".muster", "config.yaml")
		viper.Set("config_file", configFile)
	}

	if _, err := os.Stat(configFile); err == nil {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	cfg = &appConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

func initializeLogger() error {
	var zapCfg zap.Config

	if cfg.LogFormat == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	if cfg.LogFormat == "json" {
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
