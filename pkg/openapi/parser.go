// Package openapi provides parsing and extraction of operations from OpenAPI 3.x specifications.
package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Parser extracts operations and parameters from OpenAPI documents.
type Parser struct{}

// NewParser creates a new OpenAPI parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseOptions controls parsing behavior.
type ParseOptions struct {
	SkipValidation bool
	MaxDepth       int
}

// DefaultParseOptions returns the default parse options.
func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		SkipValidation: false,
		MaxDepth:       10,
	}
}

// ParseResult holds the result of parsing an OpenAPI document.
//
// API shape (DF-MUSTER-5): Document is the fully parsed *openapi3.T
// (kin-openapi) document — there is no separate Info/Paths field on this
// struct. Access spec metadata through the document itself:
// result.Document.Info for the info section (title/version) and
// result.Document.Paths to look up path items. Version carries the spec's
// declared version string (from Info.Version) for convenience.
type ParseResult struct {
	// Version is the OpenAPI version string declared in the document's
	// info section (e.g. "1.0.0").
	Version string

	// Document is the parsed OpenAPI document (*openapi3.T). Use
	// Document.Info for metadata and Document.Paths for operations — the
	// result does not expose .Info/.Paths fields of its own.
	Document *openapi3.T
}

// Parse parses an OpenAPI document from raw bytes.
func (p *Parser) Parse(ctx context.Context, data []byte, opts ParseOptions) (*ParseResult, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, err
	}

	if doc.Info == nil {
		return nil, fmt.Errorf("not a valid OpenAPI document: missing info section")
	}

	version := doc.Info.Version
	return &ParseResult{
		Version:  version,
		Document: doc,
	}, nil
}

// RequestBodyProperty represents a property in a requestBody schema.
type RequestBodyProperty struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []interface{}
	Default     interface{}
}

// Operation represents a single API operation extracted from an OpenAPI spec.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/openapi/parser.go
type Operation struct {
	ID              string
	Tags            []string
	Path            string
	Method          string
	Summary         string
	Description     string
	Parameters      []Parameter
	RequestBody     []RequestBodyProperty
	HasMultiPart    bool                  // true if requestBody supports multipart/form-data
	MultipartFields []RequestBodyProperty // non-file fields in multipart form
	ServerURL       string                // resolved server URL for this operation
}

// Parameters is a convenience type for a slice of Parameter values.
type Parameters []Parameter

// Parameter represents a single API parameter (path, query, header, etc.).
type Parameter struct {
	In          string
	Name        string
	Type        string
	Required    bool
	Enum        []interface{}
	Default     interface{}
	Example     interface{}
	Description string
}

// ExtractOperations extracts all operations from an OpenAPI 3.x document.
// It walks all paths and their HTTP methods, collecting operation metadata.
func (p *Parser) ExtractOperations(spec *openapi3.T) ([]*Operation, error) {
	if spec == nil || spec.Paths == nil {
		return nil, nil
	}

	var operations []*Operation

	for path, pathItem := range spec.Paths.Map() {
		if pathItem == nil {
			continue
		}

		// Collect all HTTP methods for this path
		methods := map[string]*openapi3.Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"DELETE":  pathItem.Delete,
			"PATCH":   pathItem.Patch,
			"HEAD":    pathItem.Head,
			"OPTIONS": pathItem.Options,
		}

		for method, op := range methods {
			if op == nil {
				continue
			}

			// Extract tags from the operation-level tags field
			// (kin-openapi puts operation tags in the operation struct)
			serverURL := resolveServerURL(spec, pathItem, op)
			operation := &Operation{
				ID:          op.OperationID,
				Path:        path,
				Method:      method,
				Summary:     op.Summary,
				Description: op.Description,
				Tags:        op.Tags,
				Parameters:  extractParameters(op.Parameters),
				RequestBody: extractRequestBodyProperties(spec, op.RequestBody),
				ServerURL:   serverURL,
			}

			// Detect multipart/form-data support
			operation.HasMultiPart = hasMultipartBody(op.RequestBody)
			if operation.HasMultiPart {
				operation.MultipartFields = extractMultipartFields(op.RequestBody)
			}

			operations = append(operations, operation)
		}
	}

	return operations, nil
}

