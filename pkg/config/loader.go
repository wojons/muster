package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Loader handles configuration loading from multiple sources
type Loader struct {
	sources    []ConfigSource
	v          *viper.Viper
	configFile string // explicit config file path (from --config flag)
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("MUSTER")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	return &Loader{
		sources: ConfigSources,
		v:       v,
	}
}

// SetConfigFile sets an explicit config file path (e.g., from --config flag).
// Must be called before Load().
func (l *Loader) SetConfigFile(path string) {
	l.configFile = path
}

// Load loads configuration from all sources in priority order.
func (l *Loader) Load() (*Config, error) {
	cfg := &Config{
		Version: "1.0.0",
	}

	// Set default paths
	l.setDefaultPaths(cfg)

	// If an explicit config file was provided, use only that
	if l.configFile != "" {
		if err := l.loadFile(l.configFile, cfg); err != nil {
			return nil, fmt.Errorf("load config file %s: %w", l.configFile, err)
		}
		return cfg, nil
	}

	// Load from file sources in priority order (lower priority first, gets overridden)
	for _, source := range l.sources {
		path := os.ExpandEnv(source.Path)
		// Skip empty paths (env vars that aren't set)
		if path == "" {
			continue
		}
		// For env-sourced paths, skip if the file doesn't exist
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if err := l.loadFile(path, cfg); err != nil {
			// Non-fatal — missing optional configs are OK
			continue
		}
	}

	// Apply env vars as top-priority overrides (after files, before CLI flags)
	l.applyEnvOverrides(cfg)

	return cfg, nil
}

// loadFile reads a single config file and merges it into cfg.
func (l *Loader) loadFile(path string, cfg *Config) error {
	l.v.SetConfigFile(path)
	if err := l.v.MergeInConfig(); err != nil {
		return fmt.Errorf("read config from %s: %w", path, err)
	}

	// Unmarshal the accumulated state into a fresh struct
	var merged Config
	if err := l.v.Unmarshal(&merged); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Copy into cfg, preserving computed paths
	dirs := struct{ configDir, cacheDir, dataDir string }{cfg.ConfigDir, cfg.CacheDir, cfg.DataDir}
	*cfg = merged
	cfg.ConfigDir = dirs.configDir
	cfg.CacheDir = dirs.cacheDir
	cfg.DataDir = dirs.dataDir

	return nil
}

// applyEnvOverrides applies MUSTER_* environment variables as overrides.
func (l *Loader) applyEnvOverrides(cfg *Config) {
	// MUSTER_HOST overrides host
	if v := os.Getenv("MUSTER_HOST"); v != "" {
		cfg.Host = v
	}
	// MUSTER_VERBOSE overrides verbose
	if v := os.Getenv("MUSTER_VERBOSE"); v == "1" || v == "true" {
		cfg.Verbose = true
	}
	// MUSTER_LOG_LEVEL overrides log level
	if v := os.Getenv("MUSTER_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
		cfg.Logging.Level = v
	}
	// MUSTER_LOG_FORMAT overrides log format
	if v := os.Getenv("MUSTER_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
		cfg.Logging.Format = v
	}
	// MUSTER_DATABASE_URL overrides database URL
	if v := os.Getenv("MUSTER_DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	// MUSTER_AUTH_TOKEN overrides auth token
	if v := os.Getenv("MUSTER_AUTH_TOKEN"); v != "" {
		cfg.AuthToken = v
	}
	// MUSTER_TIMEOUT
	if v := os.Getenv("MUSTER_TIMEOUT"); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.Timeout = d
		}
	}
	// MUSTER_MAX_RETRIES
	if v := os.Getenv("MUSTER_MAX_RETRIES"); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.MaxRetries = n
		}
	}
	// MUSTER_CONFIG_FILE
	if v := os.Getenv("MUSTER_CONFIG_FILE"); v != "" {
		cfg.ConfigFile = v
	}
	// MUSTER_AUDIT_ENABLED
	if v := os.Getenv("MUSTER_AUDIT_ENABLED"); v == "1" || v == "true" {
		cfg.AuditEnabled = true
	}
}

func parseDuration(s string) (time.Duration, error) { return time.ParseDuration(s) }
func parseInt(s string) (int, error)                { var n int; _, err := fmt.Sscanf(s, "%d", &n); return n, err }

