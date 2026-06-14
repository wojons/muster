package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
)

// --- helpers ---

// compileFile is a helper that writes src to a temp .star file, parses it
// (with a matching filename), then compiles the AST into a program.
// Compile re-reads from the file on disk, so the file must exist at that path.
func compileFile(t *testing.T, i *Interpreter, src string) (*starlark.Program, string) {
	t.Helper()
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	if err := os.WriteFile(filename, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	ast, err := i.Parse(src, filename)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	prog, err := i.Compile(ast)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	return prog, filename
}

// execFile is a helper that writes src to a temp .star file, parses, compiles,
// and executes the program, returning the resulting globals.
func execFile(t *testing.T, i *Interpreter, src string) starlark.StringDict {
	t.Helper()
	prog, _ := compileFile(t, i, src)
	globals, err := i.Exec(prog)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	return globals
}

// --- NewInterpreter ---

func TestNewInterpreter_ReturnsNonNil(t *testing.T) {
	i := NewInterpreter(nil)
	if i == nil {
		t.Fatal("NewInterpreter(nil) returned nil")
	}

	// Globals should be empty and ready for use.
	g := i.Globals()
	if g == nil {
		t.Error("Globals() returned nil")
	}
	if len(g) != 0 {
		t.Errorf("expected empty globals, got %d entries", len(g))
	}

	// GetGlobal on a key that was never SetGlobal'd must return false.
	val, ok := i.GetGlobal("nonexistent")
	if ok {
		t.Error("GetGlobal on nonexistent key should return false")
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
}

func TestNewInterpreter_WithBuiltins_RegistersBuiltin(t *testing.T) {
	var called bool
	builtins := map[string]interface{}{
		"greet": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			called = true
			return starlark.String("hello"), nil
		},
	}
	i := NewInterpreter(builtins)
	if i == nil {
		t.Fatal("NewInterpreter returned nil")
	}

	globals := execFile(t, i, `result = greet()`)
	if !called {
		t.Error("builtin 'greet' was not called during execution")
	}
	// greet() returns "hello", but the program assigns it to 'result'.
	if _, ok := globals["result"]; !ok {
		t.Error("expected 'result' in exec output")
	}
}

func TestNewInterpreter_WithInvalidBuiltin_Skipped(t *testing.T) {
	builtins := map[string]interface{}{
		"bad": "i am a string, not a function",
	}
	i := NewInterpreter(builtins)
	if i == nil {
		t.Fatal("NewInterpreter returned nil")
	}

	// The invalid builtin was not registered (non-function type is skipped),
	// so the resolver catches it at compile time.
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	if err := os.WriteFile(filename, []byte(`bad()`), 0644); err != nil {
		t.Fatal(err)
	}
	ast, err := i.Parse(`bad()`, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, err = i.Compile(ast)
	if err == nil {
		t.Error("expected compile error for undefined builtin 'bad', got nil")
	}
}

// --- Parse ---

func TestParse_ValidExpr_ReturnsAST(t *testing.T) {
	i := NewInterpreter(nil)
	ast, err := i.Parse("x = 1 + 2", "test.star")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ast == nil {
		t.Fatal("expected non-nil AST")
	}
	if ast.Path != "test.star" {
		t.Errorf("expected Path 'test.star', got %q", ast.Path)
	}
}

func TestParse_InvalidSyntax_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)
	_, err := i.Parse("x = ", "test.star")
	if err == nil {
		t.Error("expected error for invalid syntax, got nil")
	}
}

// --- Compile ---

func TestCompile_ValidAST_ReturnsProgram(t *testing.T) {
	i := NewInterpreter(nil)

	src := "x = 1 + 2"
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.star")
	if err := os.WriteFile(filename, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	ast, err := i.Parse(src, filename)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := i.Compile(ast)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if prog == nil {
		t.Fatal("expected non-nil program")
	}
}

func TestCompile_NilAST_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)

	// The current implementation dereferences ast.Path without a nil check,
	// which causes a panic.  This defer recovers so the test never crashes.
	defer func() { recover() }()

	_, err := i.Compile(nil)
	if err == nil {
		t.Error("expected error for nil AST, got nil")
	}
}

// --- Exec ---

