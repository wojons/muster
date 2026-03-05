# CLI Command Structure Specification

## Overview

This document defines the complete command hierarchy for the Muster CLI application. Commands are organized into built-in commands (core functionality) and dynamic commands (generated from OpenAPI specifications).

## Command Tree

```
muster
├── version
├── config
│   ├── get <key>
│   ├── set <key> <value>
│   └── list
├── auth
│   ├── login
│   ├── logout
│   └── status
├── inventory
│   ├── fetch
│   ├── list
│   └── clear
├── daemon
│   ├── start
│   ├── stop
│   └── status
└── <dynamic-commands>  # Generated from OpenAPI spec
    └── <subcommands>
```

## Built-in Commands

### 1. version

**Purpose:** Display CLI version information

**Usage:**
```bash
muster version
muster version --json
muster version --short
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | false | Output in JSON format |
| `--short` | `-s` | false | Print version number only |

**Example Output:**
```
Muster CLI v1.0.0
  Build: 2024-01-15
  Commit: abc1234
  Go Version: go1.21
```

**Cobra Definition:**
```go
var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Long:  `Display detailed version information about the Muster CLI.`,
    RunE:  runVersion,
}
```

---

### 2. config

**Purpose:** Manage CLI configuration settings

#### 2.1 config get

**Usage:**
```bash
muster config get <key>
muster config get api.url
muster config get auth.token --raw
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--raw` | `-r` | false | Output raw value without formatting |

**Error Handling:**
- Exit code 1: Key not found
- Exit code 2: Config file not accessible

**Cobra Definition:**
```go
var configGetCmd = &cobra.Command{
    Use:   "get <key>",
    Short: "Get configuration value",
    Long:  `Retrieve a specific configuration value by key path.`,
    Args:  cobra.ExactArgs(1),
    RunE:  runConfigGet,
    Example: `  muster config get api.url
  muster config get auth.token --raw`,
}
```

#### 2.2 config set

**Usage:**
```bash
muster config set <key> <value>
muster config set api.url https://api.example.com
muster config set --file ~/.muster/custom-config.yaml
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-f` | "" | Load values from file |
| `--global` | `-g` | false | Set in global config (~/.muster/config.yaml) |
| `--local` | `-l` | false | Set in local config (.muster/config.yaml) |

**Validation:**
- URL format validation for `*.url` keys
- JSON validation for `*.json` keys
- Required fields: key, value (unless --file specified)

**Cobra Definition:**
```go
var configSetCmd = &cobra.Command{
    Use:   "set <key> <value>",
    Short: "Set configuration value",
    Long:  `Set a configuration value. Supports dot notation for nested keys.`,
    Args:  cobra.RangeArgs(0, 2),
    RunE:  runConfigSet,
    Example: `  muster config set api.url https://api.example.com
  muster config set auth.timeout 30s
  muster config set --file ./config-values.yaml`,
}
```

#### 2.3 config list

**Usage:**
```bash
muster config list
muster config list --global
muster config list --format json
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | "yaml" | Output format (yaml, json, table) |
| `--global` | `-g` | false | Show global config only |
| `--local` | `-l` | false | Show local config only |
| `--show-source` | `-s` | false | Show source of each value |

**Example Output:**
```yaml
api:
  url: https://api.example.com
  timeout: 30s
auth:
  token: ********
  expires_at: 2024-01-15T10:00:00Z
```

---

### 3. auth

**Purpose:** Authentication management

#### 3.1 auth login

**Usage:**
```bash
muster auth login
muster auth login --token <token>
muster auth login --method oauth
muster auth login --sso
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--token` | `-t` | "" | Provide API token directly |
| `--method` | `-m` | "oauth" | Auth method (oauth, sso, token) |
| `--sso` | | false | Use SSO authentication |
| `--browser` | `-b` | true | Open browser for OAuth flow |
| `--no-browser` | | false | Disable browser opening |

**Interactive Flow:**
1. If --token: Validate and store token
2. If --sso: Initiate SSO flow
3. Default: OAuth device flow

**Cobra Definition:**
```go
var authLoginCmd = &cobra.Command{
    Use:   "login",
    Short: "Authenticate with Muster API",
    Long: `Login to Muster using OAuth, SSO, or API token.
    
By default, opens browser for OAuth authentication.`,
    RunE: runAuthLogin,
    Example: `  muster auth login
  muster auth login --token abc123
  muster auth login --sso`,
}
```

