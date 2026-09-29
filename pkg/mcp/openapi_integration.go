package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/request"
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

// OpenAPIExecutor executes OpenAPI operations via HTTP.
// It looks up operations by name to determine method and path.
type OpenAPIExecutor struct {
	baseURL    string
	client     *http.Client
	operations []*openapi.Operation
}

// NewExecutor creates a new executor with the given base URL and operations.
// The operations slice is used to look up method and path by operation name.
func NewExecutor(baseURL string, operations []*openapi.Operation) *OpenAPIExecutor {
	return &OpenAPIExecutor{
		baseURL:    baseURL,
		client:     &http.Client{},
		operations: operations,
	}
}

// NewExecutorWithRateLimit creates a new executor with token-bucket rate
// limiting applied to outgoing requests. ratePerSec is the number of tokens
// added per second; burst is the maximum accumulated tokens.
// When ratePerSec is <= 0, rate limiting is disabled.
func NewExecutorWithRateLimit(baseURL string, operations []*openapi.Operation, ratePerSec, burst int) *OpenAPIExecutor {
	transport := http.DefaultTransport
	if ratePerSec > 0 {
		transport = client.NewRateLimitRoundTripper(transport, ratePerSec, burst)
	}
	return &OpenAPIExecutor{
		baseURL: baseURL,
		client: &http.Client{
			Transport: transport,
		},
		operations: operations,
	}
}

// NewExecutorWithCache creates a new executor that caches GET responses.
// c is the cache implementation; ttl controls how long cached responses
// are stored. The caller retains ownership of the cache and is responsible
// for calling Close when no longer needed.
//
// This composes with NewExecutorWithRateLimit: callers may create an
// OpenAPIExecutor directly and compose their own transport chain.
func NewExecutorWithCache(baseURL string, operations []*openapi.Operation, c cache.Cache, ttl time.Duration) *OpenAPIExecutor {
	transport := client.NewCacheRoundTripper(http.DefaultTransport, c, ttl)
	return &OpenAPIExecutor{
		baseURL:    baseURL,
		client:     &http.Client{Transport: transport},
		operations: operations,
	}
}

// Execute runs an OpenAPI operation by name.
// For POST/PUT/PATCH methods, body parameters are JSON-encoded in the request body.
// For GET/DELETE methods, parameters are sent as query parameters.
func (e *OpenAPIExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	// Look up the operation to determine method and path
	method, path := e.lookupOperation(name)
	if method == "" {
		// Fallback: treat as GET with the name as path
		method = "GET"
		path = "/" + name
	}

	// Build HTTP request
	builder := request.NewBuilder(e.baseURL, path, method)

	// Determine if this is a write method that should use a JSON body
	isWriteMethod := method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch

	if isWriteMethod {
		// For POST/PUT/PATCH, collect args into JSON body
		if len(args) > 0 {
			bodyJSON, err := json.Marshal(args)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			builder.JSONBody(string(bodyJSON))
			builder.SetHeader("Content-Type", "application/json")
		}
	} else {
		// For GET/DELETE, add args as query parameters
		for key, value := range args {
			if strValue, ok := value.(string); ok {
				builder.WithQueryParam(key, strValue)
			} else if slice, ok := value.([]string); ok && len(slice) > 0 {
				// DF-017: serialize slice query params as repeated keys
				builder.WithQueryParamSlice(key, slice)
			} else if slice, ok := value.([]interface{}); ok && len(slice) > 0 {
				// DF-017: serialize []interface{} query params as repeated keys
				strs := make([]string, len(slice))
				for i, s := range slice {
					strs[i] = fmt.Sprintf("%v", s)
				}
				builder.WithQueryParamSlice(key, strs)
			} else {
				builder.WithQueryParam(key, fmt.Sprintf("%v", value))
			}
		}
	}

	// Execute request using the executor's HTTP client (may be rate-limited).
	// Build the request manually so we can use e.client instead of
	// builder.Execute's internal bare http.Client{}.
	req, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req = req.WithContext(ctx)
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Read and parse response — use interface{} so JSON arrays (not just
	// objects) decode without error (DF-016).
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]interface{}{
			"message": "Request executed but response parsing failed",
			"error":   err.Error(),
			"status":  resp.StatusCode,
		}, nil
	}

	return result, nil
}

// lookupOperation finds the method and path for a given operation name.
func (e *OpenAPIExecutor) lookupOperation(name string) (method, path string) {
	for _, op := range e.operations {
		if generateCommandName(op) == name {
			return op.Method, op.Path
		}
	}
	return "", ""
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
		executor: NewExecutor(baseURL, registry.Operations()),
	}, nil
}

// RegisterHandlers registers the MCP handlers for tools
func (tr *ToolRegistration) RegisterHandlers(server *handlers.Registry) {
	listHandler := handlers.NewListToolsHandler(tr.registry)
	callHandler := handlers.NewCallToolHandler(tr.executor)

	server.Register(listHandler)
	server.Register(callHandler)
}

