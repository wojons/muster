# CLI Alias System Specification

## Overview

The alias system allows users to define custom command shortcuts and abbreviations.

## Architecture

```
internal/alias/
├── manager.go       # Alias storage and expansion
├── expansion.go     # Command expansion logic
└── storage.go      # Persistence layer
```

## Alias Manager

```go
package alias

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manager handles alias operations
type Manager struct {
	mu      sync.RWMutex
	aliases map[string]string
	file    string
}

// NewManager creates a new alias manager
func NewManager(configDir string) (*Manager, error) {
	file := filepath.Join(configDir, "aliases.yaml")
	
	m := &Manager{
		aliases: make(map[string]string),
		file:    file,
	}
	
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load aliases: %w", err)
	}
	
	return m, nil
}

// Define creates or updates an alias
func (m *Manager) Define(name, command string) error {
	if err := validateAliasName(name); err != nil {
		return err
	}
	
	m.mu.Lock()
	m.aliases[name] = command
	m.mu.Unlock()
	
	return m.save()
}

// Remove deletes an alias
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.aliases[name]; !exists {
		return fmt.Errorf("alias '%s' not found", name)
	}
	
	delete(m.aliases, name)
	return m.save()
}

// Get retrieves an alias definition
func (m *Manager) Get(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	cmd, exists := m.aliases[name]
	return cmd, exists
}

// List returns all aliases
func (m *Manager) List() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]string, len(m.aliases))
	for k, v := range m.aliases {
		result[k] = v
	}
	return result
}

// Expand replaces alias with full command
func (m *Manager) Expand(args []string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}
	
	alias := args[0]
	command, exists := m.Get(alias)
	if !exists {
		return args, false
	}
	
	// Split the aliased command
	expanded := strings.Fields(command)
	
	// Append remaining args
	if len(args) > 1 {
		expanded = append(expanded, args[1:]...)
	}
	
	return expanded, true
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.file)
	if err != nil {
		return err
	}
	
	return yaml.Unmarshal(data, &m.aliases)
}

func (m *Manager) save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	data, err := yaml.Marshal(m.aliases)
	if err != nil {
		return err
	}
	
	// Ensure directory exists
	dir := filepath.Dir(m.file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	return os.WriteFile(m.file, data, 0644)
}

func validateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}
	
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("alias name cannot contain whitespace")
	}
	
	// Reserved names
	reserved := []string{"alias", "unalias", "help", "version"}
	for _, r := range reserved {
		if name == r {
			return fmt.Errorf("'%s' is a reserved name", name)
		}
	}
	
	return nil
}
```

## Alias Commands

```go
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/anomalyco/muster/internal/alias"
	"github.com/spf13/cobra"
)

// NewAliasCommand creates alias management commands
func NewAliasCommand(manager *alias.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage command aliases",
		Long:  "Create, list, and remove command aliases for frequently used commands.",
	}

	cmd.AddCommand(newAliasSetCommand(manager))
	cmd.AddCommand(newAliasRemoveCommand(manager))
	cmd.AddCommand(newAliasListCommand(manager))

	return cmd
}

func newAliasSetCommand(manager *alias.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <command>",
		Short: "Create or update an alias",
		Long:  `Define a new alias or update an existing one.`,
		Example: `  muster alias set ls 'inventory list'
  muster alias set get 'inventory get'
  muster alias set prod 'config set api.url https://prod.example.com'`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			command := strings.Join(args[1:], " ")
			
			if err := manager.Define(name, command); err != nil {
				return err
			}
			
			fmt.Printf("Alias '%s' set to '%s'\n", name, command)
			return nil
		},
	}
}

func newAliasRemoveCommand(manager *alias.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove an alias",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			
			if err := manager.Remove(name); err != nil {
				return err
			}
			
			fmt.Printf("Alias '%s' removed\n", name)
			return nil
		},
	}
}

func newAliasListCommand(manager *alias.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases := manager.List()
			
			if len(aliases) == 0 {
				fmt.Println("No aliases defined")
				return nil
			}
			
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tCOMMAND")
			
			for name, command := range aliases {
				fmt.Fprintf(w, "%s\t%s\n", name, command)
			}
			
			return w.Flush()
		},
	}
}
```

## Default Aliases

```yaml
# Built-in aliases loaded on first run
default_aliases:
  ls: "inventory list"
  get: "inventory get"
  add: "inventory add"
  rm: "inventory remove"
  
  # Config shortcuts
  url: "config get api.url"
  seturl: "config set api.url"
  
  # Auth shortcuts  
  login: "auth login"
  logout: "auth logout"
  whoami: "auth status"
  
  # Daemon shortcuts
  start: "daemon start"
  stop: "daemon stop"
  status: "daemon status"
```

## Expansion Logic

```go
package alias

import (
	"context"
	"os"
	"strings"
)

// Expander handles alias expansion
type Expander struct {
	manager *Manager
	history []string // Prevent recursive expansion
}

// NewExpander creates a new alias expander
func NewExpander(manager *Manager) *Expander {
	return &Expander{
		manager: manager,
		history: make([]string, 0),
	}
}

// ExpandCommand expands aliases in command arguments
func (e *Expander) ExpandCommand(ctx context.Context, args []string) ([]string, error) {
	if len(args) == 0 {
		return args, nil
	}
	
	// Check for recursive expansion
	if e.isInHistory(args[0]) {
		return args, nil
	}
	
	expanded, wasExpanded := e.manager.Expand(args)
	if !wasExpanded {
		return args, nil
	}
	
	// Track expansion
	e.history = append(e.history, args[0])
	defer e.popHistory()
	
	// Limit expansion depth
	if len(e.history) > 10 {
		return nil, fmt.Errorf("alias expansion depth exceeded")
	}
	
	// Recursively expand
	return e.ExpandCommand(ctx, expanded)
}

func (e *Expander) isInHistory(name string) bool {
	for _, h := range e.history {
		if h == name {
			return true
		}
	}
	return false
}

func (e *Expander) popHistory() {
	if len(e.history) > 0 {
		e.history = e.history[:len(e.history)-1]
	}
}
```

## Storage Format

```yaml
# ~/.muster/aliases.yaml
ls: inventory list --format table
get: inventory get
prod: config set api.url https://prod.example.com
dev: config set api.url https://dev.example.com
```
