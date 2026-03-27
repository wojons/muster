package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openapi-cli/openapi-cli/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	require.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
}

func TestNewClientWithOptions(t *testing.T) {
	client := NewClient(
		WithBaseURL("https://api.example.com"),
		WithHeader("Authorization", "Bearer token123"),
		WithHeader("X-Custom", "value"),
		WithTimeout(10*time.Second),
	)

	require.NotNil(t, client)
	assert.Equal(t, "https://api.example.com", client.baseURL)
	assert.Equal(t, "Bearer token123", client.headers["Authorization"])
	assert.Equal(t, "value", client.headers["X-Custom"])
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
}

func TestNewClientWithTracer(t *testing.T) {
	tracer := telemetry.NewTracer(trace.NewNoopTracerProvider().Tracer("test"))
	client := NewClientWithTracer(tracer)

	require.NotNil(t, client)
	assert.Equal(t, tracer, client.tracer)
}

func TestClient_SetTracer(t *testing.T) {
	client := NewClient()
	tracer := telemetry.NewTracer(trace.NewNoopTracerProvider().Tracer("test"))

	client.SetTracer(tracer)
	assert.Equal(t, tracer, client.tracer)
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "success"}`))
	}))
	defer server.Close()

	client := NewClient()
	resp, err := client.Get(context.Background(), server.URL+"/test")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Get_WithBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/data", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	resp, err := client.Get(context.Background(), "/api/data")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, `{"key": "value"}`, string(body))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient()
	bodyContent := strings.NewReader(`{"key": "value"}`)
	resp, err := client.Post(context.Background(), server.URL+"/test", "application/json", bodyContent)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_HTTPMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			w.WriteHeader(http.StatusAccepted)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := NewClient()
	ctx := context.Background()

	// Test GET
	resp, err := client.Get(ctx, server.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Test POST
	resp, err = client.Post(ctx, server.URL+"/test", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	// Test PUT
	resp, err = client.Put(ctx, server.URL+"/test", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	_ = resp.Body.Close()

	// Test DELETE
	resp, err = client.Delete(ctx, server.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	// Test PATCH
	resp, err = client.Patch(ctx, server.URL+"/test", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_DefaultHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(
		WithHeader("Authorization", "Bearer test-token"),
	)

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Do_WithTracer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": "test"}`))
	}))
	defer server.Close()

	tracer := telemetry.NewTracer(trace.NewNoopTracerProvider().Tracer("test"))
	client := NewClientWithTracer(tracer)

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp, err := client.Do(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Do_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	tracer := telemetry.NewTracer(trace.NewNoopTracerProvider().Tracer("test"))
	client := NewClientWithTracer(tracer)

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp, err := client.Do(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Do_ClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	client := NewClient()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	resp, err := client.Do(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

}

func TestClient_Do_NetworkError(t *testing.T) {
	// Create client pointing to non-existent server
	client := NewClient()

	req, _ := http.NewRequest(http.MethodGet, "http://localhost:99999/test", nil)
	resp, err := client.Do(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestClient_HTTPClient(t *testing.T) {
	client := NewClient()
	httpClient := client.HTTPClient()

	assert.NotNil(t, httpClient)
	assert.Equal(t, client.httpClient, httpClient)
}

func TestClient_SetHTTPClient(t *testing.T) {
	client := NewClient()
	newHTTPClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	client.SetHTTPClient(newHTTPClient)
	assert.Equal(t, newHTTPClient, client.httpClient)
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, server.URL+"/test")
	assert.Error(t, err)
}

func TestClient_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient()
	ctx := context.Background()

	// Run concurrent requests using errgroup or wait group
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(ctx, server.URL+"/test")
			if err != nil {
				errors <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}
			_ = resp.Body.Close()

		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent request failed: %v", err)
		}
	}
}
