package mcp

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"time"

	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/monitoring"
	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

// JSONRPCRequest is declared in server.go; this file only adds the observer
// implementation and dispatch helper.

// ToolCallObserver records MCP tools at the dispatch boundary without changing
// the JSON-RPC protocol response.
type ToolCallObserver interface {
	ObserveToolCall(ctx context.Context, tool string, success bool, err error, duration time.Duration)
}

// MCPToolObserver combines the production metrics and audit sinks.
type MCPToolObserver struct {
	metrics *monitoring.MetricRecorder
	audit   *audit.Hook
	actorID string
}

// NewMCPToolObserver creates the production observer used by cmd/mcp-server.
func NewMCPToolObserver(metrics *monitoring.MetricRecorder, auditHook *audit.Hook, actorID string) *MCPToolObserver {
	return &MCPToolObserver{metrics: metrics, audit: auditHook, actorID: actorID}
}

// ObserveToolCall records one terminal tool outcome. The error label is a
// closed enum and never contains the error text.
func (o *MCPToolObserver) ObserveToolCall(ctx context.Context, tool string, success bool, err error, duration time.Duration) {
	if o == nil {
		return
	}
	status := "success"
	errorClass := "none"
	if !success {
		status = "error"
		errorClass = mcpErrorClass(err)
	}
	if o.metrics != nil {
		o.metrics.RecordMCPTool(tool, status, errorClass, duration)
	}
	if o.audit != nil {
		details := map[string]string{"status": status}
		if !success {
			details["error_class"] = errorClass
		}
		o.audit.LogToolCall(ctx, tool, o.actorID, success, details)
	}
}

// mcpErrorClass maps protocol/transport failures to the spec's closed enum.
func mcpErrorClass(err error) string {
	if err == nil {
		return "internal"
	}
	var mcpErr *handlers.MCPError
	if stderrors.As(err, &mcpErr) {
		if mcpErr.Code == -32602 {
			return "validation"
		}
		return "internal"
	}
	if apperrors.IsError(err, apperrors.CodeValidation) || apperrors.IsError(err, apperrors.CodeBadRequest) {
		return "validation"
	}
	if apperrors.IsError(err, apperrors.CodeUnauthorized) {
		return "auth"
	}
	if apperrors.IsError(err, apperrors.CodeTimeout) || apperrors.IsError(err, apperrors.CodeRateLimit) {
		return "upstream"
	}
	return "internal"
}

func observeToolCall(observer ToolCallObserver, ctx context.Context, req *JSONRPCRequest, result interface{}, err error, started time.Time) {
	if observer == nil || req.Method != "tools/call" {
		return
	}
	tool := toolNameFromParams(req.Params)
	success := err == nil && !callResultIsError(result)
	observationErr := err
	if observationErr == nil && !success {
		observationErr = toolResultError(result)
	}
	observer.ObserveToolCall(ctx, tool, success, observationErr, time.Since(started))
}

func callResultIsError(result interface{}) bool {
	encoded, err := json.Marshal(result)
	if err != nil {
		return false
	}
	var envelope struct {
		IsError bool `json:"isError"`
	}
	return json.Unmarshal(encoded, &envelope) == nil && envelope.IsError
}

func toolResultError(result interface{}) error {
	encoded, err := json.Marshal(result)
	if err == nil {
		var envelope struct {
			StructuredContent map[string]interface{} `json:"structuredContent"`
		}
		if json.Unmarshal(encoded, &envelope) == nil {
			if details, ok := envelope.StructuredContent["error"].(map[string]interface{}); ok {
				if class, ok := details["class"].(string); ok {
					code := apperrors.CodeInternal
					if class == handlers.ErrorClassClient {
						code = apperrors.CodeValidation
					}
					return apperrors.New(code, "tool returned "+class+" error")
				}
			}
		}
	}
	return stderrors.New("tool returned an error result")
}

func toolNameFromParams(params json.RawMessage) string {
	var call struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return "unknown"
	}
	return boundedToolLabel(call.Name)
}

// maxToolLabelLen bounds the tool label length. The real tool set is bounded by
// the served spec's operation count (spec §3.2), but the value arrives from the
// client, so anything that is not a plausible tool identifier collapses to
// "unknown" instead of creating a new time series.
const maxToolLabelLen = 64

// boundedToolLabel bounds the tool label so a hostile or buggy client cannot
// explode cardinality or smuggle control characters into the exposition. Real
// tool names are OpenAPI operation ids or path-shaped names, so printable ASCII
// is allowed and everything else collapses to "unknown".
func boundedToolLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxToolLabelLen {
		return "unknown"
	}
	for _, r := range name {
		if r < 0x20 || r > 0x7e {
			return "unknown"
		}
	}
	return name
}

var _ ToolCallObserver = (*MCPToolObserver)(nil)
