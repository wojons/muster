package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/request"
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
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := gen.normalizeFlagName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// NEW TESTS — covering previously 0% functions
// =============================================================================

// petstoreSpecJSON returns a minimal but realistic OpenAPI 3.0 petstore spec.
func petstoreSpecJSON() string {
	return `{"openapi":"3.0.0","info":{"title":"Pet Store","version":"1.0.0"},"paths":{"/pets":{"get":{"operationId":"listPets","summary":"List all pets","parameters":[{"name":"limit","in":"query","schema":{"type":"integer"}}],"responses":{"200":{"description":"OK"}}},"post":{"operationId":"createPet","summary":"Create a pet","requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"name":{"type":"string"}}}}}},"responses":{"201":{"description":"Created"}}}},"/pets/{petId}":{"get":{"operationId":"getPetById","summary":"Get pet by ID","parameters":[{"name":"petId","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}}}}`
}

// loadPetstoreSpec loads the embedded petstore JSON into an *openapi3.T.
func loadPetstoreSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(petstoreSpecJSON()))
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

// ---------------------------------------------------------------------------
// PHASE 1: NewGenerator
// ---------------------------------------------------------------------------

func TestNewGenerator_Valid(t *testing.T) {
	cfg := Config{
		AppName: "testapp",
		BaseURL: "https://api.example.com",
	}
	gen, err := NewGenerator(cfg)
	require.NoError(t, err)
	require.NotNil(t, gen)
}

func TestNewGenerator_MissingAppName(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.example.com",
	}
	gen, err := NewGenerator(cfg)
	require.Error(t, err)
	require.Nil(t, gen)
	require.Contains(t, err.Error(), "invalid config")
	require.Contains(t, err.Error(), "app name is required")
}

func TestNewGenerator_MissingBaseURL(t *testing.T) {
	cfg := Config{
		AppName: "testapp",
	}
	gen, err := NewGenerator(cfg)
	require.Error(t, err)
	require.Nil(t, gen)
	require.Contains(t, err.Error(), "invalid config")
	require.Contains(t, err.Error(), "base URL is required")
}

// ---------------------------------------------------------------------------
// PHASE 2: GenerateCommands (main pipeline)
// ---------------------------------------------------------------------------

func TestGenerateCommands_BasicPipeline(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	doc := loadPetstoreSpec(t)
	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	// One resource group: "pets" (all paths start with /pets)
	require.Len(t, commands, 1)

	petsCmd := commands[0]
	require.Equal(t, "pets", petsCmd.Use)
	require.Equal(t, "Manage pets", petsCmd.Short)
	require.Contains(t, petsCmd.Long, "managing pets")

	// Three subcommands: list-pets, create-pet, get-pet-by-id
	require.Len(t, petsCmd.Commands(), 3)

	// Index subcommands by their Use string
	subByName := make(map[string]*cobra.Command)
	for _, sub := range petsCmd.Commands() {
		// Extract the command name (first word of Use)
		name := sub.Use
		if idx := strings.IndexByte(sub.Use, ' '); idx > 0 {
			name = sub.Use[:idx]
		}
		subByName[name] = sub
	}

	// list-pets (GET /pets, no path params)
	listCmd := subByName["list-pets"]
	require.NotNil(t, listCmd, "should have list-pets subcommand")
	require.Equal(t, "list-pets [flags]", listCmd.Use)
	require.Equal(t, "List all pets", listCmd.Short)
	require.Contains(t, listCmd.Long, "GET /pets")
	require.Contains(t, listCmd.Long, "API Endpoint")
	// Has query param "limit" → should appear in Long
	require.Contains(t, listCmd.Long, "limit")
	// Example should be non-empty
	require.NotEmpty(t, listCmd.Example)
	require.Contains(t, listCmd.Example, "muster pets list-pets")

	// create-pet (POST /pets, no path params)
	createCmd := subByName["create-pet"]
	require.NotNil(t, createCmd, "should have create-pet subcommand")
	require.Equal(t, "create-pet [flags]", createCmd.Use)
	require.Equal(t, "Create a pet", createCmd.Short)
	require.Contains(t, createCmd.Long, "POST /pets")
	require.NotEmpty(t, createCmd.Example)

	// get-pet-by-id (GET /pets/{petId}, has path param)
	getCmd := subByName["get-pet-by-id"]
	require.NotNil(t, getCmd, "should have get-pet-by-id subcommand")
	require.Equal(t, "get-pet-by-id <petId> [flags]", getCmd.Use)
	require.Equal(t, "Get pet by ID", getCmd.Short)
	require.Contains(t, getCmd.Long, "GET /pets/{petId}")
	require.NotEmpty(t, getCmd.Example)

	// Every subcommand should have standard flags added by addFlags
	for _, sub := range petsCmd.Commands() {
		require.NotNil(t, sub.Flag("output"), "should have --output flag")
		require.NotNil(t, sub.Flag("namespace"), "should have --namespace flag")
	}
}

func TestGenerateCommands_NamingStrategies(t *testing.T) {
	strategies := []struct {
		name     string
		strategy NamingStrategy
	}{
		{"kebab", NamingKebab},
		{"camel", NamingCamel},
		{"snake", NamingSnake},
		{"flat", NamingFlat},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			gen, err := NewGenerator(Config{
				AppName:        "muster",
				BaseURL:        "https://api.example.com",
				NamingStrategy: s.strategy,
			})
			require.NoError(t, err)

			doc := loadPetstoreSpec(t)
			commands, err := gen.GenerateCommands(doc)
			require.NoError(t, err)
			require.Len(t, commands, 1)

			petsCmd := commands[0]
			require.Len(t, petsCmd.Commands(), 3)

			// With operationIds present, getCommandName uses normalizeOperationID
			// which always produces kebab-case regardless of NamingStrategy.
			// NamingStrategy only affects operationName() which is called when
			// no operationId exists.
			expectedUses := []string{
				"list-pets [flags]",
				"create-pet [flags]",
				"get-pet-by-id <petId> [flags]",
			}
			for _, sub := range petsCmd.Commands() {
				require.Contains(t, expectedUses, sub.Use,
					"subcommand Use should be one of the expected kebab-case names")
			}
		})
	}
}

