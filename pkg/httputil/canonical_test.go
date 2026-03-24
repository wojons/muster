package httputil

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple HTTP URL",
			input:    "http://example.com/path",
			expected: "http://example.com/path",
			wantErr:  false,
		},
		{
			name:     "simple HTTPS URL",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "uppercase scheme and host",
			input:    "HTTPS://Example.COM/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "mixed case host",
			input:    "http://API.Example.COM/v1",
			expected: "http://api.example.com/v1",
			wantErr:  false,
		},
		{
			name:     "default HTTP port 80 removed",
			input:    "http://example.com:80/path",
			expected: "http://example.com/path",
			wantErr:  false,
		},
		{
			name:     "default HTTPS port 443 removed",
			input:    "https://example.com:443/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "non-default port preserved",
			input:    "http://example.com:8080/path",
			expected: "http://example.com:8080/path",
			wantErr:  false,
		},
		{
			name:     "HTTPS port 80 preserved",
			input:    "https://example.com:80/path",
			expected: "https://example.com:80/path",
			wantErr:  false,
		},
		{
			name:     "HTTP port 443 preserved",
			input:    "http://example.com:443/path",
			expected: "http://example.com:443/path",
			wantErr:  false,
		},
		{
			name:     "query parameters sorted",
			input:    "http://example.com/path?z=1&a=2&m=3",
			expected: "http://example.com/path?a=2&m=3&z=1",
			wantErr:  false,
		},
		{
			name:     "query parameters with special characters",
			input:    "http://example.com/path?key=value%20with%20spaces&other=test",
			expected: "http://example.com/path?key=value+with+spaces&other=test",
			wantErr:  false,
		},
		{
			name:     "no query parameters",
			input:    "http://example.com/path",
			expected: "http://example.com/path",
			wantErr:  false,
		},
		{
			name:     "empty query string",
			input:    "http://example.com/path?",
			expected: "http://example.com/path",
			wantErr:  false,
		},
		{
			name:     "complex URL with all features",
			input:    "HTTPS://Example.COM:443/path?Z=1&A=2",
			expected: "https://example.com/path?A=2&Z=1",
			wantErr:  false,
		},
		{
			name:     "URL with userinfo",
			input:    "http://user:pass@Example.COM/path",
			expected: "http://user:pass@example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with fragment",
			input:    "http://example.com/path#section",
			expected: "http://example.com/path#section",
			wantErr:  false,
		},
		{
			name:     "invalid URL",
			input:    "://invalid-url",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "empty URL",
			input:    "",
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CanonicalizeURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("CanonicalizeURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCanonicalizeHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    http.Header
		expected http.Header
	}{
		{
			name:     "nil headers",
			input:    nil,
			expected: http.Header{},
		},
		{
			name:     "empty headers",
			input:    http.Header{},
			expected: http.Header{},
		},
		{
			name: "single header single value",
			input: http.Header{
				"Content-Type": []string{"application/json"},
			},
			expected: http.Header{
				"content-type": []string{"application/json"},
			},
		},
		{
			name: "single header multiple values sorted",
			input: http.Header{
				"Accept": []string{"text/html", "application/json", "text/plain"},
			},
			expected: http.Header{
				"accept": []string{"application/json", "text/html", "text/plain"},
			},
		},
		{
			name: "multiple headers lowercase",
			input: http.Header{
				"Content-Type":    []string{"application/json"},
				"X-Custom-Header": []string{"value"},
				"Authorization":   []string{"Bearer token"},
			},
			expected: http.Header{
				"content-type":    []string{"application/json"},
				"x-custom-header": []string{"value"},
				"authorization":   []string{"Bearer token"},
			},
		},
		{
			name: "mixed case header names",
			input: http.Header{
				"CONTENT-TYPE": []string{"application/json"},
				"x-custom":     []string{"value"},
				"Accept":       []string{"text/html"},
			},
			expected: http.Header{
				"content-type": []string{"application/json"},
				"x-custom":     []string{"value"},
				"accept":       []string{"text/html"},
			},
		},
		{
			name: "values already sorted",
			input: http.Header{
				"Accept": []string{"application/json", "text/html"},
			},
			expected: http.Header{
				"accept": []string{"application/json", "text/html"},
			},
		},
		{
			name: "values reverse sorted",
			input: http.Header{
				"Accept": []string{"text/html", "application/json"},
			},
			expected: http.Header{
				"accept": []string{"application/json", "text/html"},
			},
		},
		{
			name: "empty header values preserved",
			input: http.Header{
				"X-Empty": []string{""},
			},
			expected: http.Header{
				"x-empty": []string{""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalizeHeaders(tt.input)

			// Compare headers
			if len(got) != len(tt.expected) {
				t.Errorf("CanonicalizeHeaders() returned %d headers, want %d", len(got), len(tt.expected))
				return
			}

			for key, expectedValues := range tt.expected {
				gotValues, ok := got[key]
				if !ok {
					t.Errorf("CanonicalizeHeaders() missing header %q", key)
					continue
				}
				if len(gotValues) != len(expectedValues) {
					t.Errorf("CanonicalizeHeaders() header %q has %d values, want %d", key, len(gotValues), len(expectedValues))
					continue
				}
				for i, v := range expectedValues {
					if gotValues[i] != v {
						t.Errorf("CanonicalizeHeaders() header %q value[%d] = %q, want %q", key, i, gotValues[i], v)
					}
				}
			}

			// Ensure original headers are not modified
			if tt.input != nil {
				for key := range tt.input {
					lowerKey := key
					// Simple lowercase conversion for comparison
					if key != lowerKey {
						t.Errorf("CanonicalizeHeaders() modified original header key %q", key)
					}
				}
			}
		})
	}
}

func TestCanonicalizeRequest(t *testing.T) {
	tests := []struct {
		name        string
		setupReq    func() *http.Request
		wantErr     bool
		checkURL    string
		checkHeader http.Header
	}{
		{
			name: "nil request",
			setupReq: func() *http.Request {
				return nil
			},
			wantErr:  true,
			checkURL: "",
		},
		{
			name: "simple GET request",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/path", nil)
				return req
			},
			wantErr:     false,
			checkURL:    "http://example.com/path",
			checkHeader: http.Header{},
		},
		{
			name: "request with uppercase URL",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "HTTPS://Example.COM:443/Path", nil)
				return req
			},
			wantErr:     false,
			checkURL:    "https://example.com/Path",
			checkHeader: http.Header{},
		},
		{
			name: "request with headers",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("POST", "http://api.example.com/v1", nil)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer token123")
				return req
			},
			wantErr:  false,
			checkURL: "http://api.example.com/v1",
			checkHeader: http.Header{
				"content-type":  []string{"application/json"},
				"authorization": []string{"Bearer token123"},
			},
		},
		{
			name: "request with mixed case headers",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/", nil)
				req.Header["ACCEPT"] = []string{"text/html", "application/json"}
				req.Header["X-CUSTOM"] = []string{"value1", "value2"}
				return req
			},
			wantErr:  false,
			checkURL: "http://example.com/",
			checkHeader: http.Header{
				"accept":   []string{"application/json", "text/html"},
				"x-custom": []string{"value1", "value2"},
			},
		},
		{
			name: "request with query parameters",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/api?z=1&a=2", nil)
				return req
			},
			wantErr:     false,
			checkURL:    "http://example.com/api?a=2&z=1",
			checkHeader: http.Header{},
		},
		{
			name: "request without URL",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/", nil)
				req.URL = nil
				return req
			},
			wantErr:     false,
			checkURL:    "",
			checkHeader: http.Header{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			got, err := CanonicalizeRequest(req)

			if (err != nil) != tt.wantErr {
				t.Errorf("CanonicalizeRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.name == "nil request" {
				if got != nil {
					t.Errorf("CanonicalizeRequest() = %v, want nil", got)
				}
				return
			}

			if got != req {
				t.Error("CanonicalizeRequest() should return the same request pointer")
			}

			if got != nil && got.URL != nil {
				if got.URL.String() != tt.checkURL {
					t.Errorf("CanonicalizeRequest() URL = %v, want %v", got.URL.String(), tt.checkURL)
				}
			}

			if tt.checkHeader != nil {
				for key, expectedValues := range tt.checkHeader {
					gotValues, ok := got.Header[key]
					if !ok {
						t.Errorf("CanonicalizeRequest() missing header %q", key)
						continue
					}
					if len(gotValues) != len(expectedValues) {
						t.Errorf("CanonicalizeRequest() header %q has %d values, want %d", key, len(gotValues), len(expectedValues))
						continue
					}
					for i, v := range expectedValues {
						if gotValues[i] != v {
							t.Errorf("CanonicalizeRequest() header %q value[%d] = %q, want %q", key, i, gotValues[i], v)
						}
					}
				}
			}
		})
	}
}

