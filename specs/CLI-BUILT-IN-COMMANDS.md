# CLI Built-in Commands Specification

**Status**: ✅ COMPLETE  
**Date**: 2026-02-28  
**Purpose**: Complete implementation specification for all built-in CLI commands  
**Language**: Go 1.21+ with Cobra  

---

## Table of Contents

1. [Overview](#overview)
2. [Command Structure](#command-structure)
3. [Version Command](#1-version-command)
4. [Config Command Group](#2-config-command-group)
5. [Auth Command Group](#3-auth-command-group)
6. [Inventory Command Group](#4-inventory-command-group)
7. [Daemon Command Group](#5-daemon-command-group)
8. [Completion Command](#6-completion-command)
9. [Help Command](#7-help-command)
10. [Error Handling](#error-handling)
11. [Testing](#testing)

---

## Overview

Built-in commands are fundamental CLI operations that don't depend on external OpenAPI specifications. They provide core functionality for managing the CLI tool itself.

### Command Categories

| Category | Commands | Purpose |
|----------|----------|---------|
| **Meta** | version, help | CLI information |
| **Configuration** | config | Settings management |
| **Authentication** | auth | Credential handling |
| **Inventory** | inventory | Cache management |
| **Daemon** | daemon | Background service |
| **Shell** | completion | Shell integration |

---

## Command Structure

### Command Hierarchy

```mermaid
graph TB
    Root["muster<br/>(Root Command)"]
    
    Version["version<br/>Display version<br/>--json, --full, --check"]
    Config["config<br/>Configuration management<br/>list, get, set, remove"]
    Auth["auth<br/>Authentication<br/>login, logout, refresh, status"]
    Inventory["inventory<br/>Cache management<br/>list, get, update, remove, clear"]
    Daemon["daemon<br/>Background service<br/>start, stop, restart, status"]
    Completion["completion<br/>Shell integration<br/>bash, zsh, fish, powershell"]
    
    Root --> Version
    Root --> Config
    Root --> Auth
    Root --> Inventory
    Root --> Daemon
    Root --> Completion
    
    Config --> ConfigList["config list<br/>Show all configs"]
    Config --> ConfigGet["config get<br/>Get single config"]
    Config --> ConfigSet["config set<br/>Set config value"]
    Config --> ConfigRemove["config remove<br/>Remove config key"]
    
    Auth --> AuthLogin["auth login<br/>Login to service"]
    Auth --> AuthLogout["auth logout<br/>Logout from service"]
    Auth --> AuthStatus["auth status<br/>Show auth status"]
    
    Inventory --> InvList["cache list<br/>List cached items"]
    Inventory --> InvGet["cache get<br/>Get cached item"]
    Inventory --> InvUpdate["cache update<br/>Update cached item"]
    Inventory --> InvClear["cache clear<br/>Clear all cache"]
    
    Daemon --> DaemonStart["daemon start<br/>Start daemon"]
    Daemon --> DaemonStop["daemon stop<br/>Stop daemon"]
    Daemon --> DaemonStatus["daemon status<br/>Show daemon status"]
    
    Completion --> Bash["bash<br/>Generate bash completion"]
    Completion --> Zsh["zsh<br/>Generate zsh completion"]
    Completion --> Fish["fish<br/>Generate fish completion"]
    Completion --> PS["powershell<br/>Generate PowerShell completion"]
```

### Command Registration Pattern

```mermaid
flowchart LR
    A([Start]) --> B[Define CommandFunc Type]
    B --> C[Create Registry Array]
    C --> D[Add NewVersionCommand]
    D --> E[Add NewConfigCommand]
    E --> F[Add NewAuthCommand]
    F --> G[Add NewInventoryCommand]
    G --> H[Add NewDaemonCommand]
    H --> I[Add NewCompletionCommand]
    I --> J[Define RegisterAll Function]
    J --> K[Iterate Registry]
    K --> L[Call Each Function]
    L --> M[Return cobra.Command]
    M --> N[Add to Root Command]
    N --> O([Complete])
    
    style A fill:#9f9,stroke:#333
    style O fill:#9f9,stroke:#333
```

### Command Execution Lifecycle

```mermaid
sequenceDiagram
    participant User as User
    participant CLI as Root Command
    participant PreRun as Persistent PreRun
    participant Cmd as Subcommand
    participant PostRun as Persistent PostRun
    participant Hooks as Hooks
    
    User->>CLI: Execute command
    CLI->>PreRun: PersistentPreRunE
    
    alt PreRun Error
        PreRun-->>CLI: Return error
        CLI-->>User: Error message
        CLI-->>Hooks: Execute post-run hooks
    else PreRun Success
        PreRun-->>CLI: nil
        CLI->>Cmd: Call RunE handler
        
        alt Command Error
            Cmd-->>CLI: Return error
            CLI-->>User: Error message
            CLI->>PostRun: PersistentPostRunE
            PostRun-->>Hooks: Execute cleanup
            Hooks-->>User: Exit with error code
        else Command Success
            Cmd-->>CLI: nil
            CLI->>PostRun: PersistentPostRunE
            PostRun-->>Hooks: Execute cleanup
            Hooks-->>User: Exit with success
        end
    end
    
    Note over User,Hooks: Flags parsed, config loaded
    Note over PreRun: Validation, initialization
    Note over Cmd: Core command logic
    Note over PostRun: Cleanup, logging
```

### Root Command Setup

```go
// cmd/openapi-cli/main.go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    
    "github.com/openapi-cli/core/internal/cli/commands"
    "github.com/openapi-cli/core/internal/core/config"
    "github.com/openapi-cli/core/internal/version"
)

func main() {
    if err := Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

// Execute runs the root command
func Execute() error {
    return NewRootCommand().Execute()
}

// NewRootCommand creates the root command
func NewRootCommand() *cobra.Command {
    rootCmd := &cobra.Command{
        Use:   "openapi-cli",
        Short: "OpenAPI CLI - Dynamic API client",
        Long: `openapi-cli is a dynamic command-line client for OpenAPI specifications.

It automatically generates commands from OpenAPI specs and supports
multiple authentication methods, caching, and extensibility via WASM modules.`,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Skip config loading for certain commands
            if shouldSkipConfig(cmd) {
                return nil
            }
            return initConfig(cmd)
        },
        SilenceErrors: true,
        SilenceUsage:  true,
    }
    
    // Add global flags
    addGlobalFlags(rootCmd)
    
    // Add built-in commands
    rootCmd.AddCommand(commands.NewVersionCommand())
    rootCmd.AddCommand(commands.NewConfigCommand())
    rootCmd.AddCommand(commands.NewAuthCommand())
    rootCmd.AddCommand(commands.NewInventoryCommand())
    rootCmd.AddCommand(commands.NewDaemonCommand())
    rootCmd.AddCommand(commands.NewCompletionCommand())
    
    // Help is automatically added by Cobra
    
    return rootCmd
}

// shouldSkipConfig returns true for commands that don't need config
func shouldSkipConfig(cmd *cobra.Command) bool {
    skipCommands := []string{"version", "completion", "help"}
    for _, name := range skipCommands {
        if cmd.Name() == name {
            return true
        }
    }
    return false
}

// addGlobalFlags adds flags available on all commands
func addGlobalFlags(cmd *cobra.Command) {
    // Core
    cmd.PersistentFlags().String("host", "", "API host URL")
    cmd.PersistentFlags().String("config", "", "Config file path")
    cmd.PersistentFlags().String("profile", "", "Configuration profile")
    
    // Output
    cmd.PersistentFlags().String("output", "auto", "Output format (auto, json, yaml, table)")
    cmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
    cmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
    
    // Bind to viper
    viper.BindPFlag("host", cmd.PersistentFlags().Lookup("host"))
    viper.BindPFlag("output", cmd.PersistentFlags().Lookup("output"))
    viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
    viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))
}

// initConfig loads configuration
func initConfig(cmd *cobra.Command) error {
    cfgFile := viper.GetString("config")
    
    cfg, err := config.Load(cfgFile)
    if err != nil {
        // Config file not found is OK for some commands
        if _, ok := err.(config.FileNotFoundError); !ok {
            return fmt.Errorf("failed to load config: %w", err)
        }
    }
    
    // Store config in context
    ctx := context.WithValue(cmd.Context(), config.ContextKey, cfg)
    cmd.SetContext(ctx)
    
    return nil
}
```

### Command Registration Pattern

```go
// internal/cli/commands/commands.go
package commands

import "github.com/spf13/cobra"

// CommandFunc is the signature for command constructors
type CommandFunc func() *cobra.Command

// Registry holds all built-in commands
var Registry = []CommandFunc{
    NewVersionCommand,
    NewConfigCommand,
    NewAuthCommand,
    NewInventoryCommand,
    NewDaemonCommand,
    NewCompletionCommand,
}

// RegisterAll adds all commands to the root
func RegisterAll(root *cobra.Command) {
    for _, fn := range Registry {
        root.AddCommand(fn())
    }
}
```

---

## 1. Version Command

### Purpose
Display version information and check for updates.

### Usage

```bash
# Display version
openapi-cli version

# JSON output
openapi-cli version --json

# Check for updates
openapi-cli version --check

# Full version info
openapi-cli version --full
```

### Implementation

```go
// internal/cli/commands/version.go
package commands

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    
    "github.com/openapi-cli/core/internal/version"
)

// NewVersionCommand creates the version command
func NewVersionCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "version",
        Short: "Display version information",
        Long: `Display version information about the openapi-cli binary.

The version command shows the current version, git commit, and build date.
Use --json for machine-readable output or --check to see if updates are available.`,
        Example: `  # Display version
  openapi-cli version

  # JSON output
  openapi-cli version --json

  # Check for updates
  openapi-cli version --check`,
        RunE: runVersion,
    }
    
    // Local flags
    cmd.Flags().Bool("json", false, "Output in JSON format")
    cmd.Flags().Bool("full", false, "Display full version information")
    cmd.Flags().Bool("check", false, "Check for updates")
    
    return cmd
}

// runVersion executes the version command
func runVersion(cmd *cobra.Command, args []string) error {
    jsonOutput, _ := cmd.Flags().GetBool("json")
    fullOutput, _ := cmd.Flags().GetBool("full")
    checkUpdate, _ := cmd.Flags().GetBool("check")
    
    // Build version info
    info := version.FullInfo()
    
    if jsonOutput {
        return outputVersionJSON(cmd, info)
    }
    
    if fullOutput {
        outputVersionFull(info)
    } else {
        outputVersionSimple(info)
    }
    
    if checkUpdate {
        return checkForUpdates(cmd)
    }
    
    return nil
}

// outputVersionJSON outputs version as JSON
func outputVersionJSON(cmd *cobra.Command, info map[string]string) error {
    encoder := json.NewEncoder(cmd.OutOrStdout())
    encoder.SetIndent("", "  ")
    return encoder.Encode(info)
}

// outputVersionSimple outputs simple version string
func outputVersionSimple(info map[string]string) {
    version := info["version"]
    if version == "" || version == "dev" {
        version = "dev"
    }
    
    fmt.Printf("openapi-cli version %s\n", version)
}

// outputVersionFull outputs detailed version information
func outputVersionFull(info map[string]string) {
    bold := color.New(color.Bold).SprintFunc()
    
    fmt.Println()
    fmt.Printf("  %s: %s\n", bold("Version"), info["version"])
    fmt.Printf("  %s: %s\n", bold("Commit"), info["commit"])
    fmt.Printf("  %s: %s\n", bold("Build Date"), info["build_date"])
    fmt.Printf("  %s: %s\n", bold("Branch"), info["branch"])
    fmt.Printf("  %s: %s\n", bold("Go Version"), info["go_version"])
    fmt.Println()
}

// checkForUpdates checks if a newer version is available
func checkForUpdates(cmd *cobra.Command) error {
    fmt.Println()
    fmt.Println("Checking for updates...")
    
    // Get current version
    current := version.FullInfo()["version"]
    if current == "" || current == "dev" {
        fmt.Println("  Development build - skipping update check")
        return nil
    }
    
    // Check GitHub releases API
    latest, err := fetchLatestVersion()
    if err != nil {
        fmt.Printf("  %s Unable to check for updates: %v\n", 
            color.YellowString("⚠"), err)
        return nil
    }
    
    if latest == current {
        fmt.Printf("  %s You are using the latest version (%s)\n",
            color.GreenString("✓"), current)
    } else {
        fmt.Printf("  %s A new version is available: %s (current: %s)\n",
            color.CyanString("→"), latest, current)
        fmt.Println()
        fmt.Printf("  Update with: %s\n", color.YellowString("openapi-cli update"))
    }
    
    return nil
}

// fetchLatestVersion fetches the latest release from GitHub
func fetchLatestVersion() (string, error) {
    client := &http.Client{Timeout: 5 * time.Second}
    
    resp, err := client.Get("https://api.github.com/repos/openapi-cli/core/releases/latest")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
    }
    
    var release struct {
        TagName string `json:"tag_name"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
        return "", err
    }
    
    return release.TagName, nil
}
```

### Version Package

```go
// internal/version/version.go
package version

import (
    "fmt"
    "runtime"
)

// Build-time variables (injected via -ldflags)
var (
    Version   = "dev"
    Commit    = "unknown"
    BuildDate = "unknown"
    Branch    = "unknown"
    GoVersion = runtime.Version()
)

// FullInfo returns all version information
func FullInfo() map[string]string {
    return map[string]string{
        "version":    Version,
        "commit":     Commit,
        "build_date": BuildDate,
        "branch":     Branch,
        "go_version": GoVersion,
    }
}

// String returns the formatted version string
func String() string {
    if Version == "dev" {
        return "dev"
    }
    return fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate)
}
```

---

## 2. Config Command Group

### Purpose
Manage CLI configuration files and settings.

### Commands

| Command | Description |
|---------|-------------|
| `config get` | Read a configuration value |
| `config set` | Write a configuration value |
| `config list` | Display all configuration |
| `config edit` | Open configuration in editor |
| `config validate` | Validate configuration |

### Usage

```bash
# Get a config value
openapi-cli config get host
openapi-cli config get auth.token
openapi-cli config get --profile=prod host

# Set a config value
openapi-cli config set host api.example.com
openapi-cli config set auth.token "Bearer xyz123"
openapi-cli config set --profile=staging debug true

# List all config
openapi-cli config list
openapi-cli config list --json

# Edit config
openapi-cli config edit

# Validate config
openapi-cli config validate
```

### Implementation

```go
// internal/cli/commands/config.go
package commands

import (
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    
    "github.com/openapi-cli/core/internal/core/config"
)

// NewConfigCommand creates the config command group
func NewConfigCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "config",
        Short: "Manage CLI configuration",
        Long: `Manage openapi-cli configuration files and settings.

Configuration is loaded from multiple sources (in order of precedence):
  1. Command-line flags
  2. Environment variables
  3. Project config (./.openapi-cli.yaml)
  4. User config (~/.config/openapi-cli/config.yaml)
  5. System config (/etc/openapi-cli/config.yaml)
  6. Built-in defaults`,
    }
    
    // Add subcommands
    cmd.AddCommand(newConfigGetCommand())
    cmd.AddCommand(newConfigSetCommand())
    cmd.AddCommand(newConfigListCommand())
    cmd.AddCommand(newConfigEditCommand())
    cmd.AddCommand(newConfigValidateCommand())
    
    return cmd
}

// newConfigGetCommand creates the config get subcommand
func newConfigGetCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "get <key>",
        Short: "Get a configuration value",
        Long: `Get the value of a configuration key.

Keys can use dot notation for nested values:
  openapi-cli config get host
  openapi-cli config get auth.token
  openapi-cli config get cache.ttl`,
        Example: `  # Get the host setting
  openapi-cli config get host

  # Get nested value
  openapi-cli config get auth.api_key`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            key := args[0]
            
            value := viper.Get(key)
            if value == nil {
                return fmt.Errorf("key not found: %s", key)
            }
            
            // Format output based on type
            switch v := value.(type) {
            case string:
                fmt.Println(v)
            case bool:
                fmt.Println(v)
            case int:
                fmt.Println(v)
            case float64:
                fmt.Println(v)
            default:
                // Complex types as JSON
                data, _ := json.Marshal(v)
                fmt.Println(string(data))
            }
            
            return nil
        },
    }
}

// newConfigSetCommand creates the config set subcommand
func newConfigSetCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "set <key> <value>",
        Short: "Set a configuration value",
        Long: `Set the value of a configuration key.

The value will be written to the user configuration file.
Keys can use dot notation for nested values.

Values are automatically converted to appropriate types:
  - "true" or "false" → boolean
  - numeric strings → number
  - JSON arrays/objects → parsed as JSON
  - everything else → string`,
        Example: `  # Set simple value
  openapi-cli config set host api.example.com

  # Set boolean
  openapi-cli config set debug true

  # Set duration
  openapi-cli config set timeout 30s

  # Set nested value
  openapi-cli config set auth.api_key "xyz123"`,
        Args: cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            key := args[0]
            valueStr := args[1]
            
            // Determine value type
            value := parseConfigValue(valueStr)
            
            // Get config file path
            cfgPath := viper.GetString("config")
            if cfgPath == "" {
                cfgPath = config.UserConfigPath()
            }
            
            // Load existing config
            cfg := viper.AllSettings()
            
            // Set value using dot notation
            if err := setNestedValue(cfg, key, value); err != nil {
                return fmt.Errorf("failed to set value: %w", err)
            }
            
            // Write config
            if err := config.Write(cfgPath, cfg); err != nil {
                return fmt.Errorf("failed to write config: %w", err)
            }
            
            fmt.Printf("Set %s = %v\n", key, value)
            return nil
        },
    }
    
    cmd.Flags().String("profile", "", "Set value in specific profile")
    
    return cmd
}

