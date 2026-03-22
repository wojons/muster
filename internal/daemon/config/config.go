package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds daemon configuration with 12 fields
type Config struct {
	SocketPath      string        `json:"socket_path"`
	PIDFile         string        `json:"pid_file"`
	LogFile         string        `json:"log_file"`
	TargetURL       string        `json:"target_url"`
	SocketMode      os.FileMode   `json:"socket_mode"`
	MaxConnections  int           `json:"max_connections"`
	KeepAlive       bool          `json:"keep_alive"`
	KeepAlivePeriod time.Duration `json:"keep_alive_period"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	LogLevel        string        `json:"log_level"`
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		SocketPath:      "/var/run/muster/daemon.sock",
		PIDFile:         "/var/run/muster/daemon.pid",
		LogFile:         "/var/log/muster/daemon.log",
		TargetURL:       "http://localhost:8080",
		SocketMode:      0660,
		MaxConnections:  100,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		LogLevel:        "info",
	}
}

// Validate checks configuration validity
func (c *Config) Validate() error {
	if c.SocketPath == "" {
		return fmt.Errorf("socket_path is required")
	}
	if c.TargetURL == "" {
		return fmt.Errorf("target_url is required")
	}
	if c.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive")
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	return nil
}

// String returns a JSON representation of the config
func (c *Config) String() string {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Sprintf("<Config error: %v>", err)
	}
	return string(data)
}

// Clone creates a deep copy of the Config
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	return &Config{
		SocketPath:      c.SocketPath,
		PIDFile:         c.PIDFile,
		LogFile:         c.LogFile,
		TargetURL:       c.TargetURL,
		SocketMode:      c.SocketMode,
		MaxConnections:  c.MaxConnections,
		KeepAlive:       c.KeepAlive,
		KeepAlivePeriod: c.KeepAlivePeriod,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		ShutdownTimeout: c.ShutdownTimeout,
		LogLevel:        c.LogLevel,
	}
}

// IsValidLogLevel checks if the log level is valid
func IsValidLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
