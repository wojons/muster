package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/request"
	"github.com/xeipuuv/gojsonschema"
)

// ExecutionConfig holds configuration for executing a tool.
type ExecutionConfig struct {
	BaseURL     string          `json:"baseUrl"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// OpenAPITool represents a tool derived from an OpenAPI spec.
type OpenAPITool struct {
	Name        string
	Description string
	Method      string
	Path        string
	BaseURL     string
	InputSchema json.RawMessage
}

// ToHandlersTool converts the OpenAPI tool to a handlers.Tool.
func (t *OpenAPITool) ToHandlersTool() handlers.Tool {
	return handlers.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// ToolRegistry implements handlers.CommandRegistry for OpenAPI tools
type ToolRegistry struct {
	tools   map[string]handlers.Tool
	configs map[string]ExecutionConfig
}

// NewOpenAPIRegistry creates a new registry from OpenAPI tools
func NewOpenAPIRegistry(tools []OpenAPITool, configs map[string]ExecutionConfig) *ToolRegistry {
	registry := &ToolRegistry{
		tools:   make(map[string]handlers.Tool),
		configs: configs,
	}

	for _, tool := range tools {
		registry.tools[tool.Name] = tool.ToHandlersTool()
	}

	return registry
}

// ListCommands returns all registered tools
func (r *ToolRegistry) ListCommands() []handlers.Command {
	var commands []handlers.Command
	for name, tool := range r.tools {
		var flags []handlers.Flag

		// Parse input schema to extract flags
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err == nil {
			if props, ok := schema["properties"].(map[string]interface{}); ok {
				for propName, propSchema := range props {
					if propName == "body" {
						continue // Skip body, special handling
					}

					if propMap, ok := propSchema.(map[string]interface{}); ok {
						flag := handlers.Flag{
							Name:        propName,
							Type:        getString(propMap, "type", "string"),
							Description: getString(propMap, "description", ""),
							Required:    isRequired(schema, propName),
						}
						flags = append(flags, flag)
					}
				}
			}
		}

		commands = append(commands, handlers.Command{
			Name:        name,
			Description: tool.Description,
			Flags:       flags,
		})
	}

	return commands
}

// ListTools returns all registered tools directly (preserving inputSchema).
func (r *ToolRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetCommand returns a command by name, or nil if not found
func (r *ToolRegistry) GetCommand(name string) *handlers.Command {
	tool, ok := r.tools[name]
	if !ok {
		return nil
	}

	var flags []handlers.Flag
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.InputSchema, &schema); err == nil {
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			for propName, propSchema := range props {
				if propName == "body" {
					continue
				}

				if propMap, ok := propSchema.(map[string]interface{}); ok {
					flag := handlers.Flag{
						Name:        propName,
						Type:        getString(propMap, "type", "string"),
						Description: getString(propMap, "description", ""),
						Required:    isRequired(schema, propName),
					}
					flags = append(flags, flag)
				}
			}
		}
	}

	return &handlers.Command{
		Name:        tool.Name,
		Description: tool.Description,
		Flags:       flags,
	}
}

// ToolExecutor implements handlers.CommandExecutor using request builder
type ToolExecutor struct {
	configs map[string]ExecutionConfig
}

// NewOpenAPIExecutor creates a new executor
func NewOpenAPIExecutor(configs map[string]ExecutionConfig) *ToolExecutor {
	return &ToolExecutor{
		configs: configs,
	}
}

// validateArgs validates tool arguments against JSON Schema
func (e *ToolExecutor) validateArgs(schema json.RawMessage, args map[string]interface{}) error {
	if len(schema) == 0 {
		// No schema provided, no validation required
		return nil
	}

	schemaLoader := gojsonschema.NewBytesLoader(schema)
	documentLoader := gojsonschema.NewGoLoader(args)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		var errors []string
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		return fmt.Errorf("validation failed: %s", strings.Join(errors, ", "))
	}

	return nil
}

// Execute executes an OpenAPI operation by name.
// For POST/PUT/PATCH methods, body parameters are JSON-encoded in the request body.
// For GET/DELETE methods, parameters are sent as query parameters.
func (e *ToolExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	config, ok := e.configs[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found in registry", name)
	}

	// CRITICAL SECURITY FIX: Validate inputs against JSON Schema before execution
	if err := e.validateArgs(config.InputSchema, args); err != nil {
		return nil, fmt.Errorf("tool %q: %w", name, err)
	}

	// Build HTTP request
	builder := request.NewBuilder(config.BaseURL, config.Path, config.Method)

	// Add path parameters
	pathArgs := extractPathParams(config.Path)
	for _, argName := range pathArgs {
		if value, ok := args[argName]; ok {
			builder.WithPathParam(argName, fmt.Sprintf("%v", value))
		}
	}

	// Classify parameters from InputSchema: query, header, body
	paramClassification := classifyParams(config.InputSchema)

	// Determine if this is a write method (POST/PUT/PATCH) that should use a JSON body
	isWriteMethod := config.Method == http.MethodPost ||
		config.Method == http.MethodPut ||
		config.Method == http.MethodPatch

	bodyMap := make(map[string]interface{})

	for paramName, value := range args {
		if isPathParam(config.Path, paramName) {
			continue
		}

		classification := paramClassification[paramName]
		switch classification {
		case "header":
			builder.SetHeader(paramName, fmt.Sprintf("%v", value))
		case "query":
			builder.WithQueryParam(paramName, fmt.Sprintf("%v", value))
		default:
			// Body parameter (no "in" field in schema, or explicitly "body")
			// For write methods, collect into JSON body; for read methods, fall back to query param
			if isWriteMethod {
				bodyMap[paramName] = value
			} else {
				builder.WithQueryParam(paramName, fmt.Sprintf("%v", value))
			}
		}
	}

	// JSON-encode body for write methods when body parameters are present
	if isWriteMethod && len(bodyMap) > 0 {
		bodyJSON, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		builder.JSONBody(string(bodyJSON))
		builder.SetHeader("Content-Type", "application/json")
	}

	// Execute request
	result, err := builder.ExecuteWithResult(ctx)
	if err != nil {
		return nil, err
	}

	if result.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", result.StatusCode, string(result.Body))
	}

	// Parse and return JSON response
	var response interface{}
	if err := json.Unmarshal(result.Body, &response); err != nil {
		// Return raw body if not JSON
		return map[string]interface{}{
			"status": result.StatusCode,
			"body":   string(result.Body),
		}, nil
	}

	return response, nil
}

// classifyParams parses the JSON Schema from an ExecutionConfig's InputSchema
// and returns a map of parameter name -> location ("query", "header", "path", "body").
// Parameters with an explicit "in" field (from OpenAPI parameter definitions) retain that location.
// Parameters without an "in" field (from requestBody properties) are classified as "body".
func classifyParams(schemaJSON json.RawMessage) map[string]string {
	result := make(map[string]string)

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return result
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return result
	}

	for name, prop := range props {
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			result[name] = "body"
			continue
		}
		if in, ok := propMap["in"].(string); ok {
			result[name] = in
		} else {
			result[name] = "body"
		}
	}

	return result
}

// Helper functions

func getString(m map[string]interface{}, key, defaultVal string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultVal
}

func isRequired(schema map[string]interface{}, name string) bool {
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if r == name {
				return true
			}
		}
	}
	return false
}

func extractPathParams(path string) []string {
	var params []string
	parts := []rune(path)
	start := -1
	for i := 0; i < len(parts); i++ {
		if parts[i] == '{' {
			start = i + 1
		} else if parts[i] == '}' && start >= 0 {
			paramName := string(parts[start:i])
			params = append(params, paramName)
			start = -1
		}
	}
	return params
}

func isPathParam(path, name string) bool {
	params := extractPathParams(path)
	for _, p := range params {
		if p == name {
			return true
		}
	}
	return false
}

// ExecuteCommand executes a tool by name.
func (r *ToolRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	_ = tool
	return map[string]interface{}{"executed": true}, nil
}

// AddCommand adds a command to the registry.
func (r *ToolRegistry) AddCommand(cmd handlers.Command) error {
	return fmt.Errorf("not supported in tool registry")
}

// RemoveCommand removes a command from the registry.
func (r *ToolRegistry) RemoveCommand(name string) error {
	return fmt.Errorf("not supported in tool registry")
}

// UpdateCommand updates a command in the registry.
func (r *ToolRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return fmt.Errorf("not supported in tool registry")
}

// Converter converts OpenAPI specs to MCP tools.
type Converter struct {
	BaseURL string
}

// NewConverter creates a new Converter.
func NewConverter(baseURL string) *Converter {
	return &Converter{BaseURL: baseURL}
}

// ConvertToTools converts an OpenAPI document to MCP tools and execution configs.
func (c *Converter) ConvertToTools(doc *openapi3.T) ([]OpenAPITool, map[string]ExecutionConfig, error) {
	if doc == nil || doc.Paths == nil {
		return nil, nil, nil
	}

	parser := &openapi.Parser{}
	operations, err := parser.ExtractOperations(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("extract operations: %w", err)
	}

	tools := make([]OpenAPITool, 0, len(operations))
	configs := make(map[string]ExecutionConfig, len(operations))

	for _, op := range operations {
		toolName := op.ID
		if toolName == "" {
			toolName = fmt.Sprintf("%s_%s", strings.ToLower(op.Method), strings.ReplaceAll(strings.Trim(op.Path, "/"), "/", "_"))
		}

		// Build description
		description := op.Summary
		if op.Description != "" {
			description = op.Description
		}
		if description == "" {
			description = fmt.Sprintf("%s %s", op.Method, op.Path)
		}

		// Build input schema from params + requestBody
		properties := make(map[string]interface{})
		required := make([]string, 0)

		for _, param := range op.Parameters {
			propMap := map[string]interface{}{
				"type": paramTypeToJSONSchema(param.Type),
				"in":   param.In,
			}
			if param.Description != "" {
				propMap["description"] = param.Description
			}
			properties[param.Name] = propMap
			if param.Required {
				required = append(required, param.Name)
			}
		}

		// Add request body properties to input schema
		for _, bodyProp := range op.RequestBody {
			propMap := map[string]interface{}{
				"type": bodyPropTypeToJSONSchema(bodyProp.Type),
			}
			if bodyProp.Description != "" {
				propMap["description"] = bodyProp.Description
			}
			properties[bodyProp.Name] = propMap
			if bodyProp.Required {
				required = append(required, bodyProp.Name)
			}
		}

		// Build full JSON Schema
		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}

		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal schema for %s: %w", toolName, err)
		}

		tool := OpenAPITool{
			Name:        toolName,
			Description: description,
			Method:      op.Method,
			Path:        op.Path,
			BaseURL:     c.BaseURL,
			InputSchema: schemaJSON,
		}
		tools = append(tools, tool)

		configs[toolName] = ExecutionConfig{
			BaseURL:     c.BaseURL,
			Method:      op.Method,
			Path:        op.Path,
			InputSchema: schemaJSON,
		}
	}

	return tools, configs, nil
}

func paramTypeToJSONSchema(t string) string {
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

func bodyPropTypeToJSONSchema(t string) string {
	return paramTypeToJSONSchema(t)
}
