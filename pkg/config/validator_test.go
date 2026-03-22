package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	require.NotNil(t, validator)
	require.NotNil(t, validator.validate)
}

func TestValidator_Validate(t *testing.T) {
	// Create temporary directory for file validation tests
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.pem")
	testDir := filepath.Join(tempDir, "testdir")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))
	require.NoError(t, os.Mkdir(testDir, 0755))

	validator := NewValidator()

	tests := []struct {
		name      string
		config    *ValidatedConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: &ValidatedConfig{
				BaseURL:      "https://api.example.com",
				Timeout:      30,
				RetryCount:   3,
				CacheDir:     testDir,
				CacheTTL:     3600,
				OutputFormat: "json",
			},
			wantError: false,
		},
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
			errorMsg:  "configuration cannot be nil",
		},
		{
			name: "missing base URL",
			config: &ValidatedConfig{
				BaseURL:  "",
				Timeout:  30,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "required",
		},
		{
			name: "invalid base URL",
			config: &ValidatedConfig{
				BaseURL:  "not-a-valid-url",
				Timeout:  30,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "url",
		},
		{
			name: "timeout too low",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  0,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "required",
		},
		{
			name: "timeout too high",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  301,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "max=300",
		},
		{
			name: "retry count too high",
			config: &ValidatedConfig{
				BaseURL:    "https://api.example.com",
				Timeout:    30,
				RetryCount: 11,
				CacheDir:   testDir,
				CacheTTL:   3600,
			},
			wantError: true,
			errorMsg:  "max=10",
		},
		{
			name: "negative retry count",
			config: &ValidatedConfig{
				BaseURL:    "https://api.example.com",
				Timeout:    30,
				RetryCount: -1,
				CacheDir:   testDir,
				CacheTTL:   3600,
			},
			wantError: true,
			errorMsg:  "min=0",
		},
		{
			name: "invalid cert file",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CertFile: "/nonexistent/path/cert.pem",
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "file",
		},
		{
			name: "invalid key file",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				KeyFile:  "/nonexistent/path/key.pem",
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "file",
		},
		{
			name: "missing cache dir",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CacheDir: "",
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "required",
		},
		{
			name: "non-existent cache dir",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CacheDir: "/nonexistent/directory",
				CacheTTL: 3600,
			},
			wantError: true,
			errorMsg:  "dir",
		},
		{
			name: "negative cache TTL",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CacheDir: testDir,
				CacheTTL: -1,
			},
			wantError: true,
			errorMsg:  "min=0",
		},
		{
			name: "invalid output format",
			config: &ValidatedConfig{
				BaseURL:      "https://api.example.com",
				Timeout:      30,
				CacheDir:     testDir,
				CacheTTL:     3600,
				OutputFormat: "xml",
			},
			wantError: true,
			errorMsg:  "oneof",
		},
		{
			name: "valid with cert and key files",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CertFile: testFile,
				KeyFile:  testFile,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.config)
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateConfig(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		config     *Config
		wantErrors int
	}{
		{
			name:       "nil config",
			config:     nil,
			wantErrors: 1,
		},
		{
			name:       "empty config",
			config:     &Config{},
			wantErrors: 0,
		},
		{
			name: "valid profile with valid URL",
			config: &Config{
				Profiles: map[string]Profile{
					"test": {
						BaseURL: "https://api.example.com",
						Timeout: 30,
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "invalid profile URL",
			config: &Config{
				Profiles: map[string]Profile{
					"test": {
						BaseURL: "not-a-url",
						Timeout: 30,
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "negative profile timeout",
			config: &Config{
				Profiles: map[string]Profile{
					"test": {
						BaseURL: "https://api.example.com",
						Timeout: -1,
					},
				},
			},
			wantErrors: 1,
		},
		{
			name: "multiple profiles with errors",
			config: &Config{
				Profiles: map[string]Profile{
					"valid": {
						BaseURL: "https://api.example.com",
						Timeout: 30,
					},
					"invalid1": {
						BaseURL: "bad-url",
						Timeout: 30,
					},
					"invalid2": {
						BaseURL: "https://api.test.com",
						Timeout: -5,
					},
				},
			},
			wantErrors: 2,
		},
		{
			name: "invalid inventory URL",
			config: &Config{
				Inventory: InventoryConfig{
					URL: "not-a-valid-url",
				},
			},
			wantErrors: 1,
		},
		{
			name: "valid config with all fields",
			config: &Config{
				Profiles: map[string]Profile{
					"production": {
						BaseURL: "https://api.production.com",
						Timeout: 60,
					},
				},
				Inventory: InventoryConfig{
					URL:          "https://inventory.example.com",
					CacheEnabled: true,
					CacheTTL:     3600,
				},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateConfig(tt.config)
			assert.Len(t, errs, tt.wantErrors)

			// Verify all errors are ConfigurationError type
			for _, err := range errs {
				configErr, ok := err.(*ConfigurationError)
				assert.True(t, ok, "error should be ConfigurationError type")
				assert.NotEmpty(t, configErr.Field)
				assert.NotEmpty(t, configErr.Message)
				assert.NotEmpty(t, configErr.Source)
			}
		})
	}
}

func TestConfigurationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ConfigurationError
		expected string
	}{
		{
			name: "basic error",
			err: &ConfigurationError{
				Field:   "host",
				Message: "invalid URL",
				Source:  "config",
			},
			expected: "config error in host (config): invalid URL",
		},
		{
			name: "validation error",
			err: &ConfigurationError{
				Field:   "timeout",
				Message: "failed validation: min=1",
				Source:  "validation",
			},
			expected: "config error in timeout (validation): failed validation: min=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestCommonErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "ErrConfigNotFound",
			err:  ErrConfigNotFound,
		},
		{
			name: "ErrInvalidHost",
			err:  ErrInvalidHost,
		},
		{
			name: "ErrMissingCert",
			err:  ErrMissingCert,
		},
		{
			name: "ErrMissingKey",
			err:  ErrMissingKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.err)
			assert.NotEmpty(t, tt.err.Error())
		})
	}
}

func TestValidateConfigStruct(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "cache")
	require.NoError(t, os.Mkdir(testDir, 0755))

	tests := []struct {
		name      string
		config    *ValidatedConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: false,
		},
		{
			name: "invalid config",
			config: &ValidatedConfig{
				BaseURL: "",
				Timeout: 30,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigStruct(tt.config)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAndReturnErrors(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		wantErrors int
	}{
		{
			name:       "nil config",
			config:     nil,
			wantErrors: 1,
		},
		{
			name:       "valid config",
			config:     &Config{},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateAndReturnErrors(tt.config)
			assert.Len(t, errs, tt.wantErrors)
		})
	}
}

func TestIsConfigurationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ConfigurationError",
			err:      &ConfigurationError{Field: "test"},
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsConfigurationError(tt.err))
		})
	}
}

