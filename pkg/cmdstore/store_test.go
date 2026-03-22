package cmdstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandStore verifies the constructor creates a valid store
func TestNewCommandStore(t *testing.T) {
	tests := []struct {
		name     string
		dataPath string
	}{
		{
			name:     "creates store with absolute path",
			dataPath: "/tmp/test/commands.json",
		},
		{
			name:     "creates store with relative path",
			dataPath: "./data/commands.json",
		},
		{
			name:     "creates store with nested directory",
			dataPath: "/tmp/test/nested/deep/commands.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewCommandStore(tt.dataPath)
			require.NotNil(t, store)
			assert.Equal(t, tt.dataPath, store.dataPath)
			assert.Nil(t, store.data)
		})
	}
}

// TestCommandStore_Save verifies saving data to persistent storage
func TestCommandStore_Save(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		data    *StoredData
		wantErr bool
	}{
		{
			name: "saves valid data",
			data: &StoredData{
				Version:     "1.0",
				SpecPath:    "/path/to/spec.yaml",
				SpecHash:    "abc123",
				BaseURL:     "https://api.example.com",
				GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Commands: []CommandMetadata{
					{
						Name:        "get-pet",
						Use:         "get-pet [id]",
						Short:       "Get a pet by ID",
						Long:        "Retrieves detailed information about a specific pet",
						Example:     "muster get-pet 123",
						Path:        "/pets/{id}",
						Method:      "GET",
						GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
						SpecHash:    "abc123",
						Args:        []string{"id"},
						BaseURL:     "https://api.example.com",
						Flags: []FlagMetadata{
							{
								Name:        "format",
								Shorthand:   "f",
								Type:        "string",
								Default:     "json",
								Description: "Output format",
								Required:    false,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "saves empty commands list",
			data: &StoredData{
				Version:     "1.0",
				SpecPath:    "/path/to/spec.yaml",
				SpecHash:    "abc123",
				BaseURL:     "https://api.example.com",
				GeneratedAt: time.Now(),
				Commands:    []CommandMetadata{},
			},
			wantErr: false,
		},
		{
			name: "saves data with multiple commands",
			data: &StoredData{
				Version:     "1.0",
				SpecPath:    "/path/to/spec.yaml",
				SpecHash:    "abc123",
				BaseURL:     "https://api.example.com",
				GeneratedAt: time.Now(),
				Commands: []CommandMetadata{
					{
						Name:     "list-pets",
						Use:      "list-pets",
						Short:    "List all pets",
						Path:     "/pets",
						Method:   "GET",
						BaseURL:  "https://api.example.com",
						SpecHash: "abc123",
					},
					{
						Name:     "create-pet",
						Use:      "create-pet",
						Short:    "Create a new pet",
						Path:     "/pets",
						Method:   "POST",
						BaseURL:  "https://api.example.com",
						SpecHash: "abc123",
					},
					{
						Name:     "delete-pet",
						Use:      "delete-pet [id]",
						Short:    "Delete a pet",
						Path:     "/pets/{id}",
						Method:   "DELETE",
						Args:     []string{"id"},
						BaseURL:  "https://api.example.com",
						SpecHash: "abc123",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "saves nil data",
			data:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)

			err := store.Save(tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify file was created
			_, err = os.Stat(dataPath)
			require.NoError(t, err)

			// Verify data is stored in memory
			if tt.data != nil {
				assert.Equal(t, tt.data, store.data)
			}
		})
	}
}

// TestCommandStore_Save_CreatesDirectory verifies directory creation on save
func TestCommandStore_Save_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "deep", "nested", "dir", "commands.json")

	store := NewCommandStore(nestedPath)
	data := &StoredData{
		Version:     "1.0",
		SpecPath:    "/spec.yaml",
		SpecHash:    "hash",
		BaseURL:     "https://api.example.com",
		GeneratedAt: time.Now(),
		Commands:    []CommandMetadata{},
	}

	err := store.Save(data)
	require.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(nestedPath))
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(nestedPath)
	require.NoError(t, err)
}

// TestCommandStore_Load verifies loading data from persistent storage
func TestCommandStore_Load(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		setup      func(t *testing.T, path string)
		wantData   bool
		wantErr    bool
		checkCache bool
	}{
		{
			name: "loads existing file",
			setup: func(t *testing.T, path string) {
				data := StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "abc123",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
					Commands: []CommandMetadata{
						{
							Name:     "test-cmd",
							Use:      "test-cmd",
							Short:    "Test command",
							Path:     "/test",
							Method:   "GET",
							BaseURL:  "https://api.example.com",
							SpecHash: "abc123",
						},
					},
				}
				bytes, _ := json.MarshalIndent(data, "", "  ")
				err := os.WriteFile(path, bytes, 0644)
				require.NoError(t, err)
			},
			wantData: true,
			wantErr:  false,
		},
		{
			name:     "returns nil for non-existent file",
			setup:    func(t *testing.T, path string) {},
			wantData: false,
			wantErr:  false,
		},
		{
			name: "returns error for invalid JSON",
			setup: func(t *testing.T, path string) {
				err := os.WriteFile(path, []byte("invalid json"), 0644)
				require.NoError(t, err)
			},
			wantData: false,
			wantErr:  true,
		},
		{
			name: "uses cached data when available",
			setup: func(t *testing.T, path string) {
				// Pre-populate the store's data field
			},
			wantData:   true,
			wantErr:    false,
			checkCache: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)

			if tt.checkCache {
				// Pre-populate cache
				store.data = &StoredData{
					Version:  "1.0",
					SpecPath: "/cached",
				}
			} else {
				tt.setup(t, dataPath)
			}

			data, err := store.Load()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantData {
				require.NotNil(t, data)
				if tt.checkCache {
					assert.Equal(t, "/cached", data.SpecPath)
				}
			} else {
				assert.Nil(t, data)
			}
		})
	}
}

