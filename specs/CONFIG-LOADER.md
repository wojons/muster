# Config Loader Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Configuration

## Overview

Complete specification of **configuration file loader**.

## 1. Configuration Sources

```go
var ConfigSources = []ConfigSource{
    {Path: "/etc/openapi-cli/config.yaml", Priority: 1},
    {Path: "$HOME/.config/openapi-cli/config.yaml", Priority: 2},
    {Path: "$PWD/.openapi-cli.yaml", Priority: 3},
    {Path: "$OPENAPI_CLI_CONFIG", Priority: 4, Env: true},
}
```

## 2. Loader Implementation

```go
type ConfigLoader struct {
    sources []ConfigSource
    v       *viper.Viper
}

func NewConfigLoader() *ConfigLoader {
    v := viper.New()
    v.SetConfigType("yaml")
    
    return &ConfigLoader{
        sources: ConfigSources,
        v:       v,
    }
}

func (cl *ConfigLoader) Load() (*Config, error) {
    config := &Config{}
    
    for _, source := range cl.sources {
        if err := cl.loadSource(source, config); err != nil {
            continue
        }
    }
    
    return config, nil
}

func (cl *ConfigLoader) loadSource(source ConfigSource, config *Config) error {
    path := os.ExpandEnv(source.Path)
    
    if source.Env {
        if path == "" {
            return nil
        }
    }
    
    cl.v.SetConfigFile(path)
    
    if err := cl.v.ReadInConfig(); err != nil {
        return err
    }
    
    return cl.v.Unmarshal(config)
}
```

## 3. Configuration Structure

```go
type Config struct {
    DefaultProfile string            `mapstructure:"default_profile"`
    Profiles       map[string]Profile `mapstructure:"profiles"`
    Cache          CacheConfig       `mapstructure:"cache"`
    Auth           AuthConfig        `mapstructure:"auth"`
}

type Profile struct {
    BaseURL      string            `mapstructure:"base_url"`
    AuthType     string            `mapstructure:"auth_type"`
    AuthConfig   map[string]string `mapstructure:"auth_config"`
    DefaultFlags map[string]string `mapstructure:"default_flags"`
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
