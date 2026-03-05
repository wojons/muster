# Socket Proxy Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Network

## Overview

Complete specification of **socket proxy** for HTTP over Unix sockets.

## 1. Proxy Structure

```go
type SocketProxy struct {
    socketPath string
    mux        *http.ServeMux
    server     *http.Server
}

func NewSocketProxy(socketPath string) *SocketProxy {
    mux := http.NewServeMux()
    
    return &SocketProxy{
        socketPath: socketPath,
        mux:        mux,
        server:     &http.Server{Handler: mux},
    }
}
```

## 2. HTTP Handler

```go
func (p *SocketProxy) Register(pattern string, handler http.Handler) {
    p.mux.Handle(pattern, handler)
}

func (p *SocketProxy) Start() error {
    // Remove existing socket
    os.Remove(p.socketPath)
    
    // Create listener
    listener, err := net.Listen("unix", p.socketPath)
    if err != nil {
        return err
    }
    
    // Set permissions
    os.Chmod(p.socketPath, 0600)
    
    return p.server.Serve(listener)
}

func (p *SocketProxy) Stop() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    return p.server.Shutdown(ctx)
}
```

## 3. Client Connection

```go
func ConnectToSocket(socketPath string) (*http.Client, error) {
    transport := &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            return net.Dial("unix", socketPath)
        },
    }
    
    return &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
    }, nil
}
```

## 4. Request Routing

```go
func (p *SocketProxy) routeCommand(w http.ResponseWriter, r *http.Request) {
    command := r.URL.Path[1:] // Remove leading /
    
    handler, ok := p.handlers[command]
    if !ok {
        http.Error(w, "Unknown command", http.StatusNotFound)
        return
    }
    
    // Execute command
    result, err := handler.Execute(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