// TestCommandStore_Load_CachesData verifies data is cached after first load
func TestCommandStore_Load_CachesData(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "commands.json")

	// Create initial file
	data := StoredData{
		Version:     "1.0",
		SpecPath:    "/original",
		SpecHash:    "hash",
		BaseURL:     "https://api.example.com",
		GeneratedAt: time.Now(),
		Commands:    []CommandMetadata{},
	}
	bytes, _ := json.MarshalIndent(data, "", "  ")
	err := os.WriteFile(dataPath, bytes, 0644)
	require.NoError(t, err)

	store := NewCommandStore(dataPath)

	// First load
	loaded1, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, loaded1)

	// Modify file
	data.SpecPath = "/modified"
	bytes, _ = json.MarshalIndent(data, "", "  ")
	err = os.WriteFile(dataPath, bytes, 0644)
	require.NoError(t, err)

	// Second load should return cached data
	loaded2, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "/original", loaded2.SpecPath)
	assert.Equal(t, loaded1, loaded2) // Same pointer
}

// TestCommandStore_Clear verifies clearing stored data
func TestCommandStore_Clear(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		setup     func(t *testing.T, path string)
		wantErr   bool
		wantExist bool
	}{
		{
			name: "clears existing file",
			setup: func(t *testing.T, path string) {
				data := StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands:    []CommandMetadata{},
				}
				bytes, _ := json.MarshalIndent(data, "", "  ")
				err := os.WriteFile(path, bytes, 0644)
				require.NoError(t, err)
			},
			wantErr:   false,
			wantExist: false,
		},
		{
			name:      "succeeds when file doesn't exist",
			setup:     func(t *testing.T, path string) {},
			wantErr:   false,
			wantExist: false,
		},
		{
			name: "clears memory cache",
			setup: func(t *testing.T, path string) {
				// Just create file, data will be loaded into memory
				data := StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands:    []CommandMetadata{},
				}
				bytes, _ := json.MarshalIndent(data, "", "  ")
				err := os.WriteFile(path, bytes, 0644)
				require.NoError(t, err)
			},
			wantErr:   false,
			wantExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)
			tt.setup(t, dataPath)

			// Load data into cache first
			if tt.name == "clears memory cache" {
				_, _ = store.Load()
				require.NotNil(t, store.data)
			}

			err := store.Clear()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify file doesn't exist
			_, err = os.Stat(dataPath)
			if tt.wantExist {
				require.NoError(t, err)
			} else {
				require.True(t, os.IsNotExist(err))
			}

			// Verify cache is cleared
			assert.Nil(t, store.data)
		})
	}
}

