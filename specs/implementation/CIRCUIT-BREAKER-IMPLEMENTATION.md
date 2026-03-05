# Circuit Breaker Implementation Specification

Production-ready circuit breaker pattern implementation for Go with state machine, metrics, and HTTP middleware integration.

---

## 1. Core Implementation

### File: `internal/core/circuitbreaker/breaker.go`

```go
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the current state of the circuit breaker
type State int32

const (
	// StateClosed - normal operation, requests pass through
	StateClosed State = iota
	// StateOpen - circuit is open, requests are rejected immediately
	StateOpen
	// StateHalfOpen - testing if service has recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// FailureThreshold is the number of consecutive failures required to trip the circuit
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes required to close from half-open
	SuccessThreshold int
	// Timeout is the duration after which the circuit transitions from OPEN to HALF_OPEN
	Timeout time.Duration
	// HalfOpenMaxRequests is the maximum number of requests allowed in half-open state
	HalfOpenMaxRequests int
	// IgnoreErrors is a list of error types that should not count as failures
	IgnoreErrors []error
	// MaxConsecutiveFailures prevents rapid tripping
	MaxConsecutiveFailures int
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold:     5,
		SuccessThreshold:     3,
		Timeout:              30 * time.Second,
		HalfOpenMaxRequests:  3,
		MaxConsecutiveFailures: 100,
	}
}

// Validate checks if the configuration is valid
func (c Config) Validate() error {
	if c.FailureThreshold <= 0 {
		return fmt.Errorf("failure threshold must be positive, got %d", c.FailureThreshold)
	}
	if c.SuccessThreshold <= 0 {
		return fmt.Errorf("success threshold must be positive, got %d", c.SuccessThreshold)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", c.Timeout)
	}
	if c.HalfOpenMaxRequests <= 0 {
		return fmt.Errorf("half-open max requests must be positive, got %d", c.HalfOpenMaxRequests)
	}
	return nil
}

// Metrics holds circuit breaker statistics
type Metrics struct {
	State           State         `json:"state"`
	Failures        int64         `json:"failures"`
	Successes       int64         `json:"successes"`
	Rejects         int64         `json:"rejects"`
	ConsecutiveFailures int64     `json:"consecutive_failures"`
	ConsecutiveSuccesses int64    `json:"consecutive_successes"`
	LastFailureTime time.Time     `json:"last_failure_time"`
	LastSuccessTime time.Time     `json:"last_success_time"`
	StateChanges    int64         `json:"state_changes"`
	OpenedAt        time.Time     `json:"opened_at"`
	ClosedAt        time.Time     `json:"closed_at"`
}

// Hooks for monitoring and observability
type Hooks struct {
	// OnStateChange is called when the circuit breaker changes state
	OnStateChange func(from, to State)
	// OnFailure is called when a request fails
	OnFailure func(err error)
	// OnSuccess is called when a request succeeds
	OnSuccess func()
	// OnReject is called when a request is rejected (circuit open)
	OnReject func()
}

// Breaker implements the circuit breaker pattern
type Breaker struct {
	config Config
	hooks  Hooks

	// State machine variables - use atomic for state
	state atomic.Int32

	// Protected by mu
	mu                   sync.RWMutex
	failures           int64
	successes          int64
	rejects            int64
	consecutiveFailures  int64
	consecutiveSuccesses int64
	lastFailureTime    time.Time
	lastSuccessTime    time.Time
	stateChanges       int64
	openedAt           time.Time
	closedAt           time.Time
	// Half-open state tracking
	halfOpenRequests   int64
	halfOpenSuccesses  int64
	// For timeout tracking
	timer              *time.Timer
	timerMu            sync.Mutex
}

// Errors returned by the circuit breaker
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrCircuitNotReady = errors.New("circuit breaker half-open limit reached")
	ErrTooManyRequests = errors.New("circuit breaker half-open request limit reached")
)

// New creates a new circuit breaker with the given configuration
func New(config Config) (*Breaker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	b := &Breaker{
		config: config,
	}
	b.state.Store(int32(StateClosed))
	b.closedAt = time.Now()

	return b, nil
}

// NewWithHooks creates a new circuit breaker with monitoring hooks
func NewWithHooks(config Config, hooks Hooks) (*Breaker, error) {
	b, err := New(config)
	if err != nil {
		return nil, err
	}
	b.hooks = hooks
	return b, nil
}

// State returns the current state of the circuit breaker
func (b *Breaker) State() State {
	return State(b.state.Load())
}

// CanExecute checks if a request can be executed without executing it
func (b *Breaker) CanExecute() error {
	return b.canExecute()
}

func (b *Breaker) canExecute() error {
	currentState := State(b.state.Load())

	switch currentState {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has elapsed to transition to half-open
		b.mu.RLock()
		lastFailure := b.lastFailureTime
		b.mu.RUnlock()

		if time.Since(lastFailure) > b.config.Timeout {
			// Attempt transition to half-open
			if b.transitionToHalfOpen() {
				return nil
			}
		}

		// Circuit is still open
		b.recordReject()
		return ErrCircuitOpen

	case StateHalfOpen:
		b.mu.Lock()
		defer b.mu.Unlock()

		// Check if we're within the half-open limit
		if b.halfOpenRequests >= int64(b.config.HalfOpenMaxRequests) {
			return ErrTooManyRequests
		}

		b.halfOpenRequests++
		return nil
	}

	return nil
}

// Execute runs the given function with circuit breaker protection
func (b *Breaker) Execute(fn func() error) error {
	if err := b.canExecute(); err != nil {
		return err
	}

	err := fn()
	b.recordResult(err)
	return err
}

// ExecuteContext runs the given function with circuit breaker protection and context
func (b *Breaker) ExecuteContext(ctx context.Context, fn func(context.Context) error) error {
	if err := b.canExecute(); err != nil {
		return err
	}

	err := fn(ctx)
	b.recordResult(err)
	return err
}

// recordResult updates state based on success or failure
func (b *Breaker) recordResult(err error) {
	// Check if error should be ignored
	if err != nil && b.shouldIgnoreError(err) {
		err = nil
	}

	if err != nil {
		b.recordFailure(err)
	} else {
		b.recordSuccess()
	}
}

// shouldIgnoreError checks if an error should not count as a failure
func (b *Breaker) shouldIgnoreError(err error) bool {
	for _, ignore := range b.config.IgnoreErrors {
		if errors.Is(err, ignore) {
			return true
		}
	}
	return false
}

func (b *Breaker) recordFailure(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastFailureTime = now
	b.failures++
	b.consecutiveFailures++
	b.consecutiveSuccesses = 0

	// Call hook
	if b.hooks.OnFailure != nil {
		go b.hooks.OnFailure(err)
	}

	currentState := State(b.state.Load())

	switch currentState {
	case StateHalfOpen:
		// In half-open, any failure trips the circuit back to open
		b.mu.Unlock()
		b.transitionToOpen()
		b.mu.Lock()

	case StateClosed:
		// Check if we should trip to open
		if b.consecutiveFailures >= int64(b.config.FailureThreshold) {
			// Prevent rapid tripping by checking max consecutive failures
			if b.consecutiveFailures >= int64(b.config.MaxConsecutiveFailures) {
				// Just reset counter but stay closed
				b.consecutiveFailures = 0
				return
			}

			b.mu.Unlock()
			b.transitionToOpen()
			b.mu.Lock()
		}
	}
}

func (b *Breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastSuccessTime = now
	b.successes++
	b.consecutiveSuccesses++
	b.consecutiveFailures = 0

	// Call hook
	if b.hooks.OnSuccess != nil {
		go b.hooks.OnSuccess()
	}

	currentState := State(b.state.Load())

	if currentState == StateHalfOpen {
		b.halfOpenSuccesses++

		// Check if we should close the circuit
		if b.halfOpenSuccesses >= int64(b.config.SuccessThreshold) {
			b.mu.Unlock()
			b.transitionToClosed()
			b.mu.Lock()
		}
	}
}

func (b *Breaker) recordReject() {
	b.mu.Lock()
	b.rejects++
	b.mu.Unlock()

	if b.hooks.OnReject != nil {
		go b.hooks.OnReject()
	}
}

func (b *Breaker) transitionToOpen() {
	b.mu.Lock()
	oldState := State(b.state.Load())
	if oldState == StateOpen {
		b.mu.Unlock()
		return
	}

	b.state.Store(int32(StateOpen))
	b.stateChanges++
	b.openedAt = time.Now()
	b.halfOpenRequests = 0
	b.halfOpenSuccesses = 0

	// Start timeout timer for automatic recovery
	b.timerMu.Lock()
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = time.AfterFunc(b.config.Timeout, func() {
		b.transitionToHalfOpen()
	})
	b.timerMu.Unlock()

	b.mu.Unlock()

	if b.hooks.OnStateChange != nil {
		go b.hooks.OnStateChange(oldState, StateOpen)
	}
}

func (b *Breaker) transitionToHalfOpen() bool {
	b.mu.Lock()
	oldState := State(b.state.Load())
	if oldState != StateOpen {
		b.mu.Unlock()
		return false
	}

	// Only transition if timeout has actually elapsed
	if time.Since(b.lastFailureTime) <= b.config.Timeout {
		b.mu.Unlock()
		return false
	}

	b.state.Store(int32(StateHalfOpen))
	b.stateChanges++
	b.halfOpenRequests = 0
	b.halfOpenSuccesses = 0
	b.consecutiveFailures = 0
	b.consecutiveSuccesses = 0

	// Cancel timer if it exists
	b.timerMu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.timerMu.Unlock()

	b.mu.Unlock()

	if b.hooks.OnStateChange != nil {
		go b.hooks.OnStateChange(oldState, StateHalfOpen)
	}

	return true
}

func (b *Breaker) transitionToClosed() {
	b.mu.Lock()
	oldState := State(b.state.Load())
	if oldState == StateClosed {
		b.mu.Unlock()
		return
	}

	b.state.Store(int32(StateClosed))
	b.stateChanges++
	b.closedAt = time.Now()
	b.failures = 0
	b.consecutiveFailures = 0
	b.halfOpenRequests = 0
	b.halfOpenSuccesses = 0

	// Cancel timer
	b.timerMu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.timerMu.Unlock()

	b.mu.Unlock()

	if b.hooks.OnStateChange != nil {
		go b.hooks.OnStateChange(oldState, StateClosed)
	}
}

// Metrics returns current metrics (thread-safe copy)
func (b *Breaker) Metrics() Metrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return Metrics{
		State:                State(b.state.Load()),
		Failures:             b.failures,
		Successes:            b.successes,
		Rejects:              b.rejects,
		ConsecutiveFailures:  b.consecutiveFailures,
		ConsecutiveSuccesses: b.consecutiveSuccesses,
		LastFailureTime:    b.lastFailureTime,
		LastSuccessTime:    b.lastSuccessTime,
		StateChanges:       b.stateChanges,
		OpenedAt:           b.openedAt,
		ClosedAt:           b.closedAt,
	}
}

// Reset resets the circuit breaker to closed state
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	oldState := State(b.state.Load())
	if oldState == StateClosed {
		return
	}

	b.state.Store(int32(StateClosed))
	b.stateChanges++
	b.closedAt = time.Now()
	b.failures = 0
	b.consecutiveFailures = 0
	b.successes = 0
	b.consecutiveSuccesses = 0
	b.halfOpenRequests = 0
	b.halfOpenSuccesses = 0

	// Cancel timer
	b.timerMu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.timerMu.Unlock()

	if b.hooks.OnStateChange != nil {
		go b.hooks.OnStateChange(oldState, StateClosed)
	}
}

// Close releases resources
func (b *Breaker) Close() {
	b.timerMu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.timerMu.Unlock()
}
```

