# DSL Specification - Starlark Implementation

> STATUS: HISTORICAL — the path referenced here (`internal/core/dsl/interpreter.go`) was
> never built. The DSL is implemented by `pkg/dsl/interpreter.go`. Treat the path below as
> a design-time label.

## Overview

Complete specification for the Starlark DSL including grammar, builtins, evaluation engine, and Go implementation.

## 1. Language Definition

### Grammar (EBNF)

```
Program         ::= Statement* Expression?

Statement       ::= Assignment
                  | ForLoop
                  | IfStatement
                  | ReturnStatement
                  | Expression

Assignment      ::= Identifier "=" Expression

ForLoop         ::= "for" Identifier "in" Expression ":" Statement*

IfStatement     ::= "if" Expression ":" Statement+
                  ("elif" Expression ":" Statement+)*
                  ("else" ":" Statement+)?

ReturnStatement ::= "return" Expression?

Expression      ::= OrExpr

OrExpr          ::= AndExpr ("or" AndExpr)*
AndExpr         ::= NotExpr ("and" NotExpr)*
NotExpr         ::= "not" Comparison | Comparison
Comparison      ::= AddExpr (ComparisonOperator AddExpr)*
ComparisonOperator ::= "==" | "!=" | "<" | ">" | "<=" | ">=" | "in" | "not in"

AddExpr         ::= MulExpr (AddOperator MulExpr)*
AddOperator     ::= "+" | "-"

MulExpr         ::= UnaryExpr (MulOperator UnaryExpr)*
MulOperator     ::= "*" | "/" | "//" | "%"

UnaryExpr       ::= "-" Primary | "+" Primary | Primary

Primary         ::= Literal
                  | Identifier
                  | Call
                  | ListExpr
                  | DictExpr
                  | "(" Expression ")"
                  | Primary "." Identifier
                  | Primary "[" Expression "]"

Call            ::= Identifier "(" Arguments? ")"
                  | Primary "." Identifier "(" Arguments? ")"

Arguments       ::= Expression ("," Expression)*

ListExpr        ::= "[" Elements? "]"
Elements        ::= Expression ("," Expression)*

DictExpr        ::= "{" DictEntries? "}"
DictEntries     ::= DictEntry ("," DictEntry)*
DictEntry       ::= Expression ":" Expression

Literal         ::= IntegerLiteral
                  | FloatLiteral
                  | StringLiteral
                  | "True" | "False" | "None"

Identifier      ::= [a-zA-Z_][a-zA-Z0-9_]*
```

### Reserved Keywords

```
and    break  continue  def    elif  else  for
if     in     not       or     pass  return
True   False  None
```

### Data Types

```go
// Value represents any Starlark value
type Value interface {
    String() string
    Type() string
    Truth() Bool
    Hash() (uint32, error)
}

// Supported types:
// - NoneType (None)
// - Bool (True, False)
// - Int (arbitrary precision)
// - Float (float64)
// - String (immutable)
// - List (mutable)
// - Dict (mutable)
// - Struct (custom Go-backed types)
```

---

## 2. Built-in Functions

### HTTP Operations

```go
// http.get(url, headers=None) -> Response
// GET request to URL
//
// Parameters:
//   url: string - The URL to request
//   headers: dict|None - Optional headers
//
// Returns: Response struct
//
// Example:
//   response = http.get("/users/123")
//   if response.status_code == 200:
//       return response.json()

http_get = &Builtin{
    Name: "http.get",
    Fn: func(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
        var url string
        var headers Dict
        
        if err := UnpackArgs("http.get", args, kwargs, 
            "url", &url,
            "headers?", &headers); err != nil {
            return nil, err
        }
        
        ctx := getContext(thread)
        resp, err := ctx.HTTPClient.Get(url)
        if err != nil {
            return nil, err
        }
        
        return makeResponse(resp), nil
    },
}

// http.post(url, body=None, headers=None) -> Response
// POST request to URL
//
// Parameters:
//   url: string - The URL to request
//   body: dict|list|None - Request body
//   headers: dict|None - Optional headers

http_post = &Builtin{
    Name: "http.post",
    Fn: func(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
        var url string
        var body Value
        var headers Dict
        
        if err := UnpackArgs("http.post", args, kwargs,
            "url", &url,
            "body?", &body,
            "headers?", &headers); err != nil {
            return nil, err
        }
        
        ctx := getContext(thread)
        
        // Convert body to Go map
        var goBody map[string]interface{}
        if body != nil {
            goBody = toGoMap(body)
        }
        
        resp, err := ctx.HTTPClient.Post(url, goBody)
        if err != nil {
            return nil, err
        }
        
        return makeResponse(resp), nil
    },
}

// http.patch(url, body=None, headers=None) -> Response
// PATCH request to URL

// http.delete(url, headers=None) -> Response
// DELETE request to URL

// Response object
type Response struct {
    StatusCode int
    Body       Value  // String or parsed JSON
    Headers    Dict
    JSON       func() (Value, error)  // Method to parse body as JSON
    Text       func() String  // Method to get body as string
}

func makeResponse(resp *http.Response) *Response {
    return &Response{
        StatusCode: resp.StatusCode,
        Body:       String(resp.Body),
        Headers:    toStarlarkDict(resp.Headers),
        JSON: func() (Value, error) {
            return jsonDecode(String(resp.Body))
        },
        Text: func() String {
            return String(resp.Body)
        },
    }
}
```

