package builtin

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/generator"
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

	t.Run("requires exactly one argument when not --list/--clear", func(t *testing.T) {
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

// makeMultipartDoc creates an OpenAPI document with a multipart/form-data
// operation at the given path and method, with the given property schemas.
// Fields with Format "binary" are treated as file uploads.
func makeMultipartDoc(path, method string, properties openapi3.Schemas) *openapi3.T {
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	pi := &openapi3.PathItem{}
	switch method {
	case http.MethodPost:
		pi.Post = &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"multipart/form-data": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:       &openapi3.Types{"object"},
									Properties: properties,
								},
							},
						},
					},
				},
			},
		}
	case http.MethodPut:
		pi.Put = &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"multipart/form-data": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:       &openapi3.Types{"object"},
									Properties: properties,
								},
							},
						},
					},
				},
			},
		}
	}
	doc.Paths.Set(path, pi)
	return doc
}

// makeJSONBodyDoc creates an OpenAPI document with an application/json
// request body operation at the given path and method.
func makeJSONBodyDoc(path, method string, properties openapi3.Schemas) *openapi3.T {
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	pi := &openapi3.PathItem{}
	switch method {
	case http.MethodPost:
		pi.Post = &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:       &openapi3.Types{"object"},
									Properties: properties,
								},
							},
						},
					},
				},
			},
		}
	case http.MethodPut:
		pi.Put = &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type:       &openapi3.Types{"object"},
									Properties: properties,
								},
							},
						},
					},
				},
			},
		}
	}
	doc.Paths.Set(path, pi)
	return doc
}