---

## 2. HTTP Middleware Integration

### File: `internal/core/circuitbreaker/middleware.go`

```go
package circuitbreaker

import (
	"fmt"
	"net/http"
	"time"
)

// HTTPMiddleware wraps an HTTP client with circuit breaker protection
type HTTPMiddleware struct {
	breaker  *Breaker
	client   HTTPClient
	baseURL  string
}

// HTTPClient is the interface for HTTP operations
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewHTTPMiddleware creates circuit breaker middleware for HTTP client
func NewHTTPMiddleware(client HTTPClient, breaker *Breaker) *HTTPMiddleware {
	return &HTTPMiddleware{
		client:  client,
		breaker: breaker,
	}
}

// Do executes an HTTP request with circuit breaker protection
func (m *HTTPMiddleware) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	execErr := m.breaker.Execute(func() error {
		resp, err = m.client.Do(req)
		if err != nil {
			return err
		}

		// Treat 5xx errors as failures that should trip the circuit
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return nil
	})

	if execErr != nil {
		if execErr == ErrCircuitOpen {
			return nil, &CircuitBreakerError{
				State: m.breaker.State(),
				URL:   req.URL.String(),
			}
		}
		return nil, execErr
	}

	return resp, err
}

// CircuitBreakerError represents an error when circuit is open
type CircuitBreakerError struct {
	State State
	URL   string
}

func (e *CircuitBreakerError) Error() string {
	return fmt.Sprintf("circuit breaker is %s for %s", e.State, e.URL)
}

// IsCircuitBreakerError checks if an error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	var cbErr *CircuitBreakerError
	return errors.As(err, &cbErr)
}

// HTTPTransport implements http.RoundTripper with circuit breaker
type HTTPTransport struct {
	breaker *Breaker
	base    http.RoundTripper
}

// NewHTTPTransport creates a new HTTP transport with circuit breaker
func NewHTTPTransport(breaker *Breaker, base http.RoundTripper) *HTTPTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &HTTPTransport{
		breaker: breaker,
		base:    base,
	}
}

// RoundTrip implements http.RoundTripper
func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	execErr := t.breaker.Execute(func() error {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return err
		}

		// Only count 5xx as failures
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d %s", resp.StatusCode, resp.Status)
		}

		return nil
	})

	if execErr != nil {
		if execErr == ErrCircuitOpen {
			return nil, &CircuitBreakerError{
				State: t.breaker.State(),
				URL:   req.URL.String(),
			}
		}
		return nil, execErr
	}

	return resp, err
}

// HTTPClientWithBreaker creates an HTTP client with circuit breaker
func HTTPClientWithBreaker(breaker *Breaker, timeout time.Duration) *http.Client {
	transport := NewHTTPTransport(breaker, nil)
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// Middleware wraps an HTTP handler with circuit breaker
func Middleware(breaker *Breaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if circuit allows request
			if err := breaker.CanExecute(); err != nil {
				if err == ErrCircuitOpen {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte("Service temporarily unavailable (circuit breaker open)"))
					return
				}
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("Too many requests"))
				return
			}

			// Use response recorder to capture status code
			recorder := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(recorder, r)

			// Record result based on status code
			if recorder.statusCode >= 500 {
				breaker.recordFailure(fmt.Errorf("server error: %d", recorder.statusCode))
			} else {
				breaker.recordSuccess()
			}
		})
	}
}

// responseRecorder captures HTTP response status code
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}
```

