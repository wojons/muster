package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wojons/muster/pkg/log"
)

// MusterNamespace is the metric namespace of the minimal viable set
// (docs/observability-spec.md §3.1). It supersedes the legacy
// openapi_cli/openapi_mcp/openapi_daemon namespaces for measurement purposes;
// those legacy families stay registered (other packages and tests reference
// them) but nothing in the daemon serves them.
const MusterNamespace = "muster"

// DefaultTenantID is the tenant_id label value used when the process has no
// tenant identity of its own. The daemon is single-tenant today; the label is
// present from day one so a multi-tenant deployment does not need a metric
// rename.
const DefaultTenantID = "default"

// Placeholder label values for the zero-value seed emitted for
// muster_audit_events_written_total. The counter must be visible in every
// scrape even before any event has been written (spec §3.1: "is the record of
// truth actually being written?" is only answerable if the series exists).
const (
	auditSeedEventType = "none"
	auditSeedCategory  = "none"
)

// Placeholder label values for the zero-value seeds emitted for the two proxy
// families. A *Vec with no children is absent from a scrape, so a freshly
// started daemon — one that has not proxied anything yet — would answer "up"
// and "build" but say nothing at all about traffic, which is indistinguishable
// from a broken exporter. Creating the child series at startup (without
// observing anything: a histogram seed must NOT Observe, or the seed would
// forge a sample) makes "0 requests so far" an explicit, honest fact.
//
// "none" is never produced by StatusClass or by a real HTTP method, so a seed
// series is unambiguous in a dashboard.
const (
	proxySeedEndpoint    = "/"
	proxySeedMethod      = "none"
	proxySeedStatusClass = "none"
)

// MusterMetrics holds ONE explicit *prometheus.Registry with the metric
// families of docs/observability-spec.md §3.1 and §3.4 registered on it, plus
// the recording methods that write to them.
//
// The registry is deliberately not prometheus.DefaultRegisterer (§4.2: "Do not
// use the default prometheus registry in production code"). One explicit
// registry per process means:
//   - a scrape serves exactly the spec'd families, never unrelated promauto
//     instrumentation that happens to be linked into the binary;
//   - tests are hermetic because each MusterMetrics owns a fresh registry;
//   - double registration is impossible — nothing is registered globally.
type MusterMetrics struct {
	registry *prometheus.Registry
	tenantID string

	// BuildInfo answers "what exactly is running on this host?".
	BuildInfo *prometheus.GaugeVec
	// DaemonUp answers "is the daemon answering right now?".
	DaemonUp *prometheus.GaugeVec
	// ProxyRequests answers "is the daemon doing work, and how much of it is
	// failing?".
	ProxyRequests *prometheus.CounterVec
	// ProxyDuration answers "how slow are proxied calls?".
	ProxyDuration *prometheus.HistogramVec
	// AuditEventsWritten answers "is the record of truth actually being
	// written?".
	AuditEventsWritten *prometheus.CounterVec
	// AuditEventsDropped counts events rejected or lost by the audit sink.
	AuditEventsDropped *prometheus.CounterVec
	// AuditBufferUtilization reports the fraction of the audit buffer occupied.
	AuditBufferUtilization prometheus.Gauge
}

