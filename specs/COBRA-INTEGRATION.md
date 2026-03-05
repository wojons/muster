# Cobra CLI Integration

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The Cobra CLI Integration specification defines the command-line interface architecture using the Cobra library. Cobra provides a powerful framework for building modern CLI applications with automatic help generation, command grouping, and flag parsing capabilities.

## Command Hierarchy

```
oa (root)
├── completion    - Generate shell completion scripts
├── config        - Configuration management commands
│   ├── get       - Get configuration value
│   ├── set       - Set configuration value
│   └── profiles  - Manage configuration profiles
├── init          - Initialize a new API connection
├── inventory     - Inventory management commands
│   ├── fetch     - Fetch inventory from host
│   ├── list      - List cached inventories
│   └── validate  - Validate inventory schema
├── modules       - WASM module management
│   ├── install   - Install a WASM module
│   ├── list      - List installed modules
│   └── remove    - Remove a WASM module
└── [dynamic]     - Commands generated from inventory
    ├── users
    │   ├── list
    │   ├── get
    │   ├── create
    │   └── delete
    └── orders
        ├── list
        ├── get
        └── create
```

## Technical Implementation

### Command Structure

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

// CommandRegistry manages all CLI commands
type CommandRegistry struct {
    root     *cobra.Command
    commands map[string]*cobra.Command
    viper    *viper.Viper
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry(v *viper.Viper) *CommandRegistry {
    return &CommandRegistry{
        commands: make(map[string]*cobra.Command),
        viper:    v,
    }
}

// Execute runs the root command
func (r *CommandRegistry) Execute() error {
    return r.root.Execute()
}
```

### Root Command Implementation

```go
package cmd

func NewRootCommand(v *viper.Viper) *cobra.Command {
    rootCmd := &cobra.Command{
        Use:   "oa",
        Short: "OpenAPI CLI tool for API interaction",
        Long: `oa - A powerful CLI tool for interacting with OpenAPI-based APIs.

This tool automatically generates CLI commands from OpenAPI specifications
retrieved via inventory endpoints. It supports multiple authentication
methods, caching, and extensible middleware via WASM modules.`,
        Example: `  # Initialize connection to an API
  oa init --host https://api.example.com

  # List all users
  oa users list

  # Get a specific user with verbose output
  oa users get 123 --verbose`,
        Version:               "1.0.0",
        DisableAutoGenTag:   true,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            return initializeCommand(cmd, v)
        },
        SilenceUsage:  true,
        SilenceErrors: false,
    }
    
    // Add global flags
    addGlobalFlags(rootCmd, v)
    
    // Add built-in commands
    rootCmd.AddCommand(NewCompletionCommand())
    rootCmd.AddCommand(NewConfigCommand(v))
    rootCmd.AddCommand(NewInitCommand(v))
    rootCmd.AddCommand(NewInventoryCommand(v))
    rootCmd.AddCommand(NewModulesCommand(v))
    
    return rootCmd
}

func addGlobalFlags(cmd *cobra.Command, v *viper.Viper) {
    // Connection flags
    cmd.PersistentFlags().StringP("host", "H", "", "API host URL (required)")
    cmd.PersistentFlags().IntP("timeout", "t", 30, "Request timeout in seconds")
    cmd.PersistentFlags().IntP("retry", "r", 3, "Number of retry attempts")
    
    // Security flags
    cmd.PersistentFlags().StringP("cert", "c", "", "TLS certificate file")
    cmd.PersistentFlags().StringP("key", "k", "", "TLS key file")
    cmd.PersistentFlags().Bool("insecure", false, "Skip TLS verification (not recommended)")
    
    // Output flags
    cmd.PersistentFlags().StringP("output", "o", "json", "Output format: json, yaml, table")
    cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
    cmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
    
    // Bind to Viper
    v.BindPFlags(cmd.PersistentFlags())
}

