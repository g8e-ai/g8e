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

// Package storagetest provides test-only audit storage implementations.
// This file provides a test-only monolithic audit service with Git ledger integration.
// Production code uses SQLAuditStore (audit_store.go) instead.
package storagetest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/pathutil"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ChaosEvent represents a chaos test event (test-only).
type ChaosEvent struct {
	ID                int64
	OperatorSessionID string
	Timestamp         time.Time
	ChaosID           int
	Category          string
	Outcome           string
	ContentText       string
	CommandRaw        string
	TransactionHash   string
}

// TestSQLAuditStoreConfig holds configuration for the test-only Local-First Audit Architecture
type TestSQLAuditStoreConfig struct {
	DataDir                   string
	DBPath                    string
	LedgerDir                 string
	MaxDBSizeMB               int64
	RetentionDays             int
	PruneIntervalMinutes      int
	OutputTruncationThreshold int
	HeadTailSize              int
	// EncryptionVault is the required vault.Vault for encrypting sensitive content fields.
	// content_text, command_stdout, and command_stderr are encrypted at rest.
	EncryptionVault *vault.Vault
	// GitPath is the resolved path to the git binary. Empty string means git is unavailable.
	GitPath string
}

// DefaultTestSQLAuditStoreConfig returns the default configuration for the test audit store.
// Note: DataDir should be set by the caller based on the actual work directory.
func DefaultTestSQLAuditStoreConfig() *TestSQLAuditStoreConfig {
	return &TestSQLAuditStoreConfig{
		DataDir:                   ".g8e/data",
		DBPath:                    "g8e.db",
		LedgerDir:                 constants.LedgerDirname,
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
}

// TestSQLAuditStore provides the Local-First Audit Architecture implementation
type TestSQLAuditStore struct {
	db              *sqliteutil.DB
	config          *TestSQLAuditStoreConfig
	logger          *slog.Logger
	ledgerPath      string
	filesPath       string
	sessionsRoot    string // Root for session-specific ledgers
	gitPath         string
	encryptionVault *vault.Vault
	pruner          *sqliteutil.Pruner
	closeOnce       sync.Once

	mu       sync.RWMutex // Protects session ledger initialization
	muWrites sync.WaitGroup
}

// NewTestSQLAuditStore creates a new test-only audit store service
// EncryptionVault in config is required for encryption at rest.
func NewTestSQLAuditStore(config *TestSQLAuditStoreConfig, logger *slog.Logger) (*TestSQLAuditStore, error) {
	if config == nil {
		config = DefaultTestSQLAuditStoreConfig()
	}

	if config.EncryptionVault == nil {
		return nil, constants.ErrAuditStoreEncryptionVaultRequired
	}

	avs := &TestSQLAuditStore{
		config:          config,
		logger:          logger,
		ledgerPath:      pathutil.SafeJoin(config.DataDir, config.LedgerDir),
		filesPath:       pathutil.SafeJoin(config.DataDir, config.LedgerDir, "files"),
		sessionsRoot:    pathutil.SafeJoin(config.DataDir, config.LedgerDir, "sessions"),
		encryptionVault: config.EncryptionVault,
		gitPath:         config.GitPath,
	}

	if err := avs.bootstrap(); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreBootstrapFailed, err)
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	avs.pruner = sqliteutil.NewPruner(avs.db, logger, interval, auditVaultPrune(config))
	avs.pruner.Start()

	encryptionEnabled := avs.encryptionVault.IsUnlocked()
	avs.logger.Info("Audit vault initialized",
		"data_dir", config.DataDir,
		"db_path", pathutil.ResolveDBPath(config.DataDir, config.DBPath),
		"ledger_path", avs.ledgerPath,
		"encryption_enabled", encryptionEnabled)

	return avs, nil
}