func TestGetConfigurationError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantErr      bool
		expectedType bool
	}{
		{
			name:         "nil error",
			err:          nil,
			wantErr:      true,
			expectedType: false,
		},
		{
			name:         "ConfigurationError",
			err:          &ConfigurationError{Field: "test", Message: "test error"},
			wantErr:      false,
			expectedType: true,
		},
		{
			name:         "regular error",
			err:          errors.New("regular error"),
			wantErr:      true,
			expectedType: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configErr, ok := GetConfigurationError(tt.err)
			assert.Equal(t, tt.expectedType, ok)

			if !tt.wantErr {
				require.NotNil(t, configErr)
				assert.Equal(t, "test", configErr.Field)
				assert.Equal(t, "test error", configErr.Message)
			}
		})
	}
}

func TestValidatedConfig_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "cache")
	require.NoError(t, os.Mkdir(testDir, 0755))

	validator := NewValidator()

	tests := []struct {
		name      string
		config    *ValidatedConfig
		wantError bool
	}{
		{
			name: "boundary timeout - min",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  1,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: false,
		},
		{
			name: "boundary timeout - max",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  300,
				CacheDir: testDir,
				CacheTTL: 3600,
			},
			wantError: false,
		},
		{
			name: "boundary retry - min",
			config: &ValidatedConfig{
				BaseURL:    "https://api.example.com",
				Timeout:    30,
				RetryCount: 0,
				CacheDir:   testDir,
				CacheTTL:   3600,
			},
			wantError: false,
		},
		{
			name: "boundary retry - max",
			config: &ValidatedConfig{
				BaseURL:    "https://api.example.com",
				Timeout:    30,
				RetryCount: 10,
				CacheDir:   testDir,
				CacheTTL:   3600,
			},
			wantError: false,
		},
		{
			name: "boundary cache TTL - min",
			config: &ValidatedConfig{
				BaseURL:  "https://api.example.com",
				Timeout:  30,
				CacheDir: testDir,
				CacheTTL: 0,
			},
			wantError: false,
		},
		{
			name: "valid output formats",
			config: &ValidatedConfig{
				BaseURL:      "https://api.example.com",
				Timeout:      30,
				CacheDir:     testDir,
				CacheTTL:     3600,
				OutputFormat: "yaml",
			},
			wantError: false,
		},
		{
			name: "valid output format - table",
			config: &ValidatedConfig{
				BaseURL:      "https://api.example.com",
				Timeout:      30,
				CacheDir:     testDir,
				CacheTTL:     3600,
				OutputFormat: "table",
			},
			wantError: false,
		},
		{
			name: "empty output format allowed",
			config: &ValidatedConfig{
				BaseURL:      "https://api.example.com",
				Timeout:      30,
				CacheDir:     testDir,
				CacheTTL:     3600,
				OutputFormat: "",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.config)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidator_Concurrent(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "cache")
	require.NoError(t, os.Mkdir(testDir, 0755))

	validator := NewValidator()
	config := &ValidatedConfig{
		BaseURL:  "https://api.example.com",
		Timeout:  30,
		CacheDir: testDir,
		CacheTTL: 3600,
	}

	// Run multiple validations concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := validator.Validate(config)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
