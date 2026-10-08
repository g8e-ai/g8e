-- Operator SQLite Schema
-- Canonical schema for the g8e coordination store (g8e.operator --doctrine/--consensus/--notary mode).
-- Embedded into `gateway_db.go` via `//go:embed schema.sql` and applied on
-- database open via `CanonicalDBService.initSchema`. This file is the SINGLE
-- source of truth for the Operator schema - do not duplicate it elsewhere.
--
-- All domain data (users, sessions, operators, cases, etc.) is stored as JSON
-- documents in the documents table. g8e-compatible agentic ensembles and clients
-- interact with this store exclusively via the client HTTP API - neither component
-- holds a local SQLite database.

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

-- Enrollment hot paths use literal JSON expressions (not parameterized paths).
-- Partial indexes exclude unrelated documents and historical payloads are never
-- decoded to count capacity or discover pending requests.
CREATE INDEX IF NOT EXISTS idx_enrollment_token ON documents(json_extract(data, '$.token_hash'))
    WHERE collection = 'platform_enrollments';
CREATE INDEX IF NOT EXISTS idx_enrollment_identity ON documents(
    json_extract(data, '$.component_kind'), json_extract(data, '$.instance_id'),
    json_extract(data, '$.state'), julianday(json_extract(data, '$.expires_at')))
    WHERE collection = 'platform_enrollments';
CREATE INDEX IF NOT EXISTS idx_enrollment_capacity ON documents(
    json_extract(data, '$.component_kind'), json_extract(data, '$.state'),
    julianday(json_extract(data, '$.expires_at')))
    WHERE collection = 'platform_enrollments';
CREATE INDEX IF NOT EXISTS idx_enrollment_pending ON documents(
    json_extract(data, '$.state'), julianday(json_extract(data, '$.expires_at')))
    WHERE collection = 'platform_enrollments';

CREATE INDEX IF NOT EXISTS idx_enrollment_lease ON documents(
    json_extract(data, '$.state'), julianday(json_extract(data, '$.issuance_lease_expires_at')))
    WHERE collection = 'platform_enrollments';
CREATE INDEX IF NOT EXISTS idx_enrollment_retention ON documents(
    json_extract(data, '$.state'), julianday(json_extract(data, '$.last_transition_at')))
    WHERE collection = 'platform_enrollments';

-- KV store with TTL
-- Must be defined before document triggers that reference it.
CREATE TABLE IF NOT EXISTS kv_store (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    state_tier TEXT NOT NULL DEFAULT 'bound'
);
CREATE INDEX IF NOT EXISTS idx_kv_expires ON kv_store(expires_at);
CREATE INDEX IF NOT EXISTS idx_kv_tier ON kv_store(state_tier);

-- Trigger to invalidate KV cache when a document is inserted
CREATE TRIGGER IF NOT EXISTS trg_documents_insert_kv
AFTER INSERT ON documents
BEGIN
    DELETE FROM kv_store WHERE key = 'g8e:cache:doc:' || NEW.collection || ':' || NEW.id;
    DELETE FROM kv_store WHERE key GLOB 'g8e:cache:query:' || NEW.collection || ':*';
END;

-- Trigger to invalidate KV cache when a document is updated (only when data changes)
CREATE TRIGGER IF NOT EXISTS trg_documents_update_kv
AFTER UPDATE ON documents
WHEN OLD.data IS NOT NEW.data
BEGIN
    DELETE FROM kv_store WHERE key = 'g8e:cache:doc:' || NEW.collection || ':' || NEW.id;
    DELETE FROM kv_store WHERE key GLOB 'g8e:cache:query:' || NEW.collection || ':*';
END;

-- Trigger to invalidate KV cache when a document is deleted
CREATE TRIGGER IF NOT EXISTS trg_documents_delete_kv
AFTER DELETE ON documents
BEGIN
    DELETE FROM kv_store WHERE key = 'g8e:cache:doc:' || OLD.collection || ':' || OLD.id;
    DELETE FROM kv_store WHERE key GLOB 'g8e:cache:query:' || OLD.collection || ':*';
END;

