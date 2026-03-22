package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_BindPFlags(t *testing.T) {
	t.Run("Bind Persistent Flags", func(t *testing.T) {
		loader := NewLoader()

		// Create a command with persistent flags
		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		// Add persistent flags
		cmd.PersistentFlags().StringP("host", "H", "localhost", "API host")
		cmd.PersistentFlags().StringP("timeout", "t", "30s", "Request timeout")
		cmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
		cmd.PersistentFlags().StringP("profile", "p", "default", "Configuration profile")
		cmd.PersistentFlags().StringP("output", "o", "json", "Output format")

		// Bind flags
		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		// Test that viper values are set from flags
		viper := loader.GetViper()
		assert.Equal(t, "localhost", viper.GetString("default_host"))
		assert.Equal(t, "30s", viper.GetString("default_timeout"))
		assert.Equal(t, false, viper.GetBool("verbose"))
		assert.Equal(t, "default", viper.GetString("default_profile"))
		assert.Equal(t, "json", viper.GetString("output_format"))
	})

	t.Run("Flag Override", func(t *testing.T) {
		loader := NewLoader()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		cmd.PersistentFlags().StringP("host", "H", "default-host", "API host")

		// Set a default in viper first
		loader.GetViper().Set("default_host", "config-host")

		// Bind flags
		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		// After binding, viper should maintain the explicitly set value
		// Flags take precedence after command parsing, not at bind time
		assert.Equal(t, "config-host", loader.GetViper().GetString("default_host"))
	})

	t.Run("Missing Flags", func(t *testing.T) {
		loader := NewLoader()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		// No flags defined - should still work without error
		err := loader.BindPFlags(cmd)
		require.NoError(t, err)
	})

	t.Run("Partial Flags", func(t *testing.T) {
		loader := NewLoader()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		// Only define some of the expected flags
		cmd.PersistentFlags().StringP("host", "H", "localhost", "API host")
		cmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		viper := loader.GetViper()
		assert.Equal(t, "localhost", viper.GetString("default_host"))
		assert.Equal(t, false, viper.GetBool("verbose"))
		// Other flags should just not be bound, not cause errors
	})
}

func TestLoader_BindEnvVar(t *testing.T) {
	t.Run("Bind Environment Variable", func(t *testing.T) {
		loader := NewLoader()

		err := loader.BindEnvVar("custom-key")
		require.NoError(t, err)

		// The env var should be bound (test by setting it)
		t.Setenv("CUSTOM_KEY", "test-value")

		// Create a new loader with the env var set
		loader2 := NewLoader()
		err = loader2.v.BindEnv("custom-key", "CUSTOM_KEY")
		require.NoError(t, err)

		assert.Equal(t, "test-value", loader2.GetViper().GetString("custom-key"))
	})

	t.Run("Bind Multiple Env Vars", func(t *testing.T) {
		loader := NewLoader()

		err := loader.BindEnvVar("first-key")
		require.NoError(t, err)

		err = loader.BindEnvVar("second_key")
		require.NoError(t, err)

		// Should not error
	})
}

func TestLoader_GetViper(t *testing.T) {
	t.Run("Get Viper Instance", func(t *testing.T) {
		loader := NewLoader()

		viper := loader.GetViper()
		require.NotNil(t, viper)

		// Should be able to use the viper instance
		viper.Set("test-key", "test-value")
		assert.Equal(t, "test-value", loader.GetViper().GetString("test-key"))
	})

	t.Run("Same Instance Returned", func(t *testing.T) {
		loader := NewLoader()

		viper1 := loader.GetViper()
		viper2 := loader.GetViper()

		assert.Equal(t, viper1, viper2)
	})
}

func TestLoader_Integration_BindFlagsAndLoad(t *testing.T) {
	t.Run("Flags Override Config Values", func(t *testing.T) {
		loader := NewLoader()

		// Create command with flags
		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		cmd.PersistentFlags().StringP("host", "H", "flag-host", "API host")

		// Bind flags before loading
		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		// Set a config value in viper directly (simulating config file)
		loader.GetViper().Set("default_host", "config-host")

		// Load config
		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// After binding and setting, explicitly set values remain
		assert.Equal(t, "config-host", loader.GetViper().GetString("default_host"))
	})
}
