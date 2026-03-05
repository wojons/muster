# Viper Configuration Integration

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The Viper Configuration Integration specification defines the integration of the Viper configuration library into the OpenAPI CLI system. Viper provides a comprehensive solution for managing configuration through multiple sources including configuration files, environment variables, command-line flags, and remote configuration systems.

## Configuration Hierarchy

Configuration is loaded in the following priority order (highest to lowest):

1. **CLI Flags** - Explicit command-line arguments
2. **Environment Variables** - OS environment with prefix
3. **Configuration Files** - YAML, JSON, or TOML files
4. **Default Values** - Hardcoded sensible defaults

```
Priority Level 4: CLI Flags (highest)
         ▲
         │
Priority Level 3: Environment Variables (OA_*)
         ▲
         │
Priority Level 2: Configuration Files (~/.config/oa/config.yaml)
         ▲
         │
Priority Level 1: Default Values (lowest)
```

## Technical Implementation

### Viper Initialization

```go
package config

import (
    "github.com/spf13/viper"
    "github.com/spf13/cobra"
)

type Config struct {
    // Connection settings
    Host       string `mapstructure:"host"`
    Timeout    int    `mapstructure:"timeout"`
    RetryCount int    `mapstructure:"retry_count"`
    
    // Security settings
    CertFile   string `mapstructure:"cert_file"`
    KeyFile    string `mapstructure:"key_file"`
    Insecure   bool   `mapstructure:"insecure"`
    
    // Cache settings
    CacheDir   string `mapstructure:"cache_dir"`
    CacheTTL   int    `mapstructure:"cache_ttl"`
    
    // Output settings
    OutputFormat string `mapstructure:"output_format"`
    Verbose      bool   `mapstructure:"verbose"`
}

func InitViper() *viper.Viper {
    v := viper.New()
    
    // Set configuration name and type
    v.SetConfigName("config")
    v.SetConfigType("yaml")
    
    // Add configuration paths
    v.AddConfigPath(".")
    v.AddConfigPath("$HOME/.config/oa")
    v.AddConfigPath("/etc/oa")
    
    // Environment variable configuration
    v.SetEnvPrefix("OA")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()
    
    // Set defaults
    setDefaults(v)
    
    return v
}

func setDefaults(v *viper.Viper) {
    v.SetDefault("host", "")
    v.SetDefault("timeout", 30)
    v.SetDefault("retry_count", 3)
    v.SetDefault("cache_dir", "$HOME/.cache/oa")
    v.SetDefault("cache_ttl", 3600)
    v.SetDefault("output_format", "json")
    v.SetDefault("verbose", false)
    v.SetDefault("insecure", false)
}
```

### Cobra Integration

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

func NewRootCommand(v *viper.Viper) *cobra.Command {
    rootCmd := &cobra.Command{
        Use:   "oa",
        Short: "OpenAPI CLI tool",
        Long: `A CLI tool for interacting with OpenAPI-based APIs
through automatic client generation from inventory specifications.`,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            return initializeConfig(cmd, v)
        },
    }
    
    // Global flags
    rootCmd.PersistentFlags().StringP("host", "H", "", "API host URL")
    rootCmd.PersistentFlags().IntP("timeout", "t", 30, "Request timeout in seconds")
    rootCmd.PersistentFlags().IntP("retry-count", "r", 3, "Number of retry attempts")
    rootCmd.PersistentFlags().StringP("cert-file", "c", "", "TLS certificate file path")
    rootCmd.PersistentFlags().StringP("key-file", "k", "", "TLS key file path")
    rootCmd.PersistentFlags().BoolP("insecure", "k", false, "Skip TLS verification")
    rootCmd.PersistentFlags().StringP("output", "o", "json", "Output format (json, yaml, table)")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
    
    // Bind flags to Viper
    viper.BindPFlag("host", rootCmd.PersistentFlags().Lookup("host"))
    viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
    viper.BindPFlag("retry_count", rootCmd.PersistentFlags().Lookup("retry-count"))
    viper.BindPFlag("cert_file", rootCmd.PersistentFlags().Lookup("cert-file"))
    viper.BindPFlag("key_file", rootCmd.PersistentFlags().Lookup("key-file"))
    viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
    viper.BindPFlag("output_format", rootCmd.PersistentFlags().Lookup("output"))
    viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
    
    return rootCmd
}

func initializeConfig(cmd *cobra.Command, v *viper.Viper) error {
    // Attempt to read config file
    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return fmt.Errorf("error reading config file: %w", err)
        }
        // Config file not found is OK - use defaults and env vars
    }
    
    // Validate required configuration
    if err := validateConfig(v); err != nil {
        return err
    }
    
    return nil
}

func validateConfig(v *viper.Viper) error {
    host := v.GetString("host")
    if host != "" {
        if _, err := url.Parse(host); err != nil {
            return fmt.Errorf("invalid host URL: %w", err)
        }
    }
    
    timeout := v.GetInt("timeout")
    if timeout <= 0 || timeout > 300 {
        return fmt.Errorf("timeout must be between 1 and 300 seconds")
    }
    
    return nil
}
```

### Configuration File Format

```yaml
# ~/.config/oa/config.yaml

