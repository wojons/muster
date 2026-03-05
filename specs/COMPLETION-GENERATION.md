# Shell Completion Generation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: CLI

## Overview

Complete specification for **shell completion script generation** supporting Bash, Zsh, Fish, and PowerShell. Completion scripts provide intelligent command-line suggestions for CLI tools, improving user experience and reducing errors.

## 1. Completion System Architecture

### 1.1 Supported Shells

| Shell | Extension | Source Command | Completion Path |
|-------|-----------|----------------|-----------------|
| Bash | `.bash` | `source` | `/etc/bash_completion.d/` or `~/.bash_completion` |
| Zsh | `.zsh` | `source` | `~/.zsh/completions/` or `$fpath` |
| Fish | `.fish` | `source` | `~/.config/fish/completions/` |
| PowerShell | `.ps1` | `Import-Module` | `$PROFILE` directory |

### 1.2 Generation Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   CLI Commands  │     │   Completion     │     │   Shell Script  │
│   & Flags       │────▶│   Generator      │────▶│   Output        │
└─────────────────┘     └──────────────────┘     └─────────────────┘
         │                       │                        │
         ▼                       ▼                        ▼
  - Commands              - Shell detection         - Bash
  - Flags                 - Template selection      - Zsh
  - Arguments             - Script generation       - Fish
  - Dynamic values        - Path resolution         - PowerShell
```

## 2. Completion Generator Core

### 2.1 Generator Interface

```go
package completion

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Shell represents supported shells
type Shell string

const (
	ShellBash       Shell = "bash"
	ShellZsh        Shell = "zsh"
	ShellFish       Shell = "fish"
	ShellPowerShell Shell = "powershell"
)

// Generator creates shell completion scripts
type Generator struct {
	appName    string
	version    string
	commands   []Command
	globalFlags []Flag
	templates  map[Shell]*template.Template
}

// Command represents a CLI command
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Flags       []Flag
	Subcommands []Command
	Args        []Argument
}

// Flag represents a command flag
type Flag struct {
	Long        string
	Short       string
	Description string
	Type        string   // string, bool, int, etc.
	Required    bool
	Default     string
	Values      []string // Valid values for enum flags
}

// Argument represents a positional argument
type Argument struct {
	Name        string
	Description string
	Required    bool
	Multiple    bool
}

// NewGenerator creates a new completion generator
func NewGenerator(appName, version string) *Generator {
	return &Generator{
		appName:   appName,
		version:   version,
		templates: make(map[Shell]*template.Template),
	}
}

// AddCommand adds a command to the completion generator
func (g *Generator) AddCommand(cmd Command) {
	g.commands = append(g.commands, cmd)
}

// AddGlobalFlag adds a global flag available to all commands
func (g *Generator) AddGlobalFlag(flag Flag) {
	g.globalFlags = append(g.globalFlags, flag)
}