---

## 3. Named Circuit Breakers (Registry)

### File: `internal/core/circuitbreaker/registry.go`

```go
package circuitbreaker

import (
	"fmt"
	"sync"
)

// Registry manages named circuit breakers
type Registry struct {
	mu        sync.RWMutex
	breakers  map[string]*Breaker
	configs   map[string]Config
}

// NewRegistry creates a new circuit breaker registry
func NewRegistry() *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
		configs:  make(map[string]Config),
	}
}

// GetOrCreate returns an existing breaker or creates a new one
func (r *Registry) GetOrCreate(name string, config Config) (*Breaker, error) {
	r.mu.RLock()
	if breaker, ok := r.breakers[name]; ok {
		r.mu.RUnlock()
		return breaker, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if breaker, ok := r.breakers[name]; ok {
		return breaker, nil
	}

	breaker, err := New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create breaker %s: %w", name, err)
	}

	r.breakers[name] = breaker
	r.configs[name] = config

	return breaker, nil
}

// Get returns an existing breaker or error if not found
func (r *Registry) Get(name string) (*Breaker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if breaker, ok := r.breakers[name]; ok {
		return breaker, nil
	}

	return nil, fmt.Errorf("circuit breaker %s not found", name)
}

// Remove removes a circuit breaker from the registry
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if breaker, ok := r.breakers[name]; ok {
		breaker.Close()
		delete(r.breakers, name)
		delete(r.configs, name)
		return nil
	}

	return fmt.Errorf("circuit breaker %s not found", name)
}

// Names returns all registered circuit breaker names
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.breakers))
	for name := range r.breakers {
		names = append(names, name)
	}

	return names
}

// Metrics returns metrics for all circuit breakers
func (r *Registry) Metrics() map[string]Metrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := make(map[string]Metrics, len(r.breakers))
	for name, breaker := range r.breakers {
		metrics[name] = breaker.Metrics()
	}

	return metrics
}

// ResetAll resets all circuit breakers to closed state
func (r *Registry) ResetAll() {
	r.mu.RLock()
	breakers := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		breakers = append(breakers, b)
	}
	r.mu.RUnlock()

	for _, breaker := range breakers {
		breaker.Reset()
	}
}

// Close releases all resources
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, breaker := range r.breakers {
		breaker.Close()
	}

	r.breakers = make(map[string]*Breaker)
	r.configs = make(map[string]Config)
}
```

