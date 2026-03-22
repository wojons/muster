package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Logger is the interface for logging within the event system.
type Logger interface {
	Error(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// HandlerRegistry manages event handler registration and execution.
// It provides thread-safe operations for registering, unregistering,
// and executing handlers for different event types.
type HandlerRegistry struct {
	handlers    map[string]*HandlerInfo
	byType      map[EventType][]string
	middlewares []HandlerMiddleware
	mu          sync.RWMutex
	logger      Logger
}

// NewHandlerRegistry creates a new HandlerRegistry with the given logger.
// If logger is nil, logging is disabled.
func NewHandlerRegistry(logger Logger) *HandlerRegistry {
	return &HandlerRegistry{
		handlers:    make(map[string]*HandlerInfo),
		byType:      make(map[EventType][]string),
		middlewares: make([]HandlerMiddleware, 0),
		logger:      logger,
	}
}

// Register registers a handler with the registry.
// Returns an error if the handler info is invalid or if a handler with the same name already exists.
func (r *HandlerRegistry) Register(info *HandlerInfo) error {
	if info == nil {
		return fmt.Errorf("handler info is required")
	}

	if err := info.Validate(); err != nil {
		return fmt.Errorf("invalid handler info: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[info.Name]; exists {
		return fmt.Errorf("handler with name %q already registered", info.Name)
	}

	r.handlers[info.Name] = info

	for _, eventType := range info.EventTypes {
		r.byType[eventType] = append(r.byType[eventType], info.Name)
	}

	if r.logger != nil {
		r.logger.Info("handler registered",
			"name", info.Name,
			"event_types", info.EventTypes,
		)
	}

	return nil
}

// Unregister removes a handler from the registry.
// Returns true if the handler was found and removed, false otherwise.
func (r *HandlerRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.handlers[name]
	if !exists {
		return false
	}

	delete(r.handlers, name)

	for _, eventType := range info.EventTypes {
		handlers := r.byType[eventType]
		for i, n := range handlers {
			if n == name {
				r.byType[eventType] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}

	if r.logger != nil {
		r.logger.Info("handler unregistered",
			"name", name,
		)
	}

	return true
}

// GetHandlers returns all handlers registered for the given event type.
// Returns an empty slice if no handlers are registered for the event type.
func (r *HandlerRegistry) GetHandlers(eventType EventType) []*HandlerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.byType[eventType]
	result := make([]*HandlerInfo, 0, len(names))

	for _, name := range names {
		if info, ok := r.handlers[name]; ok {
			result = append(result, info)
		}
	}

	return result
}

// Execute executes all handlers registered for the event's type.
// Returns a slice of HandlerResult containing the outcome of each handler execution.
func (r *HandlerRegistry) Execute(ctx context.Context, event *Event) []HandlerResult {
	handlers := r.GetHandlers(event.Type)
	results := make([]HandlerResult, 0, len(handlers))

	for _, info := range handlers {
		result := r.executeSingle(ctx, info, event)
		results = append(results, result)
	}

	return results
}

// executeSingle executes a single handler for an event.
// This method applies middleware, timeout, and retry logic as configured.
func (r *HandlerRegistry) executeSingle(ctx context.Context, info *HandlerInfo, event *Event) HandlerResult {
	start := time.Now()
	result := HandlerResult{
		HandlerName: info.Name,
		EventID:     event.ID,
		ProcessedAt: time.Now(),
	}

	// Combine handler-specific middleware with registry-wide middleware
	allMiddleware := append(r.middlewares, info.Middleware...)

	handler := r.wrapWithMiddleware(info.Handler.Handle, allMiddleware)
	handler = r.wrapWithTimeout(handler, info.Timeout)
	handler = r.wrapWithRetry(handler, info.MaxRetries)

	err := handler(ctx, event)
	if err != nil && info.OnError != nil {
		err = info.OnError(ctx, event, err)
	}

	result.Duration = time.Since(start)
	result.Error = err

	if err != nil && r.logger != nil {
		r.logger.Error("handler execution failed",
			"handler", info.Name,
			"event_id", event.ID,
			"error", err,
			"duration", result.Duration,
		)
	} else if r.logger != nil {
		r.logger.Debug("handler execution completed",
			"handler", info.Name,
			"event_id", event.ID,
			"duration", result.Duration,
		)
	}

	return result
}

// wrapWithMiddleware wraps a handler function with a chain of middleware.
// Middleware is applied in reverse order so the first middleware is the outermost.
func (r *HandlerRegistry) wrapWithMiddleware(handler HandlerFunc, middlewares []HandlerMiddleware) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// wrapWithTimeout wraps a handler function with a timeout.
// If timeout is zero or negative, no timeout is applied.
func (r *HandlerRegistry) wrapWithTimeout(handler HandlerFunc, timeout time.Duration) HandlerFunc {
	if timeout <= 0 {
		return handler
	}

	return func(ctx context.Context, event *Event) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return handler(ctx, event)
	}
}

// wrapWithRetry wraps a handler function with retry logic.
// If maxRetries is zero or negative, no retry logic is applied.
func (r *HandlerRegistry) wrapWithRetry(handler HandlerFunc, maxRetries int) HandlerFunc {
	if maxRetries <= 0 {
		return handler
	}

	return func(ctx context.Context, event *Event) error {
		var err error
		for i := 0; i <= maxRetries; i++ {
			err = handler(ctx, event)
			if err == nil {
				return nil
			}
			if i < maxRetries {
				time.Sleep(time.Second * time.Duration(i+1))
			}
		}
		return err
	}
}

// Use adds middleware to the registry that will be applied to all handlers.
// Middleware is applied in the order it is added.
func (r *HandlerRegistry) Use(middleware HandlerMiddleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, middleware)
}

// List returns a list of all registered handler names.
func (r *HandlerRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered handlers.
func (r *HandlerRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

// IsRegistered returns true if a handler with the given name is registered.
func (r *HandlerRegistry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.handlers[name]
	return exists
}
