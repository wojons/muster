# Table Formatter Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **table formatter** for tabular data display.

## 1. Table Structure

```go
type Table struct {
    Headers []string
    Rows    [][]string
    Widths  []int
}

type TableFormatter struct {
    Style TableStyle
}

type TableStyle struct {
    Border       bool
    Separator    string
    Padding      int
    HeaderStyle  string
}
```

## 2. Formatting

```go
func (f *TableFormatter) Format(data interface{}) (string, error) {
    table, err := f.toTable(data)
    if err != nil {
        return "", err
    }
    
    // Calculate column widths
    f.calculateWidths(table)
    
    // Build output
    var buf strings.Builder
    
    // Header
    if f.Style.Border {
        buf.WriteString(f.borderLine(table.Widths))
        buf.WriteString("\n")
    }
    
    f.writeRow(&buf, table.Headers, table.Widths)
    buf.WriteString("\n")
    
    if f.Style.Border {
        buf.WriteString(f.borderLine(table.Widths))
        buf.WriteString("\n")
    }
    
    // Rows
    for _, row := range table.Rows {
        f.writeRow(&buf, row, table.Widths)
        buf.WriteString("\n")
    }
    
    if f.Style.Border {
        buf.WriteString(f.borderLine(table.Widths))
    }
    
    return buf.String(), nil
}

func (f *TableFormatter) writeRow(buf *strings.Builder, row []string, widths []int) {
    for i, cell := range row {
        if i > 0 {
            buf.WriteString(f.Style.Separator)
        }
        padded := fmt.Sprintf("%-*s", widths[i], cell)
        buf.WriteString(padded)
    }
}
```

## 3. Auto-Detection

```go
func (f *TableFormatter) toTable(data interface{}) (*Table, error) {
    switch v := data.(type) {
    case []map[string]interface{}:
        return f.fromMapSlice(v)
    case map[string][]interface{}:
        return f.fromColumns(v)
    default:
        return nil, fmt.Errorf("unsupported data type for table: %T", data)
    }
}

func (f *TableFormatter) fromMapSlice(data []map[string]interface{}) (*Table, error) {
    if len(data) == 0 {
        return &Table{}, nil
    }
    
    // Extract headers from first item
    headers := make([]string, 0, len(data[0]))
    for key := range data[0] {
        headers = append(headers, key)
    }
    sort.Strings(headers)
    
    // Build rows
    rows := make([][]string, len(data))
    for i, item := range data {
        row := make([]string, len(headers))
        for j, header := range headers {
            if val, ok := item[header]; ok {
                row[j] = fmt.Sprintf("%v", val)
            }
        }
        rows[i] = row
    }
    
    return &Table{
        Headers: headers,
        Rows:    rows,
    }, nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
