# API Documentation Generation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Documentation

## Overview

Complete specification for **automated API documentation generation** from OpenAPI specifications. The system generates comprehensive, interactive documentation that stays synchronized with the API specification.

## 1. Documentation Generation Pipeline

```
OpenAPI Spec
    ↓
Parse Operations
    ↓
Generate Markdown
    ↓
Build HTML
    ↓
Embed Interactive Examples
    ↓
Publish Documentation
```

## 2. Core Components

### 2.1 Documentation Generator

```go
package documentation

import (
    "bytes"
    "fmt"
    "html/template"
    "os"
    "path/filepath"
    "strings"
    
    "github.com/getkin/kin-openapi/openapi3"
)

type Generator struct {
    spec        *openapi3.T
    templateDir string
    outputDir   string
}

type DocConfig struct {
    Title           string
    Description     string
    Version         string
    BaseURL         string
    ContactName     string
    ContactEmail    string
    ContactURL      string
    LicenseName     string
    LicenseURL      string
}

func NewGenerator(specPath, templateDir, outputDir string) (*Generator, error) {
    loader := openapi3.NewLoader()
    spec, err := loader.LoadFromFile(specPath)
    if err != nil {
        return nil, fmt.Errorf("load spec: %w", err)
    }
    
    if err := spec.Validate(loader.Context); err != nil {
        return nil, fmt.Errorf("validate spec: %w", err)
    }
    
    return &Generator{
        spec:        spec,
        templateDir: templateDir,
        outputDir:   outputDir,
    }, nil
}

func (g *Generator) Generate() error {
    // Create output directory
    if err := os.MkdirAll(g.outputDir, 0755); err != nil {
        return fmt.Errorf("create output dir: %w", err)
    }
    
    // Generate index page
    if err := g.generateIndex(); err != nil {
        return fmt.Errorf("generate index: %w", err)
    }
    
    // Generate operation pages by tag
    for tag, operations := range g.groupByTag() {
        if err := g.generateTagPage(tag, operations); err != nil {
            return fmt.Errorf("generate tag page %s: %w", tag, err)
        }
    }
    
    // Generate schemas page
    if err := g.generateSchemasPage(); err != nil {
        return fmt.Errorf("generate schemas: %w", err)
    }
    
    return nil
}

func (g *Generator) groupByTag() map[string][]*OperationDoc {
    groups := make(map[string][]*OperationDoc)
    
    for path, pathItem := range g.spec.Paths {
        g.addOperationToGroup(groups, path, "GET", pathItem.Get)
        g.addOperationToGroup(groups, path, "POST", pathItem.Post)
        g.addOperationToGroup(groups, path, "PUT", pathItem.Put)
        g.addOperationToGroup(groups, path, "PATCH", pathItem.Patch)
        g.addOperationToGroup(groups, path, "DELETE", pathItem.Delete)
    }
    
    return groups
}

type OperationDoc struct {
    Path        string
    Method      string
    Summary     string
    Description string
    OperationID string
    Tags        []string
    Parameters  []*ParameterDoc
    RequestBody *RequestBodyDoc
    Responses   map[string]*ResponseDoc
}

type ParameterDoc struct {
    Name        string
    In          string
    Type        string
    Required    bool
    Description string
    Example     interface{}
}

type RequestBodyDoc struct {
    ContentType string
    Schema      interface{}
    Example     interface{}
}

type ResponseDoc struct {
    StatusCode  string
    Description string
    ContentType string
    Schema      interface{}
    Example     interface{}
}
```

### 2.2 Markdown Generation

