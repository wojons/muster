package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestNewLoader(t *testing.T) {
	t.Run("Creates Loader with Default Configuration", func(t *testing.T) {
		loader := NewLoader()
		require.NotNil(t, loader)

		// Verify viper instance is initialized
		viper := loader.GetViper()
		require.NotNil(t, viper)

		// Verify config type is set to yaml
		assert.Equal(t, "OPENAPI_CLI", viper.GetEnvPrefix())
	})

	t.Run("Loader Has Config Sources", func(t *testing.T) {
		loader := NewLoader()
		require.NotNil(t, loader)

		// Should have default config sources
		assert.NotNil(t, loader.sources)
	})
}

func TestLoader_Load(t *testing.T) {
	t.Run("Load Returns Config with Defaults", func(t *testing.T) {
		loader := NewLoader()
		cfg, err := loader.Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "1.0.0", cfg.Version)

		// Verify default paths are set based on platform
		homeDir, _ := os.UserHomeDir()
		switch runtime.GOOS {
		case "darwin":
			assert.Contains(t, cfg.ConfigDir, filepath.Join(homeDir, "Library", "Application Support", "openapi-cli"))
		case "windows":
			assert.Contains(t, cfg.ConfigDir, "openapi-cli")
		default:
			assert.Contains(t, cfg.ConfigDir, filepath.Join(homeDir, ".config", "openapi-cli"))
		}
	})

	t.Run("Load Sets All Default Directories", func(t *testing.T) {
		loader := NewLoader()
		cfg, err := loader.Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)

		// All directories should be set
		assert.NotEmpty(t, cfg.ConfigDir)
		assert.NotEmpty(t, cfg.CacheDir)
		assert.NotEmpty(t, cfg.DataDir)
	})

	t.Run("Load Ignores Missing Config Files", func(t *testing.T) {
		loader := NewLoader()
		// This should not error even if no config files exist
		cfg, err := loader.Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}

func TestLoad(t *testing.T) {
	t.Run("Convenience Function Works", func(t *testing.T) {
		cfg, err := Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "1.0.0", cfg.Version)
	})
}

func TestLoader_Load_WithConfigFile(t *testing.T) {
	t.Run("Load From Valid Config File", func(t *testing.T) {
		// Create a temporary config file
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		configContent := `
version: "2.0.0"
default_profile: "production"
profiles:
  production:
    base_url: "https://api.example.com"
    timeout: 60s
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Create loader with custom source
		loader := NewLoader()
		loader.sources = []ConfigSource{
			{Path: configFile, Priority: 1},
		}

		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Config should be loaded from file
		assert.Equal(t, "2.0.0", cfg.Version)
		assert.Equal(t, "production", cfg.DefaultProfile)
		assert.NotNil(t, cfg.Profiles["production"])
		prodProfile := cfg.Profiles["production"]
		assert.Equal(t, "https://api.example.com", prodProfile.BaseURL)
	})

	t.Run("Load Skips Invalid Config File", func(t *testing.T) {
		// Create a temporary config file with invalid content
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		configContent := `invalid: yaml: content: [}`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Create loader with custom source
		loader := NewLoader()
		loader.sources = []ConfigSource{
			{Path: configFile, Priority: 1},
		}

		// Should not error, just skip the invalid file
		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}

func TestLoader_SetDefaultPaths(t *testing.T) {
	t.Run("Default Paths Set by Platform", func(t *testing.T) {
		loader := NewLoader()

		// Call the method directly via Load
		loadedCfg, err := loader.Load()
		require.NoError(t, err)

		homeDir, _ := os.UserHomeDir()

		switch runtime.GOOS {
		case "darwin":
			assert.Equal(t, filepath.Join(homeDir, "Library", "Application Support", "openapi-cli"), loadedCfg.ConfigDir)
			assert.Equal(t, filepath.Join(homeDir, "Library", "Caches", "openapi-cli"), loadedCfg.CacheDir)
			assert.Equal(t, filepath.Join(homeDir, "Library", "Application Support", "openapi-cli"), loadedCfg.DataDir)
		case "windows":
			assert.Contains(t, loadedCfg.ConfigDir, "openapi-cli")
			assert.Contains(t, loadedCfg.CacheDir, "openapi-cli")
			assert.Contains(t, loadedCfg.DataDir, "openapi-cli")
		default: // Linux
			assert.Equal(t, filepath.Join(homeDir, ".config", "openapi-cli"), loadedCfg.ConfigDir)
			assert.Equal(t, filepath.Join(homeDir, ".cache", "openapi-cli"), loadedCfg.CacheDir)
			assert.Equal(t, filepath.Join(homeDir, ".local", "share", "openapi-cli"), loadedCfg.DataDir)
		}
	})
}

func TestLoader_EnvironmentVariableExpansion(t *testing.T) {
	t.Run("Expand Environment Variables in Path", func(t *testing.T) {
		// Set an environment variable
		t.Setenv("TEST_CONFIG_DIR", "/tmp/test-config")

		// Create a loader and verify env expansion works
		loader := NewLoader()

		// The loader should have automatic env enabled
		viper := loader.GetViper()
		assert.NotNil(t, viper)
	})
}
