package alias

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewYAMLStorage(t *testing.T) {
	t.Run("creates storage with given path", func(t *testing.T) {
		path := "/tmp/test/aliases.yaml"
		storage := NewYAMLStorage(path)
		require.NotNil(t, storage)
		assert.Equal(t, path, storage.Path())
	})
}

func TestYAMLStorage_Load(t *testing.T) {
	t.Run("returns empty map for non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		aliases, err := storage.Load()

		require.NoError(t, err)
		assert.NotNil(t, aliases)
		assert.Empty(t, aliases)
	})

	t.Run("loads aliases from existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		content := `ls:
  name: ls
  command: inventory list
  description: List inventory items
get:
  name: get
  command: inventory get
`
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)

		storage := NewYAMLStorage(path)
		aliases, err := storage.Load()

		require.NoError(t, err)
		assert.Len(t, aliases, 2)

		ls, exists := aliases["ls"]
		require.True(t, exists)
		assert.Equal(t, "ls", ls.Name)
		assert.Equal(t, "inventory list", ls.Command)
		assert.Equal(t, "List inventory items", ls.Description)

		get, exists := aliases["get"]
		require.True(t, exists)
		assert.Equal(t, "get", get.Name)
		assert.Equal(t, "inventory get", get.Command)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		err := os.WriteFile(path, []byte("invalid: yaml: content: ["), 0644)
		require.NoError(t, err)

		storage := NewYAMLStorage(path)
		aliases, err := storage.Load()

		require.Error(t, err)
		assert.Nil(t, aliases)
		assert.Contains(t, err.Error(), "failed to parse")
	})

	t.Run("returns error for unreadable file", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("Cannot test permission errors as root")
		}

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		err := os.WriteFile(path, []byte("test: content"), 0644)
		require.NoError(t, err)

		err = os.Chmod(path, 0000)
		if err != nil {
			t.Skip("Cannot change file permissions")
		}
		defer func() {
			_ = os.Chmod(path, 0644) // Restore for cleanup
		}()

		storage := NewYAMLStorage(path)
		aliases, err := storage.Load()

		require.Error(t, err)
		assert.Nil(t, aliases)
	})
}

func TestYAMLStorage_Save(t *testing.T) {
	t.Run("saves aliases to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		aliases := map[string]*Alias{
			"ls": {
				Name:        "ls",
				Command:     "inventory list",
				Description: "List inventory items",
			},
			"get": {
				Name:    "get",
				Command: "inventory get",
			},
		}

		err := storage.Save(aliases)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Verify content by loading
		storage2 := NewYAMLStorage(path)
		loaded, err := storage2.Load()
		require.NoError(t, err)

		assert.Len(t, loaded, 2)
		assert.Equal(t, "inventory list", loaded["ls"].Command)
		assert.Equal(t, "inventory get", loaded["get"].Command)
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "subdir1", "subdir2", "aliases.yaml")

		storage := NewYAMLStorage(path)
		aliases := map[string]*Alias{
			"test": {Name: "test", Command: "test cmd"},
		}

		err := storage.Save(aliases)
		require.NoError(t, err)

		// Verify directories were created
		_, err = os.Stat(filepath.Dir(path))
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(path)
		require.NoError(t, err)
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		// Create initial file
		storage := NewYAMLStorage(path)
		err := storage.Save(map[string]*Alias{
			"old": {Name: "old", Command: "old cmd"},
		})
		require.NoError(t, err)

		// Overwrite with new aliases
		err = storage.Save(map[string]*Alias{
			"new": {Name: "new", Command: "new cmd"},
		})
		require.NoError(t, err)

		// Verify new content
		loaded, err := storage.Load()
		require.NoError(t, err)

		assert.Len(t, loaded, 1)
		assert.Equal(t, "new cmd", loaded["new"].Command)
		_, exists := loaded["old"]
		assert.False(t, exists)
	})

	t.Run("handles empty alias map", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		err := storage.Save(map[string]*Alias{})
		require.NoError(t, err)

		// Verify file was created (empty YAML map)
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Verify it loads as empty
		loaded, err := storage.Load()
		require.NoError(t, err)
		assert.Empty(t, loaded)
	})

	t.Run("saves with correct file permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		err := storage.Save(map[string]*Alias{
			"test": {Name: "test", Command: "test cmd"},
		})
		require.NoError(t, err)

		info, err := os.Stat(path)
		require.NoError(t, err)

		// Check permissions (0644 = owner read/write, group read, others read)
		mode := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0644), mode)
	})
}

func TestYAMLStorage_Path(t *testing.T) {
	t.Run("returns configured path", func(t *testing.T) {
		path := "/custom/path/to/aliases.yaml"
		storage := NewYAMLStorage(path)
		assert.Equal(t, path, storage.Path())
	})
}

func TestYAMLStorage_RoundTrip(t *testing.T) {
	t.Run("load after save returns same data", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		original := map[string]*Alias{
			"ls": {
				Name:        "ls",
				Command:     "inventory list",
				Description: "List inventory",
			},
			"get": {
				Name:        "get",
				Command:     "inventory get",
				Description: "Get item",
			},
			"prod": {
				Name:    "prod",
				Command: "config set api.url https://prod.example.com",
			},
		}

		storage := NewYAMLStorage(path)
		err := storage.Save(original)
		require.NoError(t, err)

		loaded, err := storage.Load()
		require.NoError(t, err)

		require.Len(t, loaded, len(original))
		for name, orig := range original {
			loaded, exists := loaded[name]
			require.True(t, exists, "alias %s should exist", name)
			assert.Equal(t, orig.Name, loaded.Name)
			assert.Equal(t, orig.Command, loaded.Command)
			assert.Equal(t, orig.Description, loaded.Description)
		}
	})
}

func TestYAMLStorage_Concurrency(t *testing.T) {
	t.Run("handles concurrent reads", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		// Setup initial data
		storage := NewYAMLStorage(path)
		err := storage.Save(map[string]*Alias{
			"test": {Name: "test", Command: "test cmd"},
		})
		require.NoError(t, err)

		// Concurrent reads
		done := make(chan bool, 3)
		for i := 0; i < 3; i++ {
			go func() {
				aliases, err := storage.Load()
				assert.NoError(t, err)
				assert.NotNil(t, aliases)
				done <- true
			}()
		}

		for i := 0; i < 3; i++ {
			<-done
		}
	})

	t.Run("handles concurrent writes", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		done := make(chan bool, 3)

		for i := 0; i < 3; i++ {
			go func(id int) {
				aliases := map[string]*Alias{
					string(rune('a' + id)): {Name: string(rune('a' + id)), Command: "cmd"},
				}
				err := storage.Save(aliases)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		for i := 0; i < 3; i++ {
			<-done
		}

		// Verify file is in a valid state
		loaded, err := storage.Load()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(loaded), 0)
	})

	t.Run("handles concurrent read and write", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "aliases.yaml")

		storage := NewYAMLStorage(path)
		done := make(chan bool, 4)

		// Writer
		go func() {
			for i := 0; i < 50; i++ {
				aliases := map[string]*Alias{
					"key": {Name: "key", Command: "cmd"},
				}
				_ = storage.Save(aliases)
			}
			done <- true
		}()

		// Readers
		for i := 0; i < 3; i++ {
			go func() {
				for i := 0; i < 50; i++ {
					_, _ = storage.Load()
				}
				done <- true
			}()
		}

		for i := 0; i < 4; i++ {
			<-done
		}
	})
}
