# Configuration Schema Specification

## Overview

JSON Schema definitions for all configuration files with validation rules and Go struct mappings.

## 1. Main Configuration Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://openapi-cli.io/schemas/config.json",
  "title": "OpenAPI CLI Configuration",
  "type": "object",
  "properties": {
    "version": {
      "type": "string",
      "description": "Configuration file version"
    },
    "host": {
      "type": "string",
      "format": "uri",
      "description": "Default API host"
    },
    "inventory_url": {
      "type": "string",
      "format": "uri",
      "description": "Custom inventory URL"
    },
    "current_profile": {
      "type": "string",
      "description": "Active configuration profile"
    },
    "profiles": {
      "type": "object",
      "patternProperties": {
        "^[a-zA-Z0-9_-]+$": {
          "$ref": "#/$defs/profile"
        }
      },
      "additionalProperties": false
    },
    "tls": {
      "$ref": "#/$defs/tlsConfig"
    },
    "auth": {
      "$ref": "#/$defs/authConfig"
    },
    "cache": {
      "$ref": "#/$defs/cacheConfig"
    },
    "rate_limit": {
      "$ref": "#/$defs/rateLimitConfig"
    },
    "retry": {
      "$ref": "#/$defs/retryConfig"
    },
    "proxy": {
      "$ref": "#/$defs/proxyConfig"
    },
    "log_level": {
      "type": "string",
      "enum": ["debug", "info", "warn", "error"],
      "default": "info"
    },
    "fingerprinting": {
      "$ref": "#/$defs/fingerprintConfig"
    },
    "aliases": {
      "type": "object",
      "patternProperties": {
        "^[a-zA-Z0-9_-]+$": {
          "$ref": "#/$defs/alias"
        }
      }
    },
    "trusted_keys": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/trustedKey"
      }
    }
  },
  
  "$defs": {
    "profile": {
      "type": "object",
      "properties": {
        "host": {
          "type": "string",
          "format": "uri"
        },
        "version": {
          "type": "string"
        },
        "pinned": {
          "type": "boolean",
          "default": false
        },
        "headers": {
          "type": "object",
          "additionalProperties": {
            "type": "string"
          }
        },
        "auth": {
          "$ref": "#/$defs/authConfig"
        }
      },
      "required": ["host"]
    },
    
    "tlsConfig": {
      "type": "object",
      "properties": {
        "cert_file": {
          "type": "string",
          "description": "Path to client certificate"
        },
        "key_file": {
          "type": "string",
          "description": "Path to client key"
        },
        "ca_file": {
          "type": "string",
          "description": "Path to CA bundle"
        },
        "insecure": {
          "type": "boolean",
          "default": false,
          "description": "Skip TLS verification (dev only)"
        }
      }
    },
    
    "authConfig": {
      "type": "object",
      "properties": {
        "token": {
          "type": "string",
          "description": "Bearer token"
        },
        "token_env": {
          "type": "string",
          "description": "Environment variable containing token"
        },
        "api_key": {
          "type": "string"
        },
        "api_key_env": {
          "type": "string"
        },
        "oauth": {
          "type": "object",
          "properties": {
            "client_id": {
              "type": "string"
            },
            "client_secret": {
              "type": "string"
            },
            "client_secret_env": {
              "type": "string"
            },
            "token_url": {
              "type": "string",
              "format": "uri"
            },
            "scopes": {
              "type": "array",
              "items": {
                "type": "string"
              }
            }
          },
          "required": ["client_id", "token_url"]
        },
        "mtls": {
          "type": "boolean",
          "default": false
        }
      }
    },
    
    "cacheConfig": {
      "type": "object",
      "properties": {
        "dir": {
          "type": "string",
          "description": "Cache directory path"
        },
        "max_size": {
          "type": "integer",
          "description": "Maximum cache size in bytes",
          "minimum": 104857600,
          "default": 1073741824
        },
        "max_age": {
          "type": "string",
          "pattern": "^[0-9]+(d|h|m|s)$",
          "description": "Maximum cache age",
          "default": "30d"
        },
        "ttl": {
          "type": "string",
          "pattern": "^[0-9]+(d|h|m|s)$",
          "description": "Cache TTL",
          "default": "1h"
        }
      }
    },
    
    "rateLimitConfig": {
      "type": "object",
      "properties": {
        "enabled": {
          "type": "boolean",
          "default": true
        },
        "requests_per_min": {
          "type": "integer",
          "minimum": 1,
          "default": 100
        },
        "burst": {
          "type": "integer",
          "minimum": 1,
          "default": 10
        }
      }
    },
    
    "retryConfig": {
      "type": "object",
      "properties": {
        "max_attempts": {
          "type": "integer",
          "minimum": 1,
          "maximum": 10,
          "default": 3
        },
        "initial_delay": {
          "type": "string",
          "pattern": "^[0-9]+(ms|s|m)$",
          "default": "1s"
        },
        "max_delay": {
          "type": "string",
          "pattern": "^[0-9]+(ms|s|m)$",
          "default": "30s"
        },
        "backoff": {
          "type": "string",
          "enum": ["linear", "exponential", "exponential_jitter"],
          "default": "exponential_jitter"
        }
      }
    },
    
    "proxyConfig": {
      "type": "object",
      "properties": {
        "url": {
          "type": "string",
          "format": "uri"
        },
        "socks5": {
          "type": "boolean",
          "default": false
        }
      }
    },
    
    "fingerprintConfig": {
      "type": "object",
      "properties": {
        "enabled": {
          "type": "boolean",
          "default": true
        },
        "regenerate_on_ip_change": {
          "type": "boolean",
          "default": true
        },
        "components": {
          "type": "array",
          "items": {
            "enum": ["ip", "hardware", "random"]
          },
          "default": ["ip", "random"]
        }
      }
    },
    
    "alias": {
      "type": "object",
      "properties": {
        "host": {
          "type": "string",
          "format": "uri"
        },
        "version": {
          "type": "string"
        },
        "description": {
          "type": "string"
        }
      },
      "required": ["host"]
    },
    
    "trustedKey": {
      "type": "object",
      "properties": {
        "host": {
          "type": "string"
        },
        "method": {
          "type": "string",
          "enum": ["tls", "gpg", "ssh"]
        },
        "key": {
          "type": "string"
        },
        "pin": {
          "type": "string",
          "description": "Certificate pin hash"
        }
      },
      "required": ["host", "method"]
    }
  }
}
```

---

## 2. Environment Variables

```go
// internal/core/config/env.go

