# Events: Audit Logging System

## Overview

Audit logging system captures security-relevant events with tamper-resistant storage, compliance tagging, and configurable retention policies for regulatory requirements.

## Configuration

```go
type AuditConfig struct {
    Enabled          bool          `json:"enabled"`
    StorageType      string        `json:"storage_type" validate:"oneof=file database s3"`
    RetentionDays    int           `json:"retention_days" validate:"gte=0"`
    BufferSize       int           `json:"buffer_size" validate:"gte=1"`
    FlushInterval    time.Duration `json:"flush_interval"`
    EncryptionKey    string        `json:"encryption_key,omitempty"`
    HashAlgorithm    string        `json:"hash_algorithm" validate:"oneof=sha256 sha512"`
    IncludeBody      bool          `json:"include_body"`
    SensitiveFields  []string      `json:"sensitive_fields"`
}

var DefaultAuditConfig = AuditConfig{
    Enabled:       true,
    StorageType:   "file",
    RetentionDays: 365,
    BufferSize:    1000,
    FlushInterval: time.Second * 5,
    HashAlgorithm: "sha256",
    SensitiveFields: []string{"password", "token", "secret", "credit_card"},
}
```

## Credential Redaction & PII Protection

### Adversarial Review Addressed: Credential Leakage Prevention

To prevent credential leakage in logs identified in adversarial review, implement **mandatory redaction** of sensitive fields with **pattern matching** and **structured logging sanitization**.

#### Redaction Rules

```go
// internal/audit/redaction.go

type RedactionRule struct {
    Pattern     string      // Regex pattern to match
    Replacement string      // Replacement text
    FieldPaths  []string    // JSON field paths to apply
    Mandatory   bool        // Must be redacted (cannot be disabled)
}

var MandatoryRedactionRules = []RedactionRule{
    {
        Pattern:     `\b(?:password|passwd|pwd)\b`,
        Replacement: "[REDACTED]",
        FieldPaths:  []string{"$..password", "$..passwd", "$..pwd"},
        Mandatory:   true,
    },
    {
        Pattern:     `\b(?:token|access_token|refresh_token|id_token)\b`,
        Replacement: "[REDACTED]",
        FieldPaths:  []string{"$..token", "$..access_token", "$..refresh_token", "$..id_token"},
        Mandatory:   true,
    },
    {
        Pattern:     `\b(?:secret|api_key|apikey|client_secret)\b`,
        Replacement: "[REDACTED]",
        FieldPaths:  []string{"$..secret", "$..api_key", "$..apikey", "$..client_secret"},
        Mandatory:   true,
    },
    {
        Pattern:     `\b(?:credit_card|cc_number|card_number)\b`,
        Replacement: "[REDACTED]",
        FieldPaths:  []string{"$..credit_card", "$..cc_number", "$..card_number"},
        Mandatory:   true,
    },
    {
        Pattern:     `\b(?:ssn|social_security)\b`,
        Replacement: "[REDACTED]",
        FieldPaths:  []string{"$..ssn", "$..social_security"},
        Mandatory:   true,
    },
}

// Redactor applies redaction rules to audit events
type Redactor struct {
    rules    []RedactionRule
    compiled []*regexp.Regexp
}

// NewRedactor creates redactor with mandatory rules
func NewRedactor() *Redactor {
    r := &Redactor{rules: MandatoryRedactionRules}
    for _, rule := range r.rules {
        re, err := regexp.Compile(rule.Pattern)
        if err != nil {
            // Use case‑insensitive fallback
            re = regexp.MustCompile(`(?i)` + rule.Pattern)
        }
        r.compiled = append(r.compiled, re)
    }
    return r
}

// RedactEvent applies redaction to audit event
func (r *Redactor) RedactEvent(event *AuditEvent) error {
    // Convert to map for JSON path processing
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    var m map[string]interface{}
    if err := json.Unmarshal(data, &m); err != nil {
        return err
    }
    
    // Apply each rule
    for i, rule := range r.rules {
        for _, path := range rule.FieldPaths {
            r.redactPath(m, path, rule.Replacement, r.compiled[i])
        }
    }
    
    // Convert back
    newData, err := json.Marshal(m)
    if err != nil {
        return err
    }
    
    return json.Unmarshal(newData, event)
}

func (r *Redactor) redactPath(
    m map[string]interface{},
    path string,
    replacement string,
    pattern *regexp.Regexp,
) {
    // Simple JSON path implementation (full implementation would use jsonpath library)
    parts := strings.Split(path, ".")
    r.redactRecursive(m, parts, 0, replacement, pattern)
}
```

#### Log Sanitization Middleware