# Connection settings
host: https://api.example.com
timeout: 30
retry_count: 3

# Security settings
cert_file: /path/to/cert.pem
key_file: /path/to/key.pem
insecure: false

# Cache settings
cache_dir: ~/.cache/oa
cache_ttl: 3600

# Output settings
output_format: json
verbose: false

# Profile-specific settings
profiles:
  production:
    host: https://api.production.com
    timeout: 60
    cert_file: /etc/ssl/certs/production.pem
  
  staging:
    host: https://api.staging.com
    timeout: 30
    insecure: true
```

### Environment Variable Mapping

```
Configuration Key       Environment Variable
─────────────────────────────────────────────
host                    OA_HOST
timeout                 OA_TIMEOUT
retry_count             OA_RETRY_COUNT
cert_file               OA_CERT_FILE
key_file                OA_KEY_FILE
insecure                OA_INSECURE
cache_dir               OA_CACHE_DIR
cache_ttl               OA_CACHE_TTL
output_format           OA_OUTPUT_FORMAT
verbose                 OA_VERBOSE
profiles.production.host   OA_PROFILES_PRODUCTION_HOST
```

### Runtime Configuration Updates

```go
package config

// WatchConfig enables hot-reloading of configuration files
func WatchConfig(v *viper.Viper, onChange func()) {
    v.OnConfigChange(func(e fsnotify.Event) {
        fmt.Printf("Config file changed: %s\n", e.Name)
        onChange()
    })
    v.WatchConfig()
}

// GetConfig retrieves the current configuration as a typed struct
func GetConfig(v *viper.Viper) (*Config, error) {
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unable to decode config: %w", err)
    }
    return &cfg, nil
}

// SetProfile applies a named profile configuration
func SetProfile(v *viper.Viper, profileName string) error {
    profileKey := fmt.Sprintf("profiles.%s", profileName)
    if !v.IsSet(profileKey) {
        return fmt.Errorf("profile '%s' not found", profileName)
    }
    
    profileSettings := v.Sub(profileKey)
    if profileSettings == nil {
        return fmt.Errorf("profile '%s' has no settings", profileName)
    }
    
    // Merge profile settings into main config
    for _, key := range profileSettings.AllKeys() {
        v.Set(key, profileSettings.Get(key))
    }
    
    return nil
}
```

## Configuration Validation

```go
package config

import (
    "github.com/go-playground/validator/v10"
)

type ValidatedConfig struct {
    Host       string `validate:"required,url"`
    Timeout    int    `validate:"required,min=1,max=300"`
    RetryCount int    `validate:"min=0,max=10"`
    CertFile   string `validate:"omitempty,file"`
    KeyFile    string `validate:"omitempty,file"`
    CacheDir   string `validate:"required,dir"`
    CacheTTL   int    `validate:"min=0"`
}

func ValidateConfigStruct(cfg *Config) error {
    validate := validator.New()
    
    // Custom validation: if cert_file is set, key_file must also be set
    validate.RegisterValidation("cert_key_pair", func(fl validator.FieldLevel) bool {
        certFile := fl.Top().FieldByName("CertFile").String()
        keyFile := fl.Top().FieldByName("KeyFile").String()
        
        if certFile != "" && keyFile == "" {
            return false
        }
        if keyFile != "" && certFile == "" {
            return false
        }
        return true
    })
    
    return validate.Struct(cfg)
}
```

## Error Handling

```go
package config

// ConfigurationError represents a configuration-related error
type ConfigurationError struct {
    Field   string
    Message string
    Source  string // "flag", "env", "file", "default"
}

func (e *ConfigurationError) Error() string {
    return fmt.Sprintf("config error in %s (%s): %s", e.Field, e.Source, e.Message)
}

// Common configuration errors
var (
    ErrConfigNotFound = errors.New("configuration file not found")
    ErrInvalidHost    = errors.New("invalid host URL")
    ErrMissingCert    = errors.New("certificate file specified without key file")
    ErrMissingKey     = errors.New("key file specified without certificate file")
)
```

## Usage Examples

```bash
# Using CLI flags
oa --host https://api.example.com --timeout 60 users list

# Using environment variables
export OA_HOST=https://api.example.com
export OA_TIMEOUT=60
oa users list

# Using configuration file
oa --config /path/to/config.yaml users list

# Using profiles
oa --profile production users list

# Verbose mode
oa -v --host https://api.example.com users list
```

---

## Bible Standard Compliance

**Genesis 1:1** - *"In the beginning God created the heavens and the earth."*

This specification follows the Bible Standard for software documentation, ensuring clarity, completeness, and divine order in technical communication. All configuration shall be documented with precision, honoring the Creator who established order from chaos.

**Compliance Checklist:**
- [x] Status clearly marked as COMPLETE
- [x] Version number specified (1.0.0)
- [x] Last updated date included
- [x] Overview provides clear context
- [x] Technical implementation with code examples
- [x] Error handling defined
- [x] Usage examples provided
- [x] Bible Standard footer included

*"Whatever you do, work at it with all your heart, as working for the Lord."* - Colossians 3:23