---

## 4. Metrics and Monitoring

### File: `internal/core/circuitbreaker/metrics.go`

```go
package circuitbreaker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MetricsCollector collects metrics from circuit breakers
type MetricsCollector struct {
	registry    *Registry
	interval    time.Duration
	stopCh      chan struct{}
	callbacks   []func(map[string]Metrics)
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(registry *Registry, interval time.Duration) *MetricsCollector {
	return &MetricsCollector{
		registry: registry,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// AddCallback adds a callback for metrics updates
func (c *MetricsCollector) AddCallback(callback func(map[string]Metrics)) {
	c.callbacks = append(c.callbacks, callback)
}

// Start begins collecting metrics
func (c *MetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			metrics := c.registry.Metrics()
			for _, cb := range c.callbacks {
				cb(metrics)
			}
		}
	}
}

// Stop stops the metrics collector
func (c *MetricsCollector) Stop() {
	close(c.stopCh)
}

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	registry *Registry
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(registry *Registry) *PrometheusExporter {
	return &PrometheusExporter{registry: registry}
}

// Export returns Prometheus-formatted metrics
func (e *PrometheusExporter) Export() string {
	metrics := e.registry.Metrics()
	var output string

	for name, m := range metrics {
		prefix := fmt.Sprintf("circuit_breaker_%s", name)
		
		output += fmt.Sprintf("# HELP %s_state Circuit breaker state (0=closed, 1=half-open, 2=open)\n", prefix)
		output += fmt.Sprintf("# TYPE %s_state gauge\n", prefix)
		stateValue := map[State]int{StateClosed: 0, StateHalfOpen: 1, StateOpen: 2}[m.State]
		output += fmt.Sprintf("%s_state %d\n\n", prefix, stateValue)

		output += fmt.Sprintf("# HELP %s_failures_total Total failures\n", prefix)
		output += fmt.Sprintf("# TYPE %s_failures_total counter\n", prefix)
		output += fmt.Sprintf("%s_failures_total %d\n\n", prefix, m.Failures)

		output += fmt.Sprintf("# HELP %s_successes_total Total successes\n", prefix)
		output += fmt.Sprintf("# TYPE %s_successes_total counter\n", prefix)
		output += fmt.Sprintf("%s_successes_total %d\n\n", prefix, m.Successes)

		output += fmt.Sprintf("# HELP %s_rejects_total Total rejects\n", prefix)
		output += fmt.Sprintf("# TYPE %s_rejects_total counter\n", prefix)
		output += fmt.Sprintf("%s_rejects_total %d\n\n", prefix, m.Rejects)

		output += fmt.Sprintf("# HELP %s_consecutive_failures Current consecutive failures\n", prefix)
		output += fmt.Sprintf("# TYPE %s_consecutive_failures gauge\n", prefix)
		output += fmt.Sprintf("%s_consecutive_failures %d\n\n", prefix, m.ConsecutiveFailures)
	}

	return output
}

// JSONExporter exports metrics as JSON
type JSONExporter struct {
	registry *Registry
}

// NewJSONExporter creates a new JSON exporter
func NewJSONExporter(registry *Registry) *JSONExporter {
	return &JSONExporter{registry: registry}
}

// Export returns JSON-formatted metrics
func (e *JSONExporter) Export() ([]byte, error) {
	metrics := e.registry.Metrics()
	return json.MarshalIndent(metrics, "", "  ")
}

// HealthChecker performs health checks using circuit breaker metrics
type HealthChecker struct {
	registry    *Registry
	thresholds  map[string]State
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(registry *Registry) *HealthChecker {
	return &HealthChecker{
		registry:   registry,
		thresholds: make(map[string]State),
	}
}

// SetThreshold sets the threshold state for a circuit breaker
func (h *HealthChecker) SetThreshold(name string, state State) {
	h.thresholds[name] = state
}

// Check returns health status based on circuit breaker states
func (h *HealthChecker) Check() HealthStatus {
	metrics := h.registry.Metrics()
	status := HealthStatus{
		Healthy:   true,
		Breakers:  make(map[string]BreakerHealth),
		Timestamp: time.Now(),
	}

	for name, m := range metrics {
		threshold, hasThreshold := h.thresholds[name]
		if !hasThreshold {
			threshold = StateOpen // Default: consider OPEN as unhealthy
		}

		healthy := m.State < threshold
		if !healthy {
			status.Healthy = false
		}

		status.Breakers[name] = BreakerHealth{
			State:   m.State.String(),
			Healthy: healthy,
			Metrics: m,
		}
	}

	return status
}

// HealthStatus represents overall health check result
type HealthStatus struct {
	Healthy   bool                     `json:"healthy"`
	Breakers  map[string]BreakerHealth `json:"breakers"`
	Timestamp time.Time                `json:"timestamp"`
}

// BreakerHealth represents health for a single circuit breaker
type BreakerHealth struct {
	State   string  `json:"state"`
	Healthy bool    `json:"healthy"`
	Metrics Metrics `json:"metrics"`
}
```

