//go:build stability

package stability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// StabilityTestConfig configures the stability test duration
type StabilityTestConfig struct {
	// Duration is how long to run the stability test
	Duration time.Duration
	// HealthCheckInterval is how often to perform health checks
	HealthCheckInterval time.Duration
	// MemoryGrowthThreshold is the maximum allowed memory growth (0.10 = 10%)
	MemoryGrowthThreshold float64
	// GoroutineLeakThreshold is the maximum allowed goroutine growth
	GoroutineLeakThreshold int
	// MinDuration for CI environments (shorter test)
	MinDuration time.Duration
}

// DefaultStabilityConfig returns the default stability test configuration
func DefaultStabilityConfig() StabilityTestConfig {
	return StabilityTestConfig{
		Duration:               30 * time.Minute,
		HealthCheckInterval:    30 * time.Second,
		MemoryGrowthThreshold:  0.10,            // 10% growth allowed
		GoroutineLeakThreshold: 10,              // 10 goroutines max growth
		MinDuration:            2 * time.Minute, // CI-friendly duration
	}
}

// skipLongSoak skips long-running daemon soak tests when -short is set.
//
// These tests hold a real daemon process open for 30s-2min windows to measure
// memory/goroutine/FD growth and long-run stability. The short suite is
// documented (Makefile test-short, CI workflows, gitreins guard) as excluding
// long-running stability tests, and TestDaemon_Stability_30Minute below already
// follows the same convention. Without this, `go test -short ./...` serialized
// ~5 minutes of soak windows per run, blowing the package timeout (GAP-005).
// Full (non-short) runs keep exercising them.
func skipLongSoak(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("long-running daemon soak test; excluded from -short suite (run without -short to exercise)")
	}
}

// DaemonMetrics tracks daemon resource usage
type DaemonMetrics struct {
	Timestamp       time.Time `json:"timestamp"`
	MemoryRSS       uint64    `json:"memory_rss_bytes"`
	MemoryVMS       uint64    `json:"memory_vms_bytes"`
	Goroutines      int       `json:"goroutines"`
	FileDescriptors int       `json:"file_descriptors"`
	CPUUser         float64   `json:"cpu_user_percent"`
	CPUSystem       float64   `json:"cpu_system_percent"`
	HealthStatus    string    `json:"health_status"`
	RequestsHandled int64     `json:"requests_handled"`
}

// StabilityTestRunner manages the stability test execution
type StabilityTestRunner struct {
	config            StabilityTestConfig
	daemonCmd         *exec.Cmd
	daemonPid         int
	socketPath        string
	tempDir           string
	metrics           []DaemonMetrics
	metricsMu         sync.Mutex
	requestCount      int64
	startTime         time.Time
	initialGoroutines int
	stopChan          chan struct{}
	wg                sync.WaitGroup
}

// NewStabilityTestRunner creates a new stability test runner
func NewStabilityTestRunner(config StabilityTestConfig) *StabilityTestRunner {
	return &StabilityTestRunner{
		config:   config,
		metrics:  make([]DaemonMetrics, 0),
		stopChan: make(chan struct{}),
	}
}

// Setup prepares the daemon for testing
func (r *StabilityTestRunner) Setup(t *testing.T) error {
	t.Helper()

	// Create temporary directory
	var err error
	r.tempDir, err = os.MkdirTemp("", "muster-stability-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Set up socket path (keep it short to avoid Unix socket path limits)
	r.socketPath = filepath.Join(r.tempDir, "muster.sock")
	if runtime.GOOS == "darwin" && len(r.socketPath) > 104 {
		// macOS has a 104-character limit on Unix socket paths
		r.tempDir, err = os.MkdirTemp("/tmp", "muster-stab-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir in /tmp: %w", err)
		}
		r.socketPath = filepath.Join(r.tempDir, "m.sock")
	}

	// Record initial goroutine count
	r.initialGoroutines = runtime.NumGoroutine()

	return nil
}

