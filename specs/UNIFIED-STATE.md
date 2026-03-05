# Unified State Management Specification

## Overview

Shared state management with synchronization and conflict resolution between CLI and MCP.

## State Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Unified State Management                    │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ CLI State   │  │ MCP State   │  │ Shared Core │     │
│  │  (local)    │  │  (local)    │  │  (manager)  │     │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘     │
│         └────────────────┼────────────────┘              │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  State Store          │                  │
│              │  (SQLite with CRDT)   │                  │
│              └───────────┬───────────┘                  │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  Conflict Resolver    │                  │
│              │  (LWW / Vector Clocks) │                  │
│              └───────────────────────┘                  │
└─────────────────────────────────────────────────────────┘
```

## State Manager

```go
// internal/unified/state.go
package unified

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

// StateValue represents a value with metadata
type StateValue struct {
    Value      interface{} `json:"value"`
    Version    int64       `json:"version"`
    Timestamp  time.Time   `json:"timestamp"`
    Source     string      `json:"source"` // "cli" or "mcp"
    ModifiedBy string      `json:"modified_by"`
}

// StateManager manages shared state
type StateManager struct {
    store     StateStore
    syncer    StateSyncer
    mu        sync.RWMutex
    local     map[string]*StateValue
    listeners []StateListener
}

// StateStore interface for persistence
type StateStore interface {
    Get(ctx context.Context, key string) (*StateValue, error)
    Set(ctx context.Context, key string, value *StateValue) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}

// StateSyncer interface for synchronization
type StateSyncer interface {
    Broadcast(ctx context.Context, changes map[string]*StateValue) error
    Subscribe(ctx context.Context, handler func(map[string]*StateValue))
}

// StateListener is called when state changes
type StateListener func(key string, old, new *StateValue)

// NewStateManager creates a state manager
func NewStateManager(store StateStore, syncer StateSyncer) *StateManager {
    return &StateManager{
        store:     store,
        syncer:    syncer,
        local:     make(map[string]*StateValue),
        listeners: make([]StateListener, 0),
    }
}

// Get retrieves a state value
func (sm *StateManager) Get(key string) (*StateValue, error) {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    if val, ok := sm.local[key]; ok {
        return val, nil
    }
    
    return sm.store.Get(context.Background(), key)
}

// Set updates a state value
func (sm *StateManager) Set(ctx context.Context, key string, value interface{}, source string) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    oldVal := sm.local[key]
    
    newVal := &StateValue{
        Value:      value,
        Version:    sm.nextVersion(oldVal),
        Timestamp:  time.Now(),
        Source:     source,
        ModifiedBy: sm.getClientID(),
    }
    
    // Save locally
    sm.local[key] = newVal
    
    // Persist to store
    if err := sm.store.Set(ctx, key, newVal); err != nil {
        return fmt.Errorf("failed to persist state: %w", err)
    }
    
    // Sync to other interfaces
    changes := map[string]*StateValue{key: newVal}
    sm.syncer.Broadcast(ctx, changes)
    
    // Notify listeners
    for _, listener := range sm.listeners {
        listener(key, oldVal, newVal)
    }
    
    return nil
}

// Delete removes a state value
func (sm *StateManager) Delete(ctx context.Context, key string) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    oldVal := sm.local[key]
    delete(sm.local, key)
    
    if err := sm.store.Delete(ctx, key); err != nil {
        return err
    }
    
    // Notify with nil new value
    for _, listener := range sm.listeners {
        listener(key, oldVal, nil)
    }
    
    return nil
}

// Watch adds a state change listener
func (sm *StateManager) Watch(listener StateListener) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    sm.listeners = append(sm.listeners, listener)
}

func (sm *StateManager) nextVersion(old *StateValue) int64 {
    if old == nil {
        return 1
    }
    return old.Version + 1
}

func (sm *StateManager) getClientID() string {
    // Generate or retrieve unique client ID
    return fmt.Sprintf("client-%d", time.Now().UnixNano())
}
```

## State Synchronization

```go
// internal/unified/state_sync.go
package unified

import (
    "context"
    "sync"
    "time"
)

// StateSync implements bidirectional state synchronization
type StateSync struct {
    manager    *StateManager
    syncChan   chan SyncMessage
    ctx        context.Context
    cancel     context.CancelFunc
    mu         sync.Mutex
}

// SyncMessage represents a state sync message
type SyncMessage struct {
    Type      string                 `json:"type"`
    Changes   map[string]*StateValue `json:"changes"`
    Timestamp time.Time              `json:"timestamp"`
    Source    string                 `json:"source"`
}

// NewStateSync creates a state synchronizer
func NewStateSync(manager *StateManager) *StateSync {
    ctx, cancel := context.WithCancel(context.Background())
    return &StateSync{
        manager:  manager,
        syncChan: make(chan SyncMessage, 100),
        ctx:      ctx,
        cancel:   cancel,
    }
}

// Start begins the sync loop
func (s *StateSync) Start() {
    go s.syncLoop()
}

// Stop halts the sync loop
func (s *StateSync) Stop() {
    s.cancel()
    close(s.syncChan)
}

// Broadcast sends state changes to all peers
func (s *StateSync) Broadcast(ctx context.Context, changes map[string]*StateValue) error {
    msg := SyncMessage{
        Type:      "update",
        Changes:   changes,
        Timestamp: time.Now(),
        Source:    s.getSource(),
    }
    
    select {
    case s.syncChan <- msg:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

// Subscribe registers a sync message handler
func (s *StateSync) Subscribe(ctx context.Context, handler func(map[string]*StateValue)) {
    // Implementation would subscribe to IPC channel
}

func (s *StateSync) syncLoop() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case msg := <-s.syncChan:
            s.handleSyncMessage(msg)
        case <-ticker.C:
            s.pushPendingChanges()
        case <-s.ctx.Done():
            return
        }
    }
}

