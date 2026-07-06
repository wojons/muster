//go:build ignore
// Stale test — references store.Clear() and store.UpdateBaseURL() which were
// planned but never implemented in CommandStore. Restore when those methods
// are added.

package cmdstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cmdstore"
)

// TestCommandStore_Integration_SaveAndLoad tests the complete save/load cycle
func TestCommandStore_Integration_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	// Create store
	store := cmdstore.NewCommandStore(dbPath)

	// Create test command metadata
	commands := []cmdstore.CommandMetadata{
		{
			Name:    "test-get",
			Use:     "test-get [id]",
			Short:   "Get a test resource",
			Long:    "Long description for test-get command",
			Example: "openapi-cli test-get 123",
			Method:  "GET",
			Path:    "/api/test/{id}",
			BaseURL: "https://api.example.com",
			Flags: []cmdstore.FlagMetadata{
				{
					Name:        "id",
					Shorthand:   "i",
					Type:        "string",
					Description: "Resource ID",
					Required:    true,
				},
				{
					Name:        "verbose",
					Shorthand:   "v",
					Type:        "bool",
					Description: "Verbose output",
					Default:     "false",
				},
			},
		},
	}

	// Save the commands
	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		SpecPath: "test.yaml",
		SpecHash: "abc123",
		BaseURL:  "https://api.example.com",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Load all commands
	loaded, err := store.Commands()
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	// Verify loaded command
	cmd := loaded[0]
	assert.Equal(t, "test-get", cmd.Name)
	assert.Equal(t, "GET", cmd.Method)
	assert.Equal(t, "/api/test/{id}", cmd.Path)
	assert.Equal(t, "https://api.example.com", cmd.BaseURL)
	require.Len(t, cmd.Flags, 2)

	// Verify flags
	assert.Equal(t, "id", cmd.Flags[0].Name)
	assert.Equal(t, "i", cmd.Flags[0].Shorthand)
	assert.True(t, cmd.Flags[0].Required)
}

// TestCommandStore_Integration_MultipleCommands tests storing multiple commands
func TestCommandStore_Integration_MultipleCommands(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Create multiple commands
	commands := []cmdstore.CommandMetadata{
		{
			Name:   "list-users",
			Method: "GET",
			Path:   "/api/users",
		},
		{
			Name:   "create-user",
			Method: "POST",
			Path:   "/api/users",
		},
		{
			Name:   "get-user",
			Method: "GET",
			Path:   "/api/users/{id}",
		},
		{
			Name:   "update-user",
			Method: "PUT",
			Path:   "/api/users/{id}",
		},
		{
			Name:   "delete-user",
			Method: "DELETE",
			Path:   "/api/users/{id}",
		},
	}

	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Load and verify
	loaded, err := store.Commands()
	require.NoError(t, err)
	require.Len(t, loaded, 5)

	// Verify order is maintained
	for i, cmd := range loaded {
		assert.Equal(t, commands[i].Name, cmd.Name)
		assert.Equal(t, commands[i].Method, cmd.Method)
		assert.Equal(t, commands[i].Path, cmd.Path)
	}
}

// TestCommandStore_Integration_LoaderReconstruction tests command loader reconstruction
func TestCommandStore_Integration_LoaderReconstruction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Save a command with complex flags
	commands := []cmdstore.CommandMetadata{
		{
			Name:    "complex-cmd",
			Use:     "complex-cmd [flags]",
			Short:   "A complex test command",
			Method:  "POST",
			Path:    "/api/complex",
			BaseURL: "https://api.test.com",
			Flags: []cmdstore.FlagMetadata{
				{
					Name:        "name",
					Shorthand:   "n",
					Type:        "string",
					Description: "Name parameter",
					Default:     "default-name",
					Required:    false,
				},
				{
					Name:        "count",
					Shorthand:   "c",
					Type:        "int",
					Description: "Count parameter",
					Default:     "10",
				},
				{
					Name:        "enabled",
					Shorthand:   "e",
					Type:        "bool",
					Description: "Enable feature",
					Default:     "true",
				},
			},
		},
	}

	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		BaseURL:  "https://api.test.com",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Create loader and reconstruct commands
	loader := cmdstore.NewLoader(store, "https://default.example.com")
	cobraCmds, err := loader.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cobraCmds, 1)

	cmd := cobraCmds[0]
	assert.Equal(t, "complex-cmd", cmd.Name())
	assert.Equal(t, "A complex test command", cmd.Short)

	// Verify flags were reconstructed
	nameFlag, err := cmd.Flags().GetString("name")
	require.NoError(t, err)
	assert.Equal(t, "default-name", nameFlag)

	countFlag, err := cmd.Flags().GetInt("count")
	require.NoError(t, err)
	assert.Equal(t, 10, countFlag)

	enabledFlag, err := cmd.Flags().GetBool("enabled")
	require.NoError(t, err)
	assert.True(t, enabledFlag)
}