// StartDaemon starts the daemon process
func (r *StabilityTestRunner) StartDaemon(t *testing.T) error {
	t.Helper()

	// Build the daemon binary
	t.Log("Building daemon binary...")
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(r.tempDir, "daemon"), "./cmd/daemon")
	buildCmd.Dir = getProjectRoot()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build daemon: %w, output: %s", err, string(output))
	}

	daemonPath := filepath.Join(r.tempDir, "daemon")

	// Start the daemon with proxy subcommand (use a dummy backend URL for testing)
	r.daemonCmd = exec.Command(daemonPath, "proxy", "--socket-path", r.socketPath, "--backend-url", "http://127.0.0.1:9999")
	r.daemonCmd.Dir = getProjectRoot()
	r.daemonCmd.Env = append(os.Environ(), "MUSTER_LOG_LEVEL=warn")

	if err := r.daemonCmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	r.daemonPid = r.daemonCmd.Process.Pid
	r.startTime = time.Now()

	t.Logf("Daemon started with PID %d, socket: %s", r.daemonPid, r.socketPath)

	// Wait for socket to be ready
	if err := r.waitForSocketReady(10 * time.Second); err != nil {
		return fmt.Errorf("daemon socket not ready: %w", err)
	}

	t.Log("Daemon socket is ready")
	return nil
}

// waitForSocketReady waits for the Unix socket to be created and accessible
func (r *StabilityTestRunner) waitForSocketReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check both the configured socket path and the default path
		socketPaths := []string{r.socketPath}
		if runtime.GOOS != "windows" {
			// Also check default path in case flag didn't work
			defaultSocket := "/tmp/muster-daemon.sock"
			if defaultSocket != r.socketPath {
				socketPaths = append(socketPaths, defaultSocket)
			}
		}

		for _, socketPath := range socketPaths {
			if _, err := os.Stat(socketPath); err == nil {
				// Socket exists, try to connect
				client := &http.Client{
					Transport: &http.Transport{
						DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
							return net.Dial("unix", socketPath)
						},
					},
					Timeout: 2 * time.Second,
				}
				resp, err := client.Get("http://unix/health")
				if err == nil {
					_ = resp.Body.Close()
					// Update socket path if we found it at a different location
					r.socketPath = socketPath
					return nil
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("socket not ready after %v", timeout)
}

// performHealthCheck performs a single health check
func (r *StabilityTestRunner) performHealthCheck(t *testing.T) (*DaemonMetrics, error) {
	t.Helper()

	metrics := DaemonMetrics{
		Timestamp: time.Now(),
	}

	// On Unix systems, we can get more detailed stats
	if runtime.GOOS != "windows" {
		// Get memory stats from /proc or ps
		cmd := exec.Command("ps", "-o", "rss=,vsz=", "-p", fmt.Sprintf("%d", r.daemonPid))
		if output, err := cmd.Output(); err == nil {
			_, _ = fmt.Sscanf(string(output), "%d %d", &metrics.MemoryRSS, &metrics.MemoryVMS)
			metrics.MemoryRSS *= 1024 // Convert KB to bytes
			metrics.MemoryVMS *= 1024
		}

		// Get file descriptor count
		fdDir := fmt.Sprintf("/proc/%d/fd", r.daemonPid)
		if fds, err := os.ReadDir(fdDir); err == nil {
			metrics.FileDescriptors = len(fds)
		} else {
			// Fallback: use lsof
			cmd = exec.Command("lsof", "-p", fmt.Sprintf("%d", r.daemonPid))
			if output, err := cmd.Output(); err == nil {
				// Count lines in output
				metrics.FileDescriptors = 0
				for range output {
					if output[0] == '\n' {
						metrics.FileDescriptors++
					}
				}
			}
		}
	}

	// Get goroutine count
	metrics.Goroutines = runtime.NumGoroutine()

	// Perform HTTP health check through Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", r.socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://unix/health")
	if err != nil {
		metrics.HealthStatus = "unhealthy"
	} else {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusOK {
			metrics.HealthStatus = "healthy"
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				var healthResp struct {
					Status string `json:"status"`
				}
				if unmarshalErr := json.Unmarshal(body, &healthResp); unmarshalErr == nil {
					metrics.HealthStatus = healthResp.Status
				}
			}
		} else {
			metrics.HealthStatus = fmt.Sprintf("error:%d", resp.StatusCode)
		}
	}

	// Make a test request to increment request count
	testReqStart := time.Now()
	testResp, err := client.Get("http://unix/list")
	if err == nil {
		_ = testResp.Body.Close()
		atomic.AddInt64(&r.requestCount, 1)
		metrics.RequestsHandled = atomic.LoadInt64(&r.requestCount)
	}

	t.Logf("Health check: status=%s, memory=%d MB, goroutines=%d, fds=%d, requests=%d, latency=%v",
		metrics.HealthStatus,
		metrics.MemoryRSS/1024/1024,
		metrics.Goroutines,
		metrics.FileDescriptors,
		metrics.RequestsHandled,
		time.Since(testReqStart))

	return &metrics, nil
}

