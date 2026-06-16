package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wojons/muster/pkg/openapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := Config{
			AppName:          "test",
			BaseURL:          "https://api.example.com",
			DefaultFormat:    "json",
			SupportedFormats: []string{"json", "yaml"},
		}
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("missing app name", func(t *testing.T) {
		cfg := Config{
			BaseURL: "https://api.example.com",
		}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "app name is required")
	})

	t.Run("missing base URL", func(t *testing.T) {
		cfg := Config{
			AppName: "test",
		}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "base URL is required")
	})

	t.Run("defaults set", func(t *testing.T) {
		cfg := Config{
			AppName: "test",
			BaseURL: "https://api.example.com",
		}
		err := cfg.Validate()
		require.NoError(t, err)
		require.Equal(t, "table", cfg.DefaultFormat)
		require.Equal(t, []string{"table", "json", "yaml"}, cfg.SupportedFormats)
	})
}

func TestCommandCache(t *testing.T) {
	cachePath := filepath.Join(os.TempDir(), "muster-test-cache.json")
	defer func() { _ = os.Remove(cachePath) }()

	t.Run("cache miss", func(t *testing.T) {
		cache := NewCommandCache(cachePath)
		metadata, ok := cache.Load("nonexistent.json")
		require.False(t, ok)
		require.Nil(t, metadata)
	})

	t.Run("cache hit", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSpec := filepath.Join(tmpDir, "test.json")
		_ = os.WriteFile(testSpec, []byte(`{"openapi":"3.0.0"}`), 0644)

		cache := NewCommandCache(cachePath)

		testCommands := []*cobra.Command{&cobra.Command{Use: "list-users"}}

		err := cache.Save(testSpec, testCommands)
		require.NoError(t, err)

		metadata, ok := cache.Load(testSpec)
		require.True(t, ok)
		require.NotNil(t, metadata)
	})

	t.Run("cache expiration", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSpec := filepath.Join(tmpDir, "test-expired.json")
		_ = os.WriteFile(testSpec, []byte(`{"openapi":"3.0.0"}`), 0644)

		cache := &CommandCache{
			path: cachePath,
			ttl:  0,
		}

		testCommands := []*cobra.Command{&cobra.Command{Use: "test"}}

		err := cache.Save(testSpec, testCommands)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		metadata, ok := cache.Load(testSpec)
		require.False(t, ok)
		require.Nil(t, metadata)
	})

	t.Run("cache hash mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSpec := filepath.Join(tmpDir, "test-hash.json")
		_ = os.WriteFile(testSpec, []byte(`{"openapi":"3.0.0"}`), 0644)

		cache := NewCommandCache(cachePath)
		testCommands := []*cobra.Command{&cobra.Command{Use: "test"}}
		_ = cache.Save(testSpec, testCommands)

		newContent := `{"openapi":"3.0.1"}`
		_ = os.WriteFile(testSpec, []byte(newContent), 0644)

		metadata, ok := cache.Load(testSpec)
		require.False(t, ok)
		require.Nil(t, metadata)
	})
}

