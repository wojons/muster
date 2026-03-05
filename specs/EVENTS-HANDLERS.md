# Events: Event Handler Registry

## Overview

Event handler registry manages event handler registration, execution with middleware chains, and error handling for robust event processing pipelines.

## Handler Types

```go
package events

import (
    "context"
    "fmt"
    "reflect"
    "runtime"
    "sync"
    "time"
)

type HandlerFunc func(context.Context, *Event) error

type Handler interface {
    Handle(ctx context.Context, event *Event) error
    Name() string
    Supports(eventType EventType) bool
}

type HandlerMiddleware func(HandlerFunc) HandlerFunc

type HandlerInfo struct {
    Name        string
    EventTypes  []EventType
    Handler     Handler
    Middleware  []HandlerMiddleware
    Timeout     time.Duration
    MaxRetries  int
    OnError     ErrorHandler
}

type ErrorHandler func(ctx context.Context, event *Event, err error) error

type HandlerResult struct {
    HandlerName string
    EventID     string
    Duration    time.Duration
    Error       error
    ProcessedAt time.Time
}
```

## Registry Implementation

```go
type HandlerRegistry struct {
    handlers    map[string]*HandlerInfo
    byType      map[EventType][]string
    middlewares []HandlerMiddleware
    mu          sync.RWMutex
    logger      Logger
}

type Logger interface {
    Error(msg string, fields ...interface{})
    Info(msg string, fields ...interface{})
    Debug(msg string, fields ...interface{})
}

func NewHandlerRegistry(logger Logger) *HandlerRegistry {
    return &HandlerRegistry{
        handlers:    make(map[string]*HandlerInfo),
        byType:      make(map[EventType][]string),
        middlewares: make([]HandlerMiddleware, 0),
        logger:      logger,
    }
}

func (r *HandlerRegistry) Register(info *HandlerInfo) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if info.Name == "" {
        return fmt.Errorf("handler name is required")
    }

    if info.Handler == nil {
        return fmt.Errorf("handler is required")
    }

    if len(info.EventTypes) == 0 {
        return fmt.Errorf("at least one event type is required")
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

    return true
}

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

func (r *HandlerRegistry) Execute(ctx context.Context, event *Event) []HandlerResult {
    handlers := r.GetHandlers(event.Type)
    results := make([]HandlerResult, 0, len(handlers))

    for _, info := range handlers {
        result := r.executeSingle(ctx, info, event)
        results = append(results, result)
    }

    return results
}

func (r *HandlerRegistry) executeSingle(ctx context.Context, info *HandlerInfo, event *Event) HandlerResult {
    start := time.Now()
    result := HandlerResult{
        HandlerName: info.Name,
        EventID:     event.ID,
        ProcessedAt: time.Now(),
    }

    handler := r.wrapWithMiddleware(info.Handler.Handle, info.Middleware)
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
    }

    return result
}

func (r *HandlerRegistry) wrapWithMiddleware(handler HandlerFunc, middlewares []HandlerMiddleware) HandlerFunc {
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}

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

func (r *HandlerRegistry) Use(middleware HandlerMiddleware) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.middlewares = append(r.middlewares, middleware)
}

func (r *HandlerRegistry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    names := make([]string, 0, len(r.handlers))
    for name := range r.handlers {
        names = append(names, name)
    }
    return names
}
```
