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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// AuditStoreConfig holds configuration for the SQL audit store
type AuditStoreConfig struct {
	DataDir                   string
	DBPath                    string
	MaxDBSizeMB               int64
	RetentionDays             int
	PruneIntervalMinutes      int
	Enabled                   bool
	OutputTruncationThreshold int
	HeadTailSize              int
	// EncryptionVault is the required vault.Vault for encrypting sensitive content fields.
	// content_text, command_stdout, and command_stderr are encrypted at rest.
	EncryptionVault *vault.Vault
}

// DefaultAuditStoreConfig returns the default configuration for the audit store.
// Note: DataDir should be set by the caller based on the actual work directory.
func DefaultAuditStoreConfig() *AuditStoreConfig {
	return &AuditStoreConfig{
		DataDir:                   ".g8e/data",
		DBPath:                    "g8e.db",
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
}

var (
	ErrAuditEventNil       = errors.New("AUDIT_EVENT_INVALID: event required")
	ErrAuditSessionMissing = errors.New("AUDIT_SESSION_MISSING: operator_session_id required")
	ErrAuditSessionUnknown = errors.New("AUDIT_SESSION_UNKNOWN: operator_session_id must reference a pre-created session")
)

// FileMutationOperation represents the type of file operation
type FileMutationOperation string

const (
	FileMutationWrite  FileMutationOperation = "WRITE"
	FileMutationDelete FileMutationOperation = "DELETE"
	FileMutationCreate FileMutationOperation = "CREATE"
)

// OperatorSession represents a chat session in the audit log
type OperatorSession struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	UserIdentity string
}

// Event represents an event in the audit log (append-only)
type Event struct {
	ID                  int64
	OperatorSessionID   string
	Timestamp           time.Time
	Type                constants.EventType
	ContentText         string
	CommandRaw          string
	CommandExitCode     *int
	CommandStdout       string
	CommandStderr       string
	ExecutionDurationMs int64
	StoredLocally       bool
	StdoutTruncated     bool
	StderrTruncated     bool
}

// FileMutationLog represents a file mutation record linked to an event
type FileMutationLog struct {
	ID               int64
	EventID          int64
	Filepath         string
	Operation        FileMutationOperation
	LedgerHashBefore string
	LedgerHashAfter  string
	DiffStat         string
}

// SQLAuditStore provides pure SQL audit data storage
type SQLAuditStore struct {
	db              *sqliteutil.DB
	config          *AuditStoreConfig
	logger          *slog.Logger
	encryptionVault *vault.Vault
	pruner          *sqliteutil.Pruner
	closeOnce       sync.Once

	muWrites sync.WaitGroup
}