// Generate creates completion script for the specified shell
func (g *Generator) Generate(shell Shell) (string, error) {
	tmpl, ok := g.templates[shell]
	if !ok {
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
	
	data := struct {
		AppName     string
		Version     string
		Commands    []Command
		GlobalFlags []Flag
	}{
		AppName:     g.appName,
		Version:     g.version,
		Commands:    g.commands,
		GlobalFlags: g.globalFlags,
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	
	return buf.String(), nil
}

// Supports returns true if the shell is supported
func (g *Generator) Supports(shell Shell) bool {
	_, ok := g.templates[shell]
	return ok
}
```

### 2.2 Shell Detection

```go
package completion

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DetectShell attempts to detect the current shell
func DetectShell() Shell {
	// Check SHELL environment variable
	shellPath := os.Getenv("SHELL")
	if shellPath != "" {
		return shellFromPath(shellPath)
	}
	
	// Check PSModulePath for PowerShell
	if os.Getenv("PSModulePath") != "" {
		return ShellPowerShell
	}
	
	// Windows detection
	if runtime.GOOS == "windows" {
		if os.Getenv("PROMPT") != "" {
			return ShellPowerShell
		}
	}
	
	// Default to bash
	return ShellBash
}

func shellFromPath(path string) Shell {
	base := filepath.Base(path)
	
	switch {
	case strings.Contains(base, "zsh"):
		return ShellZsh
	case strings.Contains(base, "fish"):
		return ShellFish
	case strings.Contains(base, "bash"):
		return ShellBash
	default:
		return ShellBash
	}
}

// GetInstallPath returns the recommended installation path for completion script
func GetInstallPath(shell Shell) string {
	home := os.Getenv("HOME")
	
	switch shell {
	case ShellBash:
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	case ShellZsh:
		return filepath.Join(home, ".zshrc")
	case ShellFish:
		return filepath.Join(home, ".config", "fish", "config.fish")
	case ShellPowerShell:
		if profile := os.Getenv("PROFILE"); profile != "" {
			return profile
		}
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("USERPROFILE"), "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		}
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
	default:
		return ""
	}
}

// GetCompletionDir returns the completion script directory for the shell
func GetCompletionDir(shell Shell) string {
	home := os.Getenv("HOME")
	
	switch shell {
	case ShellBash:
		return "/etc/bash_completion.d/"
	case ShellZsh:
		// Check for fpath
		if fpath := os.Getenv("fpath"); fpath != "" {
			paths := strings.Split(fpath, ":")
			if len(paths) > 0 {
				return paths[0]
			}
		}
		return filepath.Join(home, ".zsh", "completions")
	case ShellFish:
		return filepath.Join(home, ".config", "fish", "completions")
	case ShellPowerShell:
		if profile := os.Getenv("PROFILE"); profile != "" {
			return filepath.Dir(profile)
		}
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("USERPROFILE"), "Documents", "PowerShell")
		}
		return filepath.Join(home, ".config", "powershell")
	default:
		return ""
	}
}
```

## 3. Shell-Specific Templates

### 3.1 Bash Template

```go
package completion

const bashTemplate = `#!/bin/bash
# {{.AppName}} completion script
# Version: {{.Version}}

_{{.AppName}}_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    # Global flags
    local global_flags="{{- range .GlobalFlags}}{{if .Short}} -{{.Short}}{{end}}{{if .Long}} --{{.Long}}{{end}}{{end}}"
    
    # Commands
    local commands="{{- range .Commands}} {{.Name}}{{range .Aliases}} {{.}}{{end}}{{end}}"
    
    # Get the current command context
    local cmd=""
    local i=1
    while [ $i -lt $COMP_CWORD ]; do
        local word="${COMP_WORDS[$i]}"
        {{range .Commands}}
        if [ "$word" == "{{.Name}}" ]{{range .Aliases}} || [ "$word" == "{{.}}" ]{{end}}; then
            cmd="{{.Name}}"
        fi
        {{end}}
        i=$((i + 1))
    done
    
    # Complete based on context
    case "$cmd" in
    {{range .Commands}}
    "{{.Name}}")
        local flags="{{- range .Flags}}{{if .Short}} -{{.Short}}{{end}}{{if .Long}} --{{.Long}}{{end}}{{end}}"
        
        # Check if completing a flag value
        case "$prev" in
        {{range .Flags}}
        {{if .Long}}"--{{.Long}}"{{end}}{{if and .Long .Short}}|{{end}}{{if .Short}}"-{{.Short}}"{{end}})
            {{if .Values}}
            COMPREPLY=( $(compgen -W "{{range .Values}}{{.}} {{end}}" -- "$cur") )
            {{else}}
            COMPREPLY=()
            {{end}}
            return 0
            ;;
        {{end}}
        esac
        
        # Complete flags or arguments
        if [[ "$cur" == -* ]]; then
            COMPREPLY=( $(compgen -W "$flags $global_flags" -- "$cur") )
        fi
        ;;
    {{end}}
    "")
        if [[ "$cur" == -* ]]; then
            COMPREPLY=( $(compgen -W "$global_flags" -- "$cur") )
        else
            COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        fi
        ;;
    esac
}

complete -F _{{.AppName}}_completion {{.AppName}}
`
```

### 3.2 Zsh Template

```go
package completion

