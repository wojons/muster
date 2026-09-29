package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// ToolExecutor implements handlers.CommandExecutor using request builder.
type ToolExecutor struct {
	configs          map[string]ExecutionConfig
	bearerToken      string
	credentialSource string
}

// ToolExecutorOption configures a ToolExecutor.
type ToolExecutorOption func(*ToolExecutor)

// DefaultCredentialSource names the mechanisms the MCP server resolves the
// outbound bearer credential from (cmd/mcp-server: --auth-token-file wins over
// MUSTER_AUTH_TOKEN). DF-038: a 401 is reported to the agent with this, so the
// failure says where the credential was expected from instead of only echoing
// the upstream body.
const DefaultCredentialSource = "MUSTER_AUTH_TOKEN environment variable or the --auth-token-file flag"

// WithBearerToken attaches a bearer credential to every outbound tool request.
func WithBearerToken(token string) ToolExecutorOption {
	return func(executor *ToolExecutor) {
		executor.bearerToken = token
	}
}

// WithCredentialSource records which mechanism the configured bearer credential
// came from (for example "the --auth-token-file flag (/etc/muster/key)"). It is
// used by the 401/403 classification and must never contain the credential
// value itself; when unset, DefaultCredentialSource is reported.
func WithCredentialSource(source string) ToolExecutorOption {
	return func(executor *ToolExecutor) {
		executor.credentialSource = source
	}
}

// NewOpenAPIExecutor creates a new executor.
func NewOpenAPIExecutor(configs map[string]ExecutionConfig, opts ...ToolExecutorOption) *ToolExecutor {
	executor := &ToolExecutor{
		configs: configs,
	}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
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
		// DF-038: an unresolvable operation is a client-side failure. It is
		// classified so the agent does not read it as an upstream API error.
		return nil, handlers.NewToolCallError(
			handlers.ErrorClassClient, handlers.ReasonUnknownOperation,
			fmt.Sprintf("tool %q not found in registry", name),
		).WithDetail("tool", name)
	}

	// CRITICAL SECURITY FIX: Validate inputs against JSON Schema before execution
	if err := e.validateArgs(config.InputSchema, args); err != nil {
		return nil, handlers.NewToolCallError(
			handlers.ErrorClassClient, handlers.ReasonInvalidArguments,
			fmt.Sprintf("tool %q: %v", name, err),
		).WithDetail("tool", name).WithCause(err)
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
			// DF-038: the caller supplied an unencodable argument, so this is
			// client-side, not an upstream failure.
			return nil, handlers.NewToolCallError(
				handlers.ErrorClassClient, handlers.ReasonInvalidArguments,
				fmt.Sprintf("marshal request body: %v", err),
			).WithDetail("tool", name).WithCause(err)
		}
		builder.JSONBody(string(bodyJSON))
		builder.SetHeader("Content-Type", "application/json")
	}

	// Process-level authentication is authoritative and applies to every tool
	// request. Set it after tool arguments so a generated header parameter
	// cannot replace the configured credential.
	if e.bearerToken != "" {
		builder.SetHeader("Authorization", "Bearer "+e.bearerToken)
	}

	// Execute request
	result, err := builder.ExecuteWithResult(ctx)
	if err != nil {
		// DF-038: the request never produced an upstream response. The message
		// text is unchanged; the classification says whose side failed.
		return nil, handlers.NewToolCallError(
			handlers.ErrorClassUpstream, handlers.ReasonUpstreamUnreachable, err.Error(),
		).WithDetail("tool", name).WithCause(err)
	}

	if result.StatusCode >= 400 {
		return nil, e.upstreamHTTPError(name, result.StatusCode, result.Body)
	}

	// Parse and return JSON response
	var response interface{}
	if err := json.Unmarshal(result.Body, &response); err != nil {
		// DF-038: a non-JSON body stays a text payload. The value reports
		// that it did NOT come from a JSON payload, so the MCP result builder
		// keeps it in the text content block instead of advertising it as
		// structured content. The marshaled shape is unchanged.
		return UnstructuredResult{
			Status: result.StatusCode,
			Body:   string(result.Body),
		}, nil
	}

	return response, nil
}

// UnstructuredResult is the result of a call whose upstream body was not JSON.
// It marshals to the same {"status","body"} object the executor has always
// returned, and implements handlers.StructuredResultProvider to report that the
// payload is not structured JSON.
type UnstructuredResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// StructuredPayload implements handlers.StructuredResultProvider. The upstream
// body was not JSON, so the payload is carried in the text content block only.
func (u UnstructuredResult) StructuredPayload() (interface{}, bool) { return u, false }

