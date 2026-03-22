package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager handles configuration with hot reload support
type Manager struct {
	path      string
	config    *Config
	mu        sync.RWMutex
	watchers  []chan *Config
	watcherMu sync.RWMutex
	reloadCh  chan struct{}
}

// NewManager creates a new config manager with the given path
func NewManager(path string) *Manager {
	return &Manager{
		path:     path,
		config:   Default(),
		reloadCh: make(chan struct{}, 1),
	}
}

// Load reads configuration from file
// Returns nil if file doesn't exist (uses defaults)
func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return m.Get(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	m.mu.Lock()
	m.config = &cfg
	m.mu.Unlock()

	m.notifyWatchers(&cfg)
	return &cfg, nil
}

// Save writes configuration to file
func (m *Manager) Save(config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	// Write to file first
	if err := m.saveToFile(config); err != nil {
		return err
	}

	// Then update state
	m.mu.Lock()
	m.config = config
	m.mu.Unlock()

	m.notifyWatchers(config)
	return nil
}

// saveToFile writes config to disk without updating state
func (m *Manager) saveToFile(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
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

// Get returns current configuration (copy)
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return nil
	}
	return m.config.Clone()
}

// Update modifies configuration atomically
func (m *Manager) Update(fn func(*Config)) error {
	if fn == nil {
		return fmt.Errorf("update function cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.config.Clone()
	fn(cfg)

	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := m.saveToFile(cfg); err != nil {
		return err
	}

	m.config = cfg
	m.notifyWatchers(cfg)
	return nil
}

// Watch returns a channel that receives config updates
func (m *Manager) Watch() <-chan *Config {
	ch := make(chan *Config, 1)
	m.watcherMu.Lock()
	m.watchers = append(m.watchers, ch)
	m.watcherMu.Unlock()
	return ch
}

// notifyWatchers sends config to all watchers (non-blocking)
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
func (m *Manager) ReloadChannel() <-chan struct{} {
	return m.reloadCh
}

// SignalReload triggers a reload
func (m *Manager) SignalReload() {
	select {
	case m.reloadCh <- struct{}{}:
	default:
	}
}

// Close stops all watchers and cleans up resources
func (m *Manager) Close() {
	m.watcherMu.Lock()
	for _, ch := range m.watchers {
		close(ch)
	}
	m.watchers = nil
	m.watcherMu.Unlock()

	close(m.reloadCh)
}
