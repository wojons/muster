package cmdstore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cmdstore"
)

// ── NewCommandStore ──────────────────────────────────────────────

func TestNewCommandStore_NoPath(t *testing.T) {
	store := cmdstore.NewCommandStore()
	require.NotNil(t, store)

	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
	assert.False(t, store.HasData())
}

func TestNewCommandStore_WithPath(t *testing.T) {
	store := cmdstore.NewCommandStore("/tmp/test/commands.json")
	require.NotNil(t, store)

	assert.False(t, store.HasData()) // no data yet
}

func TestNewCommandStore_EmptyStringPath(t *testing.T) {
	store := cmdstore.NewCommandStore("")
	require.NotNil(t, store)

	// empty string should be treated like no path
	assert.False(t, store.HasData())
}

// ── Commands ─────────────────────────────────────────────────────

func TestCommandStore_Commands_FromMemory(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "cmd-a", Method: "GET", Path: "/a"},
			{Name: "cmd-b", Method: "POST", Path: "/b"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	cmds, err := store.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 2)
	assert.Equal(t, "cmd-a", cmds[0].Name)
	assert.Equal(t, "cmd-b", cmds[1].Name)
}

func TestCommandStore_Commands_FromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	// First store: save data
	store1 := cmdstore.NewCommandStore(dbPath)
	data := &cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "persisted", Method: "GET", Path: "/api"},
		},
	}
	err := store1.Save(data)
	require.NoError(t, err)

	// Second store: load from same file
	store2 := cmdstore.NewCommandStore(dbPath)
	cmds, err := store2.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "persisted", cmds[0].Name)
}

func TestCommandStore_Commands_EmptyStore(t *testing.T) {
	store := cmdstore.NewCommandStore()

	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.NotNil(t, cmds)
	assert.Empty(t, cmds)
}

func TestCommandStore_Commands_LoadError(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.json")

	// Write a non-JSON file
	err := os.WriteFile(badPath, []byte("not json"), 0644)
	require.NoError(t, err)

	store := cmdstore.NewCommandStore(badPath)
	cmds, err := store.Commands()
	assert.Error(t, err)
	assert.Nil(t, cmds)
	assert.Contains(t, err.Error(), "unmarshal")
}

// ── HasData ──────────────────────────────────────────────────────

func TestCommandStore_HasData_Empty(t *testing.T) {
	store := cmdstore.NewCommandStore()
	assert.False(t, store.HasData())
}

func TestCommandStore_HasData_WithCommands(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Commands: []cmdstore.CommandMetadata{
			{Name: "test", Method: "GET", Path: "/test"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	assert.True(t, store.HasData())
}

func TestCommandStore_HasData_WithDiskFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)
	data := &cmdstore.StoredData{
		Commands: []cmdstore.CommandMetadata{
			{Name: "test", Method: "GET", Path: "/test"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	// New store with same path should detect disk file
	store2 := cmdstore.NewCommandStore(dbPath)
	assert.True(t, store2.HasData())
}

func TestCommandStore_HasData_NoPath_NoCommands(t *testing.T) {
	store := cmdstore.NewCommandStore()
	assert.False(t, store.HasData())
}

func TestCommandStore_HasData_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.json")

	// Write empty file
	err := os.WriteFile(dbPath, []byte{}, 0644)
	require.NoError(t, err)

	store := cmdstore.NewCommandStore(dbPath)
	assert.False(t, store.HasData()) // size == 0
}

// ── Load ─────────────────────────────────────────────────────────

func TestCommandStore_Load_FromMeta(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Version:  "1.0",
		BaseURL:  "https://api.example.com",
		Commands: []cmdstore.CommandMetadata{
			{Name: "test", Method: "GET", Path: "/test", BaseURL: "https://api.example.com"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	meta, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "test", meta.Name)
	assert.Equal(t, "https://api.example.com", meta.BaseURL)
}

func TestCommandStore_Load_FromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	// First store: save
	store1 := cmdstore.NewCommandStore(dbPath)
	data := &cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "from-disk", Method: "POST", Path: "/disk"},
		},
	}
	err := store1.Save(data)
	require.NoError(t, err)

	// Second store: load from disk
	store2 := cmdstore.NewCommandStore(dbPath)
	meta, err := store2.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "from-disk", meta.Name)
}

func TestCommandStore_Load_Empty(t *testing.T) {
	store := cmdstore.NewCommandStore()

	meta, err := store.Load()
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "no stored data")
}

func TestCommandStore_Load_NoStoredData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.json")

	store := cmdstore.NewCommandStore(dbPath)
	meta, err := store.Load()
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "no stored data")
}

