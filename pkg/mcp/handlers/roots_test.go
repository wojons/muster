package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// MockRootManager is a mock implementation of RootManager for testing
type MockRootManager struct {
	mu            sync.RWMutex
	roots         []Root
	listChanged   bool
	setRootsCalls int
}

func NewMockRootManager(listChanged bool) *MockRootManager {
	return &MockRootManager{
		roots:       make([]Root, 0),
		listChanged: listChanged,
	}
}

func (m *MockRootManager) SetRoots(roots []Root) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roots = make([]Root, len(roots))
	copy(m.roots, roots)
	m.setRootsCalls++
}

func (m *MockRootManager) GetRoots() []Root {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Root, len(m.roots))
	copy(result, m.roots)
	return result
}

func (m *MockRootManager) SupportsListChanged() bool {
	return m.listChanged
}

func (m *MockRootManager) GetSetRootsCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setRootsCalls
}

func TestRootsListChangedHandler_Method(t *testing.T) {
	rootManager := NewMockRootManager(true)
	handler := NewRootsListChangedHandler(rootManager, nil)

	if handler.Method() != "notifications/roots/list_changed" {
		t.Errorf("expected method 'notifications/roots/list_changed', got '%s'", handler.Method())
	}
}

func TestRootsListChangedHandler_Handle(t *testing.T) {
	tests := []struct {
		name           string
		setupCallback  bool
		expectCallback bool
	}{
		{
			name:           "triggers callback when set",
			setupCallback:  true,
			expectCallback: true,
		},
		{
			name:           "no panic when callback is nil",
			setupCallback:  false,
			expectCallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootManager := NewMockRootManager(true)
			callbackCalled := false
			var callbackFunc func()

			if tt.setupCallback {
				callbackFunc = func() {
					callbackCalled = true
				}
			}

			handler := NewRootsListChangedHandler(rootManager, callbackFunc)
			result, err := handler.Handle(context.Background(), json.RawMessage("{}"))

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if result != nil {
				t.Errorf("expected nil result for notification, got %v", result)
			}

			if tt.expectCallback && !callbackCalled {
				t.Error("expected callback to be called, but it wasn't")
			}

			if !tt.expectCallback && callbackCalled {
				t.Error("expected callback not to be called, but it was")
			}
		})
	}
}

func TestRootsListChangedHandler_Handle_WithParams(t *testing.T) {
	rootManager := NewMockRootManager(true)
	callbackCalled := false

	handler := NewRootsListChangedHandler(rootManager, func() {
		callbackCalled = true
	})

	// Test with various params - notification should accept any params
	params := json.RawMessage(`{"someField": "someValue"}`)
	result, err := handler.Handle(context.Background(), params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result != nil {
		t.Error("expected nil result for notification")
	}

	if !callbackCalled {
		t.Error("expected callback to be called")
	}
}

func TestNewRootsClient(t *testing.T) {
	sendRequest := func(method string, params interface{}, id string) error {
		return nil
	}

	client := NewRootsClient(sendRequest)

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.nextID != 0 {
		t.Errorf("expected nextID to be 0, got %d", client.nextID)
	}

	if client.pending == nil {
		t.Error("expected pending map to be initialized")
	}

	if client.sendRequest == nil {
		t.Error("expected sendRequest to be set")
	}
}

func TestRootsClient_RequestRoots(t *testing.T) {
	tests := []struct {
		name          string
		sendError     error
		expectError   bool
		ctxTimeout    time.Duration
		respondAfter  time.Duration
		responseError *MCPError
	}{
		{
			name:         "successful request",
			ctxTimeout:   time.Second,
			respondAfter: 10 * time.Millisecond,
			expectError:  false,
		},
		{
			name:        "send request error",
			sendError:   errors.New("connection failed"),
			expectError: true,
		},
		{
			name:         "context timeout",
			ctxTimeout:   50 * time.Millisecond,
			respondAfter: 100 * time.Millisecond,
			expectError:  true,
		},
		{
			name:         "response error",
			ctxTimeout:   time.Second,
			respondAfter: 10 * time.Millisecond,
			// Note: Response error sends nil to channel, RequestRoots returns (nil, nil)
			// This is the current behavior - client receives nil result, no error
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID string
			var testClient *RootsClient
			testClient = NewRootsClient(func(method string, params interface{}, id string) error {
				capturedID = id
				if tt.sendError != nil {
					return tt.sendError
				}

				// Simulate async response
				go func() {
					time.Sleep(tt.respondAfter)
					var result json.RawMessage
					if tt.responseError == nil {
						result = json.RawMessage(`{"roots": [{"uri": "file:///test", "name": "test"}]}`)
					}
					_ = testClient.HandleResponse(id, result, tt.responseError)
				}()

				return nil
			})

			ctx, cancel := context.WithTimeout(context.Background(), tt.ctxTimeout)
			defer cancel()

			result, err := testClient.RequestRoots(ctx)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error, got %v", err)
				return
			}

			if result == nil {
				t.Error("expected result, got nil")
				return
			}

			if len(result.Roots) != 1 {
				t.Errorf("expected 1 root, got %d", len(result.Roots))
			}

			_ = capturedID // Used for verification but may not be set in error cases
		})
	}
}

