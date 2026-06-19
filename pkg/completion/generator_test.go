package completion

import (
	"strings"
	"testing"
)

func TestGenerator(t *testing.T) {
	gen := NewGenerator("test-app", "1.0.0")

	gen.AddGlobalFlag(Flag{
		Long:        "help",
		Short:       "h",
		Description: "Show help",
	})

	gen.AddCommand(Command{
		Name:        "get",
		Description: "Get a resource",
		Flags: []Flag{
			{Long: "name", Short: "n", Description: "Resource name"},
			{Long: "format", Short: "f", Description: "Output format", Values: []string{"json", "yaml", "table"}},
		},
	})

	gen.AddCommand(Command{
		Name:        "list",
		Description: "List resources",
		Flags: []Flag{
			{Long: "limit", Short: "l", Description: "Limit results"},
		},
	})

	shells := []Shell{ShellBash, ShellZsh, ShellFish, ShellPowerShell}

	for _, shell := range shells {
		t.Run(string(shell), func(t *testing.T) {
			if !gen.Supports(shell) {
				t.Fatalf("Shell %s should be supported", shell)
			}

			script, err := gen.Generate(shell)
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			if script == "" {
				t.Fatal("Generated script is empty")
			}

			if !strings.Contains(script, "test-app") {
				t.Error("Script doesn't contain app name")
			}

			if !strings.Contains(script, "completion") {
				t.Error("Script doesn't contain 'completion' keyword")
			}
		})
	}
}

func TestGeneratorUnsupportedShell(t *testing.T) {
	gen := NewGenerator("test-app", "1.0.0")

	if gen.Supports(Shell("unknown")) {
		t.Error("Should not support unknown shell")
	}

	_, err := gen.Generate(Shell("unknown"))
	if err == nil {
		t.Error("Expected error for invalid shell")
	}
}

func TestGetInstallPath(t *testing.T) {
	tests := []struct {
		shell     Shell
		expectIn  string
		expectErr bool
	}{
		{ShellBash, "bash_completion", false},
		{ShellZsh, "site-functions", false},
		{ShellFish, "completions", false},
		{ShellPowerShell, "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			path, err := GetInstallPath(tt.shell)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(path, tt.expectIn) {
					t.Errorf("Expected path to contain '%s', got '%s'", tt.expectIn, path)
				}
			}
		})
	}
}
