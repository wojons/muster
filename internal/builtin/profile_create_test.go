package builtin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
	"gopkg.in/yaml.v3"
)

// newCreateConfig points persistence at a temp config file with no profiles
// declared, so create tests never touch the developer's real config.
func newCreateConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	return &config.Config{ConfigFile: path, Profiles: map[string]config.Profile{}}, path
}

// loadFromDisk reads the config file the way a NEW process would, and returns
// it with persistence redirected back at that file.
func loadFromDisk(t *testing.T, path string) *config.Config {
	t.Helper()
	loader := config.NewLoader()
	loader.SetConfigFile(path)
	cfg, err := loader.Load()
	require.NoError(t, err)
	cfg.ConfigFile = path
	return cfg
}

// runProfile runs a profile subcommand on a freshly built command tree. A fresh
// tree per invocation is deliberate: cobra keeps flag values for the life of a
// command, so a shared tree would accumulate --set values across calls.
func runProfile(t *testing.T, cfg *config.Config, args ...string) (string, error) {
	t.Helper()
	return executeBuiltin(t, newProfileCommand(cfg, nil), args...)
}

// readProfileMap returns profiles.<name> as it decodes from the file on disk.
func readProfileMap(t *testing.T, path, name string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	profiles, ok := doc["profiles"].(map[string]interface{})
	require.True(t, ok, "config file must declare a profiles mapping")
	node, ok := profiles[name]
	require.True(t, ok, "config file must declare profile %q (have %v)", name, profiles)
	asMap, ok := node.(map[string]interface{})
	require.True(t, ok, "profile %q must decode as a mapping, got %T", name, node)
	return asMap
}

func TestProfileCreate_CommandShape(t *testing.T) {
	cfg, _ := newCreateConfig(t)

	var create *cobra.Command
	for _, sub := range newProfileCommand(cfg, nil).Commands() {
		if sub.Name() == "create" {
			create = sub
			break
		}
	}
	require.NotNil(t, create, "config profile must expose a create subcommand")
	assert.Contains(t, create.Use, "create <profile-name>")
	assert.Contains(t, create.Short, "Create")
	assert.Contains(t, create.Long, "base_url", "help must name the keys the loader reads")

	flag := create.Flags().Lookup(profileSetFlag)
	require.NotNil(t, flag, "create must expose --%s", profileSetFlag)
	assert.Contains(t, flag.Usage, "dot-path")
	assert.NotNil(t, create.Args, "create must constrain its positional arguments")
}

func TestProfileCreate_ListShowsNewProfile(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := newCreateConfig(t)

	out, err := runProfile(t, cfg, "create", "staging")
	require.NoError(t, err)
	assert.Contains(t, out, "Created profile: staging")

	// The same process sees it immediately…
	listOut, err := runProfile(t, cfg, "list")
	require.NoError(t, err)
	assert.Contains(t, listOut, "staging")
	assert.NotContains(t, listOut, "No profiles configured")

	// …and so does a new process reading the file back.
	fresh := loadFromDisk(t, path)
	assert.Contains(t, fresh.Profiles, "staging")
	freshList, err := runProfile(t, fresh, "list")
	require.NoError(t, err)
	assert.Contains(t, freshList, "staging")
}

func TestProfileCreate_EmptyProfileLoads(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := newCreateConfig(t)

	_, err := runProfile(t, cfg, "create", "shell")
	require.NoError(t, err)

	// An unwritten profile is an empty mapping, and it must still be a profile
	// the loader and the other subcommands can see.
	assert.Empty(t, readProfileMap(t, path, "shell"))
	fresh := loadFromDisk(t, path)
	_, ok := fresh.Profiles["shell"]
	require.True(t, ok, "an empty profile must still load as a profile")

	showOut, err := runProfile(t, fresh, "show", "shell")
	require.NoError(t, err)
	assert.Contains(t, showOut, "Name: shell")
}

func TestProfileCreate_DuplicateNameIsRefused(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := newCreateConfig(t)

	_, err := runProfile(t, cfg, "create", "staging")
	require.NoError(t, err)
	first, readErr := os.ReadFile(path)
	require.NoError(t, readErr)

	_, err = runProfile(t, cfg, "create", "staging")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// A differently cased spelling names the same loaded profile.
	_, err = runProfile(t, cfg, "create", "Staging")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// The refusal wrote nothing.
	second, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, string(first), string(second))
}