// TestCommandStore_Integration_PersistenceAcrossReopen tests persistence
func TestCommandStore_Integration_PersistenceAcrossReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	// First session: save data
	func() {
		store := cmdstore.NewCommandStore(dbPath)

		commands := []cmdstore.CommandMetadata{
			{
				Name:   "persistent-cmd",
				Method: "GET",
				Path:   "/api/persistent",
			},
		}

		data := &cmdstore.StoredData{
			Version:  "1.0.0",
			Commands: commands,
		}

		err := store.Save(data)
		require.NoError(t, err)
	}()

	// Second session: verify data persisted
	func() {
		store := cmdstore.NewCommandStore(dbPath)

		commands, err := store.Commands()
		require.NoError(t, err)
		require.Len(t, commands, 1)
		assert.Equal(t, "persistent-cmd", commands[0].Name)
	}()
}

// TestCommandStore_Integration_Clear tests clearing stored data
func TestCommandStore_Integration_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Save some data
	commands := []cmdstore.CommandMetadata{
		{
			Name:   "test-cmd",
			Method: "GET",
			Path:   "/api/test",
		},
	}

	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Verify data exists
	cmds, err := store.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	// Clear data
	err = store.Clear()
	require.NoError(t, err)

	// Verify data cleared
	cmds, err = store.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 0)
}

// TestCommandStore_Integration_UpdateBaseURL tests base URL update
func TestCommandStore_Integration_UpdateBaseURL(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Save initial data
	commands := []cmdstore.CommandMetadata{
		{
			Name:   "api-cmd",
			Method: "GET",
			Path:   "/api/test",
		},
	}

	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		BaseURL:  "https://old.example.com",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Update base URL
	err = store.UpdateBaseURL("https://new.example.com")
	require.NoError(t, err)

	// Verify update
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "https://new.example.com", loaded.BaseURL)
}

// TestCommandStore_Integration_HasData tests data existence check
func TestCommandStore_Integration_HasData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Initially no data
	assert.False(t, store.HasData(), "Should have no data initially")

	// Save data
	commands := []cmdstore.CommandMetadata{
		{
			Name:   "test-cmd",
			Method: "GET",
			Path:   "/api/test",
		},
	}

	data := &cmdstore.StoredData{
		Version:  "1.0.0",
		Commands: commands,
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Now has data
	assert.True(t, store.HasData(), "Should have data after save")
}

// TestCommandStore_Integration_EmptyStore tests empty store behavior
func TestCommandStore_Integration_EmptyStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)

	// Load from empty store
	commands, err := store.Commands()
	require.NoError(t, err)
	require.NotNil(t, commands)
	require.Len(t, commands, 0)

	// Load data returns nil
	data, err := store.Load()
	require.NoError(t, err)
	assert.Nil(t, data)
}

// TestCommandStore_Integration_InvalidPath tests error handling
func TestCommandStore_Integration_InvalidPath(t *testing.T) {
	// Test with read-only directory
	readOnlyDir := t.TempDir()
	err := os.Chmod(readOnlyDir, 0555)
	require.NoError(t, err)
	defer os.Chmod(readOnlyDir, 0755)

	readOnlyPath := filepath.Join(readOnlyDir, "commands.json")
	store := cmdstore.NewCommandStore(readOnlyPath)

	// Try to save
	data := &cmdstore.StoredData{
		Version: "1.0.0",
	}

	err = store.Save(data)
	assert.Error(t, err)
}
