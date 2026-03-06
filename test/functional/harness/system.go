package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// System represents a running instance of the muster system for functional testing
type System struct {
	t             *testing.T
	ctx           context.Context
	cancel        context.CancelFunc
	cliPath       string
	mcpPath       string
	daemonPath    string
	tempDir       string
	socketPath    string
	pidFilePath   string
	daemonRunning bool
	daemonCmd     *exec.Cmd
}

// NewSystem creates a new test system instance
func NewSystem(t *testing.T) *System {
	tempDir, err := os.MkdirTemp("", "muster-functional-*")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	sys := &System{
		t:           t,
		ctx:         ctx,
		cancel:      cancel,
		tempDir:     tempDir,
		socketPath:  filepath.Join(tempDir, "muster.sock"),
		pidFilePath: filepath.Join(tempDir, "muster.pid"),
	}

	// Setup cleanup at test end
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return sys
}

// Build builds all binaries
func (s *System) Build() {
	cliBin := filepath.Join(s.tempDir, "muster-cli")
	mcpBin := filepath.Join(s.tempDir, "muster-mcp")
	daemonBin := filepath.Join(s.tempDir, "muster-daemon")

	// Get project root directory (4 levels up from test/functional/harness/")
	_, filename, _, _ := runtime.Caller(0)
	rootDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))

	cliCmd := exec.Command("go", "build", "-o", cliBin, "./cmd/openapi-cli")
	cliCmd.Dir = rootDir
	s.t.Logf("Building CLI in %s: %v", rootDir, cliCmd.Args)
	output, err := cliCmd.CombinedOutput()
	require.NoError(s.t, err, "CLI build failed: %s", output)
	s.cliPath = cliBin

	mcpCmd := exec.Command("go", "build", "-o", mcpBin, "./cmd/mcp-server")
	mcpCmd.Dir = rootDir
	s.t.Logf("Building MCP in %s: %v", rootDir, mcpCmd.Args)
	output, err = mcpCmd.CombinedOutput()
	require.NoError(s.t, err, "MCP build failed: %s", output)
	s.mcpPath = mcpBin

	daemonCmd := exec.Command("go", "build", "-o", daemonBin, "./cmd/daemon")
	daemonCmd.Dir = rootDir
	s.t.Logf("Building daemon in %s: %v", rootDir, daemonCmd.Args)
	output, err = daemonCmd.CombinedOutput()
	require.NoError(s.t, err, "daemon build failed: %s", output)
	s.daemonPath = daemonBin
}

// Start starts the daemon if not running
func (s *System) Start() {
	if s.daemonRunning {
		return
	}

	// Create a fresh context if the previous one was canceled
	if s.ctx.Err() != nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}

	s.t.Logf("Starting daemon on socket: %s, pid file: %s", s.socketPath, s.pidFilePath)
	// Use proxy command to make daemon create a socket
	// IMPORTANT: Flags must come BEFORE the subcommand for flag parsing to work
	s.daemonCmd = exec.CommandContext(s.ctx, s.daemonPath, "--socket-path", s.socketPath, "--pid-file", s.pidFilePath, "proxy")
	if err := s.daemonCmd.Start(); err != nil {
		require.NoError(s.t, err, "failed to start daemon")
	}

	s.daemonRunning = true
	s.t.Logf("Daemon started")

	// Wait for socket to be created (up to 5 seconds)
	s.WaitForReady()
}

// WaitForReady waits up to 5 seconds for the daemon to be ready
func (s *System) WaitForReady() {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			require.Fail(s.t, "Timed out waiting for daemon to creating socket (%s)", s.socketPath)
		case <-ticker.C:
			if _, err := os.Stat(s.socketPath); err == nil {
				// Socket exists, verify it's a valid socket
				if fi, err := os.Stat(s.socketPath); err == nil {
					if fi.Mode()&os.ModeSocket != 0 {
						s.t.Logf("Daemon socket ready: %s", s.socketPath)
						return
					}
				}
			}
		}
	}
}

