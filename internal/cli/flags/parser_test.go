package flags

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParser_NewParser tests creating a new parser instance.
func TestParser_NewParser(t *testing.T) {
	parser := NewParser()
	require.NotNil(t, parser)
	assert.NotNil(t, parser.definitions)
	assert.NotNil(t, parser.values)
	assert.NotNil(t, parser.result)
	assert.Empty(t, parser.definitions)
}

// TestParser_AddDefinition tests adding flag definitions.
func TestParser_AddDefinition(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name      string
		def       *FlagDefinition
		wantError bool
		errMsg    string
	}{
		{
			name: "valid string flag",
			def: &FlagDefinition{
				Name:        "host",
				Type:        FlagTypeString,
				Description: "Server host",
			},
			wantError: false,
		},
		{
			name: "valid int flag with default",
			def: &FlagDefinition{
				Name:        "port",
				Type:        FlagTypeInt,
				Default:     "8080",
				Description: "Server port",
			},
			wantError: false,
		},
		{
			name: "valid required flag",
			def: &FlagDefinition{
				Name:        "api-key",
				Type:        FlagTypeString,
				Required:    true,
				Description: "API key",
			},
			wantError: false,
		},
		{
			name:      "nil definition",
			def:       nil,
			wantError: true,
			errMsg:    "flag definition cannot be nil",
		},
		{
			name: "invalid flag type",
			def: &FlagDefinition{
				Name:        "test",
				Type:        FlagType("invalid"),
				Description: "Test flag",
			},
			wantError: true,
			errMsg:    "invalid flag type",
		},
		{
			name: "empty flag name",
			def: &FlagDefinition{
				Name:        "",
				Type:        FlagTypeString,
				Description: "Empty name",
			},
			wantError: true,
			errMsg:    "flag name cannot be empty",
		},
		{
			name: "shorthand too long",
			def: &FlagDefinition{
				Name:        "test",
				Shorthand:   "ab",
				Type:        FlagTypeString,
				Description: "Test flag",
			},
			wantError: true,
			errMsg:    "shorthand must be a single character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.AddDefinition(tt.def)
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestParser_AddDefinition_Duplicate tests adding duplicate definitions.
func TestParser_AddDefinition_Duplicate(t *testing.T) {
	parser := NewParser()

	def := &FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	}

	// First add should succeed
	err := parser.AddDefinition(def)
	require.NoError(t, err)

	// Second add should fail
	err = parser.AddDefinition(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// TestParser_RemoveDefinition tests removing flag definitions.
func TestParser_RemoveDefinition(t *testing.T) {
	parser := NewParser()

	def := &FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	}

	// Add and then remove
	err := parser.AddDefinition(def)
	require.NoError(t, err)

	removed := parser.RemoveDefinition("host")
	assert.True(t, removed)

	// Try to remove again
	removed = parser.RemoveDefinition("host")
	assert.False(t, removed)

	// Try to remove non-existent
	removed = parser.RemoveDefinition("nonexistent")
	assert.False(t, removed)
}

// TestParser_GetDefinition tests retrieving flag definitions.
func TestParser_GetDefinition(t *testing.T) {
	parser := NewParser()

	def := &FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	}

	err := parser.AddDefinition(def)
	require.NoError(t, err)

	// Get existing
	retrieved, exists := parser.GetDefinition("host")
	require.True(t, exists)
	assert.Equal(t, "host", retrieved.Name)
	assert.Equal(t, FlagTypeString, retrieved.Type)

	// Get non-existent
	_, exists = parser.GetDefinition("nonexistent")
	assert.False(t, exists)
}

// TestParser_Validate tests validating all definitions.
func TestParser_Validate(t *testing.T) {
	parser := NewParser()

	// Add valid definitions
	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "port",
		Type:        FlagTypeInt,
		Default:     "8080",
		Description: "Server port",
	})
	require.NoError(t, err)

	// Validate should pass
	err = parser.Validate()
	require.NoError(t, err)
}

