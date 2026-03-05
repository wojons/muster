# Daemon Socket - Unix Socket Management

Complete Unix socket implementation for daemon communication.

## File: internal/daemon/socket/socket.go

```go
package socket

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// Socket manages Unix socket lifecycle
type Socket struct {
	path     string
	mode     os.FileMode
	listener net.Listener
}

// Config defines socket configuration
type Config struct {
	Path string
	Mode os.FileMode
}

// New creates a new socket manager
func New(cfg Config) *Socket {
	if cfg.Mode == 0 {
		cfg.Mode = 0660
	}
	return &Socket{
		path: cfg.Path,
		mode: cfg.Mode,
	}
}

// Create initializes the Unix socket
func (s *Socket) Create() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}

	if err := os.Chmod(s.path, s.mode); err != nil {
		listener.Close()
		return fmt.Errorf("set socket permissions: %w", err)
	}

	s.listener = listener
	return nil
}

// Close shuts down the socket
func (s *Socket) Close() error {
	if s.listener != nil {
		s.listener.Close()
	}
	return os.Remove(s.path)
}

// Listener returns the socket listener
func (s *Socket) Listener() net.Listener {
	return s.listener
}

// Path returns the socket path
func (s *Socket) Path() string {
	return s.path
}

// SetMode changes socket permissions
func (s *Socket) SetMode(mode os.FileMode) error {
	s.mode = mode
	if _, err := os.Stat(s.path); err == nil {
		return os.Chmod(s.path, mode)
	}
	return nil
}

// IsValid checks if socket path is valid
func (s *Socket) IsValid() error {
	if s.path == "" {
		return fmt.Errorf("socket path is empty")
	}
	if len(s.path) > 108 {
		return fmt.Errorf("socket path too long (max 108 chars)")
	}
	return nil
}

// GetGID returns socket group ID
func (s *Socket) GetGID() (int, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("cannot get socket GID")
	}
	return int(stat.Gid), nil
}

// IsInUse checks if socket is already bound
func IsInUse(path string) bool {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Cleanup removes stale socket files
func Cleanup(path string) error {
	if IsInUse(path) {
		return fmt.Errorf("socket is in use")
	}
	return os.Remove(path)
}
```

## File: internal/daemon/socket/listener.go

```go
package socket

import (
	"context"
	"net"
	"sync"
	"time"
)

// ConnectionHandler handles incoming connections
type ConnectionHandler interface {
	Handle(ctx context.Context, conn net.Conn)
}

// Listener wraps net.Listener with connection management
type ManagedListener struct {
	listener net.Listener
	handler  ConnectionHandler
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewManagedListener creates a managed listener
func NewManagedListener(listener net.Listener, handler ConnectionHandler) *ManagedListener {
	ctx, cancel := context.WithCancel(context.Background())
	return &ManagedListener{
		listener: listener,
		handler:  handler,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins accepting connections
func (ml *ManagedListener) Start() error {
	for {
		conn, err := ml.listener.Accept()
		if err != nil {
			select {
			case <-ml.ctx.Done():
				return nil
			default:
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				return err
			}
		}

		ml.wg.Add(1)
		go ml.handleConnection(conn)
	}
}

// handleConnection processes a single connection
func (ml *ManagedListener) handleConnection(conn net.Conn) {
	defer ml.wg.Done()
	defer conn.Close()

	ctx, cancel := context.WithTimeout(ml.ctx, 30*time.Second)
	defer cancel()

	ml.handler.Handle(ctx, conn)
}

// Stop gracefully stops the listener
func (ml *ManagedListener) Stop(timeout time.Duration) error {
	ml.cancel()

	done := make(chan struct{})
	go func() {
		ml.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return ml.listener.Close()
	case <-time.After(timeout):
		return ml.listener.Close()
	}
}
```

## Usage

```go
socket := socket.New(socket.Config{
	Path: "/var/run/muster/daemon.sock",
	Mode: 0660,
})

if err := socket.Create(); err != nil {
	log.Fatal(err)
}
defer socket.Close()

listener := socket.NewManagedListener(socket.Listener(), handler)
go listener.Start()
```