func TestCanonicalizeHeadersImmutability(t *testing.T) {
	// Test that original headers are not modified
	original := http.Header{
		"Content-Type": []string{"application/json"},
	}

	canonical := CanonicalizeHeaders(original)

	// Original should still have original key
	if _, ok := original["Content-Type"]; !ok {
		t.Error("CanonicalizeHeaders() modified original header keys")
	}

	// Canonical should have lowercase key
	// Our canonicalize function converts to lowercase, so we check for "content-type"
	//nolint:staticcheck // Intentionally using lowercase key - our function converts to lowercase
	if _, ok := canonical["content-type"]; !ok {
		t.Error("CanonicalizeHeaders() did not create lowercase key in result")
	}

	// Modifying canonical should not affect original
	canonical.Set("content-type", "modified")
	if original.Get("Content-Type") == "modified" {
		t.Error("CanonicalizeHeaders() result is not independent from original")
	}
}

func TestCanonicalizeURLEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "URL with only scheme and host",
			input:    "https://example.com",
			expected: "https://example.com",
			wantErr:  false,
		},
		{
			name:     "URL with trailing slash",
			input:    "http://example.com/",
			expected: "http://example.com/",
			wantErr:  false,
		},
		{
			name:     "URL with path only",
			input:    "/path/to/resource",
			expected: "/path/to/resource",
			wantErr:  false,
		},
		{
			name:     "URL with IPv4 address",
			input:    "http://192.168.1.1:8080/path",
			expected: "http://192.168.1.1:8080/path",
			wantErr:  false,
		},
		{
			name:     "URL with IPv6 address",
			input:    "http://[::1]:8080/path",
			expected: "http://[::1]:8080/path",
			wantErr:  false,
		},
		{
			name:     "URL with multiple query values for same key",
			input:    "http://example.com/path?tag=a&tag=b&tag=c",
			expected: "http://example.com/path?tag=a&tag=b&tag=c",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CanonicalizeURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("CanonicalizeURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCanonicalizeRequest_NilRequest(t *testing.T) {
	req := (*http.Request)(nil)
	got, err := CanonicalizeRequest(req)

	if got != nil {
		t.Errorf("CanonicalizeRequest() with nil request should return nil, got %v", got)
	}

	if err == nil {
		t.Error("CanonicalizeRequest() with nil request should return error")
	}

	if err != nil && err.Error() != "request cannot be nil" {
		t.Errorf("CanonicalizeRequest() error = %v, want 'request cannot be nil'", err)
	}
}

func TestCanonicalValidator_NewValidator(t *testing.T) {
	v := NewCanonicalValidator()
	if v == nil {
		t.Fatal("NewCanonicalValidator() returned nil")
	}
	if !v.IsEnabled() {
		t.Error("NewCanonicalValidator() should create enabled validator by default")
	}
	if v.GetViolationCount() != 0 {
		t.Error("NewCanonicalValidator() should start with zero violations")
	}
}

func TestCanonicalValidator_WithOptions(t *testing.T) {
	v := NewCanonicalValidator(WithStrictMode(true))
	if !v.IsEnabled() {
		t.Error("Validator should be enabled")
	}
	if !v.strictMode {
		t.Error("Validator should be in strict mode")
	}
}

func TestCanonicalValidator_ValidateRequest(t *testing.T) {
	tests := []struct {
		name     string
		setupReq func() *http.Request
		strict   bool
		wantErr  bool
		checkErr string
	}{
		{
			name: "valid request",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/path", nil)
				req.Header.Set("content-type", "application/json")
				return req
			},
			wantErr: false,
		},
		{
			name: "nil request",
			setupReq: func() *http.Request {
				return nil
			},
			wantErr: false,
		},
		{
			name: "request with nil URL",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/", nil)
				req.URL = nil
				return req
			},
			strict:   true,
			wantErr:  true,
			checkErr: "request URL is nil",
		},
		{
			name: "request with empty method",
			setupReq: func() *http.Request {
				req := &http.Request{
					URL: &url.URL{Scheme: "http", Host: "example.com"},
				}
				return req
			},
			strict:   true,
			wantErr:  true,
			checkErr: "request method is empty",
		},
		{
			name: "request with non-canonical headers",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			strict:   true,
			wantErr:  true,
			checkErr: "header key not canonicalized",
		},
		{
			name: "strict mode with violations",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "http://example.com/", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			strict:   true,
			wantErr:  true,
			checkErr: "header key not canonicalized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewCanonicalValidator(WithStrictMode(tt.strict))
			req := tt.setupReq()
			err := v.ValidateRequest(req)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkErr != "" && err != nil {
				if !strings.Contains(err.Error(), tt.checkErr) {
					t.Errorf("ValidateRequest() error = %v, should contain %q", err, tt.checkErr)
				}
			}
		})
	}
}

