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
	apperrors "github.com/wojons/muster/pkg/errors"
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

// classifiedMarkerResult builds the isError tool result shape the CallTool
// handler renders for a classified executor failure (DF-038).
func classifiedMarkerResult(class, code string) *handlers.CallToolResult {
	return &handlers.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]interface{}{"error": map[string]interface{}{"class": class, "code": code}},
	}
}

// TestMCPErrorClass_HandlerDeclaredClasses proves each arm of the closed enum:
// a handler-declared upstream failure is upstream, a credential failure is
// auth, a client argument failure is validation, and the unclassified marker
// stays internal. Addresses OBS-MUSTER-008.
func TestMCPErrorClass_HandlerDeclaredClasses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"marker upstream http", toolResultError(classifiedMarkerResult(handlers.ErrorClassUpstream, handlers.ReasonUpstreamHTTP)), errorClassUpstream},
		{"marker upstream unreachable", toolResultError(classifiedMarkerResult(handlers.ErrorClassUpstream, handlers.ReasonUpstreamUnreachable)), errorClassUpstream},
		{"marker credential rejected", toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonCredentialRejected)), errorClassAuth},
		{"marker missing credential", toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonMissingCredential)), errorClassAuth},
		{"marker invalid arguments", toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonInvalidArguments)), errorClassValidation},
		{"marker unknown operation", toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonUnknownOperation)), errorClassValidation},
		{"marker unclassified", toolResultError(classifiedMarkerResult(handlers.ErrorClassUnclassified, handlers.ReasonUnclassified)), errorClassInternal},
		{"marker without class", toolResultError(&handlers.CallToolResult{IsError: true}), errorClassInternal},
		{"protocol invalid params", &handlers.MCPError{Code: jsonRPCCodeInvalidParams, Message: "Invalid params"}, errorClassValidation},
		{"protocol auth", &handlers.MCPError{Code: jsonRPCCodeUnauthorized, Message: "unauthorized"}, errorClassAuth},
		{"protocol internal error", &handlers.MCPError{Code: -32603, Message: "Tool executor not available"}, errorClassInternal},
		{"apperror timeout", apperrors.New(apperrors.CodeTimeout, "upstream timed out"), errorClassUpstream},
		{"apperror rate limit", apperrors.New(apperrors.CodeRateLimit, "upstream rate limited"), errorClassUpstream},
		{"apperror unauthorized", apperrors.New(apperrors.CodeUnauthorized, "credential rejected"), errorClassAuth},
		{"apperror validation", apperrors.New(apperrors.CodeValidation, "bad argument"), errorClassValidation},
	}
	for _, tc := range cases {
		if got := mcpErrorClass(tc.err); got != tc.want {
			t.Errorf("%s: mcpErrorClass = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestObserveToolCall_LabelSetIsClosedEnum drives every arm through the
// production observer and proves the recorded label set stays inside the
// closed enum and that no raw error text reaches any metric label.
func TestObserveToolCall_LabelSetIsClosedEnum(t *testing.T) {
	metrics := monitoring.NewMetricRecorder("test", nil)
	observer := NewMCPToolObserver(metrics, nil, "test-client")
	hostile := `upstream 500: {"secret":"secret-token-abc"}`
	arms := []error{
		toolResultError(classifiedMarkerResult(handlers.ErrorClassUpstream, handlers.ReasonUpstreamHTTP)),
		toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonCredentialRejected)),
		toolResultError(classifiedMarkerResult(handlers.ErrorClassClient, handlers.ReasonInvalidArguments)),
		toolResultError(classifiedMarkerResult(handlers.ErrorClassUnclassified, handlers.ReasonUnclassified)),
		&handlers.MCPError{Code: -32602, Message: hostile},
		errors.New(hostile),
		nil,
	}
	for _, arm := range arms {
		observer.ObserveToolCall(context.Background(), "getPet", false, arm, time.Millisecond)
		observer.ObserveToolCall(context.Background(), "getPet", true, nil, time.Millisecond)
	}
	observer.ObserveToolCall(context.Background(), "getPet", false, errors.New(hostile), time.Millisecond)

	// The observer-side enum spellings must be exactly the sink's enum:
	// NormalizeMCPErrorClass must accept each and reject anything else.
	for _, class := range []string{errorClassAuth, errorClassUpstream, errorClassValidation, errorClassInternal} {
		if got := monitoring.NormalizeMCPErrorClass(class); got != class {
			t.Fatalf("observer enum value %q not accepted by the sink: got %q", class, got)
		}
	}
	if got := monitoring.NormalizeMCPErrorClass(errorClassNone); got != errorClassInternal {
		t.Fatalf("error_class %q is not an error-side value: got %q", errorClassNone, got)
	}

	allowed := map[string]bool{errorClassNone: true, errorClassAuth: true, errorClassUpstream: true, errorClassValidation: true, errorClassInternal: true}
	allowedStatus := map[string]bool{"success": true, "error": true}
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "muster_mcp_tool_executions_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, pair := range metric.GetLabel() {
				value := pair.GetValue()
				switch pair.GetName() {
				case "error_class":
					if !allowed[value] {
						t.Errorf("error_class label %q is outside the closed enum", value)
					}
				case "status":
					if !allowedStatus[value] {
						t.Errorf("status label %q is outside the closed enum", value)
					}
				}
				if strings.Contains(value, "secret-token-abc") || strings.Contains(value, "upstream 500") {
					t.Errorf("raw error text leaked into label %s=%q", pair.GetName(), value)
				}
			}
		}
	}
}

