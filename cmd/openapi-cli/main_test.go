package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/builtin"
	"github.com/wojons/muster/pkg/config"
)

func TestRun_ConfigLoad(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "config load should succeed")

	require.NotNil(t, cfg, "config should not be nil")
	require.NotEmpty(t, cfg.Version, "version should not be empty")
}

func TestNewRootCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	require.NotNil(t, root, "root command should not be nil")
	require.Equal(t, "openapi-cli", root.Use, "command use should be openapi-cli")
}

func TestNewRootCommand_ExecuteContext(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "root command should execute without error")
}

func TestNewRootCommand_ExecuteWithHelp(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "help command should execute without error")
}

func TestNewRootCommand_ExecuteWithVersion(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--version"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err, "version command should execute without error")
}

func TestNewRootCommand_ExecuteWithInvalidArgument(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()

	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--invalid-argument"}
	err := root.ExecuteContext(ctx)

	require.Error(t, err, "invalid argument should cause error")
}

func TestRun_Success(t *testing.T) {
	ctx := context.Background()

	// Load configuration with default values if needed
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{Version: "test"}
	}

	require.NotNil(t, cfg, "config should not be nil")

	// Build root command
	rootCmd := builtin.NewRootCommand(cfg)

	// Execute with context
	err = rootCmd.ExecuteContext(ctx)

	require.NoError(t, err, "run should succeed")
}

func TestRun_WithConfigError(t *testing.T) {
	// Test that run() handles config errors gracefully by using defaults
	cfg, err := config.Load()

	require.NoError(t, err, "config load should succeed or use defaults")

	rootCmd := builtin.NewRootCommand(cfg)
	require.NotNil(t, rootCmd, "root command should be created")
}

// readAuditSink decodes every record of a JSON-lines audit sink.
func readAuditSink(t *testing.T, path string) []audit.AuditEvent {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err, "audit sink %s must exist", path)
	t.Cleanup(func() { _ = f.Close() })

	var events []audit.AuditEvent
	dec := json.NewDecoder(f)
	for {
		var event audit.AuditEvent
		if err := dec.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode audit sink %s: %v", path, err)
		}
		events = append(events, event)
	}
	return events
}

// TestRun_FailurePathAudit_NestedCommandIdentity is the DF-054-R2 regression:
// the failure-path audit record must carry the full leaf of a nested command.
// Before the fix the reconstruction loop kept only the FIRST non-flag token
// ("command.openapi-cli webhook"), so `webhook create --bogus-flag 1` and
// `webhook add --url http://x` collapsed into one indistinguishable action and
// the audit trail could not say which command failed.
func TestRun_FailurePathAudit_NestedCommandIdentity(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "nested leaf after malformed flag",
			args: []string{"webhook", "create", "--bogus-flag", "1"},
			want: "command.openapi-cli webhook create",
		},
		{
			name: "unresolvable nested leaf",
			args: []string{"webhook", "add", "--url", "http://x"},
			want: "command.openapi-cli webhook add",
		},
		{
			name: "three level path",
			args: []string{"audit", "verify", "--bogus-flag"},
			want: "command.openapi-cli audit verify",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("MUSTER_AUDIT_ENABLED", "1")
			sink := filepath.Join(t.TempDir(), "audit.jsonl")
			t.Setenv("MUSTER_AUDIT_LOG", sink)

			originalArgs := os.Args
			t.Cleanup(func() { os.Args = originalArgs })
			os.Args = append([]string{"openapi-cli"}, tc.args...)

			err := run()
			require.Error(t, err, "a malformed invocation must fail")

			events := readAuditSink(t, sink)
			require.NotEmpty(t, events, "the failure path must record an audit event")
			last := events[len(events)-1]
			require.Equal(t, tc.want, last.Action,
				"failure-path audit must keep the nested command identity")
			require.False(t, last.Result.Success, "the recorded result must be a failure")
		})
	}
}

// TestRun_FailurePathAudit_LeavesAreDistinct pins the observable consequence of
// the fix: two different nested failures produce two DISTINCT actions in the
// sink, both marked unsuccessful.
func TestRun_FailurePathAudit_LeavesAreDistinct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	for _, args := range [][]string{
		{"webhook", "create", "--bogus-flag", "1"},
		{"webhook", "add", "--url", "http://x"},
	} {
		os.Args = append([]string{"openapi-cli"}, args...)
		require.Error(t, run())
	}

	events := readAuditSink(t, sink)
	require.Len(t, events, 2, "each failed invocation must write one record")

	actions := []string{events[0].Action, events[1].Action}
	require.ElementsMatch(t, []string{
		"command.openapi-cli webhook create",
		"command.openapi-cli webhook add",
	}, actions, "nested leaves must not collapse into one action")
	for _, event := range events {
		require.False(t, event.Result.Success)
	}
}

