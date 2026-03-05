# Bash Completion Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: CLI Shell Integration

## Overview

Complete specification for **bash shell completion** in the CLI. Provides intelligent tab completion for commands, flags, arguments, and dynamic values fetched from the API.

## 1. Completion Architecture

```
User presses TAB
    ↓
Shell invokes completion script
    ↓
CLI completion handler
    ↓
Context-aware suggestions
    ↓
Return to shell for display
```

## 2. Core Components

### 2.1 Completion Generator

```go
package completion

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    
    "github.com/spf13/cobra"
)

type Generator struct {
    rootCmd *cobra.Command
    shell   string
}

func NewGenerator(rootCmd *cobra.Command, shell string) *Generator {
    return &Generator{
        rootCmd: rootCmd,
        shell:   shell,
    }
}

func (g *Generator) Generate() (string, error) {
    switch g.shell {
    case "bash":
        return g.generateBash()
    case "zsh":
        return g.generateZsh()
    case "fish":
        return g.generateFish()
    case "powershell":
        return g.generatePowerShell()
    default:
        return "", fmt.Errorf("unsupported shell: %s", g.shell)
    }
}

func (g *Generator) generateBash() (string, error) {
    return g.rootCmd.GenBashCompletion(os.Stdout)
}

func (g *Generator) generateZsh() (string, error) {
    return g.rootCmd.GenZshCompletion(os.Stdout)
}

func (g *Generator) generateFish() (string, error) {
    return g.rootCmd.GenFishCompletion(os.Stdout, true)
}

func (g *Generator) generatePowerShell() (string, error) {
    return g.rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
}
```

### 2.2 Dynamic Completion

```go
type DynamicCompleter struct {
    client APIClient
    cache  *Cache
}

func NewDynamicCompleter(client APIClient) *DynamicCompleter {
    return &DynamicCompleter{
        client: client,
        cache:  NewCache(5 * time.Minute),
    }
}

func (dc *DynamicCompleter) CompleteResources(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // Extract resource type from command context
    resourceType := dc.inferResourceType(cmd)
    
    // Check cache
    cacheKey := fmt.Sprintf("resources:%s", resourceType)
    if cached, ok := dc.cache.Get(cacheKey); ok {
        return dc.filterCompletions(cached.([]string), toComplete), cobra.ShellCompDirectiveNoFileComp
    }
    
    // Fetch from API
    resources, err := dc.client.ListResources(cmd.Context(), resourceType)
    if err != nil {
        return nil, cobra.ShellCompDirectiveError
    }
    
    // Extract names
    var names []string
    for _, r := range resources {
        if name, ok := r["name"].(string); ok {
            names = append(names, name)
        }
    }
    
    // Cache results
    dc.cache.Set(cacheKey, names)
    
    return dc.filterCompletions(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func (dc *DynamicCompleter) CompleteNamespaces(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    cacheKey := "namespaces"
    if cached, ok := dc.cache.Get(cacheKey); ok {
        return dc.filterCompletions(cached.([]string), toComplete), cobra.ShellCompDirectiveNoFileComp
    }
    
    namespaces, err := dc.client.ListNamespaces(cmd.Context())
    if err != nil {
        return nil, cobra.ShellCompDirectiveError
    }
    
    dc.cache.Set(cacheKey, namespaces)
    return dc.filterCompletions(namespaces, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func (dc *DynamicCompleter) CompleteProfiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    profiles, err := dc.loadProfiles()
    if err != nil {
        return nil, cobra.ShellCompDirectiveError
    }
    
    return dc.filterCompletions(profiles, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func (dc *DynamicCompleter) filterCompletions(options []string, prefix string) []string {
    if prefix == "" {
        return options
    }
    
    var filtered []string
    for _, opt := range options {
        if strings.HasPrefix(opt, prefix) {
            filtered = append(filtered, opt)
        }
    }
    return filtered
}
```

## 3. Command Registration with Completions

