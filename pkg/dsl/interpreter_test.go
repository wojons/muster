package dsl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// writeTempStarlark writes source to a temp file and returns the path.
func writeTempStarlark(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.star")
	err := os.WriteFile(path, []byte(src), 0644)
	require.NoError(t, err)
	return path
}

// TestNewInterpreter_Empty creates an interpreter with no builtins.
func TestNewInterpreter_Empty(t *testing.T) {
	i := NewInterpreter(nil)
	require.NotNil(t, i)
	require.NotNil(t, i.globals)
	require.NotNil(t, i.predeclared)
	require.Empty(t, i.globals)
	require.Empty(t, i.predeclared)
}

// TestNewInterpreter_WithBuiltins creates an interpreter with builtin functions.
func TestNewInterpreter_WithBuiltins(t *testing.T) {
	builtins := map[string]interface{}{
		"double": func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var x starlark.Int
			if err := starlark.UnpackArgs("double", args, kwargs, "x", &x); err != nil {
				return nil, err
			}
			i, _ := x.Int64()
			return starlark.MakeInt64(i * 2), nil
		},
	}
	i := NewInterpreter(builtins)
	require.NotNil(t, i)
	require.Len(t, i.predeclared, 1)
	v, ok := i.predeclared["double"]
	require.True(t, ok)
	require.NotNil(t, v)
}

// TestInterpreter_Parse_Valid parses valid Starlark source.
func TestInterpreter_Parse_Valid(t *testing.T) {
	i := NewInterpreter(nil)
	ast, err := i.Parse(`x = 1 + 2`, "test.star")
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Equal(t, "test.star", ast.Path)
}

// TestInterpreter_Parse_Function parses a function definition.
func TestInterpreter_Parse_Function(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "def add(a, b):\n    return a + b\n")
	// Compile requires the file on disk — use the temp path as filename
	ast, err := i.Parse("", path)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

// TestInterpreter_Parse_Invalid returns an error for invalid source.
func TestInterpreter_Parse_Invalid(t *testing.T) {
	i := NewInterpreter(nil)
	ast, err := i.Parse(`!@#$`, "invalid.star")
	require.Error(t, err)
	require.Nil(t, ast)
}

// TestInterpreter_Compile_Valid compiles a parsed AST to a program.
// Compile reads source from disk via ast.Path, so we use a temp file.
func TestInterpreter_Compile_Valid(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "x = 42\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)
	require.NotNil(t, prog)
}

// TestInterpreter_Compile_NilAST returns an error (or panics from nil deref).
// Verified: the current Compile panics on nil AST because it calls ast.Path.
// This is acceptable behavior for a precondition violation.
func TestInterpreter_Compile_NilAST(t *testing.T) {
	i := NewInterpreter(nil)
	require.Panics(t, func() {
		_, _ = i.Compile(nil)
	})
}

// TestInterpreter_Exec runs a compiled program and returns globals.
func TestInterpreter_Exec(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "x = 100\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	globals, err := i.Exec(prog)
	require.NoError(t, err)
	require.NotNil(t, globals)
	require.Contains(t, globals, "x")
}

// TestInterpreter_Exec_Print exercises the print handler.
func TestInterpreter_Exec_Print(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "print(\"hello\")\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	_, err = i.Exec(prog)
	require.NoError(t, err)
}

// TestInterpreter_Call calls a Starlark function with positional args.
func TestInterpreter_Call(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "def greet(name):\n    return \"Hello, \" + name\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	globals, err := i.Exec(prog)
	require.NoError(t, err)

	fn, ok := globals["greet"].(*starlark.Function)
	require.True(t, ok)

	result, err := i.Call(fn, starlark.Tuple{starlark.String("World")}, nil)
	require.NoError(t, err)
	require.Equal(t, starlark.String("Hello, World"), result)
}

// TestInterpreter_Call_WithBuiltin calls a builtin function through the interpreter.
func TestInterpreter_Call_WithBuiltin(t *testing.T) {
	builtins := map[string]interface{}{
		"triple": func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var x starlark.Int
			if err := starlark.UnpackArgs("triple", args, kwargs, "x", &x); err != nil {
				return nil, err
			}
			i, _ := x.Int64()
			return starlark.MakeInt64(i * 3), nil
		},
	}
	i := NewInterpreter(builtins)

	path := writeTempStarlark(t, "result = triple(x=7)\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	globals, err := i.Exec(prog)
	require.NoError(t, err)

	result, ok := globals["result"]
	require.True(t, ok)
	require.Equal(t, starlark.MakeInt64(21), result)
}

// TestInterpreter_SetGlobal sets a global value and verifies it.
func TestInterpreter_SetGlobal(t *testing.T) {
	i := NewInterpreter(nil)
	i.SetGlobal("answer", starlark.MakeInt64(42))

	val, ok := i.GetGlobal("answer")
	require.True(t, ok)
	require.Equal(t, starlark.MakeInt64(42), val)
}

// TestInterpreter_GetGlobal_Missing returns false for nonexistent keys.
func TestInterpreter_GetGlobal_Missing(t *testing.T) {
	i := NewInterpreter(nil)
	val, ok := i.GetGlobal("nonexistent")
	require.False(t, ok)
	require.Nil(t, val)
}

// TestInterpreter_GetGlobal_AfterExec shows that exec globals live in the program,
// not in the interpreter's explicit globals map.
func TestInterpreter_GetGlobal_AfterExec(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "pi = 3.14159\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	_, err = i.Exec(prog)
	require.NoError(t, err)

	// SetGlobal was not called — pi lives in the program's globals, not interpreter.
	val, ok := i.GetGlobal("pi")
	require.False(t, ok)
	require.Nil(t, val)
}

