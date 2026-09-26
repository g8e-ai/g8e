// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

const (
	auditChainGenesisPrevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	auditChainHashVersion     = "g8e-audit-v1"
)

// AuditChainCheckpoint is a durable pruning anchor for chain verification.
type AuditChainCheckpoint struct {
	PrunedThroughSeq  int64
	PrunedThroughHash string
	PrunedCount       int64
	CheckpointHash    string
	CreatedAt         time.Time
}

type auditChainHead struct {
	Seq      int64
	PrevHash string
}

type auditChainCheckpointPayload struct {
	PrunedThroughSeq  int64  `json:"pruned_through_seq"`
	PrunedThroughHash string `json:"pruned_through_hash"`
	PrunedCount       int64  `json:"pruned_count"`
}

type eventContentDigestInput struct {
	ContentText         string `json:"content_text"`
	CommandRaw          string `json:"command_raw"`
	CommandExitCode     int    `json:"command_exit_code"`
	CommandStdout       string `json:"command_stdout"`
	CommandStderr       string `json:"command_stderr"`
	ExecutionDurationMs int64  `json:"execution_duration_ms"`
}

const auditChainCheckpointsSchema = `
CREATE TABLE IF NOT EXISTS audit_chain_checkpoints (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pruned_through_seq INTEGER NOT NULL,
	pruned_through_hash TEXT NOT NULL,
	pruned_count INTEGER NOT NULL,
	checkpoint_hash TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_chain_checkpoints_seq ON audit_chain_checkpoints(pruned_through_seq);
`

// MigrateEventChainColumns adds chain metadata columns, backfills existing rows,
// and records a genesis checkpoint when legacy events are chained.
func MigrateEventChainColumns(db *sqliteutil.DB, logger *slog.Logger, encryptionVault *vault.Vault) error {
	if db == nil {
		return fmt.Errorf("audit chain migration: database not initialized")
	}

	if _, err := db.Exec(auditChainCheckpointsSchema); err != nil {
		return fmt.Errorf("audit chain migration: checkpoints schema: %w", err)
	}

	cols, err := tableColumns(db, "events")
	if err != nil {
		return fmt.Errorf("audit chain migration: pragma events: %w", err)
	}

	for _, col := range []struct {
		name    string
		ddl     string
	}{
		{"seq", "ALTER TABLE events ADD COLUMN seq INTEGER"},
		{"prev_hash", "ALTER TABLE events ADD COLUMN prev_hash TEXT"},
		{"content_digest", "ALTER TABLE events ADD COLUMN content_digest TEXT"},
		{"hash", "ALTER TABLE events ADD COLUMN hash TEXT"},
		{"transaction_id", "ALTER TABLE events ADD COLUMN transaction_id TEXT"},
	} {
		if cols[col.name] {
			continue
		}
		if _, err := db.Exec(col.ddl); err != nil {
			return fmt.Errorf("audit chain migration: add column %s: %w", col.name, err)
		}
		if logger != nil {
			logger.Info("Audit chain migration: added column", "column", col.name)
		}
	}

	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_events_seq ON events(seq)"); err != nil {
		return fmt.Errorf("audit chain migration: create seq index: %w", err)
	}

	return backfillEventChain(db, logger, encryptionVault)
}

