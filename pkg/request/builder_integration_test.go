package request

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecuteWithResult_ResponseWithinLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("small response"))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(100)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "small response", string(result.Body))
}

func TestExecuteWithResult_ResponseExceedsLimit(t *testing.T) {
	largeResponse := strings.Repeat("x", 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(largeResponse))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(100)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds max size")
	require.Nil(t, result)
}

func TestExecuteWithResult_ResponseExactlyAtLimit(t *testing.T) {
	exactSize := strings.Repeat("y", 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(exactSize))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(100)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Body, 100)
}

func TestExecuteWithResult_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(100)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Body)
}

func TestExecuteWithResult_LargeLimit(t *testing.T) {
	mediumResponse := strings.Repeat("z", 1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mediumResponse))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(10 << 20)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Body, 1024)
}

func TestExecuteWithResult_DefaultLimit(t *testing.T) {
	smallResponse := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(smallResponse))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "test", string(result.Body))
}

func TestExecuteWithResult_DoSTest_VeryLargeResponse(t *testing.T) {
	veryLargeResponse := strings.Repeat("a", 1<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(veryLargeResponse))
	}))
	defer server.Close()

	b := NewBuilder(server.URL, "/", "GET")
	b.WithMaxResponseBody(100 << 10)
	b.timeout = 5 * time.Second

	ctx := context.Background()
	result, err := b.ExecuteWithResult(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds max size")
	require.Nil(t, result)
}
