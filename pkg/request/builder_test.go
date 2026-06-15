package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// axiom:trace work_item=TEST-request-builder spec=pkg/request/builder.go plan=unit-tests/request-builder

func TestNewBuilder_DefaultURL(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	if b.BaseURL != "https://api.example.com" {
		t.Errorf("expected BaseURL 'https://api.example.com', got %q", b.BaseURL)
	}
	if b.Path != "" {
		t.Errorf("expected empty Path, got %q", b.Path)
	}
	if b.Method != "" {
		t.Errorf("expected empty Method, got %q", b.Method)
	}
	if b.Headers == nil {
		t.Error("expected non-nil Headers map")
	}
	if b.QueryParams == nil {
		t.Error("expected non-nil QueryParams map")
	}
}

func TestNewBuilder_WithPathAndMethod(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users", http.MethodPost)
	if b.Path != "/v1/users" {
		t.Errorf("expected Path '/v1/users', got %q", b.Path)
	}
	if b.Method != http.MethodPost {
		t.Errorf("expected Method POST, got %q", b.Method)
	}
}

func TestBuild_GET(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/users" {
		t.Errorf("expected URL 'https://api.example.com/v1/users', got %s", req.URL.String())
	}
}

func TestBuild_POST(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users", http.MethodPost)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", req.Method)
	}
}

func TestBuild_PUT(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users/42", http.MethodPut)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.Method != http.MethodPut {
		t.Errorf("expected method PUT, got %s", req.Method)
	}
}

func TestBuild_DELETE(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users/42", http.MethodDelete)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.Method != http.MethodDelete {
		t.Errorf("expected method DELETE, got %s", req.Method)
	}
}

func TestWithHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("Authorization", "Bearer token123")
	b.WithHeader("Accept", "application/json")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer token123" {
		t.Errorf("expected Authorization header 'Bearer token123', got %q", got)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Errorf("expected Accept header 'application/json', got %q", got)
	}
}

func TestWithHeader_DuplicateOverwrites(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("X-Custom", "first")
	b.WithHeader("X-Custom", "second")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Headers are stored in a map, so duplicate keys are overwritten.
	values := req.Header.Values("X-Custom")
	if len(values) != 1 {
		t.Errorf("expected 1 value for X-Custom, got %d", len(values))
	}
	if len(values) > 0 && values[0] != "second" {
		t.Errorf("expected X-Custom 'second', got %q", values[0])
	}
}

func TestSetHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.SetHeader("X-Default", "default-value")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if got := req.Header.Get("X-Default"); got != "default-value" {
		t.Errorf("expected X-Default header 'default-value', got %q", got)
	}
}

func TestJSONBody(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com"}`
	b := NewBuilder("https://api.example.com", "/v1/users", http.MethodPost)
	b.JSONBody(body)

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Verify Content-Type is NOT set by JSONBody alone (it only sets the body reader)
	// But we can verify the body content
	if req.Body == nil {
		t.Fatal("expected non-nil body")
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}
	req.Body.Close()

	if string(gotBody) != body {
		t.Errorf("expected body %q, got %q", body, string(gotBody))
	}
}

func TestWithBody(t *testing.T) {
	body := "raw body content"
	b := NewBuilder("https://api.example.com", "/v1/data", http.MethodPost)
	b.WithBody(strings.NewReader(body))

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}
	req.Body.Close()

	if string(gotBody) != body {
		t.Errorf("expected body %q, got %q", body, string(gotBody))
	}
}

func TestWithBody_NilBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/data", http.MethodGet)
	b.WithBody(nil)

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if req.Body != nil {
		// A nil body reader results in http.NewRequest setting Body to http.NoBody
		// which is still non-nil but signals "no body"
		t.Logf("Body is %T, which is acceptable for nil body", req.Body)
	}
}

