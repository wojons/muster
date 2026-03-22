package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Parser handles flag parsing with validation and binding.
// It manages a collection of flag definitions and provides methods
// for validating flag values and binding them to Cobra commands.
type Parser struct {
	definitions map[string]*FlagDefinition
	values      map[string]*Value
	result      *ParseResult
}

// NewParser creates a new flag parser with empty state.
func NewParser() *Parser {
	return &Parser{
		definitions: make(map[string]*FlagDefinition),
		values:      make(map[string]*Value),
		result: &ParseResult{
			Values: make(map[string]Value),
			Errors: make([]error, 0),
		},
	}
}

// AddDefinition registers a flag definition with the parser.
// The definition is validated before being stored.
// Returns an error if the definition is invalid or if a flag
// with the same name is already registered.
func (p *Parser) AddDefinition(def *FlagDefinition) error {
	if def == nil {
		return fmt.Errorf("flag definition cannot be nil")
	}

	if err := def.Validate(); err != nil {
		return fmt.Errorf("invalid flag definition: %w", err)
	}

	if _, exists := p.definitions[def.Name]; exists {
		return fmt.Errorf("flag '%s' already registered", def.Name)
	}

	p.definitions[def.Name] = def
	return nil
}

// RemoveDefinition removes a flag definition from the parser.
// Returns true if the definition was found and removed, false otherwise.
func (p *Parser) RemoveDefinition(name string) bool {
	if _, exists := p.definitions[name]; !exists {
		return false
	}

	delete(p.definitions, name)
	delete(p.values, name)
	delete(p.result.Values, name)
	return true
}

// GetDefinition retrieves a flag definition by name.
// Returns the definition and a boolean indicating if it was found.
func (p *Parser) GetDefinition(name string) (*FlagDefinition, bool) {
	def, exists := p.definitions[name]
	return def, exists
}

// Validate checks all registered flag definitions for validity.
// Returns an error if any definition is invalid.
func (p *Parser) Validate() error {
	for name, def := range p.definitions {
		if err := def.Validate(); err != nil {
			return fmt.Errorf("invalid flag definition '%s': %w", name, err)
		}
	}
	return nil
}

// Bind registers all flag definitions with a Cobra command.
// Each flag is added to the command with appropriate type and options.
// Returns an error if binding fails for any flag.
func (p *Parser) Bind(cmd *cobra.Command) error {
	if cmd == nil {
		return fmt.Errorf("command cannot be nil")
	}

	for name, def := range p.definitions {
		if err := p.bindFlag(cmd, def); err != nil {
			return fmt.Errorf("failed to bind flag '%s': %w", name, err)
		}
	}

	return nil
}

// bindFlag binds a single flag definition to a Cobra command.
func (p *Parser) bindFlag(cmd *cobra.Command, def *FlagDefinition) error {
	shorthand := def.Shorthand

	switch def.Type {
	case FlagTypeString:
		defaultValue := def.Default
		cmd.Flags().StringP(def.Name, shorthand, defaultValue, def.Description)

	case FlagTypeInt:
		defaultValue := 0
		if def.Default != "" {
			// Parse default value if provided
			var parsed int
			if _, err := fmt.Sscanf(def.Default, "%d", &parsed); err == nil {
				defaultValue = parsed
			}
		}
		cmd.Flags().IntP(def.Name, shorthand, defaultValue, def.Description)

	case FlagTypeBool:
		defaultValue := false
		if def.Default == "true" || def.Default == "1" || def.Default == "yes" {
			defaultValue = true
		}
		cmd.Flags().BoolP(def.Name, shorthand, defaultValue, def.Description)

	case FlagTypeURL:
		// URL flags are string flags with URL validation
		cmd.Flags().StringP(def.Name, shorthand, def.Default, def.Description)

	case FlagTypeIP:
		// IP flags are string flags with IP validation
		cmd.Flags().StringP(def.Name, shorthand, def.Default, def.Description)

	case FlagTypeEmail:
		// Email flags are string flags with email validation
		cmd.Flags().StringP(def.Name, shorthand, def.Default, def.Description)

	case FlagTypePath:
		// Path flags are string flags with path validation
		cmd.Flags().StringP(def.Name, shorthand, def.Default, def.Description)

	default:
		return fmt.Errorf("unsupported flag type: %s", def.Type)
	}

	// Mark as required if specified
	if def.Required {
		if err := cmd.MarkFlagRequired(def.Name); err != nil {
			return err
		}
	}

	// Set hidden flag
	if def.Hidden {
		if err := cmd.Flags().MarkHidden(def.Name); err != nil {
			return err
		}
	}

	// Set deprecated flag
	if def.Deprecated != "" {
		if err := cmd.Flags().MarkDeprecated(def.Name, def.Deprecated); err != nil {
			return err
		}
	}

	return nil
}