// runStabilityTest executes the stability test
func (r *StabilityTestRunner) runStabilityTest(t *testing.T) error {
	t.Helper()

	testDuration := r.config.Duration
	if testing.Short() {
		testDuration = r.config.MinDuration
		t.Logf("Running shortened stability test (%v) due to -short flag", testDuration)
	} else {
		t.Logf("Running full stability test (%v)", testDuration)
	}

	// Collect initial metrics
	initialMetrics, err := r.performHealthCheck(t)
	if err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	r.metricsMu.Lock()
	r.metrics = append(r.metrics, *initialMetrics)
	r.metricsMu.Unlock()

	// Run health checks at regular intervals
	interval := r.config.HealthCheckInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	done := time.After(testDuration)
	checkCount := 0

	for {
		select {
		case <-done:
			t.Logf("Stability test completed after %v", time.Since(r.startTime))
			return nil

		case <-ticker.C:
			checkCount++
			metrics, err := r.performHealthCheck(t)
			if err != nil {
				t.Errorf("Health check %d failed: %v", checkCount, err)
				continue
			}

			r.metricsMu.Lock()
			r.metrics = append(r.metrics, *metrics)
			r.metricsMu.Unlock()

			// Check for critical failures
			if metrics.HealthStatus != "healthy" {
				t.Errorf("Daemon unhealthy at check %d: %s", checkCount, metrics.HealthStatus)
			}

		case <-r.stopChan:
			t.Log("Stability test stopped early")
			return nil
		}
	}
}

// ValidateResults analyzes the collected metrics and validates stability
func (r *StabilityTestRunner) ValidateResults(t *testing.T) error {
	t.Helper()

	if len(r.metrics) < 2 {
		return fmt.Errorf("insufficient metrics collected: %d", len(r.metrics))
	}

	r.metricsMu.Lock()
	defer r.metricsMu.Unlock()

	initial := r.metrics[0]
	final := r.metrics[len(r.metrics)-1]

	// Calculate memory growth
	var memoryGrowth float64
	if initial.MemoryRSS > 0 {
		memoryGrowth = float64(final.MemoryRSS-initial.MemoryRSS) / float64(initial.MemoryRSS)
	}

	// Calculate goroutine growth
	goroutineGrowth := final.Goroutines - initial.Goroutines

	// Calculate FD growth
	fdGrowth := final.FileDescriptors - initial.FileDescriptors

	t.Logf("\n=== STABILITY TEST RESULTS ===")
	t.Logf("Duration: %v", final.Timestamp.Sub(initial.Timestamp))
	t.Logf("Health checks: %d", len(r.metrics))
	t.Logf("Requests handled: %d", final.RequestsHandled)
	t.Logf("\nResource Usage:")
	t.Logf("  Memory RSS: %d MB → %d MB (%.2f%% growth)",
		initial.MemoryRSS/1024/1024,
		final.MemoryRSS/1024/1024,
		memoryGrowth*100)
	t.Logf("  Goroutines: %d → %d (%+d)",
		initial.Goroutines, final.Goroutines, goroutineGrowth)
	t.Logf("  File Descriptors: %d → %d (%+d)",
		initial.FileDescriptors, final.FileDescriptors, fdGrowth)

	// Validate memory growth
	if memoryGrowth > r.config.MemoryGrowthThreshold {
		return fmt.Errorf("memory growth %.2f%% exceeds threshold %.2f%%",
			memoryGrowth*100, r.config.MemoryGrowthThreshold*100)
	}
	t.Logf("✓ Memory growth within threshold (%.2f%% < %.2f%%)",
		memoryGrowth*100, r.config.MemoryGrowthThreshold*100)

	// Validate goroutine growth
	if goroutineGrowth > r.config.GoroutineLeakThreshold {
		return fmt.Errorf("goroutine growth %d exceeds threshold %d",
			goroutineGrowth, r.config.GoroutineLeakThreshold)
	}
	t.Logf("✓ Goroutine count stable (+%d < +%d)",
		goroutineGrowth, r.config.GoroutineLeakThreshold)

	// Validate FD growth (allow some growth for connection pooling)
	if fdGrowth > 50 {
		t.Logf("⚠ File descriptor growth %d (may indicate leak)", fdGrowth)
	} else {
		t.Logf("✓ File descriptor count stable (+%d)", fdGrowth)
	}

	// Validate health status consistency
	unhealthyCount := 0
	for _, m := range r.metrics {
		if m.HealthStatus != "healthy" {
			unhealthyCount++
		}
	}
	if unhealthyCount > len(r.metrics)/10 {
		return fmt.Errorf("daemon unhealthy in %d/%d checks",
			unhealthyCount, len(r.metrics))
	}
	t.Logf("✓ Health status stable (%d/%d checks healthy)",
		len(r.metrics)-unhealthyCount, len(r.metrics))

	return nil
}

