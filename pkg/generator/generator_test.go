package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/openapi"
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
	defer os.Remove(cachePath)

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
		cache.Save(testSpec, testCommands)

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
				&openapi.Parameter{
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
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeFlagName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}
