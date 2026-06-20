package openapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructor / Defaults ---

func TestNewParser(t *testing.T) {
	p := NewParser()
	assert.NotNil(t, p, "NewParser should return a non-nil Parser")
}

func TestDefaultParseOptions(t *testing.T) {
	opts := DefaultParseOptions()
	assert.False(t, opts.SkipValidation, "SkipValidation should default to false")
	assert.Equal(t, 10, opts.MaxDepth, "MaxDepth should default to 10")
}

// --- Parse ---

func TestParser_Parse_ValidPetstore(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../demo/openapi-specs/petstore.yaml")
	require.NoError(t, err, "petstore.yaml should load without error")
	require.NotNil(t, doc)

	// Re-serialize and parse through our Parser to exercise the Parse path
	data, err := doc.MarshalJSON()
	require.NoError(t, err)

	p := NewParser()
	result, err := p.Parse(context.Background(), data, DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "1.0.0", result.Version, "version should match petstore spec")
	assert.NotNil(t, result.Document)
	assert.NotNil(t, result.Document.Info)
	assert.Equal(t, "Pet Store API", result.Document.Info.Title)
}

func TestParser_Parse_InvalidJSON(t *testing.T) {
	p := NewParser()
	result, err := p.Parse(context.Background(), []byte("not json"), DefaultParseOptions())
	assert.Error(t, err, "invalid JSON should produce an error")
	assert.Nil(t, result)
}

func TestParser_Parse_NilData(t *testing.T) {
	p := NewParser()
	result, err := p.Parse(context.Background(), nil, DefaultParseOptions())
	assert.Error(t, err, "nil data should produce an error")
	assert.Nil(t, result)
}

func TestParser_Parse_MissingInfo(t *testing.T) {
	// A valid JSON object that is not a valid OpenAPI doc (no info section)
	p := NewParser()
	result, err := p.Parse(context.Background(), []byte(`{"openapi":"3.0.0","paths":{}}`), DefaultParseOptions())
	assert.Error(t, err, "missing info section should produce an error")
	assert.Contains(t, err.Error(), "missing info section")
	assert.Nil(t, result)
}

// --- ExtractOperations ---

func loadPetstoreSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../demo/openapi-specs/petstore.yaml")
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

func TestParser_ExtractOperations_Petstore(t *testing.T) {
	spec := loadPetstoreSpec(t)
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	require.NotNil(t, ops)

	assert.Len(t, ops, 5, "petstore.yaml has 5 operations")

	// Build a lookup by operation ID
	opMap := make(map[string]*Operation)
	for _, op := range ops {
		opMap[op.ID] = op
	}

	// Verify expected operations exist
	expectedIDs := []string{"listPets", "createPet", "getPet", "updatePet", "deletePet"}
	for _, id := range expectedIDs {
		op, ok := opMap[id]
		require.True(t, ok, "operation %q should exist", id)
		assert.NotEmpty(t, op.Path)
		assert.NotEmpty(t, op.Method)
	}

	// Verify specific operation details
	listPets := opMap["listPets"]
	assert.Equal(t, "/pets", listPets.Path)
	assert.Equal(t, "GET", listPets.Method)
	assert.Equal(t, "List all pets", listPets.Summary)
	assert.Contains(t, listPets.Tags, "pets")
	assert.NotEmpty(t, listPets.ServerURL, "server URL should be resolved from root servers")

	createPet := opMap["createPet"]
	assert.Equal(t, "/pets", createPet.Path)
	assert.Equal(t, "POST", createPet.Method)
	assert.NotEmpty(t, createPet.RequestBody, "createPet has a request body")

	getPet := opMap["getPet"]
	assert.Equal(t, "/pets/{petId}", getPet.Path)
	assert.Equal(t, "GET", getPet.Method)
	assert.NotEmpty(t, getPet.Parameters, "getPet has path parameter")

	deletePet := opMap["deletePet"]
	assert.Equal(t, "DELETE", deletePet.Method)
}

