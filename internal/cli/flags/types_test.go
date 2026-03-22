package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		flagType FlagType
		want     bool
	}{
		{
			name:     "string type is valid",
			flagType: FlagTypeString,
			want:     true,
		},
		{
			name:     "int type is valid",
			flagType: FlagTypeInt,
			want:     true,
		},
		{
			name:     "bool type is valid",
			flagType: FlagTypeBool,
			want:     true,
		},
		{
			name:     "url type is valid",
			flagType: FlagTypeURL,
			want:     true,
		},
		{
			name:     "ip type is valid",
			flagType: FlagTypeIP,
			want:     true,
		},
		{
			name:     "email type is valid",
			flagType: FlagTypeEmail,
			want:     true,
		},
		{
			name:     "path type is valid",
			flagType: FlagTypePath,
			want:     true,
		},
		{
			name:     "unknown type is invalid",
			flagType: FlagType("unknown"),
			want:     false,
		},
		{
			name:     "empty type is invalid",
			flagType: FlagType(""),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.flagType.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFlagType_String(t *testing.T) {
	assert.Equal(t, "string", FlagTypeString.String())
	assert.Equal(t, "int", FlagTypeInt.String())
	assert.Equal(t, "bool", FlagTypeBool.String())
	assert.Equal(t, "url", FlagTypeURL.String())
	assert.Equal(t, "ip", FlagTypeIP.String())
	assert.Equal(t, "email", FlagTypeEmail.String())
	assert.Equal(t, "path", FlagTypePath.String())
}

func TestFlagDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		def     FlagDefinition
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid string flag definition",
			def: FlagDefinition{
				Name:        "host",
				Type:        FlagTypeString,
				Description: "Server host",
				Required:    true,
			},
			wantErr: false,
		},
		{
			name: "valid int flag with shorthand",
			def: FlagDefinition{
				Name:        "port",
				Shorthand:   "p",
				Type:        FlagTypeInt,
				Description: "Server port",
				Default:     "8080",
			},
			wantErr: false,
		},
		{
			name: "valid flag with all optional fields",
			def: FlagDefinition{
				Name:        "endpoint",
				Shorthand:   "e",
				Type:        FlagTypeURL,
				Description: "API endpoint URL",
				Default:     "https://api.example.com",
				Required:    false,
				Hidden:      false,
				Deprecated:  "",
				EnvVar:      "API_ENDPOINT",
			},
			wantErr: false,
		},
		{
			name:    "empty name fails validation",
			def:     FlagDefinition{Name: "", Type: FlagTypeString},
			wantErr: true,
			errMsg:  "flag name cannot be empty",
		},
		{
			name:    "invalid flag type fails validation",
			def:     FlagDefinition{Name: "test", Type: FlagType("invalid")},
			wantErr: true,
			errMsg:  "invalid flag type",
		},
		{
			name:    "shorthand too long fails validation",
			def:     FlagDefinition{Name: "test", Type: FlagTypeString, Shorthand: "ab"},
			wantErr: true,
			errMsg:  "shorthand must be a single character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidatorFunc(t *testing.T) {
	t.Run("validate returns error from function", func(t *testing.T) {
		vf := ValidatorFunc(func(value string) error {
			if value == "" {
				return assert.AnError
			}
			return nil
		})

		err := vf.Validate("")
		assert.Error(t, err)

		err = vf.Validate("valid")
		assert.NoError(t, err)
	})

	t.Run("name returns custom", func(t *testing.T) {
		vf := ValidatorFunc(func(value string) error { return nil })
		assert.Equal(t, "custom", vf.Name())
	})
}

func TestValue_String(t *testing.T) {
	v := Value{Raw: "test-value", Type: FlagTypeString, IsSet: true}
	assert.Equal(t, "test-value", v.String())
}

func TestValue_Int(t *testing.T) {
	tests := []struct {
		name    string
		value   Value
		want    int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid int value",
			value:   Value{Raw: "42", Type: FlagTypeInt, IsSet: true},
			want:    42,
			wantErr: false,
		},
		{
			name:    "negative int value",
			value:   Value{Raw: "-123", Type: FlagTypeInt, IsSet: true},
			want:    -123,
			wantErr: false,
		},
		{
			name:    "zero value",
			value:   Value{Raw: "0", Type: FlagTypeInt, IsSet: true},
			want:    0,
			wantErr: false,
		},
		{
			name:    "wrong type returns error",
			value:   Value{Raw: "42", Type: FlagTypeString, IsSet: true},
			want:    0,
			wantErr: true,
			errMsg:  "cannot convert string to int",
		},
		{
			name:    "invalid integer format",
			value:   Value{Raw: "not-a-number", Type: FlagTypeInt, IsSet: true},
			want:    0,
			wantErr: true,
			errMsg:  "invalid integer value",
		},
		{
			name:    "float truncated to int",
			value:   Value{Raw: "3.14", Type: FlagTypeInt, IsSet: true},
			want:    3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.value.Int()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestValue_Bool(t *testing.T) {
	tests := []struct {
		name    string
		value   Value
		want    bool
		wantErr bool
		errMsg  string
	}{
		{
			name:    "true value",
			value:   Value{Raw: "true", Type: FlagTypeBool, IsSet: true},
			want:    true,
			wantErr: false,
		},
		{
			name:    "false value",
			value:   Value{Raw: "false", Type: FlagTypeBool, IsSet: true},
			want:    false,
			wantErr: false,
		},
		{
			name:    "1 as true",
			value:   Value{Raw: "1", Type: FlagTypeBool, IsSet: true},
			want:    true,
			wantErr: false,
		},
		{
			name:    "0 as false",
			value:   Value{Raw: "0", Type: FlagTypeBool, IsSet: true},
			want:    false,
			wantErr: false,
		},
		{
			name:    "yes as true",
			value:   Value{Raw: "yes", Type: FlagTypeBool, IsSet: true},
			want:    true,
			wantErr: false,
		},
		{
			name:    "no as false",
			value:   Value{Raw: "no", Type: FlagTypeBool, IsSet: true},
			want:    false,
			wantErr: false,
		},
		{
			name:    "on as true",
			value:   Value{Raw: "on", Type: FlagTypeBool, IsSet: true},
			want:    true,
			wantErr: false,
		},
		{
			name:    "off as false",
			value:   Value{Raw: "off", Type: FlagTypeBool, IsSet: true},
			want:    false,
			wantErr: false,
		},
		{
			name:    "wrong type returns error",
			value:   Value{Raw: "true", Type: FlagTypeString, IsSet: true},
			want:    false,
			wantErr: true,
			errMsg:  "cannot convert string to bool",
		},
		{
			name:    "invalid boolean value",
			value:   Value{Raw: "maybe", Type: FlagTypeBool, IsSet: true},
			want:    false,
			wantErr: true,
			errMsg:  "invalid boolean value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.value.Bool()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseResult_Get(t *testing.T) {
	pr := ParseResult{
		Values: map[string]Value{
			"host": {Raw: "localhost", Type: FlagTypeString, IsSet: true},
			"port": {Raw: "8080", Type: FlagTypeInt, IsSet: true},
		},
		Errors: nil,
	}

	t.Run("get existing value", func(t *testing.T) {
		val, exists := pr.Get("host")
		assert.True(t, exists)
		assert.Equal(t, "localhost", val.Raw)
		assert.Equal(t, FlagTypeString, val.Type)
	})

	t.Run("get another existing value", func(t *testing.T) {
		val, exists := pr.Get("port")
		assert.True(t, exists)
		assert.Equal(t, "8080", val.Raw)
		assert.Equal(t, FlagTypeInt, val.Type)
	})

	t.Run("get non-existent value", func(t *testing.T) {
		val, exists := pr.Get("unknown")
		assert.False(t, exists)
		assert.Equal(t, Value{}, val)
	})
}

func TestParseResult_HasErrors(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		pr := ParseResult{Values: nil, Errors: nil}
		assert.False(t, pr.HasErrors())
	})

	t.Run("empty errors slice", func(t *testing.T) {
		pr := ParseResult{Values: nil, Errors: []error{}}
		assert.False(t, pr.HasErrors())
	})

	t.Run("has errors", func(t *testing.T) {
		pr := ParseResult{
			Values: nil,
			Errors: []error{assert.AnError},
		}
		assert.True(t, pr.HasErrors())
	})
}

func TestParseResult_Error(t *testing.T) {
	t.Run("no errors returns nil", func(t *testing.T) {
		pr := ParseResult{Values: nil, Errors: nil}
		assert.NoError(t, pr.Error())
	})

	t.Run("single error returns that error", func(t *testing.T) {
		pr := ParseResult{
			Values: nil,
			Errors: []error{assert.AnError},
		}
		err := pr.Error()
		require.Error(t, err)
		assert.Contains(t, err.Error(), assert.AnError.Error())
	})

	t.Run("multiple errors aggregated", func(t *testing.T) {
		pr := ParseResult{
			Values: nil,
			Errors: []error{
				assert.AnError,
				assert.AnError,
			},
		}
		err := pr.Error()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation errors:")
		assert.Contains(t, err.Error(), "1.")
		assert.Contains(t, err.Error(), "2.")
	})
}

func TestFlagTypeConstants(t *testing.T) {
	// Ensure all constants have expected values
	assert.Equal(t, FlagType("string"), FlagTypeString)
	assert.Equal(t, FlagType("int"), FlagTypeInt)
	assert.Equal(t, FlagType("bool"), FlagTypeBool)
	assert.Equal(t, FlagType("url"), FlagTypeURL)
	assert.Equal(t, FlagType("ip"), FlagTypeIP)
	assert.Equal(t, FlagType("email"), FlagTypeEmail)
	assert.Equal(t, FlagType("path"), FlagTypePath)
}
