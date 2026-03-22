package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Loader handles configuration loading from multiple sources
type Loader struct {
	sources []ConfigSource
	v       *viper.Viper
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("OPENAPI_CLI")
	v.AutomaticEnv()

	return &Loader{
		sources: ConfigSources,
		v:       v,
	}
}

// Load loads configuration from all sources
func (l *Loader) Load() (*Config, error) {
	cfg := &Config{
		Version: "1.0.0",
	}

	// Set default paths
	l.setDefaultPaths(cfg)

	// Load from sources in priority order
	for _, source := range l.sources {
		if err := l.loadSource(source, cfg); err != nil {
			// Continue on error - missing configs are OK
			continue
		}
	}

	return cfg, nil
}

// loadSource loads configuration from a single source
func (l *Loader) loadSource(source ConfigSource, cfg *Config) error {
	path := os.ExpandEnv(source.Path)

	if source.Env {
		if path == "" {
			return nil
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return err
	}

	l.v.SetConfigFile(path)
	if err := l.v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config from %s: %w", path, err)
	}

	if err := l.v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal config from %s: %w", path, err)
	}

	return nil
}

// setDefaultPaths sets platform-specific default paths
func (l *Loader) setDefaultPaths(cfg *Config) {
	homeDir, _ := os.UserHomeDir()

	var configDir, cacheDir, dataDir string

	switch runtime.GOOS {
	case "darwin":
		configDir = filepath.Join(homeDir, "Library", "Application Support", "openapi-cli")
		cacheDir = filepath.Join(homeDir, "Library", "Caches", "openapi-cli")
		dataDir = filepath.Join(homeDir, "Library", "Application Support", "openapi-cli")
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "openapi-cli")
		cacheDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "openapi-cli", "cache")
		dataDir = filepath.Join(os.Getenv("APPDATA"), "openapi-cli", "data")
	default: // Linux and others
		configDir = filepath.Join(homeDir, ".config", "openapi-cli")
		cacheDir = filepath.Join(homeDir, ".cache", "openapi-cli")
		dataDir = filepath.Join(homeDir, ".local", "share", "openapi-cli")
	}

	cfg.ConfigDir = configDir
	cfg.CacheDir = cacheDir
	cfg.DataDir = dataDir
}

// Load is a convenience function that creates a loader and loads config
func Load() (*Config, error) {
	loader := NewLoader()
	return loader.Load()
}

// BindPFlags binds Cobra command flags to Viper configuration keys.
// This allows CLI flags to override configuration file values.
// It binds persistent flags defined on the root command.
func (l *Loader) BindPFlags(cmd *cobra.Command) error {
	// Define flag-to-viper key mappings
	flagBindings := []struct {
		flagKey  string
		viperKey string
		flagType string
	}{
		{"host", "default_host", "string"},
		{"timeout", "default_timeout", "duration"},
		{"verbose", "verbose", "bool"},
		{"config", "config_file", "string"},
		{"profile", "default_profile", "string"},
		{"output", "output_format", "string"},
	}

	for _, binding := range flagBindings {
		flag := cmd.PersistentFlags().Lookup(binding.flagKey)
		if flag != nil {
			if err := l.v.BindPFlag(binding.viperKey, flag); err != nil {
				return fmt.Errorf("failed to bind flag %q: %w", binding.flagKey, err)
			}
		}
	}

	return nil
}

// BindEnvVar binds an environment variable to a Viper configuration key.
// The environment variable name is automatically prefixed with the loader's
// env prefix (e.g., OPENAPI_CLI_HOST for key "host").
func (l *Loader) BindEnvVar(key string) error {
	envVarName := strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	if err := l.v.BindEnv(key, envVarName); err != nil {
		return fmt.Errorf("failed to bind env var for key %q: %w", key, err)
	}
	return nil
}

// GetViper returns the internal viper instance for advanced usage.
func (l *Loader) GetViper() *viper.Viper {
	return l.v
}
