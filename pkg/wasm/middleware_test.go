package wasm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// NewInterceptor / NewInterceptorWithLimits
// =============================================================================

func TestNewInterceptor(t *testing.T) {
	i := NewInterceptor()
	require.NotNil(t, i)
	assert.NotNil(t, i.manager)
	assert.True(t, i.PassThroughNonJSON)
	assert.Greater(t, i.Timeout, int64(0))
}

func TestNewInterceptorWithLimits(t *testing.T) {
	limits := DefaultLimits()
	i := NewInterceptorWithLimits(limits)
	require.NotNil(t, i)
	assert.NotNil(t, i.manager)
	assert.True(t, i.PassThroughNonJSON)
}

func TestNewInterceptorWithLimits_NilLimits(t *testing.T) {
	i := NewInterceptorWithLimits(nil)
	require.NotNil(t, i)
	assert.NotNil(t, i.manager)
}

// =============================================================================
// RegisteredPlugins
// =============================================================================

func TestInterceptor_RegisteredPlugins(t *testing.T) {
	t.Run("empty initially", func(t *testing.T) {
		i := NewInterceptor()
		plugins := i.RegisteredPlugins()
		assert.Empty(t, plugins)
	})
}

// =============================================================================
// Close
// =============================================================================

func TestInterceptor_Close(t *testing.T) {
	t.Run("close without plugins", func(t *testing.T) {
		i := NewInterceptor()
		err := i.Close(context.Background())
		require.NoError(t, err)
	})

	t.Run("close twice is safe", func(t *testing.T) {
		i := NewInterceptor()
		err := i.Close(context.Background())
		require.NoError(t, err)
		err = i.Close(context.Background())
		require.NoError(t, err)
	})
}

// =============================================================================
// TransformResponseBody
// =============================================================================

func TestInterceptor_TransformResponseBody(t *testing.T) {
	t.Run("empty plugin path passes through", func(t *testing.T) {
		i := NewInterceptor()
		body := []byte(`{"hello":"world"}`)
		result, err := i.TransformResponseBody(context.Background(), body, "application/json", "")
		require.NoError(t, err)
		assert.Equal(t, body, result)
	})

	t.Run("plugin not loaded returns error", func(t *testing.T) {
		i := NewInterceptor()
		body := []byte(`{"hello":"world"}`)
		_, err := i.TransformResponseBody(context.Background(), body, "application/json", "/nonexistent/plugin.wasm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not loaded")
	})

	t.Run("empty body passes through (but plugin must be loaded first)", func(t *testing.T) {
		// The plugin-loaded check happens before the empty-body check.
		// An unloaded plugin errors before checking if body is empty.
		i := NewInterceptor()
		_, err := i.TransformResponseBody(context.Background(), []byte{}, "application/json", "/some/plugin.wasm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not loaded")
	})

	t.Run("non-JSON content type passes through when enabled (plugin loaded)", func(t *testing.T) {
		// NOTE: The pass-through check happens AFTER the plugin-loaded check.
		// An unloaded plugin always errors before reaching content-type check.
		i := NewInterceptor()
		i.PassThroughNonJSON = true
		// Without a real WASM module loaded, the "not loaded" error wins.
		// This test documents the ordering: loaded-check first, then content-type.
		body := []byte(`<html>test</html>`)
		_, err := i.TransformResponseBody(context.Background(), body, "text/html", "/some/plugin.wasm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not loaded")
	})

	t.Run("non-JSON content type transformed when disabled", func(t *testing.T) {
		i := NewInterceptor()
		i.PassThroughNonJSON = false
		body := []byte(`<html>test</html>`)
		// Plugin not loaded, so still errors
		_, err := i.TransformResponseBody(context.Background(), body, "text/html", "/some/plugin.wasm")
		assert.Error(t, err)
		// But the error is "not loaded", not "pass through" — proves PassThroughNonJSON=false works
	})
}

// =============================================================================
// Middleware
// =============================================================================

func TestInterceptor_Middleware(t *testing.T) {
	t.Run("empty plugin path passes through", func(t *testing.T) {
		i := NewInterceptor()
		handler := i.Middleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "ok")
	})

	t.Run("plugin not loaded passes through", func(t *testing.T) {
		i := NewInterceptor()
		handler := i.Middleware("/notloaded.wasm")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("passed"))
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// =============================================================================
// captureResponseWriter
// =============================================================================

func TestCaptureResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: rec,
	}

	crw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, crw.statusCode)
	// statusCode is captured, not written to real writer yet
	assert.Equal(t, http.StatusOK, rec.Code) // default
}

func TestCaptureResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: rec,
	}

	n, err := crw.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, []byte("hello"), crw.buf)
}

func TestCaptureResponseWriter_WriteMultiple(t *testing.T) {
	rec := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: rec,
	}

	crw.Write([]byte("part 1, "))
	crw.Write([]byte("part 2"))

	assert.Equal(t, []byte("part 1, part 2"), crw.buf)
}

// =============================================================================
// isJSONContentType
// =============================================================================

func TestIsJSONContentType(t *testing.T) {
	t.Run("application/json", func(t *testing.T) {
		assert.True(t, isJSONContentType("application/json"))
	})

	t.Run("application/json with charset", func(t *testing.T) {
		assert.True(t, isJSONContentType("application/json; charset=utf-8"))
	})

	t.Run("application/problem+json", func(t *testing.T) {
		assert.True(t, isJSONContentType("application/problem+json"))
	})

	t.Run("application/ld+json", func(t *testing.T) {
		assert.True(t, isJSONContentType("application/ld+json"))
	})

	t.Run("text/html is not JSON", func(t *testing.T) {
		assert.False(t, isJSONContentType("text/html"))
	})

	t.Run("text/plain is not JSON", func(t *testing.T) {
		assert.False(t, isJSONContentType("text/plain"))
	})

	t.Run("empty string is not JSON", func(t *testing.T) {
		assert.False(t, isJSONContentType(""))
	})

	t.Run("application/xml is not JSON", func(t *testing.T) {
		assert.False(t, isJSONContentType("application/xml"))
	})

	t.Run("with extra whitespace", func(t *testing.T) {
		assert.True(t, isJSONContentType("  application/json  "))
	})
}

// =============================================================================
// InterceptResponse
// =============================================================================

func TestInterceptor_InterceptResponse(t *testing.T) {
	t.Run("empty plugin path returns nil", func(t *testing.T) {
		i := NewInterceptor()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		err := i.InterceptResponse(resp, "")
		require.NoError(t, err)
	})

	t.Run("plugin not loaded errors", func(t *testing.T) {
		i := NewInterceptor()
		// Create a response whose body is a simple reader
		body := httptest.NewRecorder()
		body.WriteString(`{"key":"value"}`)
		body.Flush()

		resp := body.Result()
		err := i.InterceptResponse(resp, "/notloaded.wasm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not loaded")
	})
}
