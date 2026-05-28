package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wojons/muster/pkg/daemon"
)

// Build-time variables
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Command-line flags
var (
	backendURL string
	socketPath string
	pidFile    string
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse command-line flags
	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.StringVar(&backendURL, "backend-url", "", "Backend URL for proxy mode")
	flagSet.StringVar(&socketPath, "socket-path", "/tmp/muster-daemon.sock", "Path for Unix domain socket")
	flagSet.StringVar(&pidFile, "pid-file", "", "Path for PID file (default: <socket-path>.pid")
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	// Get command from remaining arguments
	args := flagSet.Args()

	// Default PID file to socket path + ".pid" if not specified
	if pidFile == "" {
		pidFile = socketPath + ".pid"
	}

	var action string
	if len(args) > 0 {
		action = args[0]
	}

	// Create proxy handler with backend URL
	var handler http.Handler
	if backendURL != "" {
		handler = daemon.NewProxyHandler(backendURL)
		fmt.Printf("Proxying requests to: %s\n", backendURL)
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
		fmt.Println("  --backend-url <url>     Backend URL for proxy")
		fmt.Println("  --socket-path <path>    Path for Unix domain socket (default: /tmp/muster-daemon.sock)")
		fmt.Println("")
		return nil
	}
}

func handleStart(controller *daemon.Controller) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	err := controller.Start()
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	fmt.Println("Daemon started successfully")
	fmt.Printf("PID file: %s\n", pidFile)
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("Token file: %s\n", controller.GetTokenPath())

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
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Use the socket path from command-line flags
	if socketPath == "" {
		socketPath = "/tmp/muster-daemon.sock"
	}

	// Create simple proxy handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","message":"Proxy is working","method":"%s","path":"%s"}`, r.Method, r.URL.Path)
	}

	if err := controller.StartWithSocket(handler); err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}

	fmt.Println("Proxy daemon started successfully")
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("Token file: %s\n", controller.GetTokenPath())
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
	if controller == nil {
		return fmt.Errorf("controller is required")
	}
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
	if controller == nil {
		return fmt.Errorf("controller is required")
	}
	fmt.Println("Restarting daemon...")
	return controller.Restart()
}

func handleStatus(controller *daemon.Controller) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}
	status, err := controller.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	fmt.Printf("Daemon Status:\n")
	fmt.Printf("  Running:     %v\n", status.Running)
	fmt.Printf("  PID:         %d\n", status.PID)
	fmt.Printf("  Start time:  %v\n", status.StartTime)
	fmt.Printf("  Socket:      %s\n", "/tmp/muster-daemon.sock")
	fmt.Printf("  Token file:  %s\n", controller.GetTokenPath())
	fmt.Printf("  Version:     %s\n", status.Version)
	return nil
}

// daemonLogger implements the daemon.Logger interface
type daemonLogger struct{}

func (l *daemonLogger) Info(msg string, args ...interface{}) {
	if len(args) == 0 {
		fmt.Printf("[INFO] %s\n", msg)
	} else if len(args)%2 == 0 {
		// Structured logging: key-value pairs
		pairs := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			key, ok := args[i].(string)
			if ok {
				pairs = append(pairs, fmt.Sprintf("%s=%v", key, args[i+1]))
			}
		}
		if len(pairs) > 0 {
			fmt.Printf("[INFO] %s %s\n", msg, strings.Join(pairs, " "))
		} else {
			fmt.Printf("[INFO] %s %v\n", msg, args)
		}
	} else {
		// Printf-style formatting
		fmt.Printf("[INFO] %s\n", fmt.Sprintf(msg, args...))
	}
}

func (l *daemonLogger) Warn(msg string, args ...interface{}) {
	if len(args) == 0 {
		fmt.Printf("[WARN] %s\n", msg)
	} else if len(args)%2 == 0 {
		// Structured logging: key-value pairs
		pairs := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			key, ok := args[i].(string)
			if ok {
				pairs = append(pairs, fmt.Sprintf("%s=%v", key, args[i+1]))
			}
		}
		if len(pairs) > 0 {
			fmt.Printf("[WARN] %s %s\n", msg, strings.Join(pairs, " "))
		} else {
			fmt.Printf("[WARN] %s %v\n", msg, args)
		}
	} else {
		// Printf-style formatting
		fmt.Printf("[WARN] %s\n", fmt.Sprintf(msg, args...))
	}
}

func (l *daemonLogger) Error(msg string, args ...interface{}) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", msg)
	} else if len(args)%2 == 0 {
		// Structured logging: key-value pairs
		pairs := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			key, ok := args[i].(string)
			if ok {
				pairs = append(pairs, fmt.Sprintf("%s=%v", key, args[i+1]))
			}
		}
		if len(pairs) > 0 {
			fmt.Fprintf(os.Stderr, "[ERROR] %s %s\n", msg, strings.Join(pairs, " "))
		} else {
			fmt.Fprintf(os.Stderr, "[ERROR] %s %v\n", msg, args)
		}
	} else {
		// Printf-style formatting
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", fmt.Sprintf(msg, args...))
	}
}