package config

import (
    "os"
    "strconv"
    "strings"
    "time"
)

// Environment variable mappings
var envMappings = []EnvMapping{
    // Core
    {EnvVar: "OPENAPICLI_HOST", ConfigPath: "host"},
    {EnvVar: "OPENAPICLI_INVENTORY_URL", ConfigPath: "inventory_url"},
    {EnvVar: "OPENAPICLI_CONFIG", ConfigPath: "config_file"},
    {EnvVar: "OPENAPICLI_LOG_LEVEL", ConfigPath: "log_level"},
    
    // TLS
    {EnvVar: "OPENAPICLI_CERT_FILE", ConfigPath: "tls.cert_file"},
    {EnvVar: "OPENAPICLI_KEY_FILE", ConfigPath: "tls.key_file"},
    {EnvVar: "OPENAPICLI_CA_FILE", ConfigPath: "tls.ca_file"},
    {EnvVar: "OPENAPICLI_INSECURE", ConfigPath: "tls.insecure", Type: "bool"},
    
    // Auth
    {EnvVar: "OPENAPICLI_TOKEN", ConfigPath: "auth.token"},
    {EnvVar: "OPENAPICLI_TOKEN_ENV", ConfigPath: "auth.token_env"},
    {EnvVar: "OPENAPICLI_API_KEY", ConfigPath: "auth.api_key"},
    {EnvVar: "OPENAPICLI_API_KEY_ENV", ConfigPath: "auth.api_key_env"},
    {EnvVar: "OPENAPICLI_OAUTH_CLIENT_ID", ConfigPath: "auth.oauth.client_id"},
    {EnvVar: "OPENAPICLI_OAUTH_CLIENT_SECRET", ConfigPath: "auth.oauth.client_secret"},
    {EnvVar: "OPENAPICLI_OAUTH_TOKEN_URL", ConfigPath: "auth.oauth.token_url"},
    
    // Cache
    {EnvVar: "OPENAPICLI_CACHE_DIR", ConfigPath: "cache.dir"},
    {EnvVar: "OPENAPICLI_CACHE_MAX_SIZE", ConfigPath: "cache.max_size", Type: "bytes"},
    {EnvVar: "OPENAPICLI_CACHE_TTL", ConfigPath: "cache.ttl", Type: "duration"},
    
    // Proxy
    {EnvVar: "HTTP_PROXY", ConfigPath: "proxy.url"},
    {EnvVar: "HTTPS_PROXY", ConfigPath: "proxy.url"},
    {EnvVar: "ALL_PROXY", ConfigPath: "proxy.url"},
    {EnvVar: "NO_PROXY", ConfigPath: "proxy.no_proxy"},
    
    // Rate limiting
    {EnvVar: "OPENAPICLI_RATE_LIMIT_ENABLED", ConfigPath: "rate_limit.enabled", Type: "bool"},
    {EnvVar: "OPENAPICLI_RATE_LIMIT_RPM", ConfigPath: "rate_limit.requests_per_min", Type: "int"},
    
    // Retry
    {EnvVar: "OPENAPICLI_RETRY_MAX_ATTEMPTS", ConfigPath: "retry.max_attempts", Type: "int"},
    {EnvVar: "OPENAPICLI_RETRY_INITIAL_DELAY", ConfigPath: "retry.initial_delay", Type: "duration"},
    {EnvVar: "OPENAPICLI_RETRY_MAX_DELAY", ConfigPath: "retry.max_delay", Type: "duration"},
}

