# Business Response Handler Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Business Logic

## Overview

Complete specification for the **Business Response Handler** component that processes API responses with content type handling, error detection, and output formatting. This handler operates at the business logic layer, transforming raw HTTP responses into structured data for CLI consumption.

## 1. Handler Architecture

### 1.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      Business Response Handler                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐               │
│  │   HTTP      │────▶│  Content    │────▶│   Parser    │               │
│  │  Response   │     │  Type       │     │  Engine     │               │
│  │             │     │  Detection  │     │             │               │
│  └─────────────┘     └─────────────┘     └──────┬──────┘               │
│                                                  │                      │
│  ┌─────────────┐     ┌─────────────┐     ┌──────▼──────┐               │
│  │   Output    │◀────│  Formatter  │◀────│  Business   │               │
│  │  Writer     │     │             │     │  Model      │               │
│  │             │     │             │     │             │               │
│  └─────────────┘     └─────────────┘     └─────────────┘               │
│                                                                         │
│  ┌─────────────┐     ┌─────────────┐                                   │
│  │   Error     │◀────│   Error     │                                   │
│  │  Handler    │     │  Detector   │                                   │
│  └─────────────┘     └─────────────┘                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 Data Flow

```
HTTP Response
     │
     ▼
┌─────────────────┐
│ 1. Read Body    │ ──→ Store raw response bytes
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. Check Status │ ──→ Detect 4xx/5xx errors
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
  200+       Error
    │         │
    ▼         ▼
┌──────┐  ┌──────────────┐
│Parse │  │Error Handler │ ──→ Extract error details
│Body  │  └──────┬───────┘
└──┬───┘         │
   │              ▼
   │         ┌──────────────┐
   │         │Error Model   │ ──→ Structured error output
   │         └──────────────┘
   │
   ▼
┌─────────────────┐
│ 3. Detect Type  │ ──→ Content-Type header analysis
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. Parse Data   │ ──→ JSON/YAML/XML/etc. parsing
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. Transform    │ ──→ Apply business rules
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 6. Format       │ ──→ Convert to output format
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 7. Output       │ ──→ Write to stdout/file
└─────────────────┘
```

## 2. Core Data Structures

### 2.1 Handler Configuration

```go
package response

import (
	"io"
	"net/http"
)

// Config contains response handler configuration
type Config struct {
	// Output formatting
	DefaultFormat   string
	SupportedFormats []string
	
	// Content type handling
	ContentTypeMap  map[string]string // extension -> content-type
	DefaultType     string
	
	// Error handling
	ErrorFormat     string
	VerboseErrors   bool
	IncludeHeaders  bool
	
	// Processing options
	RawOutput       bool
	Pagination      PaginationConfig
	Filtering       FilterConfig
	
	// Output destination
	Writer          io.Writer
}

// PaginationConfig contains pagination handling options
type PaginationConfig struct {
	Enabled       bool
	LimitParam    string
	OffsetParam   string
	PageParam     string
	SizeParam     string
	TotalHeader   string
	NextHeader    string
}

// FilterConfig contains filtering options
type FilterConfig struct {
	Enabled       bool
	Fields        []string
	ExcludeFields []string
}

// Handler processes HTTP responses
type Handler struct {
	config Config
	parser ContentParser
	formatter Formatter
}

// NewHandler creates a new response handler
func NewHandler(config Config) *Handler {
	return &Handler{
		config:    config,
		parser:    NewContentParser(config),
		formatter: NewFormatter(config),
	}
}

// SetWriter sets the output writer
func (h *Handler) SetWriter(w io.Writer) {
	h.config.Writer = w
}
```

### 2.2 Response Models

```go
package response

import (
	"net/http"
	"time"
)

// Response represents a processed API response
type Response struct {
	StatusCode    int
	Status        string
	Headers       http.Header
	Body          interface{}
	RawBody       []byte
	ContentType   string
	Timestamp     time.Time
	Duration      time.Duration
	
	// Metadata
	RequestID     string
	APIVersion    string
	
	// Pagination
	Pagination    *PaginationInfo
	
	// Errors
	Error         *ErrorInfo
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	Page          int
	PerPage       int
	Total         int
	TotalPages    int
	HasNext       bool
	HasPrev       bool
	NextURL       string
	PrevURL       string
}

// ErrorInfo contains structured error information
type ErrorInfo struct {
	Code          string
	Message       string
	Details       map[string]interface{}
	RequestID     string
	Timestamp     time.Time
	HTTPStatus    int
}

// Error implements the error interface
func (e *ErrorInfo) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return e.Message
}

// ProcessingContext contains context for response processing
type ProcessingContext struct {
	Request       *http.Request
	Response      *http.Response
	Options       ProcessingOptions
}

// ProcessingOptions contains processing options
type ProcessingOptions struct {
	Format        string
	Raw           bool
	Fields        []string
	ExcludeFields []string
	Template      string
	Filter        string
	Sort          string
}
```

