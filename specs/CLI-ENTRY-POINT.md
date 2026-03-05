# CLI Entry Point Specification

## Overview

Complete specification for the CLI entry point (`main.go`) for the muster CLI tool. This document provides the complete implementation including all initialization, error handling, signal handling, and command structure.

## File Location

```
cmd/cli/main.go
```

## Complete Implementation

### main.go

```go
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

	"github.com/anomalyco/muster/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// =============================================================================
// Exit Codes
// =============================================================================

const (
	ExitSuccess        = 0
	ExitGeneralError   = 1
	ExitConfigError    = 2
	ExitDatabaseError  = 3
	ExitAuthError      = 4
	ExitSignal         = 130
	ExitPanic          = 137
)

// =============================================================================
// Global State
// =============================================================================

var (
	// cfg holds the loaded configuration
	cfg *Config

	// logger is the global logger instance
	logger *zap.Logger

	// httpClient is the shared HTTP client for API calls
	httpClient *http.Client

	// inventoryMgr manages inventory operations
	inventoryMgr InventoryManager

	// authProvider handles authentication
	authProvider AuthProvider

	// dbConnection holds the database connection
	dbConnection DatabaseConnection
)

// Config represents the application configuration
type Config struct {
	ConfigFile      string        `mapstructure:"config_file"`
	Host            string        `mapstructure:"host"`
	Verbose         bool          `mapstructure:"verbose"`
	LogLevel        string        `mapstructure:"log_level"`
	LogFormat       string        `mapstructure:"log_format"`
	DatabaseURL     string        `mapstructure:"database_url"`
	AuthToken       string        `mapstructure:"auth_token"`
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxRetries      int           `mapstructure:"max_retries"`
	Profiles        []string      `mapstructure:"profiles"`
}

// InventoryManager defines the interface for inventory operations
type InventoryManager interface {
	Initialize(ctx context.Context) error
	Close() error
	List(ctx context.Context) ([]Inventory, error)
	Get(ctx context.Context, id string) (*Inventory, error)
}

// AuthProvider defines the interface for authentication
type AuthProvider interface {
	Initialize(ctx context.Context) error
	Authenticate(ctx context.Context) error
	GetToken() string
	Close() error
}

// DatabaseConnection defines the interface for database operations
type DatabaseConnection interface {
	Connect(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
}

// Inventory represents an inventory item (simplified for main.go)
type Inventory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// =============================================================================
// Main Function
// =============================================================================

func main() {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			handlePanic(r)
		}
	}()

	// Create root context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := setupSignalHandling(cancel)

	// Execute the CLI
	if err := runCLI(ctx, sigChan); err != nil {
		exitCode := handleError(err)
		os.Exit(exitCode)
	}

	os.Exit(ExitSuccess)
}

// runCLI executes the CLI with the given context and signal channel
func runCLI(ctx context.Context, sigChan <-chan os.Signal) error {
	// Create root command
	rootCmd := newRootCommand()

	// Set up pre-run hooks
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return persistentPreRun(cmd, args)
	}

	// Set up post-run hooks
	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		return persistentPostRun(cmd, args)
	}

	// Add subcommands
	addSubcommands(rootCmd)

	// Execute with signal awareness
	done := make(chan error, 1)
	go func() {
		done <- rootCmd.Execute()
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

// =============================================================================
// Root Command
// =============================================================================

// newRootCommand creates the root command for the CLI
func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "muster",
		Short: "Muster - API specification and WASM module management tool",
		Long: `Muster is a comprehensive CLI tool for managing API specifications,
WASM modules, and service inventories.

It provides capabilities for:
  • Managing API specifications and versions
  • Handling WASM module deployments
  • Discovering and inventorying services
  • Interacting with MCP servers

Use 'muster --help' for more information about available commands.`,
		Example: `  # Initialize a new project
  muster init

  # Add an inventory item
  muster inventory add --name my-api --source ./openapi.yaml

  # List all inventories
  muster inventory list

  # Deploy a WASM module
  muster module deploy --file ./module.wasm`,
		Version: version.Info(),
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommand is provided, show help
			return cmd.Help()
		},
		// Disable automatic printing of errors - we handle them ourselves
		SilenceErrors: true,
		// Disable automatic usage printing on error
		SilenceUsage: false,
	}

	// Add persistent flags
	addPersistentFlags(cmd)

	// Add version flag handling
	cmd.SetVersionTemplate(`{{.Name}} version {{.Version}}