func TestWithQueryParam(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users", http.MethodGet)
	b.WithQueryParam("page", "1").
		WithQueryParam("limit", "20")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	got := req.URL.String()
	// The query params may appear in any order since we iterate over a map
	if got != "https://api.example.com/v1/users?limit=20&page=1" &&
		got != "https://api.example.com/v1/users?page=1&limit=20" {
		t.Errorf("expected URL with query params, got %s", got)
	}
}

func TestWithQueryParam_Multiple(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/search", http.MethodGet)
	b.WithQueryParam("q", "hello world")
	b.WithQueryParam("filter", "active")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	raw := req.URL.String()
	if !strings.Contains(raw, "q=hello world") {
		t.Errorf("expected query param 'q=hello world' in URL, got %s", raw)
	}
	if !strings.Contains(raw, "filter=active") {
		t.Errorf("expected query param 'filter=active' in URL, got %s", raw)
	}
}

func TestBuild_FullRequest(t *testing.T) {
	body := `{"title":"Test"}`
	b := NewBuilder("https://api.example.com", "/v1/items", http.MethodPost)
	b.WithHeader("Authorization", "Bearer abc123")
	b.WithHeader("Accept", "application/json")
	b.JSONBody(body)
	b.WithQueryParam("sync", "true")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Verify method
	if req.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", req.Method)
	}

	// Verify URL with query param
	expectedURL := "https://api.example.com/v1/items?sync=true"
	if req.URL.String() != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, req.URL.String())
	}

	// Verify headers
	if got := req.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Errorf("expected Authorization header 'Bearer abc123', got %q", got)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Errorf("expected Accept header 'application/json', got %q", got)
	}

	// Verify body
	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	req.Body.Close()
	if string(gotBody) != body {
		t.Errorf("expected body %q, got %q", body, string(gotBody))
	}
}

func TestBuild_EmptyBaseURL(t *testing.T) {
	b := NewBuilder("")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() with empty base URL returned error: %v", err)
	}
	// An empty base URL means the request will have an empty/relative URL.
	// The http package parses this as a relative URL.
	if req.URL == nil {
		t.Fatal("expected non-nil URL even with empty base URL")
	}
	t.Logf("URL with empty base: %s", req.URL.String())
}

func TestBuild_EmptyBaseURLWithPath(t *testing.T) {
	b := NewBuilder("", "/relative/path", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.URL.Path != "/relative/path" {
		t.Errorf("expected path '/relative/path', got %s", req.URL.Path)
	}
}

func TestBuildRequest(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("Authorization", "Bearer default")

	req, err := b.BuildRequest(http.MethodGet, "/v1/data", nil, map[string]string{
		"X-Custom": "custom-value",
	})
	if err != nil {
		t.Fatalf("BuildRequest() returned error: %v", err)
	}

	if req.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/data" {
		t.Errorf("expected URL 'https://api.example.com/v1/data', got %s", req.URL.String())
	}
	// Default header should be present
	if got := req.Header.Get("Authorization"); got != "Bearer default" {
		t.Errorf("expected Authorization header 'Bearer default', got %q", got)
	}
	// Override header should be present
	if got := req.Header.Get("X-Custom"); got != "custom-value" {
		t.Errorf("expected X-Custom header 'custom-value', got %q", got)
	}
}

func TestBuildRequest_OverridesDefaultHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("Authorization", "Bearer default")

	req, err := b.BuildRequest(http.MethodGet, "/v1/data", nil, map[string]string{
		"Authorization": "Bearer override",
	})
	if err != nil {
		t.Fatalf("BuildRequest() returned error: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer override" {
		t.Errorf("expected Authorization header 'Bearer override' (override wins), got %q", got)
	}
}

func TestBuildGet(t *testing.T) {
	b := NewBuilder("https://api.example.com")

	req, err := b.BuildGet("/v1/items", map[string]string{"Accept": "application/json"})
	if err != nil {
		t.Fatalf("BuildGet() returned error: %v", err)
	}

	if req.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/items" {
		t.Errorf("expected URL 'https://api.example.com/v1/items', got %s", req.URL.String())
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Errorf("expected Accept header 'application/json', got %q", got)
	}
}