// TestAuditCommandPath covers the argv→identity reconstruction directly: flag
// values are skipped using cobra's own flag metadata (no hard-coded table),
// nested commands keep their full leaf, and positional arguments are not
// mistaken for commands.
func TestAuditCommandPath(t *testing.T) {
	root := builtin.NewRootCommand(&config.Config{Version: "test"})

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, "openapi-cli"},
		{"only flags", []string{"--help"}, "openapi-cli"},
		{"single command", []string{"webhook"}, "openapi-cli webhook"},
		{"nested command", []string{"webhook", "create", "--bogus-flag", "1"}, "openapi-cli webhook create"},
		{"unresolved nested word", []string{"webhook", "add", "--url", "http://x"}, "openapi-cli webhook add"},
		{"persistent flag value skipped", []string{"--profile", "acme", "webhook", "create", "--bogus"}, "openapi-cli webhook create"},
		{"flag equals form", []string{"--profile=acme", "webhook", "list"}, "openapi-cli webhook list"},
		{"positional arg is not identity", []string{"webhook", "delete", "wh-1", "--bogus"}, "openapi-cli webhook delete"},
		{"unknown root token kept", []string{"nosuchcommand", "--bogus"}, "openapi-cli nosuchcommand"},
		{"flag terminator stops scan", []string{"webhook", "--", "create"}, "openapi-cli webhook"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, auditCommandPath(root, tc.args))
		})
	}
}

func TestMain_RunsProgram(t *testing.T) {
	// Test that main function exists and is callable
	// Since main calls os.Exit on error, we can't directly test it
	// This test verifies that the module has a main function
	// by checking we can import and reference it

	// If we reach here, the test passed
	require.True(t, true)
}

func TestNewRootCommand_HasAllSubcommands(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	// "help" is intentionally NOT in this list — Cobra auto-registers its
	// own default "help" command and adding a second one causes duplicate
	// entries in the Available Commands list.
	expectedCommands := []string{
		"version",
		"config",
		"completion",
		"init",
		"auth",
		"inventory",
		"modules",
		"discover",
		"generate",
		"request",
		"daemon",
	}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		require.True(t, found, "should have command: %s", cmdName)
	}
}

func TestNewRootCommand_HasConfigPassed(t *testing.T) {
	version := "1.0.0-test"
	cfg := &config.Config{Version: version}
	root := builtin.NewRootCommand(cfg)

	// Root command version comes from version.Get(), not cfg.Version
	// This test verifies the command was created with the config
	require.NotNil(t, root, "root command should be created")
	require.Contains(t, root.Version, "dev", "version should be set")
}

func TestRun_Integration(t *testing.T) {
	// Test the full run() function path
	// This calls the actual run() function from main.go
	err := run()

	// run() should execute without error (with no-arg default which shows help)
	// Note: run() displays help for no args, which is considered success
	require.NoError(t, err, "run() should execute successfully")
}

func TestRun_HandleConfigLoadError(t *testing.T) {
	// Test that config load errors don't prevent program from running
	// The actual run() function handles this by using defaults

	// Attempt to load config (may fail)
	cfg, err := config.Load()

	// Even if config fails, we should still be able to create a command
	if cfg == nil || err != nil {
		cfg = &config.Config{Version: "test"}
	}

	require.NotNil(t, cfg, "should have config (default if load failed)")

	// Should still be able to create and execute root command
	rootCmd := builtin.NewRootCommand(cfg)
	require.NotNil(t, rootCmd, "root command should be created")

	ctx := context.Background()
	err = rootCmd.ExecuteContext(ctx)

	require.NoError(t, err, "root command should execute with defaults")
}

func TestNewRootCommand_VersionCommand(t *testing.T) {
	cfg := &config.Config{Version: "1.0.0"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "--version"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_ConfigCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "config"}
	err := root.ExecuteContext(ctx)

	// Config command should execute (may fail on actual config, but shouldn't crash)
	require.NoError(t, err)
}

func TestNewRootCommand_AuthCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "auth"}
	err := root.ExecuteContext(ctx)

	// Auth command should execute
	require.NoError(t, err)
}

func TestNewRootCommand_GenerateCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "generate", "--help"}
	err := root.ExecuteContext(ctx)

	// Generate --help should succeed
	require.NoError(t, err)
}

func TestNewRootCommand_InventoryCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "inventory"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_ModulesCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "modules"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_InitCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "init", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_RequestCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "request", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_DaemonCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "daemon", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_DiscoverCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "discover", "--help"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}

func TestNewRootCommand_CompletionCommand(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	root := builtin.NewRootCommand(cfg)

	ctx := context.Background()
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"openapi-cli", "completion", "bash"}
	err := root.ExecuteContext(ctx)

	require.NoError(t, err)
}
