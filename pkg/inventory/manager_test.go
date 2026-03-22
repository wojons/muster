package inventory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger is a test logger that captures log messages
type mockLogger struct {
	infoMsgs  []string
	warnMsgs  []string
	errorMsgs []string
}

func (m *mockLogger) Info(msg string, args ...interface{}) {
	m.infoMsgs = append(m.infoMsgs, formatLogMessage(msg, args...))
}

func (m *mockLogger) Warn(msg string, args ...interface{}) {
	m.warnMsgs = append(m.warnMsgs, formatLogMessage(msg, args...))
}

func (m *mockLogger) Error(msg string, args ...interface{}) {
	m.errorMsgs = append(m.errorMsgs, formatLogMessage(msg, args...))
}

// formatLogMessage formats a log message with key-value pairs
func formatLogMessage(msg string, args ...interface{}) string {
	if len(args) == 0 {
		return msg
	}
	// Handle key-value pairs
	var pairs []string
	for i := 0; i < len(args)-1; i += 2 {
		key := fmt.Sprintf("%v", args[i])
		value := fmt.Sprintf("%v", args[i+1])
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	return fmt.Sprintf("%s %s", msg, pairs)
}

// createTestInventory returns a valid inventory for testing
func createTestInventory() *Inventory {
	return &Inventory{
		SchemaVersion:  "1.0",
		GeneratedAt:    time.Now(),
		APIVersion:     "1.0.0",
		APIName:        "Test API",
		APIDescription: "Test API Description",
		BaseURL:        "https://api.example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:         "https://api.example.com/openapi.json",
			Format:      "json",
			SHA256:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:        1024,
			LastUpdated: time.Now(),
		},
		Resources: []Resource{
			{
				Name:        "users",
				Description: "User management",
				Operations: []Operation{
					{
						Name:    "list",
						Method:  "GET",
						Path:    "/users",
						Summary: "List all users",
					},
				},
			},
		},
	}
}

// createTestInventoryJSON returns a valid inventory as JSON bytes
func createTestInventoryJSON() []byte {
	return []byte(`{
		"schema_version": "1.0",
		"generated_at": "2024-01-01T00:00:00Z",
		"api_version": "1.0.0",
		"api_name": "Test API",
		"api_description": "Test API Description",
		"base_url": "https://api.example.com",
		"openapi_spec": {
			"url": "https://api.example.com/openapi.json",
			"format": "json",
			"sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"size": 1024,
			"last_updated": "2024-01-01T00:00:00Z"
		},
		"resources": [
			{
				"name": "users",
				"description": "User management",
				"operations": [
					{
						"name": "list",
						"method": "GET",
						"path": "/users",
						"summary": "List all users"
					}
				]
			}
		]
	}`)
}

func TestNewManager(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		tempDir := t.TempDir()
		config := ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		}

		manager, err := NewManager(config)
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.fetcher)
		assert.NotNil(t, manager.validator)
		assert.NotNil(t, manager.cache)
		assert.NotNil(t, manager.versioner)
		assert.NotNil(t, manager.logger)
	})

	t.Run("DefaultCacheDir", func(t *testing.T) {
		config := ManagerConfig{
			Logger: &mockLogger{},
		}

		manager, err := NewManager(config)
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.cache)
	})

	t.Run("NilLogger", func(t *testing.T) {
		tempDir := t.TempDir()
		config := ManagerConfig{
			CacheDir: tempDir,
			Logger:   nil,
		}

		manager, err := NewManager(config)
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.logger)
	})
}

