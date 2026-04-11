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

	"github.com/g8e-ai/g8e/components/vsa/services/sqliteutil"
)

// LocalStoreConfig holds configuration for the local storage service.
type LocalStoreConfig struct {
	DBPath               string
	MaxDBSizeMB          int64
	RetentionDays        int
	PruneIntervalMinutes int
	Enabled              bool
}

// DefaultLocalStoreConfig returns the default configuration.
func DefaultLocalStoreConfig() *LocalStoreConfig {
	return &LocalStoreConfig{
		DBPath:               "./.g8e/local_state.db",
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
}

// ExecutionRecord represents a stored command execution.
type ExecutionRecord struct {
	ID               string
	TimestampUTC     time.Time
	Command          string
	ExitCode         *int
	DurationMs       int64
	StdoutCompressed []byte
	StderrCompressed []byte
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

// FileDiffRecord represents a stored file diff (Sentinel-scrubbed).
type FileDiffRecord struct {
	ID                string
	TimestampUTC      time.Time
	FilePath          string
	Operation         string
	LedgerHashBefore  string
	LedgerHashAfter   string
	DiffStat          string
	DiffCompressed    []byte
	DiffHash          string
	DiffSize          int
	OperatorSessionID string
	UserID            string
	CaseID            string
	OperatorID        string
}

// LocalStoreService provides local SQLite storage for command execution results.
type LocalStoreService struct {
	db     *sqliteutil.DB
	config *LocalStoreConfig
	logger *slog.Logger
	pruner *sqliteutil.Pruner
}

// NewLocalStoreService creates a new local storage service.
func NewLocalStoreService(config *LocalStoreConfig, logger *slog.Logger) (*LocalStoreService, error) {
	if config == nil {
		config = DefaultLocalStoreConfig()
	}

	if !config.Enabled {
		logger.Info("Local storage is disabled")
		return nil, nil
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := db.RunMigrations(localStoreMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run schema migrations: %w", err)
	}

	ls := &LocalStoreService{
		config: config,
		logger: logger,
		db:     db,
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	ls.pruner = sqliteutil.NewPruner(db, logger, interval, localStorePrune(config))
	ls.pruner.Start()

	ls.logger.Info("Local storage initialized", "db_path", config.DBPath)
	return ls, nil
}

var localStoreMigrations = []sqliteutil.Migration{
	{
		Version:     1,
		Description: "Initial schema: execution_log and file_diff_log tables",
		SQL: `
		CREATE TABLE IF NOT EXISTS execution_log (
			id TEXT PRIMARY KEY,
			timestamp_utc TEXT NOT NULL,
			command TEXT NOT NULL,
			exit_code INTEGER,
			duration_ms INTEGER,
			stdout_compressed BLOB,
			stderr_compressed BLOB,
			stdout_hash TEXT,
			stderr_hash TEXT,
			stdout_size INTEGER DEFAULT 0,
			stderr_size INTEGER DEFAULT 0,
			user_id TEXT,
			case_id TEXT,
			task_id TEXT,
			investigation_id TEXT,
			operator_id TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_execution_timestamp ON execution_log(timestamp_utc);
		CREATE INDEX IF NOT EXISTS idx_execution_case ON execution_log(case_id);
		CREATE INDEX IF NOT EXISTS idx_execution_task ON execution_log(task_id);

		CREATE TABLE IF NOT EXISTS file_diff_log (
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

		CREATE INDEX IF NOT EXISTS idx_file_diff_timestamp ON file_diff_log(timestamp_utc);
		CREATE INDEX IF NOT EXISTS idx_file_diff_path ON file_diff_log(file_path);
		CREATE INDEX IF NOT EXISTS idx_file_diff_session ON file_diff_log(operator_session_id);
		`,
	},
}

// StoreExecution stores a command execution result locally.
func (ls *LocalStoreService) StoreExecution(record *ExecutionRecord) error {
	if ls == nil || ls.db == nil {
		return nil
	}
	var stdoutCompressed, stderrCompressed []byte
	var stdoutHash, stderrHash string

	if len(record.StdoutCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.StdoutCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress stdout: %w", err)
		}
		stdoutCompressed = compressed
		stdoutHash = sqliteutil.HashBytes(record.StdoutCompressed)
	}

	if len(record.StderrCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.StderrCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress stderr: %w", err)
		}
		stderrCompressed = compressed
		stderrHash = sqliteutil.HashBytes(record.StderrCompressed)
	}

	query := `
	INSERT INTO execution_log (
		id, timestamp_utc, command, exit_code, duration_ms,
		stdout_compressed, stderr_compressed, stdout_hash, stderr_hash,
		stdout_size, stderr_size, user_id, case_id, task_id, investigation_id, operator_id
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

	_, err := ls.db.Exec(query,
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
		return fmt.Errorf("failed to store execution: %w", err)
	}

	ls.logger.Info("Execution stored locally",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize,
		"compressed_size", len(stdoutCompressed)+len(stderrCompressed))

	return nil
}

// GetExecution retrieves a stored execution by ID.
func (ls *LocalStoreService) GetExecution(executionID string) (*ExecutionRecord, error) {
	if ls == nil || ls.db == nil {
		return nil, fmt.Errorf("local storage is disabled")
	}

	query := `
	SELECT id, timestamp_utc, command, exit_code, duration_ms,
		stdout_compressed, stderr_compressed, stdout_hash, stderr_hash,
		stdout_size, stderr_size, user_id, case_id, task_id, investigation_id, operator_id
	FROM execution_log WHERE id = ?
	`

	row := ls.db.QueryRow(query, executionID)

	var record ExecutionRecord
	var stdoutCompressed, stderrCompressed []byte
	var timestampStr string
	var taskID, investigationID, operatorID sql.NullString

	err := row.Scan(
		&record.ID,
		&timestampStr,
		&record.Command,
		&record.ExitCode,
		&record.DurationMs,
		&stdoutCompressed,
		&stderrCompressed,
		&record.StdoutHash,
		&record.StderrHash,
		&record.StdoutSize,
		&record.StderrSize,
		&record.UserID,
		&record.CaseID,
		&taskID,
		&investigationID,
		&operatorID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query execution: %w", err)
	}

	record.TimestampUTC, err = sqliteutil.ParseTimestamp(timestampStr)
	if err != nil {
		ls.logger.Warn("Failed to parse execution timestamp", "raw", timestampStr, "error", err)
	}

	if len(stdoutCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(stdoutCompressed)
		if err != nil {
			ls.logger.Warn("Failed to decompress stdout", "error", err)
		} else {
			record.StdoutCompressed = decompressed
		}
	}

	if len(stderrCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(stderrCompressed)
		if err != nil {
			ls.logger.Warn("Failed to decompress stderr", "error", err)
		} else {
			record.StderrCompressed = decompressed
		}
	}

	if taskID.Valid {
		record.TaskID = taskID.String
	}
	if investigationID.Valid {
		record.InvestigationID = investigationID.String
	}
	if operatorID.Valid {
		record.OperatorID = operatorID.String
	}

	return &record, nil
}

// HashString computes SHA256 hash of a string.
func (ls *LocalStoreService) HashString(data string) string {
	return sqliteutil.HashString(data)
}

// localStorePrune returns a PruneFunc for retention and size-based pruning.
func localStorePrune(config *LocalStoreConfig) sqliteutil.PruneFunc {
	return func(db *sqliteutil.DB, logger *slog.Logger) {
		cutoff := sqliteutil.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		result, err := db.Exec("DELETE FROM execution_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old records", "error", err)
			return
		}
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old execution records", "rows_deleted", rowsDeleted)
		}

		diffResult, err := db.Exec("DELETE FROM file_diff_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old file diff records", "error", err)
		} else {
			diffRowsDeleted, _ := diffResult.RowsAffected()
			if diffRowsDeleted > 0 {
				logger.Info("Pruned old file diff records (scrubbed vault)", "rows_deleted", diffRowsDeleted)
			}
		}

		dbSizeBytes, err := db.GetSizeBytes()
		if err != nil {
			logger.Warn("Failed to get database size", "error", err)
		}
		maxSizeBytes := config.MaxDBSizeMB * 1024 * 1024

		if err == nil && dbSizeBytes > maxSizeBytes {
			_, err := db.Exec(`
				DELETE FROM execution_log
				WHERE id IN (
					SELECT id FROM execution_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM execution_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune execution_log for size limit", "error", err)
			}

			_, err = db.Exec(`
				DELETE FROM file_diff_log
				WHERE id IN (
					SELECT id FROM file_diff_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM file_diff_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune file_diff_log for size limit", "error", err)
			}

			logger.Info("Pruned for size limit", "db_size_mb", dbSizeBytes/(1024*1024))
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", "error", err)
		}
	}
}

// Close shuts down the local storage service.
func (ls *LocalStoreService) Close() error {
	if ls == nil {
		return nil
	}

	if ls.pruner != nil {
		ls.pruner.Stop()
	}

	if ls.db != nil {
		return ls.db.Close()
	}

	return nil
}

// IsEnabled returns whether local storage is enabled.
func (ls *LocalStoreService) IsEnabled() bool {
	return ls != nil && ls.db != nil
}

// StoreFileDiff stores a Sentinel-scrubbed file diff in the scrubbed vault.
func (ls *LocalStoreService) StoreFileDiff(record *FileDiffRecord) error {
	if ls == nil || ls.db == nil {
		return nil
	}

	var diffCompressed []byte
	var diffHash string

	if len(record.DiffCompressed) > 0 {
		compressed, err := sqliteutil.Compress(record.DiffCompressed)
		if err != nil {
			return fmt.Errorf("failed to compress file diff: %w", err)
		}
		diffCompressed = compressed
		diffHash = sqliteutil.HashBytes(record.DiffCompressed)
	}

	query := `
	INSERT INTO file_diff_log (
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

	_, err := ls.db.Exec(query,
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
		return fmt.Errorf("failed to store file diff: %w", err)
	}

	ls.logger.Info("Scrubbed file diff stored",
		"id", record.ID,
		"file_path", record.FilePath,
		"diff_size", record.DiffSize)

	return nil
}

// GetFileDiff retrieves a Sentinel-scrubbed file diff by ID.
func (ls *LocalStoreService) GetFileDiff(diffID string) (*FileDiffRecord, error) {
	if ls == nil || ls.db == nil {
		return nil, fmt.Errorf("local storage is disabled")
	}

	query := `
	SELECT id, timestamp_utc, file_path, operation,
		ledger_hash_before, ledger_hash_after, diff_stat,
		diff_compressed, diff_hash, diff_size,
		operator_session_id, user_id, case_id, operator_id
	FROM file_diff_log WHERE id = ?
	`

	row := ls.db.QueryRow(query, diffID)

	var record FileDiffRecord
	var diffCompressed []byte
	var timestampStr string
	var hashBefore, hashAfter, operatorSessionID, userID, caseID, operatorID sql.NullString

	err := row.Scan(
		&record.ID,
		&timestampStr,
		&record.FilePath,
		&record.Operation,
		&hashBefore,
		&hashAfter,
		&record.DiffStat,
		&diffCompressed,
		&record.DiffHash,
		&record.DiffSize,
		&operatorSessionID,
		&userID,
		&caseID,
		&operatorID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query file diff: %w", err)
	}

	var parseErr error
	record.TimestampUTC, parseErr = sqliteutil.ParseTimestamp(timestampStr)
	if parseErr != nil {
		ls.logger.Warn("Failed to parse file diff timestamp", "raw", timestampStr, "error", parseErr)
	}

	if len(diffCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(diffCompressed)
		if err != nil {
			ls.logger.Warn("Failed to decompress file diff", "error", err)
		} else {
			record.DiffCompressed = decompressed
		}
	}

	if hashBefore.Valid {
		record.LedgerHashBefore = hashBefore.String
	}
	if hashAfter.Valid {
		record.LedgerHashAfter = hashAfter.String
	}
	if operatorSessionID.Valid {
		record.OperatorSessionID = operatorSessionID.String
	}
	if userID.Valid {
		record.UserID = userID.String
	}
	if caseID.Valid {
		record.CaseID = caseID.String
	}
	if operatorID.Valid {
		record.OperatorID = operatorID.String
	}

	return &record, nil
}

// GetFileDiffsBySession retrieves all file diffs for a session from the scrubbed vault.
func (ls *LocalStoreService) GetFileDiffsBySession(operatorSessionID string, limit int) ([]*FileDiffRecord, error) {
	if ls == nil || ls.db == nil {
		return nil, fmt.Errorf("local storage is disabled")
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT id, timestamp_utc, file_path, operation,
		ledger_hash_before, ledger_hash_after, diff_stat,
		diff_hash, diff_size, operator_session_id, user_id, case_id, operator_id
	FROM file_diff_log
	WHERE operator_session_id = ?
	ORDER BY timestamp_utc DESC
	LIMIT ?
	`

	rows, err := ls.db.Query(query, operatorSessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query file diffs: %w", err)
	}
	defer rows.Close()

	var records []*FileDiffRecord
	for rows.Next() {
		var record FileDiffRecord
		var hashBefore, hashAfter, webSessID, userID, caseID, operatorID sql.NullString

		var timestampStr string
		err := rows.Scan(
			&record.ID,
			&timestampStr,
			&record.FilePath,
			&record.Operation,
			&hashBefore,
			&hashAfter,
			&record.DiffStat,
			&record.DiffHash,
			&record.DiffSize,
			&webSessID,
			&userID,
			&caseID,
			&operatorID,
		)
		if err != nil {
			ls.logger.Warn("Failed to scan file diff row", "error", err)
			continue
		}

		ts, tsErr := sqliteutil.ParseTimestamp(timestampStr)
		if tsErr != nil {
			ls.logger.Warn("Failed to parse file diff timestamp", "raw", timestampStr, "error", tsErr)
		}
		record.TimestampUTC = ts

		if hashBefore.Valid {
			record.LedgerHashBefore = hashBefore.String
		}
		if hashAfter.Valid {
			record.LedgerHashAfter = hashAfter.String
		}
		if webSessID.Valid {
			record.OperatorSessionID = webSessID.String
		}
		if userID.Valid {
			record.UserID = userID.String
		}
		if caseID.Valid {
			record.CaseID = caseID.String
		}
		if operatorID.Valid {
			record.OperatorID = operatorID.String
		}

		records = append(records, &record)
	}

	return records, rows.Err()
}
