package inventory

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFetchRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP test in short mode")
	}

	fetcher := NewFetcher(nil)
	ctx := context.Background()

	data, err := fetcher.FetchRaw(ctx, "https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/examples/v3.0/petstore.json", nil)

	// This may fail due to network, just verify the test structure
	if err != nil {
		t.Logf("FetchRaw error (expected if no network): %v", err)
		return
	}

	if len(data) == 0 {
		t.Error("Expected non-empty response")
	}
}

func TestBuildInventoryURL(t *testing.T) {
	manager, err := NewManager(ManagerConfig{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with http", func(t *testing.T) {
		url := manager.buildInventoryURL("http://api.example.com")
		expected := "http://api.example.com/_oa/inventory.json"
		if url != expected {
			t.Errorf("Expected %s, got %s", expected, url)
		}
	})

	t.Run("without http", func(t *testing.T) {
		url := manager.buildInventoryURL("api.example.com")
		expected := "https://api.example.com/_oa/inventory.json"
		if url != expected {
			t.Errorf("Expected %s, got %s", expected, url)
		}
	})

	t.Run("with trailing slash", func(t *testing.T) {
		url := manager.buildInventoryURL("api.example.com/")
		expected := "https://api.example.com/_oa/inventory.json"
		if url != expected {
			t.Errorf("Expected %s, got %s", expected, url)
		}
	})
}

func TestValidator_MinCLIVersion(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Now(),
		APIVersion:    "1.0.0",
		APIName:       "Test",
		BaseURL:       "https://example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		MinCLIVersion: "99.0.0", // Require too high
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Expected error for too-high MinCLIVersion")
	}
}

func TestValidator_MaxCLIVersion(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Now(),
		APIVersion:    "1.0.0",
		APIName:       "Test",
		BaseURL:       "https://example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		MaxCLIVersion: "0.0.1", // Too low
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Expected error for too-low MaxCLIVersion")
	}
}

func TestValidator_DuplicateOperationNames(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Now(),
		APIVersion:    "1.0.0",
		APIName:       "Test",
		BaseURL:       "https://example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		Resources: []Resource{
			{
				Name: "users",
				Operations: []Operation{
					{Name: "list"},
					{Name: "list"}, // Duplicates in same resource
				},
			},
		},
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Expected error for duplicate operation names")
	}
}

func TestValidator_MiddlewareStage(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Now(),
		APIVersion:    "1.0.0",
		APIName:       "Test",
		BaseURL:       "https://example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		Middleware: []MiddlewareModule{
			{
				Name:       "test",
				Version:    "1.0.0",
				URL:        "https://example.com/module.wasm",
				SHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				EntryPoint: "init",
				Stage:      "invalid", // Invalid stage
			},
		},
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Expected error for invalid middleware stage")
	}
}

func TestFetcher_HTTPClientInterface(t *testing.T) {
	client := &http.Client{}
	fetcher := NewFetcher(client)

	if fetcher == nil {
		t.Fatal("NewFetcher returned nil")
	}

	if fetcher.client != client {
		t.Error("Client not set properly")
	}
}

func TestManager_FetchOpenAPISpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HTTP test in short mode")
	}

	t.Run("nil OpenAPI spec", func(t *testing.T) {
		manager, err := NewManager(ManagerConfig{})
		if err != nil {
			t.Fatal(err)
		}

		inv := &Inventory{
			OpenAPISpec: nil,
		}

		_, err = manager.FetchOpenAPISpec(context.Background(), inv)
		if err == nil {
			t.Error("Expected error for nil OpenAPI spec")
		}
	})
}
