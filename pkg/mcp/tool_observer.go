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

// The error-class label set is a closed enum: internal/monitoring's
// NormalizeMCPErrorClass normalizes anything else to internal at the metric
// sink. These constants are the observer-side spellings of that enum.
const (
	errorClassNone       = "none"
	errorClassAuth       = "auth"
	errorClassUpstream   = "upstream"
	errorClassValidation = "validation"
	errorClassInternal   = "internal"
)

// jsonRPCCodeInvalidParams is the JSON-RPC 2.0 "Invalid params" error code the
// tools/call handler returns before an executor runs.
const jsonRPCCodeInvalidParams = -32602

// jsonRPCCodeUnauthorized is the server-range JSON-RPC error code (-32000 ..
// -32099 are implementation reserved) muster reserves for authentication
// failures. No handler returns it today; the observer classifies it as auth so
// an auth-class protocol failure can never be mislabelled internal if one is
// introduced. Declaring it here changes no response bytes.
const jsonRPCCodeUnauthorized = -32001

// ObserveToolCall records one terminal tool outcome. The error label is a
// closed enum and never contains the error text.
func (o *MCPToolObserver) ObserveToolCall(ctx context.Context, tool string, success bool, err error, duration time.Duration) {
	if o == nil {
		return
	}
	status := "success"
	errorClass := errorClassNone
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
// Classification order: a handler-declared marker (OBS-MUSTER-008) wins, then
// the JSON-RPC protocol error codes, then muster's typed AppError codes.
func mcpErrorClass(err error) string {
	if err == nil {
		return errorClassInternal
	}
	var classErr *toolResultClassError
	if stderrors.As(err, &classErr) {
		return classErr.metricClass()
	}
	var mcpErr *handlers.MCPError
	if stderrors.As(err, &mcpErr) {
		switch mcpErr.Code {
		case jsonRPCCodeInvalidParams:
			return errorClassValidation
		case jsonRPCCodeUnauthorized:
			return errorClassAuth
		}
		return errorClassInternal
	}
	if apperrors.IsError(err, apperrors.CodeValidation) || apperrors.IsError(err, apperrors.CodeBadRequest) {
		return errorClassValidation
	}
	if apperrors.IsError(err, apperrors.CodeUnauthorized) {
		return errorClassAuth
	}
	if apperrors.IsError(err, apperrors.CodeTimeout) || apperrors.IsError(err, apperrors.CodeRateLimit) {
		return errorClassUpstream
	}
	return errorClassInternal
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

// toolResultClassError carries the class and reason code a failed tool
// result's structured marker declared (DF-038), so mcpErrorClass can label the
// metric from the handler's own classification instead of guessing from an
// error string. It never embeds the upstream error text.
type toolResultClassError struct {
	class string
	code  string
}

func (e *toolResultClassError) Error() string {
	return "tool returned " + e.class + " error"
}

// metricClass maps the handler-declared marker onto the closed metric enum.
func (e *toolResultClassError) metricClass() string {
	switch e.class {
	case handlers.ErrorClassUpstream:
		return errorClassUpstream
	case handlers.ErrorClassClient:
		switch e.code {
		case handlers.ReasonCredentialRejected, handlers.ReasonMissingCredential:
			// Both 401/403 outcomes are auth-layer failures for the metric
			// even though the agent-facing marker calls them client-side.
			return errorClassAuth
		default:
			return errorClassValidation
		}
	default:
		// handlers.ErrorClassUnclassified and anything unknown stay internal:
		// the enum has no "unclassified" label.
		return errorClassInternal
	}
}

// toolResultError inspects a tool result that carried isError=true and returns
// the failure the handler's structured marker declared. A result without a
// class marker yields a class-less error, which mcpErrorClass maps to the
// closed enum's internal value.
func toolResultError(result interface{}) error {
	encoded, err := json.Marshal(result)
	if err == nil {
		var envelope struct {
			StructuredContent map[string]interface{} `json:"structuredContent"`
		}
		if json.Unmarshal(encoded, &envelope) == nil {
			if details, ok := envelope.StructuredContent["error"].(map[string]interface{}); ok {
				if class, ok := details["class"].(string); ok && class != "" {
					code, _ := details["code"].(string)
					return &toolResultClassError{class: class, code: code}
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
