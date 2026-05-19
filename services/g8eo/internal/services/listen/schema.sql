-- operator SQLite Schema
-- Canonical schema for the g8e coordination store (g8e.operator --listen mode).
-- Embedded into `listen_db.go` via `//go:embed schema.sql` and applied on
-- database open via `ListenDBService.initSchema`. This file is the SINGLE
-- source of truth for the operator schema - do not duplicate it elsewhere.
--
-- All domain data (users, sessions, operators, cases, etc.) is stored as JSON
-- documents in the documents table. client and g8ee interact with this store
-- exclusively via the client HTTP API - neither component holds a local SQLite
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

-- SSE event buffer: per-routing-target ring buffer for reconnection replay.
-- Each row carries exactly one of three first-class routing keys, expressed as
-- distinct columns so the substrate never has to talk about a bare session id:
--   * web_session_id - browser UI session (mTLS cookie session)
--   * cli_session_id - BYO/CLI/scripted client session (mTLS cert session)
--   * user_id        - background fanout to every session a user owns
-- web and cli are routed identically by the substrate but MUST never be
-- conflated under a single shared id namespace.
CREATE TABLE IF NOT EXISTS sse_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    web_session_id TEXT,
    cli_session_id TEXT,
    user_id TEXT,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TEXT NOT NULL,
    CHECK (
        (CASE WHEN web_session_id IS NULL THEN 0 ELSE 1 END)
      + (CASE WHEN cli_session_id IS NULL THEN 0 ELSE 1 END)
      + (CASE WHEN user_id        IS NULL THEN 0 ELSE 1 END)
      = 1
    )
);
CREATE INDEX IF NOT EXISTS idx_sse_web ON sse_events(web_session_id, id) WHERE web_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_cli ON sse_events(cli_session_id, id) WHERE cli_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_user ON sse_events(user_id, id) WHERE user_id IS NOT NULL;
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

-- State Merkle Root: single row containing the current platform state root
CREATE TABLE IF NOT EXISTS state_root (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    root TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Nonces: used for transaction replay protection
CREATE TABLE IF NOT EXISTS nonces (
    nonce TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nonces_expires ON nonces(expires_at);


