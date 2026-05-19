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
		used_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_nonce_expires_at ON nonce_usage(expires_at);
	`

	_, err := rs.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create nonce_usage table: %w", err)
	}

	return nil
}

// CheckAndSetNonce returns true if the nonce was already used (replay detected).
// If not used, it marks the nonce as used and returns false.
func (rs *SQLReplayStore) CheckAndSetNonce(nonce string, expiresAt time.Time) (bool, error) {
	// First, clean up expired nonces
	if err := rs.cleanupExpiredNonces(); err != nil {
		rs.logger.Warn("Failed to cleanup expired nonces", string(constants.ConnectionStateError), err)
	}

	// Check if nonce exists
	var existingUsedAt string
	err := rs.db.QueryRow(
		"SELECT used_at FROM nonce_usage WHERE nonce = ?",
		nonce,
	).Scan(&existingUsedAt)

	if err == nil {
		// Nonce already exists - replay detected
		rs.logger.Warn("Nonce replay detected", "nonce", nonce, "used_at", existingUsedAt)
		return true, nil
	}

	if err != sql.ErrNoRows {
		// Unexpected error
		return false, fmt.Errorf("failed to check nonce: %w", err)
	}

	// Nonce doesn't exist, insert it
	usedAt := sqliteutil.FormatTimestamp(time.Now().UTC())
	expiresAtStr := sqliteutil.FormatTimestamp(expiresAt.UTC())

	_, err = rs.db.Exec(
		"INSERT INTO nonce_usage (nonce, used_at, expires_at) VALUES (?, ?, ?)",
		nonce, usedAt, expiresAtStr,
	)
	if err != nil {
		return false, fmt.Errorf("failed to insert nonce: %w", err)
	}

	return false, nil
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
