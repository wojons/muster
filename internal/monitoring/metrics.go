package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/log"
)

const (
	NamespaceCLI    = "muster_cli"
	NamespaceMCP    = "muster_mcp"
	NamespaceDaemon = "muster_daemon"
)

var (
	CommandExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceCLI,
			Name:      "command_executions_total",
			Help:      "Total number of command executions",
		},
		[]string{"command", "status"},
	)

	CommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: NamespaceCLI,
			Name:      "command_duration_seconds",
			Help:      "Command execution duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"command", "status"},
	)

	CommandErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceCLI,
			Name:      "command_errors_total",
			Help:      "Total number of command execution errors",
		},
		[]string{"command", "error_type"},
	)

	MCPConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: NamespaceMCP,
			Name:      "active_connections",
			Help:      "Number of active MCP connections",
		},
	)

	MCPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceMCP,
			Name:      "requests_total",
			Help:      "Total number of MCP requests",
		},
		[]string{"method"},
	)

	MCPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: NamespaceMCP,
			Name:      "request_duration_seconds",
			Help:      "MCP request duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method"},
	)

	MCPToolExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceMCP,
			Name:      "tool_executions_total",
			Help:      "Total number of tool executions",
		},
		[]string{"tool_name", "status"},
	)

	DaemonActiveSockets = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: NamespaceDaemon,
			Name:      "active_socket_connections",
			Help:      "Number of active Unix socket connections",
		},
	)

	DaemonProxyRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceDaemon,
			Name:      "proxy_requests_total",
			Help:      "Total number of proxy requests",
		},
		[]string{"endpoint", "method", "tenant_id"},
	)
)

// MetricRecorder is the recording surface for the muster_* families of
// docs/observability-spec.md §3.1 and §3.4.
//
// The process-scoped muster_cli/muster_mcp/muster_daemon families are
// registered through promauto (package init), while the spec-defined families
// live on an explicit registry owned by this recorder. Registry() returns that
// explicit registry and is what a caller must serve.
type MetricRecorder struct {
	tenantID string
	registry *prometheus.Registry
	muster   *MusterMetrics
	logger   *zerolog.Logger
	mu       sync.RWMutex
}

// NewMetricRecorder creates a recorder. Each recorder owns a fresh explicit
// registry holding the muster_* families, so tests never collide and double
// registration is impossible.
func NewMetricRecorder(tenantID string, logger *zerolog.Logger) *MetricRecorder {
	muster := NewMusterMetrics(tenantID, version.Get().Version, version.Get().Commit)
	audit.SetMetrics(muster)
	return &MetricRecorder{
		tenantID: tenantID,
		registry: muster.Registry(),
		muster:   muster,
		logger:   logger,
	}
}

// Registry returns the explicit registry holding the muster_* families. Serve
// this one (promhttp.HandlerFor) rather than the default registry.
func (mr *MetricRecorder) Registry() *prometheus.Registry {
	return mr.registry
}

// Muster exposes the muster_* families and their recording methods.
func (mr *MetricRecorder) Muster() *MusterMetrics {
	return mr.muster
}

func (mr *MetricRecorder) RecordCommandExecution(command string, status string, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	labels := prometheus.Labels{"command": command, "status": status}
	CommandExecutions.With(labels).Inc()
	CommandDuration.With(labels).Observe(duration.Seconds())
}

func (mr *MetricRecorder) RecordCommandError(command string, errorType string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	CommandErrors.With(prometheus.Labels{
		"command":    command,
		"error_type": errorType,
	}).Inc()
}

func (mr *MetricRecorder) RecordMCPRequest(method string, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	MCPRequests.With(prometheus.Labels{
		"method": method,
	}).Inc()
	MCPRequestDuration.With(prometheus.Labels{
		"method": method,
	}).Observe(duration.Seconds())
}

func (mr *MetricRecorder) RecordToolExecution(toolName string, status string) {
	errorClass := "internal"
	if status == "success" {
		errorClass = "none"
	}
	mr.RecordMCPTool(toolName, status, errorClass, 0)
}

// RecordMCPTool records one MCP tool execution in both the legacy recorder
// surface and the spec-defined muster_* registry.
func (mr *MetricRecorder) RecordMCPTool(tool, status, errorClass string, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	MCPToolExecutions.With(prometheus.Labels{
		"tool_name": tool,
		"status":    status,
	}).Inc()
	mr.muster.RecordMCPTool(tool, status, errorClass, duration)
}

// SetMCPActiveSessions records the current number of MCP sessions.
func (mr *MetricRecorder) SetMCPActiveSessions(sessions int) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.muster.SetMCPActiveSessions(sessions)
}

// MCPRegistry returns the muster_mcp_* only registry the textfile writer
// publishes (spec §4.2 option 1: the stdio server has no scrape target).
func (mr *MetricRecorder) MCPRegistry() *prometheus.Registry {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.muster.MCPRegistry()
}

func (mr *MetricRecorder) RecordProxyRequest(endpoint, method string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	DaemonProxyRequests.With(prometheus.Labels{
		"endpoint":  endpoint,
		"method":    method,
		"tenant_id": mr.tenantID,
	}).Inc()
}

// RecordDaemonProxyRequest records one proxied request on the explicit
// registry (muster_daemon_proxy_requests_total and
// muster_daemon_proxy_duration_seconds). status is bucketed into a
// status_class label, never emitted raw.
func (mr *MetricRecorder) RecordDaemonProxyRequest(endpoint, method string, status int, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.muster.RecordProxyRequest(endpoint, method, status, duration)
}

// RecordAuditEvent records one audit event written to the record of truth
// (muster_audit_events_written_total). See the TODO in NewMusterMetrics: the
// daemon has no audit write call site yet (OBS-MUSTER-2).
func (mr *MetricRecorder) RecordAuditEvent(eventType, category string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.muster.RecordAuditEvent(eventType, category)
}

// RecordAuditEventDropped records an audit event lost by the audit sink.
func (mr *MetricRecorder) RecordAuditEventDropped(reason string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.muster.RecordAuditEventDropped(reason)
}

// SetAuditBufferUtilization records the audit buffer occupancy ratio.
func (mr *MetricRecorder) SetAuditBufferUtilization(ratio float64) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.muster.SetAuditBufferUtilization(ratio)
}

func (mr *MetricRecorder) IncMCPConnections() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	MCPConnections.Inc()
}

func (mr *MetricRecorder) DecMCPConnections() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	MCPConnections.Dec()
}

func (mr *MetricRecorder) IncDaemonSocketConnections() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	DaemonActiveSockets.Inc()
}

func (mr *MetricRecorder) DecDaemonSocketConnections() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	DaemonActiveSockets.Dec()
}

// MetricsServer is the legacy (pre-OBS-MUSTER-1) multi-endpoint server. It is
// retained because its families and tests still exist, but it serves the
// DEFAULT registry via promhttp.Handler(), which spec §4.2 forbids for
// production code — the daemon therefore serves MusterMetricsServer (explicit
// registry) and never this type. Do not wire it into a binary.
type MetricsServer struct {
	addr   string
	server *http.Server
}

func NewMetricsServer(addr string) *MetricsServer {
	return &MetricsServer{
		addr: addr,
	}
}

func (ms *MetricsServer) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", ms.healthHandler)
	mux.HandleFunc("/health/live", ms.livenessHandler)
	mux.HandleFunc("/health/ready", ms.readinessHandler)
	mux.HandleFunc("/health/startup", ms.startupHandler)

	ms.server = &http.Server{
		Addr:         ms.addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := ms.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Global().Error().Err(err).Msg("Metrics server error")
		}
	}()

	return nil
}

func (ms *MetricsServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"alive","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ready","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) startupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"started","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) Shutdown(ctx context.Context) error {
	if ms.server != nil {
		return ms.server.Shutdown(ctx)
	}
	return nil
}