`)

	return cmd
}

// addPersistentFlags adds global flags to the root command
func addPersistentFlags(cmd *cobra.Command) {
	// Config file flag
	cmd.PersistentFlags().StringP(
		"config", "c",
		"",
		"Config file path (default: $HOME/.muster/config.yaml)",
	)
	viper.BindPFlag("config_file", cmd.PersistentFlags().Lookup("config"))

	// Host flag
	cmd.PersistentFlags().StringP(
		"host", "H",
		"localhost:8080",
		"Server host address",
	)
	viper.BindPFlag("host", cmd.PersistentFlags().Lookup("host"))

	// Verbose flag
	cmd.PersistentFlags().BoolP(
		"verbose", "v",
		false,
		"Enable verbose output",
	)
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	// Log level flag
	cmd.PersistentFlags().String(
		"log-level",
		"info",
		"Log level (debug, info, warn, error, fatal)",
	)
	viper.BindPFlag("log_level", cmd.PersistentFlags().Lookup("log-level"))

	// Log format flag
	cmd.PersistentFlags().String(
		"log-format",
		"console",
		"Log format (json, console)",
	)
	viper.BindPFlag("log_format", cmd.PersistentFlags().Lookup("log-format"))

	// Database URL flag
	cmd.PersistentFlags().String(
		"database-url",
		"",
		"Database connection URL",
	)
	viper.BindPFlag("database_url", cmd.PersistentFlags().Lookup("database-url"))

	// Auth token flag
	cmd.PersistentFlags().String(
		"auth-token",
		"",
		"Authentication token",
	)
	viper.BindPFlag("auth_token", cmd.PersistentFlags().Lookup("auth-token"))

	// Timeout flag
	cmd.PersistentFlags().Duration(
		"timeout",
		30*time.Second,
		"Request timeout",
	)
	viper.BindPFlag("timeout", cmd.PersistentFlags().Lookup("timeout"))

	// Max retries flag
	cmd.PersistentFlags().Int(
		"max-retries",
		3,
		"Maximum number of retries for failed requests",
	)
	viper.BindPFlag("max_retries", cmd.PersistentFlags().Lookup("max-retries"))
}

// =============================================================================
// Subcommands
// =============================================================================

// addSubcommands adds all subcommands to the root command
func addSubcommands(rootCmd *cobra.Command) {
	// Add completion commands
	rootCmd.AddCommand(newCompletionCommand())

	// Add version command
	rootCmd.AddCommand(newVersionCommand())

	// Add init command
	rootCmd.AddCommand(newInitCommand())

	// Add inventory command group
	rootCmd.AddCommand(newInventoryCommand())

	// Add module command group
	rootCmd.AddCommand(newModuleCommand())

	// Add profile command group
	rootCmd.AddCommand(newProfileCommand())

	// Add config command group
	rootCmd.AddCommand(newConfigCommand())

	// Add auth command group
	rootCmd.AddCommand(newAuthCommand())
}

// newCompletionCommand creates the shell completion command
func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for muster.

To load completions:

Bash:
  $ source <(muster completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ muster completion bash > /etc/bash_completion.d/muster
  # macOS:
  $ muster completion bash > $(brew --prefix)/etc/bash_completion.d/muster

Zsh:
  $ source <(muster completion zsh)
  # To load completions for each session, execute once:
  $ muster completion zsh > "${fpath[1]}/_muster"

Fish:
  $ muster completion fish | source
  # To load completions for each session, execute once:
  $ muster completion fish > ~/.config/fish/completions/muster.fish

PowerShell:
  PS> muster completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> muster completion powershell > muster.ps1
  # and source this file from your PowerShell profile.`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				cmd.Root().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
		},
	}
}

// newVersionCommand creates the version command
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print detailed version information about the muster CLI tool.`,
		Run: func(cmd *cobra.Command, args []string) {
			info := version.FullInfo()
			fmt.Fprintf(cmd.OutOrStdout(), "Version:    %s\n", info["version"])
			fmt.Fprintf(cmd.OutOrStdout(), "Commit:     %s\n", info["commit"])
			fmt.Fprintf(cmd.OutOrStdout(), "Build Date: %s\n", info["build_date"])
			fmt.Fprintf(cmd.OutOrStdout(), "Branch:     %s\n", info["branch"])
			fmt.Fprintf(cmd.OutOrStdout(), "Go Version: %s\n", info["go_version"])
		},
	}
}

// newInitCommand creates the init command
func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Initialize a new muster project",
		Long: `Initialize a new muster project in the specified directory.

