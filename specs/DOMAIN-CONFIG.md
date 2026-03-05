# Domain: Configuration

## Overview

Configuration entities define runtime settings, profiles for environment management, and authentication configurations.

## Entities

### Configuration

Root configuration entity.

```go
type Configuration struct {
    ID            string            `json:"id" validate:"required,uuid4"`
    Name          string            `json:"name" validate:"required,min=3,max=100"`
    Version       string            `json:"version" validate:"required,semver"`
    Environment   string            `json:"environment" validate:"required,oneof=development staging production"`
    Profiles      []Profile         `json:"profiles" validate:"dive"`
    Settings      GlobalSettings    `json:"settings"`
    Auth          AuthConfiguration `json:"auth"`
    Integrations  []Integration     `json:"integrations,omitempty" validate:"dive"`
    Secrets       SecretsConfig     `json:"secrets"`
    Metadata      ConfigMetadata    `json:"metadata"`
    CreatedAt     time.Time         `json:"created_at"`
    UpdatedAt     time.Time         `json:"updated_at"`
}

type ConfigMetadata struct {
    Author      string            `json:"author,omitempty" validate:"max=100"`
    Source      string            `json:"source,omitempty" validate:"omitempty,url"`
    Description string            `json:"description,omitempty" validate:"max=500"`
    Tags        []string          `json:"tags,omitempty" validate:"dive,max=50"`
}
```

### Profile

Environment profile configuration.

```go
type Profile struct {
    ID            string           `json:"id" validate:"required,uuid4"`
    Name          string           `json:"name" validate:"required,min=3,max=50,alphanum_underscore"`
    Description   string           `json:"description" validate:"max=500"`
    IsDefault     bool             `json:"is_default"`
    Server        ServerConfig     `json:"server" validate:"required"`
    Database      DatabaseConfig   `json:"database,omitempty"`
    Cache         CacheConfig      `json:"cache,omitempty"`
    Logging       LoggingConfig    `json:"logging"`
    Metrics       MetricsConfig    `json:"metrics,omitempty"`
    Tracing       TracingConfig    `json:"tracing,omitempty"`
    Features      FeatureFlags     `json:"features"`
    Overrides     map[string]any   `json:"overrides,omitempty"`
}

type ServerConfig struct {
    Host            string           `json:"host" validate:"required,hostname|ip"`
    Port            int              `json:"port" validate:"required,gte=1,lte=65535"`
    TLS             TLSConfig        `json:"tls,omitempty"`
    Timeout         TimeoutConfig    `json:"timeout"`
    RateLimit       RateLimitConfig  `json:"rate_limit,omitempty"`
    Cors            CORSConfig       `json:"cors,omitempty"`
    Compression     bool             `json:"compression"`
    MaxBodySize     int64            `json:"max_body_size" validate:"gte=0"`
    KeepAlive       bool             `json:"keep_alive"`
}

type TLSConfig struct {
    Enabled         bool             `json:"enabled"`
    CertPath        string           `json:"cert_path,omitempty" validate:"omitempty,file"`
    KeyPath         string           `json:"key_path,omitempty" validate:"omitempty,file"`
    MinVersion      string           `json:"min_version,omitempty" validate:"omitempty,oneof=1.0 1.1 1.2 1.3"`
    CipherSuites    []string         `json:"cipher_suites,omitempty"`
    ClientAuth      string           `json:"client_auth,omitempty" validate:"omitempty,oneof=none request require verify"`
}

type TimeoutConfig struct {
    Read       time.Duration `json:"read" validate:"required,gte=0"`
    Write      time.Duration `json:"write" validate:"required,gte=0"`
    Idle       time.Duration `json:"idle" validate:"required,gte=0"`
    Shutdown   time.Duration `json:"shutdown" validate:"required,gte=0"`
}

type RateLimitConfig struct {
    Enabled     bool          `json:"enabled"`
    Requests    int           `json:"requests" validate:"omitempty,gte=1"`
    Window      time.Duration `json:"window" validate:"omitempty,gte=0"`
    Burst       int           `json:"burst" validate:"omitempty,gte=1"`
}

type CORSConfig struct {
    Enabled          bool     `json:"enabled"`
    AllowOrigins     []string `json:"allow_origins,omitempty" validate:"dive,url"`
    AllowMethods     []string `json:"allow_methods,omitempty" validate:"dive,http_method"`
    AllowHeaders     []string `json:"allow_headers,omitempty" validate:"dive,max=100"`
    ExposeHeaders    []string `json:"expose_headers,omitempty" validate:"dive,max=100"`
    AllowCredentials bool     `json:"allow_credentials"`
    MaxAge           int      `json:"max_age" validate:"omitempty,gte=0"`
}
```

