package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// --- helpers ---

// compileFile writes src to a temp .star file, parses it with a matching
// filename, then compiles the AST into a program.
func compileFile(t *testing.T, i *Interpreter, src string) (*starlark.Program, string) {
	t.Helper()
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	err := os.WriteFile(filename, []byte(src), 0644)
	require.NoError(t, err, "failed to write temp file")

	ast, err := i.Parse(src, filename)
	require.NoError(t, err, "Parse failed")

	prog, err := i.Compile(ast)
	require.NoError(t, err, "Compile failed")

	return prog, filename
}

// execFile writes src to a temp .star file, parses, compiles, and executes
// the program, returning the resulting globals.
func execFile(t *testing.T, i *Interpreter, src string) starlark.StringDict {
	t.Helper()
	prog, _ := compileFile(t, i, src)
	globals, err := i.Exec(prog)
	require.NoError(t, err, "Exec failed")
	return globals
}

// --- NewInterpreter ---

func TestNewInterpreter_NilBuiltins(t *testing.T) {
	i := NewInterpreter(nil)
	require.NotNil(t, i, "NewInterpreter(nil) returned nil")

	g := i.Globals()
	require.NotNil(t, g, "Globals() returned nil after nil builtins")
	require.Empty(t, g, "expected empty globals after nil builtins")

	val, ok := i.GetGlobal("nonexistent")
	require.False(t, ok, "GetGlobal on nonexistent key should return false")
	require.Nil(t, val, "expected nil value for nonexistent key")
}

func TestNewInterpreter_EmptyBuiltins(t *testing.T) {
	i := NewInterpreter(map[string]interface{}{})
	require.NotNil(t, i, "NewInterpreter(empty) returned nil")

	g := i.Globals()
	require.Empty(t, g, "expected empty globals after empty builtins")
}

func TestNewInterpreter_WithBuiltin_RegistersAndExecutes(t *testing.T) {
	var called bool
	builtins := map[string]interface{}{
		"noop": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			called = true
			return starlark.None, nil
		},
	}
	i := NewInterpreter(builtins)
	require.NotNil(t, i)

	globals := execFile(t, i, `result = noop()`)
	require.True(t, called, "builtin 'noop' was not called during execution")

	result, ok := globals["result"]
	require.True(t, ok, "expected 'result' in exec output")
	require.Equal(t, starlark.None, result)
}

func TestNewInterpreter_WithMultipleBuiltins(t *testing.T) {
	builtins := map[string]interface{}{
		"greet": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("hello"), nil
		},
		"double": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			n, _ := starlark.AsInt32(args[0])
			return starlark.MakeInt(n * 2), nil
		},
	}
	i := NewInterpreter(builtins)
	require.NotNil(t, i)

	globals := execFile(t, i, `
greeting = greet()
doubled = double(21)
`)
	greeting, ok := globals["greeting"]
	require.True(t, ok)
	require.Equal(t, starlark.String("hello"), greeting)

	doubled, ok := globals["doubled"]
	require.True(t, ok)
	d, err := starlark.AsInt32(doubled)
	require.NoError(t, err)
	require.Equal(t, 42, d)
}

func TestNewInterpreter_WithInvalidBuiltinType_Skipped(t *testing.T) {
	builtins := map[string]interface{}{
		"bad": "i am a string, not a function",
	}
	i := NewInterpreter(builtins)
	require.NotNil(t, i)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	err := os.WriteFile(filename, []byte(`bad()`), 0644)
	require.NoError(t, err)

	ast, err := i.Parse(`bad()`, filename)
	require.NoError(t, err, "Parse should succeed even for unknown builtin")

	_, err = i.Compile(ast)
	require.Error(t, err, "expected compile error for undefined builtin 'bad'")
}

// --- Parse ---

func TestParse_ValidAssignment(t *testing.T) {
	i := NewInterpreter(nil)
	ast, err := i.Parse("x = 1 + 2", "test.star")
	require.NoError(t, err, "Parse of valid Starlark should succeed")
	require.NotNil(t, ast, "AST should not be nil")
	require.Equal(t, "test.star", ast.Path)
}