func TestParser_ExtractOperations_EmptySpec(t *testing.T) {
	// Create a minimal spec with no paths
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Empty", Version: "0.0.0"},
		Paths:   &openapi3.Paths{},
	}
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	assert.Len(t, ops, 0, "empty spec should yield 0 operations")
}

func TestParser_ExtractOperations_NilSpec(t *testing.T) {
	p := NewParser()
	ops, err := p.ExtractOperations(nil)
	require.NoError(t, err)
	assert.Nil(t, ops, "nil spec should return nil operations")
}

func TestParser_ExtractOperations_NilPaths(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "NoPaths", Version: "0.0.0"},
	}
	p := NewParser()
	ops, err := p.ExtractOperations(spec)
	require.NoError(t, err)
	assert.Nil(t, ops, "nil paths should return nil operations")
}

// --- extractParameters ---

func TestExtractParameters_Path(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name:     "petId",
				In:       "path",
				Required: true,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "petId", result[0].Name)
	assert.Equal(t, "path", result[0].In)
	assert.True(t, result[0].Required)
	assert.Equal(t, "string", result[0].Type)
}

func TestExtractParameters_Query(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name:        "limit",
				In:          "query",
				Required:    false,
				Description: "Max results",
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}},
				},
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "limit", result[0].Name)
	assert.Equal(t, "query", result[0].In)
	assert.False(t, result[0].Required)
	assert.Equal(t, "integer", result[0].Type)
	assert.Equal(t, "Max results", result[0].Description)
}

func TestExtractParameters_Header(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name: "X-API-Key",
				In:   "header",
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "X-API-Key", result[0].Name)
	assert.Equal(t, "header", result[0].In)
}

func TestExtractParameters_Empty(t *testing.T) {
	result := extractParameters(openapi3.Parameters{})
	assert.Nil(t, result, "empty parameters should return nil")
}

func TestExtractParameters_NilRef(t *testing.T) {
	params := openapi3.Parameters{nil}
	result := extractParameters(params)
	assert.Len(t, result, 0, "nil param ref should be skipped")
}

func TestExtractParameters_WithEnum(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name: "status",
				In:   "query",
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Enum: []interface{}{"active", "inactive", "pending"},
					},
				},
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, "status", result[0].Name)
	assert.Len(t, result[0].Enum, 3)
	assert.Contains(t, result[0].Enum, "active")
}

func TestExtractParameters_WithDefault(t *testing.T) {
	params := openapi3.Parameters{
		{
			Value: &openapi3.Parameter{
				Name: "limit",
				In:   "query",
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:    &openapi3.Types{"integer"},
						Default: float64(10),
					},
				},
			},
		},
	}
	result := extractParameters(params)
	require.Len(t, result, 1)
	assert.Equal(t, float64(10), result[0].Default)
}

// --- extractRequestBodyProperties ---

func TestExtractRequestBodyProperties_JSON(t *testing.T) {
	// Build a spec with a requestBody that has application/json content
	specYAML := `
openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - name
              properties:
                name:
                  type: string
                  description: The name
                age:
                  type: integer
`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specYAML))
	require.NoError(t, err)

	// Get the operation's requestBody
	pathItem := doc.Paths.Find("/test")
	require.NotNil(t, pathItem)
	op := pathItem.Post
	require.NotNil(t, op)
	require.NotNil(t, op.RequestBody)

	props := extractRequestBodyProperties(doc, op.RequestBody)
	require.Len(t, props, 2)

	// Build lookup by name
	propMap := make(map[string]RequestBodyProperty)
	for _, p := range props {
		propMap[p.Name] = p
	}

	nameProp, ok := propMap["name"]
	require.True(t, ok, "name property should exist")
	assert.Equal(t, "string", nameProp.Type)
	assert.Equal(t, "The name", nameProp.Description)
	assert.True(t, nameProp.Required)

	ageProp, ok := propMap["age"]
	require.True(t, ok, "age property should exist")
	assert.Equal(t, "integer", ageProp.Type)
	assert.False(t, ageProp.Required)
}

