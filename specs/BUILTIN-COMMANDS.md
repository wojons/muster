# Built-in Commands Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: CLI Core

## Overview

Complete specification for **built-in CLI commands** that provide core functionality independent of API specifications. These commands handle configuration, authentication, and utility operations.

## 1. Command Architecture

```
muster
├── config          # Configuration management
│   ├── get         # Get configuration value
│   ├── set         # Set configuration value
│   ├── profiles    # List profiles
│   └── use         # Switch active profile
├── auth            # Authentication
│   ├── login       # Authenticate with server
│   ├── logout      # Clear authentication
│   └── status      # Check auth status
├── completion      # Shell completion scripts
├── version         # Display version
└── docs            # Documentation commands
    └── generate    # Generate API docs
```

## 2. Configuration Commands

```go
package cmd

import (
    "fmt"
    "strings"
    
    "github.com/spf13/cobra"
    "muster/internal/config"
)

var configCmd = &cobra.Command{
    Use:   "config",
    Short: "Manage configuration",
    Long:  "View and modify muster configuration settings.",
}

var configGetCmd = &cobra.Command{
    Use:   "get [key]",
    Short: "Get configuration value",
    Args:  cobra.ExactArgs(1),
    Example: `  # Get current profile
  muster config get default_profile
  
  # Get profile base URL
  muster config get profiles.production.base_url`,
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, err := config.Load()
        if err != nil {
            return fmt.Errorf("load config: %w", err)
        }
        
        key := args[0]
        value, err := cfg.Get(key)
        if err != nil {
            return fmt.Errorf("get %s: %w", key, err)
        }
        
        fmt.Println(value)
        return nil
    },
}

var configSetCmd = &cobra.Command{
    Use:   "set [key] [value]",
    Short: "Set configuration value",
    Args:  cobra.ExactArgs(2),
    Example: `  # Set default profile
  muster config set default_profile production
  
  # Set profile base URL
  muster config set profiles.staging.base_url https://staging.example.com`,
    RunE: func(cmd *cobra.Command, args []string) error {
        key := args[0]
        value := args[1]
        
        cfg, err := config.Load()
        if err != nil {
            return fmt.Errorf("load config: %w", err)
        }
        
        if err := cfg.Set(key, value); err != nil {
            return fmt.Errorf("set %s: %w", key, err)
        }
        
        if err := config.Save(cfg); err != nil {
            return fmt.Errorf("save config: %w", err)
        }
        
        fmt.Printf("Set %s = %s\n", key, value)
        return nil
    },
}

var configProfilesCmd = &cobra.Command{
    Use:   "profiles",
    Short: "List configuration profiles",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, err := config.Load()
        if err != nil {
            return fmt.Errorf("load config: %w", err)
        }
        
        current := cfg.DefaultProfile
        
        fmt.Println("Profiles:")
        for name := range cfg.Profiles {
            marker := " "
            if name == current {
                marker = "*"
            }
            fmt.Printf("  %s %s\n", marker, name)
        }
        
        return nil
    },
}

var configUseCmd = &cobra.Command{
    Use:   "use [profile]",
    Short: "Switch to a different profile",
    Args:  cobra.ExactArgs(1),
    Example: `  muster config use production`,
    RunE: func(cmd *cobra.Command, args []string) error {
        profileName := args[0]
        
        cfg, err := config.Load()
        if err != nil {
            return fmt.Errorf("load config: %w", err)
        }
        
        if _, ok := cfg.Profiles[profileName]; !ok {
            return fmt.Errorf("profile '%s' not found", profileName)
        }
        
        cfg.DefaultProfile = profileName
        
        if err := config.Save(cfg); err != nil {
            return fmt.Errorf("save config: %w", err)
        }
        
        fmt.Printf("Now using profile: %s\n", profileName)
        return nil
    },
}

func init() {
    configCmd.AddCommand(configGetCmd, configSetCmd, configProfilesCmd, configUseCmd)
    rootCmd.AddCommand(configCmd)
}
```

## 3. Authentication Commands

```go
package cmd

import (
    "context"
    "fmt"
    
    "github.com/spf13/cobra"
    "muster/internal/auth"
)

var authCmd = &cobra.Command{
    Use:   "auth",
    Short: "Manage authentication",
    Long:  "Login, logout, and check authentication status.",
}