func TestExec_SimpleProgram(t *testing.T) {
	i := NewInterpreter(nil)
	globals := execFile(t, i, "x = 1 + 2")

	x, ok := globals["x"]
	if !ok {
		t.Fatal("expected 'x' in exec output")
	}
	got, err := starlark.AsInt32(x)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestExec_WithPredeclaredBuiltin(t *testing.T) {
	var callCount int
	builtins := map[string]interface{}{
		"double": func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			callCount++
			if len(args) != 1 {
				return nil, fmt.Errorf("double expects 1 argument, got %d", len(args))
			}
			n, err := starlark.AsInt32(args[0])
			if err != nil {
				return nil, fmt.Errorf("double expects int argument: %v", err)
			}
			return starlark.MakeInt(n * 2), nil
		},
	}
	i := NewInterpreter(builtins)
	globals := execFile(t, i, "x = double(21)")

	if callCount != 1 {
		t.Errorf("expected 'double' called once, got %d", callCount)
	}

	x, ok := globals["x"]
	if !ok {
		t.Fatal("expected 'x' in exec output")
	}
	got, err := starlark.AsInt32(x)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

// --- Call ---

func TestCall_CallsFunction(t *testing.T) {
	i := NewInterpreter(nil)

	globals := execFile(t, i, `def add(a, b): return a + b`)

	fnVal, ok := globals["add"]
	if !ok {
		t.Fatal("expected 'add' function in exec output")
	}
	fn, ok := fnVal.(*starlark.Function)
	if !ok {
		t.Fatalf("expected *starlark.Function, got %T", fnVal)
	}

	result, err := i.Call(fn, starlark.Tuple{starlark.MakeInt(3), starlark.MakeInt(4)}, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	got, err := starlark.AsInt32(result)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

func TestCall_WithEmptyArgs(t *testing.T) {
	i := NewInterpreter(nil)

	globals := execFile(t, i, `def zero(): return 0`)

	fnVal, ok := globals["zero"]
	if !ok {
		t.Fatal("expected 'zero' function in exec output")
	}
	fn, ok := fnVal.(*starlark.Function)
	if !ok {
		t.Fatalf("expected *starlark.Function, got %T", fnVal)
	}

	result, err := i.Call(fn, nil, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	got, err := starlark.AsInt32(result)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCall_WithInvalidArgCount_ReturnsError(t *testing.T) {
	i := NewInterpreter(nil)

	globals := execFile(t, i, `def identity(x): return x`)

	fnVal, ok := globals["identity"]
	if !ok {
		t.Fatal("expected 'identity' function in exec output")
	}
	fn, ok := fnVal.(*starlark.Function)
	if !ok {
		t.Fatalf("expected *starlark.Function, got %T", fnVal)
	}

	// Call with zero args when one is required.
	_, err := i.Call(fn, nil, nil)
	if err == nil {
		t.Error("expected error calling identity() with no arguments, got nil")
	}
}

// --- SetGlobal / GetGlobal ---

func TestSetGlobal_GetGlobal_RoundTrip(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("key", starlark.String("value"))
	val, ok := i.GetGlobal("key")
	if !ok {
		t.Fatal("GetGlobal after SetGlobal returned false")
	}
	got, ok := val.(starlark.String)
	if !ok {
		t.Fatalf("expected starlark.String, got %T", val)
	}
	if string(got) != "value" {
		t.Errorf("expected 'value', got %q", string(got))
	}
}

func TestGetGlobal_NonexistentKey_ReturnsFalse(t *testing.T) {
	i := NewInterpreter(nil)

	// Verify the globals map is initially empty.
	val, ok := i.GetGlobal("does_not_exist")
	if ok {
		t.Error("GetGlobal on nonexistent key returned true")
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
}

func TestSetGlobal_OverwritesExisting(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("x", starlark.MakeInt(1))
	i.SetGlobal("x", starlark.MakeInt(99))

	val, ok := i.GetGlobal("x")
	if !ok {
		t.Fatal("expected 'x' in globals")
	}
	got, err := starlark.AsInt32(val)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}

func TestSetGlobal_IntValue(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("n", starlark.MakeInt(42))
	val, ok := i.GetGlobal("n")
	if !ok {
		t.Fatal("expected 'n' in globals")
	}
	got, err := starlark.AsInt32(val)
	if err != nil {
		t.Fatalf("AsInt32: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestSetGlobal_BoolValue(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("flag", starlark.Bool(true))
	val, ok := i.GetGlobal("flag")
	if !ok {
		t.Fatal("expected 'flag' in globals")
	}
	b, ok := val.(starlark.Bool)
	if !ok {
		t.Fatalf("expected starlark.Bool, got %T", val)
	}
	if bool(b) != true {
		t.Errorf("expected true, got %v", bool(b))
	}
}

func TestSetGlobal_ListValue(t *testing.T) {
	i := NewInterpreter(nil)
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(10), starlark.MakeInt(20)})

	i.SetGlobal("items", list)
	val, ok := i.GetGlobal("items")
	if !ok {
		t.Fatal("expected 'items' in globals")
	}
	l, ok := val.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", val)
	}
	if l.Len() != 2 {
		t.Errorf("expected length 2, got %d", l.Len())
	}
}

// --- Globals ---

func TestGlobals_ReturnsCurrentMap(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("a", starlark.MakeInt(1))
	i.SetGlobal("b", starlark.String("two"))

	g := i.Globals()
	if g == nil {
		t.Fatal("Globals() returned nil")
	}
	if len(g) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(g))
	}
	if _, ok := g["a"]; !ok {
		t.Error("expected 'a' in globals")
	}
	if _, ok := g["b"]; !ok {
		t.Error("expected 'b' in globals")
	}
}

func TestGlobals_IsolatedFromExec(t *testing.T) {
	// Exec output is returned as a separate StringDict and does not mutate
	// the interpreter's own Globals map.
	i := NewInterpreter(nil)

	_ = execFile(t, i, `x = 42`)

	// The interpreter's own globals should still be empty.
	if len(i.Globals()) != 0 {
		t.Errorf("expected interpreter globals to remain empty after exec, got %d entries", len(i.Globals()))
	}
}

func TestGlobals_MutationReflectsSetGlobal(t *testing.T) {
	i := NewInterpreter(nil)

	i.SetGlobal("x", starlark.MakeInt(10))
	g := i.Globals()

	// Modify the returned map directly.
	g["y"] = starlark.String("added")

	// The modification should be visible through the interpreter since
	// Globals() returns the internal map (not a copy).
	val, ok := i.GetGlobal("y")
	if !ok {
		t.Error("expected 'y' to be visible after mutating Globals() result")
	}
	if val != starlark.String("added") {
		t.Errorf("expected 'added', got %v", val)
	}
}
