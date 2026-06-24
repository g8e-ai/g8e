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
	"fmt"
	"hash"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// StateRootService provides state merkle root calculation with caching.
// It computes two roots:
//   - Bound root: hashes only bound-state rows (config, governance, filesystem
//     mutations) — this is the freshness root that gates transaction admission.
//   - Observed root: hashes observed-state rows (telemetry, environmental
//     readings) — a separate commitment that does NOT gate admission but is
//     chained into the audit ledger.
//
// This tiering implements §8 of the position paper: conflating observed and
// bound state causes the binding root to churn continuously, making every
// in-flight envelope stale and degrading fail-closed into fail-always.
//
// The bound root also incorporates the token keymap hash from the ScrubbingService
// (§9): this binds the token→value mapping into the state Merkle root so that a
// rehydration substitution is a broken transaction, not silent corruption.
type StateRootService struct {
	db     *sqliteutil.DB
	logger *slog.Logger

	// Caching (bound root)
	mu                 sync.Mutex
	cachedStateRoot    string
	cachedStateVersion int64

	// Caching (observed root)
	cachedObservedRoot    string
	cachedObservedVersion int64

	// Optional keymap hash provider for token binding (§9)
	keymapHashProvider KeymapHashProvider
}

// KeymapHashProvider returns a deterministic hash of the token keymap.
// Implemented by ScrubbingService to bind token→value mappings into the state root.
type KeymapHashProvider interface {
	TokenKeymapHash() string
}

// NewStateRootService creates a new state root service.
func NewStateRootService(db *sqliteutil.DB, logger *slog.Logger) *StateRootService {
	return &StateRootService{
		db:     db,
		logger: logger,
	}
}

// SetKeymapHashProvider injects the token keymap hash provider.
// When set, the keymap hash is incorporated into the bound state root.
func (s *StateRootService) SetKeymapHashProvider(p KeymapHashProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keymapHashProvider = p
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
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootCalculate, err)
	}

	// Update cache
	s.cachedStateRoot = root
	s.cachedStateVersion = currentVersion

	// Check state_version before persistence
	var versionBefore int64
	if err := s.db.QueryRowWithRetry("SELECT version FROM state_version WHERE id = 1").Scan(&versionBefore); err != nil {
		s.logger.Warn("Failed to check state_version before persistence", "error", err)
	}

	// Persist to state_root table
	_, err = s.db.ExecWithRetry(
		`INSERT INTO state_root (id, root, updated_at)
		 VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root, updated_at = excluded.updated_at`,
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)

	// Check state_version after persistence
	var versionAfter int64
	if err := s.db.QueryRowWithRetry("SELECT version FROM state_version WHERE id = 1").Scan(&versionAfter); err != nil {
		s.logger.Warn("Failed to check state_version after persistence", "error", err)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootPersist, err)
	}
	return root, nil
}

// InvalidateCache clears the cached state root, forcing a recalculation on next call.
func (s *StateRootService) InvalidateCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedStateRoot = ""
	s.cachedStateVersion = 0
	s.cachedObservedRoot = ""
	s.cachedObservedVersion = 0
	return nil
}

// CalculateStateRoot computes the merkle root of all authoritative state without caching or persistence.
// This is used for initialization when the state_root table is empty.
func (s *StateRootService) CalculateStateRoot() (string, error) {
	return s.calculateStateRoot()
}

