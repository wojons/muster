package cmdstore

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLoader tests the loader constructor
func TestNewLoader(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
	loader := NewLoader(store, "https://api.example.com")

	assert.NotNil(t, loader)
	assert.Equal(t, store, loader.store)
	assert.Equal(t, "https://api.example.com", loader.baseURL)
}

// TestLoader_LoadCommands tests command loading
func TestLoader_LoadCommands(t *testing.T) {
	t.Run("empty store", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, "https://api.example.com")

		cmds, err := loader.LoadCommands()
		require.NoError(t, err)
		assert.Empty(t, cmds)
	})

	t.Run("store with commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))

		data := &StoredData{
			Version: "1.0.0",
			BaseURL: "https://api.petstore.com",
			Commands: []CommandMetadata{
				{
					Name:    "get-pet",
					Use:     "get-pet [id]",
					Short:   "Get a pet by ID",
					Long:    "Retrieve a pet from the store",
					Method:  "GET",
					Path:    "/pets/{id}",
					BaseURL: "https://api.petstore.com",
					Args:    []string{"id"},
					Flags: []FlagMetadata{
						{
							Name:        "verbose",
							Shorthand:   "v",
							Type:        "bool",
							Description: "Enable verbose output",
							Default:     "false",
							Required:    false,
						},
					},
				},
			},
		}

		err := store.Save(data)
		require.NoError(t, err)

		loader := NewLoader(store, "https://api.example.com")
		cmds, err := loader.LoadCommands()
		require.NoError(t, err)
		require.Len(t, cmds, 1)

		cmd := cmds[0]
		assert.Equal(t, "get-pet [id]", cmd.Use)
		assert.Equal(t, "Get a pet by ID", cmd.Short)
		assert.Equal(t, "Retrieve a pet from the store", cmd.Long)
	})

	t.Run("malformed JSON in store", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))

		// Write invalid JSON
		err := os.WriteFile(filepath.Join(tmpDir, "commands.json"), []byte("{invalid json}"), 0644)
		require.NoError(t, err)

		loader := NewLoader(store, "https://api.example.com")
		cmds, err := loader.LoadCommands()
		assert.Error(t, err)
		assert.Nil(t, cmds)
	})
}

// TestLoader_GetBaseURL tests base URL retrieval
func TestLoader_GetBaseURL(t *testing.T) {
	t.Run("metadata with base URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))

		data := &StoredData{
			Version: "1.0.0",
			BaseURL: "https://custom.api.com",
		}
		err := store.Save(data)
		require.NoError(t, err)

		loader := NewLoader(store, "https://default.com")
		url, err := loader.GetBaseURL()
		require.NoError(t, err)
		assert.Equal(t, "https://custom.api.com", url)
	})

	t.Run("metadata without base URL uses default", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))

		data := &StoredData{
			Version: "1.0.0",
		}
		err := store.Save(data)
		require.NoError(t, err)

		loader := NewLoader(store, "https://default.com")
		url, err := loader.GetBaseURL()
		require.NoError(t, err)
		assert.Equal(t, "https://default.com", url)
	})

	t.Run("empty store returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, "https://default.com")

		_, err := loader.GetBaseURL()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no stored data")
	})
}

// TestLoader_reconstructCommand tests command reconstruction
func TestLoader_reconstructCommand(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
	loader := NewLoader(store, "https://api.example.com")

	t.Run("basic command", func(t *testing.T) {
		meta := CommandMetadata{
			Name:    "test-cmd",
			Use:     "test [args]",
			Short:   "Test command",
			Long:    "Long description",
			Example: "muster test",
			Method:  "GET",
			Path:    "/test",
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		assert.Equal(t, "test [args]", cmd.Use)
		assert.Equal(t, "Test command", cmd.Short)
		assert.Equal(t, "Long description", cmd.Long)
		assert.Equal(t, "muster test", cmd.Example)
	})

	t.Run("string flag", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:        "format",
					Shorthand:   "f",
					Type:        "string",
					Description: "Output format",
					Default:     "json",
					Required:    false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetString("format")
		require.NoError(t, err)
		assert.Equal(t, "json", val)
	})

	t.Run("int flag", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:        "limit",
					Shorthand:   "l",
					Type:        "int",
					Description: "Page limit",
					Default:     "100",
					Required:    false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetInt("limit")
		require.NoError(t, err)
		assert.Equal(t, 100, val)
	})

	t.Run("int flag with empty default", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:     "offset",
					Type:     "int",
					Default:  "",
					Required: false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetInt("offset")
		require.NoError(t, err)
		assert.Equal(t, 0, val)
	})

	t.Run("bool flag", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:        "verbose",
					Shorthand:   "v",
					Type:        "bool",
					Description: "Verbose output",
					Default:     "true",
					Required:    false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetBool("verbose")
		require.NoError(t, err)
		assert.True(t, val)
	})

	t.Run("bool flag false default", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:     "quiet",
					Type:     "bool",
					Default:  "false",
					Required: false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetBool("quiet")
		require.NoError(t, err)
		assert.False(t, val)
	})

	t.Run("array flag", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:        "tags",
					Shorthand:   "t",
					Type:        "array",
					Description: "Filter by tags",
					Default:     "production",
					Required:    false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetStringSlice("tags")
		require.NoError(t, err)
		assert.Equal(t, []string{"production"}, val)
	})

	t.Run("array flag with empty default", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:     "filters",
					Type:     "array",
					Default:  "",
					Required: false,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetStringSlice("filters")
		require.NoError(t, err)
		assert.Empty(t, val)
	})

	t.Run("required flag", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{
					Name:        "id",
					Type:        "string",
					Description: "Resource ID",
					Required:    true,
				},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		cmd.SetArgs([]string{})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err = cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required flag(s) \"id\" not set")
	})

	t.Run("output flag always present", func(t *testing.T) {
		meta := CommandMetadata{
			Name:   "test",
			Use:    "test",
			Short:  "Test",
			Method: "GET",
			Path:   "/test",
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		val, err := cmd.Flags().GetString("output")
		require.NoError(t, err)
		assert.Equal(t, "table", val)
	})
}