func TestBuildPost(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader(`{"name":"test"}`)

	req, err := b.BuildPost("/v1/items", body, "application/json", nil)
	if err != nil {
		t.Fatalf("BuildPost() returned error: %v", err)
	}

	if req.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/items" {
		t.Errorf("expected URL 'https://api.example.com/v1/items', got %s", req.URL.String())
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", got)
	}
	gotBody, _ := io.ReadAll(req.Body)
	req.Body.Close()
	if string(gotBody) != `{"name":"test"}` {
		t.Errorf("expected body '{\"name\":\"test\"}', got %q", string(gotBody))
	}
}

func TestBuildPost_MergesHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("Authorization", "Bearer token")

	req, err := b.BuildPost("/v1/items", nil, "", map[string]string{"X-Custom": "val"})
	if err != nil {
		t.Fatalf("BuildPost() returned error: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer token" {
		t.Errorf("expected Authorization header 'Bearer token', got %q", got)
	}
	if got := req.Header.Get("X-Custom"); got != "val" {
		t.Errorf("expected X-Custom header 'val', got %q", got)
	}
}

func TestBuildPut(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader(`{"status":"updated"}`)

	req, err := b.BuildPut("/v1/items/42", body, "application/json", nil)
	if err != nil {
		t.Fatalf("BuildPut() returned error: %v", err)
	}

	if req.Method != http.MethodPut {
		t.Errorf("expected method PUT, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/items/42" {
		t.Errorf("expected URL 'https://api.example.com/v1/items/42', got %s", req.URL.String())
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", got)
	}
	gotBody, _ := io.ReadAll(req.Body)
	req.Body.Close()
	if string(gotBody) != `{"status":"updated"}` {
		t.Errorf("expected body '{\"status\":\"updated\"}', got %q", string(gotBody))
	}
}

func TestBuildDelete(t *testing.T) {
	b := NewBuilder("https://api.example.com")

	req, err := b.BuildDelete("/v1/items/42", nil)
	if err != nil {
		t.Fatalf("BuildDelete() returned error: %v", err)
	}

	if req.Method != http.MethodDelete {
		t.Errorf("expected method DELETE, got %s", req.Method)
	}
	if req.URL.String() != "https://api.example.com/v1/items/42" {
		t.Errorf("expected URL 'https://api.example.com/v1/items/42', got %s", req.URL.String())
	}
}

func TestBuild_MultipleHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/data", http.MethodGet)
	b.WithHeader("X-First", "1")
	b.WithHeader("X-Second", "2")
	b.WithHeader("X-Third", "3")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if got := req.Header.Get("X-First"); got != "1" {
		t.Errorf("expected X-First '1', got %q", got)
	}
	if got := req.Header.Get("X-Second"); got != "2" {
		t.Errorf("expected X-Second '2', got %q", got)
	}
	if got := req.Header.Get("X-Third"); got != "3" {
		t.Errorf("expected X-Third '3', got %q", got)
	}
}

func TestBuild_NoHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/data", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Default Go http.Client sets some headers automatically (like User-Agent),
	// but we should not have custom headers set by us
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("expected no Authorization header, got %q", got)
	}
}

func TestNewBuilder_NoArgs(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	if b.Path != "" {
		t.Errorf("expected empty path, got %q", b.Path)
	}
	if b.Method != "" {
		t.Errorf("expected empty method, got %q", b.Method)
	}
}

func TestNewBuilder_OnlyPath(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users")
	if b.Path != "/v1/users" {
		t.Errorf("expected path '/v1/users', got %q", b.Path)
	}
	if b.Method != "" {
		t.Errorf("expected empty method, got %q", b.Method)
	}
}

