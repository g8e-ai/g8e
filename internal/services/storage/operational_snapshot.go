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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

// OperationalEvidenceQuery selects one consistent, bounded database snapshot.
type OperationalEvidenceQuery struct {
	WindowStart time.Time
	WindowEnd   time.Time
	MaxRows     int
}

// OperationalReceiptSource preserves the stored canonical receipt body and its
// searchable selection fields without opening a writable audit store.
type OperationalReceiptSource struct {
	TransactionID string
	ExecutedAt    time.Time
	Body          []byte
}

// OperationalCommitmentSource preserves the stored commitment attestation and
// its ledger ordering metadata without rebuilding or mutating the ledger.
type OperationalCommitmentSource struct {
	Sequence      int64
	TransactionID string
	CommittedAt   time.Time
	Body          []byte
}

// OperationalAuditChainSource preserves one chained audit event and its linkage
// metadata for offline compliance verification.
type OperationalAuditChainSource struct {
	Seq               int64
	PrevHash          string
	Hash              string
	EventType         string
	OperatorSessionID string
	Timestamp         time.Time
	ContentDigest     string
	TransactionID     string
	ContentText       string
}

// OperationalEvidenceSnapshot is the bounded result of one read-only SQLite
// snapshot. A nil body records a retained row whose canonical source body is
// unavailable rather than silently dropping that row.
type OperationalEvidenceSnapshot struct {
	Receipts    []OperationalReceiptSource
	Commitments []OperationalCommitmentSource
	AuditChain  []OperationalAuditChainSource
}

// ReadOnlyOperationalEvidence reads the Gateway or Operator-local audit
// database without schema bootstrap, migration, pruning, or writes.
type ReadOnlyOperationalEvidence struct {
	db *sqliteutil.DB
}

// OpenReadOnlyOperationalEvidence opens an existing database in SQLite
// read-only mode. It does not create an absent database or runtime directory.
func OpenReadOnlyOperationalEvidence(dbPath string, logger *slog.Logger) (*ReadOnlyOperationalEvidence, error) {
	db, err := sqliteutil.OpenReadOnlyDB(sqliteutil.DefaultDBConfig(dbPath), logger)
	if err != nil {
		return nil, fmt.Errorf("operational evidence: open read-only database: %w", err)
	}
	return &ReadOnlyOperationalEvidence{db: db}, nil
}

