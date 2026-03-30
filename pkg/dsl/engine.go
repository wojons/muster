package dsl

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/log"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type Engine struct {
	interpreter *Interpreter
	builtins    map[string]interface{}
	config      *EngineConfig
}

func NewEngine(config *EngineConfig) *Engine {
	if config == nil {
		config = NewDefaultEngineConfig()
	}

	return &Engine{
		interpreter: nil,
		builtins:    DefaultBuiltins,
		config:      config,
	}
}

func (e *Engine) LoadScript(path string) (*Script, error) {
	content, err := ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}

	return e.LoadScriptFromString(string(content), path)
}

func (e *Engine) LoadScriptFromString(content, filename string) (*Script, error) {
	metadata := e.parseMetadata(content)

	ast, err := e.getInterpreter().Parse(content, filename)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return &Script{
		Name:     metadata.Name,
		Version:  metadata.Version,
		Metadata: metadata,
		AST:      ast,
		Source:   content,
	}, nil
}

func (e *Engine) Run(script *Script, inputs map[string]interface{}) (*ScriptResult, error) {
	if err := e.validateInputs(script, inputs); err != nil {
		return nil, err
	}

	ctx := &ExecutionContext{
		Inputs:    inputs,
		Config:    e.config,
		Secrets:   make(map[string]string),
		Variables: make(map[string]interface{}),
		Env:       make(map[string]string),
	}

	start := time.Now()
	result, err := e.execute(context.Background(), script, ctx)
	duration := time.Since(start)

	if err != nil {
		return &ScriptResult{
			Output:   nil,
			ExitCode: 1,
			Error:    err.Error(),
			Duration: duration.String(),
		}, nil
	}

	goResult, err := FromStarlark(result)
	if err != nil {
		return &ScriptResult{
			Output:   result,
			ExitCode: 0,
			Error:    err.Error(),
			Duration: duration.String(),
		}, nil
	}

	return &ScriptResult{
		Output:   goResult,
		ExitCode: 0,
		Duration: duration.String(),
	}, nil
}

func (e *Engine) execute(ctx context.Context, script *Script, execCtx *ExecutionContext) (starlark.Value, error) {
	program, err := e.getInterpreter().Compile(script.AST)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	thread := &starlark.Thread{
		Name:  script.Name,
		Print: func(_ *starlark.Thread, msg string) { log.Global().Info().Str("script", script.Name).Msg(msg) },
	}

	execResult, err := program.Init(thread, e.getInterpreter().predeclared)
	if err != nil {
		return nil, e.wrapError(err, script)
	}

	mainFn, ok := execResult["main"]
	if !ok {
		return nil, errors.New("no main function defined")
	}

	ctxStruct := starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			"config":  starlark.None,
			"inputs":  starlark.None,
			"secrets": starlark.None,
		},
	)

	return starlark.Call(thread, mainFn, starlark.Tuple{ctxStruct}, nil)
}

func (e *Engine) getInterpreter() *Interpreter {
	if e.interpreter == nil {
		e.interpreter = NewInterpreter(e.builtins)
	}
	return e.interpreter
}

func (e *Engine) validateInputs(script *Script, inputs map[string]interface{}) error {
	if script.Metadata == nil {
		return nil
	}

	for _, input := range script.Metadata.Inputs {
		if input.Required && inputs[input.Name] == nil {
			return fmt.Errorf("required input '%s' is missing", input.Name)
		}
	}

	return nil
}

func (e *Engine) parseMetadata(content string) *ScriptMetadata {
	metadata := &ScriptMetadata{
		Name:        "script",
		Description: "",
		Version:     "1.0.0",
	}

	lines := strings.Split(content, "\n")
	inDocstring := false
	docstringLines := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, `"""`) {
			inDocstring = !inDocstring
			continue
		}

		if inDocstring {
			if strings.Contains(trimmed, ":") {
				docstringLines = append(docstringLines, trimmed)
			}
		}
	}

	for _, docLine := range docstringLines {
		parts := strings.SplitN(docLine, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			key = strings.Trim(key, `"'`)
			value = strings.Trim(value, `"`)

			switch key {
			case "name":
				metadata.Name = value
			case "description":
				metadata.Description = value
			case "version":
				metadata.Version = value
			case "author":
				metadata.Author = value
			}
		}
	}

	return metadata
}

func (e *Engine) wrapError(err error, script *Script) *DSLError {
	if evalErr, ok := err.(*starlark.EvalError); ok {
		return &DSLError{
			Type:    ErrorRuntime,
			Message: evalErr.Msg,
		}
	}

	return &DSLError{
		Type:    ErrorRuntime,
		Message: err.Error(),
	}
}

func (e *Engine) enforceMemoryLimit() error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > uint64(e.config.MaxMemoryMB)*1024*1024 {
		return errors.New("memory limit exceeded")
	}

	return nil
}

func ReadFile(path string) (string, error) {
	lines := []string{"# This is a mock file"}
	return strings.Join(lines, "\n"), nil
}
