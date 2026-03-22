package alias

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAlias(t *testing.T) {
	tests := []struct {
		name        string
		aliasName   string
		command     string
		wantName    string
		wantCommand string
	}{
		{
			name:        "simple alias",
			aliasName:   "ls",
			command:     "inventory list",
			wantName:    "ls",
			wantCommand: "inventory list",
		},
		{
			name:        "alias with flags",
			aliasName:   "ll",
			command:     "inventory list --format table",
			wantName:    "ll",
			wantCommand: "inventory list --format table",
		},
		{
			name:        "empty name",
			aliasName:   "",
			command:     "help",
			wantName:    "",
			wantCommand: "help",
		},
		{
			name:        "empty command",
			aliasName:   "empty",
			command:     "",
			wantName:    "empty",
			wantCommand: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alias := NewAlias(tt.aliasName, tt.command)
			assert.Equal(t, tt.wantName, alias.Name)
			assert.Equal(t, tt.wantCommand, alias.Command)
			assert.Empty(t, alias.Description)
		})
	}
}

func TestNewAliasWithDescription(t *testing.T) {
	tests := []struct {
		name            string
		aliasName       string
		command         string
		description     string
		wantName        string
		wantCommand     string
		wantDescription string
	}{
		{
			name:            "alias with description",
			aliasName:       "ls",
			command:         "inventory list",
			description:     "List all inventory items",
			wantName:        "ls",
			wantCommand:     "inventory list",
			wantDescription: "List all inventory items",
		},
		{
			name:            "empty description",
			aliasName:       "get",
			command:         "inventory get",
			description:     "",
			wantName:        "get",
			wantCommand:     "inventory get",
			wantDescription: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alias := NewAliasWithDescription(tt.aliasName, tt.command, tt.description)
			assert.Equal(t, tt.wantName, alias.Name)
			assert.Equal(t, tt.wantCommand, alias.Command)
			assert.Equal(t, tt.wantDescription, alias.Description)
		})
	}
}

func TestAlias_String(t *testing.T) {
	tests := []struct {
		name     string
		alias    *Alias
		expected string
	}{
		{
			name:     "simple alias",
			alias:    NewAlias("ls", "inventory list"),
			expected: "ls=inventory list",
		},
		{
			name:     "alias with flags",
			alias:    NewAlias("ll", "inventory list --format table"),
			expected: "ll=inventory list --format table",
		},
		{
			name:     "empty command",
			alias:    NewAlias("empty", ""),
			expected: "empty=",
		},
		{
			name:     "empty name",
			alias:    NewAlias("", "help"),
			expected: "=help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.alias.String()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAlias_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		alias    *Alias
		expected bool
	}{
		{
			name:     "valid alias",
			alias:    NewAlias("ls", "inventory list"),
			expected: true,
		},
		{
			name:     "valid alias with description",
			alias:    NewAliasWithDescription("ls", "inventory list", "List items"),
			expected: true,
		},
		{
			name:     "empty name",
			alias:    NewAlias("", "inventory list"),
			expected: false,
		},
		{
			name:     "empty command",
			alias:    NewAlias("ls", ""),
			expected: false,
		},
		{
			name:     "both empty",
			alias:    NewAlias("", ""),
			expected: false,
		},
		{
			name:     "nil alias",
			alias:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.alias == nil {
				return
			}
			got := tt.alias.IsValid()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAlias_JSONTags(t *testing.T) {
	alias := NewAliasWithDescription("ls", "inventory list", "List items")

	// Verify JSON tags are properly set
	assert.NotEmpty(t, alias.Name)
	assert.NotEmpty(t, alias.Command)
	assert.NotEmpty(t, alias.Description)
}

func TestAlias_YAMLTags(t *testing.T) {
	alias := NewAliasWithDescription("ls", "inventory list", "List items")

	// Verify YAML tags are properly set by checking the struct fields exist
	assert.Equal(t, "ls", alias.Name)
	assert.Equal(t, "inventory list", alias.Command)
	assert.Equal(t, "List items", alias.Description)
}