func TestManager_Fetch(t *testing.T) {
	t.Run("FetchFromNetworkSuccess", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/_oa/inventory.json", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", "test-etag-123")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(createTestInventoryJSON())
		}))
		defer server.Close()

		tempDir := t.TempDir()
		logger := &mockLogger{}
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   logger,
		})
		require.NoError(t, err)

		ctx := context.Background()
		// Use full URL with http:// prefix so buildInventoryURL doesn't add https://
		inv, err := manager.Fetch(ctx, server.URL, FetchOptions{})
		require.NoError(t, err)
		assert.NotNil(t, inv)
		assert.Equal(t, "1.0.0", inv.APIVersion)
		assert.Equal(t, "Test API", inv.APIName)

		// Verify logging
		assert.Contains(t, logger.infoMsgs, fmt.Sprintf("Fetching inventory [host=%s]", server.URL))
	})

	t.Run("FetchFromCache", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := &mockLogger{}
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   logger,
		})
		require.NoError(t, err)

		// Pre-populate cache
		inv := createTestInventory()
		err = manager.cache.Store("test-host", inv, "test-etag")
		require.NoError(t, err)

		ctx := context.Background()
		result, err := manager.Fetch(ctx, "test-host", FetchOptions{})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, inv.APIVersion, result.APIVersion)

		// Verify cache hit was logged
		assert.Contains(t, logger.infoMsgs, "Fetching inventory [host=test-host]")
	})

	t.Run("ForceRefreshIgnoresCache", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(createTestInventoryJSON())
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Pre-populate cache with different data
		inv := createTestInventory()
		inv.APIVersion = "0.9.0"
		err = manager.cache.Store("test-host", inv, "old-etag")
		require.NoError(t, err)

		// Fetch with ForceRefresh should get from network
		ctx := context.Background()
		result, err := manager.Fetch(ctx, server.URL, FetchOptions{ForceRefresh: true})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "1.0.0", result.APIVersion) // Should be from server, not cache
	})

	t.Run("NetworkErrorFallsBackToStaleCache", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := &mockLogger{}
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   logger,
		})
		require.NoError(t, err)

		// Pre-populate cache with expired data
		inv := createTestInventory()
		inv.GeneratedAt = time.Now().Add(-2 * time.Hour) // Expired
		hostKey := "localhost:59999"
		err = manager.cache.Store(hostKey, inv, "old-etag")
		require.NoError(t, err)

		// Try to fetch from non-existent server (will fail)
		ctx := context.Background()
		result, err := manager.Fetch(ctx, hostKey, FetchOptions{})
		require.NoError(t, err) // Should not error, returns stale cache
		assert.NotNil(t, result)
		assert.Equal(t, inv.APIVersion, result.APIVersion)

		// Verify warning was logged
		assert.True(t, len(logger.warnMsgs) > 0)
	})

	t.Run("NetworkErrorNoCache", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := &mockLogger{}
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   logger,
		})
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Fetch(ctx, "localhost:59999", FetchOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fetch failed")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Fetch(ctx, server.URL, FetchOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parse failed")
	})

	t.Run("ValidationError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"schema_version": "2.0",
				"generated_at": "2024-01-01T00:00:00Z",
				"api_version": "1.0.0",
				"api_name": "Test API",
				"api_description": "Test API",
				"base_url": "https://api.example.com",
				"openapi_spec": {
					"url": "https://api.example.com/openapi.json",
					"format": "json",
					"sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					"size": 1024,
					"last_updated": "2024-01-01T00:00:00Z"
				},
				"resources": []
			}`))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Fetch(ctx, server.URL, FetchOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(createTestInventoryJSON())
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err = manager.Fetch(ctx, server.URL, FetchOptions{})
		assert.Error(t, err)
	})
}

func TestManager_FetchOpenAPISpec(t *testing.T) {
	t.Run("FetchSpecSuccess", func(t *testing.T) {
		openAPISpec := `{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {}}`
		expectedHash := calculateSHA256([]byte(openAPISpec))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/openapi.json" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(openAPISpec))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec.URL = server.URL + "/openapi.json"
		inv.OpenAPISpec.SHA256 = expectedHash

		ctx := context.Background()
		doc, err := manager.FetchOpenAPISpec(ctx, inv)
		require.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, "3.0.0", doc.OpenAPI)
		assert.Equal(t, "Test", doc.Info.Title)
	})

	t.Run("FetchSpecFromCache", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Pre-populate spec cache
		doc := &OpenAPIDocument{
			OpenAPI: "3.0.0",
			Info: struct {
				Title   string `json:"title"`
				Version string `json:"version"`
			}{
				Title:   "Cached API",
				Version: "1.0.0",
			},
		}
		err = manager.cache.StoreSpec("cached-sha256", doc)
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec.SHA256 = "cached-sha256"

		ctx := context.Background()
		result, err := manager.FetchOpenAPISpec(ctx, inv)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Cached API", result.Info.Title)
	})

	t.Run("NoOpenAPISpecReference", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec = nil

		ctx := context.Background()
		_, err = manager.FetchOpenAPISpec(ctx, inv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no OpenAPI spec reference")
	})

	t.Run("SHA256Mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}}`))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec.URL = server.URL + "/openapi.json"
		inv.OpenAPISpec.SHA256 = "wrong-hash-value-64-characters-long-0123456789abcdef"

		ctx := context.Background()
		_, err = manager.FetchOpenAPISpec(ctx, inv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SHA256 mismatch")
	})

	t.Run("FetchSpecNetworkError", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec.URL = "http://localhost:59999/spec.json"

		ctx := context.Background()
		_, err = manager.FetchOpenAPISpec(ctx, inv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch OpenAPI spec")
	})

	t.Run("InvalidSpecJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		inv := createTestInventory()
		inv.OpenAPISpec.URL = server.URL + "/spec.json"
		inv.OpenAPISpec.SHA256 = calculateSHA256([]byte("invalid json"))

		ctx := context.Background()
		_, err = manager.FetchOpenAPISpec(ctx, inv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse OpenAPI spec")
	})
}

