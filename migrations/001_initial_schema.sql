-- +migrate Up
-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 1
);

-- Create sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- Create catalog_entries table
CREATE TABLE IF NOT EXISTS catalog_entries (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT,
    description TEXT,
    phone_number TEXT,
    location TEXT,
    thumbnail_path TEXT,
    archive_path TEXT,
    archive_status TEXT DEFAULT 'pending',
    metadata_raw TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_user_id ON catalog_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_url ON catalog_entries(url);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_created_at ON catalog_entries(created_at);

-- Create tags table
CREATE TABLE IF NOT EXISTS tags (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    color TEXT DEFAULT '#6b7280',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, name)
);
CREATE INDEX IF NOT EXISTS idx_tags_user_id ON tags(user_id);

-- Create entry_tags junction table
CREATE TABLE IF NOT EXISTS entry_tags (
    entry_id TEXT NOT NULL,
    tag_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entry_id, tag_id),
    FOREIGN KEY (entry_id) REFERENCES catalog_entries(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entry_tags_entry_id ON entry_tags(entry_id);
CREATE INDEX IF NOT EXISTS idx_entry_tags_tag_id ON entry_tags(tag_id);

-- Create interactions table
CREATE TABLE IF NOT EXISTS interactions (
    id TEXT PRIMARY KEY,
    entry_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    tried BOOLEAN DEFAULT 0,
    score INTEGER,
    comments TEXT,
    contacted_at DATETIME,
    purchased_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (entry_id) REFERENCES catalog_entries(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(entry_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_interactions_entry_id ON interactions(entry_id);
CREATE INDEX IF NOT EXISTS idx_interactions_user_id ON interactions(user_id);

-- Create FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS catalog_entries_fts USING fts5(
    title,
    description,
    location,
    content='catalog_entries',
    content_rowid='rowid'
);

-- Create triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS catalog_entries_ai AFTER INSERT ON catalog_entries BEGIN
    INSERT INTO catalog_entries_fts(rowid, title, description, location)
    VALUES (NEW.rowid, NEW.title, NEW.description, NEW.location);
END;

CREATE TRIGGER IF NOT EXISTS catalog_entries_ad AFTER DELETE ON catalog_entries BEGIN
    INSERT INTO catalog_entries_fts(catalog_entries_fts, rowid, title, description, location)
    VALUES ('delete', OLD.rowid, OLD.title, OLD.description, OLD.location);
END;

CREATE TRIGGER IF NOT EXISTS catalog_entries_au AFTER UPDATE ON catalog_entries WHEN 
    OLD.title != NEW.title OR 
    OLD.description != NEW.description OR 
    OLD.location != NEW.location
BEGIN
    INSERT INTO catalog_entries_fts(catalog_entries_fts, rowid, title, description, location)
    VALUES ('delete', OLD.rowid, OLD.title, OLD.description, OLD.location);
    INSERT INTO catalog_entries_fts(rowid, title, description, location)
    VALUES (NEW.rowid, NEW.title, NEW.description, NEW.location);
END;

-- +migrate Down
DROP TRIGGER IF EXISTS catalog_entries_ai;
DROP TRIGGER IF EXISTS catalog_entries_ad;
DROP TRIGGER IF EXISTS catalog_entries_au;
DROP TABLE IF EXISTS catalog_entries_fts;
DROP TABLE IF EXISTS interactions;
DROP TABLE IF EXISTS entry_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS catalog_entries;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
