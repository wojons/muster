package response

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ErrorHandler struct {
	verbose bool
}

func NewErrorHandler(verbose bool) *ErrorHandler {
	return &ErrorHandler{verbose: verbose}
}

func (h *ErrorHandler) IsError(resp *http.Response) bool {
	return resp.StatusCode >= 400
}

func (h *ErrorHandler) ExtractError(resp *http.Response, body []byte) *ErrorInfo {
	errorInfo := &ErrorInfo{
		HTTPStatus: resp.StatusCode,
		Timestamp:  time.Now(),
	}

	for _, header := range []string{"X-Request-Id", "X-Request-ID", "Request-Id"} {
		if id := resp.Header.Get(header); id != "" {
			errorInfo.RequestID = id
			break
		}
	}

	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
		h.parseJSONError(body, errorInfo)
	} else if strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") {
		h.parseXMLError(body, errorInfo)
	} else {
		errorInfo.Message = string(body)
	}

	if errorInfo.Message == "" {
		errorInfo.Message = http.StatusText(resp.StatusCode)
	}

	return errorInfo
}

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

type ErrorInfo struct {
	Code       string
	Message    string
	Details    map[string]interface{}
	RequestID  string
	Timestamp  time.Time
	HTTPStatus int
}

func (e *ErrorInfo) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return e.Message
}
