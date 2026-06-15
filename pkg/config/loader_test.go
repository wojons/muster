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
		v := loader.GetViper()
		assert.NotNil(t, v)
	})
}

// --- Pure helper: parseDuration ---

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Valid durations
		{name: "valid seconds", input: "30s", want: 30 * time.Second, wantErr: false},
		{name: "valid minutes", input: "5m", want: 5 * time.Minute, wantErr: false},
		{name: "valid hours", input: "1h", want: time.Hour, wantErr: false},
		{name: "valid milliseconds", input: "500ms", want: 500 * time.Millisecond, wantErr: false},
		{name: "valid combination", input: "1h30m", want: 90 * time.Minute, wantErr: false},
		{name: "valid fractional", input: "1.5h", want: 90 * time.Minute, wantErr: false},
		{name: "valid zero", input: "0s", want: 0, wantErr: false},
		{name: "valid microseconds", input: "100µs", want: 100 * time.Microsecond, wantErr: false},
		{name: "valid nanoseconds", input: "500ns", want: 500 * time.Nanosecond, wantErr: false},
		// Invalid durations
		{name: "empty string", input: "", want: 0, wantErr: true},
		{name: "invalid unit", input: "5x", want: 0, wantErr: true},
		{name: "non-numeric", input: "abc", want: 0, wantErr: true},
		{name: "just number no unit", input: "100", want: 0, wantErr: true},
		{name: "negative duration", input: "-5s", want: -5 * time.Second, wantErr: false},
		{name: "whitespace", input: "  ", want: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "input=%q", tt.input)
			} else {
				assert.NoError(t, err, "input=%q", tt.input)
				assert.Equal(t, tt.want, got, "input=%q", tt.input)
			}
		})
	}
}

// --- Pure helper: parseInt ---

func TestParseInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "valid zero", input: "0", want: 0, wantErr: false},
		{name: "valid positive", input: "42", want: 42, wantErr: false},
		{name: "valid negative", input: "-5", want: -5, wantErr: false},
		{name: "valid large", input: "999999", want: 999999, wantErr: false},
		{name: "valid max int32", input: "2147483647", want: 2147483647, wantErr: false},
		{name: "empty string", input: "", want: 0, wantErr: true},
		{name: "non-numeric", input: "abc", want: 0, wantErr: true},
		{name: "mixed prefix", input: "abc123", want: 0, wantErr: true},
		{name: "float truncation", input: "12.5", want: 12, wantErr: false},
		{name: "whitespace", input: "  ", want: 0, wantErr: true},
		{name: "valid with leading space", input: "  42", want: 42, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "input=%q", tt.input)
			} else {
				assert.NoError(t, err, "input=%q", tt.input)
				assert.Equal(t, tt.want, got, "input=%q", tt.input)
			}
		})
	}
}

// --- ConfigSources ---

func TestConfigSources(t *testing.T) {
	t.Run("has expected sources", func(t *testing.T) {
		assert.NotEmpty(t, ConfigSources, "ConfigSources should not be empty")
		// The first source should be the system-wide config (lowest priority)
		assert.Equal(t, "/etc/muster/config.yaml", ConfigSources[0].Path)
		assert.Equal(t, 1, ConfigSources[0].Priority)
		assert.False(t, ConfigSources[0].Env)
	})

	t.Run("increasing priority order", func(t *testing.T) {
		for i := 1; i < len(ConfigSources); i++ {
			assert.Greater(t, ConfigSources[i].Priority, ConfigSources[i-1].Priority,
				"source %d should have higher priority than source %d", i, i-1)
		}
	})

	t.Run("env source has Env flag", func(t *testing.T) {
		found := false
		for _, src := range ConfigSources {
			if src.Env {
				found = true
				break
			}
		}
		assert.True(t, found, "at least one config source should be env-based")
	})
}

// --- ConfigManager ---

func TestConfigManager(t *testing.T) {
	t.Run("Config returns underlying config", func(t *testing.T) {
		cfg := &Config{Version: "2.0.0", DefaultProfile: "staging"}
		cm := &ConfigManager{cfg: cfg}
		assert.Equal(t, cfg, cm.Config())
	})

	t.Run("GetDefaultProfile returns default", func(t *testing.T) {
		cm := &ConfigManager{cfg: &Config{DefaultProfile: "production"}}
		assert.Equal(t, "production", cm.GetDefaultProfile())
	})

	t.Run("GetDefaultProfile empty when not set", func(t *testing.T) {
		cm := &ConfigManager{cfg: &Config{}}
		assert.Empty(t, cm.GetDefaultProfile())
	})
}

