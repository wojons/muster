package handlers

import (
	"encoding/json"
	"errors"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// DF-038. muster's audience is the agent as consumer, so a tools/call failure
// must be actionable without parsing prose: which side failed (client-side
// wiring vs the upstream API) and why (a stable reason code). The marker is
// rendered into the tool result's structuredContent under "error". The
// human-readable text content, and the isError flag semantics, are unchanged.

// Error classes reported in the marker's "class" field.
const (
	// ErrorClassClient marks a failure on this side of the call: invalid
	// arguments, a missing or rejected credential, or an operation that cannot
	// be resolved. Retrying the identical call cannot fix it — the caller must
	// change the request or the configuration.
	ErrorClassClient = "client_error"
	// ErrorClassUpstream marks a failure reported by (or reached through) the
	// upstream API: a non-2xx status, a transport failure, an upstream rate
	// limit.
	ErrorClassUpstream = "upstream_error"
	// ErrorClassUnclassified marks a failure whose class the executor did not
	// state. It is reported explicitly rather than guessed at, so an agent
	// never infers "upstream" from the absence of a marker.
	ErrorClassUnclassified = "tool_error"
)

// Reason codes reported in the marker's "code" field.
const (
	// ReasonInvalidArguments: the arguments failed schema validation or could
	// not be encoded into a request.
	ReasonInvalidArguments = "invalid_arguments"
	// ReasonUnknownOperation: no tool/operation matches the requested name.
	ReasonUnknownOperation = "unknown_operation"
	// ReasonMissingCredential: the upstream refused the request as
	// unauthenticated and no credential is configured; the marker names the
	// mechanism the credential was expected from.
	ReasonMissingCredential = "missing_credential"
	// ReasonCredentialRejected: a credential was configured and the upstream
	// rejected it; the marker names the source it came from (never its value).
	ReasonCredentialRejected = "credential_rejected"
	// ReasonUpstreamHTTP: the upstream answered with an error status.
	ReasonUpstreamHTTP = "upstream_http_error"
	// ReasonUpstreamUnreachable: the request never produced an upstream
	// response (connection refused, timeout, context cancellation).
	ReasonUpstreamUnreachable = "upstream_unreachable"
	// ReasonUpstreamError: a non-HTTP upstream failure (database, internal).
	ReasonUpstreamError = "upstream_error"
	// ReasonUnclassified: the executor returned a plain error.
	ReasonUnclassified = "unclassified"
)

// ToolErrorClassifier is implemented by an execution error that carries
// machine-readable classification metadata. It is duck-typed, like the executor
// interface NewCallToolHandler already accepts, so a protocol adapter outside
// this package can classify its own failures without depending on a concrete
// error type. The returned map is merged into the tool result's
// structuredContent under "error"; the keys muster's own executors use are
// class, code, message, status, credentialSource, upstreamBody and details.
type ToolErrorClassifier interface {
	ToolErrorMeta() map[string]interface{}
}

// StructuredResultProvider is implemented by an execution result that knows
// whether its value came from a structured (JSON) upstream payload. An executor
// that received a non-JSON body implements it and reports false, so the handler
// keeps the payload in the text content block rather than advertising a
// flattened blob as structured content.
type StructuredResultProvider interface {
	StructuredPayload() (value interface{}, structured bool)
}

// ToolCallError is a classified tools/call failure returned by an executor. Its
// Error() text is what the text content block carries (existing consumers keep
// working unchanged); its metadata is what the structured marker renders.
type ToolCallError struct {
	class            string
	code             string
	message          string
	status           int
	credentialSource string
	upstreamBody     interface{}
	details          map[string]interface{}
	cause            error
}

// NewToolCallError creates a classified execution failure. class and code
// should be one of the ErrorClass*/Reason* values above; message is the
// human-readable text the tool result carries.
func NewToolCallError(class, code, message string) *ToolCallError {
	return &ToolCallError{class: class, code: code, message: message}
}

// WithStatus records the upstream HTTP status code, when there was one.
func (e *ToolCallError) WithStatus(status int) *ToolCallError {
	e.status = status
	return e
}

// WithCredentialSource records where the outbound credential came from, or —
// when none is configured — the mechanisms it was expected from. It must never
// contain the credential value itself.
func (e *ToolCallError) WithCredentialSource(source string) *ToolCallError {
	e.credentialSource = source
	return e
}

// WithUpstreamBody records the upstream response body, parsed as JSON when it
// was JSON, so the agent receives the upstream's own structure instead of a
// stringified copy. A nil body is ignored.
func (e *ToolCallError) WithUpstreamBody(body interface{}) *ToolCallError {
	if body != nil {
		e.upstreamBody = body
	}
	return e
}

// WithDetail records one extra machine-readable fact (for example the tool
// name). A nil value is ignored so the marker never carries nulls.
func (e *ToolCallError) WithDetail(key string, value interface{}) *ToolCallError {
	if key == "" || value == nil {
		return e
	}
	if e.details == nil {
		e.details = make(map[string]interface{})
	}
	e.details[key] = value
	return e
}

// WithCause attaches the underlying error so errors.Is/errors.As keep working
// through the classification.
func (e *ToolCallError) WithCause(err error) *ToolCallError {
	e.cause = err
	return e
}

// Error returns the human-readable message, which is what the text content
// block carries.
func (e *ToolCallError) Error() string { return e.message }

// Unwrap exposes the underlying error, if any.
func (e *ToolCallError) Unwrap() error { return e.cause }

// ToolErrorMeta renders the machine-readable marker for this failure.
func (e *ToolCallError) ToolErrorMeta() map[string]interface{} {
	meta := map[string]interface{}{
		"class":   e.class,
		"code":    e.code,
		"message": e.message,
	}
	if e.status != 0 {
		meta["status"] = e.status
	}
	if e.credentialSource != "" {
		meta["credentialSource"] = e.credentialSource
	}
	if e.upstreamBody != nil {
		meta["upstreamBody"] = e.upstreamBody
	}
	if len(e.details) > 0 {
		details := make(map[string]interface{}, len(e.details))
		for k, v := range e.details {
			details[k] = v
		}
		meta["details"] = details
	}
	return meta
}

// structuredContentFor renders a successful execution result as structured
// content. The result's own value is used, unless the executor reports through
// StructuredResultProvider that the value did not come from a JSON upstream
// payload — in which case the text block alone carries it. JSON that is not an
// object (an array or a scalar) is carried under "result", because MCP's
// structuredContent is a JSON object.
func structuredContentFor(result interface{}, resultJSON []byte) (map[string]interface{}, bool) {
	payload := result
	stated := false
	if provider, ok := result.(StructuredResultProvider); ok {
		declared, structured := provider.StructuredPayload()
		if !structured {
			return nil, false
		}
		payload, stated = declared, true
	}

	encoded := resultJSON
	if stated {
		var err error
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, false
		}
	}

	var decoded interface{}
	if len(encoded) == 0 || json.Unmarshal(encoded, &decoded) != nil || decoded == nil {
		return nil, false
	}
	if object, ok := decoded.(map[string]interface{}); ok {
		return object, true
	}
	return map[string]interface{}{"result": decoded}, true
}

