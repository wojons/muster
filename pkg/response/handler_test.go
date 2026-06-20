package response

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewHandler(t *testing.T) {
	cfg := Config{
		DefaultFormat:    "json",
		SupportedFormats: []string{"json", "yaml"},
		VerboseErrors:    false,
	}
	h := NewHandler(cfg)

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if !h.TreatNon200AsError {
		t.Error("expected TreatNon200AsError = true by default")
	}
}

func TestNewHandler_DefaultsTreatNon200(t *testing.T) {
	// NewHandler should always set TreatNon200AsError to true
	h := NewHandler(Config{})
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if !h.TreatNon200AsError {
		t.Error("expected TreatNon200AsError = true")
	}
}

func TestHandler_HandleResponse_NilResponse(t *testing.T) {
	h := NewHandler(Config{})
	body, data, err := h.HandleResponse(nil)

	if err == nil {
		t.Fatal("expected error for nil response, got nil")
	}
	if body != nil {
		t.Errorf("expected nil body, got %v", body)
	}
	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
	if err.Error() != "nil response" {
		t.Errorf("expected 'nil response' error, got %q", err.Error())
	}
}

func TestHandler_HandleResponse_SuccessJSON(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"key":"value","num":42}`)),
	}

	body, data, err := h.HandleResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == nil {
		t.Fatal("expected non-nil body")
	}
	if string(body) != `{"key":"value","num":42}` {
		t.Errorf("expected body %q, got %q", `{"key":"value","num":42}`, string(body))
	}
	if data == nil {
		t.Fatal("expected parsed JSON data")
	}
	if data["key"] != "value" {
		t.Errorf("expected data['key'] = 'value', got %v", data["key"])
	}
}

func TestHandler_HandleResponse_SuccessNonJSON(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("plain text response")),
	}

	body, data, err := h.HandleResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "plain text response" {
		t.Errorf("expected body %q, got %q", "plain text response", string(body))
	}
	// Non-JSON body should return nil data
	if data != nil {
		t.Errorf("expected nil data for non-JSON, got %v", data)
	}
}

func TestHandler_HandleResponse_EmptyBody(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("")),
	}

	body, data, err := h.HandleResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty body is valid JSON? json.Unmarshal of empty string returns "unexpected end of JSON input"
	// So it should fall through to non-JSON path
	if data != nil {
		t.Errorf("empty body should not parse as JSON, got %v", data)
	}
	_ = body
}

func TestHandler_HandleResponse_ErrorStatusCode(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
	}

	body, data, err := h.HandleResponse(resp)
	if err == nil {
		t.Fatal("expected error for 404 status")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", httpErr.StatusCode)
	}
	if httpErr.Status != "404 Not Found" {
		t.Errorf("expected Status '404 Not Found', got %q", httpErr.Status)
	}
	if string(httpErr.Body) != `{"error":"not found"}` {
		t.Errorf("expected body %q, got %q", `{"error":"not found"}`, string(httpErr.Body))
	}
	// Body should still be returned alongside the error
	if string(body) != `{"error":"not found"}` {
		t.Errorf("expected body returned, got %q", string(body))
	}
	// data should be nil for error responses (JSON parsing is skipped)
	if data != nil {
		t.Errorf("expected nil data for error response, got %v", data)
	}
}

func TestHandler_HandleResponse_500Error(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
	}

	_, _, err := h.HandleResponse(resp)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", httpErr.StatusCode)
	}
}

func TestHandler_HandleResponse_400Error(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(`{"message":"invalid input"}`)),
	}

	_, _, err := h.HandleResponse(resp)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}

func TestHandler_HandleResponse_TreatNon200Disabled(t *testing.T) {
	h := NewHandler(Config{})
	h.TreatNon200AsError = false

	resp := &http.Response{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
	}

	body, data, err := h.HandleResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error when TreatNon200AsError=false: %v", err)
	}
	if string(body) != `{"error":"not found"}` {
		t.Errorf("expected body %q, got %q", `{"error":"not found"}`, string(body))
	}
	if data["error"] != "not found" {
		t.Errorf("expected data['error'] = 'not found', got %v", data["error"])
	}
}

func TestHandler_HandleResponse_401Error(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 401,
		Status:     "401 Unauthorized",
		Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
	}

	_, _, err := h.HandleResponse(resp)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("expected StatusCode 401, got %d", httpErr.StatusCode)
	}
}

func TestHandler_HandleResponse_ReadError(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(&errorReader{err: errors.New("simulated read error")}),
	}

	_, _, err := h.HandleResponse(resp)
	if err == nil {
		t.Fatal("expected error for read failure")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("expected 'read response body' in error, got %q", err.Error())
	}
}

func TestProcessResponse(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}

	err := h.ProcessResponse(resp, ProcessingOptions{Format: "json"})
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}
}

func TestProcessResponse_WithError(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Body:       io.NopCloser(strings.NewReader("boom")),
	}

	err := h.ProcessResponse(resp, ProcessingOptions{})
	if err == nil {
		t.Fatal("expected error from ProcessResponse with 500 status")
	}
}

func TestProcessResponse_WithRawOption(t *testing.T) {
	h := NewHandler(Config{})
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("raw content")),
	}

	err := h.ProcessResponse(resp, ProcessingOptions{Raw: true})
	if err != nil {
		t.Fatalf("ProcessResponse with Raw=true failed: %v", err)
	}
}

func TestHTTPError_Error(t *testing.T) {
	e := &HTTPError{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       []byte("resource not found"),
	}

	msg := e.Error()
	expected := "HTTP 404 404 Not Found: resource not found"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestHTTPError_Error_EmptyBody(t *testing.T) {
	e := &HTTPError{
		StatusCode: 503,
		Status:     "503 Service Unavailable",
		Body:       nil,
	}

	msg := e.Error()
	expected := "HTTP 503 503 Service Unavailable: "
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{
		DefaultFormat: "json",
	}
	if cfg.DefaultFormat != "json" {
		t.Errorf("expected DefaultFormat 'json', got %q", cfg.DefaultFormat)
	}
}

func TestProcessingOptions_Defaults(t *testing.T) {
	opts := ProcessingOptions{}
	if opts.Format != "" {
		t.Errorf("expected empty Format, got %q", opts.Format)
	}
	if opts.Raw {
		t.Error("expected Raw = false")
	}
}

// errorReader is a reader that always returns an error.
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

// Verify errorReader implements io.Reader
var _ io.Reader = (*errorReader)(nil)

func TestHandler_HandleResponse_LargeJSON(t *testing.T) {
	h := NewHandler(Config{})
	// Build a large JSON array
	var buf bytes.Buffer
	buf.WriteString(`{"items":[`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(`{"id":`)
		buf.WriteString(string(rune('0' + i%10)))
		buf.WriteString(`,"name":"item`)
		buf.WriteString(string(rune('0' + i%10)))
		buf.WriteString(`"}`)
	}
	buf.WriteString(`]}`)

	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	body, data, err := h.HandleResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == nil {
		t.Fatal("expected non-nil body")
	}
	if data == nil {
		t.Fatal("expected parsed JSON data")
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		t.Fatal("expected items to be an array")
	}
	if len(items) != 100 {
		t.Errorf("expected 100 items, got %d", len(items))
	}
}
