# Unified Commands Specification

## Overview

Commands that work seamlessly across both CLI and MCP interfaces with shared business logic.

## Command Unification Strategy

```
┌─────────────────────────────────────────────────────────┐
│                    Unified Commands                      │
├─────────────────────────────────────────────────────────┤
│  CLI Interface          │  MCP Interface              │
│  ─────────────            │  ─────────────              │
│  cobra commands           │  tool handlers              │
│  flag parsing             │  JSON-RPC params            │
│  stdout output            │  MCP responses              │
└─────────────────────────────────────────────────────────┘
                            │
                    ┌───────▼───────┐
                    │  Command Core  │
                    │  (shared logic)│
                    └───────┬───────┘
                            │
              ┌─────────────┼─────────────┐
              ▼             ▼             ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │Validation│ │Execution │ │  Output  │
        └──────────┘ └──────────┘ └──────────┘
```

## Shared Command Interface

```go
// internal/unified/command.go
package unified

import (
    "context"
    "encoding/json"
    "fmt"
)

// Command represents a unified command executable from CLI or MCP
type Command interface {
    Name() string
    Description() string
    Execute(ctx context.Context, input CommandInput) (*CommandResult, error)
}

// CommandInput contains unified input for any interface
type CommandInput struct {
    Args    map[string]interface{} `json:"args"`
    Flags   map[string]interface{} `json:"flags"`
    Profile string                 `json:"profile,omitempty"`
    Context map[string]interface{} `json:"context,omitempty"`
}

// CommandResult contains unified output for any interface
type CommandResult struct {
    Success bool                   `json:"success"`
    Data    interface{}            `json:"data,omitempty"`
    Error   *CommandError          `json:"error,omitempty"`
    Output  map[string]interface{} `json:"output,omitempty"`
}

// CommandError represents a structured error
type CommandError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

func (e *CommandError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Registry manages all unified commands
type Registry struct {
    commands map[string]Command
}

func NewRegistry() *Registry {
    return &Registry{
        commands: make(map[string]Command),
    }
}

func (r *Registry) Register(cmd Command) {
    r.commands[cmd.Name()] = cmd
}

func (r *Registry) Get(name string) (Command, bool) {
    cmd, ok := r.commands[name]
    return cmd, ok
}

func (r *Registry) List() []Command {
    cmds := make([]Command, 0, len(r.commands))
    for _, cmd := range r.commands {
        cmds = append(cmds, cmd)
    }
    return cmds
}

// Execute runs a command by name
func (r *Registry) Execute(ctx context.Context, name string, input CommandInput) (*CommandResult, error) {
    cmd, ok := r.Get(name)
    if !ok {
        return nil, &CommandError{
            Code:    "COMMAND_NOT_FOUND",
            Message: fmt.Sprintf("command %q not found", name),
        }
    }
    return cmd.Execute(ctx, input)
}
```

## CLI Adapter

```go
// internal/unified/adapter_cli.go
package unified

import (
    "context"
    "github.com/spf13/cobra"
)

// CLIToCommand adapts cobra commands to unified commands
type CLIToCommand struct {
    registry *Registry
}

func NewCLIToCommand(registry *Registry) *CLIToCommand {
    return &CLIToCommand{registry: registry}
}

// ToCobra converts a unified command to cobra command
func (a *CLIToCommand) ToCobra(cmd Command) *cobra.Command {
    return &cobra.Command{
        Use:   cmd.Name(),
        Short: cmd.Description(),
        RunE: func(c *cobra.Command, args []string) error {
            input := a.buildInput(c, args)
            result, err := cmd.Execute(c.Context(), input)
            if err != nil {
                return err
            }
            return a.formatOutput(result)
        },
    }
}

func (a *CLIToCommand) buildInput(c *cobra.Command, args []string) CommandInput {
    // Convert cobra flags and args to unified input
    flags := make(map[string]interface{})
    c.Flags().VisitAll(func(f *pflag.Flag) {
        if f.Changed {
            flags[f.Name] = f.Value.String()
        }
    })
    
    return CommandInput{
        Args:    map[string]interface{}{"args": args},
        Flags:   flags,
        Profile: c.Flags().Lookup("profile").Value.String(),
    }
}

func (a *CLIToCommand) formatOutput(result *CommandResult) error {
    if !result.Success {
        return fmt.Errorf("%s", result.Error.Message)
    }
    // Output formatting based on config
    return nil
}
```

## MCP Adapter

```go
// internal/unified/adapter_mcp.go
package unified

import (
    "context"
    "encoding/json"
    "github.com/mark3labs/mcp-go/mcp"
)

// MCPToCommand adapts MCP tools to unified commands
type MCPToCommand struct {
    registry *Registry
}

func NewMCPToCommand(registry *Registry) *MCPToCommand {
    return &MCPToCommand{registry: registry}
}

// ToMCPTool converts a unified command to MCP tool
func (a *MCPToCommand) ToMCPTool(cmd Command) mcp.Tool {
    return mcp.NewTool(cmd.Name(),
        mcp.WithDescription(cmd.Description()),
        mcp.WithString("profile", mcp.Description("Config profile to use")),
    )
}

// HandleToolCall processes MCP tool calls
func (a *MCPToCommand) HandleToolCall(ctx context.Context, name string, params map[string]interface{}) (*mcp.CallToolResult, error) {
    input := CommandInput{
        Args:    params,
        Flags:   params,
        Context: map[string]interface{}{"source": "mcp"},
    }
    
    result, err := a.registry.Execute(ctx, name, input)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    
    output, _ := json.MarshalIndent(result.Data, "", "  ")
    return mcp.NewToolResultText(string(output)), nil
}
```

## Platform-Specific Adapters

```go
// internal/unified/adapter.go
package unified

// PlatformAdapter interface for platform-specific adaptations
type PlatformAdapter interface {
    AdaptInput(input CommandInput) (CommandInput, error)
    AdaptOutput(result *CommandResult) (interface{}, error)
}

// CLIAdapter handles CLI-specific concerns
type CLIAdapter struct {
    format string // json, yaml, table
}

func (a *CLIAdapter) AdaptInput(input CommandInput) (CommandInput, error) {
    // Parse special CLI flags
    return input, nil
}

func (a *CLIAdapter) AdaptOutput(result *CommandResult) (interface{}, error) {
    if !result.Success {
        return nil, result.Error
    }
    // Format based on output preference
    return result.Data, nil
}

// MCPAdapter handles MCP-specific concerns
type MCPAdapter struct{}

func (a *MCPAdapter) AdaptInput(input CommandInput) (CommandInput, error) {
    // Validate MCP-specific constraints
    return input, nil
}

func (a *MCPAdapter) AdaptOutput(result *CommandResult) (interface{}, error) {
    return result, nil
}
```

## Command Registration

```go
// internal/commands/init.go
package commands

import "github.com/anomalyco/muster/internal/unified"

func RegisterAll(registry *unified.Registry) {
    // Inventory commands
    registry.Register(NewListInventoryCommand())
    registry.Register(NewAddInventoryCommand())
    registry.Register(NewGetInventoryCommand())
    
    // Module commands
    registry.Register(NewDeployModuleCommand())
    registry.Register(NewListModulesCommand())
    
    // Config commands
    registry.Register(NewViewConfigCommand())
    registry.Register(NewSetConfigCommand())
    
    // Auth commands
    registry.Register(NewLoginCommand())
    registry.Register(NewLogoutCommand())
}
```