func TestComputeHash(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")

	testContent := `{"test": "data"}`
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	hash1 := computeHash(testFile)
	require.NotEmpty(t, hash1)

	hash2 := computeHash(testFile)
	require.Equal(t, hash1, hash2)

	newContent := `{"test": "different"}`
	_ = os.WriteFile(testFile, []byte(newContent), 0644)

	hash3 := computeHash(testFile)
	require.NotEqual(t, hash1, hash3)
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "user"},
		{"cities", "city"},
		{"boxes", "box"},
		{"categories", "category"},
		{"data", "data"},
		{"", ""},
		{"test", "test"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := singularize(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected string
	}{
		{"UserName", "user-name"},
		{"userID", "user-i-d"},
		{"getUsers", "get-users"},
		{"API_KEY", "a-p-i_-k-e-y"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeOperationID(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected string
	}{
		{"getUserById", "get-user-by-id"},
		{"CreateUser", "create-user"},
		{"listAllUsers", "list-all-users"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeOperationID(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestHttpAction(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		method   string
		expected string
	}{
		{"GET", "get"},
		{"POST", "post"},
		{"PUT", "put"},
		{"PATCH", "patch"},
		{"DELETE", "delete"},
		{"HEAD", "head"},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			result := gen.httpAction(tc.method)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractResourceFromPath(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		path     string
		expected string
	}{
		{"/users", "users"},
		{"/api/v1/users", "api"},
		{"users", "users"},
		{"/", ""},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := gen.extractResourceFromPath(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestGroupOperations(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	ops := []*openapi.Operation{
		{
			ID:     "listUsers",
			Path:   "/users",
			Method: "GET",
			Tags:   []string{"users"},
		},
		{
			ID:     "createUser",
			Path:   "/users",
			Method: "POST",
			Tags:   []string{"users"},
		},
		{
			ID:     "listPosts",
			Path:   "/posts",
			Method: "GET",
			Tags:   []string{"posts"},
		},
	}

	groups := gen.groupOperations(ops)

	require.Len(t, groups, 2)
	require.Contains(t, groups["users"], ops[0])
	require.Contains(t, groups["users"], ops[1])
	require.Contains(t, groups["posts"], ops[2])
}

func TestIsExcluded(t *testing.T) {
	t.Run("exclude by tag", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:     "test",
			BaseURL:     "test",
			ExcludeTags: []string{"admin"},
		})

		op := &openapi.Operation{
			ID:   "adminCmd",
			Tags: []string{"admin"},
		}

		require.True(t, gen.isExcluded("admin", op.Tags))
	})

	t.Run("include only", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:         "test",
			BaseURL:         "test",
			IncludeOnlyTags: []string{"users"},
		})

		opUsers := &openapi.Operation{
			ID:   "listUsers",
			Tags: []string{"users"},
		}

		opPosts := &openapi.Operation{
			ID:   "listPosts",
			Tags: []string{"posts"},
		}

		require.False(t, gen.isExcluded("users", opUsers.Tags))
		require.True(t, gen.isExcluded("posts", opPosts.Tags))
	})
}

func TestBuildUsage(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("no path params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
		}
		result := gen.buildUsage("list", op)
		require.Equal(t, "list [flags]", result)
	})

	t.Run("with path params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
			Parameters: openapi.Parameters{
				openapi.Parameter{
					Name:     "id",
					In:       "path",
					Required: true,
				},
			},
		}
		result := gen.buildUsage("get", op)
		require.Equal(t, "get <id> [flags]", result)
	})
}

func TestExtractPathParams(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected []string
	}{
		{"/users", nil},
		{"/users/{id}", []string{"id"}},
		{"/users/{id}/posts/{postId}", []string{"id", "postId"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.extractPathParams(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestInferShorthand(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		name     string
		expected string
	}{
		{"namespace", "n"},
		{"output", "o"},
		{"format", "f"},
		{"config", "c"},
		{"verbose", "v"},
		{"unknown", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gen.inferShorthand(tc.name)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeFlagName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected string
	}{
		{"Namespace", "namespace"},
		{"outputFormat", "output-format"},
		{"APIKey", "a-p-i-key"},
		{"userId", "user-id"},
		{"", ""},
		{"already-kebab", "already-kebab"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeFlagName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNewGenerator(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		gen, err := NewGenerator(Config{
			AppName: "test",
			BaseURL: "https://api.example.com",
		})
		require.NoError(t, err)
		require.NotNil(t, gen)
	})

	t.Run("invalid config missing app name", func(t *testing.T) {
		gen, err := NewGenerator(Config{
			BaseURL: "https://api.example.com",
		})
		require.Error(t, err)
		require.Nil(t, gen)
		require.Contains(t, err.Error(), "invalid config")
	})

	t.Run("invalid config missing base URL", func(t *testing.T) {
		gen, err := NewGenerator(Config{
			AppName: "test",
		})
		require.Error(t, err)
		require.Nil(t, gen)
	})
}

func TestExtractResource(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("from tags", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "listUsers",
			Path: "/users",
			Tags: []string{"Users"},
		}
		result := gen.extractResource(op)
		require.Equal(t, "users", result)
	})

	t.Run("from tags with multi-word", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "listUserProfiles",
			Path: "/user-profiles",
			Tags: []string{"UserProfiles"},
		}
		result := gen.extractResource(op)
		require.Equal(t, "user-profiles", result)
	})

	t.Run("from path when no tags", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "listUsers",
			Path: "/users",
		}
		result := gen.extractResource(op)
		require.Equal(t, "users", result)
	})

	t.Run("from path with prefix when no tags", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "listItems",
			Path: "/api/v1/items",
			Tags: nil,
		}
		result := gen.extractResource(op)
		require.Equal(t, "api", result)
	})

	t.Run("empty path and no tags returns default", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "empty",
			Path: "",
		}
		result := gen.extractResource(op)
		require.Equal(t, "", result)
	})

	t.Run("tags take precedence over path", func(t *testing.T) {
		op := &openapi.Operation{
			ID:   "getItem",
			Path: "/api/v1/items",
			Tags: []string{"inventory"},
		}
		result := gen.extractResource(op)
		require.Equal(t, "inventory", result)
	})
}