### JSON Operations

```go
// json.encode(value) -> string
// Encode value as JSON string
//
// Example:
//   data = {"name": "John", "age": 30}
//   json_str = json.encode(data)

json_encode = &Builtin{
    Name: "json.encode",
    Fn: func(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
        var value Value
        if err := UnpackArgs("json.encode", args, kwargs, "value", &value); err != nil {
            return nil, err
        }
        
        // Convert Starlark value to Go, then JSON
        goValue := toGoValue(value)
        data, err := json.Marshal(goValue)
        if err != nil {
            return nil, fmt.Errorf("json.encode: %v", err)
        }
        
        return String(data), nil
    },
}

// json.decode(string) -> value
// Decode JSON string to Starlark value
//
// Example:
//   data = json.decode('{"name": "John"}')
//   print(data["name"])  # "John"

json_decode = &Builtin{
    Name: "json.decode",
    Fn: func(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
        var s string
        if err := UnpackArgs("json.decode", args, kwargs, "string", &s); err != nil {
            return nil, err
        }
        
        var goValue interface{}
        if err := json.Unmarshal([]byte(s), &goValue); err != nil {
            return nil, fmt.Errorf("json.decode: %v", err)
        }
        
        return toStarlarkValue(goValue), nil
    },
}

// json.path(data, path) -> value
// Extract value using JSONPath expression
//
// Example:
//   data = {"users": [{"name": "John"}, {"name": "Jane"}]}
//   names = json.path(data, "$.users[*].name")  # ["John", "Jane"]

json_path = &Builtin{
    Name: "json.path",
    Fn: func(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
        var data Value
        var path string
        if err := UnpackArgs("json.path", args, kwargs, "data", &data, "path", &path); err != nil {
            return nil, err
        }
        
        // Convert to Go
        goData := toGoValue(data)
        
        // Apply JSONPath
        result, err := jsonpath.Get(path, goData)
        if err != nil {
            return nil, fmt.Errorf("json.path: %v", err)
        }
        
        return toStarlarkValue(result), nil
    },
}
```

### String Operations

```go
// str.split(sep=None, maxsplit=-1) -> list
// Split string by separator
//
// Example:
//   "a,b,c".split(",")  # ["a", "b", "c"]

// str.join(iterable) -> string
// Join strings with separator
//
// Example:
//   ",".join(["a", "b", "c"])  # "a,b,c"

// str.startswith(prefix) -> bool
// str.endswith(suffix) -> bool
// str.replace(old, new, count=-1) -> string
// str.strip(chars=None) -> string
// str.lower() -> string
// str.upper() -> string
// str.format(*args, **kwargs) -> string
```

### List Operations

```go
// list.append(x) -> None
// list.extend(iterable) -> None
// list.insert(i, x) -> None
// list.remove(x) -> None
// list.pop(i=-1) -> item
// list.clear() -> None
// list.index(x, start=0, end=None) -> int
// list.count(x) -> int
// list.sort(key=None, reverse=False) -> None
// list.reverse() -> None

// len(list) -> int
// list[i] -> item
// list[i:j] -> list (slice)
// list * n -> list (repeat)
// list1 + list2 -> list (concatenate)
```

### Dict Operations