// toolErrorMeta renders the marker for any failed tools/call: a classified
// executor error contributes its own metadata, an AppError is classified from
// its code, and anything else is reported explicitly as unclassified.
func toolErrorMeta(tool string, err error) map[string]interface{} {
	meta := map[string]interface{}{}
	if classified, ok := metaFromClassifier(err); ok {
		for k, v := range classified {
			meta[k] = v
		}
	}
	if _, ok := meta["class"]; !ok {
		class, code := classifyToolError(err)
		meta["class"] = class
		meta["code"] = code
	}
	if _, ok := meta["message"]; !ok {
		meta["message"] = err.Error()
	}

	details := map[string]interface{}{}
	if existing, ok := meta["details"].(map[string]interface{}); ok {
		for k, v := range existing {
			details[k] = v
		}
	}
	if tool != "" {
		if _, ok := details["tool"]; !ok {
			details["tool"] = tool
		}
	}
	if len(details) > 0 {
		meta["details"] = details
	}
	return meta
}

// metaFromClassifier returns the marker supplied by a classified error,
// following wrapped errors.
func metaFromClassifier(err error) (map[string]interface{}, bool) {
	var classified ToolErrorClassifier
	if !errors.As(err, &classified) {
		return nil, false
	}
	meta := classified.ToolErrorMeta()
	if len(meta) == 0 {
		return nil, false
	}
	return meta, true
}

// classifyToolError maps a plain execution error onto a class and reason code.
// muster's own typed AppError codes (used by the gRPC and JSON-RPC adapters)
// already separate an argument/lookup failure from an upstream or transport
// failure, so no adapter has to duplicate the mapping.
func classifyToolError(err error) (class, code string) {
	switch {
	case err == nil:
		return ErrorClassUnclassified, ReasonUnclassified
	case apperrors.IsError(err, apperrors.CodeValidation),
		apperrors.IsError(err, apperrors.CodeBadRequest):
		return ErrorClassClient, ReasonInvalidArguments
	case apperrors.IsError(err, apperrors.CodeNotFound):
		return ErrorClassClient, ReasonUnknownOperation
	case apperrors.IsError(err, apperrors.CodeUnauthorized):
		return ErrorClassClient, ReasonCredentialRejected
	case apperrors.IsError(err, apperrors.CodeRateLimit):
		return ErrorClassUpstream, ReasonUpstreamError
	case apperrors.IsError(err, apperrors.CodeTimeout):
		return ErrorClassUpstream, ReasonUpstreamUnreachable
	case apperrors.IsError(err, apperrors.CodeDatabase),
		apperrors.IsError(err, apperrors.CodeInternal),
		apperrors.IsError(err, apperrors.CodeGeneric),
		apperrors.IsError(err, apperrors.CodeProtocolMismatch):
		return ErrorClassUpstream, ReasonUpstreamError
	default:
		return ErrorClassUnclassified, ReasonUnclassified
	}
}
