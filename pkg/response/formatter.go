package response

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Formatter interface {
	Format(data interface{}) ([]byte, error)
	ContentType() string
}

type FormatterRegistry struct {
	formatters map[string]Formatter
}

func NewFormatterRegistry() *FormatterRegistry {
	return &FormatterRegistry{
		formatters: make(map[string]Formatter),
	}
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

var DefaultFormatters = map[string]Formatter{
	"json":     &JSONFormatter{indent: true},
	"yaml":     &YAMLFormatter{},
	"table":    &TableFormatter{},
	"csv":      &CSVFormatter{},
	"raw":      &RawFormatter{},
	"template": &TemplateFormatter{},
}

func NewFormatter(config Config) Formatter {
	switch config.DefaultFormat {
	case "json":
		return &JSONFormatter{indent: true}
	case "yaml":
		return &YAMLFormatter{}
	case "table":
		return &TableFormatter{}
	case "csv":
		return &CSVFormatter{}
	case "raw":
		return &RawFormatter{}
	case "template":
		return &TemplateFormatter{}
	default:
		return &JSONFormatter{indent: true}
	}
}

type JSONFormatter struct {
	indent bool
	Sorted bool
}

func (f *JSONFormatter) Format(data interface{}) ([]byte, error) {
	if f.Sorted {
		data = SortKeys(data)
	}
	data = f.handleNumbers(data)
	if f.indent {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(data); err != nil {
			return nil, err
		}
		return bytes.TrimRight(buf.Bytes(), "\n"), nil
	}
	return json.Marshal(data)
}

func (f *JSONFormatter) ContentType() string {
	return "application/json"
}

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

func (f *JSONFormatter) handleNumbers(data interface{}) interface{} {
	switch v := data.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if fl, err := v.Float64(); err == nil {
			return fl
		}
		return v.String()
	}
	return data
}

type YAMLFormatter struct{}

func (f *YAMLFormatter) Format(data interface{}) ([]byte, error) {
	return yaml.Marshal(data)
}

func (f *YAMLFormatter) ContentType() string {
	return "application/x-yaml"
}

type TableFormatter struct {
	columns []string
}

func (f *TableFormatter) Format(data interface{}) ([]byte, error) {
	switch v := data.(type) {
	case []interface{}:
		return f.formatSlice(v)
	case []map[string]interface{}:
		return f.formatMapSlice(v)
	case map[string]interface{}:
		return f.formatMap(v)
	default:
		return json.MarshalIndent(data, "", "  ")
	}
}

func (f *TableFormatter) formatSlice(items []interface{}) ([]byte, error) {
	if len(items) == 0 {
		return []byte("No data found.\n"), nil
	}

	var maps []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			maps = append(maps, m)
		}
	}

	if len(maps) > 0 {
		return f.formatMapSlice(maps)
	}

	var buf bytes.Buffer
	for _, item := range items {
		buf.WriteString(fmt.Sprintf("- %v\n", item))
	}
	return buf.Bytes(), nil
}

func (f *TableFormatter) formatMapSlice(items []map[string]interface{}) ([]byte, error) {
	if len(items) == 0 {
		return []byte("No data found.\n"), nil
	}

	headers := f.extractHeaders(items[0])

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)

	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = strings.Repeat("-", len(headers[i]))
	}
	fmt.Fprintln(tw, strings.Join(sep, "\t"))

	for _, item := range items {
		row := f.extractRow(item, headers)
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	tw.Flush()
	return buf.Bytes(), nil
}

func (f *TableFormatter) formatMap(data map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)

	var keys []string
	for key := range data {
		keys = append(keys, key)
	}

	for _, key := range keys {
		value := data[key]
		fmt.Fprintf(tw, "%s:\t%v\n", key, value)
	}

	tw.Flush()
	return buf.Bytes(), nil
}

func (f *TableFormatter) extractHeaders(item map[string]interface{}) []string {
	if len(f.columns) > 0 {
		return f.columns
	}

	var headers []string
	for key := range item {
		headers = append(headers, key)
	}
	return headers
}

func (f *TableFormatter) extractRow(item map[string]interface{}, headers []string) []string {
	row := make([]string, len(headers))

	for i, header := range headers {
		if val, ok := item[header]; ok {
			switch v := val.(type) {
			case nil:
				row[i] = "<null>"
			case string:
				row[i] = v
			case float64:
				row[i] = fmt.Sprintf("%g", v)
			case bool:
				row[i] = fmt.Sprintf("%t", v)
			default:
				s := fmt.Sprintf("%v", v)
				if len(s) > 50 {
					s = s[:47] + "..."
				}
				row[i] = s
			}
		} else {
			row[i] = ""
		}
	}

	return row
}

func (f *TableFormatter) ContentType() string {
	return "text/plain"
}

type CSVFormatter struct{}

func (f *CSVFormatter) Format(data interface{}) ([]byte, error) {
	items, ok := data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("CSV output requires array of maps")
	}

	if len(items) == 0 {
		return []byte(""), nil
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	headers := make([]string, 0, len(items[0]))
	for key := range items[0] {
		headers = append(headers, key)
	}
	sort.Strings(headers)

	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	for _, item := range items {
		row := make([]string, len(headers))
		for i, header := range headers {
			val, ok := item[header]
			if ok {
				row[i] = fmt.Sprintf("%v", val)
			} else {
				row[i] = ""
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *CSVFormatter) ContentType() string {
	return "text/csv"
}

type RawFormatter struct{}

func (f *RawFormatter) Format(data interface{}) ([]byte, error) {
	switch v := data.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(data)
	}
}

func (f *RawFormatter) ContentType() string {
	return "text/plain"
}

var templateFuncs = template.FuncMap{
	"json":       func(v interface{}) string { b, _ := json.Marshal(v); return string(b) },
	"prettyjson": func(v interface{}) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) },
	"upper":      strings.ToUpper,
	"lower":      strings.ToLower,
	"trim":       strings.TrimSpace,
}

type TemplateFormatter struct {
	template *template.Template
}

func NewTemplateFormatter(templateStr string) (*TemplateFormatter, error) {
	tmpl, err := template.New("output").Funcs(templateFuncs).Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	return &TemplateFormatter{template: tmpl}, nil
}

func (tf *TemplateFormatter) Format(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := tf.template.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}
	return buf.Bytes(), nil
}

func (tf *TemplateFormatter) ContentType() string {
	return "text/plain"
}