func TestParse_ValidFunctionDef(t *testing.T) {
	i := NewInterpreter(nil)
	src := `def add(a, b):
    return a + b`
	ast, err := i.Parse(src, "math.star")
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Equal(t, "math.star", ast.Path)
}

func TestParse_InvalidSyntax_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)
	_, err := i.Parse("x = ", "test.star")
	require.Error(t, err, "expected error for incomplete assignment")
}

func TestParse_InvalidSyntax_MalformedDef(t *testing.T) {
	i := NewInterpreter(nil)
	_, err := i.Parse("def ():", "test.star")
	require.Error(t, err, "expected error for malformed function definition")
}

func TestParse_EmptySource(t *testing.T) {
	i := NewInterpreter(nil)
	ast, err := i.Parse("", "empty.star")
	require.NoError(t, err, "empty source is valid Starlark")
	require.NotNil(t, ast)
}

func TestParse_WithComments(t *testing.T) {
	i := NewInterpreter(nil)
	src := `# this is a comment
x = 1  # inline comment`
	ast, err := i.Parse(src, "commented.star")
	require.NoError(t, err)
	require.NotNil(t, ast)
}

// --- Compile ---

func TestCompile_ValidAST_ReturnsProgram(t *testing.T) {
	i := NewInterpreter(nil)

	src := "x = 1 + 2"
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	err := os.WriteFile(filename, []byte(src), 0644)
	require.NoError(t, err)

	ast, err := i.Parse(src, filename)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err, "Compile of valid AST should succeed")
	require.NotNil(t, prog, "Program should not be nil")
}

func TestCompile_WithBuiltinDependency(t *testing.T) {
	builtins := map[string]interface{}{
		"version": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("v1.0"), nil
		},
	}
	i := NewInterpreter(builtins)

	src := "v = version()"
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	err := os.WriteFile(filename, []byte(src), 0644)
	require.NoError(t, err)

	ast, err := i.Parse(src, filename)
	require.NoError(t, err)

	prog, err := i.Compile(ast)
	require.NoError(t, err)
	require.NotNil(t, prog)
}

func TestCompile_UndefinedFunction_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)

	src := "undefined_func()"
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	err := os.WriteFile(filename, []byte(src), 0644)
	require.NoError(t, err)

	ast, err := i.Parse(src, filename)
	require.NoError(t, err)

	_, err = i.Compile(ast)
	require.Error(t, err, "expected error for undefined function call")
}

func TestCompile_NilAST_Panics(t *testing.T) {
	i := NewInterpreter(nil)

	require.Panics(t, func() {
		i.Compile(nil)
	}, "Compile(nil) should panic due to nil pointer dereference")
}

// --- Exec ---

func TestExec_SimpleAssignment(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, "x = 1 + 2")

	x, ok := globals["x"]
	require.True(t, ok, "expected 'x' in exec output")
	n, err := starlark.AsInt32(x)
	require.NoError(t, err)
	require.Equal(t, 3, n)
}

func TestExec_StringAssignment(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `name = "hello" + " world"`)

	name, ok := globals["name"]
	require.True(t, ok, "expected 'name' in exec output")
	require.Equal(t, starlark.String("hello world"), name)
}

func TestExec_ListLiteral(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, "items = [1, 2, 3]")

	items, ok := globals["items"]
	require.True(t, ok)
	l, ok := items.(*starlark.List)
	require.True(t, ok, "expected *starlark.List")
	require.Equal(t, 3, l.Len())
}

func TestExec_WithPredeclaredBuiltin(t *testing.T) {
	var callCount int
	builtins := map[string]interface{}{
		"double": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			callCount++
			return starlark.MakeInt(42), nil
		},
	}
	i := NewInterpreter(builtins)
	globals := execFile(t, i, "x = double(21)")

	require.Equal(t, 1, callCount, "expected 'double' called once")

	x, ok := globals["x"]
	require.True(t, ok)
	n, err := starlark.AsInt32(x)
	require.NoError(t, err)
	require.Equal(t, 42, n)
}

func TestExec_RuntimeError(t *testing.T) {
	i := NewInterpreter(nil)

	prog, _ := compileFile(t, i, "x = 1 // 0")
	_, err := i.Exec(prog)
	require.Error(t, err, "expected runtime error for division by zero")
}

