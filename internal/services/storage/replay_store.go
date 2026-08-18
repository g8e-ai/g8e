// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
)

// NonceRow represents a single nonce record for export.
type NonceRow struct {
	Nonce      string
	Status     string
	ReservedAt time.Time
	UsedAt     *time.Time
	ExpiresAt  time.Time
}

// containsString checks if a string contains a substring (case-sensitive).
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ReplayStoreConfig holds configuration for the replay store service.
type ReplayStoreConfig struct {
	DBPath string
}

// DefaultReplayStoreConfig returns the default configuration.
func DefaultReplayStoreConfig() *ReplayStoreConfig {
	return &ReplayStoreConfig{
		DBPath: constants.ReplayStoreDBPath,
	}
}

// SQLReplayStore provides nonce replay protection using SQLite.
type SQLReplayStore struct {
	db     *sqliteutil.DB
	logger *slog.Logger
	config *ReplayStoreConfig
}

// NewSQLReplayStore creates a new replay store backed by SQLite.
func NewSQLReplayStore(config *ReplayStoreConfig, logger *slog.Logger) (*SQLReplayStore, error) {
	if config == nil {
		config = DefaultReplayStoreConfig()
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	rs := &SQLReplayStore{
		db:     db,
		logger: logger,
		config: config,
	}

	if err := rs.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize replay store schema: %w", err)
	}

	rs.logger.Info("Replay store initialized", "db_path", config.DBPath)
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

	_, err := rs.db.ExecWithRetry(query)
	if err != nil {
		return fmt.Errorf("failed to create nonce_usage table: %w", err)
	}

	return nil
}

// ListNonces retrieves nonce records with pagination, ordered by reserved_at ASC.
func (rs *SQLReplayStore) ListNonces(limit, offset int) ([]*NonceRow, error) {
	if rs == nil || rs.db == nil {
		return nil, fmt.Errorf("replay store not initialized")
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT nonce, reserved_at, used_at, expires_at, status
	FROM nonce_usage
	ORDER BY reserved_at ASC
	LIMIT ? OFFSET ?
	`

	type nonceRowRaw struct {
		nonce         string
		reservedAtStr string
		usedAtStr     sql.NullString
		expiresAtStr  string
		status        string
	}

	rows, err := sqliteutil.MaterializeRows(rs.db, query, []interface{}{limit, offset}, func(r *sql.Rows) (nonceRowRaw, error) {
		var row nonceRowRaw
		err := r.Scan(&row.nonce, &row.reservedAtStr, &row.usedAtStr, &row.expiresAtStr, &row.status)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("replay store: list nonces: %w", err)
	}

	var results []*NonceRow
	for _, row := range rows {
		n := &NonceRow{
			Nonce:  row.nonce,
			Status: row.status,
		}
		n.ReservedAt, _ = timesvc.ParseTimestamp(row.reservedAtStr)
		n.ExpiresAt, _ = timesvc.ParseTimestamp(row.expiresAtStr)
		if row.usedAtStr.Valid {
			t, _ := timesvc.ParseTimestamp(row.usedAtStr.String)
			n.UsedAt = &t
		}
		results = append(results, n)
	}

	return results, nil
}

// ReserveNonce atomically reserves a nonce for early replay protection.
// Returns true if the nonce was already reserved/used (replay detected).
// If not used, it reserves the nonce and returns false.
// Uses SQLite's UNIQUE constraint on nonce column for atomic replay detection.
// Fail-closed: any SQLite error during cleanup or reservation returns an error.
func (rs *SQLReplayStore) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	// First, clean up expired nonces - fail-closed on cleanup errors
	if err := rs.cleanupExpiredNonces(); err != nil {
		rs.logger.Error("Failed to cleanup expired nonces - replay protection unavailable",
			string(constants.ConnectionStateError), err)
		return false, fmt.Errorf("nonce cleanup failed: %w", err)
	}

	// Attempt atomic insert - UNIQUE constraint violation indicates replay
	reservedAt := timesvc.FormatTimestamp(time.Now().UTC())
	expiresAtStr := timesvc.FormatTimestamp(expiresAt.UTC())

	_, err := rs.db.ExecWithRetry(
		"INSERT INTO nonce_usage (nonce, reserved_at, expires_at, status) VALUES (?, ?, ?, 'reserved')",
		nonce, reservedAt, expiresAtStr,
	)
	if err != nil {
		// Check if this is a UNIQUE constraint violation (replay detected)
		// SQLite returns constraint violation errors with various formats
		errStr := err.Error()
		if containsString(errStr, "UNIQUE constraint failed") ||
			containsString(errStr, "constraint failed") {
			// Replay detected - fetch existing status for logging
			var existingStatus string
			_ = rs.db.QueryRowWithRetry("SELECT status FROM nonce_usage WHERE nonce = ?", nonce).Scan(&existingStatus)
			rs.logger.Warn("Nonce replay detected (atomic constraint)", "nonce", nonce, "status", existingStatus)
			return true, nil
		}

		// Other error - fail-closed
		rs.logger.Error("Failed to reserve nonce - replay protection unavailable",
			"nonce", nonce, string(constants.ConnectionStateError), err)
		return false, fmt.Errorf("failed to reserve nonce: %w", err)
	}

	return false, nil
}

// FinalizeNonce marks a reserved nonce as fully consumed.
func (rs *SQLReplayStore) FinalizeNonce(nonce string) error {
	usedAt := timesvc.FormatTimestamp(time.Now().UTC())

	result, err := rs.db.ExecWithRetry(
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
	result, err := rs.db.ExecWithRetry(
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
	now := timesvc.FormatTimestamp(time.Now().UTC())
	_, err := rs.db.ExecWithRetry("DELETE FROM nonce_usage WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("failed to delete expired nonces: %w", err)
	}
	return nil
}

// CleanupStaleReserved removes reserved nonces that have been in reserved state
// for too long (e.g., due to a crash before finalization).
func (rs *SQLReplayStore) CleanupStaleReserved(maxReservedDuration time.Duration) error {
	cutoff := time.Now().UTC().Add(-maxReservedDuration)
	cutoffStr := timesvc.FormatTimestamp(cutoff)

	_, err := rs.db.ExecWithRetry(
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
	cutoffStr := timesvc.FormatTimestamp(cutoff)

	_, err := rs.db.ExecWithRetry("DELETE FROM nonce_usage WHERE used_at < ?", cutoffStr)
	if err != nil {
		return fmt.Errorf("failed to prune nonce_usage: %w", err)
	}

	return nil
}

// Close shuts down the replay store service.
func (rs *SQLReplayStore) Close() error {
	if rs == nil {
		return nil
	}

	if rs.db != nil {
		return rs.db.Close()
	}

	return nil
}
