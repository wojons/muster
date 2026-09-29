package mcp

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// pathItemSpec declares the path param ONLY at path-item level (task-forge shape).
func pathItemSpec() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "DF034", "version": "1.0.0"},
  "paths": {
    "/projects/{project_id}": {
      "parameters": [
        {"name": "project_id", "in": "path", "required": true, "schema": {"type": "string"}}
      ],
      "get": {
        "summary": "Get project",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/projects/{project_id}/tasks": {
      "parameters": [
        {"name": "project_id", "in": "path", "required": true, "schema": {"type": "string"}}
      ],
      "get": {
        "summary": "List tasks",
        "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

// DF-034 production path: Converter.ConvertToTools (cmd/mcp-server) must
// advertise path-item-level path params in inputSchema.properties AND required.
func TestConvertToTools_PathItemLevelParamsAdvertised(t *testing.T) {
	l := openapi3.NewLoader()
	doc, err := l.LoadFromData([]byte(pathItemSpec()))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	conv := NewConverter("https://api.example.com")
	tools, _, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	checked := 0
	for _, tool := range tools {
		// DF-033 sanitizes tool names, so a templated operation is selected by
		// its path template (recorded on the tool) rather than by a "{"
		// appearing in the name.
		if len(extractPathParams(tool.Path)) == 0 {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: bad schema: %v", tool.Name, err)
		}
		checked++
		if _, ok := schema.Properties["project_id"]; !ok {
			t.Errorf("tool %s: project_id missing from properties", tool.Name)
		}
		found := false
		for _, r := range schema.Required {
			if r == "project_id" {
				found = true
			}
		}
		if !found {
			t.Errorf("tool %s: project_id missing from required", tool.Name)
		}
	}
	if checked == 0 {
		t.Fatal("no templated tools exercised")
	}
}