// Stop stops the daemon and cleans up
func (s *System) Stop() {
	if !s.daemonRunning {
		return
	}

	s.t.Logf("Stopping daemon")

	// Kill the daemon process if it's still running
	if s.daemonCmd != nil && s.daemonCmd.Process != nil {
		s.t.Logf("Sending SIGTERM to daemon process")
		s.daemonCmd.Process.Signal(syscall.SIGTERM)

		// Wait for process to exit (with timeout)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, err := s.daemonCmd.Process.Wait()
			done <- err
		}()

		select {
		case <-done:
			s.t.Logf("Daemon process exited")
		case <-ctx.Done():
			s.t.Logf("Daemon did not exit gracefully, killing")
			s.daemonCmd.Process.Kill()
			s.daemonCmd.Process.Wait()
		}
	}

	s.daemonRunning = false
	s.cancel()

	// Immediately remove socket and PID file
	os.Remove(s.socketPath)
	os.Remove(s.pidFilePath)
}

// CLIPath returns the CLI binary path
func (s *System) CLIPath() string {
	return s.cliPath
}

// MCPPath returns the MCP binary path
func (s *System) MCPPath() string {
	return s.mcpPath
}

// DaemonPath returns the daemon binary path
func (s *System) DaemonPath() string {
	return s.daemonPath
}

// SocketPath returns the daemon socket path
func (s *System) SocketPath() string {
	return s.socketPath
}

// TempDir returns the temp directory path
func (s *System) TempDir() string {
	return s.tempDir
}

// ExecuteCLI runs the CLI binary with arguments
func (s *System) ExecuteCLI(args ...string) (string, error) {
	output, err := exec.CommandContext(s.ctx, s.cliPath, args...).CombinedOutput()
	return string(output), err
}

// ExecuteMCP starts the MCP server with arguments
func (s *System) ExecuteMCP(args ...string) *exec.Cmd {
	return exec.CommandContext(s.ctx, s.mcpPath, args...)
}

// Cleanup removes temp directory
func (s *System) Cleanup() {
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
}

// SetConfigFile sets a custom config file path
func (s *System) SetConfigFile(path string) {
	os.Setenv("MUSTER_CONFIG", path)
}

// SetEnvVar sets an environment variable
func (s *System) SetEnvVar(key, value string) {
	os.Setenv(key, value)
}

// Ready returns true if system is ready for use
func (s *System) Ready() bool {
	// Check if daemon is running
	if !s.daemonRunning {
		return false
	}

	// Check if socket exists
	fi, err := os.Stat(s.socketPath)
	if err != nil {
		return false
	}

	// Verify it's a Unix socket
	return fi.Mode()&os.ModeSocket != 0
}

// IsRunning returns true if daemon is running
func (s *System) IsRunning() bool {
	return s.daemonRunning
}

// State returns the current system state as a string
func (s *System) State() State {
	if !s.Ready() {
		return StateStopped
	}
	if !s.IsRunning() {
		return StateInitializing
	}
	return StateReady
}