func TestExtractRequestBodyProperties_NilBody(t *testing.T) {
	spec := &openapi3.T{}
	props := extractRequestBodyProperties(spec, nil)
	assert.Nil(t, props)
}

func TestExtractRequestBodyProperties_NoJSONContent(t *testing.T) {
	// requestBody with only text/plain content (no application/json)
	specYAML := `
openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        content:
          text/plain:
            schema:
              type: string
`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specYAML))
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/test")
	require.NotNil(t, pathItem)
	op := pathItem.Post
	require.NotNil(t, op)

	props := extractRequestBodyProperties(doc, op.RequestBody)
	assert.Len(t, props, 0, "no application/json content should yield empty properties")
}

// --- hasMultipartBody ---

func TestHasMultipartBody_True(t *testing.T) {
	specYAML := `
openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths:
  /upload:
    post:
      operationId: upload
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              properties:
                file:
                  type: string
                  format: binary
`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specYAML))
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/upload")
	require.NotNil(t, pathItem)
	op := pathItem.Post
	require.NotNil(t, op)

	assert.True(t, hasMultipartBody(op.RequestBody))
}

func TestHasMultipartBody_False(t *testing.T) {
	assert.False(t, hasMultipartBody(nil))
}

// --- extractMultipartFields ---

func TestExtractMultipartFields(t *testing.T) {
	specYAML := `
openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths:
  /upload:
    post:
      operationId: upload
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              required:
                - description
              properties:
                file:
                  type: string
                  format: binary
                description:
                  type: string
                  description: File description
                category:
                  type: string
                  enum: [photo, document, other]
`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specYAML))
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/upload")
	require.NotNil(t, pathItem)
	op := pathItem.Post
	require.NotNil(t, op)

	fields := extractMultipartFields(op.RequestBody)
	// file is binary and should be skipped; description and category remain
	require.Len(t, fields, 2)

	propMap := make(map[string]RequestBodyProperty)
	for _, f := range fields {
		propMap[f.Name] = f
	}

	desc, ok := propMap["description"]
	require.True(t, ok)
	assert.Equal(t, "string", desc.Type)
	assert.True(t, desc.Required)

	cat, ok := propMap["category"]
	require.True(t, ok)
	assert.Len(t, cat.Enum, 3)
}

func TestExtractMultipartFields_Nil(t *testing.T) {
	assert.Nil(t, extractMultipartFields(nil))
}

// --- SchemaToJSON ---

func TestSchemaToJSON_Valid(t *testing.T) {
	props := []RequestBodyProperty{
		{Name: "name", Type: "string", Description: "The name", Required: true},
		{Name: "age", Type: "integer", Description: "The age", Required: false, Default: float64(0)},
		{Name: "status", Type: "string", Enum: []interface{}{"active", "inactive"}},
	}

	data, err := SchemaToJSON(props)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Parse the JSON to verify structure
	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err)

	assert.Equal(t, "object", schema["type"])

	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)

	nameProp, ok := properties["name"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", nameProp["type"])
	assert.Equal(t, "The name", nameProp["description"])

	ageProp, ok := properties["age"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "integer", ageProp["type"])
	assert.Equal(t, float64(0), ageProp["default"])

	statusProp, ok := properties["status"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", statusProp["type"])
	enumVals, ok := statusProp["enum"].([]interface{})
	require.True(t, ok)
	assert.Len(t, enumVals, 2)

	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	assert.Len(t, required, 1)
	assert.Contains(t, required, "name")
}

func TestSchemaToJSON_Empty(t *testing.T) {
	data, err := SchemaToJSON([]RequestBodyProperty{})
	require.NoError(t, err)

	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err)

	assert.Equal(t, "object", schema["type"])
	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, properties, 0)
}

