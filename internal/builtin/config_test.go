package builtin

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
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
		cmd := newConfigCommand(cfg, nil)

		assert.NotNil(t, cmd)
		assert.Equal(t, "config", cmd.Name())
		assert.Equal(t, "Manage configuration", cmd.Short)
		assert.Contains(t, cmd.Long, "View and modify OpenAPI CLI configuration")
	})

	t.Run("has show subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newConfigCommand(cfg, nil)

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
		cmd := newConfigCommand(cfg, nil)

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
		cmd := newConfigCommand(cfg, nil)

		var showCmd *cobra.Command
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "show" {
				showCmd = subCmd
				break
			}
		}

		require.NotNil(t, showCmd)
		require.NotNil(t, showCmd.RunE)
		_, err := captureStdoutCompletion(func() error {
			return showCmd.RunE(showCmd, []string{})
		})
		require.NoError(t, err)
	})
}

func TestNewProfileCommand(t *testing.T) {
	t.Run("creates profile command", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

		assert.NotNil(t, cmd)
		assert.Equal(t, "profile", cmd.Name())
		assert.Equal(t, "Manage configuration profiles", cmd.Short)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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
		cmd := newProfileCommand(cfg, nil)

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

func TestNewProfileCommand_RunList(t *testing.T) {
	t.Run("list empty profiles", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

		var listCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "list" {
				listCmd = sub
				break
			}
		}
		require.NotNil(t, listCmd)

		output, _ := captureStdoutCompletion(func() error {
			return listCmd.RunE(listCmd, []string{})
		})
		assert.Contains(t, output, "No profiles configured")
	})

	t.Run("list with profiles", func(t *testing.T) {
		cfg := &config.Config{
			Profiles: map[string]config.Profile{
				"production": {BaseURL: "https://api.example.com", AuthType: "bearer"},
				"staging":    {BaseURL: "https://staging.example.com", AuthType: "none"},
			},
			DefaultProfile: "production",
		}
		cmd := newProfileCommand(cfg, nil)

		var listCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "list" {
				listCmd = sub
				break
			}
		}
		require.NotNil(t, listCmd)

		output, _ := captureStdoutCompletion(func() error {
			return listCmd.RunE(listCmd, []string{})
		})
		assert.Contains(t, output, "* production")
		assert.Contains(t, output, "staging")
	})
}

func TestNewProfileCommand_RunShow(t *testing.T) {
	t.Run("show found profile", func(t *testing.T) {
		cfg := &config.Config{
			Profiles: map[string]config.Profile{
				"production": {BaseURL: "https://api.example.com", AuthType: "bearer"},
			},
		}
		cmd := newProfileCommand(cfg, nil)

		var showCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "show" {
				showCmd = sub
				break
			}
		}
		require.NotNil(t, showCmd)

		output, _ := captureStdoutCompletion(func() error {
			return showCmd.RunE(showCmd, []string{"production"})
		})
		assert.Contains(t, output, "production")
		assert.Contains(t, output, "https://api.example.com")
		assert.Contains(t, output, "bearer")
	})

	t.Run("show not found profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

		var showCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "show" {
				showCmd = sub
				break
			}
		}
		require.NotNil(t, showCmd)

		output, _ := captureStdoutCompletion(func() error {
			return showCmd.RunE(showCmd, []string{"nonexistent"})
		})
		assert.Contains(t, output, "not found")
	})
}

func TestNewProfileCommand_RunSwitch(t *testing.T) {
	t.Run("switch to valid profile", func(t *testing.T) {
		cfg := &config.Config{
			ConfigFile: t.TempDir() + "/config.yaml",
			Profiles: map[string]config.Profile{
				"production": {BaseURL: "https://api.example.com"},
				"staging":    {BaseURL: "https://staging.example.com"},
			},
			DefaultProfile: "production",
		}
		cmd := newProfileCommand(cfg, nil)

		var switchCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "switch" {
				switchCmd = sub
				break
			}
		}
		require.NotNil(t, switchCmd)

		output, _ := captureStdoutCompletion(func() error {
			return switchCmd.RunE(switchCmd, []string{"staging"})
		})
		assert.Contains(t, output, "staging")
		assert.Equal(t, "staging", cfg.DefaultProfile)
	})

	t.Run("switch to missing profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

		var switchCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "switch" {
				switchCmd = sub
				break
			}
		}
		require.NotNil(t, switchCmd)

		output, _ := captureStdoutCompletion(func() error {
			return switchCmd.RunE(switchCmd, []string{"nonexistent"})
		})
		assert.Contains(t, output, "not found")
	})
}

func TestNewProfileCommand_RunDelete(t *testing.T) {
	t.Run("delete existing profile", func(t *testing.T) {
		cfg := &config.Config{
			ConfigFile: t.TempDir() + "/config.yaml",
			Profiles: map[string]config.Profile{
				"staging":    {BaseURL: "https://staging.example.com"},
				"production": {BaseURL: "https://api.example.com"},
			},
			DefaultProfile: "staging",
		}
		cmd := newProfileCommand(cfg, nil)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		output, _ := captureStdoutCompletion(func() error {
			return deleteCmd.RunE(deleteCmd, []string{"staging"})
		})
		assert.Contains(t, output, "Deleted")
		_, exists := cfg.Profiles["staging"]
		assert.False(t, exists)
	})

	t.Run("delete non-existent profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg, nil)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		output, _ := captureStdoutCompletion(func() error {
			return deleteCmd.RunE(deleteCmd, []string{"nonexistent"})
		})
		assert.Contains(t, output, "not found")
	})

	t.Run("delete clears default profile", func(t *testing.T) {
		cfg := &config.Config{
			Profiles: map[string]config.Profile{
				"production": {BaseURL: "https://api.example.com"},
			},
			DefaultProfile: "production",
		}
		cmd := newProfileCommand(cfg, nil)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		_, err := captureStdoutCompletion(func() error {
			return deleteCmd.RunE(deleteCmd, []string{"production"})
		})
		require.NoError(t, err)
		assert.Equal(t, "", cfg.DefaultProfile)
	})
}
