package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/daemon"
)

// defaultSocketPath is the default Unix domain socket path used when
// --socket-path is not given. It is also the base for the default PID
// file path (socket path + ".pid"), which is resolved at help time so
// --help never shows a literal "<socket-path>" template.
const defaultSocketPath = "/tmp/openapi-cli.sock"

// Command-line flags
var (
	backendURL  string
	socketPath  string
	pidFile     string
	wasmPlugin  string
	versionFlag bool
	foreground  bool
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Go's flag package stops parsing at the first non-flag argument.
	// To support both "proxy --backend-url ..." and "--backend-url ... proxy",
	// we scan for a known subcommand among the args.
	allArgs := os.Args[1:]
	action := ""
	var flagArgs []string

	knownCommands := map[string]bool{
		"start": true, "stop": true, "restart": true, "reload": true,
		"proxy": true, "status": true,
	}

	// Find the first known subcommand; everything else is treated as flags
	for i, arg := range allArgs {
		if knownCommands[arg] {
			action = arg
			// Everything before + after is flags
			flagArgs = append(flagArgs, allArgs[:i]...)
			flagArgs = append(flagArgs, allArgs[i+1:]...)
			break
		}
	}
	if action == "" {
		flagArgs = allArgs
	}

	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.StringVar(&backendURL, "backend-url", "", "Backend URL for proxy mode")
	flagSet.StringVar(&socketPath, "socket-path", defaultSocketPath, "Path for Unix domain socket")
	flagSet.StringVar(&pidFile, "pid-file", "", fmt.Sprintf("Path for PID file (default: %s.pid)", defaultSocketPath))
	flagSet.StringVar(&wasmPlugin, "wasm-plugin", "", "Path to WASM plugin for response transformation")
	flagSet.BoolVar(&versionFlag, "version", false, "Print version information and exit")
	flagSet.BoolVar(&foreground, "foreground", false, "Run `start` in the foreground instead of daemonizing (default: daemonize)")
	// The raw flag package's default Usage only lists global flags, hiding
	// the subcommand surface. Install a custom usage func that documents
	// the six supported subcommands (start/stop/status/restart/reload/proxy).
	flagSet.Usage = func() {
		out := flagSet.Output()
		fmt.Fprintf(out, "Usage: %s [start|stop|status|restart|reload|proxy] [flags]\n\n", os.Args[0])
		fmt.Fprintln(out, "Commands:")
		for _, cmd := range []string{"start", "stop", "status", "restart", "reload", "proxy"} {
			fmt.Fprintf(out, "  %s\n", cmd)
		}
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Flags:")
		flagSet.PrintDefaults()
	}
	err := flagSet.Parse(flagArgs)
	if err != nil {
		// -h/--help is a legitimate request: the flag package has already
		// printed usage. Treat it as success (exit 0), not an error.
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if versionFlag {
		fmt.Println(version.Get().Short())
		return nil
	}

	// Default PID file to socket path + ".pid" if not specified
	if pidFile == "" {
		pidFile = socketPath + ".pid"
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
		Version:     version.Get().Short(),
		Logger:      &daemonLogger{},
		Handler:     handler,
	})
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}

	// Handle commands
	switch action {
	case "start":
		return handleStart(controller, handler)
	case "stop":
		return handleStop(controller)
	case "restart", "reload":
		return handleRestart(controller)
	case "proxy":
		return handleProxyStart(controller, handler)
	case "status":
		return handleStatus(controller)
	default:
		fmt.Printf("Muster Daemon v%s\n\n", version.Get().Short())
		fmt.Println("Usage: openapi-daemon [start|stop|status|restart|reload|proxy] [flags]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  start    Start the daemon process")
		fmt.Println("  stop     Stop the daemon process")
		fmt.Println("  status   Show daemon status")
		fmt.Println("  restart  Restart the daemon process")
		fmt.Println("  reload   Reload the daemon process")
		fmt.Println("  proxy    Start as HTTP proxy on Unix socket")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  --backend-url <url>     Backend URL for proxy")
		fmt.Println("  --socket-path <path>    Path for Unix domain socket (default: /tmp/muster-daemon.sock)")
		fmt.Println("")
		return nil
	}
}

func handleStart(controller *daemon.Controller, handler http.Handler) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Daemonize by default: `start` from a terminal spawns a detached child
	// (new session, stdio to /dev/null) that runs the same start path with
	// MUSTER_DAEMON_CHILD=1, waits until the daemon socket + PID file are
	// live, then returns to the shell. Previously this command blocked in
	// the foreground forever, making the documented start->status/stop
	// control workflow unreachable from scripts. Pass --foreground (or set
	// MUSTER_DAEMON_CHILD=1) to run in the foreground instead.
	if os.Getenv("MUSTER_DAEMON_CHILD") != "1" && !foreground {
		return spawnDaemonChild(controller)
	}

	// Start the controller with the socket handler so the Unix socket is
	// actually created and endpoints are served. With --backend-url the
	// proxy handler is served (the same one `proxy` mode uses), composed
	// with the local /health endpoints; without it, the canned daemon
	// handler (root info + /health) is served as before.
	// (Controller.Start() alone only flips the running flag — it does not
	// listen on the socket.)
	err := controller.StartWithSocket(startModeHandler(handler))
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Write PID file so status/stop can find and control this process.
	if err := writePIDFile(pidFile, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
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

	// Remove PID file before shutdown
	_ = os.Remove(pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return controller.Shutdown(ctx)
}

// spawnDaemonChild re-executes the current binary as a detached child
// (new session, stdio to /dev/null) with MUSTER_DAEMON_CHILD=1 set, then
// waits until the daemon's socket and PID file are live before returning.
// The child runs the same `start` code path but skips this function thanks
// to the env marker, so it stays alive serving the socket while the parent
// returns to the shell.
func spawnDaemonChild(controller *daemon.Controller) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devnull.Close()

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "MUSTER_DAEMON_CHILD=1")
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon child: %w", err)
	}
	// Detach: the child is no longer our job; it reports readiness by
	// creating the socket and PID file.
	_ = cmd.Process.Release()

	if err := waitForDaemonReady(socketPath, pidFile, 5*time.Second); err != nil {
		return err
	}

	fmt.Println("Daemon started successfully")
	fmt.Printf("PID file: %s\n", pidFile)
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("Token file: %s\n", controller.GetTokenPath())
	return nil
}

