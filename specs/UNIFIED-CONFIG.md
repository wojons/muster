# Unified Configuration Specification

## Overview

Shared configuration system that synchronizes settings between CLI and MCP interfaces.

## Configuration Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Unified Configuration                       │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ CLI Config   │  │ MCP Config   │  │ Shared Core │     │
│  │  (viper)     │  │  (JSON-RPC)  │  │  (layer)    │     │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘     │
│         └────────────────┼────────────────┘              │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  Config Store          │                  │
│              │  (SQLite/File)        │                  │
│              └───────────┬───────────┘                  │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  Sync Layer          │                  │
│              │  (event-driven)       │                  │
│              └───────────────────────┘                  │
└─────────────────────────────────────────────────────────┘
```

## Configuration Schema

```go
// internal/unified/config.go
package unified

import (
    "sync"
    "time"
)

// Config represents unified configuration
type Config struct {
    mu sync.RWMutex
    
    // Core settings (shared)
    Core CoreConfig `json:"core" yaml:"core"`
    
    // Profile-specific settings
    Profiles map[string]*ProfileConfig `json:"profiles" yaml:"profiles"`
    
    // Current active profile
    ActiveProfile string `json:"active_profile" yaml:"active_profile"`
    
    // Last sync timestamp
    LastSync time.Time `json:"last_sync" yaml:"last_sync"`
    
    // Version for optimistic locking
    Version int64 `json:"version" yaml:"version"`
}

// CoreConfig contains settings used by both CLI and MCP
type CoreConfig struct {
    APIURL      string        `json:"api_url" yaml:"api_url"`
    Timeout     time.Duration `json:"timeout" yaml:"timeout"`
    LogLevel    string        `json:"log_level" yaml:"log_level"`
    AuthMethod  string        `json:"auth_method" yaml:"auth_method"`
    AutoSync    bool          `json:"auto_sync" yaml:"auto_sync"`
    SyncInterval time.Duration `json:"sync_interval" yaml:"sync_interval"`
}

