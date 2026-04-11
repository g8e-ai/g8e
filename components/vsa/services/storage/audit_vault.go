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
	"bytes"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/vsa/services/vault"
)

// AuditVaultConfig holds configuration for the Local-First Audit Architecture
type AuditVaultConfig struct {
	DataDir                   string
	DBPath                    string
	LedgerDir                 string
	MaxDBSizeMB               int64
	RetentionDays             int
	PruneIntervalMinutes      int
	Enabled                   bool
	OutputTruncationThreshold int
	HeadTailSize              int
	// EncryptionVault is the optional vault.Vault for encrypting sensitive content fields.
	// When set, content_text, command_stdout, and command_stderr are encrypted at rest.
	EncryptionVault *vault.Vault
	// GitPath is the resolved path to the git binary. Empty string means git is unavailable.
	GitPath string
}

// DefaultAuditVaultConfig returns the default configuration for the audit vault.
func DefaultAuditVaultConfig() *AuditVaultConfig {
	return &AuditVaultConfig{
		DataDir:                   "./.g8e/data",
		DBPath:                    "g8e.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
}

// EventType represents the type of event in the audit log
type EventType string

const (
	EventTypeUserMsg      EventType = "USER_MSG"
	EventTypeAIMsg        EventType = "AI_MSG"
	EventTypeCmdExec      EventType = "CMD_EXEC"
	EventTypeFileMutation EventType = "FILE_MUTATION"
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
	Type                EventType
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

// AuditVaultService provides the Local-First Audit Architecture implementation
type AuditVaultService struct {
	db              *sqliteutil.DB
	config          *AuditVaultConfig
	logger          *slog.Logger
	ledgerPath      string
	filesPath       string
	gitPath         string
	encryptionVault *vault.Vault
	pruner          *sqliteutil.Pruner
	closeOnce       sync.Once
}

// NewAuditVaultService creates a new audit vault service
func NewAuditVaultService(config *AuditVaultConfig, logger *slog.Logger) (*AuditVaultService, error) {
	if config == nil {
		config = DefaultAuditVaultConfig()
	}

	if !config.Enabled {
		logger.Info("Audit vault is disabled")
		return nil, nil
	}

	avs := &AuditVaultService{
		config:          config,
		logger:          logger,
		ledgerPath:      filepath.Join(config.DataDir, config.LedgerDir),
		filesPath:       filepath.Join(config.DataDir, config.LedgerDir, "files"),
		encryptionVault: config.EncryptionVault,
		gitPath:         config.GitPath,
	}

	if err := avs.bootstrap(); err != nil {
		return nil, fmt.Errorf("audit vault bootstrap failed: %w", err)
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	avs.pruner = sqliteutil.NewPruner(avs.db, logger, interval, auditVaultPrune(config))
	avs.pruner.Start()

	encryptionEnabled := avs.encryptionVault != nil && avs.encryptionVault.IsUnlocked()
	avs.logger.Info("Audit vault initialized",
		"data_dir", config.DataDir,
		"db_path", filepath.Join(config.DataDir, config.DBPath),
		"ledger_path", avs.ledgerPath,
		"encryption_enabled", encryptionEnabled)

	return avs, nil
}

// bootstrap initializes the audit vault (directory structure, database, git repo)
func (avs *AuditVaultService) bootstrap() error {
	avs.logger.Info("Bootstrapping audit vault", "data_dir", avs.config.DataDir)

	if err := avs.createDirectoryStructure(); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	if err := avs.verifyWritePermissions(); err != nil {
		return fmt.Errorf("FATAL: storage not writable (zero tolerance for data loss risk): %w", err)
	}

	if avs.gitPath != "" {
		if err := avs.initLedgerGit(); err != nil {
			return fmt.Errorf("failed to initialize ledger git repository: %w", err)
		}
	} else {
		avs.logger.Warn("Git not available — ledger git repository will not be initialized")
	}

	if err := avs.initDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	avs.logger.Info("Audit vault bootstrap completed successfully")
	return nil
}

// createDirectoryStructure creates the audit vault directory structure
func (avs *AuditVaultService) createDirectoryStructure() error {
	dirs := []string{
		avs.config.DataDir,
		avs.ledgerPath,
		avs.filesPath,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		avs.logger.Info("Directory ensured", "path", dir)
	}

	return nil
}

// verifyWritePermissions ensures the data directory is writable
func (avs *AuditVaultService) verifyWritePermissions() error {
	testFile := filepath.Join(avs.config.DataDir, ".write_test")

	if err := os.WriteFile(testFile, []byte("write_test"), 0600); err != nil {
		return fmt.Errorf("cannot write to %s: %w", avs.config.DataDir, err)
	}

	if err := os.Remove(testFile); err != nil {
		avs.logger.Warn("Failed to remove write test file", "path", testFile, "error", err)
	}

	avs.logger.Info("Write permissions verified", "path", avs.config.DataDir)
	return nil
}

// initLedgerGit initializes git repository in the ledger directory
func (avs *AuditVaultService) initLedgerGit() error {
	gitDir := filepath.Join(avs.ledgerPath, ".git")

	if _, err := os.Stat(gitDir); err == nil {
		avs.logger.Info("Ledger git repository already initialized", "path", avs.ledgerPath)
		return nil
	}

	if err := avs.gitExec("init"); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	if err := avs.gitExec("config", "user.name", "g8e Operator"); err != nil {
		return fmt.Errorf("git config user.name failed: %w", err)
	}
	if err := avs.gitExec("config", "user.email", "operator@"+constants.DefaultEndpoint); err != nil {
		return fmt.Errorf("git config user.email failed: %w", err)
	}

	gitignore := filepath.Join(avs.ledgerPath, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# g8e Ledger\n"), 0644); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	if err := avs.gitExec("add", "-A"); err != nil {
		return fmt.Errorf("failed to git add: %w", err)
	}

	if err := avs.gitExec("commit", "-m", "Initial ledger commit", "--allow-empty"); err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	avs.logger.Info("Ledger git repository initialized", "path", avs.ledgerPath)
	return nil
}

// initDatabase creates the database and schema
func (avs *AuditVaultService) initDatabase() error {
	dbPath := filepath.Join(avs.config.DataDir, avs.config.DBPath)

	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, avs.logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.RunMigrations(auditVaultMigrations); err != nil {
		db.Close()
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	avs.db = db

	avs.logger.Info("Database schema migrations completed")
	return nil
}

// auditVaultMigrations defines the schema evolution for the audit vault database.
var auditVaultMigrations = []sqliteutil.Migration{
	{
		Version:     1,
		Description: "Initial schema: sessions, events, file_mutation_log tables",
		SQL: `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			title TEXT,
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

		CREATE INDEX IF NOT EXISTS idx_events_session_id ON events(operator_session_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_file_mutation_event_id ON file_mutation_log(event_id);
		CREATE INDEX IF NOT EXISTS idx_file_mutation_filepath ON file_mutation_log(filepath);
		`,
	},
}

// CreateSession creates a new session in the audit log
func (avs *AuditVaultService) CreateSession(id, title, userIdentity string) error {
	if avs == nil || avs.db == nil {
		return nil
	}

	query := `INSERT INTO sessions (id, title, user_identity) VALUES (?, ?, ?)`
	_, err := avs.db.Exec(query, id, title, userIdentity)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	avs.logger.Info("OperatorSession created", "operator_session_id", id, "title", title)
	return nil
}

// GetSession retrieves a session by ID
func (avs *AuditVaultService) GetSession(id string) (*OperatorSession, error) {
	if avs == nil || avs.db == nil {
		return nil, fmt.Errorf("audit vault is disabled")
	}

	query := `SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?`
	row := avs.db.QueryRow(query, id)

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

// RecordEvent records an event in the audit log
// Content fields are encrypted if an encryption vault is configured and unlocked
func (avs *AuditVaultService) RecordEvent(event *Event) (int64, error) {
	if avs == nil || avs.db == nil {
		return 0, nil
	}

	stdout, stdoutTruncated := avs.truncateOutput(event.CommandStdout)
	stderr, stderrTruncated := avs.truncateOutput(event.CommandStderr)

	encrypted := avs.IsEncryptionEnabled()

	contentTextBytes, err := avs.encryptContent(event.ContentText)
	if err != nil {
		return 0, fmt.Errorf("failed to encrypt content_text: %w", err)
	}

	stdoutBytes, err := avs.encryptContent(stdout)
	if err != nil {
		return 0, fmt.Errorf("failed to encrypt stdout: %w", err)
	}

	stderrBytes, err := avs.encryptContent(stderr)
	if err != nil {
		return 0, fmt.Errorf("failed to encrypt stderr: %w", err)
	}

	// Use transaction for atomicity
	tx, err := avs.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}

	// Upsert session to satisfy FK constraint: sessions may not be pre-created for direct terminal commands.
	if event.OperatorSessionID != "" {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO sessions (id) VALUES (?)`,
			event.OperatorSessionID,
		); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to ensure session exists: %w", err)
		}
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
		string(event.Type),
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
		tx.Rollback()
		return 0, fmt.Errorf("failed to record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	eventID, _ := result.LastInsertId()
	avs.logger.Info("Event recorded",
		"event_id", eventID,
		"type", event.Type,
		"operator_session_id", event.OperatorSessionID,
		"stdout_truncated", stdoutTruncated,
		"stderr_truncated", stderrTruncated,
		"encrypted", encrypted)

	return eventID, nil
}

// truncateOutput applies the head/tail truncation strategy for large outputs
func (avs *AuditVaultService) truncateOutput(output string) (string, bool) {
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
func (avs *AuditVaultService) GetEvents(operatorSessionID string, limit, offset int) ([]*Event, error) {
	if avs == nil || avs.db == nil {
		return nil, fmt.Errorf("audit vault is disabled")
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

	rows, err := avs.db.Query(query, operatorSessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var timestampStr string
		var contentTextBytes, commandStdoutBytes, commandStderrBytes []byte
		var commandRaw sql.NullString
		var commandExitCode sql.NullInt64
		var storedLocally, stdoutTruncated, stderrTruncated sql.NullBool
		var encryptedFlag int

		err := rows.Scan(
			&event.ID,
			&event.OperatorSessionID,
			&timestampStr,
			&event.Type,
			&contentTextBytes,
			&commandRaw,
			&commandExitCode,
			&commandStdoutBytes,
			&commandStderrBytes,
			&event.ExecutionDurationMs,
			&storedLocally,
			&stdoutTruncated,
			&stderrTruncated,
			&encryptedFlag,
		)
		if err != nil {
			avs.logger.Warn("Failed to scan event row", "error", err)
			continue
		}

		event.Timestamp, _ = sqliteutil.ParseTimestamp(timestampStr)

		if encryptedFlag == 1 && avs.IsEncryptionEnabled() {
			if len(contentTextBytes) > 0 {
				decrypted, err := avs.decryptContent(contentTextBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt content_text", "event_id", event.ID, "error", err)
				} else {
					event.ContentText = decrypted
				}
			}
			if len(commandStdoutBytes) > 0 {
				decrypted, err := avs.decryptContent(commandStdoutBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt stdout", "event_id", event.ID, "error", err)
				} else {
					event.CommandStdout = decrypted
				}
			}
			if len(commandStderrBytes) > 0 {
				decrypted, err := avs.decryptContent(commandStderrBytes)
				if err != nil {
					avs.logger.Warn("Failed to decrypt stderr", "event_id", event.ID, "error", err)
				} else {
					event.CommandStderr = decrypted
				}
			}
		} else {
			event.ContentText = string(contentTextBytes)
			event.CommandStdout = string(commandStdoutBytes)
			event.CommandStderr = string(commandStderrBytes)
		}

		if commandRaw.Valid {
			event.CommandRaw = commandRaw.String
		}
		if commandExitCode.Valid {
			exitCode := int(commandExitCode.Int64)
			event.CommandExitCode = &exitCode
		}
		if storedLocally.Valid {
			event.StoredLocally = storedLocally.Bool
		}
		if stdoutTruncated.Valid {
			event.StdoutTruncated = stdoutTruncated.Bool
		}
		if stderrTruncated.Valid {
			event.StderrTruncated = stderrTruncated.Bool
		}

		events = append(events, &event)
	}

	return events, rows.Err()
}

// RecordFileMutation records a file mutation in the audit log
func (avs *AuditVaultService) RecordFileMutation(mutation *FileMutationLog) error {
	if avs == nil || avs.db == nil {
		return nil
	}

	query := `
	INSERT INTO file_mutation_log (
		event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := avs.db.Exec(query,
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

	avs.logger.Info("File mutation recorded",
		"event_id", mutation.EventID,
		"filepath", mutation.Filepath,
		"operation", mutation.Operation)

	return nil
}

// GetFileMutations retrieves file mutations for an event
func (avs *AuditVaultService) GetFileMutations(eventID int64) ([]*FileMutationLog, error) {
	if avs == nil || avs.db == nil {
		return nil, fmt.Errorf("audit vault is disabled")
	}

	query := `
	SELECT id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat
	FROM file_mutation_log
	WHERE event_id = ?
	`

	rows, err := avs.db.Query(query, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to query file mutations: %w", err)
	}
	defer rows.Close()

	var mutations []*FileMutationLog
	for rows.Next() {
		var mutation FileMutationLog
		var hashBefore, hashAfter, diffStat sql.NullString

		err := rows.Scan(
			&mutation.ID,
			&mutation.EventID,
			&mutation.Filepath,
			&mutation.Operation,
			&hashBefore,
			&hashAfter,
			&diffStat,
		)
		if err != nil {
			avs.logger.Warn("Failed to scan file mutation row", "error", err)
			continue
		}

		if hashBefore.Valid {
			mutation.LedgerHashBefore = hashBefore.String
		}
		if hashAfter.Valid {
			mutation.LedgerHashAfter = hashAfter.String
		}
		if diffStat.Valid {
			mutation.DiffStat = diffStat.String
		}

		mutations = append(mutations, &mutation)
	}

	return mutations, rows.Err()
}

// gitExec runs a git command in the ledger directory and returns an error if it fails
func (avs *AuditVaultService) gitExec(args ...string) error {
	if avs.gitPath == "" {
		return fmt.Errorf("git not available")
	}
	cmd := exec.Command(avs.gitPath, args...)
	cmd.Dir = avs.ledgerPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitOutput runs a git command in the ledger directory and returns stdout
func (avs *AuditVaultService) gitOutput(args ...string) (string, error) {
	if avs.gitPath == "" {
		return "", fmt.Errorf("git not available")
	}
	cmd := exec.Command(avs.gitPath, args...)
	cmd.Dir = avs.ledgerPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitGetCurrentHash gets the current HEAD commit hash
func (avs *AuditVaultService) gitGetCurrentHash() (string, error) {
	return avs.gitOutput("rev-parse", "HEAD")
}

// GetLedgerGitDir returns the ledger path for use by LedgerMirrorService
func (avs *AuditVaultService) GetLedgerGitDir() string {
	return avs.ledgerPath
}

// GetGitPath returns the resolved git binary path
func (avs *AuditVaultService) GetGitPath() string {
	if avs == nil {
		return ""
	}
	return avs.gitPath
}

// IsGitAvailable returns whether a functional git binary is available
func (avs *AuditVaultService) IsGitAvailable() bool {
	return avs != nil && avs.gitPath != ""
}

// auditVaultPrune returns a PruneFunc that handles retention pruning
// for events, orphaned sessions, and orphaned file mutations.
func auditVaultPrune(config *AuditVaultConfig) sqliteutil.PruneFunc {
	return func(db *sqliteutil.DB, logger *slog.Logger) {
		cutoff := sqliteutil.FormatTimestamp(time.Now().AddDate(0, 0, -config.RetentionDays))

		// 1. Delete file mutations for old events first (satisfy FK constraints)
		_, err := db.Exec(`
			DELETE FROM file_mutation_log
			WHERE event_id IN (SELECT id FROM events WHERE timestamp < ?)
		`, cutoff)
		if err != nil {
			logger.Error("Failed to prune old file mutations", "error", err)
		}

		// 2. Delete events older than retention period
		result, err := db.Exec("DELETE FROM events WHERE timestamp < ?", cutoff)
		if err != nil {
			logger.Error("Failed to prune old events", "error", err)
			return
		}

		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			logger.Info("Pruned old events", "rows_deleted", rowsDeleted)
		}

		// 3. Delete sessions that no longer have any events
		_, err = db.Exec(`
			DELETE FROM sessions
			WHERE id NOT IN (SELECT DISTINCT operator_session_id FROM events WHERE operator_session_id IS NOT NULL)
		`)
		if err != nil {
			logger.Warn("Failed to prune orphaned sessions", "error", err)
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", "error", err)
		}
	}
}

// GetEncryptionVault returns the optional encryption vault used by this service.
func (avs *AuditVaultService) GetEncryptionVault() *vault.Vault {
	if avs == nil {
		return nil
	}
	return avs.encryptionVault
}

// Close shuts down the audit vault service. Idempotent.
func (avs *AuditVaultService) Close() error {
	if avs == nil {
		return nil
	}

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

// IsEnabled returns whether the audit vault is enabled
func (avs *AuditVaultService) IsEnabled() bool {
	return avs != nil && avs.db != nil
}

// GetDataDir returns the audit vault data directory
func (avs *AuditVaultService) GetDataDir() string {
	if avs == nil {
		return ""
	}
	return avs.config.DataDir
}

// GetLedgerPath returns the ledger directory path
func (avs *AuditVaultService) GetLedgerPath() string {
	if avs == nil {
		return ""
	}
	return avs.ledgerPath
}

// IsEncryptionEnabled returns whether content encryption is enabled
func (avs *AuditVaultService) IsEncryptionEnabled() bool {
	return avs != nil && avs.encryptionVault != nil && avs.encryptionVault.IsUnlocked()
}

// encryptContent encrypts content if encryption is enabled, otherwise returns original
func (avs *AuditVaultService) encryptContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}

	if !avs.IsEncryptionEnabled() {
		return []byte(content), nil
	}

	encrypted, err := avs.encryptionVault.Encrypt([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt content: %w", err)
	}

	return encrypted, nil
}

// decryptContent decrypts content if encryption is enabled, otherwise returns original
func (avs *AuditVaultService) decryptContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if !avs.IsEncryptionEnabled() {
		return string(data), nil
	}

	decrypted, err := avs.encryptionVault.Decrypt(data)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt content: %w", err)
	}

	return string(decrypted), nil
}
