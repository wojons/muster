package openapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Helper functions
// ============================================================================

func mustLoadDoc(t *testing.T, data string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(data))
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

// ============================================================================
// 1. NewParser — returns non-nil
// ============================================================================

func TestNewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p)
}

// ============================================================================
// 2. DefaultParseOptions — verify defaults
// ============================================================================

func TestDefaultParseOptions(t *testing.T) {
	opts := DefaultParseOptions()
	assert.False(t, opts.SkipValidation, "SkipValidation should default to false")
	assert.Equal(t, 10, opts.MaxDepth, "MaxDepth should default to 10")
}

// ============================================================================
// 3. Parse — valid minimal OpenAPI doc (openapi: 3.0.3, info, paths with one GET)
// ============================================================================

func TestParse_ValidMinimal(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/items": {
				"get": {
					"operationId": "listItems",
					"responses": {
						"200": {
							"description": "OK"
						}
					}
				}
			}
		}
	}`

	p := NewParser()
	result, err := p.Parse(context.Background(), []byte(doc), DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "1.0.0", result.Version)
	assert.NotNil(t, result.Document)
	assert.Equal(t, "3.0.3", result.Document.OpenAPI)
}

// ============================================================================
// 4. Parse — missing info section → error
// ============================================================================

func TestParse_MissingInfo(t *testing.T) {
	// Valid JSON but no "info" key
	doc := `{
		"openapi": "3.0.3",
		"paths": {}
	}`

	p := NewParser()
	_, err := p.Parse(context.Background(), []byte(doc), DefaultParseOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing info")
}

// ============================================================================
// 5. Parse — invalid JSON → error
// ============================================================================

func TestParse_InvalidJSON(t *testing.T) {
	doc := `{this is not valid json}`

	p := NewParser()
	_, err := p.Parse(context.Background(), []byte(doc), DefaultParseOptions())
	require.Error(t, err)
}

// ============================================================================
// 6. Parse — with SkipValidation=true
// ============================================================================

func TestParse_SkipValidation(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": {
			"title": "Test API",
			"version": "2.0.0"
		},
		"paths": {
			"/items": {
				"get": {
					"operationId": "skipValidate",
					"responses": {
						"200": {
							"description": "OK"
						}
					}
				}
			}
		}
	}`

	p := NewParser()
	opts := ParseOptions{SkipValidation: true, MaxDepth: 10}
	result, err := p.Parse(context.Background(), []byte(doc), opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "2.0.0", result.Version)
}

// ============================================================================
// 7. ExtractOperations — nil spec → nil, nil
// ============================================================================

func TestExtractOperations_NilSpec(t *testing.T) {
	p := NewParser()
	ops, err := p.ExtractOperations(nil)
	require.NoError(t, err)
	assert.Nil(t, ops)
}

// ============================================================================
// 8. ExtractOperations — nil Paths → nil, nil
// ============================================================================

func TestExtractOperations_NilPaths(t *testing.T) {
	p := NewParser()
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		// Paths is nil
	}
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	assert.Nil(t, ops)
}

// ============================================================================
// 9. ExtractOperations — GET, POST, PUT, DELETE, PATCH
// ============================================================================

