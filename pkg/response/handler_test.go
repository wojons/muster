package response

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestResponse is a helper that constructs an *http.Response with the given
// status code, status text, and body string. The body is wrapped in an
// io.NopCloser so it behaves like a real HTTP response body.
func newTestResponse(statusCode int, status string, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// ---------------------------------------------------------------------------
// NewHandler
// ---------------------------------------------------------------------------

func TestNewHandler(t *testing.T) {
	t.Run("creates handler with valid config", func(t *testing.T) {
		h := NewHandler(Config{
			DefaultFormat:    "json",
			SupportedFormats: []string{"json", "yaml"},
			Writer:           io.Discard,
		})
		require.NotNil(t, h, "NewHandler should return a non-nil Handler")
	})

	t.Run("sets default TreatNon200AsError to true", func(t *testing.T) {
		h := NewHandler(Config{
			DefaultFormat: "json",
			Writer:        io.Discard,
		})
		assert.True(t, h.TreatNon200AsError,
			"NewHandler should default TreatNon200AsError to true")
	})
}

// ---------------------------------------------------------------------------
// HandleResponse — main response processing logic
// ---------------------------------------------------------------------------

func TestHandler_HandleResponse(t *testing.T) {
	h := &Handler{TreatNon200AsError: true}

	t.Run("nil response returns error", func(t *testing.T) {
		body, data, err := h.HandleResponse(nil)
		assert.Error(t, err, "handleResponse with nil response should error")
		assert.Contains(t, err.Error(), "nil response")
		assert.Nil(t, body, "body should be nil")
		assert.Nil(t, data, "data should be nil")
	})

	t.Run("200 with valid JSON parses into data map", func(t *testing.T) {
		resp := newTestResponse(200, "200 OK", `{"key":"value","num":42}`)
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		require.NotNil(t, body)
		assert.JSONEq(t, `{"key":"value","num":42}`, string(body))
		require.NotNil(t, data)
		assert.Equal(t, "value", data["key"])
		assert.Equal(t, float64(42), data["num"])
	})

	t.Run("200 with non-JSON body returns raw bytes and nil data", func(t *testing.T) {
		resp := newTestResponse(200, "200 OK", "plain text body")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, "plain text body", string(body))
		assert.Nil(t, data, "data should be nil for non-JSON body")
	})

	t.Run("200 with empty body returns empty bytes and nil data", func(t *testing.T) {
		resp := newTestResponse(200, "200 OK", "")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, body)
		assert.Nil(t, data, "data should be nil for empty body")
	})

	t.Run("201 created with JSON body parses successfully", func(t *testing.T) {
		resp := newTestResponse(201, "201 Created", `{"id":"abc123","status":"created"}`)
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.Contains(t, string(body), "abc123")
		require.NotNil(t, data)
		assert.Equal(t, "abc123", data["id"])
	})

	t.Run("204 no content returns empty body", func(t *testing.T) {
		resp := newTestResponse(204, "204 No Content", "")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, body)
		assert.Nil(t, data)
	})

	t.Run("301 redirect is not treated as error", func(t *testing.T) {
		// 301 is a 3xx status — below the 400 threshold, so no HTTPError.
		resp := newTestResponse(301, "301 Moved Permanently", "redirect")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.NotEmpty(t, body)
		assert.Nil(t, data) // not JSON
	})

	t.Run("400 status returns HTTPError", func(t *testing.T) {
		resp := newTestResponse(400, "400 Bad Request", `{"error":"invalid input"}`)
		body, data, err := h.HandleResponse(resp)
		require.Error(t, err)
		assert.Nil(t, data, "data should be nil on error")

		var httpErr *HTTPError
		ok := errors.As(err, &httpErr)
		require.True(t, ok, "error should be an *HTTPError")
		assert.Equal(t, 400, httpErr.StatusCode)
		assert.Equal(t, "400 Bad Request", httpErr.Status)
		assert.Contains(t, string(httpErr.Body), "invalid input")

		// body should still be returned even on error
		require.NotNil(t, body)
		assert.Contains(t, string(body), "invalid input")
	})

	t.Run("401 status returns HTTPError", func(t *testing.T) {
		resp := newTestResponse(401, "401 Unauthorized", `{"error":"unauthorized"}`)
		_, _, err := h.HandleResponse(resp)
		require.Error(t, err)

		var httpErr *HTTPError
		ok := errors.As(err, &httpErr)
		require.True(t, ok)
		assert.Equal(t, 401, httpErr.StatusCode)
		assert.Equal(t, "401 Unauthorized", httpErr.Status)
	})

	t.Run("403 status returns HTTPError", func(t *testing.T) {
		resp := newTestResponse(403, "403 Forbidden", "forbidden")
		_, _, err := h.HandleResponse(resp)
		require.Error(t, err)

		var httpErr *HTTPError
		ok := errors.As(err, &httpErr)
		require.True(t, ok)
		assert.Equal(t, 403, httpErr.StatusCode)
	})

	t.Run("404 status returns HTTPError", func(t *testing.T) {
		resp := newTestResponse(404, "404 Not Found", "not found")
		body, _, err := h.HandleResponse(resp)
		require.Error(t, err)

		var httpErr *HTTPError
		ok := errors.As(err, &httpErr)
		require.True(t, ok)
		assert.Equal(t, 404, httpErr.StatusCode)
		assert.Equal(t, "404 Not Found", httpErr.Status)
		require.NotNil(t, body)
		assert.Equal(t, "not found", string(body))
	})

	t.Run("500 status returns HTTPError", func(t *testing.T) {
		resp := newTestResponse(500, "500 Internal Server Error", "server error")
		body, data, err := h.HandleResponse(resp)
		require.Error(t, err)
		assert.Nil(t, data)

		var httpErr *HTTPError
		ok := errors.As(err, &httpErr)
		require.True(t, ok)
		assert.Equal(t, 500, httpErr.StatusCode)
		assert.Equal(t, "500 Internal Server Error", httpErr.Status)
		require.NotNil(t, body)
		assert.Equal(t, "server error", string(body))
	})
}

