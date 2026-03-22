package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// Root represents a workspace boundary
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ListRootsResult represents the response to a roots/list request
type ListRootsResult struct {
	Roots []Root `json:"roots"`
}

// RootsListChangedNotification represents a roots/list_changed notification
type RootsListChangedNotification struct{}

// RootsListChangedHandler handles notifications/roots/list_changed from the client
type RootsListChangedHandler struct {
	rootManager  RootManager
	requestRoots func() // Callback to trigger roots/list request
}

// RootManager interface for managing roots
type RootManager interface {
	SetRoots(roots []Root)
	GetRoots() []Root
	SupportsListChanged() bool
}

// NewRootsListChangedHandler creates a new handler for roots list changed notifications
func NewRootsListChangedHandler(rootManager RootManager, requestRoots func()) *RootsListChangedHandler {
	return &RootsListChangedHandler{
		rootManager:  rootManager,
		requestRoots: requestRoots,
	}
}

// Method returns the method name this handler handles
func (h *RootsListChangedHandler) Method() string {
	return "notifications/roots/list_changed"
}

// Handle processes the roots/list_changed notification
func (h *RootsListChangedHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Notification has no response - trigger a roots/list request
	if h.requestRoots != nil {
		h.requestRoots()
	}
	return nil, nil
}

// RootsClient handles sending roots/list requests to the client
type RootsClient struct {
	mu          sync.RWMutex
	nextID      int64
	pending     map[string]chan *ListRootsResult
	sendRequest func(method string, params interface{}, id string) error
}

// NewRootsClient creates a new roots client
func NewRootsClient(sendRequest func(method string, params interface{}, id string) error) *RootsClient {
	return &RootsClient{
		nextID:      0,
		pending:     make(map[string]chan *ListRootsResult),
		sendRequest: sendRequest,
	}
}

// RequestRoots sends a roots/list request to the client and returns the result
func (c *RootsClient) RequestRoots(ctx context.Context) (*ListRootsResult, error) {
	id := c.generateID()
	resultCh := make(chan *ListRootsResult, 1)

	c.mu.Lock()
	c.pending[id] = resultCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.sendRequest("roots/list", struct{}{}, id); err != nil {
		return nil, fmt.Errorf("failed to send roots/list request: %w", err)
	}

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandleResponse processes a response to a roots/list request
func (c *RootsClient) HandleResponse(id string, result json.RawMessage, respErr *MCPError) error {
	c.mu.RLock()
	ch, ok := c.pending[id]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no pending request for id: %s", id)
	}

	if respErr != nil {
		ch <- nil
		return fmt.Errorf("roots/list request failed: %s", respErr.Message)
	}

	var listResult ListRootsResult
	if err := json.Unmarshal(result, &listResult); err != nil {
		ch <- nil
		return fmt.Errorf("failed to unmarshal roots/list result: %w", err)
	}

	ch <- &listResult
	return nil
}

func (c *RootsClient) generateID() string {
	id := atomic.AddInt64(&c.nextID, 1)
	return fmt.Sprintf("roots-%d", id)
}

// InitializeRoots sets up root management during server initialization
func InitializeRoots(rootManager RootManager, listChanged bool, client *RootsClient, onRootsUpdated func([]Root)) {
	// Request initial roots
	if client != nil {
		go func() {
			ctx := context.Background()
			result, err := client.RequestRoots(ctx)
			if err != nil {
				return
			}
			if result != nil && rootManager != nil {
				rootManager.SetRoots(result.Roots)
				if onRootsUpdated != nil {
					onRootsUpdated(result.Roots)
				}
			}
		}()
	}
}