func TestExtractOperations_MultipleMethods(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "MultiMethod", "version": "1.0" },
		"paths": {
			"/resource": {
				"get": {
					"operationId": "getResource",
					"responses": { "200": { "description": "OK" } }
				},
				"post": {
					"operationId": "createResource",
					"responses": { "201": { "description": "Created" } }
				},
				"put": {
					"operationId": "updateResource",
					"responses": { "200": { "description": "OK" } }
				},
				"delete": {
					"operationId": "deleteResource",
					"responses": { "204": { "description": "No Content" } }
				},
				"patch": {
					"operationId": "patchResource",
					"responses": { "200": { "description": "OK" } }
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 5)

	methods := make(map[string]string)
	for _, op := range ops {
		methods[op.Method] = op.ID
	}
	assert.Equal(t, "getResource", methods["GET"])
	assert.Equal(t, "createResource", methods["POST"])
	assert.Equal(t, "updateResource", methods["PUT"])
	assert.Equal(t, "deleteResource", methods["DELETE"])
	assert.Equal(t, "patchResource", methods["PATCH"])

	// All point to the same path
	for _, op := range ops {
		assert.Equal(t, "/resource", op.Path)
	}
}

// ============================================================================
// 10. ExtractOperations — multiple paths, multiple methods per path
// ============================================================================

func TestExtractOperations_MultiplePaths(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "MultiPath", "version": "1.0" },
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"responses": { "200": { "description": "OK" } }
				},
				"post": {
					"operationId": "createUser",
					"responses": { "201": { "description": "Created" } }
				}
			},
			"/users/{id}": {
				"get": {
					"operationId": "getUser",
					"responses": { "200": { "description": "OK" } }
				},
				"delete": {
					"operationId": "deleteUser",
					"responses": { "204": { "description": "No Content" } }
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 4)

	pathOps := make(map[string]map[string]bool)
	for _, op := range ops {
		if pathOps[op.Path] == nil {
			pathOps[op.Path] = make(map[string]bool)
		}
		pathOps[op.Path][op.Method] = true
	}

	assert.True(t, pathOps["/users"]["GET"])
	assert.True(t, pathOps["/users"]["POST"])
	assert.True(t, pathOps["/users/{id}"]["GET"])
	assert.True(t, pathOps["/users/{id}"]["DELETE"])
}

// ============================================================================
// 11. ExtractOperations — operation with tags, summary, description
// ============================================================================

