# OS Filesystem Specification

Complete specification for cross-platform file system operations with XDG directory support.

## Overview

Cross-platform file system utilities supporting XDG directories, configuration paths, cache locations, and file operations across macOS, Linux, and Windows.

## Core Interface

```go
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppPaths holds standard application directories
type AppPaths struct {
	Name    string
	Config  string
	Cache   string
	Data    string
	State   string
}

// NewAppPaths creates paths for the named application
func NewAppPaths(name string) (*AppPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}

	paths := &AppPaths{Name: name}

	switch runtime.GOOS {
	case "darwin":
		paths.Config = filepath.Join(home, "Library", "Application Support", name)
		paths.Cache = filepath.Join(home, "Library", "Caches", name)
		paths.Data = paths.Config
		paths.State = paths.Config

	case "windows":
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		paths.Config = filepath.Join(appData, name)
		paths.Cache = filepath.Join(localAppData, name, "Cache")
		paths.Data = filepath.Join(localAppData, name, "Data")
		paths.State = paths.Data

	default: // Linux and other Unix
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			paths.Config = filepath.Join(xdgConfig, name)
		} else {
			paths.Config = filepath.Join(home, ".config", name)
		}

		if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
			paths.Cache = filepath.Join(xdgCache, name)
		} else {
			paths.Cache = filepath.Join(home, ".cache", name)
		}

		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			paths.Data = filepath.Join(xdgData, name)
		} else {
			paths.Data = filepath.Join(home, ".local", "share", name)
		}

		if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
			paths.State = filepath.Join(xdgState, name)
		} else {
			paths.State = filepath.Join(home, ".local", "state", name)
		}
	}

	return paths, nil
}
```

## Directory Operations

```go
// EnsureDir creates directory if it doesn't exist
func EnsureDir(path string, perm os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, perm)
	}
	return nil
}

// EnsureAll creates all app directories
func (p *AppPaths) EnsureAll() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{p.Config, 0755},
		{p.Cache, 0755},
		{p.Data, 0755},
		{p.State, 0755},
	}

	for _, d := range dirs {
		if err := EnsureDir(d.path, d.perm); err != nil {
			return fmt.Errorf("create %s: %w", d.path, err)
		}
	}
	return nil
}
```

## File Operations

```go
// WriteFile writes data atomically
func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadFile reads file with size limit
func ReadFile(path string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d > %d", info.Size(), maxSize)
	}

	return os.ReadFile(path)
}

// FileExists checks if file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
```

## Lock Files

```go
// LockFile provides advisory file locking
type LockFile struct {
	path string
	file *os.File
}

// NewLockFile creates a lock file
func NewLockFile(path string) *LockFile {
	return &LockFile{path: path}
}

// TryLock attempts to acquire lock (non-blocking)
func (l *LockFile) TryLock() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	l.file = f

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("lock busy: %w", err)
	}
	return nil
}

// Unlock releases the lock
func (l *LockFile) Unlock() error {
	if l.file == nil {
		return nil
	}
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
	return err
}
```

## Usage Example

```go
func InitializeApp() error {
	paths, err := fsutil.NewAppPaths("muster")
	if err != nil {
		return err
	}

	// Create all directories
	if err := paths.EnsureAll(); err != nil {
		return err
	}

	// Store config
	configPath := filepath.Join(paths.Config, "settings.yaml")
	return fsutil.WriteFile(configPath, []byte("..."), 0600)
}
```

## Version

**1.0.0** - 2024-01-15
