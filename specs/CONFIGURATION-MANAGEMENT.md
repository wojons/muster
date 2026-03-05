# Configuration Management Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Configuration

## Overview

Complete specification of **configuration management** system.

## 1. Configuration Hierarchy

```
Defaults → Config File → Environment → Flags
   ↑            ↑            ↑          ↑
 Lowest      Low         Medium      Highest
```

## 2. Manager Structure

```go
type ConfigManager struct {
    config  *Config
    loader  *ConfigLoader
    profile string
    mu      sync.RWMutex
}

func NewConfigManager() (*ConfigManager, error) {
    loader := NewConfigLoader()
    config, err := loader.Load()
    if err != nil {
        return nil, err
    }
    
    return &ConfigManager{
        config:  config,
        loader:  loader,
        profile: config.DefaultProfile,
    }, nil
}
```

## 3. Profile Management

```go
func (cm *ConfigManager) SetProfile(name string) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if _, ok := cm.config.Profiles[name]; !ok {
        return fmt.Errorf("profile '%s' not found", name)
    }
    
    cm.profile = name
    return nil
}

func (cm *ConfigManager) GetActiveProfile() *Profile {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    return cm.config.Profiles[cm.profile]
}

func (cm *ConfigManager) CreateProfile(name string, profile Profile) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if cm.config.Profiles == nil {
        cm.config.Profiles = make(map[string]Profile)
    }
    
    cm.config.Profiles[name] = profile
    return cm.save()
}
```

## 4. Saving

```go
func (cm *ConfigManager) save() error {
    configPath := filepath.Join(getConfigDir(), "config.yaml")
    
    data, err := yaml.Marshal(cm.config)
    if err != nil {
        return err
    }
    
    return os.WriteFile(configPath, data, 0600)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
