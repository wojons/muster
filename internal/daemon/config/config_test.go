package config

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	assert.Equal(t, "/var/run/muster/daemon.sock", cfg.SocketPath, "default socket path")
	assert.Equal(t, "/var/run/muster/daemon.pid", cfg.PIDFile, "default PID file")
	assert.Equal(t, "/var/log/muster/daemon.log", cfg.LogFile, "default log file")
	assert.Equal(t, "https://localhost:8080", cfg.TargetURL, "default target URL (HTTPS)")
	assert.Equal(t, os.FileMode(0660), cfg.SocketMode, "default socket mode")
	assert.Equal(t, 100, cfg.MaxConnections, "default max connections")
	assert.True(t, cfg.KeepAlive, "default keep alive")
	assert.Equal(t, 30*time.Second, cfg.KeepAlivePeriod, "default keep alive period")
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout, "default read timeout")
	assert.Equal(t, 30*time.Second, cfg.WriteTimeout, "default write timeout")
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout, "default shutdown timeout")
	assert.Equal(t, "info", cfg.LogLevel, "default log level")
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Default()
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_EmptySocketPath(t *testing.T) {
	cfg := Default()
	cfg.SocketPath = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "socket_path is required")
}

func TestConfig_Validate_EmptyTargetURL(t *testing.T) {
	cfg := Default()
	cfg.TargetURL = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_url is required")
}

func TestConfig_Validate_ZeroMaxConnections(t *testing.T) {
	cfg := Default()
	cfg.MaxConnections = 0
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_connections must be positive")
}

func TestConfig_Validate_NegativeMaxConnections(t *testing.T) {
	cfg := Default()
	cfg.MaxConnections = -1
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_connections must be positive")
}

func TestConfig_Validate_ZeroReadTimeout(t *testing.T) {
	cfg := Default()
	cfg.ReadTimeout = 0
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read_timeout must be positive")
}

func TestConfig_Validate_ZeroWriteTimeout(t *testing.T) {
	cfg := Default()
	cfg.WriteTimeout = 0
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write_timeout must be positive")
}

func TestConfig_String(t *testing.T) {
	cfg := Default()
	str := cfg.String()

	// Verify it contains key fields
	assert.Contains(t, str, "socket_path")
	assert.Contains(t, str, "/var/run/muster/daemon.sock")
	assert.Contains(t, str, "target_url")
	assert.Contains(t, str, "max_connections")
}

func TestConfig_Clone(t *testing.T) {
	original := Default()
	cloned := original.Clone()

	// Verify all fields are copied
	assert.Equal(t, original.SocketPath, cloned.SocketPath)
	assert.Equal(t, original.PIDFile, cloned.PIDFile)
	assert.Equal(t, original.LogFile, cloned.LogFile)
	assert.Equal(t, original.TargetURL, cloned.TargetURL)
	assert.Equal(t, original.SocketMode, cloned.SocketMode)
	assert.Equal(t, original.MaxConnections, cloned.MaxConnections)
	assert.Equal(t, original.KeepAlive, cloned.KeepAlive)
	assert.Equal(t, original.KeepAlivePeriod, cloned.KeepAlivePeriod)
	assert.Equal(t, original.ReadTimeout, cloned.ReadTimeout)
	assert.Equal(t, original.WriteTimeout, cloned.WriteTimeout)
	assert.Equal(t, original.ShutdownTimeout, cloned.ShutdownTimeout)
	assert.Equal(t, original.LogLevel, cloned.LogLevel)

	// Verify modification doesn't affect original
	cloned.SocketPath = "/modified.sock"
	assert.Equal(t, "/var/run/muster/daemon.sock", original.SocketPath)
	assert.Equal(t, "/modified.sock", cloned.SocketPath)
}

func TestConfig_Clone_Nil(t *testing.T) {
	var cfg *Config
	cloned := cfg.Clone()
	assert.Nil(t, cloned)
}

func TestIsValidLogLevel(t *testing.T) {
	// Valid log levels
	assert.True(t, IsValidLogLevel("debug"))
	assert.True(t, IsValidLogLevel("info"))
	assert.True(t, IsValidLogLevel("warn"))
	assert.True(t, IsValidLogLevel("error"))

	// Invalid log levels
	assert.False(t, IsValidLogLevel(""))
	assert.False(t, IsValidLogLevel("warning"))
	assert.False(t, IsValidLogLevel("trace"))
	assert.False(t, IsValidLogLevel("INFO"))
	assert.False(t, IsValidLogLevel("invalid"))
}