type EnvMapping struct {
    EnvVar     string
    ConfigPath string
    Type       string // "string", "bool", "int", "bytes", "duration"
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() map[string]interface{} {
    config := make(map[string]interface{})
    
    for _, mapping := range envMappings {
        value := os.Getenv(mapping.EnvVar)
        if value == "" {
            continue
        }
        
        // Convert to appropriate type
        var typedValue interface{}
        switch mapping.Type {
        case "bool":
            typedValue = parseBool(value)
        case "int":
            typedValue = parseInt(value)
        case "bytes":
            typedValue = parseBytes(value)
        case "duration":
            typedValue = parseDuration(value)
        default:
            typedValue = value
        }
        
        // Set in config map at path
        setAtPath(config, mapping.ConfigPath, typedValue)
    }
    
    return config
}

func parseBool(s string) bool {
    s = strings.ToLower(s)
    return s == "true" || s == "1" || s == "yes"
}

func parseInt(s string) int {
    n, _ := strconv.Atoi(s)
    return n
}

func parseBytes(s string) int64 {
    // Parse "1GB", "500MB", etc.
    units := map[string]int64{
        "B":  1,
        "KB": 1024,
        "MB": 1024 * 1024,
        "GB": 1024 * 1024 * 1024,
    }
    
    for unit, multiplier := range units {
        if strings.HasSuffix(strings.ToUpper(s), unit) {
            num, _ := strconv.ParseInt(s[:len(s)-len(unit)], 10, 64)
            return num * multiplier
        }
    }
    
    n, _ := strconv.ParseInt(s, 10, 64)
    return n
}

func parseDuration(s string) time.Duration {
    d, _ := time.ParseDuration(s)
    return d
}

func setAtPath(config map[string]interface{}, path string, value interface{}) {
    parts := strings.Split(path, ".")
    current := config
    
    for i, part := range parts {
        if i == len(parts)-1 {
            current[part] = value
            return
        }
        
        if _, ok := current[part]; !ok {
            current[part] = make(map[string]interface{})
        }
        
        if m, ok := current[part].(map[string]interface{}); ok {
            current = m
        }
    }
}
```

---

## 3. CLI Flag Mappings

```go
// internal/core/config/flags.go

package config

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

// GlobalFlags defines flags available on all commands
func AddGlobalFlags(cmd *cobra.Command) {
    // Core
    cmd.PersistentFlags().String("host", "", "API host URL")
    cmd.PersistentFlags().String("inventory-url", "", "Custom inventory URL")
    cmd.PersistentFlags().String("config", "", "Config file path")
    cmd.PersistentFlags().String("profile", "", "Configuration profile")
    
    // TLS
    cmd.PersistentFlags().String("cert", "", "Client certificate file")
    cmd.PersistentFlags().String("key", "", "Client key file")
    cmd.PersistentFlags().String("ca", "", "CA certificate file")
    cmd.PersistentFlags().Bool("insecure", false, "Skip TLS verification (dev only)")
    
    // Auth
    cmd.PersistentFlags().String("token", "", "Bearer token")
    cmd.PersistentFlags().String("api-key", "", "API key")
    cmd.PersistentFlags().String("oauth-client-id", "", "OAuth client ID")
    cmd.PersistentFlags().String("oauth-client-secret", "", "OAuth client secret")
    
    // Cache
    cmd.PersistentFlags().String("cache-dir", "", "Cache directory")
    cmd.PersistentFlags().Duration("cache-ttl", 0, "Cache TTL")
    cmd.PersistentFlags().Bool("no-cache", false, "Disable caching")
    cmd.PersistentFlags().Bool("offline", false, "Use cached data only")
    
    // Execution
    cmd.PersistentFlags().Duration("timeout", 30*time.Second, "Request timeout")
    cmd.PersistentFlags().Int("retry", 3, "Max retry attempts")
    cmd.PersistentFlags().Duration("retry-delay", 1*time.Second, "Initial retry delay")
    
    // Output
    cmd.PersistentFlags().String("output", "auto", "Output format (auto, json, yaml, table)")
    cmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
    cmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
    
    // Security
    cmd.PersistentFlags().Bool("no-fingerprint", false, "Disable fingerprinting")
    cmd.PersistentFlags().String("fingerprint", "", "Custom fingerprint")
    
    // Version
    cmd.PersistentFlags().String("version", "", "Use specific inventory version")
    cmd.PersistentFlags().Bool("use-known-good", false, "Use last known good version")
    cmd.PersistentFlags().Bool("use-latest", false, "Force fetch latest version")
    
    // Proxy
    cmd.PersistentFlags().String("proxy", "", "HTTP proxy URL")
    cmd.PersistentFlags().Bool("socks5", false, "Use SOCKS5 proxy")
}

// BindFlags binds viper to cobra flags
func BindFlags() {
    viper.BindPFlag("host", rootCmd.PersistentFlags().Lookup("host"))
    viper.BindPFlag("inventory_url", rootCmd.PersistentFlags().Lookup("inventory-url"))
    viper.BindPFlag("tls.cert_file", rootCmd.PersistentFlags().Lookup("cert"))
    viper.BindPFlag("tls.key_file", rootCmd.PersistentFlags().Lookup("key"))
    viper.BindPFlag("auth.token", rootCmd.PersistentFlags().Lookup("token"))
    viper.BindPFlag("cache.dir", rootCmd.PersistentFlags().Lookup("cache-dir"))
    viper.BindPFlag("log_level", rootCmd.PersistentFlags().Lookup("debug"))
}
```

---

## 4. Configuration Hierarchy

```go
// internal/core/config/loader.go

package config

import (
    "github.com/knadh/koanf"
    "github.com/knadh/koanf/parsers/yaml"
    "github.com/knadh/koanf/providers/file"
    "github.com/knadh/koanf/providers/env"
)

// LoadOrder defines the configuration loading order
// Higher precedence overrides lower
var LoadOrder = []Loader{
    // 1. Built-in defaults (lowest precedence)
    &DefaultLoader{},
    
    // 2. Inventory-provided defaults
    &InventoryLoader{},
    
    // 3. System config (/etc/openapi-cli/config.yaml)
    &SystemConfigLoader{
        Path: "/etc/openapi-cli/config.yaml",
    },
    
    // 4. User config (~/.config/openapi-cli/config.yaml)
    &UserConfigLoader{
        Path: "~/.config/openapi-cli/config.yaml",
    },
    
    // 5. Project config (./.openapi-cli.yaml)
    &ProjectConfigLoader{
        Path: ".openapi-cli.yaml",
    },
    
    // 6. Environment variables
    &EnvLoader{},
    
    // 7. CLI flags (highest precedence)
    &FlagLoader{},
}

// LoadConfiguration loads configuration from all sources
func LoadConfiguration() (*Configuration, error) {
    k := koanf.New(".")
    
    // Load in order
    for _, loader := range LoadOrder {
        if err := loader.Load(k); err != nil {
            // Some loaders are optional, log but continue
            if !loader.Optional() {
                return nil, err
            }
        }
    }
    
    // Unmarshal to struct
    var cfg Configuration
    if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
        Tag: "yaml",
    }); err != nil {
        return nil, err
    }
    
    // Apply profile if specified
    if cfg.CurrentProfile != "" {
        if profile, ok := cfg.Profiles[cfg.CurrentProfile]; ok {
            cfg = mergeWithProfile(cfg, profile)
        }
    }
    
    // Validate
    if err := Validate(&cfg); err != nil {
        return nil, err
    }
    
    return &cfg, nil
}

