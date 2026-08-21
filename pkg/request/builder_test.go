package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// PHASE 1: Constructor & Basic Build
// =============================================================================

func TestNewBuilder_EmptyArgs(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	if b.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", b.BaseURL, "https://api.example.com")
	}
	if b.Path != "" {
		t.Errorf("Path = %q, want empty", b.Path)
	}
	if b.Method != "" {
		t.Errorf("Method = %q, want empty", b.Method)
	}
	if b.Headers == nil {
		t.Error("Headers map should be initialized (non-nil)")
	}
	if b.PathParams == nil {
		t.Error("PathParams map should be initialized (non-nil)")
	}
	if b.QueryParams == nil {
		t.Error("QueryParams map should be initialized (non-nil)")
	}
}

func TestNewBuilder_WithPath(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users")
	if b.Path != "/users" {
		t.Errorf("Path = %q, want %q", b.Path, "/users")
	}
	if b.Method != "" {
		t.Errorf("Method = %q, want empty", b.Method)
	}
}

func TestNewBuilder_WithPathAndMethod(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "POST")
	if b.Path != "/users" {
		t.Errorf("Path = %q, want %q", b.Path, "/users")
	}
	if b.Method != "POST" {
		t.Errorf("Method = %q, want %q", b.Method, "POST")
	}
}

func TestBuild_SimpleGET(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if req.URL.String() != "https://api.example.com/users" {
		t.Errorf("URL = %q, want https://api.example.com/users", req.URL.String())
	}
	if req.Body != nil {
		t.Error("Body should be nil for GET")
	}
}

func TestBuild_WithHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")
	b.SetHeader("Authorization", "Bearer token123")
	b.SetHeader("X-Custom", "value")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if req.Header.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization = %q, want Bearer token123", req.Header.Get("Authorization"))
	}
	if req.Header.Get("X-Custom") != "value" {
		t.Errorf("X-Custom = %q, want value", req.Header.Get("X-Custom"))
	}
}

func TestBuild_WithPathParams(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users/{id}/posts/{postId}", "GET")
	b.WithPathParam("id", "42")
	b.WithPathParam("postId", "7")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if req.URL.Path != "/users/42/posts/7" {
		t.Errorf("Path = %q, want /users/42/posts/7", req.URL.Path)
	}
}

func TestBuild_WithQueryParams(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/search", "GET")
	b.WithQueryParam("q", "hello")
	b.WithQueryParam("page", "1")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	// Query params are unordered in map iteration, check both
	qs := req.URL.RawQuery
	if !strings.Contains(qs, "q=hello") || !strings.Contains(qs, "page=1") {
		t.Errorf("RawQuery = %q, want q=hello&page=1 (order may vary)", qs)
	}
	if !strings.Contains(qs, "&") {
		t.Errorf("RawQuery = %q, expected & separator between params", qs)
	}
}

// =============================================================================
// PHASE 1b: SetHeader
// =============================================================================

func TestSetHeader_SetsDefaultHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.SetHeader("Accept", "application/json")
	if b.Headers["Accept"] != "application/json" {
		t.Errorf("Headers[Accept] = %q, want application/json", b.Headers["Accept"])
	}
}

func TestSetHeader_NilHeadersSafety(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // Headers is nil
	b.SetHeader("X-Test", "value")
	if b.Headers == nil {
		t.Fatal("Headers should be initialized after SetHeader on nil map")
	}
	if b.Headers["X-Test"] != "value" {
		t.Errorf("Headers[X-Test] = %q, want value", b.Headers["X-Test"])
	}
}

func TestSetHeader_MultipleCallsAccumulate(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.SetHeader("A", "1")
	b.SetHeader("B", "2")
	b.SetHeader("A", "3") // overwrite
	if b.Headers["A"] != "3" {
		t.Errorf("Headers[A] = %q, want 3 (overwritten)", b.Headers["A"])
	}
	if b.Headers["B"] != "2" {
		t.Errorf("Headers[B] = %q, want 2", b.Headers["B"])
	}
	if len(b.Headers) != 2 {
		t.Errorf("len(Headers) = %d, want 2", len(b.Headers))
	}
}

