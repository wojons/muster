package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeTestConfig writes a user-shaped config file and returns its path.
func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestUserConfigFilePath(t *testing.T) {
	t.Run("explicit cfg.ConfigFile wins", func(t *testing.T) {
		cfg := &Config{ConfigFile: "/custom/config.yaml"}
		path, err := UserConfigFilePath(cfg)
		require.NoError(t, err)
		assert.Equal(t, "/custom/config.yaml", path)
	})

	t.Run("MUSTER_CONFIG env honored", func(t *testing.T) {
		t.Setenv("MUSTER_CONFIG", "/env/config.yaml")
		path, err := UserConfigFilePath(&Config{})
		require.NoError(t, err)
		assert.Equal(t, "/env/config.yaml", path)
	})

	t.Run("MUSTER_CONFIG_FILE env honored", func(t *testing.T) {
		t.Setenv("MUSTER_CONFIG_FILE", "/env/config-file.yaml")
		path, err := UserConfigFilePath(&Config{})
		require.NoError(t, err)
		assert.Equal(t, "/env/config-file.yaml", path)
	})

	t.Run("prefers existing standard path over fallback", func(t *testing.T) {
		// Isolate os.UserHomeDir() on all platforms: HOME on Unix, USERPROFILE on Windows.
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		t.Setenv("USERPROFILE", tmpHome)
		// Also isolate os.UserConfigDir() on Windows (uses APPDATA/LOCALAPPDATA).
		t.Setenv("APPDATA", filepath.Join(tmpHome, "AppData", "Roaming"))
		t.Setenv("LOCALAPPDATA", filepath.Join(tmpHome, "AppData", "Local"))

		legacy := filepath.Join(tmpHome, ".muster")
		require.NoError(t, os.MkdirAll(legacy, 0o700))
		legacyCfg := filepath.Join(legacy, "config.yaml")
		require.NoError(t, os.WriteFile(legacyCfg, []byte("version: \"1.0.0\"\n"), 0o600))

		path, err := UserConfigFilePath(&Config{})
		require.NoError(t, err)
		assert.Equal(t, legacyCfg, path)
	})

	t.Run("defaults to xdg user path when nothing exists", func(t *testing.T) {
		// Isolate os.UserHomeDir() on all platforms: HOME on Unix, USERPROFILE on Windows.
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		t.Setenv("USERPROFILE", tmpHome)
		// Also isolate os.UserConfigDir() on Windows (uses APPDATA/LOCALAPPDATA).
		t.Setenv("APPDATA", filepath.Join(tmpHome, "AppData", "Roaming"))
		t.Setenv("LOCALAPPDATA", filepath.Join(tmpHome, "AppData", "Local"))

		path, err := UserConfigFilePath(&Config{})
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpHome, ".config", "muster", "config.yaml"), path)
	})

	t.Run("nil config tolerated", func(t *testing.T) {
		path, err := UserConfigFilePath(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, path)
	})
}

