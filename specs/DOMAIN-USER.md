# Domain: User

## Overview

User entities manage sessions, authentication state, and user preferences.

## Entities

### UserSession

Active user session entity.

```go
type UserSession struct {
    ID            string           `json:"id" validate:"required,uuid4"`
    UserID        string           `json:"user_id" validate:"required,max=256"`
    Token         string           `json:"token" validate:"required,len=64"`
    Type          SessionType      `json:"type" validate:"required,oneof=interactive api service"`
    AuthMethod    string           `json:"auth_method" validate:"required,max=50"`
    ProviderID    string           `json:"provider_id,omitempty" validate:"omitempty,uuid4"`
    State         SessionState     `json:"state" validate:"required"`
    Permissions   []Permission     `json:"permissions" validate:"dive"`
    Roles         []string         `json:"roles" validate:"dive,max=50"`
    Metadata      SessionMetadata  `json:"metadata"`
    ExpiresAt     time.Time        `json:"expires_at"`
    LastActivity  time.Time        `json:"last_activity"`
    CreatedAt     time.Time        `json:"created_at"`
}

type SessionType string

const (
    SessionTypeInteractive SessionType = "interactive"
    SessionTypeAPI         SessionType = "api"
    SessionTypeService     SessionType = "service"
)

type SessionState string

const (
    SessionStateActive     SessionState = "active"
    SessionStateIdle       SessionState = "idle"
    SessionStateExpired    SessionState = "expired"
    SessionStateRevoked    SessionState = "revoked"
    SessionStateTerminated SessionState = "terminated"
)

type SessionMetadata struct {
    IPAddress     string           `json:"ip_address,omitempty" validate:"omitempty,ip"`
    UserAgent     string           `json:"user_agent,omitempty" validate:"max=500"`
    DeviceID      string           `json:"device_id,omitempty" validate:"max=100"`
    Country       string           `json:"country,omitempty" validate:"omitempty,len=2"`
    Region        string           `json:"region,omitempty" validate:"max=50"`
}
```

### Permission

Permission entity for access control.

```go
type Permission struct {
    Resource      string           `json:"resource" validate:"required,max=100"`
    Action        string           `json:"action" validate:"required,max=50"`
    Scope         string           `json:"scope,omitempty" validate:"max=100"`
    Conditions    []Condition      `json:"conditions,omitempty"`
}

type Condition struct {
    Attribute     string           `json:"attribute" validate:"required,max=100"`
    Operator      string           `json:"operator" validate:"required,oneof=eq ne gt gte lt lte in contains exists"`
    Value         any              `json:"value,omitempty"`
}
```

### AuthenticationState

Current authentication state tracking.

```go
type AuthenticationState struct {
    UserID        string                `json:"user_id" validate:"required"`
    Authenticated bool                  `json:"authenticated"`
    Method        string                `json:"method,omitempty" validate:"max=50"`
    Provider      string                `json:"provider,omitempty" validate:"max=100"`
    MFARequired   bool                  `json:"mfa_required"`
    MFACompleted  bool                  `json:"mfa_completed"`
    Permissions   []Permission          `json:"permissions,omitempty"`
    Claims        map[string]any        `json:"claims,omitempty"`
    Session       *UserSession          `json:"session,omitempty"`
}
```

### UserPreferences

User preference settings.

```go
type UserPreferences struct {
    ID            string                 `json:"id" validate:"required,uuid4"`
    UserID        string                 `json:"user_id" validate:"required"`
    Display       DisplayPreferences     `json:"display"`
    Notifications NotificationPreferences `json:"notifications"`
    Editor        EditorPreferences      `json:"editor"`
    Privacy       PrivacyPreferences     `json:"privacy"`
    Custom        map[string]any         `json:"custom,omitempty"`
    UpdatedAt     time.Time              `json:"updated_at"`
}

type DisplayPreferences struct {
    Theme         string                 `json:"theme" validate:"omitempty,oneof=light dark auto"`
    Language      string                 `json:"language" validate:"omitempty,len=2"`
    TimeZone      string                 `json:"time_zone,omitempty" validate:"max=50"`
    DateFormat    string                 `json:"date_format,omitempty" validate:"max=30"`
    CompactMode   bool                   `json:"compact_mode"`
}

type NotificationPreferences struct {
    Email         bool                   `json:"email"`
    InApp         bool                   `json:"in_app"`
    Push          bool                   `json:"push"`
    Digest        string                 `json:"digest,omitempty" validate:"omitempty,oneof=immediate hourly daily weekly"`
    Types         []string               `json:"types,omitempty" validate:"dive,max=50"`
}

type EditorPreferences struct {
    AutoSave      bool                   `json:"auto_save"`
    AutoFormat    bool                   `json:"auto_format"`
    FontSize      int                    `json:"font_size" validate:"gte=8,lte=32"`
    TabSize       int                    `json:"tab_size" validate:"gte=2,lte=8"`
    WordWrap      bool                   `json:"word_wrap"`
    Minimap       bool                   `json:"minimap"`
}

type PrivacyPreferences struct {
    ShareUsage    bool                   `json:"share_usage"`
    ShareErrors   bool                   `json:"share_errors"`
    PublicProfile bool                   `json:"public_profile"`
}
```

## Validation Methods

```go
func (s *UserSession) Validate() error {
    if err := validator.Validate(s); err != nil {
        return fmt.Errorf("session validation failed: %w", err)
    }
    
    if s.ExpiresAt.Before(time.Now()) {
        return errors.New("session has expired")
    }
    
    maxIdle := 30 * time.Minute
    if time.Since(s.LastActivity) > maxIdle {
        return errors.New("session has been idle too long")
    }
    
    return nil
}

func (s *UserSession) IsValid() bool {
    return s.State == SessionStateActive && 
           s.ExpiresAt.After(time.Now()) &&
           time.Since(s.LastActivity) < 30*time.Minute
}

func (s *UserSession) Touch() {
    s.LastActivity = time.Now()
}

func (s *UserSession) Revoke() {
    s.State = SessionStateRevoked
    s.ExpiresAt = time.Now()
}

func (p *UserPreferences) Validate() error {
    return validator.Validate(p)
}

func (a *AuthenticationState) IsAuthorized(resource, action string) bool {
    if !a.Authenticated {
        return false
    }
    
    for _, perm := range a.Permissions {
        if perm.Resource == resource && perm.Action == action {
            return true
        }
        if perm.Resource == "*" && perm.Action == action {
            return true
        }
        if perm.Resource == resource && perm.Action == "*" {
            return true
        }
    }
    
    return false
}

func (a *AuthenticationState) RequireMFA() bool {
    return a.MFARequired && !a.MFACompleted
}
```