// =============================================================================
// PHASE 2: Path & Query Params
// =============================================================================

func TestWithPathParam_SetsParam(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithPathParam("id", "99")
	if b.PathParams["id"] != "99" {
		t.Errorf("PathParams[id] = %q, want 99", b.PathParams["id"])
	}
}

func TestWithPathParam_ChainingReturnsSelf(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithPathParam("a", "1")
	if result != b {
		t.Error("WithPathParam should return self for chaining")
	}
}

func TestWithPathParam_NilPathParamsSafety(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // PathParams is nil
	b.WithPathParam("id", "5")
	if b.PathParams == nil {
		t.Fatal("PathParams should be initialized after WithPathParam on nil map")
	}
	if b.PathParams["id"] != "5" {
		t.Errorf("PathParams[id] = %q, want 5", b.PathParams["id"])
	}
}

func TestWithPathParam_SubstitutionReplacesPlaceholder(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/orgs/{org}/repos/{repo}", "GET")
	b.WithPathParam("org", "acme")
	b.WithPathParam("repo", "widgets")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if req.URL.Path != "/orgs/acme/repos/widgets" {
		t.Errorf("Path = %q, want /orgs/acme/repos/widgets", req.URL.Path)
	}
}

func TestWithQueryParam_SetsParam(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithQueryParam("limit", "10")
	if b.QueryParams["limit"] != "10" {
		t.Errorf("QueryParams[limit] = %q, want 10", b.QueryParams["limit"])
	}
}

func TestWithQueryParam_Chaining(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithQueryParam("a", "1")
	if result != b {
		t.Error("WithQueryParam should return self for chaining")
	}
}

func TestWithQueryParam_NilQueryParamsSafety(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // QueryParams is nil
	b.WithQueryParam("sort", "asc")
	if b.QueryParams == nil {
		t.Fatal("QueryParams should be initialized after WithQueryParam on nil map")
	}
	if b.QueryParams["sort"] != "asc" {
		t.Errorf("QueryParams[sort] = %q, want asc", b.QueryParams["sort"])
	}
}

func TestWithQueryParam_MultipleParamsInURL(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/items", "GET")
	b.WithQueryParam("category", "books")
	b.WithQueryParam("inStock", "true")
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	qs := req.URL.RawQuery
	if !strings.Contains(qs, "category=books") || !strings.Contains(qs, "inStock=true") {
		t.Errorf("RawQuery = %q, want category=books&inStock=true (order may vary)", qs)
	}
}

// DF-017: slice query params must emit repeated keys (?tags=a&tags=b),
// never Go's %v "[a b]" formatting.
func TestWithQueryParamSlice_RepeatedKeys(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/widgets", "GET")
	b.WithQueryParamSlice("tags", []string{"a", "b"})
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	qs := req.URL.RawQuery
	// Must contain repeated keys, not "[a b]"
	if !strings.Contains(qs, "tags=a") || !strings.Contains(qs, "tags=b") {
		t.Errorf("RawQuery = %q, want repeated keys tags=a&tags=b", qs)
	}
	if strings.Contains(qs, "[a b]") {
		t.Errorf("RawQuery = %q, must NOT contain Go %%v formatting [a b]", qs)
	}
	// Verify the server-side query parsing yields a proper array
	values := req.URL.Query()
	tags := values["tags"]
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", tags)
	}
}

// DF-017: mixing scalar and slice query params should work together.
func TestWithQueryParam_MixedScalarAndSlice(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/widgets", "GET")
	b.WithQueryParam("limit", "10")
	b.WithQueryParamSlice("tags", []string{"a", "b"})
	req, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	qs := req.URL.RawQuery
	if !strings.Contains(qs, "limit=10") {
		t.Errorf("RawQuery = %q, want limit=10", qs)
	}
	if !strings.Contains(qs, "tags=a") || !strings.Contains(qs, "tags=b") {
		t.Errorf("RawQuery = %q, want repeated tags keys", qs)
	}
	if strings.Contains(qs, "[a b]") {
		t.Errorf("RawQuery = %q, must NOT contain [a b]", qs)
	}
}

