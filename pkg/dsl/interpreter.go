package dsl

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type Interpreter struct {
	globals     starlark.StringDict
	predeclared starlark.StringDict
}

func NewInterpreter(builtins map[string]interface{}) *Interpreter {
	i := &Interpreter{
		globals:     make(starlark.StringDict),
		predeclared: make(starlark.StringDict),
	}

	for name, fn := range builtins {
		if fn, ok := fn.(func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)); ok {
			i.predeclared[name] = starlark.NewBuiltin(name, fn)
		}
	}

	return i
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
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	return program.Init(thread, i.predeclared)
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