// registryMirrorExecutor mirrors ToolExecutor.Execute's classified failures so
// the server-level tests exercise the same marker shapes production renders.
type registryMirrorExecutor struct{}

func (registryMirrorExecutor) Execute(_ context.Context, name string, args map[string]interface{}) (interface{}, error) {
	if name != "echo" {
		return nil, handlers.NewToolCallError(
			handlers.ErrorClassClient, handlers.ReasonUnknownOperation,
			"tool \""+name+"\" not found in registry",
		).WithDetail("tool", name)
	}
	if args == nil {
		return nil, handlers.NewToolCallError(handlers.ErrorClassClient, handlers.ReasonInvalidArguments, "missing arguments")
	}
	return map[string]interface{}{"ok": true}, nil
}

// TestServerToolObserver_UnknownToolRecordedAsError proves a tools/call naming
// an unknown tool increments the error counter and writes a tool.call audit
// entry, while the JSON-RPC response bytes stay exactly what an unobserved
// server returns. Addresses OBS-MUSTER-008.
func TestServerToolObserver_UnknownToolRecordedAsError(t *testing.T) {
	call := &JSONRPCRequest{ID: 3, Method: "tools/call", Params: json.RawMessage(`{"name":"nosuchtool","arguments":{}}`)}

	plain := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	plain.RegisterHandler(handlers.NewCallToolHandler(registryMirrorExecutor{}))
	plainOut := captureResponse(t, plain, call)
	if !strings.Contains(plainOut, `"isError":true`) {
		t.Fatalf("unknown tool must stay a protocol-correct isError result, got %s", plainOut)
	}

	metrics := monitoring.NewMetricRecorder("test", nil)
	auditPath := t.TempDir() + "/audit.log"
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", auditPath)
	hook := audit.NewHook(audit.DefaultConfigEnabled())
	observed := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	observed.SetToolCallObserver(NewMCPToolObserver(metrics, hook, "test-client"))
	observed.RegisterHandler(handlers.NewCallToolHandler(registryMirrorExecutor{}))
	observedOut := captureResponse(t, observed, call)
	if plainOut != observedOut {
		t.Fatalf("observer changed the JSON-RPC response:\n without: %s\n    with: %s", plainOut, observedOut)
	}
	if err := hook.Close(); err != nil {
		t.Fatalf("close audit hook: %v", err)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "nosuchtool", "status": "error"}); got != 1 {
		t.Fatalf("unknown-tool error count = %v, want 1", got)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "nosuchtool", "status": "error", "error_class": "validation"}); got != 1 {
		t.Fatalf("unknown-tool validation count = %v, want 1", got)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if got := strings.Count(string(data), `"event_type":"tool.call"`); got != 1 {
		t.Fatalf("tool.call events = %d, want 1\n%s", got, data)
	}
}

// cannedExecutor always fails with the given classified error, mirroring how
// ToolExecutor.Execute surfaces upstream/credential failures through the
// CallTool handler's isError marker.
type cannedExecutor struct{ err error }

func (c cannedExecutor) Execute(context.Context, string, map[string]interface{}) (interface{}, error) {
	return nil, c.err
}

