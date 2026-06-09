// Package response provides HTTP response handling for the Muster CLI and tools.
// It handles deserialization, error extraction, and output formatting.
package response

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Handler processes HTTP responses and extracts their content.
type Handler struct {
	// TreatNon200AsError determines whether non-2xx status codes are treated as errors.
	TreatNon200AsError bool
}

// Config configures response handling.
type Config struct {
	DefaultFormat    string
	SupportedFormats []string
	Writer           io.Writer
	VerboseErrors    bool
}

// ProcessingOptions controls how a response is processed.
type ProcessingOptions struct {
	Format string
	Raw    bool
}

// NewHandler creates a new response Handler with the given config.
func NewHandler(config Config) *Handler {
	return &Handler{
		TreatNon200AsError: true,
	}
}

// ProcessResponse processes an HTTP response.
func (h *Handler) ProcessResponse(resp *http.Response, opts ProcessingOptions) error {
	_, _, err := h.HandleResponse(resp)
	return err
}

// HandleResponse reads and processes an HTTP response.
// Returns the response body as bytes, the parsed JSON (if applicable),
// and any error encountered.
func (h *Handler) HandleResponse(resp *http.Response) ([]byte, map[string]interface{}, error) {
	if resp == nil {
		return nil, nil, fmt.Errorf("nil response")
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	if h.TreatNon200AsError && resp.StatusCode >= 400 {
		return body, nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       body,
		}
	}

	// Try to parse as JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Not JSON — return raw body
		return body, nil, nil
	}

	return body, data, nil
}

// HTTPError represents an HTTP error response.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s", e.StatusCode, e.Status, string(e.Body))
}
