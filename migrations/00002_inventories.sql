-- +goose Up
-- +goose StatementBegin

-- Create inventories table
CREATE TABLE IF NOT EXISTS inventories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    url VARCHAR(512) UNIQUE NOT NULL,
    data JSON NOT NULL,
    etag VARCHAR(255),
    fetched_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for inventories
CREATE INDEX IF NOT EXISTS idx_inventories_name ON inventories(name);
CREATE INDEX IF NOT EXISTS idx_inventories_url ON inventories(url);
CREATE INDEX IF NOT EXISTS idx_inventories_fetched_at ON inventories(fetched_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS inventories;

-- +goose StatementEnd