#### 3.2 auth logout

**Usage:**
```bash
muster auth logout
muster auth logout --global
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--global` | `-g` | false | Remove from global config |
| `--all` | `-a` | false | Remove all authentication |

**Cobra Definition:**
```go
var authLogoutCmd = &cobra.Command{
    Use:   "logout",
    Short: "Logout and remove credentials",
    Long:  `Remove stored authentication credentials.`,
    RunE:  runAuthLogout,
}
```

#### 3.3 auth status

**Usage:**
```bash
muster auth status
muster auth status --verbose
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--verbose` | `-v` | false | Show detailed token info |

**Example Output:**
```
✓ Logged in as: user@example.com
  Method: OAuth
  Expires: 2024-01-15 (in 6 hours)
  Scopes: read, write
```

---

### 4. inventory

**Purpose:** Cache and manage API resource inventories

#### 4.1 inventory fetch

**Usage:**
```bash
muster inventory fetch
muster inventory fetch --force
muster inventory fetch --resources pods,services
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--force` | `-f` | false | Force refresh even if cache is valid |
| `--resources` | `-r` | "all" | Comma-separated list of resources |
| `--namespace` | `-n` | "" | Filter by namespace |
| `--timeout` | `-t` | "5m" | Fetch timeout duration |
| `--async` | `-a` | false | Run in background |

**Cobra Definition:**
```go
var inventoryFetchCmd = &cobra.Command{
    Use:   "fetch",
    Short: "Fetch and cache API resources",
    Long: `Fetch resources from the API and cache them locally.
    
Improves performance for auto-completion and offline usage.`,
    RunE: runInventoryFetch,
    Example: `  muster inventory fetch
  muster inventory fetch --force
  muster inventory fetch -r pods,services -n production`,
}
```

#### 4.2 inventory list

**Usage:**
```bash
muster inventory list
muster inventory list --stale
muster inventory list --format table
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | "table" | Output format (table, json, yaml) |
| `--stale` | `-s` | false | Show only stale inventories |
| `--age` | `-a` | "" | Filter by age (e.g., 1h, 1d) |

**Example Output:**
```
RESOURCE    COUNT    LAST FETCHED    STATUS
pods        152      2 hours ago     ✓ fresh
services    43       5 minutes ago   ✓ fresh
endpoints   0        2 days ago      ✗ stale
```

#### 4.3 inventory clear

**Usage:**
```bash
muster inventory clear
muster inventory clear --all
muster inventory clear --resource pods
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--all` | `-a` | false | Clear all inventories |
| `--resource` | `-r` | "" | Clear specific resource |
| `--force` | `-f` | false | Skip confirmation |
| `--older-than` | | "" | Clear items older than duration |

**Confirmation Prompt:**
```
This will clear 3 cached inventories. Continue? [y/N]: 
```

---

### 5. daemon

**Purpose:** Control background daemon process

#### 5.1 daemon start

**Usage:**
```bash
muster daemon start
muster daemon start --foreground
muster daemon start --port 8080
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--foreground` | `-f` | false | Run in foreground (don't detach) |
| `--port` | `-p` | 0 | Listen port (0 = auto) |
| `--log-level` | `-l` | "info" | Log level (debug, info, warn, error) |
| `--socket` | `-s` | "" | Unix socket path |

**Cobra Definition:**
```go
var daemonStartCmd = &cobra.Command{
    Use:   "start",
    Short: "Start the background daemon",
    Long: `Start the Muster daemon for caching and background tasks.
    
The daemon improves performance by maintaining persistent connections
and caching API responses.`,
    RunE: runDaemonStart,
    Example: `  muster daemon start
  muster daemon start --foreground
  muster daemon start -p 8080 --log-level debug`,
}
```

#### 5.2 daemon stop

**Usage:**
```bash
muster daemon stop
muster daemon stop --force
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--force` | `-f` | false | Force kill if graceful shutdown fails |
| `--timeout` | `-t` | "30s" | Shutdown timeout |

#### 5.3 daemon status

**Usage:**
```bash
muster daemon status
muster daemon status --watch
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--watch` | `-w` | false | Watch status continuously |
| `--format` | `-f` | "table" | Output format |

**Example Output:**
```
Status:     ✓ running
PID:        12345
Port:       8080
Uptime:     2h 15m
Memory:     45MB
Connections: 12
```

---

## Dynamic Commands

Dynamic commands are generated at runtime from OpenAPI specifications.

### Generation Process

1. **Discovery**: Load OpenAPI spec from configured source
2. **Parsing**: Extract operations grouped by tags
3. **Registration**: Create cobra commands dynamically
4. **Caching**: Store generated commands for fast startup

### Command Structure

```
muster <tag>
├── <operation-1> [resource] [flags]
├── <operation-2> [resource] [flags]
└── ...
```

### Example: Generated Commands

Given OpenAPI tag "pods":

```
muster pods
├── list [flags]              # GET /api/v1/pods
├── get <name> [flags]        # GET /api/v1/pods/{name}
├── create [flags]             # POST /api/v1/pods
├── update <name> [flags]      # PUT /api/v1/pods/{name}
├── delete <name> [flags]      # DELETE /api/v1/pods/{name}
├── logs <name> [flags]        # GET /api/v1/pods/{name}/logs
└── exec <name> [flags]        # POST /api/v1/pods/{name}/exec
```

### Common Flags for Dynamic Commands

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | "table" | Output format (table, json, yaml, custom) |
| `--namespace` | `-n` | "" | Resource namespace |
| `--selector` | `-l` | "" | Label selector |
| `--all-namespaces` | `-A` | false | All namespaces |
| `--watch` | `-w` | false | Watch for changes |
| `--follow` | `-f` | false | Follow output (for logs) |
| `--tail` | | 100 | Lines to show from end |
| `--since` | | "" | Show logs since duration |
| `--timestamps` | `-t` | false | Include timestamps |
| `--previous` | `-p` | false | Show previous container logs |
| `--dry-run` | | "" | Dry run mode (client, server) |
| `--raw` | `-r` | false | Output raw API response |
| `--template` | | "" | Go template for custom output |
| `--columns` | | "" | Custom columns for table output |

### Auto-Complete Support

Dynamic commands support shell auto-completion:

```go
// Resource name completion
ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // Use cached inventory for completions
    return getResourceNames(cmd.Context(), resourceType, toComplete)
}

