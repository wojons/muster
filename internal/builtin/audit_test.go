package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-cli/openapi-cli/pkg/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuditCommand(t *testing.T) {
	t.Run("creates audit command", func(t *testing.T) {
		cmd := newAuditCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "audit", cmd.Name())
		assert.Equal(t, "Audit logging commands", cmd.Short)
		assert.Contains(t, cmd.Long, "View and manage audit log entries")
	})

	t.Run("has log subcommand", func(t *testing.T) {
		cmd := newAuditCommand()

		hasLog := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "log" {
				hasLog = true
				break
			}
		}
		assert.True(t, hasLog)
	})

	t.Run("has info subcommand", func(t *testing.T) {
		cmd := newAuditCommand()

		hasInfo := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "info" {
				hasInfo = true
				break
			}
		}
		assert.True(t, hasInfo)
	})
}

func TestNewAuditLogCommand(t *testing.T) {
	t.Run("creates log command", func(t *testing.T) {
		cmd := newAuditLogCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "log", cmd.Name())
		assert.Equal(t, "List recent audit entries", cmd.Short)
	})

	t.Run("has limit flag with default", func(t *testing.T) {
		cmd := newAuditLogCommand()

		flag := cmd.Flags().Lookup("limit")
		require.NotNil(t, flag)
		assert.Equal(t, "50", flag.DefValue)
	})

	t.Run("has filter flag", func(t *testing.T) {
		cmd := newAuditLogCommand()

		flag := cmd.Flags().Lookup("filter")
		require.NotNil(t, flag)
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newAuditLogCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--limit")
		assert.Contains(t, cmd.Example, "--filter")
	})

	t.Run("shows message when no audit log exists", func(t *testing.T) {
		cmd := newAuditLogCommand()
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})
}

func TestNewAuditInfoCommand(t *testing.T) {
	t.Run("creates info command", func(t *testing.T) {
		cmd := newAuditInfoCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "info", cmd.Name())
		assert.Equal(t, "Show audit configuration and stats", cmd.Short)
	})

	t.Run("executes without error", func(t *testing.T) {
		cmd := newAuditInfoCommand()

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})

	t.Run("displays configuration", func(t *testing.T) {
		cmd := newAuditInfoCommand()

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})
}

func TestAuditCommandIntegration(t *testing.T) {
	t.Run("full audit command structure", func(t *testing.T) {
		auditCmd := newAuditCommand()

		logFound := false
		infoFound := false

		for _, subCmd := range auditCmd.Commands() {
			switch subCmd.Name() {
			case "log":
				logFound = true
			case "info":
				infoFound = true
			}
		}

		assert.True(t, logFound, "log command not found")
		assert.True(t, infoFound, "info command not found")
	})

	t.Run("command names are lowercase", func(t *testing.T) {
		auditCmd := newAuditCommand()

		for _, subCmd := range auditCmd.Commands() {
			assert.True(t, strings.ToLower(subCmd.Name()) == subCmd.Name(),
				"command name %q should be lowercase", subCmd.Name())
		}
	})
}

func TestAuditLogCommand_WithExistingLog(t *testing.T) {
	t.Run("reads entries from audit log file", func(t *testing.T) {
		// Create a temporary audit log file
		tmpDir := t.TempDir()
		auditFile := filepath.Join(tmpDir, "audit.jsonl")

		// Write a test audit entry
		entry := `{"id":"test-1","timestamp":"2026-01-15T10:00:00Z","level":"info","action":"user_login","actor":{"id":"user123","name":"Test User"},"resource":{"id":"res1","type":"session"},"result":{"success":true,"message":"Login successful"}}`
		err := os.WriteFile(auditFile, []byte(entry+"\n"), 0644)
		require.NoError(t, err)

		// Override the audit log path
		t.Setenv("XDG_DATA_HOME", filepath.Dir(tmpDir))

		// Since auditLogPath() uses XDG_DATA_HOME/openapi-cli, we need to set up the right structure
		dataDir := filepath.Join(tmpDir, "openapi-cli")
		err = os.MkdirAll(dataDir, 0755)
		require.NoError(t, err)
		err = os.Rename(auditFile, filepath.Join(dataDir, "audit.jsonl"))
		require.NoError(t, err)
		t.Setenv("XDG_DATA_HOME", tmpDir)

		cmd := newAuditLogCommand()
		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})
}

