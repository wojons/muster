package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

var (
	// ErrErasureFailed is returned when data erasure fails
	ErrErasureFailed = fmt.Errorf("data erasure failed")

	// ErrErasureNotAllowed is returned when erasure is not permitted
	ErrErasureNotAllowed = fmt.Errorf("data erasure not allowed")
)

// ErasureRequest represents a GDPR Article 17 data subject erasure request
type ErasureRequest struct {
	ID           string
	UserID       string
	Reason       string
	RequestType  ErasureType
	Status       ErasureStatus
	RequestedAt  time.Time
	ProcessedAt  *time.Time
	DeletedCount int64
	Error        string
}

// ErasureType defines the type of erasure request
type ErasureType string

const (
	// ErasureTypeFull - complete data deletion
	ErasureTypeFull ErasureType = "FULL"
	// ErasureTypePartial - selective data deletion
	ErasureTypePartial ErasureType = "PARTIAL"
	// ErasureTypeAnonymize - anonymize instead of delete
	ErasureTypeAnonymize ErasureType = "ANONYMIZE"
)

// ErasureStatus defines the status of an erasure request
type ErasureStatus string

const (
	// ErasureStatusPending - request awaiting processing
	ErasureStatusPending ErasureStatus = "PENDING"
	// ErasureStatusProcessing - currently being processed
	ErasureStatusProcessing ErasureStatus = "PROCESSING"
	// ErasureStatusCompleted - successfully completed
	ErasureStatusCompleted ErasureStatus = "COMPLETED"
	// ErasureStatusFailed - processing failed
	ErasureStatusFailed ErasureStatus = "FAILED"
)

// ErasureResult contains the results of an erasure operation
type ErasureResult struct {
	RequestID            string
	UserID               string
	TablesAffected       []string
	RowsDeleted          int64
	CacheKeysDeleted     int
	LogEntriesRedacted   int
	KeychainItemsDeleted int
	Duration             time.Duration
	Error                error
}

// CacheDeleter is an interface for cache deletion
type CacheDeleter interface {
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
}

// KeychainDeleter is an interface for keychain deletion
type KeychainDeleter interface {
	Delete(ctx context.Context, key string) error
}

// DataSubjectErasure performs GDPR Article 17 right to erasure
// It cascade deletes all user data across all tables while maintaining audit trail
func (sm *SQLiteManager) DataSubjectErasure(ctx context.Context, userID string, requestType ErasureType, cache CacheDeleter, keychain KeychainDeleter) (*ErasureResult, error) {
	startTime := time.Now()

	requestID := fmt.Sprintf("erasure-%s-%d", userID, startTime.UnixNano())

	result := &ErasureResult{
		RequestID:      requestID,
		UserID:         userID,
		TablesAffected: make([]string, 0),
	}

	tx, err := sm.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: begin transaction: %v", ErrErasureFailed, err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	tables := []string{
		"revocation_cache",
		"rate_limits",
		"fingerprints",
		"specs",
		"modules",
		"aliases",
		"inventories",
		"configs",
	}

	for _, table := range tables {
		deleted, err := sm.deleteUserRecordsFromTable(ctx, tx, table, userID, requestType)
		if err != nil {
			result.Error = fmt.Errorf("delete from %s: %w", table, err)
			return result, result.Error
		}

		if deleted > 0 {
			result.TablesAffected = append(result.TablesAffected, table)
			result.RowsDeleted += deleted

			sm.log.Info("GDPR erasure: deleted records",
				zap.String("table", table),
				zap.Int64("count", deleted),
				zap.String("user_id", userID),
				zap.String("request_id", requestID),
			)
		}
	}

	if cache != nil {
		cacheDeleted := sm.deleteUserCache(ctx, cache, userID)
		result.CacheKeysDeleted = cacheDeleted
	}

	logRedacted := sm.redactUserLogs(ctx, tx, userID)
	result.LogEntriesRedacted = logRedacted

	if keychain != nil {
		keychainDeleted := sm.deleteUserKeychain(ctx, keychain, userID)
		result.KeychainItemsDeleted = keychainDeleted
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("%w: commit transaction: %v", ErrErasureFailed, err)
	}

	result.Duration = time.Since(startTime)

	sm.log.Info("GDPR erasure completed",
		zap.String("user_id", userID),
		zap.String("request_id", requestID),
		zap.String("request_type", string(requestType)),
		zap.Int64("rows_deleted", result.RowsDeleted),
		zap.Int("tables_affected", len(result.TablesAffected)),
		zap.Int("cache_keys_deleted", result.CacheKeysDeleted),
		zap.Int("log_entries_redacted", result.LogEntriesRedacted),
		zap.Int("keychain_items_deleted", result.KeychainItemsDeleted),
		zap.Duration("duration", result.Duration),
	)

	return result, nil
}

func (sm *SQLiteManager) deleteUserRecordsFromTable(ctx context.Context, tx *sql.Tx, table, userID string, requestType ErasureType) (int64, error) {
	var query string
	var args []interface{}

	switch requestType {
	case ErasureTypeFull, ErasureTypePartial:
		query = fmt.Sprintf("DELETE FROM %s WHERE user_id = ?", table)
		args = []interface{}{userID}

	case ErasureTypeAnonymize:
		query = fmt.Sprintf("UPDATE %s SET user_id = ? WHERE user_id = ?", table)
		anonymizedID := fmt.Sprintf("anonymous-%d", time.Now().UnixNano())
		args = []interface{}{anonymizedID, userID}

	default:
		return 0, fmt.Errorf("unknown erasure type: %s", requestType)
	}

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (sm *SQLiteManager) deleteUserCache(ctx context.Context, cache CacheDeleter, userID string) int {
	pattern := fmt.Sprintf("user:%s:*", userID)
	keys, err := cache.Keys(ctx, pattern)
	if err != nil {
		sm.log.Warn("GDPR erasure: failed to get cache keys",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return 0
	}

	deleted := 0
	for _, key := range keys {
		if err := cache.Delete(ctx, key); err != nil {
			sm.log.Warn("GDPR erasure: failed to delete cache key",
				zap.String("key", key),
				zap.Error(err),
			)
			continue
		}
		deleted++
	}

	return deleted
}

func (sm *SQLiteManager) redactUserLogs(ctx context.Context, tx *sql.Tx, userID string) int {
	query := `UPDATE audit 
			  SET data = REPLACE(data, ?, '[REDACTED]')
			  WHERE user_id = ?`

	result, err := tx.ExecContext(ctx, query, userID, userID)
	if err != nil {
		sm.log.Warn("GDPR erasure: failed to redact logs",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return 0
	}

	count, _ := result.RowsAffected()
	return int(count)
}

func (sm *SQLiteManager) deleteUserKeychain(ctx context.Context, keychain KeychainDeleter, userID string) int {
	credentialsDeleted := 0

	if err := keychain.Delete(ctx, fmt.Sprintf("muster.credentials.%s", userID)); err == nil {
		credentialsDeleted++
	}

	if err := keychain.Delete(ctx, fmt.Sprintf("muster.tokens.%s", userID)); err == nil {
		credentialsDeleted++
	}

	if err := keychain.Delete(ctx, fmt.Sprintf("muster.refresh.%s", userID)); err == nil {
		credentialsDeleted++
	}

	return credentialsDeleted
}
