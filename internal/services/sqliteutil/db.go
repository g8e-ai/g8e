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

package sqliteutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/internal/constants"
)

// DBConfig holds common configuration for opening a SQLite database.
type DBConfig struct {
	// Path is the filesystem path to the SQLite database file.
	Path string

	// CacheSizeMB is the SQLite page cache size in megabytes.
	// Default: 64
	CacheSizeMB int

	// BusyTimeoutMs is the SQLite busy timeout in milliseconds.
	// Default: 5000
	BusyTimeoutMs int

	// SetFilePermissions controls whether to chmod the DB file to 0600 after creation.
	// Default: true
	SetFilePermissions bool

	// MaxRetries is the maximum number of retry attempts for SQLITE_BUSY errors.
	// Default: 10
	MaxRetries int

	// RetryBaseDelayMs is the base delay in milliseconds for retry backoff.
	// Actual delay is (attempt + 1) * RetryBaseDelayMs.
	// Default: 50
	RetryBaseDelayMs int
}

// DefaultDBConfig returns a DBConfig with sensible defaults.
// The caller must set Path.
func DefaultDBConfig(path string) DBConfig {
	return DBConfig{
		Path:               path,
		CacheSizeMB:        64,
		BusyTimeoutMs:      30000, // Increased to 30s for parallel test concurrency
		SetFilePermissions: true,
		MaxRetries:         10,
		RetryBaseDelayMs:   50,
	}
}

// DB represents a wrapper around *sql.DB that provides common g8eo data operations.
type DB struct {
	*sql.DB
	logger *slog.Logger
	path   string
	config DBConfig
}

// OpenDB opens (or creates) a SQLite database with best-practice settings.
func OpenDB(cfg DBConfig, logger *slog.Logger) (*DB, error) {
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("sqliteutil: create database directory %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=%d&_mutex=full",
		cfg.Path, cfg.BusyTimeoutMs)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteutil: open database %s: %w", cfg.Path, err)
	}

	// Increase connection pool size to fully utilize WAL mode
	// WAL mode allows multiple readers and one writer concurrently
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("sqliteutil: ping database %s: %w", cfg.Path, err)
	}

	cacheSizeKB := cfg.CacheSizeMB * 1024
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA cache_size = -%d", cacheSizeKB),
		"PRAGMA auto_vacuum = INCREMENTAL",
		"PRAGMA temp_store = MEMORY",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			logger.Warn("Failed to set pragma", "pragma", pragma, string(constants.ConnectionStateError), err)
		}
	}

	if cfg.SetFilePermissions {
		if err := os.Chmod(cfg.Path, 0600); err != nil {
			logger.Warn("Failed to set database file permissions", "path", cfg.Path, string(constants.ConnectionStateError), err)
		}
	}

	logger.Info("SQLite database opened", "path", cfg.Path)
	return &DB{
		DB:     sqlDB,
		logger: logger,
		path:   cfg.Path,
		config: cfg,
	}, nil
}

// GetPath returns the filesystem path to the database file.
func (db *DB) GetPath() string {
	return db.path
}

// RunIncrementalVacuum runs an incremental vacuum to reclaim free pages.
func (db *DB) RunIncrementalVacuum(pages int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", pages))
	if err != nil {
		return fmt.Errorf("sqliteutil: incremental vacuum: %w", err)
	}
	return nil
}

// GetSizeBytes returns the database size in bytes using PRAGMA page_count * page_size.
func (db *DB) GetSizeBytes() (int64, error) {
	var pageCount, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("sqliteutil: query page_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("sqliteutil: query page_size: %w", err)
	}
	return pageCount * pageSize, nil
}

// HealthCheck performs a context-aware ping to verify database connectivity.
// This is used for fail-fast health checks during startup and runtime.
func (db *DB) HealthCheck(ctx context.Context) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqliteutil: health check: %w", err)
	}
	return nil
}

// ExecWithRetry executes a SQL statement with automatic retry on SQLITE_BUSY.
// This is useful for high-concurrency scenarios where WAL mode may still encounter transient locks.
func (db *DB) ExecWithRetry(query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	var err error

	maxRetries := db.config.MaxRetries
	for i := 0; i < maxRetries; i++ {
		result, err = db.Exec(query, args...)
		if err == nil {
			return result, nil
		}

		// Check if it's a busy error
		if isBusyError(err) {
			db.logger.Debug("Database busy, retrying", "attempt", i+1, "max_retries", maxRetries)
			db.backoff(i)
			continue
		}

		// Non-busy error, return immediately
		return nil, err
	}

	return nil, fmt.Errorf("sqliteutil: exec failed after %d retries: %w", maxRetries, err)
}

