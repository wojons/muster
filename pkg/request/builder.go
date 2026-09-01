// Package request provides HTTP request building from OpenAPI operation definitions.
// It handles parameter serialization, path substitution, and request construction.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/request/builder.go
package request

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Builder constructs HTTP requests from operation metadata and user-provided parameters.
type Builder struct {
	BaseURL        string
	Path           string
	Method         string
	Headers        map[string]string
	PathParams     map[string]string
	QueryParams    map[string]string
	QueryParamList [][]string // repeated query keys, e.g. {"tags","a"},{"tags","b"} (DF-017)
	Body           io.Reader
	contentType    string
	multipartParts map[string]string // fieldName → filePath
	multipartExtra map[string]string // fieldName → value (non-file form fields)
}

// NewBuilder creates a new request Builder with the given base URL.
func NewBuilder(baseURL string, args ...string) *Builder {
	b := &Builder{
		BaseURL:     baseURL,
		Headers:     make(map[string]string),
		PathParams:  make(map[string]string),
		QueryParams: make(map[string]string),
	}
	if len(args) >= 1 {
		b.Path = args[0]
	}
	if len(args) >= 2 {
		b.Method = args[1]
	}
	return b
}

// ExecuteResult holds the result of executing a request.
type ExecuteResult struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// ExecuteWithResult executes the request and returns a structured result.
func (b *Builder) ExecuteWithResult(ctx context.Context) (*ExecuteResult, error) {
	resp, err := b.Execute(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &ExecuteResult{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}, nil
}

// WithHeader sets a request header.
func (b *Builder) WithHeader(key, value string) {
	if b.Headers == nil {
		b.Headers = make(map[string]string)
	}
	b.Headers[key] = value
}

// JSONBody sets a JSON string as the request body.
func (b *Builder) JSONBody(jsonStr string) {
	b.Body = strings.NewReader(jsonStr)
}

// WithTimeout sets the request timeout (no-op in current impl).
func (b *Builder) WithTimeout(timeout interface{}) {}

// Build builds the HTTP request.
func (b *Builder) Build() (*http.Request, error) {
	url := b.buildURL()
	req, err := http.NewRequest(b.Method, url, b.Body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for key, value := range b.Headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

// SetHeader sets a default header for all requests built by this Builder.
func (b *Builder) SetHeader(key, value string) {
	if b.Headers == nil {
		b.Headers = make(map[string]string)
	}
	b.Headers[key] = value
}

// WithPathParam sets a path parameter substitution.
func (b *Builder) WithPathParam(name, value string) *Builder {
	if b.PathParams == nil {
		b.PathParams = make(map[string]string)
	}
	b.PathParams[name] = value
	return b
}

// WithQueryParam sets a query parameter.
func (b *Builder) WithQueryParam(name, value string) *Builder {
	if b.QueryParams == nil {
		b.QueryParams = make(map[string]string)
	}
	b.QueryParams[name] = value
	return b
}

// WithQueryParamSlice sets a query parameter from a slice of strings,
// emitting each element as a repeated key (e.g. ?tags=a&tags=b).
// This avoids Go's %v formatting which produces "[a b]" (DF-017).
func (b *Builder) WithQueryParamSlice(name string, values []string) *Builder {
	for _, v := range values {
		b.QueryParamList = append(b.QueryParamList, []string{name, v})
	}
	return b
}

// WithBody sets the request body.
func (b *Builder) WithBody(body io.Reader) *Builder {
	b.Body = body
	return b
}

// WithFileUpload adds a file to be uploaded via multipart/form-data.
// fieldName is the form field name (e.g. "file"), filePath is the local path to read.
// Also auto-adds a "name" form field with the file's basename (common for upload APIs).
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/request/builder.go
func (b *Builder) WithFileUpload(fieldName, filePath string) *Builder {
	if b.multipartParts == nil {
		b.multipartParts = make(map[string]string)
	}
	b.multipartParts[fieldName] = filePath
	b.contentType = "multipart/form-data"
	// Auto-add "name" field from the file's basename
	if b.multipartExtra == nil {
		b.multipartExtra = make(map[string]string)
	}
	if _, exists := b.multipartExtra["name"]; !exists {
		b.multipartExtra["name"] = filepath.Base(filePath)
	}
	return b
}

// WithMultipartField adds a non-file form field for multipart uploads.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/request/builder.go
func (b *Builder) WithMultipartField(fieldName, value string) *Builder {
	if b.multipartExtra == nil {
		b.multipartExtra = make(map[string]string)
	}
	b.multipartExtra[fieldName] = value
	return b
}

// buildMultipartBody constructs a multipart/form-data body from registered parts.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/request/builder.go
func (b *Builder) buildMultipartBody() (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for fieldName, filePath := range b.multipartParts {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, "", fmt.Errorf("open file %s: %w", filePath, err)
		}
		part, err := w.CreateFormFile(fieldName, filepath.Base(filePath))
		if err != nil {
			_ = file.Close()
			return nil, "", fmt.Errorf("create form file: %w", err)
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = file.Close()
			return nil, "", fmt.Errorf("copy file content: %w", err)
		}
		_ = file.Close()
	}

	for fieldName, value := range b.multipartExtra {
		if err := w.WriteField(fieldName, value); err != nil {
			return nil, "", fmt.Errorf("write field %s: %w", fieldName, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return &buf, w.FormDataContentType(), nil
}

// WithContentType sets a custom Content-Type header for the request.
func (b *Builder) WithContentType(ct string) *Builder {
	b.contentType = ct
	return b
}

// Execute builds and executes the HTTP request.
//
// API shape (DF-MUSTER-5): Execute returns a standard *http.Response, not a
// []byte or decoded struct — the caller owns reading (and closing) resp.Body
// (io.ReadAll(resp.Body) is the canonical read). To set the request body,
// use JSONBody (raw JSON string) or WithBody (io.Reader) before calling
// Execute; JSONBody/WithBody are read when the request is built.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/request/builder.go
func (b *Builder) Execute(ctx context.Context) (*http.Response, error) {
	url := b.buildURL()

	var body io.Reader = b.Body

	// If multipart parts are registered, build the multipart body
	if len(b.multipartParts) > 0 {
		var err error
		body, b.contentType, err = b.buildMultipartBody()
		if err != nil {
			return nil, fmt.Errorf("build multipart body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, b.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Apply default headers
	for key, value := range b.Headers {
		req.Header.Set(key, value)
	}

	// Apply content type from multipart or explicit setting
	if b.contentType != "" {
		req.Header.Set("Content-Type", b.contentType)
	}

	client := newHTTPClient()
	return client.Do(req)
}

// defaultHTTPTimeout is the fallback timeout for generated API calls when
// the MUSTER_HTTP_TIMEOUT env var is absent, unparseable, or <= 0.
const defaultHTTPTimeout = 30 * time.Second

// newHTTPClient returns an *http.Client whose Timeout is configured from the
// MUSTER_HTTP_TIMEOUT env var (value in seconds). If the env var is absent,
// unparseable, or <= 0, the 30s default is used. This follows the same
// os.Getenv pattern as MUSTER_BASE_URL in pkg/generator/generator.go.
func newHTTPClient() *http.Client {
	timeout := defaultHTTPTimeout
	if v := os.Getenv("MUSTER_HTTP_TIMEOUT"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	return &http.Client{Timeout: timeout}
}

// BuildRequest constructs an HTTP request from the given method, path, body, and headers.
func (b *Builder) BuildRequest(method, path string, body io.Reader, headers map[string]string) (*http.Request, error) {
	url := b.buildURLFor(path)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Apply default headers
	for key, value := range b.Headers {
		req.Header.Set(key, value)
	}

	// Apply request-specific headers (override defaults)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// BuildGet constructs a GET request.
func (b *Builder) BuildGet(path string, headers map[string]string) (*http.Request, error) {
	return b.BuildRequest(http.MethodGet, path, nil, headers)
}

// BuildPost constructs a POST request with a body.
func (b *Builder) BuildPost(path string, body io.Reader, contentType string, headers map[string]string) (*http.Request, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return b.BuildRequest(http.MethodPost, path, body, headers)
}

// BuildPut constructs a PUT request with a body.
func (b *Builder) BuildPut(path string, body io.Reader, contentType string, headers map[string]string) (*http.Request, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return b.BuildRequest(http.MethodPut, path, body, headers)
}

// BuildDelete constructs a DELETE request.
func (b *Builder) BuildDelete(path string, headers map[string]string) (*http.Request, error) {
	return b.BuildRequest(http.MethodDelete, path, nil, headers)
}

// buildURL combines the base URL, path, path params, and query params.
func (b *Builder) buildURL() string {
	return b.buildURLFor(b.Path)
}

func (b *Builder) buildURLFor(path string) string {
	base := strings.TrimRight(b.BaseURL, "/")
	path = strings.TrimLeft(path, "/")

	// Substitute path parameters
	for name, value := range b.PathParams {
		placeholder := "{" + name + "}"
		path = strings.Replace(path, placeholder, value, 1)
	}

	url := base
	if path != "" {
		url += "/" + path
	}

	// Add query parameters
	parts := make([]string, 0, len(b.QueryParams)+len(b.QueryParamList))
	for name, value := range b.QueryParams {
		parts = append(parts, name+"="+value)
	}
	// Repeated query keys (DF-017): emit ?tags=a&tags=b
	for _, kv := range b.QueryParamList {
		if len(kv) == 2 {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	if len(parts) > 0 {
		url += "?" + strings.Join(parts, "&")
	}

	return url
}
