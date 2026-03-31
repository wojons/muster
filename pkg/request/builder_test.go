package request

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBuilder(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/path", "GET")

	require.NotNil(t, b)
	require.Equal(t, "https://api.example.com", b.baseURL)
	require.Equal(t, "/path", b.path)
	require.Equal(t, "GET", b.method)
	require.NotNil(t, b.pathParams)
	require.NotNil(t, b.queryParams)
}

func TestBuilder_WithPathParam(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users/{id}", "GET")

	result := b.WithPathParam("id", "123")

	require.Same(t, b, result)
	require.Equal(t, "123", b.pathParams["id"])
}

func TestBuilder_WithQueryParam(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithQueryParam("limit", "10")

	require.Same(t, b, result)
	require.Equal(t, "10", b.queryParams.Get("limit"))
}

func TestBuilder_WithHeader(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithHeader("X-Custom", "value")

	require.Same(t, b, result)
	require.Equal(t, "value", b.headers.Get("X-Custom"))
}

func TestBuilder_WithHeader_CRLFInjection_Prevented(t *testing.T) {
	t.Run("carriage return injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value\r\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("newline injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("encoded crlf injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value%0d%0aX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("encoded lf injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value%0aX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("encoded cr injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value%0dX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("mixed case encoded crlf", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value%0D%0AX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("header splitting attack", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "normal\r\nSet-Cookie: session=hacked")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("response splitting with content-type", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("Content-Type", "text/html\r\nX-XSS-Protection: 0")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})
}

func TestBuilder_WithHeader_ValidValues(t *testing.T) {
	t.Run("normal header value", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "normal-value")

		require.NoError(t, b.buildError)
		require.Equal(t, "normal-value", b.headers.Get("X-Custom"))
	})

	t.Run("header with special characters", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value-with-special!@#$%^&*()chars")

		require.NoError(t, b.buildError)
		require.Equal(t, "value-with-special!@#$%^&*()chars", b.headers.Get("X-Custom"))
	})

	t.Run("header with unicode", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value-with-unicode-日本語")

		require.NoError(t, b.buildError)
		require.Equal(t, "value-with-unicode-日本語", b.headers.Get("X-Custom"))
	})

	t.Run("header with spaces", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "value with spaces")

		require.NoError(t, b.buildError)
		require.Equal(t, "value with spaces", b.headers.Get("X-Custom"))
	})

	t.Run("empty header value", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithHeader("X-Custom", "")

		require.NoError(t, b.buildError)
		require.Equal(t, "", b.headers.Get("X-Custom"))
	})
}

func TestBuilder_WithBody(t *testing.T) {
	t.Run("string body", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "POST")
		b.WithBody("test body")

		require.NotNil(t, b.body)
	})

	t.Run("map body", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "POST")
		b.WithBody(map[string]string{"name": "test"})

		require.NotNil(t, b.body)
		require.Equal(t, "application/json", b.contentType)
	})
}

func TestBuilder_WithAuth(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithAuth("token-123")

	require.Same(t, b, result)
	require.Equal(t, "Bearer token-123", b.headers.Get("Authorization"))
}

