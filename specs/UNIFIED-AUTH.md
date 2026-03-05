# Unified Authentication Specification

## Overview

Shared authentication state between CLI and MCP with secure credential sharing.

## Authentication Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Unified Authentication                      │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ CLI Auth    │  │ MCP Auth    │  │ Shared Core │     │
│  │  (tokens)   │  │  (tokens)   │  │  (manager)  │     │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘     │
│         └────────────────┼────────────────┘              │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  Auth Store            │                  │
│              │  (OS Keychain/Secure) │                  │
│              └───────────┬───────────┘                  │
│                          │                              │
│              ┌───────────▼───────────┐                  │
│              │  Credential Sync       │                  │
│              │  (encrypted cache)     │                  │
│              └───────────────────────┘                  │
└─────────────────────────────────────────────────────────┘
```

## Authentication Manager

```go
// internal/unified/auth.go
package unified

import (
    "context"
    "fmt"
    "sync"
    "time"
)

// AuthState represents the current authentication state
type AuthState struct {
    mu sync.RWMutex
    
    Authenticated   bool      `json:"authenticated"`
    Method          string    `json:"method"`
    Username        string    `json:"username,omitempty"`
    Token           string    `json:"-"` // Never serialized
    RefreshToken    string    `json:"-"`
    ExpiresAt       time.Time `json:"expires_at"`
    LastVerified    time.Time `json:"last_verified"`
    
    // Profile-specific auth
    ProfileAuth     map[string]*ProfileAuthState `json:"profile_auth"`
}

// ProfileAuthState tracks auth per profile
type ProfileAuthState struct {
    Authenticated bool      `json:"authenticated"`
    Token         string    `json:"-"`
    ExpiresAt     time.Time `json:"expires_at"`
}

// AuthManager handles unified authentication
type AuthManager struct {
    state      *AuthState
    store      CredentialStore
    syncer     AuthSyncer
    mu         sync.RWMutex
}

// CredentialStore interface for secure storage
type CredentialStore interface {
    Save(ctx context.Context, key string, value []byte) error
    Load(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
}

// AuthSyncer interface for auth state synchronization
type AuthSyncer interface {
    Broadcast(ctx context.Context, state *AuthState) error
    Subscribe(ctx context.Context, handler func(*AuthState))
}

// NewAuthManager creates an auth manager
func NewAuthManager(store CredentialStore, syncer AuthSyncer) *AuthManager {
    return &AuthManager{
        state: &AuthState{
            ProfileAuth: make(map[string]*ProfileAuthState),
        },
        store:  store,
        syncer: syncer,
    }
}

// Login authenticates the user
func (am *AuthManager) Login(ctx context.Context, method, username, token string) error {
    am.mu.Lock()
    defer am.mu.Unlock()
    
    // Verify token with server
    if err := am.verifyToken(ctx, token); err != nil {
        return fmt.Errorf("token verification failed: %w", err)
    }
    
    // Store credentials securely
    if err := am.store.Save(ctx, "auth_token", []byte(token)); err != nil {
        return fmt.Errorf("failed to store token: %w", err)
    }
    
    // Update state
    am.state.Authenticated = true
    am.state.Method = method
    am.state.Username = username
    am.state.Token = token
    am.state.LastVerified = time.Now()
    
    // Sync to other interfaces
    am.syncer.Broadcast(ctx, am.state)
    
    return nil
}

// Logout clears authentication
func (am *AuthManager) Logout(ctx context.Context) error {
    am.mu.Lock()
    defer am.mu.Unlock()
    
    // Clear from secure storage
    am.store.Delete(ctx, "auth_token")
    am.store.Delete(ctx, "refresh_token")
    
    // Reset state
    am.state = &AuthState{
        ProfileAuth: make(map[string]*ProfileAuthState),
    }
    
    // Sync logout to other interfaces
    am.syncer.Broadcast(ctx, am.state)
    
    return nil
}

// GetState returns current auth state (without tokens)
func (am *AuthManager) GetState() *AuthState {
    am.mu.RLock()
    defer am.mu.RUnlock()
    
    // Return copy without sensitive data
    return &AuthState{
        Authenticated: am.state.Authenticated,
        Method:        am.state.Method,
        Username:      am.state.Username,
        ExpiresAt:     am.state.ExpiresAt,
        LastVerified:  am.state.LastVerified,
    }
}

// GetToken returns the current token
func (am *AuthManager) GetToken() string {
    am.mu.RLock()
    defer am.mu.RUnlock()
    return am.state.Token
}

// SetProfileAuth sets auth for a specific profile
func (am *AuthManager) SetProfileAuth(profile, token string, expires time.Time) error {
    am.mu.Lock()
    defer am.mu.Unlock()
    
    am.state.ProfileAuth[profile] = &ProfileAuthState{
        Authenticated: true,
        Token:         token,
        ExpiresAt:     expires,
    }
    
    return nil
}

func (am *AuthManager) verifyToken(ctx context.Context, token string) error {
    // Implementation would verify with auth server
    return nil
}
```

## Auth State Sync

```go
// internal/unified/auth_sync.go
package unified

import (
    "context"
    "encoding/json"
    "sync"
)

// AuthStateSync handles synchronization between CLI and MCP
type AuthStateSync struct {
    listeners []func(*AuthState)
    mu        sync.RWMutex
    stateChan chan *AuthState
}

// NewAuthStateSync creates a sync handler
func NewAuthStateSync() *AuthStateSync {
    return &AuthStateSync{
        listeners: make([]func(*AuthState), 0),
        stateChan: make(chan *AuthState, 10),
    }
}

// Broadcast sends auth state to all listeners
func (s *AuthStateSync) Broadcast(ctx context.Context, state *AuthState) error {
    // Create safe copy (no tokens)
    safeState := &AuthState{
        Authenticated: state.Authenticated,
        Method:        state.Method,
        Username:      state.Username,
        ExpiresAt:     state.ExpiresAt,
        LastVerified:  state.LastVerified,
    }
    
    // Notify all listeners
    s.mu.RLock()
    listeners := make([]func(*AuthState), len(s.listeners))
    copy(listeners, s.listeners)
    s.mu.RUnlock()
    
    for _, listener := range listeners {
        listener(safeState)
    }
    
    return nil
}

// Subscribe registers a state change listener
func (s *AuthStateSync) Subscribe(ctx context.Context, handler func(*AuthState)) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.listeners = append(s.listeners, handler)
}