func TestExec_EmptyProgram(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, "")
	require.Empty(t, globals, "expected empty globals from empty program")
}

// --- Call ---

func TestCall_SimpleFunction(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `def add(a, b): return a + b`)

	fnVal, ok := globals["add"]
	require.True(t, ok, "expected 'add' function in exec output")
	fn, ok := fnVal.(*starlark.Function)
	require.True(t, ok, "expected *starlark.Function")

	result, err := i.Call(fn, starlark.Tuple{starlark.MakeInt(3), starlark.MakeInt(4)}, nil)
	require.NoError(t, err, "Call(3,4) should succeed")
	n, err := starlark.AsInt32(result)
	require.NoError(t, err)
	require.Equal(t, 7, n)
}

func TestCall_WithNoArgs(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `def zero(): return 0`)

	fnVal, ok := globals["zero"]
	require.True(t, ok)
	fn, ok := fnVal.(*starlark.Function)
	require.True(t, ok)

	result, err := i.Call(fn, nil, nil)
	require.NoError(t, err)
	n, err := starlark.AsInt32(result)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestCall_WithKwargs(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `def repeat(msg, count=1): return msg * count`)

	fnVal, ok := globals["repeat"]
	require.True(t, ok)
	fn, ok := fnVal.(*starlark.Function)
	require.True(t, ok)

	result, err := i.Call(fn,
		starlark.Tuple{starlark.String("ha")},
		[]starlark.Tuple{{starlark.String("count"), starlark.MakeInt(3)}},
	)
	require.NoError(t, err)
	require.Equal(t, starlark.String("hahaha"), result)
}

func TestCall_InvalidArgCount_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `def identity(x): return x`)

	fnVal, ok := globals["identity"]
	require.True(t, ok)
	fn, ok := fnVal.(*starlark.Function)
	require.True(t, ok)

	_, err := i.Call(fn, nil, nil)
	require.Error(t, err, "expected error calling identity() with no arguments")
}

func TestCall_WrongArgType_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `def upper(s): return s.upper()`)

	fnVal, ok := globals["upper"]
	require.True(t, ok)
	fn, ok := fnVal.(*starlark.Function)
	require.True(t, ok)

	_, err := i.Call(fn, starlark.Tuple{starlark.MakeInt(42)}, nil)
	require.Error(t, err, "expected error calling upper(42)")
}

// --- SetGlobal / GetGlobal ---

func TestSetGlobal_GetGlobal_RoundTrip(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("key", starlark.String("value"))
	val, ok := i.GetGlobal("key")
	require.True(t, ok, "GetGlobal after SetGlobal should return true")
	require.Equal(t, starlark.String("value"), val)
}

func TestSetGlobal_GetGlobal_IntValue(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("n", starlark.MakeInt(42))
	val, ok := i.GetGlobal("n")
	require.True(t, ok)
	n, err := starlark.AsInt32(val)
	require.NoError(t, err)
	require.Equal(t, 42, n)
}

func TestSetGlobal_GetGlobal_BoolValue(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("flag", starlark.Bool(true))
	val, ok := i.GetGlobal("flag")
	require.True(t, ok)
	b, ok := val.(starlark.Bool)
	require.True(t, ok, "expected starlark.Bool")
	require.Equal(t, starlark.Bool(true), b)
}

func TestSetGlobal_GetGlobal_ListValue(t *testing.T) {
	i := NewInterpreter(nil)
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(10), starlark.MakeInt(20)})

	i.SetGlobal("items", list)
	val, ok := i.GetGlobal("items")
	require.True(t, ok)
	l, ok := val.(*starlark.List)
	require.True(t, ok, "expected *starlark.List")
	require.Equal(t, 2, l.Len())
}

func TestSetGlobal_GetGlobal_NoneValue(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("nothing", starlark.None)
	val, ok := i.GetGlobal("nothing")
	require.True(t, ok)
	require.Equal(t, starlark.None, val)
}

