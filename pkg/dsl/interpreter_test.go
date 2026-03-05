package dsl

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestNewInterpreter(t *testing.T) {
	builtins := map[string]interface{}{
		"test": func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("ok"), nil
		},
	}

	interp := NewInterpreter(builtins)
	if interp == nil {
		t.Fatal("expected non-nil interpreter")
	}

	if len(interp.predeclared) != 1 {
		t.Errorf("expected 1 predeclared builtin, got %d", len(interp.predeclared))
	}
}

func TestParse(t *testing.T) {
	interp := NewInterpreter(nil)

	src := `
def main():
    return "hello"
`

	ast, err := interp.Parse(src, "test.star")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if ast == nil {
		t.Fatal("expected non-nil AST")
	}
}

func TestToStarlark(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string", "hello"},
		{"int", 42},
		{"int64", int64(42)},
		{"float64", 3.14},
		{"bool", true},
		{"slice", []interface{}{"a", "b", "c"}},
		{"map", map[string]interface{}{"key": "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToStarlark(tt.input)
			if err != nil {
				t.Fatalf("ToStarlark failed: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}
}

func TestFromStarlark(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		val := starlark.String("hello")
		result, err := FromStarlark(val)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}
		if result != "hello" {
			t.Errorf("expected 'hello', got %v", result)
		}
	})

	t.Run("int", func(t *testing.T) {
		val := starlark.MakeInt(42)
		result, err := FromStarlark(val)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}
		if result != int64(42) {
			t.Errorf("expected 42, got %v", result)
		}
	})

	t.Run("float", func(t *testing.T) {
		val := starlark.Float(3.14)
		result, err := FromStarlark(val)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}
		if result != 3.14 {
			t.Errorf("expected 3.14, got %v", result)
		}
	})

	t.Run("bool", func(t *testing.T) {
		val := starlark.Bool(true)
		result, err := FromStarlark(val)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}
		if result != true {
			t.Errorf("expected true, got %v", result)
		}
	})

	t.Run("list", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			starlark.String("a"),
			starlark.String("b"),
		})

		result, err := FromStarlark(list)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}

		slice, ok := result.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{}, got %T", result)
		}

		if len(slice) != 2 {
			t.Errorf("expected 2 items, got %d", len(slice))
		}
	})

	t.Run("dict", func(t *testing.T) {
		dict := starlark.NewDict(1)
		dict.SetKey(starlark.String("key"), starlark.String("value"))

		result, err := FromStarlark(dict)
		if err != nil {
			t.Fatalf("FromStarlark failed: %v", err)
		}

		m, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map[string]interface{}, got %T", result)
		}

		if len(m) != 1 {
			t.Errorf("expected 1 item, got %d", len(m))
		}
	})
}