func TestManager_ListCached(t *testing.T) {
	t.Run("EmptyCache", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		cached, err := manager.ListCached()
		require.NoError(t, err)
		assert.Empty(t, cached)
	})

	t.Run("WithCachedInventories", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Store multiple inventories
		inv1 := createTestInventory()
		err = manager.cache.Store("host1.example.com", inv1, "etag1")
		require.NoError(t, err)

		inv2 := createTestInventory()
		inv2.APIVersion = "2.0.0"
		err = manager.cache.Store("host2.example.com", inv2, "etag2")
		require.NoError(t, err)

		cached, err := manager.ListCached()
		require.NoError(t, err)
		assert.Len(t, cached, 2)
	})
}

func TestManager_PurgeCache(t *testing.T) {
	t.Run("PurgeExistingHost", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Store an inventory
		inv := createTestInventory()
		err = manager.cache.Store("test-host", inv, "etag")
		require.NoError(t, err)

		// Verify it's cached
		cached, err := manager.ListCached()
		require.NoError(t, err)
		assert.Len(t, cached, 1)

		// Purge it
		err = manager.PurgeCache("test-host")
		require.NoError(t, err)

		// Verify it's gone
		cached, err = manager.ListCached()
		require.NoError(t, err)
		assert.Empty(t, cached)
	})

	t.Run("PurgeNonExistentHost", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Purging non-existent host should not error
		err = manager.PurgeCache("non-existent-host")
		require.NoError(t, err)
	})
}

func TestManager_GetVersionHistory(t *testing.T) {
	t.Run("NoHistory", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		_, err = manager.GetVersionHistory("test-host")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no history found")
	})

	t.Run("WithHistory", func(t *testing.T) {
		tempDir := t.TempDir()
		manager, err := NewManager(ManagerConfig{
			CacheDir: tempDir,
			Logger:   &mockLogger{},
		})
		require.NoError(t, err)

		// Track some versions
		inv1 := createTestInventory()
		manager.versioner.Track("test-host", inv1)

		inv2 := createTestInventory()
		inv2.APIVersion = "2.0.0"
		manager.versioner.Track("test-host", inv2)

		history, err := manager.GetVersionHistory("test-host")
		require.NoError(t, err)
		assert.Len(t, history, 2)
	})
}

