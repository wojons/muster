package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/wojons/muster/internal/version"
	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
	handlers       *handlers.Registry
	initialized    bool
	shutdownChan   chan struct{}
	mu             sync.RWMutex
	wg             sync.WaitGroup // CRITICAL FIX: Tracks in-flight goroutines
	concurrencySem chan struct{}  // CRITICAL FIX: Semaphore to prevent unbounded goroutine spawn
	// writeCh carries marshaled responses to the dedicated responseWriter
	// goroutine; done is closed when Run returns (or Shutdown is called) and
	// tells the writer to drain and exit. QA-MUSTER-19-R2: a client that stops
	// draining stdout mid-response (go1.27.3's json.Decoder consumes exactly
	// the response value from an io.Pipe and never the trailing newline) parks
	// whoever writes. Parking the writer goroutine must not park Run's
	// shutdown: s.wg covers handler completion, never client consumption.
	writeCh  chan []byte
	done     chan struct{}
	doneOnce sync.Once
}

func NewServer(stdin io.Reader, stdout, stderr io.Writer) *Server {
	// CRITICAL FIX: Limit concurrent requests to prevent DoS via goroutine exhaustion
	maxConcurrent := 100 // Maximum number of concurrent request goroutines

	s := &Server{
		stdin:          stdin,
		stdout:         stdout,
		stderr:         stderr,
		handlers:       handlers.NewRegistry(),
		shutdownChan:   make(chan struct{}),
		concurrencySem: make(chan struct{}, maxConcurrent),
		// One queued response per in-flight request cannot block an enqueue
		// while the writer is draining.
		writeCh: make(chan []byte, maxConcurrent),
		done:    make(chan struct{}),
	}

	s.registerHandlers()
	return s
}

func (s *Server) registerHandlers() {
	// Advertise the build/runtime version (DF-032) instead of a hardcoded
	// "1.0.0"; the ldflags-stamped value is the source of truth.
	s.handlers.Register(handlers.NewInitializeHandler("openapi-mcp", version.Get().Version))
	s.handlers.Register(handlers.NewInitializedHandler())
}

// jsonRPCCodeInvalidRequest is the JSON-RPC 2.0 "Invalid Request" error code.
// It is used for a message the server cannot process, including one larger
// than maxMessageSize.
const jsonRPCCodeInvalidRequest = -32600

// maxMessageSize is the largest single newline-delimited JSON-RPC message the
// server accepts on stdin, in bytes. bufio.Scanner's default token cap is
// 64 KiB, which a tools/call argument carrying a large payload can exceed; that
// ended the read loop with "bufio.Scanner: token too long" and killed the
// process with no JSON-RPC error and no way to continue (DF-046). Messages up
// to this cap are served; a larger one is drained, answered with a JSON-RPC
// error and does not stop the server.
const maxMessageSize = 4 << 20 // 4 MiB

// stdinBufSize bounds how much of stdin is buffered between reads. A message
// larger than this is reassembled from successive reads, up to maxMessageSize.
const stdinBufSize = 64 << 10 // 64 KiB

// tooLargeMessage is the message reported for a JSON-RPC request larger than
// maxMessageSize. It names the limit so a client can size its messages.
var tooLargeMessage = fmt.Sprintf("Request too large: message exceeds the %d-byte limit", maxMessageSize)

// errMessageTooLong reports a single message larger than maxMessageSize. It is
// a validation error so the read loop can answer it and keep serving instead of
// terminating the server (DF-046).
var errMessageTooLong = apperrors.New(apperrors.CodeValidation, tooLargeMessage)

// Run reads newline-delimited JSON-RPC requests from stdin and dispatches them
// until stdin is closed, the context is cancelled or Shutdown is called. Read
// failures other than EOF are returned to the caller; a message larger than
// maxMessageSize is answered with a JSON-RPC error and does not terminate the
// server (DF-046).
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReaderSize(s.stdin, stdinBufSize)

	// The dedicated writer owns every stdout write; closing done on return
	// tells it to drain queued responses and exit.
	go s.responseWriter()
	defer s.closeDone()

	for {
		select {
		case <-s.shutdownChan:
			// Wait for in-flight request goroutines before returning
			s.wg.Wait()
			return nil
		case <-ctx.Done():
			// Wait for in-flight request goroutines before returning
			s.wg.Wait()
			return ctx.Err()
		default:
		}

		line, err := readMessage(ctx, reader, maxMessageSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if apperrors.IsError(err, apperrors.CodeValidation) {
				// An oversized message is refused, not fatal: report it to the
				// client and to stderr, then keep serving subsequent messages.
				_, _ = fmt.Fprintf(s.stderr, "mcp: %v\n", err)
				s.sendError(nil, jsonRPCCodeInvalidRequest, tooLargeMessage)
				continue
			}
			// Wait for in-flight request goroutines before returning
			s.wg.Wait()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}

		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		// Acquire semaphore before spawning goroutine to prevent unbounded spawn
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.concurrencySem <- struct{}{}
			defer func() { <-s.concurrencySem }()
			s.handleRequest(ctx, &req)
		}()
	}

	// Wait for all in-flight request goroutines to complete before returning
	s.wg.Wait()
	return nil
}

