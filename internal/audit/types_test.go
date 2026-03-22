package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLevel_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		level    AuditLevel
		expected bool
	}{
		{"debug level", AuditLevelDebug, true},
		{"info level", AuditLevelInfo, true},
		{"warning level", AuditLevelWarning, true},
		{"error level", AuditLevelError, true},
		{"critical level", AuditLevelCritical, true},
		{"empty level", "", false},
		{"unknown level", "unknown", false},
		{"random level", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.IsValid())
		})
	}
}

func TestAuditCategory_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		category AuditCategory
		expected bool
	}{
		{"authentication category", AuditCategoryAuth, true},
		{"authorization category", AuditCategoryAccess, true},
		{"data_access category", AuditCategoryData, true},
		{"system category", AuditCategorySystem, true},
		{"security category", AuditCategorySecurity, true},
		{"administrative category", AuditCategoryAdmin, true},
		{"empty category", "", false},
		{"unknown category", "unknown", false},
		{"random category", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.IsValid())
		})
	}
}

func TestAuditActor_Validate(t *testing.T) {
	tests := []struct {
		name    string
		actor   AuditActor
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid actor",
			actor:   AuditActor{ID: "user-1", Type: "user", Name: "John"},
			wantErr: false,
		},
		{
			name:    "missing ID",
			actor:   AuditActor{Type: "user", Name: "John"},
			wantErr: true,
			errMsg:  "actor ID is required",
		},
		{
			name:    "missing type",
			actor:   AuditActor{ID: "user-1", Name: "John"},
			wantErr: true,
			errMsg:  "actor type is required",
		},
		{
			name:    "empty actor",
			actor:   AuditActor{},
			wantErr: true,
			errMsg:  "actor ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.actor.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuditResource_Validate(t *testing.T) {
	tests := []struct {
		name     string
		resource AuditResource
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid resource",
			resource: AuditResource{ID: "doc-1", Type: "document", Name: "Report"},
			wantErr:  false,
		},
		{
			name:     "missing ID",
			resource: AuditResource{Type: "document", Name: "Report"},
			wantErr:  true,
			errMsg:   "resource ID is required",
		},
		{
			name:     "missing type",
			resource: AuditResource{ID: "doc-1", Name: "Report"},
			wantErr:  true,
			errMsg:   "resource type is required",
		},
		{
			name:     "empty resource",
			resource: AuditResource{},
			wantErr:  true,
			errMsg:   "resource ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resource.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuditEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   *AuditEvent
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			event: &AuditEvent{
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "event ID is required",
		},
		{
			name: "missing timestamp",
			event: &AuditEvent{
				ID:       "evt-1",
				Level:    AuditLevelInfo,
				Category: AuditCategoryAuth,
				Action:   "login",
				Actor:    AuditActor{ID: "user-1", Type: "user"},
				Resource: AuditResource{ID: "auth-1", Type: "service"},
				Result:   AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "timestamp is required",
		},
		{
			name: "invalid level",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     "invalid",
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "invalid audit level",
		},
		{
			name: "invalid category",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  "invalid",
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "invalid audit category",
		},
		{
			name: "missing action",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "action is required",
		},
		{
			name: "invalid actor",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{Type: "user"}, // Missing ID
				Resource:  AuditResource{ID: "auth-1", Type: "service"},
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "invalid actor",
		},
		{
			name: "invalid resource",
			event: &AuditEvent{
				ID:        "evt-1",
				Timestamp: now,
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user"},
				Resource:  AuditResource{Type: "service"}, // Missing ID
				Result:    AuditResult{Success: true},
			},
			wantErr: true,
			errMsg:  "invalid resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuditEvent_ComputeHash(t *testing.T) {
	now := time.Now()

	event := &AuditEvent{
		ID:        "evt-1",
		Timestamp: now,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAuth,
		Action:    "login",
		Actor:     AuditActor{ID: "user-1", Type: "user", Name: "John"},
		Resource:  AuditResource{ID: "auth-1", Type: "service"},
		Result:    AuditResult{Success: true},
		Context:   AuditContext{RequestID: "req-1"},
	}

	// Compute hash
	hash := event.ComputeHash()
	require.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex is 64 characters

	// Same event should produce same hash
	hash2 := event.ComputeHash()
	assert.Equal(t, hash, hash2)

	// Different event should produce different hash
	event2 := &AuditEvent{
		ID:        "evt-2",
		Timestamp: now,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAuth,
		Action:    "login",
		Actor:     AuditActor{ID: "user-1", Type: "user", Name: "John"},
		Resource:  AuditResource{ID: "auth-1", Type: "service"},
		Result:    AuditResult{Success: true},
		Context:   AuditContext{RequestID: "req-1"},
	}
	hash3 := event2.ComputeHash()
	assert.NotEqual(t, hash, hash3)
}

func TestAuditEvent_ComputeHash_WithPreviousHash(t *testing.T) {
	now := time.Now()

	event1 := &AuditEvent{
		ID:        "evt-1",
		Timestamp: now,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAuth,
		Action:    "login",
		Actor:     AuditActor{ID: "user-1", Type: "user"},
		Resource:  AuditResource{ID: "auth-1", Type: "service"},
		Result:    AuditResult{Success: true},
	}

	hash1 := event1.ComputeHash()

	// Event with previous hash should produce different hash
	event2 := &AuditEvent{
		ID:           "evt-2",
		Timestamp:    now.Add(time.Second),
		Level:        AuditLevelInfo,
		Category:     AuditCategoryAuth,
		Action:       "logout",
		Actor:        AuditActor{ID: "user-1", Type: "user"},
		Resource:     AuditResource{ID: "auth-1", Type: "service"},
		Result:       AuditResult{Success: true},
		PreviousHash: hash1,
	}

	hash2 := event2.ComputeHash()
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2)
}

func TestAuditEvent_SetHash(t *testing.T) {
	now := time.Now()

	event := &AuditEvent{
		ID:        "evt-1",
		Timestamp: now,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAuth,
		Action:    "login",
		Actor:     AuditActor{ID: "user-1", Type: "user"},
		Resource:  AuditResource{ID: "auth-1", Type: "service"},
		Result:    AuditResult{Success: true},
	}

	previousHash := "abc123"
	event.SetHash(previousHash)

	assert.Equal(t, previousHash, event.PreviousHash)
	assert.NotEmpty(t, event.Hash)
	assert.Len(t, event.Hash, 64)
}

func TestAuditEvent_String(t *testing.T) {
	now := time.Now()

	event := &AuditEvent{
		ID:        "evt-1",
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAuth,
		Action:    "login",
		Actor:     AuditActor{Name: "user@example.com"},
		Resource:  AuditResource{Name: "auth-service"},
		Result:    AuditResult{Success: true},
		Timestamp: now,
	}

	str := event.String()
	assert.Contains(t, str, "evt-1")
	assert.Contains(t, str, "info")
	assert.Contains(t, str, "authentication")
	assert.Contains(t, str, "login")
	assert.Contains(t, str, "user@example.com")
	assert.Contains(t, str, "auth-service")
	assert.Contains(t, str, "true")
}

func TestAuditEvent_Clone(t *testing.T) {
	now := time.Now()

	original := &AuditEvent{
		ID:           "evt-1",
		Timestamp:    now,
		Level:        AuditLevelInfo,
		Category:     AuditCategoryAuth,
		Action:       "login",
		Actor:        AuditActor{ID: "user-1", Type: "user", Name: "John"},
		Resource:     AuditResource{ID: "auth-1", Type: "service"},
		Result:       AuditResult{Success: true, Code: "200"},
		Context:      AuditContext{RequestID: "req-1", Metadata: map[string]string{"key": "value"}},
		Changes:      []AuditChange{{Field: "status", OldValue: "pending", NewValue: "active"}},
		Hash:         "hash1",
		PreviousHash: "hash0",
	}

	clone := original.Clone()

	// Verify clone has same values
	require.NotNil(t, clone)
	assert.Equal(t, original.ID, clone.ID)
	assert.Equal(t, original.Timestamp, clone.Timestamp)
	assert.Equal(t, original.Level, clone.Level)
	assert.Equal(t, original.Category, clone.Category)
	assert.Equal(t, original.Action, clone.Action)
	assert.Equal(t, original.Actor, clone.Actor)
	assert.Equal(t, original.Resource, clone.Resource)
	assert.Equal(t, original.Result, clone.Result)
	assert.Equal(t, original.Context.RequestID, clone.Context.RequestID)
	assert.Equal(t, original.Hash, clone.Hash)
	assert.Equal(t, original.PreviousHash, clone.PreviousHash)
	assert.Equal(t, original.Changes, clone.Changes)
	assert.Equal(t, original.Context.Metadata, clone.Context.Metadata)

	// Verify clone is independent (deep copy)
	clone.Changes[0].Field = "modified"
	clone.Context.Metadata["key"] = "modified"
	assert.Equal(t, "status", original.Changes[0].Field)
	assert.Equal(t, "value", original.Context.Metadata["key"])
}

func TestAuditEvent_Clone_Nil(t *testing.T) {
	var event *AuditEvent
	clone := event.Clone()
	assert.Nil(t, clone)
}

func TestAuditResult(t *testing.T) {
	tests := []struct {
		name    string
		result  AuditResult
		wantErr bool
	}{
		{
			name:    "successful result",
			result:  AuditResult{Success: true, Code: "200", Message: "OK"},
			wantErr: false,
		},
		{
			name:    "failed result",
			result:  AuditResult{Success: false, Code: "404", Message: "Not Found"},
			wantErr: false,
		},
		{
			name:    "result with details",
			result:  AuditResult{Success: false, Code: "500", Message: "Error", Details: "Stack trace here"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.result.Success, tt.result.Success)
			assert.Equal(t, tt.result.Code, tt.result.Code)
			assert.Equal(t, tt.result.Message, tt.result.Message)
			assert.Equal(t, tt.result.Details, tt.result.Details)
		})
	}
}

func TestAuditContext(t *testing.T) {
	ctx := AuditContext{
		RequestID: "req-123",
		TraceID:   "trace-456",
		Metadata: map[string]string{
			"client_version": "1.0.0",
			"region":         "us-east-1",
		},
	}

	assert.Equal(t, "req-123", ctx.RequestID)
	assert.Equal(t, "trace-456", ctx.TraceID)
	assert.Equal(t, "1.0.0", ctx.Metadata["client_version"])
	assert.Equal(t, "us-east-1", ctx.Metadata["region"])
}

func TestAuditChange(t *testing.T) {
	change := AuditChange{
		Field:    "password",
		OldValue: "oldpass123",
		NewValue: "newpass456",
	}

	assert.Equal(t, "password", change.Field)
	assert.Equal(t, "oldpass123", change.OldValue)
	assert.Equal(t, "newpass456", change.NewValue)
}

func TestAuditEvent_WithChanges(t *testing.T) {
	now := time.Now()

	event := &AuditEvent{
		ID:        "evt-1",
		Timestamp: now,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAdmin,
		Action:    "update_user",
		Actor:     AuditActor{ID: "admin-1", Type: "admin"},
		Resource:  AuditResource{ID: "user-1", Type: "user"},
		Result:    AuditResult{Success: true},
		Changes: []AuditChange{
			{Field: "email", OldValue: "old@example.com", NewValue: "new@example.com"},
			{Field: "role", OldValue: "user", NewValue: "admin"},
		},
	}

	assert.Len(t, event.Changes, 2)
	assert.Equal(t, "email", event.Changes[0].Field)
	assert.Equal(t, "role", event.Changes[1].Field)
}