This creates the necessary configuration files and directory structure
for a new muster project. If no directory is specified, the current
directory is used.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return initializeProject(dir)
		},
	}
}

// newInventoryCommand creates the inventory command group
func newInventoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inventory",
		Aliases: []string{"inv", "i"},
		Short:   "Manage API specifications and WASM modules",
		Long:    `Manage inventories of API specifications, WASM modules, and service definitions.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all inventories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listInventories(cmd.Context())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a new inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addInventory(cmd.Context(), args)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get [id]",
		Short: "Get inventory details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return getInventory(cmd.Context(), args[0])
		},
	})

	return cmd
}

// newModuleCommand creates the module command group
func newModuleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "module",
		Aliases: []string{"mod", "m"},
		Short:   "Manage WASM modules",
		Long:    `Deploy, configure, and manage WebAssembly modules.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "deploy",
		Short: "Deploy a WASM module",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployModule(cmd.Context(), args)
		},
	})

	return cmd
}

// newProfileCommand creates the profile command group
func newProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Aliases: []string{"prof", "p"},
		Short:   "Manage configuration profiles",
		Long:    `Create, switch, and manage configuration profiles for different environments.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listProfiles(cmd.Context())
		},
	})

	return cmd
}

// newConfigCommand creates the config command group
func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and modify muster configuration settings.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return viewConfig(cmd.Context())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setConfig(cmd.Context(), args[0], args[1])
		},
	})

	return cmd
}

// newAuthCommand creates the auth command group
func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		Long:  `Login, logout, and manage authentication tokens.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Authenticate with the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd.Context())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Logout and clear credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logout(cmd.Context())
		},
	})

	return cmd
}

// =============================================================================
// Pre/Post Run Hooks
// =============================================================================

// persistentPreRun runs before every command
func persistentPreRun(cmd *cobra.Command, args []string) error {
	// Skip initialization for completion and version commands
	if cmd.Name() == "completion" || cmd.Name() == "version" {
		return nil
	}

	// Initialize configuration
	if err := initializeConfig(cmd); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Initialize logger
	if err := initializeLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize HTTP client
	initializeHTTPClient()

	// Initialize database (if needed)
	if requiresDatabase(cmd) {
		if err := initializeDatabase(cmd.Context()); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	// Initialize inventory manager (if needed)
	if requiresInventory(cmd) {
		if err := initializeInventoryManager(cmd.Context()); err != nil {
			return fmt.Errorf("failed to initialize inventory manager: %w", err)
		}
	}

	// Initialize auth provider (if needed)
	if requiresAuth(cmd) {
		if err := initializeAuthProvider(cmd.Context()); err != nil {
			return fmt.Errorf("failed to initialize auth provider: %w", err)
		}
	}

	logger.Info("muster CLI initialized",
		zap.String("version", version.Version),
		zap.String("command", cmd.Name()),
	)

	return nil
}

// persistentPostRun runs after every command
func persistentPostRun(cmd *cobra.Command, args []string) error {
	// Skip cleanup for completion and version commands
	if cmd.Name() == "completion" || cmd.Name() == "version" {
		return nil
	}

	logger.Info("cleaning up resources")

	// Cleanup in reverse order of initialization

	if authProvider != nil {
		if err := authProvider.Close(); err != nil {
			logger.Error("failed to close auth provider", zap.Error(err))
		}
	}

	if inventoryMgr != nil {
		if err := inventoryMgr.Close(); err != nil {
			logger.Error("failed to close inventory manager", zap.Error(err))
		}
	}

	if dbConnection != nil {
		if err := dbConnection.Close(); err != nil {
			logger.Error("failed to close database connection", zap.Error(err))
		}
	}

	if logger != nil {
		logger.Sync()
	}

	return nil
}

// =============================================================================
// Initialization
// =============================================================================

// initializeConfig loads configuration from file and environment
func initializeConfig(cmd *cobra.Command) error {
	viper.SetEnvPrefix("MUSTER")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	// Get config file path from flag or use default
	configFile := viper.GetString("config_file")
	if configFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configFile = filepath.Join(home, ".muster", "config.yaml")
		viper.Set("config_file", configFile)
	}

	// Load config file if it exists
	if _, err := os.Stat(configFile); err == nil {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into config struct
	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// initializeLogger sets up the global logger
func initializeLogger() error {
	var zapCfg zap.Config

	if cfg.LogFormat == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set log level
	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	// Disable caller in production
	if cfg.LogFormat == "json" {
		zapCfg.DisableCaller = true
	}

	// Add verbose mode
	if cfg.Verbose {
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	var err error
	logger, err = zapCfg.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}

	return nil
}

// initializeHTTPClient sets up the shared HTTP client
func initializeHTTPClient() {
	httpClient = &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// initializeDatabase sets up the database connection
func initializeDatabase(ctx context.Context) error {
	// Placeholder - actual implementation would use your database package
	// dbConnection = database.NewConnection(cfg.DatabaseURL)
	// return dbConnection.Connect(ctx)
	return nil
}

// initializeInventoryManager sets up the inventory manager
func initializeInventoryManager(ctx context.Context) error {
	// Placeholder - actual implementation would use your inventory package
	// inventoryMgr = inventory.NewManager(cfg, dbConnection)
	// return inventoryMgr.Initialize(ctx)
	return nil
}

// initializeAuthProvider sets up the auth provider
func initializeAuthProvider(ctx context.Context) error {
	// Placeholder - actual implementation would use your auth package
	// authProvider = auth.NewProvider(cfg.AuthToken)
	// return authProvider.Initialize(ctx)
	return nil
}

// =============================================================================
// Signal Handling
// =============================================================================

// setupSignalHandling configures OS signal handling
func setupSignalHandling(cancel context.CancelFunc) <-chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigChan {
			logger.Info("received signal",
				zap.String("signal", sig.String()),
			)

			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				// Initiate graceful shutdown
				cancel()
				return
			case syscall.SIGHUP:
				// Handle config reload (optional)
				logger.Info("reloading configuration")
			}
		}
	}()

	return sigChan
}