// NewMusterMetrics builds the explicit registry and registers the five
// families on it. tenantID becomes the tenant_id label value on every family
// that carries one; version/commit stamp muster_build_info.
func NewMusterMetrics(tenantID, version, commit string) *MusterMetrics {
	if tenantID == "" {
		tenantID = DefaultTenantID
	}

	m := &MusterMetrics{
		registry: prometheus.NewRegistry(),
		tenantID: tenantID,

		BuildInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: MusterNamespace,
				Name:      "build_info",
				Help:      "Build information: always 1, with the version and commit of the running binary as labels.",
			},
			[]string{"version", "commit"},
		),
		DaemonUp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: MusterNamespace,
				Name:      "daemon_up",
				Help:      "1 while the daemon is serving, 0 while it is shutting down.",
			},
			[]string{"tenant_id"},
		),
		ProxyRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MusterNamespace,
				Name:      "daemon_proxy_requests_total",
				Help:      "Total proxied requests, by endpoint, method and response status class.",
			},
			[]string{"endpoint", "method", "status_class", "tenant_id"},
		),
		ProxyDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: MusterNamespace,
				Name:      "daemon_proxy_duration_seconds",
				Help:      "Proxied request duration in seconds, by endpoint and method.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"endpoint", "method"},
		),
		AuditEventsWritten: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MusterNamespace,
				Name:      "audit_events_written_total",
				Help:      "Audit events written to the record of truth, by event type and category.",
			},
			[]string{"event_type", "category"},
		),
		AuditEventsDropped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MusterNamespace,
				Name:      "audit_events_dropped_total",
				Help:      "Audit events rejected or lost by the audit sink, by reason.",
			},
			[]string{"reason"},
		),
		AuditBufferUtilization: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: MusterNamespace,
			Name:      "audit_buffer_utilization_ratio",
			Help:      "Fraction of the audit event buffer currently occupied.",
		}),
	}

	m.registry.MustRegister(
		m.BuildInfo,
		m.DaemonUp,
		m.ProxyRequests,
		m.ProxyDuration,
		m.AuditEventsWritten,
		m.AuditEventsDropped,
		m.AuditBufferUtilization,
	)

	// build_info is always 1: the process is running, so the pair
	// (version, commit) is a fact about this scrape target.
	m.BuildInfo.WithLabelValues(version, commit).Set(1)

	// daemon_up starts at 0 and is raised by MarkUp once the socket answers,
	// so a scrape that lands during startup reports "not answering yet"
	// rather than a lie.
	m.DaemonUp.WithLabelValues(tenantID).Set(0)

	// Zero-value seeds for the two proxy families: creating the child series
	// makes "the daemon has proxied nothing yet" visible instead of absent.
	// WithLabelValues alone creates the series; nothing is counted or
	// observed, so the histogram reports count=0 rather than a forged sample.
	m.ProxyRequests.WithLabelValues(proxySeedEndpoint, proxySeedMethod, proxySeedStatusClass, tenantID)
	m.ProxyDuration.WithLabelValues(proxySeedEndpoint, proxySeedMethod)

	// Zero-value seeds keep the audit health families visible in the first scrape;
	// the audit package updates them through its injected ownership interface.
	m.AuditEventsWritten.WithLabelValues(auditSeedEventType, auditSeedCategory).Add(0)
	m.AuditEventsDropped.WithLabelValues("buffer_full").Add(0)
	m.AuditEventsDropped.WithLabelValues("write_error").Add(0)
	m.AuditBufferUtilization.Set(0)

	return m
}

