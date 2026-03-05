package response

import (
	"bytes"
	"io"
)

type mockReadCloser struct {
	*bytes.Buffer
}

func (m *mockReadCloser) Close() error {
	return nil
}

func newMockReadCloser(s string) io.ReadCloser {
	return &mockReadCloser{bytes.NewBufferString(s)}
}

type mockBody struct {
	*bytes.Buffer
}

func (m *mockBody) Close() error {
	return nil
}

func newMockBody(s string) io.ReadCloser {
	return &mockBody{bytes.NewBufferString(s)}
}
