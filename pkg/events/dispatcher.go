package events

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DispatcherConfig configures the behavior of the event dispatcher.
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

// DefaultDispatcherConfig provides sensible defaults for dispatcher configuration.
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

// Validate validates the dispatcher configuration.
func (c *DispatcherConfig) Validate() error {
	if c.BufferSize < 1 {
		return fmt.Errorf("buffer_size must be >= 1")
	}
	if c.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be >= 1")
	}
	if c.MaxQueueSize < 1 {
		return fmt.Errorf("max_queue_size must be >= 1")
	}
	if c.EnableDeadLetter && c.DeadLetterQueueSize < 1 {
		return fmt.Errorf("dead_letter_queue_size must be >= 1 when dead letter is enabled")
	}
	return nil
}

// subscription represents a handler subscribed to receive events.
type subscription struct {
	id       string
	handler  Handler
	priority int
	filter   FilterFunc
	once     bool
}

// deadLetterEntry holds events that failed processing after all retries.
type deadLetterEntry struct {
	Event     *Event
	Error     error
	Attempts  int
	LastError time.Time
}

// Dispatcher implements a thread-safe event bus for publish-subscribe patterns.
// It supports async event publishing, multiple workers, subscription management,
// retry logic, and dead letter queue for failed events.
type Dispatcher struct {
	config      DispatcherConfig
	subscribers map[EventType][]*subscription
	mu          sync.RWMutex
	eventChan   chan *Event
	done        chan struct{}
	wg          sync.WaitGroup
	deadLetter  chan *deadLetterEntry
	logger      Logger
}

// FilterFunc determines whether a handler should process a given event.
type FilterFunc func(*Event) bool

// DispatcherOption configures a Dispatcher instance.
type DispatcherOption func(*Dispatcher)

// WithLogger sets the logger for the dispatcher.
func WithLogger(logger Logger) DispatcherOption {
	return func(d *Dispatcher) {
		d.logger = logger
	}
}

// NewDispatcher creates a new event dispatcher with the given configuration.
// It starts worker goroutines to process events asynchronously and optionally
// starts a dead letter worker if enabled in the config.
func NewDispatcher(config DispatcherConfig, opts ...DispatcherOption) (*Dispatcher, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	d := &Dispatcher{
		config:      config,
		subscribers: make(map[EventType][]*subscription),
		eventChan:   make(chan *Event, config.BufferSize),
		done:        make(chan struct{}),
	}

	for _, opt := range opts {
		opt(d)
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

	return d, nil
}

// Subscribe registers a handler to receive events of a specific type.
// Returns a subscription ID that can be used to unsubscribe later.
// Options can customize the subscription with priority, filters, or one-time execution.
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

	if d.logger != nil {
		d.logger.Info("subscriber added",
			"subscription_id", sub.id,
			"event_type", eventType,
			"handler", handler.Name(),
		)
	}

	return sub.id
}

// Unsubscribe removes a subscription by ID.
// Returns true if the subscription was found and removed, false otherwise.
func (d *Dispatcher) Unsubscribe(eventType EventType, subscriptionID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	subs := d.subscribers[eventType]
	for i, sub := range subs {
		if sub.id == subscriptionID {
			d.subscribers[eventType] = append(subs[:i], subs[i+1:]...)

			if d.logger != nil {
				d.logger.Debug("subscriber removed",
					"subscription_id", subscriptionID,
					"event_type", eventType,
				)
			}

			return true
		}
	}
	return false
}

