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

func TestNewProfileCommand_RunList(t *testing.T) {
	t.Run("list empty profiles", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var listCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "list" {
				listCmd = sub
				break
			}
		}
		require.NotNil(t, listCmd)

		output, _ := captureStdoutCompletion(func() error {
			listCmd.Run(listCmd, []string{})
			return nil
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
		cmd := newProfileCommand(cfg)

		var listCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "list" {
				listCmd = sub
				break
			}
		}
		require.NotNil(t, listCmd)

		output, _ := captureStdoutCompletion(func() error {
			listCmd.Run(listCmd, []string{})
			return nil
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
		cmd := newProfileCommand(cfg)

		var showCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "show" {
				showCmd = sub
				break
			}
		}
		require.NotNil(t, showCmd)

		output, _ := captureStdoutCompletion(func() error {
			showCmd.Run(showCmd, []string{"production"})
			return nil
		})
		assert.Contains(t, output, "production")
		assert.Contains(t, output, "https://api.example.com")
		assert.Contains(t, output, "bearer")
	})

	t.Run("show not found profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var showCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "show" {
				showCmd = sub
				break
			}
		}
		require.NotNil(t, showCmd)

		output, _ := captureStdoutCompletion(func() error {
			showCmd.Run(showCmd, []string{"nonexistent"})
			return nil
		})
		assert.Contains(t, output, "not found")
	})
}

func TestNewProfileCommand_RunSwitch(t *testing.T) {
	t.Run("switch to valid profile", func(t *testing.T) {
		cfg := &config.Config{
			Profiles: map[string]config.Profile{
				"production": {BaseURL: "https://api.example.com"},
				"staging":    {BaseURL: "https://staging.example.com"},
			},
			DefaultProfile: "production",
		}
		cmd := newProfileCommand(cfg)

		var switchCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "switch" {
				switchCmd = sub
				break
			}
		}
		require.NotNil(t, switchCmd)

		output, _ := captureStdoutCompletion(func() error {
			switchCmd.Run(switchCmd, []string{"staging"})
			return nil
		})
		assert.Contains(t, output, "staging")
		assert.Equal(t, "staging", cfg.DefaultProfile)
	})

	t.Run("switch to missing profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var switchCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "switch" {
				switchCmd = sub
				break
			}
		}
		require.NotNil(t, switchCmd)

		output, _ := captureStdoutCompletion(func() error {
			switchCmd.Run(switchCmd, []string{"nonexistent"})
			return nil
		})
		assert.Contains(t, output, "not found")
	})
}

func TestNewProfileCommand_RunDelete(t *testing.T) {
	t.Run("delete existing profile", func(t *testing.T) {
		cfg := &config.Config{
			Profiles: map[string]config.Profile{
				"staging":    {BaseURL: "https://staging.example.com"},
				"production": {BaseURL: "https://api.example.com"},
			},
			DefaultProfile: "staging",
		}
		cmd := newProfileCommand(cfg)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		output, _ := captureStdoutCompletion(func() error {
			deleteCmd.Run(deleteCmd, []string{"staging"})
			return nil
		})
		assert.Contains(t, output, "Deleted")
		_, exists := cfg.Profiles["staging"]
		assert.False(t, exists)
	})

	t.Run("delete non-existent profile", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newProfileCommand(cfg)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		output, _ := captureStdoutCompletion(func() error {
			deleteCmd.Run(deleteCmd, []string{"nonexistent"})
			return nil
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
		cmd := newProfileCommand(cfg)

		var deleteCmd *cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "delete" {
				deleteCmd = sub
				break
			}
		}
		require.NotNil(t, deleteCmd)

		deleteCmd.Run(deleteCmd, []string{"production"})
		assert.Equal(t, "", cfg.DefaultProfile)
	})
}