func TestBuild_TrailingSlashBaseURL(t *testing.T) {
	// Base URL with trailing slash should be handled gracefully
	b := NewBuilder("https://api.example.com/", "/v1/users", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	expected := "https://api.example.com/v1/users"
	if req.URL.String() != expected {
		t.Errorf("expected URL %q, got %q", expected, req.URL.String())
	}
}

func TestBuild_PathWithoutLeadingSlash(t *testing.T) {
	// Path without leading slash should also work
	b := NewBuilder("https://api.example.com", "v1/users", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	expected := "https://api.example.com/v1/users"
	if req.URL.String() != expected {
		t.Errorf("expected URL %q, got %q", expected, req.URL.String())
	}
}

func TestBuild_NilBodyDefault(t *testing.T) {
	// When no body is set, Body is nil and Build should still work for GET requests
	b := NewBuilder("https://api.example.com", "/v1/status", http.MethodGet)
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if req.Body != nil {
		// http.NewRequest replaces nil body with http.NoBody, which is non-nil
		t.Logf("Body is %T (http.NoBody is expected for nil body)", req.Body)
	}
}

func TestBuild_EmptyMethod(t *testing.T) {
	// When method is empty, Build should still produce a request (Go defaults to GET)
	b := NewBuilder("https://api.example.com", "/v1/data")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	// http.NewRequest with empty method defaults to GET
	if req.Method != http.MethodGet {
		t.Errorf("expected default method GET for empty method, got %s", req.Method)
	}
	t.Logf("Empty method defaults to: %s", req.Method)
}

func TestBuildRequest_NilHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildRequest(http.MethodGet, "/v1/data", nil, nil)
	if err != nil {
		t.Fatalf("BuildRequest() returned error: %v", err)
	}
	if req.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", req.Method)
	}
}

func TestBuildGet_NilHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildGet("/v1/data", nil)
	if err != nil {
		t.Fatalf("BuildGet() returned error: %v", err)
	}
	if req.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", req.Method)
	}
}

func TestBuildPost_NilContentType(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildPost("/v1/data", nil, "", nil)
	if err != nil {
		t.Fatalf("BuildPost() returned error: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", req.Method)
	}
}

func TestBuildPut_NilContentType(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildPut("/v1/data", nil, "", nil)
	if err != nil {
		t.Fatalf("BuildPut() returned error: %v", err)
	}
	if req.Method != http.MethodPut {
		t.Errorf("expected PUT, got %s", req.Method)
	}
}

func TestBuildDelete_WithHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildDelete("/v1/items/99", map[string]string{
		"X-Request-ID": "req-123",
	})
	if err != nil {
		t.Fatalf("BuildDelete() returned error: %v", err)
	}
	if req.Method != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", req.Method)
	}
	if got := req.Header.Get("X-Request-ID"); got != "req-123" {
		t.Errorf("expected X-Request-ID header 'req-123', got %q", got)
	}
}

func TestBuild_WithMultipleQueryParams(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/search", http.MethodGet)
	b.WithQueryParam("q", "test").
		WithQueryParam("page", "2").
		WithQueryParam("sort", "asc")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	rawURL := req.URL.String()
	if !strings.Contains(rawURL, "q=test") {
		t.Errorf("expected q=test in URL, got %s", rawURL)
	}
	if !strings.Contains(rawURL, "page=2") {
		t.Errorf("expected page=2 in URL, got %s", rawURL)
	}
	if !strings.Contains(rawURL, "sort=asc") {
		t.Errorf("expected sort=asc in URL, got %s", rawURL)
	}
}

func TestBuild_WithPathParam(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users/{userId}/posts/{postId}", http.MethodGet)
	b.WithPathParam("userId", "42")
	b.WithPathParam("postId", "99")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	expected := "https://api.example.com/v1/users/42/posts/99"
	if req.URL.String() != expected {
		t.Errorf("expected URL %q, got %q", expected, req.URL.String())
	}
}

