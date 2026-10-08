// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
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
func (s *KVStoreService) KVGet(ctx context.Context, key string) (string, bool) {
	// Use a single query that filters out expired keys, avoiding the need
	// for a separate lazy-delete goroutine (which risked deadlocks).
	// Expired entries are cleaned up by RunMaintenance instead.
	var value string
	err := s.db.QueryRowWithRetry(ctx,
		"SELECT value FROM kv_store WHERE key = ? AND (expires_at IS NULL OR expires_at > ?)",
		key, timesvc.NowTimestamp(),
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// KVSet stores a key/value pair. ttlSeconds == 0 means no expiration.
// Negative ttlSeconds means the key is immediately expired.
func (s *KVStoreService) KVSet(ctx context.Context, key, value string, ttlSeconds int) error {
	now := timesvc.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(ctx,
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, value, now, expiresAt,
	)
	return err
}

// KVSetObserved stores a key/value pair as observed-state (state_tier = 'observed').
// Observed-state entries are excluded from the bound freshness root and are
// hashed separately in the observed-state commitment. ttlSeconds == 0 means no expiration.
// Negative ttlSeconds means the key is immediately expired.
func (s *KVStoreService) KVSetObserved(ctx context.Context, key, value string, ttlSeconds int) error {
	now := timesvc.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(ctx,
		`INSERT INTO kv_store (key, value, created_at, expires_at, state_tier)
		 VALUES (?, ?, ?, ?, 'observed')
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at, state_tier = 'observed'`,
		key, value, now, expiresAt,
	)
	return err
}

// KVDelete removes a key.
func (s *KVStoreService) KVDelete(ctx context.Context, key string) error {
	_, err := s.db.ExecWithRetry(ctx, "DELETE FROM kv_store WHERE key = ?", key)
	return err
}

// KVKeys returns all keys matching a glob pattern.
func (s *KVStoreService) KVKeys(ctx context.Context, pattern string) ([]string, error) {
	keys, err := sqliteutil.MaterializeRows(ctx, s.db,
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?)",
		[]interface{}{pattern, timesvc.NowTimestamp()},
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

// KVEntries returns every live key/value pair whose key matches a glob pattern,
// in one query.
func (s *KVStoreService) KVEntries(ctx context.Context, pattern string) (map[string]string, error) {
	type entry struct{ key, value string }
	entries, err := sqliteutil.MaterializeRows(ctx, s.db,
		"SELECT key, value FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?)",
		[]interface{}{pattern, timesvc.NowTimestamp()},
		func(r *sql.Rows) (entry, error) {
			var e entry
			err := r.Scan(&e.key, &e.value)
			return e, err
		})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.key] = e.value
	}
	return out, nil
}

// RunMaintenance removes expired KV entries from the database.
func (s *KVStoreService) RunMaintenance(ctx context.Context) error {
	now := timesvc.NowTimestamp()
	_, err := s.db.ExecWithRetry(ctx, "DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired kv entries: %w", err)
	}
	return nil
}