func initializeCommand(cmd *cobra.Command, v *viper.Viper) error {
    // Skip initialization for certain commands
    if cmd.Name() == "completion" || cmd.Name() == "help" {
        return nil
    }
    
    // Load configuration
    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return err
        }
    }
    
    // Validate host if provided
    if host := v.GetString("host"); host != "" {
        if err := validateHost(host); err != nil {
            return err
        }
    }
    
    return nil
}
```

### Dynamic Command Generation

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/getkin/kin-openapi/openapi3"
)

// DynamicCommandGenerator generates commands from OpenAPI specs
type DynamicCommandGenerator struct {
    loader     *InventoryLoader
    validator  *InputValidator
    executor   *RequestExecutor
}

// GenerateCommands creates CLI commands from inventory
func (g *DynamicCommandGenerator) GenerateCommands(inv *Inventory) ([]*cobra.Command, error) {
    var commands []*cobra.Command
    
    for _, resource := range inv.Resources {
        cmd := g.createResourceCommand(resource)
        commands = append(commands, cmd)
    }
    
    return commands, nil
}

func (g *DynamicCommandGenerator) createResourceCommand(res Resource) *cobra.Command {
    cmd := &cobra.Command{
        Use:   res.Name,
        Short: res.Description,
        Long:  fmt.Sprintf("Commands for managing %s", res.Name),
    }
    
    // Add subcommands for each operation
    for _, op := range res.Operations {
        subCmd := g.createOperationCommand(res.Name, op)
        cmd.AddCommand(subCmd)
    }
    
    return cmd
}

func (g *DynamicCommandGenerator) createOperationCommand(resourceName string, op Operation) *cobra.Command {
    cmd := &cobra.Command{
        Use:   fmt.Sprintf("%s [args]", op.Name),
        Short: op.Summary,
        Long:  op.Description,
        Args:  g.buildArgsValidator(op),
        RunE: func(cmd *cobra.Command, args []string) error {
            return g.executeOperation(cmd, resourceName, op, args)
        },
    }
    
    // Add flags for parameters
    for _, param := range op.Parameters {
        g.addParameterFlag(cmd, param)
    }
    
    // Add body flag for POST/PUT/PATCH
    if op.RequestBody != nil {
        cmd.Flags().StringP("body", "b", "", "Request body (JSON)")
        cmd.Flags().StringP("file", "f", "", "Request body from file")
    }
    
    return cmd
}

func (g *DynamicCommandGenerator) addParameterFlag(cmd *cobra.Command, param Parameter) {
    flagName := param.Name
    shorthand := ""
    if len(flagName) == 1 {
        shorthand = flagName
    }
    
    switch param.Type {
    case "string":
        cmd.Flags().StringP(flagName, shorthand, param.Default, param.Description)
    case "integer":
        cmd.Flags().IntP(flagName, shorthand, param.DefaultInt, param.Description)
    case "boolean":
        cmd.Flags().BoolP(flagName, shorthand, param.DefaultBool, param.Description)
    case "array":
        cmd.Flags().StringArrayP(flagName, shorthand, nil, param.Description)
    }
    
    if param.Required {
        cmd.MarkFlagRequired(flagName)
    }
}
```

### Input Validation

```go
package cmd

import (
    "github.com/go-playground/validator/v10"
)

// InputValidator validates command inputs
type InputValidator struct {
    validate *validator.Validate
}

func NewInputValidator() *InputValidator {
    v := validator.New()
    
    // Register custom validators
    v.RegisterValidation("semver", validateSemver)
    v.RegisterValidation("hosturl", validateHostURL)
    
    return &InputValidator{validate: v}
}

func (iv *InputValidator) ValidateArgs(cmd *cobra.Command, args []string, rules ValidationRules) error {
    if rules.MinArgs > 0 && len(args) < rules.MinArgs {
        return fmt.Errorf("requires at least %d arg(s), received %d", rules.MinArgs, len(args))
    }
    
    if rules.MaxArgs >= 0 && len(args) > rules.MaxArgs {
        return fmt.Errorf("accepts at most %d arg(s), received %d", rules.MaxArgs, len(args))
    }
    
    if rules.ExactArgs > 0 && len(args) != rules.ExactArgs {
        return fmt.Errorf("requires exactly %d arg(s), received %d", rules.ExactArgs, len(args))
    }
    
    for i, arg := range args {
        if err := iv.validate.Var(arg, rules.ArgValidators[i]); err != nil {
            return fmt.Errorf("invalid argument %d: %w", i+1, err)
        }
    }
    
    return nil
}

func (iv *InputValidator) ValidateFlags(cmd *cobra.Command) error {
    // Validate required flags
    cmd.Flags().VisitAll(func(flag *pflag.Flag) {
        if flag.Annotations["required"] != nil && !flag.Changed {
            return fmt.Errorf("required flag --%s not provided", flag.Name)
        }
    })
    
    return nil
}
```

### Request Execution

```go
package cmd

// RequestExecutor executes HTTP requests for commands
type RequestExecutor struct {
    client  *http.Client
    builder *RequestBuilder
    parser  *ResponseParser
}

func (e *RequestExecutor) Execute(cmd *cobra.Command, resource string, op Operation, args []string) error {
    ctx := cmd.Context()
    
    // Build request
    req, err := e.builder.Build(cmd, resource, op, args)
    if err != nil {
        return fmt.Errorf("failed to build request: %w", err)
    }
    
    // Add context
    req = req.WithContext(ctx)
    
    // Apply middleware
    for _, mw := range e.middleware {
        req = mw.Process(req)
    }
    
    // Execute request
    resp, err := e.client.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Parse and output response
    output, err := e.parser.Parse(resp, cmd.Flag("output").Value.String())
    if err != nil {
        return fmt.Errorf("failed to parse response: %w", err)
    }
    
    fmt.Fprintln(cmd.OutOrStdout(), output)
    return nil
}
```