func TestWriteDefaultProfile(t *testing.T) {
	t.Run("persists default_profile into an existing config file", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\nhost: localhost\nprofiles:\n  dev:\n    base_url: http://localhost:8080\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteDefaultProfile(cfg, "dev"))

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var doc map[string]interface{}
		require.NoError(t, yaml.Unmarshal(raw, &doc))
		// The new key lands…
		assert.Equal(t, "dev", doc["default_profile"])
		// …and every pre-existing key survives (read-modify-write, not clobber).
		assert.Equal(t, "localhost", doc["host"])
		assert.NotNil(t, doc["profiles"])
	})

	t.Run("creates the config file when missing", func(t *testing.T) {
		cfg := &Config{ConfigFile: filepath.Join(t.TempDir(), "nested", "config.yaml")}

		require.NoError(t, WriteDefaultProfile(cfg, "staging"))

		raw, err := os.ReadFile(cfg.ConfigFile)
		require.NoError(t, err)
		var doc map[string]interface{}
		require.NoError(t, yaml.Unmarshal(raw, &doc))
		assert.Equal(t, "staging", doc["default_profile"])
	})

	t.Run("is idempotent", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteDefaultProfile(cfg, "dev"))
		first, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, WriteDefaultProfile(cfg, "dev"))
		second, err := os.ReadFile(path)
		require.NoError(t, err)

		assert.Equal(t, string(first), string(second))
	})

	t.Run("empty name removes the key", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\ndefault_profile: dev\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteDefaultProfile(cfg, ""))

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var doc map[string]interface{}
		require.NoError(t, yaml.Unmarshal(raw, &doc))
		_, has := doc["default_profile"]
		assert.False(t, has)
		assert.Equal(t, "1.0.0", doc["version"])
	})

	t.Run("unparseable file errors instead of clobbering", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles: [broken\n")
		cfg := &Config{ConfigFile: path}

		err := WriteDefaultProfile(cfg, "dev")
		require.Error(t, err)
		// The broken file is left exactly as it was.
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, "profiles: [broken\n", string(raw))
	})

	t.Run("written file is readable by the loader (cross-process contract)", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  dev:\n    base_url: http://x\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteDefaultProfile(cfg, "dev"))

		loader := NewLoader()
		loader.sources = []ConfigSource{{Path: path, Priority: 1}}
		loaded, err := loader.Load()
		require.NoError(t, err)
		assert.Equal(t, "dev", loaded.DefaultProfile)
	})
}

func TestResolveProfile(t *testing.T) {
	persistedCfg := &Config{DefaultProfile: "persisted"}

	t.Run("no sources yields empty selection", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "")
		sel := ResolveProfile(&cobra.Command{Use: "root"}, &Config{})
		assert.Equal(t, "", sel.Name)
		assert.Equal(t, ProfileSourceNone, sel.Source)
	})

	t.Run("persisted config file is used when nothing else set", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "")
		sel := ResolveProfile(&cobra.Command{Use: "root"}, persistedCfg)
		assert.Equal(t, "persisted", sel.Name)
		assert.Equal(t, ProfileSourcePersisted, sel.Source)
	})

	t.Run("env overrides persisted", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "envprof")
		sel := ResolveProfile(&cobra.Command{Use: "root"}, persistedCfg)
		assert.Equal(t, "envprof", sel.Name)
		assert.Equal(t, ProfileSourceEnv, sel.Source)
	})

	t.Run("flag overrides env and persisted", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "envprof")
		cmd := &cobra.Command{Use: "root"}
		BindProfileFlag(cmd)
		require.NoError(t, cmd.PersistentFlags().Set(ProfileFlagName, "flagprof"))

		sel := ResolveProfile(cmd, persistedCfg)
		assert.Equal(t, "flagprof", sel.Name)
		assert.Equal(t, ProfileSourceFlag, sel.Source)
	})

	t.Run("flag at default value is ignored (not Changed)", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "envprof")
		cmd := &cobra.Command{Use: "root"}
		BindProfileFlag(cmd)

		sel := ResolveProfile(cmd, persistedCfg)
		assert.Equal(t, "envprof", sel.Name)
		assert.Equal(t, ProfileSourceEnv, sel.Source)
	})

	t.Run("empty env var is ignored", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "")
		sel := ResolveProfile(&cobra.Command{Use: "root"}, persistedCfg)
		assert.Equal(t, "persisted", sel.Name)
	})

	t.Run("nil cmd and nil cfg tolerated", func(t *testing.T) {
		t.Setenv(ProfileEnvVar, "")
		sel := ResolveProfile(nil, nil)
		assert.Equal(t, "", sel.Name)
		assert.Equal(t, ProfileSourceNone, sel.Source)
	})
}

func TestBindProfileFlag(t *testing.T) {
	t.Run("registers the global persistent flag once", func(t *testing.T) {
		cmd := &cobra.Command{Use: "root"}
		BindProfileFlag(cmd)
		BindProfileFlag(cmd) // second registration must not panic

		flag := cmd.PersistentFlags().Lookup(ProfileFlagName)
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})
}
