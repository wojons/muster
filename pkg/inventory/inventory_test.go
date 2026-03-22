package inventory

import (
	"testing"
	"time"
)

func TestNewInventoryManager(t *testing.T) {
	manager, err := NewManager(ManagerConfig{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestValidator_Validate(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion:  "1.0",
		GeneratedAt:    time.Now(),
		APIVersion:     "1.0.0",
		APIName:        "Test API",
		APIDescription: "Test API Description",
		BaseURL:        "https://api.example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:         "https://api.example.com/openapi.json",
			Format:      "json",
			SHA256:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:        1024,
			LastUpdated: time.Now(),
		},
		Resources: []Resource{
			{
				Name: "users",
				Operations: []Operation{
					{Name: "list", Method: "GET", Path: "/users", Summary: "List users"},
				},
			},
		},
	}

	err := validator.Validate(inv)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidator_InvalidSchemaVersion(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion:  "2.0",
		GeneratedAt:    time.Now(),
		APIVersion:     "1.0.0",
		APIName:        "Test API",
		APIDescription: "Test API",
		BaseURL:        "https://api.example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://api.example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:   1024,
		},
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Validate() expected error for invalid schema version")
	}
}

func TestValidator_DuplicateResourceName(t *testing.T) {
	validator := NewValidator()

	inv := &Inventory{
		SchemaVersion:  "1.0",
		GeneratedAt:    time.Now(),
		APIVersion:     "1.0.0",
		APIName:        "Test API",
		APIDescription: "Test API",
		BaseURL:        "https://api.example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://api.example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:   1024,
		},
		Resources: []Resource{
			{Name: "users"},
			{Name: "users"},
		},
	}

	err := validator.Validate(inv)
	if err == nil {
		t.Error("Validate() expected error for duplicate resource name")
	}
}

func TestVersionManager_Track(t *testing.T) {
	vm := NewVersionManager()

	inv := &Inventory{
		APIVersion: "1.0.0",
	}

	vm.Track("test-host", inv)

	history, err := vm.GetHistory("test-host")
	if err != nil {
		t.Fatalf("GetHistory() error = %v", err)
	}

	if len(history) != 1 {
		t.Errorf("GetHistory() returned %d records, want 1", len(history))
	}

	if history[0].Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", history[0].Version)
	}
}

func TestVersionManager_GetHistory_NotFound(t *testing.T) {
	vm := NewVersionManager()

	_, err := vm.GetHistory("non-existent-host")
	if err == nil {
		t.Error("GetHistory() expected error for non-existent host")
	}
}

func TestVersionManager_IsVersionCompatible(t *testing.T) {
	vm := NewVersionManager()

	inv := &Inventory{
		APIVersion: "1.0.0",
	}

	vm.Track("test-host", inv)

	compatible := vm.IsVersionCompatible("test-host", "1.0.0")
	if !compatible {
		t.Error("IsVersionCompatible() = false, want true")
	}

	compatible = vm.IsVersionCompatible("test-host", "2.0.0")
	if compatible {
		t.Error("IsVersionCompatible() = true for non-existent version, want false")
	}
}

func TestNewFetcher(t *testing.T) {
	fetcher := NewFetcher(nil)

	if fetcher == nil {
		t.Fatal("NewFetcher() returned nil")
	}

	if fetcher.client == nil {
		t.Fatal("NewFetcher() client is nil")
	}
}

func TestNewInventoryCache(t *testing.T) {
	cache, err := NewInventoryCache("")
	if err != nil {
		t.Fatalf("NewInventoryCache() error = %v", err)
	}

	if cache == nil {
		t.Fatal("NewInventoryCache() returned nil")
	}

	if cache.baseDir == "" {
		t.Error("NewInventoryCache() baseDir is empty")
	}
}

func TestInventoryCache_StoreAndRetrieve(t *testing.T) {
	cache, err := NewInventoryCache("")
	if err != nil {
		t.Fatalf("NewInventoryCache() error = %v", err)
	}

	inv := &Inventory{
		SchemaVersion:  "1.0",
		GeneratedAt:    time.Now(),
		APIVersion:     "1.0.0",
		APIName:        "Test API",
		APIDescription: "Test API",
		BaseURL:        "https://api.example.com",
		OpenAPISpec: &OpenAPIReference{
			URL:    "https://api.example.com/openapi.json",
			Format: "json",
			SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:   1024,
		},
		Resources: []Resource{},
	}

	err = cache.Store("test-host", inv, "test-etag")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	retrieved, err := cache.GetLatest("test-host")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}

	if retrieved.APIVersion != inv.APIVersion {
		t.Errorf("APIVersion = %s, want %s", retrieved.APIVersion, inv.APIVersion)
	}

	_ = cache.Purge("test-host")
}
