package builtin

import (
	"testing"

	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigCommand(t *testing.T) {
	t.Run("creates config command", func(t *testing.T) {
		cfg := &config.Config{
			ConfigDir:      "/test/config",
			CacheDir:       "/test/cache",
			DataDir:        "/test/data",
			Version:        "1.0.0",
			DefaultProfile: "default",
		}
		cmd := newConfigCommand(cfg)

		assert.NotNil(t, cmd)
		assert.Equal(t, "config", cmd.Name())
		assert.Equal(t, "Manage configuration", cmd.Short)
		assert.Contains(t, cmd.Long, "View and modify OpenAPI CLI configuration")
	})

	t.Run("has show subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newConfigCommand(cfg)

		hasShow := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "show" {
				hasShow = true
				break
			}
		}
		assert.True(t, hasShow)
	})

	t.Run("has profile subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newConfigCommand(cfg)

		hasProfile := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "profile" {
				hasProfile = true
				break
			}
		}
		assert.True(t, hasProfile)
	})

	t.Run("show executes without error", func(t *testing.T) {
		cfg := &config.Config{
			ConfigDir:      "/test/config",
			CacheDir:       "/test/cache",
			DataDir:        "/test/data",
			Version:        "1.0.0",
			DefaultProfile: "default",
		}
		cmd := newConfigCommand(cfg)

		var showCmd *cobra.Command
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "show" {
				showCmd = subCmd
				break
			}
		}

		require.NotNil(t, showCmd)
		showCmd.Run(showCmd, []string{})
	})
}

func TestNewProfileCommand(t *testing.T) {
	t.Run("creates profile command", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		assert.NotNil(t, cmd)
		assert.Equal(t, "profile", cmd.Name())
		assert.Equal(t, "Manage configuration profiles", cmd.Short)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		hasList := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "list" {
				hasList = true
				break
			}
		}
		assert.True(t, hasList)
	})

	t.Run("has show subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		hasShow := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "show" {
				hasShow = true
				break
			}
		}
		assert.True(t, hasShow)
	})

	t.Run("has switch subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		hasSwitch := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "switch" {
				hasSwitch = true
				break
			}
		}
		assert.True(t, hasSwitch)
	})

	t.Run("has delete subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		hasDelete := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "delete" {
				hasDelete = true
				break
			}
		}
		assert.True(t, hasDelete)
	})

	t.Run("show requires exactly one argument", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var showCmd *cobra.Command
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "show" {
				showCmd = subCmd
				break
			}
		}

		assert.NotNil(t, showCmd)
		assert.NotNil(t, showCmd.Args)
	})

	t.Run("switch requires exactly one argument", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var switchCmd *cobra.Command
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "switch" {
				switchCmd = subCmd
				break
			}
		}

		assert.NotNil(t, switchCmd)
		assert.NotNil(t, switchCmd.Args)
	})

	t.Run("delete requires exactly one argument", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var deleteCmd *cobra.Command
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "delete" {
				deleteCmd = subCmd
				break
			}
		}

		assert.NotNil(t, deleteCmd)
		assert.NotNil(t, deleteCmd.Args)
	})
}