```go
// dict.get(key, default=None) -> value
// dict.keys() -> list
// dict.values() -> list
// dict.items() -> list of tuples
// dict.pop(key, default=None) -> value
// dict.popitem() -> tuple
// dict.update(other) -> None
// dict.clear() -> None
// dict.setdefault(key, default=None) -> value

// key in dict -> bool
// dict[key] -> value
// dict[key] = value
// len(dict) -> int
```

### Utility Functions

```go
// len(x) -> int
// Return length of string, list, dict

// range(start, stop, step=1) -> list
// Generate list of integers
//
// Examples:
//   range(5)          # [0, 1, 2, 3, 4]
//   range(1, 5)       # [1, 2, 3, 4]
//   range(0, 10, 2)   # [0, 2, 4, 6, 8]

// print(*args, sep=" ", end="\n") -> None
// Print values to output
//
// Example:
//   print("Hello", "World")  # Hello World

// type(x) -> string
// Return type name of value
//
// Example:
//   type([1, 2, 3])  # "list"

// bool(x) -> bool
// Convert to boolean

// int(x, base=10) -> int
// Convert to integer

// float(x) -> float
// Convert to float

// str(x) -> string
// Convert to string
```

---

## 3. Thread Context

```go
// Thread represents an execution context
type Thread struct {
    Name        string
    Load        func(thread *Thread, module string) (StringDict, error)
    Print       func(thread *Thread, msg string)
    Steps       int64
    MaxSteps    int64
    
    // Context for HTTP operations
    Context     *ExecutionContext
}

// ExecutionContext provides Go-backed functionality
type ExecutionContext struct {
    HTTPClient  HTTPClient
    Logger      Logger
    Secrets     SecretsProvider
    Cache       CacheProvider
}
```

---

## 4. Interpreter Implementation

