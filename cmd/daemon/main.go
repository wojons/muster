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

	"github.com/wojons/muster/internal/monitoring"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/daemon"
)

// defaultSocketPath is the default Unix domain socket path used when
// --socket-path is not given. It is also the base for the default PID
// file path (socket path + ".pid"), which is resolved at help time so
// --help never shows a literal "<socket-path>" template.
const defaultSocketPath = "/tmp/openapi-cli.sock"

// knownCommands are the subcommands recognized anywhere in the argument
// list (before or after flags). Package-level so both the action scanner
// in run() and the restart child-args rewrite use the same set.
var knownCommands = map[string]bool{
	"start": true, "stop": true, "restart": true, "reload": true,
	"proxy": true, "status": true,
}

// Command-line flags
var (
	backendURL      string
	socketPath      string
	pidFile         string
	wasmPlugin      string
	versionFlag     bool
	foreground      bool
	backendAuthType string
	backendToken    string
	clientToken     string
	metricsAddr     string
)

// metricsRecorder is the process-wide recorder for the five muster_* metric
// families (docs/observability-spec.md §3.1). It owns the ONE explicit
// prometheus registry this process serves; the proxy handler writes into the
// same recorder, so recorded traffic and the /metrics endpoint can never
// diverge onto two registries (spec §4.2). Nil until run() builds it, which
// keeps the help/status/stop paths free of any listener.
var metricsRecorder *monitoring.MetricRecorder

// metricsTenantID returns the tenant_id label value: MUSTER_TENANT_ID when the
// operator configured one, otherwise monitoring.DefaultTenantID. The daemon is
// single-tenant today; the label exists from day one so multi-tenancy does not
// force a metric rename.
func metricsTenantID() string {
	if tenant := strings.TrimSpace(os.Getenv("MUSTER_TENANT_ID")); tenant != "" {
		return tenant
	}
	return monitoring.DefaultTenantID
}

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
	flagSet.StringVar(&backendAuthType, "backend-auth-type", "bearer", "Credential type for the injected upstream credential: bearer (Authorization: Bearer) or api-key (X-API-Key)")
	flagSet.StringVar(&backendToken, "backend-token-file", "", "Path to a file holding the upstream credential the daemon injects on every proxied request (env: MUSTER_BACKEND_TOKEN_FILE)")
	flagSet.StringVar(&clientToken, "client-token-file", "", "Path to a file holding the token socket clients must present — required when --backend-token-file is set (env: MUSTER_DAEMON_CLIENT_TOKEN_FILE)")
	flagSet.StringVar(&metricsAddr, "metrics-addr", monitoring.DefaultMetricsAddr, "Loopback address for the Prometheus /metrics listener, which serves only /metrics")
	// The raw flag package's default Usage only lists global flags, hiding
	// the subcommand surface. Install a custom usage func that documents
	// the six supported subcommands (start/stop/status/restart/reload/proxy).
	flagSet.Usage = func() {
		out := flagSet.Output()
		_, _ = fmt.Fprintf(out, "Usage: %s [start|stop|status|restart|reload|proxy] [flags]\n\n", os.Args[0])
		_, _ = fmt.Fprintln(out, "Commands:")
		for _, cmd := range []string{"start", "stop", "status", "restart", "reload", "proxy"} {
			_, _ = fmt.Fprintf(out, "  %s\n", cmd)
		}
		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintln(out, "Flags:")
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

	// Reject a non-loopback metrics address up front: the listener is the one
	// network-reachable surface the daemon gains, and spec §4.2 binds it to
	// loopback only. A busy port stays a warning (see startMetricsListener);
	// a wrong address is an operator error and fails here, loudly.
	if err := monitoring.ValidateLoopbackAddr(metricsAddr); err != nil {
		return err
	}

	// Create proxy handler with backend URL
	var handler http.Handler
	if backendURL != "" {
		// MUSTER-001: the daemon can hold the UPSTREAM credential itself, so
		// socket clients need neither the upstream key nor its base URL. The
		// credential file is read once at startup and never logged.
		if backendToken == "" {
			backendToken = os.Getenv("MUSTER_BACKEND_TOKEN_FILE")
		}
		if clientToken == "" {
			clientToken = os.Getenv("MUSTER_DAEMON_CLIENT_TOKEN_FILE")
		}
		if backendToken != "" {
			secret, err := readCredentialFile(backendToken, "backend credential")
			if err != nil {
				return err
			}
			if clientToken == "" {
				return fmt.Errorf("--backend-token-file requires --client-token-file: a daemon that injects an upstream credential must gate its own socket, otherwise every local caller is silently authenticated as the daemon's upstream identity")
			}
			headerName, headerValue := auth.HeaderFor(backendAuthType, secret)
			handler = daemon.NewProxyHandlerWithAuth(backendURL, headerName, headerValue)
			fmt.Printf("Proxying requests to: %s (injecting %s)\n", backendURL, headerName)
		} else {
			handler = daemon.NewProxyHandler(backendURL)
			fmt.Printf("Proxying requests to: %s\n", backendURL)
		}
	}

	// Observability (OBS-MUSTER-1, docs/observability-spec.md §4.2): ONE
	// explicit prometheus registry per daemon process. The recorder is built
	// before the handlers are used so the proxy handler records into exactly
	// the registry the /metrics listener serves — never the default registry.
	// Building it here is side-effect free (no listener), so help/status/stop
	// stay listener-free.
	metricsRecorder = monitoring.NewMetricRecorder(metricsTenantID(), nil)
	if proxyHandler, ok := handler.(*daemon.ProxyHandler); ok {
		proxyHandler.SetMetrics(metricsRecorder)
	}

	// Client tokens required on the socket (only meaningful with a handler that
	// injects an upstream credential; without injection the historical
	// unauthenticated socket is unchanged).
	var authTokens []string
	if clientToken != "" {
		token, err := readCredentialFile(clientToken, "client token")
		if err != nil {
			return err
		}
		authTokens = append(authTokens, token)
	}

	// Create controller
	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
		Version:     version.Get().Short(),
		Logger:      &daemonLogger{},
		Handler:     handler,
		AuthTokens:  authTokens,
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
		fmt.Println("  --metrics-addr <addr>   Loopback address for the /metrics listener (default: " + monitoring.DefaultMetricsAddr + ")")
		fmt.Println("")
		return nil
	}
}