func TestHasQueryParams(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("no parameters", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
		}
		require.False(t, gen.hasQueryParams(op))
	})

	t.Run("with query param", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Parameters: openapi.Parameters{
				{Name: "limit", In: "query", Type: "integer"},
			},
		}
		require.True(t, gen.hasQueryParams(op))
	})

	t.Run("with only path params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Type: "string", Required: true},
			},
		}
		require.False(t, gen.hasQueryParams(op))
	})

	t.Run("with header params only", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Parameters: openapi.Parameters{
				{Name: "X-Request-Id", In: "header", Type: "string"},
			},
		}
		require.False(t, gen.hasQueryParams(op))
	})

	t.Run("mixed params with query", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Type: "string", Required: true},
				{Name: "include", In: "query", Type: "string"},
			},
		}
		require.True(t, gen.hasQueryParams(op))
	})
}

func TestInferResourceName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"simple path", "/users", "users"},
		{"nested path", "/users/{id}", "users"},
		{"multi-level path", "/api/v1/posts", "api"},
		{"path param first", "/{id}/posts", "posts"},
		{"empty segments", "///users", "users"},
		{"no meaningful parts", "/", "resource"},
		{"camelCase path", "/UserProfiles", "userprofiles"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := &openapi.Operation{
				Path: tc.path,
			}
			result := gen.inferResourceName(op)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestGetCommandName(t *testing.T) {
	t.Run("custom mapping", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName: "test",
			BaseURL: "test",
			CustomMappings: map[string]string{
				"listUsers": "ls-users",
			},
		})
		op := &openapi.Operation{ID: "listUsers", Method: "GET", Path: "/users"}
		result := gen.getCommandName(op)
		require.Equal(t, "ls-users", result)
	})

	t.Run("from operation ID", func(t *testing.T) {
		gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})
		op := &openapi.Operation{ID: "listUsers", Method: "GET", Path: "/users"}
		result := gen.getCommandName(op)
		require.Equal(t, "list-users", result)
	})

	t.Run("fallback when no ID", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleHTTP,
		})
		op := &openapi.Operation{Method: "GET", Path: "/users"}
		result := gen.getCommandName(op)
		require.Equal(t, "get-users", result)
	})

	t.Run("custom mapping takes priority over ID", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName: "test",
			BaseURL: "test",
			CustomMappings: map[string]string{
				"getUserById": "fetch-user",
			},
		})
		op := &openapi.Operation{ID: "getUserById", Method: "GET", Path: "/users/{id}"}
		result := gen.getCommandName(op)
		require.Equal(t, "fetch-user", result)
	})

	t.Run("normalize operation ID with camelCase", func(t *testing.T) {
		gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})
		op := &openapi.Operation{ID: "createNewUser", Method: "POST", Path: "/users"}
		result := gen.getCommandName(op)
		require.Equal(t, "create-new-user", result)
	})
}