// Validate validates configuration
func Validate(cfg *Configuration) error {
    // Required fields
    if cfg.Host == "" && cfg.InventoryURL == "" {
        return fmt.Errorf("host or inventory_url required")
    }
    
    // Path validation
    if cfg.TLS.CertFile != "" && !fileExists(cfg.TLS.CertFile) {
        return fmt.Errorf("cert file not found: %s", cfg.TLS.CertFile)
    }
    
    // URL validation
    if cfg.Host != "" {
        if _, err := url.Parse(cfg.Host); err != nil {
            return fmt.Errorf("invalid host URL: %w", err)
        }
    }
    
    // Range validation
    if cfg.RateLimit.RequestsPerMin < 1 {
        return fmt.Errorf("rate_limit.requests_per_min must be >= 1")
    }
    
    if cfg.Retry.MaxAttempts < 1 || cfg.Retry.MaxAttempts > 10 {
        return fmt.Errorf("retry.max_attempts must be between 1 and 10")
    }
    
    // Security validation
    if cfg.TLS.Insecure {
        log.Warn("TLS verification disabled - this is insecure!")
    }
    
    return nil
}
```

---

## 5. Example Configuration Files

### Minimal Config

```yaml
# ~/.config/openapi-cli/config.yaml
host: https://api.example.com
```

### Full Config

```yaml
# ~/.config/openapi-cli/config.yaml
version: "1.0"