// bootstrap initializes the audit vault (directory structure, database, git repo)
func (avs *TestSQLAuditStore) bootstrap() error {
	avs.logger.Info("Bootstrapping audit vault", "data_dir", avs.config.DataDir)

	if err := avs.createDirectoryStructure(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreCreateDirFailed, err)
	}

	if err := avs.verifyWritePermissions(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreNotWritable, err)
	}

	if avs.gitPath != "" {
		if err := avs.initLedgerGit(); err != nil {
			return fmt.Errorf("failed to initialize ledger git repository: %w", err)
		}
	} else {
		avs.logger.Warn("Git not available - ledger git repository will not be initialized")
	}

	if err := avs.initDatabase(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitDBFailed, err)
	}

	avs.logger.Info("Audit vault bootstrap completed successfully")
	return nil
}

// createDirectoryStructure creates the audit vault directory structure
func (avs *TestSQLAuditStore) createDirectoryStructure() error {
	dirs := []string{
		avs.config.DataDir,
		avs.ledgerPath,
		avs.filesPath,
		avs.sessionsRoot,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("%w %s: %w", constants.ErrAuditStoreCreateDirPathFailed, dir, err)
		}
	}

	avs.logger.Info("Audit vault directory structure ensured",
		"data_dir", avs.config.DataDir,
		"ledger_dir", avs.ledgerPath,
		"sessions_dir", avs.sessionsRoot)

	return nil
}

// verifyWritePermissions ensures the data directory is writable
func (avs *TestSQLAuditStore) verifyWritePermissions() error {
	testFile := pathutil.SafeJoin(avs.config.DataDir, ".write_test")

	if err := os.WriteFile(testFile, []byte("write_test"), 0600); err != nil {
		return fmt.Errorf("%w %s: %w", constants.ErrAuditStoreCannotWrite, avs.config.DataDir, err)
	}

	if err := os.Remove(testFile); err != nil {
		avs.logger.Warn("Failed to remove write test file", "path", testFile, string(constants.ConnectionStateError), err)
	}

	avs.logger.Info("Write permissions verified", "path", avs.config.DataDir)
	return nil
}

// GetSessionLedgerPath returns the ledger path for a specific session, initializing it if needed.
func (avs *TestSQLAuditStore) GetSessionLedgerPath(operatorSessionID string) (string, error) {
	if operatorSessionID == "" {
		return avs.ledgerPath, nil
	}

	sessionPath := filepath.Join(avs.sessionsRoot, operatorSessionID)

	avs.mu.RLock()
	_, err := os.Stat(pathutil.SafeJoin(sessionPath, ".git"))
	avs.mu.RUnlock()

	if err == nil {
		return sessionPath, nil
	}

	// Initialize new session ledger
	avs.mu.Lock()
	defer avs.mu.Unlock()

	// Double check
	if _, err := os.Stat(pathutil.SafeJoin(sessionPath, ".git")); err == nil {
		return sessionPath, nil
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create Operator session ledger directory: %w", err)
	}

	if err := avs.initGitRepo(sessionPath); err != nil {
		return "", fmt.Errorf("failed to initialize Operator session git repo: %w", err)
	}

	avs.logger.Info("Initialized new session ledger", "operator_session_id", operatorSessionID, "path", sessionPath)
	return sessionPath, nil
}

// initLedgerGit initializes git repository in the global ledger directory
func (avs *TestSQLAuditStore) initLedgerGit() error {
	return avs.initGitRepo(avs.ledgerPath)
}

// initGitRepo initializes a git repository in the specified directory using native go-git
func (avs *TestSQLAuditStore) initGitRepo(path string) error {
	gitDir := pathutil.SafeJoin(path, ".git")

	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}

	repo, err := git.PlainInit(path, false)
	if err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	gitignore := pathutil.SafeJoin(path, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# g8e Ledger\n"), 0600); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if _, err := w.Add(".gitignore"); err != nil {
		return fmt.Errorf("failed to git add .gitignore: %w", err)
	}

	_, err = w.Commit("Initial ledger commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "g8e-operator",
			Email: "g8e-operator@system",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}

// initDatabase creates the database and schema
func (avs *TestSQLAuditStore) initDatabase() error {
	dbPath := pathutil.ResolveDBPath(avs.config.DataDir, avs.config.DBPath)

	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, avs.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreOpenDBFailed, err)
	}

	if _, err := db.Exec(auditVaultSchema); err != nil {
		db.Close()
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreInitSchemaFailed, err)
	}

	avs.db = db

	avs.logger.Info("Database schema initialized")
	return nil
}