// readCredentialFile reads a one-line credential from path. Surrounding
// whitespace is trimmed (a trailing newline from `echo` is not part of the
// secret); an empty file or one holding several lines is refused, so a
// multi-entry file is never silently truncated into a wrong credential.
func readCredentialFile(path, what string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s file %q: %w", what, path, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("%s file %q is empty", what, path)
	}
	if strings.ContainsAny(secret, "\r\n") {
		return "", fmt.Errorf("%s file %q must contain exactly one line", what, path)
	}
	return secret, nil
}

// startMetricsListener binds the loopback metrics listener and serves the five
// muster_* families of docs/observability-spec.md §3.1 on it.
//
// A bind failure is a WARNING, not fatal: the daemon's real surface is the Unix
// socket, and a metrics port already taken (a second daemon, a leftover probe)
// must not take that surface down with it. The listener serves only /metrics —
// no proxied path is reachable through it — so exposing it on loopback cannot
// widen the proxy's attack surface (spec §4.2).
func startMetricsListener() *monitoring.MusterMetricsServer {
	if metricsRecorder == nil {
		return nil
	}
	server := monitoring.NewMusterMetricsServer(metricsAddr, metricsRecorder.Muster())
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] metrics listener not started on %s: %v\n", metricsAddr, err)
		return nil
	}
	fmt.Printf("Metrics: http://%s/metrics\n", server.Addr())
	return server
}

// shutdownMetricsListener stops the metrics listener. It does not touch
// daemon_up: the caller marks the daemon down first so the last scrape reports
// the truth.
func shutdownMetricsListener(server *monitoring.MusterMetricsServer) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] metrics listener shutdown: %v\n", err)
	}
}

// markDaemonUp publishes muster_daemon_up = 1 once the socket answers.
func markDaemonUp() {
	if metricsRecorder != nil {
		metricsRecorder.Muster().MarkUp()
	}
}

// markDaemonDown publishes muster_daemon_up = 0 on the shutdown path, before
// the listener is closed: 1 for a dead process is the one lie a /metrics
// endpoint must never tell.
func markDaemonDown() {
	if metricsRecorder != nil {
		metricsRecorder.Muster().MarkDown()
	}
}

