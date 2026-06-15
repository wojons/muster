package events

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// SubscriptionID is a unique identifier for a subscription.
type SubscriptionID string

var subIDCounter int64

func generateSubscriptionID() SubscriptionID {
	return SubscriptionID(fmt.Sprintf("sub-%d", atomic.AddInt64(&subIDCounter, 1)))
}

// DispatcherConfig holds configuration for the event dispatcher.
type DispatcherConfig struct {
	WorkerCount         int
	BufferSize          int
	RetryAttempts       int
	RetryDelay          time.Duration
	EnableDeadLetter    bool
	DeadLetterQueueSize int
}

// DefaultDispatcherConfig provides sensible defaults for the dispatcher.
var DefaultDispatcherConfig = DispatcherConfig{
	WorkerCount:         4,
	BufferSize:          1000,
	RetryAttempts:       0,
	RetryDelay:          100 * time.Millisecond,
	EnableDeadLetter:    false,
	DeadLetterQueueSize: 100,
}

// SubscribeOption is a functional option for Subscribe.
type SubscribeOption func(*subscriberEntry)

// WithPriority sets the priority for a subscription. Higher priority handlers
// are invoked first.
func WithPriority(p int) SubscribeOption {
	return func(se *subscriberEntry) {
		se.priority = p
	}
}

// WithFilter sets a filter function for a subscription. The handler is only
// invoked when the filter returns true. A nil filter is treated as pass-through.
func WithFilter(f func(*Event) bool) SubscribeOption {
	return func(se *subscriberEntry) {
		se.filter = f
	}
}

// PublishResult holds the result of a single handler execution.
type PublishResult struct {
	HandlerName string
	Error       error
}

// Stats holds runtime counters for the dispatcher.
type Stats struct {
	EventsPublished int64
	EventsFailed    int64
	EventsInFlight  int64
}

// subscriberEntry represents a single subscription.
type subscriberEntry struct {
	id       SubscriptionID
	handler  Handler
	priority int
	filter   func(*Event) bool
}

// workItem represents a unit of work for the worker pool.
type workItem struct {
	event   *Event
	sync    bool
	doneCh  chan struct{}
	results []PublishResult
	flush   bool
	flushCh chan struct{}
}

// Dispatcher is a worker-pool-based event dispatcher.
type Dispatcher struct {
	config      DispatcherConfig
	mu          sync.RWMutex
	subscribers map[EventType][]*subscriberEntry
	workCh      chan *workItem
	quitCh      chan struct{}
	stopped     bool
	deadLetter  chan *Event
	workerWg    sync.WaitGroup
	stats       Stats
}

// NewDispatcher creates and starts a new Dispatcher with the given configuration.
func NewDispatcher(cfg DispatcherConfig) (*Dispatcher, error) {
	// If the config is completely zero-valued, apply all defaults.
	// If no config was provided, apply all defaults from DefaultDispatcherConfig.
	if cfg == (DispatcherConfig{}) {
		cfg = DefaultDispatcherConfig
	} else {
		// Negative WorkerCount falls back to the default; zero defaults to 1
		// (the minimum viable number of workers).
		if cfg.WorkerCount < 0 {
			cfg.WorkerCount = DefaultDispatcherConfig.WorkerCount
		} else if cfg.WorkerCount == 0 {
			cfg.WorkerCount = 1
		}
		if cfg.BufferSize <= 0 {
			cfg.BufferSize = DefaultDispatcherConfig.BufferSize
		}
		if cfg.RetryDelay <= 0 {
			cfg.RetryDelay = DefaultDispatcherConfig.RetryDelay
		}
	}

	// Default DeadLetterQueueSize regardless of whether DLQ is enabled.
	if cfg.DeadLetterQueueSize <= 0 {
		cfg.DeadLetterQueueSize = DefaultDispatcherConfig.DeadLetterQueueSize
	}

	// Channel buffer holds the events not yet picked up by a worker. Since each
	// active worker occupies one "slot" (processing one event) toward the
	// BufferSize total, the channel capacity is BufferSize minus WorkerCount,
	// with a minimum of 1 to avoid an unbuffered channel.
	chCap := cfg.BufferSize - cfg.WorkerCount
	if chCap < 1 {
		chCap = 1
	}

	d := &Dispatcher{
		config:      cfg,
		subscribers: make(map[EventType][]*subscriberEntry),
		workCh:      make(chan *workItem, chCap),
		quitCh:      make(chan struct{}),
	}

	if cfg.EnableDeadLetter && cfg.DeadLetterQueueSize > 0 {
		d.deadLetter = make(chan *Event, cfg.DeadLetterQueueSize)
	}

	d.startWorkers()
	return d, nil
}

func (d *Dispatcher) startWorkers() {
	for i := 0; i < d.config.WorkerCount; i++ {
		d.workerWg.Add(1)
		go d.worker()
	}
}

func (d *Dispatcher) worker() {
	defer d.workerWg.Done()
	for {
		select {
		case <-d.quitCh:
			return
		case wi := <-d.workCh:
			if wi.flush {
				close(wi.flushCh)
				continue
			}
			// If the dispatcher has been stopped since this item was
			// enqueued, discard it to avoid calling handlers after Stop().
			if d.isStopped() {
				return
			}
			d.processWorkItem(wi)
		}
	}
}

