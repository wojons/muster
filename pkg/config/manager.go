package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type ConfigManager struct {
	config   *Config
	loader   *Loader
	mu       sync.RWMutex
	filePath string
}

func NewConfigManager() (*ConfigManager, error) {
	loader := NewLoader()
	config, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	cm := &ConfigManager{
		config: config,
		loader: loader,
	}

	if config.CacheDir == "" {
		cm.setPlatformDefaults()
	}

	cm.filePath = filepath.Join(config.ConfigDir, "config.yaml")

	return cm, nil
}

func (cm *ConfigManager) setPlatformDefaults() {
	homeDir, _ := os.UserHomeDir()
	var configDir, cacheDir, dataDir string

	switch os.Getenv("GOOS") {
	case "darwin":
		configDir = filepath.Join(homeDir, "Library", "Application Support", "openapi-cli")
		cacheDir = filepath.Join(homeDir, "Library", "Caches", "openapi-cli")
		dataDir = filepath.Join(homeDir, "Library", "Application Support", "openapi-cli")
	case "windows":
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		configDir = filepath.Join(appData, "openapi-cli")
		cacheDir = filepath.Join(localAppData, "openapi-cli", "cache")
		dataDir = filepath.Join(appData, "openapi-cli", "data")
	default:
		configDir = filepath.Join(homeDir, ".config", "openapi-cli")
		cacheDir = filepath.Join(homeDir, ".cache", "openapi-cli")
		dataDir = filepath.Join(homeDir, ".local", "share", "openapi-cli")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.ConfigDir = configDir
	cm.config.CacheDir = cacheDir
	cm.config.DataDir = dataDir
}

func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

func (cm *ConfigManager) SetDefaultProfile(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.config.Profiles[name]; !ok {
		return fmt.Errorf("profile '%s' not found", name)
	}

	cm.config.DefaultProfile = name
	return cm.save()
}

func (cm *ConfigManager) GetDefaultProfile() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.DefaultProfile
}

func (cm *ConfigManager) AddProfile(name string, profile Profile) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.config.Profiles == nil {
		cm.config.Profiles = make(map[string]Profile)
	}

	if _, exists := cm.config.Profiles[name]; exists {
		return fmt.Errorf("profile '%s' already exists", name)
	}

	cm.config.Profiles[name] = profile
	return cm.save()
}

func (cm *ConfigManager) UpdateProfile(name string, profile Profile) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.config.Profiles[name]; !ok {
		return fmt.Errorf("profile '%s' not found", name)
	}

	cm.config.Profiles[name] = profile
	return cm.save()
}

func (cm *ConfigManager) GetProfile(name string) (*Profile, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	profile, ok := cm.config.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile '%s' not found", name)
	}

	return &profile, nil
}

func (cm *ConfigManager) DeleteProfile(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if name == "default" {
		return fmt.Errorf("cannot delete default profile")
	}

	if _, ok := cm.config.Profiles[name]; !ok {
		return fmt.Errorf("profile '%s' not found", name)
	}

	delete(cm.config.Profiles, name)
	return cm.save()
}

func (cm *ConfigManager) ListProfiles() []Profile {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	profiles := make([]Profile, 0, len(cm.config.Profiles))
	for _, p := range cm.config.Profiles {
		profiles = append(profiles, p)
	}

	return profiles
}

func (cm *ConfigManager) Set(key string, value interface{}) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	switch key {
	case "cache.enabled":
		if v, ok := value.(bool); ok {
			cm.config.Cache.Enabled = v
		}
	case "cache.ttl":
		if v, ok := value.(int); ok {
			cm.config.Cache.TTL = time.Duration(v)
		}
	}

	return cm.save()
}

func (cm *ConfigManager) GetConfigDir() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.ConfigDir
}

func (cm *ConfigManager) GetCacheDir() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.CacheDir
}

func (cm *ConfigManager) GetDataDir() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.DataDir
}

func (cm *ConfigManager) save() error {
	if cm.filePath == "" {
		cm.filePath = filepath.Join(cm.config.ConfigDir, "config.yaml")
	}

	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cm.filePath, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

func (cm *ConfigManager) Reload() error {
	loader := NewLoader()
	config, err := loader.Load()
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.config = config

	return nil
}
