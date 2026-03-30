package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/log"
	"github.com/openapi-cli/openapi-cli/pkg/openapi"
	"github.com/openapi-cli/openapi-cli/pkg/request"
	"github.com/spf13/cobra"
)

type Config struct {
	AppName          string
	AppDescription   string
	Version          string
	BaseURL          string
	NamingStrategy   NamingStrategy
	OperationStyle   OperationStyle
	DefaultFormat    string
	SupportedFormats []string
	CommandPrefix    string
	ExcludeTags      []string
	IncludeOnlyTags  []string
	CustomMappings   map[string]string
}

type NamingStrategy int

const (
	NamingKebab NamingStrategy = iota
	NamingCamel
	NamingSnake
	NamingFlat
)

type OperationStyle int

const (
	StyleResource OperationStyle = iota
	StyleHTTP
	StyleCRUD
	StyleFlat
)

func (c *Config) Validate() error {
	if c.AppName == "" {
		return fmt.Errorf("app name is required")
	}

	if c.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}

	if c.DefaultFormat == "" {
		c.DefaultFormat = "table"
	}

	if len(c.SupportedFormats) == 0 {
		c.SupportedFormats = []string{"table", "json", "yaml"}
	}

	return nil
}

type Generator struct {
	config Config
}

func NewGenerator(config Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Generator{config: config}, nil
}

func (g *Generator) GenerateCommands(spec *openapi3.T) ([]*cobra.Command, error) {
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	if err != nil {
		return nil, fmt.Errorf("extract operations: %w", err)
	}

	groups := g.groupOperations(ops)

	var commands []*cobra.Command
	for resource, ops := range groups {
		cmd := g.buildResourceCommand(resource, ops)
		commands = append(commands, cmd)
	}

	return commands, nil
}

func (g *Generator) groupOperations(ops []*openapi.Operation) map[string][]*openapi.Operation {
	groups := make(map[string][]*openapi.Operation)

	for _, op := range ops {
		resource := g.extractResource(op)

		if g.isExcluded(resource, op.Tags) {
			continue
		}

		groups[resource] = append(groups[resource], op)
	}

	return groups
}

func (g *Generator) extractResource(op *openapi.Operation) string {
	if len(op.Tags) > 0 {
		return g.normalizeName(op.Tags[0])
	}

	parts := strings.Split(strings.Trim(op.Path, "/"), "/")
	if len(parts) > 0 {
		return g.normalizeName(parts[0])
	}

	return "default"
}

func (g *Generator) isExcluded(resource string, tags []string) bool {
	for _, excluded := range g.config.ExcludeTags {
		if resource == excluded {
			return true
		}
		for _, tag := range tags {
			if tag == excluded {
				return true
			}
		}
	}

	if len(g.config.IncludeOnlyTags) > 0 {
		found := false
		for _, included := range g.config.IncludeOnlyTags {
			if resource == included {
				found = true
				break
			}
		}
		for _, tag := range tags {
			for _, included := range g.config.IncludeOnlyTags {
				if tag == included {
					found = true
					break
				}
			}
		}
		if !found {
			return true
		}
	}

	return false
}

func (g *Generator) buildResourceCommand(resource string, operations []*openapi.Operation) *cobra.Command {
	cmd := &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Manage %s", resource),
		Long:  fmt.Sprintf("Commands for managing %s resources", resource),
	}

	for _, op := range operations {
		subCmd := g.buildOperationCommand(op)
		cmd.AddCommand(subCmd)
	}

	return cmd
}

func (g *Generator) buildOperationCommand(op *openapi.Operation) *cobra.Command {
	name := g.getCommandName(op)

	usage := g.buildUsage(name, op)
	longDesc := g.buildLongDescription(op)
	example := g.buildExample(name, op)

	cmd := &cobra.Command{
		Use:     usage,
		Short:   op.Summary,
		Long:    longDesc,
		Example: example,
		RunE:    g.createRunHandler(op),
	}

	g.addFlags(cmd, op)

	return cmd
}