// Close releases the read-only database connection.
func (r *ReadOnlyOperationalEvidence) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Snapshot reads receipts and commitments from one read-only transaction. The
// query has no implicit pagination; exceeding MaxRows fails rather than
// silently exporting a truncated population.
func (r *ReadOnlyOperationalEvidence) Snapshot(ctx context.Context, query OperationalEvidenceQuery) (snapshot *OperationalEvidenceSnapshot, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || r.db == nil {
		return nil, constants.ErrReportStoreUnavailable
	}
	if query.WindowStart.IsZero() || query.WindowEnd.IsZero() || query.WindowEnd.Before(query.WindowStart) || query.MaxRows <= 0 {
		return nil, fmt.Errorf("%w: operational evidence window and positive row bound are required", constants.ErrValidationFailed)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("operational evidence: begin read-only snapshot: %w", err)
	}
	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && returnErr == nil {
			snapshot = nil
			returnErr = fmt.Errorf("operational evidence: close read-only snapshot: %w", rollbackErr)
		}
	}()

	snapshot = &OperationalEvidenceSnapshot{}
	if err := readOperationalReceipts(ctx, tx, query, snapshot); err != nil {
		return nil, err
	}
	if err := readOperationalCommitments(ctx, tx, query, snapshot); err != nil {
		return nil, err
	}
	if err := readOperationalAuditChain(ctx, tx, query, snapshot); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func readOperationalReceipts(ctx context.Context, tx *sql.Tx, query OperationalEvidenceQuery, snapshot *OperationalEvidenceSnapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT transaction_id, executed_at_ms, receipt_json
		FROM receipts
		WHERE executed_at_ms >= ? AND executed_at_ms <= ?
		ORDER BY executed_at_ms ASC, transaction_id ASC
	`, query.WindowStart.UnixMilli(), query.WindowEnd.UnixMilli())
	if err != nil {
		return fmt.Errorf("operational evidence: query receipts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var receipt OperationalReceiptSource
		var executedAtMs int64
		var body sql.NullString
		if err := rows.Scan(&receipt.TransactionID, &executedAtMs, &body); err != nil {
			return fmt.Errorf("operational evidence: scan receipt: %w", err)
		}
		receipt.ExecutedAt = time.UnixMilli(executedAtMs).UTC()
		if body.Valid {
			receipt.Body = []byte(body.String)
		}
		snapshot.Receipts = append(snapshot.Receipts, receipt)
		if len(snapshot.Receipts) > query.MaxRows {
			return fmt.Errorf("%w: receipt population exceeds bound %d", constants.ErrEvidenceArtifactTooLarge, query.MaxRows)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("operational evidence: iterate receipts: %w", err)
	}
	return nil
}

func readOperationalCommitments(ctx context.Context, tx *sql.Tx, query OperationalEvidenceQuery, snapshot *OperationalEvidenceSnapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, transaction_id, committed_at_unix_ms, attestation_json
		FROM commitment_ledger
		WHERE committed_at_unix_ms >= ? AND committed_at_unix_ms <= ?
		ORDER BY id ASC
	`, query.WindowStart.UnixMilli(), query.WindowEnd.UnixMilli())
	if err != nil {
		return fmt.Errorf("operational evidence: query commitments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var commitment OperationalCommitmentSource
		var committedAtMs int64
		if err := rows.Scan(&commitment.Sequence, &commitment.TransactionID, &committedAtMs, &commitment.Body); err != nil {
			return fmt.Errorf("operational evidence: scan commitment: %w", err)
		}
		commitment.CommittedAt = time.UnixMilli(committedAtMs).UTC()
		snapshot.Commitments = append(snapshot.Commitments, commitment)
		if len(snapshot.Commitments) > query.MaxRows {
			return fmt.Errorf("%w: commitment population exceeds bound %d", constants.ErrEvidenceArtifactTooLarge, query.MaxRows)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("operational evidence: iterate commitments: %w", err)
	}
	return nil
}

func readOperationalAuditChain(ctx context.Context, tx *sql.Tx, query OperationalEvidenceQuery, snapshot *OperationalEvidenceSnapshot) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT seq, prev_hash, hash, type, operator_session_id, timestamp, content_digest, transaction_id, content_text, encrypted
		FROM events
		WHERE seq IS NOT NULL
		  AND type = ?
		  AND timestamp >= ? AND timestamp <= ?
		ORDER BY seq ASC
	`, string(constants.EventOperatorReceiptRecorded), query.WindowStart.Format(time.RFC3339Nano), query.WindowEnd.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("operational evidence: query audit chain: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry OperationalAuditChainSource
		var sessionID sql.NullString
		var timestampStr string
		var transactionID sql.NullString
		var contentText sql.NullString
		var encrypted int
		if err := rows.Scan(
			&entry.Seq,
			&entry.PrevHash,
			&entry.Hash,
			&entry.EventType,
			&sessionID,
			&timestampStr,
			&entry.ContentDigest,
			&transactionID,
			&contentText,
			&encrypted,
		); err != nil {
			return fmt.Errorf("operational evidence: scan audit chain entry: %w", err)
		}
		if sessionID.Valid {
			entry.OperatorSessionID = sessionID.String
		}
		if transactionID.Valid {
			entry.TransactionID = transactionID.String
		}
		if encrypted == 0 && contentText.Valid {
			entry.ContentText = contentText.String
		}
		parsedAt, err := timesvc.ParseTimestamp(timestampStr)
		if err != nil {
			return fmt.Errorf("operational evidence: parse audit chain timestamp: %w", err)
		}
		entry.Timestamp = parsedAt.UTC()
		snapshot.AuditChain = append(snapshot.AuditChain, entry)
		if len(snapshot.AuditChain) > query.MaxRows {
			return fmt.Errorf("%w: audit chain population exceeds bound %d", constants.ErrEvidenceArtifactTooLarge, query.MaxRows)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("operational evidence: iterate audit chain: %w", err)
	}
	return nil
}
