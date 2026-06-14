package events

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// SubscriptionID uniquely identifies a subscriber registration.
type SubscriptionID string

// PublishResult holds the result of a synchronous publish to a specific handler.
type PublishResult struct {
	HandlerName string
	Error       error
}

// DispatcherConfig configures the event dispatcher.
type DispatcherConfig struct {
	// WorkerCount is the number of goroutines processing events.
	WorkerCount int
	// BufferSize is the capacity of the event channel.
	BufferSize int
	// RetryAttempts is the number of retries on handler failure.
	RetryAttempts int
	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration
	// EnableDeadLetter enables the dead-letter queue for permanently failed events.
	EnableDeadLetter bool
	// DeadLetterQueueSize is the max capacity of the dead-letter queue.
	DeadLetterQueueSize int
}

// DefaultDispatcherConfig provides sensible defaults.
var DefaultDispatcherConfig = DispatcherConfig{
	WorkerCount:         4,
	BufferSize:          1000,
	RetryAttempts:       0,
	RetryDelay:          100 * time.Millisecond,
	EnableDeadLetter:    false,
	DeadLetterQueueSize: 100,
}

// subscriber represents a registered handler with its options.
type subscriber struct {
	id       SubscriptionID
	handler  Handler
	priority int
	filter   func(*Event) bool
}

// SubscribeOption is a functional option for Subscribe.
type SubscribeOption func(*subscriber)

// WithPriority sets the priority for a subscriber. Higher values execute first.
func WithPriority(p int) SubscribeOption {
	return func(s *subscriber) {
		s.priority = p
	}
}

// WithFilter sets an event filter for a subscriber.
// The handler is only called when the filter returns true.
func WithFilter(f func(*Event) bool) SubscribeOption {
	return func(s *subscriber) {
		s.filter = f
	}
}

// Dispatcher manages event subscriptions and dispatching.
type Dispatcher struct {
	config     DispatcherConfig
	eventCh    chan *dispatchItem
	deadLetter chan *Event
	quit       chan struct{}
	wg         sync.WaitGroup

	mu          sync.RWMutex
	subscribers map[EventType][]*subscriber
	stopped     bool
}

type dispatchItem struct {
	event  *Event
	result chan []PublishResult // nil for async publishes
}

// NewDispatcher creates a new Dispatcher with the given config.
func NewDispatcher(config DispatcherConfig) (*Dispatcher, error) {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 4
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}

	d := &Dispatcher{
		config:      config,
		eventCh:     make(chan *dispatchItem, config.BufferSize),
		quit:        make(chan struct{}),
		subscribers: make(map[EventType][]*subscriber),
	}

	if config.EnableDeadLetter && config.DeadLetterQueueSize > 0 {
		d.deadLetter = make(chan *Event, config.DeadLetterQueueSize)
	}

	// Start worker pool
	for i := 0; i < config.WorkerCount; i++ {
		d.wg.Add(1)
		go d.worker()
	}

	return d, nil
}

func generateID() SubscriptionID {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return SubscriptionID("sub-" + string(b))
}

// Subscribe registers a handler for a given event type.
func (d *Dispatcher) Subscribe(eventType EventType, handler Handler, opts ...SubscribeOption) SubscriptionID {
	d.mu.Lock()
	defer d.mu.Unlock()

	sub := &subscriber{
		id:       generateID(),
		handler:  handler,
		priority: 0,
	}
	for _, opt := range opts {
		opt(sub)
	}

	d.subscribers[eventType] = append(d.subscribers[eventType], sub)
	// Keep subscribers sorted by priority descending
	sort.Slice(d.subscribers[eventType], func(i, j int) bool {
		return d.subscribers[eventType][i].priority > d.subscribers[eventType][j].priority
	})

	return sub.id
}

// Publish sends an event asynchronously to all matching subscribers.
func (d *Dispatcher) Publish(ctx context.Context, event *Event) error {
	d.mu.RLock()
	if d.stopped {
		d.mu.RUnlock()
		return fmt.Errorf("dispatcher is stopped")
	}
	d.mu.RUnlock()

	select {
	case d.eventCh <- &dispatchItem{event: event}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishSync sends an event synchronously to all matching subscribers
// and returns the result from each handler.
func (d *Dispatcher) PublishSync(ctx context.Context, event *Event) []PublishResult {
	d.mu.RLock()
	if d.stopped {
		d.mu.RUnlock()
		return nil
	}
	d.mu.RUnlock()

	resultCh := make(chan []PublishResult, 1)
	select {
	case d.eventCh <- &dispatchItem{event: event, result: resultCh}:
	case <-ctx.Done():
		return nil
	}

	select {
	case results := <-resultCh:
		return results
	case <-ctx.Done():
		return nil
	}
}

// Unsubscribe removes a subscriber by event type and subscription ID.
func (d *Dispatcher) Unsubscribe(eventType EventType, subID SubscriptionID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	subs, ok := d.subscribers[eventType]
	if !ok {
		return false
	}

	for i, sub := range subs {
		if sub.id == subID {
			d.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			return true
		}
	}
	return false
}

// Stop gracefully shuts down the dispatcher, waiting for in-flight events
// to be processed.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	d.mu.Unlock()

	close(d.quit)
	d.wg.Wait()
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.quit:
			return
		case item := <-d.eventCh:
			d.process(item)
		}
	}
}

func (d *Dispatcher) process(item *dispatchItem) {
	d.mu.RLock()
	subs := d.subscribers[item.event.Type]
	// Copy to avoid holding lock during handler execution
	subsCopy := make([]*subscriber, len(subs))
	copy(subsCopy, subs)
	d.mu.RUnlock()

	if item.result != nil {
		// Synchronous publish
		results := make([]PublishResult, 0, len(subsCopy))
		for _, sub := range subsCopy {
			if sub.filter != nil && !sub.filter(item.event) {
				continue
			}
			result := PublishResult{HandlerName: sub.handler.Name()}
			result.Error = d.executeWithRetry(sub, item.event)
			results = append(results, result)
		}
		item.result <- results
	} else {
		// Async publish
		for _, sub := range subsCopy {
			if sub.filter != nil && !sub.filter(item.event) {
				continue
			}
			if err := d.executeWithRetry(sub, item.event); err != nil {
				d.sendToDeadLetter(item.event)
				// Continue processing other subscribers
			}
		}
	}
}

func (d *Dispatcher) executeWithRetry(sub *subscriber, event *Event) error {
	var lastErr error
	maxAttempts := d.config.RetryAttempts + 1 // +1 for initial attempt

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := sub.handler.Handle(ctx, event)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < maxAttempts-1 && d.config.RetryDelay > 0 {
			time.Sleep(d.config.RetryDelay)
		}
	}
	return lastErr
}

func (d *Dispatcher) sendToDeadLetter(event *Event) {
	if d.deadLetter == nil {
		return
	}
	select {
	case d.deadLetter <- event:
	default:
		// Dead letter queue is full, drop oldest
		select {
		case <-d.deadLetter:
		default:
		}
		select {
		case d.deadLetter <- event:
		default:
		}
	}
}