func TestOperationName(t *testing.T) {
	t.Run("StyleHTTP", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleHTTP,
		})
		result := gen.operationName("GET", "/users")
		require.Equal(t, "get-users", result)
	})

	t.Run("StyleHTTP with POST", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleHTTP,
		})
		result := gen.operationName("POST", "/items")
		require.Equal(t, "post-items", result)
	})

	t.Run("StyleFlat", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleFlat,
		})
		result := gen.operationName("GET", "/users")
		require.Equal(t, "get-users", result)
	})

	t.Run("StyleFlat with DELETE", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleFlat,
		})
		result := gen.operationName("DELETE", "/users/{id}")
		require.Equal(t, "delete-users", result)
	})

	t.Run("default StyleResource/StyleCRUD with GET list", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleResource,
		})
		result := gen.operationName("GET", "/users")
		require.Equal(t, "list-users", result)
	})

	t.Run("default StyleCRUD with GET by ID", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleCRUD,
		})
		result := gen.operationName("GET", "/users/{id}")
		require.Equal(t, "get-user", result)
	})
}

func TestCrudName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		{"GET list", "GET", "/users", "list-users"},
		{"GET by ID", "GET", "/users/{id}", "get-user"},
		{"GET by ID with UUID", "GET", "/items/{uuid}", "get-item"},
		{"POST create", "POST", "/users", "create-user"},
		{"POST create with plural", "POST", "/categories", "create-category"},
		{"PUT update", "PUT", "/users/{id}", "update-user"},
		{"PATCH update", "PATCH", "/users/{id}", "update-user"},
		{"DELETE", "DELETE", "/users/{id}", "delete-user"},
		{"DELETE collection", "DELETE", "/users", "delete-user"},
		{"HEAD fallback", "HEAD", "/users", "head"},
		{"OPTIONS fallback", "OPTIONS", "/users", "options"},
		{"GET list with complex path", "GET", "/api/v1/items", "list-api"},
		{"POST with -ies singular", "POST", "/cities", "create-city"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gen.crudName(tc.method, tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildLongDescription(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})

	t.Run("with description and params", func(t *testing.T) {
		op := &openapi.Operation{
			Method:      "GET",
			Path:        "/users/{id}",
			Summary:     "Get a user by ID",
			Description: "Retrieves a single user resource by its unique identifier.",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Required: true, Description: "User identifier"},
			},
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "Retrieves a single user resource by its unique identifier.")
		require.Contains(t, result, "API Endpoint:")
		require.Contains(t, result, "GET /users/{id}")
		require.Contains(t, result, "Parameters:")
		require.Contains(t, result, "id (required): User identifier")
	})

	t.Run("without description", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/health",
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "API Endpoint:")
		require.Contains(t, result, "GET /health")
	})

	t.Run("without params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/version",
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "API Endpoint:")
		require.NotContains(t, result, "Parameters:")
	})

	t.Run("with optional param", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Parameters: openapi.Parameters{
				{Name: "limit", In: "query", Required: false, Description: "Max results"},
			},
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "limit: Max results")
		require.NotContains(t, result, "(required)")
	})
}

func TestBuildExample(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "https://api.example.com"})

	t.Run("basic usage without path params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Summary: "List users",
		}
		result := gen.buildExample("list-users", op)
		require.Contains(t, result, "muster users list-users")
		require.Contains(t, result, "JSON output")
		require.Contains(t, result, "--output json")
	})

	t.Run("with required path param", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Required: true},
			},
		}
		result := gen.buildExample("get-user", op)
		require.Contains(t, result, "muster users get-user")
	})

	t.Run("with query params includes filter section", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Parameters: openapi.Parameters{
				{Name: "role", In: "query", Type: "string"},
			},
		}
		result := gen.buildExample("list-users", op)
		require.Contains(t, result, "With filters")
		require.Contains(t, result, "--namespace production")
	})

	t.Run("fallback resource name for root path", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/",
		}
		result := gen.buildExample("list", op)
		require.Contains(t, result, "muster resource list")
	})
}

func TestSingularizeEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "user"},
		{"cities", "city"},
		{"boxes", "box"},
		{"categories", "category"},
		{"data", "data"},
		{"", ""},
		{"test", "test"},
		{"class", "class"},
		{"address", "address"},
		{"buses", "bus"},
		{"queries", "query"},
		{"flies", "fly"},
		{"status", "status"},
		{"bus", "bus"},
		{"series", "series"},
		{"kiss", "kiss"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := singularize(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeNameAdditional(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"simple", "simple"},
		{"already-kebab", "already-kebab"},
		{"XMLParser", "x-m-l-parser"},
		{"HTTPServer", "h-t-t-p-server"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeOperationIDAdditional(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		input    string
		expected string
	}{
		{"getUserById", "get-user-by-id"},
		{"CreateUser", "create-user"},
		{"listAllUsers", "list-all-users"},
		{"", ""},
		{"simple", "simple"},
		{"already-kebab", "already-kebab"},
		{"parseXML", "parse-x-m-l"},
		{"handleHTTPError", "handle-h-t-t-p-error"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeOperationID(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractPathParamsEmpty(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		path     string
		expected []string
	}{
		{"", nil},
		{"/", nil},
		{"/users", nil},
		{"/users/{id}", []string{"id"}},
		{"/users/{id}/posts/{postId}", []string{"id", "postId"}},
		{"/{namespace}/{resource}", []string{"namespace", "resource"}},
		{"/prefix_{id}", nil},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := gen.extractPathParams(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildUsageEdgeCases(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("multiple path params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{userId}/posts/{postId}",
			Parameters: openapi.Parameters{
				{Name: "userId", In: "path", Required: true},
				{Name: "postId", In: "path", Required: true},
			},
		}
		result := gen.buildUsage("get-post", op)
		require.Equal(t, "get-post <userId> <postId> [flags]", result)
	})

	t.Run("optional path param", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users/{id}",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Required: false},
			},
		}
		result := gen.buildUsage("get-user", op)
		require.Equal(t, "get-user [<id>] [flags]", result)
	})
}

func TestHttpActionAdditional(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		method   string
		expected string
	}{
		{"GET", "get"},
		{"POST", "post"},
		{"PUT", "put"},
		{"PATCH", "patch"},
		{"DELETE", "delete"},
		{"HEAD", "head"},
		{"OPTIONS", "options"},
		{"get", "get"},
		{"TRACE", "trace"},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			result := gen.httpAction(tc.method)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestInferShorthandAdditional(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		name     string
		expected string
	}{
		{"namespace", "n"},
		{"output", "o"},
		{"format", "f"},
		{"selector", "l"},
		{"all", "a"},
		{"watch", "w"},
		{"follow", "f"},
		{"help", "h"},
		{"config", "c"},
		{"verbose", "v"},
		{"yes", "y"},
		{"timeout", "t"},
		{"unknown-param", ""},
		{"name", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gen.inferShorthand(tc.name)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestConfigValidateEdgeCases(t *testing.T) {
	t.Run("empty app name with base URL", func(t *testing.T) {
		cfg := Config{
			AppName: "",
			BaseURL: "https://api.example.com",
		}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "app name is required")
	})

	t.Run("default format not overwritten when set", func(t *testing.T) {
		cfg := Config{
			AppName:       "test",
			BaseURL:       "https://api.example.com",
			DefaultFormat: "yaml",
		}
		err := cfg.Validate()
		require.NoError(t, err)
		require.Equal(t, "yaml", cfg.DefaultFormat)
		require.Equal(t, []string{"table", "json", "yaml"}, cfg.SupportedFormats)
	})

	t.Run("custom supported formats preserved", func(t *testing.T) {
		cfg := Config{
			AppName:          "test",
			BaseURL:          "https://api.example.com",
			SupportedFormats: []string{"csv", "xml", "yaml"},
		}
		err := cfg.Validate()
		require.NoError(t, err)
		require.Equal(t, []string{"csv", "xml", "yaml"}, cfg.SupportedFormats)
	})

	t.Run("empty config fails", func(t *testing.T) {
		cfg := Config{}
		err := cfg.Validate()
		require.Error(t, err)
	})
}

func TestExtractResourceFromPathAdditional(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		path     string
		expected string
	}{
		{"/users", "users"},
		{"/api/v1/users", "api"},
		{"users", "users"},
		{"/", ""},
		{"/UserProfiles", "user-profiles"},
		{"/{id}", "{id}"},
		{"/filter/items", "filter"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := gen.extractResourceFromPath(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestGroupOperationsExclusion(t *testing.T) {
	t.Run("excluded operations filtered out", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:     "test",
			BaseURL:     "test",
			ExcludeTags: []string{"internal"},
		})

		ops := []*openapi.Operation{
			{ID: "listUsers", Path: "/users", Method: "GET", Tags: []string{"users"}},
			{ID: "internalOp", Path: "/internal", Method: "GET", Tags: []string{"internal"}},
			{ID: "createUser", Path: "/users", Method: "POST", Tags: []string{"users"}},
		}

		groups := gen.groupOperations(ops)
		require.Len(t, groups, 1)
		require.Contains(t, groups, "users")
		require.NotContains(t, groups, "internal")
	})

	t.Run("all excluded when none included", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:         "test",
			BaseURL:         "test",
			IncludeOnlyTags: []string{"users"},
		})

		ops := []*openapi.Operation{
			{ID: "listPosts", Path: "/posts", Method: "GET", Tags: []string{"posts"}},
			{ID: "listComments", Path: "/comments", Method: "GET", Tags: []string{"comments"}},
		}

		groups := gen.groupOperations(ops)
		require.Len(t, groups, 0)
	})
}

func TestIsExcludedEdgeCases(t *testing.T) {
	t.Run("not excluded when ExcludeTags empty and IncludeOnlyTags empty", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName: "test",
			BaseURL: "test",
		})
		op := &openapi.Operation{
			ID:   "listUsers",
			Tags: []string{"users"},
		}
		require.False(t, gen.isExcluded("users", op.Tags))
	})

	t.Run("excluded by resource name in ExcludeTags", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:     "test",
			BaseURL:     "test",
			ExcludeTags: []string{"admin"},
		})
		op := &openapi.Operation{
			ID:   "adminCmd",
			Tags: []string{"users"},
		}
		// resource == "admin" matches ExcludeTags
		require.True(t, gen.isExcluded("admin", op.Tags))
	})

	t.Run("excluded by tag match in ExcludeTags", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:     "test",
			BaseURL:     "test",
			ExcludeTags: []string{"internal"},
		})
		op := &openapi.Operation{
			ID:   "sysOp",
			Tags: []string{"internal"},
		}
		// resource == "internal" from tag, plus tag matches
		require.True(t, gen.isExcluded("internal", op.Tags))
	})

	t.Run("include only matching by tag not resource", func(t *testing.T) {
		gen, _ := NewGenerator(Config{
			AppName:         "test",
			BaseURL:         "test",
			IncludeOnlyTags: []string{"users"},
		})
		// Resource is "profiles" but tag is "users"
		op := &openapi.Operation{
			ID:   "listProfiles",
			Tags: []string{"users"},
		}
		require.False(t, gen.isExcluded("profiles", op.Tags))
	})
}

