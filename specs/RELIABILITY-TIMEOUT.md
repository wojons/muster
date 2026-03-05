# Timeout Specification

## Overview

Comprehensive timeout handling with context cancellation, deadline propagation, and hierarchical timeout management.

## Core Components

### Timeout Configuration

```go
// TimeoutConfig defines timeout behavior
type TimeoutConfig struct {
    OperationTimeout time.Duration `json:"operation_timeout"`
    ConnectTimeout   time.Duration `json:"connect_timeout"`
    ReadTimeout      time.Duration `json:"read_timeout"`
    WriteTimeout     time.Duration `json:"write_timeout"`
    GracePeriod      time.Duration `json:"grace_period"`
    PropagateCancel  bool          `json:"propagate_cancel"`
}

// DefaultTimeoutConfig provides safe defaults
func DefaultTimeoutConfig() TimeoutConfig {
    return TimeoutConfig{
        OperationTimeout: 30 * time.Second,
        ConnectTimeout:   5 * time.Second,
        ReadTimeout:      10 * time.Second,
        WriteTimeout:     10 * time.Second,
        GracePeriod:      5 * time.Second,
        PropagateCancel:  true,
    }
}

// TimeoutPolicy aggregates multiple timeout configs
type TimeoutPolicy struct {
    Global   TimeoutConfig            `json:"global"`
    Services map[string]TimeoutConfig `json:"services,omitempty"`
    Methods  map[string]TimeoutConfig `json:"methods,omitempty"`
}
```

### Timeout Manager

```go
// TimeoutManager handles timeout operations
type TimeoutManager struct {
    policy TimeoutPolicy
    clock  Clock
}

func NewTimeoutManager(policy TimeoutPolicy) *TimeoutManager {
    return &TimeoutManager{
        policy: policy,
        clock:  &realClock{},
    }
}

// Execute runs fn with timeout enforcement
func (tm *TimeoutManager) Execute(ctx context.Context, timeout time.Duration, fn TimeoutFunc) (Result, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    result := make(chan Result, 1)
    errChan := make(chan error, 1)
    
    go func() {
        defer func() {
            if r := recover(); r != nil {
                errChan <- fmt.Errorf("panic: %v", r)
            }
        }()
        
        res, err := fn(ctx)
        if err != nil {
            errChan <- err
            return
        }
        result <- res
    }()
    
    select {
    case res := <-result:
        return res, nil
    case err := <-errChan:
        return nil, err
    case <-ctx.Done():
        // Attempt graceful shutdown
        if tm.policy.Global.GracePeriod > 0 {
            select {
            case res := <-result:
                return res, nil
            case err := <-errChan:
                return nil, err
            case <-time.After(tm.policy.Global.GracePeriod):
                // Force cancel
            }
        }
        
        return nil, &TimeoutError{
            Operation: "execute",
            Deadline:  tm.clock.Now().Add(timeout),
            Cause:     ctx.Err(),
        }
    }
}
```

## Context Cancellation

```go
// CancelManager handles cancellation propagation
type CancelManager struct {
    parent context.Context
    mu     sync.Mutex
    subs   []context.CancelFunc
}

func NewCancelManager(parent context.Context) *CancelManager {
    return &CancelManager{parent: parent}
}

func (cm *CancelManager) Subscribe() context.Context {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    ctx, cancel := context.WithCancel(cm.parent)
    cm.subs = append(cm.subs, cancel)
    
    // Auto-unsubscribe on parent cancellation
    go func() {
        <-cm.parent.Done()
        cm.unsubscribe(cancel)
    }()
    
    return ctx
}

func (cm *CancelManager) unsubscribe(cancel context.CancelFunc) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    for i, sub := range cm.subs {
        if sub == cancel {
            cm.subs = append(cm.subs[:i], cm.subs[i+1:]...)
            break
        }
    }
}

func (cm *CancelManager) CancelAll() {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    for _, cancel := range cm.subs {
        cancel()
    }
    cm.subs = nil
}

// DetachedContext creates context immune to parent cancellation
func DetachedContext(parent context.Context) context.Context {
    return &detachedCtx{parent}
}

type detachedCtx struct {
    context.Context
}

func (d *detachedCtx) Done() <-chan struct{} { return nil }
func (d *detachedCtx) Err() error             { return nil }
```

## Deadline Propagation