### Example Command Implementations

```go
package cmd

// InitCommand initializes a new API connection
func NewInitCommand(v *viper.Viper) *cobra.Command {
    return &cobra.Command{
        Use:   "init --host <url>",
        Short: "Initialize connection to an API",
        Long:  "Fetch inventory from the specified host and configure the CLI for use.",
        Example: `  oa init --host https://api.example.com
  oa init --host https://api.example.com --cert /path/to/cert.pem --key /path/to/key.pem`,
        Args: cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            host, _ := cmd.Flags().GetString("host")
            if host == "" {
                return fmt.Errorf("--host is required")
            }
            
            // Fetch inventory
            loader := NewInventoryLoader(host, v)
            inv, err := loader.Fetch()
            if err != nil {
                return fmt.Errorf("failed to fetch inventory: %w", err)
            }
            
            // Save configuration
            v.Set("host", host)
            if err := v.WriteConfig(); err != nil {
                return fmt.Errorf("failed to save configuration: %w", err)
            }
            
            fmt.Fprintf(cmd.OutOrStdout(), "✓ Successfully initialized connection to %s\n", host)
            fmt.Fprintf(cmd.OutOrStdout(), "  API: %s\n", inv.APIName)
            fmt.Fprintf(cmd.OutOrStdout(), "  Version: %s\n", inv.Version)
            fmt.Fprintf(cmd.OutOrStdout(), "  Resources: %d\n", len(inv.Resources))
            
            return nil
        },
    }
}

// ConfigGetCommand gets configuration values
func NewConfigGetCommand(v *viper.Viper) *cobra.Command {
    return &cobra.Command{
        Use:   "get [key]",
        Short: "Get configuration value",
        Long:  "Retrieve a configuration value by key. If no key is specified, all configuration is shown.",
        Example: `  oa config get host
  oa config get --all`,
        Args: cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            showAll, _ := cmd.Flags().GetBool("all")
            
            if showAll || len(args) == 0 {
                // Show all configuration
                settings := v.AllSettings()
                output, _ := json.MarshalIndent(settings, "", "  ")
                fmt.Fprintln(cmd.OutOrStdout(), string(output))
                return nil
            }
            
            // Get specific key
            key := args[0]
            value := v.Get(key)
            if value == nil {
                return fmt.Errorf("configuration key '%s' not found", key)
            }
            
            fmt.Fprintf(cmd.OutOrStdout(), "%s: %v\n", key, value)
            return nil
        },
    }
}
```

### Help Generation

```go
package cmd

// CustomHelpTemplate provides enhanced help output
const CustomHelpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}

{{if .HasAvailableSubCommands}}Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

func init() {
    cobra.AddTemplateFunc("customFunc", customTemplateFunc)
    cobra.EnableCommandSorting = true
}
```

## Error Handling

```go
package cmd

// CLIError represents a CLI-specific error
type CLIError struct {
    Code    int
    Message string
    Details string
}

func (e *CLIError) Error() string {
    if e.Details != "" {
        return fmt.Sprintf("Error (%d): %s\nDetails: %s", e.Code, e.Message, e.Details)
    }
    return fmt.Sprintf("Error (%d): %s", e.Code, e.Message)
}

// Error codes
const (
    ErrCodeInvalidArgs     = 1
    ErrCodeConfigError     = 2
    ErrCodeNetworkError    = 3
    ErrCodeAuthError       = 4
    ErrCodeValidationError = 5
    ErrCodeExecutionError  = 6
)
```

## Usage Examples

```bash
# Initialize a new API connection
oa init --host https://api.example.com

# List all users
oa users list --limit 10

# Get a specific user
oa users get 123

# Create a new user
oa users create --body '{"name": "John", "email": "john@example.com"}'

# Use with verbose output
oa -v users list

# Use specific profile
oa --profile production users list

# Get help for a command
oa users --help
oa users list --help

# Generate shell completion
oa completion bash > /etc/bash_completion.d/oa
```

---

## Bible Standard Compliance

**Genesis 1:1** - *"In the beginning God created the heavens and the earth."*

This specification follows the Bible Standard for software documentation, ensuring clarity, completeness, and divine order in technical communication. All commands shall be documented with precision, honoring the Creator who established order from chaos.

**Compliance Checklist:**
- [x] Status clearly marked as COMPLETE
- [x] Version number specified (1.0.0)
- [x] Last updated date included
- [x] Overview provides clear context
- [x] Technical implementation with code examples
- [x] Command hierarchy documented
- [x] Error handling defined
- [x] Usage examples provided
- [x] Bible Standard footer included

*"Whatever you do, work at it with all your heart, as working for the Lord."* - Colossians 3:23