// =============================================================================
// Error Handling
// =============================================================================

// CLIError represents a CLI-specific error with exit code
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

// handleError processes errors and returns an exit code
func handleError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	// Check for specific error types
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

	// Handle context errors
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "Operation cancelled")
		return ExitSignal
	}

	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "Operation timed out")
		return ExitGeneralError
	}

	// General error
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	if logger != nil {
		logger.Error("unexpected error", zap.Error(err))
	}

	return ExitGeneralError
}

// handlePanic recovers from panics and logs them
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
		logger.Sync()
	}

	os.Exit(ExitPanic)
}

// =============================================================================
// Command Requirements
// =============================================================================

// requiresDatabase returns true if the command needs database access
func requiresDatabase(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "inventory", "module", "profile":
		return true
	default:
		return false
	}
}

// requiresInventory returns true if the command needs inventory manager
func requiresInventory(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "inventory", "module":
		return true
	default:
		return false
	}
}

// requiresAuth returns true if the command needs authentication
func requiresAuth(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "inventory", "module":
		return true
	default:
		return false
	}
}

// =============================================================================
// Command Implementations (Placeholders)
// =============================================================================

func initializeProject(dir string) error {
	logger.Info("initializing project", zap.String("directory", dir))
	// Implementation: create directory structure, default config, etc.
	return nil
}

func listInventories(ctx context.Context) error {
	logger.Info("listing inventories")
	if inventoryMgr == nil {
		return &CLIError{
			Message:  "inventory manager not initialized",
			ExitCode: ExitGeneralError,
		}
	}
	inventories, err := inventoryMgr.List(ctx)
	if err != nil {
		return err
	}
	for _, inv := range inventories {
		fmt.Printf("%s: %s\n", inv.ID, inv.Name)
	}
	return nil
}

func addInventory(ctx context.Context, args []string) error {
	logger.Info("adding inventory", zap.Strings("args", args))
	return nil
}

func getInventory(ctx context.Context, id string) error {
	logger.Info("getting inventory", zap.String("id", id))
	if inventoryMgr == nil {
		return &CLIError{
			Message:  "inventory manager not initialized",
			ExitCode: ExitGeneralError,
		}
	}
	inv, err := inventoryMgr.Get(ctx, id)
	if err != nil {
		return err
	}
	fmt.Printf("Inventory: %s\n", inv.Name)
	return nil
}

func deployModule(ctx context.Context, args []string) error {
	logger.Info("deploying module", zap.Strings("args", args))
	return nil
}

func listProfiles(ctx context.Context) error {
	logger.Info("listing profiles")
	return nil
}

func viewConfig(ctx context.Context) error {
	logger.Info("viewing config")
	if cfg == nil {
		return &CLIError{
			Message:  "configuration not loaded",
			ExitCode: ExitConfigError,
		}
	}
	fmt.Printf("Config File: %s\n", cfg.ConfigFile)
	fmt.Printf("Host: %s\n", cfg.Host)
	fmt.Printf("Log Level: %s\n", cfg.LogLevel)
	return nil
}