// waitForDaemonReady polls until the daemon socket file exists AND the PID
// file names a live process, or the timeout elapses.
func waitForDaemonReady(sockPath, pidPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			if pid, err := readPIDFile(pidPath); err == nil && isProcessRunning(pid) {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready within %s (socket %s / PID file %s missing)", timeout, sockPath, pidPath)
}

// createDaemonHandler returns the default HTTP handler for the daemon,
// exposing the root info endpoint plus /health endpoints.
func createDaemonHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","message":"Muster daemon is running"}`)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy"}`)
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"alive","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ready","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})
	return mux
}

// startModeHandler returns the handler `start` mode serves on its Unix
// socket. When --backend-url was given, run() builds the proxy handler and
// passes it here — the same handler `proxy` mode uses — so the daemonized
// socket serves real backend data instead of the canned status message.
// The proxy handler is composed with the daemon's local /health endpoints
// (the pkg/daemon proxy handler proxies every path, /health included), so
// health checks keep working even when the backend is down. Without a
// backend URL, the canned daemon handler (root info + /health) is served.
func startModeHandler(proxy http.Handler) http.Handler {
	if proxy == nil {
		return createDaemonHandler()
	}
	health := createDaemonHandler()
	mux := http.NewServeMux()
	mux.Handle("/health", health)
	mux.Handle("/health/", health)
	mux.Handle("/", proxy)
	return mux
}

func handleProxyStart(controller *daemon.Controller, handler http.Handler) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Use the socket path from command-line flags
	if socketPath == "" {
		socketPath = "/tmp/muster-daemon.sock"
	}

	if handler == nil {
		return fmt.Errorf("handler is required; use --backend-url to specify the backend URL")
	}

	if err := controller.StartWithSocket(handler); err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}

	// Write PID file so status/stop can find and control this process.
	if err := writePIDFile(pidFile, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
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

	// Remove PID file before shutdown
	_ = os.Remove(pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return controller.Shutdown(ctx)
}

func handleStop(controller *daemon.Controller) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Prefer PID-file based stop when available.
	if _, err := os.Stat(pidFile); err == nil {
		pid, err := readPIDFile(pidFile)
		if err != nil || !isProcessRunning(pid) {
			// Stale PID file / not running.
			_ = os.Remove(pidFile)
			_ = os.Remove(socketPath)
			fmt.Println("Daemon is not running")
			return nil
		}

		if err := terminateProcess(pid); err != nil {
			return fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !isProcessRunning(pid) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if isProcessRunning(pid) {
			return fmt.Errorf("daemon (PID %d) did not exit after SIGTERM", pid)
		}

		_ = os.Remove(pidFile)
		_ = os.Remove(socketPath)
		fmt.Println("Daemon stopped successfully")
		return nil
	}

	// Fall back to in-process Controller when no PID file exists.
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

	// If a PID file exists and the process is running, stop it first.
	if _, err := os.Stat(pidFile); err == nil {
		pid, err := readPIDFile(pidFile)
		if err == nil && isProcessRunning(pid) {
			if err := terminateProcess(pid); err != nil {
				return fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if !isProcessRunning(pid) {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			_ = os.Remove(pidFile)
			_ = os.Remove(socketPath)
			fmt.Println("Daemon stopped; restart must be initiated externally (this process cannot re-spawn itself)")
			return nil
		}
		// Stale PID file — clean up
		_ = os.Remove(pidFile)
	}

	fmt.Println("Restarting daemon...")
	return controller.Restart()
}

func handleStatus(controller *daemon.Controller) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Prefer PID-file based status when available.
	if _, err := os.Stat(pidFile); err == nil {
		pid, err := readPIDFile(pidFile)
		if err != nil {
			fmt.Println("Daemon Status:")
			fmt.Println("  Running:     false")
			fmt.Printf("  Socket:      %s\n", socketPath)
			return nil
		}
		if isProcessRunning(pid) {
			fmt.Println("Daemon Status:")
			fmt.Printf("  Running:     true\n")
			fmt.Printf("  PID:         %d\n", pid)
			fmt.Printf("  Start time:  %s\n", processStartTime(pid))
			fmt.Printf("  Socket:      %s\n", socketPath)
			fmt.Printf("  Version:     %s\n", version.Get().Short())
			return nil
		}
		fmt.Println("Daemon Status:")
		fmt.Println("  Running:     false")
		fmt.Printf("  Socket:      %s\n", socketPath)
		return nil
	}

	// Fall back to in-process Controller when no PID file exists.
	status, err := controller.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	fmt.Printf("Daemon Status:\n")
	fmt.Printf("  Running:     %v\n", status.Running)
	fmt.Printf("  PID:         %d\n", status.PID)
	fmt.Printf("  Start time:  %v\n", status.StartTime)
	fmt.Printf("  Socket:      %s\n", socketPath)
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
