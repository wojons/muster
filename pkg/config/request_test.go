package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_RequestHTTPVersionDefault pins the default of the HTTP transport
// version policy (specs/PROTOCOLS.md §4.7 N4): "auto" when nothing configures
// it.
func TestLoad_RequestHTTPVersionDefault(t *testing.T) {
	// A config file that does not mention request.http_version must not lose
	// the default (loadFile replaces the struct with the parsed document).
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1.0.0\n"), 0o600))

	loader := NewLoader()
	loader.SetConfigFile(path)
	cfg, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Request.HTTPVersion)
}

// TestLoad_RequestHTTPVersionFromFile: the configured value survives loading.
func TestLoad_RequestHTTPVersionFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("request:\n  http_version: \"2\"\n"), 0o600))

	loader := NewLoader()
	loader.SetConfigFile(path)
	cfg, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "2", cfg.Request.HTTPVersion)
}

// TestLoad_RequestHTTPVersionEnvOverridesFile: MUSTER_HTTP_VERSION wins over
// the file value.
func TestLoad_RequestHTTPVersionEnvOverridesFile(t *testing.T) {
	t.Setenv("MUSTER_HTTP_VERSION", "3")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "3", cfg.Request.HTTPVersion)
}
