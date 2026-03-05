package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/openapi-cli/openapi-cli/pkg/daemon"
)

// Build-time variables
var (
	Version    = "dev"
	Commit     = "unknown"
	BuildDate  = "unknown"
	BackendURL string
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Default paths
	pidFile := "/tmp/muster-daemon.pid"
	socketPath := "/tmp/muster-daemon.sock"

	// Parse command-line args
	args := os.Args[1:]
	var action string
	if len(args) > 0 {
		action = args[0]
	}

	// Create proxy handler with backend URL
	var handler http.Handler
	if BackendURL != "" {
		handler = daemon.NewProxyHandler(BackendURL)
		fmt.Printf("Proxying requests to: %s\n", BackendURL)
	}

	// Create controller
	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
		Version:     Version,
		Logger:      &daemonLogger{},
		Handler:     handler,
	})
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}

	// Handle commands
	switch action {
	case "start":
		return handleStart(controller)
	case "stop":
		return handleStop(controller)
	case "restart", "reload":
		return handleRestart(controller)
	case "proxy":
		return handleProxyStart(controller)
	case "status":
		return handleStatus(controller)
	default:
		fmt.Printf("Muster Daemon v%s\n\n", Version)
		fmt.Println("Usage: muster-daemon <command>")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  start    Start the daemon process")
		fmt.Println("  proxy    Start as HTTP proxy on Unix socket")
		fmt.Println("  stop     Stop the daemon process")
		fmt.Println("  restart  Restart the daemon process")
		fmt.Println("  status   Show daemon status")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  --backend-url <url>    Backend URL for proxy")
		fmt.Println("")
		return nil
	}
}

func handleStart(controller *daemon.Controller) error {
	err := controller.Start()
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	fmt.Println("Daemon started successfully")
	fmt.Printf("PID file: %s\n", "/tmp/muster-daemon.pid")
	fmt.Printf("Socket: %s\n", "/tmp/muster-daemon.sock")

	// Setup signal handlers
	controller.SetupSignalHandlers()

	// Keep process running
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Wait for shutdown signal
	<-sigCh
	fmt.Println("Shutting down daemon...")
	return controller.Stop()
}

func handleProxyStart(controller *daemon.Controller) error {
	socketPath := "/tmp/muster-daemon.sock"

	// Create simple proxy handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","message":"Proxy is working","method":"%s","path":"%s"}`, r.Method, r.URL.Path)
	}

	if err := controller.StartWithSocket(handler); err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}

	fmt.Println("Proxy daemon started successfully")
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Println("\nTest with:")
	fmt.Printf("  curl --unix-socket %s http://localhost/\n", socketPath)

	// Setup signal handlers
	controller.SetupSignalHandlers()

	// Keep process running
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Wait for shutdown signal
	<-sigCh
	fmt.Println("Shutting down proxy...")
	return nil
}

func handleStop(controller *daemon.Controller) error {
	status, err := controller.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	if !status.Running {
		fmt.Println("Daemon is not running")
		return nil
	}

	if err := controller.Stop(); err != nil {
		return fmt.Errorf("stop daemon: %w", err)
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

func handleRestart(controller *daemon.Controller) error {
	fmt.Println("Restarting daemon...")
	return controller.Restart()
}

func handleStatus(controller *daemon.Controller) error {
	status, err := controller.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	fmt.Printf("Daemon Status:\n")
	fmt.Printf("  Running:     %v\n", status.Running)
	fmt.Printf("  PID:         %d\n", status.PID)
	fmt.Printf("  Start time:  %v\n", status.StartTime)
	fmt.Printf("  Socket:      %s\n", "/tmp/muster-daemon.sock")
	fmt.Printf("  Version:     %s\n", status.Version)
	return nil
}

// daemonLogger implements the daemon.Logger interface
type daemonLogger struct{}

func (l *daemonLogger) Info(msg string, args ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, args)
}

func (l *daemonLogger) Warn(msg string, args ...interface{}) {
	fmt.Printf("[WARN] %s %v\n", msg, args)
}

func (l *daemonLogger) Error(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] %s %v\n", msg, args)
}
