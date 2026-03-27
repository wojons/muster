package auth

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"
)

type RevocationCache struct {
	ID        int64     `db:"id" json:"id"`
	TokenHash string    `db:"token_hash" json:"token_hash"`
	Reason    string    `db:"reason" json:"reason"`
	RevokedAt time.Time `db:"revoked_at" json:"revoked_at"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type DatabaseRevocationStore struct {
	db *sql.DB
}

func NewDatabaseRevocationStore(db *sql.DB) *DatabaseRevocationStore {
	return &DatabaseRevocationStore{
		db: db,
	}
}

func (s *DatabaseRevocationStore) Revoke(token string, expiresAt time.Time) error {
	tokenHash := hashToken(token)
	reason := "manually revoked"

	query := `
		INSERT INTO revocation_cache (token_hash, reason, revoked_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(token_hash) DO UPDATE SET
			reason = excluded.reason,
			revoked_at = excluded.revoked_at,
			expires_at = excluded.expires_at,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query,
		tokenHash,
		reason,
		time.Now(),
		expiresAt,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		var sqliteErr sqlite3.Error
		if ok := errors.As(err, &sqliteErr); ok {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return fmt.Errorf("token revocation constraint violation: %w", err)
			}
		}
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

func (s *DatabaseRevocationStore) IsRevoked(token string) (bool, error) {
	tokenHash := hashToken(token)

	query := `
		SELECT id, token_hash, reason, revoked_at, expires_at, created_at, updated_at
		FROM revocation_cache
		WHERE token_hash = ? AND expires_at > ?
	`

	var rc RevocationCache
	err := s.db.QueryRow(query, tokenHash, time.Now()).Scan(
		&rc.ID,
		&rc.TokenHash,
		&rc.Reason,
		&rc.RevokedAt,
		&rc.ExpiresAt,
		&rc.CreatedAt,
		&rc.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to check revocation status: %w", err)
	}

	return true, nil
}

func (s *DatabaseRevocationStore) Cleanup(before time.Time) error {
	query := `DELETE FROM revocation_cache WHERE expires_at < ?`

	_, err := s.db.Exec(query, before)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired revocations: %w", err)
	}

	return nil
}

func (s *DatabaseRevocationStore) RevokeWithReason(token, reason string, expiresAt time.Time) error {
	tokenHash := hashToken(token)

	query := `
		INSERT INTO revocation_cache (token_hash, reason, revoked_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(token_hash) DO UPDATE SET
			reason = excluded.reason,
			revoked_at = excluded.revoked_at,
			expires_at = excluded.expires_at,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query,
		tokenHash,
		reason,
		time.Now(),
		expiresAt,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		var sqliteErr sqlite3.Error
		if ok := errors.As(err, &sqliteErr); ok {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return fmt.Errorf("token revocation constraint violation: %w", err)
			}
		}
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

func (s *DatabaseRevocationStore) GetRevokedToken(token string) (*RevocationCache, error) {
	tokenHash := hashToken(token)

	query := `
		SELECT id, token_hash, reason, revoked_at, expires_at, created_at, updated_at
		FROM revocation_cache
		WHERE token_hash = ?
	`

	var rc RevocationCache
	err := s.db.QueryRow(query, tokenHash).Scan(
		&rc.ID,
		&rc.TokenHash,
		&rc.Reason,
		&rc.RevokedAt,
		&rc.ExpiresAt,
		&rc.CreatedAt,
		&rc.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get revoked token: %w", err)
	}

	return &rc, nil
}

func (s *DatabaseRevocationStore) ListRevokedTokens(limit int) ([]*RevocationCache, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, token_hash, reason, revoked_at, expires_at, created_at, updated_at
		FROM revocation_cache
		WHERE expires_at > ?
		ORDER BY revoked_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, time.Now(), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list revoked tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var revoked []*RevocationCache
	for rows.Next() {
		var rc RevocationCache
		err := rows.Scan(
			&rc.ID,
			&rc.TokenHash,
			&rc.Reason,
			&rc.RevokedAt,
			&rc.ExpiresAt,
			&rc.CreatedAt,
			&rc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan revocation cache: %w", err)
		}
		revoked = append(revoked, &rc)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating revoked tokens: %w", err)
	}

	return revoked, nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}