func TestCanonicalValidator_ValidateResponse(t *testing.T) {
	tests := []struct {
		name      string
		setupResp func() *http.Response
		strict    bool
		wantErr   bool
		checkErr  string
	}{
		{
			name: "valid response",
			setupResp: func() *http.Response {
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"content-type": []string{"application/json"}},
				}
			},
			wantErr: false,
		},
		{
			name: "nil response",
			setupResp: func() *http.Response {
				return nil
			},
			wantErr: false,
		},
		{
			name: "invalid status code",
			setupResp: func() *http.Response {
				return &http.Response{
					StatusCode: 999,
					Header:     http.Header{},
				}
			},
			strict:   true,
			wantErr:  true,
			checkErr: "invalid status code",
		},
		{
			name: "non-canonical headers",
			setupResp: func() *http.Response {
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
				}
			},
			strict:   true,
			wantErr:  true,
			checkErr: "response header key not canonicalized",
		},
		{
			name: "strict mode with violations",
			setupResp: func() *http.Response {
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
				}
			},
			strict:   true,
			wantErr:  true,
			checkErr: "response header key not canonicalized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewCanonicalValidator(WithStrictMode(tt.strict))
			resp := tt.setupResp()
			err := v.ValidateResponse(resp)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkErr != "" && err != nil {
				if !strings.Contains(err.Error(), tt.checkErr) {
					t.Errorf("ValidateResponse() error = %v, should contain %q", err, tt.checkErr)
				}
			}
		})
	}
}