func TestGenerateCommands_NilSpec(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	// nil spec: ExtractOperations returns (nil, nil) → empty commands, no error
	commands, err := gen.GenerateCommands(nil)
	require.NoError(t, err)
	require.Empty(t, commands)
}

func TestGenerateCommands_EmptySpec(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	// Spec with no paths
	emptySpecJSON := `{"openapi":"3.0.0","info":{"title":"Empty","version":"1.0"}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(emptySpecJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	require.Empty(t, commands)
}

func TestGenerateCommands_NoOperationId(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	// Spec with no operationIds — falls back to operationName → crudName
	noIDSpecJSON := `{"openapi":"3.0.0","info":{"title":"Test API","version":"1.0"},"paths":{"/users":{"get":{"summary":"List users","responses":{"200":{"description":"OK"}}},"post":{"summary":"Create user","responses":{"201":{"description":"Created"}}}}}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(noIDSpecJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	require.Len(t, commands, 1)

	usersCmd := commands[0]
	require.Equal(t, "users", usersCmd.Use)
	require.Len(t, usersCmd.Commands(), 2)

	// Without operationId, crudName produces: list-users, create-user
	expectedUses := []string{
		"list-users [flags]",
		"create-user [flags]",
	}
	for _, sub := range usersCmd.Commands() {
		require.Contains(t, expectedUses, sub.Use)
	}
}

func TestGenerateCommands_ExcludeTags(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName:     "muster",
		BaseURL:     "https://api.example.com",
		ExcludeTags: []string{"admin"},
	})
	require.NoError(t, err)

	taggedSpecJSON := `{"openapi":"3.0.0","info":{"title":"Test","version":"1.0"},"paths":{"/users":{"get":{"operationId":"listUsers","tags":["users"],"responses":{"200":{"description":"OK"}}}},"/admin":{"get":{"operationId":"adminStats","tags":["admin"],"responses":{"200":{"description":"OK"}}}}}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(taggedSpecJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	// "admin" tag is excluded → only "users" resource group appears
	require.Len(t, commands, 1)
	require.Equal(t, "users", commands[0].Use)
	require.Len(t, commands[0].Commands(), 1)
}

func TestGenerateCommands_IncludeOnlyTags(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName:         "muster",
		BaseURL:         "https://api.example.com",
		IncludeOnlyTags: []string{"users"},
	})
	require.NoError(t, err)

	taggedSpecJSON := `{"openapi":"3.0.0","info":{"title":"Test","version":"1.0"},"paths":{"/users":{"get":{"operationId":"listUsers","tags":["users"],"responses":{"200":{"description":"OK"}}}},"/posts":{"get":{"operationId":"listPosts","tags":["posts"],"responses":{"200":{"description":"OK"}}}}}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(taggedSpecJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	// Only "users" tag is included → "posts" is excluded
	require.Len(t, commands, 1)
	require.Equal(t, "users", commands[0].Use)
	require.Len(t, commands[0].Commands(), 1)
}

// ---------------------------------------------------------------------------
// PHASE 3: buildResourceCommand / buildOperationCommand (direct tests)
// ---------------------------------------------------------------------------

func TestBuildResourceCommand_Structure(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	ops := []*openapi.Operation{
		{ID: "listPets", Path: "/pets", Method: "GET", Summary: "List pets"},
		{ID: "createPet", Path: "/pets", Method: "POST", Summary: "Create pet"},
	}

	cmd := gen.buildResourceCommand("pets", ops)
	require.Equal(t, "pets", cmd.Use)
	require.Equal(t, "Manage pets", cmd.Short)
	require.Contains(t, cmd.Long, "managing pets")
	require.Len(t, cmd.Commands(), 2)

	// Subcommands should have correct names
	subNames := make([]string, len(cmd.Commands()))
	for i, sub := range cmd.Commands() {
		subNames[i] = sub.Use
	}
	require.Contains(t, subNames, "list-pets [flags]")
	require.Contains(t, subNames, "create-pet [flags]")
}

// ---------------------------------------------------------------------------
// PHASE 4: buildLongDescription / buildExample
// ---------------------------------------------------------------------------

func TestBuildLongDescription(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	t.Run("with description and params", func(t *testing.T) {
		op := &openapi.Operation{
			Method:      "GET",
			Path:        "/pets",
			Description: "Returns all pets from the store.",
			Parameters: openapi.Parameters{
				{Name: "limit", In: "query", Description: "Maximum number of results"},
			},
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "Returns all pets from the store.")
		require.Contains(t, result, "API Endpoint:")
		require.Contains(t, result, "GET /pets")
		require.Contains(t, result, "Parameters:")
		require.Contains(t, result, "limit: Maximum number of results")
	})

	t.Run("summary only, no description", func(t *testing.T) {
		op := &openapi.Operation{
			Method:  "POST",
			Path:    "/pets",
			Summary: "Create a pet",
		}
		result := gen.buildLongDescription(op)
		// Summary is NOT included in Long description (only Short)
		require.NotContains(t, result, "Create a pet")
		require.Contains(t, result, "API Endpoint:")
		require.Contains(t, result, "POST /pets")
		// No parameters section when there are no params
		require.NotContains(t, result, "Parameters:")
	})

	t.Run("with required path param", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/pets/{petId}",
			Parameters: openapi.Parameters{
				{Name: "petId", In: "path", Required: true, Description: "The pet ID"},
			},
		}
		result := gen.buildLongDescription(op)
		require.Contains(t, result, "GET /pets/{petId}")
		require.Contains(t, result, "petId (required): The pet ID")
	})
}