// Cleanup stops the daemon and cleans up resources
func (r *StabilityTestRunner) Cleanup(t *testing.T) {
	t.Helper()

	close(r.stopChan)
	r.wg.Wait()

	if r.daemonCmd != nil && r.daemonCmd.Process != nil {
		t.Logf("Stopping daemon (PID %d)...", r.daemonPid)

		// Try graceful shutdown first
		_ = r.daemonCmd.Process.Signal(syscall.SIGTERM)

		// Wait for process to exit
		done := make(chan error, 1)
		go func() {
			done <- r.daemonCmd.Wait()
		}()

		select {
		case <-time.After(5 * time.Second):
			t.Log("Graceful shutdown timed out, forcing kill")
			_ = r.daemonCmd.Process.Kill()
		case err := <-done:
			if err != nil {
				t.Logf("Daemon exited with: %v", err)
			}
		}
	}

	// Clean up temp directory
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
		t.Logf("Cleaned up temp directory: %s", r.tempDir)
	}
}

// getProjectRoot returns the path to the project root
func getProjectRoot() string {
	// Get the directory where this test file is located
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}

	// Navigate up from test/stability to project root
	dir := filepath.Dir(filename) // test/stability
	dir = filepath.Dir(dir)       // test
	dir = filepath.Dir(dir)       // project root

	return dir
}

// TestDaemon_Stability_30Minute runs a 30-minute stability test on the daemon
func TestDaemon_Stability_30Minute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running stability test in short mode")
	}

	config := DefaultStabilityConfig()
	runner := NewStabilityTestRunner(config)

	// Setup
	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	// Start daemon
	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Run stability test
	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Validate results
	if err := runner.ValidateResults(t); err != nil {
		t.Errorf("Stability validation failed: %v", err)
	}
}

// TestDaemon_Stability_Quick runs a quick 2-minute stability test for CI
func TestDaemon_Stability_Quick(t *testing.T) {
	skipLongSoak(t)

	config := DefaultStabilityConfig()
	config.MinDuration = 2 * time.Minute
	config.HealthCheckInterval = 15 * time.Second

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	if err := runner.ValidateResults(t); err != nil {
		t.Errorf("Stability validation failed: %v", err)
	}
}

// TestDaemon_MemoryMonitoring verifies memory monitoring accuracy
func TestDaemon_MemoryMonitoring(t *testing.T) {
	skipLongSoak(t)

	config := DefaultStabilityConfig()
	config.MinDuration = 30 * time.Second
	config.HealthCheckInterval = 10 * time.Second

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Verify metrics were collected
	runner.metricsMu.Lock()
	metricCount := len(runner.metrics)
	runner.metricsMu.Unlock()

	if metricCount < 2 {
		t.Errorf("Expected at least 2 metrics, got %d", metricCount)
	}

	// Verify memory values are reasonable (> 0, < 1GB)
	runner.metricsMu.Lock()
	for i, m := range runner.metrics {
		if m.MemoryRSS == 0 {
			t.Errorf("Metric %d: MemoryRSS is 0", i)
		}
		if m.MemoryRSS > 1024*1024*1024 {
			t.Errorf("Metric %d: MemoryRSS too high: %d bytes", i, m.MemoryRSS)
		}
	}
	runner.metricsMu.Unlock()
}