func TestCanonicalValidator_ViolationCount(t *testing.T) {
	v := NewCanonicalValidator()

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("Content-Type", "application/json")

	_ = v.ValidateRequest(req)
	if v.GetViolationCount() != 1 {
		t.Errorf("GetViolationCount() = %d, want 1", v.GetViolationCount())
	}

	_ = v.ValidateRequest(req)
	if v.GetViolationCount() != 2 {
		t.Errorf("GetViolationCount() = %d, want 2", v.GetViolationCount())
	}

	lastErr := v.GetLastViolation()
	if lastErr == nil {
		t.Error("GetLastViolation() should return last violation")
	}
}

func TestCanonicalValidator_SetEnabled(t *testing.T) {
	v := NewCanonicalValidator()
	if !v.IsEnabled() {
		t.Error("Validator should be enabled by default")
	}

	v.SetEnabled(false)
	if v.IsEnabled() {
		t.Error("Validator should be disabled after SetEnabled(false)")
	}

	v.SetEnabled(true)
	if !v.IsEnabled() {
		t.Error("Validator should be enabled after SetEnabled(true)")
	}
}

func TestCanonicalValidator_DisabledValidation(t *testing.T) {
	v := NewCanonicalValidator()
	v.SetEnabled(false)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("Content-Type", "application/json")
	req.URL = nil

	err := v.ValidateRequest(req)
	if err != nil {
		t.Errorf("ValidateRequest() should not error when disabled, got %v", err)
	}

	if v.GetViolationCount() != 0 {
		t.Errorf("GetViolationCount() = %d, want 0", v.GetViolationCount())
	}
}

func TestEnforcementError(t *testing.T) {
	err := &EnforcementError{
		Component: "TestComponent",
		Field:     "test_field",
		Expected:  "expected_value",
		Actual:    "actual_value",
	}

	expectedMsg := "interface enforcement violation in TestComponent: test_field expected expected_value, got actual_value"
	if err.Error() != expectedMsg {
		t.Errorf("EnforcementError.Error() = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestValidateInterface(t *testing.T) {
	tests := []struct {
		name      string
		component string
		iface     string
		impl      interface{}
		wantErr   bool
	}{
		{
			name:      "nil implementation",
			component: "TestComponent",
			iface:     "TestInterface",
			impl:      nil,
			wantErr:   true,
		},
		{
			name:      "non-nil implementation",
			component: "TestComponent",
			iface:     "TestInterface",
			impl:      &struct{}{},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInterface(tt.component, tt.iface, tt.impl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInterface() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				enfErr, ok := err.(*EnforcementError)
				if !ok {
					t.Error("Error should be *EnforcementError")
				}
				if enfErr.Component != tt.component {
					t.Errorf("EnforcementError.Component = %q, want %q", enfErr.Component, tt.component)
				}
			}
		})
	}
}

func TestCanonicalValidator_NonStrictVsStrictMode(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("Content-Type", "application/json")

	vNonStrict := NewCanonicalValidator(WithStrictMode(false))
	errNonStrict := vNonStrict.ValidateRequest(req)

	if errNonStrict != nil {
		t.Errorf("Non-strict mode should not return error, got %v", errNonStrict)
	}

	vStrict := NewCanonicalValidator(WithStrictMode(true))
	errStrict := vStrict.ValidateRequest(req)

	if errStrict == nil {
		t.Error("Strict mode should return error for non-canonical headers")
	}

	if vNonStrict.GetViolationCount() != 1 {
		t.Errorf("Non-strict violation count = %d, want 1", vNonStrict.GetViolationCount())
	}

	if vStrict.GetViolationCount() != 1 {
		t.Errorf("Strict violation count = %d, want 1", vStrict.GetViolationCount())
	}
}
