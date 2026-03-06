-- +migrate Up
CREATE TABLE IF NOT EXISTS archive_revisions (
    id TEXT PRIMARY KEY,
    entry_id TEXT NOT NULL,
    revision_no INTEGER NOT NULL,
    engine TEXT NOT NULL CHECK (engine IN ('playwright', 'http')),
    root_path TEXT NOT NULL,
    index_path TEXT NOT NULL,
    manifest_path TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('success', 'partial', 'failed', 'blocked')),
    failure_reason TEXT,
    captured_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (entry_id) REFERENCES catalog_entries(id) ON DELETE CASCADE,
    UNIQUE(entry_id, revision_no)
);

CREATE INDEX IF NOT EXISTS idx_archive_revisions_entry_id ON archive_revisions(entry_id);
CREATE INDEX IF NOT EXISTS idx_archive_revisions_status ON archive_revisions(status);
CREATE INDEX IF NOT EXISTS idx_archive_revisions_captured_at ON archive_revisions(captured_at);

CREATE TABLE IF NOT EXISTS archive_assets (
    id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    source_url TEXT NOT NULL,
    local_path TEXT NOT NULL,
    content_hash TEXT,
    mime_type TEXT,
    bytes INTEGER NOT NULL DEFAULT 0,
    kind TEXT NOT NULL CHECK (kind IN ('css', 'js', 'img', 'font', 'media', 'other')),
    download_status TEXT NOT NULL CHECK (download_status IN ('ok', 'missing', 'skipped', 'error')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (revision_id) REFERENCES archive_revisions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_archive_assets_revision_id ON archive_assets(revision_id);
CREATE INDEX IF NOT EXISTS idx_archive_assets_content_hash ON archive_assets(content_hash);
CREATE INDEX IF NOT EXISTS idx_archive_assets_kind ON archive_assets(kind);

CREATE TABLE IF NOT EXISTS thumbnail_candidates (
    id TEXT PRIMARY KEY,
    entry_id TEXT NOT NULL,
    revision_id TEXT,
    source_type TEXT NOT NULL CHECK (source_type IN ('local_asset', 'screenshot', 'remote_meta', 'upload')),
    path TEXT NOT NULL,
    score REAL NOT NULL DEFAULT 0,
    selected BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (entry_id) REFERENCES catalog_entries(id) ON DELETE CASCADE,
    FOREIGN KEY (revision_id) REFERENCES archive_revisions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_thumbnail_candidates_entry_id ON thumbnail_candidates(entry_id);
CREATE INDEX IF NOT EXISTS idx_thumbnail_candidates_revision_id ON thumbnail_candidates(revision_id);
CREATE INDEX IF NOT EXISTS idx_thumbnail_candidates_selected ON thumbnail_candidates(selected);

ALTER TABLE catalog_entries ADD COLUMN archive_current_revision_id TEXT;
ALTER TABLE catalog_entries ADD COLUMN archive_fidelity TEXT NOT NULL DEFAULT 'low' CHECK (archive_fidelity IN ('high', 'partial', 'low'));
ALTER TABLE catalog_entries ADD COLUMN thumbnail_source TEXT NOT NULL DEFAULT 'auto' CHECK (thumbnail_source IN ('auto', 'user_selected', 'upload'));

CREATE INDEX IF NOT EXISTS idx_catalog_entries_archive_current_revision_id ON catalog_entries(archive_current_revision_id);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_archive_fidelity ON catalog_entries(archive_fidelity);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_thumbnail_source ON catalog_entries(thumbnail_source);

-- +migrate Down
DROP INDEX IF EXISTS idx_catalog_entries_thumbnail_source;
DROP INDEX IF EXISTS idx_catalog_entries_archive_fidelity;
DROP INDEX IF EXISTS idx_catalog_entries_archive_current_revision_id;

-- SQLite does not support DROP COLUMN directly in all supported versions.
-- We keep additive columns in place on rollback for compatibility.

DROP INDEX IF EXISTS idx_thumbnail_candidates_selected;
DROP INDEX IF EXISTS idx_thumbnail_candidates_revision_id;
DROP INDEX IF EXISTS idx_thumbnail_candidates_entry_id;
DROP TABLE IF EXISTS thumbnail_candidates;

DROP INDEX IF EXISTS idx_archive_assets_kind;
DROP INDEX IF EXISTS idx_archive_assets_content_hash;
DROP INDEX IF EXISTS idx_archive_assets_revision_id;
DROP TABLE IF EXISTS archive_assets;

DROP INDEX IF EXISTS idx_archive_revisions_captured_at;
DROP INDEX IF EXISTS idx_archive_revisions_status;
DROP INDEX IF EXISTS idx_archive_revisions_entry_id;
DROP TABLE IF EXISTS archive_revisions;