// =============================================================================
// PHASE 3: Body & Content Type
// =============================================================================

func TestJSONBody_SetsBodyToReader(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/items", "POST")
	b.JSONBody(`{"name":"test"}`)
	if b.Body == nil {
		t.Fatal("Body should not be nil after JSONBody")
	}
	data, err := io.ReadAll(b.Body)
	if err != nil {
		t.Fatalf("ReadAll body: %v", err)
	}
	if string(data) != `{"name":"test"}` {
		t.Errorf("Body content = %q, want {\"name\":\"test\"}", string(data))
	}
}

func TestWithBody_SetsBody(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader("custom body")
	b.WithBody(body)
	if b.Body != body {
		t.Error("WithBody should set Body to the provided io.Reader")
	}
}

func TestWithBody_ChainingReturnsSelf(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithBody(strings.NewReader("x"))
	if result != b {
		t.Error("WithBody should return self for chaining")
	}
}

func TestWithContentType_SetsContentType(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithContentType("application/xml")
	if b.contentType != "application/xml" {
		t.Errorf("contentType = %q, want application/xml", b.contentType)
	}
}

func TestWithContentType_ChainingReturnsSelf(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithContentType("text/plain")
	if result != b {
		t.Error("WithContentType should return self for chaining")
	}
}

// =============================================================================
// PHASE 4: Multipart
// =============================================================================

func TestWithFileUpload_RegistersFile(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithFileUpload("attachment", "/tmp/test.pdf")
	if b.multipartParts["attachment"] != "/tmp/test.pdf" {
		t.Errorf("multipartParts[attachment] = %q, want /tmp/test.pdf", b.multipartParts["attachment"])
	}
}

func TestWithFileUpload_AutoAddsNameField(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithFileUpload("file", "/path/to/document.pdf")
	if b.multipartExtra["name"] != "document.pdf" {
		t.Errorf("multipartExtra[name] = %q, want document.pdf", b.multipartExtra["name"])
	}
}

func TestWithFileUpload_SecondUploadSameFieldNameOverwrites(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithFileUpload("file", "/tmp/first.txt")
	b.WithFileUpload("file", "/tmp/second.txt")
	if b.multipartParts["file"] != "/tmp/second.txt" {
		t.Errorf("multipartParts[file] = %q, want /tmp/second.txt (overwritten)", b.multipartParts["file"])
	}
}

func TestWithFileUpload_NilMultipartMapsHandled(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // multipartParts and multipartExtra are nil
	b.WithFileUpload("file", "/tmp/readme.md")
	if b.multipartParts == nil {
		t.Fatal("multipartParts should be initialized")
	}
	if b.multipartExtra == nil {
		t.Fatal("multipartExtra should be initialized")
	}
	if b.multipartExtra["name"] != "readme.md" {
		t.Errorf("multipartExtra[name] = %q, want readme.md", b.multipartExtra["name"])
	}
}

func TestWithFileUpload_NameAutoGeneratedOnce(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	// First upload sets "name" from basename
	b.WithFileUpload("file1", "/tmp/alpha.txt")
	if b.multipartExtra["name"] != "alpha.txt" {
		t.Errorf("name after first upload = %q, want alpha.txt", b.multipartExtra["name"])
	}
	// Second upload with different fieldName should NOT overwrite "name"
	b.WithFileUpload("file2", "/tmp/beta.txt")
	if b.multipartExtra["name"] != "alpha.txt" {
		t.Errorf("name after second upload = %q, want alpha.txt (should not be overwritten)", b.multipartExtra["name"])
	}
}

func TestWithMultipartField_RegistersExtraField(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithMultipartField("description", "test file")
	if b.multipartExtra["description"] != "test file" {
		t.Errorf("multipartExtra[description] = %q, want test file", b.multipartExtra["description"])
	}
}

