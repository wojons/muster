# DSL Engine Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: DSL Engine

## Overview

Complete specification of the **DSL execution engine**, managing script lifecycle, resource limits, and integration with host system.

## 1. Engine Architecture

```go
type DSLEngine struct {
    interpreter *StarlarkInterpreter
    builtins    map[string]interface{}
    config      *EngineConfig
    executor    *CommandExecutor
}

type EngineConfig struct {
    MaxSteps        int64
    MaxExecutionTime time.Duration
    MaxMemoryMB     int
    EnableNetwork   bool
    AllowedHosts    []string
}
```

## 2. Script Loading

```go
type ScriptMetadata struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Version     string   `json:"version"`
    Author      string   `json:"author"`
    Inputs      []Input  `json:"inputs"`
    Outputs     []Output `json:"outputs"`
}

type Input struct {
    Name        string `json:"name"`
    Type        string `json:"type"`
    Required    bool   `json:"required"`
    Default     interface{} `json:"default,omitempty"`
    Description string `json:"description"`
}

func (e *DSLEngine) LoadScript(path string) (*Script, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read script: %w", err)
    }
    
    return e.LoadScriptFromString(string(content), path)
}

func (e *DSLEngine) LoadScriptFromString(content, filename string) (*Script, error) {
    // Parse metadata from docstring
    metadata := e.parseMetadata(content)
    
    // Parse and compile
    ast, err := e.interpreter.Parse(content, filename)
    if err != nil {
        return nil, fmt.Errorf("parse error: %w", err)
    }
    
    return &Script{
        Metadata: metadata,
        AST:      ast,
        Source:   content,
    }, nil
}
```

## 3. Script Execution

```go
func (e *DSLEngine) Run(script *Script, inputs map[string]interface{}) (*ScriptResult, error) {
    // Validate inputs
    if err := e.validateInputs(script, inputs); err != nil {
        return nil, err
    }
    
    // Create execution context
    ctx := &ExecutionContext{
        Inputs:    inputs,
        Config:    e.config,
        Executor:  e.executor,
        Variables: make(map[string]interface{}),
    }
    
    // Execute with timeout
    ctxExec, cancel := context.WithTimeout(context.Background(), e.config.MaxExecutionTime)
    defer cancel()
    
    result, err := e.execute(ctxExec, script, ctx)
    if err != nil {
        return nil, fmt.Errorf("execution failed: %w", err)
    }
    
    return &ScriptResult{
        Outputs:  result,
        Duration: time.Since(start),
    }, nil
}
```

## 4. Resource Limits

```go
func (e *DSLEngine) setupLimits(thread *starlark.Thread) {
    // Step limit
    thread.SetMaxSteps(e.config.MaxSteps)
    
    // Execution time limit
    thread.SetMaxExecutionTime(e.config.MaxExecutionTime)
}

func (e *DSLEngine) enforceMemoryLimit() error {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    if m.Alloc > uint64(e.config.MaxMemoryMB)*1024*1024 {
        return errors.New("memory limit exceeded")
    }
    
    return nil
}
```

## 5. Error Handling

```go
type DSLError struct {
    Type    ErrorType `json:"type"`
    Message string    `json:"message"`
    Line    int       `json:"line,omitempty"`
    Column  int       `json:"column,omitempty"`
}

type ErrorType string

const (
    ErrorParse      ErrorType = "parse"
    ErrorRuntime    ErrorType = "runtime"
    ErrorValidation ErrorType = "validation"
    ErrorResource   ErrorType = "resource"
)

func (e *DSLEngine) wrapError(err error, script *Script) *DSLError {
    if evalErr, ok := err.(*starlark.EvalError); ok {
        return &DSLError{
            Type:    ErrorRuntime,
            Message: evalErr.Msg,
            Line:    evalErr.Pos.Line,
            Column:  evalErr.Pos.Col,
        }
    }
    
    return &DSLError{
        Type:    ErrorRuntime,
        Message: err.Error(),
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
