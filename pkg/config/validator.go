package config

import (
	"fmt"
	"net/url"
	"reflect"

	"github.com/go-playground/validator/v10"
)

// ValidatedConfig represents a configuration with validation tags
// for use with go-playground/validator
type ValidatedConfig struct {
	// Connection settings
	BaseURL string `validate:"required,url"`
	Timeout int    `validate:"required,min=1,max=300"`

	// Retry settings
	RetryCount int `validate:"min=0,max=10"`

	// TLS settings
	CertFile string `validate:"omitempty,file"`
	KeyFile  string `validate:"omitempty,file"`
	Insecure bool

	// Cache settings
	CacheDir string `validate:"required,dir"`
	CacheTTL int    `validate:"min=0"`

	// Output settings
	OutputFormat string `validate:"omitempty,oneof=json yaml table"`
	Verbose      bool
}

// Validator provides configuration validation functionality
type Validator struct {
	validate *validator.Validate
}

// NewValidator creates a new configuration validator
func NewValidator() *Validator {
	v := validator.New()

	// Register custom validation: if cert_file is set, key_file must also be set
	_ = v.RegisterValidation("cert_key_pair", func(fl validator.FieldLevel) bool {
		// Get the top-level struct
		parent := fl.Top()
		if parent.Kind() != reflect.Struct {
			return true
		}

		// Find CertFile and KeyFile fields
		var certFile, keyFile string

		certField := parent.FieldByName("CertFile")
		if certField.IsValid() && certField.Kind() == reflect.String {
			certFile = certField.String()
		}

		keyField := parent.FieldByName("KeyFile")
		if keyField.IsValid() && keyField.Kind() == reflect.String {
			keyFile = keyField.String()
		}

		// If one is set, both must be set
		if certFile != "" && keyFile == "" {
			return false
		}
		if keyFile != "" && certFile == "" {
			return false
		}
		return true
	})

	// Register custom URL validation that allows empty strings when marked as omitempty
	_ = v.RegisterValidation("url_or_empty", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		if value == "" {
			return true
		}
		_, err := url.ParseRequestURI(value)
		return err == nil
	})

	return &Validator{
		validate: v,
	}
}

// Validate validates a ValidatedConfig struct
func (v *Validator) Validate(cfg *ValidatedConfig) error {
	if cfg == nil {
		return &ConfigurationError{
			Field:   "config",
			Message: "configuration cannot be nil",
			Source:  "validation",
		}
	}

	err := v.validate.Struct(cfg)
	if err == nil {
		return nil
	}

	// Convert validator errors to ConfigurationError
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		if len(validationErrors) > 0 {
			ve := validationErrors[0]
			return &ConfigurationError{
				Field:   ve.Field(),
				Message: fmt.Sprintf("failed validation: %s=%s", ve.Tag(), ve.Param()),
				Source:  "validation",
			}
		}
	}

	return &ConfigurationError{
		Field:   "config",
		Message: err.Error(),
		Source:  "validation",
	}
}

// ValidateConfig validates a Config struct and returns validation errors
func (v *Validator) ValidateConfig(cfg *Config) []error {
	var errors []error

	if cfg == nil {
		return []error{&ConfigurationError{
			Field:   "config",
			Message: "configuration cannot be nil",
			Source:  "validation",
		}}
	}

	// Validate profiles
	for name, profile := range cfg.Profiles {
		if profile.BaseURL != "" {
			if _, err := url.ParseRequestURI(profile.BaseURL); err != nil {
				errors = append(errors, &ConfigurationError{
					Field:   fmt.Sprintf("profiles.%s.base_url", name),
					Message: fmt.Sprintf("invalid URL: %v", err),
					Source:  "config",
				})
			}
		}

		if profile.Timeout < 0 {
			errors = append(errors, &ConfigurationError{
				Field:   fmt.Sprintf("profiles.%s.timeout", name),
				Message: "timeout cannot be negative",
				Source:  "config",
			})
		}
	}

	// Validate inventory URL if set
	if cfg.Inventory.URL != "" {
		if _, err := url.ParseRequestURI(cfg.Inventory.URL); err != nil {
			errors = append(errors, &ConfigurationError{
				Field:   "inventory.url",
				Message: fmt.Sprintf("invalid URL: %v", err),
				Source:  "config",
			})
		}
	}

	return errors
}

// ConfigurationError represents a configuration-related error
type ConfigurationError struct {
	Field   string
	Message string
	Source  string // "flag", "env", "file", "default", "validation"
}

// Error implements the error interface
func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("config error in %s (%s): %s", e.Field, e.Source, e.Message)
}

// Common configuration errors
var (
	ErrConfigNotFound = fmt.Errorf("configuration file not found")
	ErrInvalidHost    = fmt.Errorf("invalid host URL")
	ErrMissingCert    = fmt.Errorf("certificate file specified without key file")
	ErrMissingKey     = fmt.Errorf("key file specified without certificate file")
)

// ValidateConfigStruct is a convenience function that validates a ValidatedConfig
// using a new Validator instance
func ValidateConfigStruct(cfg *ValidatedConfig) error {
	validator := NewValidator()
	return validator.Validate(cfg)
}

// ValidateAndReturnErrors validates a Config and returns all validation errors
func ValidateAndReturnErrors(cfg *Config) []error {
	validator := NewValidator()
	return validator.ValidateConfig(cfg)
}

// IsConfigurationError checks if an error is a ConfigurationError
func IsConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ConfigurationError)
	return ok
}

// GetConfigurationError extracts a ConfigurationError from an error if it is one
func GetConfigurationError(err error) (*ConfigurationError, bool) {
	if err == nil {
		return nil, false
	}
	if configErr, ok := err.(*ConfigurationError); ok {
		return configErr, true
	}
	return nil, false
}
