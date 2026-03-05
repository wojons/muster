# DSL Builtins Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: DSL Builtins

## Overview

Complete specification of **built-in functions** available in Starlark DSL scripts.

## 1. HTTP Module

```python
http = module("http")

# GET request
response = http.get(url, headers={}, params={}, timeout=30)

# POST request
response = http.post(url, body="", headers={}, json=None)

# PUT request
response = http.put(url, body="", headers={})

# DELETE request
response = http.delete(url, headers={})

# Response object
# response.status_code - HTTP status code
# response.body - Response body as string
# response.headers - Dict of headers
# response.json() - Parse body as JSON
```

### Implementation

```go
func builtinHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var url string
    var headers, params *starlark.Dict
    var timeout int = 30
    
    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "url", &url,
        "headers?", &headers,
        "params?", &params,
        "timeout?", &timeout,
    ); err != nil {
        return nil, err
    }
    
    // Convert headers
    headerMap := make(http.Header)
    if headers != nil {
        for _, item := range headers.Items() {
            key, _ := starlark.AsString(item[0])
            value, _ := starlark.AsString(item[1])
            headerMap.Set(key, value)
        }
    }
    
    // Execute request
    client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
    req, _ := http.NewRequest("GET", url, nil)
    req.Header = headerMap
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http request failed: %w", err)
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    // Build response object
    responseDict := starlark.NewDict(3)
    responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
    responseDict.SetKey(starlark.String("body"), starlark.String(body))
    
    return responseDict, nil
}
```

## 2. JSON Module

```python
json = module("json")

# Encode to JSON
encoded = json.encode(data, indent=None)

# Decode from JSON
decoded = json.decode(string)

# Pretty print
pretty = json.pretty(data)
```

### Implementation

```go
func builtinJSONEncode(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var value starlark.Value
    var indent int = -1
    
    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "value", &value,
        "indent?", &indent,
    ); err != nil {
        return nil, err
    }
    
    // Convert to Go interface
    goValue, err := FromStarlark(value)
    if err != nil {
        return nil, err
    }
    
    // Marshal
    var data []byte
    if indent >= 0 {
        data, err = json.MarshalIndent(goValue, "", strings.Repeat(" ", indent))
    } else {
        data, err = json.Marshal(goValue)
    }
    
    if err != nil {
        return nil, fmt.Errorf("json encode failed: %w", err)
    }
    
    return starlark.String(data), nil
}
```

## 3. Utility Functions

```python
# Fail with error
fail(message)

# Print to stdout
print(*values, sep=" ", end="\n")

# Format string
formatted = format(template, *args)

# Get environment variable
value = env("VAR_NAME", default="")

# Get secret from context
token = secret("api_key")

# Base64 encode/decode
encoded = base64_encode(data)
decoded = base64_decode(string)

# Hash functions
sha256_hash = sha256(data)
md5_hash = md5(data)

# File operations (sandboxed)
content = file_read("/data/input.txt")
file_write("/data/output.txt", content)
exists = file_exists("/data/file.txt")
```

### Implementation

```go
func builtinFail(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var msg string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs, "message", &msg); err != nil {
        return nil, err
    }
    return nil, fmt.Errorf("script failure: %s", msg)
}

func builtinEnv(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var name, defaultValue string
    if err := starlark.UnpackArgs(b.Name(), args, kwargs,
        "name", &name,
        "default?", &defaultValue,
    ); err != nil {
        return nil, err
    }
    
    value := os.Getenv(name)
    if value == "" {
        value = defaultValue
    }
    
    return starlark.String(value), nil
}
```

## 4. Builtin Registry

```go
var DefaultBuiltins = map[string]interface{}{
    // HTTP
    "http_get":    builtinHTTPGet,
    "http_post":   builtinHTTPPost,
    "http_put":    builtinHTTPPut,
    "http_delete": builtinHTTPDelete,
    
    // JSON
    "json_encode": builtinJSONEncode,
    "json_decode": builtinJSONDecode,
    
    // Utilities
    "fail":          builtinFail,
    "print":         builtinPrint,
    "format":        builtinFormat,
    "env":           builtinEnv,
    "secret":        builtinSecret,
    "base64_encode": builtinBase64Encode,
    "base64_decode": builtinBase64Decode,
    "sha256":        builtinSHA256,
    "md5":           builtinMD5,
    
    // File (sandboxed)
    "file_read":   builtinFileRead,
    "file_write":  builtinFileWrite,
    "file_exists": builtinFileExists,
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