// calculateStateRootUncached performs a full state root calculation without caching.
// Used as a fallback when state_version tracking is unavailable.
func (s *StateRootService) calculateStateRootUncached() (string, error) {
	root, err := s.calculateStateRoot()
	if err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootCalculate, err)
	}
	_, err = s.db.ExecWithRetry(
		`INSERT INTO state_root (id, root, updated_at)
		 VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root, updated_at = excluded.updated_at`,
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootPersist, err)
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
			return fmt.Errorf("%w: %v", constants.ErrStateRootScanDocuments, err)
		}
		return writeRowToHash(h, "documents", collection, id, data)
	}); err != nil {
		s.logger.Error("Failed to query documents for state root calculation", "error", err)
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootHashDocuments, err)
	}

	now := sqliteutil.NowTimestamp()

	// 2. KV Store (Bound state only)
	// Filter for active entries only. Exclude created_at.
	// expires_at IS included because it affects the active state of the entry.
	// Exclude cache management entries (g8e:cache:*) as they are ephemeral and not authoritative state.
	// Only include bound-state rows (state_tier = 'bound') — observed-state rows
	// are hashed separately in calculateObservedStateRoot().
	if err := s.hashTableToStream(h, "SELECT key, value, COALESCE(expires_at, '') FROM kv_store WHERE key NOT LIKE 'g8e:cache:%' AND state_tier = 'bound' AND (expires_at IS NULL OR expires_at > ?) ORDER BY key", []interface{}{now}, func(r *sql.Rows) error {
		var key, value, expiresAt string
		if err := r.Scan(&key, &value, &expiresAt); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrStateRootScanKVStore, err)
		}
		return writeRowToHash(h, "kv_store", key, value, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query kv_store for state root calculation", "error", err)
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootHashKVStore, err)
	}

	// 3. Blobs (Bound state only)
	// Filter for active entries only. Exclude created_at.
	// data is included (as hex for determinism).
	// Only include bound-state rows (state_tier = 'bound').
	if err := s.hashTableToStream(h, "SELECT namespace, id, size, content_type, hex(data), COALESCE(expires_at, '') FROM blobs WHERE state_tier = 'bound' AND (expires_at IS NULL OR expires_at > ?) ORDER BY namespace, id", []interface{}{now}, func(r *sql.Rows) error {
		var namespace, id, contentType, dataHex, expiresAt string
		var size int64
		if err := r.Scan(&namespace, &id, &size, &contentType, &dataHex, &expiresAt); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrStateRootScanBlobs, err)
		}
		return writeRowToHash(h, "blobs", namespace, id, size, contentType, dataHex, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query blobs for state root calculation", "error", err)
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootHashBlobs, err)
	}

	// 4. Nonces and SSE events are EXCLUDED (volatile/metadata)
	// 5. Observed-state rows (state_tier = 'observed') are EXCLUDED from the bound
	//    root — they are hashed separately in calculateObservedStateRoot().

	// 6. Token keymap hash (§9 binding)
	// The token→value mapping is bound into the state root so that a rehydration
	// substitution (altering the keymap after a transaction hash was computed)
	// produces a broken transaction, not silent corruption.
	if s.keymapHashProvider != nil {
		keymapHash := s.keymapHashProvider.TokenKeymapHash()
		if keymapHash != "" {
			h.Write([]byte(`{"table":"token_keymap","values":["`))
			h.Write([]byte(keymapHash))
			h.Write([]byte(`"]}`))
			h.Write([]byte("\n"))
		}
	}

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// calculateObservedStateRoot computes a separate commitment over observed-state
// rows only. This root does NOT gate transaction admission — it is chained into
// the audit ledger so observed evidence is tamper-evident without churning the
// bound root that in-flight envelopes depend on.
func (s *StateRootService) calculateObservedStateRoot() (string, error) {
	h := sha256.New()
	now := sqliteutil.NowTimestamp()

	// Observed KV entries (state_tier = 'observed')
	if err := s.hashTableToStream(h, "SELECT key, value, COALESCE(expires_at, '') FROM kv_store WHERE key NOT LIKE 'g8e:cache:%' AND state_tier = 'observed' AND (expires_at IS NULL OR expires_at > ?) ORDER BY key", []interface{}{now}, func(r *sql.Rows) error {
		var key, value, expiresAt string
		if err := r.Scan(&key, &value, &expiresAt); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrStateRootScanKVStore, err)
		}
		return writeRowToHash(h, "kv_store_observed", key, value, expiresAt)
	}); err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootHashKVStore, err)
	}

	// Observed blobs (state_tier = 'observed')
	if err := s.hashTableToStream(h, "SELECT namespace, id, size, content_type, hex(data), COALESCE(expires_at, '') FROM blobs WHERE state_tier = 'observed' AND (expires_at IS NULL OR expires_at > ?) ORDER BY namespace, id", []interface{}{now}, func(r *sql.Rows) error {
		var namespace, id, contentType, dataHex, expiresAt string
		var size int64
		if err := r.Scan(&namespace, &id, &size, &contentType, &dataHex, &expiresAt); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrStateRootScanBlobs, err)
		}
		return writeRowToHash(h, "blobs_observed", namespace, id, size, contentType, dataHex, expiresAt)
	}); err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootHashBlobs, err)
	}

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// GetObservedStateRoot returns the observed-state commitment root.
// This root is separate from the bound root and does NOT gate transaction
// admission. It is used for audit ledger chaining of observed evidence.
func (s *StateRootService) GetObservedStateRoot() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var currentVersion int64
	err := s.db.QueryRowWithRetry("SELECT version FROM state_version WHERE id = 1").Scan(&currentVersion)
	if err != nil {
		root, err := s.calculateObservedStateRoot()
		if err != nil {
			return "", fmt.Errorf("%w: %v", constants.ErrStateRootCalculate, err)
		}
		return root, nil
	}

	if currentVersion == s.cachedObservedVersion && s.cachedObservedRoot != "" {
		return s.cachedObservedRoot, nil
	}

	root, err := s.calculateObservedStateRoot()
	if err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrStateRootCalculate, err)
	}

	s.cachedObservedRoot = root
	s.cachedObservedVersion = currentVersion
	return root, nil
}

// hashTableToStream executes a query and streams each row to the hash writer.
// This avoids materializing all rows in memory, which is critical for large databases.
func (s *StateRootService) hashTableToStream(h hash.Hash, query string, args []interface{}, scan func(*sql.Rows) error) error {
	rows, err := s.db.QueryWithRetry(query, args...)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrStateRootQueryTable, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrStateRootIterateRows, err)
	}
	return nil
}

// writeRowToHash writes a row's values to the hash in a deterministic format.
// The format matches the previous JSON structure for compatibility:
// {"table":"table_name","values":[v1,v2,...]}
func writeRowToHash(h hash.Hash, table string, values ...interface{}) error {
	// Write deterministic JSON-like format directly to hash
	// This avoids allocating intermediate JSON strings
	h.Write([]byte(`{"table":"`))
	h.Write([]byte(table))
	h.Write([]byte(`","values":[`))

	for i, v := range values {
		if i > 0 {
			h.Write([]byte(","))
		}
		switch val := v.(type) {
		case string:
			h.Write([]byte(`"`))
			h.Write([]byte(val))
			h.Write([]byte(`"`))
		case int64:
			fmt.Fprintf(h, "%d", val)
		default:
			return fmt.Errorf("%w: %T", constants.ErrStateRootUnsupportedType, v)
		}
	}

	h.Write([]byte("]}\n"))
	return nil
}