// TestParser_Bind tests binding definitions to Cobra commands.
func TestParser_Bind(t *testing.T) {
	parser := NewParser()

	// Add various flag types
	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Shorthand:   "H",
		Type:        FlagTypeString,
		Default:     "localhost",
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "port",
		Shorthand:   "p",
		Type:        FlagTypeInt,
		Default:     "8080",
		Description: "Server port",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "verbose",
		Shorthand:   "v",
		Type:        FlagTypeBool,
		Default:     "false",
		Description: "Enable verbose output",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "config",
		Type:        FlagTypePath,
		Description: "Config file path",
	})
	require.NoError(t, err)

	// Create command and bind
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Verify flags were added
	hostFlag := cmd.Flags().Lookup("host")
	require.NotNil(t, hostFlag)
	assert.Equal(t, "localhost", hostFlag.DefValue)

	portFlag := cmd.Flags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "8080", portFlag.DefValue)

	verboseFlag := cmd.Flags().Lookup("verbose")
	require.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)

	configFlag := cmd.Flags().Lookup("config")
	require.NotNil(t, configFlag)
}

// TestParser_Bind_NilCommand tests binding to nil command.
func TestParser_Bind_NilCommand(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.Bind(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command cannot be nil")
}

// TestParser_Bind_RequiredFlag tests binding required flags.
func TestParser_Bind_RequiredFlag(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Type:        FlagTypeString,
		Required:    true,
		Description: "API authentication key",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		Run:   func(cmd *cobra.Command, args []string) {},
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Verify flag is marked as required by checking it exists
	flag := cmd.Flags().Lookup("api-key")
	require.NotNil(t, flag)

	// The flag should be registered; required validation happens at parse time
	// We'll verify the required flag validation works in Parse tests
	assert.Equal(t, "api-key", flag.Name)
}

// TestParser_Bind_HiddenFlag tests binding hidden flags.
func TestParser_Bind_HiddenFlag(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "internal",
		Type:        FlagTypeString,
		Hidden:      true,
		Description: "Internal flag",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	flag := cmd.Flags().Lookup("internal")
	require.NotNil(t, flag)
	assert.True(t, flag.Hidden)
}

// TestParser_Bind_DeprecatedFlag tests binding deprecated flags.
func TestParser_Bind_DeprecatedFlag(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "old-flag",
		Type:        FlagTypeString,
		Deprecated:  "Use --new-flag instead",
		Description: "Deprecated flag",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	flag := cmd.Flags().Lookup("old-flag")
	require.NotNil(t, flag)
	assert.Equal(t, "Use --new-flag instead", flag.Deprecated)
}

// TestParser_Parse tests parsing flag values.
func TestParser_Parse(t *testing.T) {
	parser := NewParser()

	// Add definitions
	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Default:     "localhost",
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "port",
		Type:        FlagTypeInt,
		Default:     "8080",
		Description: "Server port",
	})
	require.NoError(t, err)

	// Create command and bind
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Set flag values
	cmd.SetArgs([]string{"--host", "example.com", "--port", "9090"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Parse
	result, err := parser.Parse(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check values
	hostValue, exists := result.Get("host")
	require.True(t, exists)
	assert.Equal(t, "example.com", hostValue.Raw)
	assert.True(t, hostValue.IsSet)

	portValue, exists := result.Get("port")
	require.True(t, exists)
	assert.Equal(t, "9090", portValue.Raw)
	assert.True(t, portValue.IsSet)
}

// TestParser_Parse_RequiredValidation tests validation of required flags.
func TestParser_Parse_RequiredValidation(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Type:        FlagTypeString,
		Required:    true,
		Description: "API key",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Parse without setting required flag
	result, err := parser.Parse(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "required flag --api-key not provided")
}

// TestParser_Parse_NilCommand tests parsing with nil command.
func TestParser_Parse_NilCommand(t *testing.T) {
	parser := NewParser()

	_, err := parser.Parse(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command cannot be nil")
}

// TestParser_GetValue tests getting individual values.
func TestParser_GetValue(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Default:     "localhost",
		Description: "Server host",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Parse first
	_, err = parser.Parse(cmd)
	require.NoError(t, err)

	// Get value
	value, exists := parser.GetValue("host")
	require.True(t, exists)
	assert.Equal(t, "localhost", value.Raw)

	// Get non-existent
	_, exists = parser.GetValue("nonexistent")
	assert.False(t, exists)
}

// TestParser_Clear tests clearing parser state.
func TestParser_Clear(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	assert.Len(t, parser.definitions, 1)

	parser.Clear()

	assert.Len(t, parser.definitions, 0)
	assert.NotNil(t, parser.result)
}

// TestParser_ListDefinitions tests listing all definitions.
func TestParser_ListDefinitions(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "port",
		Type:        FlagTypeInt,
		Description: "Server port",
	})
	require.NoError(t, err)

	names := parser.ListDefinitions()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "host")
	assert.Contains(t, names, "port")
}