func TestProfileCreate_SetPrefillsKeys(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := newCreateConfig(t)

	out, err := runProfile(t, cfg, "create", "ci",
		"--set", "base_url=https://ci.example.com/v1",
		"--set", "auth_type=bearer",
		"--set", "default_flags.tier=pro",
		"--set", "timeout=60s",
		"--set", "headers.X-Trace=abc=def",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Created profile: ci")
	assert.NotContains(t, out, "Note:", "documented keys must not warn")

	profile := cfg.Profiles["ci"]
	assert.Equal(t, "https://ci.example.com/v1", profile.BaseURL)
	assert.Equal(t, "bearer", profile.AuthType)
	assert.Equal(t, "pro", profile.DefaultFlags["tier"])
	assert.Equal(t, time.Minute, profile.Timeout)
	// The split happens on the first '=' only, so a header value keeps its own
	// '='; keys are lowercased because the loader reads keys case-insensitively.
	assert.Equal(t, "abc=def", profile.Headers["x-trace"])

	node := readProfileMap(t, path, "ci")
	assert.Equal(t, "https://ci.example.com/v1", node["base_url"])
	nested, ok := node["default_flags"].(map[string]interface{})
	require.True(t, ok, "default_flags.tier must be written as a nested mapping")
	assert.Equal(t, "pro", nested["tier"])

	fresh := loadFromDisk(t, path)
	assert.Equal(t, "https://ci.example.com/v1", fresh.Profiles["ci"].BaseURL)
	assert.Equal(t, "pro", fresh.Profiles["ci"].DefaultFlags["tier"])
}

func TestProfileCreate_ThenSwitchPersistsSelection(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")

	// Process 1: create the profile.
	cfg1, path := newCreateConfig(t)
	out, err := runProfile(t, cfg1, "create", "staging", "--set", "base_url=https://staging.example.com")
	require.NoError(t, err)
	assert.Contains(t, out, "Created profile: staging")

	// Process 2: a fresh load sees the profile, so switch accepts it and the
	// DF-022 persistence path records the selection.
	cfg2 := loadFromDisk(t, path)
	switchOut, err := runProfile(t, cfg2, "switch", "staging")
	require.NoError(t, err)
	assert.Contains(t, switchOut, "Switched to profile: staging")

	// Process 3: the selection is on disk, resolves as persisted, and the
	// profile it names is still there.
	raw, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	assert.Equal(t, "staging", doc["default_profile"])
	assert.NotNil(t, doc["profiles"], "switching must not disturb the profiles block")

	cfg3 := loadFromDisk(t, path)
	assert.Equal(t, "staging", cfg3.DefaultProfile)
	assert.Contains(t, cfg3.Profiles, "staging")
	showOut, err := executeBuiltin(t, newConfigCommand(cfg3, nil), "show")
	require.NoError(t, err)
	assert.Contains(t, showOut, "Active Profile: staging (source: persisted)")

	// Creating a second profile after switching leaves the selection in place
	// (read-modify-write, not a rewrite of the document).
	_, err = runProfile(t, cfg3, "create", "production")
	require.NoError(t, err)
	cfg4 := loadFromDisk(t, path)
	assert.Equal(t, "staging", cfg4.DefaultProfile)
	assert.Contains(t, cfg4.Profiles, "production")
}

func TestProfileCreate_RejectsBadInput(t *testing.T) {
	t.Run("dotted name", func(t *testing.T) {
		cfg, path := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create", "team.prod")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid profile name")
		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr), "a rejected name must not write a config file")
	})

	t.Run("empty name", func(t *testing.T) {
		cfg, _ := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create", " ")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("name with whitespace", func(t *testing.T) {
		cfg, _ := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create", "my profile")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid profile name")
	})

	t.Run("missing name argument", func(t *testing.T) {
		cfg, _ := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create")
		require.Error(t, err)
	})

	t.Run("--set without =", func(t *testing.T) {
		cfg, path := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create", "broken", "--set", "base_url")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr), "a rejected --set must not write a config file")
	})

	t.Run("--set with empty key", func(t *testing.T) {
		cfg, _ := newCreateConfig(t)
		_, err := runProfile(t, cfg, "create", "broken", "--set", "=value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key must not be empty")
	})
}

func TestProfileCreate_UnknownKeyIsNotedButPersisted(t *testing.T) {
	t.Setenv(config.ProfileEnvVar, "")
	cfg, path := newCreateConfig(t)

	out, err := runProfile(t, cfg, "create", "docs", "--set", "spec.base_url=https://docs.example.com")
	require.NoError(t, err)
	assert.Contains(t, out, "Created profile: docs")
	assert.Contains(t, out, "Note:")
	assert.Contains(t, out, "spec")

	// The pair is persisted verbatim…
	spec, ok := readProfileMap(t, path, "docs")["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://docs.example.com", spec["base_url"])

	// …but it is inert: the loader reads base_url at the profile top level.
	assert.Equal(t, "", cfg.Profiles["docs"].BaseURL)
	fresh := loadFromDisk(t, path)
	assert.Equal(t, "", fresh.Profiles["docs"].BaseURL)
}
