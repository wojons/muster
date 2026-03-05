package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
)

// Converter transforms OpenAPI operations into MCP tools
type Converter struct {
	baseURL string
}

// NewConverter creates a new OpenAPI-to-MCP converter
func NewConverter(baseURL string) *Converter {
	return &Converter{
		baseURL: baseURL,
	}
}

// OpenAPITool represents an MCP tool derived from an OpenAPI operation
type OpenAPITool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToHandlersTool converts OpenAPITool to handlers.Tool
func (t *OpenAPITool) ToHandlersTool() handlers.Tool {
	return handlers.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// ExecutionConfig contains execution information for a tool
type ExecutionConfig struct {
	Path    string `json:"path"`
	Method  string `json:"method"`
	BaseURL string `json:"base_url"`
}

// ConvertToTools converts all operations in an OpenAPI spec to MCP tools
func (c *Converter) ConvertToTools(doc *openapi3.T) ([]OpenAPITool, map[string]ExecutionConfig, error) {
	var tools []OpenAPITool
	configs := make(map[string]ExecutionConfig)

	for path, pathItem := range doc.Paths.Map() {
		methods := map[string]*openapi3.Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"PATCH":   pathItem.Patch,
			"DELETE":  pathItem.Delete,
			"HEAD":    pathItem.Head,
			"OPTIONS": pathItem.Options,
		}

		for method, operation := range methods {
			if operation == nil {
				continue
			}

			toolName := c.getToolName(operation, path, method)
			tool := OpenAPITool{
				Name:        toolName,
				Description: c.getToolDescription(operation),
				InputSchema: c.buildInputSchema(operation, path),
			}

			tools = append(tools, tool)
			configs[toolName] = ExecutionConfig{
				Path:    path,
				Method:  method,
				BaseURL: c.baseURL,
			}
		}
	}

	return tools, configs, nil
}

// getToolName generates a unique tool name from the operation
func (c *Converter) getToolName(operation *openapi3.Operation, path, method string) string {
	if operation.OperationID != "" {
		// Convert operationID to kebab-case
		return toKebabCase(operation.OperationID)
	}

	// Generate name from path and method
	resource := c.extractResource(path)
	action := c.getAction(method)
	return fmt.Sprintf("%s-%s", action, resource)
}

// getToolDescription generates a description for the tool
func (c *Converter) getToolDescription(operation *openapi3.Operation) string {
	if operation.Summary != "" {
		return operation.Summary
	}
	if operation.Description != "" {
		return operation.Description
	}
	return "API operation"
}

// buildInputSchema creates JSON Schema for operation parameters
func (c *Converter) buildInputSchema(operation *openapi3.Operation, path string) json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	// Path parameters
	for _, paramRef := range operation.Parameters {
		if paramRef.Ref != "" {
			continue
		}

		param := paramRef.Value
		if param.In == "path" {
			prop := map[string]interface{}{
				"type":        c.getParameterType(param),
				"description": param.Description,
			}
			schema["properties"].(map[string]interface{})[param.Name] = prop

			if param.Required {
				schema["required"] = append(schema["required"].([]string), param.Name)
			}
		}
	}

	// Query parameters
	for _, paramRef := range operation.Parameters {
		if paramRef.Ref != "" {
			continue
		}

		param := paramRef.Value
		if param.In == "query" {
			prop := map[string]interface{}{
				"type":        c.getParameterType(param),
				"description": param.Description,
			}
			if param.Schema != nil && param.Schema.Value != nil && param.Schema.Value.Default != nil {
				prop["default"] = param.Schema.Value.Default
			}
			schema["properties"].(map[string]interface{})[param.Name] = prop

			if param.Required {
				schema["required"] = append(schema["required"].([]string), param.Name)
			}
		}
	}

	// Request body
	if operation.RequestBody != nil && operation.RequestBody.Ref == "" {
		bodySchema := operation.RequestBody.Value.Content
		if jsonSchema, ok := bodySchema["application/json"]; ok && jsonSchema.Schema != nil {
			schema["properties"].(map[string]interface{})["body"] = map[string]interface{}{
				"type":        "object",
				"description": "Request body",
			}

			if operation.RequestBody.Value.Required {
				schema["required"] = append(schema["required"].([]string), "body")
			}
		}
	}

	data, _ := json.Marshal(schema)
	return data
}

// getParameterType determines the type of a parameter
func (c *Converter) getParameterType(param *openapi3.Parameter) string {
	if param.Schema == nil || param.Schema.Value == nil {
		return "string"
	}

	typeSlice := *param.Schema.Value.Type
	if len(typeSlice) > 0 {
		return typeSlice[0]
	}

	return "string"
}

// extractResource extracts resource name from path
func (c *Converter) extractResource(path string) string {
	parts := []rune(path)
	for i := 0; i < len(parts); i++ {
		if parts[i] == '/' {
			parts = parts[i+1:]
			i = -1
		} else if parts[i] == '{' {
			parts = parts[:i]
			break
		}
	}

	resource := string(parts)
	if resource == "" {
		return "resource"
	}
	return resource
}

// getAction converts HTTP method to action name
func (c *Converter) getAction(method string) string {
	switch method {
	case "GET":
		return "get"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}

// toKebabCase converts CamelCase to kebab-case
func toKebabCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		result = append(result, r)
	}
	return string(result)
}
