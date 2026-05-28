package generator

import (
	"net/http"
	"testing"

	"github.com/wojons/muster/pkg/openapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestBuildResourceCommand(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	ops := []*openapi.Operation{
		{
			ID:     "listUsers",
			Path:   "/users",
			Method: "GET",
		},
		{
			ID:     "createUser",
			Path:   "/users",
			Method: "POST",
		},
	}

	cmd := gen.buildResourceCommand("users", ops)

	require.NotNil(t, cmd)
	require.Equal(t, "users", cmd.Use)
	require.NotNil(t, cmd.Short)
}

func TestBuildOperationCommand(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:          "test",
		BaseURL:          "test",
		DefaultFormat:    "json",
		SupportedFormats: []string{"json", "yaml"},
	})

	op := &openapi.Operation{
		ID:         "listUsers",
		Path:       "/users",
		Method:     "GET",
		Summary:    "List all users",
		Parameters: openapi.Parameters{},
	}

	cmd := gen.buildOperationCommand(op)

	require.NotNil(t, cmd)
	require.Equal(t, "List all users", cmd.Short)
	require.NotNil(t, cmd.RunE)
}

func TestBuildLongDescription(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("with description", func(t *testing.T) {
		op := &openapi.Operation{
			Method:      "GET",
			Path:        "/users",
			Description: "This is a description",
			Parameters:  openapi.Parameters{},
		}

		result := gen.buildLongDescription(op)
		require.Contains(t, result, "This is a description")
		require.Contains(t, result, "API Endpoint:")
		require.Contains(t, result, "GET /users")
	})

	t.Run("with parameters", func(t *testing.T) {
		op := &openapi.Operation{
			Method: "GET",
			Path:   "/users",
			Parameters: openapi.Parameters{
				&openapi.Parameter{
					Name:        "id",
					Description: "User ID",
					Required:    true,
				},
			},
		}

		result := gen.buildLongDescription(op)
		require.Contains(t, result, "Parameters:")
		require.Contains(t, result, "id (required)")
	})
}

func TestBuildExample(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("basic example", func(t *testing.T) {
		op := &openapi.Operation{
			Method: http.MethodGet,
			Path:   "/users/{id}",
			ID:     "getUser",
			Parameters: openapi.Parameters{
				&openapi.Parameter{
					Name:     "id",
					In:       "path",
					Required: true,
				},
			},
		}

		result := gen.buildExample("get", op)
		require.Contains(t, result, "muster users get <id>")
	})

	t.Run("with query params", func(t *testing.T) {
		op := &openapi.Operation{
			Method: http.MethodGet,
			Path:   "/users",
			ID:     "listUsers",
			Parameters: openapi.Parameters{
				&openapi.Parameter{
					Name: "filter",
					In:   "query",
				},
			},
		}

		result := gen.buildExample("list", op)
		require.Contains(t, result, "filters")
	})
}

func TestInferResourceName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		path     string
		expected string
	}{
		{"/users/{id}", "users"},
		{"/api/v1/posts", "api"},
		{"/users", "users"},
		{"/{id}", "resource"},
		{"/", "resource"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			op := &openapi.Operation{
				Path: tc.path,
			}
			result := gen.inferResourceName(op)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestHasQueryParams(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("has query params", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: openapi.Parameters{
				&openapi.Parameter{Name: "filter", In: "query"},
			},
		}
		require.True(t, gen.hasQueryParams(op))
	})

	t.Run("no query params", func(t *testing.T) {
		op := &openapi.Operation{
			Parameters: openapi.Parameters{
				&openapi.Parameter{Name: "id", In: "path"},
			},
		}
		require.False(t, gen.hasQueryParams(op))
	})
}

func TestAddFlags(t *testing.T) {
	gen, _ := NewGenerator(Config{
		AppName:          "test",
		BaseURL:          "test",
		DefaultFormat:    "json",
		SupportedFormats: []string{"json", "yaml"},
	})

	op := &openapi.Operation{
		Parameters: openapi.Parameters{
			&openapi.Parameter{
				Name: "filter",
				In:   "query",
				Type: "string",
			},
			&openapi.Parameter{
				Name: "limit",
				In:   "query",
				Type: "integer",
			},
			&openapi.Parameter{
				Name: "verbose",
				In:   "query",
				Type: "boolean",
			},
		},
	}

	cmd := &cobra.Command{Use: "test"}
	gen.addFlags(cmd, op)

	require.True(t, cmd.Flags().HasFlags())
}

func TestAddStringFlag(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("with enum", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		param := &openapi.Parameter{
			Name: "status",
			Type: "string",
			Enum: []interface{}{"active", "inactive"},
		}

		gen.addStringFlag(cmd, "status", "s", param, "Status")

		flag := cmd.Flags().Lookup("status")
		require.NotNil(t, flag)
		require.Contains(t, flag.Usage, "allowed")
	})

	t.Run("with default", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		param := &openapi.Parameter{
			Name:    "format",
			Type:    "string",
			Default: "json",
		}

		gen.addStringFlag(cmd, "format", "f", param, "Output format")

		flag := cmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		require.Equal(t, "json", flag.DefValue)
	})
}

func TestExtractResource(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	t.Run("from tags", func(t *testing.T) {
		op := &openapi.Operation{
			Tags: []string{"users"},
		}
		result := gen.extractResource(op)
		require.Equal(t, "users", result)
	})

	t.Run("from path", func(t *testing.T) {
		op := &openapi.Operation{
			Path: "/api/v1/users",
		}
		result := gen.extractResource(op)
		require.Equal(t, "api", result)
	})
}

func TestGetCommandName(t *testing.T) {
	t.Run("from custom mapping", func(t *testing.T) {
		config := Config{
			AppName:        "test",
			BaseURL:        "test",
			CustomMappings: map[string]string{"getUser": "fetch"},
		}
		gen, _ := NewGenerator(config)

		op := &openapi.Operation{
			ID: "getUser",
		}
		result := gen.getCommandName(op)
		require.Equal(t, "fetch", result)
	})

	t.Run("from operation ID", func(t *testing.T) {
		gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

		op := &openapi.Operation{
			ID: "listAllUsers",
		}
		result := gen.getCommandName(op)
		require.Contains(t, result, "list")
	})
}

func TestOperationName(t *testing.T) {
	t.Run("http style", func(t *testing.T) {
		config := Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleHTTP,
		}
		gen, _ := NewGenerator(config)

		result := gen.operationName(http.MethodGet, "/users")
		require.Contains(t, result, "get")
	})

	t.Run("crud style", func(t *testing.T) {
		config := Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleCRUD,
		}
		gen, _ := NewGenerator(config)

		result := gen.operationName(http.MethodGet, "/users")
		require.Contains(t, result, "list")
	})

	t.Run("flat style", func(t *testing.T) {
		config := Config{
			AppName:        "test",
			BaseURL:        "test",
			OperationStyle: StyleFlat,
		}
		gen, _ := NewGenerator(config)

		result := gen.operationName(http.MethodGet, "/users")
		require.Contains(t, result, "get")
	})
}

func TestCrudName(t *testing.T) {
	gen, _ := NewGenerator(Config{AppName: "test", BaseURL: "test"})

	tests := []struct {
		method   string
		path     string
		expected string
	}{
		{"GET", "/users", "list-users"},
		{"GET", "/users/{id}", "get-user"},
		{"POST", "/users", "create-user"},
		{"PUT", "/users/{id}", "update-user"},
		{"PATCH", "/users/{id}", "update-user"},
		{"DELETE", "/users/{id}", "delete-user"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			result := gen.crudName(tc.method, tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}