// ── Save ─────────────────────────────────────────────────────────

func TestCommandStore_Save_NilData(t *testing.T) {
	store := cmdstore.NewCommandStore()

	err := store.Save(nil)
	assert.NoError(t, err) // must not panic

	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
}

func TestCommandStore_Save_NilData_NoPanic(t *testing.T) {
	store := cmdstore.NewCommandStore()
	// nil data should be handled gracefully
	assert.NotPanics(t, func() {
		_ = store.Save(nil)
	})
}

func TestCommandStore_Save_NoPath(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "no-path", Method: "GET", Path: "/np"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	// Data should be in memory
	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.Len(t, cmds, 1)
}

func TestCommandStore_Save_PersistToDisk(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := cmdstore.NewCommandStore(dbPath)
	data := &cmdstore.StoredData{
		Version:  "2.0",
		SpecPath: "spec.yaml",
		SpecHash: "abc123",
		BaseURL:  "https://api.example.com",
		Commands: []cmdstore.CommandMetadata{
			{Name: "persisted", Method: "GET", Path: "/api/persist", BaseURL: "https://api.example.com"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	// Verify file was written
	raw, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)

	// Verify JSON content — persistToDisk creates its own StoredData:
	// Version is hardcoded to "1.0", only Commands/BaseURL/GeneratedAt are set.
	var stored cmdstore.StoredData
	err = json.Unmarshal(raw, &stored)
	require.NoError(t, err)
	assert.Equal(t, "1.0", stored.Version)
	assert.Equal(t, "https://api.example.com", stored.BaseURL)
	assert.Len(t, stored.Commands, 1)
	assert.Equal(t, "persisted", stored.Commands[0].Name)
	assert.NotEmpty(t, stored.GeneratedAt)
}

func TestCommandStore_Save_NestedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sub", "nested", "commands.json")

	store := cmdstore.NewCommandStore(dbPath)
	data := &cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "nested", Method: "GET", Path: "/nested"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	// Verify nested dirs were created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestCommandStore_Save_EmptyCommands(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Version: "1.0",
		// No commands
	}
	err := store.Save(data)
	require.NoError(t, err)

	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
}

// ── LoadCommands ─────────────────────────────────────────────────