// readMessage reads one newline-terminated message from r, up to maxSize bytes,
// and returns it without its line terminator. The final line before EOF is
// accepted even without a terminator. A message larger than maxSize is reported
// as errMessageTooLong once the rest of its line has been drained, so the next
// message on the stream can still be read.
//
// ctx is consulted between reads so an unresponsive client streaming an endless
// line cannot hold the server open past cancellation.
func readMessage(ctx context.Context, r *bufio.Reader, maxSize int) ([]byte, error) {
	var line []byte
	tooLong := false

	for {
		chunk, err := r.ReadSlice('\n')

		if !tooLong {
			// The line terminator is not part of the message, so it must not
			// count against the cap: a message of exactly maxSize bytes is
			// accepted.
			if len(line)+messageLen(chunk) > maxSize {
				// An oversized message is refused rather than parsed, so the
				// bytes read so far are dropped instead of being retained.
				tooLong = true
				line = nil
			} else {
				line = append(line, chunk...)
			}
		}

		switch {
		case err == nil:
			if tooLong {
				return nil, errMessageTooLong
			}
			return trimLineEnding(line), nil
		case errors.Is(err, bufio.ErrBufferFull):
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			continue
		case errors.Is(err, io.EOF):
			if tooLong {
				return nil, errMessageTooLong
			}
			if len(line) == 0 {
				return nil, io.EOF
			}
			return trimLineEnding(line), nil
		default:
			return nil, err
		}
	}
}

// trimLineEnding removes the line terminator, and a carriage return preceding
// it, that ReadSlice leaves on a line.
func trimLineEnding(line []byte) []byte {
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line
}

// messageLen reports how many bytes of chunk belong to the message, excluding a
// trailing newline and any carriage return before it.
func messageLen(chunk []byte) int {
	n := len(chunk)
	if n > 0 && chunk[n-1] == '\n' {
		n--
		if n > 0 && chunk[n-1] == '\r' {
			n--
		}
	}
	return n
}

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	// JSON-RPC 2.0 notifications carry no id and MUST NOT receive any
	// response, not even an error. MCP clients send notifications/initialized
	// this way; answering it corrupts the response stream.
	if req.ID == nil {
		return
	}

	handler := s.handlers.Get(req.Method)
	if handler == nil {
		s.sendError(req.ID, -32601, "Method not found")
		return
	}

	result, err := handler.Handle(ctx, req.Params)

	if err != nil {
		if mcpErr, ok := err.(*handlers.MCPError); ok {
			s.sendError(req.ID, mcpErr.Code, mcpErr.Message)
		} else {
			s.sendError(req.ID, -32000, err.Error())
		}
	} else {
		s.sendResponse(req.ID, result)
	}
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  mustMarshal(result),
	}

	s.writeResponse(resp)
}

func (s *Server) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}

	s.writeResponse(resp)
}

func (s *Server) writeResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		_, _ = fmt.Fprintf(s.stderr, "Failed to marshal response: %v\n", err)
		return
	}

	// Hand the response to the dedicated writer instead of writing inline:
	// a client that stops draining stdout mid-response parks the writer
	// goroutine, never a request goroutine, so Run's shutdown wait (s.wg)
	// cannot be held open by client backpressure (QA-MUSTER-19-R2).
	select {
	case s.writeCh <- data:
	case <-s.done:
		_, _ = fmt.Fprintf(s.stderr, "mcp: response dropped, server is shutting down\n")
	}
}

// closeDone signals the response writer to drain and exit. It is idempotent
// because both Run's return and Shutdown can close the lifecycle.
func (s *Server) closeDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

// responseWriter is the server's single stdout writer, started by Run. It
// serializes responses in enqueue order (previously s.mu did this per write)
// and is deliberately outside s.wg: once Run has returned, a write blocked on
// an unresponsive client parks this goroutine alone until the client reads or
// the stream is closed, instead of wedging shutdown.
func (s *Server) responseWriter() {
	for {
		select {
		case data := <-s.writeCh:
			s.writeLine(data)
		case <-s.done:
			// Run waited for all handlers before closing done, so every
			// response is already queued: drain them, then exit.
			for {
				select {
				case data := <-s.writeCh:
					s.writeLine(data)
				default:
					return
				}
			}
		}
	}
}

func (s *Server) writeLine(data []byte) {
	_, _ = fmt.Fprintln(s.stdout, string(data))

	// CRITICAL FIX: Flush stdout buffer to ensure JSON-RPC responses are sent immediately
	// Without flush, buffered stdout causes EOF errors in tests
	if bw, ok := s.stdout.(interface{ Flush() error }); ok {
		_ = bw.Flush()
	}
}

func (s *Server) Shutdown() error {
	close(s.shutdownChan)
	// CRITICAL FIX: Wait for all request goroutines to complete
	// This ensures no goroutines are still producing responses when we return;
	// the dedicated writer drains anything already queued asynchronously (a
	// client that stopped draining stdout cannot hold Shutdown open).
	s.wg.Wait()
	s.closeDone()
	return nil
}

func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func (s *Server) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// RegisterHandler adds a handler to the server
func (s *Server) RegisterHandler(handler handlers.Handler) {
	if s.handlers != nil {
		s.handlers.Register(handler)
	}
}