const zshTemplate = `#compdef {{.AppName}}
# {{.AppName}} completion script
# Version: {{.Version}}

_{{.AppName}}() {
    local curcontext="$curcontext" state line
    typeset -A opt_args
    
    _arguments -C \
        {{- range .GlobalFlags}}
        '{{if .Short}}-{{.Short}}{{end}}{{if and .Short .Long}}|{{end}}{{if .Long}}--{{.Long}}{{end}}[{{.Description}}]{{if .Values}}: :({{range $i, $v := .Values}}{{if $i}} {{end}}{{$v}}{{end}}){{end}}' \
        {{- end}}
        '1: :->command' \
        '*:: :->args'
    
    case "$state" in
    command)
        local commands=(
            {{- range .Commands}}
            '{{.Name}}:{{.Description}}'
            {{- end}}
        )
        _describe -t commands 'commands' commands
        ;;
    args)
        case "$line[1]" in
        {{- range .Commands}}
        {{.Name}})
            _{{$.AppName}}_{{.Name}}_cmd
            ;;
        {{- end}}
        esac
        ;;
    esac
}

{{range .Commands}}
_{{$.AppName}}_{{.Name}}_cmd() {
    _arguments \
        {{- range .Flags}}
        '{{if .Short}}-{{.Short}}{{end}}{{if and .Short .Long}}|{{end}}{{if .Long}}--{{.Long}}{{end}}[{{.Description}}]{{if .Values}}: :({{range $i, $v := .Values}}{{if $i}} {{end}}{{$v}}{{end}}){{end}}' \
        {{- end}}
        {{- range .Args}}
        '{{if .Multiple}}*:{{else}}:{{end}}{{if .Required}}+{{end}}{{.Name}}:{{.Description}}' \
        {{- end}}
}
{{end}}

compdef _{{.AppName}} {{.AppName}}
`
```

### 3.3 Fish Template

```go
package completion

const fishTemplate = `# {{.AppName}} completion script
# Version: {{.Version}}

# Global flags
{{- range .GlobalFlags}}
complete -c {{$.AppName}} -l {{.Long}}{{if .Short}} -s {{.Short}}{{end}} -d "{{.Description}}"{{if .Values}} -xa "{{range .Values}}{{.}} {{end}}"{{end}}
{{- end}}

# Commands and their completions
{{- range .Commands}}
complete -c {{$.AppName}} -n '__fish_use_subcommand' -a '{{.Name}}' -d '{{.Description}}'{{range .Aliases}} -a '{{.}}'{{end}}

# {{.Name}} flags
{{- range .Flags}}
complete -c {{$.AppName}} -n '__fish_seen_subcommand_from {{.Name}}{{range .Aliases}} {{.}}{{end}}' -l {{.Long}}{{if .Short}} -s {{.Short}}{{end}} -d "{{.Description}}"{{if .Values}} -xa "{{range .Values}}{{.}} {{end}}"{{end}}
{{- end}}
{{- end}}
`
```

### 3.4 PowerShell Template

```go
package completion

const powershellTemplate = `# {{.AppName}} completion script
# Version: {{.Version}}

Register-ArgumentCompleter -Native -CommandName {{.AppName}} -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    
    $commands = @{
        {{- range .Commands}}
        '{{.Name}}' = @{
            description = '{{.Description}}'
            flags = @(
                {{- range .Flags}}
                [PSCustomObject]@{
                    name = '{{if .Long}}--{{.Long}}{{end}}'
                    short = '{{if .Short}}{{.Short}}{{end}}'
                    description = '{{.Description}}'
                    values = @({{if .Values}}{{range .Values}}'{{.}}',{{end}}{{end}})
                }
                {{- end}}
            )
        }
        {{- end}}
    }
    
    $globalFlags = @(
        {{- range .GlobalFlags}}
        [PSCustomObject]@{
            name = '{{if .Long}}--{{.Long}}{{end}}'
            short = '{{if .Short}}{{.Short}}{{end}}'
            description = '{{.Description}}'
        }
        {{- end}}
    )
    
    # Parse command
    $command = ''
    $astTokens = $commandAst.CommandElements | ForEach-Object { $_.Value }
    
    for ($i = 1; $i -lt $astTokens.Count; $i++) {
        $token = $astTokens[$i]
        if ($token -match '^-') {
            continue
        }
        if ($commands.ContainsKey($token)) {
            $command = $token
        }
    }
    
    # Generate completions
    $completions = @()
    
    if ($wordToComplete -match '^--') {
        # Complete long flags
        if ($command -and $commands[$command]) {
            $completions += $commands[$command].flags | ForEach-Object {
                if ($_.name) {
                    [System.Management.Automation.CompletionResult]::new(
                        $_.name,
                        $_.name,
                        'ParameterName',
                        $_.description
                    )
                }
            }
        }
        $completions += $globalFlags | ForEach-Object {
            if ($_.name) {
                [System.Management.Automation.CompletionResult]::new(
                    $_.name,
                    $_.name,
                    'ParameterName',
                    $_.description
                )
            }
        }
    }
    elseif ($wordToComplete -match '^-') {
        # Complete short flags
        if ($command -and $commands[$command]) {
            $completions += $commands[$command].flags | ForEach-Object {
                if ($_.short) {
                    [System.Management.Automation.CompletionResult]::new(
                        "-$($_.short)",
                        "-$($_.short)",
                        'ParameterName',
                        $_.description
                    )
                }
            }
        }
    }
    elseif (-not $command) {
        # Complete commands
        $completions += $commands.GetEnumerator() | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new(
                $_.Key,
                $_.Key,
                'Command',
                $_.Value.description
            )
        }
    }
    
    return $completions
}
`
```

## 4. Template Registration

```go
package completion

