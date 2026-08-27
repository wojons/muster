# Daemon Package Examples

This document provides comprehensive examples for using the `pkg/daemon` package, which manages background processes and Unix socket communication.

## Overview

The `daemon` package provides:
- Daemon lifecycle management (start, stop, restart, status)
- PID file management for tracking running daemons
- Unix socket proxy for internal communication
- HTTP routing for service endpoints
- Signal handling and graceful shutdown

## Basic Daemon Management

### Starting a Daemon

```go
package main

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/wojons/muster/pkg/daemon"
)

func main() {
    ctx := context.Background()

    // Create daemon controller
    controller, err := daemon.NewController(&daemon.ControllerOptions{
        PidDirectory: "/var/run/muster",
        SocketDirectory: "/tmp/muster",
        LogDirectory: "/var/log/muster",
        User: os.Getuid(),
        Group: os.Getgid(),
    })
    if err != nil {
        panic(err)
    }

    // Create daemon configuration
    config := &daemon.Config{
        Name:        "muster-daemon",
        Description: "OpenAPI CLI Daemon Service",
        Executable:  "/usr/local/bin/openapi-cli",
        Args:        []string{"daemon", "run"},
        Environment: []string{},
        WorkingDir:  "/var/lib/muster",
        Stdout:      "/var/log/muster/stdout.log",
        Stderr:      "/var/log/muster/stderr.log",
    }

    // Start daemon
    pid, err := controller.Start(ctx, config)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Daemon started with PID: %d\n", pid)
}
```

### Stopping a Daemon

```go
func stopDaemon(ctx context.Context, controller *daemon.Controller) error {
    name := "muster-daemon"

    err := controller.Stop(ctx, name)
    if err != nil {
        return fmt.Errorf("failed to stop daemon: %w", err)
    }

    fmt.Printf("Daemon %s stopped successfully\n", name)
    return nil
}
```

### Getting Daemon Status

```go
func getDaemonStatus(ctx context.Context, controller *daemon.Controller) error {
    name := "muster-daemon"

    status, err := controller.Status(ctx, name)
    if err != nil {
        return fmt.Errorf("failed to get status: %w", err)
    }

    fmt.Printf("Status: %s\n", status.State)
    fmt.Printf("PID: %d\n", status.PID)
    fmt.Printf("Uptime: %v\n", status.Uptime)

    if status.SocketPath != "" {
        fmt.Printf("Socket: %s\n", status.SocketPath)
    }

    return nil
}
```

## PID File Management

### Creating PID Files

```go
func createPIDFile(pid *daemon.PIDManager, name string, pidFile int) error {
    info := &daemon.PIDInfo{
        PID:        pidFile,
        StartedAt:  time.Now(),
        Args:       []string{"openapi-cli", "daemon"},
        WorkingDir: "/var/lib/muster",
    }

    err := pid.Create(name, info)
    if err != nil {
        return fmt.Errorf("failed to create PID file: %w", err)
    }

    fmt.Printf("Created PID file for %s\n", name)
    return nil
}
```

### Reading PID Files

```go
func readPIDFile(pid *daemon.PIDManager, name string) (*daemon.PIDInfo, error) {
    info, err := pid.Read(name)
    if err != nil {
        return nil, fmt.Errorf("failed to read PID file: %w", err)
    }

    fmt.Printf("PID: %d\n", info.PID)
    fmt.Printf("Started: %v\n", info.StartedAt)
    fmt.Printf("Args: %v\n", info.Args)

    return info, nil
}
```

### Checking PID Validity

```go
func checkPIDValid(pid *daemon.PIDManager, name string) (bool, error) {
    valid, err := pid.IsValid(name)
    if err != nil {
        return false, fmt.Errorf("failed to check PID validity: %w", err)
    }

    if valid {
        fmt.Printf("Daemon %s is running\n", name)
    } else {
        fmt.Printf("Daemon %s is not running\n", name)
    }

    return valid, nil
}
```

### Removing PID Files

```go
func removePIDFile(pid *daemon.PIDManager, name string) error {
    err := pid.Remove(name)
    if err != nil {
        return fmt.Errorf("failed to remove PID file: %w", err)
    }

    fmt.Printf("Removed PID file for %s\n", name)
    return nil
}
```

## Unix Socket Communication

### Creating a Socket Server