# Default settings
host: https://api.example.com
log_level: info

# TLS configuration
tls:
  cert_file: ~/.certs/client.crt
  key_file: ~/.certs/client.key
  ca_file: ~/.certs/ca.crt

# Authentication
auth:
  token_env: API_TOKEN
  oauth:
    client_id: my-client
    token_url: https://auth.example.com/token
    scopes:
      - api:read
      - api:write

# Cache settings
cache:
  dir: ~/.cache/openapi-cli
  max_size: 1073741824  # 1GB
  ttl: 1h

# Rate limiting
rate_limit:
  enabled: true
  requests_per_min: 100
  burst: 10

# Retry configuration
retry:
  max_attempts: 3
  initial_delay: 1s
  max_delay: 30s
  backoff: exponential_jitter

# Proxy settings
proxy:
  url: http://proxy.example.com:8080

# Fingerprinting
fingerprinting:
  enabled: true
  regenerate_on_ip_change: true
  components:
    - ip
    - random

# Profiles
profiles:
  production:
    host: https://api.example.com
    auth:
      token_env: PROD_API_TOKEN
  
  staging:
    host: https://staging-api.example.com
    auth:
      token_env: STAGING_API_TOKEN
    tls:
      insecure: true  # Staging only
  
  development:
    host: http://localhost:8080
    cache:
      ttl: 5m
    rate_limit:
      enabled: false

# Aliases
aliases:
  prod:
    host: https://api.example.com
    version: "2.1.0"
    description: "Production environment"
  
  staging:
    host: https://staging-api.example.com