import (
	"text/template"
)

// InitTemplates initializes all shell templates
func (g *Generator) InitTemplates() error {
	// Bash template
	bashTmpl, err := template.New("bash").Parse(bashTemplate)
	if err != nil {
		return fmt.Errorf("parse bash template: %w", err)
	}
	g.templates[ShellBash] = bashTmpl
	
	// Zsh template
	zshTmpl, err := template.New("zsh").Parse(zshTemplate)
	if err != nil {
		return fmt.Errorf("parse zsh template: %w", err)
	}
	g.templates[ShellZsh] = zshTmpl
	
	// Fish template
	fishTmpl, err := template.New("fish").Parse(fishTemplate)
	if err != nil {
		return fmt.Errorf("parse fish template: %w", err)
	}
	g.templates[ShellFish] = fishTmpl
	
	// PowerShell template
	psTmpl, err := template.New("powershell").Parse(powershellTemplate)
	if err != nil {
		return fmt.Errorf("parse powershell template: %w", err)
	}
	g.templates[ShellPowerShell] = psTmpl
	
	return nil
}
```

## 5. CLI Integration

### 5.1 Completion Command

```go
package cmd

import (
	"fmt"
	"os"
	
	"github.com/spf13/cobra"
	"github.com/example/cli/pkg/completion"
)

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `Generate shell completion script for the CLI.

Supported shells:
  - bash
  - zsh
  - fish
  - powershell

Examples:
  # Generate bash completion
  openapi-cli completion bash > /etc/bash_completion.d/openapi-cli
  
  # Generate zsh completion
  openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"
  
  # Generate fish completion
  openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish
  
  # Generate PowerShell completion
  openapi-cli completion powershell | Out-String | Invoke-Expression`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := completion.Shell(args[0])
		
		// Create generator
		gen := completion.NewGenerator("openapi-cli", "1.0.0")
		
		// Initialize templates
		if err := gen.InitTemplates(); err != nil {
			return fmt.Errorf("initialize templates: %w", err)
		}
		
		// Check if shell is supported
		if !gen.Supports(shell) {
			return fmt.Errorf("unsupported shell: %s", shell)
		}
		
		// Build command tree from cobra
		buildCommandTree(gen, cmd.Root())
		
		// Generate completion script
		script, err := gen.Generate(shell)
		if err != nil {
			return fmt.Errorf("generate completion: %w", err)
		}
		
		fmt.Print(script)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

// buildCommandTree converts cobra commands to completion generator format
func buildCommandTree(gen *completion.Generator, root *cobra.Command) {
	// Add global flags
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		gen.AddGlobalFlag(completion.Flag{
			Long:        flag.Name,
			Short:       flag.Shorthand,
			Description: flag.Usage,
			Type:        flag.Value.Type(),
			Required:    false,
			Default:     flag.DefValue,
		})
	})
	
	// Add commands
	for _, cmd := range root.Commands() {
		if cmd.Name() == "completion" {
			continue
		}
		
		compCmd := buildCompletionCommand(cmd)
		gen.AddCommand(compCmd)
	}
}

