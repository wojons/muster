package flags

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_FullCommandLifecycle tests a complete command lifecycle
// with flag binding, parsing, and validation.
func TestIntegration_FullCommandLifecycle(t *testing.T) {
	parser := NewParser()

	// Add flag definitions
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

	var capturedHost string
	var capturedPort int
	var capturedVerbose bool

	// Create command
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse flags after they've been bound
			result, err := parser.Parse(cmd)
			if err != nil {
				return err
			}

			if result.HasErrors() {
				return result.Error()
			}

			// Capture values
			hostValue, _ := result.Get("host")
			capturedHost = hostValue.Raw

			portValue, _ := result.Get("port")
			portInt, _ := portValue.Int()
			capturedPort = portInt

			verboseValue, _ := result.Get("verbose")
			capturedVerbose, _ = verboseValue.Bool()

			return nil
		},
	}

	// Bind flags BEFORE executing the command
	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Set args and execute
	cmd.SetArgs([]string{"--host", "example.com", "-p", "9090", "-v"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Verify captured values
	assert.Equal(t, "example.com", capturedHost)
	assert.Equal(t, 9090, capturedPort)
	assert.True(t, capturedVerbose)
}

// TestIntegration_AllFlagTypes tests all flag types with Cobra commands.
func TestIntegration_AllFlagTypes(t *testing.T) {
	tests := []struct {
		name         string
		flagType     FlagType
		flagName     string
		defaultValue string
		inputValue   string
		expectedRaw  string
	}{
		{
			name:         "string flag",
			flagType:     FlagTypeString,
			flagName:     "name",
			defaultValue: "default",
			inputValue:   "test-value",
			expectedRaw:  "test-value",
		},
		{
			name:         "int flag",
			flagType:     FlagTypeInt,
			flagName:     "count",
			defaultValue: "0",
			inputValue:   "42",
			expectedRaw:  "42",
		},
		{
			name:         "bool flag",
			flagType:     FlagTypeBool,
			flagName:     "enable",
			defaultValue: "false",
			inputValue:   "true",
			expectedRaw:  "true",
		},
		{
			name:         "url flag",
			flagType:     FlagTypeURL,
			flagName:     "endpoint",
			defaultValue: "",
			inputValue:   "https://api.example.com",
			expectedRaw:  "https://api.example.com",
		},
		{
			name:         "ip flag",
			flagType:     FlagTypeIP,
			flagName:     "address",
			defaultValue: "",
			inputValue:   "192.168.1.1",
			expectedRaw:  "192.168.1.1",
		},
		{
			name:         "email flag",
			flagType:     FlagTypeEmail,
			flagName:     "contact",
			defaultValue: "",
			inputValue:   "test@example.com",
			expectedRaw:  "test@example.com",
		},
		{
			name:         "path flag",
			flagType:     FlagTypePath,
			flagName:     "config",
			defaultValue: "",
			inputValue:   "/etc/config.yaml",
			expectedRaw:  "/etc/config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()

			err := parser.AddDefinition(&FlagDefinition{
				Name:        tt.flagName,
				Type:        tt.flagType,
				Default:     tt.defaultValue,
				Description: fmt.Sprintf("Test %s flag", tt.flagType),
			})
			require.NoError(t, err)

			var capturedValue string

			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					value, exists := result.Get(tt.flagName)
					if exists {
						capturedValue = value.Raw
					}
					return nil
				},
			}

			// Bind flags before execution
			err = parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs([]string{fmt.Sprintf("--%s", tt.flagName), tt.inputValue})
			err = cmd.Execute()
			require.NoError(t, err)

			assert.Equal(t, tt.expectedRaw, capturedValue)
		})
	}
}

// TestIntegration_URLValidatorWithCobra tests URL validator integration.
func TestIntegration_URLValidatorWithCobra(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "api-url",
		Type:        FlagTypeURL,
		Description: "API URL",
		Validator:   URLWithSchemes("https"),
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid https URL",
			args:      []string{"--api-url", "https://api.example.com"},
			wantError: false,
		},
		{
			name:      "invalid http URL",
			args:      []string{"--api-url", "http://api.example.com"},
			wantError: true,
			errMsg:    "scheme 'http' is not allowed",
		},
		{
			name:      "invalid URL format",
			args:      []string{"--api-url", "not-a-url"},
			wantError: true,
			errMsg:    "URL must have a scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resultErr error

			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					if result.HasErrors() {
						resultErr = result.Error()
						return resultErr
					}
					return nil
				},
			}

			// Bind flags before execution
			err := parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_IPValidatorWithCobra tests IP validator integration.
