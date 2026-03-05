# CLI Plugin System Specification

## Overview

The plugin system allows extending the Muster CLI with custom commands and functionality without modifying core code.

## Architecture

```
plugins/
├── system/
│   ├── loader.go      # Plugin discovery and loading
│   ├── registry.go    # Plugin registration
│   └── manager.go     # Plugin lifecycle management
├── types/
│   └── plugin.go      # Plugin interface definitions
└── builtin/           # Built-in plugins
    └── example/
        └── main.go
```

## Plugin Interface

### Plugin Definition

```go
package types

import (
	"context"
	"github.com/spf13/cobra"
)

// Plugin represents a CLI plugin
type Plugin interface {
	// Metadata returns plugin information
	Metadata() PluginMetadata
	
	// Initialize sets up the plugin
	Initialize(ctx context.Context, config PluginConfig) error
	
	// RegisterCommands adds commands to the CLI
	RegisterCommands(root *cobra.Command) error
	
	// Cleanup performs shutdown tasks
	Cleanup() error
}

// PluginMetadata contains plugin information
type PluginMetadata struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	MinCLIVersion string          `json:"min_cli_version"`
	Hooks       []string          `json:"hooks"`
	Flags       map[string]string `json:"flags"`
}

// PluginConfig provides configuration to plugins
type PluginConfig struct {
	ConfigDir   string                 `json:"config_dir"`
	DataDir     string                 `json:"data_dir"`
	Settings    map[string]interface{} `json:"settings"`
	Logger      PluginLogger           `json:"-"`
}

// PluginLogger interface for plugin logging
type PluginLogger interface {
	Debug(msg string, keys ...interface{})
	Info(msg string, keys ...interface{})
	Warn(msg string, keys ...interface{})
	Error(msg string, keys ...interface{})
}
```

## Plugin Loader

```go
package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"plugin"

	"github.com/anomalyco/muster/internal/plugins/types"
)

// Loader handles plugin discovery and loading
type Loader struct {
	pluginDir string
	registry  *Registry
}

// NewLoader creates a new plugin loader
func NewLoader(pluginDir string, registry *Registry) *Loader {
	return &Loader{
		pluginDir: pluginDir,
		registry:  registry,
	}
}

// LoadAll discovers and loads all plugins
func (l *Loader) LoadAll(ctx context.Context) error {
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(l.pluginDir, entry.Name())
		if err := l.loadPlugin(ctx, pluginPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load plugin %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

// loadPlugin loads a single plugin
func (l *Loader) loadPlugin(ctx context.Context, path string) error {
	pluginFile := filepath.Join(path, "plugin.so")
	
	p, err := plugin.Open(pluginFile)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	symbol, err := p.Lookup("Plugin")
	if err != nil {
		return fmt.Errorf("plugin missing 'Plugin' symbol: %w", err)
	}

	cliPlugin, ok := symbol.(types.Plugin)
	if !ok {
		return fmt.Errorf("plugin does not implement Plugin interface")
	}

	metadata := cliPlugin.Metadata()
	
	config := types.PluginConfig{
		ConfigDir: path,
		DataDir:   filepath.Join(path, "data"),
		Logger:    &pluginLogger{name: metadata.Name},
	}

	if err := cliPlugin.Initialize(ctx, config); err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	l.registry.Register(metadata.Name, cliPlugin)
	return nil
}

type pluginLogger struct {
	name string
}

func (l *pluginLogger) Debug(msg string, keys ...interface{}) {
	fmt.Printf("[DEBUG][%s] %s\n", l.name, msg)
}

func (l *pluginLogger) Info(msg string, keys ...interface{})  {
	fmt.Printf("[INFO][%s] %s\n", l.name, msg)
}

func (l *pluginLogger) Warn(msg string, keys ...interface{})  {
	fmt.Printf("[WARN][%s] %s\n", l.name, msg)
}

func (l *pluginLogger) Error(msg string, keys ...interface{}) {
	fmt.Printf("[ERROR][%s] %s\n", l.name, msg)
}
```

## Plugin Registry

