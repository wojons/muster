# Daemon Entry Point - main.go

Complete Go daemon implementation with Unix socket proxy server.

## File: cmd/daemon/main.go

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultConfigPath = "/etc/openapi-cli/daemon.json"
	defaultSocketPath = "/var/run/openapi-cli/daemon.sock"
	defaultPIDFile    = "/var/run/openapi-cli/daemon.pid"
	defaultLogFile    = "/var/log/openapi-cli/daemon.log"
)

// Config holds daemon configuration
type Config struct {
	SocketPath      string        `json:"socket_path"`
	PIDFile         string        `json:"pid_file"`
	LogFile         string        `json:"log_file"`
	TargetURL       string        `json:"target_url"`
	SocketMode      os.FileMode   `json:"socket_mode"`
	MaxConnections  int           `json:"max_connections"`
	KeepAlive       bool          `json:"keep_alive"`
	KeepAlivePeriod time.Duration `json:"keep_alive_period"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		SocketPath:      defaultSocketPath,
		PIDFile:         defaultPIDFile,
		LogFile:         defaultLogFile,
		TargetURL:       "http://localhost:8080",
		SocketMode:      0660,
		MaxConnections:  100,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

// Daemon represents the daemon process
type Daemon struct {
	config     *Config
	listener   net.Listener
	server     *http.Server
	proxy      *httputil.ReverseProxy
	logger     *log.Logger
	logFile    *os.File
	pidFile    *PIDFile
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	reloadChan chan struct{}
}

// NewDaemon creates a new daemon instance
func NewDaemon(config *Config) (*Daemon, error) {
	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		reloadChan: make(chan struct{}, 1),
	}

	// Setup logging
	if err := d.setupLogging(); err != nil {
		return nil, fmt.Errorf("failed to setup logging: %w", err)
	}

	// Setup proxy
	if err := d.setupProxy(); err != nil {
		return nil, fmt.Errorf("failed to setup proxy: %w", err)
	}

	// Setup HTTP server
	d.server = &http.Server{
		Handler:      d,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return d, nil
}

// setupLogging initializes log file
func (d *Daemon) setupLogging() error {
	// Ensure log directory exists
	logDir := filepath.Dir(d.config.LogFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(d.config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	d.logFile = logFile
	d.logger = log.New(logFile, "[openapi-daemon] ", log.LstdFlags|log.Lmicroseconds)

	return nil
}

// setupProxy initializes reverse proxy
func (d *Daemon) setupProxy() error {
	target, err := url.Parse(d.config.TargetURL)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	d.proxy = httputil.NewSingleHostReverseProxy(target)
	d.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		d.logger.Printf("Proxy error: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Configure transport with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        d.config.MaxConnections,
		MaxIdleConnsPerHost: d.config.MaxConnections,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: d.config.KeepAlivePeriod,
		}).DialContext,
	}

	if d.config.KeepAlive {
		transport.DisableKeepAlives = false
	} else {
		transport.DisableKeepAlives = true
	}

	d.proxy.Transport = transport

	return nil
}

// ServeHTTP implements http.Handler
func (d *Daemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Add context for cancellation
	r = r.WithContext(d.ctx)

	// Log request
	d.logger.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Forward request
	d.proxy.ServeHTTP(w, r)

	// Log completion
	d.logger.Printf("Completed in %v", time.Since(start))
}

// Start initializes and starts the daemon
func (d *Daemon) Start() error {
	d.logger.Println("Starting daemon...")

	// Create PID file
	pidFile, err := NewPIDFile(d.config.PIDFile)
	if err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}
	d.pidFile = pidFile

	// Ensure socket directory exists
	socketDir := filepath.Dir(d.config.SocketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove existing socket
	if err := os.Remove(d.config.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	d.listener = listener

	// Set socket permissions
	if err := os.Chmod(d.config.SocketPath, d.config.SocketMode); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	d.logger.Printf("Listening on %s", d.config.SocketPath)

	// Start signal handler
	d.wg.Add(1)
	go d.signalHandler()

	// Start reload watcher
	d.wg.Add(1)
	go d.reloadWatcher()

	// Start server
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.server.Serve(d.listener); err != nil && err != http.ErrServerClosed {
			d.logger.Printf("Server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() error {
	d.logger.Println("Stopping daemon...")

	// Cancel context
	d.cancel()

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), d.config.ShutdownTimeout)
	defer cancel()

	if err := d.server.Shutdown(ctx); err != nil {
		d.logger.Printf("Server shutdown error: %v", err)
	}

	// Close listener
	if d.listener != nil {
		d.listener.Close()
	}

	// Wait for goroutines
	d.wg.Wait()

	// Remove PID file
	if d.pidFile != nil {
		d.pidFile.Remove()
	}

	// Remove socket
	os.Remove(d.config.SocketPath)

	// Close log file
	if d.logFile != nil {
		d.logFile.Close()
	}

	return nil
}

// Reload reloads configuration
func (d *Daemon) Reload() error {
	d.logger.Println("Reloading configuration...")

	// Load new config
	config, err := LoadConfig(defaultConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update configuration atomically
	d.mu.Lock()
	d.config = config
	d.mu.Unlock()

	// Reopen log file for rotation
	if d.logFile != nil {
		d.logFile.Close()
	}
	if err := d.setupLogging(); err != nil {
		return fmt.Errorf("failed to reopen log file: %w", err)
	}

	d.logger.Println("Configuration reloaded successfully")
	return nil
}

// signalHandler handles OS signals
func (d *Daemon) signalHandler() {
	defer d.wg.Done()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2)

	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				d.logger.Printf("Received signal: %v", sig)
				d.Stop()
				os.Exit(0)

			case syscall.SIGHUP:
				d.logger.Println("Received SIGHUP, reloading...")
				select {
				case d.reloadChan <- struct{}{}:
				default:
				}

			case syscall.SIGUSR1:
				d.logger.Println("Received SIGUSR1")
				// Custom action for USR1

			case syscall.SIGUSR2:
				d.logger.Println("Received SIGUSR2")
				// Custom action for USR2
			}

		case <-d.ctx.Done():
			return
		}
	}
}

// reloadWatcher watches for reload requests
func (d *Daemon) reloadWatcher() {
	defer d.wg.Done()

	for {
		select {
		case <-d.reloadChan:
			if err := d.Reload(); err != nil {
				d.logger.Printf("Reload failed: %v", err)
			}
		case <-d.ctx.Done():
			return
		}
	}
}

// PIDFile manages the PID file
type PIDFile struct {
	path string
}

// NewPIDFile creates a new PID file
func NewPIDFile(path string) (*PIDFile, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Check if already running
	if data, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil {
			if processExists(pid) {
				return nil, fmt.Errorf("daemon already running (PID %d)", pid)
			}
		}
	}

	// Write PID
	pid := os.Getpid()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return nil, err
	}

	return &PIDFile{path: path}, nil
}

// Remove removes the PID file
func (p *PIDFile) Remove() error {
	return os.Remove(p.path)
}

// processExists checks if a process exists
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Daemonize forks the process into background
func Daemonize() error {
	// Fork process
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to fork: %w", err)
	}

	// Parent exits
	fmt.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
	os.Exit(0)

	return nil // Never reached
}

// Status checks daemon status
func Status(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon not running")
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file")
	}

	if !processExists(pid) {
		return fmt.Errorf("daemon not running (stale PID file)")
	}

	fmt.Printf("Daemon running (PID %d)\n", pid)
	return nil
}

// StopDaemon stops a running daemon
func StopDaemon(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon not running")
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file")
	}

	if !processExists(pid) {
		os.Remove(pidFile)
		return fmt.Errorf("daemon not running (stale PID file removed)")
	}

	// Send SIGTERM
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 30; i++ {
			if !processExists(pid) {
				done <- nil
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		done <- fmt.Errorf("timeout waiting for daemon to stop")
	}()

	if err := <-done; err != nil {
		// Force kill
		process.Kill()
		return fmt.Errorf("daemon killed")
	}

	fmt.Println("Daemon stopped")
	return nil
}

// RestartDaemon restarts the daemon
func RestartDaemon(pidFile string) error {
	// Stop if running
	if _, err := os.Stat(pidFile); err == nil {
		if err := StopDaemon(pidFile); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Start new daemon
	return Daemonize()
}

func main() {
	var (
		configPath = flag.String("config", defaultConfigPath, "Configuration file path")
		foreground = flag.Bool("foreground", false, "Run in foreground")
		command    = flag.String("command", "", "Command: start, stop, restart, status")
	)
	flag.Parse()

	// Handle commands
	switch *command {
	case "status":
		if err := Status(defaultPIDFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "stop":
		if err := StopDaemon(defaultPIDFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "restart":
		if err := RestartDaemon(defaultPIDFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "start":
		// Continue to start

	case "":
		// No command, start daemon

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		fmt.Fprintf(os.Stderr, "Usage: daemon [-command=start|stop|restart|status] [-config=path] [-foreground]\n")
		os.Exit(1)
	}

	// Load configuration
	config, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Daemonize unless foreground mode
	if !*foreground {
		if err := Daemonize(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to daemonize: %v\n", err)
			os.Exit(1)
		}
	}

	// Create and start daemon
	daemon, err := NewDaemon(config)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	if err := daemon.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Wait for shutdown
	daemon.wg.Wait()
}
```

## File: configs/daemon.json

```json
{
  "socket_path": "/var/run/openapi-cli/daemon.sock",
  "pid_file": "/var/run/openapi-cli/daemon.pid",
  "log_file": "/var/log/openapi-cli/daemon.log",
  "target_url": "http://localhost:8080",
  "socket_mode": 416,
  "max_connections": 100,
  "keep_alive": true,
  "keep_alive_period": 30000000000,
  "read_timeout": 30000000000,
  "write_timeout": 30000000000,
  "shutdown_timeout": 30000000000
}
```

## File: Makefile

```makefile
.PHONY: build install start stop restart status clean

BINARY=openapi-daemon
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/openapi-cli
LOG_DIR=/var/log/openapi-cli
RUN_DIR=/var/run/openapi-cli

build:
	go build -o bin/$(BINARY) cmd/daemon/main.go

install: build
	mkdir -p $(INSTALL_DIR)
	mkdir -p $(CONFIG_DIR)
	mkdir -p $(LOG_DIR)
	mkdir -p $(RUN_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/
	cp configs/daemon.json $(CONFIG_DIR)/

start:
	$(INSTALL_DIR)/$(BINARY) -command=start

stop:
	$(INSTALL_DIR)/$(BINARY) -command=stop

restart:
	$(INSTALL_DIR)/$(BINARY) -command=restart

status:
	$(INSTALL_DIR)/$(BINARY) -command=status

foreground:
	$(INSTALL_DIR)/$(BINARY) -foreground

clean:
	rm -f bin/$(BINARY)
	rm -f $(INSTALL_DIR)/$(BINARY)
```

## File: scripts/daemon.sh

```bash
#!/bin/bash

DAEMON_NAME="openapi-daemon"
DAEMON_BIN="/usr/local/bin/${DAEMON_NAME}"
PID_FILE="/var/run/openapi-cli/daemon.pid"

start() {
    if [ -f "$PID_FILE" ]; then
        echo "Daemon already running"
        exit 1
    fi
    
    echo "Starting ${DAEMON_NAME}..."
    $DAEMON_BIN -command=start
}

stop() {
    echo "Stopping ${DAEMON_NAME}..."
    $DAEMON_BIN -command=stop
}

restart() {
    echo "Restarting ${DAEMON_NAME}..."
    $DAEMON_BIN -command=restart
}

status() {
    $DAEMON_BIN -command=status
}

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    restart)
        restart
        ;;
    status)
        status
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
```

## Usage

```bash
# Build
cd /Users/lexykwaii/Code/muster
make build

# Install
sudo make install

# Commands
sudo openapi-daemon -command=start      # Start daemon
sudo openapi-daemon -command=stop       # Stop daemon
sudo openapi-daemon -command=restart    # Restart daemon
sudo openapi-daemon -command=status     # Check status

# Foreground mode (for debugging)
sudo openapi-daemon -foreground
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Daemon Process                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   Signals    │    │    HTTP      │    │    Proxy     │   │
│  │  (HUP,TERM)  │───▶│   Server     │───▶│  (Reverse)   │   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
│         │                   │                     │          │
│         ▼                   ▼                     ▼          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   Reload     │    │  Unix Socket │    │  Target App  │   │
│  │  (Hot)       │    │              │    │              │   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   PID File   │    │   Logging    │    │   Config     │   │
│  │              │    │  (Rotation)  │    │  (Reload)    │   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```
