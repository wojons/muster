package monitoring

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// musterFamilies is the exact set docs/observability-spec.md §3.1 requires on
// the daemon's /metrics endpoint. The tests below assert every one of them is
// actually exposable from the explicit registry — a metric that is merely
// declared but never registered (or a *Vec with no child series) is absent
// from a scrape and therefore not evidence of anything.
var musterFamilies = []string{
	"muster_build_info",
	"muster_daemon_up",
	"muster_daemon_proxy_requests_total",
	"muster_daemon_proxy_duration_seconds",
	"muster_audit_events_written_total",
}

// gatherFamilyNames returns the set of metric family names a gatherer exports.
func gatherFamilyNames(t *testing.T, gatherer prometheus.Gatherer) map[string]bool {
	t.Helper()
	families, err := gatherer.Gather()
	require.NoError(t, err, "gather must succeed")

	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	return names
}

// missingFamilies returns the wanted family names the gatherer does not export.
func missingFamilies(t *testing.T, gatherer prometheus.Gatherer, want []string) []string {
	t.Helper()
	names := gatherFamilyNames(t, gatherer)
	var missing []string
	for _, name := range want {
		if !names[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// seriesCount returns how many series a family exports (0 when absent).
func seriesCount(t *testing.T, gatherer prometheus.Gatherer, familyName string) int {
	t.Helper()
	families, err := gatherer.Gather()
	require.NoError(t, err, "gather must succeed")
	for _, family := range families {
		if family.GetName() == familyName {
			return len(family.GetMetric())
		}
	}
	return 0
}

// sampleValue returns the value of the series in familyName whose labels match
// wanted. Only labels present in wanted are compared, so a call site states
// exactly the labels it cares about, and a counter and a gauge are both
// supported (the scrape is the same source an operator reads).
func sampleValue(t *testing.T, gatherer prometheus.Gatherer, familyName string, wanted map[string]string) float64 {
	t.Helper()
	families, err := gatherer.Gather()
	require.NoError(t, err, "gather must succeed")

	for _, family := range families {
		if family.GetName() != familyName {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if !labelsMatch(labels, wanted) {
				continue
			}
			if metric.GetCounter() != nil {
				return metric.GetCounter().GetValue()
			}
			if metric.GetGauge() != nil {
				return metric.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("%s has no series with labels %v", familyName, wanted)
	return 0
}

// labelsMatch reports whether every label in wanted has that value in have.
func labelsMatch(have, wanted map[string]string) bool {
	for key, value := range wanted {
		if have[key] != value {
			return false
		}
	}
	return true
}

// TestMusterMetrics_ExposesAllFiveFamilies is the acceptance test for
// OBS-MUSTER-1: the explicit registry must export all five spec §3.1 families
// on a freshly constructed instance, before any traffic is recorded.
func TestMusterMetrics_ExposesAllFiveFamilies(t *testing.T) {
	m := NewMusterMetrics("tenant-a", "v1.2.3", "abc123")
	require.NotNil(t, m)

	require.Empty(t, missingFamilies(t, m.Registry(), musterFamilies),
		"all five muster_* families must be exported by the explicit registry")

	// And nothing else: this registry is deliberately not the default
	// registry, so promauto families must not leak onto the scrape target.
	for name := range gatherFamilyNames(t, m.Registry()) {
		require.True(t, strings.HasPrefix(name, MusterNamespace+"_"),
			"unexpected non-muster family %q exported by the explicit registry", name)
	}
}

// TestMissingFamilies_NegativeControl proves the assertion above can fail: a
// registry holding four of the five families must be reported as missing the
// fifth. Without this, "all five present" could pass vacuously.
func TestMissingFamilies_NegativeControl(t *testing.T) {
	partial := prometheus.NewRegistry()
	partial.MustRegister(
		prometheus.NewGauge(prometheus.GaugeOpts{Namespace: MusterNamespace, Name: "build_info", Help: "x"}),
		prometheus.NewGauge(prometheus.GaugeOpts{Namespace: MusterNamespace, Name: "daemon_up", Help: "x"}),
		prometheus.NewGauge(prometheus.GaugeOpts{Namespace: MusterNamespace, Name: "daemon_proxy_requests_total", Help: "x"}),
		prometheus.NewGauge(prometheus.GaugeOpts{Namespace: MusterNamespace, Name: "daemon_proxy_duration_seconds", Help: "x"}),
	)

	require.Equal(t, []string{"muster_audit_events_written_total"},
		missingFamilies(t, partial, musterFamilies),
		"the presence check must identify exactly the absent family")
}

// TestMusterMetrics_BuildInfoIsOne checks the identity metric carries the
// running build's version and commit and reads 1.
func TestMusterMetrics_BuildInfoIsOne(t *testing.T) {
	m := NewMusterMetrics("", "v9.9.9", "deadbeef")

	require.Equal(t, 1.0,
		sampleValue(t, m.Registry(), "muster_build_info", map[string]string{"version": "v9.9.9", "commit": "deadbeef"}),
		"build_info must always be 1 for the running version and commit")

	require.Equal(t, 1, seriesCount(t, m.Registry(), "muster_build_info"),
		"build_info must be exactly one series")
}

// TestMusterMetrics_DaemonUpLifecycle checks daemon_up reports "not answering
// yet" at construction, 1 while serving and 0 again on shutdown.
func TestMusterMetrics_DaemonUpLifecycle(t *testing.T) {
	m := NewMusterMetrics("tenant-a", "dev", "unknown")
	up := map[string]string{"tenant_id": "tenant-a"}

	require.Equal(t, 0.0, sampleValue(t, m.Registry(), "muster_daemon_up", up),
		"daemon_up must start at 0 so a startup scrape cannot claim readiness")

	m.MarkUp()
	require.Equal(t, 1.0, sampleValue(t, m.Registry(), "muster_daemon_up", up))

	m.MarkDown()
	require.Equal(t, 0.0, sampleValue(t, m.Registry(), "muster_daemon_up", up),
		"daemon_up must drop to 0 on the shutdown path")
}

// TestMusterMetrics_RecordProxyRequest checks the counter and histogram land on
// the explicit registry with bounded labels, including the status class.
func TestMusterMetrics_RecordProxyRequest(t *testing.T) {
	m := NewMusterMetrics("tenant-a", "dev", "unknown")

	m.RecordProxyRequest("/projects/123", "GET", http.StatusTeapot, 250*time.Millisecond)
	m.RecordProxyRequest("/projects/123", "GET", http.StatusOK, 50*time.Millisecond)

	require.Equal(t, 1.0, sampleValue(t, m.Registry(), "muster_daemon_proxy_requests_total", map[string]string{
		"endpoint": "/projects/{id}", "method": "GET", "status_class": "4xx", "tenant_id": "tenant-a",
	}), "the 4xx request must land in the status_class=4xx series")

	require.Equal(t, 1.0, sampleValue(t, m.Registry(), "muster_daemon_proxy_requests_total", map[string]string{
		"endpoint": "/projects/{id}", "method": "GET", "status_class": "2xx", "tenant_id": "tenant-a",
	}), "the 2xx request must land in the status_class=2xx series")

	require.Greater(t, seriesCount(t, m.Registry(), "muster_daemon_proxy_duration_seconds"), 0,
		"the duration histogram must export series")
}

// TestMusterMetrics_AuditSeedIsPresentEvenWithoutCallers checks the audit
// counter is exported at 0 before any event exists — it must still appear with
// value 0 (no daemon-side audit call site yet; see the TODO in muster.go).
func TestMusterMetrics_AuditSeedIsPresentEvenWithoutCallers(t *testing.T) {
	m := NewMusterMetrics("", "dev", "unknown")

	require.Equal(t, 0.0, sampleValue(t, m.Registry(), "muster_audit_events_written_total", map[string]string{
		"event_type": auditSeedEventType, "category": auditSeedCategory,
	}), "the audit seed series must exist at 0 before any event is written")

	m.RecordAuditEvent("tool_call", "mcp")
	require.Equal(t, 1.0, sampleValue(t, m.Registry(), "muster_audit_events_written_total", map[string]string{
		"event_type": "tool_call", "category": "mcp",
	}))
}

// TestStatusClass_Bounded is the cardinality guard: status_class must be one of
// a fixed set, never the raw code.
func TestStatusClass_Bounded(t *testing.T) {
	cases := map[int]string{
		100: "1xx", 204: "2xx", 301: "3xx", 404: "4xx", 503: "5xx",
		0: "unknown", 99: "unknown", 600: "unknown",
	}
	for status, want := range cases {
		require.Equal(t, want, StatusClass(status), "status %d", status)
	}
}

// TestNormalizeEndpoint_CollapsesIdentifiers checks the endpoint label stays
// bounded when the upstream path carries resource ids.
func TestNormalizeEndpoint_CollapsesIdentifiers(t *testing.T) {
	cases := map[string]string{
		"":               "/",
		"/":              "/",
		"/projects/123/": "/projects/{id}",
		"/runs/9f2c1a3e-1111-2222-3333-444455556666/status": "/runs/{id}/status",
		"/pets/fluffy":                     "/pets/fluffy",
		"/pets/" + strings.Repeat("a", 41): "/pets/{id}",
	}
	for input, want := range cases {
		require.Equal(t, want, NormalizeEndpoint(input), "input %q", input)
	}
}

// TestValidateLoopbackAddr checks the listener refuses to bind anything that
// is not loopback (spec §4.2: "Bind loopback only").
func TestValidateLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:9110", "localhost:9110", "[::1]:9110"} {
		require.NoError(t, ValidateLoopbackAddr(addr), "addr %q must be accepted", addr)
	}
	for _, addr := range []string{"0.0.0.0:9110", ":9110", "8.8.8.8:9110", "127.0.0.1", "not-an-ip:9110"} {
		require.Error(t, ValidateLoopbackAddr(addr), "addr %q must be refused", addr)
	}
}

// TestMusterMetricsServer_ServesOnlyMetrics drives the real listener: it must
// serve the five families on /metrics and nothing else.
func TestMusterMetricsServer_ServesOnlyMetrics(t *testing.T) {
	m := NewMusterMetrics("tenant-a", "v1.0.0", "cafe")
	m.MarkUp()

	server := NewMusterMetricsServer("127.0.0.1:0", m)
	require.NoError(t, server.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	base := "http://" + server.Addr()

	body := httpGet(t, base+"/metrics")
	for _, name := range musterFamilies {
		require.Contains(t, body, name, "metric family %q must be on the endpoint", name)
	}

	// Only /metrics is routed: a proxied path must not be reachable through
	// the metrics port (spec §4.2 — a traversal cannot reach the proxy here).
	resp, err := http.Get(base + "/pets/1")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode,
		"the metrics listener must not serve anything but /metrics")
}

func httpGet(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", url)
	return string(body)
}