// TestServerToolObserver_UpstreamAndAuthClassesRecorded proves arms (a) and
// (b) end to end: a handler-declared upstream failure records error_class
// upstream and a credential rejection records error_class auth, both with
// status=error, through the real server dispatch. Addresses OBS-MUSTER-008.
func TestServerToolObserver_UpstreamAndAuthClassesRecorded(t *testing.T) {
	metrics := monitoring.NewMetricRecorder("test", nil)
	auditPath := t.TempDir() + "/audit.log"
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", auditPath)
	hook := audit.NewHook(audit.DefaultConfigEnabled())

	upstreamServer := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	upstreamServer.SetToolCallObserver(NewMCPToolObserver(metrics, hook, "test-client"))
	upstreamServer.RegisterHandler(handlers.NewCallToolHandler(cannedExecutor{
		err: handlers.NewToolCallError(handlers.ErrorClassUpstream, handlers.ReasonUpstreamHTTP, "HTTP 503: degraded").WithStatus(503),
	}))
	upstreamServer.handleRequest(context.Background(), &JSONRPCRequest{ID: 5, Method: "tools/call", Params: json.RawMessage(`{"name":"flaky","arguments":{}}`)})

	authServer := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	authServer.SetToolCallObserver(NewMCPToolObserver(metrics, hook, "test-client"))
	authServer.RegisterHandler(handlers.NewCallToolHandler(cannedExecutor{
		err: handlers.NewToolCallError(handlers.ErrorClassClient, handlers.ReasonCredentialRejected, "HTTP 401: the upstream rejected the bearer credential").WithStatus(401),
	}))
	authServer.handleRequest(context.Background(), &JSONRPCRequest{ID: 6, Method: "tools/call", Params: json.RawMessage(`{"name":"profile","arguments":{}}`)})

	if err := hook.Close(); err != nil {
		t.Fatalf("close audit hook: %v", err)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "flaky", "status": "error", "error_class": "upstream"}); got != 1 {
		t.Fatalf("upstream error count = %v, want 1", got)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "profile", "status": "error", "error_class": "auth"}); got != 1 {
		t.Fatalf("auth error count = %v, want 1", got)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if got := strings.Count(string(data), `"event_type":"tool.call"`); got != 2 {
		t.Fatalf("tool.call events = %d, want 2\n%s", got, data)
	}
	if !strings.Contains(string(data), `"error_class":"upstream"`) || !strings.Contains(string(data), `"error_class":"auth"`) {
		t.Fatalf("audit entries missing the closed-enum classes:\n%s", data)
	}
}

// TestServerToolObserver_BadParamsRecordedAsError proves the tools/call
// handler's early return (unparseable params, before the executor runs) is
// still observed: the -32602 protocol failure records an error execution
// against the bounded "unknown" tool label. Addresses OBS-MUSTER-008.
func TestServerToolObserver_BadParamsRecordedAsError(t *testing.T) {
	metrics := monitoring.NewMetricRecorder("test", nil)
	auditPath := t.TempDir() + "/audit.log"
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", auditPath)
	hook := audit.NewHook(audit.DefaultConfigEnabled())
	server := NewServer(strings.NewReader(""), io.Discard, io.Discard)
	server.SetToolCallObserver(NewMCPToolObserver(metrics, hook, "test-client"))
	server.RegisterHandler(handlers.NewCallToolHandler(registryMirrorExecutor{}))
	server.handleRequest(context.Background(), &JSONRPCRequest{ID: 4, Method: "tools/call", Params: json.RawMessage(`not json`)})
	if err := hook.Close(); err != nil {
		t.Fatalf("close audit hook: %v", err)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if got := counterValue(families, "muster_mcp_tool_executions_total", map[string]string{"tool": "unknown", "status": "error", "error_class": "validation"}); got != 1 {
		t.Fatalf("bad-params error count = %v, want 1", got)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if got := strings.Count(string(data), `"event_type":"tool.call"`); got != 1 {
		t.Fatalf("tool.call events = %d, want 1\n%s", got, data)
	}
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
	// A raw error string must never become a label value either. It carries no
	// class information, so it falls to the closed enum's designated internal
	// value (the pre-008 test asserted that hardcode as if it were the
	// upstream behaviour; the enum fallback is what this asserts now).
	if got := mcpErrorClass(errors.New(`upstream 500: {"secret":"abc"}`)); got != errorClassInternal {
		t.Fatalf("class-less raw error = %q, want closed-enum %q", got, errorClassInternal)
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
