// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sqliteutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// DefaultBusyTimeoutMs bounds the uninterruptible driver busy wait. Longer
// waits belong to the retry helpers' context-aware backoff.
const DefaultBusyTimeoutMs = 100

// DBConfig holds common configuration for opening a SQLite database.
type DBConfig struct {
	// Path is the filesystem path to the SQLite database file.
	Path string

	// CacheSizeMB is the SQLite page cache size in megabytes.
	// Default: 64
	CacheSizeMB int

	// BusyTimeoutMs is the SQLite busy timeout in milliseconds. The driver
	// sleeps inside the statement for this long and a context cannot interrupt
	// it, so it must stay short; the retry helpers do the context-aware waiting.
	// Default: DefaultBusyTimeoutMs
	BusyTimeoutMs int

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
		Path:             path,
		CacheSizeMB:      64,
		BusyTimeoutMs:    DefaultBusyTimeoutMs,
		MaxRetries:       10,
		RetryBaseDelayMs: 50,
	}
}

// walCheckpointCommitInterval is how many gated write commits accumulate
// before the background checkpointer is woken. SQLite's automatic checkpoint
// is disabled for databases opened by OpenDB because it runs inside COMMIT on
// the committing connection, which extends the serialized write section.
const walCheckpointCommitInterval = 64

// DB represents a wrapper around *sql.DB that provides common g8eo data operations.
//
// SQLite admits one writer at a time. Every in-process writer that goes through
// ExecWithRetry, ExecInTxWithRetry or ExecInImmediateTxWithRetry first takes
// the pool's writer gate, so writers queue in FIFO order on a channel instead
// of polling SQLite's lock through busy_timeout sleeps and retry backoff, and a
// queued writer holds no pooled connection that readers need. A process opens
// each database file once and shares that DB among its stores, so the gate
// covers every in-process writer. SQLITE_BUSY retries remain for contention
// with other processes.
type DB struct {
	*sql.DB
	logger *slog.Logger
	path   string
	config DBConfig

	writerOnce sync.Once
	writer     chan struct{}

	commits    int64 // guarded by the writer gate
	checkpoint chan struct{}
	stopOnce   sync.Once
	stop       context.CancelFunc
	stopped    chan struct{}
}

// OpenDB opens (or creates) a SQLite database with best-practice settings.
func OpenDB(cfg DBConfig, logger *slog.Logger) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("sqliteutil: database path is required: %w", constants.ErrMissingRequiredField)
	}
	if logger == nil {
		logger = slog.Default()
	}

	// modernc applies _pragma parameters to every physical connection. Setting
	// PRAGMAs through sql.DB.Exec configures only the connection it happens to
	// acquire; later connections would keep FULL synchronization and no busy
	// timeout. The driver does not recognize _synchronous or _busy_timeout.
	params := url.Values{}
	for _, pragma := range []string{
		fmt.Sprintf("busy_timeout(%d)", cfg.BusyTimeoutMs),
		"synchronous(NORMAL)",
		"journal_mode(WAL)",
		"foreign_keys(ON)",
		fmt.Sprintf("cache_size(-%d)", cfg.CacheSizeMB*1024),
		"auto_vacuum(INCREMENTAL)",
		"temp_store(MEMORY)",
		"wal_autocheckpoint(0)",
	} {
		params.Add("_pragma", pragma)
	}
	dsn := fmt.Sprintf("file:%s?%s", cfg.Path, params.Encode())

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteutil: open database %s: %w", cfg.Path, err)
	}

	// WAL mode allows many readers and one writer. Keep every pooled
	// connection idle-resident so a burst does not close and reopen
	// connections and re-run the DSN PRAGMAs on each.
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			return nil, fmt.Errorf("sqliteutil: ping database %s: %w", cfg.Path, errors.Join(err, fmt.Errorf("close database: %w", closeErr)))
		}
		return nil, fmt.Errorf("sqliteutil: ping database %s: %w", cfg.Path, err)
	}

	logger.Info("SQLite database opened", "path", cfg.Path)
	checkpointCtx, stop := context.WithCancel(context.Background())
	db := &DB{
		DB:         sqlDB,
		logger:     logger,
		path:       cfg.Path,
		config:     cfg,
		checkpoint: make(chan struct{}, 1),
		stop:       stop,
		stopped:    make(chan struct{}),
	}
	go db.runCheckpointer(checkpointCtx)
	return db, nil
}

