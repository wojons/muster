package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
	"github.com/openapi-cli/openapi-cli/pkg/openapi"
	"github.com/openapi-cli/openapi-cli/pkg/request"
)

// OpenAPIRegistry implements the CommandRegistry interface for OpenAPI operations
type OpenAPIRegistry struct {
	baseURL    string
	operations []*openapi.Operation
}

// NewRegistry creates a new registry from an OpenAPI spec
func NewRegistry(spec *openapi3.T, baseURL string) (*OpenAPIRegistry, error) {
	parser := &openapi.Parser{}
	operations, err := parser.ExtractOperations(spec)
	if err != nil {
		return nil, fmt.Errorf("extract operations: %w", err)
	}

	return &OpenAPIRegistry{
		baseURL:    baseURL,
		operations: operations,
	}, nil
}

// ListCommands returns all OpenAPI operations as MCP commands
func (r *OpenAPIRegistry) ListCommands() []handlers.Command {
	commands := make([]handlers.Command, 0, len(r.operations))

	for _, op := range r.operations {
		name := generateCommandName(op)
		cmd := handlers.Command{
			Name:        name,
			Description: fmt.Sprintf("%s %s", op.Summary, op.Path),
			Flags:       convertParameters(op),
		}
		commands = append(commands, cmd)
	}

	return commands
}

// GetCommand returns a command by name
func (r *OpenAPIRegistry) GetCommand(name string) *handlers.Command {
	for _, op := range r.operations {
		if generateCommandName(op) == name {
			return &handlers.Command{
				Name:        name,
				Description: fmt.Sprintf("%s %s", op.Summary, op.Path),
				Flags:       convertParameters(op),
			}
		}
	}

	return nil
}

// OpenAPIExecutor executes OpenAPI operations via HTTP
type OpenAPIExecutor struct {
	baseURL string
	client  *http.Client
}

// NewExecutor creates a new executor
func NewExecutor(baseURL string) *OpenAPIExecutor {
	return &OpenAPIExecutor{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Execute runs an OpenAPI operation
func (e *OpenAPIExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	// Look up the operation from the registry
	// For now, just try to execute with provided args
	// In a full implementation, we'd need to store the operation mapping

	// Build and execute HTTP request
	builder := request.NewBuilder(e.baseURL, "/", "GET")

	// Add query parameters from args
	for key, value := range args {
		if strValue, ok := value.(string); ok {
			builder.WithQueryParam(key, strValue)
		}
	}

	// Execute request
	resp, err := builder.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Read and parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]interface{}{
			"message": "Request executed but response parsing failed",
			"error":   err.Error(),
			"status":  resp.StatusCode,
		}, nil
	}

	return result, nil
}

// ToolRegistration helps register OpenAPI operations as MCP tools
type ToolRegistration struct {
	registry *OpenAPIRegistry
	executor *OpenAPIExecutor
}

// NewToolRegistration creates a new tool registration helper
func NewToolRegistration(spec *openapi3.T, baseURL string) (*ToolRegistration, error) {
	registry, err := NewRegistry(spec, baseURL)
	if err != nil {
		return nil, err
	}

	return &ToolRegistration{
		registry: registry,
		executor: NewExecutor(baseURL),
	}, nil
}

// RegisterHandlers registers the MCP handlers for tools
func (tr *ToolRegistration) RegisterHandlers(server *handlers.Registry) {
	listHandler := handlers.NewListToolsHandler(tr.registry)
	callHandler := handlers.NewCallToolHandler(tr.executor)

	server.Register(listHandler)
	server.Register(callHandler)
}

// ExecuteHTTP executes an HTTP request for an operation
func ExecuteHTTP(baseURL, method, path string, args map[string]interface{}) ([]byte, error) {
	builder := request.NewBuilder(baseURL, path, method)

	// Add query parameters
	for key, value := range args {
		if strValue, ok := value.(string); ok {
			builder.WithQueryParam(key, strValue)
		}
	}

	// Execute request
	resp, err := builder.Execute(context.Background())
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Read response body
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return json.MarshalIndent(result, "", "  ")
}

func generateCommandName(op *openapi.Operation) string {
	if op.ID != "" {
		return op.ID
	}

	// Generate name from method and path
	path := op.Path
	method := op.Method

	// Simple naming: method_path
	name := fmt.Sprintf("%s_%s", method, path)

	// Clean up the name
	name = fmt.Sprintf("%s%s", method, path)

	// Replace path parameters with underscores
	for i := 0; i < len(name); i++ {
		if name[i] == '{' || name[i] == '}' || name[i] == '/' {
			name = name[:i] + "_" + name[i+1:]
		}
	}

	return name
}

func convertParameters(op *openapi.Operation) []handlers.Flag {
	flags := make([]handlers.Flag, 0, len(op.Parameters))

	for _, param := range op.Parameters {
		flagType := "string"
		if param.Type != "" {
			flagType = param.Type
		}

		flags = append(flags, handlers.Flag{
			Name:        param.Name,
			Type:        flagType,
			Description: param.Description,
			Required:    param.Required && param.In == "path",
		})
	}

	return flags
}