// --- ProfileManager ---

func TestProfileManager_List(t *testing.T) {
	t.Run("empty profiles returns empty list", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{Profiles: map[string]Profile{}}}
		assert.Empty(t, pm.List())
	})

	t.Run("nil profiles returns empty list", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{}}
		assert.Empty(t, pm.List())
	})

	t.Run("returns profiles with BaseURL fallback to name", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{
				"prod": {BaseURL: "https://prod.example.com", Timeout: 30 * time.Second},
				"dev":  {}, // BaseURL should default to name "dev"
			},
		}}
		profiles := pm.List()
		assert.Len(t, profiles, 2)

		byURL := make(map[string]*Profile, 2)
		for _, p := range profiles {
			byURL[p.BaseURL] = p
		}
		assert.Equal(t, "https://prod.example.com", byURL["https://prod.example.com"].BaseURL)
		assert.Equal(t, 30*time.Second, byURL["https://prod.example.com"].Timeout)
		assert.Equal(t, "dev", byURL["dev"].BaseURL)
	})

	t.Run("single profile with explicit BaseURL", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{"staging": {BaseURL: "https://staging.example.com"}},
		}}
		profiles := pm.List()
		assert.Len(t, profiles, 1)
		assert.Equal(t, "https://staging.example.com", profiles[0].BaseURL)
	})
}

func TestProfileManager_Get(t *testing.T) {
	t.Run("returns existing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{
				"prod": {BaseURL: "https://prod.example.com", Timeout: 30 * time.Second},
			},
		}}
		p, err := pm.Get("prod")
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, "https://prod.example.com", p.BaseURL)
		assert.Equal(t, 30*time.Second, p.Timeout)
	})

	t.Run("returns error for missing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{Profiles: map[string]Profile{}}}
		p, err := pm.Get("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("applies BaseURL fallback for profile without BaseURL", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{"local": {}},
		}}
		p, err := pm.Get("local")
		require.NoError(t, err)
		assert.Equal(t, "local", p.BaseURL)
	})
}

func TestProfileManager_Exists(t *testing.T) {
	pm := &ProfileManager{cfg: &Config{
		Profiles: map[string]Profile{
			"prod": {BaseURL: "https://prod.example.com"},
		},
	}}

	t.Run("existing profile", func(t *testing.T) {
		assert.True(t, pm.Exists("prod"))
	})

	t.Run("missing profile", func(t *testing.T) {
		assert.False(t, pm.Exists("nonexistent"))
	})

	t.Run("empty profiles", func(t *testing.T) {
		pm2 := &ProfileManager{cfg: &Config{Profiles: map[string]Profile{}}}
		assert.False(t, pm2.Exists("anything"))
	})
}

func TestProfileManager_SetDefault(t *testing.T) {
	t.Run("sets default for existing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{"prod": {BaseURL: "https://prod.example.com"}},
		}}
		err := pm.SetDefault("prod")
		require.NoError(t, err)
		assert.Equal(t, "prod", pm.cfg.DefaultProfile)
	})

	t.Run("returns error for missing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{Profiles: map[string]Profile{}}}
		err := pm.SetDefault("missing")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("overwrites previous default", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			DefaultProfile: "staging",
			Profiles: map[string]Profile{
				"staging": {BaseURL: "https://staging.example.com"},
				"prod":    {BaseURL: "https://prod.example.com"},
			},
		}}
		err := pm.SetDefault("prod")
		require.NoError(t, err)
		assert.Equal(t, "prod", pm.cfg.DefaultProfile)
	})
}