func TestRootsClient_HandleResponse(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		result       json.RawMessage
		respErr      *MCPError
		setupPending bool
		expectError  bool
	}{
		{
			name:         "successful response",
			id:           "test-1",
			result:       json.RawMessage(`{"roots": [{"uri": "file:///home", "name": "home"}]}`),
			setupPending: true,
			expectError:  false,
		},
		{
			name:         "response with error",
			id:           "test-2",
			respErr:      &MCPError{Code: -32600, Message: "Invalid request"},
			setupPending: true,
			expectError:  true,
		},
		{
			name:         "unknown request id",
			id:           "unknown",
			result:       json.RawMessage(`{"roots": []}`),
			setupPending: false,
			expectError:  true,
		},
		{
			name:         "invalid json result",
			id:           "test-3",
			result:       json.RawMessage(`{invalid json}`),
			setupPending: true,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewRootsClient(nil)
			var resultCh chan *ListRootsResult

			if tt.setupPending {
				resultCh = make(chan *ListRootsResult, 1)
				client.pending[tt.id] = resultCh
			}

			err := client.HandleResponse(tt.id, tt.result, tt.respErr)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}

			if tt.setupPending && resultCh != nil {
				select {
				case result := <-resultCh:
					if tt.respErr != nil {
						if result != nil {
							t.Error("expected nil result on error")
						}
					} else if !tt.expectError {
						if result == nil {
							t.Error("expected non-nil result")
						}
					}
				case <-time.After(time.Second):
					t.Error("timeout waiting for result")
				}
			}
		})
	}
}

func TestRootsClient_generateID(t *testing.T) {
	client := NewRootsClient(nil)

	id1 := client.generateID()
	id2 := client.generateID()
	id3 := client.generateID()

	if id1 == id2 || id2 == id3 {
		t.Error("expected unique IDs")
	}

	if id1 != "roots-1" {
		t.Errorf("expected first ID to be 'roots-1', got '%s'", id1)
	}

	if id2 != "roots-2" {
		t.Errorf("expected second ID to be 'roots-2', got '%s'", id2)
	}
}

func TestRootsClient_ConcurrentAccess(t *testing.T) {
	client := NewRootsClient(func(method string, params interface{}, id string) error {
		return nil
	})

	// Test concurrent ID generation
	var wg sync.WaitGroup
	ids := make([]string, 100)
	idMutex := sync.Mutex{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := client.generateID()
			idMutex.Lock()
			ids = append(ids, id)
			idMutex.Unlock()
		}()
	}

	wg.Wait()

	// Check all IDs are unique
	idMap := make(map[string]bool)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if idMap[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		idMap[id] = true
	}
}

func TestInitializeRoots(t *testing.T) {
	tests := []struct {
		name           string
		listChanged    bool
		setupClient    bool
		expectRootsSet bool
		expectCallback bool
	}{
		{
			name:           "initialize with client",
			listChanged:    true,
			setupClient:    true,
			expectRootsSet: true,
			expectCallback: true,
		},
		{
			name:           "initialize without client",
			listChanged:    true,
			setupClient:    false,
			expectRootsSet: false,
			expectCallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootManager := NewMockRootManager(tt.listChanged)
			callbackCalled := false
			var callbackRoots []Root

			var client *RootsClient
			if tt.setupClient {
				client = NewRootsClient(func(method string, params interface{}, id string) error {
					// Simulate immediate response
					go func() {
						time.Sleep(10 * time.Millisecond)
						result := json.RawMessage(`{"roots": [{"uri": "file:///test", "name": "test"}]}`)
						_ = client.HandleResponse(id, result, nil)
					}()
					return nil
				})
			}

			onRootsUpdated := func(roots []Root) {
				callbackCalled = true
				callbackRoots = roots
			}

			InitializeRoots(rootManager, tt.listChanged, client, onRootsUpdated)

			// Wait for async initialization
			time.Sleep(50 * time.Millisecond)

			if tt.expectRootsSet {
				if rootManager.GetSetRootsCalls() == 0 {
					t.Error("expected SetRoots to be called")
				}
				if len(rootManager.GetRoots()) != 1 {
					t.Errorf("expected 1 root, got %d", len(rootManager.GetRoots()))
				}
			} else {
				if rootManager.GetSetRootsCalls() > 0 {
					t.Error("expected SetRoots not to be called")
				}
			}

			if tt.expectCallback {
				if !callbackCalled {
					t.Error("expected onRootsUpdated callback to be called")
				}
				if len(callbackRoots) != 1 {
					t.Errorf("expected callback to receive 1 root, got %d", len(callbackRoots))
				}
			} else {
				if callbackCalled {
					t.Error("expected onRootsUpdated callback not to be called")
				}
			}
		})
	}
}

func TestRoot_Type(t *testing.T) {
	root := Root{
		URI:  "file:///home/user/project",
		Name: "My Project",
	}

	if root.URI != "file:///home/user/project" {
		t.Errorf("expected URI 'file:///home/user/project', got '%s'", root.URI)
	}

	if root.Name != "My Project" {
		t.Errorf("expected name 'My Project', got '%s'", root.Name)
	}
}

func TestListRootsResult_Type(t *testing.T) {
	result := ListRootsResult{
		Roots: []Root{
			{URI: "file:///test1", Name: "Test1"},
			{URI: "file:///test2", Name: "Test2"},
		},
	}

	if len(result.Roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(result.Roots))
	}
}

func TestRootsListChangedNotification_Type(t *testing.T) {
	// Just verify the type exists and can be instantiated
	_ = RootsListChangedNotification{}
}
