# Unix Socket Proxy Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Network

## Overview

Complete specification of **Unix domain socket proxy** implementation.

## 1. Socket Creation

```go
type UnixSocketProxy struct {
    path   string
    server *http.Server
}

func NewUnixSocketProxy(path string, handler http.Handler) *UnixSocketProxy {
    return &UnixSocketProxy{
        path: path,
        server: &http.Server{
            Handler: handler,
        },
    }
}
```

## 2. Server Lifecycle

```go
func (p *UnixSocketProxy) Start() error {
    // Ensure directory exists
    dir := filepath.Dir(p.path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    // Remove existing socket
    if _, err := os.Stat(p.path); err == nil {
        if err := os.Remove(p.path); err != nil {
            return fmt.Errorf("failed to remove existing socket: %w", err)
        }
    }
    
    // Create listener
    listener, err := net.Listen("unix", p.path)
    if err != nil {
        return fmt.Errorf("failed to listen on socket: %w", err)
    }
    
    // Set secure permissions
    if err := os.Chmod(p.path, 0600); err != nil {
        listener.Close()
        return fmt.Errorf("failed to set socket permissions: %w", err)
    }
    
    go p.server.Serve(listener)
    return nil
}

func (p *UnixSocketProxy) Stop() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := p.server.Shutdown(ctx); err != nil {
        return err
    }
    
    os.Remove(p.path)
    return nil
}
```

## 3. HTTP Client

```go
type UnixSocketClient struct {
    socketPath string
    client     *http.Client
}

func NewUnixSocketClient(socketPath string) *UnixSocketClient {
    transport := &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            var d net.Dialer
            return d.DialContext(ctx, "unix", socketPath)
        },
    }
    
    return &UnixSocketClient{
        socketPath: socketPath,
        client: &http.Client{
            Transport: transport,
            Timeout:   30 * time.Second,
        },
    }
}

func (c *UnixSocketClient) Do(req *http.Request) (*http.Response, error) {
    // Force HTTP scheme for Unix socket
    req.URL.Scheme = "http"
    req.URL.Host = "localhost"
    
    return c.client.Do(req)
}
```

## 4. Security

```go
func (p *UnixSocketProxy) validateSocket() error {
    info, err := os.Stat(p.path)
    if err != nil {
        return err
    }
    
    mode := info.Mode()
    
    // Check if it's a socket
    if mode&os.ModeSocket == 0 {
        return errors.New("path is not a socket")
    }
    
    // Check permissions
    if mode.Perm()&007 != 0 {
        return errors.New("socket has unsafe permissions")
    }
    
    return nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
