package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/openapi"
)

// testSpec builds a minimal OpenAPI spec for integration testing.
func testSpec(baseURL string) *openapi3.T {
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "Test API", Version: "1.0.0"},
		Servers: openapi3.Servers{
			{URL: baseURL},
		},
		Paths: &openapi3.Paths{},
	}
	pathItem := &openapi3.PathItem{
		Get: &openapi3.Operation{
			OperationID: "testGet",
			Summary:     "Test GET endpoint",
			Responses: &openapi3.Responses{},
		},
	}
	pathItem.Get.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{Description: stringPtr("OK")},
	})
	doc.Paths.Set("/test", pathItem)
	return doc
}

func stringPtr(s string) *string { return &s }

// TestExecutor_RateLimit_BlocksExcess verifies that an OpenAPIExecutor
// created with NewExecutorWithRateLimit returns 429 errors when the
// burst is exhausted.
func TestExecutor_RateLimit_BlocksExcess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer backend.Close()

	spec := testSpec(backend.URL)
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	require.NoError(t, err)

	exec := NewExecutorWithRateLimit(backend.URL, ops, 1, 1)

	// First call consumes the single token — should succeed.
	_, err = exec.Execute(context.Background(), "testGet", nil)
	require.NoError(t, err, "first call should succeed")

	// Second call should be rate-limited (429).
	_, err = exec.Execute(context.Background(), "testGet", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error")
	assert.Contains(t, err.Error(), "429")
}

// TestExecutor_RateLimit_Disabled verifies that NewExecutorWithRateLimit
// with ratePerSec <= 0 does NOT rate-limit requests.
func TestExecutor_RateLimit_Disabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer backend.Close()

	spec := testSpec(backend.URL)
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	require.NoError(t, err)

	// ratePerSec=0 means disabled
	exec := NewExecutorWithRateLimit(backend.URL, ops, 0, 5)

	for i := 0; i < 10; i++ {
		_, err := exec.Execute(context.Background(), "testGet", nil)
		require.NoError(t, err, "request %d should succeed when rate limiting is disabled", i+1)
	}
}

// TestExecutor_RateLimit_ExistingConstructorStillWorks verifies that the
// original NewExecutor constructor is unchanged and allows unlimited calls.
func TestExecutor_RateLimit_ExistingConstructorStillWorks(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer backend.Close()

	spec := testSpec(backend.URL)
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	require.NoError(t, err)

	exec := NewExecutor(backend.URL, ops)

	for i := 0; i < 10; i++ {
		_, err := exec.Execute(context.Background(), "testGet", nil)
		require.NoError(t, err, "existing constructor should not rate-limit")
	}
}