// ExecuteCommand executes a command and returns output and error
func (s *System) ExecuteCommand(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(s.ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// MakeRequest makes a request to the system and returns the result
func (s *System) MakeRequest(url string) (string, error) {
	output, err := s.ExecuteCLI("request", url)
	return output, err
}

// NewRequest creates a new request context
func (s *System) NewRequest() *Request {
	return &Request{
		system: s,
	}
}

// HTTPClient returns an HTTP client for the system
func (s *System) HTTPClient() *HTTPClient {
	return &HTTPClient{
		system: s,
	}
}

// Request represents a test request
type Request struct {
	system *System
	method string
	url    string
	body   []byte
}

// Method sets the HTTP method
func (r *Request) Method(method string) *Request {
	r.method = method
	return r
}

// URL sets the URL
func (r *Request) URL(url string) *Request {
	r.url = url
	return r
}

// Body sets the request body
func (r *Request) Body(body []byte) *Request {
	r.body = body
	return r
}

// HTTPClient represents an HTTP client for testing
type HTTPClient struct {
	system *System
}

// Do executes an HTTP request and returns the response
func (c *HTTPClient) Do(req *Request) (*Response, error) {
	method := req.method
	if method == "" {
		method = "GET"
	}
	args := []string{"request", method, req.url}
	if len(req.body) > 0 {
		args = append(args, "--body", string(req.body))
	}

	output, err := c.system.ExecuteCLI(args...)
	return &Response{
		data:      []byte(output),
		status:    200,
		fromCache: false,
	}, err
}

// Response represents a test response
type Response struct {
	data      []byte
	status    int
	fromCache bool
}

// Data returns the response data
func (r *Response) Data() []byte {
	return r.data
}

// String returns the response data as a string
func (r *Response) String() string {
	return string(r.data)
}

// Status returns the HTTP status code
func (r *Response) Status() int {
	return r.status
}

// FromCache returns true if response came from cache
func (r *Response) FromCache() bool {
	return r.fromCache
}

// NewTest creates a new test context
func NewTest(t *testing.T) *TestContext {
	return &TestContext{
		t:       t,
		started: time.Now(),
	}
}

// TestContext provides test context utilities
type TestContext struct {
	t       *testing.T
	started time.Time
}

// T returns the testing.T instance
func (tc *TestContext) T() *testing.T {
	return tc.t
}

// Elapsed returns the elapsed time since test start
func (tc *TestContext) Elapsed() time.Duration {
	return time.Since(tc.started)
}

// RequireFatal fails the test immediately if condition is not met
func (tc *TestContext) RequireFatal(condition bool, msg string, args ...interface{}) {
	if !condition {
		tc.t.Fatalf(msg, args...)
	}
}

// Logf logs a formatted message
func (tc *TestContext) Logf(msg string, args ...interface{}) {
	tc.t.Logf(msg, args...)
}

// RunTest runs a test function with timing
func (tc *TestContext) RunTest(name string, fn func(*TestContext)) {
	start := time.Now()
	defer func() {
		tc.t.Logf("Test %s completed in %v", name, time.Since(start))
	}()

	tc.t.Run(name, func(t *testing.T) {
		ctx := &TestContext{t: t}
		fn(ctx)
	})
}

// NewLogger creates a new test logger
func NewLogger(t *testing.T) *Logger {
	return &Logger{t: t}
}

// Logger provides test logging utilities
type Logger struct {
	t *testing.T
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.t.Logf("[DEBUG] "+msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.t.Logf("[INFO] "+msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.t.Logf("[WARN] "+msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.t.Logf("[ERROR] "+msg, args...)
}

// StartTestSuite initializes the test suite
func StartTestSuite(t *testing.T, suiteName string) *TestSuite {
	return &TestSuite{
		t:         t,
		name:      suiteName,
		startTime: time.Now(),
	}
}

// TestSuite represents a test suite
type TestSuite struct {
	t         *testing.T
	name      string
	startTime time.Time
	tests     []string
}

// AddTest adds a test to the suite
func (ts *TestSuite) AddTest(name string) {
	ts.tests = append(ts.tests, name)
}

// Complete marks the suite as complete
func (ts *TestSuite) Complete() {
	elapsed := time.Since(ts.startTime)
	ts.t.Logf("Test suite %s completed in %v (%d tests)", ts.name, elapsed, len(ts.tests))
}

// NewSystemBuilder creates a new system builder
func NewSystemBuilder(t *testing.T) *SystemBuilder {
	return &SystemBuilder{
		t:       t,
		config:  &systemConfig{},
		enabled: true,
	}
}

// SystemBuilder builds test systems
type SystemBuilder struct {
	t       *testing.T
	config  *systemConfig
	enabled bool
}

type systemConfig struct {
	cliPath    string
	mcpPath    string
	daemonPath string
	socketPath string
	tempDir    string
	env        map[string]string
}

// Enable enables the builder
func (sb *SystemBuilder) Enable() *SystemBuilder {
	sb.enabled = true
	return sb
}

// Disable disables the builder
func (sb *SystemBuilder) Disable() *SystemBuilder {
	sb.enabled = false
	return sb
}

// WithCLIPath sets the CLI path
func (sb *SystemBuilder) WithCLIPath(path string) *SystemBuilder {
	sb.config.cliPath = path
	return sb
}

// WithMCPPath sets the MCP path
func (sb *SystemBuilder) WithMCPPath(path string) *SystemBuilder {
	sb.config.mcpPath = path
	return sb
}

// WithDaemonPath sets the daemon path
func (sb *SystemBuilder) WithDaemonPath(path string) *SystemBuilder {
	sb.config.daemonPath = path
	return sb
}

// WithSocketPath sets the socket path
func (sb *SystemBuilder) WithSocketPath(path string) *SystemBuilder {
	sb.config.socketPath = path
	return sb
}

// WithTempDir sets the temp directory
func (sb *SystemBuilder) WithTempDir(dir string) *SystemBuilder {
	sb.config.tempDir = dir
	return sb
}

// WithEnv sets an environment variable
func (sb *SystemBuilder) WithEnv(key, value string) *SystemBuilder {
	if sb.config.env == nil {
		sb.config.env = make(map[string]string)
	}
	sb.config.env[key] = value
	return sb
}

// Build constructs the system
func (sb *SystemBuilder) Build() *System {
	if !sb.enabled {
		return nil
	}

	system := NewSystem(sb.t)
	system.cliPath = sb.config.cliPath
	system.mcpPath = sb.config.mcpPath
	system.daemonPath = sb.config.daemonPath
	system.socketPath = sb.config.socketPath
	system.tempDir = sb.config.tempDir

	return system
}

// NewTestRunner creates a new test runner
func NewTestRunner(t *testing.T) *TestRunner {
	return &TestRunner{t: t}
}

// TestRunner runs functional tests
type TestRunner struct {
	t        *testing.T
	system   *System
	parallel int
}

// WithSystem sets the test system
func (tr *TestRunner) WithSystem(system *System) *TestRunner {
	tr.system = system
	return tr
}

// WithParallelism sets the parallelism level
func (tr *TestRunner) WithParallelism(n int) *TestRunner {
	tr.parallel = n
	return tr
}

// Run runs tests in parallel
func (tr *TestRunner) Run(tests []func()) {
	if tr.parallel > 1 {
		tr.runParallel(tests)
	} else {
		tr.runSequential(tests)
	}
}

func (tr *TestRunner) runParallel(tests []func()) {
	for _, test := range tests {
		tr.t.Cleanup(func() {
			test()
		})
	}
}

func (tr *TestRunner) runSequential(tests []func()) {
	for _, test := range tests {
		test()
	}
}

// NewTestController creates a test controller
func NewTestController(t *testing.T) *TestController {
	return &TestController{
		t:       t,
		started: time.Now(),
	}
}

// TestController controls test execution
type TestController struct {
	t       *testing.T
	started time.Time
	tests   []string
	passed  int
	failed  int
}

// RecordPass records a passing test
func (tc *TestController) RecordPass(name string) {
	tc.tests = append(tc.tests, name)
	tc.passed++
}

// RecordFail records a failing test
func (tc *TestController) RecordFail(name string, err error) {
	tc.tests = append(tc.tests, name)
	tc.failed++
	tc.t.Logf("Failed: %s: %v", name, err)
}

// Summary returns the test summary
func (tc *TestController) Summary() string {
	elapsed := time.Since(tc.started)
	return fmt.Sprintf(" passed=%d failed=%d elapsed=%v", tc.passed, tc.failed, elapsed)
}
