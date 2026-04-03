package log

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// CorrelationIDKey is the context key for correlation IDs
type CorrelationIDKey struct{}

var (
	correlationIDMu     sync.RWMutex
	correlationIDHeader = "X-Correlation-ID"
)

// SetCorrelationIDHeader sets the HTTP header name for correlation IDs
func SetCorrelationIDHeader(header string) {
	correlationIDMu.Lock()
	defer correlationIDMu.Unlock()
	correlationIDHeader = header
}

// GetCorrelationIDHeader returns the configured correlation ID header name
func GetCorrelationIDHeader() string {
	correlationIDMu.RLock()
	defer correlationIDMu.RUnlock()
	return correlationIDHeader
}

// GenerateCorrelationID generates a new unique correlation ID
func GenerateCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return generateTimestampID()
	}
	return hex.EncodeToString(b)
}

func generateTimestampID() string {
	return "cid-" + generateEventID()
}

// WithCorrelationID returns a new context with the correlation ID
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		correlationID = GenerateCorrelationID()
	}
	return context.WithValue(ctx, CorrelationIDKey{}, correlationID)
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if cid, ok := ctx.Value(CorrelationIDKey{}).(string); ok {
		return cid
	}
	return ""
}

// GetCorrelationIDOrGenerate returns the correlation ID from context or generates a new one
func GetCorrelationIDOrGenerate(ctx context.Context) string {
	if cid := GetCorrelationID(ctx); cid != "" {
		return cid
	}
	return GenerateCorrelationID()
}

// CorrelationIDMiddleware extracts correlation ID from requests and adds to context
// For HTTP servers, use this middleware to extract correlation ID from headers
func CorrelationIDMiddleware(next func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// Check if correlation ID already exists in context
		if GetCorrelationID(ctx) == "" {
			ctx = WithCorrelationID(ctx, GenerateCorrelationID())
		}
		return next(ctx)
	}
}

// LoggerWithCorrelationID returns a logger with correlation ID field
func LoggerWithCorrelationID(ctx context.Context, l *Logger) *Logger {
	if cid := GetCorrelationID(ctx); cid != "" {
		return l.WithStr("correlation_id", cid)
	}
	return l
}

// RequestContext holds request-scoped context information
type RequestContext struct {
	CorrelationID string
	TraceID       string
	SpanID        string
	RequestID     string
	TenantID      string
	UserID        string
	SessionID     string
	ClientIP      string
	UserAgent     string
}

// ToFields converts RequestContext to log fields
func (rc RequestContext) ToFields() map[string]interface{} {
	fields := make(map[string]interface{})
	if rc.CorrelationID != "" {
		fields["correlation_id"] = rc.CorrelationID
	}
	if rc.TraceID != "" {
		fields["trace_id"] = rc.TraceID
	}
	if rc.SpanID != "" {
		fields["span_id"] = rc.SpanID
	}
	if rc.RequestID != "" {
		fields["request_id"] = rc.RequestID
	}
	if rc.TenantID != "" {
		fields["tenant_id"] = rc.TenantID
	}
	if rc.UserID != "" {
		fields["user_id"] = rc.UserID
	}
	if rc.SessionID != "" {
		fields["session_id"] = rc.SessionID
	}
	if rc.ClientIP != "" {
		fields["client_ip"] = rc.ClientIP
	}
	if rc.UserAgent != "" {
		fields["user_agent"] = rc.UserAgent
	}
	return fields
}

// WithRequestContext returns a new context with all request context values
func WithRequestContext(ctx context.Context, rc RequestContext) context.Context {
	ctx = WithCorrelationID(ctx, rc.CorrelationID)

	if rc.TraceID != "" {
		ctx = WithTraceID(ctx, rc.TraceID)
	}
	if rc.SpanID != "" {
		ctx = WithSpanID(ctx, rc.SpanID)
	}
	if rc.RequestID != "" {
		ctx = WithRequestID(ctx, rc.RequestID)
	}
	if rc.TenantID != "" {
		ctx = WithTenantID(ctx, rc.TenantID)
	}
	if rc.UserID != "" {
		ctx = WithUserID(ctx, rc.UserID)
	}
	if rc.SessionID != "" {
		ctx = WithSessionID(ctx, rc.SessionID)
	}
	if rc.ClientIP != "" {
		ctx = WithClientIP(ctx, rc.ClientIP)
	}
	if rc.UserAgent != "" {
		ctx = WithUserAgent(ctx, rc.UserAgent)
	}

	return ctx
}

// Context keys for request-scoped values
type (
	traceIDKey   struct{}
	spanIDKey    struct{}
	requestIDKey struct{}
	tenantIDKey  struct{}
	userIDKey    struct{}
	sessionIDKey struct{}
	clientIPKey  struct{}
	userAgentKey struct{}
)

// Trace ID helpers
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Span ID helpers
func WithSpanID(ctx context.Context, spanID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, spanIDKey{}, spanID)
}

func GetSpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(spanIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Request ID helpers
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Tenant ID helpers
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

func GetTenantID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(tenantIDKey{}).(string); ok {
		return id
	}
	return ""
}

// User ID helpers
func WithUserID(ctx context.Context, userID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}

func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(userIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Session ID helpers
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

func GetSessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Client IP helpers
func WithClientIP(ctx context.Context, clientIP string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, clientIPKey{}, clientIP)
}

func GetClientIP(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ip, ok := ctx.Value(clientIPKey{}).(string); ok {
		return ip
	}
	return ""
}

// User Agent helpers
func WithUserAgent(ctx context.Context, userAgent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userAgentKey{}, userAgent)
}

func GetUserAgent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ua, ok := ctx.Value(userAgentKey{}).(string); ok {
		return ua
	}
	return ""
}

// GetAllContextValues returns all logging-related values from context
func GetAllContextValues(ctx context.Context) map[string]string {
	values := make(map[string]string)

	if cid := GetCorrelationID(ctx); cid != "" {
		values["correlation_id"] = cid
	}
	if tid := GetTraceID(ctx); tid != "" {
		values["trace_id"] = tid
	}
	if sid := GetSpanID(ctx); sid != "" {
		values["span_id"] = sid
	}
	if rid := GetRequestID(ctx); rid != "" {
		values["request_id"] = rid
	}
	if teid := GetTenantID(ctx); teid != "" {
		values["tenant_id"] = teid
	}
	if uid := GetUserID(ctx); uid != "" {
		values["user_id"] = uid
	}
	if sid := GetSessionID(ctx); sid != "" {
		values["session_id"] = sid
	}
	if ip := GetClientIP(ctx); ip != "" {
		values["client_ip"] = ip
	}
	if ua := GetUserAgent(ctx); ua != "" {
		values["user_agent"] = ua
	}

	return values
}
