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

package gateway

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// KVStoreService provides key/value storage with optional TTL expiration.
type KVStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewKVStoreService creates a new KV store service.
func NewKVStoreService(db *sqliteutil.DB, logger *slog.Logger) *KVStoreService {
	return &KVStoreService{
		db:     db,
		logger: logger,
	}
}

// KVGet retrieves a value by key. Returns ("", false) if not found or expired.
func (s *KVStoreService) KVGet(key string) (string, bool) {
	// Use a single query that filters out expired keys, avoiding the need
	// for a separate lazy-delete goroutine (which risked deadlocks).
	// Expired entries are cleaned up by RunMaintenance instead.
	var value string
	err := s.db.QueryRowWithRetry(
		"SELECT value FROM kv_store WHERE key = ? AND (expires_at IS NULL OR expires_at > ?)",
		key, sqliteutil.NowTimestamp(),
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// KVSet stores a key/value pair. ttlSeconds <= 0 means no expiration.
func (s *KVStoreService) KVSet(key, value string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, value, now, expiresAt,
	)
	return err
}

// KVSetObserved stores a key/value pair as observed-state (state_tier = 'observed').
// Observed-state entries are excluded from the bound freshness root and are
// hashed separately in the observed-state commitment. ttlSeconds <= 0 means no expiration.
func (s *KVStoreService) KVSetObserved(key, value string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO kv_store (key, value, created_at, expires_at, state_tier)
		 VALUES (?, ?, ?, ?, 'observed')
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at, state_tier = 'observed'`,
		key, value, now, expiresAt,
	)
	return err
}

// KVDelete removes a key.
func (s *KVStoreService) KVDelete(key string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key = ?", key)
	return err
}

// KVDeletePattern removes all keys matching a glob pattern (uses SQL GLOB).
func (s *KVStoreService) KVDeletePattern(pattern string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// KVKeys returns all keys matching a glob pattern.
func (s *KVStoreService) KVKeys(pattern string) ([]string, error) {
	keys, err := sqliteutil.MaterializeRows(s.db,
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?)",
		[]interface{}{pattern, sqliteutil.NowTimestamp()},
		func(r *sql.Rows) (string, error) {
			var k string
			if err := r.Scan(&k); err != nil {
				return "", err
			}
			return k, nil
		})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// KVScan returns keys matching a glob pattern using cursor-based pagination.
// cursor is a row offset (0 = start). count is the page size (default 100).
// Returns (nextCursor, keys, error). nextCursor == 0 means scan is complete.
func (s *KVStoreService) KVScan(pattern string, cursor, count int) (int, []string, error) {
	if count <= 0 {
		count = 100
	}
	// Fetch count+1 to detect whether a next page exists
	keys, err := sqliteutil.MaterializeRows(s.db,
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?) ORDER BY key LIMIT ? OFFSET ?",
		[]interface{}{pattern, sqliteutil.NowTimestamp(), count + 1, cursor},
		func(r *sql.Rows) (string, error) {
			var k string
			if err := r.Scan(&k); err != nil {
				return "", err
			}
			return k, nil
		})
	if err != nil {
		return 0, nil, err
	}

	if len(keys) > count {
		return cursor + count, keys[:count], nil
	}
	return 0, keys, nil
}

// KVExists checks if a key exists and is not expired.
func (s *KVStoreService) KVExists(key string) bool {
	_, found := s.KVGet(key)
	return found
}

// KVTTL returns the remaining TTL in seconds for a key. -1 if no expiry, -2 if not found.
func (s *KVStoreService) KVTTL(key string) int {
	var expiresAt sql.NullString
	err := s.db.QueryRowWithRetry(
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
func (s *KVStoreService) KVExpire(key string, ttlSeconds int) bool {
	exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
	result, err := s.db.ExecWithRetry(
		"UPDATE kv_store SET expires_at = ? WHERE key = ?", exp, key,
	)
	if err != nil {
		return false
	}
	n, _ := result.RowsAffected()
	return n > 0
}

// RunMaintenance removes expired KV entries from the database.
func (s *KVStoreService) RunMaintenance() error {
	now := sqliteutil.NowTimestamp()
	_, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired kv entries: %w", err)
	}
	return nil
}