func TestBuildExample(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	t.Run("with query params", func(t *testing.T) {
		op := &openapi.Operation{
			Path:   "/pets",
			Method: "GET",
			Parameters: openapi.Parameters{
				{Name: "limit", In: "query"},
			},
		}
		result := gen.buildExample("list-pets", op)
		require.Contains(t, result, "# Basic usage")
		require.Contains(t, result, "muster pets list-pets")
		require.Contains(t, result, "# With filters")
		require.Contains(t, result, "--namespace production")
		require.Contains(t, result, "# JSON output")
		require.Contains(t, result, "--output json")
	})

	t.Run("without query params", func(t *testing.T) {
		op := &openapi.Operation{
			Path:   "/pets/{id}",
			Method: "GET",
			Parameters: openapi.Parameters{
				{Name: "id", In: "path", Required: true},
			},
		}
		result := gen.buildExample("get-pet-by-id", op)
		require.Contains(t, result, "# Basic usage")
		require.Contains(t, result, "muster pets get-pet-by-id <id>")
		require.Contains(t, result, "# JSON output")
		// No "With filters" section when there are no query params
		require.NotContains(t, result, "With filters")
	})

	t.Run("no parameters at all", func(t *testing.T) {
		op := &openapi.Operation{
			Path:   "/pets",
			Method: "POST",
		}
		result := gen.buildExample("create-pet", op)
		require.Contains(t, result, "# Basic usage")
		require.Contains(t, result, "muster pets create-pet")
		require.Contains(t, result, "# JSON output")
		require.NotContains(t, result, "With filters")
	})
}

// ---------------------------------------------------------------------------
// PHASE 5: Naming helpers
// ---------------------------------------------------------------------------

func TestInferResourceName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		path     string
		expected string
	}{
		{"/pets", "pets"},
		{"/api/v1/users", "api"},
		{"/users/{id}", "users"},
		{"/{id}", "resource"}, // no non-param path segment
		{"/", "resource"},
		{"", "resource"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			op := &openapi.Operation{Path: tc.path}
			result := gen.inferResourceName(op)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestHasQueryParams(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("with query params", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: openapi.Parameters{
				{Name: "limit", In: "query"},
				{Name: "offset", In: "query"},
			},
		}
		require.True(t, gen.hasQueryParams(op))
	})

	t.Run("only path params", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: openapi.Parameters{
				{Name: "id", In: "path"},
			},
		}
		require.False(t, gen.hasQueryParams(op))
	})

	t.Run("no parameters", func(t *testing.T) {
		op := &openapi.Operation{}
		require.False(t, gen.hasQueryParams(op))
	})

	t.Run("mixed params includes query", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: openapi.Parameters{
				{Name: "id", In: "path"},
				{Name: "verbose", In: "query"},
			},
		}
		require.True(t, gen.hasQueryParams(op))
	})
}

// ---------------------------------------------------------------------------
// PHASE 7: Config constants
// ---------------------------------------------------------------------------

func TestNamingStrategyConstants(t *testing.T) {
	require.Equal(t, NamingStrategy(0), NamingKebab)
	require.Equal(t, NamingStrategy(1), NamingCamel)
	require.Equal(t, NamingStrategy(2), NamingSnake)
	require.Equal(t, NamingStrategy(3), NamingFlat)
}

func TestOperationStyleConstants(t *testing.T) {
	require.Equal(t, OperationStyle(0), StyleResource)
	require.Equal(t, OperationStyle(1), StyleHTTP)
	require.Equal(t, OperationStyle(2), StyleCRUD)
	require.Equal(t, OperationStyle(3), StyleFlat)
}

// ---------------------------------------------------------------------------
// Additional edge-case / integration tests
// ---------------------------------------------------------------------------