### Database and Cache Config

```go
type DatabaseConfig struct {
    Driver          string           `json:"driver" validate:"required,oneof=postgres mysql sqlite mongodb redis"`
    Host            string           `json:"host" validate:"required,hostname|ip"`
    Port            int              `json:"port" validate:"required,gte=1,lte=65535"`
    Database        string           `json:"database" validate:"required,max=100"`
    Username        string           `json:"username,omitempty" validate:"omitempty,max=100"`
    Password        string           `json:"-" validate:"omitempty,max=256"`
    SSLMode         string           `json:"ssl_mode,omitempty" validate:"omitempty,oneof=disable require verify-ca verify-full"`
    MaxConnections  int              `json:"max_connections" validate:"gte=0"`
    MaxIdleTime     time.Duration    `json:"max_idle_time" validate:"gte=0"`
    Timeout         time.Duration    `json:"timeout" validate:"gte=0"`
    Migrations      MigrationsConfig `json:"migrations,omitempty"`
}

type MigrationsConfig struct {
    Enabled     bool     `json:"enabled"`
    Path        string   `json:"path,omitempty" validate:"omitempty,dir"`
    TableName   string   `json:"table_name,omitempty" validate:"omitempty,max=100"`
}

type CacheConfig struct {
    Driver      string           `json:"driver" validate:"required,oneof=memory redis memcached"`
    Host        string           `json:"host,omitempty" validate:"omitempty,hostname|ip"`
    Port        int              `json:"port,omitempty" validate:"omitempty,gte=1,lte=65535"`
    TTL         time.Duration    `json:"ttl" validate:"gte=0"`
    MaxSize     int64            `json:"max_size" validate:"gte=0"`
    Prefix      string           `json:"prefix,omitempty" validate:"max=50"`
}
```

### Logging, Metrics, and Tracing

```go
type LoggingConfig struct {
    Level       string           `json:"level" validate:"required,oneof=debug info warn error fatal panic"`
    Format      string           `json:"format" validate:"required,oneof=json console"`
    Output      string           `json:"output" validate:"required,oneof=stdout stderr file"`
    FilePath    string           `json:"file_path,omitempty" validate:"omitempty,max=500"`
    Sampling    SamplingConfig   `json:"sampling,omitempty"`
}

type SamplingConfig struct {
    Enabled      bool    `json:"enabled"`
    Initial      int     `json:"initial" validate:"gte=0"`
    Thereafter   int     `json:"thereafter" validate:"gte=0"`
}

type MetricsConfig struct {
    Enabled     bool             `json:"enabled"`
    Port        int              `json:"port,omitempty" validate:"omitempty,gte=1,lte=65535"`
    Path        string           `json:"path,omitempty" validate:"omitempty,startswith=/"`
    Provider    string           `json:"provider,omitempty" validate:"omitempty,oneof=prometheus datadog cloudwatch"`
}

type TracingConfig struct {
    Enabled     bool             `json:"enabled"`
    Provider    string           `json:"provider,omitempty" validate:"omitempty,oneof=jaeger zipkin otel"`
    Endpoint    string           `json:"endpoint,omitempty" validate:"omitempty,url"`
    Sampling    float64          `json:"sampling" validate:"omitempty,gte=0,lte=1"`
}
```