// Flag value completion
FlagCompletion: map[string]cobra.CompletionFunc{
    "namespace": completeNamespaces,
    "output":    completeOutputFormats,
}
```

---

## Help Text Templates

### Standard Template

```
{{ .Long }}

Usage:
  {{ .CommandPath }} [flags]
{{ if .HasAvailableSubCommands }}
Available Commands:
{{ range .Commands }}{{ if (or .IsAvailableCommand (eq .Name "help")) }}
  {{ rpad .Name .NamePadding }} {{ .Short }}{{ end }}{{ end }}
{{ end }}
{{ if .HasAvailableLocalFlags }}
Flags:
{{ .LocalFlags.FlagUsages | trimTrailingWhitespaces }}
{{ end }}
{{ if .HasAvailableInheritedFlags }}
Global Flags:
{{ .InheritedFlags.FlagUsages | trimTrailingWhitespaces }}
{{ end }}
{{ if .HasExample }}
Examples:
{{ .Example }}
{{ end }}
{{ if .HasHelpSubCommands }}
Additional help topics:
{{ range .Commands }}{{ if .IsAdditionalHelpTopicCommand }}
  {{ rpad .CommandPath .CommandPathPadding }} {{ .Short }}{{ end }}{{ end }}
{{ end }}
Use "{{ .CommandPath }} --help" for more information about a command.
```

### Example Command Format

Commands should include practical examples:

```go
Example: `  # Basic usage
  muster config get api.url
  
  # With flags
  muster config set --global auth.token abc123
  
  # Complex example
  muster inventory fetch -r pods,services -n production --force`,
```

---

## Error Handling Patterns

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Misuse of command (wrong args/flags) |
| 3 | Config error |
| 4 | Auth error / not logged in |
| 5 | Network error |
| 6 | API error (4xx/5xx) |
| 7 | Resource not found |
| 8 | Permission denied |
| 9 | Timeout |
| 10 | Daemon not running |
| 126 | Command not executable |
| 127 | Command not found |

### Error Output Format

```
Error: <brief description>

Details:
  <detailed error message>
  
Suggestion:
  <helpful suggestion>

