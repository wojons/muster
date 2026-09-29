package mcp

import (
	"encoding/json"

	"github.com/wojons/muster/pkg/mcp/handlers"
	"testing"
)

// specWithUnregisteredPathParam mirrors real specs where a path template
// parameter is NOT declared in `parameters` (the DF-034 defect class).
func specWithUnregisteredPathParam() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "DF034", "version": "1.0.0"},
  "paths": {
    "/projects/{project_id}": {
      "get": {
        "operationId": "getProject",
        "summary": "Get project",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/projects/{project_id}/tasks": {
      "get": {
        "operationId": "listProjectTasks",
        "summary": "List tasks",
        "parameters": [
          {"name": "limit", "in": "query", "schema": {"type": "integer"}}
        ],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

// DF-034: every tool whose name embeds a {param} template must advertise the
// path parameter in inputSchema.properties AND inputSchema.required, even when
// the spec parser did not surface it in op.Parameters.
func TestListTools_TemplatedToolsAdvertiseUndeclaredPathParams(t *testing.T) {
	spec := loadSpec(t, specWithUnregisteredPathParam())
	r, err := NewRegistry(spec, "https://api.example.com")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	tools := r.ListTools()
	byName := map[string]handlers.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	checked := 0
	for _, op := range r.Operations() {
		tool, ok := byName[generateCommandName(op)]
		if !ok {
			t.Fatalf("tool %s missing from ListTools", generateCommandName(op))
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: bad schema: %v", tool.Name, err)
		}
		for _, p := range extractPathParamsStr(op.Path) {
			checked++
			if _, ok := schema.Properties[p]; !ok {
				t.Errorf("tool %s: path param %q missing from properties", tool.Name, p)
			}
			found := false
			for _, req := range schema.Required {
				if req == p {
					found = true
				}
			}
			if !found {
				t.Errorf("tool %s: path param %q missing from required", tool.Name, p)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no templated tool params exercised the assertion")
	}
}

// Declared path params must still be advertised exactly once (no dupes in required).
func TestListTools_DeclaredPathParamsStillValidNoDupes(t *testing.T) {
	spec := loadSpec(t, petstoreSpec())
	r, err := NewRegistry(spec, "https://api.example.com")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	for _, tool := range r.ListTools() {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: bad schema: %v", tool.Name, err)
		}
		counts := map[string]int{}
		for _, req := range schema.Required {
			counts[req]++
			if counts[req] > 1 {
				t.Errorf("tool %s: %q appears %d times in required", tool.Name, req, counts[req])
			}
			if _, ok := schema.Properties[req]; !ok {
				t.Errorf("tool %s: required %q not in properties", tool.Name, req)
			}
		}
	}
}
