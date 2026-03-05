package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugRequestHeaders(t *testing.T) {
	headers := http.Header{
		"X-Request-ID": []string{"req-67890"},
	}
	req := httptest.NewRequest("GET", "/", nil)
	resp := &http.Response{
		Request:    req,
		StatusCode: 500,
		Header:     headers,
		Body:       newMockBody(`{"error":"test"}`),
	}

	t.Logf("Headers: %v", resp.Header)
	for k, v := range resp.Header {
		t.Logf("Header[%q] = %v", k, v)
	}

	handler := NewErrorHandler(false)
	errorInfo := handler.ExtractError(resp, []byte(`{"error":"test"}`))

	t.Logf("RequestID = %q", errorInfo.RequestID)

	if resp.Header.Get("X-Request-ID") != "" {
		t.Logf("resp.Header.Get('X-Request-ID') = %q", resp.Header.Get("X-Request-ID"))
	}

	if resp.Header.Get("X-Request-Id") != "" {
		t.Logf("resp.Header.Get('X-Request-Id') = %q", resp.Header.Get("X-Request-Id"))
	}
}