func TestSchemaToJSON_NoRequired(t *testing.T) {
	props := []RequestBodyProperty{
		{Name: "optional", Type: "string", Required: false},
	}

	data, err := SchemaToJSON(props)
	require.NoError(t, err)

	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err)

	// No "required" key when no properties are required
	_, hasRequired := schema["required"]
	assert.False(t, hasRequired, "no required key when all properties are optional")
}

// --- typeToJSONSchemaType ---

func TestTypeToJSONSchemaType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"string", "string"},
		{"STRING", "string"},
		{"integer", "integer"},
		{"Integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"array", "array"},
		{"object", "object"},
		{"unknown", "string"}, // default fallback
		{"", "string"},        // empty falls back
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := typeToJSONSchemaType(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- resolveServerURL ---

func TestResolveServerURL_RootLevel(t *testing.T) {
	spec := &openapi3.T{
		Servers: openapi3.Servers{
			{URL: "https://api.example.com/v1"},
		},
	}
	pathItem := &openapi3.PathItem{}
	op := &openapi3.Operation{}

	url := resolveServerURL(spec, pathItem, op)
	assert.Equal(t, "https://api.example.com/v1", url)
}

func TestResolveServerURL_PathLevel(t *testing.T) {
	spec := &openapi3.T{
		Servers: openapi3.Servers{
			{URL: "https://api.example.com/v1"},
		},
	}
	pathItem := &openapi3.PathItem{
		Servers: openapi3.Servers{
			{URL: "https://pets.example.com/v2"},
		},
	}
	op := &openapi3.Operation{}

	url := resolveServerURL(spec, pathItem, op)
	assert.Equal(t, "https://pets.example.com/v2", url, "path-level server should override root")
}

func TestResolveServerURL_OperationLevel(t *testing.T) {
	spec := &openapi3.T{
		Servers: openapi3.Servers{
			{URL: "https://api.example.com/v1"},
		},
	}
	pathItem := &openapi3.PathItem{
		Servers: openapi3.Servers{
			{URL: "https://pets.example.com/v2"},
		},
	}
	opServers := openapi3.Servers{
		{URL: "https://special.example.com/v3"},
	}
	op := &openapi3.Operation{
		Servers: &opServers,
	}

	url := resolveServerURL(spec, pathItem, op)
	assert.Equal(t, "https://special.example.com/v3", url, "operation-level server should override all")
}

func TestResolveServerURL_NoServers(t *testing.T) {
	spec := &openapi3.T{}
	pathItem := &openapi3.PathItem{}
	op := &openapi3.Operation{}

	url := resolveServerURL(spec, pathItem, op)
	assert.Empty(t, url)
}

// --- extractPropertiesFromSchemaRef ---

func TestExtractPropertiesFromSchemaRef_Nil(t *testing.T) {
	props := extractPropertiesFromSchemaRef(nil, nil)
	assert.Nil(t, props)
}

func TestExtractPropertiesFromSchemaRef_AllOf(t *testing.T) {
	specYAML := `
openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths: {}
components:
  schemas:
    Base:
      type: object
      properties:
        id:
          type: string
    Extended:
      allOf:
        - $ref: '#/components/schemas/Base'
        - type: object
          properties:
            name:
              type: string
`
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(specYAML))
	require.NoError(t, err)

	extendedSchema := doc.Components.Schemas["Extended"]
	require.NotNil(t, extendedSchema)

	props := extractPropertiesFromSchemaRef(doc, extendedSchema)
	require.Len(t, props, 2)

	propMap := make(map[string]RequestBodyProperty)
	for _, p := range props {
		propMap[p.Name] = p
	}

	_, hasID := propMap["id"]
	assert.True(t, hasID, "allOf should merge id from Base")
	_, hasName := propMap["name"]
	assert.True(t, hasName, "allOf should include name from inline schema")
}

// --- jsonMarshalNoEscape ---

func TestJSONMarshalNoEscape(t *testing.T) {
	data, err := jsonMarshalNoEscape(map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, string(data))
}