func TestIntegration_IPValidatorWithCobra(t *testing.T) {
	parser := NewParser()

	// Create validator that only allows public IPv4
	validator := &IPValidator{
		AllowIPv4:     true,
		AllowIPv6:     false,
		AllowPrivate:  false,
		AllowLoopback: false,
	}

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "server-ip",
		Type:        FlagTypeIP,
		Description: "Server IP address",
		Validator:   validator,
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid public IP",
			args:      []string{"--server-ip", "8.8.8.8"},
			wantError: false,
		},
		{
			name:      "invalid private IP",
			args:      []string{"--server-ip", "192.168.1.1"},
			wantError: true,
			errMsg:    "private IP addresses are not allowed",
		},
		{
			name:      "invalid loopback IP",
			args:      []string{"--server-ip", "127.0.0.1"},
			wantError: true,
			errMsg:    "loopback addresses are not allowed",
		},
		{
			name:      "invalid IPv6",
			args:      []string{"--server-ip", "::1"},
			wantError: true,
			errMsg:    "IPv6 addresses are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					if result.HasErrors() {
						return result.Error()
					}
					return nil
				},
			}

			// Bind flags before execution
			err := parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_EmailValidatorWithCobra tests email validator integration.
func TestIntegration_EmailValidatorWithCobra(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "admin-email",
		Type:        FlagTypeEmail,
		Description: "Admin email address",
		Validator:   EmailWithDomains("example.com"),
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid email with allowed domain",
			args:      []string{"--admin-email", "admin@example.com"},
			wantError: false,
		},
		{
			name:      "invalid email domain",
			args:      []string{"--admin-email", "admin@gmail.com"},
			wantError: true,
			errMsg:    "domain 'gmail.com' is not allowed",
		},
		{
			name:      "invalid email format",
			args:      []string{"--admin-email", "not-an-email"},
			wantError: true,
			errMsg:    "invalid email address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					if result.HasErrors() {
						return result.Error()
					}
					return nil
				},
			}

			// Bind flags before execution
			err := parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_PathValidatorWithCobra tests path validator integration.
func TestIntegration_PathValidatorWithCobra(t *testing.T) {
	// Create temporary files and directories for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testDir := filepath.Join(tmpDir, "testdir")

	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = os.Mkdir(testDir, 0755)
	require.NoError(t, err)

	parser := NewParser()

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "config-file",
		Type:        FlagTypePath,
		Description: "Configuration file path",
		Validator:   ExistingFile(),
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid existing file",
			args:      []string{"--config-file", testFile},
			wantError: false,
		},
		{
			name:      "invalid directory instead of file",
			args:      []string{"--config-file", testDir},
			wantError: true,
			errMsg:    "must be a file",
		},
		{
			name:      "invalid non-existent file",
			args:      []string{"--config-file", "/nonexistent/file.txt"},
			wantError: true,
			errMsg:    "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					if result.HasErrors() {
						return result.Error()
					}
					return nil
				},
			}

			// Bind flags before execution
			err := parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_RequiredFlagsWithCobra tests required flag validation.
func TestIntegration_RequiredFlagsWithCobra(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Type:        FlagTypeString,
		Required:    true,
		Description: "API authentication key",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "optional-flag",
		Type:        FlagTypeString,
		Required:    false,
		Description: "Optional flag",
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		wantError bool
		errMsg    string
	}{
		{
			name:      "missing required flag",
			args:      []string{},
			wantError: true,
			errMsg:    "required flag(s)",
		},
		{
			name:      "provided required flag",
			args:      []string{"--api-key", "secret123"},
			wantError: false,
		},
		{
			name:      "provided required flag with optional",
			args:      []string{"--api-key", "secret123", "--optional-flag", "value"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				RunE: func(cmd *cobra.Command, args []string) error {
					result, err := parser.Parse(cmd)
					if err != nil {
						return err
					}

					if result.HasErrors() {
						return result.Error()
					}
					return nil
				},
			}

			// Bind flags before execution
			err := parser.Bind(cmd)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_EnvironmentVariableWithCobra tests environment variable fallback.
func TestIntegration_EnvironmentVariableWithCobra(t *testing.T) {
	// Set environment variable
	err := os.Setenv("TEST_API_KEY", "env-api-key-123")
	require.NoError(t, err)
	defer func() {
		_ = os.Unsetenv("TEST_API_KEY")
	}()

	parser := NewParser()

	// Note: EnvVar is a definition field but actual env var binding requires
	// additional integration with Cobra/Viper. This test verifies the parser
	// stores the EnvVar configuration.
	err = parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Type:        FlagTypeString,
		EnvVar:      "TEST_API_KEY",
		Description: "API key from env",
	})
	require.NoError(t, err)

	// Verify the definition stores the EnvVar
	def, exists := parser.GetDefinition("api-key")
	require.True(t, exists)
	assert.Equal(t, "TEST_API_KEY", def.EnvVar)

	// For actual env var integration with Cobra, the application would need to
	// manually set the default value from the environment variable or use Viper
}

