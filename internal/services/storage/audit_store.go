// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// AuditStoreConfig holds configuration for the SQL audit store
type AuditStoreConfig struct {
	DBPath                    string
	MaxDBSizeMB               int64
	RetentionDays             int
	PruneIntervalMinutes      int
	OutputTruncationThreshold int
	HeadTailSize              int
	// EncryptionVault is the required vault.Vault for encrypting sensitive content fields.
	// content_text, command_stdout, and command_stderr are encrypted at rest.
	EncryptionVault *vault.Vault
}

// DefaultAuditStoreConfig returns the default configuration for the audit store.
func DefaultAuditStoreConfig() *AuditStoreConfig {
	return &AuditStoreConfig{
		DBPath:                    constants.DbFilename,
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
}

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
	SessionType  string
	Title        string
	CreatedAt    time.Time
	UserIdentity string
}

// Event represents an event in the audit log (append-only)
type Event struct {
	ID                  int64
	Seq                 int64
	PrevHash            string
	ContentDigest       string
	Hash                string
	TransactionID       string
	OperatorSessionID   string
	Timestamp           time.Time
	Type                constants.EventType
	ContentText         string
	CommandRaw          string
	CommandExitCode     int
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
	db               *sqliteutil.DB
	config           *AuditStoreConfig
	logger           *slog.Logger
	fileSvc          fs.RuntimeFileService
	encryptionVault  *vault.Vault
	pruner           *sqliteutil.Pruner
	commitmentLedger *CommitmentLedger
	closeOnce        sync.Once

	muWrites sync.WaitGroup
}

// NewSQLAuditStore creates a new SQL audit store
func NewSQLAuditStore(config *AuditStoreConfig, logger *slog.Logger, fileSvc fs.RuntimeFileService) (*SQLAuditStore, error) {
	if config == nil {
		config = DefaultAuditStoreConfig()
	}

	if config.EncryptionVault == nil {
		return nil, constants.ErrAuditStoreEncryptionVaultRequired
	}
	if fileSvc == nil {
		return nil, fmt.Errorf("audit store: file service: %w", constants.ErrMissingRequiredField)
	}
	if logger == nil {
		logger = slog.Default()
	}

	ass := &SQLAuditStore{
		config:          config,
		logger:          logger,
		fileSvc:         fileSvc,
		encryptionVault: config.EncryptionVault,
	}

	if err := ass.bootstrap(); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreBootstrapFailed, err)
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	ass.pruner = sqliteutil.NewPruner(ass.db, logger, interval, auditStorePrune(config))
	ass.pruner.Start()

	encryptionEnabled := ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked()
	dataDir := ass.fileSvc.Resolve(constants.DataDirname)
	ass.logger.Info("Audit store initialized",
		"data_dir", dataDir,
		"db_path", pathutil.ResolveDBPath(dataDir, config.DBPath),
		"encryption_enabled", encryptionEnabled)

	return ass, nil
}

func (ass *SQLAuditStore) CommitmentLedger() *CommitmentLedger {
	if ass == nil {
		return nil
	}
	return ass.commitmentLedger
}

// bootstrap initializes the audit store (directory structure, database)
func (ass *SQLAuditStore) bootstrap() error {
	ass.logger.Info("Bootstrapping audit store", "data_dir", ass.fileSvc.Resolve(constants.DataDirname))

	if err := ass.createDirectoryStructure(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreCreateDirFailed, err)
	}

	if err := ass.verifyWritePermissions(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreNotWritable, err)
	}

	if err := ass.initDatabase(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitDBFailed, err)
	}

	ass.logger.Info("Audit store bootstrap completed successfully")
	return nil
}

// createDirectoryStructure creates the audit store directory structure
func (ass *SQLAuditStore) createDirectoryStructure() error {
	if err := ass.fileSvc.MkdirAll(context.Background(), constants.DataDirname, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w %s: %w", constants.ErrAuditStoreCreateDirPathFailed, constants.DataDirname, err)
	}

	ass.logger.Info("Audit store directory structure ensured",
		"data_dir", ass.fileSvc.Resolve(constants.DataDirname))

	return nil
}

// verifyWritePermissions ensures the data directory is writable
func (ass *SQLAuditStore) verifyWritePermissions() error {
	testRelPath := filepath.Join(constants.DataDirname, ".write_test")

	if err := ass.fileSvc.WriteFile(context.Background(), testRelPath, []byte("write_test"), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w %s: %w", constants.ErrAuditStoreCannotWrite, ass.fileSvc.Resolve(constants.DataDirname), err)
	}

	if err := ass.fileSvc.Remove(context.Background(), testRelPath); err != nil {
		ass.logger.Warn("Failed to remove write test file", "path", testRelPath, string(constants.ConnectionStateError), err)
	}

	ass.logger.Info("Write permissions verified", "path", ass.fileSvc.Resolve(constants.DataDirname))
	return nil
}

