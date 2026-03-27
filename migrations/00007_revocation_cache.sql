-- +goose Up
-- +goose StatementBegin

-- Create revocation_cache table for persistent token revocation
CREATE TABLE IF NOT EXISTS revocation_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash VARCHAR(64) NOT NULL,
    reason VARCHAR(255),
    revoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(token_hash)
);

-- Create indexes for efficient lookup and cleanup
CREATE INDEX IF NOT EXISTS idx_revocation_cache_token ON revocation_cache(token_hash);
CREATE INDEX IF NOT EXISTS idx_revocation_cache_expires ON revocation_cache(expires_at);
CREATE INDEX IF NOT EXISTS idx_revocation_cache_revoked_at ON revocation_cache(revoked_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS revocation_cache;

-- +goose StatementEnd