// Close stops the background checkpointer, then closes the pool. SQLite
// checkpoints the WAL when the last connection closes.
func (db *DB) Close() error {
	db.stopOnce.Do(func() {
		if db.stop != nil {
			db.stop()
			<-db.stopped
		}
	})
	return db.DB.Close()
}

// runCheckpointer folds the WAL back into the database off the write path.
// It wakes only when writers signal; PASSIVE never blocks readers or writers.
func (db *DB) runCheckpointer(ctx context.Context) {
	defer close(db.stopped)
	for {
		select {
		case <-ctx.Done():
			return
		case <-db.checkpoint:
			if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); err != nil && ctx.Err() == nil && !IsBusyError(err) {
				db.logger.Warn("SQLite WAL checkpoint failed", "path", db.path, "error", err)
			}
		}
	}
}

// writerGate returns the writer gate, creating it on first use so a
// zero-value DB is usable.
func (db *DB) writerGate() chan struct{} {
	db.writerOnce.Do(func() { db.writer = make(chan struct{}, 1) })
	return db.writer
}

// acquireWriter waits in FIFO order for the writer gate or until ctx ends.
func (db *DB) acquireWriter(ctx context.Context) error {
	select {
	case db.writerGate() <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseWriter releases the writer gate.
func (db *DB) releaseWriter() {
	<-db.writerGate()
}

// TryAcquireWriter attempts to acquire the writer gate without blocking.
// If acquired, it returns a release function and true; otherwise nil and false.
func (db *DB) TryAcquireWriter() (func(), bool) {
	select {
	case db.writerGate() <- struct{}{}:
		return db.releaseWriter, true
	default:
		return nil, false
	}
}

// noteCommit records a successful gated write and wakes the checkpointer once
// per walCheckpointCommitInterval commits. The caller holds the writer gate.
func (db *DB) noteCommit() {
	if db.checkpoint == nil {
		return
	}
	db.commits++
	if db.commits%walCheckpointCommitInterval != 0 {
		return
	}
	select {
	case db.checkpoint <- struct{}{}:
	default:
	}
}

// OpenReadOnlyDB opens an existing SQLite database without creating or
// modifying the database file. SQLite's read-only mode observes the current
// WAL state while preventing schema, migration, pruning, and write side effects.
func OpenReadOnlyDB(cfg DBConfig, logger *slog.Logger) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("sqliteutil: read-only database path is required: %w", constants.ErrMissingRequiredField)
	}
	if logger == nil {
		logger = slog.Default()
	}
	params := url.Values{"mode": {"ro"}, "_pragma": {"query_only(ON)", fmt.Sprintf("busy_timeout(%d)", cfg.BusyTimeoutMs)}}
	dsn := fmt.Sprintf("file:%s?%s", cfg.Path, params.Encode())
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteutil: open read-only database %s: %w", cfg.Path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err := sqlDB.Ping(); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			return nil, fmt.Errorf("sqliteutil: ping read-only database %s: %w", cfg.Path, errors.Join(err, fmt.Errorf("close database: %w", closeErr)))
		}
		return nil, fmt.Errorf("sqliteutil: ping read-only database %s: %w", cfg.Path, err)
	}
	return &DB{DB: sqlDB, logger: logger, path: cfg.Path, config: cfg}, nil
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
func (db *DB) ExecWithRetry(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if err := db.acquireWriter(ctx); err != nil {
		return nil, err
	}
	defer db.releaseWriter()

	var result sql.Result
	var err error

	maxRetries := db.config.MaxRetries
	for i := 0; i < maxRetries; i++ {
		result, err = db.ExecContext(ctx, query, args...)
		if err == nil {
			db.noteCommit()
			return result, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Check if it's a busy error
		if IsBusyError(err) {
			db.logger.Debug("Database busy, retrying", "attempt", i+1, "max_retries", maxRetries)
			if i+1 < maxRetries {
				if err := db.backoff(ctx, i); err != nil {
					return nil, err
				}
			}
			continue
		}

		// Non-busy error, return immediately
		return nil, err
	}

	return nil, fmt.Errorf("sqliteutil: exec failed after %d attempts: %w", maxRetries, errors.Join(constants.ErrSQLiteBusy, err))
}

// QueryWithRetry executes a query with automatic retry on SQLITE_BUSY.
func (db *DB) QueryWithRetry(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error

	maxRetries := db.config.MaxRetries
	for i := 0; i < maxRetries; i++ {
		rows, err = db.QueryContext(ctx, query, args...)
		if err == nil {
			return rows, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if IsBusyError(err) {
			db.logger.Debug("Database busy, retrying query", "attempt", i+1, "max_retries", maxRetries)
			if i+1 < maxRetries {
				if err := db.backoff(ctx, i); err != nil {
					return nil, err
				}
			}
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("sqliteutil: query failed after %d attempts: %w", maxRetries, errors.Join(constants.ErrSQLiteBusy, err))
}

// QueryRowWithRetry executes a query that returns a single row with automatic retry on SQLITE_BUSY.
// Returns the row, which will yield the error on .Scan() or .Err() if all retries fail.
func (db *DB) QueryRowWithRetry(ctx context.Context, query string, args ...interface{}) *sql.Row {
	maxRetries := db.config.MaxRetries
	var lastRow *sql.Row

	for i := 0; i < maxRetries; i++ {
		row := db.QueryRowContext(ctx, query, args...)
		err := row.Err()
		if err == nil {
			return row
		}

		lastRow = row
		if IsBusyError(err) {
			db.logger.Debug("Database busy, retrying query row", "attempt", i+1, "max_retries", maxRetries)
			if i+1 < maxRetries {
				if err := db.backoff(ctx, i); err != nil {
					// QueryRowContext carries the canceled context's error to Scan.
					return db.QueryRowContext(ctx, query, args...)
				}
			}
			continue
		}

		// Non-busy error, return the row
		return row
	}

	// All retries exhausted
	return lastRow
}

// IsBusyError checks if an error is a SQLITE_BUSY error.
func IsBusyError(err error) bool {
	if errors.Is(err, constants.ErrSQLiteBusy) {
		return true
	}
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_BUSY
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

// backoff implements exponential backoff with jitter to avoid thundering herd.
// Delay = baseDelay * 2^attempt + random jitter (0-25% of delay)
func (db *DB) backoff(ctx context.Context, attempt int) error {
	baseDelay := time.Duration(db.config.RetryBaseDelayMs) * time.Millisecond
	// #nosec G115 -- attempt is bounded by retry logic (max 10 attempts)
	exponentialDelay := baseDelay * (1 << uint(attempt))

	// Add jitter: 0-25% of the delay to spread out retry attempts
	jitter := time.Duration(float64(exponentialDelay) * 0.25 * (float64(time.Now().UnixNano()%1000) / 1000.0))

	timer := time.NewTimer(exponentialDelay + jitter)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
func (db *DB) ExecInTxWithRetry(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if err := db.acquireWriter(ctx); err != nil {
		return err
	}
	defer db.releaseWriter()

	maxRetries := db.config.MaxRetries
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			if IsBusyError(err) {
				db.logger.Debug("Database busy, retrying transaction begin", "attempt", i+1, "max_retries", maxRetries)
				if i+1 < maxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				lastErr = err
				continue
			}
			return fmt.Errorf("sqliteutil: begin transaction: %w", err)
		}

		err = fn(tx)
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("sqliteutil: rollback transaction: %w", rollbackErr))
			}
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if IsBusyError(err) {
				db.logger.Debug("Database busy during transaction, retrying", "attempt", i+1, "max_retries", maxRetries)
				if i+1 < maxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				lastErr = err
				continue
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if IsBusyError(err) {
				db.logger.Debug("Database busy during commit, retrying", "attempt", i+1, "max_retries", maxRetries)
				if i+1 < maxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				lastErr = err
				continue
			}
			return fmt.Errorf("sqliteutil: commit transaction: %w", err)
		}

		db.noteCommit()
		return nil
	}

	return fmt.Errorf("sqliteutil: transaction failed after %d attempts: %w", maxRetries, errors.Join(constants.ErrSQLiteBusy, lastErr))
}

// ExecInImmediateTxWithRetry executes a SQLite BEGIN IMMEDIATE transaction on one connection and retries SQLITE_BUSY failures.
// The writer gate is held for the whole call, so fn must not start another
// gated write on db; it must do all of its work through conn.
func (db *DB) ExecInImmediateTxWithRetry(ctx context.Context, fn func(*sql.Conn) error) error {
	if err := db.acquireWriter(ctx); err != nil {
		return err
	}
	defer db.releaseWriter()

	var lastErr error
	for i := 0; i < db.config.MaxRetries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := db.Conn(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if IsBusyError(err) {
				lastErr = err
				if i+1 < db.config.MaxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				continue
			}
			return fmt.Errorf("sqliteutil: acquire transaction connection: %w", err)
		}
		if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			closeErr := conn.Close()
			if closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close transaction connection: %w", closeErr))
			}
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if IsBusyError(err) {
				if i+1 < db.config.MaxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				lastErr = err
				continue
			}
			return fmt.Errorf("sqliteutil: begin immediate transaction: %w", err)
		}
		err = fn(conn)
		if err != nil {
			rollbackErr := func() error {
				_, rollbackErr := conn.ExecContext(context.Background(), "ROLLBACK")
				return rollbackErr
			}()
			closeErr := conn.Close()
			if rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
			}
			if closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close transaction connection: %w", closeErr))
			}
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if IsBusyError(err) {
				if i+1 < db.config.MaxRetries {
					if err := db.backoff(ctx, i); err != nil {
						return err
					}
				}
				lastErr = err
				continue
			}
			return err
		}
		_, err = conn.ExecContext(ctx, "COMMIT")
		if err != nil {
			// Closing a pooled Conn does not end an open transaction. Roll back
			// before returning it to the pool, even when the caller canceled.
			if _, rollbackErr := conn.ExecContext(context.Background(), "ROLLBACK"); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback failed commit: %w", rollbackErr))
			}
		}
		closeErr := conn.Close()
		if closeErr != nil {
			if err != nil {
				err = errors.Join(err, fmt.Errorf("close transaction connection: %w", closeErr))
			} else {
				err = fmt.Errorf("close transaction connection: %w", closeErr)
			}
		}
		if err == nil {
			db.noteCommit()
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), err)
		}
		if IsBusyError(err) {
			if i+1 < db.config.MaxRetries {
				if err := db.backoff(ctx, i); err != nil {
					return err
				}
			}
			lastErr = err
			continue
		}
		return fmt.Errorf("sqliteutil: commit immediate transaction: %w", err)
	}
	return fmt.Errorf("sqliteutil: immediate transaction failed after %d attempts: %w", db.config.MaxRetries, errors.Join(constants.ErrSQLiteBusy, lastErr))
}

// MaterializeRows executes a query and immediately materializes all rows into memory,
// closing the cursor before returning. This prevents long-held cursor locks that can
// block WAL checkpoints and write transactions. The scan function is called for each row.
func MaterializeRows[T any](ctx context.Context, db *DB, query string, args []interface{}, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := db.QueryWithRetry(ctx, query, args...)
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