// resolveServerURL determines the server URL for an operation by checking:
// 1. Per-operation servers (highest priority — operation.Servers is *Servers)
// 2. Path-level servers (pathItem.Servers)
// 3. Root-level servers (spec.Servers)
// Returns empty string if no server is configured.
func resolveServerURL(spec *openapi3.T, pathItem *openapi3.PathItem, operation *openapi3.Operation) string {
	if operation.Servers != nil && len(*operation.Servers) > 0 && (*operation.Servers)[0] != nil {
		return (*operation.Servers)[0].URL
	}
	if pathItem != nil && len(pathItem.Servers) > 0 && pathItem.Servers[0] != nil {
		return pathItem.Servers[0].URL
	}
	if len(spec.Servers) > 0 && spec.Servers[0] != nil {
		return spec.Servers[0].URL
	}
	return ""
}

// extractParameters converts OpenAPI parameter references to internal Parameter types.
func extractParameters(params openapi3.Parameters) []Parameter {
	if len(params) == 0 {
		return nil
	}

	result := make([]Parameter, 0, len(params))
	for _, paramRef := range params {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value

		p := Parameter{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Description: param.Description,
		}

		// Extract type from schema if available
		if param.Schema != nil && param.Schema.Value != nil {
			if param.Schema.Value.Type != nil {
				p.Type = param.Schema.Value.Type.Slice()[0]
			}
		}

		// Extract enum values if available
		if param.Schema != nil && param.Schema.Value != nil && len(param.Schema.Value.Enum) > 0 {
			p.Enum = param.Schema.Value.Enum
		}

		// Extract default value
		if param.Schema != nil && param.Schema.Value != nil {
			p.Default = param.Schema.Value.Default
		}

		// Extract example value
		if param.Schema != nil && param.Schema.Value != nil {
			p.Example = param.Schema.Value.Example
		}

		result = append(result, p)
	}

	return result
}

// extractRequestBodyProperties extracts properties from an OpenAPI requestBody schema.
// For POST/PUT/PATCH operations, the requestBody content's JSON schema properties
// are converted into RequestBodyProperty entries suitable for flag generation.
// Resolves $ref references (e.g. "#/components/schemas/CreateMilestone") by
// looking up the referenced schema in the document's Components.Schemas map.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/openapi/parser.go
func extractRequestBodyProperties(spec *openapi3.T, bodyRef *openapi3.RequestBodyRef) []RequestBodyProperty {
	if bodyRef == nil || bodyRef.Value == nil {
		return nil
	}

	body := bodyRef.Value

	// Look for application/json media type
	mt := body.Content.Get("application/json")
	if mt == nil || mt.Schema == nil {
		return nil
	}

	return extractPropertiesFromSchemaRef(spec, mt.Schema)
}

// hasMultipartBody returns true if the requestBody supports multipart/form-data.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/openapi/parser.go
func hasMultipartBody(bodyRef *openapi3.RequestBodyRef) bool {
	if bodyRef == nil || bodyRef.Value == nil {
		return false
	}
	return bodyRef.Value.Content.Get("multipart/form-data") != nil
}

// extractMultipartFields extracts non-file fields from a multipart/form-data request body.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/openapi/parser.go
func extractMultipartFields(bodyRef *openapi3.RequestBodyRef) []RequestBodyProperty {
	if bodyRef == nil || bodyRef.Value == nil {
		return nil
	}
	mt := bodyRef.Value.Content.Get("multipart/form-data")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return nil
	}

	schema := mt.Schema.Value
	requiredSet := make(map[string]bool)
	for _, name := range schema.Required {
		requiredSet[name] = true
	}

	var props []RequestBodyProperty
	for name, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		propSchema := propRef.Value

		// Skip binary/file fields — those are handled via --file flag
		if propSchema.Format == "binary" || (propSchema.Type != nil && propSchema.Type.Is("string") && propSchema.Format == "binary") {
			continue
		}

		propType := ""
		if propSchema.Type != nil {
			propType = propSchema.Type.Slice()[0]
		}

		props = append(props, RequestBodyProperty{
			Name:        name,
			Type:        propType,
			Description: propSchema.Description,
			Required:    requiredSet[name],
			Enum:        propSchema.Enum,
			Default:     propSchema.Default,
		})
	}

	return props
}

