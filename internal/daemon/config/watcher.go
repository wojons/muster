package config

import (
	"os"
	"path/filepath"
	"time"
)

// FileWatcher watches for configuration file changes
type FileWatcher struct {
	path    string
	manager *Manager
	lastMod time.Time
	stopCh  chan struct{}
}

// NewFileWatcher creates a new file watcher for the given config path and manager
func NewFileWatcher(path string, manager *Manager) *FileWatcher {
	return &FileWatcher{
		path:    path,
		manager: manager,
		stopCh:  make(chan struct{}),
	}
}

// Start begins watching for file changes at the specified interval
// This method blocks until Stop() is called, so it should be run in a goroutine
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

// Stop halts the file watcher
// This method is safe to call multiple times
func (w *FileWatcher) Stop() {
	select {
	case <-w.stopCh:
		// Already closed
	default:
		close(w.stopCh)
	}
}

// check compares the current file modification time with the last known time
// If the file has been modified, it triggers a config reload
func (w *FileWatcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		// File doesn't exist or can't be accessed
		return
	}

	if info.ModTime().After(w.lastMod) {
		w.lastMod = info.ModTime()
		// Load the new configuration
		if _, err := w.manager.Load(); err == nil {
			// Signal that a reload occurred
			w.manager.SignalReload()
		}
	}
}

// Initialize sets the initial modification time from the current file
// This should be called before Start() to establish a baseline
func (w *FileWatcher) Initialize() error {
	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's okay
			return nil
		}
		return err
	}
	w.lastMod = info.ModTime()
	return nil
}

// GetPath returns the absolute path of the watched file
func (w *FileWatcher) GetPath() (string, error) {
	return filepath.Abs(w.path)
}

// GetLastMod returns the last known modification time
func (w *FileWatcher) GetLastMod() time.Time {
	return w.lastMod
}
