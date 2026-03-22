package mcp

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConverter(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{
			name:    "with empty base URL",
			baseURL: "",
		},
		{
			name:    "with localhost URL",
			baseURL: "http://localhost:8080",
		},
		{
			name:    "with production URL",
			baseURL: "https://api.example.com/v1",
		},
		{
			name:    "with trailing slash",
			baseURL: "https://api.example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewConverter(tt.baseURL)
			require.NotNil(t, converter)
			assert.Equal(t, tt.baseURL, converter.baseURL)
		})
	}
}

func TestOpenAPITool_ToHandlersTool(t *testing.T) {
	tests := []struct {
		name       string
		tool       OpenAPITool
		wantName   string
		wantDesc   string
		wantSchema string
	}{
		{
			name: "basic tool conversion",
			tool: OpenAPITool{
				Name:        "list-pets",
				Description: "List all pets",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
			wantName:   "list-pets",
			wantDesc:   "List all pets",
			wantSchema: `{"type":"object"}`,
		},
		{
			name: "tool with complex schema",
			tool: OpenAPITool{
				Name:        "create-pet",
				Description: "Create a new pet",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
			},
			wantName:   "create-pet",
			wantDesc:   "Create a new pet",
			wantSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerTool := tt.tool.ToHandlersTool()
			assert.Equal(t, tt.wantName, handlerTool.Name)
			assert.Equal(t, tt.wantDesc, handlerTool.Description)
			assert.JSONEq(t, tt.wantSchema, string(handlerTool.InputSchema))
		})
	}
}

func TestConverter_ConvertToTools_EmptySpec(t *testing.T) {
	converter := NewConverter("http://localhost:8080")
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Empty API",
			Version: "1.0.0",
		},
		Paths: openapi3.NewPaths(),
	}

	tools, configs, err := converter.ConvertToTools(doc)
	require.NoError(t, err)
	assert.Empty(t, tools)
	assert.Empty(t, configs)
}

func TestConverter_ConvertToTools_SingleOperation(t *testing.T) {
	converter := NewConverter("http://localhost:8080")
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: func() *openapi3.Paths {
			paths := openapi3.NewPaths()
			paths.Set("/test", &openapi3.PathItem{
				Get: &openapi3.Operation{
					OperationID: "simpleTest",
					Summary:     "Simple test endpoint",
				},
			})
			return paths
		}(),
	}

	tools, configs, err := converter.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Len(t, configs, 1)

	assert.Equal(t, "simple-Test", tools[0].Name) // toKebabCase preserves case
	assert.Equal(t, "Simple test endpoint", tools[0].Description)
	// InputSchema may be nil for operations without parameters

	config, ok := configs["simple-Test"] // toKebabCase preserves case
	require.True(t, ok)
	assert.Equal(t, "/test", config.Path)
	assert.Equal(t, "GET", config.Method)
	assert.Equal(t, "http://localhost:8080", config.BaseURL)
}

func TestConverter_ConvertToTools_WithRequestBody(t *testing.T) {
	converter := NewConverter("http://localhost:8080")
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "API with Request Body",
			Version: "1.0.0",
		},
		Paths: func() *openapi3.Paths {
			paths := openapi3.NewPaths()
			paths.Set("/pets", &openapi3.PathItem{
				Post: &openapi3.Operation{
					OperationID: "createPet",
					Summary:     "Create a new pet",
					RequestBody: &openapi3.RequestBodyRef{
						Value: &openapi3.RequestBody{
							Required: true,
							Content: openapi3.Content{
								"application/json": &openapi3.MediaType{
									Schema: &openapi3.SchemaRef{
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"object"},
										},
									},
								},
							},
						},
					},
				},
			})
			return paths
		}(),
	}

	tools, configs, err := converter.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Len(t, configs, 1)

	// Verify body parameter is added
	var schema map[string]interface{}
	err = json.Unmarshal(tools[0].InputSchema, &schema)
	require.NoError(t, err)

	properties := schema["properties"].(map[string]interface{})
	assert.Contains(t, properties, "body")

	required := schema["required"].([]interface{})
	assert.Contains(t, required, "body")

	// Verify config includes the input schema
	config := configs["create-Pet"] // toKebabCase preserves case
	assert.NotNil(t, config.InputSchema)
}

