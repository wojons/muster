package dsl

import (
	"github.com/wojons/muster/pkg/log"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type Interpreter struct {
	globals     starlark.StringDict
	predeclared starlark.StringDict
	// printFn is the Starlark thread Print handler. If nil, Exec defaults to
	// routing print() output to the structured logger (Info level).
	printFn func(thread *starlark.Thread, msg string)
}

// PrintFunc is the signature of a Starlark thread Print handler.
type PrintFunc func(thread *starlark.Thread, msg string)

// NewInterpreter creates an interpreter whose print() output is routed to the
// structured logger (Info level) — the historical default.
func NewInterpreter(builtins map[string]interface{}) *Interpreter {
	return NewInterpreterWithPrint(builtins, nil)
}

// NewInterpreterWithPrint creates an interpreter with a custom print sink.
// Pass nil for printFn to use the default structured-log sink (same as
// NewInterpreter). Pass a function that writes to os.Stdout for CLI/script
// mode where print() should appear as plain stdout output.
func NewInterpreterWithPrint(builtins map[string]interface{}, printFn PrintFunc) *Interpreter {
	i := &Interpreter{
		globals:     make(starlark.StringDict),
		predeclared: make(starlark.StringDict),
		printFn:     printFn,
	}

	for name, fn := range builtins {
		if fn, ok := fn.(func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)); ok {
			i.predeclared[name] = starlark.NewBuiltin(name, fn)
		}
	}

	return i
}

// SetPrintFunc sets the print sink used by Exec. Pass nil to restore the
// default structured-log sink.
func (i *Interpreter) SetPrintFunc(fn PrintFunc) {
	i.printFn = fn
}

func (i *Interpreter) Parse(src string, filename string) (*syntax.File, error) {
	return syntax.Parse(filename, src, 0)
}

func (i *Interpreter) Compile(ast *syntax.File) (*starlark.Program, error) {
	_, program, err := starlark.SourceProgram(
		ast.Path,
		nil,
		i.predeclared.Has,
	)
	return program, err
}

func (i *Interpreter) Exec(program *starlark.Program) (starlark.StringDict, error) {
	thread := &starlark.Thread{
		Name:  "exec",
		Print: i.printFunc(),
	}

	return program.Init(thread, i.predeclared)
}

// printFunc returns the Print handler for a Starlark thread, defaulting to the
// structured-log sink when no custom print function is set.
func (i *Interpreter) printFunc() func(*starlark.Thread, string) {
	if i.printFn != nil {
		return i.printFn
	}
	return func(_ *starlark.Thread, msg string) { log.Global().Info().Str("thread", "exec").Msg(msg) }
}

func (i *Interpreter) Call(fn *starlark.Function, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	thread := &starlark.Thread{Name: "call"}
	return starlark.Call(thread, fn, args, kwargs)
}

func (i *Interpreter) SetGlobal(name string, value starlark.Value) {
	i.globals[name] = value
}

func (i *Interpreter) GetGlobal(name string) (starlark.Value, bool) {
	val, ok := i.globals[name]
	return val, ok
}

func (i *Interpreter) Globals() starlark.StringDict {
	return i.globals
}
