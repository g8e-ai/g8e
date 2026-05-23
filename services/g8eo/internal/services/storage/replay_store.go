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

package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sqliteutil"
)

// SQLReplayStore provides nonce replay protection using SQLite.
type SQLReplayStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLReplayStore creates a new replay store backed by SQLite.
func NewSQLReplayStore(db *sql.DB, logger *slog.Logger) (*SQLReplayStore, error) {
	rs := &SQLReplayStore{
		db:     db,
		logger: logger,
	}

	if err := rs.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize replay store schema: %w", err)
	}

	return rs, nil
}

// initSchema creates the nonce table if it doesn't exist.
func (rs *SQLReplayStore) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS nonce_usage (
		nonce TEXT PRIMARY KEY,
		reserved_at TEXT NOT NULL,
		used_at TEXT,
		expires_at TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'reserved'
	);
	CREATE INDEX IF NOT EXISTS idx_nonce_expires_at ON nonce_usage(expires_at);
	CREATE INDEX IF NOT EXISTS idx_nonce_status ON nonce_usage(status);
	`

	_, err := rs.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create nonce_usage table: %w", err)
	}

	return nil
}

// CheckAndSetNonce returns true if the nonce was already used (replay detected).
// If not used, it marks the nonce as used and returns false.
// This is the legacy method for backward compatibility.
func (rs *SQLReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	return rs.ReserveNonce(nonce, expiresAt)
}

// ReserveNonce atomically reserves a nonce for early replay protection.
// Returns true if the nonce was already reserved/used (replay detected).
// If not used, it reserves the nonce and returns false.
// Fail-closed: any SQLite error during cleanup or reservation returns an error.
func (rs *SQLReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	// First, clean up expired nonces - fail-closed on cleanup errors
	if err := rs.cleanupExpiredNonces(); err != nil {
		rs.logger.Error("Failed to cleanup expired nonces - replay protection unavailable",
			string(constants.ConnectionStateError), err)
		return false, fmt.Errorf("nonce cleanup failed: %w", err)
	}

	// Check if nonce exists in any state (reserved or used)
	var existingStatus string
	err := rs.db.QueryRow(
		"SELECT status FROM nonce_usage WHERE nonce = ?",
		nonce,
	).Scan(&existingStatus)

	if err == nil {
		// Nonce already exists - replay detected
		rs.logger.Warn("Nonce replay detected", "nonce", nonce, "status", existingStatus)
		return true, nil
	}

	if err != sql.ErrNoRows {
		// Unexpected error - fail-closed
		rs.logger.Error("Failed to check nonce - replay protection unavailable",
			"nonce", nonce, string(constants.ConnectionStateError), err)
		return false, fmt.Errorf("failed to check nonce: %w", err)
	}

	// Nonce doesn't exist, insert it as reserved
	reservedAt := sqliteutil.FormatTimestamp(time.Now().UTC())
	expiresAtStr := sqliteutil.FormatTimestamp(expiresAt.UTC())

	_, err = rs.db.Exec(
		"INSERT INTO nonce_usage (nonce, reserved_at, expires_at, status) VALUES (?, ?, ?, 'reserved')",
		nonce, reservedAt, expiresAtStr,
	)
	if err != nil {
		rs.logger.Error("Failed to reserve nonce - replay protection unavailable",
			"nonce", nonce, string(constants.ConnectionStateError), err)
		return false, fmt.Errorf("failed to reserve nonce: %w", err)
	}

	return false, nil
}

// FinalizeNonce marks a reserved nonce as fully consumed.
func (rs *SQLReplayStore) FinalizeNonce(nonce string) error {
	usedAt := sqliteutil.FormatTimestamp(time.Now().UTC())

	result, err := rs.db.Exec(
		"UPDATE nonce_usage SET used_at = ?, status = 'used' WHERE nonce = ? AND status = 'reserved'",
		usedAt, nonce,
	)
	if err != nil {
		return fmt.Errorf("failed to finalize nonce: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("nonce not found or not in reserved state: %s", nonce)
	}

	return nil
}

// ReleaseNonce removes a reservation for a failed transaction.
func (rs *SQLReplayStore) ReleaseNonce(nonce string) error {
	result, err := rs.db.Exec(
		"DELETE FROM nonce_usage WHERE nonce = ? AND status = 'reserved'",
		nonce,
	)
	if err != nil {
		return fmt.Errorf("failed to release nonce: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rows == 0 {
		// Nonce was not in reserved state - may have been finalized already
		// This is not an error, just a no-op
		return nil
	}

	return nil
}

// cleanupExpiredNonces removes nonces that have expired.
func (rs *SQLReplayStore) cleanupExpiredNonces() error {
	now := sqliteutil.FormatTimestamp(time.Now().UTC())
	_, err := rs.db.Exec("DELETE FROM nonce_usage WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("failed to delete expired nonces: %w", err)
	}
	return nil
}

// CleanupStaleReserved removes reserved nonces that have been in reserved state
// for too long (e.g., due to a crash before finalization).
func (rs *SQLReplayStore) CleanupStaleReserved(maxReservedDuration time.Duration) error {
	cutoff := time.Now().UTC().Add(-maxReservedDuration)
	cutoffStr := sqliteutil.FormatTimestamp(cutoff)

	_, err := rs.db.Exec(
		"DELETE FROM nonce_usage WHERE status = 'reserved' AND reserved_at < ?",
		cutoffStr,
	)
	if err != nil {
		return fmt.Errorf("failed to delete stale reserved nonces: %w", err)
	}
	return nil
}

// Prune removes old nonce records to prevent unbounded growth.
func (rs *SQLReplayStore) Prune(retentionDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	cutoffStr := sqliteutil.FormatTimestamp(cutoff)

	_, err := rs.db.Exec("DELETE FROM nonce_usage WHERE used_at < ?", cutoffStr)
	if err != nil {
		return fmt.Errorf("failed to prune nonce_usage: %w", err)
	}

	return nil
}
