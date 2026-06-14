package cmdstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewCommandStore ---

func TestNewCommandStore_NoPath(t *testing.T) {
	cs := NewCommandStore()
	require.NotNil(t, cs)
	assert.Empty(t, cs.commands)
	assert.Empty(t, cs.path)
	assert.False(t, cs.loaded)
}

func TestNewCommandStore_WithPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	cs := NewCommandStore(path)
	require.NotNil(t, cs)
	assert.Equal(t, path, cs.path)
	assert.Empty(t, cs.commands)
}

func TestNewCommandStore_WithEmptyPath(t *testing.T) {
	cs := NewCommandStore("")
	require.NotNil(t, cs)
	assert.Empty(t, cs.path)
}

// --- Commands ---

func TestCommands_EmptyStore(t *testing.T) {
	cs := NewCommandStore()
	cmds, err := cs.Commands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
}

func TestCommands_WithData(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{Name: "test-cmd"}}
	cs.loaded = true

	cmds, err := cs.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "test-cmd", cmds[0].Name)
}

func TestCommands_LoadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")

	// Write data to disk
	stored := StoredData{
		Version:  "1.0",
		Commands: []CommandMetadata{{Name: "disk-cmd", Method: "GET", Path: "/api/test"}},
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err)

	cs := NewCommandStore(path)
	cmds, err := cs.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "disk-cmd", cmds[0].Name)
	assert.True(t, cs.loaded)
}

func TestCommands_LoadFromDisk_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	cs := NewCommandStore(path)
	cmds, err := cs.Commands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
	assert.True(t, cs.loaded)
}

func TestCommands_LoadFromDisk_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.json")

	err := os.WriteFile(path, []byte("{not valid json}"), 0644)
	require.NoError(t, err)

	cs := NewCommandStore(path)
	_, err = cs.Commands()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// --- HasData ---

func TestHasData_EmptyStore(t *testing.T) {
	cs := NewCommandStore()
	assert.False(t, cs.HasData())
}

func TestHasData_WithCommands(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{Name: "test"}}
	assert.True(t, cs.HasData())
}

func TestHasData_WithFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	// Write a non-empty file
	err := os.WriteFile(path, []byte("{}"), 0644)
	require.NoError(t, err)

	cs := NewCommandStore(path)
	assert.True(t, cs.HasData())
}

func TestHasData_EmptyFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	err := os.WriteFile(path, []byte{}, 0644)
	require.NoError(t, err)

	cs := NewCommandStore(path)
	assert.False(t, cs.HasData())
}

func TestHasData_NoPath(t *testing.T) {
	cs := NewCommandStore()
	assert.False(t, cs.HasData())
}

// --- Load ---

func TestLoad_ReturnsMeta(t *testing.T) {
	cs := NewCommandStore()
	cs.meta = &CommandMetadata{Name: "meta-cmd"}
	cs.loaded = true

	meta, err := cs.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "meta-cmd", meta.Name)
}

func TestLoad_FromCommands(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{Name: "first-cmd"}}
	cs.loaded = true

	meta, err := cs.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "first-cmd", meta.Name)
}

func TestLoad_EmptyStore(t *testing.T) {
	cs := NewCommandStore()
	meta, err := cs.Load()
	require.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "no stored data")
}

func TestLoad_FromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "load_test.json")

	stored := StoredData{
		Version:  "1.0",
		Commands: []CommandMetadata{{Name: "disk-load"}},
	}
	data, _ := json.MarshalIndent(stored, "", "  ")
	os.WriteFile(path, data, 0644)

	cs := NewCommandStore(path)
	meta, err := cs.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "disk-load", meta.Name)
}

// --- Save ---

func TestSave_WithData(t *testing.T) {
	cs := NewCommandStore()
	data := &StoredData{
		Commands: []CommandMetadata{{Name: "saved-cmd"}},
	}

	err := cs.Save(data)
	require.NoError(t, err)
	require.Len(t, cs.commands, 1)
	assert.Equal(t, "saved-cmd", cs.commands[0].Name)
	assert.True(t, cs.loaded)
}

func TestSave_NilData(t *testing.T) {
	cs := NewCommandStore()
	err := cs.Save(nil)
	require.NoError(t, err)
	assert.Empty(t, cs.commands)
}

func TestSave_EmptyCommands(t *testing.T) {
	cs := NewCommandStore()
	err := cs.Save(&StoredData{})
	require.NoError(t, err)
	assert.Empty(t, cs.commands)
}

func TestSave_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.json")

	cs := NewCommandStore(path)
	data := &StoredData{
		Version:  "1.0",
		BaseURL:  "https://api.example.com",
		Commands: []CommandMetadata{{Name: "persist-cmd", Method: "GET"}},
	}

	err := cs.Save(data)
	require.NoError(t, err)

	// Verify file was written
	_, err = os.Stat(path)
	require.NoError(t, err, "file should exist after save")

	// Read back and verify
	cs2 := NewCommandStore(path)
	cmds, err := cs2.Commands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "persist-cmd", cmds[0].Name)
}