func buildCompletionCommand(cmd *cobra.Command) completion.Command {
	compCmd := completion.Command{
		Name:        cmd.Name(),
		Aliases:     cmd.Aliases,
		Description: cmd.Short,
	}
	
	// Add flags
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		compCmd.Flags = append(compCmd.Flags, completion.Flag{
			Long:        flag.Name,
			Short:       flag.Shorthand,
			Description: flag.Usage,
			Type:        flag.Value.Type(),
			Required:    false,
			Default:     flag.DefValue,
		})
	})
	
	// Add subcommands
	for _, sub := range cmd.Commands() {
		compCmd.Subcommands = append(compCmd.Subcommands, buildCompletionCommand(sub))
	}
	
	return compCmd
}
```

### 5.2 Installation Helper

```go
package completion

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// InstallOptions configures completion installation
type InstallOptions struct {
	Shell      Shell
	OutputPath string
	Stdout     io.Writer
}

// InstallCompletion installs the completion script
func InstallCompletion(gen *Generator, opts InstallOptions) error {
	if opts.OutputPath == "" && opts.Stdout == nil {
		opts.OutputPath = filepath.Join(GetCompletionDir(opts.Shell), gen.appName)
		switch opts.Shell {
		case ShellBash:
			opts.OutputPath += ".bash"
		case ShellZsh:
			opts.OutputPath = "_" + gen.appName
		case ShellFish:
			opts.OutputPath += ".fish"
		case ShellPowerShell:
			opts.OutputPath += ".ps1"
		}
	}
	
	// Generate script
	script, err := gen.Generate(opts.Shell)
	if err != nil {
		return fmt.Errorf("generate completion: %w", err)
	}
	
	// Output
	if opts.Stdout != nil {
		_, err = opts.Stdout.Write([]byte(script))
		return err
	}
	
	// Ensure directory exists
	dir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	
	// Write file
	if err := os.WriteFile(opts.OutputPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	
	fmt.Fprintf(os.Stderr, "Completion script installed to: %s\n", opts.OutputPath)
	fmt.Fprintf(os.Stderr, "\nTo enable completions, restart your shell or run:\n")
	
	switch opts.Shell {
	case ShellBash:
		fmt.Fprintf(os.Stderr, "  source %s\n", opts.OutputPath)
	case ShellZsh:
		fmt.Fprintf(os.Stderr, "  source %s\n", opts.OutputPath)
	case ShellFish:
		fmt.Fprintf(os.Stderr, "  source %s\n", opts.OutputPath)
	case ShellPowerShell:
		fmt.Fprintf(os.Stderr, "  . %s\n", opts.OutputPath)
	}
	
	return nil
}
```

## 6. Testing

```go
package completion_test

import (
	"strings"
	"testing"
	
	"github.com/example/cli/pkg/completion"
)

func TestGenerator(t *testing.T) {
	gen := completion.NewGenerator("test-app", "1.0.0")
	
	// Add commands
	gen.AddCommand(completion.Command{
		Name:        "get",
		Description: "Get a resource",
		Flags: []completion.Flag{
			{Long: "format", Short: "f", Description: "Output format"},
		},
	})
	
	gen.AddCommand(completion.Command{
		Name:        "list",
		Description: "List resources",
		Flags: []completion.Flag{
			{Long: "limit", Short: "l", Description: "Limit results"},
		},
	})
	
	// Initialize templates
	if err := gen.InitTemplates(); err != nil {
		t.Fatalf("InitTemplates failed: %v", err)
	}
	
	// Test each shell
	shells := []completion.Shell{
		completion.ShellBash,
		completion.ShellZsh,
		completion.ShellFish,
		completion.ShellPowerShell,
	}
	
	for _, shell := range shells {
		t.Run(string(shell), func(t *testing.T) {
			script, err := gen.Generate(shell)
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			
			if script == "" {
				t.Error("Generated script is empty")
			}
			
			// Check for app name
			if !strings.Contains(script, "test-app") {
				t.Error("Script doesn't contain app name")
			}
			
			// Check for commands
			if !strings.Contains(script, "get") {
				t.Error("Script doesn't contain 'get' command")
			}
			if !strings.Contains(script, "list") {
				t.Error("Script doesn't contain 'list' command")
			}
		})
	}
}

func TestShellDetection(t *testing.T) {
	// This test depends on environment
	shell := completion.DetectShell()
	
	validShells := []completion.Shell{
		completion.ShellBash,
		completion.ShellZsh,
		completion.ShellFish,
		completion.ShellPowerShell,
	}
	
	found := false
	for _, valid := range validShells {
		if shell == valid {
			found = true
			break
		}
	}
	
	if !found {
		t.Errorf("Detected shell %q is not valid", shell)
	}
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC