package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
)

func TestPersistedCommandLoadGating(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "commands.json")
	require.NoError(t, os.WriteFile(storePath, mustMarshalStoredData(t), 0o600))
	cfg := &config.Config{PersistedCommandsPath: storePath}

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "version", args: []string{"version"}, want: 0},
		// DF-061: completion surfaces now LOAD the store — generated
		// commands must appear in shell completion and __complete output.
		{name: "completion", args: []string{"completion", "bash"}, want: 1},
		{name: "completion hidden command", args: []string{"__complete", ""}, want: 1},
		{name: "completion hidden command no descriptions", args: []string{"__completeNoDesc", ""}, want: 1},
		{name: "root help", args: []string{"--help"}, want: 0},
		{name: "short root help", args: []string{"-h"}, want: 0},
		{name: "bare help", args: []string{"help"}, want: 0},
		{name: "bare root", args: nil, want: 1},
		{name: "generated command", args: []string{"generated-command", "--help"}, want: 1},
		{name: "help generated command", args: []string{"help", "generated-command"}, want: 1},
		{name: "-- separator then generated command", args: []string{"--", "generated-command", "--help"}, want: 1},
		{name: "-- separator only", args: []string{"--"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeDecodeCount = 0
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = append([]string{"openapi-cli"}, tc.args...)

			root := NewRootCommand(cfg)
			require.NotNil(t, root)
			assert.Equal(t, tc.want, storeDecodeCount)
			if tc.name == "generated command" || tc.name == "help generated command" || tc.name == "-- separator then generated command" {
				cmd, _, err := root.Find([]string{"generated-command"})
				require.NoError(t, err)
				require.NotNil(t, cmd)
				assert.Equal(t, "generated-command", cmd.Name())
				var output bytes.Buffer
				root.SetOut(&output)
				root.SetErr(&output)
				root.SetArgs(tc.args)
				require.NoError(t, root.Execute())
				assert.Contains(t, output.String(), "Usage:")
			} else if tc.name == "root help" {
				var output bytes.Buffer
				root.SetOut(&output)
				root.SetErr(&output)
				root.SetArgs(tc.args)
				require.NoError(t, root.Execute())
				assert.NotContains(t, output.String(), "generated-command")
			}
		})
	}
}

func TestRootHelpWarnsForTornStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	storePath := filepath.Join(home, ".local", "share", "openapi-cli", "commands.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o700))
	intact := append(mustMarshalStoredData(t), []byte(" \n	")...)
	torn := []byte(`{"version":"1.0","commands":[`)
	// largeIntact/largeTorn exceed the probe's bounded tail read, so they exercise
	// the seek-to-tail path: an intact large store must stay silent and a large
	// torn store must name its FULL byte size, not the tail length.
	largeIntact := mustMarshalLargeStoredData(t, 5000)
	largeTorn := append(append([]byte{}, largeIntact[:len(largeIntact)-1]...), []byte(`,"name":"trunc`)...)
	require.Greater(t, len(largeIntact), 4096)
	require.Greater(t, len(largeTorn), 4096)

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, flag := range []string{"--help", "-h"} {
		for _, tc := range []struct {
			name string
			data []byte
			torn bool
		}{
			{name: "torn", data: torn, torn: true},
			{name: "intact", data: intact},
			{name: "large torn", data: largeTorn, torn: true},
			{name: "large intact", data: largeIntact},
		} {
			t.Run(flag+"/"+tc.name, func(t *testing.T) {
				require.NoError(t, os.WriteFile(storePath, tc.data, 0o600))
				os.Args = []string{"openapi-cli", flag}
				storeDecodeCount = 0
				var runErr error
				var output bytes.Buffer
				stderr := captureStderrRequest(func() {
					root := NewRootCommand(&config.Config{})
					root.SetOut(&output)
					root.SetErr(os.Stderr)
					runErr = root.Execute()
				})
				require.NoError(t, runErr) // Successful execution is CLI exit code 0.
				assert.Equal(t, 0, storeDecodeCount)
				assert.Contains(t, output.String(), "Usage:")
				if tc.torn {
					assert.Contains(t, stderr, "Warning: Failed to load stored commands:")
					assert.Contains(t, stderr, storePath)
					assert.Contains(t, stderr, fmt.Sprintf("%d bytes", len(tc.data)))
				} else {
					assert.Empty(t, stderr)
				}
			})
		}
	}
}

func mustMarshalStoredData(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(cmdstore.StoredData{
		Version:     "1.0",
		BaseURL:     "http://127.0.0.1:1",
		GeneratedAt: time.Now(),
		Commands: []cmdstore.CommandMetadata{{
			Name:   "generated-command",
			Use:    "generated-command",
			Short:  "generated command",
			Method: "GET",
			Path:   "/generated",
		}},
	})
	require.NoError(t, err)
	return data
}

// mustMarshalLargeStoredData marshals a synthetic store holding count commands,
// used to prove the meta-path probe stays silent on a store larger than its
// bounded tail read without decoding it.
func mustMarshalLargeStoredData(t *testing.T, count int) []byte {
	t.Helper()
	commands := make([]cmdstore.CommandMetadata, 0, count)
	for i := 0; i < count; i++ {
		commands = append(commands, cmdstore.CommandMetadata{
			Name:   fmt.Sprintf("generated-command-%05d", i),
			Use:    fmt.Sprintf("generated-command-%05d", i),
			Short:  "generated command",
			Method: "GET",
			Path:   fmt.Sprintf("/generated/%d", i),
		})
	}
	data, err := json.Marshal(cmdstore.StoredData{
		Version:     "1.0",
		BaseURL:     "http://127.0.0.1:1",
		GeneratedAt: time.Now(),
		Commands:    commands,
	})
	require.NoError(t, err)
	return data
}

func TestNewRootCommand(t *testing.T) {
	t.Run("creates root command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
		assert.Equal(t, "openapi-cli", root.Name())
		assert.Equal(t, "OpenAPI-powered CLI and MCP server", root.Short)
		assert.Contains(t, root.Long, "A unified CLI tool that generates commands from OpenAPI specifications")
	})

	t.Run("has version command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasVersion := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "version" {
				hasVersion = true
				break
			}
		}
		assert.True(t, hasVersion)
	})

	t.Run("has config command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasConfig := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "config" {
				hasConfig = true
				break
			}
		}
		assert.True(t, hasConfig)
	})

	t.Run("has help command (Cobra provides default)", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		// Cobra auto-handles "help" via its internal routing
		// (root.HelpFunc() and the default help flag processing).
		// The "help" command is NOT in root.Commands() because
		// Cobra handles it outside the command tree, which means
		// it doesn't appear as a duplicate in the Available Commands
		// list.
		assert.NotNil(t, root, "root command should be created")
		// Verify cobra's internal help handling is intact
		assert.NotNil(t, root.HelpFunc(), "help function should be set")
	})

	t.Run("has completion command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasCompletion := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "completion" {
				hasCompletion = true
				break
			}
		}
		assert.True(t, hasCompletion)
	})

	t.Run("has init command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasInit := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "init" {
				hasInit = true
				break
			}
		}
		assert.True(t, hasInit)
	})

	t.Run("has auth command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasAuth := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "auth" {
				hasAuth = true
				break
			}
		}
		assert.True(t, hasAuth)
	})

	t.Run("has inventory command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasInventory := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "inventory" {
				hasInventory = true
				break
			}
		}
		assert.True(t, hasInventory)
	})

	t.Run("has modules command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasModules := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "modules" {
				hasModules = true
				break
			}
		}
		assert.True(t, hasModules)
	})

	t.Run("has generate command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasGenerate := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "generate" {
				hasGenerate = true
				break
			}
		}
		assert.True(t, hasGenerate)
	})

	t.Run("has request command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasRequest := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "request" {
				hasRequest = true
				break
			}
		}
		assert.True(t, hasRequest)
	})

	t.Run("has daemon command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasDaemon := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "daemon" {
				hasDaemon = true
				break
			}
		}
		assert.True(t, hasDaemon)
	})

	t.Run("root command has description", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotEmpty(t, root.Short)
		assert.NotEmpty(t, root.Long)
	})
}

func TestLoadPersistedCommands(t *testing.T) {
	t.Run("handles missing storage gracefully", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
	})

	t.Run("root command is not nil", func(t *testing.T) {
		cfg := &config.Config{
			ConfigDir:      "/tmp/config",
			CacheDir:       "/tmp/cache",
			DataDir:        "/tmp/data",
			Version:        "1.0.0",
			DefaultProfile: "default",
		}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
	})

	t.Run("all builtin commands are registered", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		// "help" is intentionally NOT in this list — Cobra auto-registers its
		// own default "help" command. Adding a second one causes duplicate
		// entries in the Available Commands list.
		expectedCommands := []string{
			"version",
			"config",
			"completion",
			"init",
			"auth",
			"inventory",
			"modules",
			"generate",
			"request",
			"daemon",
		}

		actualCommands := make(map[string]bool)
		for _, cmd := range root.Commands() {
			actualCommands[cmd.Name()] = true
		}

		for _, expected := range expectedCommands {
			assert.True(t, actualCommands[expected], "command %s not found", expected)
		}
	})
}

func TestRootCommandStructure(t *testing.T) {
	t.Run("root command has multiple subcommands", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.Greater(t, len(root.Commands()), 5)
	})

	t.Run("all subcommands have descriptions", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		builtInCommands := []string{
			"version", "config", "help", "completion", "init",
			"auth", "inventory", "modules", "generate", "request", "daemon",
		}

		for _, cmd := range root.Commands() {
			if cmd.Hidden {
				continue
			}
			isBuiltIn := false
			for _, name := range builtInCommands {
				if cmd.Name() == name {
					isBuiltIn = true
					break
				}
			}
			if isBuiltIn {
				assert.NotEmpty(t, cmd.Short, "built-in command %s missing Short", cmd.Name())
			}
		}
	})
}

func TestNewRootCommandWithAudit(t *testing.T) {
	t.Run("creates command with nil audit", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommandWithAudit(cfg, nil)
		require.NotNil(t, root)
		assert.Equal(t, "openapi-cli", root.Use)
		assert.Greater(t, len(root.Commands()), 5)
	})

	t.Run("creates command with audit hook", func(t *testing.T) {
		cfg := &config.Config{}
		hook := &audit.Hook{}
		root := NewRootCommandWithAudit(cfg, hook)
		require.NotNil(t, root)
	})

	t.Run("returns same structure as NewRootCommand", func(t *testing.T) {
		cfg := &config.Config{}
		withAudit := NewRootCommandWithAudit(cfg, nil)
		withoutAudit := NewRootCommand(cfg)
		assert.Equal(t, len(withAudit.Commands()), len(withoutAudit.Commands()))
		assert.Equal(t, withAudit.Use, withoutAudit.Use)
	})
}

func TestRootCommand_SilenceErrors(t *testing.T) {
	// DF-MUSTER-4: the root command must NOT print errors itself — cobra's
	// default error print duplicates the one in cmd/openapi-cli/main.go,
	// so every failing invocation printed "Error: ..." twice on stderr.
	// main.go owns the single print; SilenceErrors disables cobra's copy.
	cfg := &config.Config{}
	root := NewRootCommand(cfg)

	assert.True(t, root.SilenceErrors, "root command must set SilenceErrors so errors print exactly once on stderr")
}

func TestCliActorID(t *testing.T) {
	t.Run("returns USER env when set", func(t *testing.T) {
		os.Setenv("USER", "testuser")
		defer os.Unsetenv("USER")
		os.Unsetenv("LOGNAME")
		assert.Equal(t, "testuser", CliActorID())
	})

	t.Run("falls back to LOGNAME when USER empty", func(t *testing.T) {
		os.Unsetenv("USER")
		os.Setenv("LOGNAME", "lognameuser")
		defer os.Unsetenv("LOGNAME")
		assert.Equal(t, "lognameuser", CliActorID())
	})

	t.Run("returns 'unknown' when neither set", func(t *testing.T) {
		os.Unsetenv("USER")
		os.Unsetenv("LOGNAME")
		assert.Equal(t, "unknown", CliActorID())
	})

	t.Run("prefers USER over LOGNAME", func(t *testing.T) {
		os.Setenv("USER", "primary")
		os.Setenv("LOGNAME", "secondary")
		defer os.Unsetenv("USER")
		defer os.Unsetenv("LOGNAME")
		assert.Equal(t, "primary", CliActorID())
	})
}

// TestExpandAliasArgs pins the argv rewrite rules for config aliases.
func TestExpandAliasArgs(t *testing.T) {
	cfg := &config.Config{Aliases: map[string]string{
		"tasks": "config show",
		"pf":    "config profile",
		"blank": "   ",
		"loop":  "loop now",
	}}

	t.Run("maps an alias to the mapped command line", func(t *testing.T) {
		got, ok := expandAliasArgs(cfg, []string{"tasks"})
		require.True(t, ok)
		assert.Equal(t, []string{"config", "show"}, got)
	})

	t.Run("appends the original trailing arguments", func(t *testing.T) {
		got, ok := expandAliasArgs(cfg, []string{"pf", "list", "--verbose"})
		require.True(t, ok)
		assert.Equal(t, []string{"config", "profile", "list", "--verbose"}, got)
	})

	t.Run("leaves non-alias argv untouched", func(t *testing.T) {
		args := []string{"version"}
		got, ok := expandAliasArgs(cfg, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)
	})

	t.Run("ignores a leading global flag", func(t *testing.T) {
		// Only the command word is aliased: `--profile staging tasks` must
		// keep its normal meaning rather than resolving `--profile` as a
		// command name.
		args := []string{"--profile", "staging", "tasks"}
		got, ok := expandAliasArgs(cfg, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)
	})

	t.Run("ignores a blank mapping", func(t *testing.T) {
		args := []string{"blank", "x"}
		got, ok := expandAliasArgs(cfg, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)
	})

	t.Run("never expands an alias into itself", func(t *testing.T) {
		args := []string{"loop", "x"}
		got, ok := expandAliasArgs(cfg, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)
	})

	t.Run("no aliases and no config are no-ops", func(t *testing.T) {
		args := []string{"tasks"}
		got, ok := expandAliasArgs(nil, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)

		got, ok = expandAliasArgs(&config.Config{}, args)
		assert.False(t, ok)
		assert.Equal(t, args, got)

		got, ok = expandAliasArgs(cfg, nil)
		assert.False(t, ok)
		assert.Nil(t, got)
	})
}

// TestRootCommand_AliasDispatch is the DF-024 regression for command aliases:
// an `aliases:` entry in config.yaml must be expanded before Cobra resolves the
// command, so `openapi-cli <alias>` runs the mapped command instead of failing
// with `unknown command "<alias>"`. The documented feature — the config block,
// the usage example, and the loader — existed end to end except for the one
// step that turned the alias into the command that runs.
func TestRootCommand_AliasDispatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".config", "muster")
	require.NoError(t, os.MkdirAll(cfgDir, 0700))
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte(
		"aliases:\n  tasks: config show\n  pf: config profile\n"), 0600))

	// Control: with no aliases configured the very same argv cannot dispatch.
	// Asserted, not assumed — if `tasks` ever resolves on its own the
	// assertions below stop proving anything.
	control := NewRootCommand(&config.Config{ConfigDir: cfgDir})
	control.SetArgs([]string{"tasks"})
	err := control.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")

	// The alias must arrive through the real config loader (the seam main.go
	// uses), not a hand-built struct.
	loader := config.NewLoader()
	loader.SetConfigFile(cfgFile)
	cfg, err := loader.Load()
	require.NoError(t, err)
	require.Equal(t, "config show", cfg.Aliases["tasks"], "config loader must surface the aliases block")
	require.Equal(t, "config profile", cfg.Aliases["pf"])

	// main.go executes the root without calling SetArgs, so seed os.Args the
	// way a shell invocation would.
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{"openapi-cli", "tasks"}
	out, err := captureStdout(func() error { return NewRootCommand(cfg).Execute() })
	require.NoError(t, err)
	assert.Contains(t, out, "Config Dir:", "`tasks` must dispatch to `config show`")

	// Trailing arguments are appended: `pf list` -> `config profile list`.
	os.Args = []string{"openapi-cli", "pf", "list"}
	out, err = captureStdout(func() error { return NewRootCommand(cfg).Execute() })
	require.NoError(t, err)
	assert.Contains(t, out, "No profiles configured")
}