// TestParser_HasDefinition tests checking for definition existence.
func TestParser_HasDefinition(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	assert.True(t, parser.HasDefinition("host"))
	assert.False(t, parser.HasDefinition("nonexistent"))
}

// TestParser_BindAndParse tests the convenience method.
func TestParser_BindAndParse(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Default:     "localhost",
		Description: "Server host",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	result, err := parser.BindAndParse(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)

	value, exists := result.Get("host")
	require.True(t, exists)
	assert.Equal(t, "localhost", value.Raw)
}

// TestParser_ValidateFlag tests validating individual flags.
func TestParser_ValidateFlag(t *testing.T) {
	parser := NewParser()

	// Create a custom validator
	validator := ValidatorFunc(func(value string) error {
		if value == "" {
			return fmt.Errorf("value cannot be empty")
		}
		return nil
	})

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "name",
		Type:        FlagTypeString,
		Description: "Name value",
		Validator:   validator,
	})
	require.NoError(t, err)

	// Valid value
	err = parser.ValidateFlag("name", "test")
	require.NoError(t, err)

	// Invalid value
	err = parser.ValidateFlag("name", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value cannot be empty")

	// Non-existent flag
	err = parser.ValidateFlag("nonexistent", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestParser_String tests string representation.
func TestParser_String(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Type:        FlagTypeString,
		Description: "Server host",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "port",
		Type:        FlagTypeInt,
		Required:    true,
		Description: "Server port",
	})
	require.NoError(t, err)

	str := parser.String()
	assert.Contains(t, str, "Parser with 2 definitions")
	assert.Contains(t, str, "host")
	assert.Contains(t, str, "port")
	assert.Contains(t, str, "required=true")
}

// TestParser_AllFlagTypes tests binding all supported flag types.
func TestParser_AllFlagTypes(t *testing.T) {
	parser := NewParser()

	// Add all flag types
	flagTypes := []FlagType{
		FlagTypeString,
		FlagTypeInt,
		FlagTypeBool,
		FlagTypeURL,
		FlagTypeIP,
		FlagTypeEmail,
		FlagTypePath,
	}

	for i, ft := range flagTypes {
		err := parser.AddDefinition(&FlagDefinition{
			Name:        fmt.Sprintf("flag%d", i),
			Type:        ft,
			Description: fmt.Sprintf("Flag of type %s", ft),
		})
		require.NoError(t, err)
	}

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err := parser.Bind(cmd)
	require.NoError(t, err)

	// Verify all flags exist
	for i := range flagTypes {
		flagName := fmt.Sprintf("flag%d", i)
		flag := cmd.Flags().Lookup(flagName)
		require.NotNil(t, flag, "Flag %s should exist", flagName)
	}
}

// TestParser_ParseResult_ErrorAggregation tests error aggregation in ParseResult.
func TestParser_ParseResult_ErrorAggregation(t *testing.T) {
	parser := NewParser()

	// Add two required flags
	err := parser.AddDefinition(
		&FlagDefinition{
			Name:        "flag1",
			Type:        FlagTypeString,
			Required:    true,
			Description: "First required flag",
		},
	)
	require.NoError(t, err)

	err = parser.AddDefinition(
		&FlagDefinition{
			Name:        "flag2",
			Type:        FlagTypeString,
			Required:    true,
			Description: "Second required flag",
		},
	)
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	result, err := parser.Parse(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have two errors
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 2)

	// Error method should aggregate them
	err = result.Error()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation errors:")
}

// TestParser_ParseResult_NoErrors tests ParseResult with no errors.
func TestParser_ParseResult_NoErrors(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(
		&FlagDefinition{
			Name:        "optional",
			Type:        FlagTypeString,
			Description: "Optional flag",
		},
	)
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	result, err := parser.Parse(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)

	// No required flags, so no errors
	assert.False(t, result.HasErrors())
	assert.Nil(t, result.Error())
}
