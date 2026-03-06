package harness

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// State represents the current state of the system
type State string

const (
	StateStopped      State = "stopped"
	StateInitializing State = "initializing"
	StateReady        State = "ready"
	StateExecuting    State = "executing"
	StateStopping     State = "stopping"
	StateError        State = "error"
)

// StateManager manages system state transitions
type StateManager struct {
	mu          sync.RWMutex
	current     State
	transitions map[State][]State
	ch          chan StateChange
	ctx         context.Context
	cancel      context.CancelFunc
	history     []StateChange
	maxHistory  int
}

// StateChange represents a state transition
type StateChange struct {
	From     State
	To       State
	Time     time.Time
	Reason   string
	Metadata map[string]interface{}
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &StateManager{
		current:     StateStopped,
		transitions: make(map[State][]State),
		ch:          make(chan StateChange, 100),
		ctx:         ctx,
		cancel:      cancel,
		history:     make([]StateChange, 0),
		maxHistory:  100,
	}

	sm.setupTransitions()

	go sm.processChanges()

	return sm
}

func (sm *StateManager) setupTransitions() {
	sm.transitions[StateStopped] = []State{StateInitializing}
	sm.transitions[StateInitializing] = []State{StateReady, StateError}
	sm.transitions[StateReady] = []State{StateExecuting, StateStopping}
	sm.transitions[StateExecuting] = []State{StateReady, StateError}
	sm.transitions[StateStopping] = []State{StateStopped}
	sm.transitions[StateError] = []State{StateStopped}
}

func (sm *StateManager) processChanges() {
	for {
		select {
		case change := <-sm.ch:
			sm.mu.Lock()
			sm.history = append(sm.history, change)
			if len(sm.history) > sm.maxHistory {
				sm.history = sm.history[1:]
			}
			sm.mu.Unlock()

		case <-sm.ctx.Done():
			return
		}
	}
}

// Current returns the current state
func (sm *StateManager) Current() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// Transition attempts to transition to a new state
func (sm *StateManager) Transition(to State, reason string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	from := sm.current

	allowed, ok := sm.transitions[from]
	if !ok {
		return fmt.Errorf("unknown current state: %s", from)
	}

	valid := false
	for _, s := range allowed {
		if s == to {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	sm.current = to

	change := StateChange{
		From:     from,
		To:       to,
		Time:     time.Now(),
		Reason:   reason,
		Metadata: make(map[string]interface{}),
	}

	select {
	case sm.ch <- change:
	default:
	}

	return nil
}

// MustTransition transitions to the new state or panics
func (sm *StateManager) MustTransition(t *testing.T, to State, reason string) {
	err := sm.Transition(to, reason)
	require.NoError(t, err, "State transition failed")
}

// History returns the state change history
func (sm *StateManager) History() []StateChange {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return make([]StateChange, len(sm.history))
}

// Stop stops the state manager
func (sm *StateManager) Stop() {
	sm.cancel()
}

// IsState returns true if the current state matches
func (sm *StateManager) IsState(state State) bool {
	return sm.Current() == state
}

// CanTransition returns true if a transition is valid
func (sm *StateManager) CanTransition(to State) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowed, ok := sm.transitions[sm.current]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}

	return false
}

// Reset resets the state
func (sm *StateManager) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.current = StateStopped
	sm.history = make([]StateChange, 0)
}

// StateWaiter waits for state changes
type StateWaiter struct {
	sm     *StateManager
	state  State
	ctx    context.Context
	cancel context.CancelFunc
}

// WaitForState waits until the specified state is reached
func (sm *StateManager) WaitForState(ctx context.Context, state State) <-chan State {
	sm.mu.RLock()
	if sm.current == state {
		sm.mu.RUnlock()
		ch := make(chan State, 1)
		ch <- state
		return ch
	}
	sm.mu.RUnlock()

	subctx, cancel := context.WithCancel(ctx)
	ch := make(chan State, 1)

	go func() {
		defer cancel()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-subctx.Done():
				return

			case <-ticker.C:
				if sm.IsState(state) {
					ch <- state
					return
				}
			}
		}
	}()

	return ch
}

// StateCounter counts state transitions
type StateCounter struct {
	mu      sync.Mutex
	counts  map[State]int
	started time.Time
}

// NewStateCounter creates a new state counter
func NewStateCounter() *StateCounter {
	return &StateCounter{
		counts:  make(map[State]int),
		started: time.Now(),
	}
}

