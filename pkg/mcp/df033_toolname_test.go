package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wojons/muster/pkg/mcp/handlers"
)

// mcpToolNameConvention is the conventional MCP Tool.name pattern: strict
// registries/UIs reject names outside it (DF-033).
var mcpToolNameConvention = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// df033Spec mirrors the dogfooded shape: a mix of operations with a clean
// operationId, operations with NO operationId whose generated name embeds the
// path template (get_tasks_{task_id}, …), and one operationId that itself
// embeds a {curly} template.
func df033Spec() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "DF033", "version": "1.0.0"},
  "servers": [{"url": "http://api.example.com"}],
  "paths": {
    "/health": {
      "get": {
        "operationId": "getHealth",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/tasks": {
      "get":  {"responses": {"200": {"description": "OK"}}},
      "post": {"responses": {"200": {"description": "OK"}}}
    },
    "/tasks/{task_id}": {
      "parameters": [
        {"name": "task_id", "in": "path", "required": true, "schema": {"type": "string"}}
      ],
      "get":    {"responses": {"200": {"description": "OK"}}},
      "delete": {"responses": {"200": {"description": "OK"}}}
    },
    "/projects/{project_id}": {
      "parameters": [
        {"name": "project_id", "in": "path", "required": true, "schema": {"type": "string"}}
      ],
      "get": {
        "operationId": "get_projects_{project_id}",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/projects/{project_id}/tasks": {
      "parameters": [
        {"name": "project_id", "in": "path", "required": true, "schema": {"type": "string"}}
      ],
      "get":  {"responses": {"200": {"description": "OK"}}},
      "post": {"responses": {"200": {"description": "OK"}}}
    }
  }
}`
}

// toolByName indexes converted tools for assertions.
func toolByName(t *testing.T, tools []OpenAPITool) map[string]OpenAPITool {
	t.Helper()
	byName := make(map[string]OpenAPITool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	return byName
}

// DF-033: every generated tool name must match ^[a-zA-Z0-9_-]{1,128}$ even
// when the operationId (or the derived fallback name) embeds a path template.
func TestDF033_ToolNamesMatchMCPConvention(t *testing.T) {
	doc := loadSpec(t, df033Spec())
	conv := NewConverter("")
	tools, configs, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools generated")
	}

	templated := 0
	for _, tool := range tools {
		if !mcpToolNameConvention.MatchString(tool.Name) {
			t.Errorf("tool name %q does not match the MCP convention %s", tool.Name, mcpToolNameConvention)
		}
		if len(tool.Name) > 128 {
			t.Errorf("tool name %q exceeds 128 characters (%d)", tool.Name, len(tool.Name))
		}
		if !strings.ContainsAny(tool.Name, "{}") {
			if template := tool.Annotations[AnnotationToolNameTemplate]; strings.ContainsAny(template, "{}") {
				templated++
			}
		}
	}
	// Non-vacuity: the fixture must exercise the templated defect class, and
	// the annotations must show those tools WERE templated before sanitizing.
	if templated < 5 {
		t.Fatalf("expected the fixture to exercise >=5 templated tools, got %d", templated)
	}

	// Routing configs are keyed by the same sanitized names.
	if len(configs) != len(tools) {
		t.Fatalf("configs=%d tools=%d", len(configs), len(tools))
	}
	for _, tool := range tools {
		if _, ok := configs[tool.Name]; !ok {
			t.Errorf("no execution config for sanitized name %q", tool.Name)
		}
		// Verbose evidence: the served name and the template it came from.
		t.Logf("tool %-32q <- template %-32q (path %s)", tool.Name, tool.Annotations[AnnotationToolNameTemplate], tool.Path)
	}
}

// DF-033: sanitization must not collapse two operations onto one name.
func TestDF033_SanitizedToolNamesAreUnique(t *testing.T) {
	doc := loadSpec(t, df033Spec())
	conv := NewConverter("")
	tools, configs, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}

	seen := map[string]int{}
	for _, tool := range tools {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("sanitized tool name %q appears %d times", name, count)
		}
	}
	if len(configs) != len(tools) {
		t.Errorf("configs=%d tools=%d (a collision would lose a config)", len(configs), len(tools))
	}
}

// DF-033: a client that renames a tool must be able to recover the original
// template and its path-parameter mapping from the tool object itself.
func TestDF033_PathParamMappingRecoverableFromAnnotations(t *testing.T) {
	doc := loadSpec(t, df033Spec())
	conv := NewConverter("")
	tools, _, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	byName := toolByName(t, tools)

	// The two exemplars named in the board row.
	cases := []struct {
		name     string
		template string
		path     string
		method   string
		params   string
	}{
		{"get_tasks_task_id", "get_tasks_{task_id}", "/tasks/{task_id}", "GET", "task_id"},
		{"get_projects_project_id_tasks", "get_projects_{project_id}_tasks", "/projects/{project_id}/tasks", "GET", "project_id"},
	}
	for _, tc := range cases {
		tool, ok := byName[tc.name]
		if !ok {
			t.Fatalf("tool %q missing; names=%v", tc.name, toolNames(tools))
		}
		if got := tool.Annotations[AnnotationToolNameTemplate]; got != tc.template {
			t.Errorf("%s: template annotation = %q, want %q", tc.name, got, tc.template)
		}
		if got := tool.Annotations[AnnotationOperationPath]; got != tc.path {
			t.Errorf("%s: path annotation = %q, want %q", tc.name, got, tc.path)
		}
		if got := tool.Annotations[AnnotationOperationMethod]; got != tc.method {
			t.Errorf("%s: method annotation = %q, want %q", tc.name, got, tc.method)
		}
		if got := tool.Annotations[AnnotationPathParams]; got != tc.params {
			t.Errorf("%s: path-params annotation = %q, want %q", tc.name, got, tc.params)
		}
		// Losslessness: the sanitized name is exactly the original template
		// with the template delimiters removed, so a client-side rename can be
		// reversed from the tool object alone.
		if want := stripTemplateDelimiters(tc.template); tc.name != want {
			t.Errorf("%s: name is not the reversible form %q of template %q", tc.name, want, tc.template)
		}
	}

	// Every templated op must carry the mapping keys.
	for _, tool := range tools {
		params := extractPathParams(tool.Path)
		if len(params) == 0 {
			continue
		}
		if got := tool.Annotations[AnnotationOperationPath]; got != tool.Path {
			t.Errorf("%s: path annotation = %q, want %q", tool.Name, got, tool.Path)
		}
		if got := tool.Annotations[AnnotationPathParams]; got != strings.Join(params, ",") {
			t.Errorf("%s: path-params annotation = %q, want %q", tool.Name, got, strings.Join(params, ","))
		}
		template := tool.Annotations[AnnotationToolNameTemplate]
		for _, p := range params {
			if !strings.Contains(template, "{"+p+"}") {
				t.Errorf("%s: template %q does not expose path param %q for recovery", tool.Name, template, p)
			}
		}
	}
}

// DF-033: a call by the sanitized name still resolves to the right operation
// and injects the path parameter (the pre-fix name no longer resolves).
func TestDF033_SanitizedNamesRouteWithPathParams(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	doc := loadSpec(t, df033Spec())
	conv := NewConverter(server.URL)
	tools, configs, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	if _, ok := configs["get_tasks_task_id"]; !ok {
		t.Fatalf("no config for sanitized name; names=%v", toolNames(tools))
	}

	exec := NewOpenAPIExecutor(configs)
	if _, err := exec.Execute(context.Background(), "get_tasks_task_id", map[string]interface{}{"task_id": "42"}); err != nil {
		t.Fatalf("Execute(get_tasks_task_id): %v", err)
	}
	if gotPath != "/tasks/42" {
		t.Errorf("request path = %q, want /tasks/42", gotPath)
	}

	// The pre-fix templated name must not be callable any more — the served
	// name and the routable name are the same (sanitized) string.
	if _, err := exec.Execute(context.Background(), "get_tasks_{task_id}", map[string]interface{}{"task_id": "42"}); err == nil {
		t.Error("Execute(get_tasks_{task_id}) succeeded; templated name is still routable")
	}
}

// df033CollisionSpec declares two operations whose sanitized names collide
// (get_items_{item_id} and get_items_item_id both sanitize to
// get_items_item_id).
func df033CollisionSpec() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "DF033-collision", "version": "1.0.0"},
  "servers": [{"url": "http://api.example.com"}],
  "paths": {
    "/items/{item_id}": {
      "get": {
        "operationId": "get_items_{item_id}",
        "parameters": [{"name": "item_id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/items/{item_id}/history": {
      "get": {
        "operationId": "get_items_item_id",
        "parameters": [{"name": "item_id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

// DF-033: when sanitizing collides, disambiguation must be deterministic and
// collision-free — never dependent on kin-openapi's map iteration order.
func TestDF033_SanitizedNameCollisionsDisambiguatedDeterministically(t *testing.T) {
	doc := loadSpec(t, df033CollisionSpec())
	conv := NewConverter("")
	tools, configs, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	nameToPath := map[string]string{}
	for _, tool := range tools {
		if !mcpToolNameConvention.MatchString(tool.Name) {
			t.Errorf("tool name %q does not match the MCP convention", tool.Name)
		}
		if strings.Contains(tool.Name, "{") || strings.Contains(tool.Name, "}") {
			t.Errorf("tool name %q still contains a template delimiter", tool.Name)
		}
		if prev, ok := nameToPath[tool.Name]; ok {
			t.Errorf("name %q used twice (paths %q and %q)", tool.Name, prev, tool.Path)
		}
		nameToPath[tool.Name] = tool.Path
		if _, ok := configs[tool.Name]; !ok {
			t.Errorf("no execution config for %q", tool.Name)
		}
		if tool.Annotations[AnnotationToolNameTemplate] == "" {
			t.Errorf("%s: missing template annotation", tool.Name)
		}
	}

	// Determinism: converting the same document again yields the identical
	// name -> path mapping, whichever order the paths map is walked in.
	for i := 0; i < 8; i++ {
		again, _, err := NewConverter("").ConvertToTools(loadSpec(t, df033CollisionSpec()))
		if err != nil {
			t.Fatalf("ConvertToTools (run %d): %v", i, err)
		}
		if len(again) != len(nameToPath) {
			t.Fatalf("run %d: tool count %d, want %d", i, len(again), len(nameToPath))
		}
		for _, tool := range again {
			if want, ok := nameToPath[tool.Name]; !ok || want != tool.Path {
				t.Fatalf("run %d: name %q -> %q, want stable mapping %v", i, tool.Name, tool.Path, nameToPath)
			}
		}
	}
}

// DF-033: over-long generated names must be truncated into the pattern rather
// than served at 200 characters.
func TestDF033_LongGeneratedNamesBoundedByConvention(t *testing.T) {
	longID := strings.Repeat("segment_", 30) // 240 chars
	spec := `{
  "openapi": "3.0.0",
  "info": {"title": "DF033-long", "version": "1.0.0"},
  "paths": {
    "/a/{id}": {
      "get": {
        "operationId": "` + longID + `",
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
	doc := loadSpec(t, spec)
	tools, _, err := NewConverter("").ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	name := tools[0].Name
	if !mcpToolNameConvention.MatchString(name) {
		t.Errorf("name %q (len %d) does not match the MCP convention", name, len(name))
	}
	if tools[0].Annotations[AnnotationToolNameTemplate] != longID {
		t.Errorf("template annotation lost the original name")
	}
}

// DF-033: the served tools/list surface (cmd/mcp-server wires the registry
// produced here) carries both the sanitized names and the recovery mapping.
func TestDF033_ToolsListServesSanitizedNamesAndAnnotations(t *testing.T) {
	doc := loadSpec(t, df033Spec())
	conv := NewConverter("")
	tools, configs, err := conv.ConvertToTools(doc)
	if err != nil {
		t.Fatalf("ConvertToTools: %v", err)
	}
	registry := NewOpenAPIRegistry(tools, configs)
	lister := handlers.NewListToolsHandler(registry)

	raw, err := lister.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}

	var payload struct {
		Tools []struct {
			Name        string            `json:"name"`
			Annotations map[string]string `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal tools/list payload: %v", err)
	}
	if len(payload.Tools) != len(tools) {
		t.Fatalf("served %d tools, want %d", len(payload.Tools), len(tools))
	}

	served := 0
	for _, tool := range payload.Tools {
		if !mcpToolNameConvention.MatchString(tool.Name) {
			t.Errorf("served tool name %q violates the MCP convention", tool.Name)
		}
		if strings.ContainsAny(tool.Name, "{}") {
			t.Errorf("served tool name %q still contains template delimiters", tool.Name)
		}
		if template := tool.Annotations[AnnotationToolNameTemplate]; template != "" {
			if !strings.ContainsAny(template, "{}") {
				t.Errorf("%s: template annotation %q has no delimiters", tool.Name, template)
			}
			if tool.Annotations[AnnotationOperationPath] == "" {
				t.Errorf("%s: served tool has no path annotation", tool.Name)
			}
			served++
		}
	}
	if served == 0 {
		t.Fatal("no served tool carried a recovery mapping")
	}
}

// stripTemplateDelimiters applies DF-033's reversal rule: the sanitized name is
// the template name with the { } delimiters removed.
func stripTemplateDelimiters(name string) string {
	return strings.NewReplacer("{", "", "}", "").Replace(name)
}

func toolNames(tools []OpenAPITool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
