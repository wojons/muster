package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestErrorHandler_IsError(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		want     bool
	}{
		{
			name:     "2XX status code",
			response: &http.Response{StatusCode: 200},
			want:     false,
		},
		{
			name:     "3XX status code",
			response: &http.Response{StatusCode: 301},
			want:     false,
		},
		{
			name:     "4XX error",
			response: &http.Response{StatusCode: 404},
			want:     true,
		},
		{
			name:     "5XX error",
			response: &http.Response{StatusCode: 500},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(false)

			got := handler.IsError(tt.response)

			if got != tt.want {
				t.Errorf("IsError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorHandler_ExtractError(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		body     []byte
		wantCode string
		wantMsg  string
	}{
		{
			name: "JSON error with nested error object",
			response: &http.Response{
				StatusCode: 404,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
			},
			body:     []byte(`{"error":{"code":"NOT_FOUND","message":"Resource not found"}}`),
			wantCode: "NOT_FOUND",
			wantMsg:  "Resource not found",
		},
		{
			name: "JSON error with top-level fields",
			response: &http.Response{
				StatusCode: 400,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
			},
			body:     []byte(`{"code":"BAD_REQUEST","message":"Missing required field"}`),
			wantCode: "BAD_REQUEST",
			wantMsg:  "Missing required field",
		},
		{
			name: "XML error",
			response: &http.Response{
				StatusCode: 500,
				Header: http.Header{
					"Content-Type": []string{"application/xml"},
				},
			},
			body:     []byte(`<error><code>SERVER_ERROR</code><message>Internal server error</message></error>`),
			wantCode: "SERVER_ERROR",
			wantMsg:  "Internal server error",
		},
		{
			name: "plain text error",
			response: &http.Response{
				StatusCode: 403,
				Header: http.Header{
					"Content-Type": []string{"text/plain"},
				},
			},
			body:     []byte("Forbidden"),
			wantCode: "",
			wantMsg:  "Forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(false)

			errorInfo := handler.ExtractError(tt.response, tt.body)

			if errorInfo.Code != tt.wantCode {
				t.Errorf("ExtractError().Code = %v, want %v", errorInfo.Code, tt.wantCode)
			}

			if errorInfo.Message != tt.wantMsg {
				t.Errorf("ExtractError().Message = %v, want %v", errorInfo.Message, tt.wantMsg)
			}

			if errorInfo.HTTPStatus != tt.response.StatusCode {
				t.Errorf("ExtractError().HTTPStatus = %v, want %v", errorInfo.HTTPStatus, tt.response.StatusCode)
			}
		})
	}
}

func TestErrorHandler_FormatError(t *testing.T) {
	tests := []struct {
		name      string
		verbose   bool
		errInfo   *ErrorInfo
		wantParts []string
	}{
		{
			name:    "basic error formatting",
			verbose: false,
			errInfo: &ErrorInfo{
				Code:       "NOT_FOUND",
				Message:    "Resource not found",
				HTTPStatus: 404,
			},
			wantParts: []string{
				"Error (HTTP 404)",
				"Code: NOT_FOUND",
				"Message: Resource not found",
			},
		},
		{
			name:    "verbose with RequestID",
			verbose: true,
			errInfo: &ErrorInfo{
				Code:       "SERVER_ERROR",
				Message:    "Internal server error",
				RequestID:  "req-12345",
				HTTPStatus: 500,
				Timestamp:  time.Now(),
			},
			wantParts: []string{
				"Error (HTTP 500)",
				"Code: SERVER_ERROR",
				"Message: Internal server error",
				"Request ID: req-12345",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(tt.verbose)

			result := handler.FormatError(tt.errInfo)

			for _, part := range tt.wantParts {
				if !contains(result, part) {
					t.Errorf("FormatError() should contain '%s', got:\n%s", part, result)
				}
			}
		})
	}
}

func TestErrorInfo_Error(t *testing.T) {
	tests := []struct {
		name    string
		errInfo *ErrorInfo
		want    string
	}{
		{
			name: "error with code",
			errInfo: &ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Resource not found",
			},
			want: "[NOT_FOUND] Resource not found",
		},
		{
			name: "error without code",
			errInfo: &ErrorInfo{
				Message: "Resource not found",
			},
			want: "Resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errInfo.Error()

			if result != tt.want {
				t.Errorf("Error() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestErrorHandler_ExtractError_WithRequestID(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		wantID  string
	}{
		{
			name: "X-Request-Id header",
			headers: http.Header{
				"X-Request-Id": []string{"req-12345"},
			},
			wantID: "req-12345",
		},
		{
			name: "X-Request-ID header",
			headers: http.Header{
				"X-Request-ID": []string{"req-67890"},
			},
			wantID: "req-67890",
		},
		{
			name: "Request-Id header",
			headers: http.Header{
				"Request-Id": []string{"req-11111"},
			},
			wantID: "req-11111",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			headers := http.Header{}
			for k, v := range tt.headers {
				headers.Set(k, v[0])
			}

			resp := &http.Response{
				Request:    req,
				StatusCode: 500,
				Header:     headers,
				Body:       newMockBody(`{"error":"test"}`),
			}

			handler := NewErrorHandler(false)
			errorInfo := handler.ExtractError(resp, []byte(`{"error":"test"}`))

			if errorInfo.RequestID != tt.wantID {
				t.Errorf("RequestID = %v, want %v", errorInfo.RequestID, tt.wantID)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || findInString(s, substr))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewErrorHandler(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
	}{
		{
			name:    "verbose handler",
			verbose: true,
		},
		{
			name:    "non-verbose handler",
			verbose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(tt.verbose)

			if handler == nil {
				t.Fatal("NewErrorHandler() should not return nil")
			}

			if handler.verbose != tt.verbose {
				t.Errorf("verbose = %v, want %v", handler.verbose, tt.verbose)
			}
		})
	}
}

func TestErrorHandler_FormatError_RedactsSensitiveDetails(t *testing.T) {
	tests := []struct {
		name       string
		details    map[string]interface{}
		wantRedact []string
	}{
		{
			name: "password field",
			details: map[string]interface{}{
				"password": "secret123",
				"username": "john",
			},
			wantRedact: []string{"password"},
		},
		{
			name: "token field",
			details: map[string]interface{}{
				"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
				"user_id":      "12345",
			},
			wantRedact: []string{"access_token"},
		},
		{
			name: "api_key field",
			details: map[string]interface{}{
				"api_key":  "sk-1234567890abcdef",
				"endpoint": "/api/v1/users",
				"method":   "GET",
			},
			wantRedact: []string{"api_key"},
		},
		{
			name: "multiple sensitive fields",
			details: map[string]interface{}{
				"password":      "secret123",
				"client_secret": "cs-abcdef123456",
				"username":      "john",
				"email":         "john@example.com",
			},
			wantRedact: []string{"password", "client_secret"},
		},
		{
			name: "case insensitive sensitive fields",
			details: map[string]interface{}{
				"PASSWORD":     "secret123",
				"Api_Key":      "sk-1234567890",
				"ACCESS_TOKEN": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			},
			wantRedact: []string{"PASSWORD", "Api_Key", "ACCESS_TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(true)

			errInfo := &ErrorInfo{
				Code:       "VALIDATION_ERROR",
				Message:    "Invalid input",
				HTTPStatus: 400,
				Details:    tt.details,
			}

			result := handler.FormatError(errInfo)

			for _, sensitiveField := range tt.wantRedact {
				if contains(result, tt.details[sensitiveField].(string)) {
					t.Errorf("FormatError() should redact sensitive field '%s', but result contains value:\n%s", sensitiveField, result)
				}
				if !contains(result, "[REDACTED]") {
					t.Errorf("FormatError() should contain '[REDACTED]' for sensitive field '%s', got:\n%s", sensitiveField, result)
				}
			}
		})
	}
}

func TestErrorHandler_FormatError_AllowedDetails(t *testing.T) {
	tests := []struct {
		name           string
		details        map[string]interface{}
		wantVisible    []string
		wantNotVisible []string
	}{
		{
			name: "safe validation error details",
			details: map[string]interface{}{
				"field":    "email",
				"reason":   "invalid_format",
				"expected": "valid email address",
				"received": "not-an-email",
				"password": "secret123",
			},
			wantVisible:    []string{"field", "email", "reason", "invalid_format", "expected", "received", "not-an-email"},
			wantNotVisible: []string{"secret123"},
		},
		{
			name: "pagination details",
			details: map[string]interface{}{
				"page":    1,
				"limit":   10,
				"total":   100,
				"offset":  0,
				"api_key": "sk-secret",
			},
			wantVisible:    []string{"page", "1", "limit", "10", "total", "100"},
			wantNotVisible: []string{"sk-secret"},
		},
		{
			name: "error metadata",
			details: map[string]interface{}{
				"id":         "err-12345",
				"status":     "failed",
				"type":       "validation",
				"timestamp":  "2026-03-31T12:00:00Z",
				"secret_key": "sk-abcdef",
			},
			wantVisible:    []string{"id", "err-12345", "status", "failed", "type", "validation"},
			wantNotVisible: []string{"sk-abcdef"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(true)

			errInfo := &ErrorInfo{
				Code:       "ERROR",
				Message:    "Test error",
				HTTPStatus: 400,
				Details:    tt.details,
			}

			result := handler.FormatError(errInfo)

			for _, visible := range tt.wantVisible {
				if !contains(result, visible) {
					t.Errorf("FormatError() should show allowed field '%s', got:\n%s", visible, result)
				}
			}

			for _, notVisible := range tt.wantNotVisible {
				if contains(result, notVisible) {
					t.Errorf("FormatError() should not show sensitive value '%s', got:\n%s", notVisible, result)
				}
			}
		})
	}
}

func TestErrorHandler_FormatError_NonVerboseMode(t *testing.T) {
	handler := NewErrorHandler(false)

	errInfo := &ErrorInfo{
		Code:       "ERROR",
		Message:    "Test error",
		HTTPStatus: 400,
		RequestID:  "req-12345",
		Timestamp:  time.Now(),
		Details: map[string]interface{}{
			"field":    "email",
			"password": "secret",
		},
	}

	result := handler.FormatError(errInfo)

	if contains(result, "Request ID:") {
		t.Errorf("FormatError() should not show Request ID in non-verbose mode, got:\n%s", result)
	}

	if contains(result, "Timestamp:") {
		t.Errorf("FormatError() should not show Timestamp in non-verbose mode, got:\n%s", result)
	}

	if contains(result, "Details:") {
		t.Errorf("FormatError() should not show Details in non-verbose mode, got:\n%s", result)
	}
}

func TestErrorHandler_AddToAllowlist(t *testing.T) {
	handler := NewErrorHandler(true)

	handler.AddToAllowlist("custom_safe_field")

	errInfo := &ErrorInfo{
		Code:       "ERROR",
		Message:    "Test error",
		HTTPStatus: 400,
		Details: map[string]interface{}{
			"custom_safe_field": "visible_value",
			"password":          "secret",
		},
	}

	result := handler.FormatError(errInfo)

	if !contains(result, "visible_value") {
		t.Errorf("FormatError() should show allowlisted field 'custom_safe_field', got:\n%s", result)
	}

	if !contains(result, "[REDACTED]") {
		t.Errorf("FormatError() should redact 'password', got:\n%s", result)
	}
}

func TestErrorHandler_RemoveFromAllowlist(t *testing.T) {
	handler := NewErrorHandler(true)

	customField := "custom_xyz_prop"
	handler.AddToAllowlist(customField)
	handler.RemoveFromAllowlist(customField)

	errInfo := &ErrorInfo{
		Code:       "ERROR",
		Message:    "Test error",
		HTTPStatus: 400,
		Details: map[string]interface{}{
			customField: "should_not_show",
			"status":    "visible",
			"field":     "also_visible",
		},
	}

	result := handler.FormatError(errInfo)

	if contains(result, "should_not_show") {
		t.Errorf("FormatError() should not show removed allowlist field '%s', got:\n%s", customField, result)
	}

	if !contains(result, "visible") {
		t.Errorf("FormatError() should show 'status' field from default allowlist, got:\n%s", result)
	}

	if !contains(result, "also_visible") {
		t.Errorf("FormatError() should show 'field' from default allowlist, got:\n%s", result)
	}
}

func TestErrorHandler_isSensitiveDetail(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"password exact", "password", true},
		{"password substring", "user_password", true},
		{"token exact", "token", true},
		{"token substring", "auth_token", true},
		{"api_key exact", "api_key", true},
		{"api_key substring", "x_api_key", true},
		{"secret exact", "secret", true},
		{"secret substring", "client_secret", true},
		{"safe field", "username", false},
		{"safe field 2", "email", false},
		{"safe field 3", "status", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(false)
			got := handler.isSensitiveDetail(tt.key)
			if got != tt.want {
				t.Errorf("isSensitiveDetail(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestErrorHandler_isAllowedDetail(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"field exact", "field", true},
		{"field substring", "missing_field", true},
		{"reason exact", "reason", true},
		{"status exact", "status", true},
		{"id exact", "id", true},
		{"unknown key", "xyz123abc", false},
		{"random key", "qwerty", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewErrorHandler(false)
			got := handler.isAllowedDetail(tt.key)
			if got != tt.want {
				t.Errorf("isAllowedDetail(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestErrorHandler_FormatError_UnknownDetailsHidden(t *testing.T) {
	handler := NewErrorHandler(true)

	errInfo := &ErrorInfo{
		Code:       "ERROR",
		Message:    "Test error",
		HTTPStatus: 400,
		Details: map[string]interface{}{
			"known_field": "value1",
			"random_xyz":  "value2",
			"status":      "active",
			"qwerty123":   "value3",
		},
	}

	result := handler.FormatError(errInfo)

	if !contains(result, "value1") {
		t.Errorf("FormatError() should show known field, got:\n%s", result)
	}

	if !contains(result, "active") {
		t.Errorf("FormatError() should show status field, got:\n%s", result)
	}

	if contains(result, "value2") {
		t.Errorf("FormatError() should not show random field 'random_xyz', got:\n%s", result)
	}

	if contains(result, "value3") {
		t.Errorf("FormatError() should not show random field 'qwerty123', got:\n%s", result)
	}
}

func TestErrorHandler_ComprehensiveSecurityRedaction(t *testing.T) {
	sensitiveFields := map[string]interface{}{
		"password":          "pass123",
		"secret":            "mysecret",
		"api_key":           "ak_123456",
		"apikey":            "ak_abcdef",
		"token":             "tok_123456",
		"access_token":      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"refresh_token":     "ref_123456",
		"private_key":       "-----BEGIN RSA PRIVATE KEY-----",
		"credential":        "cred_123",
		"credentials":       "creds_456",
		"authorization":     "Bearer tok_123",
		"auth":              "auth_token",
		"bearer":            "bearer_token",
		"jwt":               "jwt_token",
		"session":           "session_id",
		"cookie":            "cookie_value",
		"x-api-key":         "xapi_123",
		"x-auth-token":      "xauth_123",
		"client_secret":     "cs_123456",
		"encryption_key":    "enc_key",
		"master_key":        "mk_secret_value",
		"signing_key":       "sign_key",
		"ssh_key":           "ssh-rsa AAAA...",
		"rsa_key":           "rsa_key_data",
		"privatekey":        "private_key_data",
		"secret_key":        "sk_123456",
		"secretkey":         "sk_abcdef",
		"aws_secret":        "aws_secret_val",
		"aws_access_key":    "aws_access_val",
		"database_url":      "postgres://user:pass@host/db",
		"db_password":       "dbpass123",
		"db_user":           "dbuser",
		"connection_string": "Server=myServer;Database=myDB;",
	}

	handler := NewErrorHandler(true)

	errInfo := &ErrorInfo{
		Code:       "SECURITY_ERROR",
		Message:    "Multiple sensitive fields detected",
		HTTPStatus: 400,
		Details:    sensitiveFields,
	}

	result := handler.FormatError(errInfo)

	for field, value := range sensitiveFields {
		if contains(result, value.(string)) {
			t.Errorf("FormatError() should redact sensitive field '%s' with value '%s', got:\n%s", field, value, result)
		}
	}

	expectedRedactions := len(sensitiveFields)
	redactionCount := countOccurrences(result, "[REDACTED]")
	if redactionCount != expectedRedactions {
		t.Errorf("FormatError() should redact all %d sensitive fields, but found %d [REDACTED] markers, got:\n%s", expectedRedactions, redactionCount, result)
	}
}

func countOccurrences(s, substr string) int {
	count := 0
	start := 0
	for {
		pos := findInString(s[start:], substr)
		if !pos {
			break
		}
		count++
		idx := indexOf(s[start:], substr)
		start += idx + 1
	}
	return count
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