Use "muster <command> --help" for more information.
```

### Error Handling Pattern

```go
func runCommand(cmd *cobra.Command, args []string) error {
    // 1. Validate inputs
    if err := validateArgs(args); err != nil {
        return fmt.Errorf("%w\n\nUse \"%s --help\" for usage information", err, cmd.CommandPath())
    }
    
    // 2. Load config
    cfg, err := config.Load()
    if err != nil {
        return &ConfigError{Cause: err}
    }
    
    // 3. Execute with context
    ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
    defer cancel()
    
    if err := execute(ctx, cfg, args); err != nil {
        // Wrap with command-specific error
        return wrapError(err, cmd.Name())
    }
    
    return nil
}
```

---

## Global Flags

These flags are available on all commands:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | false | Show help |
| `--config` | | "" | Config file path |
| `--verbose` | `-v` | false | Enable verbose output |
| `--quiet` | `-q` | false | Suppress non-error output |
| `--no-color` | | false | Disable colored output |
| `--debug` | | false | Enable debug logging |
| `--api-url` | | "" | Override API base URL |
| `--timeout` | `-t` | "30s" | Request timeout |

---

## Implementation Notes

### Cobra Integration

```go
// Root command setup
var rootCmd = &cobra.Command{
    Use:   "muster",
    Short: "Muster CLI - Resource management tool",
    Long: `Muster is a CLI for managing resources through a REST API.
    
Complete documentation is available at https://docs.muster.io`,
    SilenceUsage:  true,
    SilenceErrors: true,
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return initializeConfig(cmd)
    },
}

// Register built-in commands
func init() {
    rootCmd.AddCommand(versionCmd)
    rootCmd.AddCommand(configCmd)
    rootCmd.AddCommand(authCmd)
    rootCmd.AddCommand(inventoryCmd)
    rootCmd.AddCommand(daemonCmd)
}

// Register dynamic commands
func registerDynamicCommands(spec *openapi.Spec) error {
    for _, tag := range spec.Tags {
        cmd := createCommandFromTag(tag)
        rootCmd.AddCommand(cmd)
    }
    return nil
}
```

### Configuration Loading

```go
func initializeConfig(cmd *cobra.Command) error {
    // Load from:
    // 1. Built-in defaults
    // 2. Global config (~/.muster/config.yaml)
    // 3. Local config (.muster/config.yaml)
    // 4. Environment variables (MUSTER_*)
    // 5. Command-line flags
    
    viper.SetConfigName("config")
    viper.AddConfigPath("$HOME/.muster")
    viper.AddConfigPath(".muster")
    
    viper.SetEnvPrefix("MUSTER")
    viper.AutomaticEnv()
    
    return viper.ReadInConfig()
}
```

---

## Appendix: Command Quick Reference

### Built-in Commands

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `version` | Show version info | `--json`, `--short` |
| `config get` | Get config value | `--raw` |
| `config set` | Set config value | `--global`, `--local`, `--file` |
| `config list` | List config | `--format`, `--show-source` |
| `auth login` | Authenticate | `--token`, `--sso`, `--method` |
| `auth logout` | Remove credentials | `--global`, `--all` |
| `auth status` | Check auth status | `--verbose` |
| `inventory fetch` | Cache resources | `--force`, `--resources`, `--namespace` |
| `inventory list` | Show cached inventories | `--format`, `--stale` |
| `inventory clear` | Clear cache | `--all`, `--resource`, `--older-than` |
| `daemon start` | Start daemon | `--foreground`, `--port`, `--log-level` |
| `daemon stop` | Stop daemon | `--force`, `--timeout` |
| `daemon status` | Daemon status | `--watch`, `--format` |

### Dynamic Command Patterns

| Pattern | HTTP Method | Example |
|---------|-------------|---------|
| `list` | GET | `GET /api/v1/pods` |
| `get` | GET | `GET /api/v1/pods/{name}` |
| `create` | POST | `POST /api/v1/pods` |
| `update` | PUT/PATCH | `PUT /api/v1/pods/{name}` |
| `delete` | DELETE | `DELETE /api/v1/pods/{name}` |
| `logs` | GET | `GET /api/v1/pods/{name}/logs` |
| `exec` | POST | `POST /api/v1/pods/{name}/exec` |
| `watch` | GET (stream) | `GET /api/v1/pods?watch=true` |

---

## Version

This specification version: **1.0.0**

Last updated: 2024-01-15
