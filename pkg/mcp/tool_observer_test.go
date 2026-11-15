package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/monitoring"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

func TestServerToolObserver_RecordsSuccessAndBadParams(t *testing.T) {
	metrics := monitoring.NewMetricRecorder("test", nil)
	auditPath := t.TempDir() + "/audit.log"
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", auditPath)
	hook := audit.NewHook(audit.DefaultConfigEnabled())
	observer := NewMCPToolObserver(metrics, hook, "test-client")

	server := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	server.SetToolCallObserver(observer)
	server.RegisterHandler(handlers.NewCallToolHandler(testExecutor{}))

	server.handleRequest(context.Background(), &JSONRPCRequest{ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"echo","arguments":{"value":"ok"}}`)})
	server.handleRequest(context.Background(), &JSONRPCRequest{ID: 2, Method: "tools/call", Params: json.RawMessage(`{"name":"echo"}`)})
	if err := hook.Close(); err != nil {
		t.Fatalf("close audit hook: %v", err)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "echo", "status": "success", "error_class": "none"}); got != 1 {
		t.Fatalf("success count = %v, want 1", got)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "echo", "status": "error", "error_class": "validation"}); got != 1 {
		t.Fatalf("error count = %v, want 1", got)
	}
	if got := histogramCount(families, "muster_mcp_tool_duration_seconds", map[string]string{"tool": "echo", "status": "success"}); got != 1 {
		t.Fatalf("success duration count = %d, want 1", got)
	}
	if got := histogramCount(families, "muster_mcp_tool_duration_seconds", map[string]string{"tool": "echo", "status": "error"}); got != 1 {
		t.Fatalf("error duration count = %d, want 1", got)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if got := strings.Count(string(data), `"event_type":"tool.call"`); got != 2 {
		t.Fatalf("tool.call events = %d, want 2\n%s", got, data)
	}
}

func TestMCPErrorClass_UnknownIsClosed(t *testing.T) {
	class := mcpErrorClass(errors.New("arbitrary raw failure with user input"))
	if class != "internal" {
		t.Fatalf("unknown error class = %q, want internal", class)
	}
	for _, allowed := range []string{"auth", "upstream", "validation", "internal"} {
		if class == allowed {
			return
		}
	}
	t.Fatalf("class %q is not closed enum", class)
}

func TestToolLabel_RejectsArbitraryClientText(t *testing.T) {
	cases := map[string]string{
		`{"name":"getPet"}`:                          "getPet",
		`{"name":"/pets/{id}/things"}`:               "/pets/{id}/things",
		`{"name":""}`:                                "unknown",
		`{"name":"   "}`:                             "unknown",
		`{"name":"pets\u0000\u0001"}`:                "unknown",
		`{"name":"tool with spaces and {braces}"}`:   "tool with spaces and {braces}",
		`{"name":"tool ünïcode"}`:                    "unknown",
		`{"name":"` + strings.Repeat("x", 65) + `"}`: "unknown",
		`not json at all`:                            "unknown",
	}
	for params, want := range cases {
		if got := toolNameFromParams(json.RawMessage(params)); got != want {
			t.Errorf("toolNameFromParams(%s) = %q, want %q", params, got, want)
		}
	}
	// A raw error string must never become a label value either.
	if got := mcpErrorClass(errors.New(`upstream 500: {"secret":"abc"}`)); got != "internal" {
		t.Fatalf("raw upstream text leaked as class %q", got)
	}
}

func TestServerToolObserver_LeavesProtocolResponseUnchanged(t *testing.T) {
	call := &JSONRPCRequest{ID: 7, Method: "tools/call", Params: json.RawMessage(`{"name":"echo","arguments":{"value":"ok"}}`)}

	plain := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	plain.RegisterHandler(handlers.NewCallToolHandler(testExecutor{}))
	plainOut := captureResponse(t, plain, call)

	observed := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	metrics := monitoring.NewMetricRecorder("test", nil)
	observed.SetToolCallObserver(NewMCPToolObserver(metrics, nil, "test-client"))
	observed.RegisterHandler(handlers.NewCallToolHandler(testExecutor{}))
	observedOut := captureResponse(t, observed, call)

	if plainOut != observedOut {
		t.Fatalf("observer changed the JSON-RPC response:\n without: %s\n    with: %s", plainOut, observedOut)
	}
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "echo", "status": "success"}); got != 1 {
		t.Fatalf("observed success count = %v, want 1", got)
	}
}

// captureResponse runs one request through a server and returns the framed
// JSON-RPC reply the server queued for its stdout writer.
func captureResponse(t *testing.T, server *Server, req *JSONRPCRequest) string {
	t.Helper()
	server.handleRequest(context.Background(), req)
	select {
	case data := <-server.writeCh:
		return string(data)
	default:
		t.Fatalf("server queued no response for %s", req.Method)
		return ""
	}
}

func TestToolObserver_IgnoresNonToolRequests(t *testing.T) {
	metrics := monitoring.NewMetricRecorder("test", nil)
	observer := NewMCPToolObserver(metrics, nil, "test-client")
	observeToolCall(observer, context.Background(), &JSONRPCRequest{ID: 1, Method: "tools/list"}, nil, nil, time.Now())
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"status": "success"}); got != 0 {
		t.Fatalf("tools/list recorded as a tool execution (%v)", got)
	}
}

type testExecutor struct{}

func (testExecutor) Execute(_ context.Context, _ string, args map[string]interface{}) (interface{}, error) {
	if args == nil {
		return nil, handlers.NewToolCallError(handlers.ErrorClassClient, handlers.ReasonInvalidArguments, "missing arguments")
	}
	return map[string]interface{}{"ok": true}, nil
}

func counterValue(families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelsMatch(metric, labels) && metric.GetCounter() != nil {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func histogramCount(families []*dto.MetricFamily, name string, labels map[string]string) uint64 {
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelsMatch(metric, labels) && metric.GetHistogram() != nil {
				return metric.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func metricLabelsMatch(metric *dto.Metric, wanted map[string]string) bool {
	for key, value := range wanted {
		found := false
		for _, pair := range metric.GetLabel() {
			if pair.GetName() == key && pair.GetValue() == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
