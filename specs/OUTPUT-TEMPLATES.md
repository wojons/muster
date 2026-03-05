# Output Templates Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **output templating** for custom formatting.

## 1. Template Engine

```go
type TemplateFormatter struct {
    template *template.Template
}

func NewTemplateFormatter(templateStr string) (*TemplateFormatter, error) {
    tmpl, err := template.New("output").Parse(templateStr)
    if err != nil {
        return nil, fmt.Errorf("template parse error: %w", err)
    }
    
    return &TemplateFormatter{template: tmpl}, nil
}

func (tf *TemplateFormatter) Format(data interface{}) (string, error) {
    var buf bytes.Buffer
    if err := tf.template.Execute(&buf, data); err != nil {
        return "", fmt.Errorf("template execution error: %w", err)
    }
    return buf.String(), nil
}
```

## 2. Template Functions

```go
var templateFuncs = template.FuncMap{
    "json": func(v interface{}) string {
        b, _ := json.Marshal(v)
        return string(b)
    },
    "prettyjson": func(v interface{}) string {
        b, _ := json.MarshalIndent(v, "", "  ")
        return string(b)
    },
    "upper": strings.ToUpper,
    "lower": strings.ToLower,
    "trim":  strings.TrimSpace,
}
```

## 3. Example Templates

```
# Simple table
{{range .}}{{.Name}}\t{{.Status}}\n{{end}}

# JSON
{{. | prettyjson}}

# CSV
{{range $i, $v := .}}{{if $i}},{{end}}{{$v}}{{end}}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
