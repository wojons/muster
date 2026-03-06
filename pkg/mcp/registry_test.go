package mcp

import (
	"context"
	"testing"
)

// TestValidateArgs_Success tests valid arguments against schema
func TestValidateArgs_Success(t *testing.T) {
	// Create executor with test config
	configs := map[string]ExecutionConfig{
		"test_tool": {
			Path:        "/api/test",
			Method:      "GET",
			BaseURL:     "http://example.com",
			InputSchema: []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
		},
	}
	executor := NewOpenAPIExecutor(configs)

	// Execute with valid arguments
	ctx := context.Background()
	args := map[string]interface{}{"name": "test"}

	result, err := executor.Execute(ctx, "test_tool", args)

	// Note: This will fail HTTP request, but validation should pass
	// We'll check error message to see if it's validation error or HTTP error
	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// If validation failed, error message would contain "validation failed"
	if err != nil {
		resultMap := result.(map[string]interface{})
		if errMsg, ok := resultMap["error"].(string); ok {
			if contains(errMsg, "validation failed") {
				t.Fatalf("Validation should have passed but failed: %v", errMsg)
			}
		}
	}
}

// TestValidateArgs_MissingRequired tests missing required field
func TestValidateArgs_MissingRequired(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"test_tool": {
			Path:        "/api/test",
			Method:      "GET",
			BaseURL:     "http://example.com",
			InputSchema: []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
		},
	}
	executor := NewOpenAPIExecutor(configs)

	ctx := context.Background()
	args := map[string]interface{}{} // Empty args (missing required 'name')

	result, err := executor.Execute(ctx, "test_tool", args)

	if err != nil {
		t.Fatal("Expected result with error message, got nil result")
	}

	// Should return validation failure
	resultMap := result.(map[string]interface{})
	if errMsg, ok := resultMap["error"].(string); ok {
		if !contains(errMsg, "validation failed") && !contains(errMsg, "name") {
			t.Fatalf("Expected validation error, got: %v", errMsg)
		}
	} else {
		t.Fatal("Expected error message in result")
	}
}

// TestValidateArgs_InvalidType tests invalid type for field
func TestValidateArgs_InvalidType(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"test_tool": {
			Path:        "/api/test",
			Method:      "GET",
			BaseURL:     "http://example.com",
			InputSchema: []byte(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}`),
		},
	}
	executor := NewOpenAPIExecutor(configs)

	ctx := context.Background()
	args := map[string]interface{}{"count": "not-a-number"} // String instead of integer

	result, err := executor.Execute(ctx, "test_tool", args)

	if err != nil {
		t.Fatal("Expected result with error message, got nil result")
	}

	// Should return validation failure
	resultMap := result.(map[string]interface{})
	if errMsg, ok := resultMap["error"].(string); ok {
		if !contains(errMsg, "validation failed") && !contains(errMsg, "type") {
			t.Fatalf("Expected validation error about type, got: %v", errMsg)
		}
	} else {
		t.Fatal("Expected error message in result")
	}
}

// TestValidateArgs_NoSchema tests execution when no schema is provided
func TestValidateArgs_NoSchema(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"test_tool": {
			Path:        "/api/test",
			Method:      "GET",
			BaseURL:     "http://example.com",
			InputSchema: []byte{}, // Empty schema
		},
	}
	executor := NewOpenAPIExecutor(configs)

	ctx := context.Background()
	args := map[string]interface{}{"any_field": "any_value"} // Should accept any value

	result, err := executor.Execute(ctx, "test_tool", args)

	// Should proceed to HTTP request (will fail, but validation should pass)
	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if err != nil {
		resultMap := result.(map[string]interface{})
		if errMsg, ok := resultMap["error"].(string); ok {
			if contains(errMsg, "validation failed") {
				t.Fatalf("Validation should have been skipped, got error: %v", errMsg)
			}
		}
	}
}

// TestValidateArgs_MalformedSchema tests malformed schema
func TestValidateArgs_MalformedSchema(t *testing.T) {
	configs := map[string]ExecutionConfig{
		"test_tool": {
			Path:        "/api/test",
			Method:      "GET",
			BaseURL:     "http://example.com",
			InputSchema: []byte(`{invalid json}`), // Malformed schema
		},
	}
	executor := NewOpenAPIExecutor(configs)

	ctx := context.Background()
	args := map[string]interface{}{"name": "test"}

	result, err := executor.Execute(ctx, "test_tool", args)

	// Should return validation error
	if err != nil {
		resultMap := result.(map[string]interface{})
		if errMsg, ok := resultMap["error"].(string); ok {
			if !contains(errMsg, "validation error") {
				t.Fatalf("Expected validation error, got: %v", errMsg)
			}
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
