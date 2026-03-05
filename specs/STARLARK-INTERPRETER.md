# Starlark Interpreter Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Interpreter

## Overview

Complete specification of the **Starlark interpreter** integration for Go.

## 1. Interpreter Setup

```go
type StarlarkInterpreter struct {
    globals    starlark.StringDict
    predeclared starlark.StringDict
    thread     *starlark.Thread
}

func NewInterpreter(builtins map[string]interface{}) *StarlarkInterpreter {
    i := &StarlarkInterpreter{
        globals:     make(starlark.StringDict),
        predeclared: make(starlark.StringDict),
    }
    
    // Add builtins
    for name, fn := range builtins {
        i.predeclared[name] = starlark.NewBuiltin(name, fn)
    }
    
    return i
}
```

## 2. Script Parsing

```go
func (i *StarlarkInterpreter) Parse(src string, filename string) (*syntax.File, error) {
    return syntax.Parse(filename, src, 0)
}

func (i *StarlarkInterpreter) Compile(ast *syntax.File) (*starlark.Program, error) {
    _, program, err := starlark.SourceProgram(
        ast.Path,
        nil, // Source will be from AST
        i.predeclared.Has,
    )
    return program, err
}
```

## 3. Execution

```go
func (i *StarlarkInterpreter) Exec(program *starlark.Program, args starlark.Tuple) (starlark.Value, error) {
    thread := &starlark.Thread{
        Name:  "exec",
        Print: func(_ *starlark.Thread, msg string) { log.Println(msg) },
    }
    
    // Set resource limits
    thread.SetMaxSteps(100000)
    thread.SetMaxExecutionTime(30 * time.Second)
    
    return program.Init(thread, i.predeclared)
}

func (i *StarlarkInterpreter) Call(fn *starlark.Function, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    thread := &starlark.Thread{Name: "call"}
    return starlark.Call(thread, fn, args, kwargs)
}
```

## 4. Value Conversion

```go
func ToStarlark(v interface{}) (starlark.Value, error) {
    switch val := v.(type) {
    case string:
        return starlark.String(val), nil
    case int:
        return starlark.MakeInt(val), nil
    case int64:
        return starlark.MakeInt64(val), nil
    case float64:
        return starlark.Float(val), nil
    case bool:
        return starlark.Bool(val), nil
    case []interface{}:
        list := starlark.NewList([]starlark.Value{})
        for _, item := range val {
            sv, err := ToStarlark(item)
            if err != nil {
                return nil, err
            }
            list.Append(sv)
        }
        return list, nil
    case map[string]interface{}:
        dict := starlark.NewDict(len(val))
        for k, v := range val {
            sk := starlark.String(k)
            sv, err := ToStarlark(v)
            if err != nil {
                return nil, err
            }
            dict.SetKey(sk, sv)
        }
        return dict, nil
    default:
        return nil, fmt.Errorf("unsupported type: %T", v)
    }
}

func FromStarlark(v starlark.Value) (interface{}, error) {
    switch val := v.(type) {
    case starlark.String:
        return string(val), nil
    case starlark.Int:
        return val.BigInt().Int64(), nil
    case starlark.Float:
        return float64(val), nil
    case starlark.Bool:
        return bool(val), nil
    case *starlark.List:
        result := make([]interface{}, 0, val.Len())
        for i := 0; i < val.Len(); i++ {
            item, _ := val.Index(i)
            converted, err := FromStarlark(item)
            if err != nil {
                return nil, err
            }
            result = append(result, converted)
        }
        return result, nil
    case *starlark.Dict:
        result := make(map[string]interface{})
        for _, item := range val.Items() {
            key, _ := FromStarlark(item[0])
            value, _ := FromStarlark(item[1])
            result[key.(string)] = value
        }
        return result, nil
    default:
        return nil, fmt.Errorf("unsupported starlark type: %T", v)
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
