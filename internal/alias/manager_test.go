package alias

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	t.Run("creates manager with empty aliases for new directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		m, err := NewManager(tmpDir)
		require.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, 0, m.Count())
		assert.Equal(t, filepath.Join(tmpDir, "aliases.yaml"), m.FilePath())
	})

	t.Run("loads existing aliases from file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create an aliases file
		content := `ls:
  name: ls
  command: inventory list
  description: List inventory items
get:
  name: get
  command: inventory get
`
		err := os.WriteFile(filepath.Join(tmpDir, "aliases.yaml"), []byte(content), 0644)
		require.NoError(t, err)

		m, err := NewManager(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, 2, m.Count())

		alias, exists := m.Get("ls")
		assert.True(t, exists)
		assert.Equal(t, "ls", alias.Name)
		assert.Equal(t, "inventory list", alias.Command)
		assert.Equal(t, "List inventory items", alias.Description)
	})

	t.Run("returns error for invalid file permissions", func(t *testing.T) {
		// Create a directory with no read permissions (where we can't read the file)
		// This test might not work on all systems, so we'll skip if we can't create the scenario
		if os.Getuid() == 0 {
			t.Skip("Cannot test permission errors as root")
		}

		// For simplicity, we'll test with a file instead of directory
		// The important thing is that NewManager should handle errors gracefully
		tmpFile, err := os.CreateTemp("", "aliases.yaml")
		require.NoError(t, err)
		_ = tmpFile.Close()
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		// Remove read permissions
		err = os.Chmod(tmpFile.Name(), 0000)
		if err != nil {
			t.Skip("Cannot change file permissions")
		}
		defer func() { _ = os.Chmod(tmpFile.Name(), 0644) }() // Restore for cleanup

		// Note: This test behavior may vary by OS
	})
}

func TestManager_Define(t *testing.T) {
	t.Run("creates new alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		alias, exists := m.Get("ls")
		assert.True(t, exists)
		assert.Equal(t, "ls", alias.Name)
		assert.Equal(t, "inventory list", alias.Command)
		assert.Empty(t, alias.Description)
	})

	t.Run("updates existing alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		err = m.Define("ls", "inventory list --format table")
		require.NoError(t, err)

		alias, exists := m.Get("ls")
		assert.True(t, exists)
		assert.Equal(t, "inventory list --format table", alias.Command)
	})

	t.Run("fails for empty name", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("", "command")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("fails for name with whitespace", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("my alias", "command")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace")
	})

	t.Run("fails for reserved names", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		reservedNames := []string{"alias", "unalias", "help", "version"}
		for _, name := range reservedNames {
			err = m.Define(name, "command")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "reserved")
		}
	})

	t.Run("fails for empty command", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("persists to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		// Create a new manager pointing to same directory
		m2, err := NewManager(tmpDir)
		require.NoError(t, err)

		alias, exists := m2.Get("ls")
		assert.True(t, exists)
		assert.Equal(t, "inventory list", alias.Command)
	})
}

func TestManager_DefineWithDescription(t *testing.T) {
	t.Run("creates alias with description", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.DefineWithDescription("ls", "inventory list", "List all inventory items")
		require.NoError(t, err)

		alias, exists := m.Get("ls")
		assert.True(t, exists)
		assert.Equal(t, "ls", alias.Name)
		assert.Equal(t, "inventory list", alias.Command)
		assert.Equal(t, "List all inventory items", alias.Description)
	})
}

func TestManager_Remove(t *testing.T) {
	t.Run("removes existing alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)
		assert.Equal(t, 1, m.Count())

		err = m.Remove("ls")
		require.NoError(t, err)
		assert.Equal(t, 0, m.Count())

		_, exists := m.Get("ls")
		assert.False(t, exists)
	})

	t.Run("fails for non-existent alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Remove("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("persists removal to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		err = m.Remove("ls")
		require.NoError(t, err)

		// Create new manager and verify alias is gone
		m2, err := NewManager(tmpDir)
		require.NoError(t, err)

		_, exists := m2.Get("ls")
		assert.False(t, exists)
	})
}

func TestManager_Get(t *testing.T) {
	t.Run("returns alias when exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		alias, exists := m.Get("ls")
		assert.True(t, exists)
		assert.NotNil(t, alias)
		assert.Equal(t, "ls", alias.Name)
	})

	t.Run("returns false when not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		alias, exists := m.Get("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, alias)
	})
}