// newConfigListCommand creates the config list subcommand
func newConfigListCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "list",
        Short: "List all configuration",
        Long: `Display all configuration values from all sources.

Shows the merged configuration after applying all sources
of configuration (defaults, files, env vars, flags).`,
        Example: `  # List all config
  openapi-cli config list

  # JSON output
  openapi-cli config list --json`,
        RunE: func(cmd *cobra.Command, args []string) error {
            jsonOutput, _ := cmd.Flags().GetBool("json")
            
            if jsonOutput {
                return outputConfigJSON(cmd)
            }
            
            return outputConfigTable(cmd)
        },
    }
}

// newConfigEditCommand creates the config edit subcommand
func newConfigEditCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "edit",
        Short: "Open configuration in editor",
        Long: `Open the user configuration file in the default editor.

Uses the $EDITOR environment variable to determine which
text editor to open. Falls back to common editors (vi, nano, notepad).`,
        Example: `  # Edit config
  openapi-cli config edit

  # With custom editor
  EDITOR=code openapi-cli config edit`,
        RunE: func(cmd *cobra.Command, args []string) error {
            cfgPath := viper.GetString("config")
            if cfgPath == "" {
                cfgPath = config.UserConfigPath()
            }
            
            // Ensure directory exists
            dir := filepath.Dir(cfgPath)
            if err := os.MkdirAll(dir, 0755); err != nil {
                return fmt.Errorf("failed to create config directory: %w", err)
            }
            
            // Get editor
            editor := os.Getenv("EDITOR")
            if editor == "" {
                editor = findDefaultEditor()
            }
            
            if editor == "" {
                return fmt.Errorf("no editor found. Set $EDITOR environment variable")
            }
            
            // Open file
            execCmd := exec.Command(editor, cfgPath)
            execCmd.Stdin = os.Stdin
            execCmd.Stdout = os.Stdout
            execCmd.Stderr = os.Stderr
            
            return execCmd.Run()
        },
    }
}