// TestInterpreter_Globals returns the globals dict.
func TestInterpreter_Globals(t *testing.T) {
	i := NewInterpreter(nil)
	g := i.Globals()
	require.NotNil(t, g)
	require.Empty(t, g)

	i.SetGlobal("version", starlark.String("1.0.0"))
	g = i.Globals()
	require.Len(t, g, 1)
	require.Contains(t, g, "version")
}

// TestInterpreter_EndToEnd exercises the full parse → compile → exec pipeline.
func TestInterpreter_EndToEnd(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, `def factorial(n):
    result = 1
    for i in range(1, n + 1):
        result = result * i
    return result

result = factorial(5)
`)

	// Parse
	ast, err := i.Parse("", path)
	require.NoError(t, err)

	// Compile
	prog, err := i.Compile(ast)
	require.NoError(t, err)

	// Exec
	globals, err := i.Exec(prog)
	require.NoError(t, err)

	// Verify result
	result, ok := globals["result"]
	require.True(t, ok)
	iVal, ok := result.(starlark.Int)
	require.True(t, ok)
	n, _ := iVal.Int64()
	require.Equal(t, int64(120), n)
}

// TestInterpreter_MultipleFiles compiles and executes different scripts.
func TestInterpreter_MultipleFiles(t *testing.T) {
	i := NewInterpreter(nil)

	tests := []struct {
		name   string
		src    string
		key    string
		expect starlark.Value
	}{
		{"integer", "x = 1\n", "x", starlark.MakeInt64(1)},
		{"string", "s = \"hello\"\n", "s", starlark.String("hello")},
		{"boolean", "t = True\n", "t", starlark.True},
		{"list", "lst = [1, 2, 3]\n", "lst", nil}, // check existence, not deep equality
		{"none", "n = None\n", "n", starlark.None},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempStarlark(t, tt.src)
			ast, err := i.Parse("", path)
			require.NoError(t, err)

			prog, err := i.Compile(ast)
			require.NoError(t, err)

			globals, err := i.Exec(prog)
			require.NoError(t, err)

			val, ok := globals[tt.key]
			require.True(t, ok)

			if tt.expect != nil {
				require.Equal(t, tt.expect, val)
			}
		})
	}
}

// TestInterpreter_Exec_Error propagates runtime errors.
func TestInterpreter_Exec_Error(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "x = 1 / 0\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	_, err = i.Exec(prog)
	require.Error(t, err)
}

// TestInterpreter_Call_Kwargs calls a function with keyword arguments.
func TestInterpreter_Call_Kwargs(t *testing.T) {
	i := NewInterpreter(nil)
	path := writeTempStarlark(t, "def describe(name, age):\n    return name + \" is \" + str(age)\n")

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	globals, err := i.Exec(prog)
	require.NoError(t, err)

	fn, ok := globals["describe"].(*starlark.Function)
	require.True(t, ok)

	result, err := i.Call(fn, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("Alice")},
		{starlark.String("age"), starlark.MakeInt64(30)},
	})
	require.NoError(t, err)
	require.Equal(t, starlark.String("Alice is 30"), result)
}

// TestInterpreter_SetGlobal_Multiple sets and retrieves many globals.
func TestInterpreter_SetGlobal_Multiple(t *testing.T) {
	i := NewInterpreter(nil)
	i.SetGlobal("a", starlark.MakeInt64(1))
	i.SetGlobal("b", starlark.MakeInt64(2))
	i.SetGlobal("c", starlark.MakeInt64(3))

	g := i.Globals()
	require.Len(t, g, 3)

	a, ok := i.GetGlobal("a")
	require.True(t, ok)
	require.Equal(t, starlark.MakeInt64(1), a)
}

// TestInterpreter_SetGlobal_Overwrite overwrites an existing global.
func TestInterpreter_SetGlobal_Overwrite(t *testing.T) {
	i := NewInterpreter(nil)
	i.SetGlobal("key", starlark.String("old"))
	i.SetGlobal("key", starlark.String("new"))

	val, ok := i.GetGlobal("key")
	require.True(t, ok)
	require.Equal(t, starlark.String("new"), val)
}

// TestInterpreter_NewInterpreter_NilMap treats nil builtins as empty.
func TestInterpreter_NewInterpreter_NilMap(t *testing.T) {
	i := NewInterpreter(nil)
	require.NotNil(t, i)
	require.NotNil(t, i.predeclared)
	require.Empty(t, i.predeclared)
}

// TestNewInterpreterWithPrint_CustomSink verifies that a custom print sink
// receives print() output instead of the structured logger.
func TestNewInterpreterWithPrint_CustomSink(t *testing.T) {
	var captured []string
	printFn := func(_ *starlark.Thread, msg string) {
		captured = append(captured, msg)
	}
	i := NewInterpreterWithPrint(nil, printFn)
	require.NotNil(t, i)

	path := writeTempStarlark(t, `print("hello from custom sink")
print("second line")
`)

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	_, err = i.Exec(prog)
	require.NoError(t, err)

	require.Len(t, captured, 2)
	require.Equal(t, "hello from custom sink", captured[0])
	require.Equal(t, "second line", captured[1])
}

// TestSetPrintFunc verifies that SetPrintFunc changes the print sink at runtime.
func TestSetPrintFunc(t *testing.T) {
	var captured []string
	i := NewInterpreter(nil)
	i.SetPrintFunc(func(_ *starlark.Thread, msg string) {
		captured = append(captured, msg)
	})

	path := writeTempStarlark(t, `print("after set")
`)

	ast, err := i.Parse("", path)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)

	_, err = i.Exec(prog)
	require.NoError(t, err)

	require.Len(t, captured, 1)
	require.Equal(t, "after set", captured[0])
}
