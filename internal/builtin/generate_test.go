package builtin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/config"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGenerateCommand(t *testing.T) {
	t.Run("creates generate command", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		assert.NotNil(t, cmd)
		assert.Equal(t, "generate", cmd.Name())
		assert.Equal(t, "Generate commands from OpenAPI specification", cmd.Short)
		assert.Contains(t, cmd.Long, "Generate CLI commands dynamically from an OpenAPI")
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s), received 0")
	})

	t.Run("has base-url flag", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		flag := cmd.Flags().Lookup("base-url")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has short flag for base-url", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		flag := cmd.Flags().ShorthandLookup("u")
		assert.NotNil(t, flag)
		assert.Equal(t, "base-url", flag.Name)
	})

	t.Run("fails with non-existent spec file", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		err := cmd.RunE(cmd, []string{"/nonexistent/spec.yaml"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "read spec file")
	})

	t.Run("executes with valid spec file", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		err = cmd.RunE(cmd, []string{specPath})

		require.NoError(t, err)
	})

	t.Run("executes with custom base-url", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("base-url", "https://api.test.com")

		err = cmd.RunE(cmd, []string{specPath})

		require.NoError(t, err)
	})

	t.Run("executes with URL-based spec", func(t *testing.T) {
		specContent := `{
  "openapi": "3.0.0",
  "info": {"title": "URL API", "version": "1.0.0"},
  "paths": {
    "/health": {
      "get": {
        "operationId": "getHealth",
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, specContent)
		}))
		defer server.Close()

		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("base-url", server.URL)

		err := cmd.RunE(cmd, []string{server.URL})

		require.NoError(t, err)
	})

	t.Run("rejects bad URL gracefully", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)

		err := cmd.RunE(cmd, []string{"http://localhost:19999/nonexistent"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetch spec")
	})
}

func TestCommandsToMetadata(t *testing.T) {
	t.Run("converts commands to metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths: {}
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		metadata, err := commandsToMetadata(nil, specPath, "https://api.test.com", nil, specContent)

		require.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, specPath, metadata.SpecPath)
		assert.Equal(t, "https://api.test.com", metadata.BaseURL)
		assert.NotEmpty(t, metadata.SpecHash)
	})

	t.Run("handles empty commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "spec.yaml")

		metadata, err := commandsToMetadata(nil, specPath, "https://api.test.com", nil, nil)

		require.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Empty(t, metadata.Commands)
	})
}

func TestExtractPathParamsFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "no params",
			path:     "/users",
			expected: []string{},
		},
		{
			name:     "single param",
			path:     "/users/{id}",
			expected: []string{"id"},
		},
		{
			name:     "multiple params",
			path:     "/users/{userId}/posts/{postId}",
			expected: []string{"userId", "postId"},
		},
		{
			name:     "nested path with param",
			path:     "/api/v1/resources/{resourceId}",
			expected: []string{"resourceId"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPathParamsFromPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"getHealth", "get-health"},
		{"listUsers", "list-users"},
		{"createUserPost", "create-user-post"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toKebabCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user", "users"},
		{"post", "posts"},
		{"resource", "resources"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := pluralize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "user"},
		{"posts", "post"},
		{"resources", "resource"},
		{"user", "user"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := singularize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInferFromCommandName(t *testing.T) {
	tests := []struct {
		name           string
		commandName    string
		expectedPath   string
		expectedMethod string
	}{
		{
			name:           "list command",
			commandName:    "list-pet",
			expectedPath:   "/pets",
			expectedMethod: "GET",
		},
		{
			name:           "get command",
			commandName:    "get-pet",
			expectedPath:   "/pets/{petId}",
			expectedMethod: "GET",
		},
		{
			name:           "create command",
			commandName:    "create-pet",
			expectedPath:   "/pets",
			expectedMethod: "POST",
		},
		{
			name:           "update command",
			commandName:    "update-pet",
			expectedPath:   "/pets/{petId}",
			expectedMethod: "PUT",
		},
		{
			name:           "delete command",
			commandName:    "delete-pet",
			expectedPath:   "/pets/{petId}",
			expectedMethod: "DELETE",
		},
		{
			name:           "unknown action",
			commandName:    "unknown-pet",
			expectedPath:   "",
			expectedMethod: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, method := inferFromCommandName(tt.commandName)
			assert.Equal(t, tt.expectedPath, path)
			assert.Equal(t, tt.expectedMethod, method)
		})
	}
}

func TestComputeSpecHash(t *testing.T) {
	t.Run("computes hash for data", func(t *testing.T) {
		specContent := []byte(`openapi: 3.0.0`)

		hash := computeSpecHash(specContent)

		assert.NotEmpty(t, hash)
		assert.Len(t, hash, 16)
	})

	t.Run("different content produces different hash", func(t *testing.T) {
		hash1 := computeSpecHash([]byte(`openapi: 3.0.0`))
		hash2 := computeSpecHash([]byte(`openapi: 3.1.0`))

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("empty data produces valid hash", func(t *testing.T) {
		hash := computeSpecHash([]byte{})

		// Returns SHA256 hash of empty data truncated to 16 hex chars
		assert.NotEmpty(t, hash)
		assert.Equal(t, "e3b0c44298fc1c14", hash) // SHA256 of empty data truncated
	})
}

func TestGetFlagType(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("str", "test", "")
	fs.Int("num", 42, "")
	fs.Bool("flag", true, "")
	fs.StringSlice("slice", []string{"a", "b"}, "")

	tests := []struct {
		name     string
		flagName string
		expected string
	}{
		{"string", "str", "string"},
		{"int", "num", "int"},
		{"bool", "flag", "bool"},
		{"stringSlice", "slice", "array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := fs.Lookup(tt.flagName)
			require.NotNil(t, flag)
			result := getFlagType(flag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeFlagName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple lowercase", "name", "name"},
		{"camelCase", "camelCase", "camel-case"},
		{"PascalCase", "PascalCase", "pascal-case"},
		{"multipleWords", "requestBodySchema", "request-body-schema"},
		{"singleUppercase", "ID", "i-d"},
		{"leadingUppercase", "HTTPHeader", "h-t-t-p-header"},
		{"empty string", "", ""},
		{"already kebab", "already-kebab", "already-kebab"},
		{"numbers", "maxRetries3", "max-retries3"},
		{"allUppercase", "URL", "u-r-l"},
		{"mixedWithUnderscore", "some_Value", "some_-value"},
		{"unicode", "Straße", "straße"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFlagName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveRequestBodySchema(t *testing.T) {
	t.Run("nil media type returns nil", func(t *testing.T) {
		doc := &openapi3.T{}
		result := resolveRequestBodySchema(doc, nil)
		assert.Nil(t, result)
	})

	t.Run("media type with nil schema returns nil", func(t *testing.T) {
		doc := &openapi3.T{}
		mt := &openapi3.MediaType{Schema: nil}
		result := resolveRequestBodySchema(doc, mt)
		assert.Nil(t, result)
	})

	t.Run("media type with inline schema", func(t *testing.T) {
		doc := &openapi3.T{}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Properties: openapi3.Schemas{
						"name": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		require.NotNil(t, result)
		assert.Len(t, result.Properties, 1)
	})

	t.Run("media type with empty properties returns nil", func(t *testing.T) {
		doc := &openapi3.T{}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Properties: openapi3.Schemas{},
				},
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		assert.Nil(t, result)
	})

	t.Run("media type with $ref resolves from components", func(t *testing.T) {
		doc := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{
					"Task": &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: openapi3.Schemas{
								"title": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							},
						},
					},
				},
			},
		}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Ref: "#/components/schemas/Task",
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		require.NotNil(t, result)
		assert.Len(t, result.Properties, 1)
	})

	t.Run("media type with $ref to non-existent component returns nil", func(t *testing.T) {
		doc := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{},
			},
		}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Ref: "#/components/schemas/NonExistent",
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		assert.Nil(t, result)
	})

	t.Run("media type with non-components $ref returns nil", func(t *testing.T) {
		doc := &openapi3.T{}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Ref: "#/paths/~1tasks/post/requestBody",
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		assert.Nil(t, result)
	})

	t.Run("null components returns nil for $ref", func(t *testing.T) {
		doc := &openapi3.T{Components: nil}
		mt := &openapi3.MediaType{
			Schema: &openapi3.SchemaRef{
				Ref: "#/components/schemas/Task",
			},
		}
		result := resolveRequestBodySchema(doc, mt)
		assert.Nil(t, result)
	})
}
