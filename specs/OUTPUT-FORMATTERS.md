# Output Formatters Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **output formatters** for command results.

## 1. Formatter Interface

```go
type Formatter interface {
    Format(data interface{}) (string, error)
    ContentType() string
}

type FormatterRegistry struct {
    formatters map[string]Formatter
}

func (r *FormatterRegistry) Register(name string, f Formatter) {
    r.formatters[name] = f
}

func (r *FormatterRegistry) Get(name string) (Formatter, error) {
    f, ok := r.formatters[name]
    if !ok {
        return nil, fmt.Errorf("unknown format: %s", name)
    }
    return f, nil
}
```

## 2. Supported Formats

```go
var DefaultFormats = map[string]Formatter{
    "json":   &JSONFormatter{},
    "yaml":   &YAMLFormatter{},
    "table":  &TableFormatter{},
    "raw":    &RawFormatter{},
    "template": &TemplateFormatter{},
}
```

## 3. Format Selection

```go
func SelectFormatter(flags *pflag.FlagSet, data interface{}) (Formatter, error) {
    // Check explicit format flag
    if format, _ := flags.GetString("format"); format != "" {
        return registry.Get(format)
    }
    
    // Auto-detect based on output
    if isatty.IsTerminal(os.Stdout.Fd()) {
        return registry.Get("table")
    }
    
    return registry.Get("json")
}
```

## 4. Raw Formatter

```go
type RawFormatter struct{}

func (f *RawFormatter) Format(data interface{}) (string, error) {
    switch v := data.(type) {
    case string:
        return v, nil
    case []byte:
        return string(v), nil
    default:
        return fmt.Sprintf("%v", v), nil
    }
}

func (f *RawFormatter) ContentType() string {
    return "text/plain"
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