func TestBuild_WithPathParamAndQueryParam(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/v1/users/{id}", http.MethodGet)
	b.WithPathParam("id", "7")
	b.WithQueryParam("include", "profile")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	expected := "https://api.example.com/v1/users/7?include=profile"
	if req.URL.String() != expected {
		t.Errorf("expected URL %q, got %q", expected, req.URL.String())
	}
}

func TestBuild_JSONBodyAndContentType(t *testing.T) {
	body := `{"key":"value"}`
	b := NewBuilder("https://api.example.com", "/v1/data", http.MethodPost)
	b.JSONBody(body)
	b.WithContentType("application/json")

	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Check Content-Type was set via WithContentType (which sets contentType field)
	// Note: Build() does not apply contentType — that's done in Execute().
	// So after Build(), we may not see the Content-Type header.
	// Let's just verify the body is correct.
	gotBody, _ := io.ReadAll(req.Body)
	req.Body.Close()
	if string(gotBody) != body {
		t.Errorf("expected body %q, got %q", body, string(gotBody))
	}
	t.Logf("Note: Build() does not set Content-Type from WithContentType; that's handled in Execute()")
}

func TestWithContentType(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithContentType("application/xml")
	if result != b {
		t.Error("WithContentType should return the same builder for chaining")
	}
	if b.contentType != "application/xml" {
		t.Errorf("expected contentType 'application/xml', got %q", b.contentType)
	}
}

func TestWithTimeout(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	// WithTimeout is currently a no-op; verify it doesn't panic.
	b.WithTimeout(30)
	b.WithTimeout("30s")
}

func TestWithFileUpload(t *testing.T) {
	// Create a temp file to upload
	dir := t.TempDir()
	filePath := dir + "/test.txt"
	if err := writeFile(filePath, "hello world"); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder("https://api.example.com", "/upload", "POST")
	result := b.WithFileUpload("file", filePath)
	if result != b {
		t.Error("WithFileUpload should return the same builder for chaining")
	}
}

func TestWithMultipartField(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.txt"
	if err := writeFile(filePath, "data"); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder("https://api.example.com", "/upload", "POST")
	b.WithFileUpload("file", filePath)
	result := b.WithMultipartField("description", "test upload")
	if result != b {
		t.Error("WithMultipartField should return the same builder for chaining")
	}
}

func TestBuilder_Execute(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	defer srv.Close()

	b := NewBuilder(srv.URL, "/test", http.MethodGet)
	resp, err := b.Execute(ctxb)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBuilder_Execute_POST(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"test"}` {
			t.Errorf("unexpected body: %s", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"abc"}`))
	})
	defer srv.Close()

	b := NewBuilder(srv.URL, "/items", http.MethodPost)
	b.JSONBody(`{"name":"test"}`)
	resp, err := b.Execute(ctxb)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestBuilder_ExecuteWithResult(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`))
	})
	defer srv.Close()

	b := NewBuilder(srv.URL, "/data", http.MethodGet)
	result, err := b.ExecuteWithResult(ctxb)
	if err != nil {
		t.Fatalf("ExecuteWithResult() returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if string(result.Body) != `{"key":"value"}` {
		t.Errorf("expected body %q, got %q", `{"key":"value"}`, string(result.Body))
	}
	if ct := result.Headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestBuilder_ExecuteWithResult_Error(t *testing.T) {
	// Point at a server that returns an error
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	})
	defer srv.Close()

	b := NewBuilder(srv.URL, "/fail", http.MethodGet)
	result, err := b.ExecuteWithResult(ctxb)
	if err != nil {
		t.Fatalf("ExecuteWithResult() returned unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", result.StatusCode)
	}
}

// Test helpers

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

var ctxb = context.Background()

// newTestServer creates an httptest server with the given handler function.
func newTestServer(handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}
