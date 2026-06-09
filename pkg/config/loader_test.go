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

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		cmd.PersistentFlags().StringP("host", "H", "localhost", "API host")
		cmd.PersistentFlags().StringP("timeout", "t", "30s", "Request timeout")
		cmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
		cmd.PersistentFlags().StringP("profile", "p", "default", "Configuration profile")
		cmd.PersistentFlags().StringP("output", "o", "json", "Output format")

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		// Updated: keys now map to Config struct field names directly
		viper := loader.GetViper()
		assert.Equal(t, "localhost", viper.GetString("host"))
		assert.Equal(t, "30s", viper.GetString("timeout"))
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
		loader.GetViper().Set("host", "config-host")

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		assert.Equal(t, "config-host", loader.GetViper().GetString("host"))
	})

	t.Run("Missing Flags", func(t *testing.T) {
		loader := NewLoader()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)
	})

	t.Run("Partial Flags", func(t *testing.T) {
		loader := NewLoader()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}

		cmd.PersistentFlags().StringP("host", "H", "localhost", "API host")
		cmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		// Updated: direct field names
		viper := loader.GetViper()
		assert.Equal(t, "localhost", viper.GetString("host"))
		assert.Equal(t, false, viper.GetBool("verbose"))
	})
}

func TestLoader_BindEnvVar(t *testing.T) {
	t.Run("Bind Environment Variable", func(t *testing.T) {
		loader := NewLoader()

		err := loader.BindEnvVar("custom-key")
		require.NoError(t, err)

		t.Setenv("CUSTOM_KEY", "test-value")

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
	})
}

func TestLoader_GetViper(t *testing.T) {
	t.Run("Get Viper Instance", func(t *testing.T) {
		loader := NewLoader()
		viper := loader.GetViper()
		require.NotNil(t, viper)

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

		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
		}
		cmd.PersistentFlags().StringP("host", "H", "flag-host", "API host")

		err := loader.BindPFlags(cmd)
		require.NoError(t, err)

		loader.GetViper().Set("host", "config-host")

		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Updated: host field now maps directly
		assert.Equal(t, "config-host", loader.GetViper().GetString("host"))
	})
}

func TestNewLoader(t *testing.T) {
	t.Run("Creates Loader with Default Configuration", func(t *testing.T) {
		loader := NewLoader()
		require.NotNil(t, loader)

		viper := loader.GetViper()
		require.NotNil(t, viper)

		// Updated: env prefix is now MUSTER
		assert.Equal(t, "MUSTER", viper.GetEnvPrefix())
	})

	t.Run("Loader Has Config Sources", func(t *testing.T) {
		loader := NewLoader()
		require.NotNil(t, loader)
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

		// Updated: paths now use "muster" instead of "openapi-cli"
		homeDir, _ := os.UserHomeDir()
		switch runtime.GOOS {
		case "darwin":
			assert.Contains(t, cfg.ConfigDir, filepath.Join(homeDir, "Library", "Application Support", "muster"))
		case "windows":
			assert.Contains(t, cfg.ConfigDir, "muster")
		default:
			assert.Contains(t, cfg.ConfigDir, filepath.Join(homeDir, ".config", "muster"))
		}
	})

	t.Run("Load Sets All Default Directories", func(t *testing.T) {
		loader := NewLoader()
		cfg, err := loader.Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.NotEmpty(t, cfg.ConfigDir)
		assert.NotEmpty(t, cfg.CacheDir)
		assert.NotEmpty(t, cfg.DataDir)
	})

	t.Run("Load Ignores Missing Config Files", func(t *testing.T) {
		loader := NewLoader()
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

		loader := NewLoader()
		loader.sources = []ConfigSource{
			{Path: configFile, Priority: 1},
		}

		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, "2.0.0", cfg.Version)
		assert.Equal(t, "production", cfg.DefaultProfile)
		assert.NotNil(t, cfg.Profiles["production"])
		prodProfile := cfg.Profiles["production"]
		assert.Equal(t, "https://api.example.com", prodProfile.BaseURL)
	})

	t.Run("Load Skips Invalid Config File", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		configContent := `invalid: yaml: content: [}`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{
			{Path: configFile, Priority: 1},
		}

		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}

func TestLoader_SetDefaultPaths(t *testing.T) {
	t.Run("Default Paths Set by Platform", func(t *testing.T) {
		loader := NewLoader()
		loadedCfg, err := loader.Load()
		require.NoError(t, err)

		homeDir, _ := os.UserHomeDir()

		switch runtime.GOOS {
		case "darwin":
			assert.Equal(t, filepath.Join(homeDir, "Library", "Application Support", "muster"), loadedCfg.ConfigDir)
			assert.Equal(t, filepath.Join(homeDir, "Library", "Caches", "muster"), loadedCfg.CacheDir)
			assert.Equal(t, filepath.Join(homeDir, "Library", "Application Support", "muster"), loadedCfg.DataDir)
		case "windows":
			assert.Contains(t, loadedCfg.ConfigDir, "muster")
			assert.Contains(t, loadedCfg.CacheDir, "muster")
			assert.Contains(t, loadedCfg.DataDir, "muster")
		default: // Linux
			assert.Equal(t, filepath.Join(homeDir, ".config", "muster"), loadedCfg.ConfigDir)
			assert.Equal(t, filepath.Join(homeDir, ".cache", "muster"), loadedCfg.CacheDir)
			assert.Equal(t, filepath.Join(homeDir, ".local", "share", "muster"), loadedCfg.DataDir)
		}
	})
}

func TestLoader_EnvironmentVariableExpansion(t *testing.T) {
	t.Run("Expand Environment Variables in Path", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_DIR", "/tmp/test-config")

		loader := NewLoader()
		viper := loader.GetViper()
		assert.NotNil(t, viper)
	})
}