func TestConverter_getToolName(t *testing.T) {
	converter := NewConverter("http://localhost:8080")

	tests := []struct {
		name      string
		operation *openapi3.Operation
		path      string
		method    string
		want      string
	}{
		{
			name: "operationID takes precedence",
			operation: &openapi3.Operation{
				OperationID: "getUserById",
			},
			path:   "/users/{id}",
			method: "GET",
			want:   "get-User-By-Id", // Note: toKebabCase preserves case
		},
		{
			name: "generates from path when no operationID",
			operation: &openapi3.Operation{
				Summary: "Get all users",
			},
			path:   "/users",
			method: "GET",
			want:   "get-users",
		},
		{
			name: "handles nested paths - stops at first parameter",
			operation: &openapi3.Operation{
				Summary: "Create order",
			},
			path:   "/users/{id}/orders",
			method: "POST",
			want:   "create-resource", // extractResource stops at first {, returns resource if empty
		},
		{
			name: "handles path parameters",
			operation: &openapi3.Operation{
				Summary: "Update item",
			},
			path:   "/items/{itemId}",
			method: "PUT",
			want:   "update-resource", // extractResource stops at {, returns resource if empty
		},
		{
			name: "handles root path",
			operation: &openapi3.Operation{
				Summary: "Health check",
			},
			path:   "/",
			method: "GET",
			want:   "get-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.getToolName(tt.operation, tt.path, tt.method)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConverter_getToolDescription(t *testing.T) {
	converter := NewConverter("http://localhost:8080")

	tests := []struct {
		name      string
		operation *openapi3.Operation
		want      string
	}{
		{
			name: "summary takes precedence",
			operation: &openapi3.Operation{
				Summary:     "Quick summary",
				Description: "This is a longer description that should not be used",
			},
			want: "Quick summary",
		},
		{
			name: "falls back to description",
			operation: &openapi3.Operation{
				Description: "Only description available",
			},
			want: "Only description available",
		},
		{
			name: "default when neither present",
			operation: &openapi3.Operation{
				OperationID: "someOperation",
			},
			want: "API operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.getToolDescription(tt.operation)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConverter_getParameterType(t *testing.T) {
	converter := NewConverter("http://localhost:8080")

	tests := []struct {
		name  string
		param *openapi3.Parameter
		want  string
	}{
		{
			name: "returns string type",
			param: &openapi3.Parameter{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			want: "string",
		},
		{
			name: "returns integer type",
			param: &openapi3.Parameter{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
			},
			want: "integer",
		},
		{
			name: "returns boolean type",
			param: &openapi3.Parameter{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"boolean"},
					},
				},
			},
			want: "boolean",
		},
		{
			name: "defaults to string when no schema",
			param: &openapi3.Parameter{
				Name: "test",
			},
			want: "string",
		},
		{
			name: "defaults to string when empty type slice",
			param: &openapi3.Parameter{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{},
					},
				},
			},
			want: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.getParameterType(tt.param)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConverter_extractResource(t *testing.T) {
	converter := NewConverter("http://localhost:8080")

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "simple resource",
			path: "/users",
			want: "users",
		},
		{
			name: "resource with parameter - stops at parameter",
			path: "/users/{id}",
			want: "resource", // extractResource stops at {, returns resource if empty after truncation
		},
		{
			name: "nested resource with parameter",
			path: "/users/{id}/orders",
			want: "resource", // stops at first {, returns resource
		},
		{
			name: "deep nesting with parameters",
			path: "/api/v1/users/{id}/orders/{orderId}/items",
			want: "resource", // stops at first {, returns resource
		},
		{
			name: "root path",
			path: "/",
			want: "resource",
		},
		{
			name: "just parameter",
			path: "/{id}",
			want: "resource",
		},
		{
			name: "empty path returns resource",
			path: "",
			want: "resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.extractResource(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConverter_getAction(t *testing.T) {
	converter := NewConverter("http://localhost:8080")

	tests := []struct {
		name   string
		method string
		want   string
	}{
		{"GET returns get", "GET", "get"},
		{"POST returns create", "POST", "create"},
		{"PUT returns update", "PUT", "update"},
		{"PATCH returns patch", "PATCH", "patch"},
		{"DELETE returns delete", "DELETE", "delete"},
		{"HEAD returns HEAD", "HEAD", "HEAD"},
		{"OPTIONS returns OPTIONS", "OPTIONS", "OPTIONS"},
		{"UNKNOWN returns UNKNOWN", "UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converter.getAction(tt.method)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "CamelCase with multiple words",
			input: "GetUserById",
			want:  "Get-User-By-Id",
		},
		{
			name:  "single word",
			input: "Users",
			want:  "Users",
		},
		{
			name:  "all lowercase",
			input: "users",
			want:  "users",
		},
		{
			name:  "already kebab-case",
			input: "get-user-by-id",
			want:  "get-user-by-id",
		},
		{
			name:  "consecutive capitals",
			input: "GetURLPath",
			want:  "Get-U-R-L-Path",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "numbers in name",
			input: "GetUserV2Info",
			want:  "Get-User-V2-Info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toKebabCase(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConverter_ConvertToTools_RefsIgnored(t *testing.T) {
	converter := NewConverter("http://localhost:8080")
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "API with Refs",
			Version: "1.0.0",
		},
		Paths: func() *openapi3.Paths {
			paths := openapi3.NewPaths()
			paths.Set("/pets", &openapi3.PathItem{
				Get: &openapi3.Operation{
					OperationID: "listPets",
					Summary:     "List pets",
					Parameters: openapi3.Parameters{
						{
							Ref: "#/components/parameters/limit", // This should be ignored
						},
						{
							Value: &openapi3.Parameter{
								Name: "valid",
								In:   "query",
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"string"},
									},
								},
							},
						},
					},
				},
			})
			return paths
		}(),
	}

	tools, _, err := converter.ConvertToTools(doc)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	// Verify that only the non-ref parameter is included
	var schema map[string]interface{}
	err = json.Unmarshal(tools[0].InputSchema, &schema)
	require.NoError(t, err)

	properties := schema["properties"].(map[string]interface{})
	assert.Contains(t, properties, "valid")
	assert.NotContains(t, properties, "limit")
}
