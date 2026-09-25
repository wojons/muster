package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
	"gopkg.in/yaml.v3"
)

// executeBuiltin runs a cobra command with the given args while capturing
// stdout, returning the output and the execution error.
func executeBuiltin(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	cmd.SetArgs(args)
	return captureStdoutCompletion(func() error {
		return cmd.Execute()
	})
}

// testProfileConfig builds a config pointing persistence at a temp file so
// tests never touch the developer's real ~/.config/muster/config.yaml.
func testProfileConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{
		ConfigFile: path,
		Profiles: map[string]config.Profile{
			"development": {BaseURL: "https://dev.example.com"},
			"production":  {BaseURL: "https://api.example.com"},
		},
	}
	return cfg, path
}

func readConfigYAMLKey(t *testing.T, path, key string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	v, ok := doc[key]
	if !ok {
		return "", false
	}
	s, isStr := v.(string)
	require.True(t, isStr, "key %s should be a string", key)
	return s, true
}

func TestProfileSwitch_PersistsAcrossProcesses(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := testProfileConfig(t)

	profileCmd := newProfileCommand(cfg, nil)
	out, err := executeBuiltin(t, profileCmd, "switch", "development")
	require.NoError(t, err)
	assert.Contains(t, out, "development")

	// The choice landed on disk…
	got, ok := readConfigYAMLKey(t, path, "default_profile")
	require.True(t, ok, "default_profile key must be written")
	assert.Equal(t, "development", got)

	// …and a fresh load (what a new process would see) picks it up.
	loader := config.NewLoader()
	loader.SetConfigFile(path)
	reloaded, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "development", reloaded.DefaultProfile)
}

func TestProfileSwitch_UnknownProfile_DoesNotWrite(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := testProfileConfig(t)

	profileCmd := newProfileCommand(cfg, nil)
	out, err := executeBuiltin(t, profileCmd, "switch", "nonexistent")
	require.NoError(t, err)
	assert.Contains(t, out, "not found")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "no config file should be written for an unknown profile")
}

func TestProfileDelete_ClearsPersistedDefault(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := testProfileConfig(t)
	// Persist production as the default first.
	require.NoError(t, config.WriteDefaultProfile(cfg, "production"))
	cfg.DefaultProfile = "production"

	profileCmd := newProfileCommand(cfg, nil)
	out, err := executeBuiltin(t, profileCmd, "delete", "production")
	require.NoError(t, err)
	assert.Contains(t, out, "Deleted")

	// The stale default_profile key is gone from disk; other keys survive.
	_, ok := readConfigYAMLKey(t, path, "default_profile")
	assert.False(t, ok, "default_profile should be cleared when its profile is deleted")
	assert.Equal(t, "", cfg.DefaultProfile)
}

func TestConfigShow_ActiveProfile(t *testing.T) {
	t.Run("reports persisted profile with source", func(t *testing.T) {
		t.Setenv(config.ProfileEnvVar, "")
		cfg, _ := testProfileConfig(t)
		cfg.DefaultProfile = "staging"

		out, err := executeBuiltin(t, newConfigCommand(cfg, nil), "show")
		require.NoError(t, err)
		assert.Contains(t, out, "Active Profile: staging (source: persisted)")
	})

	t.Run("env override changes source to env", func(t *testing.T) {
		t.Setenv(config.ProfileEnvVar, "envprof")
		cfg, _ := testProfileConfig(t)
		cfg.DefaultProfile = "staging"

		out, err := executeBuiltin(t, newConfigCommand(cfg, nil), "show")
		require.NoError(t, err)
		assert.Contains(t, out, "Active Profile: envprof (source: env)")
	})

	t.Run("flag overrides env and persisted", func(t *testing.T) {
		t.Setenv(config.ProfileEnvVar, "envprof")
		cfg, _ := testProfileConfig(t)
		cfg.DefaultProfile = "staging"

		root := &cobra.Command{Use: "root"}
		root.AddCommand(newConfigCommand(cfg, root))
		out, err := executeBuiltin(t, root, "config", "show", "--profile", "flagprof")
		require.NoError(t, err)
		assert.Contains(t, out, "Active Profile: flagprof (source: flag)")
	})

	t.Run("no active profile keeps the old output shape", func(t *testing.T) {
		t.Setenv(config.ProfileEnvVar, "")
		cfg, _ := testProfileConfig(t)

		out, err := executeBuiltin(t, newConfigCommand(cfg, nil), "show")
		require.NoError(t, err)
		assert.Contains(t, out, "Default Profile:")
		assert.NotContains(t, out, "Active Profile:")
	})
}

func TestRootCommand_ProfileFlagWired(t *testing.T) {
	cfg, _ := testProfileConfig(t)
	root := NewRootCommand(cfg)

	flag := root.PersistentFlags().Lookup(config.ProfileFlagName)
	require.NotNil(t, flag, "root must expose the global persistent --profile flag")
	assert.Contains(t, flag.Usage, "profile")
}

func TestRootCommand_ProfileFlagReachesConfigShow(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, _ := testProfileConfig(t)
	cfg.DefaultProfile = "staging"

	root := NewRootCommand(cfg)
	out, err := executeBuiltin(t, root, "config", "show", "--profile", "flagprof")
	require.NoError(t, err)
	assert.Contains(t, out, "Active Profile: flagprof (source: flag)")
}
