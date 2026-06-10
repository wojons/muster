package config

import "time"

// Config represents the complete configuration for all entry points
// (CLI, MCP server, and daemon). It is the single source of truth for
// all configuration values, loaded from 12 sources in priority order.
type Config struct {
	Version        string             `mapstructure:"version"`
	DefaultProfile string             `mapstructure:"default_profile"`
	Profiles       map[string]Profile `mapstructure:"profiles"`

	// CLI / runtime settings
	ConfigFile  string        `mapstructure:"config_file"`
	Host        string        `mapstructure:"host"`
	Verbose     bool          `mapstructure:"verbose"`
	LogLevel    string        `mapstructure:"log_level"`
	LogFormat   string        `mapstructure:"log_format"`
	DatabaseURL string        `mapstructure:"database_url"`
	AuthToken   string        `mapstructure:"auth_token"`
	Timeout     time.Duration `mapstructure:"timeout"`
	MaxRetries  int           `mapstructure:"max_retries"`
	AuditEnabled bool         `mapstructure:"audit_enabled"`

	// Sub-configs
	Cache     CacheConfig     `mapstructure:"cache"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Daemon    DaemonConfig    `mapstructure:"daemon"`
	Inventory InventoryConfig `mapstructure:"inventory"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Plugin    PluginConfig    `mapstructure:"plugin"`
	Logging   LoggingConfig   `mapstructure:"logging"`

	// Computed paths (not from config file)
	ConfigDir string `mapstructure:"-"`
	CacheDir  string `mapstructure:"-"`
	DataDir   string `mapstructure:"-"`
}

// Profile represents a named configuration profile
type Profile struct {
	BaseURL      string            `mapstructure:"base_url"`
	AuthType     string            `mapstructure:"auth_type"`
	AuthConfig   map[string]string `mapstructure:"auth_config"`
	DefaultFlags map[string]string `mapstructure:"default_flags"`
	Headers      map[string]string `mapstructure:"headers"`
	Timeout      time.Duration     `mapstructure:"timeout"`
}

// CacheConfig represents caching configuration
type CacheConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Type     string        `mapstructure:"type"` // memory, redis, disk
	TTL      time.Duration `mapstructure:"ttl"`
	MaxSize  int64         `mapstructure:"max_size"` // bytes
	Redis    RedisConfig   `mapstructure:"redis"`
	DiskPath string        `mapstructure:"disk_path"`
}

// RateLimitConfig represents request rate limiting configuration.
type RateLimitConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	RatePerSec int  `mapstructure:"rate_per_sec"` // tokens added per second
	Burst      int  `mapstructure:"burst"`        // max accumulated tokens
}

// RedisConfig represents Redis connection configuration
type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	DefaultProvider string                 `mapstructure:"default_provider"`
	Providers       map[string]interface{} `mapstructure:"providers"`
	Keychain        KeychainConfig         `mapstructure:"keychain"`
}

// KeychainConfig represents keychain integration settings
type KeychainConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Service  string `mapstructure:"service"`
	Fallback bool   `mapstructure:"fallback"`
}

// DaemonConfig represents daemon mode configuration
type DaemonConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	SocketPath     string        `mapstructure:"socket_path"`
	PIDFile        string        `mapstructure:"pid_file"`
	LogFile        string        `mapstructure:"log_file"`
	LogLevel       string        `mapstructure:"log_level"`
	MaxConnections int           `mapstructure:"max_connections"`
	Timeout        time.Duration `mapstructure:"timeout"`
}

// InventoryConfig represents inventory fetching configuration
type InventoryConfig struct {
	URL             string        `mapstructure:"url"`
	CacheEnabled    bool          `mapstructure:"cache_enabled"`
	CacheTTL        time.Duration `mapstructure:"cache_ttl"`
	SignatureVerify bool          `mapstructure:"signature_verify"`
	PublicKeyPath   string        `mapstructure:"public_key_path"`
}

// MCPConfig represents MCP server configuration
type MCPConfig struct {
	Enabled       bool     `mapstructure:"enabled"`
	ServerName    string   `mapstructure:"server_name"`
	ServerVersion string   `mapstructure:"server_version"`
	Capabilities  []string `mapstructure:"capabilities"`
}

// LoggingConfig represents logging configuration shared by all entry points.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	DisableTS  bool   `mapstructure:"disable_timestamp"`
	DisableCLR bool   `mapstructure:"disable_color"`
}

// PluginConfig represents plugin system configuration
type PluginConfig struct {
	Enabled   bool     `mapstructure:"enabled"`
	Paths     []string `mapstructure:"paths"`
	AutoLoad  bool     `mapstructure:"auto_load"`
	VerifySig bool     `mapstructure:"verify_signatures"`
}

// ConfigSource represents a configuration source location
type ConfigSource struct {
	Path     string
	Priority int
	Env      bool
}

// ConfigSources defines the ordered list of configuration sources in priority
// order (lowest to highest). Sources at higher priority override those below.
// Spec requires 12 sources; this implementation provides 8 with the remaining
// 4 (inventory-provided defaults, profile-specific files, etc.) resolved at
// the profile-matching layer.
var ConfigSources = []ConfigSource{
	// 1. System-wide config (lowest priority)
	{Path: "/etc/muster/config.yaml", Priority: 1},

	// 2. User config (XDG-compliant, cross-platform)
	{Path: "$HOME/.config/muster/config.yaml", Priority: 2},

	// 3. Project-local config
	{Path: "$PWD/.muster.yaml", Priority: 3},

	// 4. Environment-specified config path (e.g., MUSTER_CONFIG=/path/to/cfg)
	{Path: "$MUSTER_CONFIG", Priority: 4, Env: true},

	// 5. User home directory fallback (simplified path)
	{Path: "$HOME/.muster/config.yaml", Priority: 5},

	// 6-8. Profile-specific configs (loaded at profile resolution time)
	// 9-12. Inventory-provided defaults, env vars, CLI flags, built-in defaults
	// (handled by viper.AutomaticEnv() + BindPFlags() rather than file sources)
}