// newConfigValidateCommand creates the config validate subcommand
func newConfigValidateCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "validate",
        Short: "Validate configuration",
        Long: `Validate the current configuration against the schema.

Checks that all configuration values are valid and that
required settings are present. Reports any errors found.`,
        Example: `  # Validate current config
  openapi-cli config validate`,
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg := viper.AllSettings()
            
            errors := config.Validate(cfg)
            if len(errors) > 0 {
                fmt.Fprintf(cmd.ErrOrStderr(), "Configuration errors:\n")
                for _, err := range errors {
                    fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", err)
                }
                return fmt.Errorf("configuration is invalid")
            }
            
            fmt.Println("Configuration is valid")
            return nil
        },
    }
}

// Helper functions

// parseConfigValue converts a string to the appropriate type
func parseConfigValue(s string) interface{} {
    // Check for boolean
    if s == "true" {
        return true
    }
    if s == "false" {
        return false
    }
    
    // Check for number
    if i, err := fmt.Sscanf(s, "%d", new(int)); err == nil && i == 1 {
        var n int
        fmt.Sscanf(s, "%d", &n)
        return n
    }
    
    // Check for duration
    if d, err := time.ParseDuration(s); err == nil {
        return d.String()
    }
    
    // Try JSON parsing for arrays/objects
    if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") {
        var v interface{}
        if err := json.Unmarshal([]byte(s), &v); err == nil {
            return v
        }
    }
    
    // Return as string
    return s
}

// setNestedValue sets a value in nested map using dot notation
func setNestedValue(m map[string]interface{}, key string, value interface{}) error {
    parts := strings.Split(key, ".")
    
    current := m
    for i, part := range parts {
        if i == len(parts)-1 {
            current[part] = value
            return nil
        }
        
        if _, ok := current[part]; !ok {
            current[part] = make(map[string]interface{})
        }
        
        next, ok := current[part].(map[string]interface{})
        if !ok {
            return fmt.Errorf("cannot set nested value: %s is not a map", part)
        }
        current = next
    }
    
    return nil
}

// outputConfigJSON outputs config as JSON
func outputConfigJSON(cmd *cobra.Command) error {
    encoder := json.NewEncoder(cmd.OutOrStdout())
    encoder.SetIndent("", "  ")
    return encoder.Encode(viper.AllSettings())
}

// outputConfigTable outputs config as formatted table
func outputConfigTable(cmd *cobra.Command) error {
    bold := color.New(color.Bold).SprintFunc()
    
    fmt.Println()
    fmt.Printf("%s\n", bold("Configuration"))
    fmt.Println()
    
    settings := viper.AllSettings()
    printSettings(cmd, settings, "")
    
    // Show config file location
    fmt.Println()
    if viper.ConfigFileUsed() != "" {
        fmt.Printf("Source: %s\n", viper.ConfigFileUsed())
    } else {
        fmt.Println("Source: defaults only (no config file)")
    }
    
    return nil
}

// printSettings recursively prints settings
func printSettings(cmd *cobra.Command, settings map[string]interface{}, prefix string) {
    for key, value := range settings {
        fullKey := prefix + key
        
        switch v := value.(type) {
        case map[string]interface{}:
            printSettings(cmd, v, fullKey+".")
        default:
            fmt.Printf("  %s: %v\n", fullKey, v)
        }
    }
}

// findDefaultEditor finds a default text editor
func findDefaultEditor() string {
    editors := []string{"vi", "vim", "nano", "emacs", "notepad", "code"}
    
    for _, editor := range editors {
        if _, err := exec.LookPath(editor); err == nil {
            return editor
        }
    }
    
    return ""
}
```

### Config Package

```go
// internal/core/config/config.go
package config

import (
    "fmt"
    "os"
    "path/filepath"
)

// ContextKey is the key used to store config in context
type ContextKey string

const ConfigContextKey ContextKey = "config"

// UserConfigPath returns the user config file path
func UserConfigPath() string {
    if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
        return filepath.Join(xdg, "openapi-cli", "config.yaml")
    }
    
    home, _ := os.UserHomeDir()
    if home == "" {
        return "/etc/openapi-cli/config.yaml"
    }
    
    return filepath.Join(home, ".config", "openapi-cli", "config.yaml")
}

