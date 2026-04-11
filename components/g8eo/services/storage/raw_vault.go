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

	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
)

// RawVaultConfig holds configuration for the raw vault service
type RawVaultConfig struct {
	// DBPath is the path to the SQLite database file for raw (unscrubbed) data.
	// CRITICAL: This database is COMPLETELY SEPARATE from the scrubbed vault.
	// Default: ./.g8e/raw_vault.db (relative to current working directory)
	DBPath string

	// MaxDBSizeMB is the maximum database size in megabytes before pruning
	// Default: 2048 (2GB) - larger than scrubbed vault since raw data is bigger
	MaxDBSizeMB int64

	// RetentionDays is the maximum age of records in days
	// Default: 30
	RetentionDays int

	// PruneIntervalMinutes is how often to check for pruning
	// Default: 60
	PruneIntervalMinutes int

	// Enabled controls whether raw vault storage is active
	// Default: true
	Enabled bool
}

// DefaultRawVaultConfig returns the default configuration for raw vault.
func DefaultRawVaultConfig() *RawVaultConfig {
	return &RawVaultConfig{
		DBPath:               "./.g8e/raw_vault.db",
		MaxDBSizeMB:          2048,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
}

// RawExecutionRecord represents a stored raw (unscrubbed) command execution
type RawExecutionRecord struct {
	ID               string
	TimestampUTC     time.Time
	Command          string
	ExitCode         *int
	DurationMs       int64
	StdoutCompressed []byte // Raw, unscrubbed stdout (gzip compressed)
	StderrCompressed []byte // Raw, unscrubbed stderr (gzip compressed)
	StdoutHash       string
	StderrHash       string
	StdoutSize       int
	StderrSize       int
	UserID           string
	CaseID           string
	TaskID           string
	InvestigationID  string
	OperatorID       string
}

// RawFileDiffRecord represents a stored raw (unscrubbed) file diff
// LFAA: This is the RAW vault - customer's authoritative data, AI NEVER reads from here
type RawFileDiffRecord struct {
	ID                string
	TimestampUTC      time.Time
	FilePath          string
	Operation         string
	LedgerHashBefore  string
	LedgerHashAfter   string
	DiffStat          string
	DiffCompressed    []byte // Raw, unscrubbed diff content (gzip compressed)
	DiffHash          string
	DiffSize          int // Original size before compression
	OperatorSessionID string
	UserID            string
	CaseID            string
	OperatorID        string
}

// RawVaultService provides local SQLite storage for raw (unscrubbed) command output.
// This is the customer's authoritative data store - full data retained locally.
// The AI NEVER reads from this vault in SCRUBBED mode.
//
// SECURITY: This vault is completely separate from the scrubbed vault.
// Different database file, different service, different code path.
type RawVaultService struct {
	db     *sqliteutil.DB
	config *RawVaultConfig
	logger *slog.Logger
	pruner *sqliteutil.Pruner
}

// NewRawVaultService creates a new raw vault service
func NewRawVaultService(config *RawVaultConfig, logger *slog.Logger) (*RawVaultService, error) {
	if config == nil {
		config = DefaultRawVaultConfig()
	}

	if !config.Enabled {
		logger.Info("Raw vault is disabled")
		return nil, nil
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize raw vault database: %w", err)
	}

	if err := db.RunMigrations(rawVaultMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run raw vault schema migrations: %w", err)
	}

	rv := &RawVaultService{
		config: config,
		logger: logger,
		db:     db,
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	rv.pruner = sqliteutil.NewPruner(db, logger, interval, rawVaultPrune(config))
	rv.pruner.Start()

	rv.logger.Info("Raw vault initialized (customer data store)", "db_path", config.DBPath)
	return rv, nil
}

// rawVaultMigrations defines the schema evolution for the raw vault database.
var rawVaultMigrations = []sqliteutil.Migration{
	{
		Version:     1,
		Description: "Initial schema: raw_execution_log and raw_file_diff_log tables",
		SQL: `
		CREATE TABLE IF NOT EXISTS raw_execution_log (
			id TEXT PRIMARY KEY,
			timestamp_utc TEXT NOT NULL,
			command TEXT NOT NULL,
			exit_code INTEGER,
			duration_ms INTEGER,
			stdout_compressed BLOB,
			stderr_compressed BLOB,
			stdout_hash TEXT,
			stderr_hash TEXT,
			stdout_size INTEGER,
			stderr_size INTEGER,
			user_id TEXT,
			case_id TEXT,
			task_id TEXT,
			investigation_id TEXT,
			operator_id TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_raw_exec_timestamp ON raw_execution_log(timestamp_utc);
		CREATE INDEX IF NOT EXISTS idx_raw_exec_case ON raw_execution_log(case_id);
		CREATE INDEX IF NOT EXISTS idx_raw_exec_investigation ON raw_execution_log(investigation_id);

		CREATE TABLE IF NOT EXISTS raw_file_diff_log (
			id TEXT PRIMARY KEY,
			timestamp_utc TEXT NOT NULL,
			file_path TEXT NOT NULL,
			operation TEXT NOT NULL,
			ledger_hash_before TEXT,
			ledger_hash_after TEXT,
			diff_stat TEXT,
			diff_compressed BLOB,
			diff_hash TEXT,
			diff_size INTEGER DEFAULT 0,
			operator_session_id TEXT,
			user_id TEXT,
			case_id TEXT,
			operator_id TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_raw_diff_timestamp ON raw_file_diff_log(timestamp_utc);
		CREATE INDEX IF NOT EXISTS idx_raw_diff_path ON raw_file_diff_log(file_path);
		CREATE INDEX IF NOT EXISTS idx_raw_diff_session ON raw_file_diff_log(operator_session_id);
		`,
	},
}

// IsEnabled returns whether raw vault storage is active
func (rv *RawVaultService) IsEnabled() bool {
	if rv == nil || rv.db == nil {
		return false
	}
	return rv.config.Enabled
}

// StoreRawExecution stores a raw (unscrubbed) command execution result
func (rv *RawVaultService) StoreRawExecution(record *RawExecutionRecord) error {
	if rv == nil || rv.db == nil {
		return nil // Gracefully handle disabled state
	}

	var stdoutCompressed, stderrCompressed []byte
	var stdoutHash, stderrHash string

	if len(record.StdoutCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.StdoutCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress raw stdout: %w", err)
		}
		stdoutCompressed = compressed
		stdoutHash = sqliteutil.HashBytes(record.StdoutCompressed)
	}

	if len(record.StderrCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.StderrCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress raw stderr: %w", err)
		}
		stderrCompressed = compressed
		stderrHash = sqliteutil.HashBytes(record.StderrCompressed)
	}

	query := `
		INSERT INTO raw_execution_log (
			id, timestamp_utc, command, exit_code, duration_ms,
			stdout_compressed, stderr_compressed,
			stdout_hash, stderr_hash, stdout_size, stderr_size,
			user_id, case_id, task_id, investigation_id, operator_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			command = excluded.command,
			exit_code = excluded.exit_code,
			duration_ms = excluded.duration_ms,
			stdout_compressed = excluded.stdout_compressed,
			stderr_compressed = excluded.stderr_compressed,
			stdout_hash = excluded.stdout_hash,
			stderr_hash = excluded.stderr_hash,
			stdout_size = excluded.stdout_size,
			stderr_size = excluded.stderr_size
	`

	_, err := rv.db.Exec(query,
		record.ID,
		sqliteutil.FormatTimestamp(record.TimestampUTC),
		record.Command,
		record.ExitCode,
		record.DurationMs,
		stdoutCompressed,
		stderrCompressed,
		stdoutHash,
		stderrHash,
		record.StdoutSize,
		record.StderrSize,
		record.UserID,
		record.CaseID,
		record.TaskID,
		record.InvestigationID,
		record.OperatorID,
	)

	if err != nil {
		return fmt.Errorf("failed to store raw execution: %w", err)
	}

	rv.logger.Info("Raw execution stored in raw vault",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)

	return nil
}

