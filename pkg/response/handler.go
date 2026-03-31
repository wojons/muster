// Package response provides HTTP response processing, parsing, formatting, and error handling for CLI output.
// It handles content-type detection, response parsing, output formatting (JSON, YAML, table, plain),
// streaming responses, pagination, filtering, and comprehensive error message generation.
//
// # Overview
//
// The response package is the final stage in the HTTP request-response cycle, transforming raw HTTP
// responses into user-friendly output. It supports:
//
//   - Automatic content-type detection (JSON, YAML, XML, HTML, plain text, binary)
//   - Multiple output formats (JSON, YAML, table, CSV, plain, raw)
//   - Streaming response processing (SSE, NDJSON, line-by-line, chunked)
//   - Pagination handling and navigation
//   - Field filtering and projection
//   - Human-readable error message generation with sensitive data redaction
//   - Syntax highlighting and colorization
//
// # Architecture
//
// The package consists of four main components:
//
// 1. Handler: Main entry point for response processing
// 2. Processor: Advanced response processing with pagination and streaming
// 3. Formatter: Output formatting in multiple formats (JSON, YAML, table, etc.)
// 4. ErrorHandler: Human-readable error message generation with redaction
//
// # Usage
//
// Basic response handling:
//
//	handler := response.NewHandler(response.Config{
//	    DefaultFormat:    "json",
//	    SupportedFormats: []string{"json", "yaml", "table"},
//	    VerboseErrors:    true,
//	})
//
//	resp, err := http.Get("https://api.example.com/pets")
//	if err != nil {
//	    return err
//	}
//
//	err = handler.Handle(resp)
//	if err != nil {
//	    return err
//	}
//
// Streaming response processing:
//
//	processor := response.NewProcessor(response.Config{
//	    DefaultFormat: "json",
//	})
//
//	err := processor.ProcessStream(resp.Body, func(event response.StreamEvent) error {
//	    fmt.Printf("Received: %+v\n", event)
//	    return nil
//	})
//
// Error handling with redaction:
//
//	errorHandler := response.NewErrorHandler(true) // verbose mode
//	appErr := errors.NewCode(errors.CodeValidation, "invalid email")
//	message := errorHandler.FormatError(appErr)
//	fmt.Println(message) // "Validation error: invalid email"
//
// # Content Type Detection
//
// The package automatically detects response content types:
//
//   - application/json → JSON parser
//   - application/yaml, text/yaml → YAML parser
//   - application/xml → XML parser
//   - text/html → HTML parser
//   - text/plain → Plain text parser
//   - application/octet-stream → Binary handler
//
// # Output Formats
//
// Supported output formats:
//
//   - JSON: Pretty-printed with syntax highlighting
//   - YAML: Clean YAML output
//   - Table: Columnar format for arrays of objects
//   - CSV: Comma-separated values
//   - Plain: Human-readable key-value pairs
//   - Raw: Unprocessed response body
//
// # Streaming Protocols
//
// The processor supports multiple streaming protocols:
//
//   - Server-Sent Events (SSE): event/data streams with retry handling
//   - NDJSON: Newline-delimited JSON streams
//   - Line-by-line: Text streams with line buffering
//   - Chunked: HTTP chunked transfer encoding
//
// # Error Redaction
//
// The ErrorHandler automatically redacts sensitive information:
//
//   - Passwords, secrets, tokens, API keys
//   - Credentials, private keys, connection strings
//   - 35+ sensitive field patterns detected and redacted
//
// Sensitive values are replaced with "[REDACTED]" marker.
//
// # Thread Safety
//
// All components are stateless and thread-safe for concurrent use.
package response

import (
	"io"
)

type Config struct {
	DefaultFormat    string
	SupportedFormats []string
	ContentTypeMap   map[string]string
	DefaultType      string
	ErrorFormat      string
	VerboseErrors    bool
	IncludeHeaders   bool
	RawOutput        bool
	Pagination       PaginationConfig
	Filtering        FilterConfig
	Writer           io.Writer
}

type PaginationConfig struct {
	Enabled     bool
	LimitParam  string
	OffsetParam string
	PageParam   string
	SizeParam   string
	TotalHeader string
	NextHeader  string
}

type FilterConfig struct {
	Enabled       bool
	Fields        []string
	ExcludeFields []string
}

type Handler struct {
	config    Config
	parser    ContentParser
	formatter Formatter
}

func NewHandler(config Config) *Handler {
	return &Handler{
		config:    config,
		parser:    NewContentParser(config),
		formatter: NewFormatter(config),
	}
}

func (h *Handler) SetWriter(w io.Writer) {
	h.config.Writer = w
}
