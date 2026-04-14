// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package listen

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
)

// ListenDBService provides the unified SQLite persistence layer for listen mode.
// Three subsystems:
//   - Document store: collection/id based CRUD (replaces g8ed+g8ee separate SQLite DBs)
//   - KV store with TTL: key/value with optional expiration
//   - SSE event buffer: per-session event ring buffer
type ListenDBService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewListenDBService opens (or creates) the unified SQLite database.
func NewListenDBService(dataDir string, sslDir string, logger *slog.Logger) (*ListenDBService, error) {
	dbPath := filepath.Join(dataDir, "g8e.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)

	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open listen database: %w", err)
	}

	svc := &ListenDBService{db: db, logger: logger}

	if err := svc.initSchema(sslDir); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	logger.Info("Listen database initialized", "path", dbPath)
	return svc, nil
}

func (s *ListenDBService) initSchema(sslDir string) error {
	_, err := s.db.Exec(listenSchema)
	if err != nil {
		return err
	}
	sm := NewSecretManager(s.db, sslDir, s.logger)
	return sm.InitPlatformSettings()
}

// Close closes the database connection.
func (s *ListenDBService) Close() error {
	return s.db.Close()
}

// =============================================================================
// Document Store — collection/id based CRUD
// =============================================================================

// DocGet retrieves a document by collection and id.
// Returns a typed Document with native time.Time timestamps, or nil if not found.
func (s *ListenDBService) DocGet(collection, id string) (*models.Document, error) {
	var dataJSON string
	var createdAtStr, updatedAtStr string
	err := s.db.QueryRow(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&dataJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr)
}

