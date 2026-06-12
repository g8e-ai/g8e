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

// ReplayStoreService provides nonce replay protection for gateway mode.
type ReplayStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewReplayStoreService creates a new replay store service.
func NewReplayStoreService(db *sqliteutil.DB, logger *slog.Logger) *ReplayStoreService {
	return &ReplayStoreService{
		db:     db,
		logger: logger,
	}
}

// ReserveNonce atomically reserves a nonce for early replay protection.
// Returns true if the nonce was already reserved/used (replay detected).
// If not used, it reserves the nonce and returns false.
func (s *ReplayStoreService) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	// 1. Check if exists
	var existing string
	err := s.db.QueryRowWithRetry("SELECT nonce FROM nonces WHERE nonce = ?", nonce).Scan(&existing)
	if err == nil {
		return true, nil // Replay detected
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	// 2. Not used, insert as reserved
	expStr := sqliteutil.FormatTimestamp(expiresAt)
	_, err = s.db.ExecWithRetry("INSERT INTO nonces (nonce, expires_at, status) VALUES (?, ?, 'reserved')", nonce, expStr)
	if err != nil {
		// Concurrent insert might fail with constraint violation - that's a replay
		if sqliteutil.IsUniqueConstraintError(err) {
			return true, nil
		}
		return false, err
	}

	return false, nil
}

// FinalizeNonce marks a reserved nonce as fully consumed.
func (s *ReplayStoreService) FinalizeNonce(nonce string) error {
	_, err := s.db.ExecWithRetry("UPDATE nonces SET status = 'used' WHERE nonce = ? AND status = 'reserved'", nonce)
	if err != nil {
		return fmt.Errorf("failed to finalize nonce: %w", err)
	}
	return nil
}

// ReleaseNonce removes a reservation for a failed transaction.
func (s *ReplayStoreService) ReleaseNonce(nonce string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM nonces WHERE nonce = ? AND status = 'reserved'", nonce)
	if err != nil {
		return fmt.Errorf("failed to release nonce: %w", err)
	}
	return nil
}

// Close is a no-op; ReplayStoreService shares the DB connection managed by CanonicalDBService.
func (s *ReplayStoreService) Close() error {
	return nil
}

// CleanupExpiredNonces removes expired nonces from the database.
func (s *ReplayStoreService) CleanupExpiredNonces() error {
	now := sqliteutil.NowTimestamp()
	_, err := s.db.ExecWithRetry("DELETE FROM nonces WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired nonces: %w", err)
	}
	return nil
}