// Parse reads flag values from a Cobra command and validates them.
// It should be called after Bind and after the command has been executed.
// Returns a ParseResult containing all parsed values and any validation errors.
func (p *Parser) Parse(cmd *cobra.Command) (*ParseResult, error) {
	if cmd == nil {
		return nil, fmt.Errorf("command cannot be nil")
	}

	result := &ParseResult{
		Values: make(map[string]Value),
		Errors: make([]error, 0),
	}

	// Visit all flags (both set and unset)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		// Get the definition for this flag
		def, exists := p.definitions[flag.Name]
		if !exists {
			// Skip flags that aren't managed by this parser
			return
		}

		value := Value{
			Raw:   flag.Value.String(),
			Type:  def.Type,
			IsSet: flag.Changed,
		}

		// Check if flag is required but not set
		if def.Required && !flag.Changed {
			result.Errors = append(result.Errors,
				fmt.Errorf("required flag --%s not provided", flag.Name))
		}

		// Run custom validator if one is defined
		if def.Validator != nil && flag.Changed {
			if err := def.Validator.Validate(value.Raw); err != nil {
				result.Errors = append(result.Errors,
					fmt.Errorf("flag --%s: %w", flag.Name, err))
			}
		}

		// Check environment variable if flag is not set
		if !flag.Changed && def.EnvVar != "" {
			if envValue := cmd.Flags().Lookup(flag.Name).DefValue; envValue != value.Raw {
				// Environment variable was used as default
				value.IsSet = true
			}
		}

		result.Values[flag.Name] = value
	})

	p.result = result
	return result, nil
}

// GetResult returns the result from the most recent Parse call.
func (p *Parser) GetResult() *ParseResult {
	return p.result
}

// GetValue retrieves a parsed value by name.
// Returns the value and a boolean indicating if it was found.
func (p *Parser) GetValue(name string) (Value, bool) {
	if p.result == nil {
		return Value{}, false
	}
	value, exists := p.result.Values[name]
	return value, exists
}

// Clear resets the parser state, removing all definitions and values.
func (p *Parser) Clear() {
	p.definitions = make(map[string]*FlagDefinition)
	p.values = make(map[string]*Value)
	p.result = &ParseResult{
		Values: make(map[string]Value),
		Errors: make([]error, 0),
	}
}

// ListDefinitions returns a list of all registered flag names.
func (p *Parser) ListDefinitions() []string {
	names := make([]string, 0, len(p.definitions))
	for name := range p.definitions {
		names = append(names, name)
	}
	return names
}

// HasDefinition checks if a flag definition exists.
func (p *Parser) HasDefinition(name string) bool {
	_, exists := p.definitions[name]
	return exists
}

// BindAndParse is a convenience method that binds definitions to a command
// and then parses the values. Useful for simple use cases.
// Returns the parse result and any binding errors.
func (p *Parser) BindAndParse(cmd *cobra.Command) (*ParseResult, error) {
	if err := p.Bind(cmd); err != nil {
		return nil, fmt.Errorf("bind failed: %w", err)
	}

	return p.Parse(cmd)
}

// ValidateFlag validates a single flag value against its definition.
// Returns an error if validation fails.
func (p *Parser) ValidateFlag(name string, value string) error {
	def, exists := p.definitions[name]
	if !exists {
		return fmt.Errorf("flag '%s' not found", name)
	}

	// Check if flag type is valid
	if !def.Type.IsValid() {
		return fmt.Errorf("invalid flag type for '%s': %s", name, def.Type)
	}

	// Run custom validator if present
	if def.Validator != nil {
		if err := def.Validator.Validate(value); err != nil {
			return fmt.Errorf("validation failed for flag '%s': %w", name, err)
		}
	}

	return nil
}

// String returns a string representation of the parser state.
func (p *Parser) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Parser with %d definitions:\n", len(p.definitions))
	for name, def := range p.definitions {
		fmt.Fprintf(&sb, "  - %s (%s, required=%v)\n", name, def.Type, def.Required)
	}
	return sb.String()
}