func TestDetectMultipartOperation(t *testing.T) {
	t.Run("nil document returns false", func(t *testing.T) {
		isMulti, fields := detectMultipartOperation(nil, "/upload", http.MethodPost)
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("empty path returns false", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{})
		isMulti, fields := detectMultipartOperation(doc, "", http.MethodPost)
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("empty method returns false", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{})
		isMulti, fields := detectMultipartOperation(doc, "/upload", "")
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("non-existent path returns false", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{})
		isMulti, fields := detectMultipartOperation(doc, "/nonexistent", http.MethodPost)
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("no operation for method returns false", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodGet)
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("operation without requestBody returns false", func(t *testing.T) {
		doc := &openapi3.T{Paths: openapi3.NewPaths()}
		pi := &openapi3.PathItem{
			Post: &openapi3.Operation{}, // no RequestBody
		}
		doc.Paths.Set("/tasks", pi)
		isMulti, fields := detectMultipartOperation(doc, "/tasks", http.MethodPost)
		assert.False(t, isMulti)
		assert.Nil(t, fields)
	})

	t.Run("valid multipart with no fields returns true with empty map", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPost)
		assert.True(t, isMulti)
		assert.NotNil(t, fields)
		assert.Empty(t, fields)
	})

	t.Run("multipart with only binary fields returns true with empty map", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{
			"fileName": &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:   &openapi3.Types{"string"},
					Format: "binary",
				},
			},
		})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPost)
		assert.True(t, isMulti)
		assert.Empty(t, fields)
	})

	t.Run("multipart with non-binary fields returns normalized names", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{
			"title": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPost)
		assert.True(t, isMulti)
		assert.True(t, fields["title"])
	})

	t.Run("multipart with mixed binary and non-binary skips binary", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{
			"fileName": &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:   &openapi3.Types{"string"},
					Format: "binary",
				},
			},
			"title": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
			"description": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPost)
		assert.True(t, isMulti)
		assert.True(t, fields["title"])
		assert.True(t, fields["description"])
		assert.False(t, fields["file-name"], "binary fields should be skipped")
	})

	t.Run("nil property value skipped gracefully", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPost, openapi3.Schemas{
			"bad":   &openapi3.SchemaRef{Value: nil},
			"valid": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPost)
		assert.True(t, isMulti)
		assert.True(t, fields["valid"])
		assert.False(t, fields["bad"])
	})

	t.Run("PUT method works", func(t *testing.T) {
		doc := makeMultipartDoc("/upload", http.MethodPut, openapi3.Schemas{
			"name": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		isMulti, fields := detectMultipartOperation(doc, "/upload", http.MethodPut)
		assert.True(t, isMulti)
		assert.True(t, fields["name"])
	})
}

func TestExtractBodyPropertyNames(t *testing.T) {
	t.Run("nil document returns nil", func(t *testing.T) {
		result := extractBodyPropertyNames(nil, "/tasks", http.MethodPost)
		assert.Nil(t, result)
	})

	t.Run("empty path returns nil", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{
			"name": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "", http.MethodPost)
		assert.Nil(t, result)
	})

	t.Run("empty method returns nil", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{})
		result := extractBodyPropertyNames(doc, "/tasks", "")
		assert.Nil(t, result)
	})

	t.Run("non-existent path returns nil", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{})
		result := extractBodyPropertyNames(doc, "/nonexistent", http.MethodPost)
		assert.Nil(t, result)
	})

	t.Run("no operation for method returns nil", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{
			"name": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodGet)
		assert.Nil(t, result)
	})

	t.Run("operation without requestBody returns nil", func(t *testing.T) {
		doc := &openapi3.T{Paths: openapi3.NewPaths()}
		pi := &openapi3.PathItem{
			Post: &openapi3.Operation{}, // no RequestBody
		}
		doc.Paths.Set("/tasks", pi)
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPost)
		assert.Nil(t, result)
	})

	t.Run("valid JSON body returns normalized to original mapping", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{
			"firstName": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
			"lastName": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPost)
		assert.NotNil(t, result)
		assert.Equal(t, "firstName", result["first-name"])
		assert.Equal(t, "lastName", result["last-name"])
	})

	t.Run("no properties returns nil (resolveRequestBodySchema returns nil)", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{})
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPost)
		assert.Nil(t, result)
	})

	t.Run("PUT method works", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPut, openapi3.Schemas{
			"taskId": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPut)
		assert.NotNil(t, result)
		assert.Equal(t, "taskId", result["task-id"])
	})

	t.Run("camelCase with multiple words normalizes correctly", func(t *testing.T) {
		doc := makeJSONBodyDoc("/users", http.MethodPost, openapi3.Schemas{
			"createdAtTimestamp": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "/users", http.MethodPost)
		assert.NotNil(t, result)
		assert.Equal(t, "createdAtTimestamp", result["created-at-timestamp"])
	})

	t.Run("already-lowercase property preserved", func(t *testing.T) {
		doc := makeJSONBodyDoc("/tasks", http.MethodPost, openapi3.Schemas{
			"name": &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
			},
		})
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPost)
		assert.NotNil(t, result)
		assert.Equal(t, "name", result["name"])
	})

	t.Run("requestBody with nil value returns nil", func(t *testing.T) {
		doc := &openapi3.T{Paths: openapi3.NewPaths()}
		pi := &openapi3.PathItem{
			Post: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: nil,
				},
			},
		}
		doc.Paths.Set("/tasks", pi)
		result := extractBodyPropertyNames(doc, "/tasks", http.MethodPost)
		assert.Nil(t, result)
	})
}

// ── generate --list / --clear / overwrite warning ────────────────

func TestGenerate_ListFlag(t *testing.T) {
	t.Run("--list with no store prints no commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("list", "true")

		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	t.Run("--list after generate shows store info", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{PersistedCommandsPath: storePath}
		cmd := newGenerateCommand(cfg)
		err = cmd.RunE(cmd, []string{specPath})
		require.NoError(t, err)

		// Now --list
		cmd2 := newGenerateCommand(cfg)
		_ = cmd2.Flags().Set("list", "true")
		err = cmd2.RunE(cmd2, nil)
		require.NoError(t, err)
	})
}