# Trusted keys for signature verification
trusted_keys:
  - host: api.example.com
    method: tls
    pin: "sha256//abc123..."
  
  - host: api.enterprise.com
    method: gpg
    key: |
      -----BEGIN PGP PUBLIC KEY BLOCK-----
      ...
      -----END PGP PUBLIC KEY BLOCK-----
```

---

## 6. Configuration Validation

```go
// internal/core/config/validate.go

package config

import (
    "fmt"
    "os"
    "path/filepath"
    
    "github.com/xeipuuv/gojsonschema"
)

// JSONSchema is the compiled schema
var JSONSchema *gojsonschema.Schema

func init() {
    schemaData := []byte(schemaJSON) // Schema from above
    schemaLoader := gojsonschema.NewBytesLoader(schemaData)
    JSONSchema, _ = gojsonschema.NewSchema(schemaLoader)
}

// ValidateFile validates a configuration file
func ValidateFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("failed to read config: %w", err)
    }
    
    documentLoader := gojsonschema.NewBytesLoader(data)
    result, err := JSONSchema.Validate(documentLoader)
    if err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    if !result.Valid() {
        var errors []string
        for _, err := range result.Errors() {
            errors = append(errors, fmt.Sprintf("%s: %s", err.Field(), err.Description()))
        }
        return fmt.Errorf("config validation failed:\n  %s", strings.Join(errors, "\n  "))
    }
    
    return nil
}

// ValidateEnv validates environment variables
func ValidateEnv() error {
    // Check for conflicting settings
    if os.Getenv("OPENAPICLI_TOKEN") != "" && os.Getenv("OPENAPICLI_TOKEN_ENV") != "" {
        return fmt.Errorf("OPENAPICLI_TOKEN and OPENAPICLI_TOKEN_ENV both set - use only one")
    }
    
    // Check for insecure settings in production
    if os.Getenv("OPENAPICLI_INSECURE") == "true" {
        if os.Getenv("ENV") == "production" {
            return fmt.Errorf("OPENAPICLI_INSECURE=true in production is not allowed")
        }
    }
    
    return nil
}
```

---

## Deterministic Guarantees

This configuration schema specification must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Schema Validation

Schema validation must produce identical results across platforms:

- **JSON Schema validation**: Same validation errors for same configuration
- **Type coercion**: Identical type conversion rules (string→int, string→bool, etc.)
- **Default value injection**: Deterministic default value application order

### 2. Platform‑Independent Environment Variable Mapping

Environment variable handling must be platform‑independent:

- **Case‑insensitive lookup**: Windows environment variable case‑insensitivity (see `PLATFORM-BEHAVIOR.md` section 3.1)
- **Variable precedence**: Identical precedence resolution across platforms
- **Value normalization**: Same whitespace trimming, same encoding handling

### 3. Deterministic CLI Flag Parsing

CLI flag parsing must be deterministic:

- **Flag ordering**: Same flag processing order across platforms
- **Value parsing**: Identical type parsing (duration, size, etc.)
- **Error messages**: Same error messages for same invalid input

### 4. Cross‑Language Schema Consistency

When generating SDKs for 12 languages, configuration schemas must:

- **Same JSON Schema**: Identical schema definitions across languages
- **Same validation logic**: Identical validation error messages
- **Same default values**: Identical default value injection

### 5. Testing Determinism

All configuration schema tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestConfigurationSchemaDeterminism(t *testing.T) {
	// Verify identical schema validation
	// Verify identical environment variable mapping
	// Verify identical CLI flag parsing
}
```

### 6. Integration with SDK Generation

Configuration schemas must integrate deterministically with 12 language SDKs:

- **Schema generation**: Identical schema export across languages
- **Validation integration**: Identical validation hook points
- **Error reporting**: Identical error format and reporting

**Verification**: Run `./scripts/validate‑determinism‑config‑schema.sh` to confirm cross‑platform consistency.

This configuration schema specification provides complete JSON Schema definitions, environment variable mappings, CLI flag mappings, and validation rules for Ralph Loop.
