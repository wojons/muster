package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/wojons/muster/pkg/openapi"
)

// =============================================================================
// QA-MUSTER-041: MCP parity for form-encoded request bodies.
//
// Pre-fix, PostCustomers advertised inputSchema {properties:{}} while its spec
// declares urlencoded properties, and a tools/call carrying them serialized a
// JSON body — a media type the API does not accept. Both MCP surfaces
// (Converter.ConvertToTools → ToolExecutor, and OpenAPIRegistry.ListTools →
// ExecuteCommand) are pinned here.
// =============================================================================

func qa041MCPSpec() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "Billing", "version": "1.0.0"},
  "paths": {
    "/v1/customers": {
      "post": {
        "operationId": "PostCustomers",
        "summary": "Create customer",
        "requestBody": {
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {
                  "email": {"type": "string", "description": "Customer email"},
                  "name": {"type": "string"},
                  "balance": {"type": "integer"},
                  "metadata": {"type": "object"}
                },
                "required": ["email"]
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      },
      "get": {
        "operationId": "GetCustomers",
        "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

func qa041MCPDoc(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(qa041MCPSpec()))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	return doc
}

type qa041MCPSchema struct {
	Type       string                            `json:"type"`
	Properties map[string]map[string]interface{} `json:"properties"`
	Required   []string                          `json:"required"`
}

func qa041SchemaByName(t *testing.T, tools []OpenAPITool, name string) qa041MCPSchema {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		var schema qa041MCPSchema
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("unmarshal schema for %s: %v", name, err)
		}
		return schema
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	t.Fatalf("tool %q not generated; tools: %v", name, names)
	return qa041MCPSchema{}
}

func TestQA041_MCP_FormBodyPropsInSchema(t *testing.T) {
	conv := NewConverter("https://api.example.com")
	tools, configs, err := conv.ConvertToTools(qa041MCPDoc(t))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	post := qa041SchemaByName(t, tools, "PostCustomers")
	for _, prop := range []string{"email", "name", "balance", "metadata"} {
		if _, ok := post.Properties[prop]; !ok {
			t.Errorf("PostCustomers inputSchema is missing body property %q (properties: %v)", prop, post.Properties)
		}
	}
	if got := post.Properties["email"]["description"]; got != "Customer email" {
		t.Errorf("body property description not carried into the schema: got %v", got)
	}
	if len(post.Required) != 1 || post.Required[0] != "email" {
		t.Errorf("required body properties not advertised: got %v", post.Required)
	}

	// The config must record the media type so the executor encodes correctly.
	if got := configs["PostCustomers"].ContentType; got != "application/x-www-form-urlencoded" {
		t.Errorf("ExecutionConfig.ContentType = %q, want application/x-www-form-urlencoded", got)
	}

	// Control: a query-parameter operation is unchanged.
	get := qa041SchemaByName(t, tools, "GetCustomers")
	if _, ok := get.Properties["limit"]; !ok {
		t.Errorf("GetCustomers lost its query parameter: %v", get.Properties)
	}
	if _, ok := get.Properties["email"]; ok {
		t.Errorf("GetCustomers must not inherit body properties: %v", get.Properties)
	}
}

func TestQA041_MCP_OpenAPIRegistrySchemaCarriesBodyProps(t *testing.T) {
	reg, err := NewRegistry(qa041MCPDoc(t), "https://api.example.com")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	var postOp *openapi.Operation
	for _, op := range reg.operations {
		if op.Method == http.MethodPost {
			postOp = op
		}
	}
	if postOp == nil {
		t.Fatal("no POST operation in the registry")
	}

	var found bool
	for _, tool := range reg.ListTools() {
		if tool.Name != generateCommandName(postOp) {
			continue
		}
		found = true
		var schema qa041MCPSchema
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := schema.Properties["email"]; !ok {
			t.Errorf("OpenAPIRegistry schema is missing form body property: %v", schema.Properties)
		}
	}
	if !found {
		t.Fatal("registry did not list the form-encoded tool")
	}
}

func TestQA041_MCP_ToolExecutorSendsURLEncoded(t *testing.T) {
	var gotCT, gotBody, gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		gotBody = string(b)
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cus_1"}`))
	}))
	defer srv.Close()

	conv := NewConverter(srv.URL)
	_, configs, err := conv.ConvertToTools(qa041MCPDoc(t))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	exec := NewOpenAPIExecutor(configs)
	if _, err := exec.Execute(context.Background(), "PostCustomers", map[string]interface{}{
		"email":    "foo@example.com",
		"name":     "Foo Bar",
		"metadata": map[string]interface{}{"owner": "qa"},
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/customers" {
		t.Errorf("wrong request: %s %s", gotMethod, gotPath)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	values, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("body is not urlencoded (%q): %v", gotBody, err)
	}
	if values.Get("email") != "foo@example.com" || values.Get("name") != "Foo Bar" {
		t.Errorf("body did not carry the arguments: %v", values)
	}
	if values.Get("metadata[owner]") != "qa" {
		t.Errorf("nested object not bracket-encoded: %v", values)
	}
}

func TestQA041_MCP_ExecuteHTTPWithContentType_SendsURLEncoded(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if _, err := ExecuteHTTPWithContentType(srv.URL, http.MethodPost, "/v1/customers",
		"application/x-www-form-urlencoded", map[string]interface{}{"email": "a@b.c"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	if gotBody != "email=a%40b.c" {
		t.Errorf("body = %q, want email=a%%40b.c", gotBody)
	}

	// Default (empty contentType) keeps the historical JSON encoding.
	if _, err := ExecuteHTTP(srv.URL, http.MethodPost, "/v1/customers",
		map[string]interface{}{"email": "a@b.c"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("default Content-Type = %q, want application/json", gotCT)
	}
}