func (d *Dispatcher) processWorkItem(wi *workItem) {
	subs := d.getSubscribers(wi.event.Type)

	// Sort by priority descending so highest-priority handlers run first.
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].priority > subs[j].priority
	})

	results := make([]PublishResult, 0, len(subs))
	for _, sub := range subs {
		if sub.handler == nil {
			continue
		}
		if sub.filter != nil && !sub.filter(wi.event) {
			continue
		}
		if !sub.handler.Supports(wi.event.Type) {
			continue
		}
		result := d.dispatchWithRetry(sub, wi.event)
		results = append(results, result)
	}

	if wi.sync {
		wi.results = results
		close(wi.doneCh)
	}
}

func (d *Dispatcher) dispatchWithRetry(sub *subscriberEntry, event *Event) PublishResult {
	atomic.AddInt64(&d.stats.EventsPublished, 1)
	var lastErr error
	attempts := 1 + d.config.RetryAttempts

	for i := 0; i < attempts; i++ {
		err := sub.handler.Handle(context.Background(), event)
		if err == nil {
			return PublishResult{HandlerName: sub.handler.Name()}
		}
		lastErr = err
		atomic.AddInt64(&d.stats.EventsFailed, 1)
		if i < attempts-1 && d.config.RetryDelay > 0 {
			time.Sleep(d.config.RetryDelay)
		}
	}

	// All attempts failed — route to dead-letter queue if enabled.
	if d.config.EnableDeadLetter && d.deadLetter != nil {
		select {
		case d.deadLetter <- event:
		default:
			// Circular buffer: drop oldest, add new.
			<-d.deadLetter
			d.deadLetter <- event
		}
	}

	return PublishResult{HandlerName: sub.handler.Name(), Error: lastErr}
}

// Subscribe registers a handler for the given event type and returns a
// SubscriptionID that can be used to unsubscribe. Returns an empty ID if
// handler is nil.
func (d *Dispatcher) Subscribe(eventType EventType, handler Handler, opts ...SubscribeOption) SubscriptionID {
	if handler == nil {
		return SubscriptionID("")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	entry := &subscriberEntry{
		id:      generateSubscriptionID(),
		handler: handler,
	}

	for _, opt := range opts {
		opt(entry)
	}

	d.subscribers[eventType] = append(d.subscribers[eventType], entry)
	return entry.id
}

// Unsubscribe removes a subscription for the given event type. Returns true
// if the subscription was found and removed.
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

// Publish publishes an event asynchronously (non-blocking when the buffer has
// room). Returns an error if the dispatcher is stopped or the context is
// cancelled while waiting to enqueue.
func (d *Dispatcher) Publish(ctx context.Context, event *Event) error {
	if d.isStopped() {
		return errors.New("dispatcher is stopped")
	}

	select {
	case d.workCh <- &workItem{event: event}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishSync publishes an event synchronously. If there are pending events
// in the buffer, it first waits for the buffer to drain (acting as a barrier),
// then dispatches directly to subscribers in the calling goroutine. Returns
// nil if the dispatcher is stopped or the context is cancelled.
func (d *Dispatcher) PublishSync(ctx context.Context, event *Event) []PublishResult {
	if d.isStopped() {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}

	// Always send a flush barrier through the worker queue so that any
	// previously published async events are processed before we continue.
	// If there were pending events we act as a drain barrier and return nil.
	hadPending := len(d.workCh) > 0

	barrier := make(chan struct{})
	select {
	case d.workCh <- &workItem{flush: true, flushCh: barrier}:
		select {
		case <-barrier:
			// Queue drained.
		case <-ctx.Done():
			return nil
		}
	case <-ctx.Done():
		return nil
	}

	if hadPending {
		// There were pending events — we only acted as a drain barrier.
		return nil
	}

	// No pending events before the flush — directly dispatch to subscribers
	// in this goroutine.
	subs := d.getSubscribers(event.Type)
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].priority > subs[j].priority
	})

	results := make([]PublishResult, 0, len(subs))
	for _, sub := range subs {
		if sub.handler == nil {
			continue
		}
		if sub.filter != nil && !sub.filter(event) {
			continue
		}
		if !sub.handler.Supports(event.Type) {
			continue
		}
		result := d.dispatchWithRetry(sub, event)
		results = append(results, result)
	}
	return results
}

// Stop gracefully shuts down the dispatcher. It signals all workers to stop
// and waits for them to finish processing the current work item.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	d.mu.Unlock()

	close(d.quitCh)
	d.workerWg.Wait()
}

// GetStats returns a snapshot of the dispatcher's runtime counters.
func (d *Dispatcher) GetStats() *Stats {
	s := Stats{
		EventsPublished: atomic.LoadInt64(&d.stats.EventsPublished),
		EventsFailed:    atomic.LoadInt64(&d.stats.EventsFailed),
		EventsInFlight:  atomic.LoadInt64(&d.stats.EventsInFlight),
	}
	return &s
}

func (d *Dispatcher) isStopped() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stopped
}

func (d *Dispatcher) getSubscribers(eventType EventType) []*subscriberEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	subs := d.subscribers[eventType]
	result := make([]*subscriberEntry, len(subs))
	copy(result, subs)
	return result
}