// GetRawExecution retrieves a raw execution record by ID
func (rv *RawVaultService) GetRawExecution(executionID string) (*RawExecutionRecord, error) {
	if rv == nil || rv.db == nil {
		return nil, fmt.Errorf("raw vault is not enabled")
	}

	query := `
		SELECT id, timestamp_utc, command, exit_code, duration_ms,
			   stdout_compressed, stderr_compressed,
			   stdout_hash, stderr_hash, stdout_size, stderr_size,
			   user_id, case_id, task_id, investigation_id, operator_id
		FROM raw_execution_log
		WHERE id = ?
	`

	row := rv.db.QueryRow(query, executionID)

	var record RawExecutionRecord
	var timestampStr string

	err := row.Scan(
		&record.ID,
		&timestampStr,
		&record.Command,
		&record.ExitCode,
		&record.DurationMs,
		&record.StdoutCompressed,
		&record.StderrCompressed,
		&record.StdoutHash,
		&record.StderrHash,
		&record.StdoutSize,
		&record.StderrSize,
		&record.UserID,
		&record.CaseID,
		&record.TaskID,
		&record.InvestigationID,
		&record.OperatorID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get raw execution: %w", err)
	}

	record.TimestampUTC, _ = sqliteutil.ParseTimestamp(timestampStr)

	if len(record.StdoutCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(record.StdoutCompressed)
		if err != nil {
			rv.logger.Warn("Failed to decompress raw stdout", "error", err)
		} else {
			record.StdoutCompressed = decompressed
		}
	}

	if len(record.StderrCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(record.StderrCompressed)
		if err != nil {
			rv.logger.Warn("Failed to decompress raw stderr", "error", err)
		} else {
			record.StderrCompressed = decompressed
		}
	}

	return &record, nil
}