```go
package cmd

import (
    "github.com/spf13/cobra"
    "muster/internal/completion"
)

func registerCompletions(rootCmd *cobra.Command, completer *completion.DynamicCompleter) {
    // Resource commands with dynamic completion
    resourceCmd := &cobra.Command{
        Use:   "resources",
        Short: "Manage resources",
    }
    
    getCmd := &cobra.Command{
        Use:   "get [name]",
        Short: "Get a resource by name",
        Args:  cobra.MaximumNArgs(1),
        ValidArgsFunction: completer.CompleteResources,
        RunE:  runGet,
    }
    
    deleteCmd := &cobra.Command{
        Use:   "delete [name]",
        Short: "Delete a resource",
        Args:  cobra.ExactArgs(1),
        ValidArgsFunction: completer.CompleteResources,
        RunE:  runDelete,
    }
    
    resourceCmd.AddCommand(getCmd, deleteCmd)
    
    // Add namespace flag with completion
    resourceCmd.PersistentFlags().StringP("namespace", "n", "default", "Target namespace")
    resourceCmd.RegisterFlagCompletionFunc("namespace", completer.CompleteNamespaces)
    
    // Add profile flag with completion
    rootCmd.PersistentFlags().StringP("profile", "p", "", "Configuration profile")
    rootCmd.RegisterFlagCompletionFunc("profile", completer.CompleteProfiles)
    
    rootCmd.AddCommand(resourceCmd)
}
```

## 4. Shell Completion Scripts

### 4.1 Bash Completion Script

```bash
# bash completion for muster                             -*- shell-script -*-

__muster_debug()
{
    if [[ -n ${BASH_COMP_DEBUG_FILE:-} ]]; then
        echo "$*" >> "${BASH_COMP_DEBUG_FILE}"
    fi
}

_muster_completion() {
    local cur prev words cword
    _init_completion || return
    
    local completions
    local result
    
    # Call the CLI completion endpoint
    result=$(muster __complete "$cur" "${words[@]}" 2>/dev/null)
    
    # Parse the result
    local IFS=$'\n'
    completions=($result)
    
    # Check for directives in the last line
    local directive="${completions[${#completions[@]}-1]}"
    unset completions[${#completions[@]}-1]
    
    # Handle directives
    if [[ $directive -eq 1 ]]; then
        # ShellCompDirectiveError
        return 1
    fi
    if [[ $directive -eq 2 ]]; then
        # ShellCompDirectiveNoSpace
        compopt -o nospace
    fi
    if [[ $directive -eq 4 ]]; then
        # ShellCompDirectiveNoFileComp
        compopt +o default
    fi
    
    COMPREPLY=($(compgen -W '${completions[*]}' -- "$cur"))
}

complete -F _muster_completion muster
```

### 4.2 Zsh Completion Script

```zsh
#compdef muster

_muster() {
    local -a completions
    local -a response
    local result
    
    # Call the CLI completion endpoint
    result=$(muster __complete "${words[@]}" 2>/dev/null)
    
    # Parse response
    response=(${(f)result})
    
    # Extract directive
    local directive="${response[-1]}"
    response=(${response[1,-2]})
    
    # Build completions array
    for line in $response; do
        completions+=($line)
    done
    
    # Handle directive
    if [[ $directive -eq 4 ]]; then
        _describe -t commands "muster commands" completions
    else
        _alternative \
            'files:filename:_files' \
            'commands:command:compadd -a completions'
    fi
}

compdef _muster muster
```

### 4.3 Fish Completion Script

```fish
function __muster_complete
    set -l cur (commandline -t)
    set -l words (commandline -opc)
    
    muster __complete $cur $words 2>/dev/null | while read -l line
        echo $line
    end
end

complete -c muster -f -a '(__muster_complete)'
```

## 5. Completion Caching