-- SSE event buffer: per-routing-target ring buffer for reconnection replay.
-- Every row carries user_id (ownership/identity, always NOT NULL) plus exactly
-- one session column (delivery/routing):
--   * web_session_id - browser UI session (mTLS cookie session)
--   * cli_session_id - BYO/CLI/scripted client session (mTLS cert session)
-- user_id alone is not a valid route — it is always paired with a session.
CREATE TABLE IF NOT EXISTS sse_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    web_session_id TEXT,
    cli_session_id TEXT,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    producer_id TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (CASE WHEN web_session_id IS NULL THEN 0 ELSE 1 END)
      + (CASE WHEN cli_session_id IS NULL THEN 0 ELSE 1 END)
      = 1
    )
);
CREATE INDEX IF NOT EXISTS idx_sse_web ON sse_events(web_session_id, id) WHERE web_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_cli ON sse_events(cli_session_id, id) WHERE cli_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_user ON sse_events(user_id, id);
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
    state_tier   TEXT NOT NULL DEFAULT 'bound',
    PRIMARY KEY (namespace, id)
);
CREATE INDEX IF NOT EXISTS idx_blobs_namespace ON blobs(namespace);
CREATE INDEX IF NOT EXISTS idx_blobs_expires   ON blobs(expires_at);
CREATE INDEX IF NOT EXISTS idx_blobs_tier      ON blobs(state_tier);

-- The full-scan state root (algorithm 1) kept a global state_version counter and
-- a persisted root row. Both are replaced by the incremental commitment below.
DROP TRIGGER IF EXISTS trg_documents_insert_version;
DROP TRIGGER IF EXISTS trg_documents_update_version;
DROP TRIGGER IF EXISTS trg_documents_delete_version;
DROP TRIGGER IF EXISTS trg_kv_store_insert_version;
DROP TRIGGER IF EXISTS trg_kv_store_update_version;
DROP TRIGGER IF EXISTS trg_kv_store_delete_version;
DROP TRIGGER IF EXISTS trg_blobs_insert_version;
DROP TRIGGER IF EXISTS trg_blobs_update_version;
DROP TRIGGER IF EXISTS trg_blobs_delete_version;
DROP TABLE IF EXISTS state_version;
DROP TABLE IF EXISTS state_root;

-- Incremental state Merkle commitment, owned by StateRootService.
-- Triggers record each changed committed row in the writer's own transaction;
-- a root read rehashes only those leaves and their bucket ancestor paths.
-- Cache keys (g8e:cache:*), nonces and SSE events are not committed.
CREATE TABLE IF NOT EXISTS state_commitment_dirty (
    source TEXT NOT NULL,
    k1     TEXT NOT NULL,
    k2     TEXT NOT NULL,
    PRIMARY KEY (source, k1, k2)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS state_leaves (
    leaf_id BLOB PRIMARY KEY,
    tier    TEXT NOT NULL,
    bucket  INTEGER NOT NULL,
    digest  BLOB NOT NULL
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_state_leaves_bucket ON state_leaves(tier, bucket, leaf_id);

-- Only non-empty nodes are stored; level 0 is the root, level 4 the buckets.
CREATE TABLE IF NOT EXISTS state_nodes (
    tier   TEXT NOT NULL,
    level  INTEGER NOT NULL,
    idx    INTEGER NOT NULL,
    digest BLOB NOT NULL,
    PRIMARY KEY (tier, level, idx)
) WITHOUT ROWID;

-- Algorithm of the persisted tree; a mismatch triggers one rebuild at open.
CREATE TABLE IF NOT EXISTS state_commitment (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    algorithm INTEGER NOT NULL
);

-- Dirty marks use NOT EXISTS rather than INSERT OR IGNORE: an outer statement's
-- conflict clause (for example an UPSERT) overrides OR IGNORE inside a trigger.
-- Triggers are replaced on open so existing databases receive rule changes.
DROP TRIGGER IF EXISTS trg_documents_insert_commitment;
CREATE TRIGGER trg_documents_insert_commitment
AFTER INSERT ON documents
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'documents', NEW.collection, NEW.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'documents' AND k1 = NEW.collection AND k2 = NEW.id);
END;

-- Metadata-only updates (updated_at) do not change the commitment.
DROP TRIGGER IF EXISTS trg_documents_update_commitment;
CREATE TRIGGER trg_documents_update_commitment
AFTER UPDATE ON documents
WHEN OLD.data IS NOT NEW.data OR OLD.collection IS NOT NEW.collection OR OLD.id IS NOT NEW.id
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'documents', OLD.collection, OLD.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'documents' AND k1 = OLD.collection AND k2 = OLD.id);
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'documents', NEW.collection, NEW.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'documents' AND k1 = NEW.collection AND k2 = NEW.id);
END;