```go
// File: internal/core/dsl/interpreter.go

package dsl

import (
    "context"
    "fmt"
    
    "go.starlark.net/starlark"
    "go.starlark.net/starlarkstruct"
)

// Interpreter executes Starlark scripts
type Interpreter struct {
    globals    starlark.StringDict
    builtins   starlark.StringDict
    maxSteps   int64
    maxMemory  int64
}

// Options for creating interpreter
type Options struct {
    MaxSteps   int64
    MaxMemory  int64
    HTTPClient HTTPClient
    Logger     Logger
}

// NewInterpreter creates a new interpreter
func NewInterpreter(opts Options) *Interpreter {
    i := &Interpreter{
        maxSteps:  opts.MaxSteps,
        maxMemory: opts.MaxMemory,
    }
    
    // Initialize builtins
    i.builtins = starlark.StringDict{
        // HTTP operations
        "http": starlarkstruct.FromStringDict(
            starlarkstruct.Default,
            starlark.StringDict{
                "get":    starlark.NewBuiltin("http.get", i.httpGet),
                "post":   starlark.NewBuiltin("http.post", i.httpPost),
                "patch":  starlark.NewBuiltin("http.patch", i.httpPatch),
                "delete": starlark.NewBuiltin("http.delete", i.httpDelete),
            },
        ),
        
        // JSON operations
        "json": starlarkstruct.FromStringDict(
            starlarkstruct.Default,
            starlark.StringDict{
                "encode": starlark.NewBuiltin("json.encode", i.jsonEncode),
                "decode": starlark.NewBuiltin("json.decode", i.jsonDecode),
                "path":   starlark.NewBuiltin("json.path", i.jsonPath),
            },
        ),
        
        // Utility functions
        "len":   starlark.NewBuiltin("len", starlark.Len),
        "print": starlark.NewBuiltin("print", i.print),
        "type":  starlark.NewBuiltin("type", starlark.Type),
        "range": starlark.NewBuiltin("range", starlark.Range),
    }
    
    return i
}

// Execute runs a script
func (i *Interpreter) Execute(ctx context.Context, script string, env map[string]interface{}) (*Result, error) {
    // Create thread with context
    thread := &starlark.Thread{
        Name: "workflow",
        Load: nil, // Disallow imports
        Print: func(thread *starlark.Thread, msg string) {
            if i.logger != nil {
                i.logger.Info(msg)
            }
        },
    }
    
    // Set execution limits
    thread.SetMaxExecutionSteps(i.maxSteps)
    
    // Attach context
    thread.SetLocal("context", ctx)
    thread.SetLocal("http_client", i.httpClient)
    
    // Convert environment to Starlark values
    globals := make(starlark.StringDict)
    for k, v := range env {
        globals[k] = toStarlark(v)
    }
    
    // Merge with builtins
    for k, v := range i.builtins {
        globals[k] = v
    }
    
    // Parse and execute
    start := time.Now()
    result, err := starlark.ExecFile(thread, "workflow.star", script, globals)
    duration := time.Since(start)
    
    if err != nil {
        return nil, fmt.Errorf("execution failed: %w", err)
    }
    
    // Extract return value
    var returnValue interface{}
    if val, ok := result["result"]; ok {
        returnValue = fromStarlark(val)
    }
    
    return &Result{
        Value:       returnValue,
        Steps:       thread.Steps(),
        Duration:    duration,
        MemoryUsed:  estimateMemory(result),
    }, nil
}

// Validate checks script syntax
func (i *Interpreter) Validate(script string) error {
    _, err := starlark.Parse("workflow.star", []byte(script), 0)
    return err
}

// httpGet implements http.get builtin
func (i *Interpreter) httpGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var url string
    var headers starlark.Dict
    
    if err := starlark.UnpackArgs("http.get", args, kwargs,
        "url", &url,
        "headers?", &headers); err != nil {
        return nil, err
    }
    
    // Get HTTP client from thread
    client, ok := thread.Local("http_client").(HTTPClient)
    if !ok {
        return nil, fmt.Errorf("HTTP client not available")
    }
    
    ctx, ok := thread.Local("context").(context.Context)
    if !ok {
        ctx = context.Background()
    }
    
    // Convert headers
    goHeaders := fromStarlarkDict(headers)
    
    // Make request
    resp, err := client.Get(ctx, url, goHeaders)
    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    
    return toStarlarkResponse(resp), nil
}

// jsonEncode implements json.encode builtin
func (i *Interpreter) jsonEncode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var value starlark.Value
    if err := starlark.UnpackArgs("json.encode", args, kwargs, "value", &value); err != nil {
        return nil, err
    }
    
    goValue := fromStarlark(value)
    data, err := json.Marshal(goValue)
    if err != nil {
        return nil, fmt.Errorf("json.encode: %w", err)
    }
    
    return starlark.String(data), nil
}

// jsonDecode implements json.decode builtin
func (i *Interpreter) jsonDecode(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var s starlark.String
    if err := starlark.UnpackArgs("json.decode", args, kwargs, "string", &s); err != nil {
        return nil, err
    }
    
    var goValue interface{}
    if err := json.Unmarshal([]byte(s), &goValue); err != nil {
        return nil, fmt.Errorf("json.decode: %w", err)
    }
    
    return toStarlark(goValue), nil
}

// print implements print builtin
func (i *Interpreter) print(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if i.logger != nil {
        var parts []string
        for _, arg := range args {
            parts = append(parts, arg.String())
        }
        i.logger.Info(strings.Join(parts, " "))
    }
    return starlark.None, nil
}

// Conversion helpers
func toStarlark(v interface{}) starlark.Value {
    switch val := v.(type) {
    case nil:
        return starlark.None
    case bool:
        return starlark.Bool(val)
    case int:
        return starlark.MakeInt(val)
    case int64:
        return starlark.MakeInt64(val)
    case float64:
        return starlark.Float(val)
    case string:
        return starlark.String(val)
    case []interface{}:
        list := starlark.NewList(nil)
        for _, item := range val {
            list.Append(toStarlark(item))
        }
        return list
    case map[string]interface{}:
        dict := starlark.NewDict(len(val))
        for k, v := range val {
            dict.SetKey(starlark.String(k), toStarlark(v))
        }
        return dict
    default:
        return starlark.String(fmt.Sprintf("%v", val))
    }
}

func fromStarlark(v starlark.Value) interface{} {
    switch val := v.(type) {
    case starlark.NoneType:
        return nil
    case starlark.Bool:
        return bool(val)
    case starlark.Int:
        i, _ := val.Int64()
        return i
    case starlark.Float:
        return float64(val)
    case starlark.String:
        return string(val)
    case *starlark.List:
        var result []interface{}
        iter := val.Iterate()
        defer iter.Done()
        var x starlark.Value
        for iter.Next(&x) {
            result = append(result, fromStarlark(x))
        }
        return result
    case *starlark.Dict:
        result := make(map[string]interface{})
        for _, item := range val.Items() {
            key, _ := starlark.AsString(item[0])
            result[key] = fromStarlark(item[1])
        }
        return result
    default:
        return v.String()
    }
}
```