// HashString computes SHA256 hash of a string.
func (rv *RawVaultService) HashString(s string) string {
	return sqliteutil.HashString(s)
}

func rawVaultPrune(config *RawVaultConfig) sqliteutil.PruneFunc {
	return func(db *sqliteutil.DB, logger *slog.Logger) {
		cutoff := sqliteutil.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		result, err := db.Exec("DELETE FROM raw_execution_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old raw vault records", "error", err)
			return
		}
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old raw execution records", "rows_deleted", rowsDeleted)
		}

		diffResult, err := db.Exec("DELETE FROM raw_file_diff_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old raw file diff records", "error", err)
		} else {
			diffRowsDeleted, _ := diffResult.RowsAffected()
			if diffRowsDeleted > 0 {
				logger.Info("Pruned old raw file diff records", "rows_deleted", diffRowsDeleted)
			}
		}

		dbSizeBytes, err := db.GetSizeBytes()
		if err != nil {
			logger.Warn("Failed to get database size", "error", err)
		}
		maxSizeBytes := config.MaxDBSizeMB * 1024 * 1024

		if err == nil && dbSizeBytes > maxSizeBytes {
			_, err := db.Exec(`
				DELETE FROM raw_execution_log
				WHERE id IN (
					SELECT id FROM raw_execution_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM raw_execution_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune raw_execution_log for size limit", "error", err)
			}

			_, err = db.Exec(`
				DELETE FROM raw_file_diff_log
				WHERE id IN (
					SELECT id FROM raw_file_diff_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM raw_file_diff_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune raw_file_diff_log for size limit", "error", err)
			}

			logger.Info("Pruned raw vault for size limit", "db_size_mb", dbSizeBytes/(1024*1024))
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", "error", err)
		}
	}
}

// Close closes the raw vault database
func (rv *RawVaultService) Close() error {
	if rv == nil {
		return nil
	}

	if rv.pruner != nil {
		rv.pruner.Stop()
	}

	if rv.db != nil {
		return rv.db.Close()
	}

	return nil
}