```go
// DeadlinePropagator manages deadline inheritance
type DeadlinePropagator struct {
    minTimeout time.Duration
    maxTimeout time.Duration
}

func NewDeadlinePropagator(min, max time.Duration) *DeadlinePropagator {
    return &DeadlinePropagator{
        minTimeout: min,
        maxTimeout: max,
    }
}

// Propagate calculates child deadline from parent
func (dp *DeadlinePropagator) Propagate(parent context.Context, requested time.Duration) (context.Context, context.CancelFunc) {
    parentDeadline, hasDeadline := parent.Deadline()
    
    if !hasDeadline {
        // No parent deadline, use requested
        return context.WithTimeout(parent, dp.clamp(requested))
    }
    
    remaining := time.Until(parentDeadline)
    if remaining <= 0 {
        // Parent already expired
        return context.WithCancel(parent)
    }
    
    // Use minimum of parent remaining and requested
    timeout := requested
    if remaining < timeout {
        timeout = remaining
    }
    
    return context.WithTimeout(parent, dp.clamp(timeout))
}

func (dp *DeadlinePropagator) clamp(d time.Duration) time.Duration {
    if d < dp.minTimeout {
        return dp.minTimeout
    }
    if d > dp.maxTimeout {
        return dp.maxTimeout
    }
    return d
}

// Chain combines multiple timeouts hierarchically
func Chain(parent context.Context, timeouts ...time.Duration) (context.Context, context.CancelFunc) {
    if len(timeouts) == 0 {
        return context.WithCancel(parent)
    }
    
    ctx, cancel := context.WithTimeout(parent, timeouts[0])
    if len(timeouts) == 1 {
        return ctx, cancel
    }
    
    // Chain remaining timeouts
    for _, t := range timeouts[1:] {
        var innerCancel context.CancelFunc
        ctx, innerCancel = context.WithTimeout(ctx, t)
        
        // Combine cancels
        prevCancel := cancel
        cancel = func() {
            prevCancel()
            innerCancel()
        }
    }
    
    return ctx, cancel
}
```

## Timeout Hierarchies

```go
// TimeoutHierarchy manages nested timeouts
type TimeoutHierarchy struct {
    root      *TimeoutNode
    current   *TimeoutNode
    mu        sync.RWMutex
}

type TimeoutNode struct {
    ID        string
    Timeout   time.Duration
    Deadline  time.Time
    Parent    *TimeoutNode
    Children  []*TimeoutNode
    Context   context.Context
    Cancel    context.CancelFunc
}

func NewTimeoutHierarchy(rootTimeout time.Duration) *TimeoutHierarchy {
    ctx, cancel := context.WithTimeout(context.Background(), rootTimeout)
    root := &TimeoutNode{
        ID:       "root",
        Timeout:  rootTimeout,
        Deadline: time.Now().Add(rootTimeout),
        Context:  ctx,
        Cancel:   cancel,
    }
    
    return &TimeoutHierarchy{
        root:    root,
        current: root,
    }
}

func (th *TimeoutHierarchy) Push(id string, timeout time.Duration) *TimeoutNode {
    th.mu.Lock()
    defer th.mu.Unlock()
    
    // Child deadline cannot exceed parent
    parentDeadline := th.current.Deadline
    childDeadline := time.Now().Add(timeout)
    
    if childDeadline.After(parentDeadline) {
        childDeadline = parentDeadline
        timeout = time.Until(parentDeadline)
    }
    
    ctx, cancel := context.WithDeadline(th.current.Context, childDeadline)
    
    node := &TimeoutNode{
        ID:       id,
        Timeout:  timeout,
        Deadline: childDeadline,
        Parent:   th.current,
        Context:  ctx,
        Cancel:   cancel,
    }
    
    th.current.Children = append(th.current.Children, node)
    th.current = node
    
    return node
}

func (th *TimeoutHierarchy) Pop() {
    th.mu.Lock()
    defer th.mu.Unlock()
    
    if th.current.Parent != nil {
        th.current = th.current.Parent
    }
}

func (th *TimeoutHierarchy) Remaining() time.Duration {
    th.mu.RLock()
    defer th.mu.RUnlock()
    
    return time.Until(th.current.Deadline)
}
```

### Error Types

```go
// TimeoutError indicates operation exceeded deadline
type TimeoutError struct {
    Operation string        `json:"operation"`
    Deadline  time.Time     `json:"deadline"`
    Elapsed   time.Duration `json:"elapsed"`
    Cause     error         `json:"cause,omitempty"`
}

func (e *TimeoutError) Error() string {
    return fmt.Sprintf("timeout: %s exceeded deadline %v (elapsed: %v): %v",
        e.Operation, e.Deadline, e.Elapsed, e.Cause)
}

func (e *TimeoutError) Timeout() bool { return true }
func (e *TimeoutError) Unwrap() error  { return e.Cause }

// DeadlineExceededError wraps context.DeadlineExceeded
type DeadlineExceededError struct {
    NodeID   string        `json:"node_id"`
    Deadline time.Time     `json:"deadline"`
}

func (e *DeadlineExceededError) Error() string {
    return fmt.Sprintf("deadline exceeded for node %s at %v", e.NodeID, e.Deadline)
}
```

## Usage Examples

```go
// Basic timeout
manager := NewTimeoutManager(DefaultTimeoutConfig())
result, err := manager.Execute(ctx, 5*time.Second, func(ctx context.Context) (Result, error) {
    return operation.Call(ctx)
})

// Hierarchical timeouts
hierarchy := NewTimeoutHierarchy(30 * time.Second)
defer hierarchy.root.Cancel()

// Child inherits parent's deadline
child := hierarchy.Push("api_call", 10*time.Second)
result, err := api.Call(child.Context)
hierarchy.Pop()
```