func TestExtractOperations_TagsSummaryDescription(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "Meta", "version": "1.0" },
		"paths": {
			"/items": {
				"get": {
					"operationId": "getItems",
					"tags": ["items", "catalog"],
					"summary": "List all items",
					"description": "Returns a paginated list of items in the catalog.",
					"responses": { "200": { "description": "OK" } }
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	assert.Equal(t, "getItems", op.ID)
	assert.Equal(t, "GET", op.Method)
	assert.Equal(t, "/items", op.Path)
	assert.Equal(t, []string{"items", "catalog"}, op.Tags)
	assert.Equal(t, "List all items", op.Summary)
	assert.Equal(t, "Returns a paginated list of items in the catalog.", op.Description)
}

// ============================================================================
// 12. ExtractOperations — with parameters (path param, query param, header param)
// ============================================================================

func TestExtractOperations_Parameters(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "Params", "version": "1.0" },
		"paths": {
			"/items/{id}": {
				"get": {
					"operationId": "getItem",
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true,
							"schema": { "type": "integer" }
						},
						{
							"name": "page",
							"in": "query",
							"required": false,
							"schema": { "type": "integer", "default": 1 }
						},
						{
							"name": "Authorization",
							"in": "header",
							"required": true,
							"schema": { "type": "string" }
						}
					],
					"responses": { "200": { "description": "OK" } }
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	params := ops[0].Parameters
	require.Len(t, params, 3)

	// path param
	assert.Equal(t, "id", params[0].Name)
	assert.Equal(t, "path", params[0].In)
	assert.Equal(t, "integer", params[0].Type)
	assert.True(t, params[0].Required)

	// query param
	assert.Equal(t, "page", params[1].Name)
	assert.Equal(t, "query", params[1].In)
	assert.Equal(t, "integer", params[1].Type)
	assert.False(t, params[1].Required)
	// The default is stored via schema; extractParameters copies it
	assert.Equal(t, float64(1), params[1].Default)

	// header param
	assert.Equal(t, "Authorization", params[2].Name)
	assert.Equal(t, "header", params[2].In)
	assert.Equal(t, "string", params[2].Type)
	assert.True(t, params[2].Required)
}

// ============================================================================
// 13. ExtractOperations — nil pathItem skipped
// ============================================================================

func TestExtractOperations_NilPathItemSkipped(t *testing.T) {
	p := NewParser()

	paths := openapi3.NewPaths()
	paths.Set("/valid", &openapi3.PathItem{
		Get: &openapi3.Operation{
			OperationID: "validOp",
			Responses:   openapi3.NewResponses(),
		},
	})
	// Directly set a nil entry in the underlying map.
	// We need to use the unexported map, so we wrap via JSON that produces nil.
	// Instead, create a doc via JSON with a null pathItem.
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "NilPath", "version": "1.0" },
		"paths": {
			"/valid": {
				"get": {
					"operationId": "validOp",
					"responses": { "200": { "description": "OK" } }
				}
			},
			"/null-path": null
		}
	}`

	spec := mustLoadDoc(t, doc)
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	// Only the valid path should produce an operation
	require.Len(t, ops, 1)
	assert.Equal(t, "validOp", ops[0].ID)
	assert.Equal(t, "/valid", ops[0].Path)
}

// ============================================================================
// 14. ExtractOperations — empty paths map
// ============================================================================

func TestExtractOperations_EmptyPaths(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "Empty", "version": "1.0" },
		"paths": {}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	assert.Empty(t, ops)
}

// ============================================================================
// 15. resolveServerURL — operation-level server (highest priority)
// ============================================================================

func TestResolveServerURL_OperationLevel(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Servers: openapi3.Servers{
			{URL: "https://root.example.com/api"},
		},
	}
	pathItem := &openapi3.PathItem{
		Servers: openapi3.Servers{
			{URL: "https://path.example.com/api"},
		},
	}
	operation := &openapi3.Operation{
		OperationID: "opLevel",
		Servers: &openapi3.Servers{
			{URL: "https://op.example.com/api"},
		},
	}

	url := resolveServerURL(spec, pathItem, operation)
	assert.Equal(t, "https://op.example.com/api", url)
}

// ============================================================================
// 16. resolveServerURL — path-level server
// ============================================================================

func TestResolveServerURL_PathLevel(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Servers: openapi3.Servers{
			{URL: "https://root.example.com/api"},
		},
	}
	pathItem := &openapi3.PathItem{
		Servers: openapi3.Servers{
			{URL: "https://path.example.com/api"},
		},
	}
	// No operation-level server
	operation := &openapi3.Operation{
		OperationID: "pathLevel",
	}

	url := resolveServerURL(spec, pathItem, operation)
	assert.Equal(t, "https://path.example.com/api", url)
}

// ============================================================================
// 17. resolveServerURL — root-level server
// ============================================================================

func TestResolveServerURL_RootLevel(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Servers: openapi3.Servers{
			{URL: "https://root.example.com/api"},
		},
	}
	pathItem := &openapi3.PathItem{} // No path-level servers
	operation := &openapi3.Operation{
		OperationID: "rootLevel",
	}

	url := resolveServerURL(spec, pathItem, operation)
	assert.Equal(t, "https://root.example.com/api", url)
}

// ============================================================================
// 18. resolveServerURL — no servers → empty string
// ============================================================================

func TestResolveServerURL_NoServers(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		// No servers
	}
	pathItem := &openapi3.PathItem{}
	operation := &openapi3.Operation{
		OperationID: "noServer",
	}

	url := resolveServerURL(spec, pathItem, operation)
	assert.Equal(t, "", url)
}

// ============================================================================
// 19. extractParameters — empty params list → nil
// ============================================================================

func TestExtractParameters_Empty(t *testing.T) {
	result := extractParameters(openapi3.Parameters{})
	assert.Nil(t, result)
}

// ============================================================================
// 20. extractParameters — param with schema type
// ============================================================================

func TestExtractParameters_WithSchemaType(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name:     "userId",
				In:       "path",
				Required: true,
				Schema:   openapi3.NewSchemaRef("", &openapi3.Schema{Type: &openapi3.Types{"string"}}),
			},
		},
	}

	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "userId", result[0].Name)
	assert.Equal(t, "path", result[0].In)
	assert.True(t, result[0].Required)
	assert.Equal(t, "string", result[0].Type)
}

// ============================================================================
// 21. extractParameters — param with enum values
// ============================================================================

func TestExtractParameters_WithEnum(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name: "status",
				In:   "query",
				Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
					Type: &openapi3.Types{"string"},
					Enum: []interface{}{"active", "inactive", "pending"},
				}),
			},
		},
	}

	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "status", result[0].Name)
	assert.Equal(t, []interface{}{"active", "inactive", "pending"}, result[0].Enum)
}

// ============================================================================
// 22. extractParameters — param with default value
// ============================================================================

func TestExtractParameters_WithDefault(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name: "limit",
				In:   "query",
				Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
					Type:    &openapi3.Types{"integer"},
					Default: float64(20),
				}),
			},
		},
	}

	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "limit", result[0].Name)
	assert.Equal(t, float64(20), result[0].Default)
}

// ============================================================================
// 23. extractParameters — nil paramRef skipped
// ============================================================================

func TestExtractParameters_NilParamRefSkipped(t *testing.T) {
	params := openapi3.Parameters{
		nil, // nil ref should be skipped
		{
			Value: &openapi3.Parameter{
				Name: "id",
				In:   "path",
				Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
					Type: &openapi3.Types{"integer"},
				}),
			},
		},
	}

	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "id", result[0].Name)
}

// ============================================================================
// 24. extractRequestBodyProperties — nil bodyRef → nil
// ============================================================================

func TestExtractRequestBodyProperties_NilBodyRef(t *testing.T) {
	result := extractRequestBodyProperties(nil, nil)
	assert.Nil(t, result)
}

// ============================================================================
// 25. extractRequestBodyProperties — JSON content with properties
// ============================================================================

func TestExtractRequestBodyProperties_JSONContent(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
	}

	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"name": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type:        &openapi3.Types{"string"},
								Description: "User's full name",
							}),
							"age": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type:    &openapi3.Types{"integer"},
								Default: float64(0),
							}),
							"role": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type: &openapi3.Types{"string"},
								Enum: []interface{}{"admin", "user", "viewer"},
							}),
						},
						Required: []string{"name"},
					}),
				},
			},
		},
	}

	result := extractRequestBodyProperties(spec, bodyRef)
	require.Len(t, result, 3)

	// Build a lookup map
	byName := make(map[string]*RequestBodyProperty)
	for i := range result {
		byName[result[i].Name] = &result[i]
	}

	// name: required string with description
	prop, ok := byName["name"]
	require.True(t, ok, "expected property 'name'")
	assert.Equal(t, "string", prop.Type)
	assert.True(t, prop.Required)
	assert.Equal(t, "User's full name", prop.Description)

	// age: integer with default
	prop, ok = byName["age"]
	require.True(t, ok, "expected property 'age'")
	assert.Equal(t, "integer", prop.Type)
	assert.False(t, prop.Required) // not in required list
	assert.Equal(t, float64(0), prop.Default)

	// role: string with enum
	prop, ok = byName["role"]
	require.True(t, ok, "expected property 'role'")
	assert.Equal(t, "string", prop.Type)
	assert.Equal(t, []interface{}{"admin", "user", "viewer"}, prop.Enum)
}

// ============================================================================
// 26. hasMultipartBody — true for multipart/form-data
// ============================================================================

func TestHasMultipartBody_True(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
						Type: &openapi3.Types{"object"},
					}),
				},
			},
		},
	}

	assert.True(t, hasMultipartBody(bodyRef))
}

// ============================================================================
// 27. hasMultipartBody — false for non-multipart
// ============================================================================

func TestHasMultipartBody_False(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{}),
				},
			},
		},
	}

	assert.False(t, hasMultipartBody(bodyRef))
}

// Also test nil bodyRef
func TestHasMultipartBody_NilBodyRef(t *testing.T) {
	assert.False(t, hasMultipartBody(nil))
}

// ============================================================================
// 28. extractMultipartFields — skips binary fields
// ============================================================================

func TestExtractMultipartFields_SkipsBinary(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"file": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type:   &openapi3.Types{"string"},
								Format: "binary",
							}),
							"name": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type: &openapi3.Types{"string"},
							}),
							"count": openapi3.NewSchemaRef("", &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
							}),
						},
					}),
				},
			},
		},
	}

	result := extractMultipartFields(bodyRef)
	require.Len(t, result, 2)

	byName := make(map[string]bool)
	for _, p := range result {
		byName[p.Name] = true
	}

	assert.True(t, byName["name"], "should include non-binary 'name' field")
	assert.True(t, byName["count"], "should include non-binary 'count' field")
	assert.False(t, byName["file"], "should skip binary 'file' field")
}

// Also test nil bodyRef
func TestExtractMultipartFields_NilBodyRef(t *testing.T) {
	result := extractMultipartFields(nil)
	assert.Nil(t, result)
}

// ============================================================================
// 29. SchemaToJSON — empty props → valid JSON
// ============================================================================

func TestSchemaToJSON_EmptyProps(t *testing.T) {
	data, err := SchemaToJSON(nil)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "object", result["type"])

	props, ok := result["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, props)

	// No required field for empty props
	_, hasRequired := result["required"]
	assert.False(t, hasRequired)
}

// ============================================================================
// 30. SchemaToJSON — props with types, required, enum, default
// ============================================================================

func TestSchemaToJSON_WithProps(t *testing.T) {
	props := []RequestBodyProperty{
		{
			Name:        "title",
			Type:        "string",
			Description: "The title",
			Required:    true,
		},
		{
			Name:     "count",
			Type:     "integer",
			Required: false,
			Default:  float64(0),
		},
		{
			Name:  "status",
			Type:  "string",
			Enum:  []interface{}{"draft", "published", "archived"},
			Default: "draft",
		},
		{
			Name:     "active",
			Type:     "boolean",
			Required: true,
		},
	}

	data, err := SchemaToJSON(props)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "object", result["type"])

	propsMap, ok := result["properties"].(map[string]interface{})
	require.True(t, ok)

	// title: string with description, required
	titleProp := propsMap["title"].(map[string]interface{})
	assert.Equal(t, "string", titleProp["type"])
	assert.Equal(t, "The title", titleProp["description"])

	// count: integer with default
	countProp := propsMap["count"].(map[string]interface{})
	assert.Equal(t, "integer", countProp["type"])
	assert.Equal(t, float64(0), countProp["default"])

	// status: string with enum and default
	statusProp := propsMap["status"].(map[string]interface{})
	assert.Equal(t, "string", statusProp["type"])
	assert.Equal(t, []interface{}{"draft", "published", "archived"}, statusProp["enum"])
	assert.Equal(t, "draft", statusProp["default"])

	// active: boolean, required
	activeProp := propsMap["active"].(map[string]interface{})
	assert.Equal(t, "boolean", activeProp["type"])

	// required array
	required, ok := result["required"].([]interface{})
	require.True(t, ok)
	assert.ElementsMatch(t, []interface{}{"title", "active"}, required)
}

// ============================================================================
// 31. typeToJSONSchemaType — all types
// ============================================================================

func TestTypeToJSONSchemaType_AllTypes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"string", "string"},
		{"integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"array", "array"},
		{"object", "object"},
		// Unknown types should default to "string"
		{"unknown", "string"},
		{"", "string"},
		{"float", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := typeToJSONSchemaType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Additional edge cases
// ============================================================================

// TestExtractOperations_NilOperations ensures nil operations within a pathItem are skipped.
func TestExtractOperations_NilOperationsSkipped(t *testing.T) {
	p := NewParser()
	paths := openapi3.NewPaths()
	paths.Set("/test", &openapi3.PathItem{
		Get:  nil,  // skipped
		Post: nil,  // skipped
		Put: &openapi3.Operation{
			OperationID: "updateTest",
			Responses:   openapi3.NewResponses(),
		},
	})

	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Paths:   paths,
	}

	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "PUT", ops[0].Method)
	assert.Equal(t, "updateTest", ops[0].ID)
}

// TestExtractParameters_NilValue ensures a paramRef with nil Value is skipped.
func TestExtractParameters_NilValueSkipped(t *testing.T) {
	params := openapi3.Parameters{
		{Value: nil}, // nil Value should be skipped
		{
			Value: &openapi3.Parameter{
				Name: "valid",
				In:   "query",
				Schema: openapi3.NewSchemaRef("", &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				}),
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "valid", result[0].Name)
}

// TestExtractRequestBodyProperties_NilContentMediaType ensures safe handling of nil media type.
func TestExtractRequestBodyProperties_NilContentMediaType(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
	}

	// RequestBody with no application/json content
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"application/xml": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{}),
				},
			},
		},
	}

	result := extractRequestBodyProperties(spec, bodyRef)
	assert.Nil(t, result)
}

// TestExtractRequestBodyProperties_NilSchemaRef ensures safe handling of nil schema in media type.
func TestExtractRequestBodyProperties_NilSchemaRef(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
	}

	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: nil,
				},
			},
		},
	}

	result := extractRequestBodyProperties(spec, bodyRef)
	assert.Nil(t, result)
}

// TestExtractRequestBodyProperties_NilRequestBodyValue ensures safe handling of nil requestBody value.
func TestExtractRequestBodyProperties_NilRequestBodyValue(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
	}

	bodyRef := &openapi3.RequestBodyRef{
		Value: nil,
	}

	result := extractRequestBodyProperties(spec, bodyRef)
	assert.Nil(t, result)
}

// TestHasMultipartBody_NilBodyRefValue ensures safe handling of nil value.
func TestHasMultipartBody_NilValue(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{Value: nil}
	assert.False(t, hasMultipartBody(bodyRef))
}

// TestExtractMultipartFields_NilContent ensures safe handling of nil media type.
func TestExtractMultipartFields_NilContent(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"multipart/form-data": nil,
			},
		},
	}
	result := extractMultipartFields(bodyRef)
	assert.Nil(t, result)
}

// TestExtractMultipartFields_NilSchema ensures safe handling when schema is nil.
func TestExtractMultipartFields_NilSchema(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", nil),
				},
			},
		},
	}
	result := extractMultipartFields(bodyRef)
	assert.Nil(t, result)
}

// TestExtractMultipartFields_NoMultipartContent returns nil when there's no multipart content.
func TestExtractMultipartFields_NoMultipartContent(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", &openapi3.Schema{}),
				},
			},
		},
	}
	result := extractMultipartFields(bodyRef)
	assert.Nil(t, result)
}

// TestExtractMultipartFields_NilBodyRefValue ensures safe handling of nil value.
func TestExtractMultipartFields_NilValue(t *testing.T) {
	bodyRef := &openapi3.RequestBodyRef{Value: nil}
	result := extractMultipartFields(bodyRef)
	assert.Nil(t, result)
}

// TestExtractOperations_NilPathItemMapValue ensures paths that evaluate to nil are skipped.
// We construct a spec via JSON where kin-openapi may strip null entries from the paths map.
// Instead, we directly test the iteration logic by ensuring a clean spec with valid path.
func TestExtractOperations_SkipNilPathItem(t *testing.T) {
	// Directly construct a Paths with a nil entry using Set.
	paths := openapi3.NewPaths()
	paths.Set("/nil-path", nil)

	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Paths:   paths,
	}

	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	assert.Empty(t, ops)
}

// TestResolveServerURL_OperationLevelNil ensures no panic when operation servers are empty.
func TestResolveServerURL_OperationLevelNilServers(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Servers: openapi3.Servers{
			{URL: "https://root.example.com/api"},
		},
	}
	pathItem := &openapi3.PathItem{}
	operation := &openapi3.Operation{
		OperationID: "test",
		Servers:     &openapi3.Servers{}, // empty servers slice
	}

	url := resolveServerURL(spec, pathItem, operation)
	// Should fall through to root servers since the servers slice is empty
	assert.Equal(t, "https://root.example.com/api", url)
}

// TestSchemaToJSON_NilProps ensures nil input returns valid JSON.
func TestSchemaToJSON_NilProps(t *testing.T) {
	data, err := SchemaToJSON(nil)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "object", result["type"])
}

// TestSchemaToJSON_EmptySlice ensures empty slice returns valid JSON.
func TestSchemaToJSON_EmptySlice(t *testing.T) {
	data, err := SchemaToJSON([]RequestBodyProperty{})
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "object", result["type"])
}

// TestTypeToJSONSchemaType_CaseInsensitive ensures the mapping is case-insensitive.
func TestTypeToJSONSchemaType_CaseInsensitive(t *testing.T) {
	assert.Equal(t, "string", typeToJSONSchemaType("STRING"))
	assert.Equal(t, "integer", typeToJSONSchemaType("INTEGER"))
	assert.Equal(t, "number", typeToJSONSchemaType("Number"))
	assert.Equal(t, "boolean", typeToJSONSchemaType("Boolean"))
}

// TestExtractOperations_OperationWithServerURL ensures the extracted operation
// has the correct ServerURL based on the priority hierarchy.
func TestExtractOperations_ServerURLInOperation(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "ServerTest", "version": "1.0" },
		"servers": [{ "url": "https://root.example.com/api" }],
		"paths": {
			"/items": {
				"servers": [{ "url": "https://path.example.com/api" }],
				"get": {
					"operationId": "pathLevel",
					"responses": { "200": { "description": "OK" } }
				}
			},
			"/special": {
				"get": {
					"operationId": "opLevel",
					"servers": [{ "url": "https://op.example.com/api" }],
					"responses": { "200": { "description": "OK" } }
				}
			},
			"/root": {
				"get": {
					"operationId": "rootLevel",
					"responses": { "200": { "description": "OK" } }
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 3)

	urlByID := make(map[string]string)
	for _, op := range ops {
		urlByID[op.ID] = op.ServerURL
	}

	assert.Equal(t, "https://path.example.com/api", urlByID["pathLevel"])
	assert.Equal(t, "https://op.example.com/api", urlByID["opLevel"])
	assert.Equal(t, "https://root.example.com/api", urlByID["rootLevel"])
}

// TestExtractOperations_RequestBodyProperties ensures request body properties are extracted
// correctly via ExtractOperations.
func TestExtractOperations_RequestBodyProperties(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "BodyTest", "version": "1.0" },
		"paths": {
			"/items": {
				"post": {
					"operationId": "createItem",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"properties": {
										"name": { "type": "string", "description": "Item name" },
										"price": { "type": "number" }
									},
									"required": ["name"]
								}
							}
						}
					},
					"responses": {
						"201": { "description": "Created" }
					}
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	bodyProps := ops[0].RequestBody
	require.Len(t, bodyProps, 2)

	byName := make(map[string]RequestBodyProperty)
	for _, p := range bodyProps {
		byName[p.Name] = p
	}

	nameProp, ok := byName["name"]
	require.True(t, ok)
	assert.Equal(t, "string", nameProp.Type)
	assert.Equal(t, "Item name", nameProp.Description)
	assert.True(t, nameProp.Required)

	priceProp, ok := byName["price"]
	require.True(t, ok)
	assert.Equal(t, "number", priceProp.Type)
	assert.False(t, priceProp.Required)
}

// TestExtractOperations_HasMultiPart ensures multipart detection works through ExtractOperations.
func TestExtractOperations_HasMultiPart(t *testing.T) {
	doc := `{
		"openapi": "3.0.3",
		"info": { "title": "MultiPartTest", "version": "1.0" },
		"paths": {
			"/upload": {
				"post": {
					"operationId": "uploadFile",
					"requestBody": {
						"content": {
							"multipart/form-data": {
								"schema": {
									"type": "object",
									"properties": {
										"file": { "type": "string", "format": "binary" },
										"name": { "type": "string" }
									}
								}
							}
						}
					},
					"responses": {
						"201": { "description": "Created" }
					}
				}
			}
		}
	}`

	spec := mustLoadDoc(t, doc)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	assert.True(t, ops[0].HasMultiPart)
	require.Len(t, ops[0].MultipartFields, 1)
	assert.Equal(t, "name", ops[0].MultipartFields[0].Name)
}

// TestParse_NilBytes ensures safe handling of nil input bytes.
func TestParse_NilBytes(t *testing.T) {
	p := NewParser()
	_, err := p.Parse(context.Background(), nil, DefaultParseOptions())
	require.Error(t, err)
}

// TestParse_EmptyBytes ensures safe handling of empty input.
func TestParse_EmptyBytes(t *testing.T) {
	p := NewParser()
	_, err := p.Parse(context.Background(), []byte{}, DefaultParseOptions())
	require.Error(t, err)
}