// initDatabase creates the database and schema
func (ass *SQLAuditStore) initDatabase() error {
	dbPath := pathutil.ResolveDBPath(ass.fileSvc.Resolve(constants.DataDirname), ass.config.DBPath)

	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, ass.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreOpenDBFailed, err)
	}

	if _, err := db.Exec(auditStoreSchema); err != nil {
		db.Close()
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitSchemaFailed, err)
	}

	if err := migrateReceiptsColumns(db, ass.logger); err != nil {
		db.Close()
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitSchemaFailed, err)
	}
	if err := migrateCommitmentColumns(db, ass.logger); err != nil {
		db.Close()
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitSchemaFailed, err)
	}
	if err := MigrateEventChainColumns(db, ass.logger, ass.encryptionVault); err != nil {
		db.Close()
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitSchemaFailed, err)
	}

	ass.db = db
	ass.commitmentLedger = NewCommitmentLedger(db, ass.logger)

	ass.logger.Info("Database schema initialized")
	return nil
}

// migrateReceiptsColumns adds identity and canonical receipt columns to the
// receipts table for databases created before these columns existed. The
// CREATE TABLE IF NOT EXISTS schema includes them for fresh databases, but
// existing databases need an ALTER TABLE. SQLite does not support ADD COLUMN
// IF NOT EXISTS, so we check pragma table_info first.
func migrateReceiptsColumns(db *sqliteutil.DB, logger *slog.Logger) error {
	cols, err := db.QueryWithRetry("PRAGMA table_info(receipts)")
	if err != nil {
		return fmt.Errorf("audit_store: migrate receipts: pragma: %w", err)
	}
	defer cols.Close()

	existing := make(map[string]bool)
	for cols.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("audit_store: migrate receipts: scan: %w", err)
		}
		existing[name] = true
	}

	for _, col := range []string{"requestor_user_id", "acting_app_id", "investigation_id", "receipt_json", "event_type"} {
		if existing[col] {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE receipts ADD COLUMN %s TEXT", col)); err != nil {
			return fmt.Errorf("audit_store: migrate receipts: add column %s: %w", col, err)
		}
		logger.Info("Audit store migration: added column", "column", col)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_receipts_investigation_id ON receipts(investigation_id)"); err != nil {
		return fmt.Errorf("audit_store: migrate receipts: create investigation index: %w", err)
	}
	return nil
}

func migrateCommitmentColumns(db *sqliteutil.DB, logger *slog.Logger) error {
	cols, err := db.QueryWithRetry("PRAGMA table_info(commitment_ledger)")
	if err != nil {
		return fmt.Errorf("audit_store: migrate commitments: pragma: %w", err)
	}
	defer cols.Close()

	existing := make(map[string]bool)
	for cols.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("audit_store: migrate commitments: scan: %w", err)
		}
		existing[name] = true
	}
	if existing["warden_intent_signature_digest"] {
		return nil
	}
	if !existing["actuator_intent_signature_digest"] {
		return fmt.Errorf("audit_store: migrate commitments: %w", constants.ErrMissingRequiredField)
	}
	if _, err := db.Exec("ALTER TABLE commitment_ledger RENAME COLUMN actuator_intent_signature_digest TO warden_intent_signature_digest"); err != nil {
		return fmt.Errorf("audit_store: migrate commitments: rename intent digest: %w", err)
	}
	logger.Info("Audit store migration: renamed commitment intent digest column")
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
	seq INTEGER UNIQUE,
	prev_hash TEXT,
	content_digest TEXT,
	hash TEXT,
	transaction_id TEXT,
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
	investigation_id TEXT,
	operator_id TEXT NOT NULL,
	operator_session_id TEXT,
	requestor_user_id TEXT,
	acting_app_id TEXT,
	event_type TEXT,
	action_type TEXT NOT NULL,
	target_resource TEXT,
	status TEXT NOT NULL,
	result_summary TEXT,
	state_root_before TEXT,
	state_root_after TEXT,
	executed_at_ms INTEGER NOT NULL,
	signer_key_id TEXT NOT NULL,
	signature TEXT NOT NULL,
	receipt_json TEXT,
	timestamp TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS commitment_ledger (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	transaction_id TEXT NOT NULL,
	transaction_hash TEXT NOT NULL,
	prior_commitment_hash TEXT NOT NULL,
	state_root_at_commit TEXT,
	l2_signature_digest TEXT,
	warden_intent_signature_digest TEXT,
	human_signature_digest TEXT,
	action_type TEXT,
	target_resource TEXT,
	committed_at_unix_ms INTEGER NOT NULL,
	auditor_key_id TEXT,
	signature TEXT,
	hash TEXT NOT NULL,
	attestation_json TEXT NOT NULL,
	UNIQUE(hash)
);

