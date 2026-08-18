// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
)

// replayStoreDB defines the database operations required by ReplayStoreService.
// This interface enables dependency injection for Tier 1 unit testing.
type replayStoreDB interface {
	QueryRowWithRetry(query string, args ...any) rowScanner
	ExecWithRetry(query string, args ...any) (sql.Result, error)
}

// rowScanner is an interface that matches the Scan method of sql.Row
type rowScanner interface {
	Scan(dest ...any) error
}

// dbAdapter wraps *sqliteutil.DB to implement replayStoreDB interface
type dbAdapter struct {
	*sqliteutil.DB
}

func (a *dbAdapter) QueryRowWithRetry(query string, args ...any) rowScanner {
	return a.DB.QueryRowWithRetry(query, args...)
}

// ReplayStoreService provides nonce replay protection for gateway mode.
type ReplayStoreService struct {
	db     replayStoreDB
	logger *slog.Logger
}

// NewReplayStoreService creates a new replay store service.
func NewReplayStoreService(db *sqliteutil.DB, logger *slog.Logger) *ReplayStoreService {
	return &ReplayStoreService{
		db:     &dbAdapter{DB: db},
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
	expStr := timesvc.FormatTimestamp(expiresAt)
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
		return fmt.Errorf("finalize nonce: %w", err)
	}
	return nil
}

// ReleaseNonce removes a reservation for a failed transaction.
func (s *ReplayStoreService) ReleaseNonce(nonce string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM nonces WHERE nonce = ? AND status = 'reserved'", nonce)
	if err != nil {
		return fmt.Errorf("release nonce: %w", err)
	}
	return nil
}

// Close is a no-op; ReplayStoreService shares the DB connection managed by CanonicalDBService.
func (s *ReplayStoreService) Close() error {
	return nil
}

// CleanupExpiredNonces removes expired nonces from the database.
func (s *ReplayStoreService) CleanupExpiredNonces() error {
	now := timesvc.NowTimestamp()
	_, err := s.db.ExecWithRetry("DELETE FROM nonces WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("cleanup expired nonces: %w", err)
	}
	return nil
}