// setDefaultPaths sets platform-specific default paths
func (l *Loader) setDefaultPaths(cfg *Config) {
	homeDir, _ := os.UserHomeDir()

	var configDir, cacheDir, dataDir string

	switch runtime.GOOS {
	case "darwin":
		configDir = filepath.Join(homeDir, "Library", "Application Support", "muster")
		cacheDir = filepath.Join(homeDir, "Library", "Caches", "muster")
		dataDir = filepath.Join(homeDir, "Library", "Application Support", "muster")
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "muster")
		cacheDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "muster", "cache")
		dataDir = filepath.Join(os.Getenv("APPDATA"), "muster", "data")
	default: // Linux and others
		configDir = filepath.Join(homeDir, ".config", "muster")
		cacheDir = filepath.Join(homeDir, ".cache", "muster")
		dataDir = filepath.Join(homeDir, ".local", "share", "muster")
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
	flagBindings := []struct {
		flagKey  string
		viperKey string
	}{
		{"config", "config_file"},
		{"host", "host"},
		{"verbose", "verbose"},
		{"log-level", "log_level"},
		{"log-format", "log_format"},
		{"database-url", "database_url"},
		{"auth-token", "auth_token"},
		{"timeout", "timeout"},
		{"max-retries", "max_retries"},
		{"profile", "default_profile"},
		{"output", "output_format"},
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
// env prefix (e.g., MUSTER_HOST for key "host").
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

// ApplyCLIFlags applies CLI flag values to a Config. This is called after
// Load() so that CLI flags (highest priority) override everything else.
func (l *Loader) ApplyCLIFlags(cmd *cobra.Command, cfg *Config) error {
	// After all other sources are loaded, unmarshal the accumulated viper state
	// (which now includes CLI flag bindings) into the config.
	if err := l.v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal final config: %w", err)
	}
	return nil
}

// ConfigManager provides high-level configuration management.
// axiom:trace work_item=rebuild-foundation-03 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-2 impl=pkg/config/loader.go
type ConfigManager struct {
	cfg *Config
}

// NewConfigManager creates a new ConfigManager by loading configuration
// from all configured sources.
func NewConfigManager() (*ConfigManager, error) {
	cfg, err := Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &ConfigManager{cfg: cfg}, nil
}

// Config returns the underlying configuration.
func (cm *ConfigManager) Config() *Config {
	return cm.cfg
}

// GetDefaultProfile returns the default profile name.
func (cm *ConfigManager) GetDefaultProfile() string {
	return cm.cfg.DefaultProfile
}

// ProfileManager provides profile-specific configuration operations.
type ProfileManager struct {
	cfg *Config
}

// NewProfileManager creates a new ProfileManager.
func NewProfileManager(cm *ConfigManager) *ProfileManager {
	return &ProfileManager{cfg: cm.cfg}
}

// List returns all configured profiles.
func (pm *ProfileManager) List() []*Profile {
	profiles := make([]*Profile, 0, len(pm.cfg.Profiles))
	for name := range pm.cfg.Profiles {
		p := pm.cfg.Profiles[name]
		if p.BaseURL == "" {
			p.BaseURL = name
		}
		profiles = append(profiles, &p)
	}
	return profiles
}

// Get returns a profile by name.
func (pm *ProfileManager) Get(name string) (*Profile, error) {
	profile, ok := pm.cfg.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	if profile.BaseURL == "" {
		profile.BaseURL = name
	}
	return &profile, nil
}

// Exists checks whether a profile with the given name exists.
func (pm *ProfileManager) Exists(name string) bool {
	_, ok := pm.cfg.Profiles[name]
	return ok
}

// SetDefault sets the named profile as the default.
func (pm *ProfileManager) SetDefault(name string) error {
	if _, ok := pm.cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	pm.cfg.DefaultProfile = name
	return nil
}

// Delete removes a profile by name.
func (pm *ProfileManager) Delete(name string) error {
	if _, ok := pm.cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(pm.cfg.Profiles, name)
	if pm.cfg.DefaultProfile == name {
		pm.cfg.DefaultProfile = ""
	}
	return nil
}