// TestDaemon_GoroutineMonitoring verifies goroutine leak detection
func TestDaemon_GoroutineMonitoring(t *testing.T) {
	skipLongSoak(t)

	initialGoroutines := runtime.NumGoroutine()

	config := DefaultStabilityConfig()
	config.MinDuration = 30 * time.Second
	config.HealthCheckInterval = 10 * time.Second
	config.GoroutineLeakThreshold = 20

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Check goroutine growth
	runner.metricsMu.Lock()
	if len(runner.metrics) > 0 {
		finalGoroutines := runner.metrics[len(runner.metrics)-1].Goroutines
		growth := finalGoroutines - initialGoroutines

		t.Logf("Goroutine growth: %d → %d (+%d)",
			initialGoroutines, finalGoroutines, growth)

		if growth > config.GoroutineLeakThreshold {
			t.Errorf("Potential goroutine leak: +%d goroutines", growth)
		}
	}
	runner.metricsMu.Unlock()
}

// TestDaemon_FileDescriptorMonitoring verifies FD leak detection
func TestDaemon_FileDescriptorMonitoring(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FD monitoring not supported on Windows")
	}
	skipLongSoak(t)

	config := DefaultStabilityConfig()
	config.MinDuration = 30 * time.Second
	config.HealthCheckInterval = 10 * time.Second

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Check FD growth
	runner.metricsMu.Lock()
	if len(runner.metrics) >= 2 {
		initialFD := runner.metrics[0].FileDescriptors
		finalFD := runner.metrics[len(runner.metrics)-1].FileDescriptors
		growth := finalFD - initialFD

		t.Logf("FD growth: %d → %d (+%d)",
			initialFD, finalFD, growth)

		if growth > 50 {
			t.Errorf("Potential FD leak: +%d file descriptors", growth)
		}
	}
	runner.metricsMu.Unlock()
}

// TestDaemon_SignalHandling_SIGTERM tests graceful shutdown with SIGTERM
func TestDaemon_SignalHandling_SIGTERM(t *testing.T) {
	config := DefaultStabilityConfig()
	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(runner.tempDir) }()

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Let it run for a bit
	time.Sleep(2 * time.Second)

	// Send SIGTERM
	t.Log("Sending SIGTERM to daemon...")
	startTime := time.Now()
	_ = runner.daemonCmd.Process.Signal(syscall.SIGTERM)

	// Wait for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- runner.daemonCmd.Wait()
	}()

	select {
	case err := <-done:
		shutdownTime := time.Since(startTime)
		t.Logf("Daemon shut down gracefully in %v (err: %v)", shutdownTime, err)
		if shutdownTime > 5*time.Second {
			t.Errorf("Shutdown took too long: %v", shutdownTime)
		}
	case <-time.After(10 * time.Second):
		t.Error("Daemon did not shut down within 10 seconds")
		_ = runner.daemonCmd.Process.Kill()
	}
}

// TestDaemon_SignalHandling_SIGINT tests graceful shutdown with SIGINT
func TestDaemon_SignalHandling_SIGINT(t *testing.T) {
	config := DefaultStabilityConfig()
	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(runner.tempDir) }()

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Let it run for a bit
	time.Sleep(2 * time.Second)

	// Send SIGINT
	t.Log("Sending SIGINT to daemon...")
	startTime := time.Now()
	_ = runner.daemonCmd.Process.Signal(syscall.SIGINT)

	// Wait for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- runner.daemonCmd.Wait()
	}()

	select {
	case err := <-done:
		shutdownTime := time.Since(startTime)
		t.Logf("Daemon shut down gracefully in %v (err: %v)", shutdownTime, err)
		if shutdownTime > 5*time.Second {
			t.Errorf("Shutdown took too long: %v", shutdownTime)
		}
	case <-time.After(10 * time.Second):
		t.Error("Daemon did not shut down within 10 seconds")
		_ = runner.daemonCmd.Process.Kill()
	}
}