// StoreRawFileDiff stores a raw (unscrubbed) file diff in the raw vault
// LFAA: Customer's authoritative data store - AI NEVER reads from here
func (rv *RawVaultService) StoreRawFileDiff(record *RawFileDiffRecord) error {
	if rv == nil || rv.db == nil {
		return nil
	}

	var diffCompressed []byte
	var diffHash string

	if len(record.DiffCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.DiffCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress raw file diff: %w", err)
		}
		diffCompressed = compressed
		diffHash = sqliteutil.HashBytes(record.DiffCompressed)
	}

	query := `
		INSERT INTO raw_file_diff_log (
			id, timestamp_utc, file_path, operation,
			ledger_hash_before, ledger_hash_after, diff_stat,
			diff_compressed, diff_hash, diff_size,
			operator_session_id, user_id, case_id, operator_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			diff_compressed = excluded.diff_compressed,
			diff_hash = excluded.diff_hash,
			diff_size = excluded.diff_size
	`

	_, err := rv.db.Exec(query,
		record.ID,
		sqliteutil.FormatTimestamp(record.TimestampUTC),
		record.FilePath,
		record.Operation,
		record.LedgerHashBefore,
		record.LedgerHashAfter,
		record.DiffStat,
		diffCompressed,
		diffHash,
		record.DiffSize,
		record.OperatorSessionID,
		record.UserID,
		record.CaseID,
		record.OperatorID,
	)

	if err != nil {
		return fmt.Errorf("failed to store raw file diff: %w", err)
	}

	rv.logger.Info("Raw file diff stored in raw vault (customer data)",
		"id", record.ID,
		"file_path", record.FilePath,
		"diff_size", record.DiffSize)

	return nil
}

// GetRawFileDiff retrieves a raw (unscrubbed) file diff by ID
// LFAA: For customer/user access only - NOT for AI
func (rv *RawVaultService) GetRawFileDiff(diffID string) (*RawFileDiffRecord, error) {
	if rv == nil || rv.db == nil {
		return nil, fmt.Errorf("raw vault is not enabled")
	}

	query := `
		SELECT id, timestamp_utc, file_path, operation,
			   ledger_hash_before, ledger_hash_after, diff_stat,
			   diff_compressed, diff_hash, diff_size,
			   operator_session_id, user_id, case_id, operator_id
		FROM raw_file_diff_log
		WHERE id = ?
	`

	row := rv.db.QueryRow(query, diffID)

	var record RawFileDiffRecord
	var timestampStr string

	err := row.Scan(
		&record.ID,
		&timestampStr,
		&record.FilePath,
		&record.Operation,
		&record.LedgerHashBefore,
		&record.LedgerHashAfter,
		&record.DiffStat,
		&record.DiffCompressed,
		&record.DiffHash,
		&record.DiffSize,
		&record.OperatorSessionID,
		&record.UserID,
		&record.CaseID,
		&record.OperatorID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get raw file diff: %w", err)
	}

	record.TimestampUTC, _ = sqliteutil.ParseTimestamp(timestampStr)

	if len(record.DiffCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(record.DiffCompressed)
		if err != nil {
			rv.logger.Warn("Failed to decompress raw file diff", "error", err)
		} else {
			record.DiffCompressed = decompressed
		}
	}

	return &record, nil
}

// GetRawFileDiffsBySession retrieves all raw file diffs for a session
// LFAA: For customer/user access only - NOT for AI
func (rv *RawVaultService) GetRawFileDiffsBySession(operatorSessionID string, limit int) ([]*RawFileDiffRecord, error) {
	if rv == nil || rv.db == nil {
		return nil, fmt.Errorf("raw vault is not enabled")
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, timestamp_utc, file_path, operation,
			   ledger_hash_before, ledger_hash_after, diff_stat,
			   diff_hash, diff_size, operator_session_id, user_id, case_id, operator_id
		FROM raw_file_diff_log
		WHERE operator_session_id = ?
		ORDER BY timestamp_utc DESC
		LIMIT ?
	`

	rows, err := rv.db.Query(query, operatorSessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query raw file diffs: %w", err)
	}
	defer rows.Close()

	var records []*RawFileDiffRecord
	for rows.Next() {
		var record RawFileDiffRecord
		var timestampStr string

		err := rows.Scan(
			&record.ID,
			&timestampStr,
			&record.FilePath,
			&record.Operation,
			&record.LedgerHashBefore,
			&record.LedgerHashAfter,
			&record.DiffStat,
			&record.DiffHash,
			&record.DiffSize,
			&record.OperatorSessionID,
			&record.UserID,
			&record.CaseID,
			&record.OperatorID,
		)
		if err != nil {
			rv.logger.Warn("Failed to scan raw file diff row", "error", err)
			continue
		}

		record.TimestampUTC, _ = sqliteutil.ParseTimestamp(timestampStr)
		records = append(records, &record)
	}

	return records, rows.Err()
}