// Registry exposes the explicit registry. Serve this one — promhttp.
// HandlerFor(m.Registry()) — never prometheus.DefaultGatherer.
func (m *MusterMetrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Handler returns the scrape handler for this registry. It is the only
// handler the daemon's TCP metrics listener serves.
func (m *MusterMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}

// TenantID returns the label value applied to tenant-scoped families.
func (m *MusterMetrics) TenantID() string {
	if m == nil {
		return DefaultTenantID
	}
	return m.tenantID
}

// MarkUp records that the daemon is answering (daemon_up = 1).
func (m *MusterMetrics) MarkUp() {
	if m == nil {
		return
	}
	m.DaemonUp.WithLabelValues(m.tenantID).Set(1)
}

// MarkDown records that the daemon is no longer answering (daemon_up = 0).
// It is called on the shutdown path so the last scrape before exit reports
// the truth instead of a stale 1.
func (m *MusterMetrics) MarkDown() {
	if m == nil {
		return
	}
	m.DaemonUp.WithLabelValues(m.tenantID).Set(0)
}

// RecordProxyRequest records one proxied request. statusClass is derived from
// status, never a raw code+text, so the label set stays bounded.
func (m *MusterMetrics) RecordProxyRequest(endpoint, method string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	if method == "" {
		method = "UNKNOWN"
	}
	m.ProxyRequests.WithLabelValues(
		NormalizeEndpoint(endpoint),
		method,
		StatusClass(status),
		m.tenantID,
	).Inc()
	m.ProxyDuration.WithLabelValues(
		NormalizeEndpoint(endpoint),
		method,
	).Observe(duration.Seconds())
}

// RecordAuditEvent records one audit event written to the record of truth.
func (m *MusterMetrics) RecordAuditEvent(eventType, category string) {
	if m == nil {
		return
	}
	if eventType == "" {
		eventType = "unknown"
	}
	if category == "" {
		category = "unknown"
	}
	m.AuditEventsWritten.WithLabelValues(eventType, category).Inc()
}

// RecordAuditEventDropped records an event rejected or lost by the audit sink.
func (m *MusterMetrics) RecordAuditEventDropped(reason string) {
	if m == nil {
		return
	}
	m.AuditEventsDropped.WithLabelValues(reason).Inc()
}

// SetAuditBufferUtilization records the current occupied-buffer ratio.
func (m *MusterMetrics) SetAuditBufferUtilization(ratio float64) {
	if m == nil {
		return
	}
	m.AuditBufferUtilization.Set(ratio)
}

// StatusClass buckets an HTTP status code into a bounded label value
// ("1xx".."5xx", or "unknown" for a code outside the HTTP range). Spec §3.1
// requires status_class, never the raw code, so the counter cannot explode
// its cardinality per response.
func StatusClass(status int) string {
	switch {
	case status >= 100 && status < 200:
		return "1xx"
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "unknown"
	}
}

// NormalizeEndpoint reduces a request path to a bounded endpoint label: path
// segments that are opaque identifiers (pure digits, UUIDs/hex hashes, or very
// long tokens) collapse to "{id}". Without this the endpoint label would grow
// with every distinct resource id in the upstream API.
func NormalizeEndpoint(path string) string {
	if path == "" {
		return "/"
	}
	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" {
		return "/"
	}
	segments := strings.Split(trimmed, "/")
	for i, segment := range segments {
		if isIdentifierSegment(segment) {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}

// isIdentifierSegment reports whether a path segment looks like an opaque
// resource id rather than a static route component.
func isIdentifierSegment(segment string) bool {
	if segment == "" {
		return false
	}
	if len(segment) > 40 {
		// Long opaque token (signed blob, base64 handle): never a route name.
		return true
	}
	// Pure digits: /projects/123.
	if _, err := strconv.ParseUint(segment, 10, 64); err == nil {
		return true
	}
	// UUID / hex hash: /runs/9f2c1a3e-...
	if len(segment) >= 8 {
		hexish := true
		hexChars := 0
		for _, r := range segment {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
				hexChars++
			case r == '-' || r == '_':
				// separators allowed
			default:
				hexish = false
			}
			if !hexish {
				break
			}
		}
		if hexish && hexChars >= 8 {
			return true
		}
	}
	return false
}

// MusterMetricsServer serves muster metrics over a loopback TCP listener.
//
// The listener exists because the daemon's only other HTTP surface is a Unix
// socket, which Prometheus cannot scrape (spec §4.1/§4.2). It serves ONLY
// /metrics: no proxied path is reachable through it, so a path-traversal
// against the metrics port cannot reach the upstream proxy.
type MusterMetricsServer struct {
	addr    string
	metrics *MusterMetrics

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
}

// NewMusterMetricsServer creates a metrics listener for addr. Start validates
// that addr is loopback before binding.
func NewMusterMetricsServer(addr string, m *MusterMetrics) *MusterMetricsServer {
	return &MusterMetricsServer{
		addr:    addr,
		metrics: m,
	}
}

// DefaultMetricsAddr is the loopback address the daemon binds by default.
const DefaultMetricsAddr = "127.0.0.1:9110"

// ValidateLoopbackAddr checks that addr names a loopback host and a port.
// Binding the metrics port on every interface would expose internal traffic
// shape to the network, which spec §4.2 forbids ("Bind loopback only").
func ValidateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("metrics addr %q: %w", addr, err)
	}
	if port == "" {
		return fmt.Errorf("metrics addr %q: missing port", addr)
	}
	if host == "" {
		return fmt.Errorf("metrics addr %q: empty host binds every interface; use %s", addr, DefaultMetricsAddr)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("metrics addr %q: %q is not an IP address (use an IP like 127.0.0.1)", addr, host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("metrics addr %q: %s is not a loopback address; the metrics listener binds loopback only", addr, ip)
	}
	return nil
}

// Start binds the loopback listener and serves /metrics. It returns an error
// if the address is not loopback or the bind fails; the caller decides whether
// that is fatal (the daemon treats it as a loud warning so a busy metrics port
// cannot take the socket down with it).
func (s *MusterMetricsServer) Start() error {
	if s.metrics == nil {
		return errors.New("metrics server: nil metrics")
	}
	if err := ValidateLoopbackAddr(s.addr); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", s.metrics.Handler())

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("metrics listen %q: %w", s.addr, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	s.mu.Lock()
	s.listener = listener
	s.server = server
	s.mu.Unlock()

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Global().Error().Err(serveErr).Str("addr", listener.Addr().String()).Msg("metrics server error")
		}
	}()

	return nil
}

// Addr returns the address the listener actually bound (useful when the
// configured port is 0).
func (s *MusterMetricsServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// Shutdown stops the metrics listener.
func (s *MusterMetricsServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	listener := s.listener
	s.server = nil
	s.listener = nil
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		if listener != nil {
			_ = listener.Close()
		}
		return err
	}
	return nil
}
