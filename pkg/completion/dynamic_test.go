package completion

import (
	"context"
	"strings"
	"testing"
)

type mockAPIClient struct{}

func (m *mockAPIClient) ListResources(ctx context.Context, resourceType string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{"name": "pod-1"},
		{"name": "pod-2"},
		{"name": "pod-3"},
	}, nil
}

func (m *mockAPIClient) ListNamespaces(ctx context.Context) ([]string, error) {
	return []string{"default", "staging", "production"}, nil
}

func (m *mockAPIClient) ListProfiles(ctx context.Context) ([]string, error) {
	return []string{"default", "dev", "prod"}, nil
}

func TestDynamicCompleter(t *testing.T) {
	client := &mockAPIClient{}
	completer := NewDynamicCompleter(client)

	t.Run("CompleteResources", func(t *testing.T) {
		results, err := completer.CompleteResources(context.Background(), "pod")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(results) == 0 {
			t.Errorf("Expected at least 1 result, got %d", len(results))
		}

		hasMatch := false
		for _, r := range results {
			if strings.HasPrefix(r, "pod") {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			t.Errorf("Expected at least one result to start with 'pod'")
		}
	})

	t.Run("CompleteNamespaces", func(t *testing.T) {
		results, err := completer.CompleteNamespaces(context.Background(), "def")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if results[0] != "default" {
			t.Errorf("Expected 'default', got '%s'", results[0])
		}
	})

	t.Run("CompleteProfiles", func(t *testing.T) {
		results, err := completer.CompleteProfiles(context.Background(), "d")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		expected := map[string]bool{"default": true, "dev": true}
		for _, r := range results {
			if !expected[r] {
				t.Errorf("Unexpected result '%s'", r)
			}
		}
	})

	t.Run("CompleteHTTPMethods", func(t *testing.T) {
		results := completer.CompleteHTTPMethods("G")

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if results[0] != "GET" {
			t.Errorf("Expected 'GET', got '%s'", results[0])
		}
	})

	t.Run("CompleteOutputFormats", func(t *testing.T) {
		results := completer.CompleteOutputFormats("js")

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if results[0] != "json" {
			t.Errorf("Expected 'json', got '%s'", results[0])
		}
	})

	t.Run("Caching", func(t *testing.T) {
		callCount := 0
		cachedClient := &cachedMockClient{&mockAPIClient{}, &callCount}
		cachedCompleter := NewDynamicCompleter(cachedClient)

		_, _ = cachedCompleter.CompleteResources(context.Background(), "")
		_, _ = cachedCompleter.CompleteResources(context.Background(), "")

		if callCount != 1 {
			t.Errorf("Expected 1 API call (cached), got %d", callCount)
		}
	})
}

type cachedMockClient struct {
	*mockAPIClient
	callCount *int
}

func (c *cachedMockClient) ListResources(ctx context.Context, resourceType string) ([]map[string]interface{}, error) {
	*c.callCount++
	return c.mockAPIClient.ListResources(ctx, resourceType)
}

func (c *cachedMockClient) ListNamespaces(ctx context.Context) ([]string, error) {
	return c.mockAPIClient.ListNamespaces(ctx)
}

func (c *cachedMockClient) ListProfiles(ctx context.Context) ([]string, error) {
	return c.mockAPIClient.ListProfiles(ctx)
}

// =============================================================================
// DefaultHTTPClient tests
// =============================================================================

func TestNewDefaultHTTPClient(t *testing.T) {
	c := NewDefaultHTTPClient()
	if c == nil {
		t.Fatal("NewDefaultHTTPClient() returned nil")
	}
	if c.client == nil {
		t.Error("DefaultHTTPClient.client is nil")
	}
}

func TestDefaultHTTPClient_ListResources_NotImplemented(t *testing.T) {
	c := NewDefaultHTTPClient()
	resources, err := c.ListResources(context.Background(), "pods")
	if err == nil {
		t.Error("expected error from ListResources")
	}
	if resources != nil {
		t.Error("expected nil resources on error")
	}
}

func TestDefaultHTTPClient_ListNamespaces_NotImplemented(t *testing.T) {
	c := NewDefaultHTTPClient()
	namespaces, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Error("expected error from ListNamespaces")
	}
	if namespaces != nil {
		t.Error("expected nil namespaces on error")
	}
}

func TestDefaultHTTPClient_ListProfiles_NotImplemented(t *testing.T) {
	c := NewDefaultHTTPClient()
	profiles, err := c.ListProfiles(context.Background())
	if err == nil {
		t.Error("expected error from ListProfiles")
	}
	if profiles != nil {
		t.Error("expected nil profiles on error")
	}
}