// SyncToMCP syncs auth state from CLI to MCP
func (s *AuthStateSync) SyncToMCP(state *AuthState) error {
    // Serialize to JSON for IPC
    data, err := json.Marshal(state)
    if err != nil {
        return err
    }
    
    // Send to MCP via appropriate channel
    _ = data
    return nil
}

// SyncFromMCPSyncs auth state from MCP to CLI
func (s *AuthStateSync) SyncFromMCP(data []byte) (*AuthState, error) {
    var state AuthState
    if err := json.Unmarshal(data, &state); err != nil {
        return nil, err
    }
    return &state, nil
}
```

## Credential Sharing

```go
// internal/unified/credentials.go
package unified

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
)

// SharedCredentialStore implements credential sharing
type SharedCredentialStore struct {
    basePath string
}

// NewSharedCredentialStore creates a shared credential store
func NewSharedCredentialStore() (*SharedCredentialStore, error) {
    var basePath string
    
    switch runtime.GOOS {
    case "darwin":
        home, _ := os.UserHomeDir()
        basePath = filepath.Join(home, "Library", "Application Support", "muster")
    case "windows":
        appData := os.Getenv("APPDATA")
        basePath = filepath.Join(appData, "muster")
    default: // linux and others
        home, _ := os.UserHomeDir()
        basePath = filepath.Join(home, ".config", "muster")
    }
    
    return &SharedCredentialStore{basePath: basePath}, nil
}

// Save stores credentials securely
func (s *SharedCredentialStore) Save(ctx context.Context, key string, value []byte) error {
    // In production, use OS keychain (keychain on macOS, Secret Service on Linux)
    // This is a simplified file-based implementation
    
    path := filepath.Join(s.basePath, "credentials", key)
    if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
        return fmt.Errorf("failed to create credentials directory: %w", err)
    }
    
    return os.WriteFile(path, value, 0600)
}

// Load retrieves credentials
func (s *SharedCredentialStore) Load(ctx context.Context, key string) ([]byte, error) {
    path := filepath.Join(s.basePath, "credentials", key)
    return os.ReadFile(path)
}

// Delete removes credentials
func (s *SharedCredentialStore) Delete(ctx context.Context, key string) error {
    path := filepath.Join(s.basePath, "credentials", key)
    return os.Remove(path)
}
```
