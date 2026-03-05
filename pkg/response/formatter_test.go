package response

import (
	"strings"
	"testing"
)

func TestJSONFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		indent  bool
		data    interface{}
		wantErr bool
	}{
		{
			name:   "object with indent",
			indent: true,
			data:   map[string]string{"key": "value"},
		},
		{
			name:   "object without indent",
			indent: false,
			data:   map[string]string{"key": "value"},
		},
		{
			name:   "array",
			indent: true,
			data:   []string{"a", "b", "c"},
		},
		{
			name:   "number",
			indent: true,
			data:   42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := JSONFormatter{indent: tt.indent}
			_, err := formatter.Format(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYAMLFormatter_Format(t *testing.T) {
	formatter := YAMLFormatter{}

	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "object",
			data:    map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "array",
			data:    []string{"a", "b", "c"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := formatter.Format(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTableFormatter_Format(t *testing.T) {
	formatter := TableFormatter{}

	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "array of interfaces",
			data:    []interface{}{"a", "b", "c"},
			wantErr: false,
		},
		{
			name: "array of maps",
			data: []map[string]interface{}{
				{"id": "1", "name": "John"},
				{"id": "2", "name": "Jane"},
			},
			wantErr: false,
		},
		{
			name:    "single map",
			data:    map[string]interface{}{"id": "1", "name": "John"},
			wantErr: false,
		},
		{
			name:    "empty array",
			data:    []interface{}{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := formatter.Format(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTableFormatter_extractHeaders(t *testing.T) {
	formatter := TableFormatter{}

	tests := []struct {
		name     string
		columns  []string
		item     map[string]interface{}
		expected int
	}{
		{
			name:     "with specified columns",
			columns:  []string{"id", "name"},
			item:     map[string]interface{}{"id": "1", "name": "John", "age": "30"},
			expected: 2,
		},
		{
			name:     "without specified columns",
			columns:  []string{},
			item:     map[string]interface{}{"id": "1", "name": "John", "age": "30"},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter.columns = tt.columns
			headers := formatter.extractHeaders(tt.item)

			if len(headers) != tt.expected {
				t.Errorf("extractHeaders() = %d, want %d", len(headers), tt.expected)
			}
		})
	}
}

func TestTableFormatter_extractRow(t *testing.T) {
	formatter := TableFormatter{}

	item := map[string]interface{}{
		"id":   "1",
		"name": "John",
		"nil":  nil,
		"bool": true,
	}

	headers := []string{"id", "name", "nil", "bool"}

	row := formatter.extractRow(item, headers)

	if len(row) != len(headers) {
		t.Errorf("extractRow() returned %d elements, want %d", len(row), len(headers))
	}

	if row[0] != "1" {
		t.Errorf("extractRow()[0] = %s, want 1", row[0])
	}

	if row[1] != "John" {
		t.Errorf("extractRow()[1] = %s, want John", row[1])
	}
}

func TestRawFormatter_Format(t *testing.T) {
	formatter := RawFormatter{}

	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "string",
			data:    "Hello, World!",
			wantErr: false,
		},
		{
			name:    "bytes",
			data:    []byte("Hello, World!"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(result) == 0 && !tt.wantErr {
				t.Error("expected non-empty output")
			}
		})
	}
}

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "JSON format",
			cfg: Config{
				DefaultFormat: "json",
			},
			want: "json",
		},
		{
			name: "YAML format",
			cfg: Config{
				DefaultFormat: "yaml",
			},
			want: "yaml",
		},
		{
			name: "table format",
			cfg: Config{
				DefaultFormat: "table",
			},
			want: "table",
		},
		{
			name: "csv format",
			cfg: Config{
				DefaultFormat: "csv",
			},
			want: "csv",
		},
		{
			name: "raw format",
			cfg: Config{
				DefaultFormat: "raw",
			},
			want: "raw",
		},
		{
			name: "unknown format defaults to JSON",
			cfg: Config{
				DefaultFormat: "unknown",
			},
			want: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewFormatter(tt.cfg)

			var fType string
			switch formatter.(type) {
			case *JSONFormatter:
				fType = "json"
			case *YAMLFormatter:
				fType = "yaml"
			case *TableFormatter:
				fType = "table"
			case *CSVFormatter:
				fType = "csv"
			case *RawFormatter:
				fType = "raw"
			case *TemplateFormatter:
				fType = "template"
			default:
				fType = "unknown"
			}

			if fType != tt.want {
				t.Errorf("NewFormatter() = %v, want %v", fType, tt.want)
			}
		})
	}
}

func TestCSVFormatter_Format(t *testing.T) {
	formatter := CSVFormatter{}

	tests := []struct {
		name string
		data interface{}
		want bool
	}{
		{
			name: "array of maps",
			data: []map[string]interface{}{
				{"id": "1", "name": "John"},
				{"id": "2", "name": "Jane"},
			},
			want: true,
		},
		{
			name: "empty array",
			data: []map[string]interface{}{},
			want: true,
		},
		{
			name: "invalid type",
			data: "not a map slice",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)

			if (err == nil) != tt.want {
				t.Errorf("Format() err = %v, want %v", err, tt.want)
			}

			if tt.want && len(result) > 0 && !strings.Contains(string(result), ",") {
				t.Error("expected CSV output with commas")
			}
		})
	}
}