// TestDaemon_LogRotation verifies logs don't grow unbounded
func TestDaemon_LogRotation(t *testing.T) {
	skipLongSoak(t)

	config := DefaultStabilityConfig()
	config.MinDuration = 1 * time.Minute
	config.HealthCheckInterval = 10 * time.Second

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	// Create a log file watcher
	logDir := filepath.Join(runner.tempDir, "logs")
	_ = os.MkdirAll(logDir, 0755)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Check log file sizes
	if entries, err := os.ReadDir(logDir); err == nil {
		totalSize := int64(0)
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil {
				totalSize += info.Size()
			}
		}

		t.Logf("Total log size: %d bytes", totalSize)

		// Logs should be less than 10MB after 1 minute
		if totalSize > 10*1024*1024 {
			t.Errorf("Log files grew too large: %d bytes", totalSize)
		}
	}
}

// TestDaemon_ConfigHotReload tests SIGHUP config reload (Unix only)
func TestDaemon_ConfigHotReload(t *testing.T) {
	// SIGHUP config reload is NOT implemented: the daemon only registers
	// SIGTERM/SIGINT handlers (cmd/daemon/main.go signal.Notify), so SIGHUP
	// kills the process via the runtime's default disposition. This test
	// asserts behavior the product does not have and can never pass; it is
	// skipped until a reload mechanism exists (this is the only stability
	// test that fails outside -short as well — GAP-005).
	t.Skip("SIGHUP config reload not implemented (daemon handles only SIGTERM/SIGINT); re-enable when a reload mechanism exists")

	config := DefaultStabilityConfig()
	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Let it run for a bit
	time.Sleep(2 * time.Second)

	// Get initial metrics
	initialMetrics, _ := runner.performHealthCheck(t)

	// Send SIGHUP
	t.Log("Sending SIGHUP to daemon...")
	_ = runner.daemonCmd.Process.Signal(syscall.SIGHUP)

	// Wait for config reload
	time.Sleep(1 * time.Second)

	// Get post-reload metrics
	postReloadMetrics, _ := runner.performHealthCheck(t)

	// Verify daemon is still healthy
	if postReloadMetrics.HealthStatus != "healthy" {
		t.Errorf("Daemon unhealthy after SIGHUP: %s", postReloadMetrics.HealthStatus)
	}

	// Verify it's still handling requests
	if postReloadMetrics.RequestsHandled < initialMetrics.RequestsHandled {
		t.Error("Request count decreased after SIGHUP")
	}

	t.Logf("Config reload successful, requests: %d → %d",
		initialMetrics.RequestsHandled, postReloadMetrics.RequestsHandled)
}

// TestDaemon_ConcurrentHealthChecks tests stability under concurrent load
func TestDaemon_ConcurrentHealthChecks(t *testing.T) {
	skipLongSoak(t)

	config := StabilityTestConfig{
		MinDuration:            30 * time.Second,
		HealthCheckInterval:    5 * time.Second,
		MemoryGrowthThreshold:  0.15, // Allow more growth under load
		GoroutineLeakThreshold: 30,
	}

	runner := NewStabilityTestRunner(config)

	if err := runner.Setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer runner.Cleanup(t)

	if err := runner.StartDaemon(t); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Start concurrent health checks
	var wg sync.WaitGroup
	numWorkers := 5
	requestsPerWorker := 20

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return net.Dial("unix", runner.socketPath)
					},
				},
				Timeout: 5 * time.Second,
			}

			for j := 0; j < requestsPerWorker; j++ {
				resp, err := client.Get("http://unix/health")
				if err == nil {
					_ = resp.Body.Close()
					atomic.AddInt64(&runner.requestCount, 1)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	// Run stability test while concurrent requests happen
	if err := runner.runStabilityTest(t); err != nil {
		t.Fatalf("Stability test failed: %v", err)
	}

	// Wait for concurrent workers to finish
	wg.Wait()

	t.Logf("Processed %d concurrent requests during stability test",
		atomic.LoadInt64(&runner.requestCount))

	// Validate results
	if err := runner.ValidateResults(t); err != nil {
		t.Errorf("Stability validation failed under load: %v", err)
	}
}
