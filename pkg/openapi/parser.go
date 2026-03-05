package openapi

import (
	"context"
	"fmt"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

type ParseOptions struct {
	BaseURL        string
	SkipValidation bool
	MaxDepth       int
	AllowCircular  bool
}

func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		MaxDepth:      10,
		AllowCircular: true,
	}
}

type ParseResult struct {
	Document     *openapi3.T
	Version      string
	Warnings     []Warning
	ParseTime    time.Duration
	ResolveTime  time.Duration
	ValidateTime time.Duration
}

type Warning struct {
	Type    string
	Message string
	Path    string
}

type Parser struct {
	loader *openapi3.Loader
}

func NewParser() *Parser {
	return &Parser{
		loader: openapi3.NewLoader(),
	}
}

func (p *Parser) Parse(ctx context.Context, data []byte, opts ParseOptions) (*ParseResult, error) {
	start := time.Now()

	doc, err := p.loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}

	if !opts.SkipValidation {
		if err := doc.Validate(ctx); err != nil {
			return nil, fmt.Errorf("validate spec: %w", err)
		}
	}

	parseTime := time.Since(start)

	var warnings []Warning
	if doc.Info != nil && doc.Info.Title == "" {
		warnings = append(warnings, Warning{
			Type:    "semantic",
			Message: "info title is required but missing",
		})
	}

	return &ParseResult{
		Document:     doc,
		Version:      doc.Info.Version,
		Warnings:     warnings,
		ParseTime:    parseTime,
		ResolveTime:  0,
		ValidateTime: 0,
	}, nil
}

type Operation struct {
	ID          string
	Summary     string
	Description string
	Method      string
	Path        string
	Tags        []string
	Parameters  Parameters
	RequestBody *RequestBody
}

type Parameter struct {
	Name        string
	In          string
	Required    bool
	Schema      *openapi3.Schema
	Description string
	Type        string
	Default     interface{}
	Enum        []interface{}
}

type Parameters []*Parameter

type RequestBody struct {
	Content     map[string]MediaType
	Required    bool
	Description string
}

type MediaType struct {
	Schema *openapi3.Schema
}

func (p *Parser) ExtractOperations(doc *openapi3.T) ([]*Operation, error) {
	var operations []*Operation

	for path, pathItem := range doc.Paths.Map() {
		methods := map[string]*openapi3.Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"PATCH":   pathItem.Patch,
			"DELETE":  pathItem.Delete,
			"HEAD":    pathItem.Head,
			"OPTIONS": pathItem.Options,
			"TRACE":   pathItem.Trace,
		}

		for method, operation := range methods {
			if operation == nil {
				continue
			}

			ops := []*Operation{
				{
					ID:          operation.OperationID,
					Summary:     operation.Summary,
					Description: operation.Description,
					Method:      method,
					Path:        path,
					Tags:        operation.Tags,
					Parameters:  p.extractParameters(operation),
					RequestBody: p.extractRequestBody(operation),
				},
			}
			operations = append(operations, ops...)
		}
	}

	return operations, nil
}

func (p *Parser) extractParameters(operation *openapi3.Operation) Parameters {
	var params Parameters

	for _, paramRef := range operation.Parameters {
		if paramRef.Ref != "" {
			continue
		}

		param := paramRef.Value

		pType := ""
		if param.Schema != nil && param.Schema.Value != nil {
			typeSlice := *param.Schema.Value.Type
			if len(typeSlice) > 0 {
				pType = typeSlice[0]
			}
		}

		pDefault := interface{}(nil)
		if param.Schema != nil && param.Schema.Value != nil {
			pDefault = param.Schema.Value.Default
		}

		pEnum := []interface{}(nil)
		if param.Schema != nil && param.Schema.Value != nil {
			pEnum = param.Schema.Value.Enum
		}

		params = append(params, &Parameter{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Schema:      param.Schema.Value,
			Description: param.Description,
			Type:        pType,
			Default:     pDefault,
			Enum:        pEnum,
		})
	}

	return params
}

func (p *Parser) extractRequestBody(operation *openapi3.Operation) *RequestBody {
	if operation.RequestBody == nil {
		return nil
	}

	if operation.RequestBody.Ref != "" {
		return nil
	}

	rb := &RequestBody{
		Required:    operation.RequestBody.Value.Required,
		Description: operation.RequestBody.Value.Description,
		Content:     make(map[string]MediaType),
	}

	for contentType, mediaType := range operation.RequestBody.Value.Content {
		rb.Content[contentType] = MediaType{
			Schema: mediaType.Schema.Value,
		}
	}

	return rb
}