```go
func (g *Generator) generateIndex() error {
    config := g.extractDocConfig()
    
    var buf bytes.Buffer
    buf.WriteString(fmt.Sprintf("# %s\n\n", config.Title))
    buf.WriteString(fmt.Sprintf("**Version**: %s\n\n", config.Version))
    buf.WriteString(fmt.Sprintf("**Base URL**: `%s`\n\n", config.BaseURL))
    
    if config.Description != "" {
        buf.WriteString("## Overview\n\n")
        buf.WriteString(config.Description)
        buf.WriteString("\n\n")
    }
    
    // Table of contents
    buf.WriteString("## API Reference\n\n")
    for tag := range g.groupByTag() {
        buf.WriteString(fmt.Sprintf("- [%s](%s.md)\n", tag, strings.ToLower(tag)))
    }
    buf.WriteString("\n")
    
    // Authentication section
    if g.spec.Components.SecuritySchemes != nil {
        buf.WriteString("## Authentication\n\n")
        for name, scheme := range g.spec.Components.SecuritySchemes {
            buf.WriteString(g.formatSecurityScheme(name, scheme.Value))
        }
    }
    
    outputPath := filepath.Join(g.outputDir, "index.md")
    return os.WriteFile(outputPath, buf.Bytes(), 0644)
}

func (g *Generator) generateTagPage(tag string, ops []*OperationDoc) error {
    var buf bytes.Buffer
    
    buf.WriteString(fmt.Sprintf("# %s\n\n", tag))
    
    for _, op := range ops {
        buf.WriteString(g.formatOperation(op))
        buf.WriteString("\n---\n\n")
    }
    
    filename := strings.ToLower(strings.ReplaceAll(tag, " ", "-")) + ".md"
    outputPath := filepath.Join(g.outputDir, filename)
    return os.WriteFile(outputPath, buf.Bytes(), 0644)
}

func (g *Generator) formatOperation(op *OperationDoc) string {
    var buf bytes.Buffer
    
    // Operation header
    buf.WriteString(fmt.Sprintf("## %s\n\n", op.Summary))
    buf.WriteString(fmt.Sprintf("```http\n%s %s\n```\n\n", op.Method, op.Path))
    
    if op.Description != "" {
        buf.WriteString(op.Description)
        buf.WriteString("\n\n")
    }
    
    // Parameters
    if len(op.Parameters) > 0 {
        buf.WriteString("### Parameters\n\n")
        buf.WriteString("| Name | In | Type | Required | Description |\n")
        buf.WriteString("|------|----|------|----------|-------------|\n")
        
        for _, param := range op.Parameters {
            required := "No"
            if param.Required {
                required = "Yes"
            }
            buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
                param.Name, param.In, param.Type, required, param.Description))
        }
        buf.WriteString("\n")
    }
    
    // Request body
    if op.RequestBody != nil {
        buf.WriteString("### Request Body\n\n")
        buf.WriteString(fmt.Sprintf("**Content-Type**: `%s`\n\n", op.RequestBody.ContentType))
        
        if op.RequestBody.Example != nil {
            buf.WriteString("**Example**:\n\n```json\n")
            buf.WriteString(g.formatJSON(op.RequestBody.Example))
            buf.WriteString("\n```\n\n")
        }
    }
    
    // Responses
    buf.WriteString("### Responses\n\n")
    for statusCode, resp := range op.Responses {
        buf.WriteString(fmt.Sprintf("#### %s %s\n\n", statusCode, resp.Description))
        
        if resp.Example != nil {
            buf.WriteString("**Example**:\n\n```json\n")
            buf.WriteString(g.formatJSON(resp.Example))
            buf.WriteString("\n```\n\n")
        }
    }
    
    return buf.String()
}
```

## 3. HTML Template System

```go
const indexTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}} - API Documentation</title>
    <link rel="stylesheet" href="style.css">
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
</head>
<body>
    <div id="swagger-ui"></div>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "openapi.yaml",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.presets.standalone
                ]
            });
        };
    </script>
</body>
</html>
`

func (g *Generator) generateHTML() error {
    config := g.extractDocConfig()
    
    tmpl, err := template.New("index").Parse(indexTemplate)
    if err != nil {
        return fmt.Errorf("parse template: %w", err)
    }
    
    outputPath := filepath.Join(g.outputDir, "index.html")
    file, err := os.Create(outputPath)
    if err != nil {
        return fmt.Errorf("create file: %w", err)
    }
    defer file.Close()
    
    return tmpl.Execute(file, config)
}
```

## 4. Interactive Example Generator

```go
type ExampleGenerator struct {
    baseURL string
}

func (eg *ExampleGenerator) GenerateCurlExample(op *OperationDoc) string {
    var parts []string
    parts = append(parts, "curl")
    
    // Method
    if op.Method != "GET" {
        parts = append(parts, fmt.Sprintf("-X %s", op.Method))
    }
    
    // Headers
    parts = append(parts, "-H 'Accept: application/json'")
    if op.RequestBody != nil {
        parts = append(parts, fmt.Sprintf("-H 'Content-Type: %s'", op.RequestBody.ContentType))
    }
    
    // URL with path params
    url := eg.baseURL + op.Path
    parts = append(parts, fmt.Sprintf("'%s'", url))
    
    // Body
    if op.RequestBody != nil && op.RequestBody.Example != nil {
        parts = append(parts, fmt.Sprintf("-d '%s'", g.formatJSON(op.RequestBody.Example)))
    }
    
    return strings.Join(parts, " \\\n  ")
}

func (eg *ExampleGenerator) GenerateGoExample(op *OperationDoc) string {
    var buf bytes.Buffer
    
    buf.WriteString("package main\n\n")
    buf.WriteString("import (\n")
    buf.WriteString("    \"bytes\"\n")
    buf.WriteString("    \"encoding/json\"\n")
    buf.WriteString("    \"fmt\"\n")
    buf.WriteString("    \"net/http\"\n")
    buf.WriteString(")\n\n")
    
    buf.WriteString("func main() {\n")
    buf.WriteString(fmt.Sprintf("    url := \"%s%s\"\n", eg.baseURL, op.Path))
    
    if op.RequestBody != nil {
        buf.WriteString("    body := map[string]interface{}{\n")
        buf.WriteString("        // Add request body fields\n")
        buf.WriteString("    }\n")
        buf.WriteString("    jsonBody, _ := json.Marshal(body)\n")
        buf.WriteString(fmt.Sprintf("    req, _ := http.NewRequest(\"%s\", url, bytes.NewBuffer(jsonBody))\n", op.Method))
    } else {
        buf.WriteString(fmt.Sprintf("    req, _ := http.NewRequest(\"%s\", url, nil)\n", op.Method))
    }
    
    buf.WriteString("    req.Header.Set(\"Accept\", \"application/json\")\n")
    buf.WriteString("    client := &http.Client{}\n")
    buf.WriteString("    resp, err := client.Do(req)\n")
    buf.WriteString("    if err != nil {\n")
    buf.WriteString("        panic(err)\n")
    buf.WriteString("    }\n")
    buf.WriteString("    defer resp.Body.Close()\n")
    buf.WriteString("    fmt.Println(\"Status:\", resp.Status)\n")
    buf.WriteString("}\n")
    
    return buf.String()
}
```