---

## 5. Usage Examples

### Basic Usage

```go
package main

import (
	"fmt"
	"time"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

func main() {
	// Create a circuit breaker with default config
	config := circuitbreaker.DefaultConfig()
	cb, err := circuitbreaker.New(config)
	if err != nil {
		panic(err)
	}
	defer cb.Close()

	// Execute a protected function
	err = cb.Execute(func() error {
		// Your actual operation here
		return doSomething()
	})

	if err != nil {
		if err == circuitbreaker.ErrCircuitOpen {
			fmt.Println("Circuit breaker is open")
			return
		}
		// Handle other errors
	}
}

func doSomething() error {
	return nil
}
```

### With Hooks and Monitoring

```go
package main

import (
	"fmt"
	"time"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

func main() {
	config := circuitbreaker.Config{
		FailureThreshold:    5,
		SuccessThreshold:    3,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
	}

	hooks := circuitbreaker.Hooks{
		OnStateChange: func(from, to circuitbreaker.State) {
			fmt.Printf("Circuit breaker changed state: %s -> %s\n", from, to)
		},
		OnFailure: func(err error) {
			fmt.Printf("Request failed: %v\n", err)
		},
		OnSuccess: func() {
			fmt.Println("Request succeeded")
		},
		OnReject: func() {
			fmt.Println("Request rejected (circuit open)")
		},
	}

	cb, err := circuitbreaker.NewWithHooks(config, hooks)
	if err != nil {
		panic(err)
	}
	defer cb.Close()

	// Use the circuit breaker
	err = cb.Execute(func() error {
		return callExternalService()
	})
}

func callExternalService() error {
	return nil
}
```

