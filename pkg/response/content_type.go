package response

import (
	"mime"
	"net/http"
	"strings"
)

type ContentTypeDetector struct {
	typeMap map[string]string
}

func NewContentTypeDetector() *ContentTypeDetector {
	return &ContentTypeDetector{
		typeMap: map[string]string{
			"application/json":         "json",
			"application/xml":          "xml",
			"application/x-yaml":       "yaml",
			"text/yaml":                "yaml",
			"text/plain":               "text",
			"text/html":                "html",
			"text/csv":                 "csv",
			"application/octet-stream": "binary",
		},
	}
}

func (d *ContentTypeDetector) Detect(resp *http.Response) string {
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			if format, ok := d.typeMap[mediaType]; ok {
				return format
			}
		}
	}

	if format := d.detectFromPath(resp.Request.URL.Path); format != "" {
		return format
	}

	return d.detectFromContent(resp)
}

func (d *ContentTypeDetector) detectFromPath(path string) string {
	extMap := map[string]string{
		".json": "json",
		".xml":  "xml",
		".yaml": "yaml",
		".yml":  "yaml",
		".csv":  "csv",
		".txt":  "text",
		".html": "html",
	}

	for ext, format := range extMap {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return format
		}
	}

	return ""
}

func (d *ContentTypeDetector) detectFromContent(resp *http.Response) string {
	return "text"
}

func (d *ContentTypeDetector) GetParser(format string) ContentParser {
	switch format {
	case "json":
		return &JSONParser{}
	case "xml":
		return &XMLParser{}
	case "yaml":
		return &YAMLParser{}
	case "csv":
		return &CSVParser{}
	default:
		return &TextParser{}
	}
}