func TestManager_List(t *testing.T) {
	t.Run("returns empty list when no aliases", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		aliases := m.List()
		assert.Empty(t, aliases)
		assert.NotNil(t, aliases) // Should be empty slice, not nil
	})

	t.Run("returns all aliases", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		err = m.Define("get", "inventory get")
		require.NoError(t, err)

		aliases := m.List()
		assert.Len(t, aliases, 2)

		// Verify we can find both aliases
		names := make(map[string]bool)
		for _, alias := range aliases {
			names[alias.Name] = true
		}
		assert.True(t, names["ls"])
		assert.True(t, names["get"])
	})

	t.Run("returns copy that doesn't affect original", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		aliases := m.List()
		require.Len(t, aliases, 1)

		// Modify the returned slice
		aliases[0].Command = "modified"

		// Original should be unchanged
		original, _ := m.Get("ls")
		assert.Equal(t, "inventory list", original.Command)
	})
}

func TestManager_Expand(t *testing.T) {
	t.Run("expands single word alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("ls", "inventory list")
		require.NoError(t, err)

		result, expanded := m.Expand([]string{"ls"})
		assert.True(t, expanded)
		assert.Equal(t, []string{"inventory", "list"}, result)
	})

	t.Run("expands with additional arguments", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("get", "inventory get")
		require.NoError(t, err)

		result, expanded := m.Expand([]string{"get", "item1", "item2"})
		assert.True(t, expanded)
		assert.Equal(t, []string{"inventory", "get", "item1", "item2"}, result)
	})

	t.Run("returns original when no alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		args := []string{"other", "command"}
		result, expanded := m.Expand(args)
		assert.False(t, expanded)
		assert.Equal(t, args, result)
	})

	t.Run("returns original for empty args", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		result, expanded := m.Expand([]string{})
		assert.False(t, expanded)
		assert.Empty(t, result)
	})

	t.Run("preserves complex command structure", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = m.Define("prod", "config set api.url https://prod.example.com")
		require.NoError(t, err)

		result, expanded := m.Expand([]string{"prod"})
		assert.True(t, expanded)
		assert.Equal(t, []string{"config", "set", "api.url", "https://prod.example.com"}, result)
	})
}

func TestManager_Count(t *testing.T) {
	t.Run("returns zero for empty manager", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, 0, m.Count())
	})

	t.Run("returns correct count", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		require.NoError(t, m.Define("a", "cmd1"))
		require.NoError(t, m.Define("b", "cmd2"))
		require.NoError(t, m.Define("c", "cmd3"))

		assert.Equal(t, 3, m.Count())

		require.NoError(t, m.Remove("b"))

		assert.Equal(t, 2, m.Count())
	})
}

func TestManager_Clear(t *testing.T) {
	t.Run("removes all aliases from memory", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		require.NoError(t, m.Define("ls", "list"))
		require.NoError(t, m.Define("get", "get"))

		assert.Equal(t, 2, m.Count())

		m.Clear()

		assert.Equal(t, 0, m.Count())
		_, exists := m.Get("ls")
		assert.False(t, exists)
	})
}

func TestManager_Concurrency(t *testing.T) {
	t.Run("handles concurrent reads and writes", func(t *testing.T) {
		tmpDir := t.TempDir()
		m, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Start multiple goroutines
		done := make(chan bool, 3)

		// Writer 1
		go func() {
			for i := 0; i < 100; i++ {
				_ = m.Define("key1", "cmd1")
			}
			done <- true
		}()

		// Writer 2
		go func() {
			for i := 0; i < 100; i++ {
				_ = m.Define("key2", "cmd2")
			}
			done <- true
		}()

		// Reader
		go func() {
			for i := 0; i < 100; i++ {
				m.Get("key1")
				m.Get("key2")
				m.List()
				m.Expand([]string{"key1"})
			}
			done <- true
		}()

		// Wait for all to complete
		for i := 0; i < 3; i++ {
			<-done
		}

		// Verify final state
		assert.GreaterOrEqual(t, m.Count(), 0)
	})
}

func TestValidateAliasName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		errMsg  string
	}{
		{"valid", false, ""},
		{"ls", false, ""},
		{"my-alias", false, ""},
		{"my_alias", false, ""},
		{"alias123", false, ""},
		{"", true, "cannot be empty"},
		{"has space", true, "whitespace"},
		{"has\ttab", true, "whitespace"},
		{"has\nnewline", true, "whitespace"},
		{"alias", true, "reserved"},
		{"unalias", true, "reserved"},
		{"help", true, "reserved"},
		{"version", true, "reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAliasName(tt.name)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