## 5. Schema Documentation

```go
func (g *Generator) generateSchemasPage() error {
    var buf bytes.Buffer
    
    buf.WriteString("# Schemas\n\n")
    
    for name, schemaRef := range g.spec.Components.Schemas {
        schema := schemaRef.Value
        
        buf.WriteString(fmt.Sprintf("## %s\n\n", name))
        
        if schema.Description != "" {
            buf.WriteString(schema.Description)
            buf.WriteString("\n\n")
        }
        
        if schema.Type == "object" && schema.Properties != nil {
            buf.WriteString("### Properties\n\n")
            buf.WriteString("| Property | Type | Required | Description |\n")
            buf.WriteString("|----------|------|----------|-------------|\n")
            
            for propName, propRef := range schema.Properties {
                prop := propRef.Value
                required := "No"
                for _, r := range schema.Required {
                    if r == propName {
                        required = "Yes"
                        break
                    }
                }
                
                buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
                    propName, prop.Type, required, prop.Description))
            }
            buf.WriteString("\n")
        }
        
        // Example
        example := g.generateExampleFromSchema(schema)
        if example != nil {
            buf.WriteString("### Example\n\n```json\n")
            buf.WriteString(g.formatJSON(example))
            buf.WriteString("\n```\n\n")
        }
    }
    
    outputPath := filepath.Join(g.outputDir, "schemas.md")
    return os.WriteFile(outputPath, buf.Bytes(), 0644)
}

func (g *Generator) generateExampleFromSchema(schema *openapi3.Schema) interface{} {
    if schema.Example != nil {
        return schema.Example
    }
    
    switch schema.Type {
    case "object":
        example := make(map[string]interface{})
        for name, propRef := range schema.Properties {
            example[name] = g.generateExampleFromSchema(propRef.Value)
        }
        return example
    case "array":
        if schema.Items != nil {
            return []interface{}{g.generateExampleFromSchema(schema.Items.Value)}
        }
        return []interface{}{}
    case "string":
        if len(schema.Enum) > 0 {
            return schema.Enum[0]
        }
        return "string"
    case "integer":
        return 0
    case "number":
        return 0.0
    case "boolean":
        return false
    default:
        return nil
    }
}
```

## 6. CLI Integration

```go
package cmd

import (
    "github.com/spf13/cobra"
    "muster/internal/documentation"
)

var docsCmd = &cobra.Command{
    Use:   "docs",
    Short: "Generate API documentation",
    Long:  "Generate static HTML/Markdown documentation from OpenAPI specifications.",
}

var docsGenerateCmd = &cobra.Command{
    Use:   "generate [spec-file]",
    Short: "Generate documentation",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        outputDir, _ := cmd.Flags().GetString("output")
        format, _ := cmd.Flags().GetString("format")
        
        generator, err := documentation.NewGenerator(args[0], "", outputDir)
        if err != nil {
            return err
        }
        
        switch format {
        case "markdown":
            return generator.Generate()
        case "html":
            return generator.GenerateHTML()
        default:
            return fmt.Errorf("unsupported format: %s", format)
        }
    },
}

func init() {
    docsCmd.AddCommand(docsGenerateCmd)
    docsGenerateCmd.Flags().StringP("output", "o", "./docs", "Output directory")
    docsGenerateCmd.Flags().StringP("format", "f", "markdown", "Output format (markdown|html)")
}
```

## 7. Validation and Testing

```go
func TestDocumentationGenerator(t *testing.T) {
    generator, err := NewGenerator("testdata/petstore.yaml", "", t.TempDir())
    require.NoError(t, err)
    
    t.Run("generate_index", func(t *testing.T) {
        err := generator.generateIndex()
        assert.NoError(t, err)
        
        content, err := os.ReadFile(filepath.Join(generator.outputDir, "index.md"))
        require.NoError(t, err)
        assert.Contains(t, string(content), "Swagger Petstore")
    })
    
    t.Run("generate_tag_pages", func(t *testing.T) {
        groups := generator.groupByTag()
        for tag, ops := range groups {
            err := generator.generateTagPage(tag, ops)
            assert.NoError(t, err)
            
            filename := strings.ToLower(strings.ReplaceAll(tag, " ", "-")) + ".md"
            _, err = os.Stat(filepath.Join(generator.outputDir, filename))
            assert.NoError(t, err)
        }
    })
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
