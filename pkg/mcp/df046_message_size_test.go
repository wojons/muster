package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// df046EchoHandler answers with the size of the params it was given, so a test
// can prove a large request line arrived whole instead of truncated.
type df046EchoHandler struct{}

// Method returns the MCP method name this handler handles.
func (df046EchoHandler) Method() string { return "df046/echo" }

// Handle answers with the number of bytes of params it received.
func (df046EchoHandler) Handle(_ context.Context, params json.RawMessage) (interface{}, error) {
	return map[string]int{"paramsBytes": len(params)}, nil
}

// TestMaxMessageSize_BeatsScannerDefault pins the invariant behind DF-046: the
// documented cap must exceed the 64 KiB bufio.Scanner token limit it replaces,
// or a large tools/call argument would again end the read loop.
func TestMaxMessageSize_BeatsScannerDefault(t *testing.T) {
	if maxMessageSize <= 64<<10 {
		t.Errorf("maxMessageSize = %d bytes, want > 64 KiB (the bufio.Scanner default cap it replaces)", maxMessageSize)
	}
}

// TestServer_Run_LargeMessageWithinCapIsServed is the DF-046 regression: a
// single request line larger than bufio.Scanner's default 64 KiB token cap
// must be read, parsed and answered. With the old Scanner this request
// produced no response at all — the read loop returned "bufio.Scanner: token
// too long" and the server process died.
func TestServer_Run_LargeMessageWithinCapIsServed(t *testing.T) {
	const padding = 128 << 10 // twice the 64 KiB Scanner cap, well under maxMessageSize

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)
	s.RegisterHandler(df046EchoHandler{})

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"df046/echo","params":{"padding":%q}}`,
		strings.Repeat("a", padding))

	go func() {
		_, _ = stdinW.Write([]byte(line + "\n"))
		_ = stdinW.Close()
	}()

	resp := df046DecodeResponses(t, stdoutR, 1, 10*time.Second)[0]

	if resp.Error != nil {
		t.Fatalf("unexpected error for a %d-byte message: code=%d msg=%s", len(line), resp.Error.Code, resp.Error.Message)
	}
	var result struct {
		ParamsBytes int `json:"paramsBytes"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result %s: %v", resp.Result, err)
	}
	if result.ParamsBytes < padding {
		t.Errorf("handler saw %d params bytes, want >= %d (the large message was truncated)",
			result.ParamsBytes, padding)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil for a large but valid message", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after stdin was closed")
	}
	stdoutR.Close()
}