// ExecuteHTTP executes an HTTP request for an operation.
// For POST/PUT/PATCH methods, arguments are JSON-encoded in the request body.
// For GET/DELETE methods, arguments are sent as query parameters.
// This matches the behavior of the builtin request subcommand (internal/builtin/request.go).
func ExecuteHTTP(baseURL, method, path string, args map[string]interface{}) ([]byte, error) {
	// Extract path parameters from path template (e.g., /users/{id})
	pathArgs := extractPathParamsStr(path)
	pathParamSet := make(map[string]bool)
	for _, p := range pathArgs {
		pathParamSet[p] = true
	}

	builder := request.NewBuilder(baseURL, path, method)

	// Apply path parameters
	for key, value := range args {
		if pathParamSet[key] {
			builder.WithPathParam(key, fmt.Sprintf("%v", value))
		}
	}

	// Determine if this is a write method that should use a JSON body
	isWriteMethod := method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch

	if isWriteMethod {
		// For POST/PUT/PATCH, collect non-path args into JSON body
		bodyMap := make(map[string]interface{})
		for key, value := range args {
			if !pathParamSet[key] {
				bodyMap[key] = value
			}
		}
		if len(bodyMap) > 0 {
			bodyJSON, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			builder.JSONBody(string(bodyJSON))
			builder.SetHeader("Content-Type", "application/json")
		}
	} else {
		// For GET/DELETE, add non-path args as query parameters
		for key, value := range args {
			if !pathParamSet[key] {
				if strValue, ok := value.(string); ok {
					builder.WithQueryParam(key, strValue)
				} else if slice, ok := value.([]string); ok && len(slice) > 0 {
					// DF-017: serialize slice query params as repeated keys
					builder.WithQueryParamSlice(key, slice)
				} else if slice, ok := value.([]interface{}); ok && len(slice) > 0 {
					// DF-017: serialize []interface{} query params as repeated keys
					strs := make([]string, len(slice))
					for i, s := range slice {
						strs[i] = fmt.Sprintf("%v", s)
					}
					builder.WithQueryParamSlice(key, strs)
				} else {
					builder.WithQueryParam(key, fmt.Sprintf("%v", value))
				}
			}
		}
	}

	// Execute request
	resp, err := builder.Execute(context.Background())
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Read response body — use interface{} so JSON arrays (not just
	// objects) decode without error (DF-016).
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return json.MarshalIndent(result, "", "  ")
}

// containsString reports whether s is present in list.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// extractPathParamsStr extracts {param} names from a path template string.
func extractPathParamsStr(path string) []string {
	var params []string
	runes := []rune(path)
	start := -1
	for i := 0; i < len(runes); i++ {
		if runes[i] == '{' {
			start = i + 1
		} else if runes[i] == '}' && start >= 0 {
			params = append(params, string(runes[start:i]))
			start = -1
		}
	}
	return params
}

func generateCommandName(op *openapi.Operation) string {
	if op.ID != "" {
		return op.ID
	}

	// Generate name from method and path
	path := op.Path
	method := op.Method

	// Simple naming: method + path (cleaned up)
	name := fmt.Sprintf("%s%s", method, path)

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

// ExecuteCommand executes an OpenAPI operation by name.
func (r *OpenAPIRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	for _, op := range r.operations {
		if generateCommandName(op) == name {
			return ExecuteHTTP(r.baseURL, op.Method, op.Path, params)
		}
	}
	return nil, fmt.Errorf("command %q not found", name)
}

// AddCommand adds a command to the registry (not supported for OpenAPI-derived).
func (r *OpenAPIRegistry) AddCommand(cmd handlers.Command) error {
	return fmt.Errorf("openapi registry is read-only")
}

// ListTools implements handlers.ToolLister by returning tools with populated InputSchema.
// Each tool's InputSchema is built from the OpenAPI operation's parameters as a JSON Schema object.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/mcp/openapi_integration.go
func (r *OpenAPIRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.operations))

	for _, op := range r.operations {
		name := generateCommandName(op)

		description := op.Summary
		if op.Description != "" {
			description = op.Description
		}
		if description == "" {
			description = fmt.Sprintf("%s %s", op.Method, op.Path)
		}

		// Build JSON Schema properties from operation parameters
		properties := make(map[string]interface{})
		required := make([]string, 0)

		for _, param := range op.Parameters {
			propMap := map[string]interface{}{
				"type": paramTypeToJSONSchema(param.Type),
			}
			if param.Description != "" {
				propMap["description"] = param.Description
			}
			properties[param.Name] = propMap
			if param.Required {
				required = append(required, param.Name)
			}
		}

		// DF-034: templated tool names embed path parameters ({task_id}, ...) —
		// ensure every path template param is advertised in the schema even when the
		// spec/parser did not surface it in op.Parameters (spec-validating clients
		// reject calls otherwise). Path params are always required by the route.
		for _, pathParam := range extractPathParamsStr(op.Path) {
			if _, ok := properties[pathParam]; !ok {
				properties[pathParam] = map[string]interface{}{
					"type":        "string",
					"description": fmt.Sprintf("Path parameter %s (from %s)", pathParam, op.Path),
				}
			}
			if !containsString(required, pathParam) {
				required = append(required, pathParam)
			}
		}

		// Add request body properties
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

		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}

		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			// Fallback: empty schema
			schemaJSON = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		tools = append(tools, handlers.Tool{
			Name:        name,
			Description: description,
			InputSchema: schemaJSON,
		})
	}

	return tools
}

// Operations returns the underlying OpenAPI operations.
func (r *OpenAPIRegistry) Operations() []*openapi.Operation {
	return r.operations
}

// RemoveCommand removes a command from the registry (not supported for OpenAPI-derived).
func (r *OpenAPIRegistry) RemoveCommand(name string) error {
	return fmt.Errorf("openapi registry is read-only")
}

// UpdateCommand updates a command in the registry (not supported for OpenAPI-derived).
func (r *OpenAPIRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return fmt.Errorf("openapi registry is read-only")
}