// auditVaultSchema defines the initial schema for the audit vault database.
const auditVaultSchema = `
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

CREATE TABLE IF NOT EXISTS chaos_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	operator_session_id TEXT,
	timestamp TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
	chaos_id INTEGER NOT NULL,
	category TEXT NOT NULL,
	outcome TEXT NOT NULL,
	content_text TEXT,
	command_raw TEXT,
	transaction_hash TEXT,
	FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_events_session_id ON events(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_file_mutation_event_id ON file_mutation_log(event_id);
CREATE INDEX IF NOT EXISTS idx_file_mutation_filepath ON file_mutation_log(filepath);
CREATE INDEX IF NOT EXISTS idx_receipts_session_id ON receipts(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_receipts_timestamp ON receipts(timestamp);
CREATE INDEX IF NOT EXISTS idx_chaos_events_session_id ON chaos_events(operator_session_id);
CREATE INDEX IF NOT EXISTS idx_chaos_events_timestamp ON chaos_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_chaos_events_category ON chaos_events(category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_id_type ON sessions(id, session_type);
`

// CreateSession creates a new session in the audit log
func (avs *TestSQLAuditStore) CreateSession(id, sessionType, title, userIdentity string) error {
	if avs == nil || avs.db == nil {
		return nil
	}
	if id == "" || strings.TrimSpace(id) != id {
		return constants.ErrAuditSessionMissing
	}
	if sessionType == "" {
		sessionType = string(constants.UserRoleOperator)
	}

	query := `INSERT INTO sessions (id, session_type, title, user_identity) VALUES (?, ?, ?, ?)`
	_, err := avs.db.ExecWithRetry(query, id, sessionType, title, userIdentity)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreCreateSessionFailed, err)
	}

	avs.logger.Info("OperatorSession created", "operator_session_id", id, "session_type", sessionType, "title", title)
	return nil
}

// GetOperatorSession retrieves a session by ID
func (avs *TestSQLAuditStore) GetOperatorSession(id string) (*storage.OperatorSession, error) {
	if avs == nil || avs.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	query := `SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?`
	row := avs.db.QueryRowWithRetry(query, id)

	var session storage.OperatorSession
	var title, userIdentity sql.NullString
	var createdAtStr string
	err := row.Scan(&session.ID, &title, &createdAtStr, &userIdentity)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreGetSessionFailed, err)
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

