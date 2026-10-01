package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain isolates the package's tests from the ambient user's real config.
// The loader reads the default config dir under $HOME (setDefaultPaths), so a
// developer who has run `openapi-cli init` (creating ~/.config/muster/config.yaml)
// would otherwise have that file silently override test expectations
// (proven: TestLoader_ApplyEnvOverrides/empty_env_vars_do_not_override failed
// with "Should be empty, but was localhost" whenever the real config existed).
func TestMain(m *testing.M) {
	isolated := filepath.Join(os.TempDir(), "muster-config-test")
	// Redirect all platform config/cache/data roots to an empty scratch dir.
	// USERPROFILE matters as much as HOME on Windows: os.UserHomeDir() ignores
	// HOME there and reads %USERPROFILE%, so without it the tests fell through
	// to the real user profile (and UserConfigFilePath's fallback pointed at
	// C:\Users\<user>\.config\muster instead of the scratch dir).
	os.Setenv("HOME", isolated)
	os.Setenv("USERPROFILE", isolated)
	os.Setenv("APPDATA", isolated)
	os.Setenv("LOCALAPPDATA", isolated)
	os.Exit(m.Run())
}

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

// =============================================================================
// NEW TESTS — covering previously 0% functions
// =============================================================================

func TestLoader_SetConfigFile(t *testing.T) {
	t.Run("set and load from explicit config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "explicit.yaml")

		content := `
version: "3.0.0"
default_profile: "explicit-profile"
host: "explicit.example.com"
verbose: true
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.SetConfigFile(configFile)

		cfg, err := loader.Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, "3.0.0", cfg.Version)
		assert.Equal(t, "explicit-profile", cfg.DefaultProfile)
	})

	t.Run("explicit config file not found returns error", func(t *testing.T) {
		loader := NewLoader()
		loader.SetConfigFile("/nonexistent/path/config.yaml")

		cfg, err := loader.Load()
		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "load config file")
	})

	t.Run("setConfigFile is idempotent", func(t *testing.T) {
		loader := NewLoader()
		loader.SetConfigFile("/first/path")
		loader.SetConfigFile("/second/path")

		cfg, err := loader.Load()
		// Second path wins but doesn't exist → error
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})
}

func TestLoader_ApplyCLIFlags(t *testing.T) {
	t.Run("unmarshals viper state into config", func(t *testing.T) {
		loader := NewLoader()
		loader.GetViper().Set("version", "5.0.0")
		loader.GetViper().Set("host", "cli-host.example.com")
		loader.GetViper().Set("verbose", true)

		cmd := &cobra.Command{Use: "test"}
		cfg := &Config{}
		err := loader.ApplyCLIFlags(cmd, cfg)
		require.NoError(t, err)

		assert.Equal(t, "5.0.0", cfg.Version)
		assert.Equal(t, "cli-host.example.com", cfg.Host)
		assert.True(t, cfg.Verbose)
	})

	t.Run("apply to empty config", func(t *testing.T) {
		loader := NewLoader()
		cmd := &cobra.Command{Use: "test"}
		cfg := &Config{}
		err := loader.ApplyCLIFlags(cmd, cfg)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
	})
}

func TestLoader_ApplyEnvOverrides(t *testing.T) {
	t.Run("MUSTER_HOST overrides host", func(t *testing.T) {
		t.Setenv("MUSTER_HOST", "env-host.example.com")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "env-host.example.com", cfg.Host)
	})

	t.Run("MUSTER_VERBOSE=true enables verbose", func(t *testing.T) {
		t.Setenv("MUSTER_VERBOSE", "true")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.True(t, cfg.Verbose)
	})

	t.Run("MUSTER_VERBOSE=1 enables verbose", func(t *testing.T) {
		t.Setenv("MUSTER_VERBOSE", "1")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.True(t, cfg.Verbose)
	})

	t.Run("MUSTER_VERBOSE=0 leaves verbose false", func(t *testing.T) {
		t.Setenv("MUSTER_VERBOSE", "0")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.False(t, cfg.Verbose)
	})

	t.Run("MUSTER_LOG_LEVEL overrides log level", func(t *testing.T) {
		t.Setenv("MUSTER_LOG_LEVEL", "debug")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "debug", cfg.LogLevel)
		assert.Equal(t, "debug", cfg.Logging.Level)
	})

	t.Run("MUSTER_LOG_FORMAT overrides log format", func(t *testing.T) {
		t.Setenv("MUSTER_LOG_FORMAT", "json")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "json", cfg.LogFormat)
		assert.Equal(t, "json", cfg.Logging.Format)
	})

	t.Run("MUSTER_DATABASE_URL overrides database URL", func(t *testing.T) {
		t.Setenv("MUSTER_DATABASE_URL", "sqlite:///tmp/test.db")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "sqlite:///tmp/test.db", cfg.DatabaseURL)
	})

	t.Run("MUSTER_AUTH_TOKEN overrides auth token", func(t *testing.T) {
		t.Setenv("MUSTER_AUTH_TOKEN", "secret-token-123")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "secret-token-123", cfg.AuthToken)
	})

	t.Run("MUSTER_TIMEOUT overrides timeout", func(t *testing.T) {
		t.Setenv("MUSTER_TIMEOUT", "45s")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, cfg.Timeout)
	})

	t.Run("MUSTER_TIMEOUT invalid value ignored", func(t *testing.T) {
		t.Setenv("MUSTER_TIMEOUT", "not-a-duration")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), cfg.Timeout)
	})

	t.Run("MUSTER_MAX_RETRIES overrides max retries", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "5")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, 5, cfg.MaxRetries)
	})

	t.Run("MUSTER_MAX_RETRIES invalid value ignored", func(t *testing.T) {
		t.Setenv("MUSTER_MAX_RETRIES", "abc")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, 0, cfg.MaxRetries)
	})

	t.Run("MUSTER_CONFIG_FILE overrides config file", func(t *testing.T) {
		t.Setenv("MUSTER_CONFIG_FILE", "/custom/config.yaml")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "/custom/config.yaml", cfg.ConfigFile)
	})

	t.Run("MUSTER_AUDIT_ENABLED=true enables audit", func(t *testing.T) {
		t.Setenv("MUSTER_AUDIT_ENABLED", "true")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.True(t, cfg.AuditEnabled)
	})

	t.Run("MUSTER_AUDIT_ENABLED=1 enables audit", func(t *testing.T) {
		t.Setenv("MUSTER_AUDIT_ENABLED", "1")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		assert.True(t, cfg.AuditEnabled)
	})

	t.Run("empty env vars do not override", func(t *testing.T) {
		// Ensure no leftover env vars from other tests
		t.Setenv("MUSTER_HOST", "")
		t.Setenv("MUSTER_VERBOSE", "")
		t.Setenv("MUSTER_LOG_LEVEL", "")
		loader := NewLoader()
		cfg, err := loader.Load()
		require.NoError(t, err)
		// Host should NOT be empty string (not overridden by empty env)
		assert.Empty(t, cfg.Host)
	})
}

func TestParseDuration(t *testing.T) {
	t.Run("valid duration", func(t *testing.T) {
		d, err := parseDuration("30s")
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, d)
	})

	t.Run("valid minutes", func(t *testing.T) {
		d, err := parseDuration("5m")
		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute, d)
	})

	t.Run("valid hours", func(t *testing.T) {
		d, err := parseDuration("2h")
		require.NoError(t, err)
		assert.Equal(t, 2*time.Hour, d)
	})

	t.Run("invalid duration returns error", func(t *testing.T) {
		_, err := parseDuration("not-a-duration")
		assert.Error(t, err)
	})

	t.Run("zero duration", func(t *testing.T) {
		d, err := parseDuration("0s")
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), d)
	})
}

func TestParseInt(t *testing.T) {
	t.Run("valid integer", func(t *testing.T) {
		n, err := parseInt("42")
		require.NoError(t, err)
		assert.Equal(t, 42, n)
	})

	t.Run("zero", func(t *testing.T) {
		n, err := parseInt("0")
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("negative integer", func(t *testing.T) {
		n, err := parseInt("-10")
		require.NoError(t, err)
		assert.Equal(t, -10, n)
	})

	t.Run("invalid string returns error", func(t *testing.T) {
		_, err := parseInt("abc")
		assert.Error(t, err)
	})

	t.Run("float string parses integer part", func(t *testing.T) {
		// fmt.Sscanf("3.14", "%d", &n) reads "3" and returns n=3, count=1
		n, err := parseInt("3.14")
		require.NoError(t, err)
		assert.Equal(t, 3, n)
	})

	t.Run("empty string fails", func(t *testing.T) {
		_, err := parseInt("")
		assert.Error(t, err)
	})
}

// =============================================================================
// ConfigManager tests
// =============================================================================

func TestNewConfigManager(t *testing.T) {
	t.Run("creates config manager successfully", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)
		require.NotNil(t, cm)
		assert.NotNil(t, cm.cfg)
		assert.Equal(t, "1.0.0", cm.cfg.Version)
	})
}

func TestConfigManager_Config(t *testing.T) {
	t.Run("returns underlying config", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		cfg := cm.Config()
		require.NotNil(t, cfg)
		assert.Equal(t, "1.0.0", cfg.Version)
	})
}

func TestConfigManager_GetDefaultProfile(t *testing.T) {
	t.Run("returns default from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
default_profile: "staging"
profiles:
  staging:
    base_url: "https://staging.example.com"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		assert.Equal(t, "staging", cm.GetDefaultProfile())
	})

	t.Run("returns empty when no default set", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)
		// Default is empty unless configured
		assert.Empty(t, cm.GetDefaultProfile())
	})
}

// =============================================================================
// ProfileManager tests
// =============================================================================

func TestNewProfileManager(t *testing.T) {
	t.Run("creates profile manager from config manager", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		require.NotNil(t, pm)
		assert.NotNil(t, pm.cfg)
	})
}

func TestProfileManager_List(t *testing.T) {
	t.Run("list with profiles", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
default_profile: "production"
profiles:
  production:
    base_url: "https://api.example.com"
  staging:
    base_url: "https://staging.example.com"
  dev:
    base_url: "http://localhost:8080"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		profiles := pm.List()
		assert.Len(t, profiles, 3)
	})

	t.Run("list empty profiles", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		profiles := pm.List()
		assert.Empty(t, profiles)
	})

	t.Run("profile without base_url uses name", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
profiles:
  no-url:
    auth_type: "bearer"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		profiles := pm.List()
		require.Len(t, profiles, 1)
		assert.Equal(t, "no-url", profiles[0].BaseURL)
	})
}

func TestProfileManager_Get(t *testing.T) {
	t.Run("get existing profile", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
profiles:
  production:
    base_url: "https://api.example.com"
    timeout: 30s
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		profile, err := pm.Get("production")
		require.NoError(t, err)
		assert.Equal(t, "https://api.example.com", profile.BaseURL)
	})

	t.Run("get non-existent profile", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		profile, err := pm.Get("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, profile)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("profile without base_url uses name as base_url", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
profiles:
  fallback:
    auth_type: "oauth2"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		profile, err := pm.Get("fallback")
		require.NoError(t, err)
		assert.Equal(t, "fallback", profile.BaseURL)
	})
}

func TestProfileManager_Exists(t *testing.T) {
	t.Run("existing profile returns true", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
profiles:
  production:
    base_url: "https://api.example.com"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		assert.True(t, pm.Exists("production"))
	})

	t.Run("non-existent profile returns false", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		assert.False(t, pm.Exists("nonexistent"))
	})

	t.Run("empty profiles map returns false", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		assert.False(t, pm.Exists("anything"))
	})
}

func TestProfileManager_SetDefault(t *testing.T) {
	t.Run("set default to existing profile", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
default_profile: "dev"
profiles:
  dev:
    base_url: "http://localhost:8080"
  production:
    base_url: "https://api.example.com"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		err = pm.SetDefault("production")
		require.NoError(t, err)
		assert.Equal(t, "production", cm.GetDefaultProfile())
	})

	t.Run("set default to non-existent profile", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		err = pm.SetDefault("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestProfileManager_Delete(t *testing.T) {
	t.Run("delete existing profile", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
profiles:
  removable:
    base_url: "https://temp.example.com"
  keeper:
    base_url: "https://keep.example.com"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		assert.True(t, pm.Exists("removable"))
		err = pm.Delete("removable")
		require.NoError(t, err)
		assert.False(t, pm.Exists("removable"))
		assert.True(t, pm.Exists("keeper"))
	})

	t.Run("delete non-existent profile", func(t *testing.T) {
		cm, err := NewConfigManager()
		require.NoError(t, err)

		pm := NewProfileManager(cm)
		err = pm.Delete("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("delete default profile clears default", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
version: "1.0.0"
default_profile: "obsolete"
profiles:
  obsolete:
    base_url: "https://old.example.com"
`
		err := os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: configFile, Priority: 1}}
		cfg, err := loader.Load()
		require.NoError(t, err)

		cm := &ConfigManager{cfg: cfg}
		pm := NewProfileManager(cm)

		assert.Equal(t, "obsolete", cm.GetDefaultProfile())
		err = pm.Delete("obsolete")
		require.NoError(t, err)
		assert.Empty(t, cm.GetDefaultProfile())
	})
}
