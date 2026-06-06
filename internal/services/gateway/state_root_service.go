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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"hash"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// StateRootService provides state merkle root calculation with caching.
type StateRootService struct {
	db     *sqliteutil.DB
	logger *slog.Logger

	// Caching
	mu                 sync.Mutex
	cachedStateRoot    string
	cachedStateVersion int64
}

// NewStateRootService creates a new state root service.
func NewStateRootService(db *sqliteutil.DB, logger *slog.Logger) *StateRootService {
	return &StateRootService{
		db:     db,
		logger: logger,
	}
}

// GetCurrentStateRoot returns the current state merkle root.
// Uses caching based on state_version to avoid full table scans when state hasn't changed.
func (s *StateRootService) GetCurrentStateRoot() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current state version
	var currentVersion int64
	err := s.db.QueryRowWithRetry("SELECT version FROM state_version WHERE id = 1").Scan(&currentVersion)
	if err != nil {
		// Fallback to full calculation if version table is unavailable
		return s.calculateStateRootUncached()
	}

	// If version hasn't changed, return cached root
	if currentVersion == s.cachedStateVersion && s.cachedStateRoot != "" {
		return s.cachedStateRoot, nil
	}

	// Version changed or cache is empty, recalculate
	root, err := s.calculateStateRoot()
	if err != nil {
		return "", err
	}

	// Update cache
	s.cachedStateRoot = root
	s.cachedStateVersion = currentVersion

	// Persist to state_root table
	_, err = s.db.ExecWithRetry(
		`INSERT INTO state_root (id, root, updated_at)
		 VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root, updated_at = excluded.updated_at`,
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)
	if err != nil {
		return "", err
	}
	return root, nil
}

// InvalidateCache clears the cached state root, forcing a recalculation on next call.
func (s *StateRootService) InvalidateCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedStateRoot = ""
	s.cachedStateVersion = 0
	return nil
}

// calculateStateRootUncached performs a full state root calculation without caching.
// Used as a fallback when state_version tracking is unavailable.
func (s *StateRootService) calculateStateRootUncached() (string, error) {
	root, err := s.calculateStateRoot()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecWithRetry(
		`INSERT INTO state_root (id, root, updated_at)
		 VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root, updated_at = excluded.updated_at`,
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)
	if err != nil {
		return "", err
	}
	return root, nil
}

// calculateStateRoot computes the merkle root of all authoritative state.
func (s *StateRootService) calculateStateRoot() (string, error) {
	h := sha256.New()

	// 1. Documents (Authoritative)
	// Exclude metadata-only timestamps (created_at, updated_at) to ensure
	// the state root only changes when the content actually changes.
	if err := s.hashTableToStream(h, "SELECT collection, id, data FROM documents ORDER BY collection, id", nil, func(r *sql.Rows) error {
		var collection, id, data string
		if err := r.Scan(&collection, &id, &data); err != nil {
			return err
		}
		return writeRowToHash(h, "documents", collection, id, data)
	}); err != nil {
		s.logger.Error("Failed to query documents for state root calculation", "error", err)
		return "", err
	}

	now := sqliteutil.NowTimestamp()

	// 2. KV Store (Authoritative)
	// Filter for active entries only. Exclude created_at.
	// expires_at IS included because it affects the active state of the entry.
	if err := s.hashTableToStream(h, "SELECT key, value, COALESCE(expires_at, '') FROM kv_store WHERE expires_at IS NULL OR expires_at > ? ORDER BY key", []interface{}{now}, func(r *sql.Rows) error {
		var key, value, expiresAt string
		if err := r.Scan(&key, &value, &expiresAt); err != nil {
			return err
		}
		return writeRowToHash(h, "kv_store", key, value, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query kv_store for state root calculation", "error", err)
		return "", err
	}

	// 3. Blobs (Authoritative)
	// Filter for active entries only. Exclude created_at.
	// data is included (as hex for determinism).
	if err := s.hashTableToStream(h, "SELECT namespace, id, size, content_type, hex(data), COALESCE(expires_at, '') FROM blobs WHERE expires_at IS NULL OR expires_at > ? ORDER BY namespace, id", []interface{}{now}, func(r *sql.Rows) error {
		var namespace, id, contentType, dataHex, expiresAt string
		var size int64
		if err := r.Scan(&namespace, &id, &size, &contentType, &dataHex, &expiresAt); err != nil {
			return err
		}
		return writeRowToHash(h, "blobs", namespace, id, size, contentType, dataHex, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query blobs for state root calculation", "error", err)
		return "", err
	}

	// 4. Nonces and SSE events are EXCLUDED (volatile/metadata)

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// hashTableToStream executes a query and streams each row to the hash writer.
// This avoids materializing all rows in memory, which is critical for large databases.
func (s *StateRootService) hashTableToStream(h hash.Hash, query string, args []interface{}, scan func(*sql.Rows) error) error {
	rows, err := s.db.QueryWithRetry(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