func (s *StateSync) handleSyncMessage(msg SyncMessage) {
    // Skip messages from self
    if msg.Source == s.getSource() {
        return
    }
    
    // Apply changes with conflict resolution
    for key, remoteVal := range msg.Changes {
        s.applyChange(key, remoteVal)
    }
}

func (s *StateSync) applyChange(key string, remoteVal *StateValue) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get local value
    localVal, _ := s.manager.Get(key)
    
    // Resolve conflict
    resolved := s.resolveConflict(localVal, remoteVal)
    
    // Apply resolved value
    if resolved != localVal {
        s.manager.Set(s.ctx, key, resolved.Value, "sync")
    }
}

func (s *StateSync) pushPendingChanges() {
    // Push any pending local changes
}

func (s *StateSync) getSource() string {
    // Return "cli" or "mcp" based on current context
    return "cli"
}
```

## Conflict Resolution

```go
// internal/unified/conflict.go
package unified

import "time"

// ConflictResolver handles state conflicts
type ConflictResolver struct {
    strategy ConflictStrategy
}

// ConflictStrategy defines how to resolve conflicts
type ConflictStrategy int

const (
    LastWriteWins ConflictStrategy = iota
    FirstWriteWins
    MergeValues
    RejectConflict
)

// NewConflictResolver creates a resolver
func NewConflictResolver(strategy ConflictStrategy) *ConflictResolver {
    return &ConflictResolver{strategy: strategy}
}

// Resolve determines which value wins
func (r *ConflictResolver) Resolve(local, remote *StateValue) *StateValue {
    switch r.strategy {
    case LastWriteWins:
        return r.lastWriteWins(local, remote)
    case FirstWriteWins:
        return r.firstWriteWins(local, remote)
    case MergeValues:
        return r.mergeValues(local, remote)
    default:
        return local // Keep local on rejection
    }
}

func (r *ConflictResolver) lastWriteWins(local, remote *StateValue) *StateValue {
    if local == nil {
        return remote
    }
    if remote == nil {
        return local
    }
    
    // Compare timestamps
    if remote.Timestamp.After(local.Timestamp) {
        return remote
    }
    return local
}

func (r *ConflictResolver) firstWriteWins(local, remote *StateValue) *StateValue {
    if local == nil {
        return remote
    }
    return local
}

func (r *ConflictResolver) mergeValues(local, remote *StateValue) *StateValue {
    // For complex types, attempt merge
    if local == nil {
        return remote
    }
    if remote == nil {
        return local
    }
    
    // Simple merge: take remote timestamp, merge values
    merged := &StateValue{
        Value:      remote.Value,
        Version:    max(local.Version, remote.Version) + 1,
        Timestamp:  time.Now(),
        Source:     "merge",
        ModifiedBy: "conflict_resolver",
    }
    
    return merged
}

func max(a, b int64) int64 {
    if a > b {
        return a
    }
    return b
}
```

## SQLite State Store

```go
// internal/unified/state_sqlite.go
package unified

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    
    _ "github.com/mattn/go-sqlite3"
)

// SQLiteStateStore implements state storage with SQLite
type SQLiteStateStore struct {
    db *sql.DB
}

// NewSQLiteStateStore creates a SQLite-backed state store
func NewSQLiteStateStore(dbPath string) (*SQLiteStateStore, error) {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    
    // Create table if not exists
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS state (
            key TEXT PRIMARY KEY,
            value BLOB NOT NULL,
            version INTEGER NOT NULL,
            timestamp DATETIME NOT NULL,
            source TEXT NOT NULL,
            modified_by TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_timestamp ON state(timestamp);
    `)
    if err != nil {
        return nil, fmt.Errorf("failed to create table: %w", err)
    }
    
    return &SQLiteStateStore{db: db}, nil
}

// Get retrieves a state value
func (s *SQLiteStateStore) Get(ctx context.Context, key string) (*StateValue, error) {
    var data []byte
    var val StateValue
    
    err := s.db.QueryRowContext(ctx, `
        SELECT value, version, timestamp, source, modified_by
        FROM state WHERE key = ?
    `, key).Scan(&data, &val.Version, &val.Timestamp, &val.Source, &val.ModifiedBy)
    
    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    
    if err := json.Unmarshal(data, &val.Value); err != nil {
        return nil, err
    }
    
    return &val, nil
}

// Set stores a state value
func (s *SQLiteStateStore) Set(ctx context.Context, key string, value *StateValue) error {
    data, err := json.Marshal(value.Value)
    if err != nil {
        return err
    }
    
    _, err = s.db.ExecContext(ctx, `
        INSERT INTO state (key, value, version, timestamp, source, modified_by)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(key) DO UPDATE SET
            value = excluded.value,
            version = excluded.version,
            timestamp = excluded.timestamp,
            source = excluded.source,
            modified_by = excluded.modified_by
    `, key, data, value.Version, value.Timestamp, value.Source, value.ModifiedBy)
    
    return err
}

// Delete removes a state value
func (s *SQLiteStateStore) Delete(ctx context.Context, key string) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM state WHERE key = ?`, key)
    return err
}

// List returns keys matching prefix
func (s *SQLiteStateStore) List(ctx context.Context, prefix string) ([]string, error) {
    rows, err := s.db.QueryContext(ctx, `SELECT key FROM state WHERE key LIKE ?`, prefix+"%")
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var keys []string
    for rows.Next() {
        var key string
        if err := rows.Scan(&key); err != nil {
            return nil, err
        }
        keys = append(keys, key)
    }
    
    return keys, rows.Err()
}

// Close closes the database
func (s *SQLiteStateStore) Close() error {
    return s.db.Close()
}
```
