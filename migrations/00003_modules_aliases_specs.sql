-- +goose Up
-- +goose StatementBegin

-- Create modules table
CREATE TABLE IF NOT EXISTS modules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    path TEXT NOT NULL,
    language VARCHAR(50) NOT NULL,
    inventory_id INTEGER NOT NULL,
    spec_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inventory_id) REFERENCES inventories(id) ON DELETE CASCADE,
    FOREIGN KEY (spec_id) REFERENCES specs(id) ON DELETE SET NULL
);

-- Create aliases table
CREATE TABLE IF NOT EXISTS aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    inventory_id INTEGER NOT NULL,
    module_id INTEGER,
    spec_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inventory_id) REFERENCES inventories(id) ON DELETE CASCADE,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE SET NULL,
    FOREIGN KEY (spec_id) REFERENCES specs(id) ON DELETE SET NULL
);

-- Create specs table
CREATE TABLE IF NOT EXISTS specs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    fingerprint VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_modules_name ON modules(name);
CREATE INDEX IF NOT EXISTS idx_modules_inventory ON modules(inventory_id);
CREATE INDEX IF NOT EXISTS idx_modules_language ON modules(language);
CREATE INDEX IF NOT EXISTS idx_aliases_name ON aliases(name);
CREATE INDEX IF NOT EXISTS idx_aliases_inventory ON aliases(inventory_id);
CREATE INDEX IF NOT EXISTS idx_aliases_module ON aliases(module_id);
CREATE INDEX IF NOT EXISTS idx_aliases_spec ON aliases(spec_id);
CREATE INDEX IF NOT EXISTS idx_specs_name ON specs(name);
CREATE INDEX IF NOT EXISTS idx_specs_fingerprint ON specs(fingerprint);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS aliases;
DROP TABLE IF EXISTS modules;
DROP TABLE IF EXISTS specs;

-- +goose StatementEnd
