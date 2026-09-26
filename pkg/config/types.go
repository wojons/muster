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
	ConfigFile   string        `mapstructure:"config_file"`
	Host         string        `mapstructure:"host"`
	Verbose      bool          `mapstructure:"verbose"`
	LogLevel     string        `mapstructure:"log_level"`
	LogFormat    string        `mapstructure:"log_format"`
	DatabaseURL  string        `mapstructure:"database_url"`
	AuthToken    string        `mapstructure:"auth_token"`
	Timeout      time.Duration `mapstructure:"timeout"`
	MaxRetries   int           `mapstructure:"max_retries"`
	AuditEnabled bool          `mapstructure:"audit_enabled"`

	// Sub-configs
	Cache     CacheConfig     `mapstructure:"cache"`
	Request   RequestConfig   `mapstructure:"request"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Daemon    DaemonConfig    `mapstructure:"daemon"`
	Inventory InventoryConfig `mapstructure:"inventory"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Plugin    PluginConfig    `mapstructure:"plugin"`
	Logging   LoggingConfig   `mapstructure:"logging"`

	// Aliases maps a user-defined command word to the command line it
	// expands to (docs/guides/configuration.md, "Command Aliases"). The CLI
	// rewrites argv[1] with the mapped command before Cobra resolves it.
	Aliases map[string]string `mapstructure:"aliases"`

	// Computed paths (not from config file)
	ConfigDir string `mapstructure:"-"`
	CacheDir  string `mapstructure:"-"`
	DataDir   string `mapstructure:"-"`

	// PersistedCommandsPath overrides the default on-disk location used to
	// store and load generated command metadata. When empty, the loader
	// falls back to $XDG_DATA_HOME/openapi-cli/commands.json. Tests use
	// this to redirect persistence to a temp directory and avoid
	// polluting the production data directory.
	PersistedCommandsPath string `mapstructure:"-"`
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
	Redis    RedisConfig   `mapstructure:"redis"`
	Type     string        `mapstructure:"type"` // memory, redis, disk
	DiskPath string        `mapstructure:"disk_path"`
	MaxSize  int64         `mapstructure:"max_size"` // bytes
	TTL      time.Duration `mapstructure:"ttl"`
	Enabled  bool          `mapstructure:"enabled"`
}

// RateLimitConfig represents request rate limiting configuration.
type RateLimitConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	RatePerSec int  `mapstructure:"rate_per_sec"` // tokens added per second
	Burst      int  `mapstructure:"burst"`        // max accumulated tokens
}

// RequestConfig represents outbound HTTP request behaviour.
type RequestConfig struct {
	// HTTPVersion is the HTTP transport version policy
	// (specs/PROTOCOLS.md §4.7 N4): "auto" (default — negotiate
	// opportunistically, currently HTTP/2 over TLS), "1.1" or "2" (the named
	// version is required; a peer that offers another one fails with
	// CodeProtocolMismatch), or "3" (HTTP/3 preferred; requires an HTTP/3
	// transport, PROTO-015). The `request` verb's --http-version flag
	// overrides this value per invocation.
	HTTPVersion string `mapstructure:"http_version"`
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
	MTLS            MTLSConfig             `mapstructure:"mtls"`
}

// MTLSConfig represents mutual TLS configuration for the CLI.
type MTLSConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CertPath     string `mapstructure:"cert_path"`
	KeyPath      string `mapstructure:"key_path"`
	CAPath       string `mapstructure:"ca_path"`
	ServerName   string `mapstructure:"server_name"`
	InsecureSkip bool   `mapstructure:"insecure_skip"`
}

// KeychainConfig represents keychain integration settings
type KeychainConfig struct {
	Service  string `mapstructure:"service"`
	Enabled  bool   `mapstructure:"enabled"`
	Fallback bool   `mapstructure:"fallback"`
}

// DaemonConfig represents daemon mode configuration
type DaemonConfig struct {
	SocketPath     string        `mapstructure:"socket_path"`
	PIDFile        string        `mapstructure:"pid_file"`
	LogFile        string        `mapstructure:"log_file"`
	LogLevel       string        `mapstructure:"log_level"`
	Timeout        time.Duration `mapstructure:"timeout"`
	MaxConnections int           `mapstructure:"max_connections"`
	Enabled        bool          `mapstructure:"enabled"`
}

// InventoryConfig represents inventory fetching configuration
type InventoryConfig struct {
	URL             string        `mapstructure:"url"`
	PublicKeyPath   string        `mapstructure:"public_key_path"`
	CacheTTL        time.Duration `mapstructure:"cache_ttl"`
	CacheEnabled    bool          `mapstructure:"cache_enabled"`
	SignatureVerify bool          `mapstructure:"signature_verify"`
}

// MCPConfig represents MCP server configuration
type MCPConfig struct {
	ServerName    string   `mapstructure:"server_name"`
	ServerVersion string   `mapstructure:"server_version"`
	Capabilities  []string `mapstructure:"capabilities"`
	Enabled       bool     `mapstructure:"enabled"`
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
	Paths     []string `mapstructure:"paths"`
	Enabled   bool     `mapstructure:"enabled"`
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