// TestIntegration_SubcommandsWithFlags tests subcommands with different flag sets.
func TestIntegration_SubcommandsWithFlags(t *testing.T) {
	// Subcommand parser
	subParser := NewParser()
	err := subParser.AddDefinition(&FlagDefinition{
		Name:        "name",
		Type:        FlagTypeString,
		Required:    true,
		Description: "Resource name",
	})
	require.NoError(t, err)

	err = subParser.AddDefinition(&FlagDefinition{
		Name:        "count",
		Type:        FlagTypeInt,
		Default:     "1",
		Description: "Number of resources",
	})
	require.NoError(t, err)

	var subName string
	var subCount int

	// Create subcommand
	subCmd := &cobra.Command{
		Use:   "create",
		Short: "Create resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse subcommand flags
			result, err := subParser.Parse(cmd)
			if err != nil {
				return err
			}

			if result.HasErrors() {
				return result.Error()
			}

			nameValue, _ := result.Get("name")
			subName = nameValue.Raw

			countValue, _ := result.Get("count")
			subCount, _ = countValue.Int()

			return nil
		},
	}

	// Bind subcommand flags
	err = subParser.Bind(subCmd)
	require.NoError(t, err)

	// Create parent command
	parentCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Add subcommand
	parentCmd.AddCommand(subCmd)

	// Execute subcommand flags
	parentCmd.SetArgs([]string{"create", "--name", "myresource", "--count", "5"})
	err = parentCmd.Execute()
	require.NoError(t, err)

	// Verify values
	assert.Equal(t, "myresource", subName)
	assert.Equal(t, 5, subCount)
}

// TestIntegration_FlagShorthands tests flag shorthands work correctly.
func TestIntegration_FlagShorthands(t *testing.T) {
	parser := NewParser()

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

	var capturedHost string
	var capturedPort int

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return err
			}

			hostValue, _ := result.Get("host")
			capturedHost = hostValue.Raw

			portValue, _ := result.Get("port")
			capturedPort, _ = portValue.Int()

			return nil
		},
	}

	// Bind flags before execution
	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Test using shorthands
	cmd.SetArgs([]string{"-H", "example.com", "-p", "9090"})
	err = cmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "example.com", capturedHost)
	assert.Equal(t, 9090, capturedPort)
}

// TestIntegration_HiddenAndDeprecatedFlags tests hidden and deprecated flags.
func TestIntegration_HiddenAndDeprecatedFlags(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "visible",
		Type:        FlagTypeString,
		Description: "Visible flag",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "hidden",
		Type:        FlagTypeString,
		Hidden:      true,
		Description: "Hidden flag",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "deprecated",
		Type:        FlagTypeString,
		Deprecated:  "Use --new-flag instead",
		Description: "Deprecated flag",
	})
	require.NoError(t, err)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := parser.Parse(cmd)
			return err
		},
	}

	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Check that hidden flag is marked hidden
	hiddenFlag := cmd.Flags().Lookup("hidden")
	require.NotNil(t, hiddenFlag)
	assert.True(t, hiddenFlag.Hidden)

	// Check that deprecated flag has deprecation message
	deprecatedFlag := cmd.Flags().Lookup("deprecated")
	require.NotNil(t, deprecatedFlag)
	assert.Equal(t, "Use --new-flag instead", deprecatedFlag.Deprecated)

	// Check that visible flag is not hidden
	visibleFlag := cmd.Flags().Lookup("visible")
	require.NotNil(t, visibleFlag)
	assert.False(t, visibleFlag.Hidden)
}

// TestIntegration_MultipleValidationErrors tests multiple validation errors.
func TestIntegration_MultipleValidationErrors(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Type:        FlagTypeString,
		Required:    true,
		Description: "API key",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "endpoint",
		Type:        FlagTypeURL,
		Required:    true,
		Description: "API endpoint",
		Validator:   URL(),
	})
	require.NoError(t, err)

	var output bytes.Buffer

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return err
			}

			if result.HasErrors() {
				return result.Error()
			}
			return nil
		},
	}
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	// Bind flags before execution
	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Execute without any flags - should get multiple errors
	cmd.SetArgs([]string{})
	err = cmd.Execute()

	require.Error(t, err)
	// Cobra combines required flag errors differently
	errStr := err.Error()
	assert.Contains(t, errStr, "required flag(s)")
	assert.Contains(t, errStr, "api-key")
	assert.Contains(t, errStr, "endpoint")
}