func TestGetGlobal_NonexistentKey_ReturnsFalse(t *testing.T) {
	i := NewInterpreter(nil)
	val, ok := i.GetGlobal("does_not_exist")
	require.False(t, ok, "GetGlobal on nonexistent key should return false")
	require.Nil(t, val, "expected nil for nonexistent key")
}

func TestSetGlobal_OverwritesExisting(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("x", starlark.MakeInt(1))
	i.SetGlobal("x", starlark.MakeInt(99))

	val, ok := i.GetGlobal("x")
	require.True(t, ok)
	n, err := starlark.AsInt32(val)
	require.NoError(t, err)
	require.Equal(t, 99, n)
}

func TestSetGlobal_OverwritesWithDifferentType(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("x", starlark.MakeInt(1))
	i.SetGlobal("x", starlark.String("now a string"))

	val, ok := i.GetGlobal("x")
	require.True(t, ok)
	require.Equal(t, starlark.String("now a string"), val)
}

func TestSetGlobal_MultipleKeys(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("a", starlark.MakeInt(1))
	i.SetGlobal("b", starlark.MakeInt(2))
	i.SetGlobal("c", starlark.MakeInt(3))

	require.Len(t, i.Globals(), 3)
}

// --- Globals ---

func TestGlobals_ReturnsCurrentMap(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("a", starlark.MakeInt(1))
	i.SetGlobal("b", starlark.String("two"))

	g := i.Globals()
	require.NotNil(t, g)
	require.Len(t, g, 2)
	require.Contains(t, g, "a")
	require.Contains(t, g, "b")
}

func TestGlobals_IsolatedFromExec(t *testing.T) {
	i := NewInterpreter(nil)

	_ = execFile(t, i, `x = 42`)

	require.Len(t, i.Globals(), 0, "interpreter globals should remain empty after exec")
}

func TestGlobals_MutationReflectsSetGlobal(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("x", starlark.MakeInt(10))
	g := i.Globals()

	g["y"] = starlark.String("added")

	val, ok := i.GetGlobal("y")
	require.True(t, ok, "'y' should be visible after mutating Globals() result")
	require.Equal(t, starlark.String("added"), val)
}

func TestGlobals_EmptyAfterInit(t *testing.T) {
	i := NewInterpreter(nil)
	require.Empty(t, i.Globals())
}

func TestGlobals_NotNilAfterInit(t *testing.T) {
	i := NewInterpreter(nil)
	require.NotNil(t, i.Globals())
}

// --- Integration: Parse -> Compile -> Exec pipeline ---

func TestPipeline_MultipleExecCallsIndependent(t *testing.T) {
	i := NewInterpreter(nil)

	globals1 := execFile(t, i, `a = 10`)
	globals2 := execFile(t, i, `b = 20`)

	require.Contains(t, globals1, "a")
	require.NotContains(t, globals1, "b")
	require.Contains(t, globals2, "b")
	require.NotContains(t, globals2, "a")
}

func TestPipeline_ExecWithSetGlobalIsSeparate(t *testing.T) {
	i := NewInterpreter(nil)
	i.SetGlobal("config", starlark.String("production"))

	globals := execFile(t, i, `x = 1`)
	_, ok := globals["config"]
	require.False(t, ok, "'config' from SetGlobal should not leak into exec scope")

	val, ok := i.GetGlobal("config")
	require.True(t, ok)
	require.Equal(t, starlark.String("production"), val)
}

func TestNewInterpreter_BuiltinReturnsError_Propagates(t *testing.T) {
	builtins := map[string]interface{}{
		"fail": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return nil, fmt.Errorf("intentional failure")
		},
	}
	i := NewInterpreter(builtins)

	prog, _ := compileFile(t, i, `result = fail()`)
	_, err := i.Exec(prog)
	require.Error(t, err, "expected error from builtin that returns an error")
	require.Contains(t, err.Error(), "intentional failure")
}

func TestParse_UnicodeSource(t *testing.T) {
	i := NewInterpreter(nil)
	src := `greeting = "héllo wörld"`
	ast, err := i.Parse(src, "unicode.star")
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestCompile_ExecRoundTripWithUnicode(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, `msg = "héllo"`)

	msg, ok := globals["msg"]
	require.True(t, ok)
	require.Equal(t, starlark.String("héllo"), msg)
}
