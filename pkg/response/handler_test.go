package response

import (
	"bytes"
	"testing"
)

func TestNewHandler(t *testing.T) {
	config := Config{
		DefaultFormat:    "json",
		SupportedFormats: []string{"json", "yaml"},
		VerboseErrors:    true,
	}

	handler := NewHandler(config)

	if handler == nil {
		t.Fatal("handler should not be nil")
	}

	if handler.config.DefaultFormat != "json" {
		t.Errorf("expected DefaultFormat to be json, got %s", handler.config.DefaultFormat)
	}

	if handler.config.VerboseErrors != true {
		t.Error("expected VerboseErrors to be true")
	}
}

func TestHandler_SetWriter(t *testing.T) {
	var buf bytes.Buffer

	config := Config{
		DefaultFormat: "json",
	}

	handler := NewHandler(config)

	handler.SetWriter(&buf)

	if handler.config.Writer != &buf {
		t.Error("writer should be updated")
	}
}