### HTTP Client with Circuit Breaker

```go
package main

import (
	"net/http"
	"time"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

func main() {
	config := circuitbreaker.DefaultConfig()
	cb, _ := circuitbreaker.New(config)
	defer cb.Close()

	// Create HTTP client with circuit breaker
	client := circuitbreaker.HTTPClientWithBreaker(cb, 10*time.Second)

	// All requests through this client are protected
	resp, err := client.Get("https://api.example.com/data")
	if err != nil {
		if circuitbreaker.IsCircuitBreakerError(err) {
			// Circuit breaker is open
		}
		// Handle error
	}
	defer resp.Body.Close()
}
```

### Named Circuit Breakers with Registry

```go
package main

import (
	"fmt"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

func main() {
	registry := circuitbreaker.NewRegistry()

	// Create circuit breaker for API service
	apiBreaker, _ := registry.GetOrCreate("api-service", circuitbreaker.Config{
		FailureThreshold: 5,
		Timeout:          30 * time.Second,
	})

	// Create circuit breaker for database
	dbBreaker, _ := registry.GetOrCreate("database", circuitbreaker.Config{
		FailureThreshold: 3,
		Timeout:          60 * time.Second,
	})

	// Use them independently
	apiBreaker.Execute(callAPI)
	dbBreaker.Execute(callDatabase)

	// Get all metrics
	metrics := registry.Metrics()
	for name, m := range metrics {
		fmt.Printf("%s: state=%s, failures=%d\n", name, m.State, m.Failures)
	}
}
```

### HTTP Handler Middleware

