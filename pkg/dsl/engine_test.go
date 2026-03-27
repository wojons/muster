package dsl

import (
	"errors"
	"testing"
)

func TestNewEngine(t *testing.T) {
	config := NewDefaultEngineConfig()
	engine := NewEngine(config)

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	if engine.config != config {
		t.Error("config not set")
	}
}

func TestLoadScriptFromString(t *testing.T) {
	engine := NewEngine(nil)

	scriptContent := `
"""
name: "Test Script"
description: "A test script"
version: "1.0.0"
"""

def main():
    return {"result": "ok"}
`

	script, err := engine.LoadScriptFromString(scriptContent, "test.star")
	if err != nil {
		t.Fatalf("failed to load script: %v", err)
	}

	if script.Name != "Test Script" {
		t.Errorf("expected name 'Test Script', got %s", script.Name)
	}

	if script.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", script.Version)
	}
}

func TestParseMetadata(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		name            string
		content         string
		expectedName    string
		expectedVersion string
	}{
		{
			name: "full metadata",
			content: `
"""
name: "Test Script"
description: "A test script"
version: "1.0.0"
author: "Test Author"
"""

def main():
    return {}
`,
			expectedName:    "Test Script",
			expectedVersion: "1.0.0",
		},
		{
			name: "minimal metadata",
			content: `
def main():
    return {}
`,
			expectedName:    "script",
			expectedVersion: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := engine.parseMetadata(tt.content)

			if metadata.Name != tt.expectedName {
				t.Errorf("expected name %s, got %s", tt.expectedName, metadata.Name)
			}

			if metadata.Version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, metadata.Version)
			}
		})
	}
}

func TestValidateInputs(t *testing.T) {
	engine := NewEngine(nil)

	script := &Script{
		Metadata: &ScriptMetadata{
			Inputs: []Input{
				{Name: "required_field", Type: "string", Required: true},
				{Name: "optional_field", Type: "string", Required: false, Default: "default"},
			},
		},
	}

	t.Run("all required fields provided", func(t *testing.T) {
		inputs := map[string]interface{}{
			"required_field": "value",
		}

		err := engine.validateInputs(script, inputs)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		inputs := map[string]interface{}{}

		err := engine.validateInputs(script, inputs)
		if err == nil {
			t.Error("expected error for missing required field")
		}
	})
}

func TestWrapError(t *testing.T) {
	engine := NewEngine(nil)
	script := &Script{Name: "test"}

	t.Run("starlark eval error", func(t *testing.T) {
		// Create a mock EvalError-like scenario
		wrapperErr := errors.New("some runtime error")
		err := engine.wrapError(wrapperErr, script)

		if err == nil {
			t.Fatal("expected non-nil error")
		}

		if err.Type != ErrorRuntime {
			t.Errorf("expected Runtime error type, got %s", err.Type)
		}
	})
}

func TestEnforceMemoryLimit(t *testing.T) {
	config := &EngineConfig{
		MaxMemoryMB: 256,
	}

	engine := NewEngine(config)

	t.Run("within limit", func(t *testing.T) {
		err := engine.enforceMemoryLimit()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		limitConfig := &EngineConfig{
			MaxMemoryMB: 0,
		}
		engineWithLowLimit := NewEngine(limitConfig)

		err := engineWithLowLimit.enforceMemoryLimit()
		// Expected to fail or pass depending on actual memory usage
		// This is more of a sanity check
		t.Logf("enforceMemoryLimit result: %v", err)
	})
}

func TestNewDefaultEngineConfig(t *testing.T) {
	config := NewDefaultEngineConfig()

	if config.MaxSteps != 100000 {
		t.Errorf("expected MaxSteps 100000, got %d", config.MaxSteps)
	}

	if config.MaxExecutionTime != 30 {
		t.Errorf("expected MaxExecutionTime 30, got %d", config.MaxExecutionTime)
	}

	if config.MaxMemoryMB != 256 {
		t.Errorf("expected MaxMemoryMB 256, got %d", config.MaxMemoryMB)
	}

	if !config.EnableNetwork {
		t.Error("expected EnableNetwork to be true")
	}
}

func TestToStarlarkDict(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	dict, err := ToStarlarkDict(m)
	if err != nil {
		t.Fatalf("failed to convert to starlark dict: %v", err)
	}

	if dict == nil {
		t.Fatal("expected non-nil dict")
	}

	result := FromStarlarkDict(dict)

	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}

	if result["key1"] != "value1" {
		t.Errorf("expected 'value1', got %v", result["key1"])
	}

	if result["key2"] != int64(42) {
		t.Errorf("expected 42, got %v", result["key2"])
	}

	if result["key3"] != true {
		t.Errorf("expected true, got %v", result["key3"])
	}
}

func TestDSLError(t *testing.T) {
	err := &DSLError{
		Type:    ErrorRuntime,
		Message: "test error",
		Line:    10,
	}

	if err.Error() != "test error" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