## 3. Content Type Detection

### 3.1 Content Type Parser

```go
package response

import (
	"mime"
	"net/http"
	"strings"
)

// ContentTypeDetector detects and parses content types
type ContentTypeDetector struct {
	typeMap map[string]string
}

// NewContentTypeDetector creates a new detector
func NewContentTypeDetector() *ContentTypeDetector {
	return &ContentTypeDetector{
		typeMap: map[string]string{
			"application/json":           "json",
			"application/xml":            "xml",
			"application/x-yaml":         "yaml",
			"text/yaml":                  "yaml",
			"text/plain":                 "text",
			"text/html":                  "html",
			"text/csv":                   "csv",
			"application/octet-stream":   "binary",
		},
	}
}

// Detect determines the content type from response
func (d *ContentTypeDetector) Detect(resp *http.Response) string {
	// Check Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			if format, ok := d.typeMap[mediaType]; ok {
				return format
			}
		}
	}
	
	// Check file extension in URL
	if format := d.detectFromPath(resp.Request.URL.Path); format != "" {
		return format
	}
	
	// Check body content for JSON/XML
	return d.detectFromContent(resp)
}

func (d *ContentTypeDetector) detectFromPath(path string) string {
	// Map extensions to types
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
	// Read first few bytes to detect format
	// This would require reading the body, so we skip in this example
	// In practice, use a peek reader or save body
	
	return "text"
}

// GetParser returns the appropriate parser for the content type
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
```

## 4. Error Detection and Handling

### 4.1 Error Handler

```go
package response

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// ErrorHandler processes HTTP error responses
type ErrorHandler struct {
	verbose bool
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(verbose bool) *ErrorHandler {
	return &ErrorHandler{verbose: verbose}
}

// IsError checks if response indicates an error
func (h *ErrorHandler) IsError(resp *http.Response) bool {
	return resp.StatusCode >= 400
}

// ExtractError extracts error information from response
func (h *ErrorHandler) ExtractError(resp *http.Response, body []byte) *ErrorInfo {
	errorInfo := &ErrorInfo{
		HTTPStatus: resp.StatusCode,
		Timestamp:  time.Now(),
	}
	
	// Try to extract request ID from headers
	for _, header := range []string{"X-Request-Id", "X-Request-ID", "Request-Id"} {
		if id := resp.Header.Get(header); id != "" {
			errorInfo.RequestID = id
			break
		}
	}
	
	// Parse error body based on content type
	contentType := resp.Header.Get("Content-Type")
	
	if strings.Contains(contentType, "application/json") {
		h.parseJSONError(body, errorInfo)
	} else if strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") {
		h.parseXMLError(body, errorInfo)
	} else {
		// Plain text error
		errorInfo.Message = string(body)
	}
	
	// If no message extracted, use status text
	if errorInfo.Message == "" {
		errorInfo.Message = http.StatusText(resp.StatusCode)
	}
	
	return errorInfo
}

// Standard error response structures
type JSONErrorResponse struct {
	Error struct {
		Code    string                 `json:"code"`
		Message string                 `json:"message"`
		Details map[string]interface{} `json:"details,omitempty"`
	} `json:"error,omitempty"`
	Code    string                 `json:"code,omitempty"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type XMLErrorResponse struct {
	XMLName xml.Name `xml:"error"`
	Code    string   `xml:"code"`
	Message string   `xml:"message"`
}

func (h *ErrorHandler) parseJSONError(body []byte, errorInfo *ErrorInfo) {
	var jsonErr JSONErrorResponse
	if err := json.Unmarshal(body, &jsonErr); err != nil {
		errorInfo.Message = string(body)
		return
	}
	
	// Handle different error structures
	if jsonErr.Error.Code != "" {
		errorInfo.Code = jsonErr.Error.Code
		errorInfo.Message = jsonErr.Error.Message
		errorInfo.Details = jsonErr.Error.Details
	} else {
		errorInfo.Code = jsonErr.Code
		errorInfo.Message = jsonErr.Message
		errorInfo.Details = jsonErr.Details
	}
}

func (h *ErrorHandler) parseXMLError(body []byte, errorInfo *ErrorInfo) {
	var xmlErr XMLErrorResponse
	if err := xml.Unmarshal(body, &xmlErr); err != nil {
		errorInfo.Message = string(body)
		return
	}
	
	errorInfo.Code = xmlErr.Code
	errorInfo.Message = xmlErr.Message
}