func TestNewCommandCache(t *testing.T) {
	t.Run("default ttl is 24 hours", func(t *testing.T) {
		cache := NewCommandCache("/tmp/test-cache.json")
		require.NotNil(t, cache)
		require.Equal(t, 24*time.Hour, cache.ttl)
	})
}

func TestCommandCacheExpiredLoad(t *testing.T) {
	cachePath := filepath.Join(os.TempDir(), "muster-test-expired-load.json")
	defer func() { _ = os.Remove(cachePath) }()

	tmpDir := t.TempDir()
	testSpec := filepath.Join(tmpDir, "test-expired-load.json")
	_ = os.WriteFile(testSpec, []byte(`{"key":"value"}`), 0644)

	// Create cache with zero TTL so it's already expired
	cache := &CommandCache{
		path: cachePath,
		ttl:  0,
	}
	_ = cache.Save(testSpec, nil)

	// Force expire by sleeping a tiny bit and checking
	time.Sleep(1 * time.Millisecond)

	// Load should miss because TTL=0 means already expired
	meta, ok := cache.Load(testSpec)
	require.False(t, ok)
	require.Nil(t, meta)
}

func TestCommandCachePersist(t *testing.T) {
	cachePath := filepath.Join(os.TempDir(), "muster-test-persist.json")
	defer func() { _ = os.Remove(cachePath) }()

	cache := NewCommandCache(cachePath)

	// Nil metadata should result in no-op
	err := cache.persist()
	require.NoError(t, err)

	// Verify no file was created
	_, err = os.Stat(cachePath)
	require.True(t, os.IsNotExist(err))
}