func TestManager_buildInventoryURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "WithHTTPSPrefix",
			host:     "https://api.example.com",
			expected: "https://api.example.com/_oa/inventory.json",
		},
		{
			name:     "WithHTTPPrefix",
			host:     "http://api.example.com",
			expected: "http://api.example.com/_oa/inventory.json",
		},
		{
			name:     "WithoutPrefix",
			host:     "api.example.com",
			expected: "https://api.example.com/_oa/inventory.json",
		},
		{
			name:     "WithTrailingSlash",
			host:     "https://api.example.com/",
			expected: "https://api.example.com/_oa/inventory.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			manager, err := NewManager(ManagerConfig{
				CacheDir: tempDir,
				Logger:   &mockLogger{},
			})
			require.NoError(t, err)

			result := manager.buildInventoryURL(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_parseInventory(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(t *testing.T, inv *Inventory)
	}{
		{
			name:    "ValidInventory",
			json:    string(createTestInventoryJSON()),
			wantErr: false,
			check: func(t *testing.T, inv *Inventory) {
				assert.Equal(t, "1.0.0", inv.APIVersion)
				assert.Equal(t, "Test API", inv.APIName)
				assert.Len(t, inv.Resources, 1)
			},
		},
		{
			name:    "InvalidJSON",
			json:    "not json at all",
			wantErr: true,
		},
		{
			name:    "PartialInventory",
			json:    `{"schema_version": "1.0", "api_version": "1.0.0"}`,
			wantErr: false,
			check: func(t *testing.T, inv *Inventory) {
				assert.Equal(t, "1.0.0", inv.APIVersion)
				assert.Empty(t, inv.APIName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			manager, err := NewManager(ManagerConfig{
				CacheDir: tempDir,
				Logger:   &mockLogger{},
			})
			require.NoError(t, err)

			inv, err := manager.parseInventory([]byte(tt.json))
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, inv)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, inv)
				if tt.check != nil {
					tt.check(t, inv)
				}
			}
		})
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		CacheDir: tempDir,
		Logger:   &mockLogger{},
	})
	require.NoError(t, err)

	// Store initial data
	inv := createTestInventory()
	err = manager.cache.Store("test-host", inv, "etag")
	require.NoError(t, err)

	// Run concurrent operations
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 10; i++ {
			_, _ = manager.ListCached()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			inv := createTestInventory()
			inv.APIVersion = fmt.Sprintf("1.0.%d", i)
			_ = manager.cache.Store(fmt.Sprintf("host%d", i), inv, "etag")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			manager.versioner.Track("test-host", inv)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestManager_FullLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_oa/inventory.json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", "lifecycle-etag")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(createTestInventoryJSON())
		case "/openapi.json":
			spec := `{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {}}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(spec))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	logger := &mockLogger{}
	manager, err := NewManager(ManagerConfig{
		CacheDir: tempDir,
		Logger:   logger,
	})
	require.NoError(t, err)

	ctx := context.Background()
	host := server.URL // Keep http:// prefix so buildInventoryURL doesn't add https://

	// Step 1: Fetch inventory
	inv, err := manager.Fetch(ctx, host, FetchOptions{})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", inv.APIVersion)

	// Step 2: Verify it was cached
	cached, err := manager.ListCached()
	require.NoError(t, err)
	assert.Len(t, cached, 1)

	// Update OpenAPI spec URL to point to test server
	specData := `{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {}}`
	inv.OpenAPISpec.URL = server.URL + "/openapi.json"
	inv.OpenAPISpec.SHA256 = calculateSHA256([]byte(specData))

	// Step 3: Fetch OpenAPI spec
	doc, err := manager.FetchOpenAPISpec(ctx, inv)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", doc.OpenAPI)

	// Step 4: Check version history
	history, err := manager.GetVersionHistory(host)
	require.NoError(t, err)
	assert.Len(t, history, 1)

	// Step 5: Fetch from cache (no network call)
	inv2, err := manager.Fetch(ctx, host, FetchOptions{})
	require.NoError(t, err)
	assert.Equal(t, inv.APIVersion, inv2.APIVersion)

	// Step 6: Force refresh
	inv3, err := manager.Fetch(ctx, host, FetchOptions{ForceRefresh: true})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", inv3.APIVersion)

	// Step 7: Purge cache
	err = manager.PurgeCache(host)
	require.NoError(t, err)

	// Step 8: Verify cache is empty
	cached, err = manager.ListCached()
	require.NoError(t, err)
	assert.Empty(t, cached)
}