func TestGenerateCommands_OperationWithNoSummary(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	specJSON := `{"openapi":"3.0.0","info":{"title":"Test","version":"1.0"},"paths":{"/items":{"delete":{"operationId":"deleteItem","responses":{"204":{"description":"No Content"}}}}}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	require.Len(t, commands, 1)

	itemsCmd := commands[0]
	require.Len(t, itemsCmd.Commands(), 1)

	sub := itemsCmd.Commands()[0]
	require.Equal(t, "delete-item [flags]", sub.Use)
	// Summary is empty in spec → Short should be empty
	require.Empty(t, sub.Short)
	// Long should still have the API endpoint info
	require.Contains(t, sub.Long, "DELETE /items")
}

func TestGenerateCommands_MultipleResources(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	// Two distinct resources: /pets and /stores
	specJSON := `{"openapi":"3.0.0","info":{"title":"Multi","version":"1.0"},"paths":{"/pets":{"get":{"operationId":"listPets","tags":["pets"],"responses":{"200":{"description":"OK"}}}},"/stores":{"get":{"operationId":"listStores","tags":["stores"],"responses":{"200":{"description":"OK"}}}}}}`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specJSON))
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	require.Len(t, commands, 2)

	// Find both resource commands
	names := make([]string, len(commands))
	for i, cmd := range commands {
		names[i] = cmd.Use
	}
	require.Contains(t, names, "pets")
	require.Contains(t, names, "stores")
}

func TestGenerateCommands_CommandPrefix(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName:       "muster",
		BaseURL:       "https://api.example.com",
		CommandPrefix: "api",
	})
	require.NoError(t, err)

	doc := loadPetstoreSpec(t)
	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	require.Len(t, commands, 1)

	// CommandPrefix is stored in config but not currently prepended to
	// resource command names by buildResourceCommand. The prefix is
	// available for external wiring.
	petsCmd := commands[0]
	require.Equal(t, "pets", petsCmd.Use)
}

func TestGenerateCommands_RequestBodyFlags(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	doc := loadPetstoreSpec(t)
	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	petsCmd := commands[0]
	// createPet has a requestBody with property "name" (string)
	var createCmd *cobra.Command
	for _, sub := range petsCmd.Commands() {
		if strings.HasPrefix(sub.Use, "create-pet") {
			createCmd = sub
			break
		}
	}
	require.NotNil(t, createCmd, "should find create-pet subcommand")

	// Should have a --name flag from the request body property
	nameFlag := createCmd.Flag("name")
	require.NotNil(t, nameFlag, "create-pet should have --name flag from request body")
	require.Equal(t, "name (request body)", nameFlag.Usage)
}

func TestGenerateCommands_PathParamFlag(t *testing.T) {
	gen, err := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	doc := loadPetstoreSpec(t)
	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	petsCmd := commands[0]
	// getPetById has a path param "petId" — should appear in usage, NOT as a flag
	var getCmd *cobra.Command
	for _, sub := range petsCmd.Commands() {
		if strings.HasPrefix(sub.Use, "get-pet-by-id") {
			getCmd = sub
			break
		}
	}
	require.NotNil(t, getCmd)

	// Path params are positional args in usage, not flags
	require.Contains(t, getCmd.Use, "<petId>")
	// Path param should NOT be a flag (addFlags skips In=="path")
	require.Nil(t, getCmd.Flag("pet-id"), "path params should not become flags")
}

// ---------------------------------------------------------------------------
// Additional coverage: addStringFlag, getFlagValue, crudName, operationName
// ---------------------------------------------------------------------------

func TestAddStringFlag_WithEnum(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	cmd := &cobra.Command{}
	param := &openapi.Parameter{
		Name:        "status",
		Description: "Filter by status",
		Enum:        []interface{}{"active", "inactive", "pending"},
		Default:     "active",
	}
	gen.addStringFlag(cmd, "status", "s", param, param.Description)

	flag := cmd.Flag("status")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "allowed: active, inactive, pending")
	require.Equal(t, "active", flag.DefValue)
	require.Equal(t, "s", flag.Shorthand)
}

func TestAddStringFlag_NoEnum(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	cmd := &cobra.Command{}
	param := &openapi.Parameter{
		Name:        "label",
		Description: "A label",
		Default:     "default-label",
	}
	gen.addStringFlag(cmd, "label", "l", param, param.Description)

	flag := cmd.Flag("label")
	require.NotNil(t, flag)
	require.Equal(t, "A label", flag.Usage)
	require.Equal(t, "default-label", flag.DefValue)
	require.Equal(t, "l", flag.Shorthand)
}

func TestGetFlagValue(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	t.Run("string", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("name", "", "")
		_ = cmd.Flags().Set("name", "test-value")
		val, err := gen.getFlagValue(cmd, "Name", "string")
		require.NoError(t, err)
		require.Equal(t, "test-value", val)
	})

	t.Run("integer", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Int("limit", 0, "")
		_ = cmd.Flags().Set("limit", "42")
		val, err := gen.getFlagValue(cmd, "Limit", "integer")
		require.NoError(t, err)
		require.Equal(t, 42, val)
	})

	t.Run("number", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Float64("ratio", 0, "")
		_ = cmd.Flags().Set("ratio", "3.14")
		val, err := gen.getFlagValue(cmd, "Ratio", "number")
		require.NoError(t, err)
		require.Equal(t, 3.14, val)
	})

	t.Run("boolean", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("verbose", false, "")
		_ = cmd.Flags().Set("verbose", "true")
		val, err := gen.getFlagValue(cmd, "Verbose", "boolean")
		require.NoError(t, err)
		require.Equal(t, true, val)
	})

	t.Run("array", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().StringSlice("tags", nil, "")
		_ = cmd.Flags().Set("tags", "a,b,c")
		val, err := gen.getFlagValue(cmd, "Tags", "array")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a", "b", "c"}, val.([]string))
	})

	t.Run("default/unknown type", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("custom", "", "")
		_ = cmd.Flags().Set("custom", "fallback")
		val, err := gen.getFlagValue(cmd, "Custom", "unknown-type")
		require.NoError(t, err)
		require.Equal(t, "fallback", val)
	})
}

func TestCrudName_AllMethods(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	t.Run("GET collection", func(t *testing.T) {
		result := gen.crudName("GET", "/users")
		require.Equal(t, "list-users", result)
	})

	t.Run("GET single resource", func(t *testing.T) {
		result := gen.crudName("GET", "/users/{id}")
		require.Equal(t, "get-user", result)
	})

	t.Run("POST", func(t *testing.T) {
		result := gen.crudName("POST", "/users")
		require.Equal(t, "create-user", result)
	})

	t.Run("PUT", func(t *testing.T) {
		result := gen.crudName("PUT", "/users/{id}")
		require.Equal(t, "update-user", result)
	})

	t.Run("PATCH", func(t *testing.T) {
		result := gen.crudName("PATCH", "/users/{id}")
		require.Equal(t, "update-user", result)
	})

	t.Run("DELETE", func(t *testing.T) {
		result := gen.crudName("DELETE", "/users/{id}")
		require.Equal(t, "delete-user", result)
	})

	t.Run("unknown method", func(t *testing.T) {
		result := gen.crudName("OPTIONS", "/users")
		require.Equal(t, "options", result)
	})
}

func TestGetCommandName_CustomMapping(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
		CustomMappings: map[string]string{
			"listPets": "my-custom-list",
		},
	})

	op := &openapi.Operation{
		ID:     "listPets",
		Path:   "/pets",
		Method: "GET",
	}
	result := gen.getCommandName(op)
	require.Equal(t, "my-custom-list", result)
}

func TestGetCommandName_NoCustomMapping(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName: "muster",
		BaseURL: "https://api.example.com",
	})

	op := &openapi.Operation{
		ID:     "listPets",
		Path:   "/pets",
		Method: "GET",
	}
	result := gen.getCommandName(op)
	require.Equal(t, "list-pets", result)
}

func TestOperationName_StyleHTTP(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:        "muster",
		BaseURL:        "https://api.example.com",
		OperationStyle: StyleHTTP,
	})

	result := gen.operationName("GET", "/users")
	require.Equal(t, "get-users", result)
}

func TestOperationName_StyleFlat(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:        "muster",
		BaseURL:        "https://api.example.com",
		OperationStyle: StyleFlat,
	})

	result := gen.operationName("GET", "/users")
	require.Equal(t, "get-users", result)
}

func TestBuildUsage_OptionalPathParam(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	op := &openapi.Operation{
		Method: "GET",
		Path:   "/users/{id}",
		Parameters: openapi.Parameters{
			{Name: "id", In: "path", Required: false},
		},
	}
	result := gen.buildUsage("get", op)
	// Optional path param gets wrapped in brackets
	require.Equal(t, "get [<id>] [flags]", result)
}

func TestBuildUsage_MultiplePathParams(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	op := &openapi.Operation{
		Method: "GET",
		Path:   "/orgs/{orgId}/users/{userId}",
		Parameters: openapi.Parameters{
			{Name: "orgId", In: "path", Required: true},
			{Name: "userId", In: "path", Required: true},
		},
	}
	result := gen.buildUsage("get", op)
	require.Equal(t, "get <orgId> <userId> [flags]", result)
}

func TestExtractResource_TagFallback(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})

	t.Run("uses first tag", func(t *testing.T) {
		op := &openapi.Operation{
			Path: "/api/v1/users",
			Tags: []string{"Users"},
		}
		result := gen.extractResource(op)
		require.Equal(t, "users", result)
	})

	t.Run("no tags, uses path", func(t *testing.T) {
		op := &openapi.Operation{
			Path: "/api/v1/orders",
		}
		result := gen.extractResource(op)
		require.Equal(t, "api", result)
	})

	t.Run("no tags, empty path", func(t *testing.T) {
		op := &openapi.Operation{
			Path: "/",
		}
		result := gen.extractResource(op)
		// strings.Split(strings.Trim("/", "/"), "/") returns [""] (len=1),
		// so normalizeName("") returns "" — not "default"
		require.Equal(t, "", result)
	})
}

func TestIsExcluded_ResourceMatch(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:     "muster",
		BaseURL:     "https://api.example.com",
		ExcludeTags: []string{"admin"},
	})

	// Resource name matches excluded tag directly
	require.True(t, gen.isExcluded("admin", nil))
	// Resource name doesn't match
	require.False(t, gen.isExcluded("users", nil))
}

func TestIsExcluded_IncludeOnlyResourceMatch(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:         "muster",
		BaseURL:         "https://api.example.com",
		IncludeOnlyTags: []string{"users"},
	})

	// Resource name matches included tag
	require.False(t, gen.isExcluded("users", nil))
	// Resource name doesn't match
	require.True(t, gen.isExcluded("posts", nil))
}

// =============================================================================
// AC-351: addFlags coverage (39.7% → 70%+)
// =============================================================================

// makeTestGen returns a Generator for use in addFlags tests.
func makeTestGen(t *testing.T) *Generator {
	t.Helper()
	gen, err := NewGenerator(Config{AppName: "muster", BaseURL: "https://api.example.com"})
	require.NoError(t, err)
	return gen
}

func TestAddFlags_StringParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "limit", In: "query", Type: "string", Description: "Max results"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("limit")
	require.NotNil(t, flag)
	require.Equal(t, "Max results", flag.Usage)
}

func TestAddFlags_IntegerParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "count", In: "query", Type: "integer", Description: "Item count"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("count")
	require.NotNil(t, flag)
	require.Equal(t, "Item count", flag.Usage)
	// Default should be 0 when no Default set
	val, err := cmd.Flags().GetInt("count")
	require.NoError(t, err)
	require.Equal(t, 0, val)
}

func TestAddFlags_IntegerParamWithDefault(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "count", In: "query", Type: "integer", Description: "Item count", Default: float64(25)},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetInt("count")
	require.NoError(t, err)
	require.Equal(t, 25, val)
}

func TestAddFlags_NumberParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "ratio", In: "query", Type: "number", Description: "Some ratio"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetFloat64("ratio")
	require.NoError(t, err)
	require.Equal(t, 0.0, val)
}

func TestAddFlags_NumberParamWithDefault(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "ratio", In: "query", Type: "number", Description: "Some ratio", Default: float64(3.14)},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetFloat64("ratio")
	require.NoError(t, err)
	require.Equal(t, 3.14, val)
}

func TestAddFlags_BooleanParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "verbose", In: "query", Type: "boolean", Description: "Verbose output"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetBool("verbose")
	require.NoError(t, err)
	require.Equal(t, false, val)
}

func TestAddFlags_BooleanParamDefaultTrue(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "enabled", In: "query", Type: "boolean", Description: "Enable feature", Default: true},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetBool("enabled")
	require.NoError(t, err)
	require.Equal(t, true, val)
}

func TestAddFlags_ArrayParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "tags", In: "query", Type: "array", Description: "Filter tags"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("tags")
	require.NotNil(t, flag)
	// Should be a StringSlice flag
	vals, err := cmd.Flags().GetStringSlice("tags")
	require.NoError(t, err)
	require.Empty(t, vals)
}

func TestAddFlags_SkipsPathParam(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "id", In: "path", Type: "string", Required: true},
		},
	}
	gen.addFlags(cmd, op)
	// Path param should NOT be added as a flag
	flag := cmd.Flags().Lookup("id")
	require.Nil(t, flag, "path param should not be a flag")
}

func TestAddFlags_RequiredFlag(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "name", In: "query", Type: "string", Required: true, Description: "User name"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("name")
	require.NotNil(t, flag)
	// Verify the annotation exists on the flag
	_, exists := flag.Annotations[cobra.BashCompOneRequiredFlag]
	require.True(t, exists, "required flag should have required annotation")
}

func TestAddFlags_MultipleParams(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			{Name: "limit", In: "query", Type: "integer", Default: float64(10)},
			{Name: "offset", In: "query", Type: "integer"},
			{Name: "filter", In: "query", Type: "string"},
		},
	}
	gen.addFlags(cmd, op)
	require.NotNil(t, cmd.Flags().Lookup("limit"))
	require.NotNil(t, cmd.Flags().Lookup("offset"))
	require.NotNil(t, cmd.Flags().Lookup("filter"))
	// Verify defaults
	lim, _ := cmd.Flags().GetInt("limit")
	require.Equal(t, 10, lim)
}

func TestAddFlags_HasMultiPart(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		HasMultiPart: true,
	}
	gen.addFlags(cmd, op)
	// Should have --file flag
	fileFlag := cmd.Flags().Lookup("file")
	require.NotNil(t, fileFlag)
	require.Equal(t, "File path to upload (multipart/form-data)", fileFlag.Usage)
}

func TestAddFlags_MultipartFields(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		HasMultiPart: true,
		MultipartFields: []openapi.RequestBodyProperty{
			{Name: "title", Description: "Document title"},
			{Name: "tag", Description: ""},
		},
	}
	gen.addFlags(cmd, op)
	titleFlag := cmd.Flags().Lookup("title")
	require.NotNil(t, titleFlag)
	require.Contains(t, titleFlag.Usage, "Document title")

	tagFlag := cmd.Flags().Lookup("tag")
	require.NotNil(t, tagFlag)
	require.Contains(t, tagFlag.Usage, "(form field)")
}

func TestAddFlags_RequestBodyString(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "name", Type: "string", Description: "User name"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("name")
	require.NotNil(t, flag)
	require.Equal(t, "User name", flag.Usage)
}

func TestAddFlags_RequestBodyStringDefault(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "role", Type: "string", Description: "User role", Default: "viewer"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetString("role")
	require.NoError(t, err)
	require.Equal(t, "viewer", val)
}

func TestAddFlags_RequestBodyStringWithEnum(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "role", Type: "string", Description: "User role", Enum: []interface{}{"admin", "editor", "viewer"}},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("role")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "allowed:")
	require.Contains(t, flag.Usage, "admin, editor, viewer")
}

func TestAddFlags_RequestBodyInteger(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "age", Type: "integer", Description: "User age"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetInt("age")
	require.NoError(t, err)
	require.Equal(t, 0, val)
}

func TestAddFlags_RequestBodyIntegerDefaultFloat64(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "age", Type: "integer", Description: "User age", Default: float64(30)},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetInt("age")
	require.NoError(t, err)
	require.Equal(t, 30, val)
}

func TestAddFlags_RequestBodyIntegerDefaultInt(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "age", Type: "integer", Description: "User age", Default: int(42)},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetInt("age")
	require.NoError(t, err)
	require.Equal(t, 42, val)
}

func TestAddFlags_RequestBodyNumber(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "score", Type: "number", Description: "User score"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetFloat64("score")
	require.NoError(t, err)
	require.Equal(t, 0.0, val)
}

func TestAddFlags_RequestBodyNumberDefault(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "score", Type: "number", Description: "User score", Default: float64(99.5)},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetFloat64("score")
	require.NoError(t, err)
	require.Equal(t, 99.5, val)
}

func TestAddFlags_RequestBodyBoolean(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "active", Type: "boolean", Description: "Is active"},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetBool("active")
	require.NoError(t, err)
	require.Equal(t, false, val)
}

func TestAddFlags_RequestBodyBooleanDefaultTrue(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "active", Type: "boolean", Description: "Is active", Default: true},
		},
	}
	gen.addFlags(cmd, op)
	val, err := cmd.Flags().GetBool("active")
	require.NoError(t, err)
	require.Equal(t, true, val)
}

func TestAddFlags_RequestBodyDedupMultipart(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		HasMultiPart: true,
		MultipartFields: []openapi.RequestBodyProperty{
			{Name: "title", Description: "From multipart"},
		},
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "title", Type: "string", Description: "From body"},
			{Name: "description", Type: "string", Description: "Only in body"},
		},
	}
	gen.addFlags(cmd, op)
	// title should exist (from multipart) with multipart description
	titleFlag := cmd.Flags().Lookup("title")
	require.NotNil(t, titleFlag)
	require.Contains(t, titleFlag.Usage, "From multipart")
	// description should exist (only in body, not deduplicated)
	descFlag := cmd.Flags().Lookup("description")
	require.NotNil(t, descFlag)
	require.Contains(t, descFlag.Usage, "Only in body")
}

func TestAddFlags_EmptyBodyDescription(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "data", Type: "string"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("data")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "(request body)")
}

// =============================================================================
// executeAndPrint tests (AC-366)
// =============================================================================

// newExecuteAndPrintCmd creates a cobra command with the --output and --raw
// flags that executeAndPrint expects, wired to the given buffer.
func newExecuteAndPrintCmd(buf *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.Flags().StringP("output", "o", "table", "Output format (table, json, yaml)")
	cmd.Flags().BoolP("raw", "r", false, "Output raw API response")
	return cmd
}

func TestExecuteAndPrint_Success(t *testing.T) {
	// httptest server that returns JSON
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "json")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, `"status": "ok"`)
}

func TestExecuteAndPrint_HTTPError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "json")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP error")
}

func TestExecuteAndPrint_EmptyResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "DELETE")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
}

func TestExecuteAndPrint_ContextCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`ok`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before execute

	err = executeAndPrint(gen, builder, cmd, ctx)
	require.Error(t, err)
}

func TestExecuteAndPrint_404Response(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/missing", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "json")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP error")
}

// =============================================================================
// executeAndPrint format rendering tests (DF-005)
// =============================================================================

func TestExecuteAndPrint_TableFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/items", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "table")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// Table should have header row with keys and data rows
	require.Contains(t, output, "id")
	require.Contains(t, output, "name")
	require.Contains(t, output, "alpha")
	require.Contains(t, output, "beta")
	// Should NOT be raw JSON (no brackets)
	require.NotContains(t, output, "[{")
}

func TestExecuteAndPrint_TableFormat_SingleObject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","count":42}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/status", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "table")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "status")
	require.Contains(t, output, "ok")
	require.Contains(t, output, "count")
	require.Contains(t, output, "42")
}

func TestExecuteAndPrint_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "json")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// Indented JSON
	require.Contains(t, output, `"status": "ok"`)
}

func TestExecuteAndPrint_DefaultFormatIsTable(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"value"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	// Don't set --output — default is "table" from flag default

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// Table format: key/value rows
	require.Contains(t, output, "key")
	require.Contains(t, output, "value")
	// Should NOT be raw JSON
	require.NotContains(t, output, `{"key":"value"}`)
}

func TestExecuteAndPrint_RawFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"raw":"data","nested":{"a":1}}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("raw", "true")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// Raw output = exact body
	require.Contains(t, output, `{"raw":"data","nested":{"a":1}}`)
}

func TestExecuteAndPrint_YAMLFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","count":42}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "yaml")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// YAML output should contain key: value pairs (no JSON braces)
	require.Contains(t, output, "status: ok")
	require.Contains(t, output, "count: 42")
	require.NotContains(t, output, `{`)
}

func TestExecuteAndPrint_UnknownFormatError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "bogus")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported output format")
	require.Contains(t, err.Error(), "bogus")
}

func TestExecuteAndPrint_NonJSONBodyDumpsRaw(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`plain text response`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{AppName: "test", BaseURL: ts.URL})
	require.NoError(t, err)

	builder := request.NewBuilder(ts.URL, "/api/test", "GET")
	var buf bytes.Buffer
	cmd := newExecuteAndPrintCmd(&buf)
	cmd.Flags().Set("output", "table")

	err = executeAndPrint(gen, builder, cmd, context.Background())
	require.NoError(t, err)
	output := buf.String()
	// Non-JSON body is dumped raw regardless of format
	require.Contains(t, output, "plain text response")
}

// =============================================================================
// InitializeDynamicCommands tests (AC-366)
// =============================================================================

func TestInitializeDynamicCommands_ValidSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test-spec.yaml")
	err := os.WriteFile(specPath, []byte(petstoreSpecJSON()), 0644)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "test-cli"}

	err = InitializeDynamicCommands(rootCmd, specPath)
	require.NoError(t, err)

	// Should have at least one subcommand (pets group)
	require.NotEmpty(t, rootCmd.Commands())
}

func TestInitializeDynamicCommands_CachedResult(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test-spec.yaml")
	err := os.WriteFile(specPath, []byte(petstoreSpecJSON()), 0644)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "test-cli"}

	// First call should load and generate
	err = InitializeDynamicCommands(rootCmd, specPath)
	require.NoError(t, err)
	require.NotEmpty(t, rootCmd.Commands())

	// Second call should find cached result and return nil
	rootCmd2 := &cobra.Command{Use: "test-cli2"}
	err = InitializeDynamicCommands(rootCmd2, specPath)
	require.NoError(t, err)
	// Cached returns nil — no new subcommands added
}

func TestInitializeDynamicCommands_NonExistentFile(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test-cli"}

	err := InitializeDynamicCommands(rootCmd, "/tmp/nonexistent-openapi-spec-12345.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load spec")
}

func TestInitializeDynamicCommands_InvalidSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "bad-spec.yaml")
	err := os.WriteFile(specPath, []byte(`not a valid yaml!!!`), 0644)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "test-cli"}

	err = InitializeDynamicCommands(rootCmd, specPath)
	require.Error(t, err)
}

// =============================================================================
// DF-002: nested object body props marshaled as JSON objects, array props
// accepting JSON array syntax alongside CSV.
// =============================================================================

// TestParseObjectBodyValue_ValidObject proves an object-typed body prop value
// is parsed into a real JSON object (not a string) in the assembled body.
func TestParseObjectBodyValue_ValidObject(t *testing.T) {
	raw := `{"color":"red","size":1.5}`
	parsed, err := parseObjectBodyValue("meta", raw)
	require.NoError(t, err)

	// Marshal the parsed value — it should be a JSON object, not a string
	out, err := json.Marshal(parsed)
	require.NoError(t, err)
	require.JSONEq(t, `{"color":"red","size":1.5}`, string(out))

	// Verify it's a map (object), not a string
	_, isMap := parsed.(map[string]interface{})
	require.True(t, isMap, "parsed object should be a map[string]interface{}")
}

// TestParseObjectBodyValue_InvalidJSON proves invalid JSON object input
// returns a clear error with the flag name and value.
func TestParseObjectBodyValue_InvalidJSON(t *testing.T) {
	raw := `{bad json}`
	_, err := parseObjectBodyValue("meta", raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--meta")
	require.Contains(t, err.Error(), "invalid JSON")
}

// TestParseArrayBodyValue_JSONArray proves a JSON array string is accepted
// for an array-typed body prop.
func TestParseArrayBodyValue_JSONArray(t *testing.T) {
	raw := `["a","b"]`
	parsed, err := parseArrayBodyValue("labels", raw)
	require.NoError(t, err)

	out, err := json.Marshal(parsed)
	require.NoError(t, err)
	require.JSONEq(t, `["a","b"]`, string(out))
}

// TestParseArrayBodyValue_CSV proves CSV still works for backward compat.
func TestParseArrayBodyValue_CSV(t *testing.T) {
	raw := `a,b`
	parsed, err := parseArrayBodyValue("labels", raw)
	require.NoError(t, err)

	out, err := json.Marshal(parsed)
	require.NoError(t, err)
	// CSV split produces ["a","b"] which marshals to a JSON array
	require.JSONEq(t, `["a","b"]`, string(out))
}

// TestParseArrayBodyValue_CSVWithSpaces proves CSV items have spaces trimmed,
// matching pflag StringSlice semantics.
func TestParseArrayBodyValue_CSVWithSpaces(t *testing.T) {
	raw := `a, b , c`
	parsed, err := parseArrayBodyValue("labels", raw)
	require.NoError(t, err)

	out, err := json.Marshal(parsed)
	require.NoError(t, err)
	require.JSONEq(t, `["a","b","c"]`, string(out))
}

// TestParseArrayBodyValue_InvalidJSON proves invalid JSON array returns error.
func TestParseArrayBodyValue_InvalidJSON(t *testing.T) {
	raw := `["a",bad]`
	_, err := parseArrayBodyValue("labels", raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--labels")
	require.Contains(t, err.Error(), "invalid JSON array")
}

// TestBodyAssembly_ObjectInRequestBody is an integration test that proves the
// full createRunHandler path assembles an object body prop as a real JSON
// object. It sets up a test HTTP server that echoes the request body, then
// invokes the cobra command with --meta '{"color":"red","size":1.5}'.
func TestBodyAssembly_ObjectInRequestBody(t *testing.T) {
	// Test server echoes the request body
	var receivedBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{
		AppName: "test",
		BaseURL: ts.URL,
	})
	require.NoError(t, err)

	op := &openapi.Operation{
		ID:     "createWidget",
		Path:   "/widgets",
		Method: "POST",
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "name", Type: "string"},
			{Name: "meta", Type: "object"},
		},
	}

	cmd := &cobra.Command{
		Use:  "create-widget",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)

	cmd.SetArgs([]string{"--name", "gadget", "--meta", `{"color":"red","size":1.5}`})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	execErr := cmd.Execute()
	require.NoError(t, execErr)

	// The received body should have meta as a JSON OBJECT, not a string
	require.NotEmpty(t, receivedBody)
	var bodyMap map[string]interface{}
	err = json.Unmarshal([]byte(receivedBody), &bodyMap)
	require.NoError(t, err)

	metaVal, ok := bodyMap["meta"]
	require.True(t, ok, "meta should be in the body")

	// meta should be a map (object), not a string
	metaMap, isMap := metaVal.(map[string]interface{})
	require.True(t, isMap, "meta should be a JSON object, not a string; got %T", metaVal)
	require.Equal(t, "red", metaMap["color"])
	require.Equal(t, 1.5, metaMap["size"])
}

// TestBodyAssembly_ArrayJSONInRequestBody proves the full createRunHandler
// path accepts JSON array syntax for array-typed body props.
func TestBodyAssembly_ArrayJSONInRequestBody(t *testing.T) {
	var receivedBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{
		AppName: "test",
		BaseURL: ts.URL,
	})
	require.NoError(t, err)

	op := &openapi.Operation{
		ID:     "createWidget",
		Path:   "/widgets",
		Method: "POST",
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "name", Type: "string"},
			{Name: "labels", Type: "array"},
		},
	}

	cmd := &cobra.Command{
		Use:  "create-widget",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)

	cmd.SetArgs([]string{"--name", "gadget", "--labels", `["a","b"]`})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	execErr := cmd.Execute()
	require.NoError(t, execErr)

	require.NotEmpty(t, receivedBody)
	var bodyMap map[string]interface{}
	err = json.Unmarshal([]byte(receivedBody), &bodyMap)
	require.NoError(t, err)

	labelsVal, ok := bodyMap["labels"]
	require.True(t, ok, "labels should be in the body")

	labelsArr, isSlice := labelsVal.([]interface{})
	require.True(t, isSlice, "labels should be a JSON array, not a string; got %T", labelsVal)
	require.Len(t, labelsArr, 2)
	require.Equal(t, "a", labelsArr[0])
	require.Equal(t, "b", labelsArr[1])
}

// TestBodyAssembly_ArrayCSVInRequestBody proves CSV still works for array
// body props (backward compatibility).
func TestBodyAssembly_ArrayCSVInRequestBody(t *testing.T) {
	var receivedBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{
		AppName: "test",
		BaseURL: ts.URL,
	})
	require.NoError(t, err)

	op := &openapi.Operation{
		ID:     "createWidget",
		Path:   "/widgets",
		Method: "POST",
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "name", Type: "string"},
			{Name: "labels", Type: "array"},
		},
	}

	cmd := &cobra.Command{
		Use:  "create-widget",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)

	cmd.SetArgs([]string{"--name", "gadget", "--labels", "a,b"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	execErr := cmd.Execute()
	require.NoError(t, execErr)

	require.NotEmpty(t, receivedBody)
	var bodyMap map[string]interface{}
	err = json.Unmarshal([]byte(receivedBody), &bodyMap)
	require.NoError(t, err)

	labelsVal, ok := bodyMap["labels"]
	require.True(t, ok)

	labelsArr, isSlice := labelsVal.([]interface{})
	require.True(t, isSlice, "labels should be a JSON array; got %T", labelsVal)
	require.Len(t, labelsArr, 2)
	require.Equal(t, "a", labelsArr[0])
	require.Equal(t, "b", labelsArr[1])
}

// TestBodyAssembly_InvalidJSONObject proves invalid JSON in an object flag
// produces a clear error and non-zero exit.
func TestBodyAssembly_InvalidJSONObject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	gen, err := NewGenerator(Config{
		AppName: "test",
		BaseURL: ts.URL,
	})
	require.NoError(t, err)

	op := &openapi.Operation{
		ID:     "createWidget",
		Path:   "/widgets",
		Method: "POST",
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "name", Type: "string"},
			{Name: "meta", Type: "object"},
		},
	}

	cmd := &cobra.Command{
		Use:  "create-widget",
		RunE: gen.createRunHandler(op),
	}
	gen.addFlags(cmd, op)

	cmd.SetArgs([]string{"--name", "gadget", "--meta", `{bad json}`})
	var errBuf bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errBuf)

	execErr := cmd.Execute()
	require.Error(t, execErr)
	require.Contains(t, execErr.Error(), "--meta")
	require.Contains(t, execErr.Error(), "invalid JSON")
}

// TestAddFlags_RequestBodyObjectFlagHelp verifies the flag help text for
// object-typed body props documents JSON input syntax.
func TestAddFlags_RequestBodyObjectFlagHelp(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "meta", Type: "object", Description: "Widget metadata"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("meta")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "JSON object")
}

// TestAddFlags_RequestBodyArrayFlagHelp verifies the flag help text for
// array-typed body props documents both JSON array and CSV syntax.
func TestAddFlags_RequestBodyArrayFlagHelp(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "labels", Type: "array", Description: "Widget labels"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("labels")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "JSON array or comma-separated")
}

// TestAddFlags_RequestBodyObjectFlagHelp_EmptyDesc verifies the flag help
// text for object-typed body props with no description from the spec.
func TestAddFlags_RequestBodyObjectFlagHelp_EmptyDesc(t *testing.T) {
	gen := makeTestGen(t)
	cmd := &cobra.Command{Use: "test"}
	op := &openapi.Operation{
		RequestBody: []openapi.RequestBodyProperty{
			{Name: "meta", Type: "object"},
		},
	}
	gen.addFlags(cmd, op)
	flag := cmd.Flags().Lookup("meta")
	require.NotNil(t, flag)
	require.Contains(t, flag.Usage, "JSON object")
	require.Contains(t, flag.Usage, "request body")
}