// extractPropertiesFromSchemaRef extracts property metadata from a SchemaRef,
// resolving $ref references by looking up the referenced schema in the
// OpenAPI document's Components.Schemas map.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/openapi/parser.go
func extractPropertiesFromSchemaRef(spec *openapi3.T, schemaRef *openapi3.SchemaRef) []RequestBodyProperty {
	if schemaRef == nil {
		return nil
	}

	// kin-openapi should resolve $ref at parse time into schemaRef.Value,
	// but when the requestBody itself uses $ref (e.g. "$ref": "#/components/schemas/CreateMilestone")
	// the properties may live in the referenced schema rather than being inlined.
	// As a fallback, explicitly look up the reference when inline Properties
	// are missing.
	schema := schemaRef.Value
	if schema == nil {
		return nil
	}

	// Collect required field names
	requiredSet := make(map[string]bool)
	for _, name := range schema.Required {
		requiredSet[name] = true
	}

	var props []RequestBodyProperty
	for name, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		propSchema := propRef.Value

		propType := ""
		if propSchema.Type != nil {
			propType = propSchema.Type.Slice()[0]
		}

		props = append(props, RequestBodyProperty{
			Name:        name,
			Type:        propType,
			Description: propSchema.Description,
			Required:    requiredSet[name],
			Enum:        propSchema.Enum,
			Default:     propSchema.Default,
		})
	}

	// If no properties were found on the resolved Value, check if this
	// schema was a $ref and look up the referenced schema explicitly.
	if len(props) == 0 && schemaRef.Ref != "" && strings.HasPrefix(schemaRef.Ref, "#/components/schemas/") {
		schemaName := strings.TrimPrefix(schemaRef.Ref, "#/components/schemas/")
		if spec != nil && spec.Components != nil && spec.Components.Schemas != nil {
			if refSchema, ok := spec.Components.Schemas[schemaName]; ok && refSchema != nil && refSchema.Value != nil {
				schema = refSchema.Value
				for name, propRef := range schema.Properties {
					if propRef == nil || propRef.Value == nil {
						continue
					}
					propSchema := propRef.Value

					propType := ""
					if propSchema.Type != nil {
						propType = propSchema.Type.Slice()[0]
					}

					props = append(props, RequestBodyProperty{
						Name:        name,
						Type:        propType,
						Description: propSchema.Description,
						Required:    requiredSet[name],
						Enum:        propSchema.Enum,
						Default:     propSchema.Default,
					})
				}
			}
		}
	}

	// Handle allOf composition — merge properties from all allOf schemas
	for _, subSchemaRef := range schema.AllOf {
		subProps := extractPropertiesFromSchemaRef(spec, subSchemaRef)
		props = append(props, subProps...)
	}

	return props
}

// SchemaToJSON produces a JSON Schema for a set of request body properties.
// This is used by MCP tool generation to create inputSchema for tools.
func SchemaToJSON(props []RequestBodyProperty) ([]byte, error) {
	if len(props) == 0 {
		return []byte(`{"type":"object","properties":{}}`), nil
	}

	properties := make(map[string]interface{})
	required := make([]string, 0)

	for _, p := range props {
		propMap := map[string]interface{}{
			"type": typeToJSONSchemaType(p.Type),
		}
		if p.Description != "" {
			propMap["description"] = p.Description
		}
		if p.Default != nil {
			propMap["default"] = p.Default
		}
		if len(p.Enum) > 0 {
			propMap["enum"] = p.Enum
		}
		properties[p.Name] = propMap

		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return jsonMarshalNoEscape(schema)
}

func typeToJSONSchemaType(t string) string {
	switch strings.ToLower(t) {
	case "string":
		return "string"
	case "integer":
		return "integer"
	case "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}

func jsonMarshalNoEscape(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