// handleStart ensures the daemon comes up serving the socket.
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

	// The socket answers, so the daemon is up: publish that and start the
	// loopback scrape target (OBS-MUSTER-1).
	markDaemonUp()
	metricsServer := startMetricsListener()

	fmt.Println("Daemon started successfully")
	fmt.Printf("PID file: %s\n", pidFile)
	fmt.Printf("Socket: %s\n", socketPath)

	// Setup signal handlers
	controller.SetupSignalHandlers()

	// Keep process running
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Wait for shutdown signal
	<-sigCh
	fmt.Println("Shutting down daemon...")

	// Report down BEFORE the listener closes, or the final scrape leaves a
	// stale muster_daemon_up = 1 behind.
	markDaemonDown()

	// Remove PID file before shutdown
	_ = os.Remove(pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := controller.Shutdown(ctx)
	shutdownMetricsListener(metricsServer)
	return shutdownErr
}

// spawnDaemonChild re-executes the current binary as a detached child
// (new session, stdio to /dev/null) with MUSTER_DAEMON_CHILD=1 set, then
// waits until the daemon's socket and PID file are live before returning.
// The child runs the same `start` code path but skips this function thanks
// to the env marker, so it stays alive serving the socket while the parent
// returns to the shell.
func spawnDaemonChild(controller *daemon.Controller) error {
	return spawnDaemonChildWithArgs(controller, os.Args[1:])
}

// spawnDaemonChildWithArgs is spawnDaemonChild with an explicit child
// command line, used by `restart` to bring a fresh daemon up with the
// subcommand rewritten to "start" (see startChildArgs).
func spawnDaemonChildWithArgs(controller *daemon.Controller, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer func() { _ = devnull.Close() }()

	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "MUSTER_DAEMON_CHILD=1")
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = daemonProcAttr()

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
	return nil
}

// startChildArgs returns the child command-line args for a daemon spawned
// by `restart`: the invoking command line with the subcommand token
// rewritten to "start". The re-exec'd child then runs the `start` path —
// serving the socket in the foreground because MUSTER_DAEMON_CHILD=1 skips
// the daemonize branch — instead of recursing into restart's
// stop-then-spawn logic. If no subcommand token is present (defensive),
// "start" is prepended.
func startChildArgs() []string {
	args := os.Args[1:]
	for i, arg := range args {
		if knownCommands[arg] {
			rewritten := make([]string, len(args))
			copy(rewritten, args)
			rewritten[i] = "start"
			return rewritten
		}
	}
	return append([]string{"start"}, args...)
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

	// Socket answers: publish daemon_up = 1 and expose /metrics on loopback.
	markDaemonUp()
	metricsServer := startMetricsListener()

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

	// Publish down before the listener closes (see handleStart).
	markDaemonDown()

	// Remove PID file before shutdown
	_ = os.Remove(pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := controller.Shutdown(ctx)
	shutdownMetricsListener(metricsServer)
	return shutdownErr
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

// spawnRestartChild is the spawn entry point `restart` uses to bring a
// fresh daemon up after stopping the old one. It is a package-level
// variable so unit tests can stub the re-exec side without spawning the
// test binary as a daemon child.
var spawnRestartChild = spawnDaemonChildWithArgs

// handleRestart stops the running daemon (if any) and spawns a fresh one
// via the same re-exec pattern as `start`. Restart runs in the invoking
// process, which CAN re-spawn the daemon — the previous behavior left the
// advertised first-class command stopping the daemon without bringing it
// back, so a subsequent `status` showed Running: false (DF-011).
func handleRestart(controller *daemon.Controller) error {
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	// Stop the running daemon first (PID-file based, same as `stop`) so the
	// socket is released before the fresh daemon binds it.
	if _, err := os.Stat(pidFile); err == nil {
		pid, err := readPIDFile(pidFile)
		if err != nil || !isProcessRunning(pid) {
			// Stale PID file — clean up and start fresh below.
			_ = os.Remove(pidFile)
			_ = os.Remove(socketPath)
			fmt.Println("Daemon is not running; starting fresh")
		} else {
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
		}
	} else {
		fmt.Println("Daemon is not running; starting fresh")
	}

	// Spawn a fresh daemon child with the subcommand rewritten to "start"
	// (startChildArgs), so the child serves the socket in the foreground
	// (MUSTER_DAEMON_CHILD=1) instead of recursing into this function.
	return spawnRestartChild(controller, startChildArgs())
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
