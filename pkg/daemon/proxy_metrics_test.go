package daemon

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recordedProxyCall is one observation the handler reported.
type recordedProxyCall struct {
	endpoint string
	method   string
	status   int
	duration time.Duration
}

// recordingMetrics is a ProxyMetrics stub that keeps every observation, so a
// test can assert on the label values (and on the count) rather than trusting
// that "something" was recorded.
type recordingMetrics struct {
	mu    sync.Mutex
	calls []recordedProxyCall
}

func (r *recordingMetrics) RecordDaemonProxyRequest(endpoint, method string, status int, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedProxyCall{
		endpoint: endpoint,
		method:   method,
		status:   status,
		duration: duration,
	})
}

func (r *recordingMetrics) snapshot() []recordedProxyCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedProxyCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// newProxyTestBackend starts an upstream server whose status and body are fixed
// by the test.
func newProxyTestBackend(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestProxyHandler_RecordsProxyMetrics is the OBS-MUSTER-1 acceptance test for
// the daemon side: one proxied call must produce exactly one observation, with
// the request path, the method and the status the caller actually received.
func TestProxyHandler_RecordsProxyMetrics(t *testing.T) {
	backend := newProxyTestBackend(t, http.StatusCreated, `{"id":7}`)

	metrics := &recordingMetrics{}
	handler := NewProxyHandler(backend.URL)
	handler.SetMetrics(metrics)

	req := httptest.NewRequest(http.MethodGet, "/projects/7", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "the proxy must forward the upstream status")
	require.Equal(t, `{"id":7}`, rec.Body.String(), "wrapping the writer must not alter the body")

	calls := metrics.snapshot()
	require.Len(t, calls, 1, "exactly one observation per proxied request")
	require.Equal(t, "/projects/7", calls[0].endpoint)
	require.Equal(t, http.MethodGet, calls[0].method)
	require.Equal(t, http.StatusCreated, calls[0].status)
	require.GreaterOrEqual(t, calls[0].duration, time.Duration(0))
}

// TestProxyHandler_RecordsUpstreamError checks a failing upstream is counted
// with its real status — the 5xx rate is the metric an operator alerts on.
func TestProxyHandler_RecordsUpstreamError(t *testing.T) {
	backend := newProxyTestBackend(t, http.StatusInternalServerError, `{"error":"boom"}`)

	metrics := &recordingMetrics{}
	handler := NewProxyHandler(backend.URL)
	handler.SetMetrics(metrics)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	calls := metrics.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, "/orders", calls[0].endpoint)
	require.Equal(t, http.MethodPost, calls[0].method)
	require.Equal(t, http.StatusInternalServerError, calls[0].status)
}

// TestProxyHandler_RecordsRejectedPath checks a request the proxy refuses is
// still measured, so failures cannot hide in silence.
func TestProxyHandler_RecordsRejectedPath(t *testing.T) {
	backend := newProxyTestBackend(t, http.StatusOK, `{}`)

	metrics := &recordingMetrics{}
	handler := NewProxyHandler(backend.URL)
	handler.SetMetrics(metrics)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../etc/passwd", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	calls := metrics.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, http.StatusBadRequest, calls[0].status)
	require.Empty(t, calls[0].endpoint,
		"a rejected path must not become an endpoint label; the recorder maps it to a bounded value")
}

// TestProxyHandler_NoMetricsIsUnchanged checks instrumentation is opt-in: the
// default handler still proxies, and nothing panics on a nil recorder.
func TestProxyHandler_NoMetricsIsUnchanged(t *testing.T) {
	backend := newProxyTestBackend(t, http.StatusOK, `{"ok":true}`)

	handler := NewProxyHandler(backend.URL)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, `{"ok":true}`, rec.Body.String())
}

// TestProxyHandler_ImplicitStatusIsRecorded checks a handler that writes a body
// without calling WriteHeader is still reported as 200, not as status 0.
func TestProxyHandler_ImplicitStatusIsRecorded(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder()}

	_, err := recorder.Write([]byte("body"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Status())
}

// TestStatusRecorder_WriteHeaderAfterWriteKeepsFirstStatus checks the wrapper
// reports the status net/http actually sends: the FIRST WriteHeader wins.
func TestStatusRecorder_WriteHeaderAfterWriteKeepsFirstStatus(t *testing.T) {
	inner := httptest.NewRecorder()
	recorder := &statusRecorder{ResponseWriter: inner}

	_, err := recorder.Write([]byte("body"))
	require.NoError(t, err)
	recorder.WriteHeader(http.StatusTeapot) // ignored by net/http too

	require.Equal(t, http.StatusOK, recorder.Status())
	require.Equal(t, http.StatusOK, inner.Code)
}
