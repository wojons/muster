-- +goose Up
-- +goose StatementBegin

-- Create data_residency_policies table for GDPR Article 44-49 compliance
CREATE TABLE IF NOT EXISTS data_residency_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    allowed_regions TEXT,
    enforce_mode VARCHAR(20) NOT NULL DEFAULT 'audit',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create data_residency_violations table for tracking violations
CREATE TABLE IF NOT EXISTS data_residency_violations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    policy_id INTEGER,
    policy_name VARCHAR(255) NOT NULL,
    entity_type VARCHAR(100) NOT NULL,
    entity_id INTEGER NOT NULL,
    attempted_region VARCHAR(10) NOT NULL,
    allowed_regions TEXT NOT NULL,
    action VARCHAR(50) NOT NULL,
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    user_id VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (policy_id) REFERENCES data_residency_policies(id) ON DELETE SET NULL
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_residency_policies_created ON data_residency_policies(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_residency_violations_policy ON data_residency_violations(policy_id);
CREATE INDEX IF NOT EXISTS idx_residency_violations_entity ON data_residency_violations(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_residency_violations_region ON data_residency_violations(attempted_region);
CREATE INDEX IF NOT EXISTS idx_residency_violations_created ON data_residency_violations(created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_residency_violations_created;
DROP INDEX IF EXISTS idx_residency_violations_region;
DROP INDEX IF EXISTS idx_residency_violations_entity;
DROP INDEX IF EXISTS idx_residency_violations_policy;
DROP INDEX IF EXISTS idx_residency_policies_created;
DROP TABLE IF EXISTS data_residency_violations;
DROP TABLE IF EXISTS data_residency_policies;

-- +goose StatementEnd