var authLoginCmd = &cobra.Command{
    Use:   "login",
    Short: "Authenticate with the server",
    Long: `Authenticate with the API server using various methods.

Authentication methods:
  - token: API token authentication
  - oauth: OAuth2 flow (interactive)
  - basic: Username/password (not recommended)`,
    Example: `  # Login with token
  muster auth login --method token --token abc123
  
  # Login with OAuth
  muster auth login --method oauth`,
    RunE: func(cmd *cobra.Command, args []string) error {
        method, _ := cmd.Flags().GetString("method")
        
        switch method {
        case "token":
            return loginWithToken(cmd)
        case "oauth":
            return loginWithOAuth(cmd)
        case "basic":
            return loginWithBasic(cmd)
        default:
            return fmt.Errorf("unknown auth method: %s", method)
        }
    },
}

func loginWithToken(cmd *cobra.Command) error {
    token, _ := cmd.Flags().GetString("token")
    if token == "" {
        fmt.Print("Enter API token: ")
        fmt.Scanln(&token)
    }
    
    client := auth.NewTokenClient()
    if err := client.Validate(cmd.Context(), token); err != nil {
        return fmt.Errorf("invalid token: %w", err)
    }
    
    if err := auth.StoreToken(token); err != nil {
        return fmt.Errorf("store token: %w", err)
    }
    
    fmt.Println("Successfully authenticated with token")
    return nil
}

func loginWithOAuth(cmd *cobra.Command) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    
    profile := cfg.GetCurrentProfile()
    oauth := auth.NewOAuth2Client(profile.OAuthConfig)
    
    token, err := oauth.Authenticate(cmd.Context())
    if err != nil {
        return fmt.Errorf("oauth authentication failed: %w", err)
    }
    
    if err := auth.StoreOAuthToken(token); err != nil {
        return fmt.Errorf("store token: %w", err)
    }
    
    fmt.Println("Successfully authenticated with OAuth")
    return nil
}

var authLogoutCmd = &cobra.Command{
    Use:   "logout",
    Short: "Clear authentication credentials",
    RunE: func(cmd *cobra.Command, args []string) error {
        if err := auth.ClearCredentials(); err != nil {
            return fmt.Errorf("clear credentials: %w", err)
        }
        
        fmt.Println("Successfully logged out")
        return nil
    },
}

var authStatusCmd = &cobra.Command{
    Use:   "status",
    Short: "Check authentication status",
    RunE: func(cmd *cobra.Command, args []string) error {
        status, err := auth.GetStatus(cmd.Context())
        if err != nil {
            return fmt.Errorf("check status: %w", err)
        }
        
        if status.Authenticated {
            fmt.Printf("Authenticated: Yes\n")
            fmt.Printf("Method: %s\n", status.Method)
            fmt.Printf("Expires: %s\n", status.ExpiresAt.Format("2006-01-02 15:04:05"))
        } else {
            fmt.Println("Authenticated: No")
        }
        
        return nil
    },
}

func init() {
    authLoginCmd.Flags().StringP("method", "m", "token", "Authentication method (token|oauth|basic)")
    authLoginCmd.Flags().StringP("token", "t", "", "API token (for token method)")
    
    authCmd.AddCommand(authLoginCmd, authLogoutCmd, authStatusCmd)
    rootCmd.AddCommand(authCmd)
}
```

## 4. Version Command

```go
package cmd

import (
    "fmt"
    "runtime"
    
    "github.com/spf13/cobra"
)

var (
    Version   = "dev"
    Commit    = "unknown"
    BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Display version information",
    Long:  "Display detailed version information including build details.",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("muster version %s\n", Version)
        fmt.Printf("  Commit: %s\n", Commit)
        fmt.Printf("  Build Date: %s\n", BuildDate)
        fmt.Printf("  Go Version: %s\n", runtime.Version())
        fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
    },
}

func init() {
    rootCmd.AddCommand(versionCmd)
}
```

## 5. Documentation Commands

```go
package cmd

import (
    "fmt"
    
    "github.com/spf13/cobra"
    "muster/internal/docs"
)

var docsCmd = &cobra.Command{
    Use:   "docs",
    Short: "Generate and view documentation",
    Long:  "Generate API documentation from OpenAPI specifications.",
}

