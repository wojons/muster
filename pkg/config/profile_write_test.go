package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// readConfigDoc parses the config file at path into a generic document so a
// test can assert on what actually landed on disk.
func readConfigDoc(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

// profileNode returns profiles.<name> from the config file at path.
func profileNode(t *testing.T, path, name string) map[string]interface{} {
	t.Helper()
	doc := readConfigDoc(t, path)
	profiles, ok := doc["profiles"].(map[string]interface{})
	require.True(t, ok, "config must declare a profiles mapping")
	node, ok := profiles[name].(map[string]interface{})
	require.True(t, ok, "config must declare profile %q (have %v)", name, profiles)
	return node
}

func TestWriteProfile(t *testing.T) {
	t.Run("adds a profile beside existing keys", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\nhost: localhost\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{"base_url": "http://localhost:8080"}))

		doc := readConfigDoc(t, path)
		assert.Equal(t, "1.0.0", doc["version"])
		assert.Equal(t, "localhost", doc["host"])
		assert.Equal(t, "http://localhost:8080", profileNode(t, path, "dev")["base_url"])
	})

	t.Run("preserves the persisted default_profile", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path}
		require.NoError(t, WriteDefaultProfile(cfg, "production"))

		require.NoError(t, WriteProfile(cfg, "staging", map[string]string{"base_url": "https://staging.example.com"}))

		doc := readConfigDoc(t, path)
		assert.Equal(t, "production", doc["default_profile"], "creating a profile must not disturb the active selection")
		profiles := doc["profiles"].(map[string]interface{})
		assert.Contains(t, profiles, "staging")
	})

	t.Run("nested dot-paths become mappings", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{
			"default_flags.tier": "pro",
			"headers.x-api-key":  "secret",
		}))

		dev := profileNode(t, path, "dev")
		flags, ok := dev["default_flags"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "pro", flags["tier"])
		headers, ok := dev["headers"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "secret", headers["x-api-key"])
	})

	t.Run("values are stored as strings, never coerced", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{
			"timeout":   "30s",
			"auth_type": "true",
		}))

		dev := profileNode(t, path, "dev")
		assert.Equal(t, "30s", dev["timeout"])
		assert.Equal(t, "true", dev["auth_type"], "a string that looks like a bool must stay a string")
	})

	t.Run("an empty value omits the key", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  dev:\n    base_url: http://old\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{"base_url": ""}))

		assert.NotContains(t, profileNode(t, path, "dev"), "base_url")
	})

	t.Run("is idempotent", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path}
		values := map[string]string{"base_url": "http://x", "default_flags.tier": "pro"}

		require.NoError(t, WriteProfile(cfg, "dev", values))
		first, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, WriteProfile(cfg, "dev", values))
		second, err := os.ReadFile(path)
		require.NoError(t, err)

		assert.Equal(t, string(first), string(second))
	})

	t.Run("creates the config file and its directory when missing", func(t *testing.T) {
		cfg := &Config{ConfigFile: filepath.Join(t.TempDir(), "nested", "config.yaml")}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{"base_url": "http://x"}))

		assert.Equal(t, "http://x", profileNode(t, cfg.ConfigFile, "dev")["base_url"])
	})

	t.Run("upgrades an empty profiles key", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\nprofiles:\n")
		cfg := &Config{ConfigFile: path}

		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{"base_url": "http://x"}))

		assert.Equal(t, "http://x", profileNode(t, path, "dev")["base_url"])
		assert.Equal(t, "1.0.0", readConfigDoc(t, path)["version"])
	})

	t.Run("refuses a non-mapping profiles key", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  - dev\n")
		cfg := &Config{ConfigFile: path}

		err := WriteProfile(cfg, "dev", map[string]string{"base_url": "http://x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a mapping")
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, "profiles:\n  - dev\n", string(raw), "the refused file is left untouched")
	})

	t.Run("refuses a non-mapping profile value", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  dev: scalar\n")
		cfg := &Config{ConfigFile: path}

		err := WriteProfile(cfg, "dev", map[string]string{"base_url": "http://x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a mapping")
	})

	t.Run("unparsable file errors instead of clobbering", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles: [broken\n")
		cfg := &Config{ConfigFile: path}

		err := WriteProfile(cfg, "dev", map[string]string{"base_url": "http://x"})

		require.Error(t, err)
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, "profiles: [broken\n", string(raw))
	})
}

func TestReloadProfiles(t *testing.T) {
	t.Run("reads back what WriteProfile wrote", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "version: \"1.0.0\"\n")
		cfg := &Config{ConfigFile: path, Profiles: map[string]Profile{"stale": {}}}
		require.NoError(t, WriteProfile(cfg, "dev", map[string]string{
			"base_url":           "http://localhost:8080",
			"default_flags.tier": "pro",
		}))

		require.NoError(t, ReloadProfiles(cfg))

		assert.NotContains(t, cfg.Profiles, "stale", "reload replaces the in-memory copy")
		require.Contains(t, cfg.Profiles, "dev")
		assert.Equal(t, "http://localhost:8080", cfg.Profiles["dev"].BaseURL)
		assert.Equal(t, "pro", cfg.Profiles["dev"].DefaultFlags["tier"])
	})

	t.Run("nil config errors", func(t *testing.T) {
		require.Error(t, ReloadProfiles(nil))
	})
}