func TestWithMultipartField_Chaining(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	result := b.WithMultipartField("key", "val")
	if result != b {
		t.Error("WithMultipartField should return self for chaining")
	}
}

func TestWithMultipartField_NilMultipartExtraSafety(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // multipartExtra is nil
	b.WithMultipartField("tag", "important")
	if b.multipartExtra == nil {
		t.Fatal("multipartExtra should be initialized")
	}
	if b.multipartExtra["tag"] != "important" {
		t.Errorf("multipartExtra[tag] = %q, want important", b.multipartExtra["tag"])
	}
}

func TestBuildMultipartBody_TempFileWithContent(t *testing.T) {
	// Create a temp file with known content
	tmpFile, err := os.CreateTemp("", "muster-test-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	content := "hello multipart world"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	tmpFile.Close()

	b := NewBuilder("https://api.example.com")
	b.WithFileUpload("file", tmpFile.Name())
	b.WithMultipartField("description", "a test upload")

	body, ct, err := b.buildMultipartBody()
	if err != nil {
		t.Fatalf("buildMultipartBody() error: %v", err)
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("content type = %q, want multipart/form-data prefix", ct)
	}

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll body: %v", err)
	}
	if !strings.Contains(string(data), content) {
		t.Errorf("multipart body should contain %q, got: %s", content, string(data))
	}
	if !strings.Contains(string(data), "a test upload") {
		t.Errorf("multipart body should contain extra field value, got: %s", string(data))
	}
}

func TestBuildMultipartBody_MissingFileError(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithFileUpload("file", "/nonexistent/path/definitely/not/there.txt")
	_, _, err := b.buildMultipartBody()
	if err == nil {
		t.Fatal("buildMultipartBody should return error for missing file")
	}
}

// =============================================================================
// PHASE 5: HTTP Execution (httptest.NewServer)
// =============================================================================