// ---------------------------------------------------------------------------
// TreatNon200AsError flag
// ---------------------------------------------------------------------------

func TestHandler_TreatNon200AsError(t *testing.T) {
	t.Run("when false, 400 does not return error", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: false}
		resp := newTestResponse(400, "400 Bad Request", `{"error":"bad"}`)
		// Without TreatNon200AsError, the response is processed normally.
		// The body is valid JSON, so data should be populated.
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err, "should not error when TreatNon200AsError is false")
		require.NotNil(t, body)
		require.NotNil(t, data, "valid JSON should be parsed")
		assert.Equal(t, "bad", data["error"])
	})

	t.Run("when false, 500 returns body as normal", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: false}
		resp := newTestResponse(500, "500 Server Error", "internal error")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, "internal error", string(body))
		assert.Nil(t, data) // not JSON
	})

	t.Run("when true, 500 returns HTTPError", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: true}
		resp := newTestResponse(500, "500 Server Error", "internal error")
		_, _, err := h.HandleResponse(resp)
		require.Error(t, err)
		var httpErr *HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, 500, httpErr.StatusCode)
	})
}

// ---------------------------------------------------------------------------
// ProcessResponse convenience wrapper
// ---------------------------------------------------------------------------

func TestHandler_ProcessResponse(t *testing.T) {
	t.Run("returns nil on successful response", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: true}
		resp := newTestResponse(200, "200 OK", `{"ok":true}`)
		err := h.ProcessResponse(resp, ProcessingOptions{Format: "json"})
		assert.NoError(t, err)
	})

	t.Run("returns error on HTTP error response", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: true}
		resp := newTestResponse(500, "500 Error", "fail")
		err := h.ProcessResponse(resp, ProcessingOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("returns error on nil response", func(t *testing.T) {
		h := &Handler{TreatNon200AsError: true}
		err := h.ProcessResponse(nil, ProcessingOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil response")
	})
}

// ---------------------------------------------------------------------------
// HTTPError type
// ---------------------------------------------------------------------------

func TestHTTPError(t *testing.T) {
	t.Run("Error formats correctly", func(t *testing.T) {
		err := &HTTPError{
			StatusCode: 404,
			Status:     "404 Not Found",
			Body:       []byte(`{"error":"missing"}`),
		}
		msg := err.Error()
		assert.Contains(t, msg, "404")
		assert.Contains(t, msg, "Not Found")
		assert.Contains(t, msg, `{"error":"missing"}`)
	})

	t.Run("implements error interface", func(t *testing.T) {
		err := &HTTPError{
			StatusCode: 500,
			Status:     "500 Internal Server Error",
			Body:       []byte("oops"),
		}
		var e error = err
		assert.Error(t, e)
		assert.Equal(t, "HTTP 500 500 Internal Server Error: oops", e.Error())
	})

	t.Run("Error with empty body still produces output", func(t *testing.T) {
		err := &HTTPError{
			StatusCode: 204,
			Status:     "204 No Content",
			Body:       []byte{},
		}
		msg := err.Error()
		assert.Contains(t, msg, "204")
		assert.NotEmpty(t, msg)
	})
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestHandler_HandleResponse_EdgeCases(t *testing.T) {
	h := &Handler{TreatNon200AsError: true}

	t.Run("malformed JSON is treated as non-JSON body", func(t *testing.T) {
		// Trailing comma makes this invalid JSON.
		resp := newTestResponse(200, "200 OK", `{"key":"value",}`)
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		require.NotNil(t, body)
		assert.Nil(t, data, "malformed JSON should not be parsed as data")
		assert.Equal(t, `{"key":"value",}`, string(body))
	})

	t.Run("body with only whitespace is non-JSON", func(t *testing.T) {
		resp := newTestResponse(200, "200 OK", "   ")
		body, data, err := h.HandleResponse(resp)
		require.NoError(t, err)
		require.NotNil(t, body)
		assert.Nil(t, data)
	})

	t.Run("body is properly closed after HandleResponse", func(t *testing.T) {
		resp := newTestResponse(200, "200 OK", "data")
		_, _, err := h.HandleResponse(resp)
		require.NoError(t, err)
		// Reading the body again should return empty — proves it was closed.
		remaining, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Empty(t, remaining, "body should be fully consumed and closed")
	})

	t.Run("multiple calls on different responses work independently", func(t *testing.T) {
		resp1 := newTestResponse(200, "200 OK", `{"a":1}`)
		resp2 := newTestResponse(400, "400 Bad", `{"error":"bad"}`)
		resp3 := newTestResponse(200, "200 OK", "raw text")

		b1, d1, e1 := h.HandleResponse(resp1)
		require.NoError(t, e1)
		require.NotNil(t, b1)
		require.NotNil(t, d1)
		assert.Equal(t, float64(1), d1["a"])

		_, _, e2 := h.HandleResponse(resp2)
		require.Error(t, e2)

		b3, d3, e3 := h.HandleResponse(resp3)
		require.NoError(t, e3)
		assert.Equal(t, "raw text", string(b3))
		assert.Nil(t, d3)
	})
}