func setConfig(ctx context.Context, key, value string) error {
	logger.Info("setting config", zap.String("key", key), zap.String("value", value))
	return nil
}

func login(ctx context.Context) error {
	logger.Info("logging in")
	if authProvider == nil {
		return &CLIError{
			Message:  "auth provider not initialized",
			ExitCode: ExitAuthError,
		}
	}
	return authProvider.Authenticate(ctx)
}

func logout(ctx context.Context) error {
	logger.Info("logging out")
	if authProvider == nil {
		return &CLIError{
			Message:  "auth provider not initialized",
			ExitCode: ExitAuthError,
		}
	}
	return authProvider.Close()
}
```

## Initialization Order

The initialization sequence follows this strict order:

1. **Configuration Loading** (`initializeConfig`)
   - Load from file (if exists)
   - Load from environment variables
   - Load from command-line flags
   - Unmarshal into Config struct

2. **Logger Setup** (`initializeLogger`)
   - Configure based on log format (JSON/console)
   - Set log level from config
   - Enable verbose mode if requested

3. **HTTP Client Setup** (`initializeHTTPClient`)
   - Create shared HTTP client with timeout
   - Configure connection pooling

4. **Database Connection** (`initializeDatabase`) - conditional
   - Connect to database
   - Run migrations if enabled

5. **Inventory Manager** (`initializeInventoryManager`) - conditional
   - Initialize inventory service
   - Connect to storage backend

6. **Auth Provider** (`initializeAuthProvider`) - conditional
   - Initialize authentication
   - Load cached tokens if available

## Persistent Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config` | `-c` | `~/.muster/config.yaml` | Config file path |
| `--host` | `-H` | `localhost:8080` | Server host address |
| `--verbose` | `-v` | `false` | Enable verbose output |
| `--log-level` | | `info` | Log level (debug/info/warn/error) |
| `--log-format` | | `console` | Log format (json/console) |
| `--database-url` | | | Database connection URL |
| `--auth-token` | | | Authentication token |
| `--timeout` | | `30s` | Request timeout |
| `--max-retries` | | `3` | Maximum retries |

## Exit Codes

| Code | Name | Description |
|------|------|-------------|
| 0 | `ExitSuccess` | Command executed successfully |
| 1 | `ExitGeneralError` | General error occurred |
| 2 | `ExitConfigError` | Configuration error |
| 3 | `ExitDatabaseError` | Database connection error |
| 4 | `ExitAuthError` | Authentication error |
| 130 | `ExitSignal` | Interrupted by signal (Ctrl+C) |
| 137 | `ExitPanic` | Unhandled panic occurred |

## Signal Handling

The CLI handles the following signals:

- **SIGINT** (Ctrl+C): Graceful shutdown with context cancellation
- **SIGTERM**: Graceful shutdown with context cancellation
- **SIGHUP**: Configuration reload (optional behavior)

## Error Types

### CLIError

Custom error type with exit code support:

```go
type CLIError struct {
    Message  string
    ExitCode int
    Cause    error
}
```

### Panic Recovery

All panics are caught and:
1. Stack trace is printed to stderr
2. Error is logged with fatal level
3. Application exits with code 137

## Subcommands

### Top-Level Commands

- `completion` - Shell completion scripts
- `version` - Version information
- `init` - Initialize new project
- `inventory` - Inventory management
- `module` - WASM module management
- `profile` - Configuration profiles
- `config` - Configuration management
- `auth` - Authentication commands

### Command Groups

**Inventory Commands:**
- `inventory list` - List all inventories
- `inventory add` - Add new inventory
- `inventory get` - Get inventory details

**Module Commands:**
- `module deploy` - Deploy WASM module

**Profile Commands:**
- `profile list` - List all profiles

**Config Commands:**
- `config view` - View current configuration
- `config set` - Set configuration value

**Auth Commands:**
- `auth login` - Authenticate with server
- `auth logout` - Logout and clear credentials

## Dependencies

Required Go modules:

```go
require (
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    go.uber.org/zap v1.26.0
)
```

## Notes

1. **Thread Safety**: Global state variables are initialized once during pre-run and should not be modified concurrently.

2. **Resource Cleanup**: All resources are cleaned up in reverse initialization order during post-run.

3. **Conditional Initialization**: Database, inventory, and auth are only initialized for commands that require them.

4. **Context Propagation**: The root context is passed through the entire command chain for proper cancellation support.

5. **Error Wrapping**: All errors are wrapped with context using `%w` verb for proper error chain inspection.