// TestServer_Run_OversizedMessageIsRefusedAndServerSurvives proves the second
// half of DF-046: a message past maxMessageSize is answered with a JSON-RPC
// error (code -32600), its line is drained, and the next request on the same
// stream is still served — the oversized message does not terminate the server.
func TestServer_Run_OversizedMessageIsRefusedAndServerSurvives(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// One byte over the documented cap, then a normal request on the stream.
	go func() {
		_, _ = stdinW.Write([]byte(strings.Repeat("x", maxMessageSize+1)))
		_, _ = stdinW.Write([]byte("\n"))
		_, _ = stdinW.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}` + "\n"))
		_ = stdinW.Close()
	}()

	responses := df046DecodeResponses(t, stdoutR, 2, 15*time.Second)

	refusal := responses[0]
	if refusal.Error == nil {
		t.Fatalf("oversized message produced no error response: result=%s", refusal.Result)
	}
	if refusal.Error.Code != jsonRPCCodeInvalidRequest {
		t.Errorf("oversized-message error code = %d, want %d", refusal.Error.Code, jsonRPCCodeInvalidRequest)
	}
	if !strings.Contains(refusal.Error.Message, fmt.Sprintf("%d", maxMessageSize)) {
		t.Errorf("oversized-message error message %q does not name the %d-byte limit", refusal.Error.Message, maxMessageSize)
	}

	served := responses[1]
	if served.Error != nil {
		t.Fatalf("request after an oversized message failed: code=%d msg=%s", served.Error.Code, served.Error.Message)
	}
	if id, ok := served.ID.(float64); !ok || id != 2 {
		t.Errorf("second response id = %v, want 2 (the server must keep serving)", served.ID)
	}
	if served.Result == nil {
		t.Error("result should not be nil for initialize")
	}

	if !strings.Contains(stderr.String(), "VALIDATION_ERROR") {
		t.Errorf("stderr = %q, want the refusal logged for the operator", stderr.String())
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil (an oversized message must not terminate the server)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after stdin was closed")
	}
	stdoutR.Close()
}

// TestReadMessage_Boundaries covers the line reader that replaced the Scanner,
// including the size boundary and the refill path taken by messages larger than
// the reader's own buffer.
func TestReadMessage_Boundaries(t *testing.T) {
	ctx := context.Background()

	t.Run("accepts a message of exactly the cap", func(t *testing.T) {
		const limit = 16
		msg := strings.Repeat("a", limit)
		got, err := readMessage(ctx, bufio.NewReader(strings.NewReader(msg+"\n")), limit)
		if err != nil {
			t.Fatalf("readMessage returned %v, want no error", err)
		}
		if string(got) != msg {
			t.Errorf("readMessage = %q, want %q", got, msg)
		}
	})

	t.Run("accepts a CRLF-terminated message of exactly the cap", func(t *testing.T) {
		// The terminator must not count against the cap, otherwise a message
		// of exactly maxSize bytes would be refused.
		const limit = 16
		msg := strings.Repeat("a", limit)
		got, err := readMessage(ctx, bufio.NewReader(strings.NewReader(msg+"\r\n")), limit)
		if err != nil {
			t.Fatalf("readMessage returned %v, want no error", err)
		}
		if string(got) != msg {
			t.Errorf("readMessage = %q, want %q", got, msg)
		}
	})

	t.Run("refuses a message over the cap and drains its line", func(t *testing.T) {
		const limit = 16
		r := bufio.NewReader(strings.NewReader(strings.Repeat("a", limit+1) + "\nnext\n"))

		if _, err := readMessage(ctx, r, limit); !apperrors.IsError(err, apperrors.CodeValidation) {
			t.Fatalf("readMessage error = %v, want a validation error (message too long)", err)
		}
		// The oversized line must be fully consumed, not mistaken for input.
		got, err := readMessage(ctx, r, limit)
		if err != nil {
			t.Fatalf("readMessage after an oversized message returned %v, want no error", err)
		}
		if string(got) != "next" {
			t.Errorf("message after an oversized one = %q, want %q", got, "next")
		}
	})

	t.Run("refuses an oversized message split across buffer refills", func(t *testing.T) {
		// Larger than the reader's 4096-byte buffer, so the message arrives as
		// several chunks ending in bufio.ErrBufferFull.
		const limit = 8192
		r := bufio.NewReader(strings.NewReader(strings.Repeat("a", limit+1) + "\nnext\n"))

		if _, err := readMessage(ctx, r, limit); !apperrors.IsError(err, apperrors.CodeValidation) {
			t.Fatalf("readMessage error = %v, want a validation error (message too long)", err)
		}
		got, err := readMessage(ctx, r, limit)
		if err != nil {
			t.Fatalf("readMessage after an oversized message returned %v, want no error", err)
		}
		if string(got) != "next" {
			t.Errorf("message after an oversized one = %q, want %q", got, "next")
		}
	})

	t.Run("accepts the last line without a terminator", func(t *testing.T) {
		got, err := readMessage(ctx, bufio.NewReader(strings.NewReader("no-newline")), 64)
		if err != nil {
			t.Fatalf("readMessage returned %v, want no error", err)
		}
		if string(got) != "no-newline" {
			t.Errorf("readMessage = %q, want %q", got, "no-newline")
		}
	})

	t.Run("refuses an oversized last line at EOF", func(t *testing.T) {
		const limit = 16
		_, err := readMessage(ctx, bufio.NewReader(strings.NewReader(strings.Repeat("a", limit+1))), limit)
		if !apperrors.IsError(err, apperrors.CodeValidation) {
			t.Fatalf("readMessage error = %v, want a validation error (message too long)", err)
		}
	})

	t.Run("strips a trailing CRLF", func(t *testing.T) {
		got, err := readMessage(ctx, bufio.NewReader(strings.NewReader("line\r\n")), 64)
		if err != nil {
			t.Fatalf("readMessage returned %v, want no error", err)
		}
		if string(got) != "line" {
			t.Errorf("readMessage = %q, want %q", got, "line")
		}
	})

	t.Run("reports EOF on empty input", func(t *testing.T) {
		_, err := readMessage(ctx, bufio.NewReader(strings.NewReader("")), 16)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("readMessage error = %v, want io.EOF", err)
		}
	})

	t.Run("honours context cancellation while draining", func(t *testing.T) {
		// A client that streams an endless line must not hold the server open
		// past cancellation: the reader is only drained until ctx is done.
		cctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := bufio.NewReaderSize(&df046EndlessReader{cancel: cancel}, 32)

		done := make(chan error, 1)
		go func() {
			_, err := readMessage(cctx, r, 64)
			done <- err
		}()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("readMessage error = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("readMessage kept draining after the context was cancelled")
		}
	})
}

// df046EndlessReader yields a non-stop stream of bytes and never produces the
// newline that would end a message. It cancels its context on the first read.
type df046EndlessReader struct {
	cancel context.CancelFunc
}

// Read fills p with filler bytes and then cancels the reader's context.
func (r *df046EndlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	r.cancel()
	return len(p), nil
}

// df046DecodeResponses decodes want JSON-RPC responses from r in order, failing
// the test if fewer arrive within timeout. A single decoder is reused so no
// buffered response bytes are lost between reads.
func df046DecodeResponses(t *testing.T, r io.Reader, want int, timeout time.Duration) []JSONRPCResponse {
	t.Helper()

	type decoded struct {
		resp JSONRPCResponse
		err  error
	}
	ch := make(chan decoded, want)
	go func() {
		dec := json.NewDecoder(r)
		for i := 0; i < want; i++ {
			var resp JSONRPCResponse
			if err := dec.Decode(&resp); err != nil {
				ch <- decoded{err: err}
				return
			}
			ch <- decoded{resp: resp}
		}
	}()

	responses := make([]JSONRPCResponse, 0, want)
	for i := 0; i < want; i++ {
		select {
		case res := <-ch:
			if res.err != nil {
				t.Fatalf("response %d/%d: %v", i+1, want, res.err)
			}
			responses = append(responses, res.resp)
		case <-time.After(timeout):
			t.Fatalf("response %d/%d did not arrive within %s", i+1, want, timeout)
		}
	}
	return responses
}