// ProfileConfig contains profile-specific settings
type ProfileConfig struct {
    Name      string                 `json:"name" yaml:"name"`
    APIURL    string                 `json:"api_url" yaml:"api_url"`
    Auth      AuthConfig             `json:"auth" yaml:"auth"`
    Headers   map[string]string      `json:"headers" yaml:"headers"`
    Defaults  map[string]interface{} `json:"defaults" yaml:"defaults"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
    Method   string `json:"method" yaml:"method"`
    Token    string `json:"token,omitempty" yaml:"token,omitempty"`
    APIKey   string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
    Username string `json:"username,omitempty" yaml:"username,omitempty"`
}
```

## Config Manager

```go
// internal/unified/config_manager.go
package unified

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    
    "github.com/fsnotify/fsnotify"
    "github.com/spf13/viper"
)

// ConfigManager handles configuration loading and syncing
type ConfigManager struct {
    config     *Config
    viper      *viper.Viper
    storePath  string
    watchers   []ConfigWatcher
    mu         sync.RWMutex
}

// ConfigWatcher is called when config changes
type ConfigWatcher func(old, new *Config)

// NewConfigManager creates a new config manager
func NewConfigManager(configPath string) (*ConfigManager, error) {
    v := viper.New()
    v.SetConfigFile(configPath)
    v.SetConfigType("yaml")
    
    // Environment variable support
    v.SetEnvPrefix("MUSTER")
    v.AutomaticEnv()
    
    cm := &ConfigManager{
        viper:     v,
        storePath: configPath,
        config:    &Config{},
        watchers:  make([]ConfigWatcher, 0),
    }
    
    // Watch for file changes
    v.WatchConfig()
    v.OnConfigChange(func(e fsnotify.Event) {
        cm.reloadConfig()
    })
    
    return cm, nil
}

// Load loads configuration from file
func (cm *ConfigManager) Load() error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if err := cm.viper.ReadInConfig(); err != nil {
        if !os.IsNotExist(err) {
            return fmt.Errorf("failed to read config: %w", err)
        }
        // Use defaults for new config
    }
    
    if err := cm.viper.Unmarshal(cm.config); err != nil {
        return fmt.Errorf("failed to unmarshal config: %w", err)
    }
    
    return nil
}

// Save persists configuration to file
func (cm *ConfigManager) Save() error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    // Ensure directory exists
    dir := filepath.Dir(cm.storePath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("failed to create config directory: %w", err)
    }
    
    // Increment version for optimistic locking
    cm.config.Version++
    cm.config.LastSync = time.Now()
    
    data, err := yaml.Marshal(cm.config)
    if err != nil {
        return fmt.Errorf("failed to marshal config: %w", err)
    }
    
    if err := os.WriteFile(cm.storePath, data, 0600); err != nil {
        return fmt.Errorf("failed to write config: %w", err)
    }
    
    return nil
}

// Get returns the current config (copy)
func (cm *ConfigManager) Get() *Config {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    // Return deep copy
    data, _ := json.Marshal(cm.config)
    var copy Config
    json.Unmarshal(data, &copy)
    return &copy
}

// Set updates configuration
func (cm *ConfigManager) Set(key string, value interface{}) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    oldConfig := cm.config
    cm.viper.Set(key, value)
    
    if err := cm.viper.Unmarshal(cm.config); err != nil {
        cm.config = oldConfig
        return fmt.Errorf("failed to update config: %w", err)
    }
    
    // Notify watchers
    for _, watcher := range cm.watchers {
        watcher(oldConfig, cm.config)
    }
    
    return nil
}

// Watch adds a config change watcher
func (cm *ConfigManager) Watch(watcher ConfigWatcher) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    cm.watchers = append(cm.watchers, watcher)
}

// GetProfile returns a profile by name
func (cm *ConfigManager) GetProfile(name string) (*ProfileConfig, bool) {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    profile, ok := cm.config.Profiles[name]
    return profile, ok
}

// SetProfile updates or creates a profile
func (cm *ConfigManager) SetProfile(name string, profile *ProfileConfig) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if cm.config.Profiles == nil {
        cm.config.Profiles = make(map[string]*ProfileConfig)
    }
    
    profile.Name = name
    cm.config.Profiles[name] = profile
    
    return cm.Save()
}
```

## Config Sync

```go
// internal/unified/config_sync.go
package unified

import (
    "context"
    "time"
)

// ConfigSync handles bidirectional synchronization
type ConfigSync struct {
    manager    *ConfigManager
    syncChan   chan ConfigEvent
    ctx        context.Context
    cancel     context.CancelFunc
}

// ConfigEvent represents a config change event
type ConfigEvent struct {
    Type    string                 `json:"type"`
    Key     string                 `json:"key"`
    Value   interface{}            `json:"value"`
    Source  string                 `json:"source"` // "cli" or "mcp"
    Timestamp time.Time            `json:"timestamp"`
}

// NewConfigSync creates a sync handler
func NewConfigSync(manager *ConfigManager) *ConfigSync {
    ctx, cancel := context.WithCancel(context.Background())
    return &ConfigSync{
        manager:  manager,
        syncChan: make(chan ConfigEvent, 100),
        ctx:      ctx,
        cancel:   cancel,
    }
}

// Start begins the sync loop
func (s *ConfigSync) Start() {
    go s.syncLoop()
}

// Stop halts the sync loop
func (s *ConfigSync) Stop() {
    s.cancel()
    close(s.syncChan)
}

// Notify triggers a config sync event
func (s *ConfigSync) Notify(event ConfigEvent) {
    select {
    case s.syncChan <- event:
    case <-s.ctx.Done():
    }
}

func (s *ConfigSync) syncLoop() {
    ticker := time.NewTicker(s.manager.Get().Core.SyncInterval)
    defer ticker.Stop()
    
    for {
        select {
        case event := <-s.syncChan:
            s.handleEvent(event)
        case <-ticker.C:
            s.periodicSync()
        case <-s.ctx.Done():
            return
        }
    }
}

func (s *ConfigSync) handleEvent(event ConfigEvent) {
    // Apply changes based on event type
    switch event.Type {
    case "set":
        s.manager.Set(event.Key, event.Value)
    case "delete":
        s.manager.Set(event.Key, nil)
    case "profile_activate":
        s.manager.Set("active_profile", event.Value)
    }
    
    // Persist changes
    s.manager.Save()
}

func (s *ConfigSync) periodicSync() {
    if !s.manager.Get().Core.AutoSync {
        return
    }
    // Reload from disk to pick up external changes
    s.manager.Load()
}
```
