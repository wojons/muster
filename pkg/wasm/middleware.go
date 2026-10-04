// Package wasm — HTTP response interceptor middleware for WASM plugins.
//
// axiom:trace work_item=AC-007 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-2
package wasm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Interceptor wraps an HTTP response with WASM transformation.
type Interceptor struct {
	manager *ModuleManager
	// Timeout for WASM execution (per-request)
	Timeout time.Duration
	// PassThroughNonJSON causes the interceptor to skip transformation
	// for non-JSON content types (binary, images, etc.)
	PassThroughNonJSON bool
}

// NewInterceptor creates a new WASM interceptor.
func NewInterceptor() *Interceptor {
	return &Interceptor{
		manager:            NewModuleManager(nil),
		Timeout:            5 * time.Second,
		PassThroughNonJSON: true,
	}
}

// NewInterceptorWithLimits creates a new WASM interceptor with custom limits.
func NewInterceptorWithLimits(limits *SandboxLimits) *Interceptor {
	return &Interceptor{
		manager:            NewModuleManager(limits),
		Timeout:            5 * time.Second,
		PassThroughNonJSON: true,
	}
}

// LoadPlugin loads a WASM plugin from the given path.
func (i *Interceptor) LoadPlugin(ctx context.Context, path string) error {
	_, err := i.manager.Load(ctx, path)
	return err
}

// UnloadPlugin unloads a WASM plugin.
func (i *Interceptor) UnloadPlugin(ctx context.Context, path string) error {
	return i.manager.Unload(ctx, path)
}

// RegisteredPlugins returns the list of loaded plugin paths.
func (i *Interceptor) RegisteredPlugins() []string {
	return i.manager.List()
}

// Close releases all plugins.
func (i *Interceptor) Close(ctx context.Context) error {
	return i.manager.Close(ctx)
}

// TransformResponseBody applies WASM transformation to the response body.
// If the module is not loaded or the content type is non-JSON, the body
// is returned unchanged (pass-through).
func (i *Interceptor) TransformResponseBody(ctx context.Context, body []byte, contentType string, pluginPath string) ([]byte, error) {
	// If no plugin is registered, pass through
	if pluginPath == "" {
		return body, nil
	}

	lm := i.manager.Get(pluginPath)
	if lm == nil {
		return nil, fmt.Errorf("wasm: plugin %q is not loaded", pluginPath)
	}

	// If body is empty or content type is non-JSON, pass through
	if len(body) == 0 {
		return body, nil
	}
	if i.PassThroughNonJSON && !isJSONContentType(contentType) {
		return body, nil
	}

	// The caller's context deadline governs the transformation (PROTOCOLS.md
	// §4.3): the interceptor's Timeout is a default applied only when the
	// caller declared no deadline, never a clamp on an existing one.
	execCtx := ctx
	if _, hasDeadline := execCtx.Deadline(); !hasDeadline && i.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, i.Timeout)
		defer cancel()
	}

	// Execute transformation
	result, err := i.manager.Transform(execCtx, lm, body)
	if err != nil {
		return nil, fmt.Errorf("wasm: transform error: %w", err)
	}

	return result, nil
}

// InterceptResponse wraps an http.Response with WASM transformation.
// It reads the response body, applies the WASM plugin, and returns
// a new body reader with the transformed content.
func (i *Interceptor) InterceptResponse(resp *http.Response, pluginPath string) error {
	if pluginPath == "" {
		return nil
	}

	// Read original body
	originalBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("wasm: read response body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// Preserve the caller's context: the response's request context when
	// one is attached, so its deadline and cancellation govern the
	// transformation (PROTOCOLS.md §4.3); responses built without a
	// request keep background semantics.
	transformCtx := context.Background()
	if resp.Request != nil {
		transformCtx = resp.Request.Context()
	}

	transformed, err := i.TransformResponseBody(transformCtx, originalBody, contentType, pluginPath)
	if err != nil {
		return err
	}

	// Replace body and update Content-Length
	resp.Body = io.NopCloser(strings.NewReader(string(transformed)))
	resp.ContentLength = int64(len(transformed))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(transformed)))

	return nil
}

// Middleware returns an HTTP middleware function that intercepts responses.
// Usage with standard HTTP middleware:
//
//	interceptor := wasm.NewInterceptor(runtime)
//	interceptor.LoadPlugin(ctx, "transform.wasm")
//	handler := interceptor.Middleware("transform.wasm")(myHandler)
func (i *Interceptor) Middleware(pluginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pluginPath == "" || i.manager.Get(pluginPath) == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap the response writer to capture the response
			crw := &captureResponseWriter{
				ResponseWriter: w,
				pluginPath:     pluginPath,
				interceptor:    i,
				req:            r,
			}
			next.ServeHTTP(crw, r)
		})
	}
}

// captureResponseWriter captures the HTTP response, applies WASM transformation,
// and then writes the transformed output to the original writer.
type captureResponseWriter struct {
	http.ResponseWriter
	pluginPath  string
	interceptor *Interceptor
	buf         []byte
	statusCode  int
	// req is the request whose context the transformation runs under; nil
	// is allowed (bare-literal construction in tests) and falls back to
	// context.Background().
	req *http.Request
}

func (crw *captureResponseWriter) transformContext() context.Context {
	if crw.req != nil {
		return crw.req.Context()
	}
	return context.Background()
}

func (crw *captureResponseWriter) WriteHeader(statusCode int) {
	crw.statusCode = statusCode
}

func (crw *captureResponseWriter) Write(data []byte) (int, error) {
	crw.buf = append(crw.buf, data...)
	return len(data), nil
}

// Flush sends the captured response through WASM transformation and writes to the real writer.
func (crw *captureResponseWriter) Flush() error {
	contentType := crw.Header().Get("Content-Type")

	transformed, err := crw.interceptor.TransformResponseBody(
		crw.transformContext(),
		crw.buf,
		contentType,
		crw.pluginPath,
	)
	if err != nil {
		return err
	}

	// Write the actual status code and transformed body
	if crw.statusCode != 0 {
		crw.ResponseWriter.WriteHeader(crw.statusCode)
	} else {
		crw.ResponseWriter.WriteHeader(http.StatusOK)
	}

	_, writeErr := crw.ResponseWriter.Write(transformed)
	return writeErr
}

// isJSONContentType checks if the content type indicates JSON data.
func isJSONContentType(ct string) bool {
	if ct == "" {
		return false
	}
	// Strip parameters (e.g., "application/json; charset=utf-8")
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)
	return ct == "application/json" || strings.HasSuffix(ct, "+json")
}
