package flags

import "fmt"

// FlagType represents the type of a CLI flag.
// It defines the kind of value expected for the flag and determines
// how the flag value is parsed and validated.
type FlagType string

const (
	// FlagTypeString represents a string flag type.
	// Accepts any string value without validation unless a custom validator is provided.
	FlagTypeString FlagType = "string"

	// FlagTypeInt represents an integer flag type.
	// Accepts signed integer values (e.g., -123, 456).
	FlagTypeInt FlagType = "int"

	// FlagTypeBool represents a boolean flag type.
	// Accepts true/false values or can be used as a toggle flag.
	FlagTypeBool FlagType = "bool"

	// FlagTypeURL represents a URL flag type.
	// Accepts valid URL strings and can be validated with URLValidator.
	FlagTypeURL FlagType = "url"

	// FlagTypeIP represents an IP address flag type.
	// Accepts IPv4 or IPv6 addresses and can be validated with IPValidator.
	FlagTypeIP FlagType = "ip"

	// FlagTypeEmail represents an email address flag type.
	// Accepts email addresses and can be validated with EmailValidator.
	FlagTypeEmail FlagType = "email"

	// FlagTypePath represents a file system path flag type.
	// Accepts file/directory paths and can be validated with PathValidator.
	FlagTypePath FlagType = "path"
)

// IsValid checks if the FlagType is a valid known type.
func (ft FlagType) IsValid() bool {
	switch ft {
	case FlagTypeString, FlagTypeInt, FlagTypeBool, FlagTypeURL,
		FlagTypeIP, FlagTypeEmail, FlagTypePath:
		return true
	}
	return false
}

// String returns the string representation of the FlagType.
func (ft FlagType) String() string {
	return string(ft)
}

// FlagDefinition defines the structure and behavior of a CLI flag.
// It encapsulates all metadata needed to register and validate a flag.
type FlagDefinition struct {
	// Name is the flag name (e.g., "host", "port").
	// This is the long form used with double dashes (e.g., --host).
	Name string

	// Shorthand is the single-character abbreviation (e.g., "h" for --host).
	// Optional. Use empty string if no shorthand is desired.
	Shorthand string

	// Type defines the flag's data type and parsing behavior.
	Type FlagType

	// Description provides help text shown in CLI usage information.
	Description string

	// Default is the default value for the flag as a string.
	// For boolean flags, use "true" or "false".
	// For other types, use the string representation.
	Default string

	// Required indicates whether the flag must be provided.
	// If true, validation will fail if the flag is not set.
	Required bool

	// Validator provides custom validation logic for the flag value.
	// Optional. If nil, only type validation is performed.
	Validator Validator

	// Hidden indicates whether the flag should be hidden from help text.
	// Useful for deprecated or internal flags.
	Hidden bool

	// Deprecated marks the flag as deprecated and shows a warning when used.
	// If non-empty, contains the deprecation message.
	Deprecated string

	// EnvVar is the environment variable name to use as a fallback.
	// Optional. If set and the flag is not provided, the env var value is used.
	EnvVar string
}

// Validate checks if the FlagDefinition is valid and can be used.
func (fd FlagDefinition) Validate() error {
	if fd.Name == "" {
		return fmt.Errorf("flag name cannot be empty")
	}

	if !fd.Type.IsValid() {
		return fmt.Errorf("invalid flag type: %s", fd.Type)
	}

	if len(fd.Shorthand) > 1 {
		return fmt.Errorf("shorthand must be a single character, got: %s", fd.Shorthand)
	}

	return nil
}

// Validator defines the interface for flag value validation.
// Validators can be composed to create complex validation rules.
type Validator interface {
	// Validate checks if the provided value is valid.
	// Returns an error if validation fails with a descriptive message.
	Validate(value string) error

	// Name returns the validator's name for error messages and logging.
	Name() string
}

// ValidatorFunc is an adapter to allow ordinary functions to be used as Validators.
type ValidatorFunc func(value string) error

// Validate implements the Validator interface for ValidatorFunc.
func (f ValidatorFunc) Validate(value string) error {
	return f(value)
}

// Name returns a generic name for function-based validators.
func (f ValidatorFunc) Name() string {
	return "custom"
}

// Value represents a parsed flag value.
// It provides type-safe access to the underlying value.
type Value struct {
	// Raw is the original string value from the command line.
	Raw string

	// Type is the flag type that was used to parse this value.
	Type FlagType

	// IsSet indicates whether the flag was explicitly set by the user.
	IsSet bool
}

// String returns the raw string value.
func (v Value) String() string {
	return v.Raw
}

// Int parses and returns the value as an integer.
// Returns an error if the value cannot be parsed as an integer.
func (v Value) Int() (int, error) {
	if v.Type != FlagTypeInt {
		return 0, fmt.Errorf("cannot convert %s to int", v.Type)
	}

	var result int
	_, err := fmt.Sscanf(v.Raw, "%d", &result)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value: %s", v.Raw)
	}

	return result, nil
}

// Bool parses and returns the value as a boolean.
// Returns false if the value cannot be parsed.
func (v Value) Bool() (bool, error) {
	if v.Type != FlagTypeBool {
		return false, fmt.Errorf("cannot convert %s to bool", v.Type)
	}

	switch v.Raw {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", v.Raw)
	}
}

// ParseResult holds the result of parsing flags for a command.
type ParseResult struct {
	// Values maps flag names to their parsed values.
	Values map[string]Value

	// Errors contains validation errors encountered during parsing.
	Errors []error
}

// Get retrieves a parsed value by name.
// Returns the Value and a boolean indicating if the flag was found.
func (pr ParseResult) Get(name string) (Value, bool) {
	value, exists := pr.Values[name]
	return value, exists
}

// HasErrors returns true if any validation errors occurred.
func (pr ParseResult) HasErrors() bool {
	return len(pr.Errors) > 0
}

// Error aggregates all validation errors into a single error.
// Returns nil if there are no errors.
func (pr ParseResult) Error() error {
	if !pr.HasErrors() {
		return nil
	}

	if len(pr.Errors) == 1 {
		return pr.Errors[0]
	}

	errMsg := "validation errors:\n"
	for i, err := range pr.Errors {
		errMsg += fmt.Sprintf("  %d. %s\n", i+1, err.Error())
	}
	return fmt.Errorf("%s", errMsg)
}