func TestGenerate_ClearFlag(t *testing.T) {
	t.Run("--clear removes store", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{PersistedCommandsPath: storePath}
		cmd := newGenerateCommand(cfg)
		err = cmd.RunE(cmd, []string{specPath})
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(storePath)
		require.NoError(t, err)

		// --clear
		cmd2 := newGenerateCommand(cfg)
		_ = cmd2.Flags().Set("clear", "true")
		err = cmd2.RunE(cmd2, nil)
		require.NoError(t, err)

		// File should be gone
		_, err = os.Stat(storePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("--clear with no store is a no-op", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("clear", "true")

		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})
}

func TestGenerate_OverwriteWarning(t *testing.T) {
	t.Run("second generate with different spec prints warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")

		// Spec A
		specAPath := filepath.Join(tmpDir, "specA.yaml")
		specAContent := []byte(`
openapi: 3.0.0
info:
  title: Spec A
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specAPath, specAContent, 0644)
		require.NoError(t, err)

		// Spec B (different content → different hash)
		specBPath := filepath.Join(tmpDir, "specB.yaml")
		specBContent := []byte(`
openapi: 3.0.0
info:
  title: Spec B
  version: 1.0.0
paths:
  /gadgets:
    get:
      operationId: getGadgets
      responses:
        '200':
          description: OK
`)
		err = os.WriteFile(specBPath, specBContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{PersistedCommandsPath: storePath}

		// First generate: spec A
		cmdA := newGenerateCommand(cfg)
		err = cmdA.RunE(cmdA, []string{specAPath})
		require.NoError(t, err)

		// Second generate: spec B — should warn
		cmdB := newGenerateCommand(cfg)
		// Capture stderr
		origStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		err = cmdB.RunE(cmdB, []string{specBPath})
		w.Close()
		os.Stderr = origStderr
		require.NoError(t, err)

		// Read captured stderr
		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)
		assert.Contains(t, stderrOutput, "Warning: overwriting persisted commands from a different spec")
	})

	t.Run("second generate with same spec does not warn", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")

		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := []byte(`
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      responses:
        '200':
          description: OK
`)
		err := os.WriteFile(specPath, specContent, 0644)
		require.NoError(t, err)

		cfg := &config.Config{PersistedCommandsPath: storePath}

		// First generate
		cmd1 := newGenerateCommand(cfg)
		err = cmd1.RunE(cmd1, []string{specPath})
		require.NoError(t, err)

		// Second generate with same spec — no warning
		cmd2 := newGenerateCommand(cfg)
		origStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		err = cmd2.RunE(cmd2, []string{specPath})
		w.Close()
		os.Stderr = origStderr
		require.NoError(t, err)

		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)
		assert.NotContains(t, stderrOutput, "Warning: overwriting")
	})
}

func TestGenerate_ListClearArgValidation(t *testing.T) {
	t.Run("--list accepts no args", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("list", "true")

		err := cmd.Args(cmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("--list rejects extra args", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("list", "true")

		err := cmd.Args(cmd, []string{"extra"})
		assert.Error(t, err)
	})

	t.Run("--clear accepts no args", func(t *testing.T) {
		cfg := &config.Config{}
		cmd := newGenerateCommand(cfg)
		_ = cmd.Flags().Set("clear", "true")

		err := cmd.Args(cmd, []string{})
		assert.NoError(t, err)
	})
}

func TestWarnOnOverwrite(t *testing.T) {
	t.Run("no existing store returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "nonexistent.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}
		metadata := &cmdstore.StoredData{SpecHash: "newhash"}

		err := warnOnOverwrite(metadata, cfg)
		assert.NoError(t, err)
	})

	t.Run("same hash returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}

		// Save with hash "abc"
		store := cmdstore.NewCommandStore(storePath)
		_ = store.Save(&cmdstore.StoredData{
			SpecHash: "abc",
			Commands: []cmdstore.CommandMetadata{{Name: "test", Method: "GET", Path: "/t"}},
		})

		metadata := &cmdstore.StoredData{SpecHash: "abc"}
		err := warnOnOverwrite(metadata, cfg)
		assert.NoError(t, err)
	})
}

func TestListPersistedStore(t *testing.T) {
	t.Run("empty store prints no commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}

		err := listPersistedStore(cfg)
		assert.NoError(t, err)
	})
}

func TestClearPersistedStore(t *testing.T) {
	t.Run("clear removes file", func(t *testing.T) {
		tmpDir := t.TempDir()
		storePath := filepath.Join(tmpDir, "commands.json")
		cfg := &config.Config{PersistedCommandsPath: storePath}

		// Create store
		store := cmdstore.NewCommandStore(storePath)
		_ = store.Save(&cmdstore.StoredData{
			Commands: []cmdstore.CommandMetadata{{Name: "test", Method: "GET", Path: "/t"}},
		})

		_, err := os.Stat(storePath)
		require.NoError(t, err)

		err = clearPersistedStore(cfg)
		assert.NoError(t, err)

		_, err = os.Stat(storePath)
		assert.True(t, os.IsNotExist(err))
	})
}

// ---------------------------------------------------------------------------
// DF-007: metadata extraction must use the exact operation metadata stamped
// by the generator, not name-based inference (which produced doubled-s
// paths, truncated names, and wrong base URLs on operationId-less specs).
// ---------------------------------------------------------------------------

// taskForgeNoIDSpecJSON is a compact stand-in for
// demo/services/task-forge/openapi.json: zero operationIds, nested
// resources, and a spec server URL.
const taskForgeNoIDSpecJSON = `{
  "openapi": "3.0.3",
  "info": {"title": "Task Forge API", "version": "1.0.0"},
  "servers": [{"url": "http://localhost:9801"}],
  "paths": {
    "/health": {"get": {"summary": "Health check", "responses": {"200": {"description": "Healthy"}}}},
    "/projects": {
      "get": {"summary": "List all projects", "responses": {"200": {"description": "Project list"}}},
      "post": {"summary": "Create project", "responses": {"201": {"description": "Created"}}}
    },
    "/projects/{project_id}": {
      "get": {"summary": "Get project", "responses": {"200": {"description": "Project"}}},
      "patch": {"summary": "Update project (partial)", "responses": {"200": {"description": "Updated"}}},
      "delete": {"summary": "Delete project", "responses": {"200": {"description": "Deleted"}}}
    },
    "/projects/{project_id}/tasks": {
      "get": {"summary": "List tasks in project", "responses": {"200": {"description": "Task list"}}},
      "post": {"summary": "Create task in project", "responses": {"201": {"description": "Created"}}}
    },
    "/projects/{project_id}/milestones": {
      "get": {"summary": "List milestones", "responses": {"200": {"description": "Milestone list"}}},
      "post": {"summary": "Create milestone", "responses": {"201": {"description": "Created"}}}
    },
    "/tasks": {"get": {"summary": "List all tasks", "responses": {"200": {"description": "Task list"}}}},
    "/tasks/{task_id}": {
      "get": {"summary": "Get task", "responses": {"200": {"description": "Task"}}},
      "patch": {"summary": "Update task (partial)", "responses": {"200": {"description": "Updated"}}},
      "put": {"summary": "Update task (full)", "responses": {"200": {"description": "Updated"}}},
      "delete": {"summary": "Delete task", "responses": {"200": {"description": "Deleted"}}}
    },
    "/tasks/{task_id}/comments": {
      "get": {"summary": "List comments on task", "responses": {"200": {"description": "Comment list"}}},
      "post": {"summary": "Add comment to task", "responses": {"201": {"description": "Created"}}}
    },
    "/comments/{comment_id}": {"delete": {"summary": "Delete comment", "responses": {"200": {"description": "Deleted"}}}},
    "/milestones/{milestone_id}": {
      "get": {"summary": "Get milestone", "responses": {"200": {"description": "Milestone"}}},
      "delete": {"summary": "Delete milestone", "responses": {"200": {"description": "Deleted"}}}
    },
    "/tags": {
      "get": {"summary": "List all tags", "responses": {"200": {"description": "Tag list"}}},
      "post": {"summary": "Create tag", "responses": {"201": {"description": "Created"}}}
    }
  }
}`

func TestCommandsToMetadata_AnnotationExactPaths(t *testing.T) {
	specJSON := []byte(taskForgeNoIDSpecJSON)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(specJSON)
	require.NoError(t, err)

	gen, err := generator.NewGenerator(generator.Config{
		AppName: "openapi-cli",
		BaseURL: "https://api.example.com",
	})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)

	// generate passes its --base-url default; use the legacy placeholder to
	// prove per-command BaseURLs come from the spec, not the fallback.
	metadata, err := commandsToMetadata(commands, "spec.json", "https://api.example.com", doc, specJSON)
	require.NoError(t, err)
	require.Len(t, metadata.Commands, 22)

	// Every operation in the spec is present with its EXACT path+method.
	expected := make(map[string]bool)
	for p, pathItem := range doc.Paths.Map() {
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
			if pathItem.GetOperation(method) != nil {
				expected[method+" "+p] = true
			}
		}
	}
	require.Len(t, expected, 22)

	for _, cmd := range metadata.Commands {
		key := cmd.Method + " " + cmd.Path
		require.True(t, expected[key],
			"stored command %q has inferred path %s (not an exact spec operation)", cmd.Name, key)
		// BaseURL must come from the spec's servers[].url, never the
		// api.example.com placeholder.
		require.Equal(t, "http://localhost:9801", cmd.BaseURL,
			"command %q has wrong base URL", cmd.Name)
	}

	// Path parameters must be the spec's exact names (previously inferred
	// as {projectId}/{milestonId} etc.).
	var getProject *cmdstore.CommandMetadata
	for i := range metadata.Commands {
		if metadata.Commands[i].Name == "get-project" && metadata.Commands[i].Method == "GET" {
			getProject = &metadata.Commands[i]
		}
	}
	require.NotNil(t, getProject)
	require.Equal(t, []string{"project_id"}, getProject.Args)

	// No mangled paths anywhere in the persisted store (DF-007): the exact
	// doubled-s (/projectss) and truncated (/milestons/{milestonId}) forms
	// must never appear.
	for _, cmd := range metadata.Commands {
		require.NotContains(t, cmd.Path, "ss", "doubled-s path on %q", cmd.Name)
		require.NotContains(t, cmd.Path, "/milestons", "truncated path on %q", cmd.Name)
		require.NotContains(t, cmd.Path, "milestonId", "truncated param on %q", cmd.Name)
	}
}

func TestDefaultBaseURLFromSpec(t *testing.T) {
	t.Run("absolute server URL wins", func(t *testing.T) {
		doc := &openapi3.T{Servers: openapi3.Servers{{URL: "http://localhost:9801"}}}
		require.Equal(t, "http://localhost:9801", defaultBaseURLFromSpec(doc))
	})

	t.Run("relative server URL falls back to placeholder", func(t *testing.T) {
		doc := &openapi3.T{Servers: openapi3.Servers{{URL: "/api"}}}
		require.Equal(t, "https://api.example.com", defaultBaseURLFromSpec(doc))
	})

	t.Run("no servers falls back to placeholder", func(t *testing.T) {
		require.Equal(t, "https://api.example.com", defaultBaseURLFromSpec(&openapi3.T{}))
		require.Equal(t, "https://api.example.com", defaultBaseURLFromSpec(nil))
	})
}
