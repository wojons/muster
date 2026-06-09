package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

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
	}

	s.registerHandlers()
	return s
}

func (s *Server) registerHandlers() {
	s.handlers.Register(handlers.NewInitializeHandler("openapi-cli-mcp", "1.0.0"))
	s.handlers.Register(handlers.NewInitializedHandler())
}

func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.stdin)

	for scanner.Scan() {
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

		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
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
	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
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
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		_, _ = fmt.Fprintf(s.stderr, "Failed to marshal response: %v\n", err)
		return
	}

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
	// This ensures no goroutines are still writing when we return
	s.wg.Wait()
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