// upstreamHTTPError classifies an upstream error response (DF-038). A 401/403
// is an authentication outcome: with no credential configured the failure is
// client-side wiring and the message names the mechanisms the credential is
// expected from; with one configured the credential was rejected and the
// message names its source. Every other error status is an upstream error whose
// body is preserved as structured content when it was JSON.
func (e *ToolExecutor) upstreamHTTPError(tool string, status int, body []byte) error {
	return newUpstreamHTTPError(
		fmt.Sprintf("HTTP %d: %s", status, string(body)),
		e.credentialSourceName(),
		e.bearerToken != "",
		status,
		body,
	).WithDetail("tool", tool)
}

// newUpstreamHTTPError builds the classified failure for an upstream HTTP error
// status, shared by every executor in this package so the classes and reason
// codes reported to an agent consumer stay identical across protocol paths.
// message is the executor's own wording (unchanged for existing consumers),
// credentialSource names where the outbound credential came from or was
// expected from, and credentialConfigured says whether one was sent.
func newUpstreamHTTPError(message, credentialSource string, credentialConfigured bool, statusCode int, body []byte) *handlers.ToolCallError {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		if !credentialConfigured {
			return handlers.NewToolCallError(
				handlers.ErrorClassClient, handlers.ReasonMissingCredential,
				fmt.Sprintf("%s — the upstream rejected this request as unauthenticated and no bearer credential is configured: set %s",
					message, credentialSource),
			).
				WithStatus(statusCode).
				WithCredentialSource(credentialSource).
				WithUpstreamBody(parseJSONBody(body))
		}
		return handlers.NewToolCallError(
			handlers.ErrorClassClient, handlers.ReasonCredentialRejected,
			fmt.Sprintf("%s — the upstream rejected the bearer credential configured from %s",
				message, credentialSource),
		).
			WithStatus(statusCode).
			WithCredentialSource(credentialSource).
			WithUpstreamBody(parseJSONBody(body))
	}

	return handlers.NewToolCallError(
		handlers.ErrorClassUpstream, handlers.ReasonUpstreamHTTP, message,
	).
		WithStatus(statusCode).
		WithUpstreamBody(parseJSONBody(body))
}

// credentialSourceName reports where the outbound credential came from, or the
// mechanisms it was expected from when none is configured. It never contains
// the credential value.
func (e *ToolExecutor) credentialSourceName() string {
	if e.credentialSource != "" {
		return e.credentialSource
	}
	return DefaultCredentialSource
}

// parseJSONBody decodes an upstream response body as JSON for the error marker,
// returning nil when the body was not a JSON document (or was JSON null).
func parseJSONBody(body []byte) interface{} {
	if len(body) == 0 {
		return nil
	}
	var parsed interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed
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

// effectiveBaseURL resolves the base URL for a single operation, mirroring
// pkg/generator's resolution chain: an explicit base URL (the converter's
// BaseURL, sourced from --base-url / MUSTER_BASE_URL in cmd/mcp-server) wins,
// then the operation's resolved server URL from the spec (operation- →
// path- → root-level servers[], already extracted by pkg/openapi), then "".
// A relative spec server URL (e.g. "/api/v3") cannot be dereferenced on its
// own, so it does not satisfy an otherwise-unset base URL (DF-MUSTER-1).
func (c *Converter) effectiveBaseURL(opServerURL string) string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	if opServerURL == "" {
		return ""
	}
	if u, err := url.Parse(opServerURL); err != nil || u.Scheme == "" {
		return ""
	}
	return opServerURL
}

// containsStr reports whether s is present in list.
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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
		// DF-034 safety net: ensure every path template param is advertised
		// (properties+required) even if the parser missed it. Path params are
		// always required by the route.
		for _, pathParam := range extractPathParams(op.Path) {
			if _, ok := properties[pathParam]; !ok {
				properties[pathParam] = map[string]interface{}{
					"type":        "string",
					"description": fmt.Sprintf("Path parameter %s (from %s)", pathParam, op.Path),
				}
			}
			if !containsStr(required, pathParam) {
				required = append(required, pathParam)
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
			return nil, nil, fmt.Errorf("marshal schema for %s: %w", toolName, err)
		}

		// DF-MUSTER-1: default to the spec's servers[] URL so tools/call
		// works with --spec only; an explicit base URL (converter BaseURL
		// from --base-url / MUSTER_BASE_URL) still overrides.
		baseURL := c.effectiveBaseURL(op.ServerURL)

		tool := OpenAPITool{
			Name:        toolName,
			Description: description,
			Method:      op.Method,
			Path:        op.Path,
			BaseURL:     baseURL,
			InputSchema: schemaJSON,
		}
		tools = append(tools, tool)

		configs[toolName] = ExecutionConfig{
			BaseURL:     baseURL,
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