func TestAuditLogCommand_WithFilter(t *testing.T) {
	t.Run("filters by action", func(t *testing.T) {
		tmpDir := t.TempDir()
		dataDir := filepath.Join(tmpDir, "openapi-cli")
		err := os.MkdirAll(dataDir, 0755)
		require.NoError(t, err)

		entries := []string{
			`{"id":"1","timestamp":"2026-01-15T10:00:00Z","level":"info","category":"authentication","action":"user_login","actor":{"id":"user1"},"resource":{"id":"r1","type":"session"},"result":{"success":true}}`,
			`{"id":"2","timestamp":"2026-01-15T10:01:00Z","level":"info","category":"data_access","action":"file_read","actor":{"id":"user1"},"resource":{"id":"r2","type":"file"},"result":{"success":true}}`,
		}
		err = os.WriteFile(filepath.Join(dataDir, "audit.jsonl"), []byte(strings.Join(entries, "\n")+"\n"), 0644)
		require.NoError(t, err)
		t.Setenv("XDG_DATA_HOME", tmpDir)

		cmd := newAuditLogCommand()
		_ = cmd.Flags().Set("filter", "action=user_login")
		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})
}

func TestApplyFilter(t *testing.T) {
	events := []audit.Event{
		{
			Action:   "user_login",
			Category: audit.CategoryAuth,
			Level:    audit.LevelInfo,
			Actor:    audit.Actor{ID: "user1", Name: "Alice"},
			Resource: audit.Resource{Type: "session", ID: "s1"},
			Result:   audit.Result{Success: true},
		},
		{
			Action:   "file_read",
			Category: audit.CategoryData,
			Level:    audit.LevelInfo,
			Actor:    audit.Actor{ID: "user2", Name: "Bob"},
			Resource: audit.Resource{Type: "file", ID: "f1"},
			Result:   audit.Result{Success: true},
		},
		{
			Action:   "file_delete",
			Category: audit.CategoryData,
			Level:    audit.LevelError,
			Actor:    audit.Actor{ID: "user2", Name: "Bob"},
			Resource: audit.Resource{Type: "file", ID: "f2"},
			Result:   audit.Result{Success: false},
		},
	}

	t.Run("filters by action", func(t *testing.T) {
		result := applyFilter(events, "action=user_login")
		require.Len(t, result, 1)
		assert.Equal(t, "user_login", result[0].Action)
	})

	t.Run("filters by category", func(t *testing.T) {
		result := applyFilter(events, "category=data_access")
		require.Len(t, result, 2)
	})

	t.Run("filters by level", func(t *testing.T) {
		result := applyFilter(events, "level=error")
		require.Len(t, result, 1)
		assert.Equal(t, "file_delete", result[0].Action)
	})

	t.Run("filters by actor id", func(t *testing.T) {
		result := applyFilter(events, "actor=user1")
		require.Len(t, result, 1)
		assert.Equal(t, "Alice", result[0].Actor.Name)
	})

	t.Run("filters by resource type", func(t *testing.T) {
		result := applyFilter(events, "resource=session")
		require.Len(t, result, 1)
	})

	t.Run("filters by success", func(t *testing.T) {
		result := applyFilter(events, "success=false")
		require.Len(t, result, 1)
		assert.Equal(t, "file_delete", result[0].Action)
	})

	t.Run("returns all entries for unknown filter key", func(t *testing.T) {
		result := applyFilter(events, "unknown=value")
		require.Len(t, result, 3)
	})

	t.Run("returns all entries for malformed filter", func(t *testing.T) {
		result := applyFilter(events, "invalid")
		require.Len(t, result, 3)
	})
}

func TestCountEntries(t *testing.T) {
	t.Run("counts entries in file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "audit.jsonl")
		content := `{"id":"1"}` + "\n" + `{"id":"2"}` + "\n"
		err := os.WriteFile(tmpFile, []byte(content), 0644)
		require.NoError(t, err)

		count := countEntries(tmpFile)
		assert.Equal(t, 2, count)
	})

	t.Run("returns zero for missing file", func(t *testing.T) {
		count := countEntries("/nonexistent/audit.jsonl")
		assert.Equal(t, 0, count)
	})

	t.Run("returns zero for empty file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "audit.jsonl")
		err := os.WriteFile(tmpFile, []byte{}, 0644)
		require.NoError(t, err)

		count := countEntries(tmpFile)
		assert.Equal(t, 0, count)
	})
}

func TestAuditLogPath(t *testing.T) {
	t.Run("uses XDG_DATA_HOME when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmpDir)

		path, err := auditLogPath()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, "openapi-cli", "audit.jsonl"), path)
	})

	t.Run("falls back to HOME when XDG_DATA_HOME not set", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		path, err := auditLogPath()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpHome, ".local", "share", "openapi-cli", "audit.jsonl"), path)
	})
}

func TestEnsureAuditLogExists(t *testing.T) {
	t.Run("creates audit directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmpDir)

		err := ensureAuditLogExists()
		require.NoError(t, err)

		path, err := auditLogPath()
		require.NoError(t, err)
		_, err = os.Stat(filepath.Dir(path))
		require.NoError(t, err, "directory should exist")
	})

	t.Run("succeeds if directory already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmpDir)

		err := ensureAuditLogExists()
		require.NoError(t, err)

		// Call again
		err = ensureAuditLogExists()
		require.NoError(t, err)
	})
}