// FileNotFoundError indicates config file was not found
type FileNotFoundError struct {
    Path string
}

func (e FileNotFoundError) Error() string {
    return fmt.Sprintf("config file not found: %s", e.Path)
}

// Write writes configuration to file
func Write(path string, cfg map[string]interface{}) error {
    // Ensure directory exists
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    // Convert to YAML
    data, err := yaml.Marshal(cfg)
    if err != nil {
        return err
    }
    
    return os.WriteFile(path, data, 0600)
}

// Validate validates configuration
func Validate(cfg map[string]interface{}) []error {
    var errors []error
    
    // TODO: Implement schema validation
    
    return errors
}
```

---

## 3. Auth Command Group

### Purpose
Manage authentication credentials and profiles.

### Commands

| Command | Description |
|---------|-------------|
| `auth login` | Authenticate with a host |
| `auth logout` | Remove credentials |
| `auth status` | Show authentication status |
| `auth switch` | Switch between profiles |

### Supported Auth Methods

- **OAuth2**: Authorization code flow with PKCE
- **API Key**: Simple token-based authentication
- **mTLS**: Client certificate authentication
- **Basic**: Username/password (discouraged)

### Usage

```bash
# OAuth2 login
openapi-cli auth login --method=oauth --host=api.example.com

# API key login
openapi-cli auth login --method=apikey --token="my-api-key"

# mTLS login
openapi-cli auth login --method=mtls --cert=client.crt --key=client.key

# Logout
openapi-cli auth logout
openapi-cli auth logout --host=api.example.com

# Check status
openapi-cli auth status
openapi-cli auth status --json

# Switch profile
openapi-cli auth switch --profile=production
```

### Implementation

```go
// internal/cli/commands/auth.go
package commands

import (
    "context"
    "fmt"
    "os"
    
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    
    "github.com/openapi-cli/core/internal/core/auth"
    "github.com/openapi-cli/core/internal/core/config"
)

// NewAuthCommand creates the auth command group
func NewAuthCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "auth",
        Short: "Manage authentication",
        Long: `Manage authentication credentials and profiles.

Supports multiple authentication methods:
  - OAuth2: Authorization code flow with PKCE
  - API Key: Simple token-based authentication  
  - mTLS: Client certificate authentication
  - Basic: Username/password (not recommended)`,
    }
    
    cmd.AddCommand(newAuthLoginCommand())
    cmd.AddCommand(newAuthLogoutCommand())
    cmd.AddCommand(newAuthStatusCommand())
    cmd.AddCommand(newAuthSwitchCommand())
    
    return cmd
}

// newAuthLoginCommand creates the auth login subcommand
func newAuthLoginCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "login",
        Short: "Authenticate with a host",
        Long: `Authenticate with an API host using various methods.

OAuth2 Login:
  Opens a browser for OAuth2 authorization code flow.
  Requires --oauth-client-id and optionally --oauth-client-secret.

API Key Login:
  Stores an API key for authentication.
  Use --token or you will be prompted.

mTLS Login:
  Configures client certificate authentication.
  Requires --cert and --key flags.`,
        Example: `  # OAuth2 login
  openapi-cli auth login --method=oauth --oauth-client-id=abc123

  # API key login
  openapi-cli auth login --method=apikey --token="my-api-key"

  # mTLS login
  openapi-cli auth login --method=mtls --cert=client.crt --key=client.key`,
        RunE: runAuthLogin,
    }
    
    // OAuth2 flags
    cmd.Flags().String("method", "oauth", "Auth method (oauth, apikey, mtls, basic)")
    cmd.Flags().String("host", "", "Host to authenticate with")
    cmd.Flags().String("oauth-client-id", "", "OAuth2 client ID")
    cmd.Flags().String("oauth-client-secret", "", "OAuth2 client secret")
    cmd.Flags().String("oauth-scopes", "", "OAuth2 scopes (comma-separated)")
    
    // API Key flags
    cmd.Flags().String("token", "", "API token/key")
    
    // mTLS flags
    cmd.Flags().String("cert", "", "Client certificate file")
    cmd.Flags().String("key", "", "Client key file")
    
    // Basic auth flags
    cmd.Flags().String("username", "", "Username for basic auth")
    cmd.Flags().String("password", "", "Password for basic auth")
    
    return cmd
}

// runAuthLogin executes the login flow
func runAuthLogin(cmd *cobra.Command, args []string) error {
    method, _ := cmd.Flags().GetString("method")
    host, _ := cmd.Flags().GetString("host")
    
    if host == "" {
        host = viper.GetString("host")
    }
    
    if host == "" {
        return fmt.Errorf("--host is required (or set default with: openapi-cli config set host <url>)")
    }
    
    authMgr := auth.NewManager()
    
    switch method {
    case "oauth":
        return runOAuthLogin(cmd, authMgr, host)
    case "apikey":
        return runAPIKeyLogin(cmd, authMgr, host)
    case "mtls":
        return runMTLSLogin(cmd, authMgr, host)
    case "basic":
        return runBasicLogin(cmd, authMgr, host)
    default:
        return fmt.Errorf("unknown auth method: %s", method)
    }
}

// runOAuthLogin handles OAuth2 authentication
func runOAuthLogin(cmd *cobra.Command, authMgr *auth.Manager, host string) error {
    clientID, _ := cmd.Flags().GetString("oauth-client-id")
    clientSecret, _ := cmd.Flags().GetString("oauth-client-secret")
    scopes, _ := cmd.Flags().GetString("oauth-scopes")
    
    if clientID == "" {
        return fmt.Errorf("--oauth-client-id is required for OAuth2")
    }
    
    fmt.Println("Initiating OAuth2 login...")
    fmt.Printf("Client ID: %s\n", clientID)
    fmt.Printf("Scopes: %s\n", scopes)
    fmt.Println()
    
    // Start OAuth2 flow
    creds, err := authMgr.OAuth2Login(context.Background(), auth.OAuth2Config{
        Host:         host,
        ClientID:     clientID,
        ClientSecret: clientSecret,
        Scopes:       parseScopes(scopes),
        OpenBrowser:  true,
    })
    
    if err != nil {
        return fmt.Errorf("OAuth2 login failed: %w", err)
    }
    
    // Store credentials
    if err := authMgr.StoreCredentials(host, creds); err != nil {
        return fmt.Errorf("failed to store credentials: %w", err)
    }
    
    fmt.Println()
    fmt.Println(color.GreenString("✓"), "Successfully authenticated with", host)
    return nil
}

// runAPIKeyLogin handles API key authentication
func runAPIKeyLogin(cmd *cobra.Command, authMgr *auth.Manager, host string) error {
    token, _ := cmd.Flags().GetString("token")
    
    if token == "" {
        fmt.Print("API Key: ")
        fmt.Scanln(&token)
    }
    
    if token == "" {
        return fmt.Errorf("API key is required")
    }
    
    creds := &auth.Credentials{
        Type:  auth.TypeAPIKey,
        Token: token,
    }
    
    if err := authMgr.StoreCredentials(host, creds); err != nil {
        return fmt.Errorf("failed to store credentials: %w", err)
    }
    
    fmt.Println(color.GreenString("✓"), "API key stored for", host)
    return nil
}

// runMTLSLogin handles mTLS authentication
func runMTLSLogin(cmd *cobra.Command, authMgr *auth.Manager, host string) error {
    certFile, _ := cmd.Flags().GetString("cert")
    keyFile, _ := cmd.Flags().GetString("key")
    
    if certFile == "" || keyFile == "" {
        return fmt.Errorf("--cert and --key are required for mTLS")
    }
    
    // Read certificate files
    certPEM, err := os.ReadFile(certFile)
    if err != nil {
        return fmt.Errorf("failed to read certificate: %w", err)
    }
    
    keyPEM, err := os.ReadFile(keyFile)
    if err != nil {
        return fmt.Errorf("failed to read key: %w", err)
    }
    
    creds := &auth.Credentials{
        Type:      auth.TypeMTLS,
        Cert:      string(certPEM),
        Key:       string(keyPEM),
        CertFile:  certFile,
        KeyFile:   keyFile,
    }
    
    if err := authMgr.StoreCredentials(host, creds); err != nil {
        return fmt.Errorf("failed to store credentials: %w", err)
    }
    
    fmt.Println(color.GreenString("✓"), "mTLS credentials stored for", host)
    return nil
}