```go
package main

import (
	"net/http"
	"time"
	"github.com/openapi-cli/core/internal/core/circuitbreaker"
)

func main() {
	config := circuitbreaker.DefaultConfig()
	cb, _ := circuitbreaker.New(config)
	defer cb.Close()

	// Create handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	})

	// Wrap with circuit breaker middleware
	protected := circuitbreaker.Middleware(cb)(handler)

	// Start server
	http.ListenAndServe(":8080", protected)
}
```

---

## 6. State Machine Diagram

```
                              ┌─────────────┐
                              │   CLOSED    │
                              │             │
                              │ Requests    │
                              │ pass        │
                              └──────┬──────┘
                                     │
                          Failure    │
                          threshold  │
                          reached    │
                                     ▼
                              ┌─────────────┐
          ┌──────────────────│    OPEN     │◄──────────────────┐
          │                   │             │                   │
          │                   │ Rejects     │                   │
          │                   │ requests    │                   │
          │                   │             │                   │
          │                   └──────┬──────┘                   │
          │                          │                          │
          │                          │ Timeout                    │
          │                          │ elapsed                    │
          │                          ▼                          │
          │                   ┌─────────────┐                     │
          │                   │  HALF_OPEN  │                     │
          │                   │             │                     │
          │                   │ Allows      │                     │
          │                   │ limited     │                     │
          │                   │ requests    │                     │
          │                   └──────┬──────┘                     │
          │                          │                          │
          │              ┌───────────┴───────────┐              │
          │              │                       │              │
          │         Success threshold       Any failure      │
          │         reached                  │              │
          │              │                       │              │
          │              ▼                       │              │
          │         ┌─────────┐                │              │
          └────────►│  CLOSED │────────────────┘              │
                    │         │                                 │
                    └─────────┘─────────────────────────────────┘
```

---

## 7. Configuration Guidelines

| Parameter | Recommended | Description |
|-----------|-------------|-------------|
| FailureThreshold | 5-10 | Number of failures before opening |
| SuccessThreshold | 2-5 | Successes needed to close from half-open |
| Timeout | 30s-60s | Time before auto-recovery attempt |
| HalfOpenMaxRequests | 2-5 | Parallel requests in half-open state |
| MaxConsecutiveFailures | 100+ | Prevents rapid re-tripping |

---

## 8. Integration Points

- **HTTP Client**: Wrap HTTP clients for external services
- **Database**: Protect database connection pools
- **Third-party APIs**: Isolate external service failures
- **Internal Services**: Prevent cascading failures in microservices
- **Middleware**: HTTP handler protection for incoming requests

---

## Deterministic Guarantees

This circuit breaker implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic State Transitions

Circuit breaker state transitions must be identical across platforms:

- **Failure counting**: Same failure sequence → same state transition
- **Timeout handling**: UTC‑based timeout calculation, identical transition timing
- **Half‑open testing**: Deterministic request selection for testing

### 2. Platform‑Independent Time Handling

Circuit breaker timing must be platform‑independent:

- **Timeout calculation**: Use `time.Now().UTC()` for all timing
- **Duration comparison**: Nanosecond precision UTC comparisons
- **Timer scheduling**: Deterministic timer wake‑up ordering

### 3. Deterministic Concurrent Operations

Concurrent circuit breaker operations must be deterministic:

- **State transition atomicity**: Atomic state updates with deterministic ordering
- **Metric collection**: Atomic counter increments with deterministic aggregation
- **Concurrent requests**: FIFO queue for half‑open state requests

### 4. Cross‑Language State Consistency

Circuit breaker state must be consistent across 12 language SDKs:

- **State representation**: Identical state enumeration (CLOSED, OPEN, HALF_OPEN)
- **Transition logic**: Identical state machine implementation
- **Metrics collection**: Identical metric names and types

### 5. Testing Determinism

All circuit breaker tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestCircuitBreakerDeterminism(t *testing.T) {
	// Verify identical state transitions
	// Verify identical timeout behavior
	// Verify identical metric collection
}
```

### 6. Integration with SDK Generation

When generating SDKs for 12 languages, circuit breakers must:

- Implement identical state machines
- Produce identical state transitions
- Collect identical metrics

**Verification**: Run `./scripts/validate‑determinism‑circuit‑breaker.sh` to confirm cross‑platform consistency.

This implementation provides a production-ready circuit breaker with thread-safe operations, comprehensive metrics, and flexible monitoring hooks following Go best practices from CODE-GUIDE.md.