// TestCommandStore_HasData verifies checking for stored data
func TestCommandStore_HasData(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		setup    func(t *testing.T, path string, store *CommandStore)
		wantData bool
	}{
		{
			name: "returns true when data exists",
			setup: func(t *testing.T, path string, store *CommandStore) {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands: []CommandMetadata{
						{
							Name:     "test",
							Path:     "/test",
							Method:   "GET",
							BaseURL:  "https://api.example.com",
							SpecHash: "hash",
						},
					},
				}
				err := store.Save(data)
				require.NoError(t, err)
			},
			wantData: true,
		},
		{
			name:     "returns false when no file exists",
			setup:    func(t *testing.T, path string, store *CommandStore) {},
			wantData: false,
		},
		{
			name: "returns false when commands list is empty",
			setup: func(t *testing.T, path string, store *CommandStore) {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands:    []CommandMetadata{},
				}
				err := store.Save(data)
				require.NoError(t, err)
			},
			wantData: false,
		},
		{
			name: "returns false for invalid JSON",
			setup: func(t *testing.T, path string, store *CommandStore) {
				err := os.WriteFile(path, []byte("invalid"), 0644)
				require.NoError(t, err)
			},
			wantData: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)
			tt.setup(t, dataPath, store)

			hasData := store.HasData()
			assert.Equal(t, tt.wantData, hasData)
		})
	}
}

// TestCommandStore_Commands verifies retrieving command list
func TestCommandStore_Commands(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		setup        func(t *testing.T, path string, store *CommandStore)
		wantErr      bool
		wantCount    int
		wantFirstCmd string
	}{
		{
			name:      "returns empty slice when no data",
			setup:     func(t *testing.T, path string, store *CommandStore) {},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name: "returns commands when data exists",
			setup: func(t *testing.T, path string, store *CommandStore) {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands: []CommandMetadata{
						{
							Name:     "list-pets",
							Path:     "/pets",
							Method:   "GET",
							BaseURL:  "https://api.example.com",
							SpecHash: "hash",
						},
						{
							Name:     "create-pet",
							Path:     "/pets",
							Method:   "POST",
							BaseURL:  "https://api.example.com",
							SpecHash: "hash",
						},
					},
				}
				err := store.Save(data)
				require.NoError(t, err)
			},
			wantErr:      false,
			wantCount:    2,
			wantFirstCmd: "list-pets",
		},
		{
			name: "returns error for invalid JSON",
			setup: func(t *testing.T, path string, store *CommandStore) {
				err := os.WriteFile(path, []byte("invalid"), 0644)
				require.NoError(t, err)
			},
			wantErr:   true,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)
			tt.setup(t, dataPath, store)

			commands, err := store.Commands()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, commands, tt.wantCount)

			if tt.wantCount > 0 && tt.wantFirstCmd != "" {
				assert.Equal(t, tt.wantFirstCmd, commands[0].Name)
			}
		})
	}
}

// TestCommandStore_UpdateBaseURL verifies updating base URL
func TestCommandStore_UpdateBaseURL(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		setup       func(t *testing.T, path string, store *CommandStore)
		newBaseURL  string
		wantErr     bool
		wantErrMsg  string
		wantBaseURL string
		wantCmdURLs []string
	}{
		{
			name: "updates base URL successfully",
			setup: func(t *testing.T, path string, store *CommandStore) {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://old.example.com",
					GeneratedAt: time.Now(),
					Commands: []CommandMetadata{
						{
							Name:     "cmd1",
							Path:     "/test",
							Method:   "GET",
							BaseURL:  "https://old.example.com",
							SpecHash: "hash",
						},
						{
							Name:     "cmd2",
							Path:     "/test2",
							Method:   "POST",
							BaseURL:  "https://old.example.com",
							SpecHash: "hash",
						},
					},
				}
				err := store.Save(data)
				require.NoError(t, err)
			},
			newBaseURL:  "https://new.example.com",
			wantErr:     false,
			wantBaseURL: "https://new.example.com",
			wantCmdURLs: []string{"https://new.example.com", "https://new.example.com"},
		},
		{
			name:        "returns error when no data exists",
			setup:       func(t *testing.T, path string, store *CommandStore) {},
			newBaseURL:  "https://new.example.com",
			wantErr:     true,
			wantErrMsg:  "no stored data",
			wantBaseURL: "",
		},
		{
			name: "handles empty command list",
			setup: func(t *testing.T, path string, store *CommandStore) {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://old.example.com",
					GeneratedAt: time.Now(),
					Commands:    []CommandMetadata{},
				}
				err := store.Save(data)
				require.NoError(t, err)
			},
			newBaseURL:  "https://new.example.com",
			wantErr:     false,
			wantBaseURL: "https://new.example.com",
			wantCmdURLs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath := filepath.Join(tmpDir, tt.name+".json")
			store := NewCommandStore(dataPath)
			tt.setup(t, dataPath, store)

			err := store.UpdateBaseURL(tt.newBaseURL)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)

			// Verify data in memory
			data, err := store.Load()
			require.NoError(t, err)
			assert.Equal(t, tt.wantBaseURL, data.BaseURL)

			for i, cmd := range data.Commands {
				if i < len(tt.wantCmdURLs) {
					assert.Equal(t, tt.wantCmdURLs[i], cmd.BaseURL)
				}
			}
		})
	}
}

