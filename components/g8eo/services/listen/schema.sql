-- g8es SQLite Schema
-- Canonical schema for the g8e coordination store (g8e.operator --listen mode).
-- Embedded into `listen_db.go` via `//go:embed schema.sql` and applied on
-- database open via `ListenDBService.initSchema`. This file is the SINGLE
-- source of truth for the g8es schema — do not duplicate it elsewhere.
--
-- All domain data (users, sessions, operators, cases, etc.) is stored as JSON
-- documents in the documents table. g8ed and g8ee interact with this store
-- exclusively via the g8ed HTTP API — neither component holds a local SQLite
-- database.

-- Document store: unified collection/id based storage
CREATE TABLE IF NOT EXISTS documents (
    collection TEXT NOT NULL,
    id TEXT NOT NULL,
    data JSON NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (collection, id)
);
CREATE INDEX IF NOT EXISTS idx_documents_collection ON documents(collection);
CREATE INDEX IF NOT EXISTS idx_documents_updated ON documents(collection, updated_at);

-- KV store with TTL
CREATE TABLE IF NOT EXISTS kv_store (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_kv_expires ON kv_store(expires_at);

-- SSE event buffer: per-session ring buffer for reconnection replay
CREATE TABLE IF NOT EXISTS sse_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operator_session_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sse_session ON sse_events(operator_session_id, id);
CREATE INDEX IF NOT EXISTS idx_sse_created ON sse_events(created_at);

-- Blob store: raw binary attachments keyed by namespace + id
CREATE TABLE IF NOT EXISTS blobs (
    id           TEXT NOT NULL,
    namespace    TEXT NOT NULL,
    size         INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    data         BLOB NOT NULL,
    created_at   TEXT NOT NULL,
    expires_at   TEXT,
    PRIMARY KEY (namespace, id)
);
CREATE INDEX IF NOT EXISTS idx_blobs_namespace ON blobs(namespace);
CREATE INDEX IF NOT EXISTS idx_blobs_expires   ON blobs(expires_at);
