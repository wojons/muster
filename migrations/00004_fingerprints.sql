-- +goose Up
-- +goose StatementBegin

-- Create fingerprints table
CREATE TABLE IF NOT EXISTS fingerprints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hash VARCHAR(255) UNIQUE NOT NULL,
    algorithm VARCHAR(50) NOT NULL DEFAULT 'sha256',
    spec_id INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (spec_id) REFERENCES specs(id) ON DELETE CASCADE
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_fingerprints_hash ON fingerprints(hash);
CREATE INDEX IF NOT EXISTS idx_fingerprints_spec ON fingerprints(spec_id);
CREATE INDEX IF NOT EXISTS idx_fingerprints_algorithm ON fingerprints(algorithm);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS fingerprints;

-- +goose StatementEnd
