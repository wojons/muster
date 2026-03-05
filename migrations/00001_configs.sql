-- +goose Up
-- +goose StatementBegin

-- Create configs table
CREATE TABLE IF NOT EXISTS configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key VARCHAR(255) UNIQUE NOT NULL,
    value TEXT NOT NULL,
    profile VARCHAR(255) DEFAULT 'default',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for configs
CREATE INDEX IF NOT EXISTS idx_configs_key ON configs(key);
CREATE INDEX IF NOT EXISTS idx_configs_profile ON configs(profile);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS configs;

-- +goose StatementEnd