var docsGenerateCmd = &cobra.Command{
    Use:   "generate [spec-file]",
    Short: "Generate API documentation",
    Args:  cobra.ExactArgs(1),
    Example: `  # Generate Markdown docs
  muster docs generate ./openapi.yaml --output ./docs
  
  # Generate HTML docs
  muster docs generate ./openapi.yaml --format html`,
    RunE: func(cmd *cobra.Command, args []string) error {
        specFile := args[0]
        outputDir, _ := cmd.Flags().GetString("output")
        format, _ := cmd.Flags().GetString("format")
        
        generator, err := docs.NewGenerator(specFile, outputDir)
        if err != nil {
            return fmt.Errorf("create generator: %w", err)
        }
        
        switch format {
        case "markdown", "md":
            if err := generator.GenerateMarkdown(); err != nil {
                return fmt.Errorf("generate markdown: %w", err)
            }
        case "html":
            if err := generator.GenerateHTML(); err != nil {
                return fmt.Errorf("generate html: %w", err)
            }
        default:
            return fmt.Errorf("unsupported format: %s", format)
        }
        
        fmt.Printf("Documentation generated in %s\n", outputDir)
        return nil
    },
}

var docsServeCmd = &cobra.Command{
    Use:   "serve [spec-file]",
    Short: "Serve documentation with live reload",
    Args:  cobra.ExactArgs(1),
    Example: `  muster docs serve ./openapi.yaml --port 8080`,
    RunE: func(cmd *cobra.Command, args []string) error {
        specFile := args[0]
        port, _ := cmd.Flags().GetInt("port")
        
        server := docs.NewServer(specFile, port)
        fmt.Printf("Serving documentation at http://localhost:%d\n", port)
        return server.Start()
    },
}

func init() {
    docsGenerateCmd.Flags().StringP("output", "o", "./docs", "Output directory")
    docsGenerateCmd.Flags().StringP("format", "f", "markdown", "Output format (markdown|html)")
    
    docsServeCmd.Flags().IntP("port", "p", 8080, "Server port")
    
    docsCmd.AddCommand(docsGenerateCmd, docsServeCmd)
    rootCmd.AddCommand(docsCmd)
}
```

## 6. Root Command Setup

```go
package cmd

import (
    "os"
    
    "github.com/spf13/cobra"
    "muster/internal/config"
)

var rootCmd = &cobra.Command{
    Use:   "muster",
    Short: "CLI tool for OpenAPI-driven API interaction",
    Long: `muster is a CLI tool that generates commands from OpenAPI specifications,
enabling type-safe, self-documenting API interaction.`,
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Skip for built-in commands
        if isBuiltInCommand(cmd) {
            return nil
        }
        
        // Load configuration
        cfg, err := config.Load()
        if err != nil {
            return fmt.Errorf("load config: %w", err)
        }
        
        // Apply profile settings
        profile, _ := cmd.Flags().GetString("profile")
        if profile != "" {
            cfg.SetProfile(profile)
        }
        
        return nil
    },
}

func isBuiltInCommand(cmd *cobra.Command) bool {
    builtIn := []string{"config", "auth", "completion", "version", "docs", "help"}
    for _, name := range builtIn {
        if cmd.Name() == name || cmd.HasParent() && cmd.Parent().Name() == name {
            return true
        }
    }
    return false
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func init() {
    // Global flags
    rootCmd.PersistentFlags().StringP("profile", "p", "", "Configuration profile")
    rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format (table|json|yaml)")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
```

## 7. Testing

```go
package cmd_test

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "muster/cmd"
)

func TestBuiltInCommands(t *testing.T) {
    t.Run("version", func(t *testing.T) {
        output, err := executeCommand("version")
        require.NoError(t, err)
        assert.Contains(t, output, "muster version")
    })
    
    t.Run("config_get", func(t *testing.T) {
        // Setup test config
        setupTestConfig(t)
        
        output, err := executeCommand("config", "get", "default_profile")
        require.NoError(t, err)
        assert.Equal(t, "default\n", output)
    })
    
    t.Run("config_profiles", func(t *testing.T) {
        output, err := executeCommand("config", "profiles")
        require.NoError(t, err)
        assert.Contains(t, output, "Profiles:")
    })
}

func executeCommand(args ...string) (string, error) {
    buf := new(bytes.Buffer)
    rootCmd := cmd.NewRootCommand()
    rootCmd.SetOut(buf)
    rootCmd.SetErr(buf)
    rootCmd.SetArgs(args)
    
    err := rootCmd.Execute()
    return buf.String(), err
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