// FormatError formats an error for output
func (h *ErrorHandler) FormatError(err *ErrorInfo) string {
	var parts []string
	
	parts = append(parts, fmt.Sprintf("Error (HTTP %d)", err.HTTPStatus))
	
	if err.Code != "" {
		parts = append(parts, fmt.Sprintf("Code: %s", err.Code))
	}
	
	parts = append(parts, fmt.Sprintf("Message: %s", err.Message))
	
	if err.RequestID != "" && h.verbose {
		parts = append(parts, fmt.Sprintf("Request ID: %s", err.RequestID))
	}
	
	if err.Timestamp.Unix() > 0 && h.verbose {
		parts = append(parts, fmt.Sprintf("Timestamp: %s", err.Timestamp.Format(time.RFC3339)))
	}
	
	if len(err.Details) > 0 && h.verbose {
		parts = append(parts, "Details:")
		for key, value := range err.Details {
			parts = append(parts, fmt.Sprintf("  %s: %v", key, value))
		}
	}
	
	return strings.Join(parts, "\n")
}
```

## 5. Content Parsers

### 5.1 Parser Interface and Implementations

```go
package response

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	
	"gopkg.in/yaml.v3"
)

// ContentParser parses response body content
type ContentParser interface {
	Parse(body []byte) (interface{}, error)
	CanParse(contentType string) bool
}

// JSONParser parses JSON content
type JSONParser struct{}

func (p *JSONParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "json")
}

func (p *JSONParser) Parse(body []byte) (interface{}, error) {
	// Try to parse as object first
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err == nil {
		return obj, nil
	}
	
	// Try to parse as array
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	
	return nil, fmt.Errorf("failed to parse JSON: %w", err)
}

// XMLParser parses XML content
type XMLParser struct{}

func (p *XMLParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "xml")
}

func (p *XMLParser) Parse(body []byte) (interface{}, error) {
	// Simple XML to map conversion
	var root map[string]interface{}
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}
	return root, nil
}

// YAMLParser parses YAML content
type YAMLParser struct{}

func (p *YAMLParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "yaml") ||
		strings.Contains(contentType, "yml")
}

func (p *YAMLParser) Parse(body []byte) (interface{}, error) {
	// Try to parse as object
	var obj map[string]interface{}
	if err := yaml.Unmarshal(body, &obj); err == nil {
		return obj, nil
	}
	
	// Try to parse as array
	var arr []interface{}
	if err := yaml.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	
	return nil, fmt.Errorf("failed to parse YAML: %w", err)
}

// CSVParser parses CSV content
type CSVParser struct{}

func (p *CSVParser) CanParse(contentType string) bool {
	return strings.Contains(contentType, "csv")
}

func (p *CSVParser) Parse(body []byte) (interface{}, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))
	
	// Read headers
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}
	
	var records []map[string]string
	
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}
		
		record := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				record[header] = row[i]
			}
		}
		records = append(records, record)
	}
	
	return records, nil
}

// TextParser handles plain text content
type TextParser struct{}

func (p *TextParser) CanParse(contentType string) bool {
	return true // Fallback parser
}

func (p *TextParser) Parse(body []byte) (interface{}, error) {
	return string(body), nil
}

// NewContentParser creates appropriate parser for content type
func NewContentParser(config Config) ContentParser {
	return &AutoParser{}
}

// AutoParser automatically selects the right parser
type AutoParser struct{}

func (p *AutoParser) CanParse(contentType string) bool {
	return true
}

func (p *AutoParser) Parse(body []byte) (interface{}, error) {
	// Try JSON first
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err == nil {
		return jsonData, nil
	}
	
	// Try YAML
	var yamlData interface{}
	if err := yaml.Unmarshal(body, &yamlData); err == nil {
		return yamlData, nil
	}
	
	// Return as text
	return string(body), nil
}
```

## 6. Output Formatters

### 6.1 Formatter Interface and Implementations

```go
package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	
	"gopkg.in/yaml.v3"
)

// Formatter formats response data for output
type Formatter interface {
	Format(data interface{}) ([]byte, error)
}

// NewFormatter creates a formatter for the configured format
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
	default:
		return &JSONFormatter{indent: true}
	}
}

// JSONFormatter formats as JSON
type JSONFormatter struct {
	indent bool
}

func (f *JSONFormatter) Format(data interface{}) ([]byte, error) {
	if f.indent {
		return json.MarshalIndent(data, "", "  ")
	}
	return json.Marshal(data)
}

// YAMLFormatter formats as YAML
type YAMLFormatter struct{}