// TestLoader_createRunHandler tests the HTTP request handler
func TestLoader_createRunHandler(t *testing.T) {
	t.Run("successful GET request", func(t *testing.T) {
		responseBody := `{"name": "Fluffy"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/pets/123", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(responseBody))
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, server.URL)

		meta := CommandMetadata{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Short:  "Get pet",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		cmd.SetArgs([]string{"123", "--output=json"})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err = cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Fluffy")
	})

	t.Run("wrong number of arguments", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, server.URL)

		meta := CommandMetadata{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Short:  "Get pet",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		cmd.SetArgs([]string{})
		err = cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 1 arguments, got 0")
	})

	t.Run("query parameters from flags", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "limit=50&offset=100", r.URL.RawQuery)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, server.URL)

		meta := CommandMetadata{
			Name:   "list-pets",
			Use:    "list-pets",
			Short:  "List pets",
			Method: "GET",
			Path:   "/pets",
			Flags: []FlagMetadata{
				{Name: "limit", Type: "int", Default: "50"},
				{Name: "offset", Type: "int", Default: "100"},
			},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		cmd.SetArgs([]string{})
		err = cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("HTTP error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, "Not found")
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, server.URL)

		meta := CommandMetadata{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Short:  "Get pet",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		cmd.SetArgs([]string{"999"})
		err = cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP error")
	})

	t.Run("JSON output", func(t *testing.T) {
		responseBody := `{"name": "Fluffy", "type": "cat"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(responseBody))
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
		loader := NewLoader(store, server.URL)

		meta := CommandMetadata{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Short:  "Get pet",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		}

		cmd, err := loader.reconstructCommand(meta)
		require.NoError(t, err)

		cmd.SetArgs([]string{"123", "--output=json"})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err = cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Fluffy")
	})
}

// TestLoader_createRunHandler_BaseURLOverride tests base URL override
func TestLoader_createRunHandler_BaseURLOverride(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "server1")
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"server": "server2"}`)
	}))
	defer server2.Close()

	tmpDir := t.TempDir()
	store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))

	meta := CommandMetadata{
		Name:    "test",
		Use:     "test",
		Short:   "Test",
		Method:  "GET",
		Path:    "/",
		BaseURL: server2.URL,
	}

	loader := NewLoader(store, server1.URL)

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	cmd.SetArgs([]string{"--output=json"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "server2")
}

// TestLoader_getFlagValue tests flag value extraction
func TestLoader_getFlagValue(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewCommandStore(filepath.Join(tmpDir, "commands.json"))
	loader := NewLoader(store, "https://api.example.com")

	t.Run("string flag", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("str-flag", "default", "")

		flagMeta := FlagMetadata{Name: "str-flag", Type: "string"}
		val, err := loader.getFlagValue(cmd, flagMeta)
		require.NoError(t, err)
		assert.Equal(t, "default", val)
	})

	t.Run("int flag", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int("int-flag", 42, "")

		flagMeta := FlagMetadata{Name: "int-flag", Type: "int"}
		val, err := loader.getFlagValue(cmd, flagMeta)
		require.NoError(t, err)
		assert.Equal(t, 42, val)
	})

	t.Run("bool flag", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("bool-flag", true, "")

		flagMeta := FlagMetadata{Name: "bool-flag", Type: "bool"}
		val, err := loader.getFlagValue(cmd, flagMeta)
		require.NoError(t, err)
		boolVal, ok := val.(bool)
		require.True(t, ok)
		assert.True(t, boolVal)
	})

	t.Run("array flag", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().StringSlice("array-flag", []string{"a", "b"}, "")

		flagMeta := FlagMetadata{Name: "array-flag", Type: "array"}
		val, err := loader.getFlagValue(cmd, flagMeta)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, val)
	})

	t.Run("unknown flag type", func(t *testing.T) {
		cmd := &cobra.Command{}
		flagMeta := FlagMetadata{Name: "unknown", Type: "unknown"}
		val, err := loader.getFlagValue(cmd, flagMeta)
		require.NoError(t, err)
		assert.Nil(t, val)
	})
}

// TestLoader_LoadCommands_Integration tests full integration with file store
func TestLoader_LoadCommands_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "commands.json")

	store := NewCommandStore(dbPath)

	data := &StoredData{
		Version: "1.0.0",
		BaseURL: "https://httpbin.org",
		Commands: []CommandMetadata{
			{
				Name:   "integration-test",
				Use:    "integration-test [id]",
				Short:  "Integration test",
				Long:   "Test command for integration testing",
				Method: "GET",
				Path:   "/test/{id}",
				Args:   []string{"id"},
				Flags: []FlagMetadata{
					{
						Name:        "verbose",
						Shorthand:   "v",
						Type:        "bool",
						Description: "Verbose output",
						Default:     "false",
					},
				},
			},
		},
		GeneratedAt: time.Now(),
	}

	err := store.Save(data)
	require.NoError(t, err)

	loader := NewLoader(store, "https://default.com")
	cmds, err := loader.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmd := cmds[0]
	assert.Equal(t, "integration-test [id]", cmd.Use)
	assert.Equal(t, "Integration test", cmd.Short)
}
