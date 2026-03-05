package response

import (
	"net/http"
	"net/url"
	"testing"
)

func TestContentTypeDetector_Detect(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		want     string
	}{
		{
			name: "JSON from Content-Type",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
			},
			want: "json",
		},
		{
			name: "XML from Content-Type",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"application/xml"},
				},
			},
			want: "xml",
		},
		{
			name: "YAML from Content-Type",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"text/yaml"},
				},
			},
			want: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewContentTypeDetector()

			if tt.response.Request == nil {
				tt.response.Request = &http.Request{
					URL: &url.URL{},
				}
			}

			got := detector.Detect(tt.response)

			if got != tt.want {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContentTypeDetector_detectFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/users.json", "json"},
		{"/api/data.xml", "xml"},
		{"/config/settings.yaml", "yaml"},
		{"/data/export.csv", "csv"},
	}

	detector := NewContentTypeDetector()

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detector.detectFromPath(tt.path)

			if got != tt.want {
				t.Errorf("detectFromPath(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestContentTypeDetector_GetParser(t *testing.T) {
	detector := NewContentTypeDetector()

	tests := []struct {
		format string
	}{
		{"json"},
		{"xml"},
		{"yaml"},
		{"csv"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			parser := detector.GetParser(tt.format)

			if parser == nil {
				t.Fatal("parser should not be nil")
			}
		})
	}
}