```go
package system

import (
	"fmt"
	"sync"

	"github.com/anomalyco/muster/internal/plugins/types"
)

// Registry manages loaded plugins
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]types.Plugin
	order   []string
}

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]types.Plugin),
		order:   []string{},
	}
}

// Register adds a plugin to the registry
func (r *Registry) Register(name string, plugin types.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	r.plugins[name] = plugin
	r.order = append(r.order, name)
	return nil
}

// Get retrieves a plugin by name
func (r *Registry) Get(name string) (types.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[name]
	return plugin, exists
}

// List returns all registered plugins
func (r *Registry) List() []types.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]types.Plugin, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.plugins[name])
	}
	return result
}

// Unload removes and cleans up a plugin
func (r *Registry) Unload(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if err := plugin.Cleanup(); err != nil {
		return fmt.Errorf("failed to cleanup plugin: %w", err)
	}

	delete(r.plugins, name)
	
	newOrder := make([]string, 0, len(r.order)-1)
	for _, n := range r.order {
		if n != name {
			newOrder = append(newOrder, n)
		}
	}
	r.order = newOrder

	return nil
}
```

## Plugin Commands

```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// NewPluginCommand creates the plugin management command
func NewPluginCommand(loader *system.Loader, registry *system.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage CLI plugins",
		Long:  "Discover, load, and manage plugins that extend CLI functionality.",
	}

	cmd.AddCommand(newPluginListCommand(registry))
	cmd.AddCommand(newPluginInstallCommand(loader))
	cmd.AddCommand(newPluginRemoveCommand(registry))
	cmd.AddCommand(newPluginInfoCommand(registry))

	return cmd
}

func newPluginListCommand(registry *system.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			plugins := registry.List()
			if len(plugins) == 0 {
				fmt.Println("No plugins installed")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION")
			
			for _, p := range plugins {
				meta := p.Metadata()
				fmt.Fprintf(w, "%s\t%s\t%s\n", meta.Name, meta.Version, meta.Description)
			}
			
			return w.Flush()
		},
	}
}

func newPluginInstallCommand(loader *system.Loader) *cobra.Command {
	var source string
	
	cmd := &cobra.Command{
		Use:   "install [source]",
		Short: "Install a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			fmt.Printf("Installing plugin from %s...\n", source)
			
			// Download and install logic here
			return nil
		},
	}
	
	cmd.Flags().StringVarP(&source, "source", "s", "", "Plugin source URL")
	return cmd
}

func newPluginRemoveCommand(registry *system.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return registry.Unload(name)
		},
	}
}

func newPluginInfoCommand(registry *system.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show plugin information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			plugin, exists := registry.Get(name)
			if !exists {
				return fmt.Errorf("plugin %s not found", name)
			}

			meta := plugin.Metadata()
			fmt.Printf("Name:        %s\n", meta.Name)
			fmt.Printf("Version:     %s\n", meta.Version)
			fmt.Printf("Author:      %s\n", meta.Author)
			fmt.Printf("Description: %s\n", meta.Description)
			fmt.Printf("Min CLI:     %s\n", meta.MinCLIVersion)
			
			return nil
		},
	}
}
```

## Plugin Configuration

```yaml
# ~/.muster/plugins.yaml
plugins:
  - name: custom-commands
    enabled: true
    settings:
      api_url: https://api.example.com
      timeout: 30s
  
  - name: git-helpers
    enabled: true
    settings:
      default_branch: main
  
  - name: dev-tools
    enabled: false
```

## Plugin Hooks

```go
// Hook types for plugin integration
type HookType string

const (
	HookPreCommand  HookType = "pre_command"
	HookPostCommand HookType = "post_command"
	HookPreRequest  HookType = "pre_request"
	HookPostResponse HookType = "post_response"
)

// HookManager manages plugin hooks
type HookManager struct {
	hooks map[HookType][]HookFunc
}

type HookFunc func(ctx context.Context, data interface{}) error

func (hm *HookManager) Register(hookType HookType, fn HookFunc) {
	hm.hooks[hookType] = append(hm.hooks[hookType], fn)
}

func (hm *HookManager) Execute(ctx context.Context, hookType HookType, data interface{}) error {
	for _, fn := range hm.hooks[hookType] {
		if err := fn(ctx, data); err != nil {
			return err
		}
	}
	return nil
}
```

## Security

Plugins run with same permissions as CLI. Sandboxing planned for v2.
