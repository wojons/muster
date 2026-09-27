package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolExecutor_BearerAuthOnEveryOutboundMethod(t *testing.T) {
	const token = "executor-regression-token"

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer "+token {
					t.Errorf("Authorization = %q, want configured bearer credential", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			t.Cleanup(api.Close)

			configs := map[string]ExecutionConfig{
				"authenticated": {
					BaseURL:     api.URL,
					Method:      method,
					Path:        "/api/v1/resource",
					InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
				},
			}
			executor := NewOpenAPIExecutor(configs, WithBearerToken(token))
			if _, err := executor.Execute(context.Background(), "authenticated", map[string]interface{}{}); err != nil {
				t.Fatalf("Execute(%s) failed: %v", method, err)
			}
		})
	}
}

func TestToolExecutor_ConfiguredBearerCannotBeOverriddenByToolArguments(t *testing.T) {
	const token = "configured-regression-token"

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q, want process-level credential", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	configs := map[string]ExecutionConfig{
		"authenticated": {
			BaseURL: api.URL,
			Method:  http.MethodGet,
			Path:    "/api/v1/resource",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"Authorization":{"type":"string","in":"header"}}
			}`),
		},
	}
	executor := NewOpenAPIExecutor(configs, WithBearerToken(token))
	_, err := executor.Execute(context.Background(), "authenticated", map[string]interface{}{
		"Authorization": "Bearer caller-supplied-value",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestToolExecutor_WithoutBearerPreservesUnauthenticatedRequests(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no header", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	configs := map[string]ExecutionConfig{
		"public": {
			BaseURL:     api.URL,
			Method:      http.MethodGet,
			Path:        "/public",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	if _, err := NewOpenAPIExecutor(configs).Execute(context.Background(), "public", map[string]interface{}{}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}