// TestIntegration_ComplexScenario tests a complex real-world scenario.
func TestIntegration_ComplexScenario(t *testing.T) {
	parser := NewParser()

	// Add multiple flags simulating a real CLI tool
	err := parser.AddDefinition(&FlagDefinition{
		Name:        "host",
		Shorthand:   "H",
		Type:        FlagTypeURL,
		Required:    true,
		Description: "API host URL",
		Validator:   URLWithSchemes("https"),
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "api-key",
		Shorthand:   "k",
		Type:        FlagTypeString,
		Required:    true,
		Description: "API authentication key",
	})
	require.NoError(t, err)

	err = parser.AddDefinition(&FlagDefinition{
		Name:        "timeout",
		Shorthand:   "t",
		Type:        FlagTypeInt,
		Default:     "30",
		Description: "Request timeout in seconds",
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
		Shorthand:   "c",
		Type:        FlagTypePath,
		Description: "Configuration file path",
		Validator:   ExistingFile(),
	})
	require.NoError(t, err)

	type capturedConfig struct {
		Host    string
		APIKey  string
		Timeout int
		Verbose bool
		Config  string
	}

	var config capturedConfig

	cmd := &cobra.Command{
		Use:   "api-client",
		Short: "API client CLI",
		Long:  `A comprehensive API client demonstrating the flags package integration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return fmt.Errorf("failed to bind/parse: %w", err)
			}

			if result.HasErrors() {
				return result.Error()
			}

			// Extract all values
			hostValue, _ := result.Get("host")
			config.Host = hostValue.Raw

			apiKeyValue, _ := result.Get("api-key")
			config.APIKey = apiKeyValue.Raw

			timeoutValue, _ := result.Get("timeout")
			config.Timeout, _ = timeoutValue.Int()

			verboseValue, _ := result.Get("verbose")
			config.Verbose, _ = verboseValue.Bool()

			configValue, exists := result.Get("config")
			if exists {
				config.Config = configValue.Raw
			}

			return nil
		},
	}

	// Bind flags before execution
	err = parser.Bind(cmd)
	require.NoError(t, err)

	// Test with all valid values
	cmd.SetArgs([]string{
		"-H", "https://api.example.com",
		"-k", "secret-api-key",
		"-t", "60",
		"-v",
	})

	err = cmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "https://api.example.com", config.Host)
	assert.Equal(t, "secret-api-key", config.APIKey)
	assert.Equal(t, 60, config.Timeout)
	assert.True(t, config.Verbose)
	assert.Empty(t, config.Config) // Not provided

	// Test with invalid URL
	cmd.SetArgs([]string{
		"-H", "http://insecure.com",
		"-k", "secret",
	})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheme 'http' is not allowed")

	// Create new command for the next test (since previous command failed validation)
	cmd2 := &cobra.Command{
		Use:   "api-client",
		Short: "API client CLI",
		Long:  `A comprehensive API client demonstrating the flags package integration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return fmt.Errorf("failed to bind/parse: %w", err)
			}

			if result.HasErrors() {
				return result.Error()
			}
			return nil
		},
	}

	// Bind flags to new command
	err = parser.Bind(cmd2)
	require.NoError(t, err)

	// Test with missing required flags
	cmd2.SetArgs([]string{
		"-v",
	})

	err = cmd2.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

// TestIntegration_ParserReusability tests that parsers can be reused across commands.
func TestIntegration_ParserReusability(t *testing.T) {
	parser := NewParser()

	err := parser.AddDefinition(&FlagDefinition{
		Name:        "shared",
		Type:        FlagTypeString,
		Default:     "default-value",
		Description: "Shared flag",
	})
	require.NoError(t, err)

	// First command
	cmd1 := &cobra.Command{
		Use:   "cmd1",
		Short: "First command",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return err
			}

			value, _ := result.Get("shared")
			assert.Equal(t, "value1", value.Raw)
			return nil
		},
	}

	// Second command
	cmd2 := &cobra.Command{
		Use:   "cmd2",
		Short: "Second command",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parser.Parse(cmd)
			if err != nil {
				return err
			}

			value, _ := result.Get("shared")
			assert.Equal(t, "value2", value.Raw)
			return nil
		},
	}

	// Bind to both commands
	err = parser.Bind(cmd1)
	require.NoError(t, err)

	err = parser.Bind(cmd2)
	require.NoError(t, err)

	// Test first command
	cmd1.SetArgs([]string{"--shared", "value1"})
	err = cmd1.Execute()
	require.NoError(t, err)

	// Test second command (parser should still work)
	cmd2.SetArgs([]string{"--shared", "value2"})
	err = cmd2.Execute()
	require.NoError(t, err)
}
