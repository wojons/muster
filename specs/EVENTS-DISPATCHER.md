# Events: Event Dispatcher

## Overview

Event dispatcher implements a thread-safe event bus for publish-subscribe patterns with async handling, subscription management, and guaranteed delivery semantics.

## Configuration

```go
type DispatcherConfig struct {
    BufferSize          int           `json:"buffer_size" validate:"gte=1"`
    WorkerCount         int           `json:"worker_count" validate:"gte=1"`
    Timeout             time.Duration `json:"timeout"`
    RetryAttempts       int           `json:"retry_attempts" validate:"gte=0"`
    RetryDelay          time.Duration `json:"retry_delay"`
    MaxQueueSize        int           `json:"max_queue_size" validate:"gte=1"`
    EnableDeadLetter    bool          `json:"enable_dead_letter"`
    DeadLetterQueueSize int           `json:"dead_letter_queue_size"`
}

var DefaultDispatcherConfig = DispatcherConfig{
    BufferSize:          1000,
    WorkerCount:         10,
    Timeout:             time.Second * 30,
    RetryAttempts:       3,
    RetryDelay:          time.Second,
    MaxQueueSize:        10000,
    EnableDeadLetter:    true,
    DeadLetterQueueSize: 1000,
}
```

## Event Types

```go
package events

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

type EventType string

const (
    EventTypeInventoryCreated EventType = "inventory.created"
    EventTypeInventoryUpdated   EventType = "inventory.updated"
    EventTypeInventoryDeleted   EventType = "inventory.deleted"
    EventTypeUserLogin          EventType = "user.login"
    EventTypeUserLogout         EventType = "user.logout"
    EventTypeAuditLog           EventType = "audit.log"
)

type Event struct {
    ID          string          `json:"id"`
    Type        EventType       `json:"type"`
    Source      string          `json:"source"`
    Timestamp   time.Time       `json:"timestamp"`
    Data        json.RawMessage `json:"data"`
    Metadata    EventMetadata   `json:"metadata"`
}

type EventMetadata struct {
    CorrelationID string            `json:"correlation_id,omitempty"`
    CausationID   string            `json:"causation_id,omitempty"`
    UserID        string            `json:"user_id,omitempty"`
    TraceID       string            `json:"trace_id,omitempty"`
    Attributes    map[string]string `json:"attributes,omitempty"`
}

type EventResult struct {
    EventID string
    Error   error
    Handler string
}
```

## Dispatcher Implementation

```go
type Dispatcher struct {
    config       DispatcherConfig
    subscribers  map[EventType][]*subscription
    mu           sync.RWMutex
    eventChan    chan *Event
    done         chan struct{}
    wg           sync.WaitGroup
    deadLetter   chan *deadLetterEntry
}

type subscription struct {
    id       string
    handler  Handler
    priority int
    filter   FilterFunc
    once     bool
}

type deadLetterEntry struct {
    Event      *Event
    Error      error
    Attempts   int
    LastError  time.Time
}

type Handler func(context.Context, *Event) error
type FilterFunc func(*Event) bool

func NewDispatcher(config DispatcherConfig) *Dispatcher {
    d := &Dispatcher{
        config:      config,
        subscribers: make(map[EventType][]*subscription),
        eventChan:   make(chan *Event, config.BufferSize),
        done:        make(chan struct{}),
    }
    
    if config.EnableDeadLetter {
        d.deadLetter = make(chan *deadLetterEntry, config.DeadLetterQueueSize)
    }

    for i := 0; i < config.WorkerCount; i++ {
        d.wg.Add(1)
        go d.worker(i)
    }

    if config.EnableDeadLetter {
        d.wg.Add(1)
        go d.deadLetterWorker()
    }

    return d
}

func (d *Dispatcher) Subscribe(eventType EventType, handler Handler, opts ...SubscribeOption) string {
    sub := &subscription{
        id:       generateID(),
        handler:  handler,
        priority: 0,
    }
    
    for _, opt := range opts {
        opt(sub)
    }

    d.mu.Lock()
    defer d.mu.Unlock()
    
    d.subscribers[eventType] = append(d.subscribers[eventType], sub)
    sortSubscribers(d.subscribers[eventType])
    
    return sub.id
}

func (d *Dispatcher) Unsubscribe(eventType EventType, subscriptionID string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    subs := d.subscribers[eventType]
    for i, sub := range subs {
        if sub.id == subscriptionID {
            d.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
            return true
        }
    }
    return false
}

func (d *Dispatcher) Publish(ctx context.Context, event *Event) error {
    if event.ID == "" {
        event.ID = generateID()
    }
    if event.Timestamp.IsZero() {
        event.Timestamp = time.Now()
    }

    select {
    case d.eventChan <- event:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        return fmt.Errorf("event queue full")
    }
}

func (d *Dispatcher) PublishSync(ctx context.Context, event *Event) []EventResult {
    d.mu.RLock()
    subs := d.subscribers[event.Type]
    d.mu.RUnlock()

    var results []EventResult
    for _, sub := range subs {
        if sub.filter != nil && !sub.filter(event) {
            continue
        }

        err := d.executeHandler(ctx, sub, event)
        results = append(results, EventResult{
            EventID: event.ID,
            Error:   err,
            Handler: sub.id,
        })

        if sub.once {
            d.Unsubscribe(event.Type, sub.id)
        }
    }

    return results
}

func (d *Dispatcher) executeHandler(ctx context.Context, sub *subscription, event *Event) error {
    ctx, cancel := context.WithTimeout(ctx, d.config.Timeout)
    defer cancel()

    var err error
    for attempt := 0; attempt <= d.config.RetryAttempts; attempt++ {
        err = sub.handler(ctx, event)
        if err == nil {
            return nil
        }
        
        if attempt < d.config.RetryAttempts {
            time.Sleep(d.config.RetryDelay * time.Duration(attempt+1))
        }
    }

    if d.config.EnableDeadLetter {
        select {
        case d.deadLetter <- &deadLetterEntry{
            Event:     event,
            Error:     err,
            Attempts:  d.config.RetryAttempts + 1,
            LastError: time.Now(),
        }:
        default:
        }
    }

    return err
}

func (d *Dispatcher) worker(id int) {
    defer d.wg.Done()
    
    for {
        select {
        case event := <-d.eventChan:
            d.processEvent(event)
        case <-d.done:
            return
        }
    }
}

func (d *Dispatcher) processEvent(event *Event) {
    ctx := context.Background()
    d.PublishSync(ctx, event)
}

func (d *Dispatcher) deadLetterWorker() {
    defer d.wg.Done()
    
    for {
        select {
        case entry := <-d.deadLetter:
            d.handleDeadLetter(entry)
        case <-d.done:
            return
        }
    }
}

func (d *Dispatcher) handleDeadLetter(entry *deadLetterEntry) {
    // Log or persist dead letter events
}

func (d *Dispatcher) Stop() error {
    close(d.done)
    d.wg.Wait()
    return nil
}

type SubscribeOption func(*subscription)

func WithPriority(priority int) SubscribeOption {
    return func(s *subscription) {
        s.priority = priority
    }
}

func WithFilter(filter FilterFunc) SubscribeOption {
    return func(s *subscription) {
        s.filter = filter
    }
}

func Once() SubscribeOption {
    return func(s *subscription) {
        s.once = true
    }
}

func sortSubscribers(subs []*subscription) {
    sort.Slice(subs, func(i, j int) bool {
        return subs[i].priority > subs[j].priority
    })
}
```
