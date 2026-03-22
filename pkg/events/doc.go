// Package events provides an event handler registry for managing event processing pipelines.
//
// The events package implements a robust event handler registry that supports:
//   - Handler registration and lifecycle management
//   - Middleware chains for cross-cutting concerns
//   - Timeout and retry mechanisms
//   - Event type filtering and routing
//   - Comprehensive error handling
//
// Basic Usage:
//
//	registry := events.NewHandlerRegistry(logger)
//
//	// Register a handler
//	err := registry.Register(&events.HandlerInfo{
//	    Name:       "audit-logger",
//	    EventTypes: []events.EventType{events.EventTypeRequest, events.EventTypeResponse},
//	    Handler:    myHandler,
//	    Timeout:    5 * time.Second,
//	    MaxRetries: 3,
//	})
//
//	// Execute handlers for an event
//	results := registry.Execute(ctx, event)
//
// Handler Interface:
//
// Handlers implement the Handler interface with three methods:
//   - Handle(ctx context.Context, event *Event) error: Process the event
//   - Name() string: Return the handler's unique name
//   - Supports(eventType EventType) bool: Check if handler supports event type
//
// Middleware:
//
// Middleware functions wrap handlers to add cross-cutting concerns:
//
//	// Timeout middleware
//	timeoutMiddleware := events.TimeoutMiddleware(5 * time.Second)
//
//	// Retry middleware
//	retryMiddleware := events.RetryMiddleware(3, 100 * time.Millisecond)
//
//	// Register with middleware
//	registry.Register(&events.HandlerInfo{
//	    Name:       "my-handler",
//	    Handler:    myHandler,
//	    Middleware: []events.HandlerMiddleware{timeoutMiddleware, retryMiddleware},
//	})
//
// Event Types:
//
// Event types categorize events for routing to appropriate handlers:
//   - EventTypeRequest: HTTP/API request events
//   - EventTypeResponse: HTTP/API response events
//   - EventTypeError: Error events
//   - EventTypeAudit: Audit log events
//   - EventTypeCustom: User-defined event types
//
// Thread Safety:
//
// The HandlerRegistry is safe for concurrent use. All operations use read/write
// mutexes to ensure thread safety across multiple goroutines.
//
// Error Handling:
//
// Handlers can return errors which are captured in HandlerResult. Use the OnError
// callback in HandlerInfo for custom error handling:
//
//	registry.Register(&events.HandlerInfo{
//	    Name:    "my-handler",
//	    Handler: myHandler,
//	    OnError: func(ctx context.Context, event *Event, err error) error {
//	        log.Printf("Handler failed: %v", err)
//	        return nil // Don't propagate error
//	    },
//	})
//
// See the documentation for HandlerRegistry, HandlerInfo, and HandlerMiddleware
// for more detailed usage examples.
package events
