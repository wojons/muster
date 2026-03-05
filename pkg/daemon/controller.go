package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Controller manages daemon lifecycle
type Controller struct {
	pidManager *Manager
	logger     Logger
	socketPath string
	httpServer *http.Server
	mux        *http.ServeMux
}

// Logger interface for daemon logging
type Logger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// Config for controller initialization
type ControllerConfig struct {
	PidFilePath string
	SocketPath  string
	Version     string
	Logger      Logger
	Handler     http.Handler
}

// NewController creates a new daemon controller
func NewController(cfg ControllerConfig) (*Controller, error) {
	pidManager, err := NewManager(Config{
		PidFilePath: cfg.PidFilePath,
		SocketPath:  cfg.SocketPath,
		Version:     cfg.Version,
	})
	if err != nil {
		return nil, err
	}

	if cfg.Logger == nil {
		cfg.Logger = &defaultLogger{}
	}

	mux := http.NewServeMux()
	if cfg.Handler != nil {
		mux.Handle("/", cfg.Handler)
	}

	return &Controller{
		pidManager: pidManager,
		logger:     cfg.Logger,
		socketPath: cfg.SocketPath,
		mux:        mux,
		httpServer: &http.Server{
			Handler: mux,
		},
	}, nil
}

// StartWithSocket starts the daemon with Unix socket listener
func (c *Controller) StartWithSocket(handler http.HandlerFunc) error {
	if err := c.pidManager.Acquire(); err != nil {
		return err
	}

	c.httpServer = &http.Server{
		Handler: http.HandlerFunc(handler),
	}

	// Remove existing socket file if present
	os.Remove(c.socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}

	// Set restrictive permissions (owner only)
	if err := os.Chmod(c.socketPath, 0600); err != nil {
		return fmt.Errorf("set socket permissions: %w", err)
	}

	c.logger.Info("daemon started with socket", "socket", c.socketPath)

	// Start serving in background
	go func() {
		if err := c.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			c.logger.Error("socket server error", "error", err)
		}
	}()

	return nil
}

// ServeHTTP serves HTTP requests on the Unix socket
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}

// Shutdown gracefully shuts down the HTTP server
func (c *Controller) Shutdown(ctx context.Context) error {
	c.logger.Info("shutting down daemon HTTP server")
	return c.httpServer.Shutdown(ctx)
}

// Start attempts to start the daemon process
func (c *Controller) Start() error {
	// Check if already running
	if status, err := c.pidManager.GetStatus(); err == nil {
		if status.Running {
			return fmt.Errorf("daemon already running (PID: %d)", status.PID)
		}
	}

	// For macOS/Linux, we would fork the process
	// For this implementation, we'll assume the caller will handle forking
	// and just acquire the PID lock
	if err := c.pidManager.Acquire(); err != nil {
		return err
	}

	c.logger.Info("daemon started", "pid", os.Getpid())
	return nil
}

// Stop attempts to stop the daemon process
func (c *Controller) Stop() error {
	status, err := c.pidManager.GetStatus()
	if err != nil {
		return fmt.Errorf("get daemon status: %w", err)
	}

	if !status.Running {
		return fmt.Errorf("daemon is not running")
	}

	pid := status.PID
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	// Graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait for shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			// Force kill if still running
			c.logger.Warn("daemon did not shut down gracefully, force killing", "pid", pid)
			process.Kill()

			// Clean up PID file
			c.pidManager.Release()

			return nil
		case <-ticker.C:
			// Check if process still running
			if !isProcessRunning(pid) {
				c.logger.Info("daemon stopped", "pid", pid)
				// Clean up PID file
				c.pidManager.Release()
				return nil
			}
		}
	}
}

// Status returns the current daemon status
func (c *Controller) Status() (*Status, error) {
	return c.pidManager.GetStatus()
}

// IsRunning checks if daemon is running
func (c *Controller) IsRunning() bool {
	status, err := c.Status()
	if err != nil {
		return false
	}
	return status.Running
}

// SetupSignalHandlers installs signal handlers for graceful shutdown
func (c *Controller) SetupSignalHandlers() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		sig := <-sigCh
		c.logger.Info("received signal, shutting down", "signal", sig)

		// Release PID file
		if err := c.pidManager.Release(); err != nil {
			c.logger.Error("failed to release PID file", "error", err)
		}

		os.Exit(0)
	}()
}

// Restart attempts to restart the daemon
func (c *Controller) Restart() error {
	c.logger.Info("restarting daemon")

	// Stop if running
	if c.IsRunning() {
		if err := c.Stop(); err != nil {
			return fmt.Errorf("stop daemon: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Start new instance
	return c.Start()
}

// Kill forcefully terminates the daemon
func (c *Controller) Kill() error {
	status, err := c.Status()
	if err != nil {
		return err
	}

	if !status.Running {
		return fmt.Errorf("daemon is not running")
	}

	pid := status.PID
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Kill(); err != nil {
		return err
	}

	// Clean up PID file
	c.pidManager.Release()
	return nil
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...interface{})  {}
func (l *defaultLogger) Warn(msg string, args ...interface{})  {}
func (l *defaultLogger) Error(msg string, args ...interface{}) {}
