// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
)

// CommitmentRow represents a single row from the commitment_ledger table.
type CommitmentRow struct {
	Seq                           int64
	TransactionID                 string
	TransactionHash               string
	PriorCommitmentHash           string
	Hash                          string
	StateRootAtCommit             string
	L2SignatureDigest             string
	ActuatorIntentSignatureDigest string
	HumanSignatureDigest          string
	ActionType                    string
	TargetResource                string
	CommittedAt                   time.Time
	AuditorKeyID                  string
	Signature                     string
}

// CommitmentLedger is the SQLite-backed storage for commitment attestations.
// It stores raw JSON attestations with atomic append operations to guarantee
// chain integrity under concurrent writes. This type is in the storage package
// to avoid import cycles with the governance package.
type CommitmentLedger struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewCommitmentLedger creates a new commitment ledger backed by the given SQLite database.
// The database must already have the commitment_ledger table created via migrations.
func NewCommitmentLedger(db *sqliteutil.DB, logger *slog.Logger) *CommitmentLedger {
	return &CommitmentLedger{
		db:     db,
		logger: logger,
	}
}

// ListCommitments retrieves all commitments ordered by committed_at_unix_ms ASC (chain order).
func (cl *CommitmentLedger) ListCommitments() ([]*CommitmentRow, error) {
	if cl == nil || cl.db == nil {
		return nil, fmt.Errorf("commitment ledger not initialized")
	}

	query := `
	SELECT id, transaction_id, transaction_hash, prior_commitment_hash, hash,
		state_root_at_commit, l2_signature_digest, actuator_intent_signature_digest,
		human_signature_digest, action_type, target_resource,
		committed_at_unix_ms, auditor_key_id, signature
	FROM commitment_ledger
	ORDER BY committed_at_unix_ms ASC
	`

	type commitRow struct {
		row            CommitmentRow
		stateRoot      sql.NullString
		l2Digest       sql.NullString
		actuatorDigest sql.NullString
		humanDigest    sql.NullString
		actionType     sql.NullString
		targetResource sql.NullString
		auditorKeyID   sql.NullString
		signature      sql.NullString
		committedAtMs  int64
	}

	rows, err := sqliteutil.MaterializeRows(cl.db, query, nil, func(r *sql.Rows) (commitRow, error) {
		var row commitRow
		err := r.Scan(
			&row.row.Seq, &row.row.TransactionID, &row.row.TransactionHash,
			&row.row.PriorCommitmentHash, &row.row.Hash,
			&row.stateRoot, &row.l2Digest, &row.actuatorDigest, &row.humanDigest,
			&row.actionType, &row.targetResource, &row.committedAtMs,
			&row.auditorKeyID, &row.signature,
		)
		return row, err
	})
	if err != nil {
		return nil, fmt.Errorf("commitment ledger: list: %w", err)
	}

	var results []*CommitmentRow
	for _, row := range rows {
		row.row.CommittedAt = time.UnixMilli(row.committedAtMs)
		if row.stateRoot.Valid {
			row.row.StateRootAtCommit = row.stateRoot.String
		}
		if row.l2Digest.Valid {
			row.row.L2SignatureDigest = row.l2Digest.String
		}
		if row.actuatorDigest.Valid {
			row.row.ActuatorIntentSignatureDigest = row.actuatorDigest.String
		}
		if row.humanDigest.Valid {
			row.row.HumanSignatureDigest = row.humanDigest.String
		}
		if row.actionType.Valid {
			row.row.ActionType = row.actionType.String
		}
		if row.targetResource.Valid {
			row.row.TargetResource = row.targetResource.String
		}
		if row.auditorKeyID.Valid {
			row.row.AuditorKeyID = row.auditorKeyID.String
		}
		if row.signature.Valid {
			row.row.Signature = row.signature.String
		}
		results = append(results, &row.row)
	}

	return results, nil
}

// AppendCommitmentJSON atomically appends a new commitment (as JSON) to the ledger.
// This operation is transactional to ensure two concurrent attestations cannot
// both chain to the same prior_hash. The JSON must contain a "prior_commitment_hash" field.
func (cl *CommitmentLedger) AppendCommitmentJSON(attestationJSON []byte, priorHash, hash string) error {
	if cl == nil || cl.db == nil {
		return fmt.Errorf("commitment ledger not initialized")
	}
	if len(attestationJSON) == 0 {
		return fmt.Errorf("attestation JSON is empty")
	}

	// Parse JSON to extract required fields for the structured columns
	var fields struct {
		TransactionID                 string `json:"transaction_id"`
		TransactionHash               string `json:"transaction_hash"`
		StateRootAtCommit             string `json:"state_root_at_commit"`
		L2SignatureDigest             string `json:"l2_signature_digest"`
		ActuatorIntentSignatureDigest string `json:"actuator_intent_signature_digest"`
		HumanSignatureDigest          string `json:"human_signature_digest"`
		ActionType                    string `json:"action_type"`
		TargetResource                string `json:"target_resource"`
		CommittedAtUnixMs             int64  `json:"committed_at_unix_ms"`
		AuditorKeyID                  string `json:"auditor_key_id"`
		Signature                     string `json:"signature"`
	}

	if err := json.Unmarshal(attestationJSON, &fields); err != nil {
		return fmt.Errorf("failed to unmarshal attestation JSON: %w", err)
	}

	return cl.db.ExecInTxWithRetry(func(tx *sql.Tx) error {
		// Verify the prior_hash matches the current latest commitment (if any)
		var currentPriorHash string
		checkQuery := `
		SELECT hash
		FROM commitment_ledger
		ORDER BY committed_at_unix_ms DESC
		LIMIT 1
		`
		err := tx.QueryRow(checkQuery).Scan(&currentPriorHash)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to query current prior hash: %w", err)
		}

		// If ledger is not empty, verify chain integrity
		if err != sql.ErrNoRows && currentPriorHash != priorHash {
			return fmt.Errorf("prior_commitment_hash mismatch: expected %s, got %s", currentPriorHash, priorHash)
		}

		// Insert the new commitment
		insertQuery := `
		INSERT INTO commitment_ledger (
			transaction_id,
			transaction_hash,
			prior_commitment_hash,
			state_root_at_commit,
			l2_signature_digest,
			actuator_intent_signature_digest,
			human_signature_digest,
			action_type,
			target_resource,
			committed_at_unix_ms,
			auditor_key_id,
			signature,
			hash,
			attestation_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		_, err = tx.Exec(insertQuery,
			fields.TransactionID,
			fields.TransactionHash,
			priorHash,
			fields.StateRootAtCommit,
			fields.L2SignatureDigest,
			fields.ActuatorIntentSignatureDigest,
			fields.HumanSignatureDigest,
			fields.ActionType,
			fields.TargetResource,
			fields.CommittedAtUnixMs,
			fields.AuditorKeyID,
			fields.Signature,
			hash,
			attestationJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to insert commitment: %w", err)
		}

		if cl.logger != nil {
			cl.logger.Info("Commitment appended to ledger",
				"transaction_id", fields.TransactionID,
				"commitment_hash", hash,
				"prior_commitment_hash", priorHash)
		}

		return nil
	})
}