```go
type APIServer struct{}

func (s *APIServer) Serve(lis net.Listener) error {
    for {
        conn, err := lis.Accept()
        if err != nil {
            return err
        }

        go s.handleConnection(conn)
    }
}

func (s *APIServer) handleConnection(conn net.Conn) {
    defer conn.Close()

    decoder := json.NewDecoder(conn)
    encoder := json.NewEncoder(conn)

    for {
        var request map[string]interface{}
        if err := decoder.Decode(&request); err != nil {
            if err == io.EOF {
                break
            }
            log.Printf("Decode error: %v", err)
            continue
        }

        // Process request
        response := map[string]interface{}{
            "status": "ok",
            "result": "processed",
        }

        if err := encoder.Encode(response); err != nil {
            log.Printf("Encode error: %v", err)
            break
        }
    }
}

func createSocketServer(ctx context.Context) error {
    socketDir := "/tmp/muster"
    socketPath := filepath.Join(socketDir, "muster.sock")

    // Ensure socket directory exists
    os.MkdirAll(socketDir, 0755)

    // Remove existing socket if present
    os.Remove(socketPath)

    // Create listener
    lis, err := net.Listen("unix", socketPath)
    if err != nil {
        return fmt.Errorf("failed to listen on socket: %w", err)
    }
    defer lis.Close()

    // Set socket permissions
    os.Chmod(socketPath, 0660)

    // Start server
    server := &APIServer{}
    go server.Serve(lis)

    fmt.Printf("Socket server listening on %s\n", socketPath)

    // Wait for context cancellation
    <-ctx.Done()
    fmt.Println("Shutting down socket server")

    return nil
}

func main() {
    ctx := context.Background()
    _, cancel := context.WithCancel(ctx)
    defer cancel()

    // Setup signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        cancel()
    }()

    if err := createSocketServer(ctx); err != nil {
        panic(err)
    }
}
```

### Connecting to Socket Server

```go
type SocketClient struct {
    conn net.Conn
    dec  *json.Decoder
}

func NewSocketClient(socketPath string) (*SocketClient, error) {
    conn, err := net.Dial("unix", socketPath)
    if err != nil {
        return nil, err
    }

    return &SocketClient{
        conn: conn,
        dec:  json.NewDecoder(conn),
    }, nil
}

func (c *SocketClient) Request(request map[string]interface{}) (map[string]interface{}, error) {
    encoder := json.NewEncoder(c.conn)

    if err := encoder.Encode(request); err != nil {
        return nil, fmt.Errorf("encode error: %w", err)
    }

    var response map[string]interface{}
    if err := c.dec.Decode(&response); err != nil {
        return nil, fmt.Errorf("decode error: %w", err)
    }

    return response, nil
}

func (c *SocketClient) Close() error {
    return c.conn.Close()
}

func main() {
    socketPath := "/tmp/muster/muster.sock"

    client, err := NewSocketClient(socketPath)
    if err != nil {
        panic(err)
    }
    defer client.Close()

    request := map[string]interface{}{
        "method": "GET",
        "path":   "/api/v1/users",
    }

    response, err := client.Request(request)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Response: %+v\n", response)
}
```

## HTTP Proxy

### Creating HTTP Proxy

```go
type HTTPProxy struct {
    transport *http.Transport
    socketDir string
}

func NewHTTPProxy(socketDir string) (*HTTPProxy, error) {
    return &HTTPProxy{
        socketDir: socketDir,
        transport: &http.Transport{
            DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
                socketPath := filepath.Join(socketDir, "muster.sock")

                dialer := &net.Dialer{}
                conn, err := dialer.DialContext(ctx, "unix", socketPath)
                if err != nil {
                    return nil, err
                }

                return conn, nil
            },
        },
    }, nil
}

func (p *HTTPProxy) ListenAndServe(ctx context.Context, addr string) error {
    server := &http.Server{
        Addr:     addr,
        Handler:  p,
        BaseContext: func(_ net.Listener) context.Context {
            return ctx
        },
    }

    errCh := make(chan error, 1)
    go func() {
        errCh <- server.ListenAndServe()
    }()

    select {
    case err := <-errCh:
        return err
    case <-ctx.Done():
        server.Shutdown(ctx)
        return ctx.Err()
    }
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Parse request
    var request map[string]interface{}
    if r.Body != nil {
        json.NewDecoder(r.Body).Decode(&request)
    }

    // Add method and path
    if request == nil {
        request = make(map[string]interface{})
    }
    request["method"] = r.Method
    request["path"] = r.URL.Path

    // Forward to socket via HTTP client
    client := &http.Client{
        Transport: p.transport,
    }

    // Create HTTP request
    reqBody, _ := json.Marshal(request)
    req, err := http.NewRequest("POST", "http://unix/", bytes.NewReader(reqBody))
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    resp, err := client.Do(req)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    defer resp.Body.Close()

    // Copy response
    w.Header().Set("Content-Type", "application/json")
    io.Copy(w, resp.Body)
}

func main() {
    ctx := context.Background()
    _, cancel := context.WithCancel(ctx)
    defer cancel()

    proxy, err := NewHTTPProxy("/tmp/muster")
    if err != nil {
        panic(err)
    }

    // Start HTTP proxy
    go func() {
        if err := proxy.ListenAndServe(ctx, ":8080"); err != nil {
            log.Printf("HTTP proxy error: %v", err)
        }
    }()

    fmt.Println("HTTP proxy listening on :8080")

    // Wait for SIGINT/SIGTERM
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    cancel()
}
```

## Signal Handling

### Setup Signal Handlers