// runBasicLogin handles basic authentication
func runBasicLogin(cmd *cobra.Command, authMgr *auth.Manager, host string) error {
    username, _ := cmd.Flags().GetString("username")
    password, _ := cmd.Flags().GetString("password")
    
    if username == "" {
        fmt.Print("Username: ")
        fmt.Scanln(&username)
    }
    
    if password == "" {
        fmt.Print("Password: ")
        // In real implementation, use terminal.ReadPassword for hidden input
        fmt.Scanln(&password)
    }
    
    creds := &auth.Credentials{
        Type:     auth.TypeBasic,
        Username: username,
        Password: password,
    }
    
    if err := authMgr.StoreCredentials(host, creds); err != nil {
        return fmt.Errorf("failed to store credentials: %w", err)
    }
    
    fmt.Println(color.GreenString("✓"), "Basic auth credentials stored for", host)
    return nil
}

// newAuthLogoutCommand creates the auth logout subcommand
func newAuthLogoutCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "logout [host]",
        Short: "Remove authentication credentials",
        Long: `Remove stored authentication credentials.

If no host is specified, removes credentials for the
default host. Use --all to remove all stored credentials.`,
        Example: `  # Logout from default host
  openapi-cli auth logout

  # Logout from specific host
  openapi-cli auth logout api.example.com

  # Remove all credentials
  openapi-cli auth logout --all`,
        RunE: func(cmd *cobra.Command, args []string) error {
            all, _ := cmd.Flags().GetBool("all")
            
            authMgr := auth.NewManager()
            
            if all {
                if err := authMgr.ClearAllCredentials(); err != nil {
                    return fmt.Errorf("failed to clear credentials: %w", err)
                }
                fmt.Println(color.GreenString("✓"), "All credentials removed")
                return nil
            }
            
            host := viper.GetString("host")
            if len(args) > 0 {
                host = args[0]
            }
            
            if host == "" {
                return fmt.Errorf("host required (specify as argument or set default)")
            }
            
            if err := authMgr.ClearCredentials(host); err != nil {
                return fmt.Errorf("failed to clear credentials: %w", err)
            }
            
            fmt.Println(color.GreenString("✓"), "Credentials removed for", host)
            return nil
        },
    }
}

// newAuthStatusCommand creates the auth status subcommand
func newAuthStatusCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show authentication status",
        Long: `Display current authentication status for all hosts.

Shows which hosts have stored credentials and the
authentication method used for each.`,
        Example: `  # Show auth status
  openapi-cli auth status

  # JSON output
  openapi-cli auth status --json`,
        RunE: func(cmd *cobra.Command, args []string) error {
            jsonOutput, _ := cmd.Flags().GetBool("json")
            
            authMgr := auth.NewManager()
            status, err := authMgr.GetAllStatus()
            if err != nil {
                return fmt.Errorf("failed to get status: %w", err)
            }
            
            if jsonOutput {
                return outputAuthStatusJSON(cmd, status)
            }
            
            return outputAuthStatusTable(cmd, status)
        },
    }
}

// newAuthSwitchCommand creates the auth switch subcommand
func newAuthSwitchCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "switch --profile=<name>",
        Short: "Switch authentication profile",
        Long: `Switch to a different authentication profile.

Profiles allow you to manage multiple sets of credentials
for different environments (development, staging, production).`,
        Example: `  # Switch to production profile
  openapi-cli auth switch --profile=production

  # Switch to development profile
  openapi-cli auth switch --profile=dev`,
        RunE: func(cmd *cobra.Command, args []string) error {
            profile, _ := cmd.Flags().GetString("profile")
            
            if profile == "" {
                return fmt.Errorf("--profile is required")
            }
            
            // Switch profile in config
            viper.Set("profile", profile)
            
            // Write to config
            cfgPath := config.UserConfigPath()
            cfg := viper.AllSettings()
            if err := config.Write(cfgPath, cfg); err != nil {
                return fmt.Errorf("failed to save profile: %w", err)
            }
            
            fmt.Println(color.GreenString("✓"), "Switched to profile:", profile)
            return nil
        },
    }
}

// Helper functions

func parseScopes(scopes string) []string {
    if scopes == "" {
        return []string{"openid", "profile", "email"}
    }
    return strings.Split(scopes, ",")
}

func outputAuthStatusJSON(cmd *cobra.Command, status []auth.Status) error {
    encoder := json.NewEncoder(cmd.OutOrStdout())
    encoder.SetIndent("", "  ")
    return encoder.Encode(status)
}

func outputAuthStatusTable(cmd *cobra.Command, status []auth.Status) error {
    if len(status) == 0 {
        fmt.Println("No stored credentials")
        return nil
    }
    
    fmt.Println()
    fmt.Println("Authentication Status")
    fmt.Println()
    
    for _, s := range status {
        statusIcon := color.GreenString("✓")
        if !s.Valid {
            statusIcon = color.RedString("✗")
        }
        
        fmt.Printf("  %s %s\n", statusIcon, s.Host)
        fmt.Printf("    Method: %s\n", s.Method)
        if s.ExpiresAt != nil {
            fmt.Printf("    Expires: %s\n", s.ExpiresAt.Format("2006-01-02 15:04:05"))
        }
        fmt.Println()
    }
    
    return nil
}
```

---

## 4. Inventory Command Group

### Purpose
Manage cached OpenAPI inventory files.

### Commands

| Command | Description |
|---------|-------------|
| `inventory fetch` | Download inventory from host |
| `inventory list` | Show cached versions |
| `inventory clear` | Remove from cache |
| `inventory validate` | Check signatures |

### Usage

```bash
# Fetch inventory
openapi-cli inventory fetch
openapi-cli inventory fetch --version=1.2.3
openapi-cli inventory fetch --force

# List cached inventories
openapi-cli inventory list
openapi-cli inventory list --host=api.example.com

# Clear cache
openapi-cli inventory clear
openapi-cli inventory clear --version=1.2.3
openapi-cli inventory clear --all

# Validate signatures
openapi-cli inventory validate
openapi-cli inventory validate --signature=gpg
```

### Implementation

```go
// internal/cli/commands/inventory.go
package commands

import (
    "context"
    "fmt"
    "time"
    
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    
    "github.com/openapi-cli/core/internal/core/inventory"
)

// NewInventoryCommand creates the inventory command group
func NewInventoryCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "inventory",
        Short:   "Manage inventory cache",
        Long:    `Manage cached OpenAPI inventory files and versions.`,
        Aliases: []string{"inv"},
    }
    
    cmd.AddCommand(newInventoryFetchCommand())
    cmd.AddCommand(newInventoryListCommand())
    cmd.AddCommand(newInventoryClearCommand())
    cmd.AddCommand(newInventoryValidateCommand())
    
    return cmd
}

