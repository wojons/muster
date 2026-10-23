package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MUSTER-001: an api-key API (e.g. DuckBrain) rejects Authorization: Bearer and
// expects the secret in its own header. With WithCredentialType("api-key") the
// executor attaches X-API-Key and leaves Authorization unset, using the same
// convention as the CLI's stored api-key credentials.
func TestToolExecutor_ApiKeyCredentialTypeUsesXAPIKeyHeader(t *testing.T) {
	const token = "api-key-regression-token"

	var sawAuthorization, sawAPIKey string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthorization = r.Header.Get("Authorization")
		sawAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	configs := map[string]ExecutionConfig{
		"create-memory": {
			BaseURL:     api.URL,
			Method:      http.MethodPost,
			Path:        "/api/memories",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	executor := NewOpenAPIExecutor(configs, WithBearerToken(token), WithCredentialType("api-key"))
	if _, err := executor.Execute(context.Background(), "create-memory", map[string]interface{}{}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if sawAPIKey != token {
		t.Errorf("X-API-Key = %q, want %q", sawAPIKey, token)
	}
	if sawAuthorization != "" {
		t.Errorf("Authorization must stay unset for an api-key credential, got %q", sawAuthorization)
	}
}

// The default (no credential type configured) stays exactly as before:
// Authorization: Bearer <token>.
func TestToolExecutor_DefaultCredentialTypeStaysBearer(t *testing.T) {
	const token = "bearer-regression-token"

	var sawAuthorization, sawAPIKey string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthorization = r.Header.Get("Authorization")
		sawAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	configs := map[string]ExecutionConfig{
		"list": {
			BaseURL:     api.URL,
			Method:      http.MethodGet,
			Path:        "/api/memories",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	executor := NewOpenAPIExecutor(configs, WithBearerToken(token))
	if _, err := executor.Execute(context.Background(), "list", map[string]interface{}{}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if sawAuthorization != "Bearer "+token {
		t.Errorf("Authorization = %q, want configured bearer", sawAuthorization)
	}
	if sawAPIKey != "" {
		t.Errorf("X-API-Key must stay unset for a bearer credential, got %q", sawAPIKey)
	}
}