func tableColumns(db *sqliteutil.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryWithRetry(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, nil
}

func backfillEventChain(db *sqliteutil.DB, logger *slog.Logger, encryptionVault *vault.Vault) error {
	var pending int
	if err := db.QueryRowWithRetry(`SELECT COUNT(*) FROM events WHERE seq IS NULL`).Scan(&pending); err != nil {
		return fmt.Errorf("audit chain backfill: count pending: %w", err)
	}
	if pending == 0 {
		return nil
	}

	return db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(context.Background(), `
			SELECT id, operator_session_id, timestamp, type, content_text, command_raw,
				command_exit_code, command_stdout, command_stderr, execution_duration_ms,
				COALESCE(encrypted, 0), transaction_id
			FROM events
			WHERE seq IS NULL
			ORDER BY id ASC
		`)
		if err != nil {
			return fmt.Errorf("audit chain backfill: select: %w", err)
		}
		defer rows.Close()

		head, err := loadAuditChainHeadConn(context.Background(), conn)
		if err != nil {
			return err
		}

		backfilled := 0
		for rows.Next() {
			var (
				id                  int64
				sessionID           sql.NullString
				timestampStr        string
				eventType           string
				contentTextBytes    []byte
				commandRaw          sql.NullString
				commandExitCode     sql.NullInt64
				commandStdoutBytes  []byte
				commandStderrBytes  []byte
				executionDurationMs sql.NullInt64
				encryptedFlag       int
				transactionID       sql.NullString
			)
			if err := rows.Scan(
				&id, &sessionID, &timestampStr, &eventType, &contentTextBytes, &commandRaw,
				&commandExitCode, &commandStdoutBytes, &commandStderrBytes, &executionDurationMs,
				&encryptedFlag, &transactionID,
			); err != nil {
				return fmt.Errorf("audit chain backfill: scan: %w", err)
			}

			event := &Event{
				OperatorSessionID: sessionID.String,
				Type:              constants.EventType(eventType),
				TransactionID:     transactionID.String,
			}
			event.Timestamp, _ = timesvc.ParseTimestamp(timestampStr)
			if commandRaw.Valid {
				event.CommandRaw = commandRaw.String
			}
			if commandExitCode.Valid {
				event.CommandExitCode = int(commandExitCode.Int64)
			} else {
				event.CommandExitCode = constants.ExitCodeNone
			}
			if executionDurationMs.Valid {
				event.ExecutionDurationMs = executionDurationMs.Int64
			}

			if encryptedFlag == 1 && encryptionVault != nil && encryptionVault.IsUnlocked() {
				event.ContentText = decryptAuditBytes(encryptionVault, contentTextBytes)
				event.CommandStdout = decryptAuditBytes(encryptionVault, commandStdoutBytes)
				event.CommandStderr = decryptAuditBytes(encryptionVault, commandStderrBytes)
			} else {
				event.ContentText = string(contentTextBytes)
				event.CommandStdout = string(commandStdoutBytes)
				event.CommandStderr = string(commandStderrBytes)
			}

			nextSeq := head.Seq + 1
			contentDigest, err := computeEventContentDigest(event)
			if err != nil {
				return err
			}
			hash := computeAuditEventHash(
				nextSeq,
				head.PrevHash,
				string(event.Type),
				event.OperatorSessionID,
				timesvc.FormatTimestamp(event.Timestamp),
				contentDigest,
				event.TransactionID,
			)

			if _, err := conn.ExecContext(context.Background(), `
				UPDATE events
				SET seq = ?, prev_hash = ?, content_digest = ?, hash = ?
				WHERE id = ?
			`, nextSeq, head.PrevHash, contentDigest, hash, id); err != nil {
				return fmt.Errorf("audit chain backfill: update id %d: %w", id, err)
			}

			head = auditChainHead{Seq: nextSeq, PrevHash: hash}
			backfilled++
		}

		if backfilled > 0 {
			payload, err := json.Marshal(auditChainCheckpointPayload{
				PrunedThroughSeq:  0,
				PrunedThroughHash: auditChainGenesisPrevHash,
				PrunedCount:       int64(backfilled),
			})
			if err != nil {
				return fmt.Errorf("audit chain backfill: marshal genesis checkpoint: %w", err)
			}
			if _, err := conn.ExecContext(context.Background(), `
				INSERT INTO audit_chain_checkpoints (
					pruned_through_seq, pruned_through_hash, pruned_count, checkpoint_hash, created_at
				) VALUES (?, ?, ?, ?, ?)
			`, 0, auditChainGenesisPrevHash, backfilled, head.PrevHash, timesvc.FormatTimestamp(time.Now().UTC())); err != nil {
				return fmt.Errorf("audit chain backfill: genesis checkpoint: %w", err)
			}
			if logger != nil {
				logger.Info("Audit chain backfill completed",
					"events_chained", backfilled,
					"head_seq", head.Seq,
					"head_hash", head.PrevHash,
					"genesis_checkpoint_payload", string(payload))
			}
		}
		return nil
	})
}

func decryptAuditBytes(encryptionVault *vault.Vault, data []byte) string {
	if len(data) == 0 || encryptionVault == nil || !encryptionVault.IsUnlocked() {
		return string(data)
	}
	decrypted, err := encryptionVault.Decrypt(data)
	if err != nil {
		return string(data)
	}
	return string(decrypted)
}

