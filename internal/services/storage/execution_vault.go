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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/interfaces"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// ExecutionVaultConfig holds configuration for the execution vault service.
type ExecutionVaultConfig struct {
	DBPath               string
	MaxDBSizeMB          int64
	RetentionDays        int
	PruneIntervalMinutes int
	Enabled              bool
}

// DefaultExecutionVaultConfig returns the default configuration.
func DefaultExecutionVaultConfig() *ExecutionVaultConfig {
	return &ExecutionVaultConfig{
		DBPath:               ".g8e/execution_vault.db",
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
}

// ExecutionVaultService provides SQLite storage for command execution results and file diffs.
// This is the execution vault - all data encrypted at rest when configured.
type ExecutionVaultService struct {
	db     *sqliteutil.DB
	config *ExecutionVaultConfig
	logger *slog.Logger
	pruner *sqliteutil.Pruner
	vault  *vault.Vault

	wg sync.WaitGroup
}

// Ensure ExecutionVaultService implements interfaces.ExecutionVault.
var _ interfaces.ExecutionVault = (*ExecutionVaultService)(nil)

// NewExecutionVaultService creates a new execution vault service.
func NewExecutionVaultService(config *ExecutionVaultConfig, logger *slog.Logger, v *vault.Vault) (*ExecutionVaultService, error) {
	if config == nil {
		config = DefaultExecutionVaultConfig()
	}

	if !config.Enabled {
		logger.Info("Execution vault is disabled")
		return nil, nil
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if _, err := db.Exec(executionVaultSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	ev := &ExecutionVaultService{
		config: config,
		logger: logger,
		db:     db,
		vault:  v,
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	ev.pruner = sqliteutil.NewPruner(db, logger, interval, executionVaultPrune(config))
	ev.pruner.Start()

	encryptionEnabled := ev.vault != nil && ev.vault.IsUnlocked()
	ev.logger.Info("Execution vault initialized",
		"db_path", config.DBPath,
		"encryption_enabled", encryptionEnabled)
	return ev, nil
}

// executionVaultSchema defines the initial schema for the execution vault database.
const executionVaultSchema = `
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

CREATE INDEX IF NOT EXISTS idx_execution_timestamp ON execution_log(timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_execution_case ON execution_log(case_id);
CREATE INDEX IF NOT EXISTS idx_execution_task ON execution_log(task_id);
CREATE INDEX IF NOT EXISTS idx_file_diff_timestamp ON file_diff_log(timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_file_diff_path ON file_diff_log(file_path);
CREATE INDEX IF NOT EXISTS idx_file_diff_session ON file_diff_log(operator_session_id);
`

// StoreExecution stores a command execution result locally.
// Content is encrypted at rest if an encryption vault is configured.
func (ev *ExecutionVaultService) StoreExecution(record *models.ExecutionRecord) error {
	if ev == nil || ev.db == nil {
		return nil
	}
	ev.wg.Add(1)
	defer ev.wg.Done()
	var stdoutCompressed, stderrCompressed []byte
	var stdoutHash, stderrHash string

	if len(record.StdoutCompressed) > 0 {
		stdoutBytes, err := ev.encryptContent(string(record.StdoutCompressed))
		if err != nil {
			return fmt.Errorf("failed to encrypt stdout: %w", err)
		}
		compressed, err := sqliteutil.Compress(stdoutBytes)
		if err != nil {
			return fmt.Errorf("failed to compress stdout: %w", err)
		}
		stdoutCompressed = compressed
		stdoutHash = sqliteutil.HashBytes(record.StdoutCompressed)
	}

	if len(record.StderrCompressed) > 0 {
		stderrBytes, err := ev.encryptContent(string(record.StderrCompressed))
		if err != nil {
			return fmt.Errorf("failed to encrypt stderr: %w", err)
		}
		compressed, err := sqliteutil.Compress(stderrBytes)
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

	_, err := ev.db.ExecWithRetry(query,
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

	ev.logger.Info("Execution stored locally",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize,
		"compressed_size", len(stdoutCompressed)+len(stderrCompressed))

	return nil
}

// GetExecution retrieves a stored execution by ID.
func (ev *ExecutionVaultService) GetExecution(executionID string) (*models.ExecutionRecord, error) {
	if ev == nil || ev.db == nil {
		return nil, fmt.Errorf("execution vault is disabled")
	}

	query := `
	SELECT id, timestamp_utc, command, exit_code, duration_ms,
		stdout_compressed, stderr_compressed, stdout_hash, stderr_hash,
		stdout_size, stderr_size, user_id, case_id, task_id, investigation_id, operator_id
	FROM execution_log WHERE id = ?
	`

	row := ev.db.QueryRowWithRetry(query, executionID)

	var record models.ExecutionRecord
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
		ev.logger.Warn("Failed to parse execution timestamp", "raw", timestampStr, string(constants.ConnectionStateError), err)
	}

	if len(stdoutCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(stdoutCompressed)
		if err != nil {
			ev.logger.Warn("Failed to decompress stdout", string(constants.ConnectionStateError), err)
		} else {
			decrypted, err := ev.decryptContent(decompressed)
			if err != nil {
				ev.logger.Warn("Failed to decrypt stdout", string(constants.ConnectionStateError), err)
			} else {
				record.StdoutCompressed = []byte(decrypted)
			}
		}
	}

	if len(stderrCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(stderrCompressed)
		if err != nil {
			ev.logger.Warn("Failed to decompress stderr", string(constants.ConnectionStateError), err)
		} else {
			decrypted, err := ev.decryptContent(decompressed)
			if err != nil {
				ev.logger.Warn("Failed to decrypt stderr", string(constants.ConnectionStateError), err)
			} else {
				record.StderrCompressed = []byte(decrypted)
			}
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

// StoreFileDiff stores a file diff in the execution vault.
// Content is encrypted at rest if an encryption vault is configured.
func (ev *ExecutionVaultService) StoreFileDiff(record *models.FileDiffRecord) error {
	if ev == nil || ev.db == nil {
		return nil
	}
	ev.wg.Add(1)
	defer ev.wg.Done()

	var diffCompressed []byte
	var diffHash string

	if len(record.DiffCompressed) > 0 {
		diffBytes, err := ev.encryptContent(string(record.DiffCompressed))
		if err != nil {
			return fmt.Errorf("failed to encrypt file diff: %w", err)
		}
		compressed, err := sqliteutil.Compress(diffBytes)
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

	_, err := ev.db.ExecWithRetry(query,
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

	ev.logger.Info("Scrubbed file diff stored",
		"id", record.ID,
		"file_path", record.FilePath,
		"diff_size", record.DiffSize)

	return nil
}

// GetFileDiff retrieves a file diff by ID.
func (ev *ExecutionVaultService) GetFileDiff(diffID string) (*models.FileDiffRecord, error) {
	if ev == nil || ev.db == nil {
		return nil, fmt.Errorf("execution vault is disabled")
	}

	query := `
	SELECT id, timestamp_utc, file_path, operation,
		ledger_hash_before, ledger_hash_after, diff_stat,
		diff_compressed, diff_hash, diff_size,
		operator_session_id, user_id, case_id, operator_id
	FROM file_diff_log WHERE id = ?
	`

	row := ev.db.QueryRowWithRetry(query, diffID)

	var record models.FileDiffRecord
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
		ev.logger.Warn("Failed to parse file diff timestamp", "raw", timestampStr, string(constants.ConnectionStateError), parseErr)
	}

	if len(diffCompressed) > 0 {
		decompressed, err := sqliteutil.Decompress(diffCompressed)
		if err != nil {
			ev.logger.Warn("Failed to decompress file diff", string(constants.ConnectionStateError), err)
		} else {
			decrypted, err := ev.decryptContent(decompressed)
			if err != nil {
				ev.logger.Warn("Failed to decrypt file diff", string(constants.ConnectionStateError), err)
			} else {
				record.DiffCompressed = []byte(decrypted)
			}
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

// GetFileDiffsBySession retrieves all file diffs for a session from the execution vault.
func (ev *ExecutionVaultService) GetFileDiffsBySession(operatorSessionID string, limit int) ([]*models.FileDiffRecord, error) {
	if ev == nil || ev.db == nil {
		return nil, fmt.Errorf("execution vault is disabled")
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

	type fileDiffRow struct {
		record       models.FileDiffRecord
		hashBefore   sql.NullString
		hashAfter    sql.NullString
		webSessID    sql.NullString
		userID       sql.NullString
		caseID       sql.NullString
		operatorID   sql.NullString
		timestampStr string
	}

	rows, err := sqliteutil.MaterializeRows(ev.db, query, []interface{}{operatorSessionID, limit}, func(r *sql.Rows) (fileDiffRow, error) {
		var row fileDiffRow
		err := r.Scan(
			&row.record.ID,
			&row.timestampStr,
			&row.record.FilePath,
			&row.record.Operation,
			&row.hashBefore,
			&row.hashAfter,
			&row.record.DiffStat,
			&row.record.DiffHash,
			&row.record.DiffSize,
			&row.webSessID,
			&row.userID,
			&row.caseID,
			&row.operatorID,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query file diffs: %w", err)
	}

	var records []*models.FileDiffRecord
	for _, row := range rows {
		ts, tsErr := sqliteutil.ParseTimestamp(row.timestampStr)
		if tsErr != nil {
			ev.logger.Warn("Failed to parse file diff timestamp", "raw", row.timestampStr, string(constants.ConnectionStateError), tsErr)
		}
		row.record.TimestampUTC = ts

		if row.hashBefore.Valid {
			row.record.LedgerHashBefore = row.hashBefore.String
		}
		if row.hashAfter.Valid {
			row.record.LedgerHashAfter = row.hashAfter.String
		}
		if row.webSessID.Valid {
			row.record.OperatorSessionID = row.webSessID.String
		}
		if row.userID.Valid {
			row.record.UserID = row.userID.String
		}
		if row.caseID.Valid {
			row.record.CaseID = row.caseID.String
		}
		if row.operatorID.Valid {
			row.record.OperatorID = row.operatorID.String
		}

		records = append(records, &row.record)
	}

	return records, nil
}

// executionVaultPrune returns a PruneFunc for retention and size-based pruning.
func executionVaultPrune(config *ExecutionVaultConfig) sqliteutil.PruneFunc {
	return func(db *sqliteutil.DB, logger *slog.Logger) {
		cutoff := sqliteutil.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		result, err := db.ExecWithRetry("DELETE FROM execution_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old records", string(constants.ConnectionStateError), err)
			return
		}
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old execution records", "rows_deleted", rowsDeleted)
		}

		diffResult, err := db.ExecWithRetry("DELETE FROM file_diff_log WHERE timestamp_utc < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old file diff records", string(constants.ConnectionStateError), err)
		} else {
			diffRowsDeleted, _ := diffResult.RowsAffected()
			if diffRowsDeleted > 0 {
				logger.Info("Pruned old file diff records (execution vault)", "rows_deleted", diffRowsDeleted)
			}
		}

		dbSizeBytes, err := db.GetSizeBytes()
		if err != nil {
			logger.Warn("Failed to get database size", string(constants.ConnectionStateError), err)
		}
		maxSizeBytes := config.MaxDBSizeMB * 1024 * 1024

		if err == nil && dbSizeBytes > maxSizeBytes {
			_, err := db.ExecWithRetry(`
				DELETE FROM execution_log
				WHERE id IN (
					SELECT id FROM execution_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM execution_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune execution_log for size limit", string(constants.ConnectionStateError), err)
			}

			_, err = db.ExecWithRetry(`
				DELETE FROM file_diff_log
				WHERE id IN (
					SELECT id FROM file_diff_log
					ORDER BY timestamp_utc ASC
					LIMIT (SELECT COUNT(*) / 10 FROM file_diff_log)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune file_diff_log for size limit", string(constants.ConnectionStateError), err)
			}

			logger.Info("Pruned for size limit", "db_size_mb", dbSizeBytes/(1024*1024))
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", string(constants.ConnectionStateError), err)
		}
	}
}

// Close shuts down the execution vault service.
func (ev *ExecutionVaultService) Close() error {
	if ev == nil {
		return nil
	}

	if ev.pruner != nil {
		ev.pruner.Stop()
	}

	if ev.db != nil {
		return ev.db.Close()
	}

	return nil
}

// IsEnabled returns whether the execution vault is enabled.
func (ev *ExecutionVaultService) IsEnabled() bool {
	return ev != nil && ev.db != nil
}

// IsEncryptionEnabled returns whether content encryption is enabled
func (ev *ExecutionVaultService) IsEncryptionEnabled() bool {
	return ev != nil && ev.vault != nil && ev.vault.IsUnlocked()
}

// encryptContent encrypts content if encryption is enabled, otherwise returns original
func (ev *ExecutionVaultService) encryptContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}

	if ev.vault != nil && ev.vault.IsUnlocked() {
		encrypted, err := ev.vault.Encrypt([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt content: %w", err)
		}
		return encrypted, nil
	}

	return []byte(content), nil
}

// decryptContent decrypts content if encryption is enabled, otherwise returns original
func (ev *ExecutionVaultService) decryptContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if ev.vault != nil && ev.vault.IsUnlocked() {
		decrypted, err := ev.vault.Decrypt(data)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt content: %w", err)
		}
		return string(decrypted), nil
	}

	return string(data), nil
}

// Wait blocks until all background workers and writes have finished.
func (ev *ExecutionVaultService) Wait() {
	ev.wg.Wait()
}