DROP TRIGGER IF EXISTS trg_documents_delete_commitment;
CREATE TRIGGER trg_documents_delete_commitment
AFTER DELETE ON documents
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'documents', OLD.collection, OLD.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'documents' AND k1 = OLD.collection AND k2 = OLD.id);
END;

DROP TRIGGER IF EXISTS trg_kv_store_insert_commitment;
CREATE TRIGGER trg_kv_store_insert_commitment
AFTER INSERT ON kv_store
WHEN NEW.key NOT LIKE 'g8e:cache:%'
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'kv_store', NEW.key, ''
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'kv_store' AND k1 = NEW.key AND k2 = '');
END;

DROP TRIGGER IF EXISTS trg_kv_store_update_commitment;
CREATE TRIGGER trg_kv_store_update_commitment
AFTER UPDATE ON kv_store
WHEN OLD.key IS NOT NEW.key OR OLD.value IS NOT NEW.value
  OR OLD.expires_at IS NOT NEW.expires_at OR OLD.state_tier IS NOT NEW.state_tier
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'kv_store', OLD.key, ''
        WHERE OLD.key NOT LIKE 'g8e:cache:%'
          AND NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'kv_store' AND k1 = OLD.key AND k2 = '');
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'kv_store', NEW.key, ''
        WHERE NEW.key NOT LIKE 'g8e:cache:%'
          AND NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'kv_store' AND k1 = NEW.key AND k2 = '');
END;

DROP TRIGGER IF EXISTS trg_kv_store_delete_commitment;
CREATE TRIGGER trg_kv_store_delete_commitment
AFTER DELETE ON kv_store
WHEN OLD.key NOT LIKE 'g8e:cache:%'
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'kv_store', OLD.key, ''
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'kv_store' AND k1 = OLD.key AND k2 = '');
END;

DROP TRIGGER IF EXISTS trg_blobs_insert_commitment;
CREATE TRIGGER trg_blobs_insert_commitment
AFTER INSERT ON blobs
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'blobs', NEW.namespace, NEW.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'blobs' AND k1 = NEW.namespace AND k2 = NEW.id);
END;

DROP TRIGGER IF EXISTS trg_blobs_update_commitment;
CREATE TRIGGER trg_blobs_update_commitment
AFTER UPDATE ON blobs
WHEN OLD.namespace IS NOT NEW.namespace OR OLD.id IS NOT NEW.id OR OLD.size IS NOT NEW.size
  OR OLD.content_type IS NOT NEW.content_type OR OLD.data IS NOT NEW.data
  OR OLD.expires_at IS NOT NEW.expires_at OR OLD.state_tier IS NOT NEW.state_tier
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'blobs', OLD.namespace, OLD.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'blobs' AND k1 = OLD.namespace AND k2 = OLD.id);
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'blobs', NEW.namespace, NEW.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'blobs' AND k1 = NEW.namespace AND k2 = NEW.id);
END;

DROP TRIGGER IF EXISTS trg_blobs_delete_commitment;
CREATE TRIGGER trg_blobs_delete_commitment
AFTER DELETE ON blobs
BEGIN
    INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'blobs', OLD.namespace, OLD.id
        WHERE NOT EXISTS (SELECT 1 FROM state_commitment_dirty WHERE source = 'blobs' AND k1 = OLD.namespace AND k2 = OLD.id);
END;

-- Nonces: used for transaction replay protection
CREATE TABLE IF NOT EXISTS nonces (
    nonce TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'reserved'
);
CREATE INDEX IF NOT EXISTS idx_nonces_expires ON nonces(expires_at);

CREATE TABLE IF NOT EXISTS suspended_transactions (
    transaction_hash TEXT PRIMARY KEY,
    envelope TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_arguments TEXT,
    user_id TEXT,
    operator_id TEXT,
    approved INTEGER DEFAULT 0 CHECK (approved IN (0, 1)),
    approved_at TEXT,
    approved_by TEXT,
    approval_signature TEXT,
    expected_cert_fingerprint TEXT
);
CREATE INDEX IF NOT EXISTS idx_suspended_expires ON suspended_transactions(expires_at);
CREATE INDEX IF NOT EXISTS idx_suspended_pending ON suspended_transactions(approved, created_at);