func computeEventContentDigest(event *Event) (string, error) {
	if event == nil {
		return "", constants.ErrAuditEventNil
	}
	payload := eventContentDigestInput{
		ContentText:         event.ContentText,
		CommandRaw:          event.CommandRaw,
		CommandExitCode:     event.CommandExitCode,
		CommandStdout:       event.CommandStdout,
		CommandStderr:       event.CommandStderr,
		ExecutionDurationMs: event.ExecutionDurationMs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("audit chain: content digest marshal: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func computeAuditEventHash(seq int64, prevHash, eventType, sessionID, timestamp, contentDigest, transactionID string) string {
	canonical := strings.Join([]string{
		auditChainHashVersion,
		strconv.FormatInt(seq, 10),
		prevHash,
		eventType,
		sessionID,
		timestamp,
		contentDigest,
		transactionID,
	}, "|")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func loadAuditChainHeadConn(ctx context.Context, conn *sql.Conn) (auditChainHead, error) {
	var head auditChainHead
	err := conn.QueryRowContext(ctx, `
		SELECT seq, hash FROM events WHERE seq IS NOT NULL ORDER BY seq DESC LIMIT 1
	`).Scan(&head.Seq, &head.PrevHash)
	if err == nil {
		return head, nil
	}
	if err != sql.ErrNoRows {
		return auditChainHead{}, fmt.Errorf("audit chain: select head: %w", err)
	}

	var checkpointHash string
	err = conn.QueryRowContext(ctx, `
		SELECT pruned_through_hash FROM audit_chain_checkpoints ORDER BY pruned_through_seq DESC LIMIT 1
	`).Scan(&checkpointHash)
	if err == sql.ErrNoRows {
		return auditChainHead{Seq: 0, PrevHash: auditChainGenesisPrevHash}, nil
	}
	if err != nil {
		return auditChainHead{}, fmt.Errorf("audit chain: select checkpoint: %w", err)
	}
	return auditChainHead{Seq: 0, PrevHash: checkpointHash}, nil
}

// PreparedAuditEventInsert holds encrypted event fields ready for chained append.
type PreparedAuditEventInsert struct {
	Event             *Event
	TimestampStr      string
	ContentTextBytes  []byte
	StdoutBytes       []byte
	StderrBytes       []byte
	StdoutTruncated   bool
	StderrTruncated   bool
	EncryptedFlag     int
	StdoutPlaintext   string
	StderrPlaintext   string
}

// AppendPreparedAuditEvent appends one event to the audit chain under SQLite's write lock.
func AppendPreparedAuditEvent(ctx context.Context, conn *sql.Conn, prepared PreparedAuditEventInsert) (int64, int64, string, error) {
	head, err := loadAuditChainHeadConn(ctx, conn)
	if err != nil {
		return 0, 0, "", err
	}

	nextSeq := head.Seq + 1
	digestEvent := *prepared.Event
	digestEvent.CommandStdout = prepared.StdoutPlaintext
	digestEvent.CommandStderr = prepared.StderrPlaintext
	contentDigest, err := computeEventContentDigest(&digestEvent)
	if err != nil {
		return 0, 0, "", err
	}
	hash := computeAuditEventHash(
		nextSeq,
		head.PrevHash,
		string(prepared.Event.Type),
		prepared.Event.OperatorSessionID,
		prepared.TimestampStr,
		contentDigest,
		prepared.Event.TransactionID,
	)

	sessionID := sql.NullString{}
	if prepared.Event.OperatorSessionID != "" {
		sessionID = sql.NullString{String: prepared.Event.OperatorSessionID, Valid: true}
	}

	result, err := conn.ExecContext(ctx, `
		INSERT INTO events (
			operator_session_id, timestamp, type, content_text,
			command_raw, command_exit_code, command_stdout, command_stderr,
			execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted,
			seq, prev_hash, content_digest, hash, transaction_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		sessionID,
		prepared.TimestampStr,
		prepared.Event.Type,
		prepared.ContentTextBytes,
		prepared.Event.CommandRaw,
		prepared.Event.CommandExitCode,
		prepared.StdoutBytes,
		prepared.StderrBytes,
		prepared.Event.ExecutionDurationMs,
		true,
		prepared.StdoutTruncated,
		prepared.StderrTruncated,
		prepared.EncryptedFlag,
		nextSeq,
		head.PrevHash,
		contentDigest,
		hash,
		prepared.Event.TransactionID,
	)
	if err != nil {
		return 0, 0, "", fmt.Errorf("%w: %w", constants.ErrAuditStoreRecordEventFailed, err)
	}
	eventID, _ := result.LastInsertId()
	return eventID, nextSeq, hash, nil
}

func appendAuditChainCheckpoint(
	ctx context.Context,
	conn *sql.Conn,
	prunedThroughSeq int64,
	prunedThroughHash string,
	prunedCount int64,
) (string, error) {
	payload, err := json.Marshal(auditChainCheckpointPayload{
		PrunedThroughSeq:  prunedThroughSeq,
		PrunedThroughHash: prunedThroughHash,
		PrunedCount:       prunedCount,
	})
	if err != nil {
		return "", fmt.Errorf("audit chain checkpoint: marshal payload: %w", err)
	}

	now := time.Now().UTC()
	checkpointEvent := &Event{
		Timestamp:   now,
		Type:        constants.EventPlatformAuditChainCheckpointed,
		ContentText: string(payload),
	}
	prepared := PreparedAuditEventInsert{
		Event:            checkpointEvent,
		TimestampStr:     timesvc.FormatTimestamp(now),
		ContentTextBytes: payload,
		StdoutBytes:      nil,
		StderrBytes:      nil,
		EncryptedFlag:    0,
	}
	if _, _, _, err := AppendPreparedAuditEvent(ctx, conn, prepared); err != nil {
		return "", err
	}

	head, err := loadAuditChainHeadConn(ctx, conn)
	if err != nil {
		return "", err
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO audit_chain_checkpoints (
			pruned_through_seq, pruned_through_hash, pruned_count, checkpoint_hash, created_at
		) VALUES (?, ?, ?, ?, ?)
	`, prunedThroughSeq, prunedThroughHash, prunedCount, head.PrevHash, timesvc.FormatTimestamp(now)); err != nil {
		return "", fmt.Errorf("audit chain checkpoint: persist anchor: %w", err)
	}
	return head.PrevHash, nil
}

// VerifyAuditChainOnDB walks the audit event chain on an open SQLite database.
func VerifyAuditChainOnDB(ctx context.Context, db *sqliteutil.DB, fromSeq int64) error {
	if db == nil {
		return constants.ErrAuditStoreDisabled
	}
	return (&SQLAuditStore{db: db}).VerifyChain(ctx, fromSeq)
}

// VerifyChain walks the audit event chain from fromSeq and recomputes hashes.
// When fromSeq is zero, verification starts after the latest durable checkpoint.
func (ass *SQLAuditStore) VerifyChain(ctx context.Context, fromSeq int64) error {
	if ass == nil || ass.db == nil {
		return constants.ErrAuditStoreDisabled
	}

	var (
		expectedPrevHash string
		startSeq         int64
	)
	if fromSeq > 0 {
		startSeq = fromSeq
		expectedPrevHash = auditChainGenesisPrevHash
		if startSeq > 1 {
			err := ass.db.QueryRowWithRetry(`SELECT hash FROM events WHERE seq = ?`, startSeq-1).Scan(&expectedPrevHash)
			if err == sql.ErrNoRows {
				var checkpoint AuditChainCheckpoint
				err = ass.db.QueryRowWithRetry(`
					SELECT pruned_through_seq, pruned_through_hash
					FROM audit_chain_checkpoints
					WHERE pruned_through_seq < ?
					ORDER BY pruned_through_seq DESC
					LIMIT 1
				`, startSeq).Scan(&checkpoint.PrunedThroughSeq, &checkpoint.PrunedThroughHash)
				if err != nil && err != sql.ErrNoRows {
					return fmt.Errorf("audit chain verify: checkpoint lookup: %w", err)
				}
				if err == nil && checkpoint.PrunedThroughSeq == startSeq-1 {
					expectedPrevHash = checkpoint.PrunedThroughHash
				} else {
					return fmt.Errorf("audit chain verify: missing predecessor for seq %d", startSeq)
				}
			} else if err != nil {
				return fmt.Errorf("audit chain verify: predecessor lookup: %w", err)
			}
		}
	} else {
		var checkpointSeq int64
		var checkpointHash string
		err := ass.db.QueryRowWithRetry(`
			SELECT pruned_through_seq, pruned_through_hash
			FROM audit_chain_checkpoints
			ORDER BY pruned_through_seq DESC
			LIMIT 1
		`).Scan(&checkpointSeq, &checkpointHash)
		if err == sql.ErrNoRows {
			startSeq = 1
			expectedPrevHash = auditChainGenesisPrevHash
		} else if err != nil {
			return fmt.Errorf("audit chain verify: latest checkpoint: %w", err)
		} else {
			startSeq = checkpointSeq + 1
			expectedPrevHash = checkpointHash
		}
	}

	rows, err := ass.db.QueryWithRetry(`
		SELECT seq, prev_hash, hash, type, operator_session_id, timestamp, content_digest, transaction_id
		FROM events
		WHERE seq IS NOT NULL AND seq >= ?
		ORDER BY seq ASC
	`, startSeq)
	if err != nil {
		return fmt.Errorf("audit chain verify: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			seq           int64
			prevHash      string
			hash          string
			eventType     string
			sessionID     sql.NullString
			timestampStr  string
			contentDigest string
			transactionID sql.NullString
		)
		if err := rows.Scan(&seq, &prevHash, &hash, &eventType, &sessionID, &timestampStr, &contentDigest, &transactionID); err != nil {
			return fmt.Errorf("audit chain verify: scan: %w", err)
		}
		if prevHash != expectedPrevHash {
			return fmt.Errorf("audit chain verify: prev_hash mismatch at seq %d", seq)
		}
		recomputed := computeAuditEventHash(
			seq,
			prevHash,
			eventType,
			sessionID.String,
			timestampStr,
			contentDigest,
			transactionID.String,
		)
		if recomputed != hash {
			return fmt.Errorf("audit chain verify: hash mismatch at seq %d", seq)
		}
		expectedPrevHash = hash
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("audit chain verify: rows: %w", err)
	}
	return nil
}

// PruneChainedAuditEvents checkpoints and deletes chained audit events older than cutoff.
func PruneChainedAuditEvents(ctx context.Context, db *sqliteutil.DB, logger *slog.Logger, cutoff string) error {
	if db == nil {
		return nil
	}

	return db.ExecInImmediateTxWithRetry(ctx, func(conn *sql.Conn) error {
		var pruneCount int64
		err := conn.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM events
			WHERE timestamp < ?
			AND type != ?
			AND seq IS NOT NULL
		`, cutoff, string(constants.EventPlatformAuditChainCheckpointed)).Scan(&pruneCount)
		if err != nil {
			return fmt.Errorf("audit chain prune: count: %w", err)
		}
		if pruneCount == 0 {
			return nil
		}

		var (
			prunedThroughSeq  int64
			prunedThroughHash string
		)
		err = conn.QueryRowContext(ctx, `
			SELECT seq, hash FROM events
			WHERE timestamp < ?
			AND type != ?
			AND seq IS NOT NULL
			ORDER BY seq DESC
			LIMIT 1
		`, cutoff, string(constants.EventPlatformAuditChainCheckpointed)).Scan(&prunedThroughSeq, &prunedThroughHash)
		if err != nil {
			return fmt.Errorf("audit chain prune: select tail: %w", err)
		}

		if _, err := appendAuditChainCheckpoint(ctx, conn, prunedThroughSeq, prunedThroughHash, pruneCount); err != nil {
			return err
		}

		result, err := conn.ExecContext(ctx, `
			DELETE FROM events
			WHERE timestamp < ?
			AND type != ?
		`, cutoff, string(constants.EventPlatformAuditChainCheckpointed))
		if err != nil {
			return fmt.Errorf("audit chain prune: delete: %w", err)
		}
		rowsDeleted, _ := result.RowsAffected()
		if logger != nil && rowsDeleted > 0 {
			logger.Info("Pruned chained audit events",
				"rows_deleted", rowsDeleted,
				"pruned_through_seq", prunedThroughSeq,
				"pruned_through_hash", prunedThroughHash)
		}
		return nil
	})
}