CREATE INDEX IF NOT EXISTS idx_events_session_id ON events(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_file_mutation_event_id ON file_mutation_log(event_id);
CREATE INDEX IF NOT EXISTS idx_file_mutation_filepath ON file_mutation_log(filepath);
CREATE INDEX IF NOT EXISTS idx_receipts_session_id ON receipts(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_receipts_timestamp ON receipts(timestamp);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_id_type ON sessions(id, session_type);
CREATE INDEX IF NOT EXISTS idx_commitment_ledger_committed_at ON commitment_ledger(committed_at_unix_ms);
`

// CreateSession creates a new session in the audit log
func (ass *SQLAuditStore) CreateSession(id string, sessionType constants.SessionType, title, userIdentity string) error {
	if ass == nil || ass.db == nil {
		return nil
	}
	if id == "" || strings.TrimSpace(id) != id {
		return constants.ErrAuditSessionMissing
	}
	if sessionType == "" {
		sessionType = constants.SessionTypeOperator
	}

	query := `INSERT INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`
	_, err := ass.db.ExecWithRetry(query, id, string(sessionType), title, userIdentity)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreCreateSessionFailed, err)
	}

	ass.logger.Info("OperatorSession created", "operator_session_id", id, "session_type", string(sessionType), "title", title)
	return nil
}

// GetOperatorSession retrieves a session by ID
func (ass *SQLAuditStore) GetOperatorSession(id string) (*OperatorSession, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	query := `SELECT id, session_type, title, created_at, user_identity FROM sessions WHERE id = ?`
	row := ass.db.QueryRowWithRetry(query, id)

	var session OperatorSession
	var sessionType, title, userIdentity sql.NullString
	var createdAtStr string
	err := row.Scan(&session.ID, &sessionType, &title, &createdAtStr, &userIdentity)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreGetSessionFailed, err)
	}

	session.CreatedAt, _ = timesvc.ParseTimestamp(createdAtStr)

	if sessionType.Valid {
		session.SessionType = sessionType.String
	}
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
		return constants.ErrAuditEventNil
	}
	if event.OperatorSessionID == "" || strings.TrimSpace(event.OperatorSessionID) != event.OperatorSessionID {
		return constants.ErrAuditSessionMissing
	}

	var exists int
	err := tx.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, event.OperatorSessionID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s", constants.ErrAuditSessionUnknown, event.OperatorSessionID)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreVerifySessionFailed, err)
	}
	return nil
}

func (ass *SQLAuditStore) requireExistingSessionConn(conn *sql.Conn, event *Event) error {
	if event == nil {
		return constants.ErrAuditEventNil
	}
	if event.Type == constants.EventPlatformAuditChainCheckpointed {
		return nil
	}
	if event.OperatorSessionID == "" || strings.TrimSpace(event.OperatorSessionID) != event.OperatorSessionID {
		return constants.ErrAuditSessionMissing
	}

	var exists int
	err := conn.QueryRowContext(context.Background(), `SELECT 1 FROM sessions WHERE id = ?`, event.OperatorSessionID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s", constants.ErrAuditSessionUnknown, event.OperatorSessionID)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreVerifySessionFailed, err)
	}
	return nil
}

func (ass *SQLAuditStore) prepareAuditEventInsert(event *Event) (PreparedAuditEventInsert, error) {
	stdout, stdoutTruncated := ass.truncateOutput(event.CommandStdout)
	stderr, stderrTruncated := ass.truncateOutput(event.CommandStderr)

	contentTextBytes, err := ass.encryptContent(event.ContentText)
	if err != nil {
		return PreparedAuditEventInsert{}, fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptContentFailed, err)
	}

	stdoutBytes, err := ass.encryptContent(stdout)
	if err != nil {
		return PreparedAuditEventInsert{}, fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStdoutFailed, err)
	}

	stderrBytes, err := ass.encryptContent(stderr)
	if err != nil {
		return PreparedAuditEventInsert{}, fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStderrFailed, err)
	}

	encryptedFlag := 0
	if ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked() {
		encryptedFlag = 1
	}

	return PreparedAuditEventInsert{
		Event:            event,
		TimestampStr:     timesvc.FormatTimestamp(event.Timestamp),
		ContentTextBytes: contentTextBytes,
		StdoutBytes:      stdoutBytes,
		StderrBytes:      stderrBytes,
		StdoutTruncated:  stdoutTruncated,
		StderrTruncated:  stderrTruncated,
		EncryptedFlag:    encryptedFlag,
		StdoutPlaintext:  stdout,
		StderrPlaintext:  stderr,
	}, nil
}

// RecordEvents records multiple events in a single database transaction.
func (ass *SQLAuditStore) RecordEvents(events []*Event) error {
	if ass == nil || len(events) == 0 {
		return nil
	}
	if ass.db == nil {
		return constants.ErrAuditStoreDBNotInitialized
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	return ass.db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
		for _, event := range events {
			if err := ass.requireExistingSessionConn(conn, event); err != nil {
				return err
			}
			prepared, err := ass.prepareAuditEventInsert(event)
			if err != nil {
				return err
			}
			if _, _, _, err := AppendPreparedAuditEvent(context.Background(), conn, prepared); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditStoreExecuteBatchFailed, err)
			}
		}

		ass.logger.Info("Batch of events recorded", "count", len(events))
		return nil
	})
}

// RecordEvent records an event in the audit log
// Content fields are encrypted if an encryption vault is configured and unlocked
func (ass *SQLAuditStore) RecordEvent(event *Event) (int64, error) {
	if ass == nil {
		return 0, nil
	}
	if ass.db == nil {
		return 0, constants.ErrAuditStoreDBNotInitialized
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	var eventID int64
	err := ass.db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
		// Auto-create session row for app sessions to avoid FK race conditions
		// This mirrors the behavior in RecordActionReceipt
		if event.OperatorSessionID != "" {
			if _, err := conn.ExecContext(context.Background(),
				`INSERT OR IGNORE INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`,
				event.OperatorSessionID, string(constants.SessionTypeApp), event.OperatorSessionID, event.OperatorSessionID,
			); err != nil {
				return fmt.Errorf("audit store: create app session: %w", err)
			}
		}

		if err := ass.requireExistingSessionConn(conn, event); err != nil {
			return err
		}

		prepared, err := ass.prepareAuditEventInsert(event)
		if err != nil {
			return err
		}

		id, _, _, err := AppendPreparedAuditEvent(context.Background(), conn, prepared)
		if err != nil {
			return err
		}
		eventID = id

		ass.logger.Info("Event recorded",
			"event_id", eventID,
			"type", event.Type,
			"operator_session_id", event.OperatorSessionID,
			"stdout_truncated", prepared.StdoutTruncated,
			"stderr_truncated", prepared.StderrTruncated,
			"encrypted", prepared.EncryptedFlag,
			"exit_code", event.CommandExitCode)

		return nil
	})

	return eventID, err
}

// RecordEventChained records an audit event and returns the chain metadata for acknowledgement.
func (ass *SQLAuditStore) RecordEventChained(event *Event) (int64, int64, string, error) {
	if ass == nil {
		return 0, 0, "", nil
	}
	if ass.db == nil {
		return 0, 0, "", constants.ErrAuditStoreDBNotInitialized
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	var eventID, seq int64
	var hash string
	err := ass.db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
		if event.OperatorSessionID != "" {
			if _, err := conn.ExecContext(context.Background(),
				`INSERT OR IGNORE INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`,
				event.OperatorSessionID, string(constants.SessionTypeApp), event.OperatorSessionID, event.OperatorSessionID,
			); err != nil {
				return fmt.Errorf("audit store: create app session: %w", err)
			}
		}

		if err := ass.requireExistingSessionConn(conn, event); err != nil {
			return err
		}

		prepared, err := ass.prepareAuditEventInsert(event)
		if err != nil {
			return err
		}

		id, chainSeq, chainHash, err := AppendPreparedAuditEvent(context.Background(), conn, prepared)
		if err != nil {
			return err
		}
		eventID = id
		seq = chainSeq
		hash = chainHash

		ass.logger.Info("Event recorded",
			"event_id", eventID,
			"type", event.Type,
			"operator_session_id", event.OperatorSessionID,
			"seq", seq,
			"stdout_truncated", prepared.StdoutTruncated,
			"stderr_truncated", prepared.StderrTruncated,
			"encrypted", prepared.EncryptedFlag,
			"exit_code", event.CommandExitCode)

		return nil
	})

	return eventID, seq, hash, err
}

// GetEventChainMetaByTransactionID returns chain metadata for an idempotent audit append.
func (ass *SQLAuditStore) GetEventChainMetaByTransactionID(transactionID string) (int64, string, bool, error) {
	if ass == nil || ass.db == nil {
		return 0, "", false, constants.ErrAuditStoreDBNotInitialized
	}
	if transactionID == "" {
		return 0, "", false, nil
	}

	var seq int64
	var hash string
	err := ass.db.QueryRowContext(context.Background(), `
		SELECT seq, hash FROM events WHERE transaction_id = ? ORDER BY id DESC LIMIT 1
	`, transactionID).Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryEventsFailed, err)
	}
	return seq, hash, true, nil
}

// RecordActionReceipt upserts the latest-stage receipt projection and appends a
// chained operator.receipt.recorded fact for every stage write.
func (ass *SQLAuditStore) RecordActionReceipt(record *models.ActionReceiptRecord) error {
	if ass == nil {
		return nil
	}
	if ass.db == nil {
		return constants.ErrAuditStoreDBNotInitialized
	}
	if record == nil {
		return constants.ErrAuditStoreRecordReceiptFailed
	}

	ass.muWrites.Add(1)
	defer ass.muWrites.Done()

	receiptJSON := []byte(nil)
	if record.ActionReceipt != nil {
		var err error
		receiptJSON, err = compliancev1.MarshalCanonical(record.ActionReceipt)
		if err != nil {
			return fmt.Errorf("%w: marshal canonical receipt: %w", constants.ErrAuditStoreRecordReceiptFailed, err)
		}
	}

	timestamp := record.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	err := ass.db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
		if record.OperatorSessionID != "" {
			if _, err := conn.ExecContext(context.Background(),
				`INSERT OR IGNORE INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`,
				record.OperatorSessionID, string(constants.SessionTypeOperator), record.OperatorSessionID, record.OperatorID,
			); err != nil {
				return fmt.Errorf("audit store: create operator session: %w", err)
			}
		}

		if err := ass.upsertActionReceiptConn(conn, record, receiptJSON, timestamp); err != nil {
			return err
		}

		chainEvent := &Event{
			OperatorSessionID: record.OperatorSessionID,
			Timestamp:         timestamp,
			Type:              constants.EventOperatorReceiptRecorded,
			ContentText:       string(receiptJSON),
			TransactionID:     record.TransactionID,
		}
		if chainEvent.OperatorSessionID != "" {
			if err := ass.requireExistingSessionConn(conn, chainEvent); err != nil {
				return err
			}
		}
		prepared, err := ass.prepareAuditEventInsert(chainEvent)
		if err != nil {
			return err
		}
		_, _, _, err = AppendPreparedAuditEvent(context.Background(), conn, prepared)
		return err
	})
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreRecordReceiptFailed, err)
	}

	ass.logger.Info("ActionReceipt recorded",
		"transaction_id", record.TransactionID,
		"status", record.Status)

	return nil
}

func (ass *SQLAuditStore) upsertActionReceiptConn(conn *sql.Conn, record *models.ActionReceiptRecord, receiptJSON []byte, timestamp time.Time) error {
	var sessionID sql.NullString
	if record.OperatorSessionID != "" {
		sessionID = sql.NullString{String: record.OperatorSessionID, Valid: true}
	}

	query := `
	INSERT INTO receipts (
		transaction_id, transaction_hash, investigation_id, operator_id, operator_session_id,
		requestor_user_id, acting_app_id, event_type,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, receipt_json, timestamp
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(transaction_id) DO UPDATE SET
		investigation_id = excluded.investigation_id,
		status = excluded.status,
		result_summary = excluded.result_summary,
		state_root_after = excluded.state_root_after,
		executed_at_ms = excluded.executed_at_ms,
		signature = excluded.signature,
		receipt_json = excluded.receipt_json,
		timestamp = excluded.timestamp
	`

	_, err := conn.ExecContext(context.Background(), query,
		record.TransactionID,
		record.TransactionHash,
		record.InvestigationID,
		record.OperatorID,
		sessionID,
		record.RequestorUserID,
		record.ActingAppID,
		record.EventType,
		record.ActionType,
		record.TargetResource,
		record.Status,
		record.ResultSummary,
		record.StateRootBefore,
		record.StateRootAfter,
		record.ExecutedAt.UnixMilli(),
		record.SignerKeyID,
		record.Signature,
		receiptJSON,
		timesvc.FormatTimestamp(timestamp),
	)
	if err != nil {
		return fmt.Errorf("audit store: upsert receipt projection: %w", err)
	}
	return nil
}

// GetAuditChainHead returns the latest chained audit event sequence and hash.
func (ass *SQLAuditStore) GetAuditChainHead(_ context.Context) (int64, string, error) {
	if ass == nil || ass.db == nil {
		return 0, "", constants.ErrAuditStoreDisabled
	}

	var headSeq int64
	var headHash string
	err := ass.db.QueryRowWithRetry(`
		SELECT seq, hash FROM events WHERE seq IS NOT NULL ORDER BY seq DESC LIMIT 1
	`).Scan(&headSeq, &headHash)
	if err == sql.ErrNoRows {
		return 0, auditChainGenesisPrevHash, nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("audit store: chain head: %w", err)
	}
	return headSeq, headHash, nil
}

func parseStoredActionReceipt(receiptJSON sql.NullString) (*operatorv1.ActionReceipt, error) {
	if !receiptJSON.Valid || receiptJSON.String == "" {
		return nil, nil
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical([]byte(receiptJSON.String), receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

// DocSet implements governance.TransactionAuditStore for outbound mode. The
// outbound-mode L5Actuator persists signed ActionReceipt records via this
// method; the data payload is a JSON-encoded models.ActionReceiptRecord that
// is decoded and recorded in the receipts table via the transaction-native
// RecordActionReceipt API. The collection and id parameters are ignored —
// the receipts table is keyed by transaction_id embedded in the record.
func (ass *SQLAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	if ass == nil || ass.db == nil {
		return constants.ErrAuditStoreDisabled
	}
	var receipt models.ActionReceiptRecord
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("%w: SQLAuditStore.DocSet: failed to decode action receipt record: %w", constants.ErrInvalidJSONBody, err)
	}
	return ass.RecordActionReceipt(&receipt)
}

// DocDelete implements governance.TransactionAuditStore for outbound mode.
// The outbound operator does not persist governed documents — document
// mutations are a gateway-side concern. This is a no-op that returns nil.
func (ass *SQLAuditStore) DocDelete(collection, id string) error {
	return nil
}

// GetActionReceipt retrieves a single action receipt by transaction ID.
func (ass *SQLAuditStore) GetActionReceipt(transactionID string) (*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	query := `
	SELECT transaction_id, transaction_hash, investigation_id, operator_id, operator_session_id,
		requestor_user_id, acting_app_id, event_type,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, receipt_json, timestamp
	FROM receipts
	WHERE transaction_id = ?
	`

	var r models.ActionReceiptRecord
	var executedAtMs int64
	var timestampStr string
	var investigationID sql.NullString
	var sessionID sql.NullString
	var receiptJSON sql.NullString
	err := ass.db.QueryRowWithRetry(query, transactionID).Scan(
		&r.TransactionID, &r.TransactionHash, &investigationID, &r.OperatorID, &sessionID,
		&r.RequestorUserID, &r.ActingAppID, &r.EventType,
		&r.ActionType, &r.TargetResource, &r.Status, &r.ResultSummary,
		&r.StateRootBefore, &r.StateRootAfter, &executedAtMs,
		&r.SignerKeyID, &r.Signature, &receiptJSON, &timestampStr,
	)
	r.InvestigationID = investigationID.String
	r.OperatorSessionID = sessionID.String
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreGetReceiptFailed, err)
	}

	r.ExecutedAt = time.UnixMilli(executedAtMs)
	r.Timestamp, _ = timesvc.ParseTimestamp(timestampStr)
	r.ActionReceipt, err = parseStoredActionReceipt(receiptJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: unmarshal canonical receipt: %w", constants.ErrAuditStoreGetReceiptFailed, err)
	}

	return &r, nil
}

func (ass *SQLAuditStore) GetActionReceiptByInvestigationID(
	investigationID string,
	actionType constants.ActionType,
) (*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	rows, err := sqliteutil.MaterializeRows(
		ass.db,
		"SELECT transaction_id FROM receipts WHERE investigation_id = ? AND action_type = ? ORDER BY timestamp DESC LIMIT 2",
		[]interface{}{investigationID, actionType},
		func(row *sql.Rows) (string, error) {
			var transactionID string
			if err := row.Scan(&transactionID); err != nil {
				return "", err
			}
			return transactionID, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: investigation correlation: %w", constants.ErrAuditStoreGetReceiptFailed, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > 1 {
		return nil, constants.ErrAuditStoreReceiptCorrelationDuplicate
	}
	return ass.GetActionReceipt(rows[0])
}

// ListActionReceipts retrieves action receipts with optional filtering and pagination.
func (ass *SQLAuditStore) ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	if limit <= 0 {
		limit = 50
	}

	var query strings.Builder
	query.WriteString(`
	SELECT transaction_id, transaction_hash, investigation_id, operator_id, operator_session_id,
		requestor_user_id, acting_app_id, event_type,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, receipt_json, timestamp
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
		record          models.ActionReceiptRecord
		executedAtMs    int64
		timestampStr    string
		investigationID sql.NullString
		sessionID       sql.NullString
		receiptJSON     sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query.String(), args, func(r *sql.Rows) (receiptRow, error) {
		var row receiptRow
		err := r.Scan(
			&row.record.TransactionID, &row.record.TransactionHash, &row.investigationID, &row.record.OperatorID, &row.sessionID,
			&row.record.RequestorUserID, &row.record.ActingAppID, &row.record.EventType,
			&row.record.ActionType, &row.record.TargetResource, &row.record.Status, &row.record.ResultSummary,
			&row.record.StateRootBefore, &row.record.StateRootAfter, &row.executedAtMs,
			&row.record.SignerKeyID, &row.record.Signature, &row.receiptJSON, &row.timestampStr,
		)
		row.record.InvestigationID = row.investigationID.String
		row.record.OperatorSessionID = row.sessionID.String
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryReceiptsFailed, err)
	}

	var results []*models.ActionReceiptRecord
	for _, row := range rows {
		row.record.ExecutedAt = time.UnixMilli(row.executedAtMs)
		row.record.Timestamp, _ = timesvc.ParseTimestamp(row.timestampStr)
		row.record.ActionReceipt, err = parseStoredActionReceipt(row.receiptJSON)
		if err != nil {
			return nil, fmt.Errorf("%w: unmarshal canonical receipt: %w", constants.ErrAuditStoreQueryReceiptsFailed, err)
		}
		results = append(results, &row.record)
	}

	return results, nil
}

// ListActionReceiptsSince retrieves action receipts newer than the given timestamp.
func (ass *SQLAuditStore) ListActionReceiptsSince(since time.Time, limit int) ([]*models.ActionReceiptRecord, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT transaction_id, transaction_hash, investigation_id, operator_id, operator_session_id,
		requestor_user_id, acting_app_id, event_type,
		action_type, target_resource, status, result_summary,
		state_root_before, state_root_after, executed_at_ms,
		signer_key_id, signature, receipt_json, timestamp
	FROM receipts
	WHERE timestamp > ?
	ORDER BY timestamp ASC
	LIMIT ?
	`

	type receiptRow struct {
		record          models.ActionReceiptRecord
		executedAtMs    int64
		timestampStr    string
		investigationID sql.NullString
		sessionID       sql.NullString
		receiptJSON     sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{timesvc.FormatTimestamp(since), limit}, func(r *sql.Rows) (receiptRow, error) {
		var row receiptRow
		err := r.Scan(
			&row.record.TransactionID, &row.record.TransactionHash, &row.investigationID, &row.record.OperatorID, &row.sessionID,
			&row.record.RequestorUserID, &row.record.ActingAppID, &row.record.EventType,
			&row.record.ActionType, &row.record.TargetResource, &row.record.Status, &row.record.ResultSummary,
			&row.record.StateRootBefore, &row.record.StateRootAfter, &row.executedAtMs,
			&row.record.SignerKeyID, &row.record.Signature, &row.receiptJSON, &row.timestampStr,
		)
		row.record.InvestigationID = row.investigationID.String
		row.record.OperatorSessionID = row.sessionID.String
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("%w %v: %w", constants.ErrAuditStoreQueryReceiptsSinceFailed, since, err)
	}

	var results []*models.ActionReceiptRecord
	for _, row := range rows {
		row.record.ExecutedAt = time.UnixMilli(row.executedAtMs)
		row.record.Timestamp, _ = timesvc.ParseTimestamp(row.timestampStr)
		row.record.ActionReceipt, err = parseStoredActionReceipt(row.receiptJSON)
		if err != nil {
			return nil, fmt.Errorf("%w: unmarshal canonical receipt: %w", constants.ErrAuditStoreQueryReceiptsSinceFailed, err)
		}
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
		return nil, constants.ErrAuditStoreDisabled
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
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryEventsFailed, err)
	}

	var events []*Event
	for _, row := range rows {
		row.event.Timestamp, _ = timesvc.ParseTimestamp(row.timestampStr)

		if row.encryptedFlag == 1 && ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked() {
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
			row.event.CommandExitCode = int(row.commandExitCode.Int64)
		} else {
			row.event.CommandExitCode = constants.ExitCodeNone
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
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreRecordFileMutationFailed, err)
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
		return nil, constants.ErrAuditStoreDisabled
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
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryFileMutationsFailed, err)
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

// ListSessions retrieves all sessions with pagination, ordered by created_at ASC.
func (ass *SQLAuditStore) ListSessions(limit, offset int) ([]*OperatorSession, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT id, title, session_type, created_at, user_identity
	FROM sessions
	ORDER BY created_at ASC
	LIMIT ? OFFSET ?
	`

	type sessionRow struct {
		session      OperatorSession
		sessionType  sql.NullString
		title        sql.NullString
		userIdentity sql.NullString
		createdAtStr string
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{limit, offset}, func(r *sql.Rows) (sessionRow, error) {
		var row sessionRow
		err := r.Scan(&row.session.ID, &row.title, &row.sessionType, &row.createdAtStr, &row.userIdentity)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreGetSessionFailed, err)
	}

	var sessions []*OperatorSession
	for _, row := range rows {
		row.session.CreatedAt, _ = timesvc.ParseTimestamp(row.createdAtStr)
		if row.sessionType.Valid {
			row.session.SessionType = row.sessionType.String
		}
		if row.title.Valid {
			row.session.Title = row.title.String
		}
		if row.userIdentity.Valid {
			row.session.UserIdentity = row.userIdentity.String
		}
		sessions = append(sessions, &row.session)
	}

	return sessions, nil
}

// ListEvents retrieves events with optional session filter and pagination, ordered by timestamp ASC.
// When sessionID is empty, all events across all sessions are returned.
func (ass *SQLAuditStore) ListEvents(sessionID string, limit, offset int) ([]*Event, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	if limit <= 0 {
		limit = 100
	}

	var query strings.Builder
	query.WriteString(`
	SELECT id, operator_session_id, timestamp, type, content_text,
		command_raw, command_exit_code, command_stdout, command_stderr,
		execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated,
		COALESCE(encrypted, 0) as encrypted
	FROM events
	`)

	args := []interface{}{}
	if sessionID != "" {
		query.WriteString(" WHERE operator_session_id = ?")
		args = append(args, sessionID)
	}
	query.WriteString(" ORDER BY timestamp ASC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

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

	rows, err := sqliteutil.MaterializeRows(ass.db, query.String(), args, func(r *sql.Rows) (eventRow, error) {
		var row eventRow
		err := r.Scan(
			&row.event.ID, &row.event.OperatorSessionID, &row.timestampStr, &row.event.Type,
			&row.contentTextBytes, &row.commandRaw, &row.commandExitCode,
			&row.commandStdoutBytes, &row.commandStderrBytes, &row.event.ExecutionDurationMs,
			&row.storedLocally, &row.stdoutTruncated, &row.stderrTruncated, &row.encryptedFlag,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryEventsFailed, err)
	}

	var events []*Event
	for _, row := range rows {
		row.event.Timestamp, _ = timesvc.ParseTimestamp(row.timestampStr)

		if row.encryptedFlag == 1 && ass.encryptionVault != nil && ass.encryptionVault.IsUnlocked() {
			if len(row.contentTextBytes) > 0 {
				if dec, err := ass.decryptContent(row.contentTextBytes); err == nil {
					row.event.ContentText = dec
				}
			}
			if len(row.commandStdoutBytes) > 0 {
				if dec, err := ass.decryptContent(row.commandStdoutBytes); err == nil {
					row.event.CommandStdout = dec
				}
			}
			if len(row.commandStderrBytes) > 0 {
				if dec, err := ass.decryptContent(row.commandStderrBytes); err == nil {
					row.event.CommandStderr = dec
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
			row.event.CommandExitCode = int(row.commandExitCode.Int64)
		} else {
			row.event.CommandExitCode = constants.ExitCodeNone
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

// ListFileMutations retrieves file mutations with pagination, ordered by id ASC.
func (ass *SQLAuditStore) ListFileMutations(limit, offset int) ([]*FileMutationLog, error) {
	if ass == nil || ass.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	FROM file_mutation_log
	ORDER BY id ASC
	LIMIT ? OFFSET ?
	`

	type mutationRow struct {
		mutation   FileMutationLog
		hashBefore sql.NullString
		hashAfter  sql.NullString
		diffStat   sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(ass.db, query, []interface{}{limit, offset}, func(r *sql.Rows) (mutationRow, error) {
		var row mutationRow
		err := r.Scan(
			&row.mutation.ID, &row.mutation.EventID, &row.mutation.Filepath, &row.mutation.Operation,
			&row.hashBefore, &row.hashAfter, &row.diffStat,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryFileMutationsFailed, err)
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
		cutoff := timesvc.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		// 1. Delete file mutations for old events first (satisfy FK constraints)
		_, err := db.ExecWithRetry(`
			DELETE FROM file_mutation_log
			WHERE event_id IN (SELECT id FROM events WHERE timestamp < ?)
		`, cutoff)
		if err != nil {
			logger.Error("Failed to prune old file mutations", string(constants.ConnectionStateError), err)
			return err
		}

		// 2. Checkpoint and delete chained events older than retention period
		if err := PruneChainedAuditEvents(ctx, db, logger, cutoff); err != nil {
			logger.Error("Failed to prune chained audit events", string(constants.ConnectionStateError), err)
			return err
		}

		// 3. Delete receipts older than retention period
		result, err := db.ExecWithRetry("DELETE FROM receipts WHERE timestamp < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old receipts", string(constants.ConnectionStateError), err)
			return err
		}
		rowsDeleted, _ := result.RowsAffected()
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

// GetDataDir returns the audit store data directory
func (ass *SQLAuditStore) GetDataDir() string {
	if ass == nil || ass.fileSvc == nil {
		return ""
	}
	return ass.fileSvc.Resolve(constants.DataDirname)
}

// encryptContent encrypts content using the encryption vault.
// Vault is required and must be unlocked. Returns error if vault is locked (fail-closed).
func (ass *SQLAuditStore) encryptContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}

	if !ass.encryptionVault.IsUnlocked() {
		return nil, constants.ErrAuditStoreVaultLocked
	}

	encrypted, err := ass.encryptionVault.Encrypt([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptFailed, err)
	}

	return encrypted, nil
}

// decryptContent decrypts content using the encryption vault
func (ass *SQLAuditStore) decryptContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if !ass.encryptionVault.IsUnlocked() {
		return "", constants.ErrAuditStoreVaultLocked
	}

	decrypted, err := ass.encryptionVault.Decrypt(data)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrAuditStoreDecryptFailed, err)
	}

	return string(decrypted), nil
}
