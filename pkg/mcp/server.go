package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/openapi-cli/openapi-cli/pkg/mcp/handlers"
)

type Server struct {
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	handlers     *handlers.Registry
	initialized  bool
	shutdownChan chan struct{}
	mu           sync.RWMutex
}

func NewServer(stdin io.Reader, stdout, stderr io.Writer) *Server {
	s := &Server{
		stdin:        stdin,
		stdout:       stdout,
		stderr:       stderr,
		handlers:     handlers.NewRegistry(),
		shutdownChan: make(chan struct{}),
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
			return nil
		case <-ctx.Done():
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

		go s.handleRequest(ctx, &req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	result, err := s.handlers.Handle(ctx, req.Method, req.Params)

	if err != nil {
		mcpErr := handlers.HandleError(err)
		s.sendError(req.ID, mcpErr.Code, mcpErr.Message)
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
		fmt.Fprintf(s.stderr, "Failed to marshal response: %v\n", err)
		return
	}

	fmt.Fprintln(s.stdout, string(data))
}

func (s *Server) Shutdown() error {
	close(s.shutdownChan)
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
