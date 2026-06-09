// Package openapi provides parsing and extraction of operations from OpenAPI 3.x specifications.
package openapi

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// Parser extracts operations and parameters from OpenAPI documents.
type Parser struct{}

// Operation represents a single API operation extracted from an OpenAPI spec.
type Operation struct {
	ID          string
	Tags        []string
	Path        string
	Method      string
	Summary     string
	Description string
	Parameters  []Parameter
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

		operation := &Operation{
			ID:          op.OperationID,
			Path:        path,
			Method:      method,
			Summary:     op.Summary,
			Description: op.Description,
			Tags:        op.Tags,
			Parameters:  extractParameters(op.Parameters),
		}

			// Derive operation ID if not set
			operations = append(operations, operation)
		}
	}

	return operations, nil
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

		result = append(result, p)
	}

	return result
}