// Publish sends an event to be processed asynchronously by workers.
// Returns an error if the event queue is full or the context is cancelled.
// Automatically sets event ID and timestamp if not provided.
func (d *Dispatcher) Publish(ctx context.Context, event *Event) error {
	if event.ID == "" {
		id, err := GenerateEventID()
		if err != nil {
			return fmt.Errorf("generate event ID: %w", err)
		}
		event.ID = id
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	select {
	case d.eventChan <- event:
		if d.logger != nil {
			d.logger.Debug("event published",
				"event_id", event.ID,
				"event_type", event.Type,
			)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("event queue full")
	}
}

// PublishSync synchronously processes an event through all matching handlers.
// Returns results from each handler execution.
// This blocks until all handlers have completed or the context is cancelled.
func (d *Dispatcher) PublishSync(ctx context.Context, event *Event) []HandlerResult {
	if event.ID == "" {
		id, err := GenerateEventID()
		if err != nil {
			return []HandlerResult{{
				HandlerName: "dispatcher",
				EventID:     "",
				Error:       fmt.Errorf("generate event ID: %w", err),
				ProcessedAt: time.Now(),
			}}
		}
		event.ID = id
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	d.mu.RLock()
	subs := d.subscribers[event.Type]
	d.mu.RUnlock()

	var results []HandlerResult
	for _, sub := range subs {
		if sub.filter != nil && !sub.filter(event) {
			continue
		}

		result := d.executeHandler(ctx, sub, event)
		results = append(results, result)

		if sub.once {
			d.Unsubscribe(event.Type, sub.id)
		}
	}

	return results
}

// executeHandler executes a single handler for an event with retry logic.
// Failed attempts are retried according to config, then sent to dead letter queue.
func (d *Dispatcher) executeHandler(ctx context.Context, sub *subscription, event *Event) HandlerResult {
	start := time.Now()
	result := HandlerResult{
		HandlerName: sub.handler.Name(),
		EventID:     event.ID,
		ProcessedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(ctx, d.config.Timeout)
	defer cancel()

	var err error
	for attempt := 0; attempt <= d.config.RetryAttempts; attempt++ {
		err = sub.handler.Handle(ctx, event)
		if err == nil {
			result.Duration = time.Since(start)
			result.Error = nil
			return result
		}

		if attempt < d.config.RetryAttempts {
			delay := d.config.RetryDelay * time.Duration(attempt+1)
			if d.logger != nil {
				d.logger.Debug("handler failed, retrying",
					"handler", sub.handler.Name(),
					"event_id", event.ID,
					"attempt", attempt+1,
					"max_attempts", d.config.RetryAttempts,
					"delay", delay,
					"error", err,
				)
			}
			time.Sleep(delay)
		}
	}

	result.Duration = time.Since(start)
	result.Error = err

	if d.config.EnableDeadLetter {
		entry := &deadLetterEntry{
			Event:     event,
			Error:     err,
			Attempts:  d.config.RetryAttempts + 1,
			LastError: time.Now(),
		}
		select {
		case d.deadLetter <- entry:
			if d.logger != nil {
				d.logger.Error("event sent to dead letter queue",
					"event_id", event.ID,
					"handler", sub.handler.Name(),
					"error", err,
					"attempts", entry.Attempts,
				)
			}
		default:
			if d.logger != nil {
				d.logger.Error("dead letter queue full, dropping event",
					"event_id", event.ID,
					"handler", sub.handler.Name(),
				)
			}
		}
	}

	return result
}

// worker processes events from the event channel.
// Each worker runs in its own goroutine and processes events concurrently.
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()

	if d.logger != nil {
		d.logger.Debug("worker started", "worker_id", id)
	}

	for {
		select {
		case event := <-d.eventChan:
			d.processEvent(event)
		case <-d.done:
			if d.logger != nil {
				d.logger.Debug("worker stopped", "worker_id", id)
			}
			return
		}
	}
}

// processEvent handles a single event by publishing it synchronously.
// This is called by workers to process events from the queue.
func (d *Dispatcher) processEvent(event *Event) {
	ctx := context.Background()
	d.PublishSync(ctx, event)
}

// deadLetterWorker processes events that failed all retry attempts.
// It runs in its own goroutine and handles dead letter events.
func (d *Dispatcher) deadLetterWorker() {
	defer d.wg.Done()

	if d.logger != nil {
		d.logger.Debug("dead letter worker started")
	}

	for {
		select {
		case entry := <-d.deadLetter:
			d.handleDeadLetter(entry)
		case <-d.done:
			if d.logger != nil {
				d.logger.Debug("dead letter worker stopped")
			}
			return
		}
	}
}

// handleDeadLetter processes a dead letter entry.
// Default implementation logs the failed event. Override for custom handling.
func (d *Dispatcher) handleDeadLetter(entry *deadLetterEntry) {
	if d.logger != nil {
		d.logger.Error("dead letter event",
			"event_id", entry.Event.ID,
			"event_type", entry.Event.Type,
			"error", entry.Error,
			"attempts", entry.Attempts,
			"last_error", entry.LastError,
		)
	}
}

// Stop gracefully shuts down the dispatcher.
// It stops accepting new events, waits for in-flight events to complete,
// and closes all workers.
func (d *Dispatcher) Stop() error {
	close(d.done)
	d.wg.Wait()

	if d.logger != nil {
		d.logger.Info("dispatcher stopped")
	}

	return nil
}

// SubscribeOption configures a subscription.
type SubscribeOption func(*subscription)

// WithPriority sets the priority of a subscription.
// Higher priority subscriptions are processed first.
func WithPriority(priority int) SubscribeOption {
	return func(s *subscription) {
		s.priority = priority
	}
}

// WithFilter sets a filter function to determine which events the handler receives.
func WithFilter(filter FilterFunc) SubscribeOption {
	return func(s *subscription) {
		s.filter = filter
	}
}

// Once marks the subscription as one-time only.
// The subscription is automatically removed after the first event is processed.
func Once() SubscribeOption {
	return func(s *subscription) {
		s.once = true
	}
}

// sortSubscribers sorts subscriptions by priority in descending order.
func sortSubscribers(subs []*subscription) {
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].priority > subs[j].priority
	})
}

// generateID generates a unique subscription ID.
func generateID() string {
	id, _ := GenerateEventID()
	return "sub_" + id
}