// QueryWithRetry executes a query with automatic retry on SQLITE_BUSY.
func (db *DB) QueryWithRetry(query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error

	maxRetries := db.config.MaxRetries
	for i := 0; i < maxRetries; i++ {
		rows, err = db.Query(query, args...)
		if err == nil {
			return rows, nil
		}

		if isBusyError(err) {
			db.logger.Debug("Database busy, retrying query", "attempt", i+1, "max_retries", maxRetries)
			db.backoff(i)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("sqliteutil: query failed after %d retries: %w", maxRetries, err)
}

// QueryRowWithRetry executes a query that returns a single row with automatic retry on SQLITE_BUSY.
// Returns the row, which will yield the error on .Scan() or .Err() if all retries fail.
func (db *DB) QueryRowWithRetry(query string, args ...interface{}) *sql.Row {
	maxRetries := db.config.MaxRetries
	var lastRow *sql.Row

	for i := 0; i < maxRetries; i++ {
		row := db.QueryRow(query, args...)
		err := row.Err()
		if err == nil {
			return row
		}

		lastRow = row
		if isBusyError(err) {
			db.logger.Debug("Database busy, retrying query row", "attempt", i+1, "max_retries", maxRetries)
			db.backoff(i)
			continue
		}

		// Non-busy error, return the row
		return row
	}

	// All retries exhausted
	return lastRow
}

// isBusyError checks if an error is a SQLITE_BUSY error.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "database is locked") || contains(errStr, "SQLITE_BUSY")
}

// IsUniqueConstraintError checks if an error is a UNIQUE constraint violation.
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, constants.ErrAlreadyExists) {
		return true
	}
	return contains(err.Error(), "UNIQUE constraint failed")
}

// IsDuplicateColumnError checks if an error indicates a column already exists.
func IsDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, constants.ErrDuplicateColumn) {
		return true
	}
	return contains(err.Error(), "duplicate column name")
}

// backoff implements exponential backoff with jitter to avoid thundering herd.
// Delay = baseDelay * 2^attempt + random jitter (0-25% of delay)
func (db *DB) backoff(attempt int) {
	baseDelay := time.Duration(db.config.RetryBaseDelayMs) * time.Millisecond
	// #nosec G115 -- attempt is bounded by retry logic (max 10 attempts)
	exponentialDelay := baseDelay * (1 << uint(attempt))

	// Add jitter: 0-25% of the delay to spread out retry attempts
	jitter := time.Duration(float64(exponentialDelay) * 0.25 * (float64(time.Now().UnixNano()%1000) / 1000.0))

	time.Sleep(exponentialDelay + jitter)
}

// contains is a simple string contains helper to avoid importing strings.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

// findSubstring checks if substr exists in s.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ExecInTxWithRetry executes a function within a transaction with automatic retry on SQLITE_BUSY.
// The function receives the transaction and should return an error if the transaction should be rolled back.
// If the function returns nil, the transaction is committed.
// This handles SQLITE_BUSY errors at both the Begin() and Commit() stages.
func (db *DB) ExecInTxWithRetry(fn func(tx *sql.Tx) error) error {
	maxRetries := db.config.MaxRetries
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		tx, err := db.Begin()
		if err != nil {
			if isBusyError(err) {
				db.logger.Debug("Database busy, retrying transaction begin", "attempt", i+1, "max_retries", maxRetries)
				db.backoff(i)
				lastErr = err
				continue
			}
			return fmt.Errorf("sqliteutil: begin transaction: %w", err)
		}

		err = fn(tx)
		if err != nil {
			_ = tx.Rollback()
			if isBusyError(err) {
				db.logger.Debug("Database busy during transaction, retrying", "attempt", i+1, "max_retries", maxRetries)
				db.backoff(i)
				lastErr = err
				continue
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			if isBusyError(err) {
				db.logger.Debug("Database busy during commit, retrying", "attempt", i+1, "max_retries", maxRetries)
				db.backoff(i)
				lastErr = err
				continue
			}
			return fmt.Errorf("sqliteutil: commit transaction: %w", err)
		}

		return nil
	}

	return fmt.Errorf("sqliteutil: transaction failed after %d retries: %w", maxRetries, lastErr)
}

// MaterializeRows executes a query and immediately materializes all rows into memory,
// closing the cursor before returning. This prevents long-held cursor locks that can
// block WAL checkpoints and write transactions. The scan function is called for each row.
func MaterializeRows[T any](db *DB, query string, args []interface{}, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := db.QueryWithRetry(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