func TestBuilder_WithAuth_CRLFInjection_Prevented(t *testing.T) {
	t.Run("token with carriage return", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAuth("token\r\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("token with newline", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAuth("token\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("token with encoded crlf", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAuth("token%0d%0aX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})
}

func TestBuilder_WithAPIKey(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithAPIKey("X-API-Key", "secret-123")

	require.Same(t, b, result)
	require.Equal(t, "secret-123", b.headers.Get("X-API-Key"))
}

func TestBuilder_WithAPIKey_CRLFInjection_Prevented(t *testing.T) {
	t.Run("key with injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAPIKey("X-API-Key\r\nX-Injected: malicious", "secret")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("value with carriage return", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAPIKey("X-API-Key", "secret\r\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("value with newline", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAPIKey("X-API-Key", "secret\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("value with encoded crlf", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithAPIKey("X-API-Key", "secret%0d%0aX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})
}

func TestBuilder_WithBasicAuth_CRLFInjection_Prevented(t *testing.T) {
	t.Run("username with injection", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithBasicAuth("user\r\nX-Injected: malicious", "pass")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("password with carriage return", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithBasicAuth("user", "pass\r\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("password with newline", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithBasicAuth("user", "pass\nX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})

	t.Run("password with encoded crlf", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithBasicAuth("user", "pass%0d%0aX-Injected: malicious")

		require.ErrorIs(t, b.buildError, ErrInvalidHeaderValue)
	})
}

func TestBuilder_WithTimeout(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithTimeout(60)

	require.Same(t, b, result)
	require.Equal(t, 60, int(b.timeout))
}

func TestBuilder_buildPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pathParams map[string]string
		expected   string
	}{
		{
			name:       "single param",
			path:       "/users/{id}",
			pathParams: map[string]string{"id": "123"},
			expected:   "/users/123",
		},
		{
			name:       "multiple params",
			path:       "/users/{id}/posts/{postId}",
			pathParams: map[string]string{"id": "123", "postId": "456"},
			expected:   "/users/123/posts/456",
		},
		{
			name:       "no params",
			path:       "/users",
			pathParams: map[string]string{},
			expected:   "/users",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Builder{path: tc.path, pathParams: tc.pathParams}
			result := b.buildPath()
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestBuilder_Build(t *testing.T) {
	t.Run("simple request", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users/{id}", "GET")
		b.WithPathParam("id", "123")
		b.WithQueryParam("limit", "10")
		b.WithAuth("token-123")

		req, err := b.Build()

		require.NoError(t, err)
		require.NotNil(t, req)
		require.Equal(t, "GET", req.Method)
		require.Equal(t, "Bearer token-123", req.Header.Get("Authorization"))
	})

	t.Run("query params", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "GET")
		b.WithQueryParam("limit", "10")
		b.WithQueryParam("offset", "0")

		req, err := b.Build()

		require.NoError(t, err)
		require.Contains(t, req.URL.String(), "limit=10")
		require.Contains(t, req.URL.String(), "offset=0")
	})

	t.Run("content type", func(t *testing.T) {
		b := NewBuilder("https://api.example.com", "/users", "POST")
		b.WithBody(map[string]string{"name": "test"})

		req, err := b.Build()

		require.NoError(t, err)
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
	})
}

func TestParamSerializer_Serialize(t *testing.T) {
	ps := NewParamSerializer("form", true)

	t.Run("string", func(t *testing.T) {
		result := ps.Serialize("name", "test")
		require.Equal(t, "test", result.Get("name"))
	})

	t.Run("integer", func(t *testing.T) {
		result := ps.Serialize("count", 42)
		require.Equal(t, "42", result.Get("count"))
	})

	t.Run("float", func(t *testing.T) {
		result := ps.Serialize("price", 3.14)
		require.Equal(t, "3.14", result.Get("price"))
	})

	t.Run("boolean", func(t *testing.T) {
		result := ps.Serialize("active", true)
		require.Equal(t, "true", result.Get("active"))
	})
}

func TestParamSerializer_serializeArray(t *testing.T) {
	t.Run("explode form", func(t *testing.T) {
		ps := NewParamSerializer("form", true)
		result := make(url.Values)
		ps.serializeArray(result, "tags", []string{"a", "b", "c"})

		require.Equal(t, "a", result["tags"][0])
		require.Equal(t, "b", result["tags"][1])
		require.Equal(t, "c", result["tags"][2])
	})

	t.Run("non-explode form", func(t *testing.T) {
		ps := NewParamSerializer("form", false)
		result := make(url.Values)
		ps.serializeArray(result, "tags", []string{"a", "b", "c"})

		require.Equal(t, "a,b,c", result.Get("tags"))
	})
}

func TestBuilder_MultipartBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/upload", "POST")

	fields := map[string]string{
		"name": "test",
		"desc": "description",
	}

	_, err := b.MultipartBody(fields, map[string]string{})

	if err != nil {
		t.Errorf("MultipartBody error: %v", err)
	} else {
		require.NotNil(t, b.body)
		require.Contains(t, b.contentType, "multipart/form-data")
	}
}

func TestBuilder_XMLBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/data", "POST")

	type Data struct {
		Name string `xml:"name"`
	}

	b.XMLBody(Data{Name: "test"})

	require.NotNil(t, b.body)
	require.Equal(t, "application/xml", b.contentType)
}

func TestBuilder_PlainTextBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/data", "POST")

	b.PlainTextBody("test text")

	require.NotNil(t, b.body)
	require.Equal(t, "text/plain", b.contentType)
}

func TestBuilder_WithBasicAuth(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	b.WithBasicAuth("user", "pass")

	auth := b.headers.Get("Authorization")
	require.NotEmpty(t, auth)
	require.Contains(t, auth, "Basic")
}

