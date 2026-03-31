package inventory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidSHA256(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid lowercase SHA256",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			expected: true,
		},
		{
			name:     "valid uppercase SHA256",
			input:    "A591A6D40BF420404A011733CFB7B190D62C65BF0BCDA32B57B277D9AD9F146E",
			expected: true,
		},
		{
			name:     "valid mixed case SHA256",
			input:    "a591A6d40Bf420404A011733Cfb7B190d62c65Bf0bcda32b57b277d9ad9f146e",
			expected: true,
		},
		{
			name:     "too short",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146",
			expected: false,
		},
		{
			name:     "too long",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146ee",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "contains invalid hex character g",
			input:    "g591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			expected: false,
		},
		{
			name:     "contains invalid hex character z",
			input:    "z591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			expected: false,
		},
		{
			name:     "contains hyphen",
			input:    "a591a6d4-0bf4-2040-4a01-1733cfb7b190",
			expected: false,
		},
		{
			name:     "path traversal attempt with dots",
			input:    "../../../etc/passwd",
			expected: false,
		},
		{
			name:     "path traversal with valid hex but dots",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f14../../etc/passwd",
			expected: false,
		},
		{
			name:     "null byte injection",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e\x00",
			expected: false,
		},
		{
			name:     "newline injection",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e\n",
			expected: false,
		},
		{
			name:     "carriage return injection",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e\r",
			expected: false,
		},
		{
			name:     "space in hash",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146 ",
			expected: false,
		},
		{
			name:     "special characters",
			input:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146!",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSHA256(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInventoryCache_StoreSpec_PathTraversalPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewInventoryCache(tmpDir)
	require.NoError(t, err)

	doc := &OpenAPIDocument{
		OpenAPI: "3.0.0",
		Info: struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		}{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}

	tests := []struct {
		name        string
		sha256      string
		expectError bool
	}{
		{
			name:        "valid SHA256 should succeed",
			sha256:      "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			expectError: false,
		},
		{
			name:        "path traversal with dots",
			sha256:      "../../../etc/passwd",
			expectError: true,
		},
		{
			name:        "path traversal with encoded dots",
			sha256:      "..%2F..%2F..%2Fetc%2Fpasswd",
			expectError: true,
		},
		{
			name:        "absolute path attempt",
			sha256:      "/etc/passwd",
			expectError: true,
		},
		{
			name:        "backslash path traversal",
			sha256:      "..\\..\\..\\etc\\passwd",
			expectError: true,
		},
		{
			name:        "null byte injection",
			sha256:      "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e\x00evil.txt",
			expectError: true,
		},
		{
			name:        "newline injection",
			sha256:      "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e\nevil.txt",
			expectError: true,
		},
		{
			name:        "too short hash",
			sha256:      "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146",
			expectError: true,
		},
		{
			name:        "too long hash",
			sha256:      "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146ee",
			expectError: true,
		},
		{
			name:        "invalid hex characters",
			sha256:      "g591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cache.StoreSpec(tt.sha256, doc)
			if tt.expectError {
				assert.Error(t, err, "Expected error for SHA256: %s", tt.sha256)
				if err != nil {
					assert.ErrorIs(t, err, ErrInvalidSHA256, "Expected ErrInvalidSHA256 for invalid SHA256")
				}
			} else {
				assert.NoError(t, err, "Expected success for valid SHA256")

				// Verify file was created
				specPath := filepath.Join(tmpDir, "specs", tt.sha256+".json")
				assert.FileExists(t, specPath, "Spec file should be created for valid SHA256")
			}
		})
	}
}

func TestInventoryCache_GetSpec_PathTraversalPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewInventoryCache(tmpDir)
	require.NoError(t, err)

	// First store a valid spec
	validSHA := "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e"
	doc := &OpenAPIDocument{
		OpenAPI: "3.0.0",
		Info: struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		}{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}
	err = cache.StoreSpec(validSHA, doc)
	require.NoError(t, err)

	tests := []struct {
		name        string
		sha256      string
		expectNil   bool
		shouldExist bool
	}{
		{
			name:        "valid SHA256 should return spec",
			sha256:      validSHA,
			expectNil:   false,
			shouldExist: true,
		},
		{
			name:        "path traversal with dots",
			sha256:      "../../../etc/passwd",
			expectNil:   true,
			shouldExist: false,
		},
		{
			name:        "path traversal attempting to read stored file",
			sha256:      "../../../.cache/muster/specs/" + validSHA,
			expectNil:   true,
			shouldExist: false,
		},
		{
			name:        "absolute path attempt",
			sha256:      "/etc/passwd",
			expectNil:   true,
			shouldExist: false,
		},
		{
			name:        "backslash path traversal",
			sha256:      "..\\..\\..\\etc\\passwd",
			expectNil:   true,
			shouldExist: false,
		},
		{
			name:        "null byte injection",
			sha256:      validSHA + "\x00evil.txt",
			expectNil:   true,
			shouldExist: false,
		},
		{
			name:        "non-existent valid SHA256",
			sha256:      "0000000000000000000000000000000000000000000000000000000000000000",
			expectNil:   true,
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cache.GetSpec(tt.sha256)
			if tt.expectNil {
				assert.Nil(t, result, "Expected nil for invalid/non-existent SHA256: %s", tt.sha256)
			} else {
				assert.NotNil(t, result, "Expected spec for valid SHA256")
				assert.Equal(t, "3.0.0", result.OpenAPI)
			}
		})
	}
}

func TestInventoryCache_StoreSpec_ValidSHA256Variations(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewInventoryCache(tmpDir)
	require.NoError(t, err)

	doc := &OpenAPIDocument{
		OpenAPI: "3.0.0",
		Info: struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		}{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}

	// Test with different valid SHA256 values
	validHashes := []string{
		"a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e", // lowercase
		"A591A6D40BF420404A011733CFB7B190D62C65BF0BCDA32B57B277D9AD9F146E", // uppercase
		"a591A6D40Bf420404A011733Cfb7B190d62c65Bf0bcda32b57b277d9ad9f146e", // mixed case
		"0000000000000000000000000000000000000000000000000000000000000000", // all zeros
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // all f's
	}

	for _, hash := range validHashes {
		t.Run("valid_hash_"+hash[:8], func(t *testing.T) {
			err := cache.StoreSpec(hash, doc)
			assert.NoError(t, err)

			// Verify file was created
			specPath := filepath.Join(tmpDir, "specs", hash+".json")
			assert.FileExists(t, specPath)

			// Verify we can retrieve it
			retrieved := cache.GetSpec(hash)
			assert.NotNil(t, retrieved)
			assert.Equal(t, "3.0.0", retrieved.OpenAPI)
		})
	}
}

func TestInventoryCache_ErrorInvalidSHA256(t *testing.T) {
	// Verify that ErrInvalidSHA256 is properly defined
	assert.Error(t, ErrInvalidSHA256)
	assert.Contains(t, ErrInvalidSHA256.Error(), "invalid SHA256")
}
