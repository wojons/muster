// Package benchmarks provides request building benchmarks
//
// Benchmarks:
//
//   - BenchmarkRequest_BuilderCreate: HTTP request builder creation
//   - BenchmarkRequest_WithHeader: Header manipulation
//   - BenchmarkRequest_WithBody: Body setting
//   - BenchmarkRequest_Execute: Full request execution
//
// Performance Budgets:
//
// All benchmarks must meet their defined performance budgets (see budgets.go).
package benchmarks

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
)

// SimpleRequestBuilder is a minimal HTTP request builder for benchmarking
type SimpleRequestBuilder struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// NewSimpleRequestBuilder creates a new request builder
func NewSimpleRequestBuilder(method, url string) *SimpleRequestBuilder {
	return &SimpleRequestBuilder{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string),
	}
}

// WithHeader adds a header
func (b *SimpleRequestBuilder) WithHeader(key, value string) *SimpleRequestBuilder {
	b.Headers[key] = value
	return b
}

// WithBody sets the body
func (b *SimpleRequestBuilder) WithBody(body []byte) *SimpleRequestBuilder {
	b.Body = body
	return b
}

// Build creates an HTTP request
func (b *SimpleRequestBuilder) Build() (*http.Request, error) {
	var body *bytes.Reader
	if b.Body != nil {
		body = bytes.NewReader(b.Body)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(b.Method, b.URL, body)
	if err != nil {
		return nil, err
	}

	for key, value := range b.Headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// BenchmarkRequest_BuilderCreate measures request builder creation
func BenchmarkRequest_BuilderCreate(b *testing.B) {
	_ = GetBudgetOrDefault("Request_BuilderCreate")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewSimpleRequestBuilder("GET", "https://api.example.com/v1/resource")
	}
}

// BenchmarkRequest_WithHeader measures header manipulation
func BenchmarkRequest_WithHeader(b *testing.B) {
	_ = GetBudgetOrDefault("Request_WithHeader")

	builder := NewSimpleRequestBuilder("GET", "https://api.example.com/v1/resource")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = builder.WithHeader("Authorization", "Bearer token123")
	}
}

// BenchmarkRequest_WithBody measures body setting
func BenchmarkRequest_WithBody(b *testing.B) {
	_ = GetBudgetOrDefault("Request_WithBody")

	builder := NewSimpleRequestBuilder("POST", "https://api.example.com/v1/resource")
	bodyData := []byte(`{"key": "value", "number": 123, "nested": {"field": "data"}}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = builder.WithBody(bodyData)
	}
}

// BenchmarkRequest_Execute measures full request building
func BenchmarkRequest_Execute(b *testing.B) {
	_ = GetBudgetOrDefault("Request_Execute")

	bodyData := []byte(`{"key": "value", "number": 123}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		builder := NewSimpleRequestBuilder("POST", "https://api.example.com/v1/resource")
		req, err := builder.
			WithHeader("Authorization", "Bearer token123").
			WithHeader("Content-Type", "application/json").
			WithBody(bodyData).
			Build()

		if err != nil {
			b.Fatal(err)
		}
		_ = req
	}
}

// BenchmarkRequest_WithQueryParam measures query parameter handling
func BenchmarkRequest_WithQueryParam(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate adding query params
		url := fmt.Sprintf("https://api.example.com/v1/resource?page=%d&limit=%d", i, 100)
		_ = NewSimpleRequestBuilder("GET", url)
	}
}

// BenchmarkRequest_Parallel measures concurrent request building
func BenchmarkRequest_Parallel(b *testing.B) {
	bodyData := []byte(`{"key": "value"}`)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			builder := NewSimpleRequestBuilder("POST", "https://api.example.com/v1/resource")
			req, _ := builder.
				WithHeader("Authorization", fmt.Sprintf("Bearer token-%d", i)).
				WithHeader("Content-Type", "application/json").
				WithBody(bodyData).
				Build()
			_ = req
			i++
		}
	})
}

// BenchmarkRequest_LargeHeaders measures performance with large headers
func BenchmarkRequest_LargeHeaders(b *testing.B) {
	largeValue := make([]byte, 8192) // 8KB header value
	for i := range largeValue {
		largeValue[i] = 'x'
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		builder := NewSimpleRequestBuilder("GET", "https://api.example.com/v1/resource")
		req, _ := builder.
			WithHeader("X-Large-Header", string(largeValue)).
			Build()
		_ = req
	}
}

// BenchmarkRequest_MultipleHeaders measures header handling with many headers
func BenchmarkRequest_MultipleHeaders(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		builder := NewSimpleRequestBuilder("GET", "https://api.example.com/v1/resource")
		for j := 0; j < 20; j++ {
			builder.WithHeader(fmt.Sprintf("X-Header-%d", j), fmt.Sprintf("value-%d", j))
		}
		_, _ = builder.Build()
	}
}