func TestTemplateFormatter_Format(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     interface{}
		wantErr  bool
		contains string
	}{
		{
			name:     "simple template",
			template: `Name: {{.name}}`,
			data:     map[string]string{"name": "John"},
			contains: "Name: John",
		},
		{
			name:     "range template",
			template: `{{range .}}- {{.}}{{"\n"}}{{end}}`,
			data:     []string{"a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "json function",
			template: `{{. | json}}`,
			data:     map[string]string{"key": "value"},
			contains: "key",
		},
		{
			name:     "upper function",
			template: `{{.name | upper}}`,
			data:     map[string]string{"name": "john"},
			contains: "JOHN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter, err := NewTemplateFormatter(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTemplateFormatter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			result, err := formatter.Format(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.contains != "" && !strings.Contains(string(result), tt.contains) {
				t.Errorf("Format() output = %v, expected to contain %v", string(result), tt.contains)
			}
		})
	}
}

func TestFormatterRegistry(t *testing.T) {
	registry := NewFormatterRegistry()

	t.Run("Register and Get", func(t *testing.T) {
		registry.Register("json", &JSONFormatter{indent: true})

		f, err := registry.Get("json")
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}

		if _, ok := f.(*JSONFormatter); !ok {
			t.Error("Get() did not return JSONFormatter")
		}
	})

	t.Run("Get unknown format", func(t *testing.T) {
		_, err := registry.Get("unknown")
		if err == nil {
			t.Error("expected error for unknown format")
		}
	})
}

func TestSortKeys(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		check func(interface{}) bool
	}{
		{
			name: "map with string keys",
			input: map[string]interface{}{
				"z":  3,
				"a":  1,
				"m":  2,
				"zz": 4,
			},
			check: func(v interface{}) bool {
				sorted, ok := v.(map[string]interface{})
				if !ok {
					return false
				}
				if len(sorted) != 4 {
					return false
				}
				if sorted["z"] != 3 || sorted["a"] != 1 || sorted["m"] != 2 || sorted["zz"] != 4 {
					return false
				}
				return true
			},
		},
		{
			name: "slice of maps",
			input: []interface{}{
				map[string]interface{}{"c": 3, "a": 1},
				map[string]interface{}{"d": 4, "b": 2},
			},
			check: func(v interface{}) bool {
				slice, ok := v.([]interface{})
				if !ok || len(slice) != 2 {
					return false
				}
				return true
			},
		},
		{
			name:  "nested map",
			input: map[string]interface{}{"b": map[string]interface{}{"d": 2, "c": 1}, "a": 3},
			check: func(v interface{}) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SortKeys(tt.input)
			if !tt.check(result) {
				t.Error("SortKeys() did not sort correctly")
			}
		})
	}
}