func TestBuilder_JSONBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "POST")
	data := map[string]string{"name": "test", "email": "test@example.com"}

	b.JSONBody(data)

	require.NotNil(t, b.body)
	require.Equal(t, "application/json", b.contentType)
}

func TestBuilder_FormBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/create", "POST")
	data := map[string]string{
		"name":  "test",
		"email": "test@example.com",
	}

	b.FormBody(data)

	require.NotNil(t, b.body)
	require.Equal(t, "application/x-www-form-urlencoded", b.contentType)
}

func TestBuilder_WithTypedQueryParam(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		expect string
	}{
		{"string", "test-value", "test-value"},
		{"int", 42, "42"},
		{"int64", int64(123456789), "123456789"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder("https://api.example.com", "/users", "GET")
			b.WithTypedQueryParam("param", tc.value)

			require.Equal(t, tc.expect, b.queryParams.Get("param"))
		})
	}
}

func TestBuilder_WithBody_Reader(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/data", "POST")
	provider := &strings.Reader{}

	b.WithBody(provider)

	require.NotNil(t, b.body)
}

func TestBuilder_WithBody_Bytes(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/data", "POST")
	data := []byte("test data")

	b.WithBody(data)

	require.NotNil(t, b.body)
}

func TestBuilder_MultipartBodyWithFiles(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/upload", "POST")

	fields := map[string]string{"name": "test"}
	files := map[string]string{
		"file": "/dev/null",
	}

	_, err := b.MultipartBody(fields, files)

	if err != nil {
		t.Logf("MultipartBody with file error (expected for non-existent file): %v", err)
	}
}

func TestBuilder_Build_DefaultHeaders(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	req, err := b.Build()

	require.NoError(t, err)
	require.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestBuilder_MethodCaseNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"get", "GET"},
		{"post", "POST"},
		{"Put", "PUT"},
		{"DELETE", "DELETE"},
		{"patch", "PATCH"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			b := NewBuilder("https://api.example.com", "/users", tc.input)
			require.Equal(t, tc.expected, b.method)
		})
	}
}

func TestBuilder_QueryEmptyValue(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	b.WithQueryParam("empty", "")
	b.WithQueryParam("filled", "value")

	require.Empty(t, b.queryParams.Get("empty"))
	require.Equal(t, "value", b.queryParams.Get("filled"))
}

func TestDeepObjectSerialization_Primitive(t *testing.T) {
	result := serializeDeepObject("user", map[string]interface{}{"name": "test"})

	require.NotEmpty(t, result)
}

func TestWithDeepObjectParam_Nested(t *testing.T) {
	type Address struct {
		City string `json:"city"`
	}

	type User struct {
		Name    string `json:"name"`
		Address *Address
	}

	user := User{Name: "test", Address: &Address{City: "NY"}}
	b := NewBuilder("https://api.example.com", "/users", "GET")
	b.WithDeepObjectParam("user", user)

	require.Contains(t, b.queryParams, "user[name]")
}

func TestWithDeepObjectParam_Array(t *testing.T) {
	type User struct {
		Tags []string `json:"tags"`
	}

	user := User{Tags: []string{"a", "b", "c"}}
	b := NewBuilder("https://api.example.com", "/users", "GET")
	b.WithDeepObjectParam("user", user)

	require.Contains(t, b.queryParams, "user[tags]")
}

func TestBuilder_URLConstruction(t *testing.T) {
	b := NewBuilder("https://api.example.com/", "/users/{id}", "GET")
	b.WithPathParam("id", "123")

	req, err := b.Build()

	require.NoError(t, err)
	require.Contains(t, req.URL.String(), "/users/123")
}

func TestBuilder_MaxResponseBody(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	require.Equal(t, int64(10<<20), b.maxResponseBody)

	b.WithMaxResponseBody(5 << 20)
	require.Equal(t, int64(5<<20), b.maxResponseBody)

	b.WithMaxResponseBody(100 << 20)
	require.Equal(t, int64(100<<20), b.maxResponseBody)
}

func TestBuilder_MaxResponseBody_Zero(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")
	b.WithMaxResponseBody(0)

	require.Equal(t, int64(0), b.maxResponseBody)
}

func TestBuilder_MaxResponseBody_Chainable(t *testing.T) {
	b := NewBuilder("https://api.example.com", "/users", "GET")

	result := b.WithMaxResponseBody(5 << 20)
	require.Same(t, b, result)
}
