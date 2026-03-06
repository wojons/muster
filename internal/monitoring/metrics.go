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
)

const (
	NamespaceCLI    = "openapi_cli"
	NamespaceMCP    = "openapi_mcp"
	NamespaceDaemon = "openapi_daemon"
)

var (
	CommandExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceCLI,
			Name:      "command_executions_total",
			Help:      "Total number of command executions",
		},
		[]string{"command", "status", "tenant_id"},
	)

	CommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: NamespaceCLI,
			Name:      "command_duration_seconds",
			Help:      "Command execution duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"command", "tenant_id"},
	)

	CommandErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceCLI,
			Name:      "command_errors_total",
			Help:      "Total number of command execution errors",
		},
		[]string{"command", "error_type", "tenant_id"},
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
		[]string{"method", "tool", "tenant_id"},
	)

	MCPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: NamespaceMCP,
			Name:      "request_duration_seconds",
			Help:      "MCP request duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "tenant_id"},
	)

	MCPToolExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NamespaceMCP,
			Name:      "tool_executions_total",
			Help:      "Total number of tool executions",
		},
		[]string{"tool_name", "status", "tenant_id"},
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

	DaemonCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: NamespaceDaemon,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
	)

	DaemonCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: NamespaceDaemon,
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
	)
)

type MetricRecorder struct {
	tenantID string
	registry *prometheus.Registry
	logger   *zerolog.Logger
	mu       sync.RWMutex
}

func NewMetricRecorder(tenantID string, logger *zerolog.Logger) *MetricRecorder {
	return &MetricRecorder{
		tenantID: tenantID,
		registry: prometheus.NewRegistry(),
		logger:   logger,
	}
}

func (mr *MetricRecorder) RecordCommandExecution(command string, status string, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	labels := prometheus.Labels{
		"command":   command,
		"status":    status,
		"tenant_id": mr.tenantID,
	}
	CommandExecutions.With(labels).Inc()
	CommandDuration.With(labels).Observe(duration.Seconds())
}

func (mr *MetricRecorder) RecordCommandError(command string, errorType string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	CommandErrors.With(prometheus.Labels{
		"command":    command,
		"error_type": errorType,
		"tenant_id":  mr.tenantID,
	}).Inc()
}

func (mr *MetricRecorder) RecordMCPRequest(method string, duration time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	MCPRequests.With(prometheus.Labels{
		"method":    method,
		"tenant_id": mr.tenantID,
	}).Inc()
	MCPRequestDuration.With(prometheus.Labels{
		"method":    method,
		"tenant_id": mr.tenantID,
	}).Observe(duration.Seconds())
}

func (mr *MetricRecorder) RecordToolExecution(toolName string, status string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	MCPToolExecutions.With(prometheus.Labels{
		"tool_name": toolName,
		"status":    status,
		"tenant_id": mr.tenantID,
	}).Inc()
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

func (mr *MetricRecorder) RecordCacheHit() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	DaemonCacheHits.Inc()
}

func (mr *MetricRecorder) RecordCacheMiss() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	DaemonCacheMisses.Inc()
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
			fmt.Printf("metrics server error: %v\n", err)
		}
	}()

	return nil
}

func (ms *MetricsServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"alive","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ready","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) startupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"started","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ms *MetricsServer) Shutdown(ctx context.Context) error {
	if ms.server != nil {
		return ms.server.Shutdown(ctx)
	}
	return nil
}