// --- Clear ---

func TestClear_WithoutPath(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{Name: "test"}}
	cs.meta = &CommandMetadata{Name: "test"}

	err := cs.Clear()
	require.NoError(t, err)
	assert.Empty(t, cs.commands)
	assert.Nil(t, cs.meta)
	assert.True(t, cs.loaded)
}

func TestClear_WithPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clear.json")

	cs := NewCommandStore(path)
	data := &StoredData{
		Commands: []CommandMetadata{{Name: "to-clear"}},
	}
	err := cs.Save(data)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Clear
	err = cs.Clear()
	require.NoError(t, err)
	assert.Empty(t, cs.commands)
	assert.Nil(t, cs.meta)

	// File should still exist (written with empty data)
	_, err = os.Stat(path)
	require.NoError(t, err, "file should still exist after clear")
}

// --- UpdateBaseURL ---

func TestUpdateBaseURL_WithoutPath(t *testing.T) {
	cs := NewCommandStore()
	cs.meta = &CommandMetadata{Name: "test"}

	err := cs.UpdateBaseURL("https://new.example.com")
	require.NoError(t, err)
	assert.Equal(t, "https://new.example.com", cs.meta.BaseURL)
}

func TestUpdateBaseURL_CreatesMetaIfNil(t *testing.T) {
	cs := NewCommandStore()

	err := cs.UpdateBaseURL("https://example.com")
	require.NoError(t, err)
	require.NotNil(t, cs.meta)
	assert.Equal(t, "https://example.com", cs.meta.BaseURL)
}

func TestUpdateBaseURL_WithPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseurl.json")

	cs := NewCommandStore(path)
	// Must save data first so loadFromDisk can populate cs.meta
	err := cs.Save(&StoredData{
		Commands: []CommandMetadata{{Name: "test"}},
	})
	require.NoError(t, err)

	err = cs.UpdateBaseURL("https://updated.example.com")
	require.NoError(t, err)

	// Verify persisted
	cs2 := NewCommandStore(path)
	meta, err := cs2.Load()
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "https://updated.example.com", meta.BaseURL)
}

// --- persistToDisk ---

func TestPersistToDisk_NoPath(t *testing.T) {
	cs := NewCommandStore()
	err := cs.persistToDisk()
	require.NoError(t, err)
}

func TestPersistToDisk_WritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "store.json")

	cs := NewCommandStore(path)
	cs.commands = []CommandMetadata{{Name: "nested-cmd"}}
	cs.meta = &CommandMetadata{Name: "nested-cmd", BaseURL: "https://base.url"}

	err := cs.persistToDisk()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Read and verify content
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var stored StoredData
	err = json.Unmarshal(data, &stored)
	require.NoError(t, err)
	assert.Equal(t, "1.0", stored.Version)
	assert.Equal(t, "https://base.url", stored.BaseURL)
	require.Len(t, stored.Commands, 1)
	assert.Equal(t, "nested-cmd", stored.Commands[0].Name)
}

// --- loadFromDisk ---

func TestLoadFromDisk_NoPath(t *testing.T) {
	cs := NewCommandStore()
	err := cs.loadFromDisk()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no store path")
}

func TestLoadFromDisk_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	cs := NewCommandStore(path)
	err := cs.loadFromDisk()
	require.NoError(t, err)
	assert.True(t, cs.loaded)
	assert.Empty(t, cs.commands)
}

func TestLoadFromDisk_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	os.WriteFile(path, []byte("{bad json}"), 0644)

	cs := NewCommandStore(path)
	err := cs.loadFromDisk()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestLoadFromDisk_ValidData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.json")

	stored := StoredData{
		Version: "1.0",
		BaseURL: "https://api.test.com",
		Commands: []CommandMetadata{
			{Name: "cmd1", Method: "GET", Path: "/api/v1/resource"},
			{Name: "cmd2", Method: "POST", Path: "/api/v1/resource"},
		},
	}
	data, _ := json.MarshalIndent(stored, "", "  ")
	os.WriteFile(path, data, 0644)

	cs := NewCommandStore(path)
	err := cs.loadFromDisk()
	require.NoError(t, err)
	assert.True(t, cs.loaded)
	require.Len(t, cs.commands, 2)
	assert.Equal(t, "cmd1", cs.commands[0].Name)
	assert.Equal(t, "cmd2", cs.commands[1].Name)
	require.NotNil(t, cs.meta)
	assert.Equal(t, "cmd1", cs.meta.Name)
}

// --- LoadCommands (compatibility shim) ---

func TestLoadCommands(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{{Name: "compat-cmd"}}
	cs.loaded = true

	cmds, err := LoadCommands(context.Background(), cs)
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "compat-cmd", cmds[0].Name)
}

// --- ReconstructCommand (compatibility shim) ---

func TestReconstructCommand(t *testing.T) {
	meta := CommandMetadata{Name: "test-cmd"}
	result, err := ReconstructCommand(context.Background(), meta)
	require.NoError(t, err)
	assert.Nil(t, result)
}