// DocSet creates or replaces a document. data must be valid JSON.
// Timestamps are managed by the service — created_at is set once on insert and
// never overwritten. updated_at is refreshed on every upsert.
func (s *ListenDBService) DocSet(collection, id string, data json.RawMessage) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("failed to unmarshal document: %w", err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	_, err = s.db.Exec(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(collection, id) DO UPDATE SET
		   data = excluded.data,
		   updated_at = excluded.updated_at`,
		collection, id, string(dataJSON), nowStr, nowStr,
	)
	return err
}

// DocUpdate merges fields into an existing document. fields must be valid JSON.
// Returns the updated Document with native time.Time timestamps.
func (s *ListenDBService) DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error) {
	var existingJSON string
	var createdAtStr, updatedAtStr string
	err := s.db.QueryRow(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&existingJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found: %s/%s", collection, id)
	}
	if err != nil {
		return nil, err
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(existingJSON), &doc); err != nil {
		return nil, err
	}

	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(fields, &incoming); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fields: %w", err)
	}

	for k, v := range incoming {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		if string(v) == "null" {
			delete(doc, k)
		} else {
			doc[k] = v
		}
	}

	dataJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	_, err = s.db.Exec(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = ? AND id = ?",
		string(dataJSON), nowStr, collection, id,
	)
	if err != nil {
		return nil, err
	}

	return scanDocument(collection, id, string(dataJSON), createdAtStr, nowStr)
}

// DocDelete removes a document. Returns (true, nil) if deleted, (false, nil) if not found.
func (s *ListenDBService) DocDelete(collection, id string) (bool, error) {
	result, err := s.db.Exec(
		"DELETE FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DocQuery returns documents matching field conditions.
// Supported ops: ==, !=, <, >, <=, >=. orderBy is "field" or "field DESC". limit 0 means no limit.
func (s *ListenDBService) DocQuery(collection string, filters []models.DocFilter, orderBy string, limit int) ([]*models.Document, error) {
	query := "SELECT id, data, created_at, updated_at FROM documents WHERE collection = ?"
	args := []interface{}{collection}

	for _, f := range filters {
		if f.Field == "" || f.Op == "" {
			continue
		}

		switch f.Op {
		case "==", "!=", "<", ">", "<=", ">=":
		default:
			continue
		}

		if err := sqliteutil.ValidateIdentifier(f.Field); err != nil {
			return nil, fmt.Errorf("invalid filter field: %w", err)
		}

		sqlOp := f.Op
		if sqlOp == "==" {
			sqlOp = "="
		}
		query += fmt.Sprintf(" AND json_extract(data, '$.%s') %s ?", f.Field, sqlOp)
		var nativeVal interface{}
		if err := json.Unmarshal(f.Value, &nativeVal); err != nil {
			return nil, fmt.Errorf("invalid filter value: %w", err)
		}
		args = append(args, nativeVal)
	}

	if orderBy != "" {
		parts := strings.Fields(orderBy)
		orderField := parts[0]
		dir := "ASC"
		if len(parts) > 1 && strings.ToUpper(parts[1]) == "DESC" {
			dir = "DESC"
		}

		if err := sqliteutil.ValidateIdentifier(orderField); err != nil {
			return nil, fmt.Errorf("invalid orderBy field: %w", err)
		}

		query += fmt.Sprintf(" ORDER BY json_extract(data, '$.%s') %s", orderField, dir)
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Document
	for rows.Next() {
		var docID, dataJSON, createdAtStr, updatedAtStr string
		if err := rows.Scan(&docID, &dataJSON, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		doc, err := scanDocument(collection, docID, dataJSON, createdAtStr, updatedAtStr)
		if err != nil {
			return nil, err
		}
		results = append(results, doc)
	}
	return results, rows.Err()
}

// scanDocument parses a raw SQLite row into a typed Document.
// This is the single point where TEXT timestamps are converted to time.Time.
func scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr string) (*models.Document, error) {
	createdAt, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid created_at for document %s/%s: %w", collection, id, err)
	}
	updatedAt, err := sqliteutil.ParseTimestamp(updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid updated_at for document %s/%s: %w", collection, id, err)
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, fmt.Errorf("invalid data JSON for document %s/%s: %w", collection, id, err)
	}
	if data == nil {
		data = make(map[string]json.RawMessage)
	}

	return &models.Document{
		ID:         id,
		Collection: collection,
		Data:       data,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// =============================================================================
// KV Store with TTL
// =============================================================================

// KVGet retrieves a value by key. Returns ("", false) if not found or expired.
func (s *ListenDBService) KVGet(key string) (string, bool) {
	// Use a single query that filters out expired keys, avoiding the need
	// for a separate lazy-delete goroutine (which risked deadlocks).
	// Expired entries are cleaned up by RunTTLCleanup instead.
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM kv_store WHERE key = ? AND (expires_at IS NULL OR expires_at > ?)",
		key, sqliteutil.NowTimestamp(),
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// KVSet stores a key/value pair. ttlSeconds <= 0 means no expiration.
func (s *ListenDBService) KVSet(key, value string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.Exec(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, value, now, expiresAt,
	)
	return err
}

// KVDelete removes a key.
func (s *ListenDBService) KVDelete(key string) error {
	_, err := s.db.Exec("DELETE FROM kv_store WHERE key = ?", key)
	return err
}

// KVDeletePattern removes all keys matching a glob pattern (uses SQL GLOB).
func (s *ListenDBService) KVDeletePattern(pattern string) (int64, error) {
	result, err := s.db.Exec("DELETE FROM kv_store WHERE key GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// KVKeys returns all keys matching a glob pattern.
func (s *ListenDBService) KVKeys(pattern string) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?)",
		pattern, sqliteutil.NowTimestamp(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// KVScan returns keys matching a glob pattern using cursor-based pagination.
// cursor is a row offset (0 = start). count is the page size (default 100).
// Returns (nextCursor, keys, error). nextCursor == 0 means scan is complete.
func (s *ListenDBService) KVScan(pattern string, cursor, count int) (int, []string, error) {
	if count <= 0 {
		count = 100
	}
	// Fetch count+1 to detect whether a next page exists
	rows, err := s.db.Query(
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?) ORDER BY key LIMIT ? OFFSET ?",
		pattern, sqliteutil.NowTimestamp(), count+1, cursor,
	)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return 0, nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	if len(keys) > count {
		return cursor + count, keys[:count], nil
	}
	return 0, keys, nil
}

// KVExists checks if a key exists and is not expired.
func (s *ListenDBService) KVExists(key string) bool {
	_, found := s.KVGet(key)
	return found
}

// KVTTL returns the remaining TTL in seconds for a key. -1 if no expiry, -2 if not found.
func (s *ListenDBService) KVTTL(key string) int {
	var expiresAt sql.NullString
	err := s.db.QueryRow(
		"SELECT expires_at FROM kv_store WHERE key = ?", key,
	).Scan(&expiresAt)
	if err != nil {
		return -2
	}
	if !expiresAt.Valid {
		return -1
	}
	exp, err := time.Parse(time.RFC3339Nano, expiresAt.String)
	if err != nil {
		return -2
	}
	remaining := int(time.Until(exp).Seconds())
	if remaining < 0 {
		return -2
	}
	return remaining
}

// KVExpire sets a TTL on an existing key. Returns false if key not found.
func (s *ListenDBService) KVExpire(key string, ttlSeconds int) bool {
	exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
	result, err := s.db.Exec(
		"UPDATE kv_store SET expires_at = ? WHERE key = ?", exp, key,
	)
	if err != nil {
		return false
	}
	n, _ := result.RowsAffected()
	return n > 0
}

// SSEEventsAppend inserts a row into the sse_events table.
func (s *ListenDBService) SSEEventsAppend(operatorSessionID, eventType, payload string) error {
	now := sqliteutil.NowTimestamp()
	_, err := s.db.Exec(
		"INSERT INTO sse_events (operator_session_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)",
		operatorSessionID, eventType, payload, now,
	)
	return err
}

// SSEEventsWipe deletes all rows from the sse_events table. Returns the number of rows deleted.
func (s *ListenDBService) SSEEventsWipe() (int64, error) {
	result, err := s.db.Exec("DELETE FROM sse_events")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SSEEventsCount returns the total number of rows in the sse_events table.
func (s *ListenDBService) SSEEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM sse_events").Scan(&count)
	return count, err
}

// RunTTLCleanup periodically removes expired KV entries and expired blobs.
func (s *ListenDBService) RunTTLCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := sqliteutil.NowTimestamp()
			s.db.Exec("DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?", now)
			s.db.Exec("DELETE FROM blobs WHERE expires_at IS NOT NULL AND expires_at < ?", now)
		}
	}
}

// =============================================================================
// Blob Store — raw binary storage keyed by namespace + id
// =============================================================================

// BlobRecord is the metadata returned for a stored blob (data excluded).
type BlobRecord struct {
	ID          string
	Namespace   string
	Size        int64
	ContentType string
	CreatedAt   time.Time
}

// BlobPut stores raw bytes under namespace/id. ttlSeconds == 0 means no expiration.
// Negative ttlSeconds means the blob is immediately expired (will never be returned by BlobGet).
// An existing blob at the same namespace/id is replaced.
func (s *ListenDBService) BlobPut(namespace, id string, data []byte, contentType string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.Exec(
		`INSERT INTO blobs (namespace, id, size, content_type, data, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(namespace, id) DO UPDATE SET
		   size         = excluded.size,
		   content_type = excluded.content_type,
		   data         = excluded.data,
		   expires_at   = excluded.expires_at`,
		namespace, id, int64(len(data)), contentType, data, now, expiresAt,
	)
	return err
}

// BlobGet retrieves the raw bytes and content type for a blob.
// Returns (nil, "", false) if not found or expired.
func (s *ListenDBService) BlobGet(namespace, id string) ([]byte, string, bool) {
	var data []byte
	var contentType string
	err := s.db.QueryRow(
		"SELECT data, content_type FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, sqliteutil.NowTimestamp(),
	).Scan(&data, &contentType)
	if err != nil {
		return nil, "", false
	}
	return data, contentType, true
}

// BlobMeta retrieves metadata for a blob without loading the data.
// Returns (nil, false) if not found or expired.
func (s *ListenDBService) BlobMeta(namespace, id string) (*BlobRecord, bool) {
	var rec BlobRecord
	var createdAtStr string
	err := s.db.QueryRow(
		"SELECT id, namespace, size, content_type, created_at FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, sqliteutil.NowTimestamp(),
	).Scan(&rec.ID, &rec.Namespace, &rec.Size, &rec.ContentType, &createdAtStr)
	if err != nil {
		return nil, false
	}
	t, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, false
	}
	rec.CreatedAt = t
	return &rec, true
}

// BlobDelete removes a single blob. Returns (true, nil) if deleted, (false, nil) if not found.
func (s *ListenDBService) BlobDelete(namespace, id string) (bool, error) {
	result, err := s.db.Exec(
		"DELETE FROM blobs WHERE namespace = ? AND id = ?",
		namespace, id,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// BlobDeleteNamespace removes all blobs under a namespace.
// Returns the count of deleted blobs.
func (s *ListenDBService) BlobDeleteNamespace(namespace string) (int64, error) {
	result, err := s.db.Exec("DELETE FROM blobs WHERE namespace = ?", namespace)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// =============================================================================
// Schema
// =============================================================================

const listenSchema = `
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

`