// newInventoryFetchCommand creates the inventory fetch subcommand
func newInventoryFetchCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "fetch",
        Short: "Download inventory from host",
        Long: `Download the inventory file from the configured host.

By default, fetches the latest version. Use --version to fetch
a specific version. The inventory is cached locally for offline use.`,
        Example: `  # Fetch latest inventory
  openapi-cli inventory fetch

  # Fetch specific version
  openapi-cli inventory fetch --version=1.2.3

  # Force refresh (ignore cache)
  openapi-cli inventory fetch --force`,
        RunE: func(cmd *cobra.Command, args []string) error {
            version, _ := cmd.Flags().GetString("version")
            force, _ := cmd.Flags().GetBool("force")
            
            host := viper.GetString("host")
            if host == "" {
                return fmt.Errorf("host not configured (use: openapi-cli config set host <url>)")
            }
            
            fmt.Println("Fetching inventory...")
            fmt.Printf("Host: %s\n", host)
            if version != "" {
                fmt.Printf("Version: %s\n", version)
            }
            fmt.Println()
            
            invMgr := inventory.NewManager()
            
            opts := inventory.FetchOptions{
                Host:    host,
                Version: version,
                Force:   force,
            }
            
            inv, err := invMgr.Fetch(context.Background(), opts)
            if err != nil {
                return fmt.Errorf("failed to fetch inventory: %w", err)
            }
            
            fmt.Println(color.GreenString("✓"), "Inventory fetched successfully")
            fmt.Printf("  Version: %s\n", inv.Version)
            fmt.Printf("  Generated: %s\n", inv.GeneratedAt.Format(time.RFC3339))
            fmt.Printf("  APIs: %d\n", len(inv.Apis))
            fmt.Printf("  Commands: %d\n", len(inv.Commands))
            
            return nil
        },
    }
}

// newInventoryListCommand creates the inventory list subcommand
func newInventoryListCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "list",
        Short: "List cached inventories",
        Long:  `Display all cached inventory versions for the current host.`,
        Example: `  # List all cached inventories
  openapi-cli inventory list

  # List for specific host
  openapi-cli inventory list --host=api.example.com

  # JSON output
  openapi-cli inventory list --json`,
        RunE: func(cmd *cobra.Command, args []string) error {
            jsonOutput, _ := cmd.Flags().GetBool("json")
            host, _ := cmd.Flags().GetString("host")
            
            if host == "" {
                host = viper.GetString("host")
            }
            
            invMgr := inventory.NewManager()
            inventories, err := invMgr.ListCached(host)
            if err != nil {
                return fmt.Errorf("failed to list inventories: %w", err)
            }
            
            if jsonOutput {
                return outputInventoriesJSON(cmd, inventories)
            }
            
            return outputInventoriesTable(cmd, inventories)
        },
    }
}

// newInventoryClearCommand creates the inventory clear subcommand
func newInventoryClearCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "clear",
        Short: "Remove cached inventory",
        Long:  `Remove cached inventory files from local storage.`,
        Example: `  # Clear current host's cache
  openapi-cli inventory clear

  # Clear specific version
  openapi-cli inventory clear --version=1.2.3

  # Clear all hosts
  openapi-cli inventory clear --all`,
        RunE: func(cmd *cobra.Command, args []string) error {
            all, _ := cmd.Flags().GetBool("all")
            version, _ := cmd.Flags().GetString("version")
            
            invMgr := inventory.NewManager()
            
            if all {
                count, err := invMgr.ClearAll()
                if err != nil {
                    return fmt.Errorf("failed to clear cache: %w", err)
                }
                fmt.Printf("Cleared %d cached inventories\n", count)
                return nil
            }
            
            host := viper.GetString("host")
            if host == "" {
                return fmt.Errorf("host not configured")
            }
            
            opts := inventory.ClearOptions{
                Host:    host,
                Version: version,
            }
            
            count, err := invMgr.Clear(opts)
            if err != nil {
                return fmt.Errorf("failed to clear cache: %w", err)
            }
            
            if version != "" {
                fmt.Printf("Cleared version %s for %s\n", version, host)
            } else {
                fmt.Printf("Cleared %d versions for %s\n", count, host)
            }
            
            return nil
        },
    }
}

// newInventoryValidateCommand creates the inventory validate subcommand
func newInventoryValidateCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "validate",
        Short: "Validate inventory signatures",
        Long: `Validate the signatures of cached inventory files.

Checks that the inventory has not been tampered with by
verifying cryptographic signatures (TLS, GPG, or SSH).`,
        Example: `  # Validate current inventory
  openapi-cli inventory validate

  # Validate specific version
  openapi-cli inventory validate --version=1.2.3

  # Show detailed validation info
  openapi-cli inventory validate --verbose`,
        RunE: func(cmd *cobra.Command, args []string) error {
            version, _ := cmd.Flags().GetString("version")
            verbose, _ := cmd.Flags().GetBool("verbose")
            
            host := viper.GetString("host")
            if host == "" {
                return fmt.Errorf("host not configured")
            }
            
            invMgr := inventory.NewManager()
            
            opts := inventory.ValidateOptions{
                Host:    host,
                Version: version,
            }
            
            result, err := invMgr.Validate(context.Background(), opts)
            if err != nil {
                return fmt.Errorf("validation failed: %w", err)
            }
            
            if result.Valid {
                fmt.Println(color.GreenString("✓"), "Inventory signature valid")
            } else {
                fmt.Println(color.RedString("✗"), "Inventory signature INVALID")
                if verbose {
                    for _, err := range result.Errors {
                        fmt.Printf("  - %s\n", err)
                    }
                }
                return fmt.Errorf("signature validation failed")
            }
            
            if verbose {
                fmt.Printf("  Signature Type: %s\n", result.SignatureType)
                fmt.Printf("  Signed By: %s\n", result.SignedBy)
                fmt.Printf("  Signed At: %s\n", result.SignedAt.Format(time.RFC3339))
            }
            
            return nil
        },
    }
}

// Helper functions

func outputInventoriesJSON(cmd *cobra.Command, inventories []inventory.Cached) error {
    encoder := json.NewEncoder(cmd.OutOrStdout())
    encoder.SetIndent("", "  ")
    return encoder.Encode(inventories)
}

func outputInventoriesTable(cmd *cobra.Command, inventories []inventory.Cached) error {
    if len(inventories) == 0 {
        fmt.Println("No cached inventories")
        return nil
    }
    
    fmt.Println()
    fmt.Println("Cached Inventories")
    fmt.Println()
    
    for _, inv := range inventories {
        current := ""
        if inv.IsCurrent {
            current = color.GreenString(" (current)")
        }
        
        fmt.Printf("  %s%s\n", inv.Version, current)
        fmt.Printf("    Host: %s\n", inv.Host)
        fmt.Printf("    Cached: %s\n", inv.CachedAt.Format("2006-01-02 15:04:05"))
        fmt.Printf("    Size: %s\n", humanizeBytes(inv.Size))
        fmt.Println()
    }
    
    return nil
}

func humanizeBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }
    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
```

---

## 5. Daemon Command Group

### Purpose
Manage the background daemon process.

### Commands

| Command | Description |
|---------|-------------|
| `daemon start` | Start the daemon |
| `daemon stop` | Stop the daemon |
| `daemon status` | Check daemon status |
| `daemon logs` | View daemon logs |

### Usage

```bash
# Start daemon
openapi-cli daemon start
openapi-cli daemon start --background

# Stop daemon
openapi-cli daemon stop
openapi-cli daemon stop --force

# Check status
openapi-cli daemon status
openapi-cli daemon status --json

# View logs
openapi-cli daemon logs
openapi-cli daemon logs --follow
openapi-cli daemon logs --tail=100
```

### Implementation

```go
// internal/cli/commands/daemon.go
package commands

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "time"
    
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    
    "github.com/openapi-cli/core/internal/daemon/client"
)

// NewDaemonCommand creates the daemon command group
func NewDaemonCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "daemon",
        Short: "Manage background daemon",
        Long: `Manage the openapi-cli background daemon process.

The daemon runs continuously in the background and provides:
  - Shared inventory caching
  - Connection pooling
  - Request coalescing
  - WebSocket proxy for webhooks

When the daemon is running, CLI commands automatically
connect to it for improved performance.`,
    }
    
    cmd.AddCommand(newDaemonStartCommand())
    cmd.AddCommand(newDaemonStopCommand())
    cmd.AddCommand(newDaemonStatusCommand())
    cmd.AddCommand(newDaemonLogsCommand())
    
    return cmd
}