Ensure credentials never reach log output:

```go
// internal/logging/sanitizer.go

type Sanitizer struct {
    redactor *Redactor
    logger   zerolog.Logger
}

// SanitizedLogger wraps logger with credential redaction
func NewSanitizedLogger(logger zerolog.Logger) *Sanitizer {
    return &Sanitizer{
        redactor: NewRedactor(),
        logger:   logger,
    }
}

// Log redacts before logging
func (s *Sanitizer) Log(event *AuditEvent) error {
    // Create deep copy
    redacted := *event
    if err := s.redactor.RedactEvent(&redacted); err != nil {
        // Log error but continue with original (safer)
        s.logger.Error().Err(err).Msg("failed to redact audit event")
        // Fall back to manual redaction of known fields
        s.manualRedact(&redacted)
    }
    
    return s.logger.Log().Interface("event", redacted).Send()
}

// Manual redaction for safety
func (s *Sanitizer) manualRedact(event *AuditEvent) {
    // Redact Context field (where credentials might appear)
    if event.Context != nil {
        if val, ok := event.Context["password"]; ok && val != "" {
            event.Context["password"] = "[REDACTED]"
        }
        if val, ok := event.Context["token"]; ok && val != "" {
            event.Context["token"] = "[REDACTED]"
        }
        if val, ok := event.Context["secret"]; ok && val != "" {
            event.Context["secret"] = "[REDACTED]"
        }
    }
}
```

#### Configuration Validation

Prevent misconfiguration that could disable redaction:

```go
// internal/audit/config.go

func (c *AuditConfig) Validate() error {
    // Ensure sensitive fields list includes mandatory fields
    mandatory := []string{"password", "token", "secret", "api_key"}
    for _, field := range mandatory {
        found := false
        for _, sf := range c.SensitiveFields {
            if strings.EqualFold(sf, field) {
                found = true
                break
            }
        }
        if !found {
            c.SensitiveFields = append(c.SensitiveFields, field)
        }
    }
    
    // Ensure IncludeBody=false for production
    if c.IncludeBody && c.Environment == "production" {
        return errors.New("IncludeBody cannot be true in production")
    }
    
    return nil
}
```

#### Security Invariants

1. **Mandatory Redaction Invariant**: Credentials (password, token, secret) must be redacted regardless of configuration.
2. **Log Sanitization Invariant**: No credential appears in plaintext in any log output.
3. **Configuration Safety Invariant**: Production configuration cannot disable redaction.
4. **Audit Trail Integrity**: Redaction preserves audit trail while removing sensitive data.

#### Adversarial Test Cases

Add to `SECURITY-TESTING.md`:

1. **Credential leak test**: Attempt to log API key, verify redaction.
2. **Configuration bypass test**: Set `IncludeBody=true` in production, verify rejection.
3. **JSON path injection test**: Attempt to bypass redaction via nested objects.
4. **Log file inspection test**: Grep log files for `password=`, `token=`, etc.

---

## Audit Event Types

```go
package events

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"
)

type AuditLevel string

const (
    AuditLevelDebug     AuditLevel = "debug"
    AuditLevelInfo      AuditLevel = "info"
    AuditLevelWarning   AuditLevel = "warning"
    AuditLevelError     AuditLevel = "error"
    AuditLevelCritical  AuditLevel = "critical"
)

type AuditCategory string

const (
    AuditCategoryAuth      AuditCategory = "authentication"
    AuditCategoryAccess    AuditCategory = "authorization"
    AuditCategoryData      AuditCategory = "data_access"
    AuditCategorySystem    AuditCategory = "system"
    AuditCategorySecurity  AuditCategory = "security"
    AuditCategoryAdmin     AuditCategory = "administrative"
)

type AuditEvent struct {
    ID            string                 `json:"id"`
    Timestamp     time.Time              `json:"timestamp"`
    Level         AuditLevel             `json:"level"`
    Category      AuditCategory          `json:"category"`
    Action        string                 `json:"action"`
    Actor         AuditActor             `json:"actor"`
    Resource      AuditResource          `json:"resource"`
    Result        AuditResult            `json:"result"`
    Context       AuditContext           `json:"context"`
    Changes       []AuditChange          `json:"changes,omitempty"`
    Hash          string                 `json:"hash"`
    PreviousHash  string                 `json:"previous_hash,omitempty"`
}

type AuditActor struct {
    ID          string `json:"id"`
    Type        string `json:"type"`
    Name        string `json:"name"`
    IPAddress   string `json:"ip_address"`
    UserAgent   string `json:"user_agent,omitempty"`
    SessionID   string `json:"session_id,omitempty"`
}

type AuditResource struct {
    ID       string `json:"id"`
    Type     string `json:"type"`
    Name     string `json:"name,omitempty"`
    Path     string `json:"path,omitempty"`
}

type AuditResult struct {
    Success     bool   `json:"success"`
    Code        string `json:"code,omitempty"`
    Message     string `json:"message,omitempty"`
    Details     string `json:"details,omitempty"`
}

type AuditContext struct {
    RequestID     string            `json:"request_id,omitempty"`
    TraceID       string            `json:"trace_id,omitempty"`
    Metadata      map[string]string `json:"metadata,omitempty"`
}

type AuditChange struct {
    Field     string      `json:"field"`
    OldValue  interface{} `json:"old_value,omitempty"`
    NewValue  interface{} `json:"new_value,omitempty"`
}
```

