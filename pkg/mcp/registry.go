package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
	"github.com/openapi-cli/openapi-cli/pkg/request"
)

// ToolRegistry implements handlers.CommandRegistry for OpenAPI tools
type ToolRegistry struct {
	tools   map[string]handlers.Tool
	configs map[string]ExecutionConfig
	baseURL string
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

// Execute executes an OpenAPI operation by name
func (e *ToolExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	config, ok := e.configs[name]
	if !ok {
		return nil, nil
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

	// Add query parameters
	for paramName, value := range args {
		if !isPathParam(config.Path, paramName) {
			builder.WithQueryParam(paramName, fmt.Sprintf("%v", value))
		}
	}

	// Execute request
	result, err := builder.ExecuteWithResult(ctx)
	if err != nil {
		return nil, err
	}

	if result.StatusCode >= 400 {
		return map[string]interface{}{
			"error": fmt.Sprintf("HTTP error: %d", result.StatusCode),
			"body":  string(result.Body),
		}, nil
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
