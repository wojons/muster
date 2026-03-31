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
	verbose   bool
	allowlist map[string]bool
}

func NewErrorHandler(verbose bool) *ErrorHandler {
	return &ErrorHandler{
		verbose:   verbose,
		allowlist: make(map[string]bool),
	}
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

var sensitiveDetailKeys = map[string]bool{
	"password":          true,
	"secret":            true,
	"api_key":           true,
	"apikey":            true,
	"token":             true,
	"access_token":      true,
	"refresh_token":     true,
	"private_key":       true,
	"credential":        true,
	"credentials":       true,
	"authorization":     true,
	"auth":              true,
	"bearer":            true,
	"jwt":               true,
	"session":           true,
	"cookie":            true,
	"x-api-key":         true,
	"x-auth-token":      true,
	"client_secret":     true,
	"encryption_key":    true,
	"master_key":        true,
	"signing_key":       true,
	"ssh_key":           true,
	"rsa_key":           true,
	"privatekey":        true,
	"secret_key":        true,
	"secretkey":         true,
	"aws_secret":        true,
	"aws_access_key":    true,
	"database_url":      true,
	"db_password":       true,
	"db_user":           true,
	"connection_string": true,
}

var allowedDetailKeys = map[string]bool{
	"field":            true,
	"reason":           true,
	"validation_error": true,
	"missing_field":    true,
	"invalid_value":    true,
	"expected":         true,
	"received":         true,
	"minimum":          true,
	"maximum":          true,
	"pattern":          true,
	"format":           true,
	"type":             true,
	"count":            true,
	"limit":            true,
	"offset":           true,
	"page":             true,
	"total":            true,
	"id":               true,
	"name":             true,
	"status":           true,
	"code":             true,
	"message":          true,
	"description":      true,
	"timestamp":        true,
	"duration":         true,
	"size":             true,
	"version":          true,
}

func (h *ErrorHandler) isSensitiveDetail(key string) bool {
	keyLower := strings.ToLower(key)
	if h.allowlist[keyLower] {
		return false
	}
	for sensitive := range sensitiveDetailKeys {
		if strings.Contains(keyLower, sensitive) {
			return true
		}
	}
	return false
}

func (h *ErrorHandler) isAllowedDetail(key string) bool {
	keyLower := strings.ToLower(key)
	if h.allowlist[keyLower] {
		return true
	}
	for allowed := range allowedDetailKeys {
		if strings.Contains(keyLower, allowed) {
			return true
		}
	}
	return false
}

func (h *ErrorHandler) AddToAllowlist(key string) {
	h.allowlist[strings.ToLower(key)] = true
}

func (h *ErrorHandler) RemoveFromAllowlist(key string) {
	delete(h.allowlist, strings.ToLower(key))
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
			if h.isSensitiveDetail(key) {
				parts = append(parts, fmt.Sprintf("  %s: [REDACTED]", key))
			} else if h.isAllowedDetail(key) {
				parts = append(parts, fmt.Sprintf("  %s: %v", key, value))
			}
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
