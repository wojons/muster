package config

import "time"

// Config represents the complete configuration
type Config struct {
	Version        string             `mapstructure:"version"`
	DefaultProfile string             `mapstructure:"default_profile"`
	Profiles       map[string]Profile `mapstructure:"profiles"`
	Cache          CacheConfig        `mapstructure:"cache"`
	Auth           AuthConfig         `mapstructure:"auth"`
	Daemon         DaemonConfig       `mapstructure:"daemon"`
	Inventory      InventoryConfig    `mapstructure:"inventory"`
	MCP            MCPConfig          `mapstructure:"mcp"`
	Plugin         PluginConfig       `mapstructure:"plugin"`

	// Computed paths
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

// ConfigSources defines the ordered list of configuration sources
var ConfigSources = []ConfigSource{
	{Path: "/etc/openapi-cli/config.yaml", Priority: 1},
	{Path: "$HOME/.config/openapi-cli/config.yaml", Priority: 2},
	{Path: "$PWD/.openapi-cli.yaml", Priority: 3},
	{Path: "$OPENAPI_CLI_CONFIG", Priority: 4, Env: true},
}