func TestExecute_GET200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/test", "GET")
	resp, err := b.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestExecute_PathAndQueryParamsReachServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/42" {
			t.Errorf("server received path %q, want /users/42", r.URL.Path)
		}
		if r.URL.Query().Get("active") != "true" {
			t.Errorf("server received query active=%q, want true", r.URL.Query().Get("active"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/users/{id}", "GET")
	b.WithPathParam("id", "42")
	b.WithQueryParam("active", "true")
	resp, err := b.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	resp.Body.Close()
}

func TestExecute_POSTWithBodyReceived(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("server received method %q, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"data":"hello"}` {
			t.Errorf("server received body %q, want {\"data\":\"hello\"}", string(body))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/items", "POST")
	b.JSONBody(`{"data":"hello"}`)
	resp, err := b.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want 201", resp.StatusCode)
	}
}

func TestExecute_HeadersForwarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret123" {
			t.Errorf("server received X-API-Key=%q, want secret123", r.Header.Get("X-API-Key"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/secure", "GET")
	b.SetHeader("X-API-Key", "secret123")
	resp, err := b.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	resp.Body.Close()
}

func TestExecute_BadURLError(t *testing.T) {
	b := NewBuilder("://invalid-url", "/path", "GET")
	_, err := b.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute should return error for bad URL")
	}
}

func TestExecuteWithResult_200Matches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-ID", "abc123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"success"}`))
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/api", "GET")
	result, err := b.ExecuteWithResult(context.Background())
	if err != nil {
		t.Fatalf("ExecuteWithResult() error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if string(result.Body) != `{"result":"success"}` {
		t.Errorf("Body = %q, want {\"result\":\"success\"}", string(result.Body))
	}
	if result.Headers.Get("X-Response-ID") != "abc123" {
		t.Errorf("Headers[X-Response-ID] = %q, want abc123", result.Headers.Get("X-Response-ID"))
	}
}

func TestExecuteWithResult_404NoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	b := NewBuilder(srv.URL, "/missing", "GET")
	result, err := b.ExecuteWithResult(context.Background())
	if err != nil {
		t.Fatalf("ExecuteWithResult() should not error on 404, got: %v", err)
	}
	if result.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", result.StatusCode)
	}
	if string(result.Body) != `{"error":"not found"}` {
		t.Errorf("Body = %q, want {\"error\":\"not found\"}", string(result.Body))
	}
}

func TestBuildGet_ConstructsGET(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildGet("/users", map[string]string{"Accept": "application/json"})
	if err != nil {
		t.Fatalf("BuildGet() error: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if req.Body != nil {
		t.Error("GET request should have nil body")
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept header = %q, want application/json", req.Header.Get("Accept"))
	}
}

func TestBuildPost_ConstructsPOSTWithBody(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader("payload")
	req, err := b.BuildPost("/items", body, "application/json", map[string]string{"X-Trace": "trace1"})
	if err != nil {
		t.Fatalf("BuildPost() error: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("Method = %q, want POST", req.Method)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-Trace") != "trace1" {
		t.Errorf("X-Trace = %q, want trace1", req.Header.Get("X-Trace"))
	}
}

func TestBuildPost_NilHeadersSafety(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildPost("/items", strings.NewReader("x"), "text/plain", nil)
	if err != nil {
		t.Fatalf("BuildPost() with nil headers error: %v", err)
	}
	if req.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", req.Header.Get("Content-Type"))
	}
}

func TestBuildPost_EmptyContentTypeDoesNotSetHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildPost("/items", strings.NewReader("x"), "", nil)
	if err != nil {
		t.Fatalf("BuildPost() error: %v", err)
	}
	if req.Header.Get("Content-Type") != "" {
		t.Errorf("Content-Type should be empty when contentType is empty, got %q", req.Header.Get("Content-Type"))
	}
}

func TestBuildPut_ConstructsPUT(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader("updated")
	req, err := b.BuildPut("/items/1", body, "application/json", nil)
	if err != nil {
		t.Fatalf("BuildPut() error: %v", err)
	}
	if req.Method != "PUT" {
		t.Errorf("Method = %q, want PUT", req.Method)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.Header.Get("Content-Type"))
	}
}

func TestBuildPut_NilHeadersSafety(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildPut("/items/1", strings.NewReader("x"), "text/plain", nil)
	if err != nil {
		t.Fatalf("BuildPut() with nil headers error: %v", err)
	}
	if req.Method != "PUT" {
		t.Errorf("Method = %q, want PUT", req.Method)
	}
}

func TestBuildDelete_ConstructsDELETEWithNilBody(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildDelete("/items/1", map[string]string{"Authorization": "Bearer tok"})
	if err != nil {
		t.Fatalf("BuildDelete() error: %v", err)
	}
	if req.Method != "DELETE" {
		t.Errorf("Method = %q, want DELETE", req.Method)
	}
	if req.Body != nil {
		t.Error("DELETE request should have nil body")
	}
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", req.Header.Get("Authorization"))
	}
}

// =============================================================================
// PHASE 6: BuildRequest
// =============================================================================

func TestBuildRequest_BasicGET(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	req, err := b.BuildRequest("GET", "/health", nil, nil)
	if err != nil {
		t.Fatalf("BuildRequest() error: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if req.URL.String() != "https://api.example.com/health" {
		t.Errorf("URL = %q, want https://api.example.com/health", req.URL.String())
	}
}

func TestBuildRequest_HeadersOverrideDefaults(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.SetHeader("Accept", "application/json")      // default
	b.SetHeader("Authorization", "Bearer default") // default
	// Request-specific headers should override defaults
	req, err := b.BuildRequest("GET", "/data", nil, map[string]string{
		"Accept":        "text/html",       // override
		"Authorization": "Bearer specific", // override
	})
	if err != nil {
		t.Fatalf("BuildRequest() error: %v", err)
	}
	if req.Header.Get("Accept") != "text/html" {
		t.Errorf("Accept = %q, want text/html (request-specific should override default)", req.Header.Get("Accept"))
	}
	if req.Header.Get("Authorization") != "Bearer specific" {
		t.Errorf("Authorization = %q, want Bearer specific (request-specific should override default)", req.Header.Get("Authorization"))
	}
}

func TestBuildRequest_POSTWithBody(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	body := strings.NewReader("post body content")
	req, err := b.BuildRequest("POST", "/submit", body, map[string]string{"Content-Type": "text/plain"})
	if err != nil {
		t.Fatalf("BuildRequest() error: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("Method = %q, want POST", req.Method)
	}
	data, _ := io.ReadAll(req.Body)
	if string(data) != "post body content" {
		t.Errorf("Body = %q, want post body content", string(data))
	}
	if req.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", req.Header.Get("Content-Type"))
	}
}

// =============================================================================
// PHASE 7: Edge Cases
// =============================================================================

func TestBuildURL_TrailingSlashBaseAndLeadingSlashPath(t *testing.T) {
	b := NewBuilder("https://api.example.com/", "/users", "GET")
	url := b.buildURL()
	if url != "https://api.example.com/users" {
		t.Errorf("buildURL() = %q, want https://api.example.com/users (single slash)", url)
	}
}

func TestBuildURL_EmptyPath(t *testing.T) {
	b := NewBuilder("https://api.example.com", "", "GET")
	url := b.buildURL()
	if url != "https://api.example.com" {
		t.Errorf("buildURL() = %q, want https://api.example.com (base URL only)", url)
	}
}

func TestBuildURL_NoQueryParamsNoQuestionMark(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")
	url := b.buildURL()
	if strings.Contains(url, "?") {
		t.Errorf("buildURL() = %q, should not contain ? when no query params", url)
	}
}

func TestBuildURL_PathParamNotFoundPlaceholderLeft(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users/{id}/posts", "GET")
	// No WithPathParam call for "id"
	url := b.buildURL()
	if url != "https://api.example.com/users/{id}/posts" {
		t.Errorf("buildURL() = %q, want https://api.example.com/users/{id}/posts (placeholder left as-is)", url)
	}
}

func TestWithTimeout_NoPanic(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	// WithTimeout is a no-op; just verify it doesn't panic
	b.WithTimeout("30s")
	b.WithTimeout(30)
	b.WithTimeout(nil)
	// If we got here without panicking, test passes
}

func TestWithHeader_SetsHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	b.WithHeader("X-Request-ID", "req-001")
	if b.Headers["X-Request-ID"] != "req-001" {
		t.Errorf("Headers[X-Request-ID] = %q, want req-001", b.Headers["X-Request-ID"])
	}
}

func TestWithHeader_NilHeadersSafety(t *testing.T) {
	b := &Builder{BaseURL: "https://api.example.com"} // Headers is nil
	b.WithHeader("X-Test", "val")
	if b.Headers == nil {
		t.Fatal("Headers should be initialized after WithHeader on nil map")
	}
	if b.Headers["X-Test"] != "val" {
		t.Errorf("Headers[X-Test] = %q, want val", b.Headers["X-Test"])
	}
}

// =============================================================================
// PHASE 15: WithTimeout (no-op)
// =============================================================================

func TestWithTimeout_IsNoOp(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	// WithTimeout is a no-op — verify it doesn't panic or mutate the builder
	b.WithTimeout(30)
	if b.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL changed after WithTimeout: %q", b.BaseURL)
	}
}

func TestWithTimeout_NilReceiver(t *testing.T) {
	// Should not panic on nil receiver
	var b *Builder
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("WithTimeout panicked on nil receiver: %v", r)
			}
		}()
		b.WithTimeout(5)
	}()
}

func TestWithTimeout_VariousDurations(t *testing.T) {
	b := NewBuilder("https://api.example.com")
	durations := []interface{}{30, 60.0, "30s", time.Second * 10, time.Millisecond * 500}
	for _, d := range durations {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("WithTimeout(%v) panicked: %v", d, r)
				}
			}()
			b.WithTimeout(d)
		}()
	}
	// Builder should still be usable after all calls
	if b.BaseURL != "https://api.example.com" {
		t.Error("BaseURL changed after multiple WithTimeout calls")
	}
}