func (avs *TestSQLAuditStore) requireExistingSessionTx(tx *sql.Tx, event *storage.Event) error {
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

// RecordEvents records multiple events in a single database transaction.
func (avs *TestSQLAuditStore) RecordEvents(events []*storage.Event) error {
	if avs == nil || avs.db == nil || len(events) == 0 {
		return nil
	}

	avs.muWrites.Add(1)
	defer avs.muWrites.Done()

	return avs.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		query := `
		INSERT INTO events (
			operator_session_id, timestamp, type, content_text,
			command_raw, command_exit_code, command_stdout, command_stderr,
			execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		stmt, err := tx.Prepare(query)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditStorePrepareBatchFailed, err)
		}
		defer stmt.Close()

		for _, event := range events {
			if err := avs.requireExistingSessionTx(tx, event); err != nil {
				return err
			}

			stdout, stdoutTruncated := avs.truncateOutput(event.CommandStdout)
			stderr, stderrTruncated := avs.truncateOutput(event.CommandStderr)

			encryptedFlag := 0
			if avs.encryptionVault.IsUnlocked() {
				encryptedFlag = 1
			}

			contentTextBytes, err := avs.encryptContent(event.ContentText)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptContentFailed, err)
			}

			stdoutBytes, err := avs.encryptContent(stdout)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStdoutFailed, err)
			}

			stderrBytes, err := avs.encryptContent(stderr)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStderrFailed, err)
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
				return fmt.Errorf("%w: %w", constants.ErrAuditStoreExecuteBatchFailed, err)
			}
		}

		avs.logger.Info("Batch of events recorded", "count", len(events))
		return nil
	})
}

// RecordChaosEvent records a chaos test event in the chaos_events table
func (avs *TestSQLAuditStore) RecordChaosEvent(event *ChaosEvent) (int64, error) {
	if avs == nil || avs.db == nil {
		return 0, nil
	}

	query := `
	INSERT INTO chaos_events (
		operator_session_id, timestamp, chaos_id, category, outcome,
		content_text, command_raw, transaction_hash
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := avs.db.ExecWithRetry(query,
		event.OperatorSessionID,
		sqliteutil.FormatTimestamp(event.Timestamp),
		event.ChaosID,
		event.Category,
		event.Outcome,
		event.ContentText,
		event.CommandRaw,
		event.TransactionHash,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to record chaos event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get chaos event id: %w", err)
	}

	return id, nil
}

// RecordChaosEvents records multiple chaos events in a single database transaction
func (avs *TestSQLAuditStore) RecordChaosEvents(events []*ChaosEvent) error {
	if avs == nil || avs.db == nil {
		return nil
	}

	if len(events) == 0 {
		return nil
	}

	return avs.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		query := `
		INSERT INTO chaos_events (
			operator_session_id, timestamp, chaos_id, category, outcome,
			content_text, command_raw, transaction_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`

		stmt, err := tx.Prepare(query)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditStorePrepareBatchFailed, err)
		}
		defer stmt.Close()

		for _, event := range events {
			_, err := stmt.Exec(
				event.OperatorSessionID,
				sqliteutil.FormatTimestamp(event.Timestamp),
				event.ChaosID,
				event.Category,
				event.Outcome,
				event.ContentText,
				event.CommandRaw,
				event.TransactionHash,
			)
			if err != nil {
				return fmt.Errorf("failed to record chaos event: %w", err)
			}
		}

		return nil
	})
}