// TestLoader_ProfileDeclaredWithoutKeysLoads pins the loader repair for
// profiles viper cannot round-trip: a profile whose value carries no leaf keys
// is flattened away, which made a freshly created profile invisible to the
// next process ("create then list" showed nothing).
func TestLoader_ProfileDeclaredWithoutKeysLoads(t *testing.T) {
	t.Run("empty mapping is a declared profile", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n    shell: {}\n")
		cfg := &Config{ConfigFile: path}
		require.NoError(t, WriteProfile(cfg, "shell", nil))

		loaded := loadFromFile(t, path)

		require.Contains(t, loaded.Profiles, "shell")
		assert.Equal(t, Profile{}, loaded.Profiles["shell"])
	})

	t.Run("bare key is a declared profile", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  empty:\n  typed:\n    base_url: http://x\n")

		loaded := loadFromFile(t, path)

		require.Contains(t, loaded.Profiles, "empty")
		assert.Equal(t, "", loaded.Profiles["empty"].BaseURL)
		assert.Equal(t, "http://x", loaded.Profiles["typed"].BaseURL)
	})

	t.Run("a declared name is keyed the way viper keys it", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  Staging: {}\n")

		loaded := loadFromFile(t, path)

		assert.Contains(t, loaded.Profiles, "staging")
	})

	t.Run("a populated profile keeps its keys", func(t *testing.T) {
		path := writeTestConfig(t, t.TempDir(), "profiles:\n  dev:\n    base_url: http://localhost\n    auth_type: bearer\n")

		loaded := loadFromFile(t, path)

		require.Contains(t, loaded.Profiles, "dev")
		assert.Equal(t, "http://localhost", loaded.Profiles["dev"].BaseURL)
		assert.Equal(t, "bearer", loaded.Profiles["dev"].AuthType)
	})
}

// loadFromFile loads a single config file the way a CLI process does.
func loadFromFile(t *testing.T, path string) *Config {
	t.Helper()
	loader := NewLoader()
	loader.SetConfigFile(path)
	cfg, err := loader.Load()
	require.NoError(t, err)
	return cfg
}

func TestNormalizeProfileName(t *testing.T) {
	t.Run("lowercases the name the loader will key", func(t *testing.T) {
		name, err := NormalizeProfileName("  Staging ")
		require.NoError(t, err)
		assert.Equal(t, "staging", name)
	})

	t.Run("keeps dashes and underscores", func(t *testing.T) {
		name, err := NormalizeProfileName("pre-prod_1")
		require.NoError(t, err)
		assert.Equal(t, "pre-prod_1", name)
	})

	t.Run("rejects a dotted name", func(t *testing.T) {
		_, err := NormalizeProfileName("team.prod")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid profile name")
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		_, err := NormalizeProfileName("   ")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("rejects whitespace inside a name", func(t *testing.T) {
		_, err := NormalizeProfileName("my profile")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid profile name")
	})
}

func TestParseProfileSetPairs(t *testing.T) {
	t.Run("no pairs yields no values", func(t *testing.T) {
		values, err := ParseProfileSetPairs(nil)
		require.NoError(t, err)
		assert.Empty(t, values)
	})

	t.Run("splits on the first = only", func(t *testing.T) {
		values, err := ParseProfileSetPairs([]string{"headers.x-trace=abc=def"})
		require.NoError(t, err)
		assert.Equal(t, "abc=def", values["headers.x-trace"])
	})

	t.Run("lowercases keys so they match the loader", func(t *testing.T) {
		values, err := ParseProfileSetPairs([]string{"Base_URL=http://x"})
		require.NoError(t, err)
		assert.Equal(t, "http://x", values["base_url"])
	})

	t.Run("keeps an empty value", func(t *testing.T) {
		values, err := ParseProfileSetPairs([]string{"base_url="})
		require.NoError(t, err)
		assert.Equal(t, "", values["base_url"])
	})

	t.Run("rejects a pair without =", func(t *testing.T) {
		_, err := ParseProfileSetPairs([]string{"base_url"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})

	t.Run("rejects an empty key", func(t *testing.T) {
		_, err := ParseProfileSetPairs([]string{"=value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key must not be empty")
	})
}

func TestUnknownProfileSetKeys(t *testing.T) {
	t.Run("names the groups the loader does not read", func(t *testing.T) {
		unknown := UnknownProfileSetKeys(map[string]string{
			"spec.base_url":      "http://x",
			"http.timeout":       "60s",
			"spec.auth_type":     "bearer",
			"base_url":           "http://y",
			"default_flags.tier": "pro",
		})
		assert.Equal(t, []string{"http", "spec"}, unknown)
	})

	t.Run("no unknown keys yields nothing", func(t *testing.T) {
		assert.Empty(t, UnknownProfileSetKeys(map[string]string{
			"base_url": "http://x", "auth_type": "bearer", "timeout": "30s",
			"auth_config.token": "t", "default_flags.tier": "pro", "headers.accept": "*/*",
		}))
	})
}