func (g *Generator) addFlags(cmd *cobra.Command, op *openapi.Operation) {
	for _, param := range op.Parameters {
		if param.In == "path" {
			continue
		}

		flagName := g.normalizeFlagName(param.Name)
		shorthand := g.inferShorthand(param.Name)
		description := param.Description

		switch param.Type {
		case "string":
			g.addStringFlag(cmd, flagName, shorthand, param, description)
		case "integer":
			defaultVal := 0
			if param.Default != nil {
				if v, ok := param.Default.(float64); ok {
					defaultVal = int(v)
				}
			}
			cmd.Flags().IntP(flagName, shorthand, defaultVal, description)
		case "number":
			defaultVal := 0.0
			if param.Default != nil {
				defaultVal = param.Default.(float64)
			}
			cmd.Flags().Float64P(flagName, shorthand, defaultVal, description)
		case "boolean":
			defaultVal := false
			if param.Default != nil {
				defaultVal = param.Default.(bool)
			}
			cmd.Flags().BoolP(flagName, shorthand, defaultVal, description)
		case "array":
			cmd.Flags().StringSliceP(flagName, shorthand, nil, description)
		}

		if param.Required {
			_ = cmd.MarkFlagRequired(flagName)
		}
	}

	cmd.Flags().StringP("output", "o", g.config.DefaultFormat, fmt.Sprintf("Output format (%s)", strings.Join(g.config.SupportedFormats, ", ")))
	cmd.Flags().StringP("namespace", "n", "", "Resource namespace")
	cmd.Flags().BoolP("watch", "w", false, "Watch for changes")
	cmd.Flags().BoolP("raw", "r", false, "Output raw API response")
}

func (g *Generator) addStringFlag(cmd *cobra.Command, name, shorthand string, param *openapi.Parameter, description string) {

	if len(param.Enum) > 0 {
		allowed := make([]string, len(param.Enum))
		for i, v := range param.Enum {
			allowed[i] = fmt.Sprintf("%v", v)
		}
		description = fmt.Sprintf("%s (allowed: %s)", description, strings.Join(allowed, ", "))

		defaultVal := ""
		if param.Default != nil {
			defaultVal = param.Default.(string)
		}
		cmd.Flags().StringP(name, shorthand, defaultVal, description)
		return
	}

	defaultVal := ""
	if param.Default != nil {
		defaultVal = param.Default.(string)
	}
	cmd.Flags().StringP(name, shorthand, defaultVal, description)
}