// RecordEvent records an event in the audit log
// Content fields are encrypted if an encryption vault is configured and unlocked
func (avs *TestSQLAuditStore) RecordEvent(event *storage.Event) (int64, error) {
	if avs == nil || avs.db == nil {
		return 0, nil
	}

	avs.muWrites.Add(1)
	defer avs.muWrites.Done()

	var eventID int64
	err := avs.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		if err := avs.requireExistingSessionTx(tx, event); err != nil {
			return err
		}

		stdout, stdoutTruncated := avs.truncateOutput(event.CommandStdout)
		stderr, stderrTruncated := avs.truncateOutput(event.CommandStderr)

		contentTextBytes, err := avs.encryptContent(event.ContentText)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptContentFailed, err)
		}

		stdoutBytes, err := avs.encryptContent(stdout)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStdoutFailed, err)
		}

		stderrBytes, err := avs.encryptContent(stderr)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptStderrFailed, err)
		}

		query := `
		INSERT INTO events (
			operator_session_id, timestamp, type, content_text,
			command_raw, command_exit_code, command_stdout, command_stderr,
			execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		encryptedFlag := 0
		if avs.encryptionVault.IsUnlocked() {
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
			return fmt.Errorf("%w: %w", constants.ErrAuditStoreRecordEventFailed, err)
		}

		id, _ := result.LastInsertId()
		eventID = id

		avs.logger.Info("Event recorded",
			"event_id", eventID,
			"type", event.Type,
			"operator_session_id", event.OperatorSessionID,
			"stdout_truncated", stdoutTruncated,
			"stderr_truncated", stderrTruncated,
			"encrypted", encryptedFlag)

		return nil
	})

	return eventID, err
}

// RecordActionReceipt records a signed ActionReceipt in the audit vault.
// This is the authoritative transaction-native audit record.
func (avs *TestSQLAuditStore) RecordActionReceipt(record *models.ActionReceiptRecord) error {
	if avs == nil || avs.db == nil {
		return nil
	}

	avs.muWrites.Add(1)
	defer avs.muWrites.Done()

	// Auto-create session row for FK satisfaction (matches production behavior)
	if record.OperatorSessionID != "" {
		_, _ = avs.db.ExecWithRetry(
			`INSERT OR IGNORE INTO sessions (id, session_type, title, user_identity) VALUES (?, 'operator', ?, ?)`,
			record.OperatorSessionID, record.OperatorSessionID, record.OperatorID,
		)
	}

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

	_, err := avs.db.ExecWithRetry(query,
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
		return fmt.Errorf("%w: %w", constants.ErrAuditStoreRecordReceiptFailed, err)
	}

	avs.logger.Info("ActionReceipt recorded",
		"transaction_id", record.TransactionID,
		"status", record.Status)

	return nil
}

// GetActionReceipt retrieves a single action receipt by transaction ID.
func (avs *TestSQLAuditStore) GetActionReceipt(transactionID string) (*models.ActionReceiptRecord, error) {
	if avs == nil || avs.db == nil {
		return nil, constants.ErrAuditStoreDisabled
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
	err := avs.db.QueryRowWithRetry(query, transactionID).Scan(
		&r.TransactionID, &r.TransactionHash, &r.OperatorID, &r.OperatorSessionID,
		&r.ActionType, &r.TargetResource, &r.Status, &r.ResultSummary,
		&r.StateRootBefore, &r.StateRootAfter, &executedAtMs,
		&r.SignerKeyID, &r.Signature, &timestampStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreGetReceiptFailed, err)
	}

	r.ExecutedAt = time.UnixMilli(executedAtMs)
	r.Timestamp, _ = sqliteutil.ParseTimestamp(timestampStr)

	return &r, nil
}

// ListActionReceipts retrieves action receipts with optional filtering and pagination.
func (avs *TestSQLAuditStore) ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error) {
	if avs == nil || avs.db == nil {
		return nil, constants.ErrAuditStoreDisabled
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

	rows, err := sqliteutil.MaterializeRows(avs.db, query.String(), args, func(r *sql.Rows) (receiptRow, error) {
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
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryReceiptsFailed, err)
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
func (avs *TestSQLAuditStore) ListActionReceiptsSince(since time.Time, limit int) ([]*models.ActionReceiptRecord, error) {
	if avs == nil || avs.db == nil {
		return nil, constants.ErrAuditStoreDisabled
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

	rows, err := sqliteutil.MaterializeRows(avs.db, query, []interface{}{sqliteutil.FormatTimestamp(since), limit}, func(r *sql.Rows) (receiptRow, error) {
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
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreQueryReceiptsSinceFailed, err)
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
func (avs *TestSQLAuditStore) truncateOutput(output string) (string, bool) {
	if len(output) <= avs.config.OutputTruncationThreshold {
		return output, false
	}

	headSize := avs.config.HeadTailSize
	tailSize := avs.config.HeadTailSize

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
func (avs *TestSQLAuditStore) GetEvents(operatorSessionID string, limit, offset int) ([]*storage.Event, error) {
	if avs == nil || avs.db == nil {
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
		event              storage.Event
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

	rows, err := sqliteutil.MaterializeRows(avs.db, query, []interface{}{operatorSessionID, limit, offset}, func(r *sql.Rows) (eventRow, error) {
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

	var events []*storage.Event
	for _, row := range rows {
		row.event.Timestamp, _ = sqliteutil.ParseTimestamp(row.timestampStr)

		if row.encryptedFlag == 1 && avs.encryptionVault.IsUnlocked() {
			if len(row.contentTextBytes) > 0 {
				decrypted, err := avs.decryptContent(row.contentTextBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt content_text", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
				} else {
					row.event.ContentText = decrypted
				}
			}
			if len(row.commandStdoutBytes) > 0 {
				decrypted, err := avs.decryptContent(row.commandStdoutBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt stdout", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
				} else {
					row.event.CommandStdout = decrypted
				}
			}
			if len(row.commandStderrBytes) > 0 {
				decrypted, err := avs.decryptContent(row.commandStderrBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt stderr", "event_id", row.event.ID, string(constants.ConnectionStateError), err)
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
func (avs *TestSQLAuditStore) RecordFileMutation(mutation *storage.FileMutationLog) error {
	if avs == nil || avs.db == nil {
		return nil
	}

	query := `
	INSERT INTO file_mutation_log (
		event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := avs.db.ExecWithRetry(query,
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

	avs.logger.Info("File mutation recorded",
		"event_id", mutation.EventID,
		"filepath", mutation.Filepath,
		"operation", mutation.Operation)

	return nil
}

// GetFileMutations retrieves file mutations for an event
func (avs *TestSQLAuditStore) GetFileMutations(eventID int64) ([]*storage.FileMutationLog, error) {
	if avs == nil || avs.db == nil {
		return nil, constants.ErrAuditStoreDisabled
	}

	query := `
	SELECT id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	FROM file_mutation_log
	WHERE event_id = ?
	`

	type mutationRow struct {
		mutation   storage.FileMutationLog
		hashBefore sql.NullString
		hashAfter  sql.NullString
		diffStat   sql.NullString
	}

	rows, err := sqliteutil.MaterializeRows(avs.db, query, []interface{}{eventID}, func(r *sql.Rows) (mutationRow, error) {
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

	var mutations []*storage.FileMutationLog
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

// GetLedgerGitDir returns the ledger path for use by LedgerMirrorService
func (avs *TestSQLAuditStore) GetLedgerGitDir() string {
	return avs.ledgerPath
}

// GetGitPath returns the resolved git binary path
func (avs *TestSQLAuditStore) GetGitPath() string {
	if avs == nil {
		return ""
	}
	return avs.gitPath
}

// IsGitAvailable returns whether a functional git binary is available
func (avs *TestSQLAuditStore) IsGitAvailable() bool {
	return avs != nil && avs.gitPath != ""
}

// auditVaultPrune returns a PruneFunc that handles retention pruning
// for events, orphaned sessions, and orphaned file mutations.
func auditVaultPrune(config *TestSQLAuditStoreConfig) sqliteutil.PruneFunc {
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
func (avs *TestSQLAuditStore) GetEncryptionVault() *vault.Vault {
	if avs == nil {
		return nil
	}
	return avs.encryptionVault
}

// Wait blocks until all in-flight writes have finished.
func (avs *TestSQLAuditStore) Wait() {
	if avs == nil {
		return
	}
	avs.muWrites.Wait()
}

// Close shuts down the audit vault service. Idempotent.
func (avs *TestSQLAuditStore) Close() error {
	if avs == nil {
		return nil
	}

	avs.Wait()

	var closeErr error
	avs.closeOnce.Do(func() {
		if avs.pruner != nil {
			avs.pruner.Stop()
		}
		if avs.db != nil {
			closeErr = avs.db.Close()
		}
	})

	return closeErr
}

// GetDataDir returns the audit vault data directory
func (avs *TestSQLAuditStore) GetDataDir() string {
	if avs == nil {
		return ""
	}
	return avs.config.DataDir
}

// GetLedgerPath returns the ledger directory path
func (avs *TestSQLAuditStore) GetLedgerPath() string {
	if avs == nil {
		return ""
	}
	return avs.ledgerPath
}

// encryptContent encrypts content. Vault is required and must be unlocked.
func (avs *TestSQLAuditStore) encryptContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}

	if !avs.encryptionVault.IsUnlocked() {
		return nil, constants.ErrAuditStoreVaultLocked
	}

	encrypted, err := avs.encryptionVault.Encrypt([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAuditStoreEncryptFailed, err)
	}

	return encrypted, nil
}

// decryptContent decrypts content. Vault is required and must be unlocked.
func (avs *TestSQLAuditStore) decryptContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if !avs.encryptionVault.IsUnlocked() {
		return "", constants.ErrAuditStoreVaultLocked
	}

	decrypted, err := avs.encryptionVault.Decrypt(data)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrAuditStoreDecryptFailed, err)
	}

	return string(decrypted), nil
}

// DocSet implements the TransactionAuditStore interface for test purposes.
// This is a no-op implementation for the chaos tester since TestSQLAuditStore
// is an audit store, not a document store. The actual audit data is stored
// via RecordActionReceipt in the receipts table.
func (avs *TestSQLAuditStore) DocSet(collection, id string, data json.RawMessage) error {
	// No-op for test infrastructure - audit data is stored via RecordActionReceipt
	avs.logger.Debug("DocSet called (no-op in test audit store)", "collection", collection, "id", id)
	return nil
}
