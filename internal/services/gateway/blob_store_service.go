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
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
)

// BlobStoreService provides binary blob storage with optional TTL expiration.
type BlobStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewBlobStoreService creates a new blob store service.
func NewBlobStoreService(db *sqliteutil.DB, logger *slog.Logger) *BlobStoreService {
	return &BlobStoreService{
		db:     db,
		logger: logger,
	}
}

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
func (s *BlobStoreService) BlobPut(namespace, id string, data []byte, contentType string, ttlSeconds int) error {
	now := timesvc.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO blobs (namespace, id, size, content_type, data, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(namespace, id) DO UPDATE SET
		   size         = excluded.size,
		   content_type = excluded.content_type,
		   data         = excluded.data,
		   expires_at   = excluded.expires_at`,
		namespace, id, int64(len(data)), contentType, data, now, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("blob_store: put %s/%s: %w", namespace, id, constants.ErrBlobStorePutFailed)
	}
	return nil
}

// BlobPutObserved stores raw bytes as observed-state (state_tier = 'observed').
// Observed-state blobs are excluded from the bound freshness root and are
// hashed separately in the observed-state commitment.
func (s *BlobStoreService) BlobPutObserved(namespace, id string, data []byte, contentType string, ttlSeconds int) error {
	now := timesvc.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := timesvc.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO blobs (namespace, id, size, content_type, data, created_at, expires_at, state_tier)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'observed')
		 ON CONFLICT(namespace, id) DO UPDATE SET
		   size         = excluded.size,
		   content_type = excluded.content_type,
		   data         = excluded.data,
		   expires_at   = excluded.expires_at,
		   state_tier   = 'observed'`,
		namespace, id, int64(len(data)), contentType, data, now, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("blob_store: put observed %s/%s: %w", namespace, id, constants.ErrBlobStorePutFailed)
	}
	return nil
}

// BlobGet retrieves the raw bytes and content type for a blob.
// Returns (nil, "", false) if not found or expired.
func (s *BlobStoreService) BlobGet(namespace, id string) ([]byte, string, bool) {
	var data []byte
	var contentType string
	err := s.db.QueryRowWithRetry(
		"SELECT data, content_type FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, timesvc.NowTimestamp(),
	).Scan(&data, &contentType)
	if err != nil {
		return nil, "", false
	}
	return data, contentType, true
}

// BlobMeta retrieves metadata for a blob without loading the data.
// Returns (nil, false) if not found or expired.
func (s *BlobStoreService) BlobMeta(namespace, id string) (*BlobRecord, bool) {
	var rec BlobRecord
	var createdAtStr string
	err := s.db.QueryRowWithRetry(
		"SELECT id, namespace, size, content_type, created_at FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, timesvc.NowTimestamp(),
	).Scan(&rec.ID, &rec.Namespace, &rec.Size, &rec.ContentType, &createdAtStr)
	if err != nil {
		return nil, false
	}
	t, err := timesvc.ParseTimestamp(createdAtStr)
	if err != nil {
		s.logger.Error("failed to parse blob creation time", "error", err, "timestamp", createdAtStr)
		return nil, false
	}
	rec.CreatedAt = t
	return &rec, true
}

// BlobDelete removes a single blob. Returns (true, nil) if deleted, (false, nil) if not found.
func (s *BlobStoreService) BlobDelete(namespace, id string) (bool, error) {
	result, err := s.db.ExecWithRetry(
		"DELETE FROM blobs WHERE namespace = ? AND id = ?",
		namespace, id,
	)
	if err != nil {
		return false, fmt.Errorf("blob_store: delete %s/%s: %w", namespace, id, constants.ErrBlobStoreDeleteFailed)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("blob_store: rows affected: %w", constants.ErrInternal)
	}
	return n > 0, nil
}

// BlobDeleteNamespace removes all blobs under a namespace.
// Returns the count of deleted blobs.
func (s *BlobStoreService) BlobDeleteNamespace(namespace string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM blobs WHERE namespace = ?", namespace)
	if err != nil {
		return 0, fmt.Errorf("blob_store: delete namespace %s: %w", namespace, constants.ErrBlobStoreDeleteFailed)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("blob_store: rows affected: %w", constants.ErrInternal)
	}
	return n, nil
}

// RunMaintenance removes expired blobs from the database.
func (s *BlobStoreService) RunMaintenance() error {
	now := timesvc.NowTimestamp()
	_, err := s.db.ExecWithRetry("DELETE FROM blobs WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("blob_store: cleanup: %w", constants.ErrBlobStoreCleanupFailed)
	}
	return nil
}