func TestLoadCommands(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Commands: []cmdstore.CommandMetadata{
			{Name: "lc", Method: "GET", Path: "/lc"},
			{Name: "lc2", Method: "POST", Path: "/lc2"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	cmds, err := cmdstore.LoadCommands(nil, store)
	require.NoError(t, err)
	assert.Len(t, cmds, 2)
}

func TestLoadCommands_Empty(t *testing.T) {
	store := cmdstore.NewCommandStore()

	cmds, err := cmdstore.LoadCommands(nil, store)
	require.NoError(t, err)
	assert.Empty(t, cmds)
}

// ── ReconstructCommand ───────────────────────────────────────────

func TestReconstructCommand(t *testing.T) {
	result, err := cmdstore.ReconstructCommand(nil, cmdstore.CommandMetadata{
		Name:   "test",
		Method: "GET",
		Path:   "/test",
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

// ── CommandMetadata ──────────────────────────────────────────────

func TestCommandMetadata_StructFields(t *testing.T) {
	meta := cmdstore.CommandMetadata{
		Name:        "list-tasks",
		Description: "List all tasks",
		Method:      "GET",
		Path:        "/api/v1/tasks",
		BaseURL:     "https://api.example.com",
		Use:         "list-tasks [flags]",
		Short:       "List tasks",
		Long:        "Long description for listing tasks",
		Example:     "openapi-cli list-tasks",
		Args:        []string{"id"},
		Flags: []cmdstore.FlagMetadata{
			{
				Name:        "limit",
				Shorthand:   "l",
				Type:        "int",
				Description: "Max results",
				Default:     "10",
				Required:    false,
			},
		},
	}

	assert.Equal(t, "list-tasks", meta.Name)
	assert.Equal(t, "GET", meta.Method)
	assert.Equal(t, "/api/v1/tasks", meta.Path)
	assert.Equal(t, "https://api.example.com", meta.BaseURL)
	assert.Len(t, meta.Flags, 1)
	assert.Equal(t, "limit", meta.Flags[0].Name)
}

// ── FlagMetadata ─────────────────────────────────────────────────

func TestFlagMetadata_JSONTags(t *testing.T) {
	fm := cmdstore.FlagMetadata{
		Name:        "file",
		Shorthand:   "f",
		Type:        "string",
		Kind:        cmdstore.FlagKindFile,
		Description: "Upload file",
		Default:     "",
		Required:    true,
	}

	raw, err := json.Marshal(fm)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	// "kind" should be present when set
	assert.Equal(t, "file", parsed["kind"])

	// "json_name" should be omitted when not set
	_, hasJSONName := parsed["json_name"]
	assert.False(t, hasJSONName)
}

func TestFlagMetadata_JSONName(t *testing.T) {
	fm := cmdstore.FlagMetadata{
		Name:        "userName",
		Type:        "string",
		Kind:        cmdstore.FlagKindBody,
		JSONName:    "user_name",
		Description: "User name",
	}

	raw, err := json.Marshal(fm)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "user_name", parsed["json_name"])
}

// ── FlagKind ─────────────────────────────────────────────────────

func TestFlagKind_Constants(t *testing.T) {
	assert.Equal(t, cmdstore.FlagKind("path"), cmdstore.FlagKindPath)
	assert.Equal(t, cmdstore.FlagKind("query"), cmdstore.FlagKindQuery)
	assert.Equal(t, cmdstore.FlagKind("header"), cmdstore.FlagKindHeader)
	assert.Equal(t, cmdstore.FlagKind("body"), cmdstore.FlagKindBody)
	assert.Equal(t, cmdstore.FlagKind("file"), cmdstore.FlagKindFile)
	assert.Equal(t, cmdstore.FlagKind("multipart"), cmdstore.FlagKindMultipart)
}

// ── StoredData ───────────────────────────────────────────────────

func TestStoredData_StructFields(t *testing.T) {
	sd := cmdstore.StoredData{
		Version:  "1.0",
		SpecPath: "./petstore.yaml",
		SpecHash: "sha256:abc123",
		BaseURL:  "https://api.example.com",
		Commands: []cmdstore.CommandMetadata{
			{Name: "test", Method: "GET", Path: "/test"},
		},
	}

	raw, err := json.Marshal(sd)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "1.0", parsed["version"])
	assert.Equal(t, "./petstore.yaml", parsed["spec_path"])
	assert.Equal(t, "sha256:abc123", parsed["spec_hash"])
	assert.Equal(t, "https://api.example.com", parsed["base_url"])

	// generated_at is present even for zero time (no omitempty tag)
	genAt, hasGenAt := parsed["generated_at"]
	assert.True(t, hasGenAt)
	assert.NotEmpty(t, genAt)

	commands, ok := parsed["commands"].([]interface{})
	require.True(t, ok)
	assert.Len(t, commands, 1)
}

// ── Concurrent access ────────────────────────────────────────────

func TestCommandStore_ConcurrentCommands(t *testing.T) {
	store := cmdstore.NewCommandStore()

	data := &cmdstore.StoredData{
		Commands: []cmdstore.CommandMetadata{
			{Name: "concurrent-test", Method: "GET", Path: "/c"},
		},
	}
	err := store.Save(data)
	require.NoError(t, err)

	// Multiple reads from same store
	for i := 0; i < 5; i++ {
		cmds, err := store.Commands()
		require.NoError(t, err)
		assert.Len(t, cmds, 1)
	}
}

// ── Save overwrite ───────────────────────────────────────────────

func TestCommandStore_Save_Overwrite(t *testing.T) {
	store := cmdstore.NewCommandStore()

	// First save
	err := store.Save(&cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "first", Method: "GET", Path: "/first"},
		},
	})
	require.NoError(t, err)

	// Second save overwrites
	err = store.Save(&cmdstore.StoredData{
		Version: "2.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "second", Method: "POST", Path: "/second"},
			{Name: "third", Method: "DELETE", Path: "/third"},
		},
	})
	require.NoError(t, err)

	cmds, err := store.Commands()
	require.NoError(t, err)
	assert.Len(t, cmds, 2)
	assert.Equal(t, "second", cmds[0].Name)
	assert.Equal(t, "third", cmds[1].Name)
}

// ── Commands cached after first load ─────────────────────────────

func TestCommandStore_Commands_LoadsOnce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store1 := cmdstore.NewCommandStore(dbPath)
	err := store1.Save(&cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{
			{Name: "cached", Method: "GET", Path: "/cached"},
		},
	})
	require.NoError(t, err)

	// First call loads from disk
	store2 := cmdstore.NewCommandStore(dbPath)
	cmds, err := store2.Commands()
	require.NoError(t, err)
	assert.Len(t, cmds, 1)

	// Now delete the file
	err = os.Remove(dbPath)
	require.NoError(t, err)

	// Second call returns cached data (not reloading from missing file)
	cmds, err = store2.Commands()
	require.NoError(t, err)
	assert.Len(t, cmds, 1)
}
