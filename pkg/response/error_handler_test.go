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
