package builtin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestCommand(t *testing.T) {
	t.Run("creates request command", func(t *testing.T) {
		cmd := newRequestCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "request", cmd.Name())
		assert.Equal(t, "Make HTTP requests", cmd.Short)
	})

	t.Run("requires exactly two arguments", func(t *testing.T) {
		cmd := newRequestCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 2 arg")
	})

	t.Run("has header flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("header")
		assert.NotNil(t, flag)
		assert.Equal(t, "[]", flag.DefValue)
	})

	t.Run("has param flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("param")
		assert.NotNil(t, flag)
	})

	t.Run("has output flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("output")
		assert.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
	})

	t.Run("has verbose flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("verbose")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has timeout flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("timeout")
		assert.NotNil(t, flag)
	})

	t.Run("has body flag", func(t *testing.T) {
		cmd := newRequestCommand()

		flag := cmd.Flags().Lookup("body")
		assert.NotNil(t, flag)
	})
}

func TestHandleRequest(t *testing.T) {
	t.Run("makes successful GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		err := handleRequest("GET", server.URL, []string{}, []string{}, "json", false, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		headers := []string{"Content-Type: application/json"}

		err := handleRequest("GET", server.URL, headers, []string{}, "json", false, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles query parameters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "value", r.URL.Query().Get("key"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		params := []string{"key=value"}

		err := handleRequest("GET", server.URL, []string{}, params, "json", false, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles verbose output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		err := handleRequest("GET", server.URL, []string{}, []string{}, "json", true, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles POST with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		body := `{"test": "data"}`

		err := handleRequest("POST", server.URL, []string{}, []string{}, "json", false, 30*time.Second, body)

		require.NoError(t, err)
	})

	t.Run("handles PUT with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PUT", r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		body := `{"test": "updated"}`

		err := handleRequest("PUT", server.URL, []string{}, []string{}, "json", false, 30*time.Second, body)

		require.NoError(t, err)
	})

	t.Run("handles 4xx error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		err := handleRequest("GET", server.URL, []string{}, []string{}, "json", false, 30*time.Second, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Found")
	})

	t.Run("handles 5xx error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		err := handleRequest("GET", server.URL, []string{}, []string{}, "json", false, 30*time.Second, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("handles invalid URL", func(t *testing.T) {
		err := handleRequest("GET", "://invalid-url", []string{}, []string{}, "json", false, 30*time.Second, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse URL")
	})

	t.Run("handles multiple headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		headers := []string{
			"Content-Type: application/json",
			"Authorization: Bearer token123",
		}

		err := handleRequest("GET", server.URL, headers, []string{}, "json", false, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles multiple params", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "value1", r.URL.Query().Get("key1"))
			assert.Equal(t, "value2", r.URL.Query().Get("key2"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		params := []string{
			"key1=value1",
			"key2=value2",
		}

		err := handleRequest("GET", server.URL, []string{}, params, "json", false, 30*time.Second, "")

		require.NoError(t, err)
	})

	t.Run("handles PATCH with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "PATCH", r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		body := `{"test": "patched"}`

		err := handleRequest("PATCH", server.URL, []string{}, []string{}, "json", false, 30*time.Second, body)

		require.NoError(t, err)
	})
}

func TestRequestCommandIntegration(t *testing.T) {
	t.Run("full request command execution", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result": "success"}`))
		}))
		defer server.Close()

		cmd := newRequestCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		_ = cmd.Flags().Set("output", "json")
		_ = cmd.Flags().Set("timeout", "30s")

		err := cmd.RunE(cmd, []string{"GET", server.URL})

		require.NoError(t, err)
	})

	t.Run("request with custom timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cmd := newRequestCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		_ = cmd.Flags().Set("timeout", "5s")

		err := cmd.RunE(cmd, []string{"GET", server.URL})

		require.NoError(t, err)
	})
}