---

## 5. Workflow Examples

### Example 1: Simple Fetch

```python
# Fetch user and their posts
user = http.get("/users/123").json()
posts = http.get("/users/123/posts").json()

result = {
    "user": user,
    "post_count": len(posts),
    "posts": posts
}
```

### Example 2: Batch Processing

```python
# Fetch all items and process each
items = http.get("/items").json()

results = []
for item in items:
    if item["status"] == "pending":
        processed = http.post("/process", {"id": item["id"]}).json()
        results.append({
            "id": item["id"],
            "status": processed["status"]
        })

result = results
```

### Example 3: Chained Requests

```python
# Create order then get details
order_data = {
    "customer_id": "cust_123",
    "items": [{"sku": "ABC", "qty": 2}]
}

created = http.post("/orders", order_data).json()
order_id = created["id"]

# Wait for processing (in real workflow, might need polling)
order = http.get("/orders/" + str(order_id)).json()

# Get shipment info
shipments = http.get("/orders/" + str(order_id) + "/shipments").json()

result = {
    "order": order,
    "shipments": shipments
}
```

### Example 4: Error Handling

```python
# Handle errors gracefully
response = http.get("/data")

if response.status_code == 200:
    data = response.json()
    result = data
elif response.status_code == 404:
    # Return empty result
    result = {"data": [], "message": "Not found"}
else:
    # Log error and return partial data
    print("Error:", response.status_code)
    result = {"error": True, "status": response.status_code}
```

---

## 6. Security Restrictions

```go
// Security settings
type SecurityConfig struct {
    // Execution limits
    MaxSteps      int64         // Maximum execution steps (default: 100000)
    MaxMemory     int64         // Maximum memory in bytes (default: 64MB)
    MaxDuration   time.Duration // Maximum execution time (default: 30s)
    
    // HTTP restrictions
    AllowedHosts  []string      // Whitelist of allowed hosts
    MaxRequestSize int64        // Maximum request body size
    
    // Forbidden operations
    ForbiddenBuiltins []string // e.g., ["exec", "eval"]
}

// Enforced restrictions:
// - No file system access (unless explicitly granted)
// - No network access except via http.* builtins
// - No imports (load is disabled)
// - No raw bytecode execution
// - No reflection or introspection
// - CPU/memory/time limits enforced
```

---

## Deterministic Guarantees

This DSL specification must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic DSL Grammar

The DSL grammar must be parsed identically across platforms:

- **Tokenization**: Same token boundaries and token types
- **AST generation**: Identical abstract syntax tree for same source
- **Error reporting**: Same error messages for same syntax errors

### 2. Platform‑Independent Built‑in Functions

Built‑in functions must behave identically across platforms:

- **HTTP functions**: Identical request formation and response parsing
- **JSON functions**: Deterministic JSON serialization (sorted keys)
- **Math functions**: IEEE 754‑2019 strict floating‑point, identical results

### 3. Deterministic Interpreter Execution

Interpreter execution must be deterministic:

- **Execution order**: Same statement execution order
- **Memory allocation**: Deterministic allocation patterns
- **Random number generation**: Seeded RNG for test mode

### 4. Cross‑Language DSL Implementation

When generating SDKs for 12 languages, DSL implementations must:

- **Same grammar**: Identical parsing rules
- **Same built‑ins**: Identical function behavior
- **Same security restrictions**: Identical security enforcement

### 5. Testing Determinism

All DSL specification tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestDSLSpecDeterminism(t *testing.T) {
	// Verify identical grammar parsing
	// Verify identical built‑in function behavior
	// Verify identical interpreter execution
}
```

### 6. Integration with SDK Generation

The DSL specification enables deterministic DSL implementation across 12 languages:

- **Grammar specification**: Deterministic grammar definition for code generation
- **Built‑in library**: Deterministic built‑in function specifications
- **Security model**: Deterministic security restriction enforcement

**Verification**: Run `./scripts/validate‑determinism‑dsl‑spec.sh` to confirm cross‑platform consistency.

This DSL specification provides complete grammar, built-in functions, interpreter implementation, and security model for Ralph Loop.
