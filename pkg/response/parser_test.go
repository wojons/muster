package response

import (
	"testing"
)

func TestJSONParser_Parse(t *testing.T) {
	parser := JSONParser{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "valid JSON object",
			body:    []byte(`{"key":"value"}`),
			wantErr: false,
		},
		{
			name:    "valid JSON array",
			body:    []byte(`["a","b","c"]`),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    []byte(`not json`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.body)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONParser_CanParse(t *testing.T) {
	parser := JSONParser{}

	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/json-patch+json", true},
		{"text/plain", false},
		{"application/xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := parser.CanParse(tt.contentType); got != tt.want {
				t.Errorf("CanParse(%s) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestYAMLParser_Parse(t *testing.T) {
	parser := YAMLParser{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "valid YAML object",
			body:    []byte(`key: value`),
			wantErr: false,
		},
		{
			name:    "valid YAML array",
			body:    []byte(`- item1\n- item2`),
			wantErr: false,
		},
		{
			name:    "invalid YAML - unparseable",
			body:    []byte(`:unparseable yaml`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.body)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYAMLParser_CanParse(t *testing.T) {
	parser := YAMLParser{}

	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/x-yaml", true},
		{"text/yaml", true},
		{"application/yaml", true},
		{"application/yml", true},
		{"text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := parser.CanParse(tt.contentType); got != tt.want {
				t.Errorf("CanParse(%s) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestTextParser_Parse(t *testing.T) {
	parser := TextParser{}

	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "plain text",
			body: []byte("Hello, World!"),
		},
		{
			name: "empty text",
			body: []byte(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.body)

			if err != nil {
				t.Errorf("Parse() error = %v", err)
			}

			resultStr, ok := result.(string)
			if !ok {
				t.Error("expected string result")
			}

			if resultStr != string(tt.body) {
				t.Errorf("Parse() = %v, want %v", resultStr, string(tt.body))
			}
		})
	}
}

func TestTextParser_CanParse(t *testing.T) {
	parser := TextParser{}

	tests := []struct {
		contentType string
	}{
		{"text/plain"},
		{"application/json"},
		{"application/xml"},
		{"text/html"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if !parser.CanParse(tt.contentType) {
				t.Errorf("TextParser should always be able to parse, but CanParse(%s) = false", tt.contentType)
			}
		})
	}
}

func TestCSVParser_Parse(t *testing.T) {
	parser := CSVParser{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "valid CSV",
			body:    []byte("name,age\nJohn,30\nJane,25"),
			wantErr: false,
		},
		{
			name:    "empty CSV",
			body:    []byte(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.body)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && result != nil {
				records, ok := result.([]map[string]string)
				if !ok {
					t.Error("expected []map[string]string result")
				}

				if !tt.wantErr && len(records) != 2 {
					t.Errorf("expected 2 records, got %d", len(records))
				}
			}
		})
	}
}

func TestCSVParser_CanParse(t *testing.T) {
	parser := CSVParser{}

	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/csv", true},
		{"application/csv", true},
		{"text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := parser.CanParse(tt.contentType); got != tt.want {
				t.Errorf("CanParse(%s) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestAutoParser_Parse(t *testing.T) {
	parser := AutoParser{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "valid JSON",
			body:    []byte(`{"key":"value"}`),
			wantErr: false,
		},
		{
			name:    "valid YAML",
			body:    []byte("key: value"),
			wantErr: false,
		},
		{
			name:    "plain text",
			body:    []byte("Hello, World!"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.body)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewContentParser(t *testing.T) {
	config := Config{
		DefaultFormat: "json",
	}

	parser := NewContentParser(config)

	if parser == nil {
		t.Fatal("parser should not be nil")
	}

	if _, ok := parser.(*AutoParser); !ok {
		t.Error("expected AutoParser, got different type")
	}
}