// NewSQLAuditStore creates a new SQL audit store
func NewSQLAuditStore(config *AuditStoreConfig, logger *slog.Logger) (*SQLAuditStore, error) {
	if config == nil {
		config = DefaultAuditStoreConfig()
	}

	if !config.Enabled {
		logger.Info("Audit store is disabled")
		return nil, nil
	}

	if config.EncryptionVault == nil {
		return nil, fmt.Errorf("EncryptionVault is required for audit store")
	}

	ass := &SQLAuditStore{
		config:          config,
		logger:          logger,
		encryptionVault: config.EncryptionVault,
	}

	if err := ass.bootstrap(); err != nil {
		return nil, fmt.Errorf("audit store bootstrap failed: %w", err)
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	ass.pruner = sqliteutil.NewPruner(ass.db, logger, interval, auditStorePrune(config))
	ass.pruner.Start()

	encryptionEnabled := ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked()
	ass.logger.Info("Audit store initialized",
		"data_dir", config.DataDir,
		"db_path", filepath.Join(config.DataDir, config.DBPath),
		"encryption_enabled", encryptionEnabled)

	return ass, nil
}

// bootstrap initializes the audit store (directory structure, database)
func (ass *SQLAuditStore) bootstrap() error {
	ass.logger.Info("Bootstrapping audit store", "data_dir", ass.config.DataDir)

	if err := ass.createDirectoryStructure(); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	if err := ass.verifyWritePermissions(); err != nil {
		return fmt.Errorf("FATAL: storage not writable (zero tolerance for data loss risk): %w", err)
	}

	if err := ass.initDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	ass.logger.Info("Audit store bootstrap completed successfully")
	return nil
}

// createDirectoryStructure creates the audit store directory structure
func (ass *SQLAuditStore) createDirectoryStructure() error {
	dirs := []string{
		ass.config.DataDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	ass.logger.Info("Audit store directory structure ensured",
		"data_dir", ass.config.DataDir)

	return nil
}

// verifyWritePermissions ensures the data directory is writable
func (ass *SQLAuditStore) verifyWritePermissions() error {
	testFile := filepath.Join(ass.config.DataDir, ".write_test")

	if err := os.WriteFile(testFile, []byte("write_test"), 0600); err != nil {
		return fmt.Errorf("cannot write to %s: %w", ass.config.DataDir, err)
	}

	if err := os.Remove(testFile); err != nil {
		ass.logger.Warn("Failed to remove write test file", "path", testFile, string(constants.ConnectionStateError), err)
	}

	ass.logger.Info("Write permissions verified", "path", ass.config.DataDir)
	return nil
}

// initDatabase creates the database and schema
func (ass *SQLAuditStore) initDatabase() error {
	dbPath := filepath.Join(ass.config.DataDir, ass.config.DBPath)

	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, ass.logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec(auditStoreSchema); err != nil {
		db.Close()
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	ass.db = db

	ass.logger.Info("Database schema initialized")
	return nil
}

// auditStoreSchema defines the initial schema for the audit store database.
const auditStoreSchema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	title TEXT,
	session_type TEXT NOT NULL DEFAULT 'operator',
	created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	user_identity TEXT
);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	operator_session_id TEXT,
	timestamp TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	type TEXT NOT NULL,
	content_text BLOB,
	command_raw TEXT,
	command_exit_code INTEGER,
	command_stdout BLOB,
	command_stderr BLOB,
	execution_duration_ms INTEGER,
	stored_locally INTEGER DEFAULT 1,
	stdout_truncated INTEGER DEFAULT 0,
	stderr_truncated INTEGER DEFAULT 0,
	encrypted INTEGER DEFAULT 0,
	FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS file_mutation_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	event_id INTEGER NOT NULL,
	filepath TEXT NOT NULL,
	operation TEXT NOT NULL,
	ledger_hash_before TEXT,
	ledger_hash_after TEXT,
	diff_stat TEXT,
	FOREIGN KEY(event_id) REFERENCES events(id)
);

CREATE TABLE IF NOT EXISTS receipts (
	transaction_id TEXT PRIMARY KEY,
	transaction_hash TEXT NOT NULL,
	operator_id TEXT NOT NULL,
	operator_session_id TEXT NOT NULL,
	action_type TEXT NOT NULL,
	target_resource TEXT,
	status TEXT NOT NULL,
	result_summary TEXT,
	state_root_before TEXT,
	state_root_after TEXT,
	executed_at_ms INTEGER NOT NULL,
	signer_key_id TEXT NOT NULL,
	signature TEXT NOT NULL,
	timestamp TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_events_session_id ON events(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_file_mutation_event_id ON file_mutation_log(event_id);
CREATE INDEX IF NOT EXISTS idx_file_mutation_filepath ON file_mutation_log(filepath);
CREATE INDEX IF NOT EXISTS idx_receipts_session_id ON receipts(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_receipts_timestamp ON receipts(timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_id_type ON sessions(id, session_type);
`

// CreateSession creates a new session in the audit log
func (ass *SQLAuditStore) CreateSession(id, sessionType, title, userIdentity string) error {
	if ass == nil || ass.db == nil {
		return nil
	}
	if id == "" || strings.TrimSpace(id) != id {
		return ErrAuditSessionMissing
	}
	if sessionType == "" {
		sessionType = string(constants.UserRoleOperator)
	}

	query := `INSERT INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`
	_, err := ass.db.ExecWithRetry(query, id, sessionType, title, userIdentity)
	if err != nil {
		return fmt.Errorf("failed to create Operator session: %w", err)
	}

	ass.logger.Info("OperatorSession created", "operator_session_id", id, "session_type", sessionType, "title", title)
	return nil
}

// GetOperatorSession retrieves a session by ID
func (ass *SQLAuditStore) GetOperatorSession(id string) (*OperatorSession, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	query := `SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?`
	row := ass.db.QueryRowWithRetry(query, id)

	var session OperatorSession
	var title, userIdentity sql.NullString
	var createdAtStr string
	err := row.Scan(&session.ID, &title, &createdAtStr, &userIdentity)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	session.CreatedAt, _ = sqliteutil.ParseTimestamp(createdAtStr)

	if title.Valid {
		session.Title = title.String
	}
	if userIdentity.Valid {
		session.UserIdentity = userIdentity.String
	}

	return &session, nil
}

func (ass *SQLAuditStore) requireExistingSessionTx(tx *sql.Tx, event *Event) error {
	if event == nil {
		return ErrAuditEventNil
	}
	if event.OperatorSessionID == "" || strings.TrimSpace(event.OperatorSessionID) != event.OperatorSessionID {
		return ErrAuditSessionMissing
	}

	var exists int
	err := tx.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, event.OperatorSessionID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s", ErrAuditSessionUnknown, event.OperatorSessionID)
	}
	if err != nil {
		return fmt.Errorf("failed to verify audit session: %w", err)
	}
	return nil
}

// RecordEvents records multiple events in a single database transaction.
func (ass *SQLAuditStore) RecordEvents(events []*Event) error {
	if ass == nil || ass.db == nil || len(events) == 0 {
		return nil
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	return ass.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		query := `
		INSERT INTO events (
			operator_session_id, timestamp, type, content_text,
			command_raw, command_exit_code, command_stdout, command_stderr,
			execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		stmt, err := tx.Prepare(query)
		if err != nil {
			return fmt.Errorf("failed to prepare batch statement: %w", err)
		}
		defer stmt.Close()

		for _, event := range events {
			if err := ass.requireExistingSessionTx(tx, event); err != nil {
				return err
			}

			stdout, stdoutTruncated := ass.truncateOutput(event.CommandStdout)
			stderr, stderrTruncated := ass.truncateOutput(event.CommandStderr)

			encrypted := ass.IsEncryptionEnabled()
			encryptedFlag := 0
			if encrypted {
				encryptedFlag = 1
			}

			contentTextBytes, err := ass.encryptContent(event.ContentText)
			if err != nil {
				return fmt.Errorf("failed to encrypt content_text: %w", err)
			}

			stdoutBytes, err := ass.encryptContent(stdout)
			if err != nil {
				return fmt.Errorf("failed to encrypt stdout: %w", err)
			}

			stderrBytes, err := ass.encryptContent(stderr)
			if err != nil {
				return fmt.Errorf("failed to encrypt stderr: %w", err)
			}

			_, err = stmt.Exec(
				event.OperatorSessionID,
				sqliteutil.FormatTimestamp(event.Timestamp),
				event.Type,
				contentTextBytes,
				event.CommandRaw,
				event.CommandExitCode,
				stdoutBytes,
				stderrBytes,
				event.ExecutionDurationMs,
				true, // stored_locally
				stdoutTruncated,
				stderrTruncated,
				encryptedFlag,
			)
			if err != nil {
				return fmt.Errorf("failed to execute batch statement: %w", err)
			}
		}

		ass.logger.Info("Batch of events recorded", "count", len(events))
		return nil
	})
}

// RecordEvent records an event in the audit log
// Content fields are encrypted if an encryption vault is configured and unlocked
func (ass *SQLAuditStore) RecordEvent(event *Event) (int64, error) {
	if ass == nil || ass.db == nil {
		return 0, nil
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	var eventID int64
	err := ass.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		if err := ass.requireExistingSessionTx(tx, event); err != nil {
			return err
		}

		stdout, stdoutTruncated := ass.truncateOutput(event.CommandStdout)
		stderr, stderrTruncated := ass.truncateOutput(event.CommandStderr)

		encrypted := ass.IsEncryptionEnabled()

		contentTextBytes, err := ass.encryptContent(event.ContentText)
		if err != nil {
			return fmt.Errorf("failed to encrypt content_text: %w", err)
		}

		stdoutBytes, err := ass.encryptContent(stdout)
		if err != nil {
			return fmt.Errorf("failed to encrypt stdout: %w", err)
		}

		stderrBytes, err := ass.encryptContent(stderr)
		if err != nil {
			return fmt.Errorf("failed to encrypt stderr: %w", err)
		}

		query := `
		INSERT INTO events (
			operator_session_id, timestamp, type, content_text,
			command_raw, command_exit_code, command_stdout, command_stderr,
			execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		encryptedFlag := 0
		if encrypted {
			encryptedFlag = 1
		}

		result, err := tx.Exec(query,
			event.OperatorSessionID,
			sqliteutil.FormatTimestamp(event.Timestamp),
			event.Type,
			contentTextBytes,
			event.CommandRaw,
			event.CommandExitCode,
			stdoutBytes,
			stderrBytes,
			event.ExecutionDurationMs,
			true, // stored_locally
			stdoutTruncated,
			stderrTruncated,
			encryptedFlag,
		)
		if err != nil {
			return fmt.Errorf("failed to record event: %w", err)
		}

		id, _ := result.LastInsertId()
		eventID = id

		ass.logger.Info("Event recorded",
			"event_id", eventID,
			"type", event.Type,
			"operator_session_id", event.OperatorSessionID,
			"stdout_truncated", stdoutTruncated,
			"stderr_truncated", stderrTruncated,
			"encrypted", encrypted)

		return nil
	})

	return eventID, err
}

// RecordActionReceipt records a signed ActionReceipt in the audit store.
// This is the authoritative transaction-native audit record.
func (ass *SQLAuditStore) RecordActionReceipt(record *models.ActionReceiptRecord) error {
	if ass == nil || ass.db == nil {
		return nil
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	query := `
	INSERT INTO receipts (
		transaction_id, transaction_hash, operator_id, operator_session_id,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, timestamp
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(transaction_id) DO UPDATE SET
		status = excluded.status,
		result_summary = excluded.result_summary,
		state_root_after = excluded.state_root_after,
		executed_at_ms = excluded.executed_at_ms,
		signature = excluded.signature,
		timestamp = excluded.timestamp
	`

	_, err := ass.db.ExecWithRetry(query,
		record.TransactionID,
		record.TransactionHash,
		record.OperatorID,
		record.OperatorSessionID,
		record.ActionType,
		record.TargetResource,
		record.Status,
		record.ResultSummary,
		record.StateRootBefore,
		record.StateRootAfter,
		record.ExecutedAt.UnixMilli(),
		record.SignerKeyID,
		record.Signature,
		sqliteutil.FormatTimestamp(record.Timestamp),
	)
	if err != nil {
		return fmt.Errorf("failed to record action receipt: %w", err)
	}

	ass.logger.Info("ActionReceipt recorded",
		"transaction_id", record.TransactionID,
		"status", record.Status)

	return nil
}

// GetActionReceipt retrieves a single action receipt by transaction ID.
func (ass *SQLAuditStore) GetActionReceipt(transactionID string) (*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	query := `
	SELECT transaction_id, transaction_hash, operator_id, operator_session_id,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, timestamp
	FROM receipts
	WHERE transaction_id = ?
	`

	var r models.ActionReceiptRecord
	var executedAtMs int64
	var timestampStr string
	err := ass.db.QueryRowWithRetry(query, transactionID).Scan(
		&r.TransactionID, &r.TransactionHash, &r.OperatorID, &r.OperatorSessionID,
		&r.ActionType, &r.TargetResource, &r.Status, &r.ResultSummary,
		&r.StateRootBefore, &r.StateRootAfter, &executedAtMs,
		&r.SignerKeyID, &r.Signature, &timestampStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get action receipt: %w", err)
	}

	r.ExecutedAt = time.UnixMilli(executedAtMs)
	r.Timestamp, _ = sqliteutil.ParseTimestamp(timestampStr)

	return &r, nil
}

// ListActionReceipts retrieves action receipts with optional filtering and pagination.
func (ass *SQLAuditStore) ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	if limit <= 0 {
		limit = 50
	}

	var query strings.Builder
	query.WriteString(`
	SELECT transaction_id, transaction_hash, operator_id, operator_session_id,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, timestamp
	FROM receipts
	`)

	args := []interface{}{}
	if operatorSessionID != "" {
		query.WriteString(" WHERE operator_session_id = ?")
		args = append(args, operatorSessionID)
	}

	query.WriteString(" ORDER BY timestamp DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	type receiptRow struct {
		record       models.ActionReceiptRecord
		executedAtMs int64
		timestampStr string
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query.String(), args, func(r *sql.Rows) (receiptRow, error) {
		var row receiptRow
		err := r.Scan(
			&row.record.TransactionID, &row.record.TransactionHash, &row.record.OperatorID, &row.record.OperatorSessionID,
			&row.record.ActionType, &row.record.TargetResource, &row.record.Status, &row.record.ResultSummary,
			&row.record.StateRootBefore, &row.record.StateRootAfter, &row.executedAtMs,
			&row.record.SignerKeyID, &row.record.Signature, &row.timestampStr,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query action receipts: %w", err)
	}

	var results []*models.ActionReceiptRecord
	for _, row := range rows {
		row.record.ExecutedAt = time.UnixMilli(row.executedAtMs)
		row.record.Timestamp, _ = sqliteutil.ParseTimestamp(row.timestampStr)
		results = append(results, &row.record)
	}

	return results, nil
}

// ListActionReceiptsSince retrieves action receipts newer than the given timestamp.
func (ass *SQLAuditStore) ListActionReceiptsSince(since time.Time, limit int) ([]*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT transaction_id, transaction_hash, operator_id, operator_session_id,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, timestamp
	FROM receipts
	WHERE timestamp > ?
	ORDER BY timestamp ASC
	LIMIT ?
	`

	type receiptRow struct {
		record       models.ActionReceiptRecord
		executedAtMs int64
		timestampStr string
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{sqliteutil.FormatTimestamp(since), limit}, func(r *sql.Rows) (receiptRow, error) {
		var row receiptRow
		err := r.Scan(
			&row.record.TransactionID, &row.record.TransactionHash, &row.record.OperatorID, &row.record.OperatorSessionID,
			&row.record.ActionType, &row.record.TargetResource, &row.record.Status, &row.record.ResultSummary,
			&row.record.StateRootBefore, &row.record.StateRootAfter, &row.executedAtMs,
			&row.record.SignerKeyID, &row.record.Signature, &row.timestampStr,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query action receipts since %v: %w", since, err)
	}

	var results []*models.ActionReceiptRecord
	for _, row := range rows {
		row.record.ExecutedAt = time.UnixMilli(row.executedAtMs)
		row.record.Timestamp, _ = sqliteutil.ParseTimestamp(row.timestampStr)
		results = append(results, &row.record)
	}

	return results, nil
}

// truncateOutput applies the head/tail truncation strategy for large outputs
func (ass *SQLAuditStore) truncateOutput(output string) (string, bool) {
	if len(output) <= ass.config.OutputTruncationThreshold {
		return output, false
	}

	headSize := ass.config.HeadTailSize
	tailSize := ass.config.HeadTailSize

	head := output[:headSize]
	tail := output[len(output)-tailSize:]

	truncated := fmt.Sprintf(constants.TruncatedOutputFormat,
		head,
		len(output)-headSize-tailSize,
		tail)

	return truncated, true
}

// GetEvents retrieves events for a session with pagination
// Content fields are decrypted if they were stored encrypted and the vault is unlocked
func (ass *SQLAuditStore) GetEvents(operatorSessionID string, limit, offset int) ([]*Event, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	if limit <= 0 {
		limit = 50
	}

	query := `
	SELECT id, operator_session_id, timestamp, type, content_text,
		command_raw, command_exit_code, command_stdout, command_stderr,
		execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated,
		COALESCE(encrypted, 0) as encrypted
	FROM events
	WHERE operator_session_id = ?
	ORDER BY timestamp DESC
	LIMIT ? OFFSET ?
	`

	type eventRow struct {
		event              Event
		timestampStr       string
		contentTextBytes   []byte
		commandStdoutBytes []byte
		commandStderrBytes []byte
		commandRaw         sql.NullString
		commandExitCode    sql.NullInt64
		storedLocally      sql.NullBool
		stdoutTruncated    sql.NullBool
		stderrTruncated    sql.NullBool
		encryptedFlag      int
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{operatorSessionID, limit, offset}, func(r *sql.Rows) (eventRow, error) {
		var row eventRow
		err := r.Scan(
			&row.event.ID,
			&row.event.OperatorSessionID,
			&row.timestampStr,
			&row.event.Type,
			&row.contentTextBytes,
			&row.commandRaw,
			&row.commandExitCode,
			&row.commandStdoutBytes,
			&row.commandStderrBytes,
			&row.event.ExecutionDurationMs,
			&row.storedLocally,
			&row.stdoutTruncated,
			&row.stderrTruncated,
			&row.encryptedFlag,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	var events []*Event
	for _, row := range rows {
		row.event.Timestamp, _ = sqliteutil.ParseTimestamp(row.timestampStr)

		if row.encryptedFlag == 1 && ass.IsEncryptionEnabled() {
			if len(row.contentTextBytes) > 0 {
				decrypted, err := ass.decryptContent(row.contentTextBytes)
				if err != nil {
					ass.logger.Warn("Failed to decrypt content_text", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
				} else {
					row.event.ContentText = decrypted
				}
			}
			if len(row.commandStdoutBytes) > 0 {
				decrypted, err := ass.decryptContent(row.commandStdoutBytes)
				if err != nil {
					ass.logger.Warn("Failed to decrypt stdout", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
				} else {
					row.event.CommandStdout = decrypted
				}
			}
			if len(row.commandStderrBytes) > 0 {
				decrypted, err := ass.decryptContent(row.commandStderrBytes)
				if err != nil {
					ass.logger.Warn("Failed to decrypt stderr", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
				} else {
					row.event.CommandStderr = decrypted
				}
			}
		} else {
			row.event.ContentText = string(row.contentTextBytes)
			row.event.CommandStdout = string(row.commandStdoutBytes)
			row.event.CommandStderr = string(row.commandStderrBytes)
		}

		if row.commandRaw.Valid {
			row.event.CommandRaw = row.commandRaw.String
		}
		if row.commandExitCode.Valid {
			exitCode := int(row.commandExitCode.Int64)
			row.event.CommandExitCode = &exitCode
		}
		if row.storedLocally.Valid {
			row.event.StoredLocally = row.storedLocally.Bool
		}
		if row.stdoutTruncated.Valid {
			row.event.StdoutTruncated = row.stdoutTruncated.Bool
		}
		if row.stderrTruncated.Valid {
			row.event.StderrTruncated = row.stderrTruncated.Bool
		}

		events = append(events, &row.event)
	}

	return events, nil
}

// RecordFileMutation records a file mutation in the audit log
func (ass *SQLAuditStore) RecordFileMutation(mutation *FileMutationLog) error {
	if ass == nil || ass.db == nil {
		return nil
	}

	query := `
	INSERT INTO file_mutation_log (
		event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := ass.db.ExecWithRetry(query,
		mutation.EventID,
		mutation.Filepath,
		string(mutation.Operation),
		mutation.LedgerHashBefore,
		mutation.LedgerHashAfter,
		mutation.DiffStat,
	)
	if err != nil {
		return fmt.Errorf("failed to record file mutation: %w", err)
	}

	ass.logger.Info("File mutation recorded",
		"event_id", mutation.EventID,
		"filepath", mutation.Filepath,
		"operation", mutation.Operation)

	return nil
}

// GetFileMutations retrieves file mutations for an event
func (ass *SQLAuditStore) GetFileMutations(eventID int64) ([]*FileMutationLog, error) {
	if ass == nil || ass.db == nil {
		return nil, fmt.Errorf("audit store is disabled")
	}

	query := `
	SELECT id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	FROM file_mutation_log
	WHERE event_id = ?
	`

	type mutationRow struct {
		mutation   FileMutationLog
		hashBefore sql.NullString
		hashAfter  sql.NullString
		diffStat   sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{eventID}, func(r *sql.Rows) (mutationRow, error) {
		var row mutationRow
		err := r.Scan(
			&row.mutation.ID,
			&row.mutation.EventID,
			&row.mutation.Filepath,
			&row.mutation.Operation,
			&row.hashBefore,
			&row.hashAfter,
			&row.diffStat,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query file mutations: %w", err)
	}

	var mutations []*FileMutationLog
	for _, row := range rows {
		if row.hashBefore.Valid {
			row.mutation.LedgerHashBefore = row.hashBefore.String
		}
		if row.hashAfter.Valid {
			row.mutation.LedgerHashAfter = row.hashAfter.String
		}
		if row.diffStat.Valid {
			row.mutation.DiffStat = row.diffStat.String
		}

		mutations = append(mutations, &row.mutation)
	}

	return mutations, nil
}

// auditStorePrune returns a PruneFunc that handles retention pruning
// for events, orphaned sessions, and orphaned file mutations.
func auditStorePrune(config *AuditStoreConfig) sqliteutil.PruneFunc {
	return func(ctx context.Context, db *sqliteutil.DB, logger *slog.Logger) error {
		cutoff := sqliteutil.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		// 1. Delete file mutations for old events first (satisfy FK constraints)
		_, err := db.ExecWithRetry(`
			DELETE FROM file_mutation_log
			WHERE event_id IN (SELECT id FROM events WHERE timestamp < ?)
		`, cutoff)
		if err != nil {
			logger.Error("Failed to prune old file mutations", string(constants.ConnectionStateError), err)
			return err
		}

		// 2. Delete events older than retention period
		result, err := db.ExecWithRetry("DELETE FROM events WHERE timestamp < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old events", string(constants.ConnectionStateError), err)
			return err
		}

		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old events", "rows_deleted", rowsDeleted)
		}

		// 3. Delete receipts older than retention period
		result, err = db.ExecWithRetry("DELETE FROM receipts WHERE timestamp < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old receipts", string(constants.ConnectionStateError), err)
			return err
		}
		rowsDeleted, _ = result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old receipts", "rows_deleted", rowsDeleted)
		}

		// 4. Delete sessions that no longer have any events or receipts
		_, err = db.ExecWithRetry(`
			DELETE FROM sessions
			WHERE id NOT IN (SELECT DISTINCT operator_session_id FROM events WHERE operator_session_id IS NOT NULL)
			AND id NOT IN (SELECT DISTINCT operator_session_id FROM receipts WHERE operator_session_id IS NOT NULL)
		`)
		if err != nil {
			logger.Warn("Failed to prune orphaned sessions", string(constants.ConnectionStateError), err)
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", string(constants.ConnectionStateError), err)
		}
		return nil
	}
}

// GetEncryptionVault returns the optional encryption vault used by this service.
func (ass *SQLAuditStore) GetEncryptionVault() *vault.Vault {
	if ass == nil {
		return nil
	}
	return ass.encryptionVault
}

// Wait blocks until all in-flight writes have finished.
func (ass *SQLAuditStore) Wait() {
	if ass == nil {
		return
	}
	ass.muWrites.Wait()
}

// Close shuts down the audit store service. Idempotent.
func (ass *SQLAuditStore) Close() error {
	if ass == nil {
		return nil
	}

	ass.Wait()

	var closeErr error
	ass.closeOnce.Do(func() {
		if ass.pruner != nil {
			ass.pruner.Stop()
		}
		if ass.db != nil {
			closeErr = ass.db.Close()
		}
	})

	return closeErr
}

// IsEnabled returns whether the audit store is enabled
func (ass *SQLAuditStore) IsEnabled() bool {
	return ass != nil && ass.db != nil
}

// GetDataDir returns the audit store data directory
func (ass *SQLAuditStore) GetDataDir() string {
	if ass == nil {
		return ""
	}
	return ass.config.DataDir
}

// IsEncryptionEnabled returns whether content encryption is enabled
func (ass *SQLAuditStore) IsEncryptionEnabled() bool {
	return ass != nil && ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked()
}

// encryptContent encrypts content using the encryption vault
func (ass *SQLAuditStore) encryptContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}

	if !ass.encryptionVault.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked, cannot encrypt content")
	}

	encrypted, err := ass.encryptionVault.Encrypt([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt content: %w", err)
	}

	return encrypted, nil
}

// decryptContent decrypts content using the encryption vault
func (ass *SQLAuditStore) decryptContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if !ass.encryptionVault.IsUnlocked() {
		return "", fmt.Errorf("vault is locked, cannot decrypt content")
	}

	decrypted, err := ass.encryptionVault.Decrypt(data)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt content: %w", err)
	}

	return string(decrypted), nil
}
