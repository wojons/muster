# MCP Market Roots Specification

MCP roots support for defining workspace boundaries.

## Overview

Roots define workspace boundaries for the client. They represent directories or URIs that the server should consider as project roots when processing requests.

## Root Types

```go
// Root represents a workspace boundary
type Root struct {
    URI  string `json:"uri"`
    Name string `json:"name,omitempty"`
}

// RootsList represents roots from client
type RootsList struct {
    Roots []Root `json:"roots"`
}

// RootsCapability indicates client supports roots
type RootsCapability struct {
    ListChanged bool `json:"listChanged,omitempty"`
}
```

## Root Management

```go
type RootManager struct {
    mu        sync.RWMutex
    roots     []Root
    listChanged bool
    handlers  []func([]Root)
}

func NewRootManager(listChanged bool) *RootManager {
    return &RootManager{
        roots:       make([]Root, 0),
        listChanged: listChanged,
        handlers:    []func([]Root){},
    }
}

func (rm *RootManager) SetRoots(roots []Root) {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    rm.roots = roots
    rm.notifyHandlers()
}

func (rm *RootManager) GetRoots() []Root {
    rm.mu.RLock()
    defer rm.mu.RUnlock()
    result := make([]Root, len(rm.roots))
    copy(result, rm.roots)
    return result
}

func (rm *RootManager) AddRoot(root Root) {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    rm.roots = append(rm.roots, root)
    rm.notifyHandlers()
}

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

func (rm *RootManager) OnChange(handler func([]Root)) {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    rm.handlers = append(rm.handlers, handler)
}

func (rm *RootManager) notifyHandlers() {
    roots := make([]Root, len(rm.roots))
    copy(roots, rm.roots)
    for _, h := range rm.handlers {
        go h(roots)
    }
}

func (rm *RootManager) SupportsListChanged() bool {
    return rm.listChanged
}
```

## Protocol Handlers

```go
// roots/list request - server requests roots from client
type ListRootsRequest struct{}

// roots/list response
type ListRootsResult struct {
    Roots []Root `json:"roots"`
}

// notifications/roots/list_changed - client notifies server

type RootsListChangedNotification struct{}

// Server-side handlers
func (s *Server) handleRootsList(req Request) {
    s.mu.RLock()
    if !s.initialized {
        s.mu.RUnlock()
        s.sendError(req.ID, InvalidRequestCode, "Server not initialized", nil)
        return
    }
    s.mu.RUnlock()
    
    roots := s.rootManager.GetRoots()
    result := ListRootsResult{Roots: roots}
    s.sendResult(req.ID, result)
}

func (s *Server) handleRootsListChanged(req Request) {
    s.logger.Info("Roots list changed notification received")
    
    // Request updated roots from client
    go s.refreshRoots()
}

func (s *Server) refreshRoots() {
    request := Request{
        JSONRPC: "2.0",
        ID:      generateRequestID(),
        Method:  "roots/list",
    }
    
    // Send request and handle response asynchronously
    s.sendRequest(request, func(resp Response) {
        if resp.Error != nil {
            s.logger.Error("Failed to get roots", map[string]interface{}{
                "error": resp.Error.Message,
            })
            return
        }
        
        var result ListRootsResult
        if err := json.Unmarshal(resp.Result, &result); err != nil {
            s.logger.Error("Failed to unmarshal roots", map[string]interface{}{
                "error": err.Error(),
            })
            return
        }
        
        s.rootManager.SetRoots(result.Roots)
        s.logger.Info("Roots updated", map[string]interface{}{
            "count": len(result.Roots),
        })
    })
}
```

## Complete Implementation

```go
// Initialize with roots capability
func (s *Server) initializeRoots(caps ClientCapabilities) {
    if caps.Roots != nil {
        s.rootManager = NewRootManager(caps.Roots.ListChanged)
        
        // Set up change handler
        s.rootManager.OnChange(func(roots []Root) {
            s.logger.Info("Roots changed", map[string]interface{}{
                "count": len(roots),
            })
        })
        
        // Request initial roots
        go s.refreshRoots()
    }
}

// Check if URI is within root
func (s *Server) isWithinRoots(uri string) bool {
    roots := s.rootManager.GetRoots()
    for _, root := range roots {
        if strings.HasPrefix(uri, root.URI) {
            return true
        }
    }
    return false
}
```
