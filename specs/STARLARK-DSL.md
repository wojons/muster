# Starlark DSL Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: DSL

## Overview

Complete specification of the **Starlark DSL** for scripting API interactions and workflows.

## 1. Language Subset

### 1.1 Supported Syntax

```python
# Variables and data types
name = "value"
count = 42
enabled = True
items = ["a", "b", "c"]
config = {"key": "value", "num": 123}

# Functions
def process_data(data):
    result = []
    for item in data:
        if item.active:
            result.append(item.name)
    return result

# List comprehensions
active_users = [u for u in users if u.active]

# Conditional expressions
status = "active" if user.active else "inactive"
```

### 1.2 Restrictions

- No `while` loops (use `for` with `range`)
- No recursive functions
- No global variables modification
- No `import` statements (builtins only)
- No `lambda` functions

## 2. Script Structure

```python
# Script metadata (optional)
"""
name: "Fetch Users"
description: "Retrieves all active users"
version: "1.0.0"
"""

# Load module (built-in only)
http = module("http")
json = module("json")

# Main function (entry point)
def main(ctx):
    # ctx provides context: config, secrets, etc.
    response = http.get("https://api.example.com/users", headers={
        "Authorization": "Bearer " + ctx.secret("api_token")
    })
    
    if response.status_code != 200:
        fail("Failed to fetch users: " + str(response.status_code))
    
    users = json.decode(response.body)
    active = [u for u in users if u.get("active", False)]
    
    return {
        "count": len(active),
        "users": active
    }
```

## 3. Execution Model

```go
type StarlarkScript struct {
    Name        string
    Description string
    Version     string
    AST         *syntax.File
    Globals     starlark.StringDict
}

func (e *DSLEngine) Execute(script *StarlarkScript, ctx *ExecutionContext) (starlark.Value, error) {
    // Create thread with limits
    thread := &starlark.Thread{
        Name:  script.Name,
        Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
    }
    
    // Set execution limits
    thread.SetMaxSteps(100000)
    
    // Execute main function
    main := script.Globals["main"]
    if main == nil {
        return nil, errors.New("no main function defined")
    }
    
    ctxValue := starlarkstruct.FromStringDict(
        starlarkstruct.Default, 
        starlark.StringDict{"config": ctx.Config},
    )
    
    return starlark.Call(thread, main, starlark.Tuple{ctxValue}, nil)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
