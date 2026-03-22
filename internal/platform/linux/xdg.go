package linux

import (
	"os"
	"path/filepath"
)

// XDGPaths holds all XDG directory paths
type XDGPaths struct {
	ConfigHome string
	CacheHome  string
	DataHome   string
	StateHome  string
	RuntimeDir string
}

// DefaultXDGPaths returns XDG paths with defaults
func DefaultXDGPaths() *XDGPaths {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}

	paths := &XDGPaths{
		ConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		CacheHome:  os.Getenv("XDG_CACHE_HOME"),
		DataHome:   os.Getenv("XDG_DATA_HOME"),
		StateHome:  os.Getenv("XDG_STATE_HOME"),
		RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
	}

	// Apply defaults
	if paths.ConfigHome == "" {
		paths.ConfigHome = filepath.Join(home, ".config")
	}
	if paths.CacheHome == "" {
		paths.CacheHome = filepath.Join(home, ".cache")
	}
	if paths.DataHome == "" {
		paths.DataHome = filepath.Join(home, ".local", "share")
	}
	if paths.StateHome == "" {
		paths.StateHome = filepath.Join(home, ".local", "state")
	}
	if paths.RuntimeDir == "" {
		// Fallback to /tmp for systems without XDG_RUNTIME_DIR
		paths.RuntimeDir = filepath.Join("/tmp", "muster-runtime-"+os.Getenv("USER"))
	}

	return paths
}

// MusterPaths returns muster-specific paths
func (x *XDGPaths) MusterPaths() *MusterPaths {
	return &MusterPaths{
		ConfigDir:  filepath.Join(x.ConfigHome, "muster"),
		CacheDir:   filepath.Join(x.CacheHome, "muster"),
		DataDir:    filepath.Join(x.DataHome, "muster"),
		StateDir:   filepath.Join(x.StateHome, "muster"),
		RuntimeDir: filepath.Join(x.RuntimeDir, "muster"),
	}
}

// MusterPaths holds muster-specific directory paths
type MusterPaths struct {
	ConfigDir  string
	CacheDir   string
	DataDir    string
	StateDir   string
	RuntimeDir string
}

// EnsureAll creates all muster directories
func (m *MusterPaths) EnsureAll() error {
	dirs := []string{
		m.ConfigDir,
		m.CacheDir,
		m.DataDir,
		m.StateDir,
		m.RuntimeDir,
		filepath.Join(m.ConfigDir, "keys"),
		filepath.Join(m.CacheDir, "inventories"),
		filepath.Join(m.CacheDir, "modules"),
		filepath.Join(m.CacheDir, "specs"),
		filepath.Join(m.DataDir, "known-good"),
		filepath.Join(m.DataDir, "audit"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// ConfigFile returns the path to a config file
func (m *MusterPaths) ConfigFile(name string) string {
	return filepath.Join(m.ConfigDir, name)
}

// CacheFile returns the path to a cache file
func (m *MusterPaths) CacheFile(name string) string {
	return filepath.Join(m.CacheDir, name)
}

// DataFile returns the path to a data file
func (m *MusterPaths) DataFile(name string) string {
	return filepath.Join(m.DataDir, name)
}

// StateFile returns the path to a state file
func (m *MusterPaths) StateFile(name string) string {
	return filepath.Join(m.StateDir, name)
}

// RuntimeFile returns the path to a runtime file
func (m *MusterPaths) RuntimeFile(name string) string {
	return filepath.Join(m.RuntimeDir, name)
}

// SocketPath returns the path to the daemon socket
func (m *MusterPaths) SocketPath() string {
	return filepath.Join(m.RuntimeDir, "daemon.sock")
}

// PidFile returns the path to the PID file
func (m *MusterPaths) PidFile() string {
	return filepath.Join(m.RuntimeDir, "daemon.pid")
}
