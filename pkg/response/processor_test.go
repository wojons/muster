package response

import (
	"bytes"
	"net/http"
	"testing"
)

func TestProcessor_ProcessResponse_Success(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Writer:        &buf,
		VerboseErrors: false,
	}

	body := bytes.NewBufferString(`{"test":"data"}`)
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    &mockBody{body},
		Request: &http.Request{},
	}

	handler := NewHandler(config)
	options := ProcessingOptions{}

	err := handler.ProcessResponse(resp, options)

	if err != nil {
		t.Errorf("ProcessResponse() unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected output to be written")
	}
}

func TestProcessor_ProcessResponse_Error(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Writer:        &buf,
		VerboseErrors: false,
	}

	body := bytes.NewBufferString(`{"error":"Not found"}`)
	resp := &http.Response{
		StatusCode: 404,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    &mockBody{body},
		Request: &http.Request{},
	}

	handler := NewHandler(config)
	options := ProcessingOptions{}

	err := handler.ProcessResponse(resp, options)

	if err == nil {
		t.Error("expected error but got nil")
	}

	errorInfo, ok := err.(*ErrorInfo)
	if !ok {
		t.Log("error type:", err)
	}

	if errorInfo != nil && errorInfo.HTTPStatus != 404 {
		t.Errorf("expected HTTPStatus 404, got %d", errorInfo.HTTPStatus)
	}

	if buf.Len() == 0 {
		t.Error("expected error output to be written")
	}
}

func TestProcessor_ProcessResponse_Raw(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Writer:        &buf,
		VerboseErrors: false,
	}

	testData := `{"test":"data"}`
	body := bytes.NewBufferString(testData)
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    &mockBody{body},
		Request: &http.Request{},
	}

	handler := NewHandler(config)
	options := ProcessingOptions{
		Raw: true,
	}

	err := handler.ProcessResponse(resp, options)

	if err != nil {
		t.Errorf("ProcessResponse() unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected output to be written")
	}
}

func TestProcessor_ProcessStream_NilResponse(t *testing.T) {
	config := Config{}

	handler := NewHandler(config)

	err := handler.ProcessStream(nil, func(interface{}) error { return nil })

	if err == nil {
		t.Error("expected error for nil response")
	}
}

func TestProcessor_ProcessStream_NilHandler(t *testing.T) {
	config := Config{}

	handler := NewHandler(config)
	resp := &http.Response{}

	err := handler.ProcessStream(resp, nil)

	if err == nil {
		t.Error("expected error for nil handler")
	}
}

func TestPaginationInfo(t *testing.T) {
	info := &PaginationInfo{
		Page:       1,
		PerPage:    10,
		Total:      100,
		TotalPages: 10,
		HasNext:    true,
		HasPrev:    false,
	}

	if info.Page != 1 {
		t.Errorf("expected Page 1, got %d", info.Page)
	}

	if !info.HasNext {
		t.Error("expected HasNext to be true")
	}

	if info.HasPrev {
		t.Error("expected HasPrev to be false")
	}
}
