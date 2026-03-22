// Package mcp provides MCP (Model Context Protocol) server and client implementations.
//
// This file implements the MCP Roots protocol for defining workspace boundaries.
// Roots represent directories or URIs that the server should consider as project
// roots when processing requests.
//
// Basic usage:
//
//	rm := mcp.NewRootManager(true) // true = supports listChanged
//	rm.SetRoots([]mcp.Root{
//	    {URI: "file:///home/user/project", Name: "project"},
//	    {URI: "file:///home/user/docs", Name: "docs"},
//	})
//
//	roots := rm.GetRoots()
//	fmt.Printf("Current roots: %d\n", len(roots))
//
// Root Manager with change notifications:
//
//	rm.OnChange(func(roots []mcp.Root) {
//	    fmt.Printf("Roots updated: %d\n", len(roots))
//	})
//
//	rm.AddRoot(mcp.Root{URI: "file:///home/user/new", Name: "new"})
//
// Thread Safety:
//
// RootManager uses sync.RWMutex for thread-safe access. All methods can be called
// concurrently without additional synchronization.
//
// Protocol Integration:
//
// The roots protocol allows servers to query the client for workspace boundaries
// and receive notifications when roots change. This is implemented through the
// roots/list request and notifications/roots/list_changed notification.
//
// Example client capability:
//
//	{
//	    "roots": {
//	        "listChanged": true
//	    }
//	}
//
// See: https://spec.modelcontextprotocol.io/specification/2025-03-26/client/roots/
package mcp

import (
	"sync"
)

// Root represents a workspace boundary.
// Roots define directories or URIs that the server should consider as project
// roots when processing requests.
type Root struct {
	// URI is the URI of the root. Must be an absolute URI.
	// Examples: file:///home/user/project, https://example.com/repo
	URI string `json:"uri"`

	// Name is an optional human-readable name for the root.
	// If empty, clients may derive a name from the URI.
	Name string `json:"name,omitempty"`
}

// RootsList represents roots from the client.
// This is the response format for the roots/list request.
type RootsList struct {
	// Roots is the list of workspace roots.
	Roots []Root `json:"roots"`
}

// RootManager manages workspace roots with thread-safe operations.
// It provides methods for adding, removing, and querying roots, as well as
// notifying registered handlers when roots change.
type RootManager struct {
	mu          sync.RWMutex
	roots       []Root
	listChanged bool
	handlers    []func([]Root)
}

// NewRootManager creates a new RootManager.
//
// Parameters:
//   - listChanged: indicates whether the client supports listChanged notifications
//
// Returns:
//   - *RootManager: a new root manager instance
func NewRootManager(listChanged bool) *RootManager {
	return &RootManager{
		roots:       make([]Root, 0),
		listChanged: listChanged,
		handlers:    make([]func([]Root), 0),
	}
}

// SetRoots replaces all roots with the provided list.
// This method is thread-safe and notifies all registered handlers after updating.
//
// Parameters:
//   - roots: the new list of roots to set
func (rm *RootManager) SetRoots(roots []Root) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Create a defensive copy
	rm.roots = make([]Root, len(roots))
	copy(rm.roots, roots)

	rm.notifyHandlers()
}

// GetRoots returns a copy of all current roots.
// This method is thread-safe and returns a defensive copy to prevent
// external modification of internal state.
//
// Returns:
//   - []Root: a defensive copy of the current roots list
func (rm *RootManager) GetRoots() []Root {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]Root, len(rm.roots))
	copy(result, rm.roots)
	return result
}

// AddRoot adds a single root to the list.
// This method is thread-safe and notifies all registered handlers after adding.
// Duplicate URIs are not checked; callers should use GetRoots and check manually
// if deduplication is required.
//
// Parameters:
//   - root: the root to add
func (rm *RootManager) AddRoot(root Root) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.roots = append(rm.roots, root)
	rm.notifyHandlers()
}

// RemoveRoot removes a root by its URI.
// This method is thread-safe and notifies all registered handlers after removing.
// If no root with the given URI exists, this is a no-op.
//
// Parameters:
//   - uri: the URI of the root to remove
func (rm *RootManager) RemoveRoot(uri string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	filtered := make([]Root, 0, len(rm.roots))
	for _, r := range rm.roots {
		if r.URI != uri {
			filtered = append(filtered, r)
		}
	}
	rm.roots = filtered
	rm.notifyHandlers()
}

// OnChange registers a handler function that will be called whenever
// the roots list changes (via SetRoots, AddRoot, or RemoveRoot).
// Handlers are invoked asynchronously in goroutines to prevent blocking.
//
// Parameters:
//   - handler: function to call when roots change, receives a defensive copy
func (rm *RootManager) OnChange(handler func([]Root)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.handlers = append(rm.handlers, handler)
}

// notifyHandlers notifies all registered handlers of a change.
// This method must be called with rm.mu held (locked for writing).
func (rm *RootManager) notifyHandlers() {
	// Create a defensive copy for handlers
	roots := make([]Root, len(rm.roots))
	copy(roots, rm.roots)

	// Notify handlers asynchronously to prevent blocking
	for _, h := range rm.handlers {
		go h(roots)
	}
}

// SupportsListChanged returns whether the client supports listChanged notifications.
//
// Returns:
//   - bool: true if listChanged is supported
func (rm *RootManager) SupportsListChanged() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.listChanged
}

// ListRootsRequest represents a roots/list request from the server.
// The server sends this to query the client for current roots.
type ListRootsRequest struct{}

// ListRootsResult represents the response to a roots/list request.
type ListRootsResult struct {
	// Roots is the list of workspace roots.
	Roots []Root `json:"roots"`
}

// RootsListChangedNotification represents a roots/list_changed notification.
// The client sends this to notify the server that roots have changed.
type RootsListChangedNotification struct{}

// IsWithinRoots checks if a given URI is within any of the registered roots.
// This is useful for access control and path validation.
// A URI is considered within a root if it is either:
//   - An exact match with the root URI
//   - A child path of the root (root + "/" + path)
//
// Parameters:
//   - uri: the URI to check
//
// Returns:
//   - bool: true if the URI is within at least one root
func (rm *RootManager) IsWithinRoots(uri string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, root := range rm.roots {
		if len(root.URI) == 0 {
			continue
		}

		// Exact match
		if uri == root.URI {
			return true
		}

		// Check if uri starts with root URI and is followed by a separator
		if len(uri) > len(root.URI) {
			prefix := uri[:len(root.URI)]
			suffix := uri[len(root.URI):]
			if prefix == root.URI {
				// Must be followed by / or ? or # to be a proper child path
				if len(suffix) > 0 && (suffix[0] == '/' || suffix[0] == '?' || suffix[0] == '#') {
					return true
				}
			}
		}
	}
	return false
}

// Count returns the number of registered roots.
//
// Returns:
//   - int: the number of roots
func (rm *RootManager) Count() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return len(rm.roots)
}

// Clear removes all roots.
// This method is thread-safe and notifies all registered handlers.
func (rm *RootManager) Clear() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.roots = make([]Root, 0)
	rm.notifyHandlers()
}

// GetRootByURI returns a root by its URI.
//
// Parameters:
//   - uri: the URI to search for
//
// Returns:
//   - (Root, bool): the root and true if found, or zero value and false
func (rm *RootManager) GetRootByURI(uri string) (Root, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, r := range rm.roots {
		if r.URI == uri {
			return r, true
		}
	}
	return Root{}, false
}
