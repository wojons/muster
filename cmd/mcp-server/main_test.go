package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	cfg := loadConfig()
	require.NotNil(t, cfg, "config should not be nil")
	require.Equal(t, "info", cfg.LogLevel, "default log level should be info")
	require.Equal(t, "stderr", cfg.LogOutput, "default output should be stderr")
	require.True(t, cfg.EnableTools, "tools should be enabled")
	require.Equal(t, 10, cfg.MaxConcurrentCommands, "default max commands should be 10")
	require.Equal(t, 30*time.Second, cfg.CommandTimeout, "default timeout should be 30s")
}

func TestLoadAndRegisterTools_InvalidSpec(t *testing.T) {
	config := &MCPServerConfig{
		SpecPath: "/nonexistent/path/to/spec.yaml",
		BaseURL:  "",
	}

	err := loadAndRegisterTools(nil, config)
	require.Error(t, err, "should fail for invalid spec")
}