// newDaemonStartCommand creates the daemon start subcommand
func newDaemonStartCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "start",
        Short: "Start the background daemon",
        Long: `Start the openapi-cli daemon process.

The daemon starts in the background and creates a Unix socket
for communication with CLI clients. It automatically restarts
if it crashes (up to max restart attempts).`,
        Example: `  # Start daemon
  openapi-cli daemon start

  # Start in foreground (for debugging)
  openapi-cli daemon start --foreground

  # Start with verbose logging
  openapi-cli daemon start --verbose`,
        RunE: func(cmd *cobra.Command, args []string) error {
            foreground, _ := cmd.Flags().GetBool("foreground")
            verbose, _ := cmd.Flags().GetBool("verbose")
            
            daemonClient := client.New()
            
            // Check if already running
            if status, err := daemonClient.Status(context.Background()); err == nil && status.Running {
                fmt.Println(color.YellowString("⚠"), "Daemon is already running")
                fmt.Printf("  PID: %d\n", status.PID)
                fmt.Printf("  Uptime: %s\n", time.Since(status.StartedAt))
                return nil
            }
            
            fmt.Println("Starting daemon...")
            
            opts := client.StartOptions{
                Foreground: foreground,
                Verbose:    verbose,
            }
            
            if err := daemonClient.Start(context.Background(), opts); err != nil {
                return fmt.Errorf("failed to start daemon: %w", err)
            }
            
            if foreground {
                // Running in foreground - this blocks
                return nil
            }
            
            // Wait a moment for daemon to start
            time.Sleep(500 * time.Millisecond)
            
            // Verify it started
            status, err := daemonClient.Status(context.Background())
            if err != nil {
                return fmt.Errorf("daemon failed to start: %w", err)
            }
            
            fmt.Println(color.GreenString("✓"), "Daemon started")
            fmt.Printf("  PID: %d\n", status.PID)
            fmt.Printf("  Socket: %s\n", status.SocketPath)
            
            return nil
        },
    }
}

// newDaemonStopCommand creates the daemon stop subcommand
func newDaemonStopCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "stop",
        Short: "Stop the background daemon",
        Long:  `Stop the running openapi-cli daemon process gracefully.`,
        Example: `  # Stop daemon
  openapi-cli daemon stop

  # Force stop (kill immediately)
  openapi-cli daemon stop --force`,
        RunE: func(cmd *cobra.Command, args []string) error {
            force, _ := cmd.Flags().GetBool("force")
            
            daemonClient := client.New()
            
            // Check if running
            status, err := daemonClient.Status(context.Background())
            if err != nil || !status.Running {
                fmt.Println("Daemon is not running")
                return nil
            }
            
            fmt.Println("Stopping daemon...")
            
            if err := daemonClient.Stop(context.Background(), force); err != nil {
                return fmt.Errorf("failed to stop daemon: %w", err)
            }
            
            fmt.Println(color.GreenString("✓"), "Daemon stopped")
            return nil
        },
    }
}

// newDaemonStatusCommand creates the daemon status subcommand
func newDaemonStatusCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show daemon status",
        Long:  `Display the current status of the openapi-cli daemon.`,
        Example: `  # Show status
  openapi-cli daemon status

  # JSON output
  openapi-cli daemon status --json`,
        RunE: func(cmd *cobra.Command, args []string) error {
            jsonOutput, _ := cmd.Flags().GetBool("json")
            
            daemonClient := client.New()
            status, err := daemonClient.Status(context.Background())
            
            if jsonOutput {
                return outputDaemonStatusJSON(cmd, status, err)
            }
            
            return outputDaemonStatusTable(cmd, status, err)
        },
    }
}

// newDaemonLogsCommand creates the daemon logs subcommand
func newDaemonLogsCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "logs",
        Short: "View daemon logs",
        Long:  `Display and follow logs from the daemon process.`,
        Example: `  # Show logs
  openapi-cli daemon logs

  # Follow logs (tail -f)
  openapi-cli daemon logs --follow

  # Show last N lines
  openapi-cli daemon logs --tail=100`,
        RunE: func(cmd *cobra.Command, args []string) error {
            follow, _ := cmd.Flags().GetBool("follow")
            tail, _ := cmd.Flags().GetInt("tail")
            
            logPath := filepath.Join(os.TempDir(), "openapi-cli-daemon.log")
            
            if follow {
                // Tail -f equivalent
                return tailFollow(logPath, cmd.OutOrStdout())
            }
            
            // Read last N lines
            lines, err := readLastLines(logPath, tail)
            if err != nil {
                if os.IsNotExist(err) {
                    fmt.Println("No logs available")
                    return nil
                }
                return fmt.Errorf("failed to read logs: %w", err)
            }
            
            for _, line := range lines {
                fmt.Fprintln(cmd.OutOrStdout(), line)
            }
            
            return nil
        },
    }
}

// Helper functions

func outputDaemonStatusJSON(cmd *cobra.Command, status *client.Status, err error) error {
    result := map[string]interface{}{
        "running": err == nil && status.Running,
    }
    
    if err == nil && status.Running {
        result["pid"] = status.PID
        result["uptime"] = time.Since(status.StartedAt).String()
        result["socket_path"] = status.SocketPath
        result["version"] = status.Version
    }
    
    encoder := json.NewEncoder(cmd.OutOrStdout())
    encoder.SetIndent("", "  ")
    return encoder.Encode(result)
}

func outputDaemonStatusTable(cmd *cobra.Command, status *client.Status, err error) error {
    if err != nil || !status.Running {
        fmt.Println(color.RedString("✗"), "Daemon is not running")
        return nil
    }
    
    fmt.Println()
    fmt.Println("Daemon Status")
    fmt.Println()
    fmt.Printf("  Status: %s\n", color.GreenString("Running"))
    fmt.Printf("  PID: %d\n", status.PID)
    fmt.Printf("  Version: %s\n", status.Version)
    fmt.Printf("  Uptime: %s\n", time.Since(status.StartedAt))
    fmt.Printf("  Socket: %s\n", status.SocketPath)
    fmt.Println()
    
    return nil
}

func tailFollow(path string, w io.Writer) error {
    // Implementation would use fsnotify or similar
    // For simplicity, this is a placeholder
    fmt.Println("Following logs... (Press Ctrl+C to exit)")
    
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Seek to end
    file.Seek(0, io.SeekEnd)
    
    buf := make([]byte, 1024)
    for {
        n, err := file.Read(buf)
        if err != nil {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        
        if n > 0 {
            w.Write(buf[:n])
        }
    }
}

func readLastLines(path string, n int) ([]string, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    lines := strings.Split(string(content), "\n")
    
    if n > 0 && len(lines) > n {
        lines = lines[len(lines)-n:]
    }
    
    return lines, nil
}
```

---

## 6. Completion Command

### Purpose
Generate shell completion scripts.

### Supported Shells

- **Bash**
- **Zsh**
- **Fish**
- **PowerShell**

### Usage

```bash
# Bash
source <(openapi-cli completion bash)
openapi-cli completion bash > /etc/bash_completion.d/openapi-cli

# Zsh
source <(openapi-cli completion zsh)
openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"

# Fish
openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish

# PowerShell
openapi-cli completion powershell | Out-String | Invoke-Expression
```

### Implementation

```go
// internal/cli/commands/completion.go
package commands

import (
    "fmt"
    "os"
    
    "github.com/spf13/cobra"
)

// NewCompletionCommand creates the completion command
func NewCompletionCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "completion [bash|zsh|fish|powershell]",
        Short: "Generate shell completion script",
        Long: `Generate shell completion script for openapi-cli.

To load completions:

Bash:
  $ source <(openapi-cli completion bash)
  # Or for system-wide installation:
  $ openapi-cli completion bash > /etc/bash_completion.d/openapi-cli

Zsh:
  $ source <(openapi-cli completion zsh)
  # Or for system-wide installation:
  $ openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"

Fish:
  $ openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish

PowerShell:
  $ openapi-cli completion powershell | Out-String | Invoke-Expression
  # Or save to file and load in profile:
  $ openapi-cli completion powershell > openapi-cli.ps1`,
        Example: `  # Bash
  openapi-cli completion bash

  # Zsh  
  openapi-cli completion zsh

  # Fish
  openapi-cli completion fish

  # PowerShell
  openapi-cli completion powershell`,
        DisableFlagsInUseLine: true,
        ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
        Args:                  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            shell := args[0]
            
            rootCmd := cmd.Root()
            
            switch shell {
            case "bash":
                return rootCmd.GenBashCompletionV2(os.Stdout, true)
            case "zsh":
                return rootCmd.GenZshCompletion(os.Stdout)
            case "fish":
                return rootCmd.GenFishCompletion(os.Stdout, true)
            case "powershell":
                return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
            default:
                return fmt.Errorf("unsupported shell: %s (valid: bash, zsh, fish, powershell)", shell)
            }
        },
    }
    
    return cmd
}
```

---

## 7. Help Command

### Purpose
Display help information with custom templates and examples.

### Usage

```bash
# Default help
openapi-cli --help
openapi-cli help

