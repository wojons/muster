package config

import (
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/wojons/muster/pkg/log"
	"github.com/spf13/viper"
)

// Watcher enables hot-reloading of configuration files using fsnotify.
// It watches for changes to the configuration file and triggers callbacks
// when changes are detected.
type Watcher struct {
	v        *viper.Viper
	onChange func()
	watching bool
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewWatcher creates a new configuration watcher for the given viper instance.
// The onChange callback is invoked whenever the configuration file changes.
func NewWatcher(v *viper.Viper, onChange func()) *Watcher {
	return &Watcher{
		v:        v,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

// WatchConfig starts watching the configuration file for changes.
// It uses Viper's built-in WatchConfig functionality with fsnotify.
// This method is safe to call multiple times - subsequent calls are no-ops
// if already watching.
func (w *Watcher) WatchConfig() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.watching {
		return nil
	}

	// Register the config change handler
	w.v.OnConfigChange(func(e fsnotify.Event) {
		log.Global().Info().Str("file", e.Name).Msg("Config file changed")
		if w.onChange != nil {
			w.onChange()
		}
	})

	// Start watching
	w.v.WatchConfig()
	w.watching = true

	return nil
}

// Stop stops watching for configuration changes.
// This method is safe to call multiple times.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.watching {
		return
	}

	select {
	case <-w.stopCh:
		// Already closed
	default:
		close(w.stopCh)
	}

	w.watching = false
}

// IsWatching returns true if the watcher is currently active.
func (w *Watcher) IsWatching() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.watching
}

// GetConfig retrieves the current configuration as a typed Config struct.
// This unmarshals the viper configuration into the Config type.
func (w *Watcher) GetConfig() (*Config, error) {
	var cfg Config
	if err := w.v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}
	return &cfg, nil
}