// Increment increments the count for a state
func (sc *StateCounter) Increment(state State) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.counts[state]++
}

// Count returns the count for a state
func (sc *StateCounter) Count(state State) int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.counts[state]
}

// Total returns the total count
func (sc *StateCounter) Total() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	total := 0
	for _, count := range sc.counts {
		total += count
	}
	return total
}

// Reset resets all counts
func (sc *StateCounter) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.counts = make(map[State]int)
}

// Snapshot returns a snapshot of counts
func (sc *StateCounter) Snapshot() map[State]int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	snap := make(map[State]int)
	for state, count := range sc.counts {
		snap[state] = count
	}
	return snap
}

// StateRecorder records state change events
type StateRecorder struct {
	mu       sync.Mutex
	events   []StateEvent
	maxSize  int
	onRecord func(StateEvent)
}

// StateEvent represents a state event
type StateEvent struct {
	Type      string
	State     State
	Timestamp time.Time
	Data      map[string]interface{}
}

// NewStateRecorder creates a new state recorder
func NewStateRecorder(maxSize int) *StateRecorder {
	return &StateRecorder{
		events:  make([]StateEvent, 0),
		maxSize: maxSize,
	}
}

// Record records a state event
func (sr *StateRecorder) Record(event StateEvent) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	event.Timestamp = time.Now()
	sr.events = append(sr.events, event)

	if len(sr.events) > sr.maxSize {
		sr.events = sr.events[1:]
	}

	if sr.onRecord != nil {
		sr.onRecord(event)
	}
}

// Events returns all events
func (sr *StateRecorder) Events() []StateEvent {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return make([]StateEvent, len(sr.events))
}

// Clear clears all events
func (sr *StateRecorder) Clear() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.events = make([]StateEvent, 0)
}

// OnRecord sets the callback for recording
func (sr *StateRecorder) OnRecord(callback func(StateEvent)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.onRecord = callback
}

// Filter filters events by type
func (sr *StateRecorder) Filter(eventType string) []StateEvent {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	filtered := make([]StateEvent, 0)
	for _, event := range sr.events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// Duration calculates duration between two state changes
func Duration(sm *StateManager, from, to State) time.Duration {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var fromTime time.Time
	var toTime time.Time

	for _, change := range sm.history {
		if change.From == from && change.To == StateReady {
			fromTime = change.Time
		}
		if change.From == StateReady && change.To == to {
			toTime = change.Time
			break
		}
	}

	if fromTime.IsZero() || toTime.IsZero() {
		return 0
	}

	return toTime.Sub(fromTime)
}

// StateValidator validates state transitions
type StateValidator struct {
	enabled bool
	rules   map[string]func(from, to State) bool
}

// NewStateValidator creates a new state validator
func NewStateValidator() *StateValidator {
	return &StateValidator{
		enabled: true,
		rules:   make(map[string]func(from, to State) bool),
	}
}

// AddRule adds a validation rule
func (sv *StateValidator) AddRule(name string, rule func(from, to State) bool) {
	sv.rules[name] = rule
}

// Validate validates a transition
func (sv *StateValidator) Validate(from, to State) bool {
	if !sv.enabled {
		return true
	}

	for _, rule := range sv.rules {
		if !rule(from, to) {
			return false
		}
	}

	return true
}

// Enable enables validation
func (sv *StateValidator) Enable() {
	sv.enabled = true
}

// Disable disables validation
func (sv *StateValidator) Disable() {
	sv.enabled = false
}

// StateSnapshot captures a snapshot of system state
type StateSnapshot struct {
	Timestamp   time.Time
	State       State
	Metadata    map[string]interface{}
	Metrics     map[string]interface{}
	Transitions []StateChange
}

// Capture captures a state snapshot
func (sm *StateManager) Capture() StateSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snap := StateSnapshot{
		Timestamp:   time.Now(),
		State:       sm.current,
		Metadata:    make(map[string]interface{}),
		Metrics:     make(map[string]interface{}),
		Transitions: make([]StateChange, len(sm.history)),
	}
	copy(sm.history, snap.Transitions)

	return snap
}

func copy(src []StateChange, dst []StateChange) {
	for i, change := range src {
		dst[i] = StateChange{
			From:     change.From,
			To:       change.To,
			Time:     change.Time,
			Reason:   change.Reason,
			Metadata: change.Metadata,
		}
	}
}
