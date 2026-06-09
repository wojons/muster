// Package request provides HTTP request building from OpenAPI operation definitions.
// It handles parameter serialization, path substitution, and request construction.
package request

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Builder constructs HTTP requests from operation metadata and user-provided parameters.
type Builder struct {
	BaseURL     string
	Path        string
	Method      string
	Headers     map[string]string
	PathParams  map[string]string
	QueryParams map[string]string
	Body        io.Reader
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
	defer resp.Body.Close()

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

// WithBody sets the request body.
func (b *Builder) WithBody(body io.Reader) *Builder {
	b.Body = body
	return b
}

// Execute builds and executes the HTTP request.
func (b *Builder) Execute(ctx context.Context) (*http.Response, error) {
	url := b.buildURL()

	req, err := http.NewRequestWithContext(ctx, b.Method, url, b.Body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Apply default headers
	for key, value := range b.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	return client.Do(req)
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
	if len(b.QueryParams) > 0 {
		parts := make([]string, 0, len(b.QueryParams))
		for name, value := range b.QueryParams {
			parts = append(parts, name+"="+value)
		}
		url += "?" + strings.Join(parts, "&")
	}

	return url
}