### Authentication Settings

```go
type AuthConfiguration struct {
    Enabled         bool              `json:"enabled"`
    DefaultProvider string            `json:"default_provider,omitempty"`
    Providers       []AuthProvider    `json:"providers,omitempty" validate:"dive"`
    Sessions        SessionConfig     `json:"sessions,omitempty"`
    JWT             JWTConfig         `json:"jwt,omitempty"`
    APIKeys         APIKeyConfig      `json:"api_keys,omitempty"`
}

type AuthProvider struct {
    ID            string           `json:"id" validate:"required,uuid4"`
    Type          string           `json:"type" validate:"required,oneof=oauth2 saml ldap oidc local"`
    Name          string           `json:"name" validate:"required,max=100"`
    Enabled       bool             `json:"enabled"`
    ClientID      string           `json:"client_id,omitempty" validate:"omitempty,max=256"`
    ClientSecret  string           `json:"-" validate:"omitempty,max=256"`
    RedirectURL   string           `json:"redirect_url,omitempty" validate:"omitempty,url"`
    Scopes        []string         `json:"scopes,omitempty" validate:"dive,max=100"`
    AuthURL       string           `json:"auth_url,omitempty" validate:"omitempty,url"`
    TokenURL      string           `json:"token_url,omitempty" validate:"omitempty,url"`
    UserInfoURL   string           `json:"user_info_url,omitempty" validate:"omitempty,url"`
    Metadata      ProviderMetadata `json:"metadata,omitempty"`
}

type SessionConfig struct {
    Enabled        bool          `json:"enabled"`
    Secret         string        `json:"-" validate:"omitempty,len=32|len=64"`
    MaxAge         time.Duration `json:"max_age" validate:"gte=0"`
    Secure         bool          `json:"secure"`
    HttpOnly       bool          `json:"http_only"`
    SameSite       string        `json:"same_site,omitempty" validate:"omitempty,oneof=lax strict none"`
    Store          string        `json:"store,omitempty" validate:"omitempty,oneof=memory redis cookie"`
}

type JWTConfig struct {
    Enabled        bool          `json:"enabled"`
    Secret         string        `json:"secret,omitempty" validate:"omitempty,min=32,max=512"`
    Algorithm      string        `json:"algorithm,omitempty" validate:"omitempty,oneof=HS256 HS384 HS512 RS256 RS384 RS512 ES256 ES384 ES512"`
    Issuer         string        `json:"issuer,omitempty" validate:"omitempty,max=200"`
    Audience       string        `json:"audience,omitempty" validate:"omitempty,max=200"`
    Expiration     time.Duration `json:"expiration" validate:"gte=0"`
    RefreshExpiry  time.Duration `json:"refresh_expiry" validate:"gte=0"`
}

type APIKeyConfig struct {
    Enabled        bool          `json:"enabled"`
    Header         string        `json:"header,omitempty" validate:"omitempty,max=100"`
    Prefix         string        `json:"prefix,omitempty" validate:"omitempty,max=20"`
    RateLimit      int           `json:"rate_limit,omitempty" validate:"omitempty,gte=0"`
}
```

### Validation

```go
func (c *Configuration) Validate() error {
    if err := validator.Validate(c); err != nil {
        return fmt.Errorf("configuration validation failed: %w", err)
    }
    
    defaultCount := 0
    for _, profile := range c.Profiles {
        if profile.IsDefault {
            defaultCount++
        }
    }
    if defaultCount != 1 {
        return fmt.Errorf("exactly one profile must be default, found %d", defaultCount)
    }
    
    profileNames := make(map[string]bool)
    for _, profile := range c.Profiles {
        if profileNames[profile.Name] {
            return fmt.Errorf("duplicate profile name: %s", profile.Name)
        }
        profileNames[profile.Name] = true
    }
    
    if c.Auth.Enabled && len(c.Auth.Providers) == 0 {
        return errors.New("auth enabled but no providers configured")
    }
    
    return nil
}
```