func TestConfig_JSONMarshaling(t *testing.T) {
	cfg := Default()

	// Test marshaling
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(data), "socket_path")
	assert.Contains(t, string(data), "max_connections")

	// Test unmarshaling
	var decoded Config
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, cfg.SocketPath, decoded.SocketPath)
	assert.Equal(t, cfg.MaxConnections, decoded.MaxConnections)
	assert.Equal(t, cfg.KeepAlive, decoded.KeepAlive)
}

func TestConfig_AllFields(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/custom/socket.sock",
		PIDFile:         "/custom/pid.pid",
		LogFile:         "/custom/log.log",
		TargetURL:       "https://example.com:9000",
		SocketMode:      0644,
		MaxConnections:  200,
		KeepAlive:       false,
		KeepAlivePeriod: 60 * time.Second,
		ReadTimeout:     45 * time.Second,
		WriteTimeout:    45 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		LogLevel:        "debug",
	}

	// Verify all 12 fields are set correctly
	assert.Equal(t, "/custom/socket.sock", cfg.SocketPath)
	assert.Equal(t, "/custom/pid.pid", cfg.PIDFile)
	assert.Equal(t, "/custom/log.log", cfg.LogFile)
	assert.Equal(t, "https://example.com:9000", cfg.TargetURL)
	assert.Equal(t, os.FileMode(0644), cfg.SocketMode)
	assert.Equal(t, 200, cfg.MaxConnections)
	assert.False(t, cfg.KeepAlive)
	assert.Equal(t, 60*time.Second, cfg.KeepAlivePeriod)
	assert.Equal(t, 45*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 45*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, "debug", cfg.LogLevel)

	// Verify validation passes
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestConfig_Validate_InsecureHTTP_Rejected(t *testing.T) {
	tests := []struct {
		name          string
		targetURL     string
		expectError   bool
		errorContains string
	}{
		{
			name:          "External HTTP rejected",
			targetURL:     "http://example.com:8080",
			expectError:   true,
			errorContains: "insecure HTTP URL detected",
		},
		{
			name:          "External HTTP without port rejected",
			targetURL:     "http://api.example.com",
			expectError:   true,
			errorContains: "insecure HTTP URL detected",
		},
		{
			name:          "HTTP with IP rejected",
			targetURL:     "http://192.168.1.100:8080",
			expectError:   true,
			errorContains: "insecure HTTP URL detected",
		},
		{
			name:        "Localhost HTTP allowed (development)",
			targetURL:   "http://localhost:8080",
			expectError: false,
		},
		{
			name:        "127.0.0.1 HTTP allowed (development)",
			targetURL:   "http://127.0.0.1:8080",
			expectError: false,
		},
		{
			name:        "HTTPS accepted",
			targetURL:   "https://example.com:8080",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.TargetURL = tt.targetURL

			err := cfg.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_ValidateDev_WarnsButAllows_HTTP(t *testing.T) {
	cfg := Default()
	cfg.TargetURL = "http://example.com:8080"

	// Should not error, but will print warning to stderr
	err := cfg.ValidateDev()
	require.NoError(t, err)
}

func TestConfig_IsProductionReady(t *testing.T) {
	tests := []struct {
		name       string
		targetURL  string
		expectSafe bool
	}{
		{
			name:       "HTTPS is production ready",
			targetURL:  "https://example.com:8080",
			expectSafe: true,
		},
		{
			name:       "HTTP localhost is NOT production ready",
			targetURL:  "http://localhost:8080",
			expectSafe: false,
		},
		{
			name:       "HTTP 127.0.0.1 is NOT production ready",
			targetURL:  "http://127.0.0.1:8080",
			expectSafe: false,
		},
		{
			name:       "HTTP external is NOT production ready",
			targetURL:  "http://api.example.com",
			expectSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.TargetURL = tt.targetURL

			result := cfg.IsProductionReady()
			assert.Equal(t, tt.expectSafe, result)
		})
	}
}

func TestConfig_isInsecureURL(t *testing.T) {
	tests := []struct {
		name      string
		targetURL string
		insecure  bool
	}{
		{
			name:      "External HTTP is insecure",
			targetURL: "http://example.com:8080",
			insecure:  true,
		},
		{
			name:      "Localhost HTTP is secure (exception)",
			targetURL: "http://localhost:8080",
			insecure:  false,
		},
		{
			name:      "127.0.0.1 HTTP is secure (exception)",
			targetURL: "http://127.0.0.1:8080",
			insecure:  false,
		},
		{
			name:      "HTTPS is secure",
			targetURL: "https://example.com:8080",
			insecure:  false,
		},
		{
			name:      "HTTP with subdomain localhost is secure (contains localhost)",
			targetURL: "http://test.localhost:8080",
			insecure:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TargetURL: tt.targetURL}
			result := cfg.isInsecureURL()
			assert.Equal(t, tt.insecure, result)
		})
	}
}