```go
type Cache struct {
    data   map[string]cacheEntry
    mu     sync.RWMutex
    ttl    time.Duration
}

type cacheEntry struct {
    value     interface{}
    expiresAt time.Time
}

func NewCache(ttl time.Duration) *Cache {
    cache := &Cache{
        data: make(map[string]cacheEntry),
        ttl:  ttl,
    }
    
    // Start cleanup goroutine
    go cache.cleanup()
    
    return cache
}

func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    if entry, ok := c.data[key]; ok {
        if time.Now().Before(entry.expiresAt) {
            return entry.value, true
        }
    }
    return nil, false
}

func (c *Cache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.data[key] = cacheEntry{
        value:     value,
        expiresAt: time.Now().Add(c.ttl),
    }
}

func (c *Cache) cleanup() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        c.mu.Lock()
        now := time.Now()
        for key, entry := range c.data {
            if now.After(entry.expiresAt) {
                delete(c.data, key)
            }
        }
        c.mu.Unlock()
    }
}
```

## 6. CLI Commands

```go
package cmd

var completionCmd = &cobra.Command{
    Use:   "completion [bash|zsh|fish|powershell]",
    Short: "Generate shell completion script",
    Long: `Generate shell completion script for muster.

To load completions:

Bash:
  $ source <(muster completion bash)
  # Or add to ~/.bashrc for persistence

Zsh:
  $ source <(muster completion zsh)
  # Or add to ~/.zshrc for persistence

Fish:
  $ muster completion fish | source
  # Or add to ~/.config/fish/completions/muster.fish

PowerShell:
  PS> muster completion powershell | Out-String | Invoke-Expression
  # Or add to $PROFILE for persistence`,
    Args:      cobra.ExactArgs(1),
    ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
    RunE: func(cmd *cobra.Command, args []string) error {
        shell := args[0]
        generator := completion.NewGenerator(rootCmd, shell)
        script, err := generator.Generate()
        if err != nil {
            return err
        }
        fmt.Println(script)
        return nil
    },
}

func init() {
    rootCmd.AddCommand(completionCmd)
}
```

## 7. Configuration and Installation

```go
func installCompletions(shell string) error {
    var installPath string
    
    switch shell {
    case "bash":
        home, _ := os.UserHomeDir()
        installPath = filepath.Join(home, ".bashrc")
    case "zsh":
        home, _ := os.UserHomeDir()
        installPath = filepath.Join(home, ".zshrc")
    case "fish":
        configDir, _ := os.UserConfigDir()
        installPath = filepath.Join(configDir, "fish", "completions", "muster.fish")
    default:
        return fmt.Errorf("unsupported shell: %s", shell)
    }
    
    generator := completion.NewGenerator(rootCmd, shell)
    script, err := generator.Generate()
    if err != nil {
        return err
    }
    
    if shell == "fish" {
        // Fish uses separate files
        os.MkdirAll(filepath.Dir(installPath), 0755)
        return os.WriteFile(installPath, []byte(script), 0644)
    }
    
    // Bash/Zsh: append to rc file
    f, err := os.OpenFile(installPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()
    
    f.WriteString("\n# muster completion\n")
    f.WriteString(script)
    f.WriteString("\n")
    
    return nil
}
```

## 8. Testing

```go
func TestCompletionGenerator(t *testing.T) {
    rootCmd := &cobra.Command{Use: "muster"}
    rootCmd.AddCommand(&cobra.Command{Use: "get"})
    rootCmd.AddCommand(&cobra.Command{Use: "list"})
    
    t.Run("bash_generation", func(t *testing.T) {
        gen := NewGenerator(rootCmd, "bash")
        script, err := gen.Generate()
        require.NoError(t, err)
        assert.Contains(t, script, "muster")
    })
    
    t.Run("dynamic_completer", func(t *testing.T) {
        client := &mockAPIClient{
            resources: []map[string]interface{}{
                {"name": "pod-1"},
                {"name": "pod-2"},
            },
        }
        
        completer := NewDynamicCompleter(client)
        comps, _ := completer.CompleteResources(nil, []string{}, "pod")
        
        assert.Contains(t, comps, "pod-1")
        assert.Contains(t, comps, "pod-2")
    })
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