// TestCommandStore_ConcurrentAccess verifies thread safety
func TestCommandStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "concurrent.json")
	store := NewCommandStore(dataPath)

	// Initial data
	initialData := &StoredData{
		Version:     "1.0",
		SpecPath:    "/spec.yaml",
		SpecHash:    "hash",
		BaseURL:     "https://api.example.com",
		GeneratedAt: time.Now(),
		Commands: []CommandMetadata{
			{
				Name:     "test-cmd",
				Path:     "/test",
				Method:   "GET",
				BaseURL:  "https://api.example.com",
				SpecHash: "hash",
			},
		},
	}
	err := store.Save(initialData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 5
	iterations := 20

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = store.Load()
				_, _ = store.Commands()
				_ = store.HasData()
			}
		}()
	}

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				data := &StoredData{
					Version:     "1.0",
					SpecPath:    "/spec.yaml",
					SpecHash:    "hash",
					BaseURL:     "https://api.example.com",
					GeneratedAt: time.Now(),
					Commands: []CommandMetadata{
						{
							Name:     "cmd",
							Path:     "/test",
							Method:   "GET",
							BaseURL:  "https://api.example.com",
							SpecHash: "hash",
						},
					},
				}
				_ = store.Save(data)
			}
		}(i)
	}

	wg.Wait()

	// Verify store is still functional
	data, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.GreaterOrEqual(t, len(data.Commands), 0)
}

// TestCommandMetadata_Integrity verifies JSON marshaling/unmarshaling
func TestCommandMetadata_Integrity(t *testing.T) {
	original := CommandMetadata{
		Name:        "get-pet",
		Use:         "get-pet [id]",
		Short:       "Get a pet by ID",
		Long:        "Retrieves detailed information about a specific pet by its unique identifier",
		Example:     "muster get-pet 123",
		Path:        "/pets/{id}",
		Method:      "GET",
		GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		SpecHash:    "abc123def456",
		Args:        []string{"id"},
		BaseURL:     "https://api.example.com",
		Flags: []FlagMetadata{
			{
				Name:        "format",
				Shorthand:   "f",
				Type:        "string",
				Default:     "json",
				Description: "Output format (json, yaml, table)",
				Required:    false,
			},
			{
				Name:        "verbose",
				Shorthand:   "v",
				Type:        "bool",
				Default:     "false",
				Description: "Enable verbose output",
				Required:    false,
			},
		},
	}

	// Marshal to JSON
	bytes, err := json.MarshalIndent(original, "", "  ")
	require.NoError(t, err)

	// Unmarshal back
	var decoded CommandMetadata
	err = json.Unmarshal(bytes, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Use, decoded.Use)
	assert.Equal(t, original.Short, decoded.Short)
	assert.Equal(t, original.Long, decoded.Long)
	assert.Equal(t, original.Example, decoded.Example)
	assert.Equal(t, original.Path, decoded.Path)
	assert.Equal(t, original.Method, decoded.Method)
	assert.Equal(t, original.GeneratedAt.Unix(), decoded.GeneratedAt.Unix())
	assert.Equal(t, original.SpecHash, decoded.SpecHash)
	assert.Equal(t, original.Args, decoded.Args)
	assert.Equal(t, original.BaseURL, decoded.BaseURL)
	assert.Len(t, decoded.Flags, len(original.Flags))
	for i, flag := range original.Flags {
		assert.Equal(t, flag.Name, decoded.Flags[i].Name)
		assert.Equal(t, flag.Shorthand, decoded.Flags[i].Shorthand)
		assert.Equal(t, flag.Type, decoded.Flags[i].Type)
		assert.Equal(t, flag.Default, decoded.Flags[i].Default)
		assert.Equal(t, flag.Description, decoded.Flags[i].Description)
		assert.Equal(t, flag.Required, decoded.Flags[i].Required)
	}
}