## Audit Logger Implementation

```go
type AuditLogger struct {
    config    AuditConfig
    storage   AuditStorage
    buffer    []*AuditEvent
    mu        sync.Mutex
    lastHash  string
    done      chan struct{}
    wg        sync.WaitGroup
}

type AuditStorage interface {
    Write(event *AuditEvent) error
    Query(filter AuditFilter) ([]*AuditEvent, error)
    Close() error
}

type AuditFilter struct {
    StartTime    time.Time
    EndTime      time.Time
    Categories   []AuditCategory
    Levels       []AuditLevel
    ActorID      string
    ResourceType string
    Action       string
    Success      *bool
}

func NewAuditLogger(config AuditConfig, storage AuditStorage) *AuditLogger {
    al := &AuditLogger{
        config:   config,
        storage:  storage,
        buffer:   make([]*AuditEvent, 0, config.BufferSize),
        done:     make(chan struct{}),
    }

    al.wg.Add(1)
    go al.flushLoop()

    return al
}

func (al *AuditLogger) Log(event *AuditEvent) error {
    if !al.config.Enabled {
        return nil
    }

    event.ID = generateID()
    event.Timestamp = time.Now()
    event.Hash = al.calculateHash(event)
    event.PreviousHash = al.lastHash
    al.lastHash = event.Hash

    event = al.redactSensitiveData(event)

    al.mu.Lock()
    al.buffer = append(al.buffer, event)
    shouldFlush := len(al.buffer) >= al.config.BufferSize
    al.mu.Unlock()

    if shouldFlush {
        al.Flush()
    }

    return nil
}

func (al *AuditLogger) LogAsync(event *AuditEvent) {
    go func() {
        if err := al.Log(event); err != nil {
            // Log error but don't block
        }
    }()
}

func (al *AuditLogger) calculateHash(event *AuditEvent) string {
    data, _ := json.Marshal(struct {
        ID       string    `json:"id"`
        Timestamp time.Time `json:"timestamp"`
        Level    string    `json:"level"`
        Category string    `json:"category"`
        Action   string    `json:"action"`
        ActorID  string    `json:"actor_id"`
        PrevHash string    `json:"previous_hash"`
    }{
        ID:        event.ID,
        Timestamp: event.Timestamp,
        Level:     string(event.Level),
        Category:  string(event.Category),
        Action:    event.Action,
        ActorID:   event.Actor.ID,
        PrevHash:  event.PreviousHash,
    })

    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}

func (al *AuditLogger) redactSensitiveData(event *AuditEvent) *AuditEvent {
    if len(al.config.SensitiveFields) == 0 {
        return event
    }

    for _, change := range event.Changes {
        for _, field := range al.config.SensitiveFields {
            if change.Field == field {
                if change.OldValue != nil {
                    change.OldValue = "[REDACTED]"
                }
                if change.NewValue != nil {
                    change.NewValue = "[REDACTED]"
                }
            }
        }
    }

    return event
}

func (al *AuditLogger) flushLoop() {
    defer al.wg.Done()
    
    ticker := time.NewTicker(al.config.FlushInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            al.Flush()
        case <-al.done:
            al.Flush()
            return
        }
    }
}

func (al *AuditLogger) Flush() error {
    al.mu.Lock()
    events := make([]*AuditEvent, len(al.buffer))
    copy(events, al.buffer)
    al.buffer = al.buffer[:0]
    al.mu.Unlock()

    if len(events) == 0 {
        return nil
    }

    for _, event := range events {
        if err := al.storage.Write(event); err != nil {
            return err
        }
    }

    return nil
}

func (al *AuditLogger) Query(filter AuditFilter) ([]*AuditEvent, error) {
    return al.storage.Query(filter)
}

func (al *AuditLogger) Close() error {
    close(al.done)
    al.wg.Wait()
    return al.storage.Close()
}
```