```go
func setupSignalHandling(cancel context.CancelFunc) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh,
        syscall.SIGINT,
        syscall.SIGTERM,
        syscall.SIGHUP,
    )

    go func() {
        for sig := range sigCh {
            log.Printf("Received signal: %v", sig)

            switch sig {
            case syscall.SIGINT, syscall.SIGTERM:
                // Graceful shutdown
                log.Println("Initiating graceful shutdown")
                cancel()
            case syscall.SIGHUP:
                // Reload configuration
                log.Println("Reloading configuration")
                reloadConfig()
            }
        }
    }()
}

func reloadConfig() {
    // Reload configuration from file
    log.Println("Configuration reloaded")
}
```

### Graceful Shutdown

```go
func main() {
    ctx := context.Background()
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    // Setup server
    server := &http.Server{
        Addr:    ":8080",
        Handler: myHandler{},
    }

    // Start server
    go func() {
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Printf("Server error: %v", err)
        }
    }()

    // Setup signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    // Wait for signal
    <-sigCh
    log.Println("Shutting down...")

    // Graceful shutdown with timeout
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        log.Printf("Shutdown error: %v", err)
    }

    log.Println("Server stopped")
}
```

## Complete Daemon Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"

    "github.com/wojons/muster/pkg/daemon"
)

func main() {
    ctx := context.Background()

    controller, err := daemon.NewController(&daemon.ControllerOptions{
        PidDirectory:    "/Users/lexykwaii/.muster/run",
        SocketDirectory: "/Users/lexykwaii/.muster/sockets",
        LogDirectory:    "/Users/lexykwaii/.muster/logs",
        User:            os.Getuid(),
        Group:           os.Getgid(),
    })

    config := &daemon.Config{
        Name:        "muster-daemon",
        Description: "OpenAPI CLI Daemon Service",
        Executable:  "/Users/lexykwaii/Code/muster/build/openapi-cli",
        Args:        []string{"daemon", "run"},
        Environment: []string{},
        WorkingDir:  "/Users/lexykwaii/Code/muster",
        Stdout:      "/Users/lexykwaii/.muster/logs/stdout.log",
        Stderr:      "/Users/lexykwaii/.muster/logs/stderr.log",
    }

    pid, err := controller.Start(ctx, config)
    if err != nil {
        log.Fatalf("Failed to start daemon: %v", err)
    }

    log.Printf("Daemon started with PID: %d", pid)

    status, err := controller.Status(ctx, "muster-daemon")
    if err != nil {
        log.Printf("Failed to get status: %v", err)
    } else {
        log.Printf("Daemon status: %s, PID: %d", status.State, status.PID)
    }
}
```

## Best Practices

### 1. Use Absolute Paths

```go
// Bad - relative path
pidDir := "run"

// Good - absolute path
pidDir := "/var/run/muster"
```

### 2. Set Proper File Permissions

```go
// Good - restrictive permissions
os.Chmod(socketPath, 0660)
os.Chmod(pidPath, 0644)
```

### 3. Handle Cleanup on Startup

```go
func startDaemon(name string, pid *daemon.PIDManager) error {
    // Check for existing PID file
    info, err := pid.Read(name)
    if err == nil {
        // Check if process is still running
        valid, _ := pid.IsValid(name)
        if valid {
            return fmt.Errorf("daemon already running with PID %d", info.PID)
        }
        // Clean up stale PID file
        pid.Remove(name)
    }

    // Start daemon
    // ...
}
```

### 4. Use Context for Cancellation

```go
// Good
go func() {
    <-ctx.Done()
    server.Shutdown(ctx)
}()

// Bad - no context
go func() {
    server.Shutdown(context.Background())
}
```

### 5. Log Daemon Events

```go
func logDaemonEvent(event, message string) {
    log.Printf("[daemon] [%s] %s\n", event, message)
}

func logDaemonEventWithDetails(event string, details map[string]interface{}) {
    data, _ := json.Marshal(details)
    log.Printf("[daemon] [%s] %s\n", event, string(data))
}
```

## Error Handling

### Daemon Errors

```go
func handleDaemonError(name string, err error) {
    switch {
    case errors.Is(err, daemon.ErrDaemonRunning):
        log.Printf("Daemon %s is already running\n", name)
    case errors.Is(err, daemon.ErrDaemonNotRunning):
        log.Printf("Daemon %s is not running\n", name)
    case errors.Is(err, daemon.ErrInvalidPID):
        log.Printf("Invalid PID file\n")
    default:
        log.Printf("Daemon error: %v\n", err)
    }
}
```

## Testing

### Mock PID Manager

```go
type MockPIDManager struct {
    pids map[string]*daemon.PIDInfo
}

func NewMockPIDManager() *MockPIDManager {
    return &MockPIDManager{
        pids: make(map[string]*daemon.PIDInfo),
    }
}

func (m *MockPIDManager) Create(name string, info *daemon.PIDInfo) error {
    m.pids[name] = info
    return nil
}

func (m *MockPIDManager) Read(name string) (*daemon.PIDInfo, error) {
    info, ok := m.pids[name]
    if !ok {
        return nil, daemon.ErrNotFound
    }
    return info, nil
}

func (m *MockPIDManager) IsValid(name string) (bool, error) {
    _, ok := m.pids[name]
    return ok, nil
}

func (m *MockPIDManager) Remove(name string) error {
    delete(m.pids, name)
    return nil
}
```
