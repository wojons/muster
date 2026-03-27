package response

import (
	"bytes"
	"io"
)

type mockBody struct {
	*bytes.Buffer
}

func (m *mockBody) Close() error {
	return nil
}

func newMockBody(s string) io.ReadCloser {
	return &mockBody{bytes.NewBufferString(s)}
}