# Help for specific command
openapi-cli help config
openapi-cli help config set
openapi-cli auth login --help

# Full help with examples
openapi-cli help --examples
```

### Implementation

```go
// internal/cli/commands/help.go
package commands

import (
    "fmt"
    "strings"
    
    "github.com/fatih/color"
    "github.com/spf13/cobra"
)

// SetupHelp configures custom help templates
func SetupHelp(rootCmd *cobra.Command) {
    // Custom usage template
    rootCmd.SetUsageTemplate(usageTemplate)
    
    // Custom help template
    rootCmd.SetHelpTemplate(helpTemplate)
    
    // Custom help command
    rootCmd.SetHelpCommand(&cobra.Command{
        Use:   "help [command]",
        Short: "Help about any command",
        Long: `Help provides help for any command in the application.
Simply type openapi-cli help [path to command] for full details.`,
        Example: `  # Get help for a command
  openapi-cli help config set

  # Get full help with examples
  openapi-cli help --examples`,
        Run: func(cmd *cobra.Command, args []string) {
            // Cobra handles this automatically
        },
    })
}

// Custom templates
const usageTemplate = `{{color "bold" "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{color "bold" "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{color "bold" "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{color "bold" "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{color "bold" "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{color "bold" "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{color "bold" "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

const helpTemplate = `{{color "bold cyan" .Long}}

{{color "bold" "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{color "bold" "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{color "bold" "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{color "bold" "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{color "bold" "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{color "bold" "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{color "bold" "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// color is a template function for coloring text
func colorFunc(colorName string, text string) string {
    switch colorName {
    case "bold":
        return color.New(color.Bold).Sprint(text)
    case "bold cyan":
        return color.New(color.Bold, color.FgCyan).Sprint(text)
    default:
        return text
    }
}

// SetupTemplateFunctions adds custom functions to cobra templates
func SetupTemplateFunctions(rootCmd *cobra.Command) {
    templateFuncs := map[string]interface{}{
        "color": colorFunc,
    }
    
    rootCmd.SetUsageTemplate(usageTemplate)
    rootCmd.SetHelpTemplate(helpTemplate)
}

// PrintExamples prints usage examples for a command
func PrintExamples(cmd *cobra.Command) {
    if cmd.Example == "" {
        return
    }
    
    fmt.Println()
    fmt.Println(color.New(color.Bold).Sprint("Examples:"))
    
    // Parse and format examples
    lines := strings.Split(cmd.Example, "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        
        // Check if it's a comment
        if strings.HasPrefix(line, "#") {
            fmt.Printf("  %s\n", color.New(color.FgHiBlack).Sprint(line))
        } else {
            fmt.Printf("  $ %s\n", line)
        }
    }
}
```

---

## Error Handling

### Command Error Pattern

All commands follow the RunE pattern for proper error handling:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    // 1. Parse flags
    flag, err := cmd.Flags().GetString("flag")
    if err != nil {
        return fmt.Errorf("failed to parse flag: %w", err)
    }
    
    // 2. Validate arguments
    if err := validateArgs(args); err != nil {
        return err
    }
    
    // 3. Execute logic
    result, err := doSomething()
    if err != nil {
        // Wrap with context
        return fmt.Errorf("operation failed: %w", err)
    }
    
    // 4. Output result
    fmt.Println(result)
    
    return nil
},
```

### Error Types

```go
// internal/cli/commands/errors.go
package commands

import (
    "fmt"
    
    "github.com/openapi-cli/core/internal/core/errors"
)

// CommandError wraps errors with command context
type CommandError struct {
    Command string
    Err     error
}

func (e *CommandError) Error() string {
    return fmt.Sprintf("%s: %v", e.Command, e.Err)
}

func (e *CommandError) Unwrap() error {
    return e.Err
}

// HandleError processes command errors for display
func HandleError(err error) int {
    if err == nil {
        return 0
    }
    
    // Check for specific error types
    var userErr *errors.UserError
    if errors.As(err, &userErr) {
        fmt.Fprintf(os.Stderr, "Error: %s\n", userErr.Message)
        if userErr.Hint != "" {
            fmt.Fprintf(os.Stderr, "\nHint: %s\n", userErr.Hint)
        }
        return userErr.ExitCode
    }
    
    // Default error display
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    return 1
}
```

---

## Testing

### Command Testing Pattern

```go
// internal/cli/commands/version_test.go
package commands

import (
    "bytes"
    "strings"
    "testing"
    
    "github.com/spf13/cobra"
    "github.com/stretchr/testify/assert"
)

func TestVersionCommand(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantText string
        wantErr  bool
    }{
        {
            name:     "simple version",
            args:     []string{},
            wantText: "version",
            wantErr:  false,
        },
        {
            name:     "json output",
            args:     []string{"--json"},
            wantText: "version",
            wantErr:  false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := NewVersionCommand()
            buf := new(bytes.Buffer)
            cmd.SetOut(buf)
            cmd.SetErr(buf)
            cmd.SetArgs(tt.args)
            
            err := cmd.Execute()
            
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Contains(t, buf.String(), tt.wantText)
            }
        })
    }
}

// Integration test with root command
func TestCommandIntegration(t *testing.T) {
    rootCmd := NewRootCommand()
    
    // Add all commands
    rootCmd.AddCommand(NewVersionCommand())
    rootCmd.AddCommand(NewConfigCommand())
    
    // Test version subcommand
    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)
    rootCmd.SetArgs([]string{"version"})
    
    err := rootCmd.Execute()
    assert.NoError(t, err)
    assert.Contains(t, buf.String(), "version")
}
```

---

## Complete File Structure

```
cmd/openapi-cli/
├── main.go                    # CLI entry point
└── commands/
    ├── version.go             # Version command
    ├── config.go              # Config command group
    ├── auth.go                # Auth command group
    ├── inventory.go           # Inventory command group
    ├── daemon.go              # Daemon command group
    ├── completion.go          # Completion command
    ├── help.go                # Help templates
    ├── errors.go              # Error handling
    └── commands.go            # Command registry
```

---

## Summary

### Command Overview

| Command | Type | Subcommands | Status |
|---------|------|-------------|--------|
| `version` | Simple | - | ✅ Complete |
| `config` | Group | get, set, list, edit, validate | ✅ Complete |
| `auth` | Group | login, logout, status, switch | ✅ Complete |
| `inventory` | Group | fetch, list, clear, validate | ✅ Complete |
| `daemon` | Group | start, stop, status, logs | ✅ Complete |
| `completion` | Simple | - | ✅ Complete |
| `help` | Built-in | - | ✅ Complete |

### Key Features

1. **Consistent Structure**: All commands use RunE pattern
2. **Error Handling**: Proper error wrapping and user-friendly messages
3. **Output Formats**: Support for JSON, table, and human-readable output
4. **Help System**: Custom templates with examples
5. **Shell Integration**: Completion scripts for all major shells
6. **Security**: Credentials stored in OS keyring
7. **Testing**: Unit and integration test patterns included

---

**Version**: 1.0  
**Last Updated**: 2026-02-28  
**Total Lines**: ~1,800