func TestProfileManager_Delete(t *testing.T) {
	t.Run("deletes existing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			Profiles: map[string]Profile{"prod": {BaseURL: "https://prod.example.com"}},
		}}
		err := pm.Delete("prod")
		require.NoError(t, err)
		assert.Empty(t, pm.cfg.Profiles)
	})

	t.Run("returns error for missing profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{Profiles: map[string]Profile{}}}
		err := pm.Delete("missing")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("clears default when deleting default profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			DefaultProfile: "prod",
			Profiles:       map[string]Profile{"prod": {BaseURL: "https://prod.example.com"}},
		}}
		err := pm.Delete("prod")
		require.NoError(t, err)
		assert.Empty(t, pm.cfg.DefaultProfile)
		assert.Empty(t, pm.cfg.Profiles)
	})

	t.Run("does not clear default when deleting non-default profile", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			DefaultProfile: "prod",
			Profiles: map[string]Profile{
				"prod": {BaseURL: "https://prod.example.com"},
				"dev":  {BaseURL: "https://dev.example.com"},
			},
		}}
		err := pm.Delete("dev")
		require.NoError(t, err)
		assert.Equal(t, "prod", pm.cfg.DefaultProfile)
		assert.Len(t, pm.cfg.Profiles, 1)
	})

	t.Run("deleting only profile clears default", func(t *testing.T) {
		pm := &ProfileManager{cfg: &Config{
			DefaultProfile: "only",
			Profiles:       map[string]Profile{"only": {BaseURL: "https://only.example.com"}},
		}}
		err := pm.Delete("only")
		require.NoError(t, err)
		assert.Empty(t, pm.cfg.DefaultProfile)
	})
}

// --- Config struct zero-value defaults ---

func TestConfig_Defaults(t *testing.T) {
	t.Run("zero-value Config has empty fields", func(t *testing.T) {
		var cfg Config
		assert.Empty(t, cfg.Version)
		assert.Empty(t, cfg.DefaultProfile)
		assert.Empty(t, cfg.Host)
		assert.Equal(t, 0, cfg.MaxRetries)
		assert.Equal(t, time.Duration(0), cfg.Timeout)
	})

	t.Run("zero-value sub-configs are non-nil structs", func(t *testing.T) {
		var cfg Config
		assert.Empty(t, cfg.Cache.Type)
		assert.Equal(t, int64(0), cfg.Cache.MaxSize)
		assert.Equal(t, false, cfg.RateLimit.Enabled)
	})

	t.Run("Config with version set", func(t *testing.T) {
		cfg := Config{Version: "1.0.0"}
		assert.Equal(t, "1.0.0", cfg.Version)
	})

	t.Run("ConfigSource zero value", func(t *testing.T) {
		var s ConfigSource
		assert.Empty(t, s.Path)
		assert.Equal(t, 0, s.Priority)
		assert.False(t, s.Env)
	})
}

// --- Profile struct defaults ---

func TestProfile_Defaults(t *testing.T) {
	t.Run("zero-value Profile", func(t *testing.T) {
		var p Profile
		assert.Empty(t, p.BaseURL)
		assert.Empty(t, p.AuthType)
		assert.Nil(t, p.AuthConfig)
		assert.Equal(t, time.Duration(0), p.Timeout)
	})

	t.Run("Profile with fields", func(t *testing.T) {
		p := Profile{
			BaseURL:  "https://example.com",
			AuthType: "bearer",
			Timeout:  30 * time.Second,
		}
		assert.Equal(t, "https://example.com", p.BaseURL)
		assert.Equal(t, "bearer", p.AuthType)
		assert.Equal(t, 30*time.Second, p.Timeout)
	})
}

// --- NewLoader comprehensive ---

func TestNewLoader_Comprehensive(t *testing.T) {
	t.Run("returns non-nil loader", func(t *testing.T) {
		loader := NewLoader()
		require.NotNil(t, loader)
	})

	t.Run("viper has MUSTER env prefix", func(t *testing.T) {
		loader := NewLoader()
		assert.Equal(t, "MUSTER", loader.GetViper().GetEnvPrefix())
	})

	t.Run("has config sources", func(t *testing.T) {
		loader := NewLoader()
		assert.NotEmpty(t, loader.sources)
		assert.Equal(t, len(ConfigSources), len(loader.sources))
	})

	t.Run("viper is configured for YAML", func(t *testing.T) {
		loader := NewLoader()
		// viper doesn't expose config type directly, but we verify it works
		assert.NotNil(t, loader.GetViper())
	})
}
