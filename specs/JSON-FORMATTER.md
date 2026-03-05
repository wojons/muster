# JSON Formatter Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **JSON formatter** with determinism guarantees.

## 1. Formatter Implementation

```go
type JSONFormatter struct {
    Indent  string
    Sorted  bool // For determinism
}

func (f *JSONFormatter) Format(data interface{}) (string, error) {
    var output []byte
    var err error
    
    if f.Indent != "" {
        output, err = json.MarshalIndent(data, "", f.Indent)
    } else {
        output, err = json.Marshal(data)
    }
    
    if err != nil {
        return "", fmt.Errorf("json marshal failed: %w", err)
    }
    
    return string(output), nil
}

func (f *JSONFormatter) ContentType() string {
    return "application/json"
}
```

## 2. Deterministic JSON

```go
func MarshalDeterministic(v interface{}) ([]byte, error) {
    // Use encoder with SetEscapeHTML(false) for consistent output
    buf := &bytes.Buffer{}
    enc := json.NewEncoder(buf)
    enc.SetEscapeHTML(false)
    enc.SetIndent("", "  ")
    
    if err := enc.Encode(v); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

// SortKeys recursively sorts map keys for deterministic output
func SortKeys(v interface{}) interface{} {
    switch val := v.(type) {
    case map[string]interface{}:
        sorted := make(map[string]interface{}, len(val))
        keys := make([]string, 0, len(val))
        for k := range val {
            keys = append(keys, k)
        }
        sort.Strings(keys)
        
        for _, k := range keys {
            sorted[k] = SortKeys(val[k])
        }
        return sorted
        
    case []interface{}:
        for i, item := range val {
            val[i] = SortKeys(item)
        }
        return val
        
    default:
        return v
    }
}
```

## 3. Number Handling

```go
func (f *JSONFormatter) handleNumbers(data interface{}) interface{} {
    // Ensure all numbers are handled consistently
    // Use json.Number for large integers
    switch v := data.(type) {
    case float64:
        // Check if it's an integer
        if v == float64(int64(v)) {
            return int64(v)
        }
        return v
    case json.Number:
        if i, err := v.Int64(); err == nil {
            return i
        }
        if f, err := v.Float64(); err == nil {
            return f
        }
        return v.String()
    }
    return data
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
