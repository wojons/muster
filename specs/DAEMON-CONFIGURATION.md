# Daemon Configuration - Config Management

Complete daemon configuration with hot reload support.

## File: internal/daemon/config/config.go

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config holds daemon configuration
type Config struct {
	SocketPath      string        `json:"socket_path"`
	PIDFile         string        `json:"pid_file"`
	LogFile         string        `json:"log_file"`
	TargetURL       string        `json:"target_url"`
	SocketMode      os.FileMode   `json:"socket_mode"`
	MaxConnections  int           `json:"max_connections"`
	KeepAlive       bool          `json:"keep_alive"`
	KeepAlivePeriod time.Duration `json:"keep_alive_period"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	LogLevel        string        `json:"log_level"`
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		SocketPath:      "/var/run/muster/daemon.sock",
		PIDFile:         "/var/run/muster/daemon.pid",
		LogFile:         "/var/log/muster/daemon.log",
		TargetURL:       "http://localhost:8080",
		SocketMode:      0660,
		MaxConnections:  100,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		LogLevel:        "info",
	}
}

// Validate checks configuration validity
func (c *Config) Validate() error {
	if c.SocketPath == "" {
		return fmt.Errorf("socket_path is required")
	}
	if c.TargetURL == "" {
		return fmt.Errorf("target_url is required")
	}
	if c.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive")
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	return nil
}

// Manager handles configuration with hot reload
type Manager struct {
	path      string
	config    *Config
	mu        sync.RWMutex
	watchers  []chan *Config
	watcherMu sync.RWMutex
	reloadCh  chan struct{}
}

// NewManager creates a config manager
func NewManager(path string) *Manager {
	return &Manager{
		path:     path,
		config:   Default(),
		reloadCh: make(chan struct{}, 1),
	}
}

// Load reads configuration from file
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	m.mu.Lock()
	m.config = &cfg
	m.mu.Unlock()

	m.notifyWatchers(&cfg)
	return nil
}

// Save writes configuration to file
func (m *Manager) Save() error {
	m.mu.RLock()
	cfg := m.config
	m.mu.RUnlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(m.path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// Get returns current configuration copy
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Update modifies configuration atomically
func (m *Manager) Update(fn func(*Config)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fn(m.config)

	if err := m.config.Validate(); err != nil {
		return err
	}

	return m.Save()
}

// Watch returns a channel that receives config updates
func (m *Manager) Watch() chan *Config {
	ch := make(chan *Config, 1)
	m.watcherMu.Lock()
	m.watchers = append(m.watchers, ch)
	m.watcherMu.Unlock()
	return ch
}

// notifyWatchers sends config to all watchers
func (m *Manager) notifyWatchers(cfg *Config) {
	m.watcherMu.RLock()
	watchers := make([]chan *Config, len(m.watchers))
	copy(watchers, m.watchers)
	m.watcherMu.RUnlock()

	for _, ch := range watchers {
		select {
		case ch <- cfg:
		default:
		}
	}
}

// ReloadChannel returns the reload signal channel
func (m *Manager) ReloadChannel() chan struct{} {
	return m.reloadCh
}

// SignalReload triggers a reload
func (m *Manager) SignalReload() {
	select {
	case m.reloadCh <- struct{}{}:
	default:
	}
}
```

## File: internal/daemon/config/watcher.go

```go
package config

import (
	"os"
	"path/filepath"
	"time"
)

// FileWatcher watches for configuration file changes
type FileWatcher struct {
	path       string
	manager    *Manager
	lastMod    time.Time
	stopCh     chan struct{}
}

// NewFileWatcher creates a file watcher
func NewFileWatcher(path string, manager *Manager) *FileWatcher {
	return &FileWatcher{
		path:    path,
		manager: manager,
		stopCh:  make(chan struct{}),
	}
}

// Start begins watching for changes
func (w *FileWatcher) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.check()
		case <-w.stopCh:
			return
		}
	}
}

// Stop halts the watcher
func (w *FileWatcher) Stop() {
	close(w.stopCh)
}

// check compares current modification time
func (w *FileWatcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		return
	}

	if info.ModTime().After(w.lastMod) {
		w.lastMod = info.ModTime()
		w.manager.Load()
		w.manager.SignalReload()
	}
}

// Initialize sets initial modification time
func (w *FileWatcher) Initialize() error {
	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	w.lastMod = info.ModTime()
	return nil
}

// GetPath returns absolute path
func (w *FileWatcher) GetPath() (string, error) {
	return filepath.Abs(w.path)
}
```

## Configuration File

```json
{
  "socket_path": "/var/run/muster/daemon.sock",
  "pid_file": "/var/run/muster/daemon.pid",
  "log_file": "/var/log/muster/daemon.log",
  "target_url": "http://localhost:8080",
  "socket_mode": 416,
  "max_connections": 100,
  "keep_alive": true,
  "keep_alive_period": 30000000000,
  "read_timeout": 30000000000,
  "write_timeout": 30000000000,
  "shutdown_timeout": 30000000000,
  "log_level": "info"
}
```

## Usage

```go
mgr := config.NewManager("/etc/muster/daemon.json")

if err := mgr.Load(); err != nil {
	log.Fatal(err)
}

watcher := config.NewFileWatcher("/etc/muster/daemon.json", mgr)
go watcher.Start(5 * time.Second)

// Hot reload
mgr.SignalReload()
```