// TestStoredData_Integrity verifies complete data structure marshaling
func TestStoredData_Integrity(t *testing.T) {
	original := StoredData{
		Version:     "1.0.0",
		SpecPath:    "/path/to/openapi.yaml",
		SpecHash:    "sha256:abc123...",
		BaseURL:     "https://api.example.com/v1",
		GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Commands: []CommandMetadata{
			{
				Name:     "list-pets",
				Use:      "list-pets",
				Short:    "List all pets",
				Path:     "/pets",
				Method:   "GET",
				BaseURL:  "https://api.example.com/v1",
				SpecHash: "sha256:abc123...",
			},
			{
				Name:     "get-pet",
				Use:      "get-pet [id]",
				Short:    "Get a pet by ID",
				Path:     "/pets/{id}",
				Method:   "GET",
				Args:     []string{"id"},
				BaseURL:  "https://api.example.com/v1",
				SpecHash: "sha256:abc123...",
			},
		},
	}

	// Marshal
	bytes, err := json.MarshalIndent(original, "", "  ")
	require.NoError(t, err)

	// Unmarshal
	var decoded StoredData
	err = json.Unmarshal(bytes, &decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.SpecPath, decoded.SpecPath)
	assert.Equal(t, original.SpecHash, decoded.SpecHash)
	assert.Equal(t, original.BaseURL, decoded.BaseURL)
	assert.Equal(t, original.GeneratedAt.Unix(), decoded.GeneratedAt.Unix())
	assert.Len(t, decoded.Commands, len(original.Commands))
}

// TestFlagMetadata_Values verifies flag metadata with various types
func TestFlagMetadata_Values(t *testing.T) {
	tests := []struct {
		name     string
		flag     FlagMetadata
		wantType string
	}{
		{
			name: "string flag",
			flag: FlagMetadata{
				Name:        "name",
				Type:        "string",
				Default:     "default",
				Description: "A string flag",
				Required:    false,
			},
			wantType: "string",
		},
		{
			name: "int flag",
			flag: FlagMetadata{
				Name:        "count",
				Type:        "int",
				Default:     "0",
				Description: "An integer flag",
				Required:    false,
			},
			wantType: "int",
		},
		{
			name: "bool flag",
			flag: FlagMetadata{
				Name:        "verbose",
				Shorthand:   "v",
				Type:        "bool",
				Default:     "false",
				Description: "A boolean flag",
				Required:    false,
			},
			wantType: "bool",
		},
		{
			name: "required flag",
			flag: FlagMetadata{
				Name:        "api-key",
				Type:        "string",
				Description: "A required flag",
				Required:    true,
			},
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal and unmarshal
			bytes, err := json.Marshal(tt.flag)
			require.NoError(t, err)

			var decoded FlagMetadata
			err = json.Unmarshal(bytes, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.flag.Name, decoded.Name)
			assert.Equal(t, tt.flag.Type, decoded.Type)
			assert.Equal(t, tt.flag.Required, decoded.Required)
		})
	}
}

// TestCommandStore_Save_NilData verifies saving nil data creates empty file
func TestCommandStore_Save_NilData(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "nil.json")
	store := NewCommandStore(dataPath)

	err := store.Save(nil)
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(dataPath)
	require.NoError(t, err)

	// Content should be "null"
	content, err := os.ReadFile(dataPath)
	require.NoError(t, err)
	assert.Equal(t, "null", string(content))
}