func (g *Generator) buildUsage(name string, op *openapi.Operation) string {
	var args []string

	for _, param := range op.Parameters {
		if param.In == "path" {
			placeholder := fmt.Sprintf("<%s>", param.Name)
			if !param.Required {
				placeholder = fmt.Sprintf("[%s]", placeholder)
			}
			args = append(args, placeholder)
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf("%s %s [flags]", name, strings.Join(args, " "))
	}
	return fmt.Sprintf("%s [flags]", name)
}

func (g *Generator) buildLongDescription(op *openapi.Operation) string {
	var b strings.Builder

	if op.Description != "" {
		b.WriteString(op.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("API Endpoint:\n")
	fmt.Fprintf(&b, "  %s %s\n\n", op.Method, op.Path)

	if len(op.Parameters) > 0 {
		b.WriteString("Parameters:\n")
		for _, param := range op.Parameters {
			required := ""
			if param.Required {
				required = " (required)"
			}
			fmt.Fprintf(&b, "  %s%s: %s\n", param.Name, required, param.Description)
		}
	}

	return b.String()
}

func (g *Generator) buildExample(name string, op *openapi.Operation) string {
	var examples []string

	resourceName := g.inferResourceName(op)
	base := fmt.Sprintf("muster %s %s", resourceName, name)

	for _, param := range op.Parameters {
		if param.In == "path" && param.Required {
			base += fmt.Sprintf(" <%s>", param.Name)
		}
	}

	examples = append(examples, fmt.Sprintf("  # Basic usage\n  %s", base))

	if g.hasQueryParams(op) {
		examples = append(examples, fmt.Sprintf("\n  # With filters\n  %s --namespace production", base))
	}

	examples = append(examples, fmt.Sprintf("\n  # JSON output\n  %s --output json", base))

	return strings.Join(examples, "\n")
}

func (g *Generator) inferResourceName(op *openapi.Operation) string {
	parts := strings.Split(op.Path, "/")
	for _, part := range parts {
		if part != "" && !strings.HasPrefix(part, "{") {
			return strings.ToLower(part)
		}
	}
	return "resource"
}

func (g *Generator) hasQueryParams(op *openapi.Operation) bool {
	for _, param := range op.Parameters {
		if param.In == "query" {
			return true
		}
	}
	return false
}

func (g *Generator) inferShorthand(name string) string {
	shorthandMap := map[string]string{
		"namespace": "n",
		"output":    "o",
		"format":    "f",
		"selector":  "l",
		"all":       "a",
		"watch":     "w",
		"follow":    "f",
		"help":      "h",
		"config":    "c",
		"verbose":   "v",
		"yes":       "y",
		"timeout":   "t",
	}

	normalized := g.normalizeFlagName(name)
	if sh, ok := shorthandMap[normalized]; ok {
		return sh
	}
	return ""
}

func (g *Generator) normalizeFlagName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func (g *Generator) getCommandName(op *openapi.Operation) string {
	if custom, ok := g.config.CustomMappings[op.ID]; ok {
		return custom
	}

	if op.ID != "" {
		return g.normalizeOperationID(op.ID)
	}

	return g.operationName(op.Method, op.Path)
}

func (g *Generator) operationName(method, path string) string {
	switch g.config.OperationStyle {
	case StyleHTTP:
		resource := g.extractResourceFromPath(path)
		return fmt.Sprintf("%s-%s", strings.ToLower(method), resource)
	case StyleFlat:
		resource := g.extractResourceFromPath(path)
		return fmt.Sprintf("%s-%s", g.httpAction(method), resource)
	default:
		return g.crudName(method, path)
	}
}

func (g *Generator) crudName(method, path string) string {
	resource := g.extractResourceFromPath(path)
	singular := singularize(resource)

	switch strings.ToUpper(method) {
	case http.MethodGet:
		if strings.Contains(path, "{") {
			return fmt.Sprintf("get-%s", singular)
		}
		return fmt.Sprintf("list-%s", resource)
	case http.MethodPost:
		return fmt.Sprintf("create-%s", singular)
	case http.MethodPut, http.MethodPatch:
		return fmt.Sprintf("update-%s", singular)
	case http.MethodDelete:
		return fmt.Sprintf("delete-%s", singular)
	default:
		return strings.ToLower(method)
	}
}

func (g *Generator) extractResourceFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		return g.normalizeName(parts[0])
	}
	return "resource"
}

func (g *Generator) httpAction(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		return "get"
	case http.MethodPost:
		return "post"
	case http.MethodPut:
		return "put"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func (g *Generator) normalizeName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func (g *Generator) normalizeOperationID(id string) string {
	var result []rune
	for i, r := range id {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

func (g *Generator) createRunHandler(op *openapi.Operation) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		builder := request.NewBuilder(g.config.BaseURL, op.Path, op.Method)

		pathParams := g.extractPathParams(op.Path)
		for i, param := range pathParams {
			if i < len(args) {
				builder.WithPathParam(param, args[i])
			} else {
				return fmt.Errorf("missing required argument: %s", param)
			}
		}

		for _, param := range op.Parameters {
			if param.In == "query" {
				value, err := g.getFlagValue(cmd, param.Name, param.Type)
				if err != nil {
					return fmt.Errorf("get flag %s: %w", param.Name, err)
				}
				// Check if flag was actually set
				flagName := g.normalizeFlagName(param.Name)
				if cmd.Flag(flagName) != nil && cmd.Flag(flagName).Changed {
					builder.WithQueryParam(param.Name, fmt.Sprintf("%v", value))
				}
			}
		}

		for _, param := range op.Parameters {
			if param.In == "header" {
				value, _ := cmd.Flags().GetString(g.normalizeFlagName(param.Name))
				if value != "" {
					builder.WithHeader(param.Name, value)
				}
			}
		}

		resp, err := builder.Execute(ctx)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}

		// Log response status
		logger := log.FromContext(ctx)
		logger.Info().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Msg("HTTP response received")

		return nil
	}
}

func (g *Generator) getFlagValue(cmd *cobra.Command, name, paramType string) (interface{}, error) {
	flagName := g.normalizeFlagName(name)

	switch paramType {
	case "string":
		return cmd.Flags().GetString(flagName)
	case "integer":
		return cmd.Flags().GetInt(flagName)
	case "number":
		return cmd.Flags().GetFloat64(flagName)
	case "boolean":
		return cmd.Flags().GetBool(flagName)
	case "array":
		return cmd.Flags().GetStringSlice(flagName)
	default:
		return cmd.Flags().GetString(flagName)
	}
}

func (g *Generator) extractPathParams(path string) []string {
	var params []string
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params = append(params, part[1:len(part)-1])
		}
	}
	return params
}

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	if strings.HasSuffix(s, "es") {
		return strings.TrimSuffix(s, "es")
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

type cachedCommand struct {
	SpecHash  string
	ExpiresAt time.Time
}

type CommandCache struct {
	path     string
	metadata *cachedCommand
	mu       sync.RWMutex
	ttl      time.Duration
}

func NewCommandCache(path string) *CommandCache {
	cache := &CommandCache{
		path: path,
		ttl:  24 * time.Hour,
	}

	return cache
}

func (cc *CommandCache) Load(specPath string) (*cachedCommand, bool) {
	hash := computeHash(specPath)

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.metadata != nil && cc.metadata.SpecHash == hash && time.Now().Before(cc.metadata.ExpiresAt) {
		return cc.metadata, true
	}
	return nil, false
}

func (cc *CommandCache) Save(specPath string, commands []*cobra.Command) error {
	hash := computeHash(specPath)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.metadata = &cachedCommand{
		SpecHash:  hash,
		ExpiresAt: time.Now().Add(cc.ttl),
	}

	return cc.persist()
}

func (cc *CommandCache) persist() error {
	if cc.metadata == nil {
		return nil
	}

	data, err := json.Marshal(cc.metadata)
	if err != nil {
		return err
	}

	return os.WriteFile(cc.path, data, 0600)
}

func computeHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func InitializeDynamicCommands(rootCmd *cobra.Command, specPath string) error {
	cache := NewCommandCache(filepath.Join(os.TempDir(), "muster-cache-commands.json"))

	_, cached := cache.Load(specPath)

	if !cached {
		loader := openapi3.NewLoader()
		doc, err := loader.LoadFromFile(specPath)
		if err != nil {
			return fmt.Errorf("load spec: %w", err)
		}

		config := Config{
			AppName:          "openapi-cli",
			AppDescription:   "Dynamic CLI from OpenAPI specs",
			Version:          "1.0.0",
			NamingStrategy:   NamingKebab,
			OperationStyle:   StyleCRUD,
			DefaultFormat:    "table",
			SupportedFormats: []string{"table", "json", "yaml"},
		}

		generator, err := NewGenerator(config)
		if err != nil {
			return fmt.Errorf("create generator: %w", err)
		}

		commands, err := generator.GenerateCommands(doc)
		if err != nil {
			return fmt.Errorf("generate commands: %w", err)
		}

		for _, cmd := range commands {
			rootCmd.AddCommand(cmd)
		}

		return cache.Save(specPath, commands)
	}

	return nil
}