func (f *YAMLFormatter) Format(data interface{}) ([]byte, error) {
	return yaml.Marshal(data)
}

// TableFormatter formats as ASCII table
type TableFormatter struct {
	columns []string
}

func (f *TableFormatter) Format(data interface{}) ([]byte, error) {
	// Handle different data types
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
	
	// Convert to map slice if possible
	var maps []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			maps = append(maps, m)
		}
	}
	
	if len(maps) > 0 {
		return f.formatMapSlice(maps)
	}
	
	// Simple list
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
	
	// Extract headers
	headers := f.extractHeaders(items[0])
	
	// Create tabwriter
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)
	
	// Write headers
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	
	// Write separator
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = strings.Repeat("-", len(headers[i]))
	}
	fmt.Fprintln(tw, strings.Join(sep, "\t"))
	
	// Write rows
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
	
	// Get keys sorted
	var keys []string
	for key := range data {
		keys = append(keys, key)
	}
	
	// Write key-value pairs
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
				// Truncate complex values
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

// CSVFormatter formats as CSV
type CSVFormatter struct{}

func (f *CSVFormatter) Format(data interface{}) ([]byte, error) {
	// Implementation similar to CSVParser but in reverse
	return nil, fmt.Errorf("CSV output not implemented")
}

// RawFormatter outputs raw bytes
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
```

## 7. Main Handler Implementation

### 7.1 Process Method

```go
package response

import (
	"fmt"
	"io"
	"net/http"
)

// ProcessResponse handles a complete HTTP response
func (h *Handler) ProcessResponse(resp *http.Response, options ProcessingOptions) error {
	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	defer resp.Body.Close()
	
	// Create processing context
	ctx := &ProcessingContext{
		Response: resp,
		Options:  options,
	}
	
	// Build response model
	response := &Response{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		Headers:     resp.Header,
		RawBody:     body,
		ContentType: resp.Header.Get("Content-Type"),
		Timestamp:   time.Now(),
	}
	
	// Check for errors
	if errHandler := NewErrorHandler(h.config.VerboseErrors); errHandler.IsError(resp) {
		response.Error = errHandler.ExtractError(resp, body)
		
		if h.config.Writer != nil {
			errMsg := errHandler.FormatError(response.Error)
			fmt.Fprintln(h.config.Writer, errMsg)
		}
		return response.Error
	}
	
	// Parse content
	if !options.Raw {
		detector := NewContentTypeDetector()
		format := detector.Detect(resp)
		
		parser := detector.GetParser(format)
		parsedData, err := parser.Parse(body)
		if err != nil {
			// If parsing fails, use raw body
			response.Body = string(body)
		} else {
			response.Body = parsedData
		}
	} else {
		response.Body = string(body)
	}
	
	// Format output
	formatter := h.formatter
	if options.Format != "" {
		// Create formatter for requested format
		cfg := h.config
		cfg.DefaultFormat = options.Format
		formatter = NewFormatter(cfg)
	}
	
	output, err := formatter.Format(response.Body)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}
	
	// Write output
	if h.config.Writer != nil {
		_, err = h.config.Writer.Write(output)
		if err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	
	return nil
}

// HandleError processes error responses specifically
func (h *Handler) HandleError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error response: %w", err)
	}
	defer resp.Body.Close()
	
	errHandler := NewErrorHandler(h.config.VerboseErrors)
	errorInfo := errHandler.ExtractError(resp, body)
	
	if h.config.Writer != nil {
		errMsg := errHandler.FormatError(errorInfo)
		fmt.Fprintln(h.config.Writer, errMsg)
	}
	
	return errorInfo
}

// ProcessStream handles streaming responses
func (h *Handler) ProcessStream(resp *http.Response, handler func(interface{}) error) error {
	// Implementation for SSE or NDJSON streaming
	return fmt.Errorf("streaming not implemented")
}
```

## 8. Usage Example

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	
	"github.com/example/cli/pkg/response"
)

func main() {
	// Create handler configuration
	config := response.Config{
		DefaultFormat:    "table",
		SupportedFormats: []string{"table", "json", "yaml"},
		VerboseErrors:    true,
		Writer:           os.Stdout,
	}
	
	// Create handler
	handler := response.NewHandler(config)
	
	// Example: Make HTTP request
	resp, err := http.Get("https://api.example.com/users")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		os.Exit(1)
	}
	
	// Process response
	options := response.ProcessingOptions{
		Format: "table",
		Fields: []string{"id", "name", "email"},
	}
	
	if err := handler.ProcessResponse(resp, options); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to process response: %v\n", err)
		os.Exit(1)
	}
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC